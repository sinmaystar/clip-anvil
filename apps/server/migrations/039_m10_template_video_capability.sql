-- +goose Up
INSERT INTO model_provider (
    id,
    display_name,
    provider_type,
    config,
    enabled
) VALUES (
    'internal_template_video',
    'Internal Template Video',
    'internal_media',
    '{"engine": "hyperframes"}',
    true
)
ON CONFLICT (id) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    provider_type = EXCLUDED.provider_type,
    config = EXCLUDED.config,
    enabled = EXCLUDED.enabled,
    updated_at = now();

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
    'internal_template_video',
    'hyperframes-html',
    'HyperFrames HTML Template Video',
    '["video"]',
    '["template_to_video", "image_to_template_video"]',
    '["text", "image"]',
    '{"max_prompt_chars": 3000, "max_attempts": 1, "async_required": true, "durations_sec": [3, 4, 5, 6, 8, 10], "resolutions": ["720p", "1080p"], "ratios": ["9:16", "16:9", "1:1"]}',
    '{"tier": "internal", "cost_class": "low", "external_api_cost": false}',
    '{"ratio": "9:16", "duration_sec": 5, "resolution": "1080p", "watermark": false}',
    true
)
ON CONFLICT (provider_id, model_id) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    output_types = EXCLUDED.output_types,
    supported_operations = EXCLUDED.supported_operations,
    supported_input_node_types = EXCLUDED.supported_input_node_types,
    limits = EXCLUDED.limits,
    pricing = EXCLUDED.pricing,
    defaults = EXCLUDED.defaults,
    enabled = EXCLUDED.enabled,
    updated_at = now();

ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_profile_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_profile_check
    CHECK (model_prompt_profile IN ('seedream_5_image', 'seedance_2_video', 'seed_audio_1', 'template_video'));

-- +goose Down
ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_profile_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_profile_check
    CHECK (model_prompt_profile IN ('seedream_5_image', 'seedance_2_video', 'seed_audio_1'));

DELETE FROM model_capability
WHERE provider_id = 'internal_template_video'
  AND model_id = 'hyperframes-html';

DELETE FROM model_provider
WHERE id = 'internal_template_video';
