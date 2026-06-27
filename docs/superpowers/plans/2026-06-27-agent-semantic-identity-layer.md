# Agent Semantic Identity Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a first-class semantic identity layer so Producer, Craftsman, Reviewer, tools, and users operate on stable semantic keys instead of raw UUIDs, including Worker-generated images and videos.

**Architecture:** UUID remains the internal database primary key and foreign-key mechanism. `semantic_key` becomes the canonical Agent-facing identity for every Agent-operable object, and a centralized resolver maps semantic references to UUIDs before business logic runs. `read_project_context` becomes the authoritative object index that returns current DB facts plus actionable semantic references for storyboard, render plans, generated artifacts, reviews, issues, and pending work.

**Tech Stack:** Go 1.26, PostgreSQL 16, pgx v5, sqlc, Hertz, CloudWeGo Eino native ToolNode, React 19 + TypeScript 6 + `@xyflow/react`.

---

## Scope

This plan implements the long-term design. It does not use `media_node.metadata` as the primary identity layer and does not ask models to remember or generate UUIDs.

Included:

- Database-level `semantic_key` and `display_name` fields for Agent-operable domain, production, review, and runtime records.
- Unique semantic-key constraints scoped by `workspace_id` where applicable.
- A centralized `ObjectResolver` that resolves semantic references into checked DB objects.
- An `agent_object_index` read model for `read_project_context`, tool validation, and canvas projection.
- Semantic key generation rules for scenes, shots, key elements, states, render plans, generated nodes, artifact versions, reviews, issues, worker jobs, and Agent runtime records.
- Tool input refactor from UUID-first parameters to `*_ref` semantic references.
- `read_project_context` input and output refactor: it accepts semantic scope refs, returns actionable object indexes instead of mostly numeric summaries, and never asks the model to copy raw UUIDs for normal actions.
- Producer PSS, tool results, runtime-trigger messages, and system reminders use semantic refs for actionable objects. Raw IDs may appear only in explicitly marked internal/debug fields.
- Worker image/video generation identity propagation using schema fields, not metadata-only conventions.
- Backend, frontend, and E2E tests proving multi-turn Agent operation without model-generated UUIDs.

Excluded:

- Changing physical media storage keys in MinIO.
- Changing account authentication.
- Replacing React Flow or the Agent canvas layout system.
- Making every internal runtime table user-editable. Runtime records get semantic keys for observability and resolution, not for arbitrary user mutation.

## Current Code Facts

- `scene`, `shot`, `key_element`, and `key_element_state` currently use `client_key`.
- `render_plan` has `render_plan_key`, but it currently includes UUID: `scope_type:scope_uuid:target_phase:revision`.
- `media_node`, `generation_job`, `artifact_version`, `review_record`, `artifact_issue`, `agent_thread`, `agent_task`, and `producer_pending_signal` do not have first-class semantic keys.
- Worker currently writes artifact kind and some semantic hints into `media_node.metadata`; this is useful for backfill, but must not remain the primary identity mechanism.
- Producer native tools registered in `apps/server/cmd/server/main.go` mostly expose UUID-based fields or tool-specific client-key fields.
- `read_project_context` currently accepts `scope.id` for non-workspace scopes and exposes executable ID summaries such as `shot_id`, `node_id`, and `version_id` when production state is requested.

## Implementation Progress

已落地：

- 数据库迁移 `030_agent_semantic_identity.sql` 已为 Agent 可操作对象、生产对象、评审对象和运行时对象增加 `semantic_key/display_name`，并创建 `agent_object_index` 读模型。
- `read_project_context` 已接入 `agent_object_index`，支持 `scope_ref` 语义范围读取，并移除了生产状态决策文本中的可执行 UUID 摘要。
- Creative state 写侧已生成 creative brief、project memory、key element、key element state、scene、shot、shot dependency 的语义键。
- Producer `dispatch_craftsman` 已支持使用 shot / key_element_state 的 `semantic_key` 派发，并把 `scope_key` 传给 Craftsman task。
- Craftsman native tool runtime 已继承 `scope_key`，`upsert_render_plan` 已用 `scope_key` 生成 `render_plan_key/semantic_key`，不再用 scope UUID 拼 RenderPlan key。
- Producer `decide_render_plan` 已支持 `render_plan_ref:{type:"render_plan",key:"..."}`，兼容 UUID 字段仅作为旧路径 fallback。
- Worker / Production 写侧已把 RenderPlan 语义身份传播到 `media_node`、`generation_job`、`artifact_version` 的 schema 字段。

仍需继续：

- Reviewer 相关工具仍有 UUID-first target 字段，需要改成 `target_ref` / `artifact_ref` 并由工程侧解析。
- `upsert_render_plan` 的 `update_draft/fork_from/mark_blocked` 仍保留 `render_plan_id/fork_from_render_plan_id` 兼容字段，需要补 `render_plan_ref/fork_from_render_plan_ref`。
- pending signal / runtime trigger / Reviewer signal 的 payload 仍需全面补 semantic refs，避免 Producer 依赖裸 ID。
- 前端 ObjectIndex 调试/渲染辅助、语义 selector 的 E2E 校验还未完成。

## File Structure

Create:

- `apps/server/migrations/030_agent_semantic_identity.sql`: add semantic identity columns, backfill existing records, create unique indexes, and create `agent_object_index`.
- `apps/server/sqlc/queries/semantic_identity.sql`: resolver and object-index queries.
- `apps/server/internal/agent/identity/types.go`: semantic reference structs, object types, selectors, and resolved object structs.
- `apps/server/internal/agent/identity/keygen.go`: deterministic semantic-key generation.
- `apps/server/internal/agent/identity/keygen_test.go`: key generation tests.
- `apps/server/internal/agent/identity/resolver.go`: centralized semantic reference resolver.
- `apps/server/internal/agent/identity/resolver_test.go`: resolver tests with fake store.
- `apps/server/internal/agent/identity/object_index.go`: typed object-index builder and natural-language renderer.
- `apps/server/internal/agent/identity/object_index_test.go`: index rendering tests.
- `apps/server/internal/agent/tools/semantic_refs.go`: shared tool input structs and validation helpers.
- `apps/server/internal/agent/tools/semantic_refs_test.go`: tool ref validation tests.
- `apps/server/internal/api/agent_object_index_handler.go`: debug/read API for semantic object index.
- `apps/web/src/lib/agentSemanticIdentity.ts`: frontend types and formatting helpers.

Modify:

- `apps/server/sqlc/queries/creative_state.sql`: rename or expose semantic-key lookups for creative objects.
- `apps/server/sqlc/queries/shot.sql`: semantic-key shot lookups and updates.
- `apps/server/sqlc/queries/shot_dependency.sql`: semantic-key dependency queries.
- `apps/server/sqlc/queries/render_plan.sql`: semantic-key render plan lookup and creation.
- `apps/server/sqlc/queries/node.sql`: semantic-key media node lookup and creation.
- `apps/server/sqlc/queries/production.sql`: semantic-key generation job and artifact version writes.
- `apps/server/sqlc/queries/review_record.sql`: semantic-key review lookup.
- `apps/server/sqlc/queries/artifact_issue.sql`: semantic-key issue lookup.
- `apps/server/sqlc/queries/agent_thread.sql`: semantic-key runtime thread lookup.
- `apps/server/sqlc/queries/agent_task.sql`: semantic-key runtime task lookup.
- `apps/server/internal/store/db/*.go`: generated by `make sqlc-generate`.
- `apps/server/internal/agent/creative/state_service.go`: write and resolve semantic keys for creative state.
- `apps/server/internal/agent/creative/types.go`: replace public `ClientKey` naming with `SemanticKey` in service inputs while keeping DB migration explicit.
- `apps/server/internal/agent/renderplan/service.go`: generate semantic render plan keys.
- `apps/server/internal/agent/tools/read_project_context.go`: return semantic object index and accept semantic scope refs.
- `apps/server/internal/agent/tools/upsert_storyboard.go`: accept `semantic_key` and semantic refs.
- `apps/server/internal/agent/tools/upsert_key_elements.go`: accept `semantic_key` and state semantic refs.
- `apps/server/internal/agent/tools/upsert_render_plan.go`: accept scope refs and artifact refs instead of naked UUIDs.
- `apps/server/internal/agent/tools/dispatch_craftsman.go`: accept semantic scope refs.
- `apps/server/internal/agent/tools/decide_render_plan.go`: accept render plan semantic refs and batch decisions by key.
- `apps/server/internal/agent/tools/dispatch_reviewer.go`: accept artifact and render plan semantic refs.
- `apps/server/internal/agent/tools/submit_review_result.go`: accept target semantic refs and resolve internally.
- `apps/server/internal/agent/tools/select_version.go`: replace or reintroduce as native `select_artifact_version` with semantic refs.
- `apps/server/internal/agent/worker/types.go`: add semantic fields to `GenerationInput`.
- `apps/server/internal/agent/worker/executor.go`: create semantic media nodes, jobs, and artifact versions.
- `apps/server/internal/agent/tools/render_plan_submitter.go`: pass semantic keys into worker input.
- `apps/server/internal/agent/pss/producer.go`: build Producer PSS from semantic object index.
- `apps/server/internal/api/agent_workbench_projection.go`: use semantic hierarchy in workbench response.
- `apps/server/internal/api/agent_canvas_detail.go`: expose semantic keys in node detail.
- `apps/server/cmd/server/main.go`: wire identity resolver into all native tools and APIs.
- `apps/web/src/components/agent-workbench/*`: display semantic keys and use them for detail/action payloads.
- `docs/engineering/agent-multiagent-architecture.md`: update Agent tool identity contract.
- `docs/engineering/database.md`: document semantic identity invariants.

## Semantic Identity Rules

Every Agent-operable object must satisfy:

```text
workspace_id + semantic_key uniquely identifies one active object of an object_type.
semantic_key is stable across title/display name edits.
display_name is user-facing and may change.
UUID is never required in model-authored tool input when a semantic ref can identify the object.
Agent-facing tool descriptions, tool results, system reminders, and Producer signals must not require UUID copying for normal flows.
If an internal/debug ID is unavoidable, label it as read-only diagnostic data and provide a semantic ref next to it.
```

Canonical key rules:

