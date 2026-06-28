# Composer Agent Sandbox Video Composition Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to execute this plan task-by-task with a fresh worker per task, or use `superpowers:executing-plans` if executing inline in one session. Keep checkboxes updated as work lands.

**Goal:** Build an Eino-native Composer Agent that creates persisted timeline plans, renders final marketing videos in the sandbox with ffmpeg, persists final outputs through the shared production path, signals Producer when composition state changes, and renders final output data on the Agent Workbench canvas.

**Architecture:** Composer follows the current Craftsman/Reviewer shape: a bounded Eino-native ReAct graph, a role-specific executor, typed native tools, runtime-managed Agent threads/tasks, and Producer pending signals. The sandbox exposes media-focused tools rather than a general shell. Phase 1 deliberately opens a controlled `run_ffmpeg_command` tool so Composer can choose ffmpeg/ffprobe arguments inside `/workspace`, while production persistence remains centralized in the production service.

**Tech Stack:** Go 1.26, CloudWeGo Eino, pgx/sqlc, PostgreSQL 16, OpenSandbox, ffmpeg/ffprobe, MinIO-backed asset storage, React 19, TypeScript 6, Vite 8, `@xyflow/react` 12, TailwindCSS 4.

---

## Task 1: Add TimelinePlan Persistence

**Files:**
- Create: `apps/server/migrations/031_composer_timeline_plan.sql`
- Create: `apps/server/sqlc/queries/timeline_plan.sql`
- Modify generated files after sqlc: `apps/server/internal/store/db/*.sql.go`, `apps/server/internal/store/db/models.go`
- Create tests: `apps/server/internal/agent/composer/timeline_plan_contract_test.go`

**Step 1: Write a failing contract test**

- [x] Add `timeline_plan_contract_test.go` that reads the migration and query file from the repository and asserts the required table/query names are present.
- [x] Test expectations:
  - migration contains `CREATE TABLE timeline_plan`
  - migration contains `workspace_id`
  - migration contains `source_storyboard_node_id`
  - migration contains `status`
  - migration contains `plan_json`
  - query file contains `-- name: CreateTimelinePlan :one`
  - query file contains `-- name: GetTimelinePlan :one`
  - query file contains `-- name: ListTimelinePlansByWorkspace :many`
  - query file contains `-- name: UpdateTimelinePlanStatus :one`

