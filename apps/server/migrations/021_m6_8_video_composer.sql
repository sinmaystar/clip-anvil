-- +goose Up
ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume',
        'craftsman_turn',
        'worker_generation',
        'reviewer_turn',
        'dependency_scheduler',
        'composer_turn'
    ));

INSERT INTO model_capability (
    provider_id,
    model_id,
    display_name,
    output_types,
    supported_operations,
    supported_input_node_types,
    limits,
    pricing,
    defaults,
    enabled
) VALUES (
    'internal_ffmpeg',
    'ffmpeg-compose',
    'Internal FFmpeg Compose',
    '["video"]',
    '["compose_final_video"]',
    '["video"]',
    '{"max_attempts": 1}',
    '{"tier": "internal"}',
    '{}',
    true
) ON CONFLICT (provider_id, model_id) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    output_types = EXCLUDED.output_types,
    supported_operations = EXCLUDED.supported_operations,
    supported_input_node_types = EXCLUDED.supported_input_node_types,
    limits = EXCLUDED.limits,
    pricing = EXCLUDED.pricing,
    defaults = EXCLUDED.defaults,
    enabled = EXCLUDED.enabled;

-- +goose Down
DELETE FROM model_capability
WHERE provider_id = 'internal_ffmpeg'
  AND model_id = 'ffmpeg-compose';

ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume',
        'craftsman_turn',
        'worker_generation',
        'reviewer_turn',
        'dependency_scheduler'
    ));
