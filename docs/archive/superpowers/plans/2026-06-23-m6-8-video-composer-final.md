# M6.8 Video Composer Final Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the M6.8 path from accepted preview images to shot videos, reviewed video winners, sandbox-backed final composition, and final video confirmation in Agent mode.

**Architecture:** Keep Eino Graph as the explicit orchestration layer. Reuse the M6.6/M6.7 `Producer -> Craftsman -> Worker -> production.GenerationIntent -> generation_job/artifact_version` path for shot video, and add a first-class `ComposerGraph` that submits final composition through `internal_ffmpeg` provider backed by `sandbox.JobService`. Do not introduce a parallel final-artifact table; final video is a normal Agent-owned video node with job/version/sandbox trace.

**Tech Stack:** Go 1.26, CloudWeGo Eino Graph, pgx/sqlc, existing Agent runtime tables, existing M4/M5 production service, existing sandbox job service, React 19/Vite Agent UI typed message blocks.

---

## Scope

M6.8 implements two slices:

- **M6.8A Shot Video**: `generate_shot_video` tool, phase-aware Craftsman/Worker, video node creation, `image_to_video` `GenerationIntent`, shot video terminal events, shot video review.
- **M6.8B Composer / Final Video**: `compose_final` tool, ComposerGraph, final video node, `internal_ffmpeg` `compose_final_video` operation through sandbox, final video card, final HITL confirmation.

M6.8 does not implement:

- Studio / Agent import-export.
- Direct user editing of Agent canvas.
- Full M6 UX Completion status bar/storyboard persistent panels beyond the minimal final/review/video rendering needed to verify this stage.
- Multi-user collaboration.
- Running FFmpeg in the app process.

## Current Code Facts

- `apps/server/internal/agent/worker` currently only accepts `mode == "preview_image"` and creates image nodes with `operation_type="text_to_image"`.
- `apps/server/internal/agent/craftsman` currently only asks for `preview_prompt`.
- `apps/server/internal/production` already supports `OutputType="video"` and mock video output.
- `VolcengineVideoRuntime` already supports async video tasks and provider-reachable image inputs.
- `sandbox.JobService` already supports sandbox execution and frame extraction.
- `InternalFFmpegProvider` currently supports `extract_first_frame` and `extract_last_frame`, but not final video composition.
- `production.Service` already links `sandbox_job` to `generation_job` when provider response contains `sandbox_job_id`.
- `review_record.target_phase` already allows `shot_video` and `final_video`.
- Agent runtime allows `role='composer'` and `scope_type='final_output'`, but `agent_task.task_type` does not yet allow `composer_turn`.
- Agent UI already supports media blocks for `video` and `final_video`, and review cards for `target_phase='shot_video' | 'final_video'`.

## File Map

### Database / sqlc

- Create `apps/server/migrations/021_m6_8_video_composer.sql`
  - Extend `agent_task_type_check` with `composer_turn`.
  - Add model capability for `internal_ffmpeg` `compose_final_video`.
  - Optionally add `final_output_id` metadata only through JSON; no new table.
- Modify `apps/server/sqlc/queries/agent_task.sql`
  - Add `ListQueuedComposerTasksAcrossWorkspaces`.
- Run `make sqlc-generate`.

### Agent runtime

- Modify `apps/server/internal/agent/runtime/service.go`
  - Accept `composer_turn`.
  - Add `GetOrCreateComposerThread`.
  - Add `ListQueuedComposerTasksAcrossWorkspaces`.
- Modify `apps/server/internal/agent/runtime/service_test.go`.

### Craftsman

- Modify `apps/server/internal/agent/craftsman/types.go`
  - Add `Mode` to `GraphInput`.
  - Replace preview-only `Strategy` with phase-aware fields while preserving preview compatibility.
  - Add `VideoPrompt`, `OperationType`, and `OutputType`.
- Modify `apps/server/internal/agent/craftsman/graph.go`
  - Compile as generic `craftsman_generation`.
  - Pass `mode` into Worker input.
- Modify `apps/server/internal/agent/craftsman/model_responder.go`
  - Support `DraftGenerationStrategy` for `preview_image` and `shot_video`.
- Modify tests in `apps/server/internal/agent/craftsman/*_test.go`.

