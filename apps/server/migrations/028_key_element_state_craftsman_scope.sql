-- +goose Up
ALTER TABLE agent_thread DROP CONSTRAINT agent_thread_scope_type_check;
ALTER TABLE agent_task DROP CONSTRAINT agent_task_scope_type_check;

ALTER TABLE agent_thread
    ADD CONSTRAINT agent_thread_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'final_output', 'render_plan', 'key_element_state'));

ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'node', 'job', 'final_output', 'render_plan', 'key_element_state'));

-- +goose Down
ALTER TABLE agent_thread DROP CONSTRAINT agent_thread_scope_type_check;
ALTER TABLE agent_task DROP CONSTRAINT agent_task_scope_type_check;

ALTER TABLE agent_thread
    ADD CONSTRAINT agent_thread_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'final_output', 'render_plan'));

ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'node', 'job', 'final_output', 'render_plan'));
