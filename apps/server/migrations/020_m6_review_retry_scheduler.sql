-- +goose Up
CREATE TABLE review_record (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    shot_id UUID REFERENCES shot(id) ON DELETE SET NULL,
    node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    artifact_version_id UUID NOT NULL REFERENCES artifact_version(id) ON DELETE CASCADE,
    generation_job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    reviewer_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    reviewer_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    parent_review_record_id UUID REFERENCES review_record(id) ON DELETE SET NULL,
    target_phase TEXT NOT NULL,
    status TEXT NOT NULL,
    attempt_no INT NOT NULL DEFAULT 1,
    max_attempts INT NOT NULL DEFAULT 3,
    overall_score REAL,
    rubric JSONB NOT NULL DEFAULT '{}',
    critique TEXT NOT NULL DEFAULT '',
    retry_recommendation JSONB NOT NULL DEFAULT '{}',
    model_provider TEXT NOT NULL DEFAULT '',
    model_id TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    CONSTRAINT review_record_phase_check CHECK (target_phase IN ('preview_image', 'shot_video', 'final_video')),
    CONSTRAINT review_record_status_check CHECK (status IN ('running', 'accepted', 'rejected', 'failed')),
    CONSTRAINT review_record_attempt_check CHECK (attempt_no >= 1 AND max_attempts >= attempt_no)
);

CREATE INDEX idx_review_record_workspace_created ON review_record(workspace_id, created_at DESC);
CREATE INDEX idx_review_record_shot_phase ON review_record(workspace_id, shot_id, target_phase, created_at DESC);
CREATE INDEX idx_review_record_node ON review_record(node_id, created_at DESC);
CREATE INDEX idx_review_record_version ON review_record(artifact_version_id, created_at DESC);
CREATE INDEX idx_review_record_task ON review_record(reviewer_task_id);
CREATE INDEX idx_review_record_parent ON review_record(parent_review_record_id);

ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume',
        'craftsman_turn',
        'worker_generation',
        'reviewer_turn',
        'dependency_scheduler'
    ));

-- +goose Down
ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume',
        'craftsman_turn',
        'worker_generation'
    ));

DROP INDEX IF EXISTS idx_review_record_parent;
DROP INDEX IF EXISTS idx_review_record_task;
DROP INDEX IF EXISTS idx_review_record_version;
DROP INDEX IF EXISTS idx_review_record_node;
DROP INDEX IF EXISTS idx_review_record_shot_phase;
DROP INDEX IF EXISTS idx_review_record_workspace_created;
DROP TABLE IF EXISTS review_record;
