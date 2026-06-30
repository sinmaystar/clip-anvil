-- +goose Up
CREATE TABLE agent_canvas_layout (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    object_type TEXT NOT NULL,
    object_id UUID NOT NULL,
    canvas_x REAL NOT NULL DEFAULT 0,
    canvas_y REAL NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_canvas_layout_object_type_check CHECK (
        object_type IN ('overview', 'scene', 'shot', 'artifact', 'final_output')
    ),
    CONSTRAINT unique_agent_canvas_layout_object UNIQUE (workspace_id, object_type, object_id)
);

CREATE INDEX idx_agent_canvas_layout_workspace
    ON agent_canvas_layout(workspace_id, object_type);

-- +goose Down
DROP INDEX IF EXISTS idx_agent_canvas_layout_workspace;
DROP TABLE IF EXISTS agent_canvas_layout;
