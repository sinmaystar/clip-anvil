-- name: CreateWorkspace :one
INSERT INTO workspace (name, owner_id)
VALUES ($1, $2)
RETURNING *;

-- name: ListWorkspacesByOwner :many
SELECT *
FROM workspace
WHERE owner_id = $1
ORDER BY created_at DESC;

-- name: GetWorkspaceByID :one
SELECT *
FROM workspace
WHERE id = $1;
