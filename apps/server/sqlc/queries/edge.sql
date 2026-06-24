-- name: CreateMediaEdge :one
INSERT INTO media_edge (
    workspace_id,
    from_node_id,
    to_node_id,
    metadata
) VALUES (
    $1,
    $2,
    $3,
    $4
) RETURNING *;

-- name: ListMediaEdgesByWorkspace :many
SELECT * FROM media_edge
WHERE workspace_id = $1
ORDER BY created_at;

-- name: GetMediaEdgeByID :one
SELECT * FROM media_edge
WHERE id = $1;

-- name: GetDependencyEdgeByEndpoints :one
SELECT * FROM media_edge
WHERE from_node_id = $1
  AND to_node_id = $2;

-- name: ListOutgoingDependencyEdges :many
SELECT * FROM media_edge
WHERE from_node_id = $1
ORDER BY created_at;

-- name: DeleteMediaEdge :exec
DELETE FROM media_edge
WHERE id = $1;
