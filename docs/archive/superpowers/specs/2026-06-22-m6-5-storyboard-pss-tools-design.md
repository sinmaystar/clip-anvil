# M6.5 Storyboard / PSS / Production State Edges Design

**Status**: Draft for review
**Date**: 2026-06-22
**Milestone**: M6 MultiAgent Agent Mode

## Goal

M6.5 makes the Agent capable of turning conversation into durable production structure. It adds `shot`, `shot_dependency`, `get_production_state`, `update_storyboard`, and the first full Producer PSS builder.

After this phase, Producer can create and update a storyboard through tools, persist shots and cross-shot dependencies, and read a deterministic project state summary built from database facts. M6.5 does not generate previews, videos, reviews, or final compositions. Those start in M6.6+ and depend on the shot/PSS foundation built here.

## Current State

The current M6 worktree already has:

- generic Agent runtime persistence: `agent_thread`, `agent_message`, `agent_task`, `agent_event`, `eino_checkpoint`;
- `/api/agent/...` and `/ws/agent`;
- right-floating ClipAnvil chat panel;
- versioned `clipanvil.agent.message.v1` blocks;
- real streaming Producer model calls with model/thinking selection;
- Edge Registry and tool call persistence;
- first tools: `read_workspace_context`, `create_agent_text_node`, `request_user_decision`;
- HITL checkpoint, card, and resume infrastructure;
- Agent attachment upload that creates Agent-owned source material nodes;
- Studio-derived read-only Agent canvas and node detail from M6.4B.

The missing piece is production semantics. Agent can talk, inspect, request decisions, and create minimal text/source nodes, but it cannot yet represent a storyboard as durable facts. There is no `shot`, no `shot_dependency`, no `update_storyboard`, and no canonical PSS builder.

## Design Decisions

### M6.5 is the Storyboard foundation, not generation

M6.5 stops at planning and state projection:

- create, update, archive, reorder shots;
- create and replace shot dependencies;
- optionally associate existing or future media nodes with `shot_id`;
- expose current state through `get_production_state`;
- inject Producer PSS into each Producer turn.

M6.5 must not submit `GenerationIntent`, create preview/video jobs, run review rubric, schedule Craftsman, or compose final video. This keeps M6.6 clean: Craftsman/Worker can assume shots already exist and are queryable.

### `read_workspace_context` remains lightweight

`read_workspace_context` is a general context tool. It answers "what exists in this workspace?" with a compact summary and source material refs. It should remain cheap and safe to call often.

`get_production_state` is the canonical PSS tool. It answers "what is the current production state?" and returns both:

- natural-language PSS for model context;
- structured state for tool results, tests, and later scheduling.

The Producer context loader should build PSS directly before model calls, not rely on the model to call `get_production_state` every turn. The tool exists so Producer can refresh or expose production state inside the same tool protocol.

### PSS is a deterministic projection, not memory

PSS is rebuilt from DB facts each turn. It is not persisted as a long-term truth source and is not an LLM-written summary.

M6.5 does not implement the full `memory_document` / `memory_revision` system from the broader M6 roadmap. It may include a "memory unavailable / not implemented" section in PSS, and it may use existing user text/source nodes as facts. Full workspace memory should remain a later focused phase unless implementation pressure proves it is required earlier.

### Storyboard is production semantics, not Studio DAG

`shot` is the stable Agent production anchor. It is not a React Flow node and not a Studio-only node.

`media_node.shot_id` is an optional projection link:

- source material, logo, BGM, and final video can have no `shot_id`;
- a shot can later have multiple nodes: brief/script text, preview image, generated video, review artifact;
- changing shot semantics does not directly mutate Studio edge semantics.

`shot_dependency` represents cross-shot production constraints. It must not be collapsed into `media_edge`; Studio edges remain dependency inputs between media nodes.

## Scope

In scope:

- `018_m6_storyboard_pss.sql` migration.
- `shot` table.
- `shot_dependency` table.
- optional nullable `media_node.shot_id`.
- sqlc queries for shots and shot dependencies.
- extending existing Edge Registry with:
  - `get_production_state`;
  - `update_storyboard`.
- Producer PSS builder package.
- Producer context loader injecting PSS into model context.
- Edge call/result messages for storyboard tools using existing UI message blocks.
- Agent read-only canvas/detail can display shot association when available.
- Unit tests, integration tests, browser E2E smoke, and DB spot checks.

