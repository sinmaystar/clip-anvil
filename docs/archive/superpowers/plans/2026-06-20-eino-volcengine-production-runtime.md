# Eino Volcengine Production Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a real Eino-first, fully asynchronous Volcengine production runtime so Studio can generate real text, image, and video outputs through ClipAnvil backend, while preserving the shared Production Core for M6 Agent reuse. Audio is held until a usable Volcengine model is available.

**Architecture:** Studio calls ClipAnvil backend only. Backend converts node state into `GenerationIntent`, creates `generation_job`, enqueues work into `ProductionRunner`, executes Eino runtime components, persists `media_asset` and `artifact_version`, and streams job events to the workspace. `ProviderBridge` remains only for mock and historical compatibility; real Volcengine execution uses `EinoProductionRuntime`.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, PostgreSQL, MinIO storage, Eino, EinoExt Ark, Volcengine official APIs, React 19, React Flow, WebSocket workspace events, shell smoke scripts.

---

## Required Local Configuration

Real-provider validation requires a local `.env` in the repo root or exported environment variables. The committed defaults must stay mock-only.

```bash
CLIPANVIL_PRODUCTION_PROVIDER_MODE=real
CLIPANVIL_PRODUCTION_DEFAULT_PROVIDER=volcengine
CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY=volcengine-api-key-from-local-env
CLIPANVIL_PRODUCTION_VOLCENGINE_BASE_URL=https://ark.cn-beijing.volces.com/api/v3
CLIPANVIL_PRODUCTION_VOLCENGINE_REGION=cn-beijing
CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL=doubao-seed-2-0-mini-260428
CLIPANVIL_PRODUCTION_VOLCENGINE_IMAGE_MODEL=doubao-seedream-5-0-260128
CLIPANVIL_PRODUCTION_VOLCENGINE_VIDEO_MODEL=doubao-seedance-1-0-pro-fast-251015
CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_MODEL=
CLIPANVIL_PRODUCTION_WORKER_CONCURRENCY=2
CLIPANVIL_PRODUCTION_PROVIDER_POLL_INTERVAL_SECONDS=5
CLIPANVIL_PRODUCTION_PROVIDER_MAX_POLL_SECONDS=1800
```

Real smoke scripts must additionally require:

```bash
CLIPANVIL_REAL_PROVIDER_SMOKE=1
```

The user must provide the actual Volcengine API key before real-provider smoke and browser E2E can pass. Text, image, and video model IDs are fixed for this plan. Audio real-provider validation is skipped because no usable audio model is available yet. Unit, integration, and mock browser E2E must pass without real provider values.

## File Structure

### Backend Runtime

- Modify: `apps/server/go.mod`, `apps/server/go.sum`  
  Add Eino and EinoExt Ark dependencies.
- Modify: `apps/server/config.yaml`, `.env.example`, `apps/server/internal/config/config.go`, `apps/server/internal/config/config_test.go`  
  Add Volcengine region, optional audio model, worker concurrency, request timeout, poll interval, and max poll settings.
- Create: `apps/server/internal/production/events.go`  
  Defines `ProductionEvent`, event type constants, payload helpers, and redaction helpers.
- Create: `apps/server/internal/production/runtime.go`  
  Defines `ProductionJob`, `ProductionOutput`, and `EinoProductionRuntime`.
- Create: `apps/server/internal/production/runner.go`  
  Owns queue, worker concurrency, job state transitions, retry, cancellation hooks, and event publication.
- Create: `apps/server/internal/production/volcengine_runtime.go`  
  Routes `GenerationIntent.OperationType` to text, image, and video components. Audio routing remains disabled until a model is available.
- Create: `apps/server/internal/production/volcengine_text.go`  
  Implements EinoExt Ark ChatModel streaming execution.
- Create: `apps/server/internal/production/volcengine_image.go`  
  Implements EinoExt Ark ImageGenerationModel execution and URL/base64 output normalization.
- Create: `apps/server/internal/production/volcengine_video.go`  
  Wraps Volcengine video official API as an Eino runtime component with task create, poll, download, and metadata extraction.
- Do not create an audio runtime in this phase. Keep `text_to_audio` capability disabled.
- Create: `apps/server/internal/production/http_download.go`  
  Downloads provider output into bounded memory or temp files, verifies MIME/size, and streams upload to MinIO.
- Modify: `apps/server/internal/production/service.go`  
  Add `SubmitNodeRun`, `SubmitGenerationIntent`, and async persistence helpers. Keep sync `RunNode` for mock compatibility.
- Modify: `apps/server/internal/production/provider.go`, `apps/server/internal/production/volcengine_provider.go`  
  Stop using `ProviderBridge` as the real Volcengine path. Keep it for mock and historical tests.
