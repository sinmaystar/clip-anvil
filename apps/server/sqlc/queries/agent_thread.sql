-- name: CreateAgentThread :one
INSERT INTO agent_thread (
    workspace_id,
    role,
    scope_type,
    scope_id,
    runtime_provider,
    runtime_agent_name,
    summary
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetAgentThreadByID :one
SELECT *
FROM agent_thread
WHERE id = $1;

-- name: GetActiveProducerThreadByWorkspace :one
SELECT *
FROM agent_thread
WHERE workspace_id = $1
  AND role = 'producer'
  AND scope_type = 'workspace'
  AND status = 'active'
ORDER BY created_at DESC
LIMIT 1;

-- name: GetActiveComposerThreadByWorkspace :one
SELECT *
FROM agent_thread
WHERE workspace_id = $1
  AND role = 'composer'
  AND scope_type = 'final_output'
  AND scope_id IS NULL
  AND status = 'active'
ORDER BY created_at DESC
LIMIT 1;

-- name: GetActiveAgentThreadByScope :one
SELECT *
FROM agent_thread
WHERE workspace_id = $1
  AND role = $2
  AND scope_type = $3
  AND scope_id = $4
  AND status = 'active'
ORDER BY created_at DESC
LIMIT 1;

-- name: ListAgentThreadsByWorkspace :many
SELECT *
FROM agent_thread
WHERE workspace_id = $1
ORDER BY created_at DESC;

-- name: UpdateAgentThreadStatus :one
UPDATE agent_thread
SET status = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetAgentThreadCheckpoint :one
UPDATE agent_thread
SET current_checkpoint_key = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;
