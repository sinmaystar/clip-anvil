-- +goose Up
ALTER TABLE review_record DROP CONSTRAINT review_record_phase_check;
ALTER TABLE review_record DROP CONSTRAINT review_record_status_check;
ALTER TABLE agent_thread DROP CONSTRAINT agent_thread_scope_type_check;
ALTER TABLE agent_task DROP CONSTRAINT agent_task_scope_type_check;

ALTER TABLE review_record
    ADD COLUMN review_task TEXT NOT NULL DEFAULT 'preview_image_review',
    ADD COLUMN target_object_type TEXT NOT NULL DEFAULT 'artifact_version',
    ADD COLUMN target_object_id UUID,
    ADD COLUMN render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    ADD COLUMN required_axes JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN escalation JSONB NOT NULL DEFAULT '{}';

UPDATE review_record
SET review_task = CASE target_phase
        WHEN 'shot_video' THEN 'shot_video_review'
        WHEN 'final_video' THEN 'final_video_review'
        ELSE 'preview_image_review'
    END,
    target_object_type = 'artifact_version',
    target_object_id = artifact_version_id;

ALTER TABLE review_record
    ADD CONSTRAINT review_record_phase_check CHECK (target_phase IN ('pre_render_plan', 'preview_image', 'shot_video', 'final_video')),
    ADD CONSTRAINT review_record_task_check CHECK (review_task IN ('pre_render_plan_review', 'preview_image_review', 'shot_video_review', 'final_video_review')),
    ADD CONSTRAINT review_record_target_type_check CHECK (target_object_type IN ('render_plan', 'artifact_version', 'shot', 'final_video')),
    ADD CONSTRAINT review_record_status_check CHECK (status IN ('running', 'accepted', 'accepted_with_warnings', 'rejected', 'blocked', 'failed'));

ALTER TABLE agent_thread
    ADD CONSTRAINT agent_thread_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'final_output', 'render_plan'));

ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'node', 'job', 'final_output', 'render_plan'));

CREATE TABLE artifact_issue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    review_record_id UUID NOT NULL REFERENCES review_record(id) ON DELETE CASCADE,
    dimension TEXT NOT NULL,
    severity TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    target_object_type TEXT NOT NULL,
    target_object_id UUID NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    evidence TEXT NOT NULL DEFAULT '',
    suggested_fix TEXT NOT NULL DEFAULT 'none',
    fix_hint TEXT NOT NULL DEFAULT '',
    requires_user_confirmation BOOLEAN NOT NULL DEFAULT false,
    superseded_by_issue_id UUID REFERENCES artifact_issue(id) ON DELETE SET NULL,
    resolved_by_review_record_id UUID REFERENCES review_record(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT artifact_issue_dimension_check CHECK (dimension IN (
        'faithfulness',
        'subject_consistency',
        'product_visibility',
        'brand_style_consistency',
        'composition_proportion',
        'motion_physics',
        'visual_quality',
        'continuity',
        'audio_sync',
        'platform_selling_power',
        'model_capability',
        'prompt_validity',
        'reference_role_validity',
        'cost_risk',
        'dependency_not_ready',
        'project_memory_conflict'
    )),
    CONSTRAINT artifact_issue_severity_check CHECK (severity IN ('info', 'warning', 'blocking')),
    CONSTRAINT artifact_issue_status_check CHECK (status IN ('open', 'resolved', 'superseded', 'accepted_risk')),
    CONSTRAINT artifact_issue_target_type_check CHECK (target_object_type IN ('render_plan', 'artifact_version', 'shot', 'final_video', 'project_memory')),
    CONSTRAINT artifact_issue_suggested_fix_check CHECK (suggested_fix IN ('none', 'regenerate', 'edit', 'extend', 'bridge', 'revise_render_plan', 'revise_shot_plan', 'manual'))
);

CREATE INDEX idx_artifact_issue_workspace_status ON artifact_issue(workspace_id, status, created_at DESC);
CREATE INDEX idx_artifact_issue_review_record ON artifact_issue(review_record_id);
CREATE INDEX idx_artifact_issue_target ON artifact_issue(target_object_type, target_object_id, status);
CREATE INDEX idx_artifact_issue_dimension ON artifact_issue(workspace_id, dimension, status);
CREATE INDEX idx_review_record_task_target ON review_record(workspace_id, review_task, target_object_type, target_object_id, created_at DESC);
CREATE INDEX idx_review_record_render_plan ON review_record(render_plan_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_review_record_render_plan;
DROP INDEX IF EXISTS idx_review_record_task_target;
DROP INDEX IF EXISTS idx_artifact_issue_dimension;
DROP INDEX IF EXISTS idx_artifact_issue_target;
DROP INDEX IF EXISTS idx_artifact_issue_review_record;
DROP INDEX IF EXISTS idx_artifact_issue_workspace_status;
DROP TABLE IF EXISTS artifact_issue;

ALTER TABLE review_record DROP CONSTRAINT review_record_phase_check;
ALTER TABLE review_record DROP CONSTRAINT review_record_status_check;
ALTER TABLE review_record DROP CONSTRAINT review_record_target_type_check;
ALTER TABLE review_record DROP CONSTRAINT review_record_task_check;
ALTER TABLE agent_thread DROP CONSTRAINT agent_thread_scope_type_check;
ALTER TABLE agent_task DROP CONSTRAINT agent_task_scope_type_check;

ALTER TABLE review_record
    DROP COLUMN escalation,
    DROP COLUMN required_axes,
    DROP COLUMN render_plan_id,
    DROP COLUMN target_object_id,
    DROP COLUMN target_object_type,
    DROP COLUMN review_task;

ALTER TABLE review_record
    ADD CONSTRAINT review_record_phase_check CHECK (target_phase IN ('preview_image', 'shot_video', 'final_video')),
    ADD CONSTRAINT review_record_status_check CHECK (status IN ('running', 'accepted', 'rejected', 'failed'));

ALTER TABLE agent_thread
    ADD CONSTRAINT agent_thread_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'final_output'));

ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'node', 'job', 'final_output'));
