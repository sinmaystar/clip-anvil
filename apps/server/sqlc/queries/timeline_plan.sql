-- name: CreateTimelinePlan :one
INSERT INTO timeline_plan (
    workspace_id,
    source_storyboard_node_id,
    output_node_id,
    production_job_id,
    artifact_version_id,
    sandbox_job_id,
    status,
    template_key,
    plan_json,
    render_settings,
    result,
    error_message,
    created_by_role,
    created_by_task_id
) VALUES (
    sqlc.arg(workspace_id),
    sqlc.narg(source_storyboard_node_id),
    sqlc.narg(output_node_id),
    sqlc.narg(production_job_id),
    sqlc.narg(artifact_version_id),
    sqlc.narg(sandbox_job_id),
    sqlc.arg(status),
    sqlc.arg(template_key),
    sqlc.arg(plan_json),
    sqlc.arg(render_settings),
    sqlc.arg(result),
    sqlc.narg(error_message),
    sqlc.arg(created_by_role),
    sqlc.narg(created_by_task_id)
)
RETURNING *;

-- name: GetTimelinePlan :one
SELECT * FROM timeline_plan
WHERE id = sqlc.arg(id);

-- name: ListTimelinePlansByWorkspace :many
SELECT * FROM timeline_plan
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: GetLatestCompletedTimelinePlanByWorkspace :one
SELECT * FROM timeline_plan
WHERE workspace_id = sqlc.arg(workspace_id)
  AND status = 'completed'
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: GetLatestTimelinePlanByWorkspace :one
SELECT * FROM timeline_plan
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: UpdateTimelinePlanStatus :one
UPDATE timeline_plan
SET
    status = sqlc.arg(status),
    output_node_id = COALESCE(sqlc.narg(output_node_id), output_node_id),
    production_job_id = COALESCE(sqlc.narg(production_job_id), production_job_id),
    artifact_version_id = COALESCE(sqlc.narg(artifact_version_id), artifact_version_id),
    sandbox_job_id = COALESCE(sqlc.narg(sandbox_job_id), sandbox_job_id),
    result = COALESCE(sqlc.narg(result), result),
    error_message = sqlc.narg(error_message),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;
