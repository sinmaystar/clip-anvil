# M6.9 UX Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the final M6 Agent workspace UX: a production overview endpoint, status bar, storyboard section, task timeline, final output confirmation surface, and richer read-only node trace.

**Architecture:** Add one backend `production-overview` projection for Agent workspaces, then make the frontend render all current production status from that structured projection. Websocket messages invalidate or patch the overview for freshness, while refresh recovery always comes from persisted DB facts.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, PostgreSQL, React 19, Vite 8, TypeScript 6, TanStack Query, existing Agent websocket, existing ClipAnvil typed message blocks.

---

## Scope Notes

This plan implements `docs/superpowers/specs/2026-06-23-m6-9-ux-completion-design.md`.

It intentionally does not modify generation providers, Eino graph topology, Studio mode, or Agent canvas edit permissions. If a backend action is not available yet, the UI must not render a fake action button. M6.9 should expose current facts clearly before adding new workflow mutations.

## File Map

### Backend

- Modify `apps/server/cmd/server/main.go`
  - Register `GET /api/agent/workspaces/:workspaceID/production-overview`.
- Modify `apps/server/sqlc/queries/agent_task.sql`
  - Add `ListAgentTasksByWorkspace`.
- Modify `apps/server/sqlc/queries/agent_event.sql`
  - Add `ListAgentEventsByWorkspace`.
- Modify `apps/server/sqlc/queries/production.sql`
  - Add workspace-level job/version list queries if existing node-scoped queries are too ncustom edge.
- Modify `apps/server/sqlc/queries/sandbox_job.sql`
  - Add `ListSandboxJobsByWorkspace`.
- Run `make sqlc-generate`.
- Create `apps/server/internal/agent/overview/types.go`
  - Typed response model for production overview.
- Create `apps/server/internal/agent/overview/builder.go`
  - DB-backed overview projection.
- Create `apps/server/internal/agent/overview/builder_test.go`
  - Pure builder tests using fake store.
- Modify `apps/server/internal/api/agent_handler.go`
  - Add `GetProductionOverview`.
- Modify `apps/server/internal/api/agent_handler_test.go`
  - Route-level tests for Agent/Studio authorization and response node.

### Frontend Data

- Modify `apps/web/src/lib/api.ts`
  - Add `AgentProductionOverview` types and `fetchAgentProductionOverview`.
- Create `apps/web/src/lib/agentProductionOverview.ts`
  - Pure helpers for phase label, progress summary, timeline label mapping, and websocket invalidation policy.
- Create `apps/web/src/lib/agentProductionOverview.test.mjs`
  - Unit tests for projection behavior.

### Frontend Components

- Create `apps/web/src/components/agent/AgentProductionStatusBar.tsx`
  - Compact current phase and counts.
- Create `apps/web/src/components/agent/AgentStoryboardPanel.tsx`
  - Persistent storyboard section.
- Create `apps/web/src/components/agent/AgentTaskTimeline.tsx`
  - Timeline with collapsed diagnostics.
- Modify `apps/web/src/components/agent/AgentFinalVideoCardBlock.tsx`
  - Polish final card states and confirmation affordance display.
- Modify `apps/web/src/components/agent/AgentReviewCardBlock.tsx`
  - Compact rubric/retry display and node ref presentation.
- Modify `apps/web/src/components/agent/AgentNodeDetailDrawer.tsx`
  - Add shot context, retry chain, composition trace and sandbox trace sections.
- Modify `apps/web/src/pages/AgentWorkspacePage.tsx`
  - Fetch overview, render status/storyboard/timeline, invalidate overview on websocket events.
- Modify `apps/web/src/main.css`
  - Style status bar, storyboard rows, timeline, detail sections, and final video states.

### Smoke / E2E

- Create `scripts/smoke-m6-9-ux-completion.sh`
  - API smoke that creates an Agent workspace, sends a terminal UX request, and prints verification commands.

---

## Task 1: Backend Overview Builder

**Files:**
- Create: `apps/server/internal/agent/overview/types.go`
- Create: `apps/server/internal/agent/overview/builder.go`
- Create: `apps/server/internal/agent/overview/builder_test.go`
- Modify: `apps/server/sqlc/queries/agent_task.sql`
- Modify: `apps/server/sqlc/queries/agent_event.sql`
- Modify: `apps/server/sqlc/queries/sandbox_job.sql`
- Generated after `make sqlc-generate`: `apps/server/internal/store/db/*.sql.go`

- [ ] **Step 1: Write failing builder tests**

Create `apps/server/internal/agent/overview/builder_test.go` with tests for phase, counts, shot summaries, final outputs, and timeline labels.

```go
package overview

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestBuilderProjectsWaitingDecisionAndStoryboard(t *testing.T) {
	workspaceID := uuidWithByte(1)
	shotID := uuidWithByte(2)
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: workspaceID, Name: "agent", Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{{
			ID:          shotID,
			WorkspaceID: workspaceID,
			ClientKey:   "shot-01",
			SortOrder:   1,
			Title:       "开场",
			Status:      "preview_ready",
		}},
		events: []db.AgentEvent{{
			ID:          uuidWithByte(3),
			WorkspaceID: workspaceID,
			EventType:   "decision_requested",
			Status:      "pending",
		}},
		nodes: []db.MediaNode{{
			ID:          uuidWithByte(4),
			WorkspaceID: workspaceID,
			ShotID:      shotID,
			NodeType:    db.NodeTypeImage,
			Source:      "agent",
			Status:      db.NodeStatusSucceeded,
			Metadata:    []byte(`{"agent_artifact_kind":"preview_image"}`),
		}},
	})

	got, err := builder.Build(context.Background(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != PhaseWaitingConfirmation {
		t.Fatalf("phase = %q, want %q", got.Phase, PhaseWaitingConfirmation)
	}
	if got.Counts.WaitingDecisions != 1 || got.Counts.ShotsTotal != 1 || got.Counts.PreviewsReady != 1 {
		t.Fatalf("counts = %#v", got.Counts)
	}
	if len(got.Shots) != 1 || got.Shots[0].ClientKey != "shot-01" || got.Shots[0].PreviewStatus != "ready" {
		t.Fatalf("shots = %#v", got.Shots)
	}
}

func TestBuilderProjectsTimelineUserLabels(t *testing.T) {
	workspaceID := uuidWithByte(1)
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: workspaceID, Name: "agent", Mode: db.WorkspaceModeAgent},
		tasks: []db.AgentTask{{
			ID:          uuidWithByte(2),
			WorkspaceID: workspaceID,
			TaskType:    "composer_turn",
			Role:        "composer",
			Status:      "running",
		}},
		events: []db.AgentEvent{{
			ID:          uuidWithByte(3),
			WorkspaceID: workspaceID,
			EventType:   "composition_succeeded",
			Status:      "pending",
		}},
	})

	got, err := builder.Build(context.Background(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Timeline) < 2 {
		t.Fatalf("timeline = %#v", got.Timeline)
	}
	if got.Timeline[0].Label == "composer_turn" || got.Timeline[1].Label == "composition_succeeded" {
		t.Fatalf("timeline leaked internal labels = %#v", got.Timeline)
	}
}
```

