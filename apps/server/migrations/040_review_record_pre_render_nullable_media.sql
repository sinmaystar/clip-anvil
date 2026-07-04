-- +goose Up
ALTER TABLE review_record
    ALTER COLUMN node_id DROP NOT NULL,
    ALTER COLUMN artifact_version_id DROP NOT NULL;

ALTER TABLE review_record DROP CONSTRAINT IF EXISTS review_record_target_presence_check;
ALTER TABLE review_record
    ADD CONSTRAINT review_record_target_presence_check CHECK (
        (
            review_task = 'pre_render_plan_review'
            AND target_phase = 'pre_render_plan'
            AND target_object_type = 'render_plan'
            AND render_plan_id IS NOT NULL
        )
        OR (
            review_task IN ('preview_image_review', 'shot_video_review', 'final_video_review')
            AND target_object_type IN ('artifact_version', 'final_video')
            AND node_id IS NOT NULL
            AND artifact_version_id IS NOT NULL
        )
    );

-- +goose Down
ALTER TABLE review_record DROP CONSTRAINT IF EXISTS review_record_target_presence_check;

DELETE FROM review_record
WHERE node_id IS NULL
   OR artifact_version_id IS NULL;

ALTER TABLE review_record
    ALTER COLUMN node_id SET NOT NULL,
    ALTER COLUMN artifact_version_id SET NOT NULL;
