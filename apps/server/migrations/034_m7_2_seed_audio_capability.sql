-- +goose Up
UPDATE model_capability
SET enabled = false,
    updated_at = now()
WHERE provider_id = 'volcengine'
  AND model_id = 'volcengine-audio-hold';

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
    'seed-audio-1.0',
    'Volcengine Seed Audio 1.0',
    '["audio"]',
    '["text_to_audio"]',
    '["text", "audio"]',
    '{"max_prompt_chars": 4000, "max_attempts": 1, "async_required": true, "max_duration_sec": 120, "formats": ["mp3", "wav", "ogg_opus", "pcm"], "sample_rates": [16000, 24000, 48000], "max_reference_audio_count": 1, "max_reference_audio_bytes": 10485760}',
    '{"tier": "real"}',
    '{"format": "mp3", "sample_rate": 48000, "watermark": false}',
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
  AND model_id = 'seed-audio-1.0';

UPDATE model_capability
SET enabled = false,
    updated_at = now()
WHERE provider_id = 'volcengine'
  AND model_id = 'volcengine-audio-hold';