| Object | Key format |
|---|---|
| CreativeBrief | `creative_brief.main` |
| ProjectMemory | `project_memory.v{version}` |
| KeyElement | `element_{slug}` |
| KeyElementState | `{element_key}.state_{slug}` |
| Scene | `scene_{slug}` |
| Shot | `shot_01`, `shot_02`, stable after creation |
| ShotDependency | `dep.{from_shot_key}.to.{to_shot_key}.{dependency_type}.{blocking_phase_or_any}` |
| RenderPlan | `{scope_key}.{target_phase}.rp{revision}` |
| MediaNode | `{render_plan_key}.output` |
| GenerationJob | `{render_plan_key}.job{attempt}` |
| ArtifactVersion | `{media_node_key}.v{version_no}` |
| ReviewRecord | `{target_key}.review.r{attempt_no}` |
| ArtifactIssue | `{target_key}.issue.{dimension}.{sequence}` |
| AgentThread | `{role}.{scope_key}` |
| AgentTask | `{thread_key}.{task_type}.{sequence}` |
| ProducerPendingSignal | `signal.{trigger_object_key}.{signal_type}.{sequence}` |

Selectors are not stored as rows:

```text
shot_03.preview_image.current
shot_03.preview_image.latest
shot_03.shot_video.winner
```

Selectors are resolver inputs that map to a current `artifact_version.id`.

Runtime object keys must prefer the semantic key of the object that triggered the runtime record. Do not make UUID fragments part of the primary Agent-facing key format. Existing records may use an internal/debug fallback during backfill only when no semantic trigger object can be recovered.

## Task 1: Schema Migration for First-Class Semantic Identity

**Files:**

- Create: `apps/server/migrations/030_agent_semantic_identity.sql`
- Modify after generation: `apps/server/internal/store/db/models.go`

- [ ] **Step 1: Write migration**

Create `apps/server/migrations/030_agent_semantic_identity.sql`:

```sql
-- +goose Up
ALTER TABLE creative_brief
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE project_memory
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE key_element
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE key_element_state
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE scene
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE shot
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE shot_dependency
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE render_plan
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE media_node
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN artifact_kind TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL;

ALTER TABLE generation_job
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL;

ALTER TABLE artifact_version
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN artifact_kind TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL;

ALTER TABLE review_record
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE artifact_issue
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE agent_thread
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE agent_task
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

ALTER TABLE producer_pending_signal
    ADD COLUMN semantic_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

UPDATE creative_brief
SET semantic_key = 'creative_brief.main',
    display_name = COALESCE(NULLIF(title, ''), 'Creative Brief')
WHERE semantic_key = '';

UPDATE project_memory
SET semantic_key = 'project_memory.v' || version::text,
    display_name = 'Project Memory v' || version::text
WHERE semantic_key = '';

UPDATE key_element
SET semantic_key = COALESCE(NULLIF(client_key, ''), 'element_' || left(id::text, 8)),
    display_name = COALESCE(NULLIF(name, ''), client_key, 'Key Element')
WHERE semantic_key = '';

UPDATE key_element_state state
SET semantic_key = element.semantic_key || '.state_' || COALESCE(NULLIF(state.client_key, ''), left(state.id::text, 8)),
    display_name = COALESCE(NULLIF(state.label, ''), state.client_key, 'Element State')
FROM key_element element
WHERE state.key_element_id = element.id
  AND state.semantic_key = '';

UPDATE scene
SET semantic_key = COALESCE(NULLIF(client_key, ''), 'scene_' || left(id::text, 8)),
    display_name = COALESCE(NULLIF(title, ''), client_key, 'Scene')
WHERE semantic_key = '';

UPDATE shot
SET semantic_key = COALESCE(NULLIF(client_key, ''), 'shot_' || lpad(sort_order::text, 2, '0')),
    display_name = COALESCE(NULLIF(title, ''), client_key, 'Shot')
WHERE semantic_key = '';

UPDATE shot_dependency dep
SET semantic_key = 'dep.' || from_shot.semantic_key || '.to.' || to_shot.semantic_key || '.' || dep.dependency_type || '.' || COALESCE(NULLIF(dep.blocking_phase, ''), 'any'),
    display_name = from_shot.semantic_key || ' -> ' || to_shot.semantic_key || ' ' || dep.dependency_type
FROM shot from_shot, shot to_shot
WHERE dep.from_shot_id = from_shot.id
  AND dep.to_shot_id = to_shot.id
  AND dep.semantic_key = '';

UPDATE render_plan plan
SET semantic_key =
    CASE
        WHEN plan.scope_type = 'shot' THEN shot.semantic_key || '.' || plan.target_phase || '.rp' || plan.revision::text
        WHEN plan.scope_type = 'key_element_state' THEN state.semantic_key || '.' || plan.target_phase || '.rp' || plan.revision::text
        ELSE plan.render_plan_key
    END,
    display_name = plan.target_phase || ' RenderPlan r' || plan.revision::text
FROM shot
FULL OUTER JOIN key_element_state state ON false
WHERE ((plan.scope_type = 'shot' AND plan.scope_id = shot.id)
    OR (plan.scope_type = 'key_element_state' AND plan.scope_id = state.id))
  AND plan.semantic_key = '';

UPDATE media_node node
SET semantic_key = COALESCE(NULLIF(node.title, ''), 'media_node_' || left(node.id::text, 8)),
    display_name = COALESCE(NULLIF(node.title, ''), 'Media Node'),
    artifact_kind = COALESCE(NULLIF(node.metadata->>'agent_artifact_kind', ''), NULLIF(node.metadata->>'artifact_kind', ''), '')
WHERE node.semantic_key = '';

UPDATE media_node node
SET source_render_plan_id = plan.id,
    semantic_key = plan.semantic_key || '.output',
    display_name = plan.display_name || ' Output',
    artifact_kind = plan.target_phase
FROM render_plan plan
WHERE node.id = plan.output_node_id
  AND plan.output_node_id IS NOT NULL;

UPDATE generation_job job
SET source_render_plan_id = task.render_plan_id,
    semantic_key = COALESCE(plan.semantic_key, 'node_' || left(job.target_node_id::text, 8)) || '.job' || job.attempt::text,
    display_name = 'Generation Job ' || job.attempt::text
FROM agent_task task
LEFT JOIN render_plan plan ON plan.id = task.render_plan_id
WHERE job.requested_by_type = 'agent_worker'
  AND job.requested_by_id = task.id::text
  AND job.semantic_key = '';

UPDATE artifact_version version
SET source_render_plan_id = node.source_render_plan_id,
    semantic_key = node.semantic_key || '.v' || version.version_no::text,
    display_name = node.display_name || ' v' || version.version_no::text,
    artifact_kind = node.artifact_kind
FROM media_node node
WHERE version.node_id = node.id
  AND version.semantic_key = '';

UPDATE review_record review
SET semantic_key =
    CASE
        WHEN version.semantic_key <> '' THEN version.semantic_key || '.review.r' || review.attempt_no::text
        WHEN plan.semantic_key <> '' THEN plan.semantic_key || '.review.r' || review.attempt_no::text
        WHEN shot.semantic_key <> '' THEN shot.semantic_key || '.' || review.target_phase || '.review.r' || review.attempt_no::text
        ELSE 'review_' || left(review.id::text, 8)
    END,
    display_name = review.review_task || ' r' || review.attempt_no::text
FROM artifact_version version
FULL OUTER JOIN render_plan plan ON false
FULL OUTER JOIN shot ON false
WHERE ((review.artifact_version_id = version.id)
    OR (review.render_plan_id = plan.id)
    OR (review.shot_id = shot.id))
  AND review.semantic_key = '';

UPDATE artifact_issue issue
SET semantic_key = COALESCE(review.semantic_key, 'target_' || left(issue.target_object_id::text, 8)) || '.issue.' || issue.dimension || '.' || left(issue.id::text, 8),
    display_name = COALESCE(NULLIF(issue.title, ''), issue.dimension || ' issue')
FROM review_record review
WHERE issue.review_record_id = review.id
  AND issue.semantic_key = '';

UPDATE agent_thread
SET semantic_key = role || '.' || scope_type || '.' || left(scope_id::text, 8),
    display_name = role || ' ' || scope_type
WHERE semantic_key = '';

UPDATE agent_task
SET semantic_key = role || '.' || task_type || '.' || left(id::text, 8),
    display_name = role || ' ' || task_type
WHERE semantic_key = '';

UPDATE producer_pending_signal
SET semantic_key = 'signal.' || signal_type || '.' || left(id::text, 8),
    display_name = signal_type
WHERE semantic_key = '';

-- Runtime backfill is a safe fallback for existing rows. Later write paths in
-- this plan must generate runtime semantic keys from the triggering semantic
-- object whenever possible, for example producer.shot_03,
-- producer.shot_03.producer_turn.4, and
-- signal.shot_03.preview_image.rp1.craftsman_render_plan_ready.1.

CREATE UNIQUE INDEX idx_creative_brief_workspace_semantic_active
    ON creative_brief(workspace_id, semantic_key)
    WHERE archived_at IS NULL AND semantic_key <> '';
CREATE UNIQUE INDEX idx_project_memory_workspace_semantic
    ON project_memory(workspace_id, semantic_key)
    WHERE semantic_key <> '';
CREATE UNIQUE INDEX idx_key_element_workspace_semantic_active
    ON key_element(workspace_id, semantic_key)
    WHERE archived_at IS NULL AND semantic_key <> '';
CREATE UNIQUE INDEX idx_key_element_state_workspace_semantic_active
    ON key_element_state(workspace_id, semantic_key)
    WHERE archived_at IS NULL AND semantic_key <> '';
CREATE UNIQUE INDEX idx_scene_workspace_semantic_active
    ON scene(workspace_id, semantic_key)
    WHERE archived_at IS NULL AND semantic_key <> '';
CREATE UNIQUE INDEX idx_shot_workspace_semantic_active
    ON shot(workspace_id, semantic_key)
    WHERE archived_at IS NULL AND semantic_key <> '';
CREATE UNIQUE INDEX idx_shot_dependency_workspace_semantic
    ON shot_dependency(workspace_id, semantic_key)
    WHERE semantic_key <> '';
CREATE UNIQUE INDEX idx_render_plan_workspace_semantic_active
    ON render_plan(workspace_id, semantic_key)
    WHERE archived_at IS NULL AND semantic_key <> '';
CREATE UNIQUE INDEX idx_media_node_workspace_semantic
    ON media_node(workspace_id, semantic_key)
    WHERE semantic_key <> '';
CREATE UNIQUE INDEX idx_generation_job_workspace_semantic
    ON generation_job(workspace_id, semantic_key)
    WHERE semantic_key <> '';
CREATE UNIQUE INDEX idx_artifact_version_workspace_semantic
    ON artifact_version(workspace_id, semantic_key)
    WHERE semantic_key <> '';
CREATE UNIQUE INDEX idx_review_record_workspace_semantic
    ON review_record(workspace_id, semantic_key)
    WHERE semantic_key <> '';
CREATE UNIQUE INDEX idx_artifact_issue_workspace_semantic
    ON artifact_issue(workspace_id, semantic_key)
    WHERE semantic_key <> '';
CREATE UNIQUE INDEX idx_agent_thread_workspace_semantic
    ON agent_thread(workspace_id, semantic_key)
    WHERE semantic_key <> '';
CREATE UNIQUE INDEX idx_agent_task_workspace_semantic
    ON agent_task(workspace_id, semantic_key)
    WHERE semantic_key <> '';
CREATE UNIQUE INDEX idx_producer_pending_signal_workspace_semantic
    ON producer_pending_signal(workspace_id, semantic_key)
    WHERE semantic_key <> '';

CREATE VIEW agent_object_index AS
SELECT workspace_id, 'creative_brief' AS object_type, id AS object_id, semantic_key, display_name, '' AS parent_object_type, NULL::uuid AS parent_object_id, '' AS parent_semantic_key, status, '' AS kind, 0 AS sort_order, updated_at
FROM creative_brief
WHERE archived_at IS NULL
UNION ALL
SELECT workspace_id, 'project_memory', id, semantic_key, display_name, '', NULL::uuid, '', status, '', version, created_at
FROM project_memory
UNION ALL
SELECT workspace_id, 'key_element', id, semantic_key, display_name, '', NULL::uuid, '', status, element_type, 0, updated_at
FROM key_element
WHERE archived_at IS NULL
UNION ALL
SELECT state.workspace_id, 'key_element_state', state.id, state.semantic_key, state.display_name, 'key_element', state.key_element_id, element.semantic_key, state.status, state.reference_status, 0, state.updated_at
FROM key_element_state state
JOIN key_element element ON element.id = state.key_element_id
WHERE state.archived_at IS NULL
UNION ALL
SELECT workspace_id, 'scene', id, semantic_key, display_name, '', NULL::uuid, '', status, 'scene', sort_order, updated_at
FROM scene
WHERE archived_at IS NULL
UNION ALL
SELECT shot.workspace_id, 'shot', shot.id, shot.semantic_key, shot.display_name, 'scene', shot.scene_id, COALESCE(scene.semantic_key, ''), shot.status, shot.shot_kind, shot.sort_order, shot.updated_at
FROM shot
LEFT JOIN scene ON scene.id = shot.scene_id
WHERE shot.archived_at IS NULL
UNION ALL
SELECT dep.workspace_id, 'shot_dependency', dep.id, dep.semantic_key, dep.display_name, 'shot', dep.to_shot_id, to_shot.semantic_key, '', dep.dependency_type, 0, dep.created_at
FROM shot_dependency dep
JOIN shot to_shot ON to_shot.id = dep.to_shot_id
UNION ALL
SELECT plan.workspace_id, 'render_plan', plan.id, plan.semantic_key, plan.display_name, plan.scope_type, plan.scope_id, COALESCE(shot.semantic_key, state.semantic_key, ''), plan.status, plan.target_phase, plan.revision, plan.updated_at
FROM render_plan plan
LEFT JOIN shot ON plan.scope_type = 'shot' AND plan.scope_id = shot.id
LEFT JOIN key_element_state state ON plan.scope_type = 'key_element_state' AND plan.scope_id = state.id
WHERE plan.archived_at IS NULL
UNION ALL
SELECT node.workspace_id, 'media_node', node.id, node.semantic_key, node.display_name, 'render_plan', node.source_render_plan_id, COALESCE(plan.semantic_key, ''), node.status::text, node.artifact_kind, 0, node.updated_at
FROM media_node node
LEFT JOIN render_plan plan ON plan.id = node.source_render_plan_id
UNION ALL
SELECT version.workspace_id, 'artifact_version', version.id, version.semantic_key, version.display_name, 'media_node', version.node_id, node.semantic_key, version.status::text, version.artifact_kind, version.version_no, version.created_at
FROM artifact_version version
JOIN media_node node ON node.id = version.node_id
UNION ALL
SELECT review.workspace_id, 'review_record', review.id, review.semantic_key, review.display_name, review.target_object_type, review.target_object_id, COALESCE(version.semantic_key, plan.semantic_key, shot.semantic_key, ''), review.status, review.review_task, review.attempt_no, review.created_at
FROM review_record review
LEFT JOIN artifact_version version ON review.target_object_type = 'artifact_version' AND review.target_object_id = version.id
LEFT JOIN render_plan plan ON review.target_object_type = 'render_plan' AND review.target_object_id = plan.id
LEFT JOIN shot ON review.target_object_type = 'shot' AND review.target_object_id = shot.id
UNION ALL
SELECT issue.workspace_id, 'artifact_issue', issue.id, issue.semantic_key, issue.display_name, issue.target_object_type, issue.target_object_id, COALESCE(version.semantic_key, plan.semantic_key, shot.semantic_key, memory.semantic_key, ''), issue.status, issue.dimension, 0, issue.updated_at
FROM artifact_issue issue
LEFT JOIN artifact_version version ON issue.target_object_type = 'artifact_version' AND issue.target_object_id = version.id
LEFT JOIN render_plan plan ON issue.target_object_type = 'render_plan' AND issue.target_object_id = plan.id
LEFT JOIN shot ON issue.target_object_type = 'shot' AND issue.target_object_id = shot.id
LEFT JOIN project_memory memory ON issue.target_object_type = 'project_memory' AND issue.target_object_id = memory.id;

-- +goose Down
DROP VIEW IF EXISTS agent_object_index;
DROP INDEX IF EXISTS idx_producer_pending_signal_workspace_semantic;
DROP INDEX IF EXISTS idx_agent_task_workspace_semantic;
DROP INDEX IF EXISTS idx_agent_thread_workspace_semantic;
DROP INDEX IF EXISTS idx_artifact_issue_workspace_semantic;
DROP INDEX IF EXISTS idx_review_record_workspace_semantic;
DROP INDEX IF EXISTS idx_artifact_version_workspace_semantic;
DROP INDEX IF EXISTS idx_generation_job_workspace_semantic;
DROP INDEX IF EXISTS idx_media_node_workspace_semantic;
DROP INDEX IF EXISTS idx_render_plan_workspace_semantic_active;
DROP INDEX IF EXISTS idx_shot_dependency_workspace_semantic;
DROP INDEX IF EXISTS idx_shot_workspace_semantic_active;
DROP INDEX IF EXISTS idx_scene_workspace_semantic_active;
DROP INDEX IF EXISTS idx_key_element_state_workspace_semantic_active;
DROP INDEX IF EXISTS idx_key_element_workspace_semantic_active;
DROP INDEX IF EXISTS idx_project_memory_workspace_semantic;
DROP INDEX IF EXISTS idx_creative_brief_workspace_semantic_active;

ALTER TABLE producer_pending_signal DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE agent_task DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE agent_thread DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE artifact_issue DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE review_record DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE artifact_version DROP COLUMN IF EXISTS source_render_plan_id, DROP COLUMN IF EXISTS artifact_kind, DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE generation_job DROP COLUMN IF EXISTS source_render_plan_id, DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE media_node DROP COLUMN IF EXISTS source_render_plan_id, DROP COLUMN IF EXISTS artifact_kind, DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE render_plan DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE shot_dependency DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE shot DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE scene DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE key_element_state DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE key_element DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE project_memory DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
ALTER TABLE creative_brief DROP COLUMN IF EXISTS display_name, DROP COLUMN IF EXISTS semantic_key;
```

