-- +goose Up
CREATE TABLE creative_brief (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '',
    video_type TEXT NOT NULL DEFAULT '',
    target_audience TEXT NOT NULL DEFAULT '',
    tone TEXT NOT NULL DEFAULT '',
    visual_style TEXT NOT NULL DEFAULT '',
    duration_sec DOUBLE PRECISION,
    aspect_ratio TEXT NOT NULL DEFAULT '',
    language TEXT NOT NULL DEFAULT '',
    objective TEXT NOT NULL DEFAULT '',
    concept TEXT NOT NULL DEFAULT '',
    constraints JSONB NOT NULL DEFAULT '[]',
    metadata JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'draft',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT creative_brief_duration_positive CHECK (duration_sec IS NULL OR duration_sec > 0),
    CONSTRAINT creative_brief_status_check CHECK (status IN ('draft', 'active', 'approved', 'archived'))
);

CREATE INDEX idx_creative_brief_workspace_active
    ON creative_brief(workspace_id, archived_at, updated_at DESC);
CREATE UNIQUE INDEX idx_creative_brief_workspace_active_unique
    ON creative_brief(workspace_id)
    WHERE archived_at IS NULL AND status <> 'archived';

CREATE TABLE project_memory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    version INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    core_intent TEXT NOT NULL DEFAULT '',
    soul TEXT NOT NULL DEFAULT '',
    brand_facts JSONB NOT NULL DEFAULT '[]',
    non_negotiables JSONB NOT NULL DEFAULT '[]',
    visual_anchors JSONB NOT NULL DEFAULT '[]',
    allowed JSONB NOT NULL DEFAULT '[]',
    forbidden JSONB NOT NULL DEFAULT '[]',
    prompt_injection_hints JSONB NOT NULL DEFAULT '[]',
    source_refs JSONB NOT NULL DEFAULT '[]',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT project_memory_status_check CHECK (status IN ('draft', 'active', 'archived'))
);

CREATE UNIQUE INDEX idx_project_memory_workspace_version
    ON project_memory(workspace_id, version);
CREATE UNIQUE INDEX idx_project_memory_workspace_active
    ON project_memory(workspace_id)
    WHERE status = 'active';

CREATE TABLE key_element (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    client_key TEXT NOT NULL DEFAULT '',
    element_type TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT '',
    source_refs JSONB NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'active',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT key_element_type_check CHECK (element_type IN ('product', 'character', 'scene', 'prop', 'style')),
    CONSTRAINT key_element_source_type_check CHECK (source_type IN ('', 'user_asset', 'material_analysis', 'prompt_derived', 'agent_created')),
    CONSTRAINT key_element_status_check CHECK (status IN ('active', 'archived'))
);

CREATE INDEX idx_key_element_workspace_type
    ON key_element(workspace_id, element_type, archived_at);
CREATE UNIQUE INDEX idx_key_element_workspace_client_key_active
    ON key_element(workspace_id, client_key)
    WHERE archived_at IS NULL AND client_key <> '';

CREATE TABLE key_element_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    key_element_id UUID NOT NULL REFERENCES key_element(id) ON DELETE CASCADE,
    client_key TEXT NOT NULL DEFAULT '',
    label TEXT NOT NULL DEFAULT 'default',
    visual_description TEXT NOT NULL DEFAULT '',
    reference_status TEXT NOT NULL DEFAULT 'none',
    reference_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    reference_version_id UUID REFERENCES artifact_version(id) ON DELETE SET NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    state_facts JSONB NOT NULL DEFAULT '[]',
    source_refs JSONB NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'active',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT key_element_state_reference_status_check CHECK (reference_status IN ('none', 'needs_reference', 'ready', 'approved', 'rejected')),
    CONSTRAINT key_element_state_status_check CHECK (status IN ('active', 'archived'))
);

CREATE INDEX idx_key_element_state_workspace_element
    ON key_element_state(workspace_id, key_element_id, archived_at);
CREATE UNIQUE INDEX idx_key_element_state_client_key_active
    ON key_element_state(key_element_id, client_key)
    WHERE archived_at IS NULL AND client_key <> '';
