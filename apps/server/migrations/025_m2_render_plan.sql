-- +goose Up
CREATE TABLE render_plan (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    scope_type TEXT NOT NULL,
    scope_id UUID NOT NULL,
    target_phase TEXT NOT NULL,
    task_type TEXT NOT NULL DEFAULT 'generate',
    model_prompt_profile TEXT NOT NULL,
    operation TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    revision INT NOT NULL DEFAULT 1,
    forked_from_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    render_plan_key TEXT NOT NULL DEFAULT '',
    reference_bindings JSONB NOT NULL DEFAULT '[]',
    subject_bindings JSONB NOT NULL DEFAULT '[]',
    prompt_parts JSONB NOT NULL DEFAULT '{}',
    params JSONB NOT NULL DEFAULT '{}',
    audit_hints JSONB NOT NULL DEFAULT '{}',
    blocker JSONB NOT NULL DEFAULT '{}',
    compiled_prompt TEXT NOT NULL DEFAULT '',
    compiled_request JSONB NOT NULL DEFAULT '{}',
    prompt_audit JSONB NOT NULL DEFAULT '{}',
    cost_estimate JSONB NOT NULL DEFAULT '{}',
    rationale TEXT NOT NULL DEFAULT '',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    submitted_worker_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    output_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    output_version_id UUID REFERENCES artifact_version(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    compiled_at TIMESTAMPTZ,
    submitted_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    CONSTRAINT render_plan_scope_type_check CHECK (scope_type IN ('key_element_state', 'shot')),
    CONSTRAINT render_plan_target_phase_check CHECK (target_phase IN ('reference_image', 'preview_image', 'shot_video')),
    CONSTRAINT render_plan_task_type_check CHECK (task_type IN ('generate', 'edit', 'extend', 'bridge')),
    CONSTRAINT render_plan_profile_check CHECK (model_prompt_profile IN ('seedream_5_image', 'seedance_2_video')),
    CONSTRAINT render_plan_status_check CHECK (status IN ('draft', 'blocked', 'compiled', 'waiting_for_approval', 'submitted', 'running', 'succeeded', 'failed', 'archived')),
    CONSTRAINT render_plan_revision_positive CHECK (revision > 0)
);

CREATE INDEX idx_render_plan_workspace_scope
    ON render_plan(workspace_id, scope_type, scope_id, archived_at, updated_at DESC);

CREATE INDEX idx_render_plan_workspace_status
    ON render_plan(workspace_id, status, updated_at DESC);

CREATE UNIQUE INDEX idx_render_plan_active_key
    ON render_plan(workspace_id, render_plan_key)
    WHERE archived_at IS NULL AND render_plan_key <> '';

CREATE UNIQUE INDEX idx_render_plan_scope_phase_revision
    ON render_plan(workspace_id, scope_type, scope_id, target_phase, revision)
    WHERE archived_at IS NULL;

ALTER TABLE agent_task
    ADD COLUMN IF NOT EXISTS render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_agent_task_render_plan
    ON agent_task(render_plan_id)
    WHERE render_plan_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_agent_task_render_plan;
ALTER TABLE agent_task DROP COLUMN IF EXISTS render_plan_id;
DROP INDEX IF EXISTS idx_render_plan_scope_phase_revision;
DROP INDEX IF EXISTS idx_render_plan_active_key;
DROP INDEX IF EXISTS idx_render_plan_workspace_status;
DROP INDEX IF EXISTS idx_render_plan_workspace_scope;
DROP TABLE IF EXISTS render_plan;
