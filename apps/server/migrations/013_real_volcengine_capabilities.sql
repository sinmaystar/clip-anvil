-- +goose Up
INSERT INTO model_provider (id, display_name, provider_type, config, enabled)
VALUES (
    'volcengine',
    'Volcengine Ark',
    'media_generation',
    '{"region": "cn-beijing", "base_url": "https://ark.cn-beijing.volces.com/api/v3", "docs": "volcengine ark"}',
    true
)
ON CONFLICT (id) DO UPDATE
SET display_name = EXCLUDED.display_name,
    provider_type = EXCLUDED.provider_type,
    config = EXCLUDED.config,
    enabled = EXCLUDED.enabled,
    updated_at = now();

UPDATE model_capability
SET enabled = false,
    updated_at = now()
WHERE provider_id = 'volcengine'
  AND model_id = 'doubao-seed-1-6-lite';

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
        'doubao-seed-2-0-mini-260428',
        'Doubao Seed 2.0 Mini',
        '["text"]',
        '["text_generation"]',
        '["text", "image"]',
        '{"max_prompt_chars": 16000, "max_attempts": 1, "async_required": true}',
        '{"tier": "real"}',
        '{"temperature": 0.2, "max_tokens": 2048}',
        true
    ),
    (
        'volcengine',
        'doubao-seedream-5-0-260128',
        'Doubao Seedream 5.0',
        '["image"]',
        '["text_to_image", "image_to_image", "multi_image_to_image"]',
        '["text", "image"]',
        '{"max_prompt_chars": 8000, "max_attempts": 1, "async_required": true, "max_input_images": 9, "max_output_bytes": 52428800}',
        '{"tier": "real"}',
        '{"size": "2048x2048", "response_format": "url", "disable_watermark": false}',
        true
    ),
    (
        'volcengine',
        'doubao-seedance-1-0-pro-fast-251015',
        'Doubao Seedance 1.0 Pro Fast',
        '["video"]',
        '["text_to_video", "image_to_video", "multi_reference_to_video"]',
        '["text", "image", "video"]',
        '{"max_prompt_chars": 8000, "max_attempts": 1, "async_required": true, "durations_sec": [5, 10], "aspect_ratios": ["16:9", "9:16", "1:1"], "max_input_images": 9, "max_output_bytes": 524288000}',
        '{"tier": "real"}',
        '{"duration_sec": 5, "aspect_ratio": "16:9", "resolution": "720p"}',
        true
    ),
    (
        'volcengine',
        'volcengine-audio-hold',
        'Volcengine Audio Hold',
        '["audio"]',
        '["text_to_audio"]',
        '["text", "audio"]',
        '{"max_prompt_chars": 8000, "max_attempts": 1, "async_required": true}',
        '{"tier": "hold"}',
        '{"status": "hold", "reason": "no usable Volcengine audio model configured"}',
        false
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
    'doubao-seed-2-0-mini-260428',
    'doubao-seedream-5-0-260128',
    'doubao-seedance-1-0-pro-fast-251015',
    'volcengine-audio-hold'
  );

UPDATE model_capability
SET enabled = true,
    updated_at = now()
WHERE provider_id = 'volcengine'
  AND model_id = 'doubao-seed-1-6-lite';
