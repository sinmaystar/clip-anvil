-- +goose Up

CREATE TABLE producer_pending_signal (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    producer_thread_id uuid NOT NULL REFERENCES agent_thread(id) ON DELETE CASCADE,
    source_role text NOT NULL,
    source_task_id uuid REFERENCES agent_task(id) ON DELETE SET NULL,
    source_thread_id uuid REFERENCES agent_thread(id) ON DELETE SET NULL,
    signal_type text NOT NULL,
    scope_type text NOT NULL,
    scope_id uuid,
    render_plan_id uuid REFERENCES render_plan(id) ON DELETE SET NULL,
    message_id uuid REFERENCES agent_message(id) ON DELETE SET NULL,
    status text NOT NULL DEFAULT 'pending',
    priority integer NOT NULL DEFAULT 100,
    dedupe_key text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    claimed_by_task_id uuid REFERENCES agent_task(id) ON DELETE SET NULL,
    claimed_at timestamptz,
    processed_by_task_id uuid REFERENCES agent_task(id) ON DELETE SET NULL,
    processed_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT producer_pending_signal_source_role_check CHECK (source_role IN ('craftsman', 'worker', 'reviewer', 'composer', 'system')),
    CONSTRAINT producer_pending_signal_status_check CHECK (status IN ('pending', 'claimed', 'processed', 'ignored', 'failed')),
    CONSTRAINT producer_pending_signal_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'render_plan', 'final_output', 'key_element_state', 'node', 'job')),
    CONSTRAINT producer_pending_signal_craftsman_ready_render_plan_check CHECK (signal_type <> 'craftsman_render_plan_ready' OR render_plan_id IS NOT NULL)
);

CREATE UNIQUE INDEX producer_pending_signal_workspace_dedupe_idx
    ON producer_pending_signal(workspace_id, dedupe_key);

CREATE INDEX producer_pending_signal_workspace_status_idx
    ON producer_pending_signal(workspace_id, status, priority, created_at);

CREATE INDEX producer_pending_signal_thread_status_idx
    ON producer_pending_signal(producer_thread_id, status, priority, created_at);

CREATE INDEX producer_pending_signal_render_plan_idx
    ON producer_pending_signal(render_plan_id);

CREATE INDEX producer_pending_signal_source_task_idx
    ON producer_pending_signal(source_task_id);

-- +goose Down

DROP TABLE IF EXISTS producer_pending_signal;
