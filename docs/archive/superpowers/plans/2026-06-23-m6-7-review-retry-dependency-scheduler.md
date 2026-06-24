# M6.7 Review Retry Dependency Scheduler Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the M6.7 quality-control loop: durable review records, ReviewerGraph, traceable retry orchestration, version selection, dependency readiness scheduling, and Agent UI review/blocked-state rendering.

**Architecture:** Keep Eino Graph as the explicit orchestration primitive. Producer remains the user-facing coordinator and invokes model-facing tools; ReviewerGraph is a first-class scoped graph that reviews generated artifacts and writes durable review facts; retry dispatch routes back through existing Craftsman/Worker preview generation. Dependency readiness is derived from DB production facts and emitted as durable Agent events, not stored in model summaries.

**Tech Stack:** Go 1.26, CloudWeGo Eino, Hertz, pgx/sqlc, PostgreSQL JSONB, existing ClipAnvil Agent runtime, existing M4/M5 production service, React 19/Vite/React Flow, Agent UI message blocks, `/ws/agent`, `/ws/canvas`.

---

## Scope

This plan implements the **M6.7: Review / Retry / Dependency Scheduler** section from `docs/superpowers/specs/2026-06-23-m6-agent-mode-completion-design.md`.

M6.6 is assumed complete:

- `dispatch_craftsman(mode=preview_image)` creates shot-scoped Craftsman tasks.
- CraftsmanGraph creates Worker tasks.
- Worker creates Agent-owned preview image nodes and submits `GenerationIntent`.
- production terminal events create Agent preview events and update Agent canvas without refresh.
- PSS includes shots, dependencies, preview nodes, jobs, and versions.

M6.7 adds:

- Durable `review_record`.
- ReviewerGraph and review dispatcher.
- `review_shot` tool.
- `select_version` tool for Agent workspaces.
- `retry_generation` tool that loops back through Craftsman/Worker with critique context.
- Dependency readiness scheduler for `shot_dependency.blocking_phase`.
- PSS and Agent UI rendering for review results, retry chains, and blocked shots.

M6.7 does **not** add:

- Shot video generation.
- ComposerGraph or final video.
- Studio / Agent import-export.
- Direct user editing of Agent canvas.
- A separate workflow engine outside Eino + existing Agent runtime.

## Current Code Facts

- Agent runtime tables already exist in `apps/server/migrations/015_m6_agent_runtime.sql`.
- `agent_task.role` already allows `reviewer`, but `agent_task.task_type` currently only allows `producer_turn`, `tool_call`, `decision_resume`, `craftsman_turn`, and `worker_generation`.
- `agent_event.source_role` / `target_role` already allow `reviewer`.
- `artifact_version.review_score` exists, but should remain a compatibility summary only.
- `production.Service.SelectArtifactVersion` already performs winner selection, node current version update, stale resolution, and downstream stale propagation.
- `RunHandler.SelectNodeVersion` is currently Studio-only because it calls `requireStudioWorkspace`; Agent must select through an Agent tool, not through that user-facing Studio endpoint.
- Producer tools are registered in `apps/server/cmd/server/main.go`.
- Current Producer tool calls already flow through Eino native tool calling and `compose.EdgesNode`.
- Agent UI message protocol already supports typed blocks; M6.7 should extend it instead of returning raw JSON.

## File Map

### Database and sqlc

- Create `apps/server/migrations/020_m6_review_retry_scheduler.sql`
  - Add `review_record`.
  - Extend `agent_task_type_check` to include `reviewer_turn` and `dependency_scheduler`.
  - Add indexes for workspace, shot, node/version, reviewer task, and retry parent.
- Create `apps/server/sqlc/queries/review_record.sql`
  - Insert running review record.
  - Complete accepted/rejected/failed review.
  - Get review by id.
  - List reviews by workspace, shot, node, and artifact version.
  - Count rejected review attempts for shot/phase.
- Modify generated files through `make sqlc-generate`.

### Backend review package

- Create `apps/server/internal/agent/reviewer/types.go`
  - Graph input/output, review context, rubric axis, review result, retry recommendation.
- Create `apps/server/internal/agent/reviewer/context_loader.go`
  - Load shot, target node, target artifact version, asset URL/model reference, generation job, input refs, prior reviews, selected memory/PSS text when available.
