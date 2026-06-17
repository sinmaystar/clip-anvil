-- name: CreateWorkspaceSandboxBinding :one
INSERT INTO workspace_sandbox (
    workspace_id,
    volume_name,
    status
) VALUES (
    $1, $2, $3
)
ON CONFLICT (workspace_id) DO UPDATE
SET updated_at = workspace_sandbox.updated_at
RETURNING *;

-- name: GetWorkspaceSandboxBinding :one
SELECT *
FROM workspace_sandbox
WHERE workspace_id = $1;

-- name: LockWorkspaceSandboxBinding :one
SELECT *
FROM workspace_sandbox
WHERE workspace_id = $1
FOR UPDATE;

-- name: MarkWorkspaceSandboxCreating :one
UPDATE workspace_sandbox
SET status = 'creating',
    error_message = NULL,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;

-- name: MarkWorkspaceSandboxRunning :one
UPDATE workspace_sandbox
SET sandbox_id = $2,
    status = 'running',
    last_health_check_at = now(),
    last_seen_at = now(),
    error_message = NULL,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;

-- name: MarkWorkspaceSandboxUnhealthy :one
UPDATE workspace_sandbox
SET status = 'unhealthy',
    last_health_check_at = now(),
    error_message = $2,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;

-- name: MarkWorkspaceSandboxTerminated :one
UPDATE workspace_sandbox
SET status = 'terminated',
    sandbox_id = NULL,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;

-- name: TouchWorkspaceSandboxSeen :one
UPDATE workspace_sandbox
SET last_health_check_at = now(),
    last_seen_at = now(),
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;
