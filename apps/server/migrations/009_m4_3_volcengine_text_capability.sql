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
) VALUES (
    'volcengine',
    'doubao-seed-1-6-lite',
    'Doubao Seed 1.6 Lite',
    '["text"]',
    '["text_generation"]',
    '["text"]',
    '{"max_prompt_chars": 8000, "max_attempts": 1}',
    '{"tier": "cheap"}',
    '{"temperature": 0.2}',
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
  AND model_id = 'doubao-seed-1-6-lite';
