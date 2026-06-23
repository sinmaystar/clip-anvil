-- +goose Up
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

ALTER TABLE media_node
    ADD COLUMN shot_id UUID REFERENCES shot(id) ON DELETE SET NULL;

CREATE INDEX idx_media_node_shot ON media_node(shot_id);

-- +goose Down
DROP INDEX IF EXISTS idx_media_node_shot;
ALTER TABLE media_node DROP COLUMN IF EXISTS shot_id;

DROP INDEX IF EXISTS idx_shot_dependency_unique_active;
DROP INDEX IF EXISTS idx_shot_dependency_to;
DROP INDEX IF EXISTS idx_shot_dependency_from;
DROP INDEX IF EXISTS idx_shot_dependency_workspace;
DROP TABLE IF EXISTS shot_dependency;

DROP INDEX IF EXISTS idx_shot_workspace_client_key_active;
DROP INDEX IF EXISTS idx_shot_workspace_order;
DROP TABLE IF EXISTS shot;
