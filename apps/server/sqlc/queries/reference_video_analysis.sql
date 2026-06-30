-- name: CreateReferenceVideoAnalysis :one
INSERT INTO reference_video_analysis (
    workspace_id,
    source_node_id,
    status,
    brief,
    focus,
    model_provider,
    model_id,
    request_summary,
    result,
    created_by_thread_id,
    created_by_task_id
) VALUES (
    sqlc.arg(workspace_id),
    sqlc.arg(source_node_id),
    sqlc.arg(status),
    sqlc.arg(brief),
    sqlc.arg(focus),
    sqlc.arg(model_provider),
    sqlc.arg(model_id),
    sqlc.arg(request_summary),
    sqlc.arg(result),
    sqlc.narg(created_by_thread_id),
    sqlc.narg(created_by_task_id)
)
RETURNING *;

-- name: GetReferenceVideoAnalysisByID :one
SELECT * FROM reference_video_analysis
WHERE id = sqlc.arg(id);

-- name: ListReferenceVideoAnalysesByWorkspace :many
SELECT * FROM reference_video_analysis
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: ListReferenceVideoAnalysesBySourceNode :many
SELECT * FROM reference_video_analysis
WHERE workspace_id = sqlc.arg(workspace_id)
  AND source_node_id = sqlc.arg(source_node_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: MarkReferenceVideoAnalysisRunning :one
UPDATE reference_video_analysis
SET status = 'running',
    request_summary = sqlc.arg(request_summary),
    model_provider = sqlc.arg(model_provider),
    model_id = sqlc.arg(model_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: MarkReferenceVideoAnalysisSucceeded :one
UPDATE reference_video_analysis
SET status = 'succeeded',
    request_summary = sqlc.arg(request_summary),
    result = sqlc.arg(result),
    error_code = '',
    error_message = '',
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: MarkReferenceVideoAnalysisFailed :one
UPDATE reference_video_analysis
SET status = 'failed',
    request_summary = sqlc.arg(request_summary),
    error_code = sqlc.arg(error_code),
    error_message = sqlc.arg(error_message),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;