- Modify: `apps/server/cmd/server/main.go`  
  Wire production runner, runtime, storage, and workspace event broadcaster.

### Backend Persistence And API

- Modify: `apps/server/sqlc/queries/production.sql`  
  Add job status transition queries and active queued/running job selection.
- Modify generated: `apps/server/internal/store/db/production.sql.go`  
  Regenerate through `make sqlc-generate`.
- Create: `apps/server/migrations/013_real_volcengine_capabilities.sql`  
  Seed real Volcengine model capabilities with enabled state matching implemented adapters.
- Create: `apps/server/migrations/014_production_async_runner.sql`  
  Add provider task metadata, progress detail, cancel reason, lock fields, or queue fields if the existing `generation_job` fields are insufficient.
- Modify: `apps/server/internal/api/run_handler.go`, `apps/server/internal/api/run_handler_test.go`  
  Change real run response to `202 Accepted` with queued job. Keep final artifact response for legacy mock tests only if needed.
- Modify: `apps/server/internal/api/ws_hub.go`, `apps/server/internal/api/ws_handler.go`, `apps/server/internal/api/ws_hub_test.go`  
  Broadcast production job events to workspace clients.
- Modify: `apps/server/internal/api/model_handler.go`, `apps/server/internal/api/model_handler_test.go`  
  Ensure disabled capabilities are not exposed.

### Frontend Studio

- Modify: `apps/web/src/lib/api.ts`  
  Add async run response type, job fetch helpers, and production event types.
- Modify: `apps/web/src/lib/ws.ts`  
  Decode production job events and expose typed subscriptions.
- Modify: `apps/web/src/lib/productionPanel.ts`, `apps/web/src/lib/productionPanel.test.mjs`  
  Represent queued/running/succeeded/failed/cancelled states and stream deltas.
