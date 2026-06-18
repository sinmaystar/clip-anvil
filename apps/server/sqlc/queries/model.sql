-- name: GetEnabledModelProvider :one
SELECT *
FROM model_provider
WHERE id = $1
  AND enabled = true;

-- name: GetEnabledModelCapability :one
SELECT *
FROM model_capability
WHERE provider_id = $1
  AND model_id = $2
  AND enabled = true;

-- name: ListEnabledModelCapabilities :many
SELECT *
FROM model_capability
WHERE enabled = true
ORDER BY provider_id, model_id;
