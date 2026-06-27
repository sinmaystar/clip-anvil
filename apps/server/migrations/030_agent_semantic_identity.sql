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
    display_name = COALESCE(NULLIF(name, ''), NULLIF(client_key, ''), 'Key Element')
WHERE semantic_key = '';

UPDATE key_element_state state
SET semantic_key = element.semantic_key || '.state_' || COALESCE(NULLIF(state.client_key, ''), left(state.id::text, 8)),
    display_name = COALESCE(NULLIF(state.label, ''), NULLIF(state.client_key, ''), 'Element State')
FROM key_element element
WHERE state.key_element_id = element.id
  AND state.semantic_key = '';

UPDATE scene
SET semantic_key = COALESCE(NULLIF(client_key, ''), 'scene_' || left(id::text, 8)),
    display_name = COALESCE(NULLIF(title, ''), NULLIF(client_key, ''), 'Scene')
WHERE semantic_key = '';

UPDATE shot
SET semantic_key = COALESCE(NULLIF(client_key, ''), 'shot_' || lpad(sort_order::text, 2, '0')),
    display_name = COALESCE(NULLIF(title, ''), NULLIF(client_key, ''), 'Shot')
WHERE semantic_key = '';

UPDATE shot_dependency dep
SET semantic_key = 'dep.' || from_shot.semantic_key || '.to.' || to_shot.semantic_key || '.' || dep.dependency_type || '.' || COALESCE(NULLIF(dep.blocking_phase, ''), 'any'),
    display_name = from_shot.semantic_key || ' -> ' || to_shot.semantic_key || ' ' || dep.dependency_type
FROM shot from_shot, shot to_shot
WHERE dep.from_shot_id = from_shot.id
  AND dep.to_shot_id = to_shot.id
  AND dep.semantic_key = '';

UPDATE render_plan plan
SET semantic_key = shot.semantic_key || '.' || plan.target_phase || '.rp' || plan.revision::text || '.' || left(plan.id::text, 8),
    display_name = plan.target_phase || ' RenderPlan r' || plan.revision::text
FROM shot
WHERE plan.scope_type = 'shot'
  AND plan.scope_id = shot.id
  AND plan.semantic_key = '';

UPDATE render_plan plan
SET semantic_key = state.semantic_key || '.' || plan.target_phase || '.rp' || plan.revision::text || '.' || left(plan.id::text, 8),
    display_name = plan.target_phase || ' RenderPlan r' || plan.revision::text
FROM key_element_state state
WHERE plan.scope_type = 'key_element_state'
  AND plan.scope_id = state.id
  AND plan.semantic_key = '';

UPDATE render_plan
SET semantic_key = COALESCE(NULLIF(render_plan_key, '') || '.' || left(id::text, 8), 'render_plan_' || left(id::text, 8)),
    display_name = COALESCE(NULLIF(display_name, ''), target_phase || ' RenderPlan r' || revision::text)
WHERE semantic_key = '';

UPDATE media_node node
SET source_render_plan_id = plan.id,
    semantic_key = plan.semantic_key || '.output.' || left(node.id::text, 8),
    display_name = COALESCE(NULLIF(node.title, ''), plan.display_name || ' Output'),
    artifact_kind = plan.target_phase
FROM render_plan plan
WHERE node.id = plan.output_node_id
  AND plan.output_node_id IS NOT NULL;

UPDATE media_node
SET semantic_key = COALESCE(NULLIF(semantic_key, ''), NULLIF(title, '') || '.' || left(id::text, 8), 'media_node_' || left(id::text, 8)),
    display_name = COALESCE(NULLIF(display_name, ''), NULLIF(title, ''), 'Media Node'),
    artifact_kind = COALESCE(NULLIF(artifact_kind, ''), NULLIF(metadata->>'agent_artifact_kind', ''), NULLIF(metadata->>'artifact_kind', ''), '')
WHERE semantic_key = ''
   OR display_name = ''
   OR artifact_kind = '';

WITH generation_job_sources AS (
    SELECT job.id,
           task.render_plan_id,
           COALESCE(plan.semantic_key, node.semantic_key, 'job_' || left(job.id::text, 8)) AS source_semantic_key
    FROM generation_job job
    JOIN media_node node ON job.target_node_id = node.id
    LEFT JOIN agent_task task ON job.requested_by_type = 'agent_worker' AND job.requested_by_id = task.id::text
    LEFT JOIN render_plan plan ON plan.id = task.render_plan_id
)
UPDATE generation_job job
SET source_render_plan_id = source.render_plan_id,
    semantic_key = source.source_semantic_key || '.job' || job.attempt::text || '.' || left(job.id::text, 8),
    display_name = 'Generation Job ' || job.attempt::text
FROM generation_job_sources source
WHERE job.id = source.id
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
SET semantic_key = version.semantic_key || '.review.r' || review.attempt_no::text,
    display_name = review.review_task || ' r' || review.attempt_no::text
FROM artifact_version version
WHERE review.target_object_type = 'artifact_version'
  AND review.target_object_id = version.id
  AND review.semantic_key = '';

