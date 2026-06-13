-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE media_type AS ENUM ('text', 'image', 'video', 'audio');
CREATE TYPE node_status AS ENUM (
    'draft',
    'ready',
    'queued',
    'running',
    'succeeded',
    'failed',
    'stale',
    'user_editing'
);

CREATE TABLE account (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    avatar_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE workspace (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    owner_id UUID NOT NULL REFERENCES account(id),
    settings JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE canvas_document (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL UNIQUE REFERENCES workspace(id) ON DELETE CASCADE,
    camera_x REAL NOT NULL DEFAULT 0,
    camera_y REAL NOT NULL DEFAULT 0,
    camera_zoom REAL NOT NULL DEFAULT 1,
    layout_version INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE media_node (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    node_type media_type NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    status node_status NOT NULL DEFAULT 'draft',
    prompt TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'user',
    canvas_x REAL NOT NULL DEFAULT 0,
    canvas_y REAL NOT NULL DEFAULT 0,
    canvas_w REAL NOT NULL DEFAULT 200,
    canvas_h REAL NOT NULL DEFAULT 120,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_account_email ON account(email);
CREATE INDEX idx_workspace_owner ON workspace(owner_id);
CREATE INDEX idx_media_node_workspace ON media_node(workspace_id);
CREATE INDEX idx_media_node_status ON media_node(workspace_id, status);

-- +goose Down
DROP INDEX IF EXISTS idx_media_node_status;
DROP INDEX IF EXISTS idx_media_node_workspace;
DROP INDEX IF EXISTS idx_workspace_owner;
DROP INDEX IF EXISTS idx_account_email;

DROP TABLE IF EXISTS media_node;
DROP TABLE IF EXISTS canvas_document;
DROP TABLE IF EXISTS workspace;
DROP TABLE IF EXISTS account;

DROP TYPE IF EXISTS node_status;
DROP TYPE IF EXISTS media_type;