The fake store in the same test file must implement the store interface and return seeded rows. Use the local `uuidWithByte` helper pattern used by adjacent Agent tests.

- [ ] **Step 2: Run tests and verify they fail**

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/overview -count=1
```

Expected: FAIL because `internal/agent/overview` does not exist.

- [ ] **Step 3: Add sqlc queries**

Append to `apps/server/sqlc/queries/agent_task.sql`:

```sql
-- name: ListAgentTasksByWorkspace :many
SELECT *
FROM agent_task
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2;
```

Append to `apps/server/sqlc/queries/agent_event.sql`:

```sql
-- name: ListAgentEventsByWorkspace :many
SELECT *
FROM agent_event
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2;
```

Append to `apps/server/sqlc/queries/sandbox_job.sql`:

```sql
-- name: ListSandboxJobsByWorkspace :many
SELECT *
FROM sandbox_job
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2;
```

- [ ] **Step 4: Generate sqlc code**

```bash
make sqlc-generate
```

Expected: generated db methods exist:

- `ListAgentTasksByWorkspace`
- `ListAgentEventsByWorkspace`
- `ListSandboxJobsByWorkspace`

- [ ] **Step 5: Add overview types**

Create `apps/server/internal/agent/overview/types.go`:

```go
package overview

type Phase string

const (
	PhasePlanning            Phase = "planning"
	PhasePreview             Phase = "preview"
	PhaseReview              Phase = "review"
	PhaseVideo               Phase = "video"
	PhaseFinal               Phase = "final"
	PhaseWaitingConfirmation Phase = "waiting_confirmation"
	PhaseComplete            Phase = "complete"
	PhaseNeedsAttention      Phase = "needs_attention"
	PhaseError               Phase = "error"
)

type ProductionOverview struct {
	WorkspaceID  string               `json:"workspace_id"`
	Phase        Phase                `json:"phase"`
	Counts       Counts               `json:"counts"`
	Shots        []ShotSummary        `json:"shots"`
	Timeline     []TimelineItem       `json:"timeline"`
	FinalOutputs []FinalOutputSummary `json:"final_outputs"`
	Diagnostics  map[string]any       `json:"diagnostics"`
}

type Counts struct {
	ActiveTasks       int `json:"active_tasks"`
	FailedTasks       int `json:"failed_tasks"`
	WaitingDecisions  int `json:"waiting_decisions"`
	ShotsTotal        int `json:"shots_total"`
	PreviewsReady     int `json:"previews_ready"`
	VideosReady       int `json:"videos_ready"`
	FinalOutputsReady int `json:"final_outputs_ready"`
}

type ShotSummary struct {
	ID            string   `json:"id"`
	ClientKey     string   `json:"client_key"`
	SortOrder     int32    `json:"sort_order"`
	Title         string   `json:"title"`
	DurationSec   *float64 `json:"duration_sec,omitempty"`
	Status        string   `json:"status"`
	PreviewStatus string   `json:"preview_status"`
	ReviewStatus  string   `json:"review_status"`
	VideoStatus   string   `json:"video_status"`
	BlockedReason string   `json:"blocked_reason,omitempty"`
	NodeIDs        []string `json:"node_ids"`
}

type TimelineItem struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Label       string         `json:"label"`
	Status      string         `json:"status"`
	Summary     string         `json:"summary,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
	Diagnostics map[string]any `json:"diagnostics,omitempty"`
}

type FinalOutputSummary struct {
	NodeID       string   `json:"node_id"`
	VersionID    string   `json:"version_id,omitempty"`
	AssetID      string   `json:"asset_id,omitempty"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	URL          string   `json:"url,omitempty"`
	SourceShots  []string `json:"source_shots"`
	DecisionID   string   `json:"decision_id,omitempty"`
}
```

- [ ] **Step 6: Add builder implementation**

Create `apps/server/internal/agent/overview/builder.go` with:

```go
package overview

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Store interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	ListActiveShotsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Shot, error)
	ListShotDependenciesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.ShotDependency, error)
	ListMediaNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
	ListAgentTasksByWorkspace(ctx context.Context, params db.ListAgentTasksByWorkspaceParams) ([]db.AgentTask, error)
	ListAgentEventsByWorkspace(ctx context.Context, params db.ListAgentEventsByWorkspaceParams) ([]db.AgentEvent, error)
	ListReviewRecordsByWorkspace(ctx context.Context, params db.ListReviewRecordsByWorkspaceParams) ([]db.ReviewRecord, error)
	ListGenerationJobsByNode(ctx context.Context, nodeID pgtype.UUID) ([]db.GenerationJob, error)
	ListArtifactVersionsByNode(ctx context.Context, nodeID pgtype.UUID) ([]db.ArtifactVersion, error)
	ListSandboxJobsByWorkspace(ctx context.Context, params db.ListSandboxJobsByWorkspaceParams) ([]db.SandboxJob, error)
}

