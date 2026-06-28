-- name: CreateReviewRecord :one
INSERT INTO review_record (
    workspace_id,
    shot_id,
    node_id,
    artifact_version_id,
    generation_job_id,
    reviewer_thread_id,
    reviewer_task_id,
    parent_review_record_id,
    target_phase,
    review_task,
    target_object_type,
    target_object_id,
    render_plan_id,
    status,
    attempt_no,
    max_attempts,
    model_provider,
    model_id,
    required_axes,
    semantic_key,
    display_name
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10, $11, $12, $13,
    'running', $14, $15, $16, $17, $18, $19, $20
) RETURNING *;

-- name: CompleteReviewRecord :one
UPDATE review_record
SET status = $2,
    overall_score = $3,
    rubric = $4,
    critique = $5,
    retry_recommendation = $6,
    escalation = $7,
    error_code = '',
    error_message = '',
    completed_at = now()
WHERE id = $1
RETURNING *;

-- name: FailReviewRecord :one
UPDATE review_record
SET status = 'failed',
    error_code = $2,
    error_message = $3,
    completed_at = now()
WHERE id = $1
RETURNING *;

-- name: GetReviewRecordByID :one
SELECT *
FROM review_record
WHERE id = $1;

-- name: ListReviewRecordsByWorkspace :many
SELECT *
FROM review_record
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListReviewRecordsByShotPhase :many
SELECT *
FROM review_record
WHERE workspace_id = $1
  AND shot_id = $2
  AND target_phase = $3
ORDER BY created_at DESC;

-- name: ListReviewRecordsByNode :many
SELECT *
FROM review_record
WHERE node_id = $1
ORDER BY created_at DESC;

-- name: ListReviewRecordsByArtifactVersion :many
SELECT *
FROM review_record
WHERE artifact_version_id = $1
ORDER BY created_at DESC;

-- name: ListReviewRecordsByTarget :many
SELECT *
FROM review_record
WHERE workspace_id = $1
  AND target_object_type = $2
  AND target_object_id = $3
ORDER BY created_at DESC
LIMIT $4;

-- name: ListReviewRecordsByRenderPlan :many
SELECT *
FROM review_record
WHERE render_plan_id = $1
ORDER BY created_at DESC;

-- name: CountReviewAttemptsByShotPhase :one
SELECT COUNT(*)::int
FROM review_record
WHERE workspace_id = $1
  AND shot_id = $2
  AND target_phase = $3
  AND status IN ('accepted', 'rejected', 'failed');