- Create `apps/server/internal/agent/reviewer/model_responder.go`
  - Eino/Ark model responder with deterministic test responder.
  - Use vision-capable text model for preview image review.
  - Return strict JSON rubric.
- Create `apps/server/internal/agent/reviewer/rubric.go`
  - Validate axis names, scores, pass flags, overall score, reject policy.
- Create `apps/server/internal/agent/reviewer/graph.go`
  - Eino Graph nodes: `load_review_context -> call_review_model -> validate_rubric -> persist_review_record -> route_accept_or_retry`.
- Create `apps/server/internal/agent/reviewer/executor.go`
  - Run queued `reviewer_turn` tasks, persist messages/events, enqueue retry when policy rejects and attempts remain.
- Create `apps/server/internal/agent/reviewer/checkpoint.go`
  - Build stable reviewer checkpoint keys.

### Backend scheduler package

- Create `apps/server/internal/agent/scheduler/dependency.go`
  - Calculate readiness for one shot/phase from `shot_dependency`, current winners, and accepted review records.
  - Emit `shot_blocked`, `shot_unblocked`, and `dependency_ready` events only when state changes enough to be useful.
- Create `apps/server/internal/agent/scheduler/dispatcher.go`
  - Consume preview terminal/review terminal events and enqueue downstream review or dispatch tasks.

### Backend tools

- Create `apps/server/internal/agent/tools/review_shot.go`
  - Producer tool to request review for one or more shot preview winners.
- Create `apps/server/internal/agent/tools/select_version.go`
  - Producer/Reviewer policy tool to set a version as current winner using `production.Service.SelectArtifactVersion`.
- Create `apps/server/internal/agent/tools/retry_generation.go`
  - Producer tool to retry a shot/phase with critique context and fixed retry cap.
- Modify `apps/server/internal/agent/tools/registry_test.go`
  - Assert tool registry includes `review_shot`, `select_version`, and `retry_generation`.
- Modify `apps/server/cmd/server/main.go`
  - Wire ReviewerGraph, reviewer executor/enqueuer/recovery, dependency scheduler, and new tools.

### PSS and API projection

- Modify `apps/server/internal/agent/pss/producer.go`
  - Include review records, accepted/rejected state, retry counts, blocked shot reasons, dependency readiness.
- Modify `apps/server/internal/api/run_handler.go`
  - Add `review_records` to `productionStateResponse` for node detail drawer.
- Modify `apps/server/internal/api/agent_response.go` if needed
  - Expose any additional Agent event/message payload fields already persisted.

### UI message protocol

- Modify `apps/server/internal/agent/uimessage/blocks.go`
  - Add `ReviewCardBlock`.
  - Add `DependencyStatusBlock` or a compact `TaskTimelineBlock` if backend emits user-facing scheduler summaries.
- Modify `apps/web/src/lib/agentMessageBlocks.ts`
  - Add `review_card` and optional `dependency_status` block types.
- Create `apps/web/src/components/agent/AgentReviewCardBlock.tsx`
  - Render accepted/rejected/failed status, overall score, axis scores, critique, retry count, linked shot/node/version.
- Modify `apps/web/src/components/agent/AgentMessageRenderer.tsx`
  - Route new block types.
- Modify `apps/web/src/components/agent/AgentNodeDetailDrawer.tsx`
  - Render review history and retry chain from `production-state`.

### Smoke and E2E

- Create `scripts/smoke-m6-7-review-retry-scheduler.sh`
  - API/database smoke for storyboard -> preview event -> review -> retry -> accepted selection -> dependency blocked/unblocked.
- Add source-level web tests in existing `apps/web/src/lib/*.test.mjs` files.
- Use browser E2E through `./scripts/dev-start.sh` and the printed Vite URL.

## Data Model

### `review_record`

Create `apps/server/migrations/020_m6_review_retry_scheduler.sql`:

