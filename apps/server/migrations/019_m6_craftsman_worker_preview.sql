-- +goose Up
ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume',
        'craftsman_turn',
        'worker_generation'
    ));

CREATE UNIQUE INDEX idx_agent_thread_active_scope_unique
    ON agent_thread(workspace_id, role, scope_type, scope_id)
    WHERE status = 'active' AND scope_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_agent_thread_active_scope_unique;

ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume'
    ));
