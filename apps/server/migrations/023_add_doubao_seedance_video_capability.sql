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
        'doubao-seedance-1-5-pro-251215',
        'Doubao Seedance 1.5 Pro 251215',
        '["video"]',
        '["text_to_video", "image_to_video", "multi_reference_to_video"]',
        '["text", "image", "video"]',
        '{"max_prompt_chars": 8000, "max_attempts": 1, "async_required": true, "durations_sec": [5, 10], "aspect_ratios": ["16:9", "9:16", "1:1"], "max_input_images": 9, "max_output_bytes": 524288000}',
        '{"tier": "real"}',
        '{"duration_sec": 5, "aspect_ratio": "16:9", "resolution": "720p"}',
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
  AND model_id = 'doubao-seedance-1-5-pro-251215';
