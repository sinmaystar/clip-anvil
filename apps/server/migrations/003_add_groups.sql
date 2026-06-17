-- +goose Up
CREATE TABLE media_group (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_media_group_workspace ON media_group(workspace_id);

ALTER TABLE media_node
    ADD COLUMN group_id UUID REFERENCES media_group(id) ON DELETE SET NULL;

CREATE INDEX idx_media_node_group ON media_node(group_id);

-- +goose Down
DROP INDEX IF EXISTS idx_media_node_group;
ALTER TABLE media_node DROP COLUMN IF EXISTS group_id;
DROP INDEX IF EXISTS idx_media_group_workspace;
DROP TABLE IF EXISTS media_group;
