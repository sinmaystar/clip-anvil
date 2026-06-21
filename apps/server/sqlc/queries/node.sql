-- name: CreateMediaNode :one
INSERT INTO media_node (
    workspace_id,
    node_type,
    title,
    prompt,
    prompt_template,
    status,
    asset_id,
    canvas_x,
    canvas_y,
    canvas_w,
    canvas_h
)
VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: CreateMediaNodeWithID :one
INSERT INTO media_node (
    id,
    workspace_id,
    node_type,
    title,
    prompt,
    prompt_template,
    status,
    asset_id,
    canvas_x,
    canvas_y,
    canvas_w,
    canvas_h
)
VALUES ($1, $2, $3, $4, $5, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: CreateAgentMediaNode :one
INSERT INTO media_node (
    workspace_id,
    node_type,
    title,
    prompt,
    prompt_template,
    status,
    source,
    asset_id,
    canvas_x,
    canvas_y,
    canvas_w,
    canvas_h
)
VALUES ($1, $2, $3, $4, $4, 'succeeded', 'agent', $5, $6, $7, $8, $9)
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
    prompt_template = $2,
    prompt_refs = $3,
    prompt_rich = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaNodeProductionConfig :one
UPDATE media_node
SET operation_type = $2,
    prompt_template = $3,
    prompt = $3,
    model_provider = $4,
    model_id = $5,
    model_params = $6,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaNodeCurrentVersion :one
UPDATE media_node
SET current_version_id = $2,
    status = 'succeeded',
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

-- name: UpdateMediaNodeGroup :one
UPDATE media_node
SET group_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaNodeAsset :one
UPDATE media_node
SET asset_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ClearMediaNodeGroup :one
UPDATE media_node
SET group_id = NULL,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListMediaNodesByGroup :many
SELECT *
FROM media_node
WHERE group_id = $1
ORDER BY created_at;

-- name: ListUpstreamDependencyNodes :many
SELECT media_node.*
FROM media_node
JOIN media_edge ON media_edge.from_node_id = media_node.id
WHERE media_edge.to_node_id = $1
ORDER BY media_edge.created_at;

-- name: ListDownstreamDependencyNodes :many
SELECT media_node.*
FROM media_node
JOIN media_edge ON media_edge.to_node_id = media_node.id
WHERE media_edge.from_node_id = $1
ORDER BY media_edge.created_at;

-- name: DeleteMediaNode :exec
DELETE FROM media_node
WHERE id = $1;