Expected failure:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/agent/composer -run TimelinePlanContract -count=1
```

The test fails because the migration and query files do not exist yet.

**Step 2: Create the migration**

- [x] Create `apps/server/migrations/031_composer_timeline_plan.sql`.
- [x] Use the existing migration style from `025_m2_render_plan.sql` and `029_producer_pending_signal.sql`.
- [x] Define `timeline_plan` with these columns:

```sql
CREATE TABLE timeline_plan (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  source_storyboard_node_id UUID REFERENCES storyboard_node(id) ON DELETE SET NULL,
  output_node_id UUID REFERENCES storyboard_node(id) ON DELETE SET NULL,
  production_job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
  artifact_version_id UUID REFERENCES artifact_version(id) ON DELETE SET NULL,
  sandbox_job_id UUID REFERENCES sandbox_job(id) ON DELETE SET NULL,
  status TEXT NOT NULL CHECK (status IN ('draft', 'rendering', 'completed', 'blocked', 'failed')),
  template_key TEXT NOT NULL,
  plan_json JSONB NOT NULL,
  render_settings JSONB NOT NULL DEFAULT '{}'::jsonb,
  result JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message TEXT,
  created_by_role TEXT NOT NULL DEFAULT 'composer',
  created_by_task_id UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [x] Add useful indexes:
  - `(workspace_id, created_at DESC)`
  - `(workspace_id, status)`
  - `(production_job_id)` where not null
  - `(artifact_version_id)` where not null
  - `(sandbox_job_id)` where not null
- [x] Add an `updated_at` trigger using the existing project trigger helper if available in previous migrations. If the helper is not available, follow the existing table pattern used by recent migrations.

**Step 3: Add sqlc queries**

- [x] Create `apps/server/sqlc/queries/timeline_plan.sql`.
- [x] Include these queries:

```sql
-- name: CreateTimelinePlan :one
INSERT INTO timeline_plan (
  workspace_id,
  source_storyboard_node_id,
  output_node_id,
  production_job_id,
  artifact_version_id,
  sandbox_job_id,
  status,
  template_key,
  plan_json,
  render_settings,
  result,
  error_message,
  created_by_role,
  created_by_task_id
) VALUES (
  @workspace_id,
  @source_storyboard_node_id,
  @output_node_id,
  @production_job_id,
  @artifact_version_id,
  @sandbox_job_id,
  @status,
  @template_key,
  @plan_json,
  @render_settings,
  @result,
  @error_message,
  @created_by_role,
  @created_by_task_id
)
RETURNING *;

-- name: GetTimelinePlan :one
SELECT * FROM timeline_plan
WHERE id = @id;

-- name: ListTimelinePlansByWorkspace :many
SELECT * FROM timeline_plan
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC, id DESC
LIMIT @limit_count;

-- name: GetLatestCompletedTimelinePlanByWorkspace :one
SELECT * FROM timeline_plan
WHERE workspace_id = @workspace_id
  AND status = 'completed'
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: UpdateTimelinePlanStatus :one
UPDATE timeline_plan
SET
  status = @status,
  production_job_id = COALESCE(@production_job_id, production_job_id),
  artifact_version_id = COALESCE(@artifact_version_id, artifact_version_id),
  sandbox_job_id = COALESCE(@sandbox_job_id, sandbox_job_id),
  result = COALESCE(@result, result),
  error_message = @error_message,
  updated_at = now()
WHERE id = @id
RETURNING *;
```

- [x] If sqlc rejects `COALESCE` on nullable UUID or JSONB parameter typing, switch to separate status update queries instead of adding fragile casts.

**Step 4: Generate and verify**

- [x] Run:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/agent/composer -run TimelinePlanContract -count=1
```

- [x] Run:

```bash
git diff --check
```

Expected result: sqlc generation succeeds and the contract test passes.

---

## Task 2: Add Sandbox Media Composition Primitives

**Files:**
- Modify: `apps/server/internal/sandbox/job_service.go`
- Modify: `apps/server/internal/sandbox/exec.go`
- Create or modify tests: `apps/server/internal/sandbox/job_service_test.go`

**Step 1: Write failing sandbox tests**

- [x] Add tests for command validation without requiring a live OpenSandbox service.
- [x] Test `run_ffmpeg_command` validation:
  - accepts executable `ffmpeg`
  - accepts executable `ffprobe`
  - rejects `bash`
  - rejects `sh`
  - rejects absolute output paths outside `/workspace`
  - rejects path traversal such as `../secret.mp4`
  - enforces cwd under `/workspace`
- [x] Test staged media paths:
  - normalized staged files are under `/workspace/input`
  - duplicate filenames get deterministic unique names
  - returned manifest records `asset_id`, `workspace_path`, `mime_type`, and `size_bytes` when available

Expected failure:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/sandbox -run 'RunFFmpeg|StageMedia' -count=1
```

The tests fail because the new helpers do not exist.

**Step 2: Add typed sandbox inputs and results**

- [x] Add these structs near the existing sandbox job input types:

```go
type StageMediaInputsInput struct {
	WorkspaceID pgtype.UUID
	SandboxID   pgtype.UUID
	Assets      []StageMediaAssetInput
	TargetDir   string
}

type StageMediaAssetInput struct {
	AssetID    pgtype.UUID
	SourceURL  string
	FileName   string
	MimeType   string
	SizeBytes  int64
}

type StageMediaInputsResult struct {
	Files []StageMediaFile `json:"files"`
}

type StageMediaFile struct {
	AssetID       pgtype.UUID `json:"asset_id"`
	WorkspacePath string      `json:"workspace_path"`
	FileName      string      `json:"file_name"`
	MimeType      string      `json:"mime_type,omitempty"`
	SizeBytes     int64       `json:"size_bytes,omitempty"`
}

type ProbeMediaInput struct {
	WorkspaceID   pgtype.UUID
	SandboxID     pgtype.UUID
	WorkspacePath string
}

type ProbeMediaResult struct {
	Format  map[string]any   `json:"format"`
	Streams []map[string]any `json:"streams"`
}

type RunFFmpegCommandInput struct {
	WorkspaceID pgtype.UUID
	SandboxID   pgtype.UUID
	Cwd         string
	Executable  string
	Args        []string
	TimeoutSec  int
}
```

- [x] Keep these as sandbox package types. Agent native tools can wrap them with role-specific schemas later.

**Step 3: Implement staging**

- [x] Add `StageMediaInputs(ctx, input)` to `JobService`.
- [x] Use existing transfer/download logic rather than duplicating MinIO download code.
- [x] Default `TargetDir` to `/workspace/input`.
- [x] Ensure the target directory is created by an OpenSandbox exec request before downloads.
- [x] Stage each asset into `/workspace/input/<safe-name>`.
- [x] Sanitize file names by preserving a safe extension and replacing unsafe characters with `-`, matching existing sandbox naming.
- [x] For duplicated file names, use a stable suffix such as `-<short-asset-id>`.
- [x] Return the staged manifest.

**Step 4: Implement media probing**

- [x] Add `ProbeMedia(ctx, input)` to `JobService`.
- [x] Validate `WorkspacePath` is inside `/workspace`.
- [x] Execute:

```bash
ffprobe -v error -print_format json -show_format -show_streams <workspace-path>
```

- [x] Parse stdout as JSON into `ProbeMediaResult`.
- [x] Return a clear error when ffprobe emits invalid JSON or exits non-zero.

**Step 5: Implement controlled ffmpeg command execution**

- [x] Add `RunFFmpegCommand(ctx, input)` to `JobService`.
- [x] Validate:
  - `Executable` is exactly `ffmpeg` or `ffprobe`
  - `Cwd` is empty or under `/workspace`
  - every argument that looks like a path is relative to `Cwd` or absolute under `/workspace`
  - timeout is capped by the existing sandbox max timeout
- [x] Build the command without joining untrusted shell text. Prefer the existing exec request path only if it can preserve argument boundaries; otherwise add a safe quote helper that emits single-quoted shell args.
- [x] Persist a `sandbox_job` row with operation `run_ffmpeg_command`.
- [x] Return stdout, stderr, exit code, duration, sandbox job id, and any file outputs declared by the caller or inferred from args.

**Step 6: Verify sandbox package**

- [x] Run:

```bash
gofmt -w apps/server/internal/sandbox
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/sandbox -run 'RunFFmpeg|StageMedia|ProbeMedia' -count=1
```

- [x] Run:

```bash
git diff --check
```

Expected result: sandbox validation and command construction tests pass.

---

## Task 3: Add Production Helper for Composer Final Artifacts

**Files:**
- Modify: `apps/server/internal/production/service.go`
- Modify tests: `apps/server/internal/production/service_test.go`

**Step 1: Write a failing production service test**

- [x] Add a test that exercises a new production helper with a synthetic provider result containing:
  - output asset id
  - output URI
  - mime type `video/mp4`
  - provider metadata with `sandbox_job_id`
- [x] Assert the helper:
  - creates or updates the target output node through the same production persistence path as normal provider completion
  - creates an `artifact_version`
  - links `generation_job.sandbox_job_id`
  - returns a `RunResult` usable by Agent callers

Expected failure:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/production -run ComposerArtifact -count=1
```

The test fails because the helper does not exist.

**Step 2: Add a public helper on production.Service**

- [x] Add a method with a narrow Composer-oriented name:

```go
type ComposerArtifactInput struct {
	WorkspaceID    pgtype.UUID
	OutputNodeID   pgtype.UUID
	Intent         GenerationIntent
	ProviderResult ProviderResult
	SandboxJobID   pgtype.UUID
	TaskID         pgtype.UUID
}

func (s *Service) PersistComposerArtifact(ctx context.Context, input ComposerArtifactInput) (RunResult, error)
```

- [x] Reuse existing internal persistence helpers for job completion and artifact version creation.
- [x] Do not expose Studio run APIs to Agent code.
- [x] Ensure the helper can persist a Composer-owned final video without submitting a second asynchronous provider job.
- [x] If the existing private helpers require a `generation_job`, create one with a Composer-specific intent and immediately persist success through the same success path.

**Step 3: Keep the provider contract compatible**

- [x] Keep `internal_ffmpeg` provider behavior working for existing code.
- [x] Do not delete `ComposeVideos` from `InternalFFmpegProvider` in this task.
- [x] Ensure the new helper accepts sandbox output from both the controlled ffmpeg tool and the existing `ComposeVideos` operation.

**Step 4: Verify production package**

- [x] Run:

```bash
gofmt -w apps/server/internal/production
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/production -run ComposerArtifact -count=1
```

- [x] Run:

```bash
git diff --check
```

Expected result: the Composer artifact helper test passes and existing production tests still compile.

---

## Task 4: Replace Legacy Composer With Native Tool Contracts

**Files:**
- Replace or heavily modify: `apps/server/internal/agent/composer/types.go`
- Create: `apps/server/internal/agent/composer/context_loader.go`
- Create: `apps/server/internal/agent/composer/system_prompt.go`
- Create native tools under `apps/server/internal/agent/tools/`:
  - `dispatch_composer_native.go`
  - `composer_context_native.go`
  - `composer_stage_media_native.go`
  - `composer_probe_media_native.go`
  - `composer_timeline_plan_native.go`
  - `composer_render_native.go`
  - `composer_ffmpeg_native.go`
  - `composer_submit_artifact_native.go`
- Keep for compatibility until main wiring is updated: `apps/server/internal/agent/tools/compose_final.go`
- Create tests:
  - `apps/server/internal/agent/composer/types_test.go`
  - `apps/server/internal/agent/tools/composer_tools_test.go`

**Step 1: Write failing Composer type/tool contract tests**

- [x] Assert Composer request and result types include:
  - workspace id
  - task id
  - thread id
  - source storyboard node id
  - timeline plan id
  - output node id
  - artifact version id
  - sandbox job id
  - status values `completed`, `blocked`, `failed`
- [x] Assert the native registry can register Composer tools:
  - `get_composition_context`
  - `stage_media_inputs`
  - `probe_media`
  - `create_timeline_plan`
  - `update_timeline_plan_status`
  - `render_timeline_template`
  - `run_ffmpeg_command`
  - `submit_composition_artifact`

Expected failure:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/agent/composer ./apps/server/internal/agent/tools -run 'Composer|Composition' -count=1
```

The tests fail because the new native contracts do not exist.

**Step 2: Define Composer domain types**

- [x] Add role-specific request and result structs:

```go
type Request struct {
	WorkspaceID            pgtype.UUID
	TaskID                 pgtype.UUID
	ThreadID               pgtype.UUID
	SourceStoryboardNodeID pgtype.UUID
	Instructions           string
}

type Result struct {
	Status            Status
	TimelinePlanID    pgtype.UUID
	OutputNodeID      pgtype.UUID
	ArtifactVersionID pgtype.UUID
	SandboxJobID      pgtype.UUID
	Summary           string
	ErrorMessage      string
}

type Status string

const (
	StatusCompleted Status = "completed"
	StatusBlocked   Status = "blocked"
	StatusFailed    Status = "failed"
)
```

- [x] Add typed timeline plan structs for template phase 1:

```go
type TimelinePlan struct {
	TemplateKey string         `json:"template_key"`
	Segments    []Segment      `json:"segments"`
	Transitions []Transition   `json:"transitions,omitempty"`
	Output      OutputSettings `json:"output"`
}

type Segment struct {
	ID            string  `json:"id"`
	AssetID       string  `json:"asset_id"`
	WorkspacePath string  `json:"workspace_path,omitempty"`
	StartSec      float64 `json:"start_sec,omitempty"`
	DurationSec   float64 `json:"duration_sec,omitempty"`
}

type Transition struct {
	FromSegmentID string  `json:"from_segment_id"`
	ToSegmentID   string  `json:"to_segment_id"`
	Type          string  `json:"type"`
	DurationSec   float64 `json:"duration_sec"`
}

type OutputSettings struct {
	WorkspacePath string `json:"workspace_path"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	FPS           int    `json:"fps,omitempty"`
	Format        string `json:"format"`
}
```

**Step 3: Add Composer context loader**

- [x] Implement `context_loader.go` that reads:
  - workspace metadata
  - storyboard nodes relevant to composition
  - selected source node
  - available successful video/image/audio artifact versions
  - latest review state if available
  - existing timeline plans for the workspace
  - sandbox id or sandbox availability for the workspace
- [x] Keep this read-only. Composer mutations happen through native tools.

**Step 4: Add Composer system prompt**

- [x] The prompt must state:
  - Composer is a final video editor, not story planner and not asset generator
  - prefer simple concat or fades templates in phase 1
  - stage all media before probing/rendering
  - probe inputs before choosing ffmpeg filters when duration or dimensions matter
  - create a timeline plan before rendering
  - submit the final artifact only after a render exists
  - return `blocked` with a precise reason if inputs are missing or unusable
  - use `run_ffmpeg_command` only for ffmpeg/ffprobe work inside `/workspace`

**Step 5: Implement native tools**

- [x] `get_composition_context`: returns the context loader output.
- [x] `stage_media_inputs`: resolves source assets and stages them into sandbox `/workspace/input`.
- [x] `probe_media`: wraps sandbox `ProbeMedia`.
- [x] `create_timeline_plan`: validates `template_key` is `simple_concat` or `concat_with_fades`, persists `timeline_plan` status `draft`.
- [x] `update_timeline_plan_status`: updates status and result fields.
- [x] `render_timeline_template`: deterministic renderer for phase 1 templates:
  - `simple_concat` uses concat demuxer when codecs match or re-encodes when needed
  - `concat_with_fades` uses ffmpeg filter_complex xfade/acrossfade when video streams are compatible
  - stores output under `/workspace/output/final.mp4`
- [x] `run_ffmpeg_command`: wraps sandbox `RunFFmpegCommand` and returns stdout/stderr/job metadata.
- [x] `submit_composition_artifact`: validates output path under `/workspace/output`, uploads via sandbox artifact submission or existing transfer path, then calls `production.Service.PersistComposerArtifact`.

**Step 6: Verify Composer type/tool contracts**

- [x] Run:

```bash
gofmt -w apps/server/internal/agent/composer apps/server/internal/agent/tools
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/agent/composer ./apps/server/internal/agent/tools -run 'Composer|Composition' -count=1
```

- [x] Run:

```bash
git diff --check
```

Expected result: native Composer contracts compile and tests pass.

---

## Task 5: Implement Eino-Native Composer Graph and Executor

**Files:**
- Replace: `apps/server/internal/agent/composer/graph.go`
- Replace: `apps/server/internal/agent/composer/executor.go`
- Modify tests:
  - `apps/server/internal/agent/composer/graph_test.go`
  - `apps/server/internal/agent/composer/executor_test.go`

**Step 1: Write failing graph loop tests**

- [ ] Mirror the Craftsman/Reviewer native loop test style.
- [ ] Test cases:
  - exits after `submit_composition_artifact` returns a completed result
  - exits blocked when no usable media inputs are found
  - stops at max tool iterations and returns failed status
  - preserves tool trace records for native tool execution
  - does not call legacy `compose_final`

Expected failure:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/agent/composer -run 'Graph|Executor' -count=1
```

