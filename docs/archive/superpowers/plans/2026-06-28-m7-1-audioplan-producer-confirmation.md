# M7.1 AudioPlan Producer Confirmation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task in this session, or use `superpowers:subagent-driven-development` with a fresh worker per task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first M7 milestone slice: a workspace-level `AudioPlan` fact source that Producer can draft, expose for user confirmation, approve, and read back before M7.2 audio RenderPlan generation begins.

**Architecture:** `audio_plan` is a new Agent-mode domain table and the only full-piece audio fact source. Shot-level `shot.audio_plan` remains a cue summary projection, while Producer writes the full voiceover script, voice profile, BGM direction, cue plan, and generation settings through a native tool. Existing HITL remains `request_user_decision`; this phase adds the persistent plan and prompt/tool contract, not audio provider execution.

**Tech Stack:** Go 1.26, PostgreSQL 16, pgx/sqlc, CloudWeGo Eino native tools, existing ClipAnvil Agent Producer graph, existing `creative.Service` context reader, existing `pss.Builder`.

---

## Current Code Facts

- `shot.audio_plan` already exists as JSONB in `apps/server/migrations/024_m1_agent_creative_state.sql`, but it is per-shot and cannot be the full-piece audio source.
- `render_plan` currently only allows `scope_type IN ('key_element_state', 'shot')` and `target_phase IN ('reference_image', 'preview_image', 'shot_video')`, so audio RenderPlan changes belong to M7.2.
- Producer native tools are wired in `apps/server/cmd/server/main.go` through `agenttools.NewNativeRegistry(...)`.
- Producer context is read through `apps/server/internal/agent/tools/read_project_context.go`, `apps/server/internal/agent/creative/state_service.go`, and `apps/server/internal/agent/pss/producer.go`.
- Human confirmation already uses `request_user_decision` from `apps/server/internal/agent/tools/decision.go`; M7.1 should reuse it instead of inventing a second decision mechanism.

## Task 1: Add AudioPlan Persistence

**Files:**
- Create: `apps/server/migrations/032_m7_1_audio_plan.sql`
- Create: `apps/server/sqlc/queries/audio_plan.sql`
- Modify after generation: `apps/server/internal/store/db/models.go`
- Create test: `apps/server/internal/agent/audio/audio_plan_contract_test.go`

- [x] **Step 1: Write the failing contract test**

Create `apps/server/internal/agent/audio/audio_plan_contract_test.go`:

```go
package audio

import (
	"os"
	"strings"
	"testing"
)

func TestAudioPlanPersistenceContract(t *testing.T) {
	migration, err := os.ReadFile("../../../migrations/032_m7_1_audio_plan.sql")
	if err != nil {
		t.Fatal(err)
	}
	query, err := os.ReadFile("../../../sqlc/queries/audio_plan.sql")
	if err != nil {
		t.Fatal(err)
	}
	migrationText := string(migration)
	queryText := string(query)
	for _, want := range []string{
		"CREATE TABLE audio_plan",
		"workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE",
		"voiceover_script TEXT NOT NULL DEFAULT ''",
		"voice_profile JSONB NOT NULL DEFAULT '{}'",
		"bgm_plan JSONB NOT NULL DEFAULT '{}'",
		"cue_plan JSONB NOT NULL DEFAULT '[]'",
		"generation_params JSONB NOT NULL DEFAULT '{}'",
		"idx_audio_plan_workspace_active",
		"status IN ('draft', 'waiting_for_user', 'approved', 'generating', 'voiceover_ready', 'composing', 'completed', 'blocked', 'failed', 'archived')",
	} {
		if !strings.Contains(migrationText, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
	for _, want := range []string{
		"-- name: CreateAudioPlan :one",
		"-- name: GetAudioPlan :one",
		"-- name: GetActiveAudioPlanByWorkspace :one",
		"-- name: ListAudioPlansByWorkspace :many",
		"-- name: UpdateAudioPlan :one",
		"-- name: ArchiveActiveAudioPlansByWorkspace :exec",
		"-- name: UpdateAudioPlanStatus :one",
	} {
		if !strings.Contains(queryText, want) {
			t.Fatalf("query missing %q", want)
		}
	}
}
```

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/audio -run AudioPlanPersistenceContract -count=1)
```

Expected: FAIL because the package, migration, and query do not exist yet.

- [x] **Step 2: Create the migration**

Create `apps/server/migrations/032_m7_1_audio_plan.sql`:

```sql
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
```

- [x] **Step 3: Add sqlc queries**

Create `apps/server/sqlc/queries/audio_plan.sql`:

```sql
-- name: CreateAudioPlan :one
INSERT INTO audio_plan (
    workspace_id,
    status,
    title,
    plan_kind,
    language,
    target_duration_sec,
    voiceover_script,
    voice_profile,
    bgm_plan,
    cue_plan,
    generation_params,
    created_by_task_id,
    semantic_key,
    display_name
) VALUES (
    sqlc.arg(workspace_id),
    sqlc.arg(status),
    sqlc.arg(title),
    sqlc.arg(plan_kind),
    sqlc.arg(language),
    sqlc.narg(target_duration_sec),
    sqlc.arg(voiceover_script),
    sqlc.arg(voice_profile),
    sqlc.arg(bgm_plan),
    sqlc.arg(cue_plan),
    sqlc.arg(generation_params),
    sqlc.narg(created_by_task_id),
    sqlc.arg(semantic_key),
    sqlc.arg(display_name)
)
RETURNING *;

