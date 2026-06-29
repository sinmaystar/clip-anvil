-- +goose Up
ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_scope_type_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_scope_type_check
    CHECK (scope_type IN ('key_element_state', 'shot', 'audio_plan'));

ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_target_phase_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_target_phase_check
    CHECK (target_phase IN ('reference_image', 'preview_image', 'shot_video', 'voiceover_audio', 'bgm_audio'));

ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_profile_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_profile_check
    CHECK (model_prompt_profile IN ('seedream_5_image', 'seedance_2_video', 'seed_audio_1'));

-- +goose Down
ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_profile_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_profile_check
    CHECK (model_prompt_profile IN ('seedream_5_image', 'seedance_2_video'));

ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_target_phase_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_target_phase_check
    CHECK (target_phase IN ('reference_image', 'preview_image', 'shot_video'));

ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_scope_type_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_scope_type_check
    CHECK (scope_type IN ('key_element_state', 'shot'));