**Step 2: Build the bounded native tool graph**

- [ ] Use the same graph shape as Craftsman/Reviewer:
  - load context
  - prepare model input
  - call model
  - parse tool calls
  - execute native tools
  - append tool results
  - finalize result
- [ ] Set an explicit maximum tool iteration count. Use the local pattern if Craftsman/Reviewer already defines a constant; otherwise set Composer to 12 iterations.
- [ ] Use `tools.NativeRegistry` and `tools.NativeRuntimeContext`.
- [ ] Persist thread messages through the existing Agent runtime service.
- [ ] Use Eino graph/checkpointing in the same style as active role graphs.

**Step 3: Implement executor**

- [ ] Executor input comes from runtime Composer task payloads.
- [ ] Use `runtime.Service.GetOrCreateComposerThread`.
- [ ] Call the Composer graph with workspace/task/thread/source node/instructions.
- [ ] On completion, create Producer pending signal:
  - `signal_type = composition_completed`
  - payload includes `timeline_plan_id`, `output_node_id`, `artifact_version_id`, `sandbox_job_id`, and summary
  - `dedupe_key = composer:<task-id>:completed`
- [ ] On blocked:
  - `signal_type = composition_blocked`
  - payload includes reason and missing input description
  - `dedupe_key = composer:<task-id>:blocked`