type Builder struct {
	store Store
}

func NewBuilder(store Store) *Builder {
	return &Builder{store: store}
}

func (b *Builder) Build(ctx context.Context, workspaceID pgtype.UUID) (ProductionOverview, error) {
	workspace, err := b.store.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return ProductionOverview{}, err
	}
	shots, err := b.store.ListActiveShotsByWorkspace(ctx, workspaceID)
	if err != nil {
		return ProductionOverview{}, err
	}
	nodes, err := b.store.ListMediaNodesByWorkspace(ctx, workspaceID)
	if err != nil {
		return ProductionOverview{}, err
	}
	tasks, err := b.store.ListAgentTasksByWorkspace(ctx, db.ListAgentTasksByWorkspaceParams{WorkspaceID: workspaceID, Limit: 200})
	if err != nil {
		return ProductionOverview{}, err
	}
	events, err := b.store.ListAgentEventsByWorkspace(ctx, db.ListAgentEventsByWorkspaceParams{WorkspaceID: workspaceID, Limit: 200})
	if err != nil {
		return ProductionOverview{}, err
	}
	reviews, err := b.store.ListReviewRecordsByWorkspace(ctx, db.ListReviewRecordsByWorkspaceParams{WorkspaceID: workspaceID, Limit: 200})
	if err != nil {
		return ProductionOverview{}, err
	}
	sandboxJobs, err := b.store.ListSandboxJobsByWorkspace(ctx, db.ListSandboxJobsByWorkspaceParams{WorkspaceID: workspaceID, Limit: 200})
	if err != nil {
		return ProductionOverview{}, err
	}
	sort.Slice(shots, func(i, j int) bool { return shots[i].SortOrder < shots[j].SortOrder })

	counts := countsFrom(shots, nodes, tasks, events)
	return ProductionOverview{
		WorkspaceID:  uuidString(workspace.ID),
		Phase:        phaseFrom(counts, nodes, tasks, events),
		Counts:       counts,
		Shots:        shotSummaries(shots, nodes, reviews),
		Timeline:     timelineItems(tasks, events, sandboxJobs),
		FinalOutputs: finalOutputSummaries(nodes),
		Diagnostics:  map[string]any{"workspace_mode": string(workspace.Mode)},
	}, nil
}
```

Then add helpers in the same file:

- `countsFrom`
- `phaseFrom`
- `shotSummaries`
- `timelineItems`
- `finalOutputSummaries`
- `artifactKind`
- `uuidString`
- `eventLabel`
- `taskLabel`

Use this user-facing label map:

```go
var taskLabels = map[string]string{
	"producer_turn":     "理解需求",
	"craftsman_turn":   "准备生成方案",
	"worker_generation": "提交生成任务",
	"reviewer_turn":    "评审画面",
	"composer_turn":    "合成成片",
	"tool_call":        "执行操作",
}

var eventLabels = map[string]string{
	"producer_turn_queued":          "理解需求",
	"tool_call_started":            "开始执行操作",
	"tool_call_completed":          "操作完成",
	"storyboard_updated":           "更新分镜",
	"craftsman_dispatched":         "开始生成预览",
	"preview_generation_succeeded": "预览完成",
	"preview_generation_failed":    "预览失败",
	"review_started":               "开始评审",
	"review_accepted":              "评审通过",
	"review_rejected":              "需要重试",
	"retry_requested":              "准备重新生成",
	"shot_video_succeeded":         "分镜视频完成",
	"shot_video_failed":            "分镜视频失败",
	"composition_succeeded":        "成片完成",
	"composition_failed":           "成片失败",
	"decision_requested":           "请求确认",
}
```

- [ ] **Step 7: Run overview package tests**

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/overview -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit checkpoint**

Do not commit unless the user has asked for commits in this session. If committing is approved:

```bash
git add apps/server/sqlc/queries/agent_task.sql apps/server/sqlc/queries/agent_event.sql apps/server/sqlc/queries/sandbox_job.sql apps/server/internal/store/db apps/server/internal/agent/overview
git commit -m "feat: add agent production overview projection"
```

## Task 2: Agent Production Overview API

**Files:**
- Modify: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/server/internal/api/agent_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Write failing API tests**

Add tests to `apps/server/internal/api/agent_handler_test.go`:

```go
func TestAgentProductionOverviewRejectsStudioWorkspace(t *testing.T) {
	workspace := db.Workspace{
		ID:   testUUID(0x41),
		Name: "studio",
		Mode: db.WorkspaceModeStudio,
	}
	if workspace.Mode == db.WorkspaceModeAgent {
		t.Fatal("test fixture must be a studio workspace")
	}

	status := exerciseAgentProductionOverviewRoute(t, workspace, nil)
	if status != 404 && status != 403 {
		t.Fatalf("status = %d, want 404 or 403", status)
	}
}

func TestAgentProductionOverviewReturnsStructuredState(t *testing.T) {
	workspace := db.Workspace{
		ID:   testUUID(0x42),
		Name: "agent",
		Mode: db.WorkspaceModeAgent,
	}
	body := exerciseAgentProductionOverviewJSON(t, workspace, overviewFixture{
		shots: []db.Shot{{
			ID:          testUUID(0x43),
			WorkspaceID: workspace.ID,
			ClientKey:   "shot-01",
			SortOrder:   1,
			Title:       "开场",
			Status:      "preview_ready",
		}},
		events: []db.AgentEvent{{
			ID:          testUUID(0x44),
			WorkspaceID: workspace.ID,
			EventType:   "decision_requested",
			Status:      "pending",
		}},
	})

	if body["workspace_id"] == "" || body["phase"] == "" {
		t.Fatalf("overview identity fields missing: %#v", body)
	}
	if _, ok := body["counts"].(map[string]any); !ok {
		t.Fatalf("counts missing: %#v", body)
	}
	if _, ok := body["shots"].([]any); !ok {
		t.Fatalf("shots missing: %#v", body)
	}
	if _, ok := body["timeline"].([]any); !ok {
		t.Fatalf("timeline missing: %#v", body)
	}
	if _, ok := body["final_outputs"].([]any); !ok {
		t.Fatalf("final_outputs missing: %#v", body)
	}
}
```

Add local test helpers in `agent_handler_test.go` for `exerciseAgentProductionOverviewRoute`, `exerciseAgentProductionOverviewJSON`, and `overviewFixture`. The helpers must build an in-memory Hertz route with `AgentHandler.GetProductionOverview`, seed the minimal workspace rows needed by `agentWorkspaceForRequest`, and avoid changing production code only for test setup.

- [ ] **Step 2: Run API tests and verify failure**

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestAgentProductionOverview' -count=1
```

