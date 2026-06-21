package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type assetURLSigner interface {
	PresignedGetURL(ctx context.Context, workspaceID pgtype.UUID, key string, expiry time.Duration) (string, error)
}

type RunHandler struct {
	service *production.Service
	queries *db.Queries
	storage assetURLSigner
}

func NewRunHandler(service *production.Service, queries *db.Queries, storage ...assetURLSigner) *RunHandler {
	h := &RunHandler{service: service, queries: queries}
	if len(storage) > 0 {
		h.storage = storage[0]
	}
	return h
}

type runNodeResponse struct {
	Node    *mediaNodeResponse       `json:"node,omitempty"`
	Job     generationJobResponse    `json:"job"`
	Version *artifactVersionResponse `json:"version,omitempty"`
}

type selectArtifactVersionResponse struct {
	Node    mediaNodeResponse       `json:"node"`
	Version artifactVersionResponse `json:"version"`
}

type runNodeRequest struct {
	MaxAttempts int `json:"max_attempts"`
}

type generationJobResponse struct {
	ID               string         `json:"id"`
	WorkspaceID      string         `json:"workspace_id"`
	TargetNodeID     string         `json:"target_node_id"`
	ParentJobID      string         `json:"parent_job_id,omitempty"`
	OperationType    string         `json:"operation_type"`
	Provider         string         `json:"provider"`
	ModelID          string         `json:"model_id"`
	Intent           map[string]any `json:"intent"`
	RenderedPrompt   string         `json:"rendered_prompt"`
	ProviderRequest  map[string]any `json:"provider_request"`
	ProviderResponse map[string]any `json:"provider_response"`
	Status           string         `json:"status"`
	Progress         int32          `json:"progress"`
	Attempt          int32          `json:"attempt"`
	MaxAttempts      int32          `json:"max_attempts"`
	ErrorCode        string         `json:"error_code,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	RequestedByType  string         `json:"requested_by_type"`
	RequestedByID    string         `json:"requested_by_id,omitempty"`
	CreatedAt        string         `json:"created_at"`
}

type staleReasonResponse struct {
	ID                string         `json:"id"`
	NodeID            string         `json:"node_id"`
	UpstreamNodeID    string         `json:"upstream_node_id"`
	UpstreamVersionID string         `json:"upstream_version_id,omitempty"`
	ReasonCode        string         `json:"reason_code"`
	ReasonMessage     string         `json:"reason_message"`
	Details           map[string]any `json:"details"`
}

type artifactVersionResponse struct {
	ID               string             `json:"id"`
	WorkspaceID      string             `json:"workspace_id"`
	NodeID           string             `json:"node_id"`
	JobID            string             `json:"job_id,omitempty"`
	AssetID          string             `json:"asset_id,omitempty"`
	VersionNo        int32              `json:"version_no"`
	Winner           bool               `json:"winner"`
	Output           map[string]any     `json:"output"`
	ReviewScore      *float32           `json:"review_score,omitempty"`
	InputHash        string             `json:"input_hash"`
	Status           string             `json:"status"`
	Progress         int32              `json:"progress"`
	ErrorCode        string             `json:"error_code,omitempty"`
	ErrorMessage     string             `json:"error_message,omitempty"`
	ProviderRequest  map[string]any     `json:"provider_request"`
	ProviderResponse map[string]any     `json:"provider_response"`
	Asset            *assetReadResponse `json:"asset,omitempty"`
	CreatedAt        string             `json:"created_at"`
	StartedAt        string             `json:"started_at,omitempty"`
	CompletedAt      string             `json:"completed_at,omitempty"`
}

type assetReadResponse struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Mime        string         `json:"mime"`
	StorageURL  string         `json:"storage_url,omitempty"`
	AccessURL   string         `json:"access_url,omitempty"`
	TextContent string         `json:"text_content,omitempty"`
	SizeBytes   int64          `json:"size_bytes,omitempty"`
	Metadata    map[string]any `json:"metadata"`
}

type sandboxJobResponse struct {
	ID              string         `json:"id"`
	WorkspaceID     string         `json:"workspace_id"`
	TargetNodeID    string         `json:"target_node_id,omitempty"`
	GenerationJobID string         `json:"generation_job_id,omitempty"`
	JobType         string         `json:"job_type"`
	OperationType   string         `json:"operation_type"`
	Status          string         `json:"status"`
	SandboxID       string         `json:"sandbox_id,omitempty"`
	Command         string         `json:"command"`
	Cwd             string         `json:"cwd"`
	Input           map[string]any `json:"input"`
	Output          map[string]any `json:"output"`
	ExitCode        *int32         `json:"exit_code,omitempty"`
	Stdout          string         `json:"stdout,omitempty"`
	Stderr          string         `json:"stderr,omitempty"`
	DurationMS      int32          `json:"duration_ms"`
	ErrorCode       string         `json:"error_code,omitempty"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	CreatedAt       string         `json:"created_at"`
}

