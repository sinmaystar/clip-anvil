-- name: CreateRenderPlan :one
INSERT INTO render_plan (
    workspace_id, scope_type, scope_id, target_phase, task_type,
    model_prompt_profile, operation, status, revision, forked_from_render_plan_id,
    render_plan_key, reference_bindings, subject_bindings, prompt_parts, params,
    audit_hints, blocker, rationale, created_by_thread_id, created_by_task_id
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15,
    $16, $17, $18, $19, $20
) RETURNING *;

-- name: GetRenderPlanByID :one
SELECT * FROM render_plan
WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL;

-- name: ListRenderPlansByWorkspace :many
SELECT * FROM render_plan
WHERE workspace_id = $1 AND archived_at IS NULL
ORDER BY updated_at DESC, created_at DESC;

-- name: ListRenderPlansByScope :many
SELECT * FROM render_plan
WHERE workspace_id = $1
  AND scope_type = $2
  AND scope_id = $3
  AND archived_at IS NULL
ORDER BY target_phase ASC, revision DESC;

-- name: GetLatestRenderPlanByScopePhase :one
SELECT * FROM render_plan
WHERE workspace_id = $1
  AND scope_type = $2
  AND scope_id = $3
  AND target_phase = $4
  AND archived_at IS NULL
ORDER BY revision DESC
LIMIT 1;

-- name: UpdateRenderPlanDraft :one
UPDATE render_plan
SET task_type = $3,
    model_prompt_profile = $4,
    operation = $5,
    reference_bindings = $6,
    subject_bindings = $7,
    prompt_parts = $8,
    params = $9,
    audit_hints = $10,
    blocker = $11,
    rationale = $12,
    status = $13,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status IN ('draft', 'blocked')
  AND archived_at IS NULL
RETURNING *;

-- name: MarkRenderPlanCompiled :one
UPDATE render_plan
SET status = 'compiled',
    compiled_prompt = $3,
    compiled_request = $4,
    prompt_audit = $5,
    cost_estimate = $6,
    compiled_at = now(),
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status = 'draft'
  AND archived_at IS NULL
RETURNING *;

-- name: MarkRenderPlanBlocked :one
UPDATE render_plan
SET status = 'blocked',
    blocker = $3,
    audit_hints = $4,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status IN ('draft', 'blocked')
  AND archived_at IS NULL
RETURNING *;

-- name: MarkRenderPlanWaitingForApproval :one
UPDATE render_plan
SET status = 'waiting_for_approval',
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status = 'compiled'
  AND archived_at IS NULL
RETURNING *;

-- name: MarkRenderPlanSubmitted :one
UPDATE render_plan
SET status = 'submitted',
    submitted_worker_task_id = $3,
    output_node_id = $4,
    submitted_at = now(),
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status IN ('compiled', 'waiting_for_approval')
  AND archived_at IS NULL
RETURNING *;

-- name: MarkRenderPlanCompleted :one
UPDATE render_plan
SET status = $3,
    output_version_id = $4,
    output_node_id = $5,
    completed_at = now(),
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status IN ('submitted', 'running')
  AND archived_at IS NULL
RETURNING *;

-- name: MarkRenderPlanRejected :one
UPDATE render_plan
SET status = 'rejected',
    blocker = $3,
    audit_hints = $4,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status IN ('compiled', 'waiting_for_approval')
  AND archived_at IS NULL
RETURNING *;

-- name: NextRenderPlanRevision :one
SELECT COALESCE(MAX(revision), 0)::int + 1 AS next_revision
FROM render_plan
WHERE workspace_id = $1
  AND scope_type = $2
  AND scope_id = $3
  AND target_phase = $4
  AND archived_at IS NULL;
