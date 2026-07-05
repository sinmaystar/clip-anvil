-- +goose Up
CREATE TABLE remotion_renderer_artifact (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    timeline_plan_id UUID NOT NULL REFERENCES timeline_plan(id) ON DELETE CASCADE,
    current_attempt_id UUID,
    status TEXT NOT NULL DEFAULT 'draft',
    route_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    summary TEXT NOT NULL DEFAULT '',
    created_by_role TEXT NOT NULL DEFAULT 'composer',
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT remotion_renderer_artifact_status_check CHECK (
        status IN ('draft', 'validating', 'validated', 'rendering', 'rendered', 'failed', 'blocked', 'fallback')
    ),
    CONSTRAINT remotion_renderer_artifact_created_by_role_check CHECK (
        created_by_role IN ('composer', 'producer', 'system')
    ),
    CONSTRAINT remotion_renderer_artifact_timeline_unique UNIQUE (timeline_plan_id)
);

CREATE TABLE remotion_renderer_attempt (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    timeline_plan_id UUID NOT NULL REFERENCES timeline_plan(id) ON DELETE CASCADE,
    renderer_artifact_id UUID NOT NULL REFERENCES remotion_renderer_artifact(id) ON DELETE CASCADE,
    attempt_no INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    source_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    props_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_hash TEXT NOT NULL DEFAULT '',
    props_hash TEXT NOT NULL DEFAULT '',
    workspace_dir TEXT NOT NULL DEFAULT '',
    validation_result JSONB NOT NULL DEFAULT '{}'::jsonb,
    compile_result JSONB NOT NULL DEFAULT '{}'::jsonb,
    render_result JSONB NOT NULL DEFAULT '{}'::jsonb,
    qa_result JSONB NOT NULL DEFAULT '{}'::jsonb,
    sandbox_job_id UUID REFERENCES sandbox_job(id) ON DELETE SET NULL,
    repair_from_attempt_id UUID REFERENCES remotion_renderer_attempt(id) ON DELETE SET NULL,
    repair_notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT remotion_renderer_attempt_no_check CHECK (attempt_no > 0),
    CONSTRAINT remotion_renderer_attempt_status_check CHECK (
        status IN ('draft', 'validation_failed', 'validated', 'compile_failed', 'render_failed', 'rendered', 'qa_failed', 'accepted', 'discarded')
    ),
    CONSTRAINT remotion_renderer_attempt_workspace_dir_check CHECK (
        workspace_dir = '' OR workspace_dir LIKE '/workspace/agent-remotion/%'
    ),
    CONSTRAINT remotion_renderer_attempt_artifact_no_unique UNIQUE (renderer_artifact_id, attempt_no),
    CONSTRAINT remotion_renderer_attempt_artifact_id_pair UNIQUE (renderer_artifact_id, id)
);

ALTER TABLE remotion_renderer_artifact
    ADD CONSTRAINT remotion_renderer_artifact_current_attempt_fk
    FOREIGN KEY (current_attempt_id) REFERENCES remotion_renderer_attempt(id) ON DELETE SET NULL;

CREATE INDEX idx_remotion_renderer_artifact_workspace_created
    ON remotion_renderer_artifact(workspace_id, created_at DESC);

CREATE INDEX idx_remotion_renderer_artifact_workspace_status
    ON remotion_renderer_artifact(workspace_id, status, updated_at DESC);

CREATE INDEX idx_remotion_renderer_artifact_current_attempt
    ON remotion_renderer_artifact(current_attempt_id)
    WHERE current_attempt_id IS NOT NULL;

CREATE INDEX idx_remotion_renderer_attempt_workspace_created
    ON remotion_renderer_attempt(workspace_id, created_at DESC);

CREATE INDEX idx_remotion_renderer_attempt_timeline
    ON remotion_renderer_attempt(timeline_plan_id, attempt_no);

CREATE INDEX idx_remotion_renderer_attempt_artifact
    ON remotion_renderer_attempt(renderer_artifact_id, attempt_no);

CREATE INDEX idx_remotion_renderer_attempt_sandbox_job
    ON remotion_renderer_attempt(sandbox_job_id)
    WHERE sandbox_job_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_remotion_renderer_attempt_sandbox_job;
DROP INDEX IF EXISTS idx_remotion_renderer_attempt_artifact;
DROP INDEX IF EXISTS idx_remotion_renderer_attempt_timeline;
DROP INDEX IF EXISTS idx_remotion_renderer_attempt_workspace_created;
DROP INDEX IF EXISTS idx_remotion_renderer_artifact_current_attempt;
DROP INDEX IF EXISTS idx_remotion_renderer_artifact_workspace_status;
DROP INDEX IF EXISTS idx_remotion_renderer_artifact_workspace_created;

ALTER TABLE remotion_renderer_artifact
    DROP CONSTRAINT IF EXISTS remotion_renderer_artifact_current_attempt_fk;

DROP TABLE IF EXISTS remotion_renderer_attempt;
DROP TABLE IF EXISTS remotion_renderer_artifact;
