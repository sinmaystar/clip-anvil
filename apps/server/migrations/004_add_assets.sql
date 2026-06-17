-- +goose Up
CREATE TABLE media_asset (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    type media_type NOT NULL,
    mime TEXT NOT NULL,
    storage_url TEXT NOT NULL,
    thumbnail_url TEXT,
    duration_ms INT,
    size_bytes BIGINT,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_media_asset_workspace ON media_asset(workspace_id);

ALTER TABLE media_node
    ADD COLUMN asset_id UUID REFERENCES media_asset(id) ON DELETE SET NULL;

CREATE INDEX idx_media_node_asset ON media_node(asset_id);

-- +goose Down
DROP INDEX IF EXISTS idx_media_node_asset;
ALTER TABLE media_node DROP COLUMN IF EXISTS asset_id;
DROP INDEX IF EXISTS idx_media_asset_workspace;
DROP TABLE IF EXISTS media_asset;
