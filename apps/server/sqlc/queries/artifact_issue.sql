-- name: CreateArtifactIssue :one
INSERT INTO artifact_issue (
    workspace_id,
    review_record_id,
    dimension,
    severity,
    status,
    target_object_type,
    target_object_id,
    title,
    description,
    evidence,
    suggested_fix,
    fix_hint,
    requires_user_confirmation,
    semantic_key,
    display_name
) VALUES (
    $1, $2, $3, $4, 'open',
    $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
) RETURNING *;

-- name: ListArtifactIssuesByWorkspace :many
SELECT *
FROM artifact_issue
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListOpenArtifactIssuesByWorkspace :many
SELECT *
FROM artifact_issue
WHERE workspace_id = $1
  AND status = 'open'
ORDER BY created_at DESC
LIMIT $2;

-- name: ListArtifactIssuesByReviewRecord :many
SELECT *
FROM artifact_issue
WHERE review_record_id = $1
ORDER BY created_at ASC;

-- name: ListArtifactIssuesByTarget :many
SELECT *
FROM artifact_issue
WHERE target_object_type = $1
  AND target_object_id = $2
ORDER BY created_at DESC;

-- name: MarkArtifactIssueResolved :one
UPDATE artifact_issue
SET status = 'resolved',
    resolved_by_review_record_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;
