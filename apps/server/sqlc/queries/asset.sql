-- name: CreateMediaAsset :one
INSERT INTO media_asset (
    workspace_id,
    type,
    mime,
    storage_url,
    thumbnail_url,
    duration_ms,
    size_bytes,
    metadata
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: CreateTextMediaAsset :one
INSERT INTO media_asset (
    workspace_id,
    type,
    mime,
    storage_url,
    text_content,
    thumbnail_url,
    duration_ms,
    size_bytes,
    metadata
) VALUES (
    $1, 'text', 'text/plain; charset=utf-8', NULL, $2, NULL, NULL, $3, $4
) RETURNING *;

-- name: GetMediaAssetByID :one
SELECT *
FROM media_asset
WHERE id = $1;

-- name: ListMediaAssetsByWorkspace :many
SELECT *
FROM media_asset
WHERE workspace_id = $1
ORDER BY created_at;