type productionStateResponse struct {
	Node               mediaNodeResponse         `json:"node"`
	CurrentVersion     *artifactVersionResponse  `json:"current_version,omitempty"`
	Versions           []artifactVersionResponse `json:"versions"`
	LatestJob          *generationJobResponse    `json:"latest_job,omitempty"`
	ActiveStaleReasons []staleReasonResponse     `json:"active_stale_reasons"`
	Capability         *modelCapabilityResponse  `json:"capability,omitempty"`
	SandboxJobs        []sandboxJobResponse      `json:"sandbox_jobs"`
}

func (r runNodeRequest) runOptions() production.RunOptions {
	maxAttempts := r.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if maxAttempts > 3 {
		maxAttempts = 3
	}
	return production.RunOptions{MaxAttempts: maxAttempts}
}

func (h *RunHandler) RunNode(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	nodeID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "node not found")
		return
	}
	node, ok := nodeForAccountByQueries(ctx, h.queries, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	if _, ok := requireStudioWorkspace(ctx, h.queries, node.WorkspaceID, accountID, c); !ok {
		return
	}
	if isSourceMaterialNode(node) {
		writeError(c, consts.StatusBadRequest, "素材节点不需要运行模型。")
		return
	}

	var req runNodeRequest
	if len(c.Request.Body()) > 0 {
		if err := c.BindJSON(&req); err != nil {
			writeError(c, consts.StatusBadRequest, "invalid request")
			return
		}
	}

	result, err := h.service.SubmitNodeRun(ctx, nodeID, production.RequestedBy{Type: "user", ID: uuidToString(accountID)}, req.runOptions())
	if err != nil {
		if statusForRunError(err) == consts.StatusBadRequest {
			if result.Job.ID.Valid {
				resp := runNodeResponse{Job: toGenerationJobResponse(result.Job)}
				if result.Version.ID.Valid {
					version, versionErr := h.versionResponse(ctx, result.Version)
					if versionErr == nil {
						resp.Version = &version
					}
				}
				c.JSON(consts.StatusBadRequest, resp)
				return
			}
			writeError(c, consts.StatusBadRequest, err.Error())
			return
		}
		latest, latestErr := h.queries.LatestGenerationJobByNode(ctx, nodeID)
		if latestErr == nil {
			c.JSON(statusForRunError(err), runNodeResponse{Job: toGenerationJobResponse(latest)})
			return
		}
		writeError(c, consts.StatusInternalServerError, "failed to run node")
		return
	}
	version, err := h.versionResponse(ctx, result.Version)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load version")
		return
	}
	c.JSON(consts.StatusAccepted, runNodeResponse{Job: toGenerationJobResponse(result.Job), Version: &version})
}

func (h *RunHandler) RetryJob(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	jobID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "job not found")
		return
	}
	job, err := h.queries.GetGenerationJobByID(ctx, jobID)
	if err != nil {
		writeError(c, consts.StatusNotFound, "job not found")
		return
	}
	node, ok := nodeForAccountByQueries(ctx, h.queries, uuidToString(job.TargetNodeID), accountID, c)
	if !ok {
		return
	}
	if _, ok := requireStudioWorkspace(ctx, h.queries, node.WorkspaceID, accountID, c); !ok {
		return
	}
	result, err := h.service.RetryJob(ctx, jobID, production.RequestedBy{Type: "user", ID: uuidToString(accountID)})
	if err != nil {
		if statusForRunError(err) == consts.StatusBadRequest {
			writeError(c, consts.StatusBadRequest, err.Error())
			return
		}
		latest, latestErr := h.queries.LatestGenerationJobInChain(ctx, jobID)
		if latestErr == nil {
			c.JSON(statusForRunError(err), runNodeResponse{Job: toGenerationJobResponse(generationJobFromLatestRow(latest))})
			return
		}
		writeError(c, statusForRunError(err), err.Error())
		return
	}
	nodeResp := toMediaNodeResponse(result.Node)
	version, err := h.versionResponse(ctx, result.Version)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load version")
		return
	}
	c.JSON(consts.StatusOK, runNodeResponse{
		Node:    &nodeResp,
		Job:     toGenerationJobResponse(result.Job),
		Version: &version,
	})
}

