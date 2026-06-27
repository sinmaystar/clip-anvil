-- name: GetAgentObjectBySemanticKey :one
SELECT workspace_id, object_type, object_id, semantic_key, display_name, parent_object_type, parent_object_id, parent_semantic_key, status, kind, sort_order, updated_at
FROM agent_object_index
WHERE workspace_id = $1
  AND object_type = $2
  AND semantic_key = $3;

-- name: ListAgentObjectsByWorkspace :many
SELECT workspace_id, object_type, object_id, semantic_key, display_name, parent_object_type, parent_object_id, parent_semantic_key, status, kind, sort_order, updated_at
FROM agent_object_index
WHERE workspace_id = $1
ORDER BY object_type, sort_order, semantic_key;

-- name: ListAgentObjectsByType :many
SELECT workspace_id, object_type, object_id, semantic_key, display_name, parent_object_type, parent_object_id, parent_semantic_key, status, kind, sort_order, updated_at
FROM agent_object_index
WHERE workspace_id = $1
  AND object_type = $2
ORDER BY sort_order, semantic_key;

-- name: ListAgentObjectsByParentSemanticKey :many
SELECT workspace_id, object_type, object_id, semantic_key, display_name, parent_object_type, parent_object_id, parent_semantic_key, status, kind, sort_order, updated_at
FROM agent_object_index
WHERE workspace_id = $1
  AND parent_semantic_key = $2
ORDER BY object_type, sort_order, semantic_key;

-- name: GetCurrentArtifactVersionByShotKeyAndKind :one
SELECT artifact_version.*
FROM artifact_version
JOIN media_node ON media_node.current_version_id = artifact_version.id
JOIN render_plan ON render_plan.id = media_node.source_render_plan_id
JOIN shot ON render_plan.scope_type = 'shot' AND render_plan.scope_id = shot.id
WHERE artifact_version.workspace_id = $1
  AND shot.semantic_key = $2
  AND media_node.artifact_kind = $3
LIMIT 1;

-- name: GetLatestArtifactVersionByShotKeyAndKind :one
SELECT artifact_version.*
FROM artifact_version
JOIN media_node ON media_node.id = artifact_version.node_id
JOIN render_plan ON render_plan.id = media_node.source_render_plan_id
JOIN shot ON render_plan.scope_type = 'shot' AND render_plan.scope_id = shot.id
WHERE artifact_version.workspace_id = $1
  AND shot.semantic_key = $2
  AND media_node.artifact_kind = $3
ORDER BY artifact_version.created_at DESC
LIMIT 1;

-- name: GetWinnerArtifactVersionByShotKeyAndKind :one
SELECT artifact_version.*
FROM artifact_version
JOIN media_node ON media_node.id = artifact_version.node_id
JOIN render_plan ON render_plan.id = media_node.source_render_plan_id
JOIN shot ON render_plan.scope_type = 'shot' AND render_plan.scope_id = shot.id
WHERE artifact_version.workspace_id = $1
  AND shot.semantic_key = $2
  AND media_node.artifact_kind = $3
  AND artifact_version.winner = true
ORDER BY artifact_version.created_at DESC
LIMIT 1;

-- name: GetArtifactVersionBySemanticKey :one
SELECT *
FROM artifact_version
WHERE workspace_id = $1
  AND semantic_key = $2;

-- name: GetRenderPlanBySemanticKey :one
SELECT *
FROM render_plan
WHERE workspace_id = $1
  AND semantic_key = $2
  AND archived_at IS NULL;

-- name: GetMediaNodeBySemanticKey :one
SELECT *
FROM media_node
WHERE workspace_id = $1
  AND semantic_key = $2;
