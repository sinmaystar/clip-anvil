-- +goose Up
CREATE TABLE timeline_plan (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    source_storyboard_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    output_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
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
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT timeline_plan_created_by_role_check CHECK (created_by_role IN ('composer', 'producer', 'system'))
);

CREATE INDEX idx_timeline_plan_workspace_created
    ON timeline_plan(workspace_id, created_at DESC);

CREATE INDEX idx_timeline_plan_workspace_status
    ON timeline_plan(workspace_id, status, updated_at DESC);

CREATE INDEX idx_timeline_plan_production_job
    ON timeline_plan(production_job_id)
    WHERE production_job_id IS NOT NULL;

CREATE INDEX idx_timeline_plan_artifact_version
    ON timeline_plan(artifact_version_id)
    WHERE artifact_version_id IS NOT NULL;

CREATE INDEX idx_timeline_plan_sandbox_job
    ON timeline_plan(sandbox_job_id)
    WHERE sandbox_job_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS timeline_plan;