```sql
-- +goose Up
CREATE TABLE review_record (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    shot_id UUID REFERENCES shot(id) ON DELETE SET NULL,
    node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    artifact_version_id UUID NOT NULL REFERENCES artifact_version(id) ON DELETE CASCADE,
    generation_job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    reviewer_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    reviewer_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    parent_review_record_id UUID REFERENCES review_record(id) ON DELETE SET NULL,
    target_phase TEXT NOT NULL,
    status TEXT NOT NULL,
    attempt_no INT NOT NULL DEFAULT 1,
    max_attempts INT NOT NULL DEFAULT 3,
    overall_score REAL,
    rubric JSONB NOT NULL DEFAULT '{}',
    critique TEXT NOT NULL DEFAULT '',
    retry_recommendation JSONB NOT NULL DEFAULT '{}',
    model_provider TEXT NOT NULL DEFAULT '',
    model_id TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    CONSTRAINT review_record_phase_check CHECK (target_phase IN ('preview_image', 'shot_video', 'final_video')),
    CONSTRAINT review_record_status_check CHECK (status IN ('running', 'accepted', 'rejected', 'failed')),
    CONSTRAINT review_record_attempt_check CHECK (attempt_no >= 1 AND max_attempts >= attempt_no)
);

CREATE INDEX idx_review_record_workspace_created ON review_record(workspace_id, created_at DESC);
CREATE INDEX idx_review_record_shot_phase ON review_record(workspace_id, shot_id, target_phase, created_at DESC);
CREATE INDEX idx_review_record_node ON review_record(node_id, created_at DESC);
CREATE INDEX idx_review_record_version ON review_record(artifact_version_id, created_at DESC);
CREATE INDEX idx_review_record_task ON review_record(reviewer_task_id);
CREATE INDEX idx_review_record_parent ON review_record(parent_review_record_id);

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

-- +goose Down
ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume',
        'craftsman_turn',
        'worker_generation'
    ));

DROP INDEX IF EXISTS idx_review_record_parent;
DROP INDEX IF EXISTS idx_review_record_task;
DROP INDEX IF EXISTS idx_review_record_version;
DROP INDEX IF EXISTS idx_review_record_node;
DROP INDEX IF EXISTS idx_review_record_shot_phase;
DROP INDEX IF EXISTS idx_review_record_workspace_created;
DROP TABLE IF EXISTS review_record;
```

Notes:

- `artifact_version.review_score` should be updated from accepted/rejected review as a summary for compatibility, but the full rubric lives in `review_record`.
- `attempt_no`, `max_attempts`, and `parent_review_record_id` make retry chains queryable without parsing JSON.
- `target_phase='preview_image'` is the only phase M6.7 must execute; `shot_video` and `final_video` are schema-compatible for M6.8.

## Edge Contracts

### `review_shot`

Model-facing description:

> Review generated output for one or more storyboard shots. For preview_image, this reads each shot's current preview image winner, creates persistent reviewer tasks, and returns queued review summaries. It does not generate new media directly.

Parameters:

```json
{
  "type": "object",
  "properties": {
    "shot_refs": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Shot UUIDs or stable client keys such as shot-01. Empty means all preview_ready shots."
    },
    "target_phase": {
      "type": "string",
      "enum": ["preview_image"],
      "description": "M6.7 supports preview_image review."
    },
    "max_attempts": {
      "type": "integer",
      "minimum": 1,
      "maximum": 3,
      "description": "Maximum review/retry attempts for this shot and phase. Defaults to 3."
    },
    "auto_retry": {
      "type": "boolean",
      "description": "When true, rejected reviews can trigger retry_generation until max_attempts is exhausted."
    }
  },
  "additionalProperties": false
}
```

User-facing label: `评审预览图`.

### `select_version`

Model-facing description:

> Select a succeeded artifact version as the current winner for an Agent-owned production node. This reuses the production version selection service, updates downstream stale state, writes Agent events, and refreshes canvas state.

Parameters:

```json
{
  "type": "object",
  "properties": {
    "node_id": { "type": "string" },
    "version_id": { "type": "string" },
    "reason": { "type": "string" },
    "target_phase": {
      "type": "string",
      "enum": ["preview_image"],
      "description": "M6.7 supports preview_image selection."
    }
  },
  "required": ["node_id", "version_id"],
  "additionalProperties": false
}
```

User-facing label: `选择版本`.

### `retry_generation`

Model-facing description:

> Retry a shot preview generation using a review critique or user revision instruction. This creates a traceable retry event and dispatches the existing Craftsman/Worker preview pipeline with force=true.

Parameters:

