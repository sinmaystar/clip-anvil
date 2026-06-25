-- name: GetActiveCreativeBriefByWorkspace :one
SELECT *
FROM creative_brief
WHERE workspace_id = $1
  AND archived_at IS NULL
  AND status <> 'archived'
ORDER BY updated_at DESC
LIMIT 1;

-- name: GetCreativeBriefByID :one
SELECT *
FROM creative_brief
WHERE id = $1
  AND workspace_id = $2;

-- name: CreateCreativeBrief :one
INSERT INTO creative_brief (
    workspace_id,
    title,
    video_type,
    target_audience,
    tone,
    visual_style,
    duration_sec,
    aspect_ratio,
    language,
    objective,
    concept,
    constraints,
    metadata,
    status,
    created_by_thread_id,
    created_by_task_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14, $15, $16
) RETURNING *;

-- name: UpdateCreativeBrief :one
UPDATE creative_brief
SET title = $3,
    video_type = $4,
    target_audience = $5,
    tone = $6,
    visual_style = $7,
    duration_sec = $8,
    aspect_ratio = $9,
    language = $10,
    objective = $11,
    concept = $12,
    constraints = $13,
    metadata = $14,
    status = $15,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL
RETURNING *;

-- name: ArchiveCreativeBrief :one
UPDATE creative_brief
SET status = 'archived',
    archived_at = now(),
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL
RETURNING *;

-- name: GetActiveProjectMemoryByWorkspace :one
SELECT *
FROM project_memory
WHERE workspace_id = $1
  AND status = 'active'
ORDER BY version DESC
LIMIT 1;

-- name: ListProjectMemoriesByWorkspace :many
SELECT *
FROM project_memory
WHERE workspace_id = $1
ORDER BY version DESC;

-- name: ArchiveActiveProjectMemoryByWorkspace :exec
UPDATE project_memory
SET status = 'archived'
WHERE workspace_id = $1
  AND status = 'active';

-- name: CreateProjectMemory :one
INSERT INTO project_memory (
    workspace_id,
    version,
    status,
    core_intent,
    soul,
    brand_facts,
    non_negotiables,
    visual_anchors,
    allowed,
    forbidden,
    prompt_injection_hints,
    source_refs,
    created_by_thread_id,
    created_by_task_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14
) RETURNING *;

-- name: GetKeyElementByClientKey :one
SELECT *
FROM key_element
WHERE workspace_id = $1
  AND client_key = $2
  AND archived_at IS NULL;

-- name: ListActiveKeyElementsByWorkspace :many
SELECT *
FROM key_element
WHERE workspace_id = $1
  AND archived_at IS NULL
ORDER BY element_type, name, created_at;

-- name: CreateKeyElement :one
INSERT INTO key_element (
    workspace_id,
    client_key,
    element_type,
    name,
    description,
    source_type,
    source_refs,
    status,
    created_by_thread_id,
    created_by_task_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: UpdateKeyElement :one
UPDATE key_element
SET element_type = $3,
    name = $4,
    description = $5,
    source_type = $6,
    source_refs = $7,
    status = $8,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL
RETURNING *;

-- name: ArchiveKeyElement :one
UPDATE key_element
SET status = 'archived',
    archived_at = now(),
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL
RETURNING *;

-- name: GetKeyElementStateByClientKey :one
SELECT *
FROM key_element_state
WHERE key_element_id = $1
  AND client_key = $2
  AND archived_at IS NULL;

-- name: GetDefaultKeyElementState :one
SELECT *
FROM key_element_state
WHERE key_element_id = $1
  AND is_default
  AND archived_at IS NULL
LIMIT 1;

-- name: ListActiveKeyElementStatesByWorkspace :many
SELECT *
FROM key_element_state
WHERE workspace_id = $1
  AND archived_at IS NULL
ORDER BY created_at;

-- name: ClearDefaultKeyElementState :exec
UPDATE key_element_state
SET is_default = false,
    updated_at = now()
WHERE key_element_id = $1
  AND archived_at IS NULL;

-- name: CreateKeyElementState :one
INSERT INTO key_element_state (
    workspace_id,
    key_element_id,
    client_key,
    label,
    visual_description,
    reference_status,
    reference_node_id,
    reference_version_id,
    is_default,
    state_facts,
    source_refs,
    status,
    created_by_thread_id,
    created_by_task_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14
) RETURNING *;

-- name: UpdateKeyElementState :one
UPDATE key_element_state
SET label = $3,
    visual_description = $4,
    reference_status = $5,
    reference_node_id = $6,
    reference_version_id = $7,
    is_default = $8,
    state_facts = $9,
    source_refs = $10,
    status = $11,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL
RETURNING *;

-- name: GetSceneByClientKey :one
SELECT *
FROM scene
WHERE workspace_id = $1
  AND client_key = $2
  AND archived_at IS NULL;

-- name: ListActiveScenesByWorkspace :many
SELECT *
FROM scene
WHERE workspace_id = $1
  AND archived_at IS NULL
ORDER BY sort_order, created_at;

-- name: CreateScene :one
INSERT INTO scene (
    workspace_id,
    client_key,
    sort_order,
    title,
    description,
    location,
    mood,
    status,
    created_by_thread_id,
    created_by_task_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: UpdateScene :one
UPDATE scene
SET client_key = $3,
    sort_order = $4,
    title = $5,
    description = $6,
    location = $7,
    mood = $8,
    status = $9,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL
RETURNING *;

-- name: ArchiveScene :one
UPDATE scene
SET status = 'archived',
    archived_at = now(),
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL
RETURNING *;

-- name: DeleteShotKeyElementsByWorkspace :exec
DELETE FROM shot_key_element
WHERE workspace_id = $1;

-- name: DeleteShotKeyElementsByShot :exec
DELETE FROM shot_key_element
WHERE workspace_id = $1
  AND shot_id = $2;

-- name: CreateShotKeyElement :one
INSERT INTO shot_key_element (
    workspace_id,
    shot_id,
    key_element_id,
    key_element_state_id,
    role,
    required,
    sort_order
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: ListShotKeyElementsByWorkspace :many
SELECT *
FROM shot_key_element
WHERE workspace_id = $1
ORDER BY shot_id, sort_order, created_at;