- [ ] On failed:
  - `signal_type = composition_failed`
  - payload includes error message
  - `dedupe_key = composer:<task-id>:failed`
- [ ] Mark the runtime task completed or failed consistently with Craftsman/Reviewer behavior.

**Step 4: Verify graph and executor**

- [ ] Run:

```bash
gofmt -w apps/server/internal/agent/composer
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/agent/composer -run 'Graph|Executor' -count=1
```

- [ ] Run:

```bash
git diff --check
```

Expected result: Composer graph and executor tests pass.

---

## Task 6: Wire Producer Dispatch and Server Startup

**Files:**
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/agent/producer/graph.go`
- Modify or create Producer tests:
  - `apps/server/internal/agent/producer/graph_test.go`
  - `apps/server/internal/agent/tools/dispatch_composer_native_test.go`

**Step 1: Write failing dispatch tests**

- [ ] Add tests that confirm Producer native registry includes `dispatch_composer`.
- [ ] Add tests that confirm dispatch creates a Composer task payload with:
  - workspace id
  - source storyboard node id
  - instructions
  - requested template key when provided
- [ ] Add tests that Producer reminder formatting includes `composition_completed`, `composition_blocked`, and `composition_failed`.

Expected failure:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/agent/producer ./apps/server/internal/agent/tools -run 'Composer|Composition|Dispatch' -count=1
```

