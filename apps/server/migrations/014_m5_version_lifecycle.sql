-- +goose Up
ALTER TABLE artifact_version
    ADD COLUMN status job_status NOT NULL DEFAULT 'succeeded',
    ADD COLUMN progress INT NOT NULL DEFAULT 100,
    ADD COLUMN error_code TEXT,
    ADD COLUMN error_message TEXT,
    ADD COLUMN provider_request JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN provider_response JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN started_at TIMESTAMPTZ,
    ADD COLUMN completed_at TIMESTAMPTZ;

CREATE UNIQUE INDEX idx_artifact_version_job_unique
    ON artifact_version(job_id)
    WHERE job_id IS NOT NULL;

CREATE INDEX idx_artifact_version_node_status
    ON artifact_version(node_id, status, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_artifact_version_node_status;
DROP INDEX IF EXISTS idx_artifact_version_job_unique;

ALTER TABLE artifact_version
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS started_at,
    DROP COLUMN IF EXISTS provider_response,
    DROP COLUMN IF EXISTS provider_request,
    DROP COLUMN IF EXISTS error_message,
    DROP COLUMN IF EXISTS error_code,
    DROP COLUMN IF EXISTS progress,
    DROP COLUMN IF EXISTS status;
