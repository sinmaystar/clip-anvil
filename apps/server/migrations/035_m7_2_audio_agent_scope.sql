-- +goose Up
ALTER TABLE agent_thread DROP CONSTRAINT IF EXISTS agent_thread_scope_type_check;
ALTER TABLE agent_thread ADD CONSTRAINT agent_thread_scope_type_check
    CHECK (scope_type IN ('workspace', 'shot', 'final_output', 'render_plan', 'key_element_state', 'audio_plan'));

ALTER TABLE agent_task DROP CONSTRAINT IF EXISTS agent_task_scope_type_check;
ALTER TABLE agent_task ADD CONSTRAINT agent_task_scope_type_check
    CHECK (scope_type IN ('workspace', 'shot', 'node', 'job', 'final_output', 'render_plan', 'key_element_state', 'audio_plan'));

ALTER TABLE producer_pending_signal DROP CONSTRAINT IF EXISTS producer_pending_signal_scope_type_check;
ALTER TABLE producer_pending_signal ADD CONSTRAINT producer_pending_signal_scope_type_check
    CHECK (scope_type IN ('workspace', 'shot', 'render_plan', 'final_output', 'key_element_state', 'audio_plan', 'node', 'job'));

-- +goose Down
ALTER TABLE producer_pending_signal DROP CONSTRAINT IF EXISTS producer_pending_signal_scope_type_check;
ALTER TABLE producer_pending_signal ADD CONSTRAINT producer_pending_signal_scope_type_check
    CHECK (scope_type IN ('workspace', 'shot', 'render_plan', 'final_output', 'key_element_state', 'node', 'job'));

ALTER TABLE agent_task DROP CONSTRAINT IF EXISTS agent_task_scope_type_check;
ALTER TABLE agent_task ADD CONSTRAINT agent_task_scope_type_check
    CHECK (scope_type IN ('workspace', 'shot', 'node', 'job', 'final_output', 'render_plan', 'key_element_state'));

ALTER TABLE agent_thread DROP CONSTRAINT IF EXISTS agent_thread_scope_type_check;
ALTER TABLE agent_thread ADD CONSTRAINT agent_thread_scope_type_check
    CHECK (scope_type IN ('workspace', 'shot', 'final_output', 'render_plan', 'key_element_state'));
