-- name: CreateSandboxJob :one
INSERT INTO sandbox_job (
    workspace_id,
    target_node_id,
    generation_job_id,
    job_type,
    operation_type,
    status,
    input
) VALUES (
    $1, $2, $3, $4, $5, 'pending', $6
) RETURNING *;

-- name: MarkSandboxJobRunning :one
UPDATE sandbox_job
SET status = 'running',
    sandbox_id = $2,
    command = $3,
    cwd = $4,
    started_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkSandboxJobSucceeded :one
UPDATE sandbox_job
SET status = 'succeeded',
    output = $2,
    exit_code = $3,
    stdout = $4,
    stderr = $5,
    duration_ms = $6,
    completed_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkSandboxJobFailed :one
UPDATE sandbox_job
SET status = 'failed',
    output = $2,
    exit_code = $3,
    stdout = $4,
    stderr = $5,
    duration_ms = $6,
    error_code = $7,
    error_message = $8,
    completed_at = now()
WHERE id = $1
RETURNING *;

-- name: LinkSandboxJobGenerationJob :one
UPDATE sandbox_job
SET generation_job_id = $2
WHERE id = $1
RETURNING *;

-- name: GetSandboxJobByID :one
SELECT *
FROM sandbox_job
WHERE id = $1;

-- name: ListSandboxJobsByWorkspace :many
SELECT *
FROM sandbox_job
WHERE workspace_id = $1
ORDER BY created_at;

-- name: ListSandboxJobsByTargetNode :many
SELECT *
FROM sandbox_job
WHERE target_node_id = $1
ORDER BY created_at;

-- name: ListSandboxJobsByGenerationJob :many
SELECT *
FROM sandbox_job
WHERE generation_job_id = $1
ORDER BY created_at;
