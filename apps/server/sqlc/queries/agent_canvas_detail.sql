-- name: GetAgentCanvasSceneByID :one
SELECT *
FROM scene
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL;

-- name: GetAgentCanvasKeyElementByID :one
SELECT *
FROM key_element
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL;

-- name: GetAgentCanvasArtifactIssueByID :one
SELECT *
FROM artifact_issue
WHERE id = $1
  AND workspace_id = $2;