UPDATE review_record review
SET semantic_key = plan.semantic_key || '.review.r' || review.attempt_no::text,
    display_name = review.review_task || ' r' || review.attempt_no::text
FROM render_plan plan
WHERE review.target_object_type = 'render_plan'
  AND review.target_object_id = plan.id
  AND review.semantic_key = '';

UPDATE review_record review
SET semantic_key = shot.semantic_key || '.' || review.target_phase || '.review.r' || review.attempt_no::text,
    display_name = review.review_task || ' r' || review.attempt_no::text
FROM shot
WHERE review.target_object_type = 'shot'
  AND review.target_object_id = shot.id
  AND review.semantic_key = '';

UPDATE review_record
SET semantic_key = COALESCE(NULLIF(semantic_key, ''), 'review_' || left(id::text, 8)),
    display_name = COALESCE(NULLIF(display_name, ''), review_task || ' r' || attempt_no::text)
WHERE semantic_key = ''
   OR display_name = '';

UPDATE artifact_issue issue
SET semantic_key = COALESCE(review.semantic_key, 'target_' || left(issue.target_object_id::text, 8)) || '.issue.' || issue.dimension || '.' || left(issue.id::text, 8),
    display_name = COALESCE(NULLIF(issue.title, ''), issue.dimension || ' issue')
FROM review_record review
WHERE issue.review_record_id = review.id
  AND issue.semantic_key = '';

UPDATE agent_thread
SET semantic_key = role || '.workspace',
    display_name = role || ' workspace'
WHERE scope_type = 'workspace'
  AND semantic_key = '';

UPDATE agent_thread thread
SET semantic_key = thread.role || '.' || shot.semantic_key,
    display_name = thread.role || ' ' || thread.scope_type
FROM shot
WHERE thread.scope_type = 'shot'
  AND thread.scope_id = shot.id
  AND thread.semantic_key = '';

UPDATE agent_thread thread
SET semantic_key = thread.role || '.' || plan.semantic_key,
    display_name = thread.role || ' ' || thread.scope_type
FROM render_plan plan
WHERE thread.scope_type = 'render_plan'
  AND thread.scope_id = plan.id
  AND thread.semantic_key = '';

UPDATE agent_thread
SET semantic_key = role || '.' || scope_type || '.' || left(COALESCE(scope_id::text, workspace_id::text), 8),
    display_name = role || ' ' || scope_type
WHERE semantic_key = '';

UPDATE agent_task task
SET semantic_key = COALESCE(thread.semantic_key, task.role || '.' || task.scope_type) || '.' || task.task_type || '.' || left(task.id::text, 8),
    display_name = task.role || ' ' || task.task_type
FROM agent_thread thread
WHERE task.thread_id = thread.id
  AND task.semantic_key = '';

UPDATE agent_task
SET semantic_key = role || '.' || task_type || '.' || left(id::text, 8),
    display_name = role || ' ' || task_type
WHERE semantic_key = '';

UPDATE producer_pending_signal signal
SET semantic_key = 'signal.' || plan.semantic_key || '.' || signal.signal_type || '.' || left(signal.id::text, 8),
    display_name = signal.signal_type
FROM render_plan plan
WHERE signal.render_plan_id = plan.id
  AND signal.semantic_key = '';

UPDATE producer_pending_signal signal
SET semantic_key = 'signal.' || shot.semantic_key || '.' || signal.signal_type || '.' || left(signal.id::text, 8),
    display_name = signal.signal_type
FROM shot
WHERE signal.scope_type = 'shot'
  AND signal.scope_id = shot.id
  AND signal.semantic_key = '';

UPDATE producer_pending_signal signal
SET semantic_key = 'signal.' || state.semantic_key || '.' || signal.signal_type || '.' || left(signal.id::text, 8),
    display_name = signal.signal_type
FROM key_element_state state
WHERE signal.scope_type = 'key_element_state'
  AND signal.scope_id = state.id
  AND signal.semantic_key = '';

UPDATE producer_pending_signal
SET semantic_key = 'signal.' || scope_type || '.' || left(COALESCE(scope_id::text, id::text), 8) || '.' || signal_type || '.' || left(id::text, 8),
    display_name = signal_type