- [ ] **Step 2: Run migration syntax check**

Run:

```bash
goose -dir apps/server/migrations postgres "$DATABASE_URL" status
```

Expected: migration list prints successfully. If local `goose` is not installed, run the repository migration command used by `scripts/dev-start.sh` and record the exact command output in the implementation notes.

- [ ] **Step 3: Generate sqlc after query task**

Do not run `make sqlc-generate` until Task 2 adds queries.

## Task 2: sqlc Queries for Semantic Identity

**Files:**

- Create: `apps/server/sqlc/queries/semantic_identity.sql`
- Modify generated: `apps/server/internal/store/db/*.go`

- [ ] **Step 1: Add resolver queries**

Create `apps/server/sqlc/queries/semantic_identity.sql`:

```sql
-- name: GetAgentObjectBySemanticKey :one
SELECT workspace_id, object_type, object_id, semantic_key, display_name, parent_object_type, parent_object_id, parent_semantic_key, status, kind, sort_order, updated_at
FROM agent_object_index
WHERE workspace_id = $1
  AND object_type = $2
  AND semantic_key = $3;

-- name: ListAgentObjectsByWorkspace :many
SELECT workspace_id, object_type, object_id, semantic_key, display_name, parent_object_type, parent_object_id, parent_semantic_key, status, kind, sort_order, updated_at
FROM agent_object_index
WHERE workspace_id = $1
ORDER BY object_type, sort_order, semantic_key;

-- name: ListAgentObjectsByParentSemanticKey :many
SELECT workspace_id, object_type, object_id, semantic_key, display_name, parent_object_type, parent_object_id, parent_semantic_key, status, kind, sort_order, updated_at
FROM agent_object_index
WHERE workspace_id = $1
  AND parent_semantic_key = $2
ORDER BY object_type, sort_order, semantic_key;

-- name: ListAgentObjectsByType :many
SELECT workspace_id, object_type, object_id, semantic_key, display_name, parent_object_type, parent_object_id, parent_semantic_key, status, kind, sort_order, updated_at
FROM agent_object_index
WHERE workspace_id = $1
  AND object_type = $2
ORDER BY sort_order, semantic_key;

-- name: GetCurrentArtifactVersionByShotAndKind :one
SELECT artifact_version.*
FROM artifact_version
JOIN media_node ON media_node.current_version_id = artifact_version.id
JOIN shot ON shot.id = media_node.shot_id
WHERE shot.workspace_id = $1
  AND shot.semantic_key = $2
  AND media_node.artifact_kind = $3
ORDER BY media_node.updated_at DESC
LIMIT 1;

-- name: GetLatestArtifactVersionByShotAndKind :one
SELECT artifact_version.*
FROM artifact_version
JOIN media_node ON media_node.id = artifact_version.node_id
JOIN shot ON shot.id = media_node.shot_id
WHERE shot.workspace_id = $1
  AND shot.semantic_key = $2
  AND media_node.artifact_kind = $3
ORDER BY artifact_version.created_at DESC
LIMIT 1;

-- name: GetArtifactVersionBySemanticKey :one
SELECT *
FROM artifact_version
WHERE workspace_id = $1
  AND semantic_key = $2;

-- name: GetRenderPlanBySemanticKey :one
SELECT *
FROM render_plan
WHERE workspace_id = $1
  AND semantic_key = $2
  AND archived_at IS NULL;

-- name: GetMediaNodeBySemanticKey :one
SELECT *
FROM media_node
WHERE workspace_id = $1
  AND semantic_key = $2;
```

