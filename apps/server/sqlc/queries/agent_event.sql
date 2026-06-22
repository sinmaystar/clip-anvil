-- name: CreateAgentEvent :one
INSERT INTO agent_event (
    workspace_id,
    thread_id,
    task_id,
    event_type,
    source_role,
    target_role,
    scope,
    payload
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetAgentEventByID :one
SELECT *
FROM agent_event
WHERE id = $1;

-- name: GetAgentEventForWorkspace :one
SELECT *
FROM agent_event
WHERE id = $1
  AND workspace_id = $2;

-- name: ListAgentEventsByWorkspaceStatus :many
SELECT *
FROM agent_event
WHERE workspace_id = $1
  AND status = $2
ORDER BY created_at DESC;

-- name: MarkAgentEventHandled :one
UPDATE agent_event
SET status = 'handled',
    handled_at = now()
WHERE id = $1
RETURNING *;