Out of scope:

- full `memory_document` / `memory_revision`;
- CraftsmanGraph;
- Worker generation;
- preview image generation;
- video generation;
- review rubric and retry;
- shot dependency scheduling;
- final video composition;
- Studio / Agent import-export;
- a full storyboard editor UI.

## Data Model

### Migration

Use the next migration number in the current worktree:

```text
apps/server/migrations/018_m6_storyboard_pss.sql
```

Do not use `016_m6_storyboard.sql`; `016` and `017` already exist for Agent vision/thinking model changes.

### `shot`

```sql
CREATE TABLE shot (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    client_key TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL,
    title TEXT NOT NULL,
    brief JSONB NOT NULL DEFAULT '{}',
    duration_sec DOUBLE PRECISION,
    narrative_purpose TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'planned',
    craftsman_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT shot_status_check CHECK (status IN (
        'planned',
        'draft',
        'waiting_for_user',
        'approved',
        'preview_running',
        'preview_ready',
        'video_running',
        'video_ready',
        'review_running',
        'approved_output',
        'failed',
        'archived'
    )),
    CONSTRAINT shot_duration_positive CHECK (duration_sec IS NULL OR duration_sec > 0)
);

CREATE INDEX idx_shot_workspace_order ON shot(workspace_id, archived_at, sort_order);
CREATE UNIQUE INDEX idx_shot_workspace_client_key_active
    ON shot(workspace_id, client_key)
    WHERE archived_at IS NULL AND client_key <> '';
```

`client_key` lets a tool call use stable model-facing IDs such as `shot-01` while the database keeps UUIDs. It is unique only among active shots in one workspace. Existing shots can be matched by UUID or `client_key`.

`brief` is JSONB because later Craftsman needs structured fields, but M6.5 should keep it readable:

```json
{
  "summary": "3秒开场钩子，用商品主图和强卖点吸引注意。",
  "visual_direction": "明亮、干净、商业广告质感",
  "script": "开场字幕：15秒看懂这款产品",
  "constraints": ["必须露出品牌名", "节奏快"]
}
```

### `shot_dependency`

```sql
CREATE TABLE shot_dependency (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    from_shot_id UUID NOT NULL REFERENCES shot(id) ON DELETE CASCADE,
    to_shot_id UUID NOT NULL REFERENCES shot(id) ON DELETE CASCADE,
    dependency_type TEXT NOT NULL,
    required_artifact TEXT NOT NULL DEFAULT '',
    injection_role TEXT NOT NULL DEFAULT '',
    blocking_phase TEXT NOT NULL DEFAULT '',
    stale_policy TEXT NOT NULL DEFAULT 'mark_downstream_stale',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT no_shot_self_dependency CHECK (from_shot_id != to_shot_id)
);

CREATE INDEX idx_shot_dependency_workspace ON shot_dependency(workspace_id);
CREATE INDEX idx_shot_dependency_from ON shot_dependency(from_shot_id);
CREATE INDEX idx_shot_dependency_to ON shot_dependency(to_shot_id);
CREATE UNIQUE INDEX idx_shot_dependency_unique_active
    ON shot_dependency(from_shot_id, to_shot_id, dependency_type, blocking_phase);
```

Allowed first-phase `dependency_type` values:

- `story_order`;
- `last_frame_continuity`;
- `same_subject_consistency`;
- `visual_reference`;
- `asset_reuse`;
- `narrative_dependency`.

Allowed first-phase `blocking_phase` values:

- empty string for non-blocking context dependency;
- `preview_generation`;
- `video_generation`;
- `review`;
- `composition`.

M6.5 validates dependency values in Go, not by database enum, so future scheduling can add types without another enum migration.

### `media_node.shot_id`

```sql
ALTER TABLE media_node
ADD COLUMN shot_id UUID REFERENCES shot(id) ON DELETE SET NULL;

CREATE INDEX idx_media_node_shot ON media_node(shot_id);
```

Add sqlc queries to update and list by `shot_id`. Existing nodes keep `NULL`.

## Query Contract

Create:

- `apps/server/sqlc/queries/shot.sql`
- `apps/server/sqlc/queries/shot_dependency.sql`

