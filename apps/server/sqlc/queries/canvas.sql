-- name: CreateCanvasDocument :one
INSERT INTO canvas_document (workspace_id)
VALUES ($1)
RETURNING *;

-- name: GetCanvasDocumentByWorkspace :one
SELECT *
FROM canvas_document
WHERE workspace_id = $1;

-- name: UpdateCamera :one
UPDATE canvas_document
SET camera_x = $2,
    camera_y = $3,
    camera_zoom = $4,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;