func (h *RunHandler) ListNodeJobs(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	node, ok := nodeForAccountByQueries(ctx, h.queries, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	jobs, err := h.queries.ListGenerationJobsByNode(ctx, node.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list jobs")
		return
	}
	resp := make([]generationJobResponse, 0, len(jobs))
	for _, job := range jobs {
		resp = append(resp, toGenerationJobResponse(job))
	}
	c.JSON(consts.StatusOK, resp)
}

func (h *RunHandler) SelectNodeVersion(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	node, ok := nodeForAccountByQueries(ctx, h.queries, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	if _, ok := requireStudioWorkspace(ctx, h.queries, node.WorkspaceID, accountID, c); !ok {
		return
	}
	versionID, ok := uuidFromString(c.Param("versionID"))
	if !ok {
		writeError(c, consts.StatusNotFound, "version not found")
		return
	}
	result, err := h.service.SelectArtifactVersion(ctx, node.ID, versionID)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	version, err := h.versionResponse(ctx, result.Version)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load version asset")
		return
	}
	c.JSON(consts.StatusOK, selectArtifactVersionResponse{
		Node:    toMediaNodeResponse(result.Node),
		Version: version,
	})
}

func (h *RunHandler) ListNodeVersions(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	node, ok := nodeForAccountByQueries(ctx, h.queries, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	versions, err := h.queries.ListArtifactVersionsByNode(ctx, node.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list versions")
		return
	}
	resp := make([]artifactVersionResponse, 0, len(versions))
	for _, version := range versions {
		item, err := h.versionResponse(ctx, version)
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to load version asset")
			return
		}
		resp = append(resp, item)
	}
	c.JSON(consts.StatusOK, resp)
}

func (h *RunHandler) ListStaleReasons(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	node, ok := nodeForAccountByQueries(ctx, h.queries, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	if _, ok := requireStudioWorkspace(ctx, h.queries, node.WorkspaceID, accountID, c); !ok {
		return
	}
	reasons, err := h.queries.ListActiveStaleReasonsByNode(ctx, node.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list stale reasons")
		return
	}
	resp := make([]staleReasonResponse, 0, len(reasons))
	for _, reason := range reasons {
		resp = append(resp, toStaleReasonResponse(reason))
	}
	c.JSON(consts.StatusOK, resp)
}

func (h *RunHandler) GetJob(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	job, ok := h.jobForAccount(ctx, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	c.JSON(consts.StatusOK, toGenerationJobResponse(job))
}

func (h *RunHandler) ListJobSandboxJobs(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	job, ok := h.jobForAccount(ctx, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	rows, err := h.queries.ListSandboxJobsByGenerationJob(ctx, job.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list sandbox jobs")
		return
	}
	resp := make([]sandboxJobResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, toSandboxJobResponse(row))
	}
	c.JSON(consts.StatusOK, resp)
}

func (h *RunHandler) GetSandboxJob(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	sandboxJobID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "sandbox job not found")
		return
	}
	job, err := h.queries.GetSandboxJobByID(ctx, sandboxJobID)
	if err != nil {
		writeError(c, consts.StatusNotFound, "sandbox job not found")
		return
	}
	if _, ok := workspaceForAccount(ctx, h.queries, job.WorkspaceID, accountID, c); !ok {
		return
	}
	c.JSON(consts.StatusOK, toSandboxJobResponse(job))
}

func (h *RunHandler) GetNodeProductionState(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	node, ok := nodeForAccountByQueries(ctx, h.queries, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	if isSourceMaterialNode(node) {
		c.JSON(consts.StatusOK, sourceMaterialProductionState(node))
		return
	}
	resp := productionStateResponse{
		Node:               toMediaNodeResponse(node),
		Versions:           []artifactVersionResponse{},
		ActiveStaleReasons: []staleReasonResponse{},
		SandboxJobs:        []sandboxJobResponse{},
	}

	versions, err := h.queries.ListArtifactVersionsByNode(ctx, node.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list versions")
		return
	}
	for _, version := range versions {
		item, err := h.versionResponse(ctx, version)
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to load version asset")
			return
		}
		if node.CurrentVersionID.Valid && version.ID == node.CurrentVersionID {
			current := item
			resp.CurrentVersion = &current
		}
		resp.Versions = append(resp.Versions, item)
	}

	if latest, err := h.queries.LatestGenerationJobByNode(ctx, node.ID); err == nil {
		jobResp := toGenerationJobResponse(latest)
		resp.LatestJob = &jobResp
		rows, err := h.queries.ListSandboxJobsByGenerationJob(ctx, latest.ID)
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to list sandbox jobs")
			return
		}
		for _, row := range rows {
			resp.SandboxJobs = append(resp.SandboxJobs, toSandboxJobResponse(row))
		}
	}

	reasons, err := h.queries.ListActiveStaleReasonsByNode(ctx, node.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list stale reasons")
		return
	}
	for _, reason := range reasons {
		resp.ActiveStaleReasons = append(resp.ActiveStaleReasons, toStaleReasonResponse(reason))
	}

	if node.ModelProvider.Valid && node.ModelID.Valid {
		capability, err := h.queries.GetEnabledModelCapability(ctx, db.GetEnabledModelCapabilityParams{
			ProviderID: node.ModelProvider.String,
			ModelID:    node.ModelID.String,
		})
		if err == nil {
			capResp := toModelCapabilityResponse(capability)
			resp.Capability = &capResp
		}
	}

	c.JSON(consts.StatusOK, resp)
}