```json
{
  "type": "object",
  "properties": {
    "shot_ref": { "type": "string" },
    "target_phase": {
      "type": "string",
      "enum": ["preview_image"]
    },
    "review_record_id": { "type": "string" },
    "critique": { "type": "string" },
    "fix_hints": {
      "type": "array",
      "items": { "type": "string" }
    },
    "max_attempts": {
      "type": "integer",
      "minimum": 1,
      "maximum": 3
    }
  },
  "required": ["shot_ref", "target_phase"],
  "additionalProperties": false
}
```

User-facing label: `重新生成`.

## Task 1: Review Schema And sqlc

**Files:**

- Create: `apps/server/migrations/020_m6_review_retry_scheduler.sql`
- Create: `apps/server/sqlc/queries/review_record.sql`
- Modify generated: `apps/server/internal/store/db/*.go`

- [ ] Add the migration above.
- [ ] Add `review_record.sql` queries:

```sql
-- name: CreateReviewRecord :one
INSERT INTO review_record (
    workspace_id, shot_id, node_id, artifact_version_id, generation_job_id,
    reviewer_thread_id, reviewer_task_id, parent_review_record_id,
    target_phase, status, attempt_no, max_attempts, model_provider, model_id
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, 'running', $10, $11, $12, $13
) RETURNING *;

-- name: CompleteReviewRecord :one
UPDATE review_record
SET status = $2,
    overall_score = $3,
    rubric = $4,
    critique = $5,
    retry_recommendation = $6,
    error_code = '',
    error_message = '',
    completed_at = now()
WHERE id = $1
RETURNING *;

-- name: FailReviewRecord :one
UPDATE review_record
SET status = 'failed',
    error_code = $2,
    error_message = $3,
    completed_at = now()
WHERE id = $1
RETURNING *;

-- name: GetReviewRecordByID :one
SELECT *
FROM review_record
WHERE id = $1;

-- name: ListReviewRecordsByWorkspace :many
SELECT *
FROM review_record
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListReviewRecordsByShotPhase :many
SELECT *
FROM review_record
WHERE workspace_id = $1
  AND shot_id = $2
  AND target_phase = $3
ORDER BY created_at DESC;

-- name: ListReviewRecordsByNode :many
SELECT *
FROM review_record
WHERE node_id = $1
ORDER BY created_at DESC;

-- name: ListReviewRecordsByArtifactVersion :many
SELECT *
FROM review_record
WHERE artifact_version_id = $1
ORDER BY created_at DESC;

-- name: CountReviewAttemptsByShotPhase :one
SELECT COUNT(*)::int
FROM review_record
WHERE workspace_id = $1
  AND shot_id = $2
  AND target_phase = $3
  AND status IN ('accepted', 'rejected', 'failed');
```

- [ ] Run:

```bash
make sqlc-generate
```

Expected: sqlc generates `review_record.sql.go` and updates `querier.go`.

- [ ] Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/store/db -run '^$' -count=1
```

Expected: package compiles; command may report `no test files` or no matching tests.

## Task 2: Review Rubric Validation

**Files:**

- Create: `apps/server/internal/agent/reviewer/types.go`
- Create: `apps/server/internal/agent/reviewer/rubric.go`
- Create: `apps/server/internal/agent/reviewer/rubric_test.go`

- [ ] Add rubric types with required axes:

```go
var RequiredPreviewAxes = []string{
	"proportion",
	"physics",
	"style",
	"visual_quality",
	"product_visibility",
	"selling_power",
	"platform_fit",
}
```

- [ ] Implement `ValidateRubric(result ReviewResult, policy ReviewPolicy) (ReviewDecision, error)`.
- [ ] Reject invalid JSON-shaped results when an axis is missing, score is outside `[0, 1]`, or required axis pass is false.
- [ ] Default policy:

```go
ReviewPolicy{
	OverallThreshold: 0.75,
	AxisThreshold:    0.70,
	RequiredAxes:     RequiredPreviewAxes,
	MaxAttempts:      3,
}
```

- [ ] Tests:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run 'TestValidateRubric' -count=1
```

Expected first: FAIL before implementation. Expected after implementation: PASS.

## Task 3: Reviewer Context Loader

**Files:**

- Create: `apps/server/internal/agent/reviewer/context_loader.go`
- Create: `apps/server/internal/agent/reviewer/context_loader_test.go`
- Consider extracting reusable model image helpers from `apps/server/internal/agent/producer/context_loader.go` into a small shared package if duplication becomes awkward.