### Worker

- Modify `apps/server/internal/agent/worker/types.go`
  - Extend `GenerationInput` for `shot_video`.
  - Add `TargetPhase`, `OutputType`, `OperationType`.
- Modify `apps/server/internal/agent/worker/executor.go`
  - `preview_image`: current behavior.
  - `shot_video`: create Agent-owned video node, use accepted/current preview winner as input, submit `OutputType="video"` and `OperationType="image_to_video"` unless strategy overrides compatible operation.
  - Emit `shot_video_submitted` in addition to generic worker event.
- Add `apps/server/internal/agent/video/status.go`
  - Stable shot video status reducer.
- Modify `apps/server/internal/agent/worker/input_refs.go`
  - Support role-aware refs later, but M6.8 only requires video generation to resolve the preview image node/version.
- Modify worker tests.

### Edges

- Create `apps/server/internal/agent/tools/generate_shot_video.go`
  - Producer-facing tool to schedule shot video generation.
  - Resolves shot refs like `dispatch_craftsman`.
  - Requires accepted preview review or current preview winner, depending policy.
  - Creates shot-scoped `craftsman_turn` with `mode='shot_video'`.
- Create `apps/server/internal/agent/tools/compose_final.go`
  - Producer-facing tool to schedule final composition.
  - Resolves ordered shots and selected video winners.
  - Creates final-output scoped `composer_turn`.
- Modify `apps/server/internal/agent/tools/registry_test.go`.

### Production terminal bridge

- Modify `apps/server/internal/api/production_broadcaster.go`
  - Generalize preview-only terminal event publishing to Agent production terminal events:
    - `preview_generation_succeeded` / `preview_generation_failed`
    - `shot_video_succeeded` / `shot_video_failed`
    - `composition_succeeded` / `composition_failed`
  - Update shot status for video nodes.
  - Broadcast canvas `NodeUpdated` as today.
- Modify `apps/server/internal/api/production_broadcaster_test.go`.

### Reviewer

- Modify `apps/server/internal/agent/reviewer/context_loader.go`
  - Load video artifact URL/metadata for `target_phase='shot_video'`.
- Modify `apps/server/internal/agent/reviewer/model_responder.go`
  - Prompt video review rubric when target is video.
- Modify `apps/server/internal/agent/tools/review_shot.go`
  - Accept `target_phase='shot_video'`.
- Modify reviewer and tool tests.

### Composer

- Create `apps/server/internal/agent/composer/types.go`
  - Graph input/output, composition plan, selected shot videos.
- Create `apps/server/internal/agent/composer/context_loader.go`
  - Load ordered shots and current video winners.
- Create `apps/server/internal/agent/composer/graph.go`
  - Eino Graph:
    `load_composition_context -> draft_composition_plan -> validate_assets_ready -> create_final_video_node -> submit_final_generation_intent -> request_final_hitl`
- Create `apps/server/internal/agent/composer/executor.go`
  - Runs `composer_turn`.
- Create `apps/server/internal/agent/composer/model_responder.go`
  - Static responder for tests, Volcengine responder optional for composition metadata.
- Add composer tests.

### Sandbox / internal FFmpeg

- Modify `apps/server/internal/sandbox/job_service.go`
  - Add `ComposeVideos(ctx, ComposeVideosInput)`.
  - Stage video assets into sandbox via presigned GET.
  - Run FFmpeg concat/composition command.
  - Upload final MP4 through presigned PUT.
- Modify `apps/server/internal/production/internal_ffmpeg_provider.go`
  - Support `compose_final_video`.
  - Return `ProviderResult{AssetStorageURL, AssetMIME:"video/mp4", ProviderResponse:{"sandbox_job_id":...}}`.
- Modify tests for sandbox job service and internal FFmpeg provider.

### UI message protocol / frontend

- Modify `apps/server/internal/agent/uimessage/blocks.go`
  - Add `FinalVideoCardBlock`.
- Modify `apps/web/src/lib/agentMessageBlocks.ts`
  - Add `final_video_card` block type guard.
- Create `apps/web/src/components/agent/AgentFinalVideoCardBlock.tsx`
  - Render final video player, source shots, version, confirmation status.