-- name: GetAudioPlan :one
SELECT * FROM audio_plan
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id);

-- name: GetActiveAudioPlanByWorkspace :one
SELECT * FROM audio_plan
WHERE workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
  AND status IN ('draft', 'waiting_for_user', 'approved', 'generating', 'voiceover_ready', 'composing', 'blocked', 'failed')
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: ListAudioPlansByWorkspace :many
SELECT * FROM audio_plan
WHERE workspace_id = sqlc.arg(workspace_id)
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: UpdateAudioPlan :one
UPDATE audio_plan
SET
    status = sqlc.arg(status),
    title = sqlc.arg(title),
    language = sqlc.arg(language),
    target_duration_sec = sqlc.narg(target_duration_sec),
    voiceover_script = sqlc.arg(voiceover_script),
    voice_profile = sqlc.arg(voice_profile),
    bgm_plan = sqlc.arg(bgm_plan),
    cue_plan = sqlc.arg(cue_plan),
    generation_params = sqlc.arg(generation_params),
    display_name = sqlc.arg(display_name),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
RETURNING *;

-- name: UpdateAudioPlanStatus :one
UPDATE audio_plan
SET
    status = sqlc.arg(status),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
RETURNING *;

-- name: ArchiveActiveAudioPlansByWorkspace :exec
UPDATE audio_plan
SET
    status = 'archived',
    archived_at = now(),
    updated_at = now()
WHERE workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
  AND status IN ('draft', 'waiting_for_user', 'approved', 'generating', 'voiceover_ready', 'composing', 'blocked', 'failed');
```

- [x] **Step 4: Generate and verify**

Run:

```bash
make sqlc-generate
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/audio -run AudioPlanPersistenceContract -count=1)
git diff --check
```

Expected: sqlc generation succeeds and the contract test passes.

## Task 2: Add AudioPlan Domain Service

**Files:**
- Create: `apps/server/internal/agent/audio/service.go`
- Create: `apps/server/internal/agent/audio/service_test.go`

- [x] **Step 1: Write failing service tests**

Create `apps/server/internal/agent/audio/service_test.go` with these test names:

```go
package audio

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestServiceCreateReplacesExistingActivePlan(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	workspaceID := uuidWithByte(1)
	store.workspace = db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeAgent}
	store.active = db.AudioPlan{ID: uuidWithByte(2), WorkspaceID: workspaceID, Status: "draft"}

	created, err := service.Upsert(context.Background(), UpsertInput{
		WorkspaceID: workspaceID,
		TaskID:      uuidWithByte(3),
		Mode:        "replace_draft",
		Title:       "营销短视频音频方案",
		Language:    "zh",
		VoiceoverScript: "现在出发，让旅程更轻松。",
		VoiceProfile: map[string]any{"speaker": "marketing_female_clear"},
		BGMPlan: map[string]any{"source": "generated", "model": "seed-audio-1.0"},
		CuePlan: []CueInput{{ShotRef: "shot_01", StartSec: 0, EndSec: 3.2, Text: "现在出发，让旅程更轻松。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.archivedActive {
		t.Fatal("expected existing active audio plans to be archived")
	}
	if created.Status != "waiting_for_user" {
		t.Fatalf("status = %q, want waiting_for_user", created.Status)
	}
}

func TestServiceApproveRequiresExistingActivePlan(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	workspaceID := uuidWithByte(1)
	store.workspace = db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeAgent}
	store.activeErr = pgx.ErrNoRows

	_, err := service.Upsert(context.Background(), UpsertInput{WorkspaceID: workspaceID, Mode: "approve"})
	if !errors.Is(err, ErrAudioPlanNotFound) {
		t.Fatalf("error = %v, want ErrAudioPlanNotFound", err)
	}
}

func TestServiceRejectsStudioWorkspace(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	workspaceID := uuidWithByte(1)
	store.workspace = db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeStudio}

	_, err := service.Upsert(context.Background(), UpsertInput{WorkspaceID: workspaceID, Mode: "replace_draft"})
	if !errors.Is(err, ErrAgentWorkspaceRequired) {
		t.Fatalf("error = %v, want ErrAgentWorkspaceRequired", err)
	}
}
```

Implement the fake store in the same test file with exact methods required by the service: `GetWorkspaceByID`, `GetActiveAudioPlanByWorkspace`, `ArchiveActiveAudioPlansByWorkspace`, `CreateAudioPlan`, `UpdateAudioPlan`, and `UpdateAudioPlanStatus`.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/audio -run 'Service' -count=1)
```