- Modify: `apps/web/src/components/PropertyPanel.tsx`  
  Make Run button submit async job, show running state, disable duplicate submit for the same node, and surface errors.
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`  
  Subscribe to workspace job events and refresh production state when versions change.
- Modify: `apps/web/src/lib/productionPreview.ts`, `apps/web/src/lib/productionPreview.test.mjs`  
  Render text stream preview, final text, images, and video outputs from latest job/current version. Audio preview remains existing upload/playback behavior only.

### Tests And Smoke Scripts

- Create: `scripts/smoke-real-volcengine-text.sh`
- Create: `scripts/smoke-real-volcengine-image.sh`
- Create: `scripts/smoke-real-volcengine-video.sh`
- Create: `scripts/smoke-studio-real-provider-e2e.mjs`
- Modify: `docs/superpowers/specs/2026-06-20-eino-volcengine-production-runtime-design.md` only if implementation reveals a spec mismatch.
- Create or modify focused tests under:
  - `apps/server/internal/production/*_test.go`
  - `apps/server/internal/api/*_test.go`
  - `apps/web/src/lib/*.test.mjs`

---

## Task 1: Configuration And Capability Seeds

**Files:**
- Modify: `.env.example`
- Modify: `apps/server/config.yaml`
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/internal/config/config_test.go`
- Create: `apps/server/migrations/013_real_volcengine_capabilities.sql`

**Delivery Standard:**
- Local config can express real Volcengine text/image/video models, optional future audio model, and runner settings.
- Secrets remain environment-only.
- DB capabilities expose only implemented adapters.

**Acceptance Standard:**
- Config tests prove `.env` overrides YAML values.
- `model_capability` seed contains Volcengine text/image/video rows enabled with correct output/input/operation metadata, and an audio row disabled until a model is available.
- Missing API key causes a persisted failed job in later tasks, not a panic or server startup failure.

- [ ] **Step 1: Add failing config tests**

Add tests in `apps/server/internal/config/config_test.go` that set:

```go
t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_REGION", "cn-beijing")
t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_MODEL", "")
t.Setenv("CLIPANVIL_PRODUCTION_WORKER_CONCURRENCY", "2")
t.Setenv("CLIPANVIL_PRODUCTION_PROVIDER_POLL_INTERVAL_SECONDS", "5")
t.Setenv("CLIPANVIL_PRODUCTION_PROVIDER_MAX_POLL_SECONDS", "1800")
```

Assert:

```go
if cfg.Production.Volcengine.Region != "cn-beijing" {
    t.Fatalf("Region = %q", cfg.Production.Volcengine.Region)
}
if cfg.Production.Volcengine.AudioModel != "" {
    t.Fatalf("AudioModel = %q", cfg.Production.Volcengine.AudioModel)
}
if cfg.Production.WorkerConcurrency != 2 {
    t.Fatalf("WorkerConcurrency = %d", cfg.Production.WorkerConcurrency)
}
```

- [ ] **Step 2: Run config tests and confirm failure**

Run:

```bash
go test ./internal/config -count=1
```

Expected: FAIL because the new config fields do not exist.

- [ ] **Step 3: Implement config fields and env edges**

Add fields:

```go
type ProductionConfig struct {
    ProviderMode              string `mapstructure:"provider_mode"`
    DefaultProvider           string `mapstructure:"default_provider"`
    DefaultTextModel          string `mapstructure:"default_text_model"`
    WorkerConcurrency         int    `mapstructure:"worker_concurrency"`
    ProviderPollIntervalSeconds int  `mapstructure:"provider_poll_interval_seconds"`
    ProviderMaxPollSeconds    int    `mapstructure:"provider_max_poll_seconds"`
    Volcengine                VolcengineConfig
}

type VolcengineConfig struct {
    APIKey     string `mapstructure:"api_key"`
    BaseURL    string `mapstructure:"base_url"`
    Region     string `mapstructure:"region"`
    TextModel  string `mapstructure:"text_model"`
    ImageModel string `mapstructure:"image_model"`
    VideoModel string `mapstructure:"video_model"`
    AudioModel string `mapstructure:"audio_model"`
}
```

Bind the matching env keys in the existing config edge list.

- [ ] **Step 4: Add `.env.example` and YAML defaults**

Defaults:

```yaml
production:
  provider_mode: "mock"
  default_provider: "mock"
  default_text_model: "mock-text"
  worker_concurrency: 2
  provider_poll_interval_seconds: 5
  provider_max_poll_seconds: 1800
  volcengine:
    api_key: ""
    base_url: "https://ark.cn-beijing.volces.com/api/v3"
    region: "cn-beijing"
    text_model: "doubao-seed-2-0-mini-260428"
    image_model: "doubao-seedream-5-0-260128"
    video_model: "doubao-seedance-1-0-pro-fast-251015"
    audio_model: ""
```

- [ ] **Step 5: Add real capability seed migration**

Create `apps/server/migrations/013_real_volcengine_capabilities.sql` with upserts for:

- `volcengine` provider
- text model capability for `text_generation`
- image model capability for `text_to_image`, `image_to_image`, `multi_image_to_image`
- video model capability for `text_to_video`, `image_to_video`, `multi_reference_to_video`
- audio model capability for `text_to_audio`, with `enabled=false`

Set `enabled=true` only for adapters completed by the current task set. During Task 1, keep video disabled until Task 6 is complete. Keep audio disabled for the whole plan because no usable model is available yet.

- [ ] **Step 6: Verify**

Run:

```bash
go test ./internal/config -count=1
make migrate-up
git diff --check
```

Expected: config tests pass, migrations apply, diff check passes.

---

## Task 2: Async Job State Machine And Production Runner

**Files:**
- Modify: `apps/server/sqlc/queries/production.sql`
- Regenerate: `apps/server/internal/store/db/production.sql.go`
- Create: `apps/server/internal/production/events.go`
- Create: `apps/server/internal/production/runtime.go`
- Create: `apps/server/internal/production/runner.go`
- Modify: `apps/server/internal/production/service.go`
- Modify: `apps/server/internal/production/service_test.go`

**Delivery Standard:**
- Real production requests create queued jobs and return immediately.
- Runner owns transitions from queued to running to terminal state.
- Runner has bounded concurrency and does not run provider calls on HTTP request goroutines.

**Acceptance Standard:**
- Unit test proves `SubmitNodeRun` creates `generation_job(status=queued)`.
- Unit test proves fake runtime moves job to `running` then `succeeded`.
- Unit test proves fake runtime failure moves job to `failed` with `error_code` and sanitized response.
- Unit test proves worker concurrency limit is honored.

- [ ] **Step 1: Add sqlc queries for job transitions**

Add queries:

```sql
-- name: MarkGenerationJobRunning :one
UPDATE generation_job
SET status = 'running',
    progress = $2,
    provider_response = $3,
    started_at = COALESCE(started_at, now())
WHERE id = $1
RETURNING *;

-- name: MarkGenerationJobProgress :one
UPDATE generation_job
SET progress = $2,
    provider_response = $3
WHERE id = $1
RETURNING *;

-- name: MarkGenerationJobFailed :one
UPDATE generation_job
SET status = 'failed',
    progress = $2,
    provider_response = $3,
    error_code = $4,
    error_message = $5,
    completed_at = now()
WHERE id = $1
RETURNING *;
```

Add a succeeded transition if the existing success path cannot be reused without sync `RunNode`.

- [ ] **Step 2: Regenerate sqlc and confirm generated methods exist**

Run:

```bash
make sqlc-generate
rg -n "MarkGenerationJobRunning|MarkGenerationJobProgress|MarkGenerationJobFailed" apps/server/internal/store/db/production.sql.go
```

Expected: generated methods are present.

- [ ] **Step 3: Define production runtime contracts**

Create `apps/server/internal/production/runtime.go`:

```go
package production

import "context"

type ProductionJob struct {
    ID string
}

type ProductionOutput struct {
    RenderedPrompt   string
    TextContent      string
    AssetContent     []byte
    AssetMIME        string
    AssetStorageURL  string
    AssetSizeBytes   int64
    AssetMetadata    map[string]any
    RequestSummary   map[string]any
    ResponseSummary  map[string]any
}

type EinoProductionRuntime interface {
    Start(ctx context.Context, job ProductionJob, intent GenerationIntent) (<-chan ProductionEvent, error)
}
```

- [ ] **Step 4: Define production events**

Create event constants:

```go
const (
    ProductionEventJobStarted       = "job.started"
    ProductionEventModelStreamDelta = "model.stream_delta"
    ProductionEventProviderTaskCreated = "provider.task_created"
    ProductionEventProviderProgress = "provider.progress"
    ProductionEventAssetDownloading = "asset.downloading"
    ProductionEventAssetUploading   = "asset.uploading"
    ProductionEventJobSucceeded     = "job.succeeded"
    ProductionEventJobFailed        = "job.failed"
    ProductionEventJobCancelled     = "job.cancelled"
)
```

`ProductionEvent` must include `JobID`, `WorkspaceID`, `TargetNodeID`, `Type`, `Progress`, `Payload`, `Output`, and `Error`.

- [ ] **Step 5: Add failing service tests**

In `apps/server/internal/production/service_test.go`, add tests named:

- `TestSubmitNodeRunCreatesQueuedJob`
- `TestRunnerMarksJobSucceededFromFakeRuntime`
- `TestRunnerMarksJobFailedFromFakeRuntime`
- `TestRunnerLimitsConcurrentRuntimeStarts`

Use a fake runtime that emits:

```go
ProductionEvent{Type: ProductionEventJobStarted, Progress: 1}
ProductionEvent{Type: ProductionEventModelStreamDelta, Progress: 30, Payload: map[string]any{"delta": "hello"}}
ProductionEvent{Type: ProductionEventJobSucceeded, Progress: 100, Output: ProductionOutput{TextContent: "hello"}}
```

- [ ] **Step 6: Implement `ProductionRunner`**

Runner requirements:

- accepts jobs through `Enqueue`
- starts at most `WorkerConcurrency` jobs at once
- marks job `running` before consuming runtime events
- persists progress events
- persists success through existing asset/version helper
- persists failure with `provider_error`, `provider_timeout`, or `provider_config_error`
- never logs API key or authorization headers

- [ ] **Step 7: Add async submission service methods**

Add:

```go
func (s *Service) SubmitNodeRun(ctx context.Context, nodeID pgtype.UUID, requestedBy RequestedBy, options RunOptions) (db.GenerationJob, error)
func (s *Service) SubmitGenerationIntent(ctx context.Context, intent GenerationIntent, options RunOptions) (db.GenerationJob, error)
```

Both methods must validate capability, compute input refs, create a queued job, and enqueue the runner.

- [ ] **Step 8: Verify**

Run:

```bash
make sqlc-generate
go test ./internal/production -count=1
make server-build
git diff --check
```

Expected: production tests pass and build succeeds.

---

## Task 3: API And Workspace Event Streaming

**Files:**
- Modify: `apps/server/internal/api/run_handler.go`
- Modify: `apps/server/internal/api/run_handler_test.go`
- Modify: `apps/server/internal/api/ws_hub.go`
- Modify: `apps/server/internal/api/ws_handler.go`
- Modify: `apps/server/internal/api/ws_hub_test.go`
- Modify: `apps/server/cmd/server/main.go`

**Delivery Standard:**
- Studio run API returns `202 Accepted` and queued job for real async execution.
- Workspace WebSocket broadcasts typed production events.
- Existing production state API continues to show latest job, versions, and stale reasons.

**Acceptance Standard:**
- `POST /api/nodes/:id/run` returns `202` with `job.status=queued`.
- WebSocket client receives `production.job.updated` and `production.model.delta` events.
- Failed run returns the job id and stores failure, not only an HTTP error.

- [ ] **Step 1: Add API tests**

Add tests:

- `TestRunNodeReturnsAcceptedQueuedJob`
- `TestRunNodeFailureReturnsPersistedJob`
- `TestWorkspaceWebSocketBroadcastsProductionJobEvent`

Expected response node:

```json
{
  "job": {
    "id": "job-id-example",
    "status": "queued",
    "target_node_id": "node-id-example"
  }
}
```

- [ ] **Step 2: Update `RunHandler.RunNode`**

Replace the real path call:

```go
job, err := h.service.SubmitNodeRun(ctx, nodeID, production.RequestedBy{Type: "user", ID: uuidToString(accountID)}, req.runOptions())
```

Return:

```go
c.JSON(consts.StatusAccepted, runNodeResponse{Job: toGenerationJobResponse(job)})
```

Keep legacy sync response only for explicitly configured mock compatibility if required by existing tests.

- [ ] **Step 3: Broadcast production events**

Extend workspace WebSocket event payloads with:

```json
{
  "type": "production.job.updated",
  "workspace_id": "workspace-id-example",
  "node_id": "node-id-example",
  "job_id": "job-id-example",
  "status": "running",
  "progress": 45
}
```

For text deltas:

```json
{
  "type": "production.model.delta",
  "workspace_id": "workspace-id-example",
  "node_id": "node-id-example",
  "job_id": "job-id-example",
  "delta": "partial text"
}
```

- [ ] **Step 4: Wire runner event publisher in `main.go`**

Pass a broadcaster into `ProductionRunner` so runner events are persisted and published through the same workspace hub used by canvas updates.

- [ ] **Step 5: Verify**

Run:

```bash
go test ./internal/api -count=1
make server-build
git diff --check
```

Expected: API tests pass and async response semantics are visible in tests.

---

## Task 4: Eino Ark Text Runtime

**Files:**
- Modify: `apps/server/go.mod`, `apps/server/go.sum`
- Create: `apps/server/internal/production/volcengine_runtime.go`
- Create: `apps/server/internal/production/volcengine_text.go`
- Create: `apps/server/internal/production/volcengine_text_test.go`
- Modify: `apps/server/internal/production/provider.go`
- Modify: `apps/server/internal/production/volcengine_provider.go`

**Delivery Standard:**
- Text generation uses EinoExt Ark `ChatModel.Stream`.
- Text deltas are emitted as production events.
- Final text is persisted as text asset and artifact version.
- Missing API key fails through job state and persisted error.

**Acceptance Standard:**
- Fake Eino stream unit test produces `model.stream_delta` events and final output.
- Real smoke with configured key produces non-empty text.
- No API key appears in logs, `provider_request`, or `provider_response`.

- [ ] **Step 1: Add dependencies**

Run:

```bash
cd apps/server && go get github.com/cloudwego/eino@latest github.com/cloudwego/eino-ext/components/model/ark@latest
```

Expected: `apps/server/go.mod` includes Eino dependencies.

- [ ] **Step 2: Add fake-stream text tests**

Create `TestVolcengineTextRuntimeStreamsDeltasAndFinalText`:

- input prompt: `Write one short sentence about a quiet studio.`
- fake chunks: `A quiet`, ` studio glows.`
- expected deltas: two `model.stream_delta` events
- expected final text: `A quiet studio glows.`

- [ ] **Step 3: Implement text runtime**

`volcengine_text.go` must:

- build Eino messages from `GenerationIntent`
- set Ark model from `intent.Model.ModelID` or config default
- call `Stream`
- emit each chunk as `ProductionEventModelStreamDelta`
- concatenate chunks
- emit `ProductionEventJobSucceeded` with `ProductionOutput.TextContent`

- [ ] **Step 4: Add API key config failure test**

Test name: `TestVolcengineTextRuntimeFailsWithoutAPIKey`

Expected error code: `provider_config_error`.

- [ ] **Step 5: Verify**

Run:

```bash
go test ./internal/production -run 'TextRuntime|SubmitNodeRun|Runner' -count=1
make server-build
```

Expected: text runtime and runner tests pass.

---

## Task 5: Eino Ark Image Runtime

**Files:**
- Create: `apps/server/internal/production/volcengine_image.go`
- Create: `apps/server/internal/production/volcengine_image_test.go`
- Create: `apps/server/internal/production/http_download.go`
- Create: `apps/server/internal/production/http_download_test.go`
- Modify: `apps/server/internal/production/volcengine_runtime.go`

**Delivery Standard:**
- Image generation uses EinoExt Ark `ImageGenerationModel`.
- Provider URL and base64 outputs are normalized into MinIO-backed assets.
- Text-to-image is available in Studio through async runner.

**Acceptance Standard:**
- URL output test downloads fake PNG and uploads to fake asset store.
- Base64 output test decodes fake PNG and uploads to fake asset store.
- Invalid MIME or oversized output fails with persisted `provider_error`.

- [ ] **Step 1: Add output normalization tests**

Tests:

- `TestImageRuntimeDownloadsURLAndUploadsAsset`
- `TestImageRuntimeDecodesBase64AndUploadsAsset`
- `TestImageRuntimeRejectsUnexpectedMIME`

Expected asset metadata:

```json
{
  "provider": "volcengine",
  "operation": "text_to_image",
  "source": "provider_output"
}
```

- [ ] **Step 2: Implement `http_download.go`**

Implement bounded download:

- max bytes from capability limit or default 50 MB
- MIME sniffing with `http.DetectContentType`
- allow `image/png`, `image/jpeg`, `image/webp`
- return `provider_error` for invalid output

- [ ] **Step 3: Implement image runtime**

`volcengine_image.go` must:

- build Eino image generation input from rendered prompt
- pass size, response format, watermark, and seed params from `intent.Params`
- support image refs only when capability enables input node type `image`
- emit `asset.downloading`, `asset.uploading`, and `job.succeeded`

- [ ] **Step 4: Verify**

Run:

```bash
go test ./internal/production -run 'ImageRuntime|Download' -count=1
make server-build
```

Expected: image runtime tests pass.

---

## Task 6: Volcengine Video Runtime Through Eino Component

**Files:**
- Create: `apps/server/internal/production/volcengine_video.go`
- Create: `apps/server/internal/production/volcengine_video_test.go`
- Modify: `apps/server/internal/production/volcengine_runtime.go`
- Modify: `apps/server/migrations/013_real_volcengine_capabilities.sql`

**Delivery Standard:**
- Video runtime wraps Volcengine video official API behind an Eino component boundary.
- Runtime supports task create, polling, timeout, cancellation, download, MinIO upload, and final artifact version.
- Video capability is enabled only after this runtime passes tests.

**Acceptance Standard:**
- Fake video API test covers task create -> running -> succeeded.
- Fake video API test covers provider failure.
- Fake video API test covers polling timeout.
- Browser E2E can start a video job without blocking UI.

- [ ] **Step 1: Add fake video API tests**

Tests:

- `TestVideoRuntimeCreatesPollsDownloadsAndUploads`
- `TestVideoRuntimePersistsProviderFailure`
- `TestVideoRuntimeTimesOutAfterMaxPollSeconds`

Fake API responses:

```json
{ "task_id": "video-task-1", "status": "queued" }
{ "task_id": "video-task-1", "status": "running", "progress": 50 }
{ "task_id": "video-task-1", "status": "succeeded", "url": "http://127.0.0.1:18080/video.mp4" }
```

- [ ] **Step 2: Implement video component**

`volcengine_video.go` must:

- create provider task with prompt, input refs, duration, aspect ratio, resolution
- emit `provider.task_created` with redacted task id metadata
- poll using configured interval and max duration
- emit progress events
- download final video
- upload as `media_asset(type=video, mime=video/mp4)`

- [ ] **Step 3: Enable video capability**

Update migration seed so video capabilities become `enabled=true` once the implementation exists:

- `text_to_video`
- `image_to_video`
- `multi_reference_to_video`

- [ ] **Step 4: Verify**

Run:

```bash
go test ./internal/production -run 'VideoRuntime|Runner' -count=1
make server-build
git diff --check
```

Expected: video runtime tests pass.

---

## Task 7: Audio Hold And Disabled Capability Guard

**Files:**
- Modify: `apps/server/internal/production/volcengine_runtime.go`
- Modify: `apps/server/internal/production/capability.go`
- Modify: `apps/server/internal/production/service_test.go`
- Modify: `apps/server/internal/api/model_handler_test.go`
- Modify: `apps/server/migrations/013_real_volcengine_capabilities.sql`

**Delivery Standard:**
- Audio real generation is explicitly out of scope for this phase.
- `text_to_audio` capability remains disabled and is not shown as a runnable real model in Studio.
- If a user manually submits an audio generation request with Volcengine provider, backend returns `capability_mismatch` or `provider_config_error` and persists a failed job.
- Existing uploaded audio assets remain viewable/playable.

**Acceptance Standard:**
- No audio API key or model is required for the final verification set.
- Model list API does not expose enabled Volcengine audio generation.
- Backend tests prove audio generation cannot silently fall back to mock when provider mode is real.

- [ ] **Step 1: Add disabled audio capability tests**

Tests:

- `TestVolcengineAudioCapabilityIsDisabledWithoutModel`
- `TestSubmitAudioNodeRunWithVolcenginePersistsFailure`
- `TestModelListDoesNotExposeDisabledVolcengineAudio`

Expected failed job:

```json
{
  "status": "failed",
  "error_code": "capability_mismatch"
}
```

- [ ] **Step 2: Keep audio runtime route disabled**

`volcengine_runtime.go` must return a controlled error for `text_to_audio` while the capability is disabled:

```go
return nil, fmt.Errorf("%w: volcengine audio model is not configured", ErrCapabilityMismatch)
```

- [ ] **Step 3: Keep audio capability disabled**

Update migration seed so `text_to_audio` is present but `enabled=false`, with metadata:

```json
{
  "status": "hold",
  "reason": "no usable Volcengine audio model configured"
}
```

- [ ] **Step 4: Verify**

Run:

```bash
go test ./internal/production -run 'AudioCapability|SubmitAudio' -count=1
go test ./internal/api -run 'ModelListDoesNotExposeDisabledVolcengineAudio' -count=1
make server-build
git diff --check
```

Expected: audio is disabled intentionally and does not block text/image/video delivery.

---

## Task 8: Studio Async UX

**Files:**
- Modify: `apps/web/src/lib/api.ts`
- Modify: `apps/web/src/lib/ws.ts`
- Modify: `apps/web/src/lib/productionPanel.ts`
- Modify: `apps/web/src/lib/productionPanel.test.mjs`
- Modify: `apps/web/src/lib/productionPreview.ts`
- Modify: `apps/web/src/lib/productionPreview.test.mjs`
- Modify: `apps/web/src/components/PropertyPanel.tsx`
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`

**Delivery Standard:**
- Studio run button starts async job and does not wait for final artifact.
- User sees queued/running/progress/succeeded/failed states.
- Text stream deltas show progressively when available.
- Final image/video/text preview refreshes from persisted production state.

**Acceptance Standard:**
- Web tests prove queued/running/final states render correctly.
- Duplicate run click is disabled while same node has active job.
- Failed jobs show actionable error code/message.

- [ ] **Step 1: Add frontend state tests**

Add tests:

- `productionPanelStateLabelsQueuedRunningSucceededFailed`
- `productionPanelAccumulatesTextDeltas`
- `productionPreviewRendersVideoAssets`

Expected state labels:

```ts
["Queued", "Running", "Succeeded", "Failed", "Cancelled"]
```

- [ ] **Step 2: Update API types**

`runNode` must return:

```ts
type RunNodeResponse = {
  job: GenerationJob
  node?: MediaNode
  version?: ArtifactVersion
}
```

For async real runs, `node` and `version` can be absent until production state refresh.

- [ ] **Step 3: Update WebSocket event handling**

Decode:

```ts
type ProductionJobUpdatedEvent = {
  type: "production.job.updated"
  workspace_id: string
  node_id: string
  job_id: string
  status: string
  progress: number
}

type ProductionModelDeltaEvent = {
  type: "production.model.delta"
  workspace_id: string
  node_id: string
  job_id: string
  delta: string
}
```

- [ ] **Step 4: Update PropertyPanel**

Run button behavior:

- sends async run request
- stores active job id for selected node
- disables duplicate run while status is `queued` or `running`
- shows progress and stream delta in the production area
- refreshes production state on terminal event

- [ ] **Step 5: Verify**

Run:

```bash
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
```

Expected: lint and build pass.

---

## Task 9: Real Provider Smoke Scripts And Browser E2E

**Files:**
- Create: `scripts/smoke-real-volcengine-text.sh`
- Create: `scripts/smoke-real-volcengine-image.sh`
- Create: `scripts/smoke-real-volcengine-video.sh`
- Create: `scripts/smoke-studio-real-provider-e2e.mjs`
- Modify: `docs/superpowers/specs/2026-06-20-eino-volcengine-production-runtime-design.md` only if actual commands differ from the spec.

**Delivery Standard:**
- Real-provider smoke tests are explicit opt-in and never run by default CI.
- Browser E2E covers Studio real execution from user action to final preview.
- Smoke failures print exact missing env vars or job error details.

**Acceptance Standard:**
- Without `CLIPANVIL_REAL_PROVIDER_SMOKE=1`, scripts exit with a clear skip message and status 0.
- With required env and valid Volcengine text/image/video models, each script creates a real job and waits for `succeeded`.
- Browser E2E verifies text, image, and video node flows. Audio is skipped with an explicit hold message.

- [ ] **Step 1: Add shared smoke preflight behavior**

Each script must check:

```bash
: "${CLIPANVIL_REAL_PROVIDER_SMOKE:=}"
: "${CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY:=}"
```

If real smoke is not enabled:

```bash
echo "SKIP: set CLIPANVIL_REAL_PROVIDER_SMOKE=1 to run real Volcengine smoke"
exit 0
```

If required model id is missing, exit non-zero with the missing variable name.

- [ ] **Step 2: Implement text smoke**

Flow:

1. Register a test user.
2. Create a Studio workspace.
3. Create a text node with prompt `Write a one sentence production smoke result.`
4. Set provider/model to Volcengine text model.
5. Run node.
6. Poll job until terminal.
7. Assert `status=succeeded` and text asset is non-empty.

- [ ] **Step 3: Implement image smoke**

Flow:

1. Create image node with prompt `A simple studio desk with a single lamp.`
2. Run node.
3. Poll until `succeeded`.
4. Assert current version asset type is `image` and `access_url` loads.

- [ ] **Step 4: Implement video smoke**

Flow:

1. Create video node with prompt `A five second shot of a quiet editing timeline glowing on a monitor.`
2. Run node.
3. Assert initial response is queued or running.
4. Poll until terminal.
5. Assert asset type is `video`, MIME starts with `video/`, and size is non-zero.

- [ ] **Step 5: Implement browser E2E**

`scripts/smoke-studio-real-provider-e2e.mjs` must:

- open the Vite URL produced by `./scripts/dev-start.sh`
- register/login
- create a Studio workspace
- create text, image, and video nodes
- select Volcengine models
- click Run for each node
- verify queued/running state appears immediately
- wait for final preview or player
- collect job ids and final asset ids in stdout

- [ ] **Step 6: Verify smoke scripts with mock skip path**

Run:

```bash
scripts/smoke-real-volcengine-text.sh
scripts/smoke-real-volcengine-image.sh
scripts/smoke-real-volcengine-video.sh
git diff --check
```

Expected: scripts skip without real env and return status 0.

---

## End-To-End Test Matrix

### Mock E2E

**Purpose:** Verify Studio async UX and persistence without spending provider credits.

**Setup:**

```bash
CLIPANVIL_PRODUCTION_PROVIDER_MODE=mock ./scripts/dev-start.sh
```

**Cases:**

1. Text node mock run returns queued job, then succeeded job, then text preview.
2. Image node mock run returns queued job, then succeeded job, then image preview.
3. Video node mock run returns queued job, then succeeded job, then video preview.
4. Failed mock provider path stores `generation_job(status=failed)` and displays error.

**Pass Criteria:**

- No HTTP request blocks until final artifact.
- Workspace events update visible state.
- Latest Job, Versions, and preview agree with backend production state.

### Real Text E2E

**Setup Required:**

```bash
CLIPANVIL_REAL_PROVIDER_SMOKE=1
CLIPANVIL_PRODUCTION_PROVIDER_MODE=real
CLIPANVIL_PRODUCTION_DEFAULT_PROVIDER=volcengine
CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY=volcengine-api-key-from-local-env
CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL=doubao-seed-2-0-mini-260428
```

**Input:** `Write a short tagline for an AI video studio.`

**Expected Output:** A non-empty text asset, streamed delta events, `generation_job.status=succeeded`, and a current artifact version.

### Real Image E2E

**Setup Required:** Text setup plus `CLIPANVIL_PRODUCTION_VOLCENGINE_IMAGE_MODEL`.

**Input:** `A cinematic desk setup with a compact editing console and soft daylight.`

**Expected Output:** A MinIO-backed image asset, visible canvas preview, `storage_url` not pointing to a Volcengine temporary URL.

### Real Video E2E

**Setup Required:** Text setup plus `CLIPANVIL_PRODUCTION_VOLCENGINE_VIDEO_MODEL`.

**Input:** `A five second cinematic shot of an editing timeline animating on a monitor.`

**Expected Output:** Immediate queued/running state, provider task id in sanitized response summary, final video asset, playable preview, non-zero size.

### Audio Hold Verification

**Setup Required:** No audio model is required.

**Input:** Attempt to select or run Volcengine `text_to_audio`.

**Expected Output:** Volcengine audio generation is not exposed as an enabled model in Studio. If an API request is manually crafted, backend persists a failed job with `capability_mismatch` instead of calling a provider.

---

## Final Verification Set

Run after all tasks:

```bash
make sqlc-generate
make server-test
make server-build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

Run local app:

```bash
./scripts/dev-start.sh
```

Run mock browser smoke against the Vite URL printed by the script.

Run real smoke only when `.env` has valid Volcengine credentials:

```bash
CLIPANVIL_REAL_PROVIDER_SMOKE=1 scripts/smoke-real-volcengine-text.sh
CLIPANVIL_REAL_PROVIDER_SMOKE=1 scripts/smoke-real-volcengine-image.sh
CLIPANVIL_REAL_PROVIDER_SMOKE=1 scripts/smoke-real-volcengine-video.sh
```

---

## Spec Coverage Self-Review

- Real Volcengine text/image/video execution: covered by Tasks 4 through 6.
- Audio hold and disabled capability behavior: covered by Task 7.
- Eino-first execution layer: covered by Tasks 2, 4, 5, 6, and 7.
- All real model calls asynchronous: covered by Tasks 2, 3, 8, and E2E matrix.
- Studio real execution through ClipAnvil backend: covered by Tasks 3, 8, and 9.
- Mock retained for local tests: covered by Tasks 2 and 9.
- `ProviderBridge` not used as new core: covered by Tasks 2 and 4.
- `.env`-based secrets and model IDs: covered by Task 1 and Required Local Configuration.
- Delivery standards, acceptance standards, and E2E cases: included per task and in the E2E matrix.