- [ ] Load:
  - workspace
  - shot
  - preview image node
  - current artifact version
  - asset access/model URL
  - generation job
  - prior review records
  - source input refs from generation intent/provider request where available
  - current Producer PSS text
- [ ] For image model input, support `http`, `https`, and base64 data URLs; reuse the same safety checks as Producer image attachment handling.
- [ ] Test that an internal MinIO URL is converted into model-safe data URL when object reader is configured.
- [ ] Test that context includes prior rejected review critique.

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run 'TestContextLoader' -count=1
```

Expected: PASS.

## Task 4: ReviewerGraph And Executor

**Files:**

- Create: `apps/server/internal/agent/reviewer/model_responder.go`
- Create: `apps/server/internal/agent/reviewer/model_responder_test.go`
- Create: `apps/server/internal/agent/reviewer/graph.go`
- Create: `apps/server/internal/agent/reviewer/graph_test.go`
- Create: `apps/server/internal/agent/reviewer/executor.go`
- Create: `apps/server/internal/agent/reviewer/executor_test.go`

- [ ] Build Eino Graph:

```text
load_review_context
-> call_review_model
-> validate_rubric
-> persist_review_record
-> route_accept_or_retry
```

- [ ] Reviewer task input:

```json
{
  "target_phase": "preview_image",
  "shot_id": "...",
  "node_id": "...",
  "artifact_version_id": "...",
  "generation_job_id": "...",
  "parent_review_record_id": "...",
  "attempt_no": 1,
  "max_attempts": 3,
  "auto_retry": true
}
```

- [ ] Persist an assistant message on reviewer thread with a `review_card` UI block.
- [ ] Persist events:
  - `review_started`
  - `review_accepted`
  - `review_rejected`
  - `review_failed`
  - `retry_requested` when auto-retry is selected and attempts remain
  - `retry_exhausted` when attempts are exhausted
- [ ] On accepted review, call version selection service through an internal interface, not raw SQL.
- [ ] On rejected review, do not submit generation directly inside ReviewerGraph; create retry task/tool path through `retry_generation` orchestration.
- [ ] Add deterministic responder for tests:
  - `mode=accept` returns passing rubric.
  - `mode=reject_once` rejects first attempt and accepts second.

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run 'TestReviewerGraph|TestReviewerExecutor' -count=1
```

Expected: PASS.

## Task 5: Agent Edges For Review, Selection, And Retry

**Files:**

- Create: `apps/server/internal/agent/tools/review_shot.go`
- Create: `apps/server/internal/agent/tools/review_shot_test.go`
- Create: `apps/server/internal/agent/tools/select_version.go`
- Create: `apps/server/internal/agent/tools/select_version_test.go`
- Create: `apps/server/internal/agent/tools/retry_generation.go`
- Create: `apps/server/internal/agent/tools/retry_generation_test.go`
- Modify: `apps/server/internal/agent/tools/registry_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] `review_shot` resolves shot refs to current preview winners and creates `reviewer_turn` tasks.
- [ ] `review_shot` rejects shots without `preview_ready` or without a current preview image version with a natural-language summary.
- [ ] `select_version` uses `production.Service.SelectArtifactVersion`, writes `version_selected` Agent event, and broadcasts canvas node update.
- [ ] `retry_generation` creates `retry_requested` event and dispatches `dispatch_craftsman` with:
  - `mode=preview_image`
  - `force=true`
  - critique context in task input or Craftsman context source
  - remaining retry cap
- [ ] Update registry expected names:

```go
[]string{
	"read_workspace_context",
	"create_agent_text_node",
	"request_user_decision",
	"dispatch_craftsman",
	"review_shot",
	"select_version",
	"retry_generation",
}
```

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'TestReviewShot|TestSelectVersion|TestRetryGeneration|TestRegistry' -count=1
```

Expected: PASS.

## Task 6: Dependency Readiness Scheduler

**Files:**

