-- +goose Up
CREATE TYPE edge_type AS ENUM ('dependency', 'reference', 'sequence');
CREATE TYPE transition_type AS ENUM ('cut', 'crossfade', 'dissolve', 'wipe');

CREATE TABLE media_edge (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    from_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    to_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    edge_type edge_type NOT NULL DEFAULT 'dependency',
    transition_type transition_type,
    transition_duration REAL,
    source TEXT NOT NULL DEFAULT 'user',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT no_self_loop CHECK (from_node_id != to_node_id),
    CONSTRAINT unique_edge UNIQUE (from_node_id, to_node_id, edge_type)
);

CREATE INDEX idx_media_edge_workspace ON media_edge(workspace_id);
CREATE INDEX idx_media_edge_from ON media_edge(from_node_id);
CREATE INDEX idx_media_edge_to ON media_edge(to_node_id);

-- +goose Down
DROP INDEX IF EXISTS idx_media_edge_to;
DROP INDEX IF EXISTS idx_media_edge_from;
DROP INDEX IF EXISTS idx_media_edge_workspace;
DROP TABLE IF EXISTS media_edge;
DROP TYPE IF EXISTS transition_type;
DROP TYPE IF EXISTS edge_type;