Expected: FAIL because the route and handler do not exist.

- [ ] **Step 3: Add handler method**

In `apps/server/internal/api/agent_handler.go`, import:

```go
agentoverview "github.com/sinmaystar/clip-anvil/internal/agent/overview"
```

Add:

```go
func (h *AgentHandler) GetProductionOverview(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}
	builder := agentoverview.NewBuilder(h.queries)
	overview, err := builder.Build(ctx, workspace.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load agent production overview")
		return
	}
	c.JSON(consts.StatusOK, overview)
}
```

- [ ] **Step 4: Register route**

In `apps/server/cmd/server/main.go`, register near other Agent routes:

```go
h.GET("/api/agent/workspaces/:workspaceID/production-overview", authMiddleware, agentHandler.GetProductionOverview)
```

- [ ] **Step 5: Run API tests**

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestAgentProductionOverview' -count=1
```

Expected: PASS.

## Task 3: Frontend API Types And Pure Projection Helpers

**Files:**
- Modify: `apps/web/src/lib/api.ts`
- Create: `apps/web/src/lib/agentProductionOverview.ts`
- Create: `apps/web/src/lib/agentProductionOverview.test.mjs`

- [ ] **Step 1: Write failing frontend tests**

Create `apps/web/src/lib/agentProductionOverview.test.mjs`:

```js
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentPhaseLabel,
  overviewProgressText,
  shouldRefreshOverviewForAgentEvent,
  timelineItemLabel,
} from "../../dist-test/lib/agentProductionOverview.js";

describe("agent production overview", () => {
  it("formats current phase labels without internal role names", () => {
    assert.equal(agentPhaseLabel("preview"), "生成预览");
    assert.equal(agentPhaseLabel("waiting_confirmation"), "等待确认");
    assert.equal(agentPhaseLabel("final"), "合成成片");
  });

  it("formats compact progress", () => {
    assert.equal(
      overviewProgressText({
        shots_total: 3,
        previews_ready: 2,
        videos_ready: 1,
        final_outputs_ready: 0,
      }),
      "预览 2/3 · 视频 1/3 · 成片 0/1",
    );
  });

  it("maps timeline internals to user labels", () => {
    assert.equal(timelineItemLabel({ kind: "task", type: "composer_turn" }), "合成成片");
    assert.equal(timelineItemLabel({ kind: "event", type: "review_rejected" }), "需要重试");
  });

  it("refreshes overview for task event production and canvas messages", () => {
    for (const type of [
      "agent.task.updated",
      "agent.event.created",
      "agent.message.created",
      "agent.message.updated",
      "production.job.updated",
      "NodeCreated",
      "NodeUpdated",
    ]) {
      assert.equal(shouldRefreshOverviewForAgentEvent(type), true, type);
    }
    assert.equal(shouldRefreshOverviewForAgentEvent("unknown"), false);
  });
});
```

- [ ] **Step 2: Run frontend test and verify failure**

```bash
pnpm --filter @clip-anvil/web test:connections -- agentProductionOverview
```

Expected: FAIL because `agentProductionOverview` module does not exist.

- [ ] **Step 3: Add API types**

In `apps/web/src/lib/api.ts`, add:

```ts
export type AgentProductionPhase =
  | "planning"
  | "preview"
  | "review"
  | "video"
  | "final"
  | "waiting_confirmation"
  | "complete"
  | "needs_attention"
  | "error";

export interface AgentProductionCounts {
  active_tasks: number;
  failed_tasks: number;
  waiting_decisions: number;
  shots_total: number;
  previews_ready: number;
  videos_ready: number;
  final_outputs_ready: number;
}

export interface AgentShotOverview {
  id: string;
  client_key: string;
  sort_order: number;
  title: string;
  duration_sec?: number;
  status: string;
  preview_status: string;
  review_status: string;
  video_status: string;
  blocked_reason?: string;
  node_ids: string[];
}

export interface AgentTimelineItem {
  id: string;
  kind: string;
  label: string;
  status: string;
  summary?: string;
  created_at?: string;
  diagnostics?: Record<string, unknown>;
}

export interface AgentFinalOutputOverview {
  node_id: string;
  version_id?: string;
  asset_id?: string;
  title: string;
  status: string;
  url?: string;
  source_shots: string[];
  decision_id?: string;
}

export interface AgentProductionOverview {
  workspace_id: string;
  phase: AgentProductionPhase;
  counts: AgentProductionCounts;
  shots: AgentShotOverview[];
  timeline: AgentTimelineItem[];
  final_outputs: AgentFinalOutputOverview[];
  diagnostics: Record<string, unknown>;
}
```

Add API function near other Agent API functions:

```ts
export function fetchAgentProductionOverview(workspaceId: string) {
  return apiFetch<AgentProductionOverview>(
    `/agent/workspaces/${workspaceId}/production-overview`,
  );
}
```

- [ ] **Step 4: Add pure helper implementation**

Create `apps/web/src/lib/agentProductionOverview.ts`:

```ts
import type {
  AgentProductionCounts,
  AgentProductionPhase,
} from "./api";

const phaseLabels: Record<AgentProductionPhase, string> = {
  planning: "规划中",
  preview: "生成预览",
  review: "评审中",
  video: "生成视频",
  final: "合成成片",
  waiting_confirmation: "等待确认",
  complete: "已完成",
  needs_attention: "需要处理",
  error: "出错",
};

