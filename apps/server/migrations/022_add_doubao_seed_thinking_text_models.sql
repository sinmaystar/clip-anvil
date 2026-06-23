-- +goose Up
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
) VALUES
    (
        'volcengine',
        'doubao-seed-2-0-lite-260428',
        'Doubao Seed 2.0 Lite',
        '["text"]',
        '["text_generation"]',
        '["text", "image"]',
        '{"max_prompt_chars": 32000, "max_attempts": 1, "async_required": true, "max_input_images": 8, "reasoning_efforts": ["minimal", "low", "medium", "high"]}',
        '{"tier": "real"}',
        '{"temperature": 0.3, "max_completion_tokens": 16384, "reasoning_effort": "minimal"}',
        true
    ),
    (
        'volcengine',
        'doubao-seed-2-1-turbo-260628',
        'Doubao Seed 2.1 Turbo',
        '["text"]',
        '["text_generation"]',
        '["text", "image"]',
        '{"max_prompt_chars": 32000, "max_attempts": 1, "async_required": true, "max_input_images": 8, "reasoning_efforts": ["minimal", "low", "medium", "high"]}',
        '{"tier": "real"}',
        '{"temperature": 0.3, "max_completion_tokens": 16384, "reasoning_effort": "minimal"}',
        true
    ),
    (
        'volcengine',
        'doubao-seed-2-1-pro-260628',
        'Doubao Seed 2.1 Pro',
        '["text"]',
        '["text_generation"]',
        '["text", "image"]',
        '{"max_prompt_chars": 32000, "max_attempts": 1, "async_required": true, "max_input_images": 8, "reasoning_efforts": ["minimal", "low", "medium", "high"]}',
        '{"tier": "real"}',
        '{"temperature": 0.3, "max_completion_tokens": 16384, "reasoning_effort": "minimal"}',
        true
    )
ON CONFLICT (provider_id, model_id) DO UPDATE
SET display_name = EXCLUDED.display_name,
    output_types = EXCLUDED.output_types,
    supported_operations = EXCLUDED.supported_operations,
    supported_input_node_types = EXCLUDED.supported_input_node_types,
    limits = EXCLUDED.limits,
    pricing = EXCLUDED.pricing,
    defaults = EXCLUDED.defaults,
    enabled = EXCLUDED.enabled,
    updated_at = now();

-- +goose Down
DELETE FROM model_capability
WHERE provider_id = 'volcengine'
  AND model_id IN (
    'doubao-seed-2-0-lite-260428',
    'doubao-seed-2-1-turbo-260628',
    'doubao-seed-2-1-pro-260628'
  );
