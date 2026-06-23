-- name: CreateShotDependency :one
INSERT INTO shot_dependency (
    workspace_id,
    from_shot_id,
    to_shot_id,
    dependency_type,
    required_artifact,
    injection_role,
    blocking_phase,
    stale_policy,
    reason
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: ListShotDependenciesByWorkspace :many
SELECT *
FROM shot_dependency
WHERE workspace_id = $1
ORDER BY created_at;

-- name: DeleteShotDependenciesByWorkspace :exec
DELETE FROM shot_dependency
WHERE workspace_id = $1;

-- name: DeleteShotDependenciesForShot :exec
DELETE FROM shot_dependency
WHERE workspace_id = $1
  AND (from_shot_id = $2 OR to_shot_id = $2);
