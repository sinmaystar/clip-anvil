-- name: CreateWorkspace :one
INSERT INTO workspace (name, owner_id, mode)
VALUES ($1, $2, $3)
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

-- name: UpdateWorkspaceSettings :one
UPDATE workspace
SET settings = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;
