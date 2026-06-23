# M6.6 Craftsman / Worker Preview Generation Design

**Status**: Draft for review
**Date**: 2026-06-22
**Milestone**: M6 MultiAgent Agent Mode

## Goal

M6.6 turns the existing Agent storyboard into the first real production loop:

```text
Producer conversation
-> dispatch_craftsman tool
-> one persistent CraftsmanGraph per shot
-> one independent Worker task per generation request
-> existing GenerationIntent / generation_job / artifact_version pipeline
-> Agent read-only canvas shows generated preview image nodes
```

This phase focuses on preview image generation. It does not implement video generation, review rubric, critique-based prompt rewriting, cross-shot dependency scheduling, or final composition. Those remain later M6 phases.

## Current State

The worktree already has:

- Agent runtime persistence: `agent_thread`, `agent_message`, `agent_task`, `agent_event`, `eino_checkpoint`.
- Producer conversation thread and right-side Agent chat panel.
- Eino-based Producer model calls with streaming text and thinking.
- Eino-native tool calling through `schema.Message.ToolCalls` and `compose.ToolsNode`.
- Producer tools including `read_workspace_context`, `get_production_state`, `update_storyboard`, `request_user_decision`, and minimal node creation.
- Storyboard tables: `shot`, `shot_dependency`, and `media_node.shot_id`.
- Production pipeline from M4/M5:
  - `production.GenerationIntent`;
  - `production.Service.SubmitGenerationIntent`;
  - `generation_job`;
  - `artifact_version`;
  - provider defaults and model capability selection.

The missing capability is dispatching from Producer into shot-scoped creative execution and then into the existing generation pipeline.

## Confirmed Decisions

1. Producer calls a tool named `dispatch_craftsman`.
2. `CraftsmanGraph` is an independent Eino Graph, not an inline Producer subroutine.
3. `CraftsmanGraph` must have checkpoint persistence and historical message persistence.
4. Worker is an independent `agent_task`, not a direct function body inside Producer or Craftsman.
5. For preview generation, Worker can create the corresponding image node by default before submitting `GenerationIntent`.
6. Failure retry starts with a fixed retry count. M6.6 uses `3` attempts by default.

## Design Position

Use custom Eino Graph orchestration for Craftsman, matching the Producer direction:

```text
load_shot_context
-> draft_preview_strategy
-> persist_strategy
-> enqueue_worker_generation
-> finalize_dispatch
```

Do not use high-level ReAct Agent wrappers in this phase. ClipAnvil needs explicit control over:

- checkpoint keys;
- `agent_thread` and `agent_message` persistence;
- task/event lifecycle;
- frontend status broadcasting;
- fixed retry behavior;
- later review / retry / dependency scheduling branches.

Eino remains the model and graph runtime. Production generation remains the existing ClipAnvil `production.Service` boundary.

## Scope

### In Scope

- Add Producer tool `dispatch_craftsman`.
- Add `CraftsmanGraph` as an independent Eino Graph.
- Create or reuse one active `agent_thread(role='craftsman', scope_type='shot')` per shot.
- Link `shot.craftsman_thread_id` to that thread.
- Persist Craftsman system/context/assistant messages in `agent_message`.
- Persist CraftsmanGraph checkpoints in `eino_checkpoint`.
- Add task types:
  - `craftsman_turn`;
  - `worker_generation`.
- Add Worker executor for preview image generation.
- Worker creates Agent-owned image nodes for preview generation when no target node is supplied.
- Worker calls `production.Service.SubmitGenerationIntent`.
- Generated nodes are associated with the shot through `media_node.shot_id`.
- Generation results continue to be tracked by `generation_job` and `artifact_version`.
- Fixed retry policy with default `max_attempts = 3`.
- Structured logs and tests for dispatch, Craftsman persistence, Worker submission, and failure paths.
- Browser E2E for real Agent conversation that dispatches preview generation from storyboard shots.

### Out of Scope

- Full review rubric.
- Critique-based prompt rewriting.
- Automatic retry after visual review rejection.
- Cross-shot dependency scheduling.
- Video generation.
- Composer / final video.
- Studio / Agent import-export.
- Running shell, FFmpeg, or script work outside the sandbox.

## Producer Tool: `dispatch_craftsman`

### Purpose