func isSourceMaterialNode(node db.MediaNode) bool {
	return node.OperationType == "upload" ||
		(node.OperationType == "manual" && node.NodeType == db.NodeTypeText) ||
		node.AssetID.Valid
}

func sourceMaterialProductionState(node db.MediaNode) productionStateResponse {
	return productionStateResponse{
		Node:               toMediaNodeResponse(node),
		Versions:           []artifactVersionResponse{},
		ActiveStaleReasons: []staleReasonResponse{},
		SandboxJobs:        []sandboxJobResponse{},
	}
}

func statusForRunError(err error) int {
	if errors.Is(err, production.ErrUnsupportedNodeType) ||
		errors.Is(err, production.ErrProviderConfig) ||
		errors.Is(err, production.ErrProviderUnavailable) ||
		errors.Is(err, production.ErrCapabilityMismatch) ||
		errors.Is(err, production.ErrRetryExhausted) {
		return consts.StatusBadRequest
	}
	return consts.StatusInternalServerError
}

func toGenerationJobResponse(job db.GenerationJob) generationJobResponse {
	return generationJobResponse{
		ID:               uuidToString(job.ID),
		WorkspaceID:      uuidToString(job.WorkspaceID),
		TargetNodeID:     uuidToString(job.TargetNodeID),
		ParentJobID:      uuidString(job.ParentJobID),
		OperationType:    job.OperationType,
		Provider:         job.Provider,
		ModelID:          job.ModelID,
		Intent:           jsonObject(job.Intent),
		RenderedPrompt:   job.RenderedPrompt,
		ProviderRequest:  jsonObject(job.ProviderRequest),
		ProviderResponse: jsonObject(job.ProviderResponse),
		Status:           string(job.Status),
		Progress:         job.Progress,
		Attempt:          job.Attempt,
		MaxAttempts:      job.MaxAttempts,
		ErrorCode:        textString(job.ErrorCode),
		ErrorMessage:     textString(job.ErrorMessage),
		RequestedByType:  job.RequestedByType,
		RequestedByID:    textString(job.RequestedByID),
		CreatedAt:        timeString(job.CreatedAt),
	}
}

