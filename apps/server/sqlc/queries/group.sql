-- name: CreateMediaGroup :one
INSERT INTO media_group (workspace_id, name, sort_order)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListMediaGroupsByWorkspace :many
SELECT *
FROM media_group
WHERE workspace_id = $1
ORDER BY sort_order, created_at;

-- name: GetMediaGroupByID :one
SELECT *
FROM media_group
WHERE id = $1;

-- name: UpdateMediaGroupName :one
UPDATE media_group
SET name = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaGroupSortOrder :one
UPDATE media_group
SET sort_order = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMediaGroup :exec
DELETE FROM media_group
WHERE id = $1;