`dispatch_craftsman` is the Producer-visible scheduling tool. Producer does not directly generate prompts or call production providers. It selects target shots and asks the runtime to start shot-scoped Craftsman work.

### Tool Schema

```json
{
  "type": "object",
  "properties": {
    "shot_refs": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Shot UUIDs or stable client keys such as shot-01. Empty means all active planned shots."
    },
    "mode": {
      "type": "string",
      "enum": ["preview_image"],
      "description": "The production phase to dispatch. M6.6 only supports preview_image."
    },
    "force": {
      "type": "boolean",
      "description": "When true, create a new preview attempt even if the shot already has a submitted preview job."
    },
    "max_attempts": {
      "type": "integer",
      "minimum": 1,
      "maximum": 3,
      "description": "Fixed retry cap for Craftsman/Worker work. Defaults to 3."
    }
  },
  "required": ["mode"]
}
```

### Behavior

1. Resolve `shot_refs` by UUID or `shot.client_key`.
2. If `shot_refs` is empty, select active shots whose status is `planned`, `draft`, `failed`, or `preview_ready` when `force=true`.
3. For each shot:
   - get or create the active Craftsman thread;
   - set `shot.craftsman_thread_id` if missing;
   - create `agent_task(role='craftsman', task_type='craftsman_turn', scope_type='shot')`;
   - enqueue the Craftsman runner.
4. Return structured dispatch results:

```json
{
  "dispatched": [
    {
      "shot_id": "...",
      "client_key": "shot-01",
      "craftsman_thread_id": "...",
      "craftsman_task_id": "...",
      "status": "queued"
    }
  ],
  "skipped": []
}
```

The tool result should be visible to the model, persisted as a tool result message, and broadcast to the frontend as an Agent status event.

## CraftsmanGraph

### Thread and Scope

Each shot has one persistent Craftsman thread:

```text
agent_thread.role = craftsman
agent_thread.scope_type = shot
agent_thread.scope_id = shot.id
```

`shot.craftsman_thread_id` is the fast link. If a thread exists by scope but the shot link is missing, runtime should repair the link instead of creating a duplicate thread.

### Checkpoint Keys

Craftsman checkpoint keys are deterministic per task:

```text
craftsman:<workspace_id>:<shot_id>:<task_id>
```

`agent_thread.current_checkpoint_key` points to the latest checkpoint for that shot's active Craftsman execution.

### Message Persistence

Craftsman messages are stored in the Craftsman thread, not in the Producer thread:

- system/context message with scoped PSS;
- assistant message containing strategy and prompt draft;
- status/error messages when a Worker task is created or fails to be queued.

Producer can summarize dispatch results in its own thread, but the detailed shot creative chain belongs to the Craftsman thread.

### Scoped Context

Craftsman uses a scoped PSS builder:

- the target shot;
- shot dependencies relevant to this shot;
- source material nodes required for the shot;
- existing nodes attached to the shot;
- latest generation jobs and artifact versions for those nodes;
- Producer instruction that triggered this dispatch.

Craftsman should not receive the full workspace conversation unless later context compression explicitly selects it. Historical Producer chat is already represented by durable storyboard/PSS facts.

### Model Output Contract

M6.6 Craftsman model output should be constrained to structured JSON, parsed and validated by Go:

```json
{
  "strategy": "One paragraph creative direction for this shot.",
  "preview_prompt": "Provider-ready image generation prompt.",
  "negative_prompt": "Optional negative prompt.",
  "style_notes": ["commercial", "clean light"],
  "input_node_refs": ["..."],
  "model": {
    "provider": "",
    "model_id": ""
  },
  "params": {}
}
```

Empty `model.provider` / `model_id` means Worker uses production provider defaults for `text_to_image`.

### Graph Flow

```text
load_shot_context
-> call_craftsman_model
-> validate_strategy
-> persist_strategy_message
-> create_worker_generation_task
-> mark_craftsman_task_succeeded
```

Failure flow:

```text
model/config/validation failure
-> retry up to max_attempts
-> mark_craftsman_task_failed
-> emit agent_event(craftsman_failed)
```

## Worker Generation Task

### Role

Worker is an independent task:

```text
agent_task.role = worker
agent_task.task_type = worker_generation
agent_task.scope_type = shot
agent_task.scope_id = shot.id
```

