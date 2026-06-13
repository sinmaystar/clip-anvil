-- name: CreateMediaNode :one
INSERT INTO media_node (
    workspace_id,
    node_type,
    title,
    prompt,
    canvas_x,
    canvas_y,
    canvas_w,
    canvas_h
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: CreateMediaNodeWithID :one
INSERT INTO media_node (
    id,
    workspace_id,
    node_type,
    title,
    prompt,
    status,
    canvas_x,
    canvas_y,
    canvas_w,
    canvas_h
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: ListMediaNodesByWorkspace :many
SELECT *
FROM media_node
WHERE workspace_id = $1
ORDER BY created_at;

-- name: GetMediaNodeByID :one
SELECT *
FROM media_node
WHERE id = $1;

-- name: UpdateMediaNodeTitle :one
UPDATE media_node
SET title = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaNodePrompt :one
UPDATE media_node
SET prompt = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaNodeStatus :one
UPDATE media_node
SET status = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaNodePosition :one
UPDATE media_node
SET canvas_x = $2,
    canvas_y = $3,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMediaNode :exec
DELETE FROM media_node
WHERE id = $1;
