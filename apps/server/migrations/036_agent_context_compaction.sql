-- +goose Up
CREATE TABLE agent_context_compaction (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    role TEXT NOT NULL,
    mode TEXT NOT NULL,
    trigger TEXT NOT NULL,
    semantic_key TEXT NOT NULL,
    source_seq_start BIGINT NOT NULL,
    source_seq_end BIGINT NOT NULL,
    source_message_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_media_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    original_token_estimate BIGINT NOT NULL DEFAULT 0,
    compacted_token_estimate BIGINT NOT NULL DEFAULT 0,
    original_bytes BIGINT NOT NULL DEFAULT 0,
    summary TEXT NOT NULL,
    detail_files JSONB NOT NULL DEFAULT '[]'::jsonb,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_context_compaction_role_not_empty CHECK (btrim(role) <> ''),
    CONSTRAINT agent_context_compaction_mode_check CHECK (mode IN ('micro', 'full')),
    CONSTRAINT agent_context_compaction_trigger_not_empty CHECK (btrim(trigger) <> ''),
    CONSTRAINT agent_context_compaction_semantic_key_not_empty CHECK (btrim(semantic_key) <> ''),
    CONSTRAINT agent_context_compaction_source_seq_check CHECK (source_seq_start <= source_seq_end),
    CONSTRAINT agent_context_compaction_tokens_check CHECK (
        original_token_estimate >= 0
        AND compacted_token_estimate >= 0
        AND original_bytes >= 0
    )
);

CREATE UNIQUE INDEX idx_agent_context_compaction_workspace_semantic
    ON agent_context_compaction(workspace_id, semantic_key);

CREATE INDEX idx_agent_context_compaction_thread_created
    ON agent_context_compaction(thread_id, created_at DESC);

CREATE INDEX idx_agent_context_compaction_workspace_created
    ON agent_context_compaction(workspace_id, created_at DESC);

CREATE TABLE agent_message_compaction (
    message_id UUID PRIMARY KEY REFERENCES agent_message(id) ON DELETE CASCADE,
    compaction_id UUID NOT NULL REFERENCES agent_context_compaction(id) ON DELETE CASCADE,
    compacted_role TEXT NOT NULL,
    compacted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_message_compaction_role_not_empty CHECK (btrim(compacted_role) <> '')
);

CREATE INDEX idx_agent_message_compaction_compaction
    ON agent_message_compaction(compaction_id);

-- +goose Down
DROP TABLE IF EXISTS agent_message_compaction;
DROP TABLE IF EXISTS agent_context_compaction;
