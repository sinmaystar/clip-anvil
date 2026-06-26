-- name: CreateProducerPendingSignal :one
INSERT INTO producer_pending_signal (
    workspace_id,
    producer_thread_id,
    source_role,
    source_task_id,
    source_thread_id,
    signal_type,
    scope_type,
    scope_id,
    render_plan_id,
    message_id,
    status,
    priority,
    dedupe_key,
    payload,
    last_error
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    'pending', $11, $12, $13, NULL
)
ON CONFLICT (workspace_id, dedupe_key) DO UPDATE
SET producer_thread_id = EXCLUDED.producer_thread_id,
    source_role = EXCLUDED.source_role,
    source_task_id = EXCLUDED.source_task_id,
    source_thread_id = EXCLUDED.source_thread_id,
    signal_type = EXCLUDED.signal_type,
    scope_type = EXCLUDED.scope_type,
    scope_id = EXCLUDED.scope_id,
    render_plan_id = EXCLUDED.render_plan_id,
    message_id = EXCLUDED.message_id,
    status = CASE
        WHEN producer_pending_signal.status IN ('processed', 'ignored') THEN producer_pending_signal.status
        ELSE 'pending'
    END,
    priority = EXCLUDED.priority,
    payload = EXCLUDED.payload,
    claimed_by_task_id = CASE
        WHEN producer_pending_signal.status IN ('processed', 'ignored') THEN producer_pending_signal.claimed_by_task_id
        ELSE NULL
    END,
    claimed_at = CASE
        WHEN producer_pending_signal.status IN ('processed', 'ignored') THEN producer_pending_signal.claimed_at
        ELSE NULL
    END,
    last_error = NULL,
    updated_at = now()
RETURNING *;

-- name: ClaimProducerPendingSignals :many
WITH picked AS (
    SELECT producer_pending_signal.id
    FROM producer_pending_signal
    WHERE producer_pending_signal.workspace_id = $1
      AND producer_pending_signal.producer_thread_id = $2
      AND (
        producer_pending_signal.status = 'pending'
        OR (
            producer_pending_signal.status = 'claimed'
            AND producer_pending_signal.claimed_at < now() - make_interval(secs => sqlc.arg(stale_after_seconds)::int)
        )
      )
    ORDER BY producer_pending_signal.priority ASC, producer_pending_signal.created_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(row_limit)
)
UPDATE producer_pending_signal
SET status = 'claimed',
    claimed_by_task_id = sqlc.arg(claimed_by_task_id),
    claimed_at = now(),
    last_error = NULL,
    updated_at = now()
WHERE id IN (SELECT id FROM picked)
RETURNING *;

-- name: ListClaimedProducerSignalsByTask :many
SELECT *
FROM producer_pending_signal
WHERE workspace_id = $1
  AND producer_thread_id = $2
  AND status = 'claimed'
  AND claimed_by_task_id = $3
ORDER BY priority ASC, created_at ASC;

-- name: MarkProducerPendingSignalProcessed :one
UPDATE producer_pending_signal
SET status = 'processed',
    processed_by_task_id = $3,
    processed_at = now(),
    last_error = NULL,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status IN ('pending', 'claimed', 'failed')
RETURNING *;

-- name: MarkProducerPendingSignalsProcessedByRenderPlan :many
UPDATE producer_pending_signal
SET status = 'processed',
    processed_by_task_id = $3,
    processed_at = now(),
    last_error = NULL,
    updated_at = now()
WHERE workspace_id = $1
  AND render_plan_id = $2
  AND signal_type = 'craftsman_render_plan_ready'
  AND status IN ('pending', 'claimed', 'failed')
RETURNING *;

-- name: MarkProducerPendingSignalIgnored :one
UPDATE producer_pending_signal
SET status = 'ignored',
    processed_by_task_id = $3,
    processed_at = now(),
    last_error = $4,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status IN ('pending', 'claimed', 'failed')
RETURNING *;

-- name: ReleaseProducerPendingSignalsForTask :many
UPDATE producer_pending_signal
SET status = 'pending',
    claimed_by_task_id = NULL,
    claimed_at = NULL,
    last_error = $3,
    updated_at = now()
WHERE workspace_id = $1
  AND claimed_by_task_id = $2
  AND status = 'claimed'
RETURNING *;

-- name: ListPendingProducerSignals :many
SELECT *
FROM producer_pending_signal
WHERE workspace_id = $1
  AND status IN ('pending', 'claimed', 'failed')
ORDER BY priority ASC, created_at ASC
LIMIT $2;