func toArtifactVersionResponse(version db.ArtifactVersion, asset *db.MediaAsset, accessURL string) artifactVersionResponse {
	resp := artifactVersionResponse{
		ID:               uuidToString(version.ID),
		WorkspaceID:      uuidToString(version.WorkspaceID),
		NodeID:           uuidToString(version.NodeID),
		JobID:            uuidString(version.JobID),
		AssetID:          uuidString(version.AssetID),
		VersionNo:        version.VersionNo,
		Winner:           version.Winner,
		Output:           jsonObject(version.Output),
		InputHash:        version.InputHash,
		Status:           string(version.Status),
		Progress:         version.Progress,
		ErrorCode:        textString(version.ErrorCode),
		ErrorMessage:     textString(version.ErrorMessage),
		ProviderRequest:  jsonObject(version.ProviderRequest),
		ProviderResponse: jsonObject(version.ProviderResponse),
		CreatedAt:        timeString(version.CreatedAt),
		StartedAt:        timeString(version.StartedAt),
		CompletedAt:      timeString(version.CompletedAt),
	}
	if version.ReviewScore.Valid {
		value := version.ReviewScore.Float32
		resp.ReviewScore = &value
	}
	if asset != nil {
		resp.Asset = &assetReadResponse{
			ID:          uuidToString(asset.ID),
			Type:        string(asset.Type),
			Mime:        asset.Mime,
			StorageURL:  textString(asset.StorageUrl),
			AccessURL:   accessURL,
			TextContent: textString(asset.TextContent),
			SizeBytes:   asset.SizeBytes.Int64,
			Metadata:    jsonObject(asset.Metadata),
		}
	}
	return resp
}

func toSandboxJobResponse(job db.SandboxJob) sandboxJobResponse {
	resp := sandboxJobResponse{
		ID:              uuidToString(job.ID),
		WorkspaceID:     uuidToString(job.WorkspaceID),
		TargetNodeID:    uuidString(job.TargetNodeID),
		GenerationJobID: uuidString(job.GenerationJobID),
		JobType:         job.JobType,
		OperationType:   job.OperationType,
		Status:          string(job.Status),
		SandboxID:       textString(job.SandboxID),
		Command:         job.Command,
		Cwd:             job.Cwd,
		Input:           jsonObject(job.Input),
		Output:          jsonObject(job.Output),
		Stdout:          job.Stdout,
		Stderr:          job.Stderr,
		DurationMS:      job.DurationMs,
		ErrorCode:       textString(job.ErrorCode),
		ErrorMessage:    textString(job.ErrorMessage),
		CreatedAt:       timeString(job.CreatedAt),
	}
	if job.ExitCode.Valid {
		value := job.ExitCode.Int32
		resp.ExitCode = &value
	}
	return resp
}

func (h *RunHandler) versionResponse(ctx context.Context, version db.ArtifactVersion) (artifactVersionResponse, error) {
	if !version.AssetID.Valid {
		return toArtifactVersionResponse(version, nil, ""), nil
	}
	asset, err := h.queries.GetMediaAssetByID(ctx, version.AssetID)
	if err != nil {
		return artifactVersionResponse{}, err
	}
	accessURL := ""
	if h.storage != nil && asset.StorageUrl.Valid {
		key, err := storage.KeyFromStorageURL(asset.WorkspaceID, asset.StorageUrl.String)
		if err != nil {
			return artifactVersionResponse{}, err
		}
		accessURL, err = h.storage.PresignedGetURL(ctx, asset.WorkspaceID, key, 15*time.Minute)
		if err != nil {
			return artifactVersionResponse{}, err
		}
	}
	return toArtifactVersionResponse(version, &asset, accessURL), nil
}

func (h *RunHandler) jobForAccount(ctx context.Context, id string, accountID pgtype.UUID, c *app.RequestContext) (db.GenerationJob, bool) {
	jobID, ok := uuidFromString(id)
	if !ok {
		writeError(c, consts.StatusNotFound, "job not found")
		return db.GenerationJob{}, false
	}
	job, err := h.queries.GetGenerationJobByID(ctx, jobID)
	if err != nil {
		writeError(c, consts.StatusNotFound, "job not found")
		return db.GenerationJob{}, false
	}
	if _, ok := nodeForAccountByQueries(ctx, h.queries, uuidToString(job.TargetNodeID), accountID, c); !ok {
		return db.GenerationJob{}, false
	}
	return job, true
}

func jsonObject(raw []byte) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func textString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuidToString(value)
}

func timeString(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format(time.RFC3339Nano)
}

func generationJobFromLatestRow(row db.LatestGenerationJobInChainRow) db.GenerationJob {
	return db.GenerationJob(row)
}

func toStaleReasonResponse(reason db.NodeStaleReason) staleReasonResponse {
	return staleReasonResponse{
		ID:                uuidToString(reason.ID),
		NodeID:            uuidToString(reason.NodeID),
		UpstreamNodeID:    uuidToString(reason.UpstreamNodeID),
		UpstreamVersionID: uuidString(reason.UpstreamVersionID),
		ReasonCode:        reason.ReasonCode,
		ReasonMessage:     reason.ReasonMessage,
		Details:           jsonObject(reason.Details),
	}
}