- [ ] **Step 2: Generate sqlc**

Run:

```bash
make sqlc-generate
```

Expected: PASS and generated structs/functions include `AgentObjectIndex`, `GetAgentObjectBySemanticKey`, and artifact selector queries.

- [ ] **Step 3: Build generated code**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: PASS or fail only because service code has not yet been updated for newly generated structs. If it fails, keep the exact compiler errors for the next task.

## Task 3: Semantic Key Generator

**Files:**

- Create: `apps/server/internal/agent/identity/types.go`
- Create: `apps/server/internal/agent/identity/keygen.go`
- Create: `apps/server/internal/agent/identity/keygen_test.go`

- [ ] **Step 1: Write tests**

Create `apps/server/internal/agent/identity/keygen_test.go`:

```go
package identity

import "testing"

func TestSemanticKeyGeneration(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"scene", SceneKey("机场出发大厅"), "scene_ji_chang_chu_fa_da_ting"},
		{"shot", ShotKey(3), "shot_03"},
		{"element", ElementKey("悦行银色行李箱"), "element_yue_xing_yin_se_xing_li_xiang"},
		{"state", ElementStateKey("element_luggage", "银色参考款"), "element_luggage.state_yin_se_can_kao_kuan"},
		{"render plan", RenderPlanKey("shot_03", "preview_image", 2), "shot_03.preview_image.rp2"},
		{"media node", MediaNodeKey("shot_03.preview_image.rp2"), "shot_03.preview_image.rp2.output"},
		{"artifact", ArtifactVersionKey("shot_03.preview_image.rp2.output", 1), "shot_03.preview_image.rp2.output.v1"},
	}
	for _, tt := range cases {
		if tt.got != tt.want {
			t.Fatalf("%s key = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestSlugKeepsKnownASCIIKeysStable(t *testing.T) {
	if got := NormalizeKeyPart("shot_03"); got != "shot_03" {
		t.Fatalf("NormalizeKeyPart = %q", got)
	}
	if got := NormalizeKeyPart("preview image!!"); got != "preview_image" {
		t.Fatalf("NormalizeKeyPart punctuation = %q", got)
	}
}
```

- [ ] **Step 2: Run failing test**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/identity -run TestSemanticKeyGeneration -count=1
```

Expected: FAIL because package does not exist.

- [ ] **Step 3: Add identity types**

Create `apps/server/internal/agent/identity/types.go`:

```go
package identity

import "github.com/jackc/pgx/v5/pgtype"

const (
	ObjectCreativeBrief   = "creative_brief"
	ObjectProjectMemory   = "project_memory"
	ObjectKeyElement      = "key_element"
	ObjectKeyElementState = "key_element_state"
	ObjectScene           = "scene"
	ObjectShot            = "shot"
	ObjectShotDependency  = "shot_dependency"
	ObjectRenderPlan      = "render_plan"
	ObjectMediaNode       = "media_node"
	ObjectGenerationJob   = "generation_job"
	ObjectArtifactVersion = "artifact_version"
	ObjectReviewRecord    = "review_record"
	ObjectArtifactIssue   = "artifact_issue"
	ObjectAgentThread     = "agent_thread"
	ObjectAgentTask       = "agent_task"
)

type ObjectRef struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

type ArtifactSelectorRef struct {
	Scope        ObjectRef `json:"scope"`
	ArtifactKind string    `json:"artifact_kind"`
	Selector     string    `json:"selector"`
	Key          string    `json:"key"`
}

type ResolvedObject struct {
	WorkspaceID       pgtype.UUID
	ObjectType        string
	ObjectID          pgtype.UUID
	SemanticKey       string
	DisplayName       string
	ParentObjectType  string
	ParentObjectID    pgtype.UUID
	ParentSemanticKey string
	Status            string
	Kind              string
	SortOrder         int32
}
```

- [ ] **Step 4: Add key generator**

Create `apps/server/internal/agent/identity/keygen.go`:

```go
package identity

import (
	"fmt"
	"strings"
	"unicode"
)

var pinyinWords = map[rune]string{
	'机': "ji", '场': "chang", '出': "chu", '发': "fa", '大': "da", '厅': "ting",
	'悦': "yue", '行': "xing", '银': "yin", '色': "se", '李': "li", '箱': "xiang",
	'参': "can", '考': "kao", '款': "kuan",
}

func NormalizeKeyPart(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return "unnamed"
	}
	parts := make([]string, 0, len(input))
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			parts = append(parts, current.String())
			current.Reset()
		}
	}
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			current.WriteRune(r)
		case r >= '0' && r <= '9':
			current.WriteRune(r)
		case r == '_':
			flush()
		case unicode.IsSpace(r) || r == '-' || r == '.' || r == '!' || r == '！':
			flush()
		default:
			flush()
			if word, ok := pinyinWords[r]; ok {
				parts = append(parts, word)
			}
		}
	}
	flush()
	if len(parts) == 0 {
		return "unnamed"
	}
	return strings.Join(parts, "_")
}

func SceneKey(title string) string {
	return "scene_" + NormalizeKeyPart(title)
}

func ShotKey(sortOrder int32) string {
	if sortOrder <= 0 {
		sortOrder = 1
	}
	return fmt.Sprintf("shot_%02d", sortOrder)
}

func ElementKey(name string) string {
	return "element_" + NormalizeKeyPart(name)
}

func ElementStateKey(elementKey string, label string) string {
	return strings.TrimSpace(elementKey) + ".state_" + NormalizeKeyPart(label)
}

func RenderPlanKey(scopeKey string, targetPhase string, revision int32) string {
	if revision <= 0 {
		revision = 1
	}
	return fmt.Sprintf("%s.%s.rp%d", strings.TrimSpace(scopeKey), NormalizeKeyPart(targetPhase), revision)
}

func MediaNodeKey(renderPlanKey string) string {
	return strings.TrimSpace(renderPlanKey) + ".output"
}

func GenerationJobKey(renderPlanKey string, attempt int32) string {
	if attempt <= 0 {
		attempt = 1
	}
	return fmt.Sprintf("%s.job%d", strings.TrimSpace(renderPlanKey), attempt)
}

func ArtifactVersionKey(mediaNodeKey string, versionNo int32) string {
	if versionNo <= 0 {
		versionNo = 1
	}
	return fmt.Sprintf("%s.v%d", strings.TrimSpace(mediaNodeKey), versionNo)
}
```

- [ ] **Step 5: Run identity tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/identity -count=1
```

Expected: PASS.

## Task 4: Object Resolver

**Files:**

- Create: `apps/server/internal/agent/identity/resolver.go`
- Create: `apps/server/internal/agent/identity/resolver_test.go`

- [ ] **Step 1: Write resolver tests**

Create `apps/server/internal/agent/identity/resolver_test.go` with tests for:

```go
package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestResolverResolvesObjectBySemanticKey(t *testing.T) {
	workspaceID := uuidWithByte(1)
	store := &fakeStore{object: db.AgentObjectIndex{WorkspaceID: workspaceID, ObjectType: ObjectShot, ObjectID: uuidWithByte(2), SemanticKey: "shot_03", DisplayName: "箱体细节"}}
	resolver := NewResolver(store)
	got, err := resolver.ResolveObject(context.Background(), workspaceID, ObjectRef{Type: ObjectShot, Key: "shot_03"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ObjectType != ObjectShot || got.SemanticKey != "shot_03" || got.DisplayName != "箱体细节" {
		t.Fatalf("resolved = %#v", got)
	}
}

func TestResolverRejectsMissingKey(t *testing.T) {
	workspaceID := uuidWithByte(1)
	resolver := NewResolver(&fakeStore{err: errors.New("no rows")})
	_, err := resolver.ResolveObject(context.Background(), workspaceID, ObjectRef{Type: ObjectShot, Key: ""})
	if !errors.Is(err, ErrInvalidRef) {
		t.Fatalf("err = %v, want ErrInvalidRef", err)
	}
}

func TestResolverResolvesCurrentArtifactByShotAndKind(t *testing.T) {
	workspaceID := uuidWithByte(1)
	version := db.ArtifactVersion{ID: uuidWithByte(9), WorkspaceID: workspaceID, SemanticKey: "shot_03.preview_image.rp1.output.v1", ArtifactKind: "preview_image"}
	resolver := NewResolver(&fakeStore{artifact: version})
	got, err := resolver.ResolveArtifact(context.Background(), workspaceID, ArtifactSelectorRef{
		Scope:        ObjectRef{Type: ObjectShot, Key: "shot_03"},
		ArtifactKind: "preview_image",
		Selector:     "current",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SemanticKey != version.SemanticKey {
		t.Fatalf("artifact = %#v", got)
	}
}
```

Add fake store helpers in the same file:

