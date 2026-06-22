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

-- +goose Down
ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume'
    ));
