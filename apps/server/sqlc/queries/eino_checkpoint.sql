-- name: UpsertEinoCheckpoint :one
INSERT INTO eino_checkpoint (
    key,
    workspace_id,
    thread_id,
    task_id,
    value,
    metadata
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (key)
DO UPDATE SET
    workspace_id = EXCLUDED.workspace_id,
    thread_id = EXCLUDED.thread_id,
    task_id = EXCLUDED.task_id,
    value = EXCLUDED.value,
    metadata = EXCLUDED.metadata,
    updated_at = now()
RETURNING *;

-- name: GetEinoCheckpoint :one
SELECT *
FROM eino_checkpoint
WHERE key = $1;

-- name: DeleteEinoCheckpoint :exec
DELETE FROM eino_checkpoint
WHERE key = $1;

-- name: ListEinoCheckpointsByThread :many
SELECT *
FROM eino_checkpoint
WHERE thread_id = $1
ORDER BY updated_at DESC;
