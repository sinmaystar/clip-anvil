-- +goose Up
ALTER TYPE node_type ADD VALUE IF NOT EXISTS 'reference_pack';

CREATE TABLE reference_pack_item (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    pack_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    member_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    position INT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT reference_pack_item_no_self_member CHECK (pack_node_id <> member_node_id),
    CONSTRAINT unique_reference_pack_member UNIQUE (pack_node_id, member_node_id)
);

CREATE INDEX idx_reference_pack_item_pack
    ON reference_pack_item(pack_node_id, position, created_at);

CREATE INDEX idx_reference_pack_item_member
    ON reference_pack_item(member_node_id);

CREATE INDEX idx_reference_pack_item_workspace
    ON reference_pack_item(workspace_id);

-- +goose Down
DROP INDEX IF EXISTS idx_reference_pack_item_workspace;
DROP INDEX IF EXISTS idx_reference_pack_item_member;
DROP INDEX IF EXISTS idx_reference_pack_item_pack;
DROP TABLE IF EXISTS reference_pack_item;