Expected: FAIL because the service does not exist.

- [x] **Step 2: Implement service types and validation**

Create `apps/server/internal/agent/audio/service.go` with:

```go
package audio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrAgentWorkspaceRequired = errors.New("agent workspace required")
	ErrInvalidAudioPlanInput  = errors.New("invalid audio plan input")
	ErrAudioPlanNotFound      = errors.New("audio plan not found")
)

type Store interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	GetActiveAudioPlanByWorkspace(ctx context.Context, workspaceID pgtype.UUID) (db.AudioPlan, error)
	ArchiveActiveAudioPlansByWorkspace(ctx context.Context, workspaceID pgtype.UUID) error
	CreateAudioPlan(ctx context.Context, arg db.CreateAudioPlanParams) (db.AudioPlan, error)
	UpdateAudioPlan(ctx context.Context, arg db.UpdateAudioPlanParams) (db.AudioPlan, error)
	UpdateAudioPlanStatus(ctx context.Context, arg db.UpdateAudioPlanStatusParams) (db.AudioPlan, error)
}

type Service struct {
	store Store
}

type UpsertInput struct {
	WorkspaceID       pgtype.UUID
	TaskID            pgtype.UUID
	Mode              string
	Title             string
	Language          string
	TargetDurationSec *float64
	VoiceoverScript   string
	VoiceProfile      map[string]any
	BGMPlan           map[string]any
	CuePlan           []CueInput
	GenerationParams  map[string]any
}

type CueInput struct {
	ShotRef  string  `json:"shot_ref"`
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
	Text     string  `json:"text"`
}
```

Service behavior:
- `replace_draft` archives any active plan and creates a new plan with `status='waiting_for_user'`.
- `patch` updates the current active plan and keeps the provided status as `waiting_for_user` unless the current plan is already `approved`.
- `approve` updates only status to `approved`.
- `block` updates only status to `blocked`.
- all modes require Agent workspace.
- `replace_draft` / `patch` require non-empty `voiceover_script`, valid cue ranges, and `bgm_plan.source='generated'` when BGM is present.

- [x] **Step 3: Run service tests**

Run:

```bash
gofmt -w apps/server/internal/agent/audio
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/audio -run 'Service|AudioPlanPersistenceContract' -count=1)
git diff --check
```

Expected: PASS.

## Task 3: Add Producer Native Tool

**Files:**
- Create: `apps/server/internal/agent/tools/audio_plan.go`
- Create: `apps/server/internal/agent/tools/audio_plan_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [x] **Step 1: Write failing tool tests**

Create `apps/server/internal/agent/tools/audio_plan_test.go` with tests:

```go
func TestUpsertAudioPlanNativeToolCreatesNaturalResult(t *testing.T)
func TestUpsertAudioPlanNativeToolRequiresRuntimeContext(t *testing.T)
func TestUpsertAudioPlanValidationRejectsMissingScript(t *testing.T)
func TestUpsertAudioPlanValidationRejectsInvalidCueTiming(t *testing.T)
```

The success test should call `InvokableRun` with a native runtime context and JSON:

```json
{
  "brief": "保存全片旁白和 BGM 方案，等待用户确认。",
  "mode": "replace_draft",
  "title": "营销短视频音频方案",
  "language": "zh",
  "voiceover_script": "现在出发，让旅程更轻松。",
  "voice_profile": {"source": "preset", "speaker": "marketing_female_clear"},
  "bgm_plan": {"source": "generated", "provider": "volcengine", "model": "seed-audio-1.0", "style": "轻快电子流行"},
  "cue_plan": [{"shot_ref": "shot_01", "start_sec": 0, "end_sec": 3.2, "text": "现在出发，让旅程更轻松。"}]
}
```

Expected natural result contains `AudioPlan` and `waiting_for_user`.

- [x] **Step 2: Implement native tool**

Create `apps/server/internal/agent/tools/audio_plan.go` with:

```go
type AudioPlanUpserter interface {
	Upsert(ctx context.Context, input agentaudio.UpsertInput) (db.AudioPlan, error)
}