The Worker is not an Eino model graph in M6.6. It is deterministic execution that translates a validated Craftsman strategy into a node and a `GenerationIntent`.

### Input

```json
{
  "mode": "preview_image",
  "shot_id": "...",
  "craftsman_thread_id": "...",
  "craftsman_task_id": "...",
  "strategy": "...",
  "prompt": "...",
  "negative_prompt": "",
  "input_node_refs": [],
  "target_node_id": "",
  "model": {
    "provider": "",
    "model_id": ""
  },
  "params": {},
  "max_attempts": 3
}
```

### Default Node Creation

If `target_node_id` is empty, Worker creates an Agent-owned image node before submitting generation:

- `media_node.workspace_id = workspace.id`
- `media_node.node_type = 'image'`
- `media_node.source = 'agent'`
- `media_node.status = 'queued'`
- `media_node.operation_type = 'text_to_image'`
- `media_node.prompt` and `prompt_template` = Craftsman preview prompt
- `media_node.shot_id = shot.id`
- `media_node.model_provider` / `model_id` / `model_params` from Craftsman or provider defaults when known
- `media_node.metadata` includes:
  - `agent_artifact_kind = 'preview_image'`
  - `craftsman_thread_id`
  - `craftsman_task_id`
  - `worker_task_id`
  - `shot_client_key`

Use a focused sqlc query for this rather than creating a generic node and patching every field separately. The Agent canvas must render this through the same Studio canvas node renderer in read-only mode.

### GenerationIntent

Worker submits:

```go
production.GenerationIntent{
    WorkspaceID:   workspaceID,
    TargetNodeID:  targetNodeID,
    OutputType:    "image",
    OperationType: "text_to_image",
    PromptTemplate: prompt,
    RenderedPrompt: prompt,
    InputRefs:     resolvedInputRefs,
    Model:         selectedModel,
    Params:        params,
    RequestedBy: production.RequestedBy{
        Type: "agent_worker",
        ID: workerTaskID,
    },
}
```

`production.Service.SubmitGenerationIntent` remains the only path that creates `generation_job` and `artifact_version`.

### Worker Success Semantics

M6.6 defines Worker success as:

```text
target node created or resolved
AND generation intent accepted
AND generation_job/artifact_version created
```

The actual provider run may still be queued/running/succeeded/failed after the Worker task succeeds. The long-running generation status remains in `generation_job` and `artifact_version`; PSS and frontend should read those facts instead of overloading `agent_task.status`.

Worker task output:

```json
{
  "status": "submitted",
  "node_id": "...",
  "generation_job_id": "...",
  "artifact_version_id": "...",
  "operation_type": "text_to_image"
}
```

## Retry Policy

M6.6 uses a fixed retry cap:

- default `max_attempts = 3`;
- tool schema caps `max_attempts` at `3`;
- Craftsman model call and strategy validation can retry up to this cap;
- Worker node creation / `SubmitGenerationIntent` synchronous failures can retry up to this cap;
- production job provider retry should use existing `RunOptions.MaxAttempts = 3` where supported.

Out of scope for M6.6:

- visual review rejection retry;
- prompt critique rewrite retry;
- dependency-aware retry;
- automatic retry after an already-submitted async job later fails.

Those belong to M6.7 review/retry.

## Data Model Changes

Add a migration such as:

```text
apps/server/migrations/019_m6_craftsman_worker_preview.sql
```

Required changes:

1. Extend `agent_task.task_type` check to include:
   - `craftsman_turn`;
   - `worker_generation`.
2. Ensure Go-side validation allows these task types.
3. No new table is required for M6.6.
4. Add sqlc query for Agent generation node creation.

The existing tables remain the source of truth:

- `agent_thread`: Producer and Craftsman threads.
- `agent_message`: Producer/Craftsman history.
- `agent_task`: Producer/Craftsman/Worker execution lifecycle.
- `agent_event`: runtime and UI events.
- `eino_checkpoint`: graph state.
- `shot`: storyboard semantics and Craftsman link.
- `media_node`: canvas projection and generated node metadata.
- `generation_job`: production run state.
- `artifact_version`: generated artifact versions.

## Events and Observability

Emit structured events:

- `craftsman_dispatched`
- `craftsman_started`
- `craftsman_strategy_created`
- `worker_generation_queued`
- `worker_generation_submitted`
- `worker_generation_failed`
- `craftsman_failed`

