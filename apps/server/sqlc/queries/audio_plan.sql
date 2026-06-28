-- name: CreateAudioPlan :one
INSERT INTO audio_plan (
    workspace_id,
    status,
    title,
    plan_kind,
    language,
    target_duration_sec,
    voiceover_script,
    voice_profile,
    bgm_plan,
    cue_plan,
    generation_params,
    created_by_task_id,
    semantic_key,
    display_name
) VALUES (
    sqlc.arg(workspace_id),
    sqlc.arg(status),
    sqlc.arg(title),
    sqlc.arg(plan_kind),
    sqlc.arg(language),
    sqlc.narg(target_duration_sec),
    sqlc.arg(voiceover_script),
    sqlc.arg(voice_profile),
    sqlc.arg(bgm_plan),
    sqlc.arg(cue_plan),
    sqlc.arg(generation_params),
    sqlc.narg(created_by_task_id),
    sqlc.arg(semantic_key),
    sqlc.arg(display_name)
)
RETURNING *;

-- name: GetAudioPlan :one
SELECT *
FROM audio_plan
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id);

-- name: GetActiveAudioPlanByWorkspace :one
SELECT *
FROM audio_plan
WHERE workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
  AND status IN ('draft', 'waiting_for_user', 'approved', 'generating', 'voiceover_ready', 'composing', 'blocked', 'failed')
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: ListAudioPlansByWorkspace :many
SELECT *
FROM audio_plan
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: UpdateAudioPlan :one
UPDATE audio_plan
SET
    status = sqlc.arg(status),
    title = sqlc.arg(title),
    language = sqlc.arg(language),
    target_duration_sec = sqlc.narg(target_duration_sec),
    voiceover_script = sqlc.arg(voiceover_script),
    voice_profile = sqlc.arg(voice_profile),
    bgm_plan = sqlc.arg(bgm_plan),
    cue_plan = sqlc.arg(cue_plan),
    generation_params = sqlc.arg(generation_params),
    display_name = sqlc.arg(display_name),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
RETURNING *;

-- name: UpdateAudioPlanStatus :one
UPDATE audio_plan
SET
    status = sqlc.arg(status),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
RETURNING *;

-- name: SetAudioPlanVoiceoverRenderPlan :one
UPDATE audio_plan
SET
    voiceover_render_plan_id = sqlc.arg(voiceover_render_plan_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
RETURNING *;

-- name: SetAudioPlanBGMRenderPlan :one
UPDATE audio_plan
SET
    bgm_render_plan_id = sqlc.arg(bgm_render_plan_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
RETURNING *;

-- name: SetAudioPlanVoiceoverNode :one
UPDATE audio_plan
SET
    voiceover_node_id = sqlc.arg(voiceover_node_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
RETURNING *;

-- name: SetAudioPlanBGMNode :one
UPDATE audio_plan
SET
    bgm_node_id = sqlc.arg(bgm_node_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
RETURNING *;

-- name: ArchiveActiveAudioPlansByWorkspace :exec
UPDATE audio_plan
SET
    status = 'archived',
    archived_at = now(),
    updated_at = now()
WHERE workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
  AND status IN ('draft', 'waiting_for_user', 'approved', 'generating', 'voiceover_ready', 'composing', 'blocked', 'failed');