**Step 2: Register Composer executor in main**

- [ ] Replace the current unused Composer executor wiring in `main.go` with active dependencies.
- [ ] Construct Composer with:
  - llm client
  - runtime service
  - native Composer tool registry
  - sandbox job service
  - production service
  - store/query access already available to other role agents
- [ ] Pass Composer enqueue/dispatch dependencies into the Producer native registry.
- [ ] Remove dead `_ = composerExecutor` style placeholders.

**Step 3: Add `dispatch_composer` native tool**

- [ ] Schema fields:

```json
{
  "source_storyboard_node_id": "uuid",
  "instructions": "string",
  "template_key": "simple_concat"
}
```

- [ ] Allow `template_key` values:
  - `simple_concat`
  - `concat_with_fades`
- [ ] Create a Composer task via runtime service with role `composer`.
- [ ] Return task id, thread id when available, and a short accepted summary.

**Step 4: Update Producer signal handling**

- [ ] Include Composer signals in pending signal reminder text.
- [ ] Ensure `composition_completed` lets Producer know a final output is available.
- [ ] Ensure blocked/failed signals remain visible until Producer handles them.
- [ ] Avoid marking Composer completion as a Worker generation completion.

**Step 5: Verify dispatch wiring**

- [ ] Run:

```bash
gofmt -w apps/server/cmd/server apps/server/internal/agent/producer apps/server/internal/agent/tools
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/agent/producer ./apps/server/internal/agent/tools -run 'Composer|Composition|Dispatch' -count=1
```

