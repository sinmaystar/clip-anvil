-- name: CreateAgentTask :one
INSERT INTO agent_task (
    workspace_id,
    thread_id,
    role,
    scope_type,
    scope_id,
    task_type,
    max_attempts,
    input
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetAgentTaskByID :one
SELECT *
FROM agent_task
WHERE id = $1;

-- name: ListAgentTasksByWorkspaceStatus :many
SELECT *
FROM agent_task
WHERE workspace_id = $1
  AND status = $2
ORDER BY created_at DESC;

-- name: ListActiveAgentTasksByWorkspace :many
SELECT *
FROM agent_task
WHERE workspace_id = $1
  AND status IN ('queued', 'running', 'waiting_for_user')
ORDER BY created_at DESC;

-- name: ListQueuedProducerTasks :many
SELECT *
FROM agent_task
WHERE workspace_id = $1
  AND role = 'producer'
  AND task_type = 'producer_turn'
  AND status = 'queued'
ORDER BY created_at ASC
LIMIT $2;

-- name: ListQueuedProducerTasksAcrossWorkspaces :many
SELECT *
FROM agent_task
WHERE role = 'producer'
  AND task_type = 'producer_turn'
  AND status = 'queued'
ORDER BY created_at ASC
LIMIT $1;

-- name: ListQueuedCraftsmanTasksAcrossWorkspaces :many
SELECT *
FROM agent_task
WHERE role = 'craftsman'
  AND task_type = 'craftsman_turn'
  AND status = 'queued'
ORDER BY created_at ASC
LIMIT $1;

-- name: ListQueuedWorkerTasksAcrossWorkspaces :many
SELECT *
FROM agent_task
WHERE role = 'worker'
  AND task_type = 'worker_generation'
  AND status = 'queued'
ORDER BY created_at ASC
LIMIT $1;

-- name: MarkAgentTaskRunning :one
UPDATE agent_task
SET status = 'running',
    attempt = attempt + 1,
    started_at = COALESCE(started_at, now()),
    error_code = NULL,
    error_message = NULL
WHERE id = $1
RETURNING *;

-- name: MarkAgentTaskSucceeded :one
UPDATE agent_task
SET status = 'succeeded',
    output = $2,
    error_code = NULL,
    error_message = NULL,
    completed_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkAgentTaskFailed :one
UPDATE agent_task
SET status = 'failed',
    error_code = $2,
    error_message = $3,
    completed_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkAgentTaskCancelled :one
UPDATE agent_task
SET status = 'cancelled',
    completed_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkAgentTaskWaitingForUser :one
UPDATE agent_task
SET status = 'waiting_for_user'
WHERE id = $1
RETURNING *;
