-- +goose Up
CREATE TABLE model_provider (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE model_capability (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id TEXT NOT NULL REFERENCES model_provider(id) ON DELETE CASCADE,
    model_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    output_types JSONB NOT NULL DEFAULT '[]',
    supported_operations JSONB NOT NULL DEFAULT '[]',
    supported_input_node_types JSONB NOT NULL DEFAULT '[]',
    limits JSONB NOT NULL DEFAULT '{}',
    pricing JSONB NOT NULL DEFAULT '{}',
    defaults JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_model_capability UNIQUE (provider_id, model_id)
);

CREATE INDEX idx_model_capability_provider ON model_capability(provider_id);
CREATE INDEX idx_model_capability_enabled ON model_capability(provider_id, enabled);

INSERT INTO model_provider (id, display_name, provider_type, config, enabled)
VALUES
    ('mock', 'Mock Provider', 'media_generation', '{}', true),
    ('volcengine', 'Volcengine Ark', 'media_generation', '{}', true),
    ('internal_ffmpeg', 'Internal FFmpeg', 'internal_media', '{}', true);

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
        'mock',
        'mock-text',
        'Mock Text',
        '["text"]',
        '["text_generation"]',
        '["text"]',
        '{"max_prompt_chars": 8000, "max_attempts": 3}',
        '{"tier": "mock"}',
        '{"temperature": 0.2}',
        true
    ),
    (
        'mock',
        'mock-image-only',
        'Mock Image Only',
        '["image"]',
        '["text_to_image"]',
        '["text", "image"]',
        '{"max_prompt_chars": 8000, "max_attempts": 3}',
        '{"tier": "mock"}',
        '{}',
        true
    ),
    (
        'mock',
        'mock-video',
        'Mock Video',
        '["video"]',
        '["text_to_video", "image_to_video"]',
        '["text", "image", "video"]',
        '{"max_prompt_chars": 8000, "max_attempts": 3, "durations_sec": [4, 5, 8]}',
        '{"tier": "mock"}',
        '{"duration_sec": 5}',
        true
    ),
    (
        'internal_ffmpeg',
        'ffmpeg',
        'Internal FFmpeg',
        '["image"]',
        '["extract_first_frame", "extract_last_frame"]',
        '["video"]',
        '{"max_attempts": 1}',
        '{"tier": "internal"}',
        '{}',
        true
    );

-- +goose Down
DROP INDEX IF EXISTS idx_model_capability_enabled;
DROP INDEX IF EXISTS idx_model_capability_provider;
DROP TABLE IF EXISTS model_capability;
DROP TABLE IF EXISTS model_provider;
