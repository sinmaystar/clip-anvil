-- +goose Up
DELETE FROM model_capability
WHERE provider_id = 'internal_template_video'
  AND model_id = 'hyperframes-html';

DELETE FROM model_provider
WHERE id = 'internal_template_video';

INSERT INTO model_provider (
    id,
    display_name,
    provider_type,
    config,
    enabled
) VALUES (
    'internal_motion_video',
    'Internal Motion Video',
    'internal_media',
    '{"engine": "remotion"}',
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
    'internal_motion_video',
    'remotion-motion-shot-v1',
    'Remotion Motion Shot Video',
    '["video"]',
    '["image_to_motion_video"]',
    '["image", "text"]',
    '{"max_prompt_chars": 3000, "max_attempts": 1, "async_required": true, "durations_sec": [3, 4, 5, 6, 8], "resolutions": ["720p", "1080p"], "ratios": ["9:16", "16:9", "1:1"], "max_input_images": 4}',
    '{"tier": "internal", "cost_class": "low", "external_api_cost": false}',
    '{"ratio": "9:16", "duration_sec": 5, "resolution": "1080p", "fps": 30, "watermark": false}',
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

UPDATE render_plan
SET model_prompt_profile = 'motion_shot_video',
    updated_at = now()
WHERE model_prompt_profile = 'template_video';

ALTER TABLE render_plan ADD CONSTRAINT render_plan_profile_check
    CHECK (model_prompt_profile IN ('seedream_5_image', 'seedance_2_video', 'seed_audio_1', 'motion_shot_video'));

-- +goose Down
ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_profile_check;

UPDATE render_plan
SET model_prompt_profile = 'seedance_2_video',
    updated_at = now()
WHERE model_prompt_profile = 'motion_shot_video';

ALTER TABLE render_plan ADD CONSTRAINT render_plan_profile_check
    CHECK (model_prompt_profile IN ('seedream_5_image', 'seedance_2_video', 'seed_audio_1'));

DELETE FROM model_capability
WHERE provider_id = 'internal_motion_video'
  AND model_id = 'remotion-motion-shot-v1';

DELETE FROM model_provider
WHERE id = 'internal_motion_video';