- Create: `apps/server/internal/agent/scheduler/dependency.go`
- Create: `apps/server/internal/agent/scheduler/dependency_test.go`
- Create: `apps/server/internal/agent/scheduler/dispatcher.go`
- Create: `apps/server/internal/agent/scheduler/dispatcher_test.go`
- Modify: `apps/server/internal/api/production_broadcaster.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] Implement phase readiness:
  - `preview`: upstream shot has preview image current winner.
  - `review`: upstream shot has accepted `review_record` for `preview_image`.
  - `video`: reserved for M6.8, return blocked with `unsupported_phase` for now.
  - `composer`: reserved for M6.8.
- [ ] For blocked downstream shots, do not enqueue Craftsman/reviewer tasks.
- [ ] Emit events:
  - `shot_blocked`
  - `shot_unblocked`
  - `dependency_ready`
- [ ] Trigger readiness checks when these events occur:
  - `preview_generation_succeeded`
  - `review_accepted`
  - `review_rejected`
  - `retry_exhausted`
- [ ] Keep scheduler idempotent. Re-running it should not spam duplicate blocked events for the same shot/phase/reason.

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/scheduler -run 'TestDependencyReadiness|TestSchedulerDispatcher' -count=1
```

Expected: PASS.

## Task 7: PSS, Production State API, And UI Message Blocks

**Files:**

- Modify: `apps/server/internal/agent/pss/producer.go`
- Modify: `apps/server/internal/agent/pss/producer_test.go`
- Modify: `apps/server/internal/api/run_handler.go`
- Modify: `apps/server/internal/api/run_handler_test.go`
- Modify: `apps/server/internal/agent/uimessage/blocks.go`
- Modify: `apps/server/internal/agent/uimessage/blocks_test.go`
- Modify: `apps/web/src/lib/agentMessageBlocks.ts`
- Modify: `apps/web/src/lib/agentMessageBlocks.test.mjs`
- Create: `apps/web/src/components/agent/AgentReviewCardBlock.tsx`
- Modify: `apps/web/src/components/agent/AgentMessageRenderer.tsx`
- Modify: `apps/web/src/components/agent/AgentNodeDetailDrawer.tsx`
- Modify: `apps/web/src/lib/api.ts`

- [ ] PSS text includes review summary per shot:

```text
Review: accepted, score=0.86, version=v2
Review: rejected, score=0.52, retry=1/3, critique=...
Blocked: waiting for shot-01 preview review accepted
```

- [ ] PSS structured state includes:
  - `reviews`
  - `retry_chains`
  - `blocked_shots`
  - `accepted_winners`
- [ ] `production-state` includes `review_records` for selected node.
- [ ] Add `review_card` block node:

```json
{
  "id": "blk_review",
  "type": "review_card",
  "review_id": "...",
  "status": "accepted",
  "target_phase": "preview_image",
  "shot_ref": "shot-01",
  "node_id": "...",
  "version_id": "...",
  "overall_score": 0.86,
  "rubric": {
    "visual_quality": { "score": 0.9, "pass": true, "reason": "...", "fix_hint": "" }
  },
  "critique": "...",
  "retry_count": 1,
  "max_attempts": 3
}
```

- [ ] Render review cards collapsed by default for axis details, with status/score visible.
- [ ] Node detail drawer shows review history and retry chain, read-only.

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
```

Expected: PASS.

## Task 8: Wiring, Recovery, And Smoke Script

**Files:**

- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/agent/runtime/service.go`
- Modify: `apps/server/sqlc/queries/agent_task.sql`
- Create: `scripts/smoke-m6-7-review-retry-scheduler.sh`

- [ ] Add runtime method:

```go
ListQueuedReviewerTasksAcrossWorkspaces(ctx context.Context, limit int32) ([]db.AgentTask, error)
```

- [ ] Add SQL:

```sql
-- name: ListQueuedReviewerTasksAcrossWorkspaces :many
SELECT *
FROM agent_task
WHERE role = 'reviewer'
  AND task_type = 'reviewer_turn'
  AND status = 'queued'
ORDER BY created_at ASC
LIMIT $1;
```

- [ ] Wire reviewer executor in `main.go`.
- [ ] Add `recoverQueuedReviewerTasks`.
- [ ] Wire scheduler event sink so preview/review terminal events can re-evaluate blocked downstream shots.
- [ ] Smoke script flow:
  1. Ensure dev auth token.
  2. Create Agent workspace.
  3. Call Agent message to create two shots with a dependency from shot-01 to shot-02.
  4. Use mock/deterministic path to mark shot-01 preview succeeded.
  5. Queue review for shot-01.
  6. Force deterministic reject once.
  7. Verify retry task/event exists.
  8. Force deterministic accept.
  9. Verify current winner selected and downstream dependency unblocked.