Required shot queries:

- `CreateShot`
- `GetShotByID`
- `GetShotByClientKey`
- `ListActiveShotsByWorkspace`
- `UpdateShot`
- `ArchiveShot`
- `ReorderShots`
- `SetShotCraftsmanThread`

Required dependency queries:

- `CreateShotDependency`
- `ListShotDependenciesByWorkspace`
- `DeleteShotDependenciesByWorkspace`
- `DeleteShotDependenciesForShot`

Required node query additions:

- `UpdateMediaNodeShot`
- `ListMediaNodesByShot`

Use transactions in service/tool code when replacing storyboard state. A partial `update_storyboard` must not leave shots updated but dependencies from the old storyboard still attached.

## Service Boundaries

Add focused packages:

```text
apps/server/internal/agent/storyboard/
  service.go
  service_test.go

apps/server/internal/agent/pss/
  producer.go
  producer_test.go
```

### Storyboard service

Responsibilities:

- validate Agent workspace ownership/mode before writes;
- resolve shot references by UUID or `client_key`;
- create/update/archive/reorder shots;
- replace dependencies after all referenced shots are resolved;
- optionally attach existing media nodes to shots when requested;
- create `storyboard_updated` agent events for observability;
- return a structured summary for tool result and PSS refresh.

The service should not call model providers or generation services.

### PSS builder

Responsibilities:

- query workspace facts;
- query source material/media nodes;
- query shots and dependencies;
- query generation jobs, current versions, and stale reasons when available through existing M4/M5 read APIs;
- query pending decisions and running tasks;
- render deterministic natural-language PSS;
- return structured state alongside text.

The PSS builder should be deterministic enough for unit tests. Avoid nondeterministic ordering; sort shots by `sort_order`, nodes by creation time/title, tasks/events by created time.

## Edges

M6.5 extends the existing registry in `apps/server/internal/agent/tools`.

### `get_production_state`

Definition:

```json
{
  "name": "get_production_state",
  "description": "Read the current Agent production state, including storyboard shots, shot dependencies, source materials, canvas nodes, versions, stale reasons, pending decisions, and running tasks. Returns deterministic PSS text plus structured state.",
  "parameters": {
    "type": "object",
    "properties": {
      "include_structured": { "type": "boolean" },
      "include_recent_activity": { "type": "boolean" }
    },
    "additionalProperties": false
  }
}
```

Safety:

```json
{
  "read_only": true,
  "writes_canvas": false,
  "uses_production_service": false,
  "max_calls_per_turn": 10
}
```

Result:

```json
{
  "pss": "当前项目：...",
  "structured": {
    "workspace": {},
    "source_materials": [],
    "shots": [],
    "shot_dependencies": [],
    "nodes": [],
    "versions": [],
    "jobs": [],
    "stale_reasons": [],
    "pending_decisions": [],
    "running_tasks": []
  }
}
```

### `update_storyboard`

Definition:

```json
{
  "name": "update_storyboard",
  "description": "Create or modify the Agent storyboard. This writes shot and shot dependency facts only. It does not generate preview images, videos, reviews, or final composition.",
  "parameters": {
    "type": "object",
    "required": ["intent", "shots"],
    "properties": {
      "intent": {
        "type": "string",
        "enum": ["replace", "upsert", "patch", "archive"]
      },
      "shots": {
        "type": "array",
        "items": {
          "type": "object",
          "required": ["client_key", "title"],
          "properties": {
            "id": { "type": "string" },
            "client_key": { "type": "string" },
            "sort_order": { "type": "integer" },
            "title": { "type": "string" },
            "brief": { "type": "object" },
            "duration_sec": { "type": "number" },
            "narrative_purpose": { "type": "string" },
            "status": { "type": "string" },
            "linked_node_ids": {
              "type": "array",
              "items": { "type": "string" }
            }
          }
        }
      },
      "dependencies": {
        "type": "array",
        "items": {
          "type": "object",
          "required": ["from", "to", "dependency_type"],
          "properties": {
            "from": { "type": "string" },
            "to": { "type": "string" },
            "dependency_type": { "type": "string" },
            "required_artifact": { "type": "string" },
            "injection_role": { "type": "string" },
            "blocking_phase": { "type": "string" },
            "stale_policy": { "type": "string" },
            "reason": { "type": "string" }
          }
        }
      },
      "summary": { "type": "string" }
    },
    "additionalProperties": false
  }
}
```

