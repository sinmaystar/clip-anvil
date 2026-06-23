-- +goose Up
CREATE TABLE agent_thread (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    scope_type TEXT NOT NULL DEFAULT 'workspace',
    scope_id UUID,
    runtime_provider TEXT NOT NULL DEFAULT 'eino',
    runtime_agent_name TEXT NOT NULL DEFAULT '',
    current_checkpoint_key TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    summary TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_thread_role_check CHECK (role IN ('producer', 'craftsman', 'reviewer', 'composer')),
    CONSTRAINT agent_thread_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'final_output')),
    CONSTRAINT agent_thread_status_check CHECK (status IN ('active', 'paused', 'archived', 'failed'))
);

CREATE INDEX idx_agent_thread_workspace ON agent_thread(workspace_id, role, status);
CREATE INDEX idx_agent_thread_scope ON agent_thread(workspace_id, scope_type, scope_id);
CREATE UNIQUE INDEX idx_agent_thread_active_producer
    ON agent_thread(workspace_id)
    WHERE role = 'producer' AND scope_type = 'workspace' AND status = 'active';

CREATE TABLE agent_task (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    role TEXT NOT NULL,
    scope_type TEXT NOT NULL DEFAULT 'workspace',
    scope_id UUID,
    task_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    attempt INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 1,
    input JSONB NOT NULL DEFAULT '{}',
    output JSONB NOT NULL DEFAULT '{}',
    error_code TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    CONSTRAINT agent_task_role_check CHECK (role IN ('producer', 'craftsman', 'reviewer', 'composer', 'worker', 'system')),
    CONSTRAINT agent_task_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'node', 'job', 'final_output')),
    CONSTRAINT agent_task_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled', 'waiting_for_user')),
    CONSTRAINT agent_task_attempt_check CHECK (attempt >= 0 AND max_attempts >= 1),
    CONSTRAINT agent_task_type_check CHECK (task_type IN ('producer_turn', 'tool_call', 'decision_resume'))
);

CREATE INDEX idx_agent_task_workspace_status ON agent_task(workspace_id, status, created_at DESC);
CREATE INDEX idx_agent_task_thread ON agent_task(thread_id, created_at DESC);
CREATE INDEX idx_agent_task_scope ON agent_task(workspace_id, scope_type, scope_id, status);

CREATE TABLE agent_event (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    source_role TEXT NOT NULL DEFAULT 'system',
    target_role TEXT,
    scope JSONB NOT NULL DEFAULT '{}',
    payload JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    handled_at TIMESTAMPTZ,
    CONSTRAINT agent_event_source_role_check CHECK (source_role IN ('user', 'producer', 'craftsman', 'reviewer', 'composer', 'worker', 'system')),
    CONSTRAINT agent_event_status_check CHECK (status IN ('pending', 'handled', 'failed', 'cancelled'))
);

CREATE INDEX idx_agent_event_workspace_status ON agent_event(workspace_id, status, created_at DESC);
CREATE INDEX idx_agent_event_thread ON agent_event(thread_id, created_at DESC);
CREATE INDEX idx_agent_event_task ON agent_event(task_id, created_at DESC);
CREATE INDEX idx_agent_event_type ON agent_event(workspace_id, event_type, created_at DESC);

CREATE TABLE agent_message (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID NOT NULL REFERENCES agent_thread(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    role TEXT NOT NULL,
    message_type TEXT NOT NULL DEFAULT 'text',
    content JSONB NOT NULL DEFAULT '{}',
    raw_message JSONB NOT NULL DEFAULT '{}',
    task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    event_id UUID REFERENCES agent_event(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_message_role_check CHECK (role IN ('user', 'assistant', 'tool', 'system')),
    CONSTRAINT agent_message_type_check CHECK (message_type IN ('text', 'tool_call', 'tool_result', 'ui_card', 'error', 'status'))
);

CREATE UNIQUE INDEX idx_agent_message_thread_seq ON agent_message(thread_id, seq);
CREATE INDEX idx_agent_message_workspace_created ON agent_message(workspace_id, created_at DESC);
CREATE INDEX idx_agent_message_task ON agent_message(task_id) WHERE task_id IS NOT NULL;
CREATE INDEX idx_agent_message_event ON agent_message(event_id) WHERE event_id IS NOT NULL;

CREATE TABLE eino_checkpoint (
    key TEXT PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    value BYTEA NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_eino_checkpoint_workspace ON eino_checkpoint(workspace_id, updated_at DESC);
CREATE INDEX idx_eino_checkpoint_thread ON eino_checkpoint(thread_id, updated_at DESC);
CREATE INDEX idx_eino_checkpoint_task ON eino_checkpoint(task_id, updated_at DESC);

-- +goose Down
DROP TABLE IF EXISTS eino_checkpoint;
DROP TABLE IF EXISTS agent_message;
DROP TABLE IF EXISTS agent_event;
DROP TABLE IF EXISTS agent_task;
DROP TABLE IF EXISTS agent_thread;