```go
type fakeStore struct {
	object   db.AgentObjectIndex
	artifact db.ArtifactVersion
	err      error
}

func (f *fakeStore) GetAgentObjectBySemanticKey(context.Context, db.GetAgentObjectBySemanticKeyParams) (db.AgentObjectIndex, error) {
	if f.err != nil {
		return db.AgentObjectIndex{}, f.err
	}
	return f.object, nil
}

func (f *fakeStore) GetCurrentArtifactVersionByShotAndKind(context.Context, db.GetCurrentArtifactVersionByShotAndKindParams) (db.ArtifactVersion, error) {
	if f.err != nil {
		return db.ArtifactVersion{}, f.err
	}
	return f.artifact, nil
}

func (f *fakeStore) GetLatestArtifactVersionByShotAndKind(context.Context, db.GetLatestArtifactVersionByShotAndKindParams) (db.ArtifactVersion, error) {
	if f.err != nil {
		return db.ArtifactVersion{}, f.err
	}
	return f.artifact, nil
}

func (f *fakeStore) GetArtifactVersionBySemanticKey(context.Context, db.GetArtifactVersionBySemanticKeyParams) (db.ArtifactVersion, error) {
	if f.err != nil {
		return db.ArtifactVersion{}, f.err
	}
	return f.artifact, nil
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{15: b}, Valid: true}
}
```

- [ ] **Step 2: Run failing resolver test**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/identity -run TestResolver -count=1
```

Expected: FAIL because resolver is not implemented.

- [ ] **Step 3: Implement resolver**

Create `apps/server/internal/agent/identity/resolver.go`:

```go
package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrInvalidRef = errors.New("invalid semantic ref")
	ErrNotFound   = errors.New("semantic ref not found")
)

type Store interface {
	GetAgentObjectBySemanticKey(ctx context.Context, params db.GetAgentObjectBySemanticKeyParams) (db.AgentObjectIndex, error)
	GetCurrentArtifactVersionByShotAndKind(ctx context.Context, params db.GetCurrentArtifactVersionByShotAndKindParams) (db.ArtifactVersion, error)
	GetLatestArtifactVersionByShotAndKind(ctx context.Context, params db.GetLatestArtifactVersionByShotAndKindParams) (db.ArtifactVersion, error)
	GetArtifactVersionBySemanticKey(ctx context.Context, params db.GetArtifactVersionBySemanticKeyParams) (db.ArtifactVersion, error)
}

type Resolver struct {
	store Store
}

func NewResolver(store Store) *Resolver {
	return &Resolver{store: store}
}

func (r *Resolver) ResolveObject(ctx context.Context, workspaceID pgtype.UUID, ref ObjectRef) (ResolvedObject, error) {
	if r == nil || r.store == nil || !workspaceID.Valid {
		return ResolvedObject{}, ErrInvalidRef
	}
	objectType := strings.TrimSpace(ref.Type)
	key := strings.TrimSpace(ref.Key)
	if objectType == "" || key == "" {
		return ResolvedObject{}, ErrInvalidRef
	}
	row, err := r.store.GetAgentObjectBySemanticKey(ctx, db.GetAgentObjectBySemanticKeyParams{
		WorkspaceID:  workspaceID,
		ObjectType:   objectType,
		SemanticKey:  key,
	})
	if err != nil {
		return ResolvedObject{}, fmt.Errorf("%w: %s %s", ErrNotFound, objectType, key)
	}
	return ResolvedObject{
		WorkspaceID:       row.WorkspaceID,
		ObjectType:        row.ObjectType,
		ObjectID:          row.ObjectID,
		SemanticKey:       row.SemanticKey,
		DisplayName:       row.DisplayName,
		ParentObjectType:  row.ParentObjectType,
		ParentObjectID:    row.ParentObjectID,
		ParentSemanticKey: row.ParentSemanticKey,
		Status:            row.Status,
		Kind:              row.Kind,
		SortOrder:         row.SortOrder,
	}, nil
}

func (r *Resolver) ResolveArtifact(ctx context.Context, workspaceID pgtype.UUID, ref ArtifactSelectorRef) (db.ArtifactVersion, error) {
	if r == nil || r.store == nil || !workspaceID.Valid {
		return db.ArtifactVersion{}, ErrInvalidRef
	}
	if key := strings.TrimSpace(ref.Key); key != "" {
		return r.store.GetArtifactVersionBySemanticKey(ctx, db.GetArtifactVersionBySemanticKeyParams{WorkspaceID: workspaceID, SemanticKey: key})
	}
	if ref.Scope.Type != ObjectShot || strings.TrimSpace(ref.Scope.Key) == "" || strings.TrimSpace(ref.ArtifactKind) == "" {
		return db.ArtifactVersion{}, ErrInvalidRef
	}
	switch strings.TrimSpace(ref.Selector) {
	case "", "current", "winner":
		return r.store.GetCurrentArtifactVersionByShotAndKind(ctx, db.GetCurrentArtifactVersionByShotAndKindParams{
			WorkspaceID:  workspaceID,
			SemanticKey:  strings.TrimSpace(ref.Scope.Key),
			ArtifactKind: strings.TrimSpace(ref.ArtifactKind),
		})
	case "latest":
		return r.store.GetLatestArtifactVersionByShotAndKind(ctx, db.GetLatestArtifactVersionByShotAndKindParams{
			WorkspaceID:  workspaceID,
			SemanticKey:  strings.TrimSpace(ref.Scope.Key),
			ArtifactKind: strings.TrimSpace(ref.ArtifactKind),
		})
	default:
		return db.ArtifactVersion{}, ErrInvalidRef
	}
}
```

- [ ] **Step 4: Run identity package tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/identity -count=1
```

Expected: PASS.

## Task 5: Read Project Context as Actionable Semantic Object Index

**Files:**

- Create: `apps/server/internal/agent/identity/object_index.go`
- Create: `apps/server/internal/agent/identity/object_index_test.go`
- Modify: `apps/server/internal/agent/tools/read_project_context.go`
- Modify: `apps/server/internal/agent/tools/creative_tools_test.go`

- [ ] **Step 0: Refactor read_project_context input contract**

Change the public input schema from UUID-oriented `scope.id` to semantic `scope_ref`:

```go
type ReadProjectContextToolInput struct {
	Brief       string        `json:"brief" jsonschema:"required" jsonschema_description:"本次读取上下文的目的。"`
	ScopeRef    ToolObjectRef `json:"scope_ref" jsonschema_description:"可选读取范围。为空表示整个 workspace；局部读取时填写 read_project_context 返回的 semantic_key，例如 type=shot,key=shot_03。不要填写 UUID。"`
	Include     []string      `json:"include" jsonschema_description:"要返回的对象类型。可选值包括 brief、memory、elements、scenes、shots、dependencies、render_plans、object_index、production_state。"`
	DetailLevel string        `json:"detail_level" jsonschema:"enum=summary,enum=full" jsonschema_description:"summary 返回摘要和可操作索引；full 返回更完整事实。默认 summary。"`
}
```

The tool must still infer the current workspace from runtime context. It must not ask the model for `workspace_id`, `shot_id`, `scene_id`, or other raw UUIDs. If `scope_ref` cannot be resolved, return a natural-language retry message that tells the model to call `read_project_context` without a scope and use a listed `semantic_key`.

- [ ] **Step 1: Add object index renderer test**

Create `apps/server/internal/agent/identity/object_index_test.go`:

```go
package identity

import (
	"strings"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestRenderObjectIndexIncludesActionableKeys(t *testing.T) {
	rows := []db.AgentObjectIndex{
		{ObjectType: ObjectScene, SemanticKey: "scene_airport", DisplayName: "机场出发大厅", Status: "planned", Kind: "scene", SortOrder: 1},
		{ObjectType: ObjectShot, SemanticKey: "shot_01", DisplayName: "产品开场", ParentSemanticKey: "scene_airport", Status: "preview_ready", Kind: "lifestyle", SortOrder: 1},
		{ObjectType: ObjectRenderPlan, SemanticKey: "shot_01.preview_image.rp1", DisplayName: "预览图计划", ParentSemanticKey: "shot_01", Status: "succeeded", Kind: "preview_image", SortOrder: 1},
		{ObjectType: ObjectArtifactVersion, SemanticKey: "shot_01.preview_image.rp1.output.v1", DisplayName: "预览图 v1", ParentSemanticKey: "shot_01.preview_image.rp1.output", Status: "succeeded", Kind: "preview_image", SortOrder: 1},
	}
	text := RenderObjectIndex(rows)
	for _, want := range []string{
		"Scene scene_airport｜机场出发大厅｜planned",
		"Shot shot_01｜产品开场｜preview_ready",
		"RenderPlan shot_01.preview_image.rp1｜preview_image｜succeeded",
		"Artifact shot_01.preview_image.rp1.output.v1｜preview_image｜succeeded",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("index text missing %q:\n%s", want, text)
		}
	}
}
```

- [ ] **Step 2: Implement object index renderer**

Create `apps/server/internal/agent/identity/object_index.go`:

