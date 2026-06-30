-- +goose Up
CREATE TABLE reference_video_analysis (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    source_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    brief TEXT NOT NULL DEFAULT '',
    focus JSONB NOT NULL DEFAULT '[]',
    model_provider TEXT NOT NULL DEFAULT '',
    model_id TEXT NOT NULL DEFAULT '',
    request_summary JSONB NOT NULL DEFAULT '{}',
    result JSONB NOT NULL DEFAULT '{}',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT reference_video_analysis_status_check CHECK (
        status IN ('pending', 'running', 'succeeded', 'failed')
    )
);

CREATE INDEX idx_reference_video_analysis_workspace
    ON reference_video_analysis(workspace_id, created_at DESC);

CREATE INDEX idx_reference_video_analysis_source_node
    ON reference_video_analysis(source_node_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_reference_video_analysis_source_node;
DROP INDEX IF EXISTS idx_reference_video_analysis_workspace;
DROP TABLE IF EXISTS reference_video_analysis;
