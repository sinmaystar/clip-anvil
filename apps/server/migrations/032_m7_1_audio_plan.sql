-- +goose Up
CREATE TABLE audio_plan (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'draft',
    title TEXT NOT NULL DEFAULT '',
    plan_kind TEXT NOT NULL DEFAULT 'marketing_voiceover_bgm',
    language TEXT NOT NULL DEFAULT 'zh',
    target_duration_sec DOUBLE PRECISION,
    voiceover_script TEXT NOT NULL DEFAULT '',
    voice_profile JSONB NOT NULL DEFAULT '{}',
    bgm_plan JSONB NOT NULL DEFAULT '{}',
    cue_plan JSONB NOT NULL DEFAULT '[]',
    generation_params JSONB NOT NULL DEFAULT '{}',
    voiceover_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    bgm_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    voiceover_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    bgm_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    timeline_plan_id UUID REFERENCES timeline_plan(id) ON DELETE SET NULL,
    created_by_role TEXT NOT NULL DEFAULT 'producer',
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT audio_plan_status_check CHECK (status IN ('draft', 'waiting_for_user', 'approved', 'generating', 'voiceover_ready', 'composing', 'completed', 'blocked', 'failed', 'archived')),
    CONSTRAINT audio_plan_kind_check CHECK (plan_kind IN ('marketing_voiceover_bgm')),
    CONSTRAINT audio_plan_duration_positive CHECK (target_duration_sec IS NULL OR target_duration_sec > 0)
);

CREATE UNIQUE INDEX idx_audio_plan_workspace_active
    ON audio_plan(workspace_id)
    WHERE archived_at IS NULL
      AND status IN ('draft', 'waiting_for_user', 'approved', 'generating', 'voiceover_ready', 'composing', 'blocked', 'failed');

CREATE INDEX idx_audio_plan_workspace_updated
    ON audio_plan(workspace_id, updated_at DESC);

CREATE INDEX idx_audio_plan_voiceover_render_plan
    ON audio_plan(voiceover_render_plan_id)
    WHERE voiceover_render_plan_id IS NOT NULL;

CREATE INDEX idx_audio_plan_bgm_render_plan
    ON audio_plan(bgm_render_plan_id)
    WHERE bgm_render_plan_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_audio_plan_bgm_render_plan;
DROP INDEX IF EXISTS idx_audio_plan_voiceover_render_plan;
DROP INDEX IF EXISTS idx_audio_plan_workspace_updated;
DROP INDEX IF EXISTS idx_audio_plan_workspace_active;
DROP TABLE IF EXISTS audio_plan;