```go
package identity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func RenderObjectIndex(rows []db.AgentObjectIndex) string {
	if len(rows) == 0 {
		return "可操作对象索引：当前没有可操作对象。"
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ObjectType != rows[j].ObjectType {
			return objectRank(rows[i].ObjectType) < objectRank(rows[j].ObjectType)
		}
		if rows[i].SortOrder != rows[j].SortOrder {
			return rows[i].SortOrder < rows[j].SortOrder
		}
		return rows[i].SemanticKey < rows[j].SemanticKey
	})
	lines := []string{"可操作对象索引："}
	for _, row := range rows {
		switch row.ObjectType {
		case ObjectScene:
			lines = append(lines, fmt.Sprintf("- Scene %s｜%s｜%s", row.SemanticKey, row.DisplayName, row.Status))
		case ObjectShot:
			lines = append(lines, fmt.Sprintf("  - Shot %s｜%s｜%s｜parent=%s", row.SemanticKey, row.DisplayName, row.Status, row.ParentSemanticKey))
		case ObjectKeyElement:
			lines = append(lines, fmt.Sprintf("- KeyElement %s｜%s｜%s｜%s", row.SemanticKey, row.DisplayName, row.Kind, row.Status))
		case ObjectKeyElementState:
			lines = append(lines, fmt.Sprintf("  - ElementState %s｜%s｜reference=%s｜parent=%s", row.SemanticKey, row.DisplayName, row.Kind, row.ParentSemanticKey))
		case ObjectRenderPlan:
			lines = append(lines, fmt.Sprintf("    - RenderPlan %s｜%s｜%s｜parent=%s", row.SemanticKey, row.Kind, row.Status, row.ParentSemanticKey))
		case ObjectMediaNode:
			lines = append(lines, fmt.Sprintf("      - MediaNode %s｜%s｜%s｜parent=%s", row.SemanticKey, row.Kind, row.Status, row.ParentSemanticKey))
		case ObjectArtifactVersion:
			lines = append(lines, fmt.Sprintf("        - Artifact %s｜%s｜%s｜parent=%s", row.SemanticKey, row.Kind, row.Status, row.ParentSemanticKey))
		case ObjectReviewRecord:
			lines = append(lines, fmt.Sprintf("      - Review %s｜%s｜%s｜target=%s", row.SemanticKey, row.Kind, row.Status, row.ParentSemanticKey))
		case ObjectArtifactIssue:
			lines = append(lines, fmt.Sprintf("      - Issue %s｜%s｜%s｜target=%s", row.SemanticKey, row.Kind, row.Status, row.ParentSemanticKey))
		}
	}
	return strings.Join(lines, "\n")
}

func objectRank(objectType string) int {
	switch objectType {
	case ObjectCreativeBrief:
		return 0
	case ObjectProjectMemory:
		return 1
	case ObjectKeyElement:
		return 2
	case ObjectKeyElementState:
		return 3
	case ObjectScene:
		return 4
	case ObjectShot:
		return 5
	case ObjectShotDependency:
		return 6
	case ObjectRenderPlan:
		return 7
	case ObjectMediaNode:
		return 8
	case ObjectArtifactVersion:
		return 9
	case ObjectReviewRecord:
		return 10
	case ObjectArtifactIssue:
		return 11
	default:
		return 100
	}
}
```

- [ ] **Step 3: Extend read_project_context store contract**

In `apps/server/internal/agent/creative/state_service.go`, extend the store interface with:

```go
ListAgentObjectsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.AgentObjectIndex, error)
ListAgentObjectsByType(ctx context.Context, params db.ListAgentObjectsByTypeParams) ([]db.AgentObjectIndex, error)
ListAgentObjectsByParentSemanticKey(ctx context.Context, params db.ListAgentObjectsByParentSemanticKeyParams) ([]db.AgentObjectIndex, error)
```

In `apps/server/internal/agent/creative/types.go`, add to `ContextPacket`:

```go
ObjectIndex []db.AgentObjectIndex
```

- [ ] **Step 4: Load object index from service**

In `ReadProjectContext`, after render plans are loaded, add:

```go
packet.ObjectIndex, err = s.store.ListAgentObjectsByWorkspace(ctx, input.WorkspaceID)
if err != nil {
	return ContextPacket{}, err
}
```

- [ ] **Step 5: Return object index in tool result**

In `apps/server/internal/agent/tools/read_project_context.go`, add import:

```go
agentidentity "github.com/sinmaystar/clip-anvil/internal/agent/identity"
```

Append this item before `ProductionState`:

```go
items = append(items, NaturalResultItem{Label: "ObjectIndex", Value: agentidentity.RenderObjectIndex(packet.ObjectIndex)})
```

Remove the old "可执行 ID 摘要" behavior from `productionStateDecisionText`. Production state must use semantic refs such as `shot_03.preview_image.current`, `shot_03.preview_image.rp1.output.v1`, and `shot_03.shot_video.current`. If internal IDs are kept for debugging, they must be labeled `internal_debug_id` and excluded from normal tool instructions.

- [ ] **Step 6: Run tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/identity ./internal/agent/tools ./internal/agent/creative -count=1
```

Expected: PASS after fake stores are updated to implement the new object-index methods.

## Task 6: Semantic Tool Reference Types

**Files:**

- Create: `apps/server/internal/agent/tools/semantic_refs.go`
- Create: `apps/server/internal/agent/tools/semantic_refs_test.go`

- [ ] **Step 1: Add tool ref tests**

Create `apps/server/internal/agent/tools/semantic_refs_test.go`:

```go
package tools

import "testing"

func TestValidateObjectRefRequiresTypeAndKey(t *testing.T) {
	if err := validateObjectRef(ToolObjectRef{Type: "shot", Key: "shot_03"}, "shot_ref"); err != nil {
		t.Fatal(err)
	}
	if err := validateObjectRef(ToolObjectRef{Type: "shot"}, "shot_ref"); err == nil {
		t.Fatal("expected missing key error")
	}
}

