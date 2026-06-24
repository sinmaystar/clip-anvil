# M4.6 Production Read API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the backend read surface M5 Studio needs to inspect model capabilities, node versions, production state, generation jobs, sandbox jobs, and output assets without implementing the M5 UI yet.

**Architecture:** Keep M4 as a backend production foundation. Add small, authenticated REST endpoints backed by existing `generation_job`, `artifact_version`, `media_asset`, `model_capability`, `node_stale_reason`, and `sandbox_job` records. Responses should be UI-ready but not UI-coupled: stable IDs, status, current flags, asset previews/access URLs, failure summaries, and sandbox trace links.

**Tech Stack:** Go 1.26, Hertz handlers, pgx/sqlc, existing MinIO storage service, existing production/sandbox tables, shell smoke scripts.

---

## Scope

M4.6 includes:

- Model capability listing for Studio model/operation selectors.
- Node artifact version listing with current winner and asset access data.
- Node production state summary for M5 property panel hydration.
- Generation job detail and sandbox job linkage.
- Sandbox job read endpoints for debugging internal media and future Agent shell execution.
- M4.6 smoke script that verifies the read surface after real production runs.

M4.6 does not include:

- M5 property panel UI.
- Prompt `@` editor UI.
- Manual model selector frontend.
- Agent runtime, conversation, PSS, shot, or HITL.
- Editing historical versions or choosing a non-current winner.

## File Structure

- Modify: `apps/server/sqlc/queries/production.sql`
  - Add current/list query helpers for version read API if existing queries are not enough.
- Modify: `apps/server/sqlc/queries/sandbox_job.sql`
  - Add `ListSandboxJobsByGenerationJob`.
- Modify: `apps/server/internal/api/run_handler.go`
  - Add generation job detail, sandbox job list, node version list, and production state handlers.
- Create: `apps/server/internal/api/model_handler.go`
  - Add model capability listing handler.
- Create: `apps/server/internal/api/model_handler_test.go`
  - Unit tests for capability response node.
- Modify: `apps/server/internal/api/run_handler_test.go`
  - Unit tests for version/job/sandbox response mapping.
- Modify: `apps/server/cmd/server/main.go`
  - Register new routes and pass storage service into run handler.
- Create: `scripts/smoke-m4-6.sh`
  - End-to-end read API smoke.
- Modify: `docs/milestones/m4-shared-production-foundation.md`
  - Add M4.6 phase, acceptance, smoke evidence slot.

## API Node

Add these authenticated endpoints:

```http
GET /api/model-capabilities
GET /api/nodes/:id/versions
GET /api/nodes/:id/production-state
GET /api/jobs/:id
GET /api/jobs/:id/sandbox-jobs
GET /api/sandbox-jobs/:id
```

All endpoints must enforce workspace ownership through the existing account/workspace checks. Studio and Agent workspaces may both read production state; ordinary write restrictions for Agent workspaces remain unchanged.

## Response Contracts

### Model Capability

```json
{
  "provider_id": "mock",
  "model_id": "mock-text",
  "display_name": "Mock Text",
  "enabled": true,
  "output_types": ["text"],
  "supported_operations": ["text_generation"],
  "supported_input_node_types": ["text"],
  "limits": {"max_prompt_chars": 8000, "max_attempts": 3},
  "pricing": {"tier": "mock"},
  "defaults": {"temperature": 0.2}
}
```

### Artifact Version

```json
{
  "id": "version-id",
  "node_id": "node-id",
  "job_id": "job-id",
  "asset_id": "asset-id",
  "version_no": 2,
  "winner": true,
  "input_hash": "sha256:...",
  "output": {},
  "asset": {
    "id": "asset-id",
    "type": "image",
    "mime": "image/png",
    "storage_url": "workspace-id/path.png",
    "access_url": "http://...",
    "text_content": ""
  },
  "created_at": "..."
}
```

### Production State

