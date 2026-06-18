-- +goose Up
CREATE TYPE node_type AS ENUM ('text', 'image', 'video', 'audio');
CREATE TYPE asset_type AS ENUM ('text', 'image', 'video', 'audio', 'json');
CREATE TYPE job_status AS ENUM ('pending', 'queued', 'running', 'succeeded', 'failed', 'cancelled');

ALTER TABLE media_node
    ALTER COLUMN node_type TYPE node_type USING node_type::text::node_type,
    ADD COLUMN operation_type TEXT NOT NULL DEFAULT 'manual',
    ADD COLUMN prompt_template TEXT NOT NULL DEFAULT '',
    ADD COLUMN prompt_rich JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN prompt_refs JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN model_provider TEXT,
    ADD COLUMN model_id TEXT,
    ADD COLUMN model_params JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN current_version_id UUID,
    ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}';

UPDATE media_node
SET prompt_template = prompt
WHERE prompt_template = '';

ALTER TABLE media_asset
    ALTER COLUMN type TYPE asset_type USING type::text::asset_type,
    ALTER COLUMN storage_url DROP NOT NULL,
    ADD COLUMN text_content TEXT;

ALTER TABLE media_asset
    ADD CONSTRAINT media_asset_has_content
    CHECK (storage_url IS NOT NULL OR text_content IS NOT NULL);

ALTER TABLE media_edge DROP CONSTRAINT IF EXISTS unique_edge;
ALTER TABLE media_edge DROP COLUMN IF EXISTS edge_type;
ALTER TABLE media_edge DROP COLUMN IF EXISTS transition_type;
ALTER TABLE media_edge DROP COLUMN IF EXISTS transition_duration;
ALTER TABLE media_edge ADD CONSTRAINT unique_edge UNIQUE (from_node_id, to_node_id);

DROP TYPE IF EXISTS transition_type;
DROP TYPE IF EXISTS edge_type;
DROP TYPE IF EXISTS media_type;

CREATE TABLE generation_job (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    target_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    parent_job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    operation_type TEXT NOT NULL,
    provider TEXT NOT NULL,
    model_id TEXT NOT NULL,
    intent JSONB NOT NULL DEFAULT '{}',
    rendered_prompt TEXT NOT NULL DEFAULT '',
    provider_request JSONB NOT NULL DEFAULT '{}',
    provider_response JSONB NOT NULL DEFAULT '{}',
    status job_status NOT NULL DEFAULT 'pending',
    progress INT NOT NULL DEFAULT 0,
    attempt INT NOT NULL DEFAULT 1,
    max_attempts INT NOT NULL DEFAULT 1,
    retry_policy JSONB NOT NULL DEFAULT '{}',
    cost_cents INT,
    error_code TEXT,
    error_message TEXT,
    requested_by_type TEXT NOT NULL DEFAULT 'user',
    requested_by_id TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE artifact_version (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    asset_id UUID REFERENCES media_asset(id) ON DELETE SET NULL,
    version_no INT NOT NULL,
    winner BOOLEAN NOT NULL DEFAULT false,
    output JSONB NOT NULL DEFAULT '{}',
    review_score REAL,
    input_hash TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_version_per_node UNIQUE (node_id, version_no)
);

ALTER TABLE media_node
    ADD CONSTRAINT fk_media_node_current_version
    FOREIGN KEY (current_version_id) REFERENCES artifact_version(id)
    ON DELETE SET NULL;

CREATE INDEX idx_generation_job_node ON generation_job(target_node_id);
CREATE INDEX idx_generation_job_status ON generation_job(workspace_id, status);
CREATE INDEX idx_artifact_version_node ON artifact_version(node_id);
CREATE UNIQUE INDEX idx_artifact_version_one_winner
    ON artifact_version(node_id)
    WHERE winner = true;

-- +goose Down
ALTER TABLE media_node DROP CONSTRAINT IF EXISTS fk_media_node_current_version;
DROP INDEX IF EXISTS idx_artifact_version_one_winner;
DROP INDEX IF EXISTS idx_artifact_version_node;
DROP INDEX IF EXISTS idx_generation_job_status;
DROP INDEX IF EXISTS idx_generation_job_node;
DROP TABLE IF EXISTS artifact_version;
DROP TABLE IF EXISTS generation_job;

CREATE TYPE media_type AS ENUM ('text', 'image', 'video', 'audio');
CREATE TYPE edge_type AS ENUM ('dependency', 'reference', 'sequence');
CREATE TYPE transition_type AS ENUM ('cut', 'crossfade', 'dissolve', 'wipe');

ALTER TABLE media_edge DROP CONSTRAINT IF EXISTS unique_edge;
ALTER TABLE media_edge ADD COLUMN edge_type edge_type NOT NULL DEFAULT 'dependency';
ALTER TABLE media_edge ADD COLUMN transition_type transition_type;
ALTER TABLE media_edge ADD COLUMN transition_duration REAL;
ALTER TABLE media_edge ADD CONSTRAINT unique_edge UNIQUE (from_node_id, to_node_id, edge_type);

ALTER TABLE media_asset DROP CONSTRAINT IF EXISTS media_asset_has_content;
ALTER TABLE media_asset DROP COLUMN IF EXISTS text_content;
ALTER TABLE media_asset ALTER COLUMN storage_url SET NOT NULL;
ALTER TABLE media_asset ALTER COLUMN type TYPE media_type USING type::text::media_type;

ALTER TABLE media_node
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS current_version_id,
    DROP COLUMN IF EXISTS model_params,
    DROP COLUMN IF EXISTS model_id,
    DROP COLUMN IF EXISTS model_provider,
    DROP COLUMN IF EXISTS prompt_refs,
    DROP COLUMN IF EXISTS prompt_rich,
    DROP COLUMN IF EXISTS prompt_template,
    DROP COLUMN IF EXISTS operation_type;

ALTER TABLE media_node
    ALTER COLUMN node_type TYPE media_type USING node_type::text::media_type;

DROP TYPE IF EXISTS job_status;
DROP TYPE IF EXISTS asset_type;
DROP TYPE IF EXISTS node_type;
