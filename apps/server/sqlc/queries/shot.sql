-- name: CreateShot :one
INSERT INTO shot (
    workspace_id,
    client_key,
    sort_order,
    title,
    brief,
    duration_sec,
    narrative_purpose,
    status,
    scene_id,
    shot_kind,
    creative_text,
    visual_intent,
    action_text,
    camera_intent,
    dialogue,
    narration,
    audio_plan
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14, $15, $16, $17
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
    scene_id = $9,
    shot_kind = $10,
    creative_text = $11,
    visual_intent = $12,
    action_text = $13,
    camera_intent = $14,
    dialogue = $15,
    narration = $16,
    audio_plan = $17,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $18
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