Safety:

```json
{
  "read_only": false,
  "requires_hitl": false,
  "writes_canvas": false,
  "uses_production_service": false,
  "max_calls_per_turn": 5
}
```

`writes_canvas=false` is intentional. M6.5 writes storyboard facts and optional `media_node.shot_id` associations, but it does not create visual nodes or generation artifacts. Later generation tools will create Agent media nodes.

Supported intents:

- `replace`: archive active shots not present in the request, upsert provided shots, replace all workspace dependencies.
- `upsert`: create or update provided shots, merge/replace dependencies included in the call.
- `patch`: update fields on referenced shots without archiving omitted shots.
- `archive`: archive referenced shots and delete dependencies connected to them.

Validation:

- only Agent workspaces can run this tool;
- `client_key` must be stable, short, and unique within active workspace shots;
- dependency references must resolve to active shots in the same workspace;
- self-dependencies are rejected;
- unknown `dependency_type` or `blocking_phase` is rejected with structured tool error;
- `replace` with empty shots is allowed only when `summary` explains intentional storyboard clearing.

Edge result:

```json
{
  "status": "succeeded",
  "storyboard": {
    "shots_created": 5,
    "shots_updated": 0,
    "shots_archived": 0,
    "dependencies_created": 2
  },
  "shots": [
    {
      "id": "uuid",
      "client_key": "shot-01",
      "title": "开场钩子",
      "status": "planned"
    }
  ],
  "pss": "Storyboard 已更新：..."
}
```

## ProducerGraph Integration

Current graph node:

```text
load_context -> draft_response -> finalize_response
```

M6.5 keeps this node but upgrades `load_context`:

```text
load_context
  -> load messages
  -> resolve model
  -> hydrate image attachments
  -> build Producer PSS
draft_response
  -> responder receives messages + same-turn tool results + PSS
  -> existing bounded tool loop can call update_storyboard/get_production_state
finalize_response
  -> unchanged
```

`ProducerContext` should gain:

```go
PSS ProducerPSS
```

or a minimal equivalent:

```go
ProductionStateText string
ProductionState map[string]any
```

The responder prompt must include PSS as system/context material. It should clearly instruct Producer:

- use `update_storyboard` for durable storyboard changes;
- do not claim storyboard was saved unless tool result succeeded;
- use `request_user_decision` if user confirmation is needed;
- do not generate previews/videos in M6.5;
- after changing storyboard, summarize saved shot IDs and ask for confirmation before generation in later stages.

## PSS Format

Producer PSS is natural language with stable section headers:

```text
当前项目
- Workspace: 622-agent-pro
- Mode: agent

用户素材
- [node:...] design_pic.png, image, source=agent, asset=...

Storyboard
- [shot-01] 开场钩子, 3s, status=planned
  目标: attention
  Brief: 用商品主图和强卖点吸引注意。
- [shot-02] 产品细节, 5s, status=planned

分镜依赖
- shot-02 -> shot-03: last_frame_continuity, blocking_phase=video_generation
  Reason: shot-03 需要承接 shot-02 末帧动作。

生产节点
- image node design_pic.png: source material, current_version=none, status=succeeded

待处理决策
- 无

正在运行
- 无
```

Rules:

- PSS should mention "当前还没有 storyboard" when no shots exist.
- PSS should not include hidden provider secrets or expiring signed URLs.
- PSS can include IDs needed for tool references.
- PSS should include enough detail for "第二个分镜重做" style instructions to resolve to `shot-02`.
- PSS should be compact by default; long fields such as full prompts can be summarized unless explicitly requested by a detail tool later.

## Frontend Behavior

M6.5 is mostly backend/runtime work. Frontend changes stay small:

- render `tool_status` blocks for `get_production_state` and `update_storyboard` using existing Agent tool status renderer;
- show `shot_id` / `client_key` / shot title in Agent read-only node detail when a node is linked to a shot;
- optionally show a compact "Storyboard" read-only summary band on the Agent page if it can be done without competing with the right floating chat.

Do not build a full storyboard editor in M6.5. User changes should go through ClipAnvil conversation and tools.

## Error Handling

