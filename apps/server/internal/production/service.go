package production

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrUnsupportedNodeType = errors.New("unsupported node type")
	ErrRetryExhausted      = errors.New("retry exhausted")
)

type Service struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	registry *ProviderRegistry
	assets   AssetStore
}

type RunResult struct {
	Node    db.MediaNode
	Job     db.GenerationJob
	Version db.ArtifactVersion
}

type AssetStore interface {
	Upload(ctx context.Context, workspaceID pgtype.UUID, key string, reader io.Reader, size int64, contentType string) (storage.ObjectRef, error)
}

type staleReasonDetails struct {
	UpstreamNodeID    string `json:"upstream_node_id"`
	UpstreamVersionID string `json:"upstream_version_id"`
	TargetVersionID   string `json:"target_version_id"`
	PreviousInputHash string `json:"previous_input_hash"`
	CurrentInputHash  string `json:"current_input_hash"`
	Reason            string `json:"reason"`
}

type inputContext struct {
	Dependencies   []InputHashDependency
	ReferencePacks []InputHashReferencePack
	InputRefs      []InputRef
}

type inputContextQueries interface {
	ListUpstreamDependencyNodes(ctx context.Context, nodeID pgtype.UUID) ([]db.MediaNode, error)
	ListReferencePackItemNodes(ctx context.Context, packNodeID pgtype.UUID) ([]db.MediaNode, error)
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
	GetMediaAssetByID(ctx context.Context, id pgtype.UUID) (db.MediaAsset, error)
}

func NewService(pool *pgxpool.Pool, queries *db.Queries, registry *ProviderRegistry, assets ...AssetStore) *Service {
	service := &Service{pool: pool, queries: queries, registry: registry}
	if len(assets) > 0 {
		service.assets = assets[0]
	}
	return service
}

func (s *Service) RunNode(ctx context.Context, nodeID pgtype.UUID, requestedBy RequestedBy, options RunOptions) (RunResult, error) {
	return s.runNodeAttempt(ctx, nodeID, requestedBy, options)
}

func (s *Service) RetryJob(ctx context.Context, jobID pgtype.UUID, requestedBy RequestedBy) (RunResult, error) {
	job, err := s.queries.GetGenerationJobByID(ctx, jobID)
	if err != nil {
		return RunResult{}, err
	}
	if job.Status != db.JobStatusFailed {
		return RunResult{}, fmt.Errorf("%w: only failed jobs can be retried", ErrCapabilityMismatch)
	}
	latest, err := s.queries.LatestGenerationJobInChain(ctx, job.ID)
	if err != nil {
		return RunResult{}, err
	}
	if latest.Attempt >= latest.MaxAttempts {
		return RunResult{}, ErrRetryExhausted
	}
	return s.runNodeAttempt(ctx, latest.TargetNodeID, requestedBy, RunOptions{
		MaxAttempts: int(latest.MaxAttempts),
		ParentJobID: latest.ID,
		Attempt:     int(latest.Attempt + 1),
	})
}

func (s *Service) MarkDownstreamStale(ctx context.Context, sourceNodeID pgtype.UUID, reasonCode string, reasonMessage string) error {
	source, err := s.queries.GetMediaNodeByID(ctx, sourceNodeID)
	if err != nil {
		return err
	}
	return s.markDownstreamStaleForSource(ctx, s.queries, source, db.ArtifactVersion{}, reasonCode, reasonMessage)
}