const timelineLabels: Record<string, string> = {
  producer_turn: "理解需求",
  craftsman_turn: "准备生成方案",
  worker_generation: "提交生成任务",
  reviewer_turn: "评审画面",
  composer_turn: "合成成片",
  tool_call: "执行操作",
  producer_turn_queued: "理解需求",
  tool_call_started: "开始执行操作",
  tool_call_completed: "操作完成",
  storyboard_updated: "更新分镜",
  craftsman_dispatched: "开始生成预览",
  preview_generation_succeeded: "预览完成",
  preview_generation_failed: "预览失败",
  review_started: "开始评审",
  review_accepted: "评审通过",
  review_rejected: "需要重试",
  retry_requested: "准备重新生成",
  shot_video_succeeded: "分镜视频完成",
  shot_video_failed: "分镜视频失败",
  composition_succeeded: "成片完成",
  composition_failed: "成片失败",
  decision_requested: "请求确认",
};

const overviewRefreshEvents = new Set([
  "agent.task.updated",
  "agent.event.created",
  "agent.message.created",
  "agent.message.updated",
  "production.job.updated",
  "NodeCreated",
  "NodeUpdated",
]);

export function agentPhaseLabel(phase: AgentProductionPhase) {
  return phaseLabels[phase] ?? "处理中";
}

export function overviewProgressText(counts: Pick<AgentProductionCounts, "shots_total" | "previews_ready" | "videos_ready" | "final_outputs_ready">) {
  const total = Math.max(0, counts.shots_total);
  return `预览 ${counts.previews_ready}/${total} · 视频 ${counts.videos_ready}/${total} · 成片 ${counts.final_outputs_ready}/1`;
}

export function timelineItemLabel(item: { kind: string; type: string }) {
  return timelineLabels[item.type] ?? "处理任务";
}

export function shouldRefreshOverviewForAgentEvent(type: string) {
  return overviewRefreshEvents.has(type);
}
```

- [ ] **Step 5: Run frontend helper tests**

```bash
pnpm --filter @clip-anvil/web test:connections -- agentProductionOverview
```

Expected: PASS.

## Task 4: Status Bar, Storyboard And Timeline Components

**Files:**
- Create: `apps/web/src/components/agent/AgentProductionStatusBar.tsx`
- Create: `apps/web/src/components/agent/AgentStoryboardPanel.tsx`
- Create: `apps/web/src/components/agent/AgentTaskTimeline.tsx`
- Modify: `apps/web/src/lib/agentReadonlyCanvas.test.mjs` or create component source tests if preferred
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Write failing source tests for component contracts**

Append to `apps/web/src/lib/agentReadonlyCanvas.test.mjs` or create `apps/web/src/lib/agentUxCompletion.test.mjs`:

```js
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { describe, it } from "node:test";
import { URL } from "node:url";

describe("agent ux completion components", () => {
  it("declares persistent status storyboard and timeline components", async () => {
    for (const file of [
      "../components/agent/AgentProductionStatusBar.tsx",
      "../components/agent/AgentStoryboardPanel.tsx",
      "../components/agent/AgentTaskTimeline.tsx",
    ]) {
      const source = await readFile(new URL(file, import.meta.url), "utf8");
      assert.match(source, /export function Agent/);
    }
  });

  it("keeps timeline diagnostics collapsed by default", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentTaskTimeline.tsx", import.meta.url),
      "utf8",
    );
    assert.match(source, /<details/);
    assert.match(source, /诊断/);
  });
});
```

- [ ] **Step 2: Run frontend test and verify failure**

```bash
pnpm --filter @clip-anvil/web test:connections -- agentUxCompletion
```

Expected: FAIL because components do not exist.

- [ ] **Step 3: Create `AgentProductionStatusBar`**

Create `apps/web/src/components/agent/AgentProductionStatusBar.tsx`:

```tsx
import type { AgentProductionOverview } from "../../lib/api";
import { agentPhaseLabel, overviewProgressText } from "../../lib/agentProductionOverview";

export function AgentProductionStatusBar({
  overview,
  connectionStatus,
}: {
  overview: AgentProductionOverview | null;
  connectionStatus: "connected" | "connecting" | "reconnecting" | "offline";
}) {
  if (!overview) {
    return <section className="agent-production-status-bar">正在读取生产状态</section>;
  }
  return (
    <section className={`agent-production-status-bar agent-production-status-${overview.phase}`}>
      <strong>{agentPhaseLabel(overview.phase)}</strong>
      <span>{overviewProgressText(overview.counts)}</span>
      <small>{overview.counts.active_tasks} 运行 · {overview.counts.waiting_decisions} 等待确认 · {overview.counts.failed_tasks} 失败</small>
      <i aria-label={connectionStatus} />
    </section>
  );
}
```

- [ ] **Step 4: Create `AgentStoryboardPanel`**

Create `apps/web/src/components/agent/AgentStoryboardPanel.tsx`:

```tsx
import type { AgentProductionOverview } from "../../lib/api";