- [ ] Run:

```bash
make server-build
git diff --check
```

Expected result: server builds and Producer can dispatch Composer through a native tool.

---

## Task 7: Add Backend Workbench Projection for Final Output

**Files:**
- Modify: `apps/server/internal/api/agent_workbench_projection.go`
- Modify: `apps/server/internal/api/agent_canvas_detail.go`
- Modify tests:
  - `apps/server/internal/api/agent_workbench_projection_test.go`
  - `apps/server/internal/api/agent_canvas_detail_test.go`

**Step 1: Write failing projection tests**

- [ ] Add a workbench projection test with:
  - one completed `timeline_plan`
  - one output node
  - one artifact version
  - one linked sandbox job
- [ ] Assert the workbench response includes `final_output`.
- [ ] Assert `final_output` includes:
  - timeline plan id
  - output node id
  - artifact version id
  - asset url or asset id
  - status
  - template key
  - render summary
  - sandbox job id
- [ ] Add a detail endpoint test for selecting the final output item.

Expected failure:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/api -run 'AgentWorkbench|AgentCanvasDetail' -count=1
```

**Step 2: Extend API response types**

- [ ] Add:

```go
type agentWorkbenchFinalOutputResponse struct {
	ID                string         `json:"id"`
	TimelinePlanID    string         `json:"timeline_plan_id"`
	OutputNodeID      string         `json:"output_node_id,omitempty"`
	ArtifactVersionID string         `json:"artifact_version_id,omitempty"`
	SandboxJobID      string         `json:"sandbox_job_id,omitempty"`
	Status            string         `json:"status"`
	TemplateKey       string         `json:"template_key"`
	Summary           string         `json:"summary,omitempty"`
	AssetURL          string         `json:"asset_url,omitempty"`
	AssetID           string         `json:"asset_id,omitempty"`
	Plan              map[string]any `json:"plan,omitempty"`
	Result            map[string]any `json:"result,omitempty"`
	UpdatedAt         time.Time      `json:"updated_at"`
}
```

- [ ] Add `FinalOutput *agentWorkbenchFinalOutputResponse` to the workbench response.

**Step 3: Build final output projection**

- [ ] Query the latest completed timeline plan for the workspace.
- [ ] If no completed timeline exists, return the latest non-completed timeline plan only when it has status `rendering`, `blocked`, or `failed`.
- [ ] Resolve artifact version and asset metadata using existing projection helpers.
- [ ] Resolve sandbox job id directly from the timeline plan when present.
- [ ] Do not infer final output from Studio run APIs.

**Step 4: Add final output detail response**

- [ ] Add detail support for object type `final_output`.
- [ ] Include timeline plan JSON, render result, artifact info, and sandbox job info.
- [ ] Keep detail response shape consistent with existing Agent canvas details.

**Step 5: Verify API**

- [ ] Run:

```bash
gofmt -w apps/server/internal/api
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/api -run 'AgentWorkbench|AgentCanvasDetail' -count=1
```

- [ ] Run:

```bash
git diff --check
```

Expected result: API tests pass and workbench responses include final output data.

---

## Task 8: Render Final Output on the Frontend Workbench Canvas

**Files:**
- Modify: `apps/web/src/lib/agentWorkbench.ts`
- Modify: `apps/web/src/lib/agentWorkbenchViewModel.ts`
- Modify: `apps/web/src/lib/agentWorkbenchSelection.ts`
- Modify: `apps/web/src/components/agent-workbench/AgentWorkbenchCanvas.tsx`
- Modify: `apps/web/src/components/agent-workbench/AgentCanvasDetailPanel.tsx`
- Create: `apps/web/src/components/agent-workbench/AgentFinalOutputNode.tsx`
- Modify: `apps/web/src/main.css`
- Modify tests:
  - `apps/web/src/lib/agentWorkbenchViewModel.test.mjs`
  - `apps/web/src/lib/agentWorkbenchSelection.test.mjs`
  - `apps/web/src/lib/agentCanvas.test.mjs`

**Step 1: Write failing frontend model tests**

- [ ] Add fixture data with `final_output`.
- [ ] Assert the view model creates one `agentFinalOutput` node.
- [ ] Assert the final output node is placed after the shot/storyboard lane, not inside scene cards.
- [ ] Assert selection key parsing supports `final_output:<id>`.
- [ ] Assert canvas test knows the `agentFinalOutput` node type.

Expected failure:

```bash
pnpm --filter @clip-anvil/web test -- agentWorkbenchViewModel
```

If the web package does not expose a matching targeted test command, run the existing MJS test directly with the same node command used by nearby tests.

**Step 2: Extend TypeScript API types**

- [ ] Add `AgentWorkbenchFinalOutput` to `agentWorkbench.ts`.
- [ ] Add optional `final_output?: AgentWorkbenchFinalOutput | null` to the workbench response type.
- [ ] Keep snake_case JSON fields aligned with backend output.

**Step 3: Add final output view model node**

- [ ] Add a node type such as:

```ts
export type AgentWorkbenchNodeType =
  | "agentOverview"
  | "agentScene"
  | "agentShot"
  | "agentFinalOutput";