CREATE UNIQUE INDEX idx_key_element_state_default_active
    ON key_element_state(key_element_id)
    WHERE archived_at IS NULL AND is_default;

CREATE TABLE scene (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    client_key TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    mood TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'planned',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT scene_status_check CHECK (status IN ('planned', 'draft', 'approved', 'archived'))
);

CREATE INDEX idx_scene_workspace_order
    ON scene(workspace_id, archived_at, sort_order);
CREATE UNIQUE INDEX idx_scene_workspace_client_key_active
    ON scene(workspace_id, client_key)
    WHERE archived_at IS NULL AND client_key <> '';

ALTER TABLE shot
    ADD COLUMN scene_id UUID REFERENCES scene(id) ON DELETE SET NULL,
    ADD COLUMN shot_kind TEXT NOT NULL DEFAULT '',
    ADD COLUMN creative_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN visual_intent TEXT NOT NULL DEFAULT '',
    ADD COLUMN action_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN camera_intent TEXT NOT NULL DEFAULT '',
    ADD COLUMN dialogue TEXT NOT NULL DEFAULT '',
    ADD COLUMN narration TEXT NOT NULL DEFAULT '',
    ADD COLUMN audio_plan JSONB NOT NULL DEFAULT '{}';

CREATE INDEX idx_shot_workspace_scene_order
    ON shot(workspace_id, scene_id, archived_at, sort_order);

CREATE TABLE shot_key_element (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    shot_id UUID NOT NULL REFERENCES shot(id) ON DELETE CASCADE,
    key_element_id UUID NOT NULL REFERENCES key_element(id) ON DELETE CASCADE,
    key_element_state_id UUID REFERENCES key_element_state(id) ON DELETE SET NULL,
    role TEXT NOT NULL DEFAULT '',
    required BOOLEAN NOT NULL DEFAULT true,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_shot_key_element_workspace_shot
    ON shot_key_element(workspace_id, shot_id, sort_order);
CREATE UNIQUE INDEX idx_shot_key_element_unique_active
    ON shot_key_element(shot_id, key_element_id, role);

-- +goose Down
DROP INDEX IF EXISTS idx_shot_key_element_unique_active;
DROP INDEX IF EXISTS idx_shot_key_element_workspace_shot;
DROP TABLE IF EXISTS shot_key_element;

DROP INDEX IF EXISTS idx_shot_workspace_scene_order;
ALTER TABLE shot
    DROP COLUMN IF EXISTS audio_plan,
    DROP COLUMN IF EXISTS narration,
    DROP COLUMN IF EXISTS dialogue,
    DROP COLUMN IF EXISTS camera_intent,
    DROP COLUMN IF EXISTS action_text,
    DROP COLUMN IF EXISTS visual_intent,
    DROP COLUMN IF EXISTS creative_text,
    DROP COLUMN IF EXISTS shot_kind,
    DROP COLUMN IF EXISTS scene_id;

DROP INDEX IF EXISTS idx_scene_workspace_client_key_active;
DROP INDEX IF EXISTS idx_scene_workspace_order;
DROP TABLE IF EXISTS scene;

DROP INDEX IF EXISTS idx_key_element_state_default_active;
DROP INDEX IF EXISTS idx_key_element_state_client_key_active;
DROP INDEX IF EXISTS idx_key_element_state_workspace_element;
DROP TABLE IF EXISTS key_element_state;

DROP INDEX IF EXISTS idx_key_element_workspace_client_key_active;
DROP INDEX IF EXISTS idx_key_element_workspace_type;
DROP TABLE IF EXISTS key_element;

DROP INDEX IF EXISTS idx_project_memory_workspace_active;
DROP INDEX IF EXISTS idx_project_memory_workspace_version;
DROP TABLE IF EXISTS project_memory;

DROP INDEX IF EXISTS idx_creative_brief_workspace_active_unique;
DROP INDEX IF EXISTS idx_creative_brief_workspace_active;
DROP TABLE IF EXISTS creative_brief;
