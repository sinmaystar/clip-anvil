-- name: NextAgentMessageSeq :one
WITH locked_thread AS (
    SELECT agent_thread.id AS thread_id
    FROM agent_thread
    WHERE agent_thread.id = $1
    FOR UPDATE
)
SELECT (COALESCE(MAX(agent_message.seq), 0) + 1)::bigint AS seq
FROM locked_thread
LEFT JOIN agent_message ON agent_message.thread_id = locked_thread.thread_id;

-- name: CreateAgentMessage :one
INSERT INTO agent_message (
    workspace_id,
    thread_id,
    seq,
    role,
    message_type,
    content,
    raw_message,
    task_id,
    event_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: ListAgentMessagesByThread :many
SELECT *
FROM agent_message
WHERE thread_id = $1
  AND seq > $2
ORDER BY seq
LIMIT $3;

-- name: ListAgentMessagesByWorkspace :many
SELECT *
FROM agent_message
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(after_created_at)::timestamptz IS NULL OR created_at > sqlc.narg(after_created_at))
ORDER BY created_at, thread_id, seq
LIMIT sqlc.arg(row_limit);

-- name: UpdateAgentMessage :one
UPDATE agent_message
SET
    content = $2,
    raw_message = $3,
    event_id = $4
WHERE id = $1
RETURNING *;