```

- [ ] Create node data with:
  - status
  - template key
  - summary
  - asset url
  - timeline plan id
  - artifact version id
  - sandbox job id
- [ ] Position it as a stable final lane after storyboard/shot content.
- [ ] Add stable dimensions so long status text or file names do not resize the layout.

**Step 4: Add final output node component**

- [ ] Create `AgentFinalOutputNode.tsx`.
- [ ] Show a video preview when `asset_url` is present.
- [ ] Show status, template key, and concise summary.
- [ ] Use existing workbench visual conventions.
- [ ] Avoid nested cards and avoid feature-explaining text.
- [ ] Use icon buttons with lucide icons if controls are needed.

**Step 5: Add detail panel support**

- [ ] Extend selection type with `final_output`.
- [ ] Fetch detail through existing detail endpoint conventions.
- [ ] Render:
  - video preview when asset URL exists
  - timeline plan status/template
  - artifact version id
  - sandbox job id
  - plan/result JSON in the same compact style as existing technical details

**Step 6: Verify frontend**

- [ ] Run the targeted tests added in this task.
- [ ] Run:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected result: web build and lint pass and final output is visible in the workbench model.

---

## Task 9: End-to-End Verification and Smoke Coverage

**Files:**
- Create if useful: `scripts/smoke-composer-agent.sh`
- Modify docs if behavior changed: `docs/engineering/agent-multiagent-architecture.md`
- Modify docs if sandbox contract changed: `docs/engineering/sandbox.md`

**Step 1: Add a smoke path when existing smoke utilities allow it**

- [ ] Prefer an automated smoke script only if test fixture creation is already available in the repo.
- [ ] Use workspace `7dfbaee8-2a4c-4449-8107-2203d1e31592` for the manual Composer smoke after development is complete; this workspace already contains generated storyboard data.
- [ ] The smoke should:
  - create or reuse a workspace with two video assets
  - create a storyboard/source node that references those assets
  - dispatch Composer with `simple_concat`
  - wait for a Composer result
  - assert a completed `timeline_plan`
  - assert a final artifact version exists
  - assert a Producer pending signal of type `composition_completed`
  - fetch workbench projection and assert `final_output` exists
- [ ] If fixture setup is not available, document a manual smoke command sequence in the script comments and keep the script executable only when it can run reliably.

**Step 2: Run full backend checks**

- [ ] Run:

```bash
make sqlc-generate
make server-build
make server-test
```

- [ ] If Go cache permission fails, rerun Go commands with:

```bash
GOCACHE=/private/tmp/clipanvil-go-build
```

**Step 3: Run frontend checks**

- [ ] Run:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

**Step 4: Run docs/whitespace checks**

- [ ] Run:

```bash
git diff --check
```

**Step 5: Review architecture consistency**

- [ ] Confirm Composer implementation is Eino-native and does not depend on legacy linear `composer_final`.
- [ ] Confirm Producer dispatches Composer through a native tool and sees Composer completion through pending signals.
- [ ] Confirm final artifact persistence goes through production service helpers.
- [ ] Confirm sandbox execution is constrained to ffmpeg/ffprobe and `/workspace`.
- [ ] Confirm Workbench canvas renders final output from backend projection, not from client-only inference.

Expected result: the repo has a working Phase 1 Composer path from Producer dispatch to sandbox render, production persistence, Producer signal, and Workbench final output projection.

---

## Implementation Notes

- Preserve the Studio/Agent boundary. Agent Composer should call production services and sandbox services directly through role tools, not Studio user run APIs.
- Treat `timeline_plan` as the durable plan/result record. The frontend should render backend projection data, not reconstruct final output state from scattered client fields.
- Keep `stage_media_inputs` as an explicit tool. Do not implicitly sync all MinIO assets into sandbox initialization; Composer should stage only the assets needed for the chosen composition.
- Keep `run_ffmpeg_command` controlled. Phase 1 allows Agent-chosen ffmpeg/ffprobe args, but not arbitrary shell commands.
- Use `simple_concat` and `concat_with_fades` as the first supported templates. More advanced marketing edits such as logo overlays, subtitles, beat sync, CTA cards, color correction, and platform-specific reframing can be added after the basic render/persist/signal loop is stable.
- Remove legacy Composer code only after native Composer tests and server wiring pass. If deletion creates a large diff, do it as a small cleanup task after Task 6 rather than mixing it into early database or sandbox changes.

---

## Plan Self-Review

- [ ] The plan starts from durable data (`timeline_plan`) before graph/tool behavior, so Composer output has a backend source of truth.
- [ ] Composer follows Craftsman/Reviewer architecture: role executor, bounded native graph, typed native tools, runtime task/thread ownership, and Producer pending signals.
- [ ] Sandbox access is narrow: Composer gets media staging, probing, template rendering, and controlled ffmpeg/ffprobe execution under `/workspace`, not arbitrary shell.
- [ ] Final artifact persistence goes through `production.Service`, keeping Agent code out of Studio-only user run APIs.
- [ ] The frontend renders backend workbench projection data, preserving the current rule that React Flow is a projection layer.
- [ ] Verification covers sqlc, Go build/test, frontend build/lint, and whitespace checks.

---

## Final Verification Matrix

- [ ] `make sqlc-generate`
- [ ] `make server-build`
- [ ] `make server-test`
- [ ] `pnpm --filter @clip-anvil/web... build`
- [ ] `pnpm --filter @clip-anvil/web lint`
- [ ] `git diff --check`

The implementation is complete when the checks above pass and a Composer task can produce a completed `timeline_plan`, a persisted final video artifact, a Producer pending signal, and a visible Workbench final output node.
