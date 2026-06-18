-- name: CreateGenerationJob :one
INSERT INTO generation_job (
    workspace_id,
    target_node_id,
    parent_job_id,
    operation_type,
    provider,
    model_id,
    intent,
    rendered_prompt,
    provider_request,
    provider_response,
    status,
    progress,
    attempt,
    max_attempts,
    retry_policy,
    cost_cents,
    error_code,
    error_message,
    requested_by_type,
    requested_by_id,
    started_at,
    completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22
) RETURNING *;

-- name: NextArtifactVersionNo :one
SELECT COALESCE(MAX(version_no), 0)::int + 1 AS version_no
FROM artifact_version
WHERE node_id = $1;

-- name: ClearArtifactWinnersForNode :exec
UPDATE artifact_version
SET winner = false
WHERE node_id = $1
  AND winner = true;

-- name: CreateArtifactVersion :one
INSERT INTO artifact_version (
    workspace_id,
    node_id,
    job_id,
    asset_id,
    version_no,
    winner,
    output,
    review_score,
    input_hash
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: ListArtifactVersionsByNode :many
SELECT *
FROM artifact_version
WHERE node_id = $1
ORDER BY version_no;

-- name: ListGenerationJobsByNode :many
SELECT *
FROM generation_job
WHERE target_node_id = $1
ORDER BY created_at;

-- name: ListGenerationJobsByParent :many
SELECT *
FROM generation_job
WHERE parent_job_id = $1
ORDER BY attempt;

-- name: LatestGenerationJobInChain :one
WITH RECURSIVE chain AS (
    SELECT generation_job.*
    FROM generation_job
    WHERE generation_job.id = $1
    UNION ALL
    SELECT child.*
    FROM generation_job child
    JOIN chain parent ON child.parent_job_id = parent.id
)
SELECT *
FROM chain
ORDER BY attempt DESC, created_at DESC
LIMIT 1;

-- name: GetGenerationJobByID :one
SELECT *
FROM generation_job
WHERE id = $1;

-- name: LatestGenerationJobByNode :one
SELECT *
FROM generation_job
WHERE target_node_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: GetArtifactVersionByID :one
SELECT *
FROM artifact_version
WHERE id = $1;

-- name: GetCurrentArtifactVersionForNode :one
SELECT artifact_version.*
FROM artifact_version
JOIN media_node ON media_node.current_version_id = artifact_version.id
WHERE media_node.id = $1;

-- name: UpsertNodeStaleReason :one
INSERT INTO node_stale_reason (
    workspace_id,
    node_id,
    upstream_node_id,
    upstream_version_id,
    reason_code,
    reason_message,
    details
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (node_id, upstream_node_id, reason_code)
WHERE resolved_at IS NULL
DO UPDATE SET
    upstream_version_id = EXCLUDED.upstream_version_id,
    reason_message = EXCLUDED.reason_message,
    details = EXCLUDED.details,
    created_at = now()
RETURNING *;

-- name: ListActiveStaleReasonsByNode :many
SELECT *
FROM node_stale_reason
WHERE node_id = $1
  AND resolved_at IS NULL
ORDER BY created_at;

-- name: ResolveActiveStaleReasonsByNode :exec
UPDATE node_stale_reason
SET resolved_at = now()
WHERE node_id = $1
  AND resolved_at IS NULL;