```json
{
  "node": {},
  "current_version": {},
  "versions": [],
  "latest_job": {},
  "active_stale_reasons": [],
  "capability": {},
  "sandbox_jobs": []
}
```

For a text asset, `asset.text_content` should be populated and `access_url` may be empty. For binary assets, `access_url` should be a short-lived presigned GET URL.

## Tasks

### Task 1: SQL Queries For Read Surface

**Files:**
- Modify: `apps/server/sqlc/queries/sandbox_job.sql`
- Modify: `apps/server/sqlc/queries/production.sql`
- Generated: `apps/server/internal/store/db/sandbox_job.sql.go`
- Generated: `apps/server/internal/store/db/production.sql.go`

- [ ] **Step 1: Add sandbox job query**

Append to `apps/server/sqlc/queries/sandbox_job.sql`:

```sql
-- name: ListSandboxJobsByGenerationJob :many
SELECT *
FROM sandbox_job
WHERE generation_job_id = $1
ORDER BY created_at;
```

- [ ] **Step 2: Reuse existing version queries**

Do not add an artifact-version join query in M4.6. Use the existing `ListArtifactVersionsByNode`, `GetCurrentArtifactVersionForNode`, and `GetMediaAssetByID` queries. This keeps the phase ncustom edge and avoids generated row types that duplicate the existing asset mapper.

- [ ] **Step 3: Generate sqlc**

Run:

```bash
make sqlc-generate
```

Expected: sqlc generation succeeds and new query methods compile.

### Task 2: Model Capability API

**Files:**
- Create: `apps/server/internal/api/model_handler.go`
- Create: `apps/server/internal/api/model_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Write failing response mapping test**

Create `apps/server/internal/api/model_handler_test.go`:

```go
package api

import (
	"encoding/json"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestToModelCapabilityResponseParsesJSONFields(t *testing.T) {
	row := db.ModelCapability{
		ProviderID:              "mock",
		ModelID:                 "mock-image",
		DisplayName:             "Mock Image",
		OutputTypes:             []byte(`["image"]`),
		SupportedOperations:     []byte(`["text_to_image","image_to_image"]`),
		SupportedInputNodeTypes: []byte(`["text","image"]`),
		Limits:                  []byte(`{"max_prompt_chars":8000,"max_attempts":3}`),
		Pricing:                 []byte(`{"tier":"mock"}`),
		Defaults:                []byte(`{"steps":20}`),
		Enabled:                 true,
	}
	got := toModelCapabilityResponse(row)
	if got.ProviderID != "mock" || got.ModelID != "mock-image" {
		t.Fatalf("ids = %s/%s", got.ProviderID, got.ModelID)
	}
	if len(got.SupportedInputNodeTypes) != 2 || got.SupportedInputNodeTypes[1] != "image" {
		raw, _ := json.Marshal(got)
		t.Fatalf("bad response: %s", raw)
	}
	if len(got.SupportedOperations) != 2 || got.SupportedOperations[0] != "text_to_image" {
		t.Fatalf("operations = %#v", got.SupportedOperations)
	}
	if got.Limits["max_prompt_chars"].(float64) != 8000 {
		t.Fatalf("limits = %#v", got.Limits)
	}
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run TestToModelCapabilityResponseParsesJSONFields -count=1
```

Expected: FAIL because `toModelCapabilityResponse` does not exist.

- [ ] **Step 3: Implement handler**

Create `apps/server/internal/api/model_handler.go`:

```go
package api

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ModelHandler struct {
	queries *db.Queries
}

func NewModelHandler(queries *db.Queries) *ModelHandler {
	return &ModelHandler{queries: queries}
}

type modelCapabilityResponse struct {
	ProviderID              string         `json:"provider_id"`
	ModelID                 string         `json:"model_id"`
	DisplayName             string         `json:"display_name"`
	OutputTypes             []string       `json:"output_types"`
	SupportedOperations     []string       `json:"supported_operations"`
	SupportedInputNodeTypes []string       `json:"supported_input_node_types"`
	Limits                  map[string]any `json:"limits"`
	Pricing                 map[string]any `json:"pricing"`
	Defaults                map[string]any `json:"defaults"`
	Enabled                 bool           `json:"enabled"`
}

func (h *ModelHandler) ListCapabilities(ctx context.Context, c *app.RequestContext) {
	if _, ok := accountIDFromContext(c); !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	rows, err := h.queries.ListEnabledModelCapabilities(ctx)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list model capabilities")
		return
	}
	resp := make([]modelCapabilityResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, toModelCapabilityResponse(row))
	}
	c.JSON(consts.StatusOK, resp)
}

