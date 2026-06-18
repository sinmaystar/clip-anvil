-- +goose Up
CREATE TYPE workspace_mode AS ENUM ('studio', 'agent');

ALTER TABLE workspace
    ADD COLUMN mode workspace_mode NOT NULL DEFAULT 'studio';

CREATE INDEX idx_workspace_owner_mode ON workspace(owner_id, mode);

-- +goose Down
DROP INDEX IF EXISTS idx_workspace_owner_mode;

ALTER TABLE workspace
    DROP COLUMN IF EXISTS mode;

DROP TYPE IF EXISTS workspace_mode;