func TestValidateArtifactRefAllowsCurrentSelector(t *testing.T) {
	ref := ToolArtifactRef{
		Scope: ToolObjectRef{Type: "shot", Key: "shot_03"},
		ArtifactKind: "preview_image",
		Selector: "current",
	}
	if err := validateArtifactRef(ref, "target_ref"); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Implement shared tool ref structs**

Create `apps/server/internal/agent/tools/semantic_refs.go`:

```go
package tools

import (
	"fmt"
	"strings"
)

type ToolObjectRef struct {
	Type string `json:"type" jsonschema:"required,enum=creative_brief,enum=project_memory,enum=key_element,enum=key_element_state,enum=scene,enum=shot,enum=shot_dependency,enum=render_plan,enum=media_node,enum=artifact_version,enum=review_record,enum=artifact_issue" jsonschema_description:"对象类型。优先使用 semantic key，不要填写 UUID。"`
	Key  string `json:"key" jsonschema:"required" jsonschema_description:"对象稳定语义键，例如 shot_03、scene_airport_departure、element_luggage.state_silver_reference。不要填写 UUID。"`
}

type ToolArtifactRef struct {
	Key          string        `json:"key" jsonschema_description:"artifact_version 的完整语义键，例如 shot_03.preview_image.rp1.output.v1。若填写 key，scope/artifact_kind/selector 可为空。"`
	Scope        ToolObjectRef `json:"scope" jsonschema_description:"按作用域选择 artifact，例如 type=shot,key=shot_03。"`
	ArtifactKind string        `json:"artifact_kind" jsonschema:"enum=reference_image,enum=preview_image,enum=shot_video,enum=final_video" jsonschema_description:"要选择的产物类型。"`
	Selector     string        `json:"selector" jsonschema:"enum=current,enum=latest,enum=winner" jsonschema_description:"选择器。current/winner 表示当前选中版本；latest 表示最新版本。默认 current。"`
}

func validateObjectRef(ref ToolObjectRef, field string) error {
	if strings.TrimSpace(ref.Type) == "" {
		return fmt.Errorf("%s.type 必填", field)
	}
	if strings.TrimSpace(ref.Key) == "" {
		return fmt.Errorf("%s.key 必填，请使用 read_project_context 返回的 semantic_key，不要编造 UUID", field)
	}
	return nil
}

func validateArtifactRef(ref ToolArtifactRef, field string) error {
	if strings.TrimSpace(ref.Key) != "" {
		return nil
	}
	if err := validateObjectRef(ref.Scope, field+".scope"); err != nil {
		return err
	}
	if strings.TrimSpace(ref.ArtifactKind) == "" {
		return fmt.Errorf("%s.artifact_kind 必填", field)
	}
	selector := strings.TrimSpace(ref.Selector)
	if selector != "" && selector != "current" && selector != "latest" && selector != "winner" {
		return fmt.Errorf("%s.selector 只能是 current、latest 或 winner", field)
	}
	return nil
}
```

- [ ] **Step 3: Run tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run TestValidate -count=1
```

Expected: PASS.

## Task 7: Refactor Creative Tools to Write Semantic Keys

**Files:**

- Modify: `apps/server/internal/agent/tools/upsert_key_elements.go`
- Modify: `apps/server/internal/agent/tools/upsert_storyboard.go`
- Modify: `apps/server/internal/agent/creative/types.go`
- Modify: `apps/server/internal/agent/creative/state_service.go`
- Modify: `apps/server/internal/agent/creative/state_service_test.go`

- [ ] **Step 1: Add failing service test**

Add a test to `state_service_test.go` proving semantic keys are written and used:

```go
func TestServiceUpsertStoryboardWritesSemanticKeys(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	workspaceID := uuidWithByte(1)
	store.workspace = db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeAgent}
	_, err := service.UpsertStoryboard(context.Background(), UpsertStoryboardInput{
		WorkspaceID: workspaceID,
		Brief:       "创建机场场景",
		Mode:        "create",
		Scope:       StoryboardScope{Type: "workspace"},
		Scenes: []SceneInput{{
			SemanticKey: "scene_airport_departure",
			Title:       "机场出发大厅",
			SortOrder:   1,
		}},
		Shots: []ShotInput{{
			SemanticKey:    "shot_01",
			SceneSemanticKey: "scene_airport_departure",
			Title:          "产品开场",
			CreativeText:   "行李箱亮相",
			SortOrder:      1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.createdScenes[0].SemanticKey != "scene_airport_departure" {
		t.Fatalf("scene semantic key = %q", store.createdScenes[0].SemanticKey)
	}
	if store.createdShots[0].SemanticKey != "shot_01" {
		t.Fatalf("shot semantic key = %q", store.createdShots[0].SemanticKey)
	}
}
```

- [ ] **Step 2: Update service input types**

In `creative/types.go`, add `SemanticKey` and replace cross-object client-key fields:

```go
type SceneInput struct {
	SemanticKey string
	SortOrder   int32
	Title       string
	Description string
	Location    string
	Mood        string
}

type ShotInput struct {
	SemanticKey      string
	SceneSemanticKey string
	SortOrder        int32
	Title            string
	ShotKind         string
	CreativeText     string
	NarrativePurpose string
	DurationSec      float64
	VisualIntent     string
	ActionText       string
	CameraIntent     string
	Dialogue         string
	Narration        string
	AudioPlan        AudioPlanInput
}
```

Keep `ClientKey` aliases in input only until all tools compile, but service writes `SemanticKey` and `DisplayName`.

- [ ] **Step 3: Update upsert tools**

In `upsert_storyboard.go`, rename tool fields in the public schema:

```go
SemanticKey string `json:"semantic_key" jsonschema:"required" jsonschema_description:"稳定语义键，例如 shot_03 或 scene_airport_departure。不要填写 UUID。创建后不要因为标题或排序变化而修改。"`
```

For relationships, use:

```go
ShotKey    string `json:"shot_key" jsonschema:"required" jsonschema_description:"分镜 semantic_key，例如 shot_03。"`
ElementKey string `json:"element_key" jsonschema:"required" jsonschema_description:"关键元素 semantic_key，例如 element_luggage。"`
StateKey   string `json:"state_key" jsonschema_description:"关键元素状态 semantic_key，例如 element_luggage.state_silver_reference。"`
```

- [ ] **Step 4: Update SQL queries**

Update `creative_state.sql` and `shot.sql` lookups from client-key names to semantic-key names:

```sql
-- name: GetSceneBySemanticKey :one
SELECT *
FROM scene
WHERE workspace_id = $1
  AND semantic_key = $2
  AND archived_at IS NULL;

-- name: GetShotBySemanticKey :one
SELECT *
FROM shot
WHERE workspace_id = $1
  AND semantic_key = $2
  AND archived_at IS NULL;
```

- [ ] **Step 5: Generate and test**

Run:

```bash
make sqlc-generate
gofmt -w apps/server/internal/agent/creative apps/server/internal/agent/tools
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/creative ./internal/agent/tools -count=1
```

Expected: PASS.

## Task 8: Semantic RenderPlan Keys

**Files:**

- Modify: `apps/server/internal/agent/renderplan/service.go`
- Modify: `apps/server/internal/agent/renderplan/service_test.go`
- Modify: `apps/server/internal/agent/tools/upsert_render_plan.go`
- Modify: `apps/server/internal/agent/tools/render_plan_submitter.go`

- [ ] **Step 1: Add render plan key test**

Add:

```go
func TestRenderPlanUsesSemanticScopeKey(t *testing.T) {
	key := semanticRenderPlanKey("shot_03", "preview_image", 2)
	if key != "shot_03.preview_image.rp2" {
		t.Fatalf("key = %q", key)
	}
}
```

- [ ] **Step 2: Pass scope semantic key into renderplan.UpsertInput**

In `renderplan/types.go`, add:

```go
ScopeKey string
```

to `Scope`.

- [ ] **Step 3: Generate semantic render plan key**

Replace:

```go
return fmt.Sprintf("%s:%s:%s:%d", input.Scope.Type, uuidString(input.Scope.ID), input.TargetPhase, revision)
```

with:

```go
return semanticRenderPlanKey(input.Scope.Key, input.TargetPhase, revision)
```

Add:

```go
func semanticRenderPlanKey(scopeKey string, targetPhase string, revision int32) string {
	return identity.RenderPlanKey(scopeKey, targetPhase, revision)
}
```

- [ ] **Step 4: Resolve scope ref in upsert_render_plan**

In `upsert_render_plan.go`, change `RenderPlanScopeInput` to:

```go
type RenderPlanScopeInput struct {
	Type string `json:"type" jsonschema:"enum=key_element_state,enum=shot" jsonschema_description:"RenderPlan 归属类型。"`
	Key  string `json:"key" jsonschema_description:"scope 语义键，例如 shot_03 或 element_luggage.state_silver_reference。优先填写。"`
	ID   string `json:"id" jsonschema_description:"兼容字段。不要主动填写 UUID；只有系统返回过且 key 不可用时才使用。"`
}
```

Use `identity.Resolver.ResolveObject` to map `Type+Key` to `Scope.ID` and `Scope.Key`.

- [ ] **Step 5: Run renderplan and tools tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/renderplan ./internal/agent/tools -count=1
```

Expected: PASS.

## Task 9: Worker Semantic Output Chain

**Files:**

- Modify: `apps/server/internal/agent/worker/types.go`
- Modify: `apps/server/internal/agent/worker/executor.go`
- Modify: `apps/server/internal/agent/worker/executor_test.go`
- Modify: `apps/server/internal/agent/tools/render_plan_submitter.go`
- Modify: `apps/server/sqlc/queries/node.sql`
- Modify: `apps/server/sqlc/queries/production.sql`

- [ ] **Step 1: Add failing Worker test**

Add to `executor_test.go`:

```go
func TestWorkerCreatesSemanticOutputChain(t *testing.T) {
	task := db.AgentTask{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ScopeType: "shot", ScopeID: uuidWithByte(3), RenderPlanID: uuidWithByte(4)}
	input := GenerationInput{
		Mode:          "preview_image",
		ScopeKey:      "shot_03",
		RenderPlanKey: "shot_03.preview_image.rp1",
		ShotClientKey: "shot_03",
		Prompt:        "生成行李箱预览图",
		OutputType:    "image",
		OperationType: "text_to_image",
		MaxAttempts:   1,
	}
	raw, _ := json.Marshal(input)
	task.Input = raw
	store := newFakeWorkerStore()
	executor := NewExecutor(store, fakeRuntime{}, fakeProductionService{})
	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if store.createdNode.SemanticKey != "shot_03.preview_image.rp1.output" {
		t.Fatalf("node semantic key = %q", store.createdNode.SemanticKey)
	}
	if store.createdJob.SemanticKey != "shot_03.preview_image.rp1.job1" {
		t.Fatalf("job semantic key = %q", store.createdJob.SemanticKey)
	}
	if store.createdVersion.SemanticKey != "shot_03.preview_image.rp1.output.v1" {
		t.Fatalf("version semantic key = %q", store.createdVersion.SemanticKey)
	}
}
```

- [ ] **Step 2: Extend Worker input**

In `worker/types.go`, add:

```go
ScopeKey      string `json:"scope_key"`
RenderPlanKey string `json:"render_plan_key"`
ArtifactKind  string `json:"artifact_kind"`
```

- [ ] **Step 3: Pass keys from RenderPlanSubmitter**

In `workerInputForShotRenderPlan`, set:

```go
ScopeKey:      shot.SemanticKey,
RenderPlanKey: plan.SemanticKey,
ArtifactKind:  plan.TargetPhase,
```

For key element state:

```go
ScopeKey:      state.SemanticKey,
RenderPlanKey: plan.SemanticKey,
ArtifactKind:  plan.TargetPhase,
```

- [ ] **Step 4: Update creation query params**

Extend `CreateAgentGenerationNodeParams`, `CreateGenerationJobParams`, and `CreateArtifactVersionParams` through SQL to include:

```sql
semantic_key,
display_name,
artifact_kind,
source_render_plan_id
```

The node insert should write:

```go
SemanticKey:        identity.MediaNodeKey(input.RenderPlanKey),
DisplayName:        nodeTitle(input),
ArtifactKind:       generationSpec(input).ArtifactKind,
SourceRenderPlanID: task.RenderPlanID,
```

The job insert should write:

```go
SemanticKey:        identity.GenerationJobKey(input.RenderPlanKey, int32(attempt)),
DisplayName:        generationSpec(input).ArtifactKind + " generation job",
SourceRenderPlanID: task.RenderPlanID,
```

The artifact version insert should write:

```go
SemanticKey:        identity.ArtifactVersionKey(node.SemanticKey, versionNo),
DisplayName:        node.DisplayName + " v" + strconv.Itoa(int(versionNo)),
ArtifactKind:       generationSpec(input).ArtifactKind,
SourceRenderPlanID: task.RenderPlanID,
```

- [ ] **Step 5: Run Worker tests**

Run:

```bash
make sqlc-generate
gofmt -w apps/server/internal/agent/worker apps/server/internal/agent/tools/render_plan_submitter.go
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/worker ./internal/agent/tools -count=1
```

Expected: PASS.

## Task 10: Semantic Producer/Craftsman/Reviewer Tools

**Files:**

- Modify: `apps/server/internal/agent/tools/dispatch_craftsman.go`
- Modify: `apps/server/internal/agent/tools/decide_render_plan.go`
- Modify: `apps/server/internal/agent/tools/dispatch_reviewer.go`
- Modify: `apps/server/internal/agent/tools/submit_review_result.go`
- Create or replace: `apps/server/internal/agent/tools/select_artifact_version.go`
- Modify: related tests in `apps/server/internal/agent/tools/*_test.go`

- [ ] **Step 1: Refactor dispatch_craftsman schema**

Use:

```go
type DispatchCraftsmanInput struct {
	Brief           string        `json:"brief" jsonschema:"required" jsonschema_description:"本次派发 Craftsman 的业务目的。"`
	ScopeRef        ToolObjectRef `json:"scope_ref" jsonschema:"required" jsonschema_description:"目标对象语义引用。reference_image 使用 key_element_state；preview_image/shot_video 使用 shot。"`
	TargetPhase     string        `json:"target_phase" jsonschema:"required,enum=reference_image,enum=preview_image,enum=shot_video" jsonschema_description:"生成阶段。"`
	ExecutionPolicy string        `json:"execution_policy" jsonschema:"required,enum=execute_immediately,enum=wait_for_producer" jsonschema_description:"execute_immediately 表示 Craftsman 编译后直接提交 Worker；wait_for_producer 表示等待 Producer 决策。"`
	Instructions    string        `json:"instructions" jsonschema_description:"给 Craftsman 的自然语言任务说明。"`
}
```

Resolve `ScopeRef` through `identity.Resolver`.

- [ ] **Step 2: Refactor decide_render_plan schema**

Use:

```go
type RenderPlanDecisionInput struct {
	Brief     string                     `json:"brief" jsonschema:"required"`
	Decisions []RenderPlanDecisionItem   `json:"decisions" jsonschema:"required"`
}

type RenderPlanDecisionItem struct {
	RenderPlanRef ToolObjectRef `json:"render_plan_ref" jsonschema:"required" jsonschema_description:"RenderPlan 语义引用，例如 shot_03.preview_image.rp1。"`
	Decision      string        `json:"decision" jsonschema:"required,enum=accept,enum=reject"`
	NextAction    string        `json:"next_action" jsonschema:"enum=submit_worker,enum=revise,enum=stop"`
	Reason        string        `json:"reason" jsonschema:"required"`
}
```

- [ ] **Step 3: Refactor Reviewer dispatch and submit**

`dispatch_reviewer` target:

```go
type ReviewTargetRef struct {
	RenderPlanRef ToolObjectRef  `json:"render_plan_ref"`
	ArtifactRef   ToolArtifactRef `json:"artifact_ref"`
	ShotRef       ToolObjectRef  `json:"shot_ref"`
}
```

`submit_review_result` must accept the same target ref copied from Reviewer task input, then resolve internally.

- [ ] **Step 4: Add native select_artifact_version**

Create native Eino tool `select_artifact_version` with:

```go
type SelectArtifactVersionInput struct {
	Brief       string          `json:"brief" jsonschema:"required"`
	ArtifactRef ToolArtifactRef `json:"artifact_ref" jsonschema:"required" jsonschema_description:"要选择的 artifact。可用完整 key，或 shot+artifact_kind+selector。"`
	Reason      string          `json:"reason" jsonschema:"required"`
}
```

It resolves artifact -> node -> production `SelectArtifactVersion`.

- [ ] **Step 5: Run tools tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -count=1
```

Expected: PASS and tool schemas no longer require model-authored UUIDs for normal operation.

## Task 11: Agent Prompts and Tool Descriptions

**Files:**

- Modify: `apps/server/internal/agent/producer/system_prompt.go`
- Modify: `apps/server/internal/agent/craftsman/system_prompt.go`
- Modify: `apps/server/internal/agent/reviewer/system_prompt.go`
- Modify: prompt tests.

- [ ] **Step 1: Add prompt regression tests**

Add tests asserting prompts contain:

```text
semantic_key 是 Agent 操作对象的主身份
不要编造 UUID
优先使用 read_project_context 返回的 semantic_key
UUID 是工程内部实现细节
```

and do not contain instructions requiring UUID copying except for internal reviewer task echoes.

Add tests for runtime-triggered model input asserting:

```text
tool results do not contain actionable shot_id/node_id/version_id instructions
system reminders contain semantic refs for pending signals
Producer signal trigger text names render_plan_ref or artifact_ref, not raw UUIDs
```

- [ ] **Step 2: Update Producer prompt**

Add:

```text
## 语义身份规则

你通过 semantic_key 操作对象。UUID 是数据库内部身份，除非工具结果明确要求复用，不要主动生成或猜测 UUID。

read_project_context 会返回可操作对象索引。后续工具调用必须优先使用索引里的 semantic_key，例如 shot_03、scene_airport_departure、element_luggage.state_silver_reference、shot_03.preview_image.rp1。
```

- [ ] **Step 3: Update Craftsman prompt**

Add:

```text
你只能为当前 task 的 scope_ref 创建或修订 RenderPlan。RenderPlan、reference binding、artifact binding 都使用 semantic_key。不要把 UUID 写入 generation_text；如果需要引用素材，使用 read_project_context 中的 artifact semantic_key 或 tool 提供的 scope_ref。
```

- [ ] **Step 4: Update Reviewer prompt**

Add:

```text
评审目标由 semantic target ref 指定。你必须复用任务中给出的 semantic key；不要把 media_node、artifact_version、render_plan 的 UUID 混用。submit_review_result 会由工程侧解析 semantic key。
```

- [ ] **Step 5: Run prompt tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer -run Prompt -count=1
```

Expected: PASS.

## Task 12: Canvas/API Semantic Projection

**Files:**

- Create: `apps/server/internal/api/agent_object_index_handler.go`
- Modify: `apps/server/internal/api/agent_workbench_projection.go`
- Modify: `apps/server/internal/api/agent_canvas_detail.go`
- Modify: `apps/server/cmd/server/main.go`
- Create: `apps/web/src/lib/agentSemanticIdentity.ts`
- Modify: Agent workbench/canvas components.

- [ ] **Step 1: Add backend API test**

Add a test asserting `/api/agent/workspaces/:workspaceID/object-index` returns:

```json
{
  "objects": [
    {
      "object_type": "shot",
      "semantic_key": "shot_03",
      "display_name": "箱体品质细节",
      "status": "preview_ready"
    }
  ]
}
```

- [ ] **Step 2: Add handler**

Create handler that calls `ListAgentObjectsByWorkspace` and returns JSON fields:

```go
type agentObjectIndexItemResponse struct {
	ObjectType        string `json:"object_type"`
	ObjectID          string `json:"object_id"`
	SemanticKey       string `json:"semantic_key"`
	DisplayName       string `json:"display_name"`
	ParentObjectType  string `json:"parent_object_type,omitempty"`
	ParentSemanticKey string `json:"parent_semantic_key,omitempty"`
	Status            string `json:"status,omitempty"`
	Kind              string `json:"kind,omitempty"`
	SortOrder         int32  `json:"sort_order,omitempty"`
}
```

Expose UUIDs as read-only debug fields, not required action parameters.

- [ ] **Step 3: Update workbench projection**

Add `semantic_key` to:

- overview brief
- memory
- key elements
- states
- scenes
- shots
- artifact slots
- render plan summaries
- review summaries
- issue summaries

- [ ] **Step 4: Update frontend types and UI**

Create `apps/web/src/lib/agentSemanticIdentity.ts`:

```ts
export type AgentObjectType =
  | 'creative_brief'
  | 'project_memory'
  | 'key_element'
  | 'key_element_state'
  | 'scene'
  | 'shot'
  | 'render_plan'
  | 'media_node'
  | 'artifact_version'
  | 'review_record'
  | 'artifact_issue';

export interface AgentObjectRef {
  type: AgentObjectType;
  key: string;
}

export function formatObjectRef(ref: AgentObjectRef): string {
  return `${ref.type}:${ref.key}`;
}
```

Display semantic keys in detail panels as compact copyable labels.

- [ ] **Step 5: Run frontend and backend checks**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

## Task 13: End-to-End Semantic Agent Flow

**Files:**

- Create: `scripts/smoke-agent-semantic-identity-e2e.sh`
- Add or modify server E2E tests under `apps/server/internal/agent/producer`.

- [ ] **Step 1: Add E2E smoke script**

Create `scripts/smoke-agent-semantic-identity-e2e.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:5175}"
WORKSPACE_ID="${WORKSPACE_ID:?WORKSPACE_ID is required}"

curl -sS "$BASE_URL/api/agent/workspaces/$WORKSPACE_ID/object-index" | jq -e '.objects | type == "array"'
curl -sS "$BASE_URL/api/agent/workspaces/$WORKSPACE_ID/workbench" | jq -e '.scenes | type == "array"'
```

- [ ] **Step 2: Add DB verification query to script**

Append:

```bash
psql "$DATABASE_URL" -v workspace_id="$WORKSPACE_ID" -c "
SELECT object_type, semantic_key, display_name, status
FROM agent_object_index
WHERE workspace_id = :'workspace_id'
ORDER BY object_type, semantic_key;
"
```

- [ ] **Step 3: Run E2E flow manually**

Use browser or API to create a workspace and ask:

```text
我要做一个悦行行李箱的抖音广告，先生成 3 个分镜预览图。
```

Expected DB facts:

```text
shot semantic keys: shot_01, shot_02, shot_03
render plan keys: shot_01.preview_image.rp1, shot_02.preview_image.rp1, shot_03.preview_image.rp1
media node keys: *.output
artifact version keys: *.output.v1
no model-authored UUID appears in tool arguments except internal tool result echo
```

- [ ] **Step 4: Verify multi-turn semantic edit**

Send:

```text
shot_02 的预览图不要了，重新生成一版，保持行李箱一致。
```

Expected:

```text
new render plan: shot_02.preview_image.rp2
new media node: shot_02.preview_image.rp2.output
new artifact version: shot_02.preview_image.rp2.output.v1
old artifact remains queryable by semantic key
current selector resolves to the new selected/winner artifact after selection
```

- [ ] **Step 5: Run full verification**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
GOCACHE=/private/tmp/clipanvil-go-build make server-lint
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected: all commands PASS.

## Task 14: Documentation and Cleanup

**Files:**

- Modify: `docs/engineering/agent-multiagent-architecture.md`
- Modify: `docs/engineering/database.md`
- Modify: `docs/design/agent-mode.md`

- [ ] **Step 1: Document identity invariant**

Add to `docs/engineering/database.md`:

```md
## Agent Semantic Identity

Agent-operable records have both UUID primary keys and stable semantic keys.
UUIDs are internal database identities. Agent tools and model-authored arguments use semantic keys.

Rules:
- `workspace_id + semantic_key` identifies one active object of the same table/object type.
- Titles and display names may change; semantic keys remain stable.
- `read_project_context` is the authoritative source for semantic keys available to a model turn.
- Tools must resolve semantic refs server-side and reject hallucinated or missing keys with natural-language retry guidance.
```

- [ ] **Step 2: Update MultiAgent architecture doc**

Add:

```md
Producer, Craftsman, and Reviewer communicate through semantic object references. Producer may say `shot_03.preview_image.current`; the resolver maps that selector to the current `artifact_version.id`. Craftsman writes RenderPlan keys such as `shot_03.preview_image.rp2`, and Worker derives output node/version keys from that RenderPlan key.
```

- [ ] **Step 3: Remove prompt/tool UUID-first wording**

Search:

```bash
rg -n "UUID|uuid|id 必填|shot_id|render_plan_id|artifact_version_id" apps/server/internal/agent docs/engineering docs/design
```

For every model-facing description, change the instruction to prefer semantic refs. Internal engineering docs may still mention UUID as implementation detail.

Also search runtime-triggered text paths:

```bash
rg -n "system-reminder|pending signal|Producer signal|shot_id|node_id|version_id|render_plan_id" apps/server/internal/agent apps/server/internal/api
```

Every model-facing reminder or trigger must include semantic refs and must not require the model to copy UUIDs.

- [ ] **Step 4: Run docs check**

Run:

```bash
git diff --check
```

Expected: PASS.

## Completion Criteria

Implementation is complete only when:

- `read_project_context` returns semantic object indexes for creative facts, render plans, generated images/videos, reviews, issues, and pending/running work.
- Producer, Craftsman, and Reviewer tool schemas no longer require model-authored UUIDs for normal user flows.
- Worker-created preview images and shot videos receive deterministic semantic keys at `media_node`, `generation_job`, and `artifact_version`.
- Selector refs such as `shot_03.preview_image.current` resolve server-side to the correct current artifact.
- Existing UUID foreign keys still enforce relational integrity.
- Browser E2E plus DB verification proves a multi-turn regenerate/edit flow without UUID hallucination.

## Execution Notes

- Implement this plan on a clean branch or worktree.
- Because this plan changes DB schema and generated sqlc code, run `make sqlc-generate` immediately after Task 2 and after every query change.
- Commit after each task that passes its verification command.
- Do not expose raw UUID requirements in model-facing tool descriptions unless the field is explicitly marked as an internal/debug fallback.
