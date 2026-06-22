-- +goose Up
UPDATE model_capability
SET defaults = jsonb_set(
        coalesce(defaults, '{}'::jsonb),
        '{max_completion_tokens}',
        '16384'::jsonb,
        true
    ),
    updated_at = now()
WHERE provider_id = 'volcengine'
  AND model_id = 'doubao-seed-2-0-pro-260215';

-- +goose Down
UPDATE model_capability
SET defaults = jsonb_set(
        coalesce(defaults, '{}'::jsonb),
        '{max_completion_tokens}',
        '4096'::jsonb,
        true
    ),
    updated_at = now()
WHERE provider_id = 'volcengine'
  AND model_id = 'doubao-seed-2-0-pro-260215';