Edge failures must be visible and diagnosable:

- validation failures return `agent_storyboard_validation_failed`;
- unresolved shot references return `agent_shot_not_found`;
- non-Agent workspace writes return `agent_workspace_mode_invalid`;
- dependency cycles can be accepted in M6.5 only if non-blocking, but blocking dependency cycles must return `agent_shot_dependency_cycle`;
- transaction failures return `agent_storyboard_update_failed` with logs containing workspace ID, tool call ID, and failing operation.

All failures should persist existing tool result/error blocks through the M6.4 tool executor path.

## Acceptance Criteria

M6.5 is complete only when:

- `shot` and `shot_dependency` migrations apply cleanly.
- `media_node.shot_id` exists and existing nodes continue to work with `NULL`.
- sqlc queries can create, update, list, archive, and link shots.
- `get_production_state` is registered and returns PSS text.
- `update_storyboard` is registered and persists shots/dependencies through a transaction.
- Producer context includes PSS before model calls.
- ProducerGraph can call `update_storyboard` through the existing tool loop.
- Edge call/result messages appear in the ClipAnvil chat.
- Refreshing the Agent workspace preserves storyboard facts.
- PSS describes shots and dependencies deterministically.
- Agent read-only node detail displays shot association when present.
- Ordinary Studio mutation APIs remain blocked in Agent mode.
- No preview/video/final generation is introduced by this phase.

## Testing Strategy

Backend unit/integration tests:

- migration and sqlc compile;
- `StoryboardService.Replace` creates shots and dependencies transactionally;
- invalid dependency references fail without partial writes;
- `ArchiveShot` removes connected dependencies or excludes archived shots from active queries;
- `ProducerPSSBuilder` renders empty and populated storyboard states;
- `get_production_state` returns PSS and structured state;
- `update_storyboard` validates Agent workspace mode and persists facts;
- Producer context loader includes PSS;
- ProducerGraph fixture can trigger `update_storyboard`.

Frontend tests:

- Agent tool renderer labels `update_storyboard` and `get_production_state` cleanly;
- read-only node detail source includes shot fields and no edit controls;
- Agent page does not introduce Studio edit affordances.

Browser E2E:

1. Start app with `./scripts/dev-start.sh` and use the printed Vite URL.
2. Create or open an Agent workspace.
3. Upload an image source material.
4. Send a message asking ClipAnvil to create a 3-shot storyboard.
5. Use deterministic fixture or a real model prompt path that triggers `update_storyboard`.
6. Confirm chat shows the `update_storyboard` tool status.
7. Refresh the page.
8. Confirm `get_production_state` / DB state still lists the shots.
9. Confirm PSS text includes those shots and dependencies.
10. Confirm ordinary Agent Workspace canvas mutation API still returns `403`.

DB spot checks:

```sql
SELECT client_key, sort_order, title, status
FROM shot
WHERE workspace_id = '<workspace-id>' AND archived_at IS NULL
ORDER BY sort_order;

SELECT dependency_type, blocking_phase, reason
FROM shot_dependency
WHERE workspace_id = '<workspace-id>';

SELECT message_type, content
FROM agent_message
WHERE workspace_id = '<workspace-id>'
ORDER BY seq DESC
LIMIT 10;
```

## Required Verification Commands

Run after implementation:

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

If frontend changes are limited to source-level tests and display fields, `pnpm --filter @clip-anvil/web test:connections`, build, and lint are still required because Agent UI is currently changing quickly.

## Follow-Up: M6.6 Boundary

M6.6 should start from these M6.5 facts:

- each shot has stable UUID and `client_key`;
- shot dependencies are queryable;
- Producer PSS can describe the storyboard;
- Agent tool messages can audit production actions.

M6.6 can then add CraftsmanGraph, shot-scoped threads, `generate_shot_preview`, and Worker generation through the existing M4/M5 production job/version/stale/provider chain.

## Spec Self-Review

- No placeholder requirements remain.
- Migration number avoids the existing `016` and `017` files.
- The phase does not include preview/video/review/composer work.
- PSS is explicitly DB-derived and not persisted as truth.
- `read_workspace_context` and `get_production_state` have separate roles.
- The design extends existing Edge Registry, UI message blocks, Agent runtime, and ProducerGraph instead of creating a parallel Agent execution path.
