-- name: CreateRemotionRendererArtifact :one
INSERT INTO remotion_renderer_artifact (
    workspace_id,
    timeline_plan_id,
    status,
    route_policy,
    summary,
    created_by_role,
    created_by_task_id
) VALUES (
    sqlc.arg(workspace_id),
    sqlc.arg(timeline_plan_id),
    sqlc.arg(status),
    sqlc.arg(route_policy),
    sqlc.arg(summary),
    sqlc.arg(created_by_role),
    sqlc.narg(created_by_task_id)
)
RETURNING *;

-- name: GetRemotionRendererArtifact :one
SELECT * FROM remotion_renderer_artifact
WHERE id = sqlc.arg(id);

-- name: GetRemotionRendererArtifactByTimelinePlan :one
SELECT * FROM remotion_renderer_artifact
WHERE timeline_plan_id = sqlc.arg(timeline_plan_id);

-- name: ListRemotionRendererArtifactsByWorkspace :many
SELECT * FROM remotion_renderer_artifact
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: UpdateRemotionRendererArtifactStatus :one
UPDATE remotion_renderer_artifact
SET
    status = sqlc.arg(status),
    route_policy = COALESCE(sqlc.narg(route_policy), route_policy),
    summary = COALESCE(sqlc.narg(summary), summary),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetCurrentRemotionRendererAttempt :one
UPDATE remotion_renderer_artifact
SET
    current_attempt_id = sqlc.arg(current_attempt_id),
    status = sqlc.arg(status),
    updated_at = now()
WHERE remotion_renderer_artifact.id = sqlc.arg(id)
  AND EXISTS (
    SELECT 1
    FROM remotion_renderer_attempt
    WHERE remotion_renderer_attempt.id = sqlc.arg(current_attempt_id)
      AND remotion_renderer_attempt.renderer_artifact_id = remotion_renderer_artifact.id
  )
RETURNING *;

-- name: CreateRemotionRendererAttempt :one
INSERT INTO remotion_renderer_attempt (
    workspace_id,
    timeline_plan_id,
    renderer_artifact_id,
    attempt_no,
    status,
    source_snapshot,
    props_json,
    source_hash,
    props_hash,
    workspace_dir,
    validation_result,
    compile_result,
    render_result,
    qa_result,
    repair_from_attempt_id,
    repair_notes
) VALUES (
    sqlc.arg(workspace_id),
    sqlc.arg(timeline_plan_id),
    sqlc.arg(renderer_artifact_id),
    sqlc.arg(attempt_no),
    sqlc.arg(status),
    sqlc.arg(source_snapshot),
    sqlc.arg(props_json),
    sqlc.arg(source_hash),
    sqlc.arg(props_hash),
    sqlc.arg(workspace_dir),
    sqlc.arg(validation_result),
    sqlc.arg(compile_result),
    sqlc.arg(render_result),
    sqlc.arg(qa_result),
    sqlc.narg(repair_from_attempt_id),
    sqlc.arg(repair_notes)
)
RETURNING *;

-- name: GetRemotionRendererAttempt :one
SELECT * FROM remotion_renderer_attempt
WHERE id = sqlc.arg(id);

-- name: GetCurrentRemotionRendererAttempt :one
SELECT remotion_renderer_attempt.*
FROM remotion_renderer_attempt
JOIN remotion_renderer_artifact
  ON remotion_renderer_artifact.current_attempt_id = remotion_renderer_attempt.id
WHERE remotion_renderer_artifact.id = sqlc.arg(renderer_artifact_id);

-- name: ListRemotionRendererAttemptsByArtifact :many
SELECT * FROM remotion_renderer_attempt
WHERE renderer_artifact_id = sqlc.arg(renderer_artifact_id)
ORDER BY attempt_no ASC, created_at ASC;

-- name: GetLatestRemotionRendererAttemptByArtifact :one
SELECT * FROM remotion_renderer_attempt
WHERE renderer_artifact_id = sqlc.arg(renderer_artifact_id)
ORDER BY attempt_no DESC, created_at DESC
LIMIT 1;

-- name: UpdateRemotionRendererAttemptSnapshot :one
UPDATE remotion_renderer_attempt
SET
    status = sqlc.arg(status),
    source_snapshot = sqlc.arg(source_snapshot),
    props_json = sqlc.arg(props_json),
    source_hash = sqlc.arg(source_hash),
    props_hash = sqlc.arg(props_hash),
    workspace_dir = sqlc.arg(workspace_dir),
    validation_result = sqlc.arg(validation_result),
    compile_result = sqlc.arg(compile_result),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateRemotionRendererAttemptRenderResult :one
UPDATE remotion_renderer_attempt
SET
    status = sqlc.arg(status),
    render_result = sqlc.arg(render_result),
    sandbox_job_id = sqlc.narg(sandbox_job_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateRemotionRendererAttemptQAResult :one
UPDATE remotion_renderer_attempt
SET
    status = sqlc.arg(status),
    qa_result = sqlc.arg(qa_result),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;