func toModelCapabilityResponse(row db.ModelCapability) modelCapabilityResponse {
	return modelCapabilityResponse{
		ProviderID:              row.ProviderID,
		ModelID:                 row.ModelID,
		DisplayName:             row.DisplayName,
		OutputTypes:             stringList(row.OutputTypes),
		SupportedOperations:     stringList(row.SupportedOperations),
		SupportedInputNodeTypes: stringList(row.SupportedInputNodeTypes),
		Limits:                  jsonMap(row.Limits),
		Pricing:                 jsonMap(row.Pricing),
		Defaults:                jsonMap(row.Defaults),
		Enabled:                 row.Enabled,
	}
}

func stringList(raw []byte) []string {
	out := []string{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func jsonMap(raw []byte) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}
```

- [ ] **Step 4: Register route**

In `apps/server/cmd/server/main.go`, after handler construction:

```go
modelHandler := api.NewModelHandler(queries)
```

Add route:

```go
h.GET("/api/model-capabilities", authMiddleware, modelHandler.ListCapabilities)
```

- [ ] **Step 5: Verify GREEN**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run TestToModelCapabilityResponseParsesJSONFields -count=1
```

Expected: PASS.

### Task 3: Version And Asset Read Responses

**Files:**
- Modify: `apps/server/internal/api/run_handler.go`
- Modify: `apps/server/internal/api/run_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Write failing mapping tests**

Append to `apps/server/internal/api/run_handler_test.go`:

```go
func TestToArtifactVersionResponseIncludesTextAsset(t *testing.T) {
	versionID := pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true}
	assetID := pgtype.UUID{Bytes: [16]byte{0x02}, Valid: true}
	version := db.ArtifactVersion{
		ID:        versionID,
		AssetID:   assetID,
		VersionNo: 2,
		Winner:    true,
		InputHash: "sha256:test",
		Output:    []byte(`{"text_preview":"hello"}`),
	}
	asset := db.MediaAsset{
		ID:          assetID,
		Type:        db.AssetTypeText,
		Mime:        "text/plain; charset=utf-8",
		TextContent: pgtype.Text{String: "hello world", Valid: true},
	}
	got := toArtifactVersionResponse(version, &asset, "")
	if got.ID == "" || got.Asset == nil || got.Asset.TextContent != "hello world" {
		t.Fatalf("response = %#v", got)
	}
	if !got.Winner || got.VersionNo != 2 {
		t.Fatalf("winner/version = %v/%d", got.Winner, got.VersionNo)
	}
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run TestToArtifactVersionResponseIncludesTextAsset -count=1
```

Expected: FAIL because `toArtifactVersionResponse` does not exist.

- [ ] **Step 3: Extend RunHandler dependencies**

Add storage dependency to `RunHandler`:

```go
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
```

- [ ] **Step 4: Add response structs**

Add to `apps/server/internal/api/run_handler.go`:

```go
type artifactVersionResponse struct {
	ID         string                 `json:"id"`
	WorkspaceID string               `json:"workspace_id"`
	NodeID     string                `json:"node_id"`
	JobID      string                `json:"job_id,omitempty"`
	AssetID    string                `json:"asset_id,omitempty"`
	VersionNo  int32                 `json:"version_no"`
	Winner     bool                  `json:"winner"`
	Output     map[string]any        `json:"output"`
	ReviewScore *float32             `json:"review_score,omitempty"`
	InputHash  string                `json:"input_hash"`
	Asset      *assetReadResponse    `json:"asset,omitempty"`
	CreatedAt  string                `json:"created_at"`
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
```

- [ ] **Step 5: Add mapper**

Add mapper:

```go
func toArtifactVersionResponse(version db.ArtifactVersion, asset *db.MediaAsset, accessURL string) artifactVersionResponse {
	resp := artifactVersionResponse{
		ID:          uuidToString(version.ID),
		WorkspaceID: uuidToString(version.WorkspaceID),
		NodeID:      uuidToString(version.NodeID),
		JobID:       uuidString(version.JobID),
		AssetID:     uuidString(version.AssetID),
		VersionNo:   version.VersionNo,
		Winner:      version.Winner,
		Output:      jsonObject(version.Output),
		InputHash:   version.InputHash,
		CreatedAt:   version.CreatedAt.Time.Format(time.RFC3339Nano),
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
```

- [ ] **Step 6: Add `ListNodeVersions` handler**

Add:

```go
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
```

- [ ] **Step 7: Add asset URL helper**

Add:

```go
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
```

Import `time` and `github.com/sinmaystar/clip-anvil/internal/storage`.

- [ ] **Step 8: Register route and update constructor call**

In `main.go`, change:

```go
runHandler := api.NewRunHandler(productionService, queries)
```

to:

```go
runHandler := api.NewRunHandler(productionService, queries, storageService)
```

Add route:

```go
h.GET("/api/nodes/:id/versions", authMiddleware, runHandler.ListNodeVersions)
```

- [ ] **Step 9: Verify GREEN**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run TestToArtifactVersionResponseIncludesTextAsset -count=1
```

Expected: PASS.

### Task 4: Job Detail And Sandbox Job Read APIs

**Files:**
- Modify: `apps/server/sqlc/queries/sandbox_job.sql`
- Modify: `apps/server/internal/api/run_handler.go`
- Modify: `apps/server/internal/api/run_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Add failing sandbox job mapper test**

Append to `apps/server/internal/api/run_handler_test.go`:

```go
func TestToSandboxJobResponseIncludesExecutionDetails(t *testing.T) {
	job := db.SandboxJob{
		ID:            pgtype.UUID{Bytes: [16]byte{0x03}, Valid: true},
		WorkspaceID:   pgtype.UUID{Bytes: [16]byte{0x04}, Valid: true},
		JobType:       "internal_media",
		OperationType: "extract_first_frame",
		Status:        db.JobStatusSucceeded,
		SandboxID:     pgtype.Text{String: "sandbox-1", Valid: true},
		Command:       "ffmpeg -y ...",
		Cwd:           "/workspace",
		Input:         []byte(`{"mode":"first"}`),
		Output:        []byte(`{"mime":"image/png"}`),
		ExitCode:      pgtype.Int4{Int32: 0, Valid: true},
		Stdout:        "ok",
		DurationMs:    12,
	}
	got := toSandboxJobResponse(job)
	if got.ID == "" || got.SandboxID != "sandbox-1" || got.ExitCode == nil || *got.ExitCode != 0 {
		t.Fatalf("response = %#v", got)
	}
	if got.Input["mode"] != "first" || got.Output["mime"] != "image/png" {
		t.Fatalf("input/output = %#v/%#v", got.Input, got.Output)
	}
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run TestToSandboxJobResponseIncludesExecutionDetails -count=1
```

Expected: FAIL because `toSandboxJobResponse` does not exist.

- [ ] **Step 3: Add response struct and mapper**

Add to `apps/server/internal/api/run_handler.go`:

```go
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
		CreatedAt:       job.CreatedAt.Time.Format(time.RFC3339Nano),
	}
	if job.ExitCode.Valid {
		value := job.ExitCode.Int32
		resp.ExitCode = &value
	}
	return resp
}
```

- [ ] **Step 4: Add ownership helpers and handlers**

Add handlers:

```go
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
```

Add helper:

```go
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
```

- [ ] **Step 5: Register routes**

In `main.go`:

```go
h.GET("/api/jobs/:id", authMiddleware, runHandler.GetJob)
h.GET("/api/jobs/:id/sandbox-jobs", authMiddleware, runHandler.ListJobSandboxJobs)
h.GET("/api/sandbox-jobs/:id", authMiddleware, runHandler.GetSandboxJob)
```

- [ ] **Step 6: Verify GREEN**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run TestToSandboxJobResponseIncludesExecutionDetails -count=1
```

Expected: PASS.

### Task 5: Node Production State Summary

**Files:**
- Modify: `apps/server/internal/api/run_handler.go`
- Modify: `apps/server/internal/api/run_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Add response struct**

Add:

```go
type productionStateResponse struct {
	Node               db.MediaNode              `json:"node"`
	CurrentVersion     *artifactVersionResponse  `json:"current_version,omitempty"`
	Versions           []artifactVersionResponse `json:"versions"`
	LatestJob          *generationJobResponse    `json:"latest_job,omitempty"`
	ActiveStaleReasons []staleReasonResponse     `json:"active_stale_reasons"`
	Capability         *modelCapabilityResponse  `json:"capability,omitempty"`
	SandboxJobs        []sandboxJobResponse       `json:"sandbox_jobs"`
}
```

- [ ] **Step 2: Implement handler**

Add:

```go
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
	resp := productionStateResponse{Node: node}

	versions, err := h.queries.ListArtifactVersionsByNode(ctx, node.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list versions")
		return
	}
	resp.Versions = make([]artifactVersionResponse, 0, len(versions))
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
```

- [ ] **Step 3: Register route**

In `main.go`:

```go
h.GET("/api/nodes/:id/production-state", authMiddleware, runHandler.GetNodeProductionState)
```

- [ ] **Step 4: Verify package tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -count=1
```

Expected: PASS.

### Task 6: M4.6 Smoke Script

**Files:**
- Create: `scripts/smoke-m4-6.sh`
- Modify: `docs/milestones/m4-shared-production-foundation.md`

- [ ] **Step 1: Create smoke script**

Create executable `scripts/smoke-m4-6.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

BASE="${CLIPANVIL_API_BASE:-http://127.0.0.1:8888/api}"

node --input-type=module <<'NODE'
const base = process.env.CLIPANVIL_API_BASE || "http://127.0.0.1:8888/api";

async function req(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  if (!res.ok) throw new Error(`${init.method || "GET"} ${path} -> ${res.status}: ${text}`);
  return text ? JSON.parse(text) : null;
}

const email = `m4-6-${Date.now()}@clip.test`;
const auth = await req("/auth/register", {
  method: "POST",
  headers: {"Content-Type": "application/json"},
  body: JSON.stringify({email, password: "password123", name: "M4.6 Smoke"}),
});
const headers = {Authorization: `Bearer ${auth.token}`};
const jsonHeaders = {...headers, "Content-Type": "application/json"};

const capabilities = await req("/model-capabilities", {headers});
if (!Array.isArray(capabilities) || !capabilities.some((cap) => cap.provider_id === "mock")) {
  throw new Error("model capabilities did not include mock provider");
}
if (!capabilities.some((cap) => cap.provider_id === "internal_ffmpeg" && cap.supported_operations.includes("extract_first_frame"))) {
  throw new Error("model capabilities did not include internal ffmpeg extract_first_frame");
}

const workspace = await req("/workspaces", {
  method: "POST",
  headers: jsonHeaders,
  body: JSON.stringify({name: "M4.6 Smoke", mode: "studio"}),
});

const node = await req("/nodes", {
  method: "POST",
  headers: jsonHeaders,
  body: JSON.stringify({
    workspace_id: workspace.id,
    node_type: "text",
    title: "Summary",
    prompt: "hello m4.6",
    operation_type: "text_generation",
    model_provider: "mock",
    model_id: "mock-text",
    model_params: {},
    canvas_x: 10,
    canvas_y: 10,
  }),
});
const run = await req(`/nodes/${node.id}/run`, {method: "POST", headers});
const versions = await req(`/nodes/${node.id}/versions`, {headers});
if (versions.length !== 1 || !versions[0].winner || !versions[0].asset?.text_content) {
  throw new Error(`bad versions response: ${JSON.stringify(versions)}`);
}
const state = await req(`/nodes/${node.id}/production-state`, {headers});
if (!state.current_version || state.latest_job.id !== run.job.id || !state.capability) {
  throw new Error(`bad production state: ${JSON.stringify(state)}`);
}
const job = await req(`/jobs/${run.job.id}`, {headers});
if (job.id !== run.job.id || job.status !== "succeeded") {
  throw new Error(`bad job detail: ${JSON.stringify(job)}`);
}
const sandboxJobs = await req(`/jobs/${run.job.id}/sandbox-jobs`, {headers});
if (!Array.isArray(sandboxJobs) || sandboxJobs.length !== 0) {
  throw new Error(`mock text job should not have sandbox jobs: ${JSON.stringify(sandboxJobs)}`);
}

console.log(JSON.stringify({
  workspaceId: workspace.id,
  nodeId: node.id,
  capabilityCount: capabilities.length,
  versionId: versions[0].id,
  jobId: job.id,
  sandboxJobs: sandboxJobs.length,
}, null, 2));
NODE
```

Run:

```bash
chmod +x scripts/smoke-m4-6.sh
bash -n scripts/smoke-m4-6.sh
```

- [ ] **Step 2: Add milestone phase**

Add M4.6 to `docs/milestones/m4-shared-production-foundation.md` with:

- Goal: expose M4 production read API for M5 Studio.
- Acceptance: capabilities, versions, production state, jobs, sandbox jobs readable.
- E2E: run text node, inspect versions/state/job; run internal ffmpeg node if a sandbox smoke environment is available and inspect sandbox jobs.

### Task 7: Verification

- [ ] **Step 1: Generate and test backend**

Run:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: all pass.

- [ ] **Step 2: Build frontend**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: build succeeds. If it hangs in the default sandbox, rerun with the same command in the approved non-sandbox environment.

- [ ] **Step 3: Run smoke**

Start the app with `./scripts/dev-start.sh` or foreground `./bin/server` using the worktree port from:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

Then run:

```bash
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-6.sh
```

Expected: smoke prints workspace id, node id, capability count, version id, job id, and `sandboxJobs: 0` for the mock text run.

- [ ] **Step 4: Regression smoke if server is already running**

Run:

```bash
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-1.sh
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-2.sh
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-3.sh
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-4.sh
CLIPANVIL_API_BASE=http://127.0.0.1:<server-port>/api scripts/smoke-m4-5.sh
```

Expected: all existing M4 smoke scripts remain green.

- [ ] **Step 5: Static checks**

Run:

```bash
rg -n 'exec\.Command|os/exec' apps/server/internal
git diff --check
```

Expected: no app-local command execution path, no whitespace errors.

## Acceptance Summary

M4.6 is complete only when:

- `GET /api/model-capabilities` returns enabled capabilities with parsed JSON arrays.
- `GET /api/nodes/:id/versions` returns version list with current winner and asset read data.
- Binary assets include presigned `access_url`; text assets include `text_content`.
- `GET /api/nodes/:id/production-state` returns node, versions, current version, latest job, active stale reasons, capability, and sandbox job summaries.
- `GET /api/jobs/:id` returns job detail.
- `GET /api/jobs/:id/sandbox-jobs` returns linked sandbox jobs.
- `GET /api/sandbox-jobs/:id` enforces workspace ownership and returns execution details.
- M4.6 smoke passes.
- Existing M4.1-M4.5 smoke remains green.