func (s *Service) runNodeAttempt(ctx context.Context, nodeID pgtype.UUID, requestedBy RequestedBy, options RunOptions) (RunResult, error) {
	node, err := s.queries.GetMediaNodeByID(ctx, nodeID)
	if err != nil {
		return RunResult{}, err
	}
	registry := s.providerRegistry()
	intent := registry.ApplyDefaults(intentForNode(node, requestedBy))

	capability, err := s.capabilityForIntent(ctx, intent)
	if err != nil {
		if _, jobErr := s.createFailedJob(ctx, node, intent, err, pgtype.UUID{}, 1, 1); jobErr != nil {
			return RunResult{}, jobErr
		}
		return RunResult{}, err
	}
	maxAttempts := maxAttemptsForRun(options, capability)
	if err := ValidateCapability(intent, capability); err != nil {
		if _, jobErr := s.createFailedJob(ctx, node, intent, err, pgtype.UUID{}, 1, maxAttempts); jobErr != nil {
			return RunResult{}, jobErr
		}
		return RunResult{}, err
	}

	provider, err := registry.Resolve(intent)
	if err != nil {
		if _, jobErr := s.createFailedJob(ctx, node, intent, err, pgtype.UUID{}, 1, maxAttempts); jobErr != nil {
			return RunResult{}, jobErr
		}
		return RunResult{}, err
	}
	inputs, err := loadInputContext(ctx, s.queries, node.ID)
	if err != nil {
		return RunResult{}, err
	}
	intent.InputRefs = inputs.InputRefs
	inputHash, err := ComputeInputHash(InputHashFactsForNode(node, intent, inputs.Dependencies, inputs.ReferencePacks))
	if err != nil {
		return RunResult{}, err
	}

	startAttempt := int32(1)
	if options.Attempt > 0 {
		startAttempt = int32(options.Attempt)
	}
	parentJobID := options.ParentJobID
	if startAttempt > maxAttempts {
		return RunResult{}, ErrRetryExhausted
	}
	var lastErr error
	for attempt := startAttempt; attempt <= maxAttempts; attempt++ {
		result, err := provider.Run(ctx, intent)
		if err == nil {
			return s.persistSuccessfulRun(ctx, node, intent, result, parentJobID, attempt, maxAttempts, inputHash)
		}
		failedJob, jobErr := s.createFailedJob(ctx, node, intent, err, parentJobID, attempt, maxAttempts)
		if jobErr != nil {
			return RunResult{}, jobErr
		}
		parentJobID = failedJob.ID
		lastErr = err
	}
	return RunResult{}, lastErr
}