- Modify `apps/web/src/components/agent/AgentMessageRenderer.tsx`.
- Modify `apps/web/src/components/agent/AgentNodeDetailDrawer.tsx`
  - Show sandbox jobs for final video node.
- Modify `apps/web/src/lib/api.ts`
  - Ensure sandbox jobs and review records are typed.

### PSS / API projection

- Modify `apps/server/internal/agent/pss/producer.go`
  - Include shot video nodes/versions/reviews and final output section.
- Modify `apps/server/internal/api/run_handler.go`
  - Existing production-state already includes sandbox jobs; ensure final nodes expose them.

### Main wiring

- Modify `apps/server/cmd/server/main.go`
  - Wire new tools.
  - Wire ComposerGraph executor/enqueuer/recovery.
  - Pass sandbox-backed internal FFmpeg provider already registered.

### Smoke / E2E

- Create `scripts/smoke-m6-8-video-composer.sh`
  - API-level smoke to create Agent workspace and send a request that reaches video/composer tools.
- Browser E2E through `./scripts/dev-start.sh`.

---

## Task 1: Runtime And Capability Foundation

**Files:**
- Create: `apps/server/migrations/021_m6_8_video_composer.sql`
- Modify: `apps/server/sqlc/queries/agent_task.sql`
- Modify: `apps/server/internal/agent/runtime/service.go`
- Modify: `apps/server/internal/agent/runtime/service_test.go`

- [ ] **Step 1: Write failing runtime tests**

Add tests asserting:

```go
func TestCreateTaskAllowsComposerTurn(t *testing.T) {
    svc := testRuntimeService(t)
    task, err := svc.CreateTask(context.Background(), agentruntime.CreateTaskParams{
        WorkspaceID: uuidWithByte(1),
        ThreadID: uuidWithByte(2),
        Role: "composer",
        ScopeType: "final_output",
        TaskType: "composer_turn",
        MaxAttempts: 1,
        Input: []byte(`{"shot_ids":["shot-01"]}`),
    })
    if err != nil {
        t.Fatal(err)
    }
    if task.Role != "composer" || task.TaskType != "composer_turn" {
        t.Fatalf("task = %#v", task)
    }
}
```

- [ ] **Step 2: Verify the test fails**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/runtime -run TestCreateTaskAllowsComposerTurn -count=1
```

Expected: FAIL because `composer_turn` is not accepted by `validTaskType` and DB constraint.

- [ ] **Step 3: Add migration**

Create `021_m6_8_video_composer.sql`:

```sql
-- +goose Up
ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume',
        'craftsman_turn',
        'worker_generation',
        'reviewer_turn',
        'dependency_scheduler',
        'composer_turn'
    ));

INSERT INTO model_capability (
    provider_id,
    model_id,
    display_name,
    output_types,
    supported_operations,
    supported_input_node_types,
    limits,
    pricing,
    defaults,
    enabled
) VALUES (
    'internal_ffmpeg',
    'ffmpeg-compose',
    'Internal FFmpeg Compose',
    '["video"]',
    '["compose_final_video"]',
    '["video", "audio"]',
    '{"max_attempts": 1}',
    '{"tier": "internal"}',
    '{"format": "mp4"}',
    true
) ON CONFLICT (provider_id, model_id) DO UPDATE
SET display_name = EXCLUDED.display_name,
    output_types = EXCLUDED.output_types,
    supported_operations = EXCLUDED.supported_operations,
    supported_input_node_types = EXCLUDED.supported_input_node_types,
    limits = EXCLUDED.limits,
    pricing = EXCLUDED.pricing,
    defaults = EXCLUDED.defaults,
    enabled = true,
    updated_at = now();

-- +goose Down
DELETE FROM model_capability
WHERE provider_id = 'internal_ffmpeg'
  AND model_id = 'ffmpeg-compose';

ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume',
        'craftsman_turn',
        'worker_generation',
        'reviewer_turn',
        'dependency_scheduler'
    ));
```

- [ ] **Step 4: Add sqlc query and runtime methods**

Add to `agent_task.sql`:

```sql
-- name: ListQueuedComposerTasksAcrossWorkspaces :many
SELECT *
FROM agent_task
WHERE role = 'composer'
  AND task_type = 'composer_turn'
  AND status = 'queued'
