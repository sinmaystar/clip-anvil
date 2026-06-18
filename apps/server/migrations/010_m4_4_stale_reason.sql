-- +goose Up
CREATE TABLE node_stale_reason (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    upstream_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    upstream_version_id UUID REFERENCES artifact_version(id) ON DELETE SET NULL,
    reason_code TEXT NOT NULL,
    reason_message TEXT NOT NULL DEFAULT '',
    details JSONB NOT NULL DEFAULT '{}',
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_node_stale_reason_active_unique
    ON node_stale_reason(node_id, upstream_node_id, reason_code)
    WHERE resolved_at IS NULL;

CREATE INDEX idx_node_stale_reason_node_active
    ON node_stale_reason(node_id, created_at)
    WHERE resolved_at IS NULL;

CREATE INDEX idx_node_stale_reason_workspace
    ON node_stale_reason(workspace_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_node_stale_reason_workspace;
DROP INDEX IF EXISTS idx_node_stale_reason_node_active;
DROP INDEX IF EXISTS idx_node_stale_reason_active_unique;
DROP TABLE IF EXISTS node_stale_reason;
