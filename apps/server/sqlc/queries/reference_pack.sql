-- name: ListReferencePackItems :many
SELECT reference_pack_item.*
FROM reference_pack_item
WHERE pack_node_id = $1
ORDER BY position, created_at;

-- name: ListReferencePackItemNodes :many
SELECT media_node.*
FROM reference_pack_item
JOIN media_node ON media_node.id = reference_pack_item.member_node_id
WHERE reference_pack_item.pack_node_id = $1
ORDER BY reference_pack_item.position, reference_pack_item.created_at;

-- name: ListReferencePacksByMember :many
SELECT media_node.*
FROM reference_pack_item
JOIN media_node ON media_node.id = reference_pack_item.pack_node_id
WHERE reference_pack_item.member_node_id = $1
ORDER BY reference_pack_item.created_at;

-- name: DeleteReferencePackItems :exec
DELETE FROM reference_pack_item
WHERE pack_node_id = $1;

-- name: CreateReferencePackItem :one
INSERT INTO reference_pack_item (
    workspace_id,
    pack_node_id,
    member_node_id,
    position,
    metadata
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;