export function AgentStoryboardPanel({
  overview,
  onFocusNode,
  onPrefill,
}: {
  overview: AgentProductionOverview | null;
  onFocusNode: (nodeId: string) => void;
  onPrefill: (text: string) => void;
}) {
  const shots = overview?.shots ?? [];
  return (
    <section className="agent-storyboard-panel">
      <header>
        <strong>分镜</strong>
        <span>{shots.length} 个</span>
      </header>
      {shots.length === 0 ? (
        <p>还没有分镜。</p>
      ) : (
        <div className="agent-storyboard-list">
          {shots.map((shot) => (
            <article className="agent-storyboard-row" key={shot.id}>
              <div>
                <b>{shot.client_key}</b>
                <strong>{shot.title || "未命名分镜"}</strong>
                {typeof shot.duration_sec === "number" ? <small>{shot.duration_sec}s</small> : null}
              </div>
              <div className="agent-storyboard-statuses">
                <span>预览 {statusText(shot.preview_status)}</span>
                <span>评审 {statusText(shot.review_status)}</span>
                <span>视频 {statusText(shot.video_status)}</span>
              </div>
              {shot.blocked_reason ? <p>{shot.blocked_reason}</p> : null}
              <footer>
                <button type="button" onClick={() => onPrefill(`请修改 ${shot.client_key}：`)}>
                  要求修改
                </button>
                {shot.node_ids[0] ? (
                  <button type="button" onClick={() => onFocusNode(shot.node_ids[0])}>
                    查看节点
                  </button>
                ) : null}
              </footer>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}

function statusText(status: string) {
  if (!status || status === "none") return "无";
  if (status === "ready" || status === "accepted" || status === "succeeded") return "完成";
  if (status === "running" || status === "queued") return "进行中";
  if (status === "failed" || status === "rejected") return "需处理";
  return status;
}
```

- [ ] **Step 5: Create `AgentTaskTimeline`**

Create `apps/web/src/components/agent/AgentTaskTimeline.tsx`:

```tsx
import type { AgentProductionOverview } from "../../lib/api";

export function AgentTaskTimeline({ overview }: { overview: AgentProductionOverview | null }) {
  const items = overview?.timeline ?? [];
  return (
    <section className="agent-task-timeline">
      <header>
        <strong>进度</strong>
        <span>{items.length} 条</span>
      </header>
      {items.length === 0 ? (
        <p>还没有后台任务。</p>
      ) : (
        <ol>
          {items.slice(0, 20).map((item) => (
            <li key={item.id}>
              <div>
                <strong>{item.label}</strong>
                <span>{statusText(item.status)}</span>
              </div>
              {item.summary ? <p>{item.summary}</p> : null}
              <details>
                <summary>诊断</summary>
                <pre>{JSON.stringify(item.diagnostics ?? {}, null, 2)}</pre>
              </details>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

function statusText(status: string) {
  if (status === "succeeded" || status === "handled") return "完成";
  if (status === "queued" || status === "running" || status === "pending") return "进行中";
  if (status === "failed" || status === "cancelled") return "失败";
  return status || "未知";
}
```

- [ ] **Step 6: Add CSS**

Append focused styles to `apps/web/src/main.css`:

```css
.agent-production-status-bar,
.agent-storyboard-panel,
.agent-task-timeline {
  border-bottom: 1px solid var(--border-subtle);
  padding: 10px 12px;
}

.agent-production-status-bar {
  display: grid;
  gap: 4px;
}

.agent-production-status-bar strong,
.agent-storyboard-panel strong,
.agent-task-timeline strong {
  color: var(--fg-primary);
  font-size: 12px;
}

.agent-production-status-bar span,
.agent-production-status-bar small,
.agent-storyboard-panel p,
.agent-task-timeline p {
  color: var(--fg-secondary);
  font-size: 11px;
}

.agent-storyboard-list,
.agent-task-timeline ol {
  display: grid;
  gap: 8px;
  margin: 8px 0 0;
  padding: 0;
}

.agent-storyboard-row,
.agent-task-timeline li {
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--fg-primary) 4%, transparent);
  list-style: none;
  padding: 8px;
}
```

- [ ] **Step 7: Run frontend tests**

```bash
pnpm --filter @clip-anvil/web test:connections -- agentUxCompletion
```

Expected: PASS.

## Task 5: Wire Overview Into Agent Workspace Page

**Files:**
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/lib/agentReadonlyCanvas.test.mjs`

- [ ] **Step 1: Write failing source test**

Add to `apps/web/src/lib/agentReadonlyCanvas.test.mjs`:

```js
it("agent workspace renders production overview sections and refreshes them from websocket events", async () => {
  const source = await readFile(
    new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
    "utf8",
  );

  assert.match(source, /fetchAgentProductionOverview/);
  assert.match(source, /AgentProductionStatusBar/);
  assert.match(source, /AgentStoryboardPanel/);
  assert.match(source, /AgentTaskTimeline/);
  assert.match(source, /shouldRefreshOverviewForAgentEvent/);
  assert.match(source, /\["agent",\s*id,\s*"production-overview"\]/);
});
```

- [ ] **Step 2: Run frontend test and verify failure**

```bash
pnpm --filter @clip-anvil/web test:connections -- agent readonly
```

Expected: FAIL because imports/usages are absent.

- [ ] **Step 3: Import API/helper/components**

In `apps/web/src/pages/AgentWorkspacePage.tsx`, add imports:

```ts
import {
  fetchAgentProductionOverview,
  type AgentProductionOverview,
} from "../lib/api";
import { shouldRefreshOverviewForAgentEvent } from "../lib/agentProductionOverview";
import { AgentProductionStatusBar } from "../components/agent/AgentProductionStatusBar";
import { AgentStoryboardPanel } from "../components/agent/AgentStoryboardPanel";
import { AgentTaskTimeline } from "../components/agent/AgentTaskTimeline";
```

- [ ] **Step 4: Add overview query**

Near other Agent queries:

```tsx
const productionOverviewQuery = useQuery({
  queryKey: ["agent", id, "production-overview"],
  queryFn: () => fetchAgentProductionOverview(id),
  enabled: Boolean(id),
  refetchInterval: agentBusy ? 5000 : false,
});

const productionOverview = productionOverviewQuery.data ?? null;
```

- [ ] **Step 5: Refresh overview from websocket events**

In the Agent websocket callback and canvas websocket callback, after reading `event.type`, add:

```ts
if (shouldRefreshOverviewForAgentEvent(event.type)) {
  void queryClient.invalidateQueries({
    queryKey: ["agent", id, "production-overview"],
  });
}
```

Keep existing canvas/message/task cache updates.

- [ ] **Step 6: Render persistent sections**

Inside `.agent-chat-float`, after `.agent-chat-header`, render:

```tsx
<AgentProductionStatusBar
  connectionStatus={connectionStatus}
  overview={productionOverview}
/>
<AgentStoryboardPanel
  overview={productionOverview}
  onFocusNode={setSelectedNodeId}
  onPrefill={(text) => {
    setDraft(text);
    requestAnimationFrame(() => {
      composerTextareaRef.current?.focus();
    });
  }}
/>
<AgentTaskTimeline overview={productionOverview} />
```

If there is no existing textarea ref, create:

```ts
const composerTextareaRef = useRef<HTMLTextAreaElement | null>(null);
```

and attach it to the composer `<textarea ref={composerTextareaRef} ... />`.

- [ ] **Step 7: Run frontend tests**

```bash
pnpm --filter @clip-anvil/web test:connections -- agent readonly
```

Expected: PASS.

## Task 6: Final And Review Card Polish

**Files:**
- Modify: `apps/web/src/components/agent/AgentFinalVideoCardBlock.tsx`
- Modify: `apps/web/src/components/agent/AgentReviewCardBlock.tsx`
- Modify: `apps/web/src/lib/agentMessageBlocks.test.mjs`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Write failing tests**

Extend `apps/web/src/lib/agentMessageBlocks.test.mjs`:

```js
it("guards accepted and revision requested final video cards", () => {
  assert.equal(
    isFinalVideoCardBlock({
      id: "blk_final",
      type: "final_video_card",
      status: "accepted",
      node_id: "node-1",
      version_id: "version-1",
      asset_id: "asset-1",
      title: "成片",
      source_shots: ["shot-01"],
    }),
    true,
  );
  assert.equal(
    isFinalVideoCardBlock({
      id: "blk_final_revision",
      type: "final_video_card",
      status: "revision_requested",
      node_id: "node-1",
      version_id: "version-1",
      asset_id: "asset-1",
      title: "成片",
      source_shots: ["shot-01"],
    }),
    true,
  );
});
```

- [ ] **Step 2: Run message block tests and verify failure**

```bash
pnpm --filter @clip-anvil/web test:connections -- agentMessageBlocks
```

Expected: FAIL because `AgentFinalVideoCardBlock["status"]` does not include accepted/revision_requested.

- [ ] **Step 3: Extend final video status type**

In `apps/web/src/lib/agentMessageBlocks.ts`, update final video status union:

```ts
status:
  | "queued"
  | "running"
  | "ready"
  | "failed"
  | "waiting_for_confirmation"
  | "accepted"
  | "revision_requested";
```

- [ ] **Step 4: Polish final video card display**

In `AgentFinalVideoCardBlock.tsx`, update `statusLabel`:

```ts
case "accepted":
  return "已确认";
case "revision_requested":
  return "已请求修改";
```

Render output metadata only if provided through future-compatible optional fields. Do not invent values.

- [ ] **Step 5: Polish review card**

In `AgentReviewCardBlock.tsx`, render a compact rubric list if `block.rubric` contains object entries:

```tsx
const rubricEntries = Object.entries(block.rubric).slice(0, 6);
```

Each entry should show axis name and score/pass when present. Keep raw JSON in a collapsed `details` block for diagnostics.

- [ ] **Step 6: Run frontend tests**

```bash
pnpm --filter @clip-anvil/web test:connections -- agentMessageBlocks
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

## Task 7: Node Detail Trace Enhancements

**Files:**
- Modify: `apps/web/src/components/agent/AgentNodeDetailDrawer.tsx`
- Modify: `apps/web/src/lib/agentReadonlyCanvas.test.mjs`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Write failing source test**

Add to `apps/web/src/lib/agentReadonlyCanvas.test.mjs`:

```js
it("node detail exposes M6.9 trace sections", async () => {
  const source = await readFile(
    new URL("../components/agent/AgentNodeDetailDrawer.tsx", import.meta.url),
    "utf8",
  );

  for (const label of [
    "Shot Context",
    "Production Trace",
    "Retry Chain",
    "Final Composition",
    "Sandbox",
  ]) {
    assert.match(source, new RegExp(label));
  }
  assert.match(source, /sandbox_jobs/);
  assert.match(source, /review_records/);
});
```

- [ ] **Step 2: Run test and verify failure**

```bash
pnpm --filter @clip-anvil/web test:connections -- agent readonly
```

Expected: FAIL because detail drawer lacks the new section labels.

- [ ] **Step 3: Add detail sections**

In `AgentNodeDetailDrawer.tsx`, keep existing sections and add:

```tsx
<section className="agent-node-detail-section">
  <p>Shot Context</p>
  <dl className="agent-node-detail-list">
    <DetailRow label="Shot ID" value={node.shot_id ?? "未关联"} />
    <DetailRow label="输入依赖" value={formatNodeNames(upstream)} />
    <DetailRow label="下游节点" value={formatNodeNames(downstream)} />
  </dl>
</section>

<section className="agent-node-detail-section">
  <p>Production Trace</p>
  <dl className="agent-node-detail-list">
    <DetailRow label="Latest Job" value={latestJob?.id ?? "无"} />
    <DetailRow label="Current Version" value={currentVersion?.id ?? "无"} />
    <DetailRow label="Stale Reasons" value={`${productionState?.active_stale_reasons?.length ?? 0}`} />
  </dl>
</section>

<section className="agent-node-detail-section">
  <p>Retry Chain</p>
  {productionState?.review_records?.length ? (
    <span className="agent-node-detail-empty">
      {productionState.review_records.length} 条评审 / 重试相关记录
    </span>
  ) : (
    <span className="agent-node-detail-empty">暂无重试链路。</span>
  )}
</section>

<section className="agent-node-detail-section">
  <p>Final Composition</p>
  <span className="agent-node-detail-empty">
    {node.operation_type === "compose_final_video" ? "该节点是成片输出。" : "该节点不是成片输出。"}
  </span>
</section>

<section className="agent-node-detail-section">
  <p>Sandbox</p>
  {productionState?.sandbox_jobs?.length ? (
    <div className="agent-node-version-list">
      {productionState.sandbox_jobs.slice(0, 5).map((job) => (
        <div className="agent-node-version-row" key={job.id}>
          <span>{job.operation_type} · {job.status}</span>
          <small>{job.duration_ms}ms · exit {job.exit_code ?? "-"}</small>
          {job.error_message ? <em>{job.error_message}</em> : null}
        </div>
      ))}
    </div>
  ) : (
    <span className="agent-node-detail-empty">暂无 sandbox 任务。</span>
  )}
</section>
```

- [ ] **Step 4: Run frontend tests**

```bash
pnpm --filter @clip-anvil/web test:connections -- agent readonly
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

## Task 8: Smoke Script And Browser E2E

**Files:**
- Create: `scripts/smoke-m6-9-ux-completion.sh`
- Modify: no production code unless tests reveal a bug

- [ ] **Step 1: Create smoke script**

Create `scripts/smoke-m6-9-ux-completion.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${CLIPANVIL_PUBLIC_BASE_URL:-http://localhost:${CLIPANVIL_WEB_PORT:-5173}}"
API_URL="${CLIPANVIL_API_BASE_URL:-http://localhost:${CLIPANVIL_SERVER_PORT:-8888}}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this smoke script." >&2
  exit 1
fi

email="m69-ux-$(date +%s)@example.test"
password="Password123!"

register_payload=$(jq -n --arg email "$email" --arg password "$password" '{email:$email,password:$password,name:"M69 UX"}')
curl -fsS -X POST "$API_URL/api/auth/register" -H 'content-type: application/json' -d "$register_payload" >/tmp/m69-register.json
token=$(jq -r '.token' /tmp/m69-register.json)
if [[ -z "$token" || "$token" == "null" ]]; then
  echo "failed to read auth token" >&2
  cat /tmp/m69-register.json >&2
  exit 1
fi

workspace_payload='{"name":"m6-9-ux-completion","mode":"agent"}'
curl -fsS -X POST "$API_URL/api/workspaces" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$workspace_payload" >/tmp/m69-workspace.json
workspace_id=$(jq -r '.workspace.id // .id' /tmp/m69-workspace.json)

overview_status=$(curl -sS -o /tmp/m69-overview.json -w "%{http_code}" "$API_URL/api/agent/workspaces/$workspace_id/production-overview" -H "authorization: Bearer $token")
if [[ "$overview_status" != "200" ]]; then
  echo "overview endpoint returned $overview_status" >&2
  cat /tmp/m69-overview.json >&2
  exit 1
fi

message_payload=$(jq -n --arg text "请只回复：M6.9 UX smoke ok。不要调用工具。" '{text:$text,client_message_id:"m69-smoke-1"}')
curl -fsS -X POST "$API_URL/api/agent/workspaces/$workspace_id/messages" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$message_payload" >/tmp/m69-message.json

echo "email=$email"
echo "password=$password"
echo "workspace_id=$workspace_id"
echo "agent_url=$BASE_URL/workspaces/$workspace_id/agent"
echo "overview_response=/tmp/m69-overview.json"
echo "message_response=/tmp/m69-message.json"
```

- [ ] **Step 2: Make script executable and check syntax**

```bash
chmod +x scripts/smoke-m6-9-ux-completion.sh
bash -n scripts/smoke-m6-9-ux-completion.sh
```

Expected: no output, exit 0.

- [ ] **Step 3: Run full local verification**

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-build
GOCACHE=/private/tmp/clipanvil-go-build make server-test
make server-lint
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected: all commands pass.

- [ ] **Step 4: Run dev server**

```bash
./scripts/dev-start.sh
```

Use the Vite URL and server port printed by the script. If the profile is already running, inspect `.dev-pids/<profile>/` and the script log paths instead of guessing ports.

- [ ] **Step 5: Run smoke script**

```bash
CLIPANVIL_API_BASE_URL=http://localhost:<server-port> \
CLIPANVIL_PUBLIC_BASE_URL=http://localhost:<vite-port> \
./scripts/smoke-m6-9-ux-completion.sh
```

Expected:

- prints `agent_url`;
- `/tmp/m69-overview.json` contains `phase`, `counts`, `shots`, `timeline`, `final_outputs`;
- `/tmp/m69-message.json` contains a queued producer task.

- [ ] **Step 6: Browser E2E**

Open the printed `agent_url` in browser automation and verify:

1. Page loads with no "项目加载失败".
2. Status bar is visible near the top of the floating panel.
3. Storyboard section is visible and says no shots for a fresh workspace.
4. Timeline section is visible.
5. Send button works for a clean ping message.
6. Assistant reply persists after refresh.
7. Console does not show uncaught React errors.

For a seeded or real long-running M6 workspace, additionally verify:

1. Status bar shows active/failed/waiting counts.
2. Storyboard rows show preview/review/video chips.
3. Timeline labels do not expose Producer/Craftsman/Worker by default.
4. Node detail drawer exposes review and sandbox sections.

## Final Acceptance

M6.9 can be considered implemented when:

- Backend overview endpoint returns structured state for Agent workspaces.
- Studio workspaces cannot access the Agent overview endpoint.
- Frontend status bar, storyboard and timeline render from overview.
- Overview refreshes on websocket task/event/message/production/canvas updates.
- Final video and review cards are structured and do not show raw JSON as the primary UI.
- Node detail drawer exposes shot context, production trace, retry chain, final composition and sandbox trace.
- Browser refresh restores the same overview from persisted data.
- Default user-facing labels do not expose internal role names.
- Diagnostics expose task/job/event ids in collapsed sections.
- All strict verification commands pass.

## Self-Review Checklist

- Spec coverage:
  - Status bar: Task 3, Task 4, Task 5.
  - Storyboard view: Task 1, Task 4, Task 5.
  - Review/final cards: Task 6.
  - Task timeline: Task 1, Task 3, Task 4, Task 5.
  - Node detail enhancements: Task 7.
  - Backend projection: Task 1, Task 2.
  - Websocket refresh: Task 3, Task 5.
  - Smoke/E2E: Task 8.
- Placeholder scan:
  - No unfinished markers should remain.
- Type consistency:
  - Backend `ProductionOverview` JSON fields match frontend `AgentProductionOverview`.
  - Phase enum values match `agentPhaseLabel`.
  - Query keys use `["agent", id, "production-overview"]` consistently.