WHERE semantic_key = '';

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
SELECT workspace_id, 'creative_brief'::text AS object_type, id AS object_id, semantic_key, display_name, ''::text AS parent_object_type, NULL::uuid AS parent_object_id, ''::text AS parent_semantic_key, status::text AS status, ''::text AS kind, 0::int AS sort_order, updated_at
FROM creative_brief
WHERE archived_at IS NULL
UNION ALL
SELECT workspace_id, 'project_memory'::text, id, semantic_key, display_name, ''::text, NULL::uuid, ''::text, status::text, ''::text, version::int, created_at
FROM project_memory
UNION ALL
SELECT workspace_id, 'key_element'::text, id, semantic_key, display_name, ''::text, NULL::uuid, ''::text, status::text, element_type::text, 0::int, updated_at
FROM key_element
WHERE archived_at IS NULL
UNION ALL
SELECT state.workspace_id, 'key_element_state'::text, state.id, state.semantic_key, state.display_name, 'key_element'::text, state.key_element_id, element.semantic_key, state.status::text, state.reference_status::text, 0::int, state.updated_at
FROM key_element_state state
JOIN key_element element ON element.id = state.key_element_id
WHERE state.archived_at IS NULL
UNION ALL
SELECT workspace_id, 'scene'::text, id, semantic_key, display_name, ''::text, NULL::uuid, ''::text, status::text, 'scene'::text, sort_order::int, updated_at
FROM scene
WHERE archived_at IS NULL
UNION ALL
SELECT shot.workspace_id, 'shot'::text, shot.id, shot.semantic_key, shot.display_name, 'scene'::text, shot.scene_id, COALESCE(scene.semantic_key, ''), shot.status::text, shot.shot_kind::text, shot.sort_order::int, shot.updated_at
FROM shot
LEFT JOIN scene ON scene.id = shot.scene_id
WHERE shot.archived_at IS NULL
UNION ALL
SELECT dep.workspace_id, 'shot_dependency'::text, dep.id, dep.semantic_key, dep.display_name, 'shot'::text, dep.to_shot_id, to_shot.semantic_key, ''::text, dep.dependency_type::text, 0::int, dep.created_at
FROM shot_dependency dep
JOIN shot to_shot ON to_shot.id = dep.to_shot_id
UNION ALL
SELECT plan.workspace_id, 'render_plan'::text, plan.id, plan.semantic_key, plan.display_name, plan.scope_type::text, plan.scope_id, COALESCE(shot.semantic_key, state.semantic_key, ''), plan.status::text, plan.target_phase::text, plan.revision::int, plan.updated_at
FROM render_plan plan
LEFT JOIN shot ON plan.scope_type = 'shot' AND plan.scope_id = shot.id
LEFT JOIN key_element_state state ON plan.scope_type = 'key_element_state' AND plan.scope_id = state.id
WHERE plan.archived_at IS NULL
UNION ALL
SELECT node.workspace_id, 'media_node'::text, node.id, node.semantic_key, node.display_name, 'render_plan'::text, node.source_render_plan_id, COALESCE(plan.semantic_key, ''), node.status::text, node.artifact_kind::text, 0::int, node.updated_at
FROM media_node node
LEFT JOIN render_plan plan ON plan.id = node.source_render_plan_id
UNION ALL
SELECT job.workspace_id, 'generation_job'::text, job.id, job.semantic_key, job.display_name, 'media_node'::text, job.target_node_id, node.semantic_key, job.status::text, job.operation_type::text, job.attempt::int, job.created_at
FROM generation_job job
JOIN media_node node ON node.id = job.target_node_id
UNION ALL
SELECT version.workspace_id, 'artifact_version'::text, version.id, version.semantic_key, version.display_name, 'media_node'::text, version.node_id, node.semantic_key, version.status::text, version.artifact_kind::text, version.version_no::int, version.created_at
FROM artifact_version version
JOIN media_node node ON node.id = version.node_id
UNION ALL
SELECT review.workspace_id, 'review_record'::text, review.id, review.semantic_key, review.display_name, review.target_object_type::text, review.target_object_id, COALESCE(version.semantic_key, plan.semantic_key, shot.semantic_key, ''), review.status::text, review.review_task::text, review.attempt_no::int, review.created_at
FROM review_record review
LEFT JOIN artifact_version version ON review.target_object_type = 'artifact_version' AND review.target_object_id = version.id
LEFT JOIN render_plan plan ON review.target_object_type = 'render_plan' AND review.target_object_id = plan.id
LEFT JOIN shot ON review.target_object_type = 'shot' AND review.target_object_id = shot.id
UNION ALL
SELECT issue.workspace_id, 'artifact_issue'::text, issue.id, issue.semantic_key, issue.display_name, issue.target_object_type::text, issue.target_object_id, COALESCE(version.semantic_key, plan.semantic_key, shot.semantic_key, memory.semantic_key, ''), issue.status::text, issue.dimension::text, 0::int, issue.updated_at
FROM artifact_issue issue
LEFT JOIN artifact_version version ON issue.target_object_type = 'artifact_version' AND issue.target_object_id = version.id
LEFT JOIN render_plan plan ON issue.target_object_type = 'render_plan' AND issue.target_object_id = plan.id
LEFT JOIN shot ON issue.target_object_type = 'shot' AND issue.target_object_id = shot.id
LEFT JOIN project_memory memory ON issue.target_object_type = 'project_memory' AND issue.target_object_id = memory.id
UNION ALL
SELECT workspace_id, 'agent_thread'::text, id, semantic_key, display_name, scope_type::text, scope_id, ''::text, status::text, role::text, 0::int, updated_at
FROM agent_thread
UNION ALL
SELECT workspace_id, 'agent_task'::text, id, semantic_key, display_name, 'agent_thread'::text, thread_id, ''::text, status::text, task_type::text, 0::int, created_at
FROM agent_task
UNION ALL
SELECT workspace_id, 'producer_pending_signal'::text, id, semantic_key, display_name, scope_type::text, scope_id, ''::text, status::text, signal_type::text, priority::int, updated_at
FROM producer_pending_signal;

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
