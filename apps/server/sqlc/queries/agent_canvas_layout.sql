-- name: ListAgentCanvasLayoutsByWorkspace :many
SELECT *
FROM agent_canvas_layout
WHERE workspace_id = $1
ORDER BY object_type, updated_at;

-- name: UpsertAgentCanvasLayout :one
INSERT INTO agent_canvas_layout (
    workspace_id,
    object_type,
    object_id,
    canvas_x,
    canvas_y
) VALUES (
    $1, $2, $3, $4, $5
)
ON CONFLICT (workspace_id, object_type, object_id)
DO UPDATE SET
    canvas_x = EXCLUDED.canvas_x,
    canvas_y = EXCLUDED.canvas_y,
    updated_at = now()
RETURNING *;
