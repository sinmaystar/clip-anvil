-- +goose Up
CREATE TABLE sandbox_job (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    target_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    generation_job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    job_type TEXT NOT NULL,
    operation_type TEXT NOT NULL,
    status job_status NOT NULL DEFAULT 'pending',
    sandbox_id TEXT,
    command TEXT NOT NULL DEFAULT '',
    cwd TEXT NOT NULL DEFAULT '/workspace',
    input JSONB NOT NULL DEFAULT '{}',
    output JSONB NOT NULL DEFAULT '{}',
    exit_code INT,
    stdout TEXT NOT NULL DEFAULT '',
    stderr TEXT NOT NULL DEFAULT '',
    duration_ms INT NOT NULL DEFAULT 0,
    error_code TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sandbox_job_workspace_status ON sandbox_job(workspace_id, status);
CREATE INDEX idx_sandbox_job_target_node ON sandbox_job(target_node_id);
CREATE INDEX idx_sandbox_job_generation_job ON sandbox_job(generation_job_id);

-- +goose Down
DROP INDEX IF EXISTS idx_sandbox_job_generation_job;
DROP INDEX IF EXISTS idx_sandbox_job_target_node;
DROP INDEX IF EXISTS idx_sandbox_job_workspace_status;
DROP TABLE IF EXISTS sandbox_job;