Structured logs must include:

- `workspace_id`
- `shot_id`
- `shot_client_key`
- `producer_task_id`
- `craftsman_thread_id`
- `craftsman_task_id`
- `worker_task_id`
- `node_id`
- `generation_job_id`
- `artifact_version_id`
- `attempt`
- `max_attempts`
- provider/model selected for generation
- full error code and concise error message on failure

The log boundary matters because provider failures may happen after Agent task submission. Logs should make it possible to correlate Producer tool call, Craftsman task, Worker task, generation job, and artifact version.

## Frontend Expectations

M6.6 does not require a new storyboard editor. The existing Agent UI should support:

- user sends a natural language request to generate previews;
- Producer emits a `dispatch_craftsman` tool call;
- chat shows dispatch/task status messages without exposing internal "Producer" naming to the user;
- Agent read-only canvas receives generated image nodes through existing canvas data/WS flow;
- node detail panel can show prompt, model, params, generation job, artifact version, and shot association in read-only mode.

The canvas node must reuse Studio node rendering. Agent mode may disable editing, dragging, and direct mutation, but it should not render a separate simplified card that hides the production fields.

## Acceptance Criteria

- Producer can call `dispatch_craftsman` through Eino native tool calling.
- `dispatch_craftsman` queues one `craftsman_turn` per selected shot.
- Each selected shot has exactly one active Craftsman thread linked by `shot.craftsman_thread_id`.
- CraftsmanGraph writes messages to the Craftsman thread.
- CraftsmanGraph writes a checkpoint to `eino_checkpoint`.
- CraftsmanGraph creates a `worker_generation` task.
- Worker creates or resolves an image target node linked to the shot.
- Worker submits a `GenerationIntent` through `production.Service.SubmitGenerationIntent`.
- `generation_job` and `artifact_version` records are created for the preview.
- Worker task output includes node/job/version IDs.
- Fixed retry count is applied and visible in task records/logs.
- Failed dispatch/model/worker paths persist useful error code/message.
- Agent canvas shows the generated image node using Studio canvas rendering in read-only mode.

## Verification Commands

Server and database:

```bash
make sqlc-generate
make server-build
make server-test
make server-lint
```

Frontend:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Runtime and E2E:

```bash
./scripts/dev-start.sh
```

Use the script output Vite URL, then run browser E2E against that URL:

1. Create or open an Agent workspace.
2. Ask ClipAnvil to create a 3-shot storyboard.
3. Ask ClipAnvil to generate preview images for all shots.
4. Confirm the model emits native `dispatch_craftsman` tool calls.
5. Confirm the chat shows dispatch/progress without exposing "Producer" to the user.
6. Confirm canvas shows one generated image node per shot.
7. Open a generated node detail panel and verify prompt/model/job/version/shot metadata are visible read-only.

Database spot checks:

```sql
SELECT id, role, scope_type, scope_id
FROM agent_thread
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;

SELECT id, role, task_type, status, attempt, max_attempts, scope_type, scope_id, output, error_code, error_message
FROM agent_task
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;

SELECT id, client_key, status, craftsman_thread_id
FROM shot
WHERE workspace_id = '<workspace-id>'
ORDER BY sort_order;

SELECT id, node_type, source, operation_type, status, shot_id, current_version_id, metadata
FROM media_node
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;

SELECT id, target_node_id, operation_type, provider, model_id, status, attempt, max_attempts, requested_by_type, requested_by_id, error_code, error_message
FROM generation_job
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;
```

Stop runtime after E2E:

```bash
./scripts/dev-stop.sh
```

General:

```bash
git diff --check
```

## Implementation Notes

- Do not add a second production pipeline for Agent. Worker must reuse `production.Service`.
- Do not let Producer create generation jobs directly.
- Do not store Craftsman creative history in the Producer thread.
- Do not make Worker an LLM agent in M6.6; keep it deterministic.
- Keep task status and generation job status separate.
- Prefer deterministic layout for auto-created preview nodes, such as shot order columns below source material, so E2E can assert their existence without fragile coordinates.
- If no real image provider is configured in local E2E, the test may use the existing mock provider, but the acceptance path must still create real `generation_job` and `artifact_version` records.