Run:

```bash
bash -n scripts/smoke-m6-7-review-retry-scheduler.sh
./scripts/smoke-m6-7-review-retry-scheduler.sh
```

Expected: PASS and prints workspace id plus review/retry/dependency event ids.

## Task 9: End-To-End Browser Acceptance

**Files:**

- No mandatory Playwright spec file required unless existing project e2e structure is active.
- Use browser automation against the Vite URL printed by `./scripts/dev-start.sh`.

- [ ] Start:

```bash
./scripts/dev-start.sh
```

Expected: script prints the actual Vite URL and backend health passes.

- [ ] Browser E2E, deterministic review mode:
  1. Create an Agent workspace.
  2. Upload one product image.
  3. Send: `创建一个 2 个分镜的 15 秒口播种草短视频 storyboard，第二个分镜依赖第一个分镜的商品展示连续性。生成预览图并评审。`
  4. Confirm Producer calls `update_storyboard`.
  5. Confirm Producer calls `dispatch_craftsman`.
  6. Confirm canvas shows preview image nodes without refresh.
  7. Confirm Agent creates `review_card`.
  8. Confirm one deterministic rejected review triggers retry.
  9. Confirm retry creates a new artifact version, not overwriting old version.
  10. Confirm accepted review selects current winner.
  11. Confirm blocked downstream shot becomes unblocked only after dependency is ready.
  12. Open node detail drawer and verify prompt/model/job/version/review/retry chain are visible.

- [ ] Browser E2E, real model smoke:
  1. Select a vision-capable text model.
  2. Ask a non-tool question about the uploaded image.
  3. Ask Agent to review one generated preview.
  4. Verify ReviewerGraph stores either accepted/rejected review with model provider/model id and no raw JSON message leaking to user UI.

- [ ] Stop:

```bash
./scripts/dev-stop.sh
```

Expected: current worktree profile processes stop cleanly.

## Required Verification Commands

Run the full set before claiming M6.7 complete:

```bash
make migrate-up
make sqlc-generate
make server-build
make server-test
make server-lint
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web test:connections
bash -n scripts/smoke-m6-7-review-retry-scheduler.sh
./scripts/smoke-m6-7-review-retry-scheduler.sh
git diff --check
```

If only planning/doc changes were made, run:

```bash
git diff --check
```

## Acceptance Criteria

- `review_record` is the source of truth for review; `artifact_version.review_score` is only a summary.
- `review_shot` queues Reviewer tasks for current preview winners.
- ReviewerGraph uses Eino Graph and persists checkpoint/message/task/event facts.
- Preview image review can use a vision-capable text model.
- Accepted review writes `review_accepted`, selects the artifact version through `production.Service.SelectArtifactVersion`, and preserves stale propagation.
- Rejected review writes `review_rejected` and, when attempts remain, creates a traceable retry chain back through Craftsman/Worker.
- Retry never deletes old versions.
- Retry exhausted writes `retry_exhausted` and Producer can explain it from PSS.
- Dependency scheduler blocks downstream dispatch until upstream dependency readiness is satisfied.
- PSS exposes reviews, retry chains, blocked reasons, and accepted winners.
- Agent UI renders review cards and node review history as structured UI, not raw JSON.
- Browser E2E proves no refresh is needed for review/retry status updates.

## Execution Checkpoints

Recommended commit checkpoints when implementation starts:

```bash
git add apps/server/migrations/020_m6_review_retry_scheduler.sql apps/server/sqlc/queries/review_record.sql apps/server/internal/store/db
git commit -m "feat: add m6 review records"

git add apps/server/internal/agent/reviewer apps/server/internal/agent/tools apps/server/cmd/server/main.go
git commit -m "feat: add agent reviewer graph and tools"

git add apps/server/internal/agent/scheduler apps/server/internal/agent/pss apps/server/internal/api
git commit -m "feat: add agent review retry scheduler"

git add apps/web/src apps/server/internal/agent/uimessage scripts/smoke-m6-7-review-retry-scheduler.sh
git commit -m "feat: render agent review workflow"
```

Do not commit during this planning step unless the user explicitly asks.
