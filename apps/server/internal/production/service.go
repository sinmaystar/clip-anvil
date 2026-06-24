package production

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinmaystar/clip-anvil/internal/promptrefs"
	"github.com/sinmaystar/clip-anvil/internal/sandbox"
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
	importer RemoteAssetImporter
	runner   *ProductionRunner
}

type RunResult struct {
	Node    db.MediaNode
	Job     db.GenerationJob
	Version db.ArtifactVersion
}

type ArtifactSelectionResult struct {
	Node    db.MediaNode
	Version db.ArtifactVersion
}

type AssetStore interface {
	Upload(ctx context.Context, workspaceID pgtype.UUID, key string, reader io.Reader, size int64, contentType string) (storage.ObjectRef, error)
}

type RemoteAssetImporter interface {
	ImportRemoteAsset(ctx context.Context, input sandbox.RemoteAssetInput) (sandbox.SandboxJobResult, error)
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

func (s *Service) SetRunner(runner *ProductionRunner) {
	s.runner = runner
}

func (s *Service) SetRemoteAssetImporter(importer RemoteAssetImporter) {
	s.importer = importer
}

func (s *Service) RunNode(ctx context.Context, nodeID pgtype.UUID, requestedBy RequestedBy, options RunOptions) (RunResult, error) {
	return s.runNodeAttempt(ctx, nodeID, requestedBy, options)
}

func (s *Service) SubmitNodeRun(ctx context.Context, nodeID pgtype.UUID, requestedBy RequestedBy, options RunOptions) (RunResult, error) {
	node, err := s.queries.GetMediaNodeByID(ctx, nodeID)
	if err != nil {
		return RunResult{}, err
	}
	registry := s.providerRegistry()
	intent := registry.ApplyDefaults(intentForNode(node, requestedBy))

	capability, err := s.capabilityForIntent(ctx, intent)
	if err != nil {
		job, jobErr := s.createFailedJob(ctx, node, intent, err, pgtype.UUID{}, 1, 1)
		if jobErr != nil {
			return RunResult{}, jobErr
		}
		version, _ := s.queries.GetArtifactVersionByJobID(ctx, job.ID)
		return RunResult{Node: node, Job: job, Version: version}, err
	}
	inputs, err := loadInputContext(ctx, s.queries, node)
	if err != nil {
		return RunResult{}, err
	}
	intent.InputRefs = inputs.InputRefs
	intent, err = renderPromptRefs(intent, node.PromptRefs, inputs.InputRefs)
	if err != nil {
		return RunResult{}, err
	}
	maxAttempts := maxAttemptsForRun(options, capability)
	if err := ValidateCapability(intent, capability); err != nil {
		job, jobErr := s.createFailedJob(ctx, node, intent, err, pgtype.UUID{}, 1, maxAttempts)
		if jobErr != nil {
			return RunResult{}, jobErr
		}
		version, _ := s.queries.GetArtifactVersionByJobID(ctx, job.ID)
		return RunResult{Node: node, Job: job, Version: version}, err
	}
	inputHash, err := ComputeInputHash(InputHashFactsForNode(node, intent, inputs.Dependencies, inputs.ReferencePacks))
	if err != nil {
		return RunResult{}, err
	}
	job, version, err := s.createQueuedJobWithVersion(ctx, node, intent, options.ParentJobID, maxAttempts, inputHash)
	if err != nil {
		return RunResult{}, err
	}
	if s.runner != nil {
		s.runner.Enqueue(ctx, job)
	}
	return RunResult{Node: node, Job: job, Version: version}, nil
}

func (s *Service) SubmitGenerationIntent(ctx context.Context, intent GenerationIntent, options RunOptions) (RunResult, error) {
	node, err := s.queries.GetMediaNodeByID(ctx, intent.TargetNodeID)
	if err != nil {
		return RunResult{}, err
	}
	intent = s.providerRegistry().ApplyDefaults(intent)
	capability, err := s.capabilityForIntent(ctx, intent)
	if err != nil {
		job, jobErr := s.createFailedJob(ctx, node, intent, err, pgtype.UUID{}, 1, 1)
		if jobErr != nil {
			return RunResult{}, jobErr
		}
		version, _ := s.queries.GetArtifactVersionByJobID(ctx, job.ID)
		return RunResult{Node: node, Job: job, Version: version}, err
	}
	maxAttempts := maxAttemptsForRun(options, capability)
	if err := ValidateCapability(intent, capability); err != nil {
		job, jobErr := s.createFailedJob(ctx, node, intent, err, pgtype.UUID{}, 1, maxAttempts)
		if jobErr != nil {
			return RunResult{}, jobErr
		}
		version, _ := s.queries.GetArtifactVersionByJobID(ctx, job.ID)
		return RunResult{Node: node, Job: job, Version: version}, err
	}
	inputs, err := loadInputContext(ctx, s.queries, node)
	if err != nil {
		return RunResult{}, err
	}
	inputHash, err := ComputeInputHash(InputHashFactsForNode(node, intent, inputs.Dependencies, inputs.ReferencePacks))
	if err != nil {
		return RunResult{}, err
	}
	job, version, err := s.createQueuedJobWithVersion(ctx, node, intent, options.ParentJobID, maxAttempts, inputHash)
	if err != nil {
		return RunResult{}, err
	}
	if s.runner != nil {
		s.runner.Enqueue(ctx, job)
	}
	return RunResult{Node: node, Job: job, Version: version}, nil
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

func (s *Service) SelectArtifactVersion(ctx context.Context, nodeID, versionID pgtype.UUID) (ArtifactSelectionResult, error) {
	node, err := s.queries.GetMediaNodeByID(ctx, nodeID)
	if err != nil {
		return ArtifactSelectionResult{}, err
	}
	version, err := s.queries.GetArtifactVersionByID(ctx, versionID)
	if err != nil {
		return ArtifactSelectionResult{}, err
	}
	if version.NodeID != node.ID {
		return ArtifactSelectionResult{}, fmt.Errorf("%w: version does not belong to node", ErrUnsupportedNodeType)
	}
	if version.Status != db.JobStatusSucceeded || !version.AssetID.Valid {
		return ArtifactSelectionResult{}, fmt.Errorf("%w: only succeeded versions can be selected", ErrCapabilityMismatch)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ArtifactSelectionResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	if err := qtx.ClearArtifactWinnersForNode(ctx, node.ID); err != nil {
		return ArtifactSelectionResult{}, err
	}
	selected, err := qtx.MarkArtifactVersionWinner(ctx, db.MarkArtifactVersionWinnerParams{
		ID:     version.ID,
		NodeID: node.ID,
	})
	if err != nil {
		return ArtifactSelectionResult{}, err
	}
	updated, err := qtx.UpdateMediaNodeCurrentVersion(ctx, db.UpdateMediaNodeCurrentVersionParams{
		ID:               node.ID,
		CurrentVersionID: selected.ID,
	})
	if err != nil {
		return ArtifactSelectionResult{}, err
	}
	if err := qtx.ResolveActiveStaleReasonsByNode(ctx, node.ID); err != nil {
		return ArtifactSelectionResult{}, err
	}
	if err := s.propagateDownstreamStale(ctx, qtx, updated, selected); err != nil {
		return ArtifactSelectionResult{}, err
	}
	if err := s.propagateReferencePackMemberStale(ctx, qtx, updated, selected); err != nil {
		return ArtifactSelectionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ArtifactSelectionResult{}, err
	}
	return ArtifactSelectionResult{Node: updated, Version: selected}, nil
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
	inputs, err := loadInputContext(ctx, s.queries, node)
	if err != nil {
		return RunResult{}, err
	}
	intent.InputRefs = inputs.InputRefs
	intent, err = renderPromptRefs(intent, node.PromptRefs, inputs.InputRefs)
	if err != nil {
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

func (s *Service) createQueuedJobWithVersion(
	ctx context.Context,
	node db.MediaNode,
	intent GenerationIntent,
	parentJobID pgtype.UUID,
	maxAttempts int32,
	inputHash string,
) (db.GenerationJob, db.ArtifactVersion, error) {
	intentJSON, err := json.Marshal(intent)
	if err != nil {
		return db.GenerationJob{}, db.ArtifactVersion{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.GenerationJob{}, db.ArtifactVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	job, err := qtx.CreateGenerationJob(ctx, db.CreateGenerationJobParams{
		WorkspaceID:      node.WorkspaceID,
		TargetNodeID:     node.ID,
		ParentJobID:      parentJobID,
		OperationType:    intent.OperationType,
		Provider:         intent.Model.Provider,
		ModelID:          intent.Model.ModelID,
		Intent:           intentJSON,
		RenderedPrompt:   intent.EffectivePrompt(),
		ProviderRequest:  []byte("{}"),
		ProviderResponse: []byte("{}"),
		Status:           db.JobStatusQueued,
		Progress:         0,
		Attempt:          1,
		MaxAttempts:      maxAttempts,
		RetryPolicy:      retryPolicyJSON(maxAttempts),
		RequestedByType:  intent.RequestedBy.Type,
		RequestedByID:    nullableText(intent.RequestedBy.ID),
	})
	if err != nil {
		return db.GenerationJob{}, db.ArtifactVersion{}, err
	}
	versionNo, err := qtx.NextArtifactVersionNo(ctx, node.ID)
	if err != nil {
		return db.GenerationJob{}, db.ArtifactVersion{}, err
	}
	version, err := qtx.CreateArtifactVersion(ctx, db.CreateArtifactVersionParams{
		WorkspaceID:      node.WorkspaceID,
		NodeID:           node.ID,
		JobID:            job.ID,
		VersionNo:        versionNo,
		Winner:           false,
		Output:           []byte("{}"),
		InputHash:        inputHash,
		Status:           db.JobStatusQueued,
		Progress:         0,
		ProviderRequest:  []byte("{}"),
		ProviderResponse: []byte("{}"),
	})
	if err != nil {
		return db.GenerationJob{}, db.ArtifactVersion{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.GenerationJob{}, db.ArtifactVersion{}, err
	}
	return job, version, nil
}

func (s *Service) markQueuedJobRunning(ctx context.Context, job db.GenerationJob, progress int32, response map[string]any) (db.GenerationJob, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.GenerationJob{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)
	updated, err := qtx.MarkGenerationJobRunning(ctx, db.MarkGenerationJobRunningParams{
		ID:               job.ID,
		Progress:         progress,
		ProviderResponse: mapJSON(response),
	})
	if err != nil {
		return db.GenerationJob{}, err
	}
	if _, err := qtx.MarkArtifactVersionRunningByJob(ctx, db.MarkArtifactVersionRunningByJobParams{
		JobID:            job.ID,
		Progress:         progress,
		ProviderResponse: mapJSON(response),
	}); err != nil {
		return db.GenerationJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.GenerationJob{}, err
	}
	return updated, nil
}

func (s *Service) markQueuedJobProgress(ctx context.Context, job db.GenerationJob, progress int32, response map[string]any) (db.GenerationJob, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.GenerationJob{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)
	updated, err := qtx.MarkGenerationJobProgress(ctx, db.MarkGenerationJobProgressParams{
		ID:               job.ID,
		Progress:         progress,
		ProviderResponse: mapJSON(response),
	})
	if err != nil {
		return db.GenerationJob{}, err
	}
	if _, err := qtx.MarkArtifactVersionProgressByJob(ctx, db.MarkArtifactVersionProgressByJobParams{
		JobID:            job.ID,
		Progress:         progress,
		ProviderResponse: mapJSON(response),
	}); err != nil {
		return db.GenerationJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.GenerationJob{}, err
	}
	return updated, nil
}

func (s *Service) markQueuedJobFailed(ctx context.Context, job db.GenerationJob, runErr error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)
	currentJob, err := qtx.GetGenerationJobByID(ctx, job.ID)
	if err != nil {
		return err
	}
	progress := currentJob.Progress
	response := providerFailureResponse(runErr)
	errorCode := pgtype.Text{String: errorCodeForRun(runErr), Valid: true}
	errorMessage := pgtype.Text{String: runErr.Error(), Valid: true}
	if _, err := qtx.MarkGenerationJobFailed(ctx, db.MarkGenerationJobFailedParams{
		ID:               job.ID,
		Progress:         progress,
		ProviderResponse: response,
		ErrorCode:        errorCode,
		ErrorMessage:     errorMessage,
	}); err != nil {
		return err
	}
	if _, err := qtx.MarkArtifactVersionFailedByJob(ctx, db.MarkArtifactVersionFailedByJobParams{
		JobID:            job.ID,
		Progress:         progress,
		ProviderResponse: response,
		ErrorCode:        errorCode,
		ErrorMessage:     errorMessage,
	}); err != nil {
		return err
	}
	if _, err := qtx.UpdateMediaNodeStatus(ctx, db.UpdateMediaNodeStatusParams{
		ID:     currentJob.TargetNodeID,
		Status: db.NodeStatusFailed,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) persistQueuedJobSuccess(ctx context.Context, jobID pgtype.UUID, result ProviderResult) (RunResult, error) {
	job, err := s.queries.GetGenerationJobByID(ctx, jobID)
	if err != nil {
		return RunResult{}, err
	}
	node, err := s.queries.GetMediaNodeByID(ctx, job.TargetNodeID)
	if err != nil {
		return RunResult{}, err
	}
	var intent GenerationIntent
	if err := json.Unmarshal(job.Intent, &intent); err != nil {
		return RunResult{}, err
	}
	inputs, err := loadInputContext(ctx, s.queries, node)
	if err != nil {
		return RunResult{}, err
	}
	intent.InputRefs = inputs.InputRefs
	intent, err = renderPromptRefs(intent, node.PromptRefs, inputs.InputRefs)
	if err != nil {
		return RunResult{}, err
	}
	inputHash, err := ComputeInputHash(InputHashFactsForNode(node, intent, inputs.Dependencies, inputs.ReferencePacks))
	if err != nil {
		return RunResult{}, err
	}
	requestJSON := mapJSON(result.ProviderRequest)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RunResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	asset, outputJSON, err := s.createOutputAsset(ctx, qtx, node, result)
	if err != nil {
		return RunResult{}, err
	}
	responseJSON := mapJSON(result.ProviderResponse)
	if err := linkSandboxJobToGenerationJob(ctx, qtx, result.ProviderResponse, job.ID); err != nil {
		return RunResult{}, err
	}
	if err := qtx.ClearArtifactWinnersForNode(ctx, node.ID); err != nil {
		return RunResult{}, err
	}
	version, err := qtx.MarkArtifactVersionSucceededByJob(ctx, db.MarkArtifactVersionSucceededByJobParams{
		JobID:            job.ID,
		AssetID:          asset.ID,
		Output:           outputJSON,
		InputHash:        inputHash,
		ProviderRequest:  requestJSON,
		ProviderResponse: responseJSON,
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
	succeeded, err := qtx.MarkGenerationJobSucceeded(ctx, db.MarkGenerationJobSucceededParams{
		ID:               job.ID,
		RenderedPrompt:   result.RenderedPrompt,
		ProviderRequest:  requestJSON,
		ProviderResponse: responseJSON,
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RunResult{}, err
	}
	return RunResult{Node: updated, Job: succeeded, Version: version}, nil
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RunResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	asset, outputJSON, err := s.createOutputAsset(ctx, qtx, node, result)
	if err != nil {
		failedJob, jobErr := s.createFailedJob(ctx, node, intent, err, parentJobID, attempt, maxAttempts)
		if jobErr != nil {
			return RunResult{}, jobErr
		}
		return RunResult{Job: failedJob}, err
	}
	responseJSON, err := json.Marshal(result.ProviderResponse)
	if err != nil {
		return RunResult{}, err
	}

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

	versionNo, err := qtx.NextArtifactVersionNo(ctx, node.ID)
	if err != nil {
		return RunResult{}, err
	}
	if err := qtx.ClearArtifactWinnersForNode(ctx, node.ID); err != nil {
		return RunResult{}, err
	}
	version, err := qtx.CreateArtifactVersion(ctx, db.CreateArtifactVersionParams{
		WorkspaceID:      node.WorkspaceID,
		NodeID:           node.ID,
		JobID:            job.ID,
		AssetID:          asset.ID,
		VersionNo:        versionNo,
		Winner:           true,
		Output:           outputJSON,
		InputHash:        inputHash,
		Status:           db.JobStatusSucceeded,
		Progress:         100,
		ProviderRequest:  requestJSON,
		ProviderResponse: responseJSON,
		StartedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		CompletedAt:      pgtype.Timestamptz{Time: now, Valid: true},
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
		if strings.TrimSpace(result.AssetSourceURL) != "" {
			if s.importer == nil {
				return db.MediaAsset{}, nil, fmt.Errorf("%w: sandbox remote asset importer is not configured", ErrProviderConfig)
			}
			imported, err := s.importer.ImportRemoteAsset(ctx, sandbox.RemoteAssetInput{
				WorkspaceID:  node.WorkspaceID,
				TargetNodeID: node.ID,
				SourceURL:    result.AssetSourceURL,
				MimeHint:     result.AssetMIME,
			})
			if err != nil {
				return db.MediaAsset{}, nil, err
			}
			metadata := result.AssetMetadata
			if metadata == nil {
				metadata = map[string]any{}
			}
			metadata["sandbox_job_id"] = uuidToString(imported.Job.ID)
			if strings.TrimSpace(imported.MIME) != "" {
				result.AssetMIME = imported.MIME
			}
			result.AssetStorageURL = imported.Asset.StorageURL
			result.AssetThumbnailURL = imported.Thumbnail.StorageURL
			result.AssetSizeBytes = imported.Size
			if result.ProviderResponse == nil {
				result.ProviderResponse = map[string]any{}
			}
			result.ProviderResponse["sandbox_job_id"] = uuidToString(imported.Job.ID)
			result.ProviderResponse["storage_url"] = imported.Asset.StorageURL
			if strings.TrimSpace(imported.Thumbnail.StorageURL) != "" {
				result.ProviderResponse["thumbnail_url"] = imported.Thumbnail.StorageURL
			}
			result.ProviderResponse["stored_mime"] = result.AssetMIME
			result.ProviderResponse["stored_size_bytes"] = imported.Size
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
				ThumbnailUrl: pgtype.Text{
					String: result.AssetThumbnailURL,
					Valid:  strings.TrimSpace(result.AssetThumbnailURL) != "",
				},
				SizeBytes: pgtype.Int8{Int64: result.AssetSizeBytes, Valid: result.AssetSizeBytes > 0},
				Metadata:  metadataJSON,
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
			ThumbnailUrl: pgtype.Text{
				String: result.AssetThumbnailURL,
				Valid:  strings.TrimSpace(result.AssetThumbnailURL) != "",
			},
			SizeBytes: pgtype.Int8{Int64: int64(len(result.AssetContent)), Valid: true},
			Metadata:  metadataJSON,
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
		marked, err := s.markTargetStaleIfHashChanged(ctx, q, source, sourceVersion, target, reasonCode, reasonMessage, false)
		if err != nil {
			return err
		}
		if marked {
			if err := s.markStaleDescendants(ctx, q, target, sourceVersion); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) markStaleDescendants(
	ctx context.Context,
	q *db.Queries,
	source db.MediaNode,
	sourceVersion db.ArtifactVersion,
) error {
	downstream, err := q.ListDownstreamDependencyNodes(ctx, source.ID)
	if err != nil {
		return err
	}
	for _, target := range downstream {
		marked, err := s.markTargetStaleIfHashChanged(
			ctx,
			q,
			source,
			sourceVersion,
			target,
			"upstream_dependency_stale",
			"Upstream dependency is stale.",
			true,
		)
		if err != nil {
			return err
		}
		if marked {
			if err := s.markStaleDescendants(ctx, q, target, sourceVersion); err != nil {
				return err
			}
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
	force bool,
) (bool, error) {
	if !target.CurrentVersionID.Valid {
		return false, nil
	}
	currentVersion, err := q.GetArtifactVersionByID(ctx, target.CurrentVersionID)
	if err != nil {
		return false, err
	}
	intent := s.providerRegistry().ApplyDefaults(intentForNode(target, RequestedBy{Type: "system"}))
	inputs, err := loadInputContext(ctx, q, target)
	if err != nil {
		return false, err
	}
	intent.InputRefs = inputs.InputRefs
	currentHash, err := ComputeInputHash(InputHashFactsForNode(target, intent, inputs.Dependencies, inputs.ReferencePacks))
	if err != nil {
		return false, err
	}
	if currentVersion.InputHash == currentHash && !force {
		return false, nil
	}
	details, err := changedInputStaleReasonDetailsWithReason(source.ID, sourceVersion.ID, currentVersion.ID, currentVersion.InputHash, currentHash, reasonCode)
	if err != nil {
		return false, err
	}
	if _, err := q.UpdateMediaNodeStatus(ctx, db.UpdateMediaNodeStatusParams{
		ID:     target.ID,
		Status: db.NodeStatusStale,
	}); err != nil {
		return false, err
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
		return false, err
	}
	return true, nil
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

func loadInputContext(ctx context.Context, q inputContextQueries, target db.MediaNode) (inputContext, error) {
	upstream, err := q.ListUpstreamDependencyNodes(ctx, target.ID)
	if err != nil {
		return inputContext{}, err
	}
	invalid, err := invalidPromptRefs(target, upstream)
	if err != nil {
		return inputContext{}, fmt.Errorf("%w: invalid prompt refs", ErrCapabilityMismatch)
	}
	if len(invalid) > 0 {
		return inputContext{}, fmt.Errorf("%w: prompt_ref_invalid", ErrCapabilityMismatch)
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
				Kind:     InputKindReferencePack,
				Required: false,
				NodeType: string(dep.NodeType),
			})
			out.InputRefs = append(out.InputRefs, refs...)
			continue
		}
		kind := inputKindForDependency(target, dep)
		depFact, ref, err := loadNodeInputFact(ctx, q, dep, kind)
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
		memberFact, ref, err := loadNodeInputFact(ctx, q, member, InputKindReferencePackMember)
		if err != nil {
			return InputHashReferencePack{}, nil, err
		}
		fact.Members = append(fact.Members, InputHashReferencePackMember(memberFact))
		refs = append(refs, ref)
	}
	return fact, refs, nil
}

func inputKindForDependency(target db.MediaNode, dep db.MediaNode) string {
	doc, _, err := promptrefs.Normalize(target.PromptRefs)
	if err != nil {
		return InputKindImplicit
	}
	depID := uuidToString(dep.ID)
	for _, ref := range doc.Refs {
		if ref.NodeID == depID {
			return InputKindExplicit
		}
	}
	return InputKindImplicit
}

func invalidPromptRefs(target db.MediaNode, upstream []db.MediaNode) ([]promptrefs.Ref, error) {
	doc, _, err := promptrefs.Normalize(target.PromptRefs)
	if err != nil {
		return nil, err
	}
	upstreamIDs := map[string]bool{}
	for _, dep := range upstream {
		upstreamIDs[uuidToString(dep.ID)] = true
	}
	invalid := []promptrefs.Ref{}
	for _, ref := range doc.Refs {
		if !upstreamIDs[ref.NodeID] {
			invalid = append(invalid, ref)
		}
	}
	return invalid, nil
}

func renderPromptRefs(intent GenerationIntent, rawRefs []byte, inputs []InputRef) (GenerationIntent, error) {
	doc, _, err := promptrefs.Normalize(rawRefs)
	if err != nil {
		return GenerationIntent{}, err
	}
	rendered := intent.PromptTemplate
	inputByNodeID := map[string]InputRef{}
	for _, input := range inputs {
		inputByNodeID[uuidToString(input.NodeID)] = input
	}
	orderedInputs := make([]InputRef, 0, len(inputs))
	explicitInputIDs := map[string]bool{}
	imageIndex := 0
	for _, ref := range doc.Refs {
		input, ok := inputByNodeID[ref.NodeID]
		if !ok {
			continue
		}
		explicitInputIDs[ref.NodeID] = true
		orderedInputs = append(orderedInputs, input)
		replacement := ""
		if strings.TrimSpace(input.TextContent) != "" {
			replacement = promptRefTextReplacement(ref.Label, input.TextContent)
		} else if isImagePromptInput(input) {
			imageIndex++
			replacement = fmt.Sprintf("图%d", imageIndex)
		}
		if replacement == "" {
			continue
		}
		rendered = replacePromptMention(rendered, ref.Label, replacement)
	}
	for _, input := range inputs {
		if explicitInputIDs[uuidToString(input.NodeID)] {
			continue
		}
		orderedInputs = append(orderedInputs, input)
	}
	if len(orderedInputs) > 0 {
		intent.InputRefs = orderedInputs
	}
	intent.RenderedPrompt = rendered
	return intent, nil
}

func isImagePromptInput(input InputRef) bool {
	return input.NodeType == "image" && strings.TrimSpace(input.StorageURL) != ""
}

func promptRefTextReplacement(label string, text string) string {
	label = strings.TrimSpace(label)
	text = strings.TrimSpace(text)
	if label == "" {
		return text
	}
	return fmt.Sprintf("[%s]\n%s", label, text)
}

func replacePromptMention(prompt string, label string, replacement string) string {
	label = strings.TrimSpace(label)
	if label == "" || prompt == "" {
		return prompt
	}
	pattern := regexp.MustCompile(`(^|[^\pL\pN_-])@` + regexp.QuoteMeta(label) + `($|[^\pL\pN_-])`)
	escapedReplacement := strings.ReplaceAll(replacement, "$", "$$")
	return pattern.ReplaceAllString(prompt, "${1}"+escapedReplacement+"${2}")
}

func loadNodeInputFact(ctx context.Context, q inputContextQueries, node db.MediaNode, kind string) (InputHashDependency, InputRef, error) {
	fact := InputHashDependency{NodeID: uuidToString(node.ID)}
	ref := InputRef{
		NodeID:   node.ID,
		Kind:     kind,
		Required: false,
		NodeType: string(node.NodeType),
	}
	if isSourceMaterialInputNode(node) {
		return loadSourceMaterialInputFact(ctx, q, node, fact, ref)
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
		ref.TextContent = textString(asset.TextContent)
	}
	return fact, ref, nil
}

func isSourceMaterialInputNode(node db.MediaNode) bool {
	return node.OperationType == "upload" ||
		(node.OperationType == "manual" && node.NodeType == db.NodeTypeText) ||
		(node.AssetID.Valid && !node.CurrentVersionID.Valid)
}

func loadSourceMaterialInputFact(ctx context.Context, q inputContextQueries, node db.MediaNode, fact InputHashDependency, ref InputRef) (InputHashDependency, InputRef, error) {
	if node.NodeType == db.NodeTypeText && node.OperationType == "manual" {
		fact.InputHash = sourceMaterialInputHash(node, nil)
		ref.AssetType = string(db.AssetTypeText)
		ref.TextContent = node.Prompt
		ref.InputHash = fact.InputHash
		return fact, ref, nil
	}
	if !node.AssetID.Valid {
		return fact, ref, nil
	}
	asset, err := q.GetMediaAssetByID(ctx, node.AssetID)
	if err != nil {
		return InputHashDependency{}, InputRef{}, err
	}
	fact.InputHash = sourceMaterialInputHash(node, &asset)
	ref.AssetID = uuidToString(asset.ID)
	ref.AssetType = string(asset.Type)
	ref.Mime = asset.Mime
	ref.StorageURL = textString(asset.StorageUrl)
	ref.TextContent = textString(asset.TextContent)
	ref.InputHash = fact.InputHash
	return fact, ref, nil
}

func sourceMaterialInputHash(node db.MediaNode, asset *db.MediaAsset) string {
	payload := map[string]any{
		"schema_version": "m5.source_material_input.v1",
		"node_id":        uuidToString(node.ID),
		"node_type":      string(node.NodeType),
		"operation_type": node.OperationType,
		"prompt":         node.Prompt,
		"asset_id":       sourceMaterialAssetID(node),
	}
	if asset != nil {
		payload["asset_type"] = string(asset.Type)
		payload["mime"] = asset.Mime
		payload["storage_url"] = textString(asset.StorageUrl)
		payload["text_content"] = textString(asset.TextContent)
		payload["size_bytes"] = asset.SizeBytes.Int64
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(uuidToString(node.ID))
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sourceMaterialAssetID(node db.MediaNode) string {
	if !node.AssetID.Valid {
		return ""
	}
	return uuidToString(node.AssetID)
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.GenerationJob{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.queries.WithTx(tx)

	response := providerFailureResponse(runErr)
	errorCode := pgtype.Text{String: errorCodeForRun(runErr), Valid: true}
	errorMessage := pgtype.Text{String: runErr.Error(), Valid: true}
	job, err := qtx.CreateGenerationJob(ctx, db.CreateGenerationJobParams{
		WorkspaceID:      node.WorkspaceID,
		TargetNodeID:     node.ID,
		ParentJobID:      parentJobID,
		OperationType:    intent.OperationType,
		Provider:         intent.Model.Provider,
		ModelID:          intent.Model.ModelID,
		Intent:           intentJSON,
		RenderedPrompt:   intent.EffectivePrompt(),
		ProviderRequest:  []byte("{}"),
		ProviderResponse: response,
		Status:           db.JobStatusFailed,
		Progress:         0,
		Attempt:          attempt,
		MaxAttempts:      maxAttempts,
		RetryPolicy:      retryPolicyJSON(maxAttempts),
		ErrorCode:        errorCode,
		ErrorMessage:     errorMessage,
		RequestedByType:  intent.RequestedBy.Type,
		RequestedByID:    nullableText(intent.RequestedBy.ID),
		StartedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		CompletedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return job, err
	}
	versionNo, err := qtx.NextArtifactVersionNo(ctx, node.ID)
	if err != nil {
		return db.GenerationJob{}, err
	}
	if _, err := qtx.CreateArtifactVersion(ctx, db.CreateArtifactVersionParams{
		WorkspaceID:      node.WorkspaceID,
		NodeID:           node.ID,
		JobID:            job.ID,
		VersionNo:        versionNo,
		Winner:           false,
		Output:           []byte("{}"),
		InputHash:        "",
		Status:           db.JobStatusFailed,
		Progress:         0,
		ErrorCode:        errorCode,
		ErrorMessage:     errorMessage,
		ProviderRequest:  []byte("{}"),
		ProviderResponse: response,
		StartedAt:        pgtype.Timestamptz{Time: now, Valid: true},
		CompletedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return db.GenerationJob{}, err
	}
	if runErrorResponse, ok := providerRunErrorResponse(runErr); ok {
		if err := linkSandboxJobToGenerationJob(ctx, qtx, runErrorResponse, job.ID); err != nil {
			return db.GenerationJob{}, err
		}
	}
	if _, err := qtx.UpdateMediaNodeStatus(ctx, db.UpdateMediaNodeStatusParams{
		ID:     node.ID,
		Status: db.NodeStatusFailed,
	}); err != nil {
		return db.GenerationJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.GenerationJob{}, err
	}
	return job, nil
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

func mapJSON(value map[string]any) []byte {
	if value == nil {
		value = map[string]any{}
	}
	raw, _ := json.Marshal(value)
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
