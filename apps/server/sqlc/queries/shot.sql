-- name: CreateShot :one
INSERT INTO shot (
    workspace_id,
    client_key,
    sort_order,
    title,
    brief,
    duration_sec,
    narrative_purpose,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetShotByID :one
SELECT *
FROM shot
WHERE id = $1;

-- name: GetShotByClientKey :one
SELECT *
FROM shot
WHERE workspace_id = $1
  AND client_key = $2
  AND archived_at IS NULL;

-- name: ListActiveShotsByWorkspace :many
SELECT *
FROM shot
WHERE workspace_id = $1
  AND archived_at IS NULL
ORDER BY sort_order, created_at;

-- name: UpdateShot :one
UPDATE shot
SET client_key = $2,
    sort_order = $3,
    title = $4,
    brief = $5,
    duration_sec = $6,
    narrative_purpose = $7,
    status = $8,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $9
  AND archived_at IS NULL
RETURNING *;

-- name: ArchiveShot :one
UPDATE shot
SET status = 'archived',
    archived_at = now(),
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL
RETURNING *;

-- name: ReorderShot :one
UPDATE shot
SET sort_order = $2,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $3
  AND archived_at IS NULL
RETURNING *;

-- name: SetShotCraftsmanThread :one
UPDATE shot
SET craftsman_thread_id = $2,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $3
RETURNING *;

-- name: UpdateShotStatus :one
UPDATE shot
SET status = $3,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL
RETURNING *;