type UpsertAudioPlanNativeTool struct {
	service AudioPlanUpserter
}

type UpsertAudioPlanInput struct {
	Brief             string                   `json:"brief" jsonschema:"required" jsonschema_description:"一句话说明为什么要写入 AudioPlan。"`
	Mode              string                   `json:"mode" jsonschema:"required,enum=replace_draft,enum=patch,enum=approve,enum=block" jsonschema_description:"replace_draft 创建新的待确认方案；patch 修改当前方案；approve 标记用户已确认；block 标记音频方案阻塞。"`
	Title             string                   `json:"title" jsonschema_description:"音频方案标题。"`
	Language          string                   `json:"language" jsonschema_description:"旁白语言，例如 zh。"`
	TargetDurationSec *float64                 `json:"target_duration_sec" jsonschema_description:"目标全片时长，单位秒。"`
	VoiceoverScript   string                   `json:"voiceover_script" jsonschema_description:"全片旁白脚本。replace_draft 和 patch 时必填。"`
	VoiceProfile      map[string]any           `json:"voice_profile" jsonschema_description:"音色方向，例如 source、speaker、style。"`
	BGMPlan           map[string]any           `json:"bgm_plan" jsonschema_description:"BGM 生成方向。第一版 source 必须是 generated，model 必须是 seed-audio-1.0。"`
	CuePlan           []agentaudio.CueInput     `json:"cue_plan" jsonschema_description:"按 shot_ref 切分的旁白 cue。"`
	GenerationParams  map[string]any           `json:"generation_params" jsonschema_description:"后续音频生成参数，例如 format、sample_rate、speech_rate。"`
}
```

Tool behavior:
- use `runtimeOrError(ctx, "upsert_audio_plan")` like existing native tools.
- validate `brief`, `mode`, script/cue requirements for draft/patch.
- map runtime `WorkspaceID` and `TaskID` into `agentaudio.UpsertInput`.
- return natural result with plan id, status, title, cue count, and next action.

- [x] **Step 3: Wire tool in server**

Modify `apps/server/cmd/server/main.go`:

```go
audioPlanService := agentaudio.NewService(queries)
```

and add to Producer native registry:

```go
agenttools.NewUpsertAudioPlanNativeTool(audioPlanService),
```

The tool should be placed near `NewUpsertStoryboardNativeTool` because it writes Producer-owned project facts.

- [x] **Step 4: Verify tool package**

Run:

```bash
gofmt -w apps/server/internal/agent/tools apps/server/cmd/server/main.go
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run AudioPlan -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/audio -count=1)
git diff --check
```

Expected: PASS.

## Task 4: Expose AudioPlan in Producer Context and Prompt

**Files:**
- Modify: `apps/server/internal/agent/creative/types.go`
- Modify: `apps/server/internal/agent/creative/state_service.go`
- Modify: `apps/server/internal/agent/tools/read_project_context.go`
- Modify: `apps/server/internal/agent/pss/producer.go`
- Modify tests: `apps/server/internal/agent/creative/state_service_test.go`
- Modify tests: `apps/server/internal/agent/pss/producer_test.go`
- Modify: `apps/server/internal/agent/producer/system_prompt.go`
- Modify tests: `apps/server/internal/agent/producer/model_responder_test.go` or `apps/server/internal/agent/producer/graph_test.go`

- [x] **Step 1: Write failing context tests**

Add tests that prove:
- `creative.Service.ReadProjectContext` returns `ActiveAudioPlan`.
- `read_project_context` natural result includes `AudioPlan`.
- `pss.Builder.BuildProducerPSS` structured output contains `audio_plan`.

Use existing fake store patterns in `state_service_test.go` and `producer_test.go`; extend fakes with `GetActiveAudioPlanByWorkspace`.

- [x] **Step 2: Extend creative context**

In `apps/server/internal/agent/creative/types.go`, add:

```go
ActiveAudioPlan *db.AudioPlan
```

to `ContextPacket`.

In `apps/server/internal/agent/creative/state_service.go`, extend `Store`:

```go
GetActiveAudioPlanByWorkspace(ctx context.Context, workspaceID pgtype.UUID) (db.AudioPlan, error)
```

and in `ReadProjectContext`, populate `packet.ActiveAudioPlan`, ignoring `pgx.ErrNoRows`.

- [x] **Step 3: Extend read_project_context result**

In `apps/server/internal/agent/tools/read_project_context.go`, add an `AudioPlan` natural result item:

```go
audioPlanStatus := "没有 active AudioPlan"
if packet.ActiveAudioPlan != nil {
	audioPlanStatus = packet.ActiveAudioPlan.Title + " / " + packet.ActiveAudioPlan.Status
}
```

Add `audio_plan` to the include schema description.

- [x] **Step 4: Extend Producer PSS**

In `apps/server/internal/agent/pss/producer.go`, extend `Store` with `GetActiveAudioPlanByWorkspace`, read it in `BuildProducerPSS`, add:

```go
"audio_plan": audioPlanSummary(audioPlan, hasAudioPlan),
```

The summary should expose status, title, language, target duration, script excerpt, voice profile, BGM plan, cue count, and semantic key.

- [x] **Step 5: Update Producer prompt**

In `apps/server/internal/agent/producer/system_prompt.go`, add AudioPlan to:
- Producer responsibilities.
- domain concepts.
- available creative state tools.
- generation scheduling rules.

Required prompt rules:
- Producer must create full-piece AudioPlan before audio generation.
- Producer must request user confirmation before approving a new AudioPlan unless the user explicitly asked to proceed automatically.
- Producer must not use per-shot independent audio generation as the first-version path.
- Producer must not treat video model self-audio as the final multi-shot track.
- Producer must use `upsert_audio_plan(mode=approve)` after user confirms the decision card.

- [x] **Step 6: Verify context and prompt tests**

Run:

```bash
gofmt -w apps/server/internal/agent/creative apps/server/internal/agent/tools apps/server/internal/agent/pss apps/server/internal/agent/producer
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/creative -run 'ReadProjectContext|AudioPlan' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/pss -run 'ProducerPSS.*AudioPlan|AudioPlan' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'AudioPlan|Prompt' -count=1)
git diff --check
```

Expected: PASS.

## Task 5: M7.1 Full Verification and Milestone Update

**Files:**
- Modify: `docs/milestones/m7-agent-audio-plan-composer.md`

- [x] **Step 1: Run full M7.1 verification**

Run:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-build
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

Expected: all commands pass.

- [x] **Step 2: Update milestone status**

In `docs/milestones/m7-agent-audio-plan-composer.md`, change M7.1 from pending acceptance text to completed evidence, including:
- migration/query/service/tool landed.
- one-active-plan constraint exists.
- Producer can write and read AudioPlan.
- verification commands and date.

- [x] **Step 3: Commit M7.1**

Commit only M7.1 files:

```bash
git status --short
git add apps/server/migrations/032_m7_1_audio_plan.sql \
  apps/server/sqlc/queries/audio_plan.sql \
  apps/server/internal/store/db \
  apps/server/internal/agent/audio \
  apps/server/internal/agent/tools \
  apps/server/internal/agent/creative \
  apps/server/internal/agent/pss \
  apps/server/internal/agent/producer \
  apps/server/cmd/server/main.go \
  docs/milestones/m7-agent-audio-plan-composer.md \
  docs/superpowers/plans/2026-06-28-m7-1-audioplan-producer-confirmation.md
git diff --cached --check
git commit -m "feat: add agent audio plan confirmation"
```

## Plan Self-Review

- Spec coverage: This plan covers M7.1 only: AudioPlan persistence, one active plan per workspace, Producer write/read tool, user confirmation through existing HITL, prompt contract, and verification. M7.2 provider/model capability work is intentionally excluded.
- Placeholder scan: There are no `TBD`, `TODO`, or generic "add tests" steps; each task names files, tests, behavior, and commands.
- Type consistency: The plan consistently uses `audio_plan`, `AudioPlan`, `UpsertInput`, `CueInput`, `upsert_audio_plan`, and statuses from the migration.