ORDER BY created_at ASC
LIMIT $1;
```

Update runtime:

```go
func (s *Service) GetOrCreateComposerThread(ctx context.Context, workspaceID pgtype.UUID) (db.AgentThread, error)
func (s *Service) ListQueuedComposerTasksAcrossWorkspaces(ctx context.Context, limit int32) ([]db.AgentTask, error)
```

Update `validTaskType` to include `composer_turn`.

- [ ] **Step 5: Regenerate and test**

Run:

```bash
make sqlc-generate
make migrate-up
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/runtime -count=1
```

Expected: PASS.

## Task 2: Phase-Aware Craftsman

**Files:**
- Modify: `apps/server/internal/agent/craftsman/types.go`
- Modify: `apps/server/internal/agent/craftsman/graph.go`
- Modify: `apps/server/internal/agent/craftsman/model_responder.go`
- Modify: `apps/server/internal/agent/craftsman/context_loader.go`
- Test: `apps/server/internal/agent/craftsman/*_test.go`

- [ ] **Step 1: Write failing strategy tests**

Add tests:

```go
func TestParseShotVideoStrategy(t *testing.T) {
    strategy, err := ParseStrategy(`{
      "strategy":"用已通过预览图做首帧，强调口播商品动作。",
      "video_prompt":"A vertical product seeding video, smooth camera push-in.",
      "operation_type":"image_to_video",
      "output_type":"video",
      "input_node_refs":["shot-01 preview image"],
      "params":{"duration_sec":5}
    }`)
    if err != nil {
        t.Fatal(err)
    }
    if strategy.VideoPrompt == "" || strategy.OperationType != "image_to_video" || strategy.OutputType != "video" {
        t.Fatalf("strategy = %#v", strategy)
    }
}
```

- [ ] **Step 2: Verify the test fails**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/craftsman -run TestParseShotVideoStrategy -count=1
```

Expected: FAIL because `VideoPrompt` and phase fields do not exist.

- [ ] **Step 3: Implement phase-aware strategy**

Extend `Strategy`:

```go
type Strategy struct {
    Strategy       string         `json:"strategy"`
    PreviewPrompt  string         `json:"preview_prompt,omitempty"`
    VideoPrompt    string         `json:"video_prompt,omitempty"`
    NegativePrompt string         `json:"negative_prompt,omitempty"`
    StyleNotes     []string       `json:"style_notes,omitempty"`
    InputNodeRefs  []string       `json:"input_node_refs,omitempty"`
    OutputType     string         `json:"output_type,omitempty"`
    OperationType  string         `json:"operation_type,omitempty"`
    Model          ModelSpec      `json:"model,omitempty"`
    Params         map[string]any `json:"params,omitempty"`
}
```

Validation rules:

- `preview_image` requires `PreviewPrompt`.
- `shot_video` requires `VideoPrompt`.
- `shot_video.OperationType` defaults to `image_to_video`.
- `shot_video.OutputType` defaults to `video`.

- [ ] **Step 4: Pass mode through graph**

Extend `GraphInput`:

```go
type GraphInput struct {
    WorkspaceID  pgtype.UUID
    ThreadID     pgtype.UUID
    TaskID       pgtype.UUID
    ShotID       pgtype.UUID
    Mode         string
    MaxAttempts  int
    WorkerParams map[string]any
}
```

When creating worker input:

```go
workerInput := agentworker.GenerationInput{
    Mode: mode,
    Prompt: strategy.PromptForMode(mode),
    OutputType: strategy.OutputType,
    OperationType: strategy.OperationType,
}
```

- [ ] **Step 5: Run craftsman tests**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/craftsman -count=1
```

Expected: PASS.

## Task 3: Worker Shot Video Mode

**Files:**
- Modify: `apps/server/internal/agent/worker/types.go`
- Modify: `apps/server/internal/agent/worker/executor.go`
- Create: `apps/server/internal/agent/video/status.go`
- Test: `apps/server/internal/agent/worker/executor_test.go`

- [ ] **Step 1: Write failing Worker test**

Add a test that passes `GenerationInput{Mode:"shot_video"}` and expects:

- created node type `video`
- operation `image_to_video`
- metadata `agent_artifact_kind="shot_video"`
- submitted intent output type `video`
- event `shot_video_submitted`

Expected assertion:

```go
if fakeProduction.intent.OutputType != "video" || fakeProduction.intent.OperationType != "image_to_video" {
    t.Fatalf("intent = %#v", fakeProduction.intent)
}
```

- [ ] **Step 2: Verify the test fails**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/worker -run TestWorkerSubmitsShotVideoGeneration -count=1
```

Expected: FAIL because Worker rejects non-preview modes.

- [ ] **Step 3: Implement mode switch**

In `parseGenerationInput`, allow:

```go
switch input.Mode {
case "preview_image", "shot_video":
default:
    return GenerationInput{}, ErrInvalidInput
}
```

In node creation:

```go
case "shot_video":
    NodeType: db.NodeTypeVideo
    OperationType: defaultString(input.OperationType, "image_to_video")
    CanvasW: 300
    CanvasH: 420
    Metadata: {"agent_artifact_kind":"shot_video", ...}
```

In intent:

```go
OutputType: "video"
OperationType: "image_to_video"
```

- [ ] **Step 4: Add video status reducer**

Create `apps/server/internal/agent/video/status.go`:

```go
package video

const (
    EventSubmitted = "shot_video_submitted"
    EventSucceeded = "shot_video_succeeded"
    EventFailed = "shot_video_failed"

    ShotStatusVideoRunning = "video_running"
    ShotStatusVideoReady = "video_ready"
    ShotStatusFailed = "failed"
)

func ShotStatusForEvent(event string) (string, bool) {
    switch event {
    case EventSubmitted:
        return ShotStatusVideoRunning, true
    case EventSucceeded:
        return ShotStatusVideoReady, true
    case EventFailed:
        return ShotStatusFailed, true
    default:
        return "", false
    }
}
```

- [ ] **Step 5: Run Worker tests**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/worker ./internal/agent/video -count=1
```

Expected: PASS.

## Task 4: `generate_shot_video` Edge

**Files:**
- Create: `apps/server/internal/agent/tools/generate_shot_video.go`
- Modify: `apps/server/internal/agent/tools/registry_test.go`
- Modify: `apps/server/cmd/server/main.go`
- Test: `apps/server/internal/agent/tools/generate_shot_video_test.go`

- [ ] **Step 1: Write tool contract test**

Expected model-facing parameters:

```json
{
  "shot_refs": ["shot-01"],
  "target_phase": "shot_video",
  "max_attempts": 3,
  "force": false
}
```

Expected result:

```json
{
  "status": "queued",
  "mode": "shot_video",
  "dispatched": [{"shot_id":"...", "craftsman_task_id":"..."}]
}
```

- [ ] **Step 2: Implement tool by reusing dispatch patterns**

Use the existing `dispatch_craftsman` resolution style, but create task input:

```go
map[string]any{
    "mode": "shot_video",
    "shot_id": uuidString(shot.ID),
    "shot_client_key": shot.ClientKey,
    "producer_thread_id": uuidString(input.ThreadID),
    "producer_task_id": uuidString(input.TaskID),
    "requested_max_attempts": args.MaxAttempts,
}
```

Set shot status to `video_running` when queued.

- [ ] **Step 3: Register tool**

Add in `cmd/server/main.go`:

```go
agenttools.NewGenerateShotVideoEdge(queries, agentRuntime, craftsmanEnqueuer)
```

- [ ] **Step 4: Test**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -count=1
make server-build
```

Expected: PASS.

## Task 5: Production Terminal Events For Shot Video

**Files:**
- Modify: `apps/server/internal/api/production_broadcaster.go`
- Modify: `apps/server/internal/api/production_broadcaster_test.go`
- Modify: `apps/server/internal/agent/scheduler/dependency.go`

- [ ] **Step 1: Write failing broadcaster test**

Set up an Agent-owned video node with metadata:

```json
{"agent_artifact_kind":"shot_video"}
```

Publish `ProductionEventJobSucceeded`, then assert:

- event type `shot_video_succeeded`
- shot status `video_ready`
- `NodeUpdated` is broadcast with UI-ready node payload

- [ ] **Step 2: Implement generalized terminal mapping**

Add:

```go
func agentArtifactKind(node db.MediaNode) string
func terminalAgentEventFor(kind string, eventType string) (statusEvent string, agentEvent string, ok bool)
```

Mapping:

- `preview_image` + succeeded -> `preview_generation_succeeded`
- `shot_video` + succeeded -> `shot_video_succeeded`
- `final_video` + succeeded -> `composition_succeeded`

- [ ] **Step 3: Update scheduler**

`blocking_phase='video'` should require upstream video node with current winner.

- [ ] **Step 4: Test**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api ./internal/agent/scheduler -count=1
```

Expected: PASS.

## Task 6: Reviewer Supports Shot Video

**Files:**
- Modify: `apps/server/internal/agent/reviewer/context_loader.go`
- Modify: `apps/server/internal/agent/reviewer/model_responder.go`
- Modify: `apps/server/internal/agent/reviewer/rubric.go`
- Modify: `apps/server/internal/agent/tools/review_shot.go`
- Test: reviewer/tool tests

- [ ] **Step 1: Write failing review tool test**

Call `review_shot` with:

```json
{"shot_refs":["shot-01"],"target_phase":"shot_video","auto_retry":true}
```

Expected: creates `reviewer_turn` with `target_phase='shot_video'`.

- [ ] **Step 2: Extend current artifact lookup**

For `shot_video`, choose current winner from Agent-owned video node where metadata `agent_artifact_kind='shot_video'`.

- [ ] **Step 3: Extend reviewer context**

For video target:

- include video URL / mime / duration metadata when available;
- include prompt and params from generation job;
- include prior `shot_video` reviews.

- [ ] **Step 4: Test**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer ./internal/agent/tools -count=1
```

Expected: PASS.

## Task 7: Sandbox Final Composition Provider

**Files:**
- Modify: `apps/server/internal/sandbox/job_service.go`
- Modify: `apps/server/internal/sandbox/job_service_test.go`
- Modify: `apps/server/internal/production/internal_ffmpeg_provider.go`
- Modify: `apps/server/internal/production/internal_ffmpeg_provider_test.go`

- [ ] **Step 1: Write failing sandbox compose test**

Use fake sandbox client and storage. Input:

```go
sandbox.ComposeVideosInput{
    WorkspaceID: uuidWithByte(1),
    TargetNodeID: uuidWithByte(2),
    Sources: []sandbox.SandboxAssetInput{
        {AssetID:"asset-1", StorageURL:"minio://workspace/video1.mp4", Mime:"video/mp4"},
        {AssetID:"asset-2", StorageURL:"minio://workspace/video2.mp4", Mime:"video/mp4"},
    },
    Format: "mp4",
}
```

Expected:

- creates sandbox job `operation_type='compose_final_video'`
- command contains `ffmpeg`
- uploads final MP4
- returns `Asset.StorageURL`

- [ ] **Step 2: Implement `ComposeVideos`**

Add:

```go
type ComposeVideosInput struct {
    WorkspaceID pgtype.UUID
    TargetNodeID pgtype.UUID
    Sources []SandboxAssetInput
    Format string
    TimeoutSeconds int
}
```

Implementation outline:

1. Create `sandbox_job`.
2. Ensure workspace sandbox and layout.
3. Download all source videos into `/workspace/assets`.
4. Write concat list.
5. Run FFmpeg concat command.
6. Inspect output.
7. Upload output through presigned PUT.
8. Mark sandbox job succeeded.

- [ ] **Step 3: Extend internal FFmpeg provider**

Support operation:

```go
case "compose_final_video":
    return p.composeFinalVideo(ctx, intent)
```

Provider result must include:

```go
ProviderResponse: map[string]any{
    "sandbox_job_id": uuidToString(result.Job.ID),
    "operation_type": "compose_final_video",
    "source_count": len(sources),
}
```

- [ ] **Step 4: Test**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox ./internal/production -run 'Compose|InternalFFmpeg' -count=1
```

Expected: PASS.

## Task 8: ComposerGraph And `compose_final` Edge

**Files:**
- Create: `apps/server/internal/agent/composer/types.go`
- Create: `apps/server/internal/agent/composer/context_loader.go`
- Create: `apps/server/internal/agent/composer/graph.go`
- Create: `apps/server/internal/agent/composer/executor.go`
- Create: `apps/server/internal/agent/composer/model_responder.go`
- Create: `apps/server/internal/agent/tools/compose_final.go`
- Modify: `apps/server/cmd/server/main.go`
- Test: `apps/server/internal/agent/composer/*_test.go`

- [ ] **Step 1: Write failing ComposerGraph test**

Input includes two shots with selected video winners. Expected:

- creates final video node with metadata `agent_artifact_kind='final_video'`
- submits `GenerationIntent{OutputType:"video", OperationType:"compose_final_video", Model.Provider:"internal_ffmpeg", Model.ModelID:"ffmpeg-compose"}`
- appends UI message with final video pending card or status markdown
- creates `composition_started` event

- [ ] **Step 2: Implement composer context**

Load:

- ordered shots from `ListActiveShotsByWorkspace`;
- shot video nodes by `shot_id`;
- current winner versions and assets;
- review records for `shot_video`;
- PSS final output context.

Reject context when any required shot lacks selected video winner.

- [ ] **Step 3: Implement graph**

Graph nodes:

```text
load_composition_context
-> draft_composition_plan
-> validate_assets_ready
-> create_final_video_node
-> submit_final_generation_intent
-> request_final_hitl
```

For M6.8, `draft_composition_plan` can be deterministic with optional model responder. It must still be a Graph node for checkpointing.

- [ ] **Step 4: Implement `compose_final` tool**

Edge schema:

```json
{
  "shot_refs": ["shot-01", "shot-02"],
  "output_format": "9:16",
  "title": "Final video",
  "request_confirmation": true
}
```

The tool creates `composer_turn` and enqueues Composer executor.

- [ ] **Step 5: Wire main recovery**

Add:

```go
go recoverQueuedComposerTasks(composerExecutor, agentRuntime)
```

- [ ] **Step 6: Test**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer ./internal/agent/tools -count=1
make server-build
```

Expected: PASS.

## Task 9: Final Video Card And Detail Projection

**Files:**
- Modify: `apps/server/internal/agent/uimessage/blocks.go`
- Modify: `apps/server/internal/agent/uimessage/blocks_test.go`
- Modify: `apps/web/src/lib/agentMessageBlocks.ts`
- Modify: `apps/web/src/lib/agentMessageBlocks.test.mjs`
- Create: `apps/web/src/components/agent/AgentFinalVideoCardBlock.tsx`
- Modify: `apps/web/src/components/agent/AgentMessageRenderer.tsx`
- Modify: `apps/web/src/components/agent/AgentNodeDetailDrawer.tsx`

- [ ] **Step 1: Write parser/rendering tests**

Add frontend test:

```js
assert.equal(isFinalVideoCardBlock({
  id: "blk_final",
  type: "final_video_card",
  status: "ready",
  node_id: "node-1",
  version_id: "version-1",
  asset_id: "asset-1",
  title: "成片",
  url: "http://localhost/final.mp4",
  source_shots: ["shot-01", "shot-02"]
}), true)
```

- [ ] **Step 2: Add backend block**

Add:

```go
type FinalVideoCardBlock struct {
    BaseBlock
    Status string `json:"status"`
    NodeID string `json:"node_id"`
    VersionID string `json:"version_id"`
    AssetID string `json:"asset_id"`
    Title string `json:"title"`
    URL string `json:"url,omitempty"`
    ThumbnailURL string `json:"thumbnail_url,omitempty"`
    SourceShots []string `json:"source_shots"`
    DecisionID string `json:"decision_id,omitempty"`
}
```

- [ ] **Step 3: Render final card**

Use `<video controls>` with status, source shots, and version diagnostics. User-facing text should say “成片” / “等待确认”，not “Composer”.

- [ ] **Step 4: Test**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentMessageBlocks
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

## Task 10: PSS Final Output And Edge Registry State

**Files:**
- Modify: `apps/server/internal/agent/pss/producer.go`
- Modify: `apps/server/internal/agent/pss/producer_test.go`
- Modify: `apps/server/internal/agent/tools/production_state.go`
- Test: PSS/tool tests

- [ ] **Step 1: Write PSS test**

Given:

- shot-01 video winner;
- shot-02 video winner;
- final video node with current winner;
- sandbox job linked to generation job.

Expected PSS text includes:

```text
分镜视频
- shot-01: video_ready
成片
- Final video: succeeded, sandbox_job=...
```

- [ ] **Step 2: Implement PSS projection**

Structured fields:

```json
{
  "shot_videos": [...],
  "final_outputs": [...],
  "composition": {"status":"succeeded", "sandbox_jobs":[...]}
}
```

- [ ] **Step 3: Test**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/pss ./internal/agent/tools -count=1
```

Expected: PASS.

## Task 11: Smoke Script And Browser E2E

**Files:**
- Create: `scripts/smoke-m6-8-video-composer.sh`
- Update: `docs/superpowers/plans/2026-06-23-m6-8-video-composer-final.md` with implementation notes if execution reveals changed commands.

- [ ] **Step 1: Create smoke script**

Script should:

1. Register test user.
2. Create Agent workspace.
3. Send a message asking for 2-shot storyboard, preview, review, video, final composition.
4. Print workspace URL and DB spot-check commands.

- [ ] **Step 2: Run backend/frontend verification**

Run:

```bash
make sqlc-generate
make server-build
make server-test
make server-lint
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected: all PASS.

- [ ] **Step 3: Browser E2E**

Start runtime:

```bash
./scripts/dev-start.sh
```

Use the printed Vite URL. Test flow:

1. Create/login test user.
2. Create Agent workspace.
3. Send request:

```text
创建一个 2 个分镜的 10 秒口播种草短视频，生成预览图，评审通过后生成分镜视频，最后合成为成片并请求我确认。
```

4. Verify Agent message stream shows tool status for `generate_shot_video` and `compose_final`.
5. Verify canvas shows Agent image nodes, video nodes, and final video node without refresh.
6. Verify node detail drawer shows versions, review records, and sandbox job trace.
7. Verify final video card is playable and has confirmation action.
8. Verify database rows:

```sql
SELECT role, task_type, status FROM agent_task WHERE workspace_id = '<workspace_id>' ORDER BY created_at;
SELECT event_type FROM agent_event WHERE workspace_id = '<workspace_id>' ORDER BY created_at;
SELECT target_phase, status FROM review_record WHERE workspace_id = '<workspace_id>' ORDER BY created_at;
SELECT job_type, operation_type, status, generation_job_id FROM sandbox_job WHERE workspace_id = '<workspace_id>' ORDER BY created_at;
```

Expected:

- `worker_generation` tasks for preview and shot video.
- `composer_turn` task for final composition.
- `shot_video_succeeded` or `shot_video_failed` terminal event.
- `composition_succeeded` terminal event.
- `sandbox_job.operation_type='compose_final_video'`.
- final video node has `source='agent'`, `node_type='video'`, metadata `agent_artifact_kind='final_video'`.

## Acceptance Criteria

M6.8 is complete only when all are true:

- Producer can call `generate_shot_video`.
- Accepted preview image can become video input through `GenerationIntent.InputRefs`.
- Worker creates Agent-owned video node with `shot_id`.
- Shot video generation produces `generation_job` and `artifact_version`.
- Shot status moves to `video_running` then terminal `video_ready` / `failed`.
- `review_shot(target_phase='shot_video')` persists review records.
- Accepted shot video winner can be selected.
- Producer can call `compose_final`.
- ComposerGraph runs as a persistent `composer_turn` with checkpoint.
- Final composition runs through sandbox-backed `internal_ffmpeg`, not app-process FFmpeg.
- Final video appears as Agent-owned video node with job/version/sandbox trace.
- Final HITL card lets user confirm or request changes.
- PSS includes shot videos and final output state.
- Browser E2E proves no refresh is needed for canvas/video/final updates.

## Self-Review

- Spec coverage: covers `generate_shot_video`, Worker `shot_video`, video status propagation, video review, ComposerGraph, sandbox final composition, final video lifecycle, final HITL card, PSS final output, and E2E.
- Current-code consistency: reuses existing Agent runtime, Craftsman/Worker, `GenerationIntent`, production runner, `InternalFFmpegProvider`, `sandbox.JobService`, review records, and typed UI message renderer.
- Boundary check: does not add Studio/Agent import-export, does not allow Agent canvas editing, and does not run FFmpeg in the app process.
