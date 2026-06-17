-- +goose Up
CREATE TABLE workspace_sandbox (
    workspace_id UUID PRIMARY KEY REFERENCES workspace(id) ON DELETE CASCADE,
    sandbox_id TEXT,
    volume_name TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('creating', 'running', 'unhealthy', 'terminated')),
    last_health_check_at TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_workspace_sandbox_status ON workspace_sandbox(status);

-- +goose Down
DROP INDEX IF EXISTS idx_workspace_sandbox_status;
DROP TABLE IF EXISTS workspace_sandbox;
