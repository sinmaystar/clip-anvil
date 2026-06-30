-- name: CreateAgentContextCompaction :one
INSERT INTO agent_context_compaction (
    workspace_id,
    thread_id,
    task_id,
    role,
    mode,
    trigger,
    semantic_key,
    source_seq_start,
    source_seq_end,
    source_message_ids,
    source_media_refs,
    original_token_estimate,
    compacted_token_estimate,
    original_bytes,
    summary,
    detail_files,
    payload
) VALUES (
    sqlc.arg(workspace_id),
    sqlc.narg(thread_id),
    sqlc.narg(task_id),
    sqlc.arg(role),
    sqlc.arg(mode),
    sqlc.arg(trigger),
    sqlc.arg(semantic_key),
    sqlc.arg(source_seq_start),
    sqlc.arg(source_seq_end),
    sqlc.arg(source_message_ids),
    sqlc.arg(source_media_refs),
    sqlc.arg(original_token_estimate),
    sqlc.arg(compacted_token_estimate),
    sqlc.arg(original_bytes),
    sqlc.arg(summary),
    sqlc.arg(detail_files),
    sqlc.arg(payload)
) RETURNING *;

-- name: LinkAgentMessageCompaction :one
INSERT INTO agent_message_compaction (
    message_id,
    compaction_id,
    compacted_role
) VALUES (
    sqlc.arg(message_id),
    sqlc.arg(compaction_id),
    sqlc.arg(compacted_role)
)
ON CONFLICT (message_id) DO UPDATE
SET compaction_id = EXCLUDED.compaction_id,
    compacted_role = EXCLUDED.compacted_role,
    compacted_at = now()
RETURNING *;

-- name: GetAgentContextCompactionBySemanticKey :one
SELECT *
FROM agent_context_compaction
WHERE workspace_id = sqlc.arg(workspace_id)
  AND semantic_key = sqlc.arg(semantic_key)
LIMIT 1;

-- name: ListAgentContextCompactionsByThread :many
SELECT *
FROM agent_context_compaction
WHERE workspace_id = sqlc.arg(workspace_id)
  AND thread_id = sqlc.arg(thread_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(row_limit);

-- name: ListAgentContextCompactionsByWorkspace :many
SELECT *
FROM agent_context_compaction
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(row_limit);

-- name: ListCompactedMessageIDsByThread :many
SELECT
    agent_message_compaction.message_id,
    agent_message_compaction.compaction_id,
    agent_message_compaction.compacted_role,
    agent_message_compaction.compacted_at,
    agent_context_compaction.semantic_key,
    agent_context_compaction.summary,
    agent_context_compaction.detail_files
FROM agent_message_compaction
JOIN agent_message ON agent_message.id = agent_message_compaction.message_id
JOIN agent_context_compaction ON agent_context_compaction.id = agent_message_compaction.compaction_id
WHERE agent_message.workspace_id = sqlc.arg(workspace_id)
  AND agent_message.thread_id = sqlc.arg(thread_id);

-- name: SearchAgentContextCompactions :many
SELECT *
FROM agent_context_compaction
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(thread_id)::uuid IS NULL OR thread_id = sqlc.narg(thread_id))
  AND (
      semantic_key = sqlc.arg(query)::text
      OR summary ILIKE '%' || sqlc.arg(query)::text || '%'
      OR payload::text ILIKE '%' || sqlc.arg(query)::text || '%'
      OR source_media_refs::text ILIKE '%' || sqlc.arg(query)::text || '%'
  )
ORDER BY created_at DESC
LIMIT sqlc.arg(row_limit);