func (s *Service) persistSuccessfulRun(
	ctx context.Context,
	node db.MediaNode,
	intent GenerationIntent,
	result ProviderResult,
	parentJobID pgtype.UUID,
	attempt int32,
	maxAttempts int32,
	inputHash string,
) (RunResult, error) {
	intentJSON, err := json.Marshal(intent)
	if err != nil {
		return RunResult{}, err
	}
	requestJSON, err := json.Marshal(result.ProviderRequest)
	if err != nil {
		return RunResult{}, err
	}
	responseJSON, err := json.Marshal(result.ProviderResponse)
	if err != nil {
		return RunResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RunResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	now := time.Now()
	job, err := qtx.CreateGenerationJob(ctx, db.CreateGenerationJobParams{
		WorkspaceID:      node.WorkspaceID,
		TargetNodeID:     node.ID,
		ParentJobID:      parentJobID,
		OperationType:    intent.OperationType,
		Provider:         intent.Model.Provider,
		ModelID:          intent.Model.ModelID,
		Intent:           intentJSON,
		RenderedPrompt:   result.RenderedPrompt,
		ProviderRequest:  requestJSON,
		ProviderResponse: responseJSON,
		Status:           db.JobStatusSucceeded,
		Progress:         100,
		Attempt:          attempt,
		MaxAttempts:      maxAttempts,
		RetryPolicy:      retryPolicyJSON(maxAttempts),
		RequestedByType:  intent.RequestedBy.Type,
		RequestedByID:    nullableText(intent.RequestedBy.ID),
		StartedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		CompletedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := linkSandboxJobToGenerationJob(ctx, qtx, result.ProviderResponse, job.ID); err != nil {
		return RunResult{}, err
	}

	asset, outputJSON, err := s.createOutputAsset(ctx, qtx, node, result)
	if err != nil {
		failedJob, jobErr := s.createFailedJob(ctx, node, intent, err, parentJobID, attempt, maxAttempts)
		if jobErr != nil {
			return RunResult{}, jobErr
		}
		return RunResult{Job: failedJob}, err
	}

	versionNo, err := qtx.NextArtifactVersionNo(ctx, node.ID)
	if err != nil {
		return RunResult{}, err
	}
	if err := qtx.ClearArtifactWinnersForNode(ctx, node.ID); err != nil {
		return RunResult{}, err
	}
	version, err := qtx.CreateArtifactVersion(ctx, db.CreateArtifactVersionParams{
		WorkspaceID: node.WorkspaceID,
		NodeID:      node.ID,
		JobID:       job.ID,
		AssetID:     asset.ID,
		VersionNo:   versionNo,
		Winner:      true,
		Output:      outputJSON,
		InputHash:   inputHash,
	})
	if err != nil {
		return RunResult{}, err
	}
	updated, err := qtx.UpdateMediaNodeCurrentVersion(ctx, db.UpdateMediaNodeCurrentVersionParams{
		ID:               node.ID,
		CurrentVersionID: version.ID,
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := qtx.ResolveActiveStaleReasonsByNode(ctx, node.ID); err != nil {
		return RunResult{}, err
	}
	if err := s.propagateDownstreamStale(ctx, qtx, updated, version); err != nil {
		return RunResult{}, err
	}
	if err := s.propagateReferencePackMemberStale(ctx, qtx, updated, version); err != nil {
		return RunResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RunResult{}, err
	}
	return RunResult{Node: updated, Job: job, Version: version}, nil
}

func (s *Service) createOutputAsset(ctx context.Context, qtx *db.Queries, node db.MediaNode, result ProviderResult) (db.MediaAsset, []byte, error) {
	switch node.NodeType {
	case db.NodeTypeText:
		outputJSON, err := json.Marshal(map[string]any{"text_preview": result.TextContent})
		if err != nil {
			return db.MediaAsset{}, nil, err
		}
		asset, err := qtx.CreateTextMediaAsset(ctx, db.CreateTextMediaAssetParams{
			WorkspaceID: node.WorkspaceID,
			TextContent: pgtype.Text{String: result.TextContent, Valid: true},
			SizeBytes:   pgtype.Int8{Int64: int64(len([]byte(result.TextContent))), Valid: true},
			Metadata:    []byte("{}"),
		})
		return asset, outputJSON, err
	case db.NodeTypeImage, db.NodeTypeVideo, db.NodeTypeAudio:
		if strings.TrimSpace(result.AssetMIME) == "" {
			return db.MediaAsset{}, nil, fmt.Errorf("%w: provider returned no asset mime", ErrProviderExecution)
		}
		if strings.TrimSpace(result.AssetStorageURL) != "" {
			metadata := result.AssetMetadata
			if metadata == nil {
				metadata = map[string]any{}
			}
			metadataJSON, err := json.Marshal(metadata)
			if err != nil {
				return db.MediaAsset{}, nil, err
			}
			asset, err := qtx.CreateMediaAsset(ctx, db.CreateMediaAssetParams{
				WorkspaceID: node.WorkspaceID,
				Type:        assetTypeForNodeType(node.NodeType),
				Mime:        result.AssetMIME,
				StorageUrl:  pgtype.Text{String: result.AssetStorageURL, Valid: true},
				SizeBytes:   pgtype.Int8{Int64: result.AssetSizeBytes, Valid: result.AssetSizeBytes > 0},
				Metadata:    metadataJSON,
			})
			if err != nil {
				return db.MediaAsset{}, nil, err
			}
			outputJSON, err := json.Marshal(map[string]any{"asset_id": uuidToString(asset.ID), "mime": result.AssetMIME})
			return asset, outputJSON, err
		}
		if len(result.AssetContent) == 0 {
			return db.MediaAsset{}, nil, fmt.Errorf("%w: provider returned no asset content", ErrProviderExecution)
		}
		if s.assets == nil {
			return db.MediaAsset{}, nil, fmt.Errorf("%w: asset store is not configured", ErrProviderConfig)
		}
		ext := extensionForMIME(result.AssetMIME)
		key := fmt.Sprintf("production/%s/%d%s", uuidToString(node.ID), time.Now().UnixNano(), ext)
		object, err := s.assets.Upload(ctx, node.WorkspaceID, key, bytes.NewReader(result.AssetContent), int64(len(result.AssetContent)), result.AssetMIME)
		if err != nil {
			return db.MediaAsset{}, nil, err
		}
		metadata := result.AssetMetadata
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return db.MediaAsset{}, nil, err
		}
		asset, err := qtx.CreateMediaAsset(ctx, db.CreateMediaAssetParams{
			WorkspaceID: node.WorkspaceID,
			Type:        assetTypeForNodeType(node.NodeType),
			Mime:        result.AssetMIME,
			StorageUrl:  pgtype.Text{String: object.StorageURL, Valid: true},
			SizeBytes:   pgtype.Int8{Int64: int64(len(result.AssetContent)), Valid: true},
			Metadata:    metadataJSON,
		})
		if err != nil {
			return db.MediaAsset{}, nil, err
		}
		outputJSON, err := json.Marshal(map[string]any{"asset_id": uuidToString(asset.ID), "mime": result.AssetMIME})
		return asset, outputJSON, err
	default:
		return db.MediaAsset{}, nil, fmt.Errorf("%w: successful %s output persistence is not implemented", ErrProviderUnavailable, node.NodeType)
	}
}

func assetTypeForNodeType(nodeType db.NodeType) db.AssetType {
	switch nodeType {
	case db.NodeTypeText:
		return db.AssetTypeText
	case db.NodeTypeImage:
		return db.AssetTypeImage
	case db.NodeTypeVideo:
		return db.AssetTypeVideo
	case db.NodeTypeAudio:
		return db.AssetTypeAudio
	default:
		return db.AssetType("")
	}
}

func extensionForMIME(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "video/mp4":
		return ".mp4"
	case "audio/mpeg":
		return ".mp3"
	default:
		return ".bin"
	}
}

func (s *Service) providerRegistry() *ProviderRegistry {
	if s.registry != nil {
		return s.registry
	}
	return NewProviderRegistry(ProviderConfig{})
}

func (s *Service) capabilityForIntent(ctx context.Context, intent GenerationIntent) (Capability, error) {
	row, err := s.queries.GetEnabledModelCapability(ctx, db.GetEnabledModelCapabilityParams{
		ProviderID: intent.Model.Provider,
		ModelID:    intent.Model.ModelID,
	})
	if err != nil {
		return Capability{}, fmt.Errorf("%w: %s/%s", ErrCapabilityMismatch, intent.Model.Provider, intent.Model.ModelID)
	}
	return CapabilityFromRow(row)
}

func (s *Service) propagateDownstreamStale(
	ctx context.Context,
	qtx *db.Queries,
	upstream db.MediaNode,
	upstreamVersion db.ArtifactVersion,
) error {
	return s.markDownstreamStaleForSource(ctx, qtx, upstream, upstreamVersion, "upstream_current_version_changed", "Upstream dependency current version changed.")
}

func (s *Service) markDownstreamStaleForSource(
	ctx context.Context,
	q *db.Queries,
	source db.MediaNode,
	sourceVersion db.ArtifactVersion,
	reasonCode string,
	reasonMessage string,
) error {
	downstream, err := q.ListDownstreamDependencyNodes(ctx, source.ID)
	if err != nil {
		return err
	}
	for _, target := range downstream {
		if err := s.markTargetStaleIfHashChanged(ctx, q, source, sourceVersion, target, reasonCode, reasonMessage); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) markTargetStaleIfHashChanged(
	ctx context.Context,
	q *db.Queries,
	source db.MediaNode,
	sourceVersion db.ArtifactVersion,
	target db.MediaNode,
	reasonCode string,
	reasonMessage string,
) error {
	if !target.CurrentVersionID.Valid {
		return nil
	}
	currentVersion, err := q.GetArtifactVersionByID(ctx, target.CurrentVersionID)
	if err != nil {
		return err
	}
	intent := s.providerRegistry().ApplyDefaults(intentForNode(target, RequestedBy{Type: "system"}))
	inputs, err := loadInputContext(ctx, q, target.ID)
	if err != nil {
		return err
	}
	intent.InputRefs = inputs.InputRefs
	currentHash, err := ComputeInputHash(InputHashFactsForNode(target, intent, inputs.Dependencies, inputs.ReferencePacks))
	if err != nil {
		return err
	}
	if currentVersion.InputHash == currentHash {
		return nil
	}
	details, err := changedInputStaleReasonDetailsWithReason(source.ID, sourceVersion.ID, currentVersion.ID, currentVersion.InputHash, currentHash, reasonCode)
	if err != nil {
		return err
	}
	if _, err := q.UpdateMediaNodeStatus(ctx, db.UpdateMediaNodeStatusParams{
		ID:     target.ID,
		Status: db.NodeStatusStale,
	}); err != nil {
		return err
	}
	if _, err := q.UpsertNodeStaleReason(ctx, db.UpsertNodeStaleReasonParams{
		WorkspaceID:       target.WorkspaceID,
		NodeID:            target.ID,
		UpstreamNodeID:    source.ID,
		UpstreamVersionID: sourceVersion.ID,
		ReasonCode:        reasonCode,
		ReasonMessage:     reasonMessage,
		Details:           details,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) propagateReferencePackMemberStale(
	ctx context.Context,
	qtx *db.Queries,
	member db.MediaNode,
	memberVersion db.ArtifactVersion,
) error {
	packs, err := qtx.ListReferencePacksByMember(ctx, member.ID)
	if err != nil {
		return err
	}
	for _, pack := range packs {
		if err := s.markDownstreamStaleForSource(
			ctx,
			qtx,
			pack,
			memberVersion,
			"reference_pack_member_version_changed",
			"Reference pack member current version changed.",
		); err != nil {
			return err
		}
	}
	return nil
}

func changedInputStaleReasonDetails(
	upstreamNodeID pgtype.UUID,
	upstreamVersionID pgtype.UUID,
	targetVersionID pgtype.UUID,
	previousInputHash string,
	currentInputHash string,
) ([]byte, error) {
	return changedInputStaleReasonDetailsWithReason(
		upstreamNodeID,
		upstreamVersionID,
		targetVersionID,
		previousInputHash,
		currentInputHash,
		"upstream_current_version_changed",
	)
}

func changedInputStaleReasonDetailsWithReason(
	upstreamNodeID pgtype.UUID,
	upstreamVersionID pgtype.UUID,
	targetVersionID pgtype.UUID,
	previousInputHash string,
	currentInputHash string,
	reason string,
) ([]byte, error) {
	return json.Marshal(staleReasonDetails{
		UpstreamNodeID:    uuidToString(upstreamNodeID),
		UpstreamVersionID: uuidToString(upstreamVersionID),
		TargetVersionID:   uuidToString(targetVersionID),
		PreviousInputHash: previousInputHash,
		CurrentInputHash:  currentInputHash,
		Reason:            reason,
	})
}

func loadInputContext(ctx context.Context, q inputContextQueries, nodeID pgtype.UUID) (inputContext, error) {
	upstream, err := q.ListUpstreamDependencyNodes(ctx, nodeID)
	if err != nil {
		return inputContext{}, err
	}
	out := inputContext{
		Dependencies:   []InputHashDependency{},
		ReferencePacks: []InputHashReferencePack{},
		InputRefs:      []InputRef{},
	}
	for _, dep := range upstream {
		if dep.NodeType == db.NodeTypeReferencePack {
			pack, refs, err := loadReferencePackFacts(ctx, q, dep)
			if err != nil {
				return inputContext{}, err
			}
			out.ReferencePacks = append(out.ReferencePacks, pack)
			out.InputRefs = append(out.InputRefs, InputRef{
				NodeID:   dep.ID,
				Kind:     "reference_pack",
				Required: false,
				NodeType: string(dep.NodeType),
			})
			out.InputRefs = append(out.InputRefs, refs...)
			continue
		}
		depFact, ref, err := loadNodeInputFact(ctx, q, dep, "dependency")
		if err != nil {
			return inputContext{}, err
		}
		out.Dependencies = append(out.Dependencies, depFact)
		out.InputRefs = append(out.InputRefs, ref)
	}
	return out, nil
}

func loadReferencePackFacts(ctx context.Context, q inputContextQueries, pack db.MediaNode) (InputHashReferencePack, []InputRef, error) {
	members, err := q.ListReferencePackItemNodes(ctx, pack.ID)
	if err != nil {
		return InputHashReferencePack{}, nil, err
	}
	fact := InputHashReferencePack{PackID: uuidToString(pack.ID), Members: []InputHashReferencePackMember{}}
	refs := []InputRef{}
	for _, member := range members {
		memberFact, ref, err := loadNodeInputFact(ctx, q, member, "reference_pack_member")
		if err != nil {
			return InputHashReferencePack{}, nil, err
		}
		fact.Members = append(fact.Members, InputHashReferencePackMember(memberFact))
		refs = append(refs, ref)
	}
	return fact, refs, nil
}

func loadNodeInputFact(ctx context.Context, q inputContextQueries, node db.MediaNode, kind string) (InputHashDependency, InputRef, error) {
	fact := InputHashDependency{NodeID: uuidToString(node.ID)}
	ref := InputRef{
		NodeID:   node.ID,
		Kind:     kind,
		Required: false,
		NodeType: string(node.NodeType),
	}
	if !node.CurrentVersionID.Valid {
		return fact, ref, nil
	}
	version, err := q.GetArtifactVersionByID(ctx, node.CurrentVersionID)
	if err != nil {
		return InputHashDependency{}, InputRef{}, err
	}
	fact.CurrentVersionID = uuidToString(version.ID)
	fact.InputHash = version.InputHash
	ref.CurrentVersionID = uuidToString(version.ID)
	ref.InputHash = version.InputHash
	if version.AssetID.Valid {
		asset, err := q.GetMediaAssetByID(ctx, version.AssetID)
		if err != nil {
			return InputHashDependency{}, InputRef{}, err
		}
		ref.AssetID = uuidToString(asset.ID)
		ref.AssetType = string(asset.Type)
		ref.Mime = asset.Mime
		ref.StorageURL = textString(asset.StorageUrl)
	}
	return fact, ref, nil
}

func (s *Service) createFailedJob(
	ctx context.Context,
	node db.MediaNode,
	intent GenerationIntent,
	runErr error,
	parentJobID pgtype.UUID,
	attempt int32,
	maxAttempts int32,
) (db.GenerationJob, error) {
	intentJSON, err := json.Marshal(intent)
	if err != nil {
		return db.GenerationJob{}, err
	}
	now := time.Now()
	job, err := s.queries.CreateGenerationJob(ctx, db.CreateGenerationJobParams{
		WorkspaceID:      node.WorkspaceID,
		TargetNodeID:     node.ID,
		ParentJobID:      parentJobID,
		OperationType:    intent.OperationType,
		Provider:         intent.Model.Provider,
		ModelID:          intent.Model.ModelID,
		Intent:           intentJSON,
		RenderedPrompt:   intent.PromptTemplate,
		ProviderRequest:  []byte("{}"),
		ProviderResponse: providerFailureResponse(runErr),
		Status:           db.JobStatusFailed,
		Progress:         0,
		Attempt:          attempt,
		MaxAttempts:      maxAttempts,
		RetryPolicy:      retryPolicyJSON(maxAttempts),
		ErrorCode:        pgtype.Text{String: errorCodeForRun(runErr), Valid: true},
		ErrorMessage:     pgtype.Text{String: runErr.Error(), Valid: true},
		RequestedByType:  intent.RequestedBy.Type,
		RequestedByID:    nullableText(intent.RequestedBy.ID),
		StartedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		CompletedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return job, err
	}
	if runErrorResponse, ok := providerRunErrorResponse(runErr); ok {
		if err := linkSandboxJobToGenerationJob(ctx, s.queries, runErrorResponse, job.ID); err != nil {
			return db.GenerationJob{}, err
		}
	}
	return job, err
}

func errorCodeForRun(err error) string {
	switch {
	case errors.Is(err, ErrUnsupportedNodeType):
		return "unsupported_node_type"
	case errors.Is(err, ErrProviderConfig):
		return "provider_config_error"
	case errors.Is(err, ErrCapabilityMismatch):
		return "capability_mismatch"
	case errors.Is(err, ErrProviderUnavailable):
		return "provider_unavailable"
	case errors.Is(err, ErrRetryExhausted):
		return "retry_exhausted"
	default:
		return "provider_error"
	}
}

func retryPolicyJSON(maxAttempts int32) []byte {
	raw, _ := json.Marshal(map[string]any{"max_attempts": maxAttempts})
	return raw
}

func providerFailureResponse(err error) []byte {
	response := map[string]any{
		"error": err.Error(),
		"code":  errorCodeForRun(err),
	}
	if runErrorResponse, ok := providerRunErrorResponse(err); ok {
		for key, value := range runErrorResponse {
			response[key] = value
		}
	}
	raw, _ := json.Marshal(response)
	return raw
}

type sandboxJobLinker interface {
	LinkSandboxJobGenerationJob(ctx context.Context, arg db.LinkSandboxJobGenerationJobParams) (db.SandboxJob, error)
}

func linkSandboxJobToGenerationJob(ctx context.Context, q sandboxJobLinker, response map[string]any, generationJobID pgtype.UUID) error {
	rawID, ok := response["sandbox_job_id"]
	if !ok {
		return nil
	}
	idText, ok := rawID.(string)
	if !ok || strings.TrimSpace(idText) == "" {
		return nil
	}
	var sandboxJobID pgtype.UUID
	if err := sandboxJobID.Scan(strings.TrimSpace(idText)); err != nil {
		return nil
	}
	if !sandboxJobID.Valid {
		return nil
	}
	_, err := q.LinkSandboxJobGenerationJob(ctx, db.LinkSandboxJobGenerationJobParams{
		ID:              sandboxJobID,
		GenerationJobID: generationJobID,
	})
	return err
}

func providerRunErrorResponse(err error) (map[string]any, bool) {
	var runErr ProviderRunError
	if !errors.As(err, &runErr) || len(runErr.Response) == 0 {
		return nil, false
	}
	return runErr.Response, true
}

func intentForNode(node db.MediaNode, requestedBy RequestedBy) GenerationIntent {
	prompt := node.PromptTemplate
	if prompt == "" {
		prompt = node.Prompt
	}
	operation := node.OperationType
	if operation == "" || operation == "manual" {
		operation = "text_generation"
	}
	provider := ""
	if node.ModelProvider.Valid {
		provider = node.ModelProvider.String
	}
	modelID := ""
	if node.ModelID.Valid {
		modelID = node.ModelID.String
	}
	params := map[string]any{}
	if len(node.ModelParams) > 0 {
		_ = json.Unmarshal(node.ModelParams, &params)
	}
	return GenerationIntent{
		WorkspaceID:    node.WorkspaceID,
		TargetNodeID:   node.ID,
		OutputType:     string(node.NodeType),
		OperationType:  operation,
		PromptTemplate: prompt,
		InputRefs:      []InputRef{},
		Model:          ModelSpec{Provider: provider, ModelID: modelID},
		Params:         params,
		RequestedBy:    requestedBy,
	}
}

func nullableText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func textString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func uuidToString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", value.Bytes[0:4], value.Bytes[4:6], value.Bytes[6:8], value.Bytes[8:10], value.Bytes[10:16])
}
