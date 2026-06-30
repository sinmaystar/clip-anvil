# Agent Canvas Workbench M1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build M1 Agent Canvas Workbench so Agent workspaces render as Scene / Shot grouped production workbenches instead of flat domain object graphs.

**Architecture:** Add an Agent-specific workbench projection API under `/api/agent/workspaces/:workspaceID/canvas/workbench`. The backend compiles existing DB facts into `overview + scenes + shots`; the frontend maps that projection into an Agent-only React Flow surface with scene groups, shot cards, artifact slots, and compact production status. Existing `CanvasPayload.domain_projection` stays available for debug/compatibility but no longer drives the default Agent canvas.

**Tech Stack:** Go 1.26, Hertz, pgx v5, sqlc-generated `db.Queries`, PostgreSQL 16, React 19, TypeScript 6, TanStack Query, `@xyflow/react` 12, Vite 8, TailwindCSS 4.

---

## Scope

M1 includes:

- A new backend workbench projection builder for Agent mode.
- A new authenticated Agent API route: `GET /api/agent/workspaces/:workspaceID/canvas/workbench`.
- TypeScript API types and fetcher for the workbench payload.
- A frontend layout mapper that produces stable React Flow nodes/edges from the workbench payload.
- A new Agent Workbench canvas surface with project overview, scene groups, shot cards, preview/video/review/status blocks.
- AgentWorkspacePage integration so the default Agent canvas uses Workbench projection.
- Unit tests for backend projection helpers and frontend view-model mapping.
- Existing websocket/polling refresh behavior wired to refetch the workbench query.
- Visual styling aligned with Studio canvas quality.

M1 excludes:

- Agent detail panel for full Shot / RenderPlan / Review details. That is M2.
- Direct canvas editing or Agent tool changes.
- Debug View toggle for flat RenderPlan / Review / Issue graph. That is M3.
- New database tables or migrations.
- Full browser E2E automation for the entire Producer/Craftsman/Worker flow. M1 should include manual smoke instructions and unit/build/lint checks; full E2E belongs to M3 unless explicitly requested during implementation.

## Current Code Facts

- Existing Agent page: `apps/web/src/pages/AgentWorkspacePage.tsx`.
- Existing canvas fetch: `apps/web/src/lib/api.ts::fetchCanvas`.
- Existing Agent API fetchers: `apps/web/src/lib/agentApi.ts`.
- Existing shared React Flow surface: `apps/web/src/components/canvas-flow/CanvasFlowSurface.tsx`.
- Existing flat domain node renderer: `apps/web/src/components/canvas-flow/DomainFlowNode.tsx`.
- Existing flat backend projection: `apps/server/internal/api/domain_canvas_projection.go`.
- Existing canvas endpoint: `apps/server/internal/api/canvas_handler.go::GetCanvas`.
- Existing Agent routes: `apps/server/cmd/server/main.go` near `/api/agent/workspaces/:workspaceID/...`.
- Existing production preview conversion helpers are in `apps/server/internal/api/canvas_handler.go` and are package-private, so new API files in package `api` can reuse them.
- Existing tests include source-contract tests in `apps/web/src/lib/agentCanvas.test.mjs` and API route contract tests in `apps/server/internal/api/agent_handler_test.go`.

## File Structure

Create:

- `apps/server/internal/api/agent_workbench_projection.go`: response structs, store interface, projection builder, grouping, status derivation, artifact slot derivation.
- `apps/server/internal/api/agent_workbench_projection_test.go`: pure helper tests with fake store-backed fixtures.
- `apps/web/src/lib/agentWorkbench.ts`: frontend Workbench payload types, status helpers, and display label helpers.
- `apps/web/src/lib/agentWorkbenchViewModel.ts`: maps Workbench payload into React Flow nodes/edges and deterministic layout.
- `apps/web/src/lib/agentWorkbenchViewModel.test.mjs`: unit tests for layout stability, node counts, and edge filtering.
- `apps/web/src/components/agent-workbench/AgentWorkbenchCanvas.tsx`: Agent-specific React Flow host.
- `apps/web/src/components/agent-workbench/AgentProjectOverviewNode.tsx`: overview node.
- `apps/web/src/components/agent-workbench/AgentSceneGroupNode.tsx`: scene group node.
- `apps/web/src/components/agent-workbench/AgentShotNode.tsx`: shot production card node.
- `apps/web/src/components/agent-workbench/AgentWorkbenchEdge.tsx`: restrained production-flow edge.

Modify:

- `apps/server/internal/api/agent_handler.go`: add `GetCanvasWorkbench`.
- `apps/server/cmd/server/main.go`: register workbench route.
- `apps/server/internal/api/agent_handler_test.go`: route contract test.
- `apps/web/src/lib/agentApi.ts`: add `fetchAgentCanvasWorkbench`.
- `apps/web/src/pages/AgentWorkspacePage.tsx`: query and render `AgentWorkbenchCanvas`; refetch workbench on canvas/agent events.
- `apps/web/src/lib/agentCanvas.test.mjs`: update assertions from old shared canvas default to Workbench default.
- `apps/web/src/main.css`: Workbench layout and node styling.
- `apps/web/package.json`: add `agentWorkbenchViewModel.test.mjs` to `test:connections`.

Do not modify:

- Agent tools, Producer/Craftsman/Reviewer prompts, Eino graphs.
- Database migrations.
- `apps/server/internal/api/domain_canvas_projection.go` except if a source contract test needs wording updates. M1 should not remove the old projection.

## Data Contract

Backend response should serialize with snake_case JSON:

```go
type agentWorkbenchResponse struct {
	Overview agentWorkbenchOverviewResponse `json:"overview"`
	Scenes   []agentWorkbenchSceneResponse  `json:"scenes"`
	Counts   agentWorkbenchCountsResponse   `json:"counts"`
}

type agentWorkbenchOverviewResponse struct {
	WorkspaceID      string                                   `json:"workspace_id"`
	Brief            *agentWorkbenchBriefResponse             `json:"brief,omitempty"`
	Memory           *agentWorkbenchMemoryResponse            `json:"memory,omitempty"`
	KeyElements      []agentWorkbenchKeyElementResponse       `json:"key_elements"`
	KeyElementStates []agentWorkbenchKeyElementStateResponse  `json:"key_element_states"`
	SourceMaterials  []agentWorkbenchSourceMaterialResponse   `json:"source_materials"`
}

type agentWorkbenchSceneResponse struct {
	ID       string                           `json:"id"`
	Title    string                           `json:"title"`
	Status   string                           `json:"status"`
	Summary  string                           `json:"summary,omitempty"`
	Location string                           `json:"location,omitempty"`
	Shots    []agentWorkbenchShotResponse     `json:"shots"`
}

type agentWorkbenchShotResponse struct {
	ID            string                                      `json:"id"`
	ClientKey     string                                      `json:"client_key"`
	Title         string                                      `json:"title"`
	Status        string                                      `json:"status"`
	SequenceIndex int32                                       `json:"sequence_index"`
	CreativeText  string                                      `json:"creative_text"`
	Dependencies []agentWorkbenchShotDependencyResponse       `json:"dependencies"`
	KeyElements   []agentWorkbenchShotKeyElementRefResponse   `json:"key_elements"`
	Preview       agentWorkbenchArtifactSlotResponse          `json:"preview"`
	Video         agentWorkbenchArtifactSlotResponse          `json:"video"`
	RenderPlans   []agentWorkbenchRenderPlanSummaryResponse   `json:"render_plans"`
	Review        *agentWorkbenchReviewSummaryResponse        `json:"review,omitempty"`
	Issues        []agentWorkbenchIssueSummaryResponse        `json:"issues"`
}

type agentWorkbenchArtifactSlotResponse struct {
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	NodeID       string `json:"node_id,omitempty"`
	Title        string `json:"title,omitempty"`
	VersionID    string `json:"version_id,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	AccessURL    string `json:"access_url,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}
```

Slot `kind` values:

- `preview_image`
- `shot_video`

Slot `status` values:

- `missing`
- `queued`
- `running`
- `succeeded`
- `failed`
- `stale`

The builder should derive artifact slots from Agent-owned `media_node` rows where:

- `media_node.source = 'agent'`
- `media_node.shot_id = shot.id`
- `media_node.metadata->>'agent_artifact_kind' = 'preview_image'` for preview
- `media_node.metadata->>'agent_artifact_kind' = 'shot_video'` for video

If multiple nodes match one slot, pick the most useful one:

1. Prefer node with `current_version_id`.
2. Then prefer `status='running'`.
3. Then prefer `status='queued'`.
4. Then prefer newest `updated_at`.

## Task 1: Backend Route Contract

**Files:**

- Modify: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/api/agent_handler_test.go`

- [ ] **Step 1: Add failing route contract test**

In `apps/server/internal/api/agent_handler_test.go`, extend `TestAgentProductionOverviewRouteContract` or add this test:

```go
func TestAgentCanvasWorkbenchRouteContract(t *testing.T) {
	handlerSource, err := os.ReadFile("agent_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(handlerSource), "func (h *AgentHandler) GetCanvasWorkbench") {
		t.Fatal("AgentHandler.GetCanvasWorkbench must be implemented")
	}

	serverSource, err := os.ReadFile("../../cmd/server/main.go")
	if err != nil {
		t.Fatal(err)
	}
	wantRoute := `GET("/api/agent/workspaces/:workspaceID/canvas/workbench", authMiddleware, agentHandler.GetCanvasWorkbench)`
	if !strings.Contains(string(serverSource), wantRoute) {
		t.Fatalf("server route %q is not registered", wantRoute)
	}
}
```

- [ ] **Step 2: Run the contract test and confirm failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run TestAgentCanvasWorkbenchRouteContract -count=1
```

Expected: FAIL because `GetCanvasWorkbench` and route are not implemented.

- [ ] **Step 3: Add handler method skeleton**

In `apps/server/internal/api/agent_handler.go`, add:

```go
func (h *AgentHandler) GetCanvasWorkbench(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}
	workbench, err := buildAgentWorkbenchProjection(ctx, h.queries, h.storage, workspace.ID)
	if err != nil {
		slog.Error("failed to build agent canvas workbench", "workspace_id", uuidToString(workspace.ID), "error", err)
		writeError(c, consts.StatusInternalServerError, "failed to load agent canvas workbench")
		return
	}
	c.JSON(consts.StatusOK, workbench)
}
```

This will not compile until Task 2 creates `buildAgentWorkbenchProjection`.

- [ ] **Step 4: Register route**

In `apps/server/cmd/server/main.go`, add the route near the other Agent routes:

```go
h.GET("/api/agent/workspaces/:workspaceID/canvas/workbench", authMiddleware, agentHandler.GetCanvasWorkbench)
```

- [ ] **Step 5: Run the contract test**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run TestAgentCanvasWorkbenchRouteContract -count=1
```

Expected at this point: compile failure referencing `buildAgentWorkbenchProjection`, which Task 2 will fix.

## Task 2: Backend Workbench Projection Builder

**Files:**

- Create: `apps/server/internal/api/agent_workbench_projection.go`
- Create: `apps/server/internal/api/agent_workbench_projection_test.go`
- Modify: `apps/server/internal/api/agent_handler.go`

- [ ] **Step 1: Create builder tests for grouping and slot selection**

Create `apps/server/internal/api/agent_workbench_projection_test.go`:

```go
package api

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestAgentWorkbenchSelectsBestArtifactNode(t *testing.T) {
	shotID := uuidWithByteForWorkbenchTest(1)
	oldSucceeded := db.MediaNode{
		ID:               uuidWithByteForWorkbenchTest(11),
		ShotID:           shotID,
		Source:           "agent",
		NodeType:         db.NodeTypeImage,
		Title:            "old preview",
		Status:           db.NodeStatusSucceeded,
		CurrentVersionID: uuidWithByteForWorkbenchTest(101),
		Metadata:         []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt:        pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true},
	}
	running := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(12),
		ShotID:    shotID,
		Source:    "agent",
		NodeType:  db.NodeTypeImage,
		Title:     "running preview",
		Status:    db.NodeStatusRunning,
		Metadata:  []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(200, 0), Valid: true},
	}
	got := bestAgentArtifactNode([]db.MediaNode{running, oldSucceeded}, "preview_image")
	if got == nil || got.ID != oldSucceeded.ID {
		t.Fatalf("best node = %#v, want succeeded current version node", got)
	}
}

func TestAgentWorkbenchMissingArtifactSlot(t *testing.T) {
	slot := agentWorkbenchArtifactSlotResponse{Kind: "shot_video", Status: "missing"}
	if slot.Kind != "shot_video" || slot.Status != "missing" || slot.NodeID != "" {
		t.Fatalf("slot = %#v", slot)
	}
}

func uuidWithByteForWorkbenchTest(value byte) pgtype.UUID {
	return pgtype.UUID{
		Bytes: [16]byte{value, value, value, value, value, value, value, value, value, value, value, value, value, value, value, value},
		Valid: true,
	}
}
```

- [ ] **Step 2: Run tests and confirm failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestAgentWorkbench' -count=1
```

Expected: FAIL because response/helper types do not exist.

- [ ] **Step 3: Create response structs and helper functions**

Create `apps/server/internal/api/agent_workbench_projection.go` with this structure:

```go
package api

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type agentWorkbenchResponse struct {
	Overview agentWorkbenchOverviewResponse `json:"overview"`
	Scenes   []agentWorkbenchSceneResponse  `json:"scenes"`
	Counts   agentWorkbenchCountsResponse   `json:"counts"`
}

type agentWorkbenchOverviewResponse struct {
	WorkspaceID      string                                  `json:"workspace_id"`
	Brief            *agentWorkbenchBriefResponse            `json:"brief,omitempty"`
	Memory           *agentWorkbenchMemoryResponse           `json:"memory,omitempty"`
	KeyElements      []agentWorkbenchKeyElementResponse      `json:"key_elements"`
	KeyElementStates []agentWorkbenchKeyElementStateResponse `json:"key_element_states"`
	SourceMaterials  []agentWorkbenchSourceMaterialResponse  `json:"source_materials"`
}

type agentWorkbenchBriefResponse struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Concept  string `json:"concept"`
	Status   string `json:"status"`
}

type agentWorkbenchMemoryResponse struct {
	ID      string `json:"id"`
	Version int32  `json:"version"`
	Soul    string `json:"soul"`
	Status  string `json:"status"`
}

type agentWorkbenchKeyElementResponse struct {
	ID        string `json:"id"`
	ClientKey string `json:"client_key"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Status    string `json:"status"`
}

type agentWorkbenchKeyElementStateResponse struct {
	ID              string `json:"id"`
	KeyElementID    string `json:"key_element_id"`
	ClientKey       string `json:"client_key"`
	Label           string `json:"label"`
	ReferenceStatus string `json:"reference_status"`
}

type agentWorkbenchSourceMaterialResponse struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	NodeType string `json:"node_type"`
	Status   string `json:"status"`
}

type agentWorkbenchSceneResponse struct {
	ID       string                           `json:"id"`
	Title    string                           `json:"title"`
	Status   string                           `json:"status"`
	Summary  string                           `json:"summary,omitempty"`
	Location string                           `json:"location,omitempty"`
	Shots    []agentWorkbenchShotResponse     `json:"shots"`
}

type agentWorkbenchShotResponse struct {
	ID            string                                    `json:"id"`
	ClientKey     string                                    `json:"client_key"`
	Title         string                                    `json:"title"`
	Status        string                                    `json:"status"`
	SequenceIndex int32                                     `json:"sequence_index"`
	CreativeText  string                                    `json:"creative_text"`
	Dependencies []agentWorkbenchShotDependencyResponse     `json:"dependencies"`
	KeyElements   []agentWorkbenchShotKeyElementRefResponse `json:"key_elements"`
	Preview       agentWorkbenchArtifactSlotResponse        `json:"preview"`
	Video         agentWorkbenchArtifactSlotResponse        `json:"video"`
	RenderPlans   []agentWorkbenchRenderPlanSummaryResponse `json:"render_plans"`
	Review        *agentWorkbenchReviewSummaryResponse      `json:"review,omitempty"`
	Issues        []agentWorkbenchIssueSummaryResponse      `json:"issues"`
}

type agentWorkbenchShotDependencyResponse struct {
	ID             string `json:"id"`
	FromShotID     string `json:"from_shot_id"`
	ToShotID       string `json:"to_shot_id"`
	DependencyType string `json:"dependency_type"`
}

type agentWorkbenchShotKeyElementRefResponse struct {
	ID                string `json:"id"`
	KeyElementID      string `json:"key_element_id"`
	KeyElementStateID string `json:"key_element_state_id,omitempty"`
	Role              string `json:"role"`
}

type agentWorkbenchArtifactSlotResponse struct {
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	NodeID       string `json:"node_id,omitempty"`
	Title        string `json:"title,omitempty"`
	VersionID    string `json:"version_id,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	AccessURL    string `json:"access_url,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type agentWorkbenchRenderPlanSummaryResponse struct {
	ID          string `json:"id"`
	Revision    int32  `json:"revision"`
	TargetPhase string `json:"target_phase"`
	Operation   string `json:"operation"`
	Status      string `json:"status"`
}

type agentWorkbenchReviewSummaryResponse struct {
	ID          string  `json:"id"`
	ReviewTask  string  `json:"review_task"`
	TargetPhase string  `json:"target_phase"`
	Status      string  `json:"status"`
	Verdict     string  `json:"verdict"`
	Score       float32 `json:"score,omitempty"`
}

type agentWorkbenchIssueSummaryResponse struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Severity     string `json:"severity"`
	Dimension    string `json:"dimension"`
	SuggestedFix string `json:"suggested_fix"`
}

type agentWorkbenchCountsResponse struct {
	Scenes             int `json:"scenes"`
	Shots              int `json:"shots"`
	PreviewSucceeded   int `json:"preview_succeeded"`
	PreviewFailed      int `json:"preview_failed"`
	VideoSucceeded     int `json:"video_succeeded"`
	VideoFailed        int `json:"video_failed"`
	OpenIssues         int `json:"open_issues"`
	NeedsReference     int `json:"needs_reference"`
}
```

Add helpers in the same file:

```go
func bestAgentArtifactNode(nodes []db.MediaNode, artifactKind string) *db.MediaNode {
	var candidates []db.MediaNode
	for _, node := range nodes {
		if node.Source != "agent" || agentArtifactKind(node.Metadata) != artifactKind {
			continue
		}
		candidates = append(candidates, node)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return agentArtifactRank(candidates[i]) < agentArtifactRank(candidates[j])
	})
	return &candidates[0]
}

func agentArtifactRank(node db.MediaNode) int {
	if node.CurrentVersionID.Valid {
		return 0
	}
	switch node.Status {
	case db.NodeStatusRunning:
		return 1
	case db.NodeStatusQueued:
		return 2
	default:
		return 3
	}
}

func agentArtifactKind(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return ""
	}
	value, _ := metadata["agent_artifact_kind"].(string)
	return value
}

func agentSlotStatus(node db.MediaNode) string {
	switch node.Status {
	case db.NodeStatusQueued:
		return "queued"
	case db.NodeStatusRunning:
		return "running"
	case db.NodeStatusSucceeded:
		return "succeeded"
	case db.NodeStatusFailed:
		return "failed"
	case db.NodeStatusStale:
		return "stale"
	default:
		if node.CurrentVersionID.Valid {
			return "succeeded"
		}
		return "missing"
	}
}

func agentUpdatedAt(node db.MediaNode) time.Time {
	if node.UpdatedAt.Valid {
		return node.UpdatedAt.Time
	}
	return time.Time{}
}
```

Update `bestAgentArtifactNode` sorting after the first test passes enough to include newest fallback:

```go
sort.SliceStable(candidates, func(i, j int) bool {
	leftRank := agentArtifactRank(candidates[i])
	rightRank := agentArtifactRank(candidates[j])
	if leftRank != rightRank {
		return leftRank < rightRank
	}
	return agentUpdatedAt(candidates[i]).After(agentUpdatedAt(candidates[j]))
})
```

- [ ] **Step 4: Run helper tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestAgentWorkbench' -count=1
```

Expected: PASS for helper tests, compile failure may remain for missing full builder if Task 1 already references it.

- [ ] **Step 5: Implement `buildAgentWorkbenchProjection`**

In `apps/server/internal/api/agent_workbench_projection.go`, add:

```go
func buildAgentWorkbenchProjection(ctx context.Context, queries *db.Queries, signer assetURLSigner, workspaceID pgtype.UUID) (agentWorkbenchResponse, error) {
	if queries == nil || !workspaceID.Valid {
		return agentWorkbenchResponse{}, nil
	}

	response := agentWorkbenchResponse{
		Overview: agentWorkbenchOverviewResponse{
			WorkspaceID: uuidToString(workspaceID),
		},
	}

	if brief, ok, err := activeCreativeBrief(ctx, queries, workspaceID); err != nil {
		return response, err
	} else if ok {
		response.Overview.Brief = &agentWorkbenchBriefResponse{
			ID:      uuidToString(brief.ID),
			Title:   brief.Title,
			Concept: brief.Concept,
			Status:  brief.Status,
		}
	}

	if memory, ok, err := activeProjectMemory(ctx, queries, workspaceID); err != nil {
		return response, err
	} else if ok {
		response.Overview.Memory = &agentWorkbenchMemoryResponse{
			ID:      uuidToString(memory.ID),
			Version: memory.Version,
			Soul:    memory.Soul,
			Status:  memory.Status,
		}
	}

	elements, err := queries.ListActiveKeyElementsByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	for _, element := range elements {
		response.Overview.KeyElements = append(response.Overview.KeyElements, agentWorkbenchKeyElementResponse{
			ID:        uuidToString(element.ID),
			ClientKey: element.ClientKey,
			Name:      element.Name,
			Type:      element.ElementType,
			Status:    element.Status,
		})
	}

	states, err := queries.ListActiveKeyElementStatesByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	for _, state := range states {
		if state.ReferenceStatus == "needs_reference" {
			response.Counts.NeedsReference++
		}
		response.Overview.KeyElementStates = append(response.Overview.KeyElementStates, agentWorkbenchKeyElementStateResponse{
			ID:              uuidToString(state.ID),
			KeyElementID:    uuidToString(state.KeyElementID),
			ClientKey:       state.ClientKey,
			Label:           state.Label,
			ReferenceStatus: state.ReferenceStatus,
		})
	}

	nodes, err := queries.ListMediaNodesByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	assets, err := queries.ListMediaAssetsByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	assetsByID := make(map[pgtype.UUID]db.MediaAsset, len(assets))
	for _, asset := range assets {
		assetsByID[asset.ID] = asset
	}
	versionsByID := make(map[pgtype.UUID]db.ArtifactVersion)
	for _, node := range nodes {
		if !node.CurrentVersionID.Valid {
			continue
		}
		version, err := queries.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil {
			return response, err
		}
		versionsByID[node.CurrentVersionID] = version
	}

	sourceMaterials := agentWorkbenchSourceMaterials(nodes)
	response.Overview.SourceMaterials = sourceMaterials

	scenes, err := queries.ListActiveScenesByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	shots, err := queries.ListActiveShotsByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	shotElements, err := queries.ListShotKeyElementsByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	deps, err := queries.ListShotDependenciesByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	renderPlans, err := queries.ListRenderPlansByWorkspace(ctx, workspaceID)
	if err != nil {
		return response, err
	}
	reviews, err := queries.ListReviewRecordsByWorkspace(ctx, db.ListReviewRecordsByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return response, err
	}
	issues, err := queries.ListOpenArtifactIssuesByWorkspace(ctx, db.ListOpenArtifactIssuesByWorkspaceParams{WorkspaceID: workspaceID, Limit: 100})
	if err != nil {
		return response, err
	}

	response.Scenes = agentWorkbenchScenes(ctx, signer, scenes, shots, nodes, assetsByID, versionsByID, shotElements, deps, renderPlans, reviews, issues, &response.Counts)
	response.Counts.Scenes = len(response.Scenes)
	response.Counts.Shots = len(shots)
	return response, nil
}
```

Then add small helpers in the same file:

```go
func agentWorkbenchSourceMaterials(nodes []db.MediaNode) []agentWorkbenchSourceMaterialResponse {
	out := []agentWorkbenchSourceMaterialResponse{}
	for _, node := range nodes {
		if agentArtifactKind(node.Metadata) != "" {
			continue
		}
		if node.OperationType != "upload" && node.OperationType != "manual" {
			continue
		}
		out = append(out, agentWorkbenchSourceMaterialResponse{
			ID:       uuidToString(node.ID),
			Title:    node.Title,
			NodeType: string(node.NodeType),
			Status:   string(node.Status),
		})
	}
	return out
}
```

Implement `agentWorkbenchScenes` with these concrete operations in order:

- Create `shotsBySceneID := map[pgtype.UUID][]db.Shot{}` and append each shot by `shot.SceneID`.
- Sort each scene shot slice by `SequenceIndex`, then `ClientKey`, then `Title`.
- Create `nodesByShotID := map[pgtype.UUID][]db.MediaNode{}` and append nodes only when `node.ShotID.Valid`.
- Create `shotElementsByShotID := map[pgtype.UUID][]db.ShotKeyElement{}` and append each link by `link.ShotID`.
- Create `depsByShotID := map[pgtype.UUID][]db.ShotDependency{}` and append each dependency to both `FromShotID` and `ToShotID` so every shot can show continuity context.
- Create `renderPlansByShotID := map[pgtype.UUID][]db.RenderPlan{}` and append plans where `ScopeType == "shot"`.
- Sort render plans by `Revision` descending and map each to `agentWorkbenchRenderPlanSummaryResponse`.
- Create `latestReviewByShotID := map[pgtype.UUID]db.ReviewRecord{}` and keep the review with the newest `CreatedAt` for each valid `review.ShotID`.
- Create `issuesByShotID := map[pgtype.UUID][]db.ArtifactIssue{}` and append issues where `TargetObjectType == "shot"` and `TargetObjectID` is a known shot ID.
- For each scene, append one `agentWorkbenchSceneResponse`; for each shot in that scene, append one `agentWorkbenchShotResponse` with preview from `agentWorkbenchArtifactSlot(ctx, signer, "preview_image", nodesByShotID[shot.ID], assets, versions)` and video from `agentWorkbenchArtifactSlot(ctx, signer, "shot_video", nodesByShotID[shot.ID], assets, versions)`.
- Increment `counts.PreviewSucceeded`, `counts.PreviewFailed`, `counts.VideoSucceeded`, `counts.VideoFailed`, and `counts.OpenIssues` while building shot responses.

Use this helper signature, and put the ordered operations above inside the function body:

```go
func agentWorkbenchScenes(
	ctx context.Context,
	signer assetURLSigner,
	scenes []db.Scene,
	shots []db.Shot,
	nodes []db.MediaNode,
	assets map[pgtype.UUID]db.MediaAsset,
	versions map[pgtype.UUID]db.ArtifactVersion,
	shotElements []db.ShotKeyElement,
	deps []db.ShotDependency,
	renderPlans []db.RenderPlan,
	reviews []db.ReviewRecord,
	issues []db.ArtifactIssue,
	counts *agentWorkbenchCountsResponse,
) []agentWorkbenchSceneResponse
```

- [ ] **Step 6: Implement artifact slot conversion**

Add:

```go
func agentWorkbenchArtifactSlot(ctx context.Context, signer assetURLSigner, kind string, nodes []db.MediaNode, assets map[pgtype.UUID]db.MediaAsset, versions map[pgtype.UUID]db.ArtifactVersion) (agentWorkbenchArtifactSlotResponse, error) {
	slot := agentWorkbenchArtifactSlotResponse{Kind: kind, Status: "missing"}
	node := bestAgentArtifactNode(nodes, kind)
	if node == nil {
		return slot, nil
	}
	slot.NodeID = uuidToString(node.ID)
	slot.Title = node.Title
	slot.Status = agentSlotStatus(*node)
	if node.CurrentVersionID.Valid {
		slot.VersionID = uuidToString(node.CurrentVersionID)
		if version, ok := versions[node.CurrentVersionID]; ok {
			preview, err := toCanvasProductionPreview(ctx, signer, version, assets)
			if err != nil {
				return slot, err
			}
			if preview != nil {
				slot.AccessURL = preview.AccessURL
				slot.ThumbnailURL = preview.ThumbnailURL
			}
		}
	}
	if slot.Status == "failed" {
		code, message := agentNodeError(node.Metadata)
		slot.ErrorCode = code
		slot.ErrorMessage = message
	}
	return slot, nil
}
```

Add:

```go
func agentNodeError(raw []byte) (string, string) {
	if len(raw) == 0 {
		return "", ""
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return "", ""
	}
	code, _ := metadata["error_code"].(string)
	message, _ := metadata["error_message"].(string)
	return code, message
}
```

- [ ] **Step 7: Run backend API tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestAgentCanvasWorkbenchRouteContract|TestAgentWorkbench' -count=1
```

Expected: PASS.

## Task 3: Frontend API Types And Fetcher

**Files:**

- Create: `apps/web/src/lib/agentWorkbench.ts`
- Modify: `apps/web/src/lib/agentApi.ts`
- Modify: `apps/web/src/lib/agentCanvas.test.mjs`

- [ ] **Step 1: Add source-contract test**

In `apps/web/src/lib/agentCanvas.test.mjs`, add:

```js
it("fetches Agent canvas workbench through the Agent API namespace", async () => {
  const agentApiSource = await readFile(
    new URL("../lib/agentApi.ts", import.meta.url),
    "utf8",
  );

  assert.match(agentApiSource, /fetchAgentCanvasWorkbench/);
  assert.match(
    agentApiSource,
    /\/agent\/workspaces\/\$\{workspaceId\}\/canvas\/workbench/,
  );
});
```

- [ ] **Step 2: Run web source test and confirm failure**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentCanvas
```

Expected: FAIL because fetcher/types do not exist.

- [ ] **Step 3: Create frontend Workbench types**

Create `apps/web/src/lib/agentWorkbench.ts`:

```ts
export interface AgentWorkbenchProjection {
  overview: AgentWorkbenchOverview;
  scenes: AgentWorkbenchScene[];
  counts: AgentWorkbenchCounts;
}

export interface AgentWorkbenchOverview {
  workspace_id: string;
  brief?: AgentWorkbenchBrief;
  memory?: AgentWorkbenchMemory;
  key_elements: AgentWorkbenchKeyElement[];
  key_element_states: AgentWorkbenchKeyElementState[];
  source_materials: AgentWorkbenchSourceMaterial[];
}

export interface AgentWorkbenchBrief {
  id: string;
  title: string;
  concept: string;
  status: string;
}

export interface AgentWorkbenchMemory {
  id: string;
  version: number;
  soul: string;
  status: string;
}

export interface AgentWorkbenchKeyElement {
  id: string;
  client_key: string;
  name: string;
  type: string;
  status: string;
}

export interface AgentWorkbenchKeyElementState {
  id: string;
  key_element_id: string;
  client_key: string;
  label: string;
  reference_status: string;
}

export interface AgentWorkbenchSourceMaterial {
  id: string;
  title: string;
  node_type: string;
  status: string;
}

export interface AgentWorkbenchScene {
  id: string;
  title: string;
  status: string;
  summary?: string;
  location?: string;
  shots: AgentWorkbenchShot[];
}

export interface AgentWorkbenchShot {
  id: string;
  client_key: string;
  title: string;
  status: string;
  sequence_index: number;
  creative_text: string;
  dependencies: AgentWorkbenchShotDependency[];
  key_elements: AgentWorkbenchShotKeyElementRef[];
  preview: AgentWorkbenchArtifactSlot;
  video: AgentWorkbenchArtifactSlot;
  render_plans: AgentWorkbenchRenderPlanSummary[];
  review?: AgentWorkbenchReviewSummary;
  issues: AgentWorkbenchIssueSummary[];
}

export interface AgentWorkbenchShotDependency {
  id: string;
  from_shot_id: string;
  to_shot_id: string;
  dependency_type: string;
}

export interface AgentWorkbenchShotKeyElementRef {
  id: string;
  key_element_id: string;
  key_element_state_id?: string;
  role: string;
}

export interface AgentWorkbenchArtifactSlot {
  kind: "preview_image" | "shot_video" | string;
  status: "missing" | "queued" | "running" | "succeeded" | "failed" | "stale" | string;
  node_id?: string;
  title?: string;
  version_id?: string;
  thumbnail_url?: string;
  access_url?: string;
  error_code?: string;
  error_message?: string;
}

export interface AgentWorkbenchRenderPlanSummary {
  id: string;
  revision: number;
  target_phase: string;
  operation: string;
  status: string;
}

export interface AgentWorkbenchReviewSummary {
  id: string;
  review_task: string;
  target_phase: string;
  status: string;
  verdict: string;
  score?: number;
}

export interface AgentWorkbenchIssueSummary {
  id: string;
  title: string;
  severity: string;
  dimension: string;
  suggested_fix: string;
}

export interface AgentWorkbenchCounts {
  scenes: number;
  shots: number;
  preview_succeeded: number;
  preview_failed: number;
  video_succeeded: number;
  video_failed: number;
  open_issues: number;
  needs_reference: number;
}

export function agentWorkbenchVisibleNodeCount(workbench: AgentWorkbenchProjection) {
  return (
    1 +
    workbench.scenes.length +
    workbench.scenes.reduce((sum, scene) => sum + scene.shots.length, 0)
  );
}
```

- [ ] **Step 4: Add fetcher**

In `apps/web/src/lib/agentApi.ts`, import the type:

```ts
import type { AgentWorkbenchProjection } from "./agentWorkbench";
```

Add fetcher near `fetchAgentProductionOverview`:

```ts
export function fetchAgentCanvasWorkbench(workspaceId: string) {
  return apiFetch<AgentWorkbenchProjection>(
    `/agent/workspaces/${workspaceId}/canvas/workbench`,
  );
}
```

- [ ] **Step 5: Run web test**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentCanvas
```

Expected: PASS for the new fetcher contract; other old Agent canvas assertions may fail once Task 6 changes page behavior.

## Task 4: Frontend Workbench View Model

**Files:**

- Create: `apps/web/src/lib/agentWorkbenchViewModel.ts`
- Create: `apps/web/src/lib/agentWorkbenchViewModel.test.mjs`
- Modify: `apps/web/package.json`

- [ ] **Step 1: Add failing view-model tests**

Create `apps/web/src/lib/agentWorkbenchViewModel.test.mjs`:

```js
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentWorkbenchToFlow,
  shotNodeId,
} from "../../dist-test/lib/agentWorkbenchViewModel.js";

const workbench = {
  overview: {
    workspace_id: "workspace-1",
    brief: { id: "brief-1", title: "行李箱广告", concept: "抖音种草", status: "active" },
    key_elements: [],
    key_element_states: [],
    source_materials: [],
  },
  scenes: [
    {
      id: "scene-1",
      title: "机场出行",
      status: "planned",
      shots: [
        {
          id: "shot-2",
          client_key: "shot_02",
          title: "第二镜",
          status: "planned",
          sequence_index: 2,
          creative_text: "展示耐磨",
          dependencies: [],
          key_elements: [],
          preview: { kind: "preview_image", status: "missing" },
          video: { kind: "shot_video", status: "missing" },
          render_plans: [],
          issues: [],
        },
        {
          id: "shot-1",
          client_key: "shot_01",
          title: "第一镜",
          status: "planned",
          sequence_index: 1,
          creative_text: "机场亮相",
          dependencies: [],
          key_elements: [],
          preview: { kind: "preview_image", status: "succeeded", thumbnail_url: "preview.png" },
          video: { kind: "shot_video", status: "queued" },
          render_plans: [],
          issues: [],
        },
      ],
    },
  ],
  counts: {
    scenes: 1,
    shots: 2,
    preview_succeeded: 1,
    preview_failed: 0,
    video_succeeded: 0,
    video_failed: 0,
    open_issues: 0,
    needs_reference: 0,
  },
};

describe("agent workbench view model", () => {
  it("creates overview, scene, and shot nodes without render plan node spam", () => {
    const flow = agentWorkbenchToFlow(workbench);

    assert.deepEqual(
      flow.nodes.map((node) => node.id),
      ["agent-workbench-overview", "agent-scene-scene-1", "agent-shot-shot-1", "agent-shot-shot-2"],
    );
    assert.equal(flow.edges.length, 2);
    assert.equal(flow.nodes.some((node) => node.id.includes("render-plan")), false);
  });

  it("sorts shots by sequence index and lays them inside the scene lane", () => {
    const flow = agentWorkbenchToFlow(workbench);
    const shot1 = flow.nodes.find((node) => node.id === shotNodeId("shot-1"));
    const shot2 = flow.nodes.find((node) => node.id === shotNodeId("shot-2"));

    assert.ok(shot1);
    assert.ok(shot2);
    assert.equal(shot1.parentId, "agent-scene-scene-1");
    assert.equal(shot2.parentId, "agent-scene-scene-1");
    assert.ok(shot1.position.x < shot2.position.x);
    assert.equal(shot1.position.y, shot2.position.y);
  });
});
```

- [ ] **Step 2: Add test script entry**

In `apps/web/package.json`, append `src/lib/agentWorkbenchViewModel.test.mjs` to `test:connections`.

- [ ] **Step 3: Run test and confirm failure**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentWorkbenchViewModel
```

Expected: FAIL because the module does not exist.

- [ ] **Step 4: Implement view model**

Create `apps/web/src/lib/agentWorkbenchViewModel.ts`:

```ts
import type { Edge, Node } from "@xyflow/react";
import type {
  AgentWorkbenchProjection,
  AgentWorkbenchScene,
  AgentWorkbenchShot,
} from "./agentWorkbench";

export type AgentWorkbenchNode =
  | Node<AgentWorkbenchOverviewNodeData, "agentOverview">
  | Node<AgentWorkbenchSceneNodeData, "agentScene">
  | Node<AgentWorkbenchShotNodeData, "agentShot">;

export type AgentWorkbenchEdge = Edge<AgentWorkbenchEdgeData, "agentWorkbench">;

export interface AgentWorkbenchOverviewNodeData extends Record<string, unknown> {
  kind: "overview";
  workbench: AgentWorkbenchProjection;
}

export interface AgentWorkbenchSceneNodeData extends Record<string, unknown> {
  kind: "scene";
  scene: AgentWorkbenchScene;
}

export interface AgentWorkbenchShotNodeData extends Record<string, unknown> {
  kind: "shot";
  shot: AgentWorkbenchShot;
}

export interface AgentWorkbenchEdgeData extends Record<string, unknown> {
  label?: string;
}

const OVERVIEW_WIDTH = 360;
const OVERVIEW_HEIGHT = 248;
const SCENE_PADDING = 28;
const SCENE_HEADER = 64;
const SHOT_WIDTH = 360;
const SHOT_HEIGHT = 292;
const SHOT_GAP = 32;
const SCENE_GAP = 72;
const ORIGIN_X = 40;
const ORIGIN_Y = 40;
const SCENE_X = ORIGIN_X + OVERVIEW_WIDTH + 80;

export function overviewNodeId() {
  return "agent-workbench-overview";
}

export function sceneNodeId(sceneId: string) {
  return `agent-scene-${sceneId}`;
}

export function shotNodeId(shotId: string) {
  return `agent-shot-${shotId}`;
}

export function agentWorkbenchToFlow(workbench: AgentWorkbenchProjection): {
  nodes: AgentWorkbenchNode[];
  edges: AgentWorkbenchEdge[];
} {
  const nodes: AgentWorkbenchNode[] = [
    {
      id: overviewNodeId(),
      type: "agentOverview",
      position: { x: ORIGIN_X, y: ORIGIN_Y },
      data: { kind: "overview", workbench },
      width: OVERVIEW_WIDTH,
      height: OVERVIEW_HEIGHT,
      measured: { width: OVERVIEW_WIDTH, height: OVERVIEW_HEIGHT },
      style: { width: OVERVIEW_WIDTH, height: OVERVIEW_HEIGHT },
      draggable: false,
      selectable: true,
    },
  ];
  const edges: AgentWorkbenchEdge[] = [];

  let sceneY = ORIGIN_Y;
  for (const scene of workbench.scenes) {
    const shots = sortedShots(scene.shots);
    const sceneWidth =
      SCENE_PADDING * 2 + Math.max(1, shots.length) * SHOT_WIDTH + Math.max(0, shots.length - 1) * SHOT_GAP;
    const sceneHeight = SCENE_HEADER + SHOT_HEIGHT + SCENE_PADDING;
    const sceneId = sceneNodeId(scene.id);

    nodes.push({
      id: sceneId,
      type: "agentScene",
      position: { x: SCENE_X, y: sceneY },
      data: { kind: "scene", scene },
      width: sceneWidth,
      height: sceneHeight,
      measured: { width: sceneWidth, height: sceneHeight },
      style: { width: sceneWidth, height: sceneHeight },
      draggable: false,
      selectable: true,
    });
    edges.push({
      id: `agent-edge-overview-${scene.id}`,
      type: "agentWorkbench",
      source: overviewNodeId(),
      target: sceneId,
      data: { label: "scene" },
    });

    shots.forEach((shot, index) => {
      const shotId = shotNodeId(shot.id);
      nodes.push({
        id: shotId,
        type: "agentShot",
        parentId: sceneId,
        extent: "parent",
        position: {
          x: SCENE_PADDING + index * (SHOT_WIDTH + SHOT_GAP),
          y: SCENE_HEADER,
        },
        data: { kind: "shot", shot },
        width: SHOT_WIDTH,
        height: SHOT_HEIGHT,
        measured: { width: SHOT_WIDTH, height: SHOT_HEIGHT },
        style: { width: SHOT_WIDTH, height: SHOT_HEIGHT },
        draggable: false,
        selectable: true,
      });
      if (index > 0) {
        edges.push({
          id: `agent-edge-shot-${shots[index - 1].id}-${shot.id}`,
          type: "agentWorkbench",
          source: shotNodeId(shots[index - 1].id),
          target: shotId,
          data: { label: "next" },
        });
      }
    });

    sceneY += sceneHeight + SCENE_GAP;
  }

  return { nodes, edges };
}

function sortedShots(shots: AgentWorkbenchShot[]) {
  return [...shots].sort((left, right) => {
    if (left.sequence_index !== right.sequence_index) {
      return left.sequence_index - right.sequence_index;
    }
    return left.client_key.localeCompare(right.client_key);
  });
}
```

- [ ] **Step 5: Run view-model test**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentWorkbenchViewModel
```

Expected: PASS.

## Task 5: Agent Workbench React Flow Components

**Files:**

- Create: `apps/web/src/components/agent-workbench/AgentWorkbenchCanvas.tsx`
- Create: `apps/web/src/components/agent-workbench/AgentProjectOverviewNode.tsx`
- Create: `apps/web/src/components/agent-workbench/AgentSceneGroupNode.tsx`
- Create: `apps/web/src/components/agent-workbench/AgentShotNode.tsx`
- Create: `apps/web/src/components/agent-workbench/AgentWorkbenchEdge.tsx`

- [ ] **Step 1: Add source-contract test for Workbench components**

In `apps/web/src/lib/agentCanvas.test.mjs`, add file URL constants:

```js
const agentWorkbenchCanvasUrl = new URL(
  "../components/agent-workbench/AgentWorkbenchCanvas.tsx",
  import.meta.url,
);
```

Add test:

```js
it("renders Agent default canvas through the Workbench surface", async () => {
  const pageSource = await readFile(agentPageUrl, "utf8");
  const workbenchSource = await readFile(agentWorkbenchCanvasUrl, "utf8");

  assert.match(pageSource, /AgentWorkbenchCanvas/);
  assert.match(workbenchSource, /agentWorkbenchToFlow/);
  assert.match(workbenchSource, /nodeTypes/);
  assert.match(workbenchSource, /agentOverview/);
  assert.match(workbenchSource, /agentScene/);
  assert.match(workbenchSource, /agentShot/);
});
```

- [ ] **Step 2: Run test and confirm failure**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentCanvas
```

Expected: FAIL because Workbench components are missing.

- [ ] **Step 3: Implement `AgentWorkbenchCanvas`**

Create `apps/web/src/components/agent-workbench/AgentWorkbenchCanvas.tsx`:

```tsx
import { useMemo } from "react";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  type EdgeTypes,
  type NodeTypes,
} from "@xyflow/react";
import type { AgentWorkbenchProjection } from "../../lib/agentWorkbench";
import {
  agentWorkbenchToFlow,
  type AgentWorkbenchEdge as AgentWorkbenchFlowEdge,
  type AgentWorkbenchNode,
} from "../../lib/agentWorkbenchViewModel";
import { AgentWorkbenchEdge } from "./AgentWorkbenchEdge";
import { AgentProjectOverviewNode } from "./AgentProjectOverviewNode";
import { AgentSceneGroupNode } from "./AgentSceneGroupNode";
import { AgentShotNode } from "./AgentShotNode";

interface AgentWorkbenchCanvasProps {
  workbench: AgentWorkbenchProjection;
  selectedObjectId: string | null;
  onSelectObject: (objectId: string | null) => void;
}

const nodeTypes: NodeTypes = {
  agentOverview: AgentProjectOverviewNode,
  agentScene: AgentSceneGroupNode,
  agentShot: AgentShotNode,
};

const edgeTypes: EdgeTypes = {
  agentWorkbench: AgentWorkbenchEdge,
};

export function AgentWorkbenchCanvas(props: AgentWorkbenchCanvasProps) {
  return (
    <ReactFlowProvider>
      <AgentWorkbenchCanvasContent {...props} />
    </ReactFlowProvider>
  );
}

function AgentWorkbenchCanvasContent({
  workbench,
  selectedObjectId,
  onSelectObject,
}: AgentWorkbenchCanvasProps) {
  const flow = useMemo(() => agentWorkbenchToFlow(workbench), [workbench]);
  const nodes = useMemo(
    () =>
      flow.nodes.map((node) => ({
        ...node,
        selected: node.id === selectedObjectId,
      })),
    [flow.nodes, selectedObjectId],
  );

  return (
    <div className="agent-workbench-surface">
      <ReactFlow<AgentWorkbenchNode, AgentWorkbenchFlowEdge>
        fitView
        minZoom={0.2}
        maxZoom={1.4}
        nodes={nodes}
        edges={flow.edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        nodesDraggable={false}
        nodesConnectable={false}
        nodesFocusable
        edgesFocusable={false}
        elementsSelectable
        deleteKeyCode={null}
        onNodeClick={(_, node) => onSelectObject(node.id)}
        onPaneClick={() => onSelectObject(null)}
        panOnDrag
        zoomOnScroll
        zoomOnPinch
        zoomOnDoubleClick
      >
        <Background />
        <MiniMap className="canvas-flow-minimap" pannable position="bottom-left" zoomable />
        <Controls position="bottom-right" showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
```

- [ ] **Step 4: Implement overview node**

Create `apps/web/src/components/agent-workbench/AgentProjectOverviewNode.tsx`:

```tsx
import type { Node, NodeProps } from "@xyflow/react";
import type { AgentWorkbenchOverviewNodeData } from "../../lib/agentWorkbenchViewModel";

type OverviewNode = Node<AgentWorkbenchOverviewNodeData, "agentOverview">;

export function AgentProjectOverviewNode({ data, selected }: NodeProps<OverviewNode>) {
  const workbench = data.workbench;
  const brief = workbench.overview.brief;
  return (
    <section className="agent-workbench-overview-node" data-selected={selected}>
      <header>
        <span>Project</span>
        <strong>{brief?.title || "未命名项目"}</strong>
      </header>
      <p>{brief?.concept || workbench.overview.memory?.soul || "等待 Producer 创建项目约束。"}</p>
      <dl>
        <div>
          <dt>Scenes</dt>
          <dd>{workbench.counts.scenes}</dd>
        </div>
        <div>
          <dt>Shots</dt>
          <dd>{workbench.counts.shots}</dd>
        </div>
        <div>
          <dt>Issues</dt>
          <dd>{workbench.counts.open_issues}</dd>
        </div>
      </dl>
      <div className="agent-workbench-overview-elements">
        {workbench.overview.key_elements.slice(0, 5).map((element) => (
          <span key={element.id}>{element.name}</span>
        ))}
        {workbench.counts.needs_reference > 0 ? (
          <span data-tone="warning">{workbench.counts.needs_reference} needs reference</span>
        ) : null}
      </div>
    </section>
  );
}
```

- [ ] **Step 5: Implement scene group node**

Create `apps/web/src/components/agent-workbench/AgentSceneGroupNode.tsx`:

```tsx
import type { Node, NodeProps } from "@xyflow/react";
import type { AgentWorkbenchSceneNodeData } from "../../lib/agentWorkbenchViewModel";

type SceneNode = Node<AgentWorkbenchSceneNodeData, "agentScene">;

export function AgentSceneGroupNode({ data, selected }: NodeProps<SceneNode>) {
  const scene = data.scene;
  return (
    <section className="agent-workbench-scene-node" data-selected={selected}>
      <header className="agent-workbench-scene-header">
        <div>
          <span>Scene</span>
          <strong>{scene.title || "未命名场景"}</strong>
        </div>
        <em>{scene.status || "planned"}</em>
      </header>
      {scene.location ? <p>{scene.location}</p> : null}
    </section>
  );
}
```

- [ ] **Step 6: Implement shot node**

Create `apps/web/src/components/agent-workbench/AgentShotNode.tsx`:

```tsx
import type { Node, NodeProps } from "@xyflow/react";
import type {
  AgentWorkbenchArtifactSlot,
  AgentWorkbenchShot,
} from "../../lib/agentWorkbench";
import type { AgentWorkbenchShotNodeData } from "../../lib/agentWorkbenchViewModel";

type ShotNode = Node<AgentWorkbenchShotNodeData, "agentShot">;

export function AgentShotNode({ data, selected }: NodeProps<ShotNode>) {
  const shot = data.shot;
  return (
    <article className="agent-workbench-shot-node" data-selected={selected} data-status={shot.status}>
      <header>
        <div>
          <span>{shot.client_key || `#${shot.sequence_index}`}</span>
          <strong>{shot.title || "未命名分镜"}</strong>
        </div>
        <em>{shot.status}</em>
      </header>
      <p>{shot.creative_text || "等待 Producer 补充分镜描述。"}</p>
      <div className="agent-workbench-shot-grid">
        <ArtifactSlot title="Preview" slot={shot.preview} />
        <ArtifactSlot title="Video" slot={shot.video} />
        <ReviewSlot shot={shot} />
      </div>
    </article>
  );
}

function ArtifactSlot({ title, slot }: { title: string; slot: AgentWorkbenchArtifactSlot }) {
  const image = slot.thumbnail_url || slot.access_url;
  return (
    <section className="agent-workbench-artifact-slot" data-status={slot.status}>
      <header>
        <span>{title}</span>
        <em>{slot.status}</em>
      </header>
      {image ? (
        <img alt={slot.title || title} src={image} />
      ) : (
        <div className="agent-workbench-slot-empty">{slot.status === "missing" ? "未生成" : slot.status}</div>
      )}
    </section>
  );
}

function ReviewSlot({ shot }: { shot: AgentWorkbenchShot }) {
  return (
    <section className="agent-workbench-review-slot" data-has-issues={shot.issues.length > 0}>
      <header>
        <span>Review</span>
        <em>{shot.review?.verdict || "none"}</em>
      </header>
      <strong>{shot.review?.score ? shot.review.score.toFixed(1) : "-"}</strong>
      <p>{shot.issues.length > 0 ? `${shot.issues.length} issues` : "无开放问题"}</p>
    </section>
  );
}
```

- [ ] **Step 7: Implement restrained edge**

Create `apps/web/src/components/agent-workbench/AgentWorkbenchEdge.tsx`:

```tsx
import {
  BaseEdge,
  getBezierPath,
  type Edge,
  type EdgeProps,
} from "@xyflow/react";
import type { AgentWorkbenchEdgeData } from "../../lib/agentWorkbenchViewModel";

type WorkbenchEdge = Edge<AgentWorkbenchEdgeData, "agentWorkbench">;

export function AgentWorkbenchEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
}: EdgeProps<WorkbenchEdge>) {
  const [path] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });
  return <BaseEdge id={id} path={path} className="agent-workbench-edge" />;
}
```

- [ ] **Step 8: Run web source test**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentCanvas
```

Expected: source-contract test for new files passes; page integration assertions may still fail until Task 6.

## Task 6: AgentWorkspacePage Integration

**Files:**

- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/lib/agentCanvas.test.mjs`

- [ ] **Step 1: Update Agent canvas source test expectations**

In `apps/web/src/lib/agentCanvas.test.mjs`, replace the test named `"renders Agent through the shared React Flow canvas surface"` with:

```js
it("renders Agent default canvas through the Workbench projection", async () => {
  const pageSource = await readFile(agentPageUrl, "utf8");

  assert.match(pageSource, /AgentWorkbenchCanvas/);
  assert.match(pageSource, /fetchAgentCanvasWorkbench/);
  assert.match(pageSource, /\["workspace", id, "agent-workbench"\]/);
  assert.doesNotMatch(pageSource, /AgentReadonlyCanvas/);
});
```

Keep the old shared-canvas tests that prove layout APIs and mode policies exist only if they still match current source. If they become misleading for default Agent canvas, rename them to state that `AgentFlowCanvas` is a retained debug/compat surface.

- [ ] **Step 2: Run test and confirm failure**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentCanvas
```

Expected: FAIL because page does not query or render Workbench.

- [ ] **Step 3: Import Workbench API and component**

In `apps/web/src/pages/AgentWorkspacePage.tsx`, add imports:

```ts
import { AgentWorkbenchCanvas } from "../components/agent-workbench/AgentWorkbenchCanvas";
import {
  agentWorkbenchVisibleNodeCount,
  type AgentWorkbenchProjection,
} from "../lib/agentWorkbench";
```

Add `fetchAgentCanvasWorkbench` to the existing `agentApi` import list.

- [ ] **Step 4: Add Workbench query**

Near `canvasQuery`, add:

```ts
const workbenchQuery = useQuery<AgentWorkbenchProjection>({
  queryKey: ["workspace", id, "agent-workbench"],
  queryFn: () => fetchAgentCanvasWorkbench(id ?? ""),
  enabled: Boolean(id),
  refetchInterval: (query) =>
    canvasConnectionStatus !== "connected" &&
    hasActiveWorkbenchProduction(query.state.data)
      ? 2_000
      : false,
});
```

Add helper near the bottom of the file:

```ts
function hasActiveWorkbenchProduction(workbench: AgentWorkbenchProjection | undefined) {
  if (!workbench) {
    return false;
  }
  return workbench.scenes.some((scene) =>
    scene.shots.some((shot) =>
      ["queued", "running"].includes(shot.preview.status) ||
      ["queued", "running"].includes(shot.video.status),
    ),
  );
}
```

- [ ] **Step 5: Refetch Workbench alongside canvas**

Update `refetchAgentCanvas`:

```ts
const refetchAgentCanvas = () => {
  void canvasQuery.refetch();
  void workbenchQuery.refetch();
};
```

Where websocket handlers currently update `CanvasPayload` query cache for NodeCreated/NodeUpdated/production events, also invalidate or refetch:

```ts
void queryClient.invalidateQueries({
  queryKey: ["workspace", id, "agent-workbench"],
});
```

Use invalidation after cache mutation to avoid stale Workbench status.

- [ ] **Step 6: Replace default render area**

In the canvas body inside `.agent-canvas-surface`, replace the old `AgentFlowCanvas` branch with:

```tsx
{workbenchQuery.isLoading ? (
  <p className="agent-empty-text">正在加载画布</p>
) : workbenchQuery.data && agentWorkbenchVisibleNodeCount(workbenchQuery.data) > 1 ? (
  <AgentWorkbenchCanvas
    workbench={workbenchQuery.data}
    selectedObjectId={selectedNodeId}
    onSelectObject={setSelectedNodeId}
  />
) : (
  <p className="agent-empty-text">Agent 尚未创建场景或分镜。</p>
)}
```

Update node count:

```ts
const agentCanvasNodeCount = workbenchQuery.data
  ? agentWorkbenchVisibleNodeCount(workbenchQuery.data)
  : 0;
```

M1 should remove the old Agent `PropertyPanel` popover from this default branch. M2 will replace it with Agent-specific details.

- [ ] **Step 7: Run Agent canvas source tests**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentCanvas
```

Expected: PASS after updating stale assertions.

## Task 7: Workbench Styling

**Files:**

- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Add CSS source assertions**

In `apps/web/src/lib/agentCanvas.test.mjs`, add:

```js
it("styles Agent Workbench as scene and shot production groups", async () => {
  const cssSource = await readFile(mainCssUrl, "utf8");

  assert.match(cssSource, /\.agent-workbench-surface/);
  assert.match(cssSource, /\.agent-workbench-scene-node/);
  assert.match(cssSource, /\.agent-workbench-shot-node/);
  assert.match(cssSource, /\.agent-workbench-artifact-slot/);
  assert.match(cssSource, /\.agent-workbench-edge/);
});
```

- [ ] **Step 2: Run test and confirm failure**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentCanvas
```

Expected: FAIL because CSS classes are absent.

- [ ] **Step 3: Add Workbench CSS**

Append near existing canvas-flow styles in `apps/web/src/main.css`:

```css
.agent-workbench-surface {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 420px;
  background:
    radial-gradient(
        circle at 1px 1px,
        var(--color-canvas-pattern) 1px,
        transparent 0
      )
      0 0 / 28px 28px,
    var(--color-canvas);
}

.agent-workbench-surface .react-flow {
  min-height: 420px;
}

.agent-workbench-overview-node,
.agent-workbench-scene-node,
.agent-workbench-shot-node {
  width: 100%;
  height: 100%;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--color-panel-elevated) 92%, transparent);
  box-shadow: var(--shadow-card);
  color: var(--fg-primary);
  overflow: hidden;
}

.agent-workbench-overview-node {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto auto;
  gap: 12px;
  padding: 16px;
}

.agent-workbench-overview-node header,
.agent-workbench-shot-node header,
.agent-workbench-artifact-slot header,
.agent-workbench-review-slot header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}

.agent-workbench-overview-node header span,
.agent-workbench-scene-header span,
.agent-workbench-shot-node header span,
.agent-workbench-artifact-slot header span,
.agent-workbench-review-slot header span {
  color: var(--fg-tertiary);
  font-size: 11px;
  font-weight: 720;
  letter-spacing: 0;
  text-transform: uppercase;
}

.agent-workbench-overview-node header strong,
.agent-workbench-scene-header strong,
.agent-workbench-shot-node header strong {
  display: block;
  overflow: hidden;
  margin-top: 4px;
  font-size: 15px;
  font-weight: 760;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-workbench-overview-node p,
.agent-workbench-shot-node > p,
.agent-workbench-scene-node > p {
  display: -webkit-box;
  overflow: hidden;
  margin: 0;
  color: var(--fg-secondary);
  font-size: 12px;
  line-height: 1.45;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
}

.agent-workbench-overview-node dl {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin: 0;
}

.agent-workbench-overview-node dl div {
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--fg-primary) 4%, transparent);
  padding: 8px;
}

.agent-workbench-overview-node dt {
  color: var(--fg-tertiary);
  font-size: 11px;
}

.agent-workbench-overview-node dd {
  margin: 4px 0 0;
  font-size: 18px;
  font-weight: 760;
}

.agent-workbench-overview-elements {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.agent-workbench-overview-elements span {
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--fg-primary) 7%, transparent);
  color: var(--fg-secondary);
  font-size: 11px;
  padding: 4px 7px;
}

.agent-workbench-overview-elements span[data-tone="warning"] {
  background: color-mix(in srgb, var(--status-stale) 16%, transparent);
  color: var(--status-stale);
}

.agent-workbench-scene-node {
  border-style: dashed;
  background: color-mix(in srgb, var(--color-panel) 58%, transparent);
  pointer-events: none;
}

.agent-workbench-scene-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 16px 18px 0;
}

.agent-workbench-scene-header em,
.agent-workbench-shot-node header em,
.agent-workbench-artifact-slot header em,
.agent-workbench-review-slot header em {
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--fg-primary) 7%, transparent);
  color: var(--fg-secondary);
  font-size: 11px;
  font-style: normal;
  line-height: 1;
  padding: 5px 7px;
  white-space: nowrap;
}

.agent-workbench-shot-node {
  display: grid;
  grid-template-rows: auto minmax(48px, 1fr) 126px;
  gap: 10px;
  padding: 14px;
  pointer-events: all;
}

.agent-workbench-shot-node[data-selected="true"],
.agent-workbench-overview-node[data-selected="true"] {
  box-shadow:
    var(--shadow-card),
    0 0 0 2px color-mix(in srgb, var(--accent) 36%, transparent);
}

.agent-workbench-shot-node[data-status="failed"] {
  border-color: color-mix(in srgb, var(--status-failed) 42%, var(--border-default));
}

.agent-workbench-shot-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  min-height: 0;
}

.agent-workbench-artifact-slot,
.agent-workbench-review-slot {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  gap: 8px;
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--fg-primary) 4%, transparent);
  padding: 8px;
}

.agent-workbench-artifact-slot[data-status="succeeded"] {
  border-color: color-mix(in srgb, var(--status-succeeded) 34%, var(--border-subtle));
}

.agent-workbench-artifact-slot[data-status="failed"],
.agent-workbench-review-slot[data-has-issues="true"] {
  border-color: color-mix(in srgb, var(--status-failed) 42%, var(--border-subtle));
}

.agent-workbench-artifact-slot img {
  width: 100%;
  height: 74px;
  border-radius: var(--radius-sm);
  object-fit: cover;
  background: var(--color-surface-2);
}

.agent-workbench-slot-empty {
  display: grid;
  min-height: 74px;
  place-items: center;
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--fg-primary) 6%, transparent);
  color: var(--fg-tertiary);
  font-size: 12px;
}

.agent-workbench-review-slot strong {
  font-size: 24px;
  line-height: 1;
}

.agent-workbench-review-slot p {
  margin: 0;
  color: var(--fg-secondary);
  font-size: 12px;
}

.agent-workbench-edge {
  stroke: color-mix(in srgb, var(--fg-secondary) 34%, transparent);
  stroke-width: 1.5;
  stroke-dasharray: 5 7;
}
```

- [ ] **Step 4: Run CSS source test**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentCanvas
```

Expected: PASS for CSS assertions.

## Task 8: Backend And Frontend Validation

**Files:**

- No new files unless fixing test fallout.

- [ ] **Step 1: Format Go files**

Run:

```bash
gofmt -w apps/server/internal/api/agent_workbench_projection.go apps/server/internal/api/agent_workbench_projection_test.go apps/server/internal/api/agent_handler.go apps/server/internal/api/agent_handler_test.go apps/server/cmd/server/main.go
```

- [ ] **Step 2: Run backend focused tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestAgentCanvasWorkbenchRouteContract|TestAgentWorkbench|TestAgentProductionOverviewRouteContract' -count=1
```

Expected: PASS.

- [ ] **Step 3: Run broader backend tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Expected: PASS.

- [ ] **Step 4: Run frontend focused tests**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections -- agentWorkbenchViewModel
pnpm --filter @clip-anvil/web test:connections -- agentCanvas
```

Expected: PASS.

- [ ] **Step 5: Run frontend lint and build**

Run:

```bash
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
```

Expected: PASS. If pnpm prints the repo's known Node version warning, note it but do not treat it as failure unless the command exits non-zero.

- [ ] **Step 6: Run diff check**

Run:

```bash
git diff --check
```

Expected: no output.

## Task 9: Manual Browser Smoke

**Files:**

- No code changes unless smoke finds defects.

- [ ] **Step 1: Inspect dev profile**

Run:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

Expected: script prints the current worktree backend and frontend URLs without starting services.

- [ ] **Step 2: Start dev stack**

Run:

```bash
./scripts/dev-start.sh
```

Expected: script starts or reuses the current worktree services and prints the Vite URL.

- [ ] **Step 3: Open an Agent workspace with existing Scene / Shot data**

Use the printed Vite URL and open:

```text
<vite-url>/workspaces/e3449333-41b0-44b1-9a56-8c1e9d77d3d5/agent
```

Expected: page loads without console/runtime errors.

- [ ] **Step 4: Verify workbench layout**

Expected visible result:

- One compact project overview node.
- One scene group for the luggage ad scene.
- Four shot cards inside the scene group.
- Preview / Video / Review blocks are inside each shot card.
- RenderPlan nodes are not rendered as a long separate vertical stack.
- Low-value dashed domain edges no longer cross the main view.

- [ ] **Step 5: Verify refresh behavior**

Trigger a backend state change by sending an Agent message or waiting for an active generation event.

Expected:

- Workbench query refetches after Agent or Canvas websocket event.
- Shot preview/video status changes without requiring a full browser reload when websocket is connected.
- If websocket is disconnected, polling updates active queued/running slots.

- [ ] **Step 6: Stop dev stack when finished**

Run:

```bash
./scripts/dev-stop.sh
```

Expected: current worktree services stop.

## Task 10: Documentation And Final Check

**Files:**

- Modify: `docs/superpowers/specs/2026-06-26-agent-canvas-workbench-design.md` only if implementation discovers a M1 scope correction.
- Optionally modify: `docs/engineering/agent-multiagent-architecture.md` if the default Agent canvas behavior changes enough that current text becomes misleading.

- [ ] **Step 1: Check whether docs need a small update**

Run:

```bash
rg -n "Agent 画布|domain_projection|RenderPlan.*画布|只读 React Flow 画布|flat|平铺" docs/engineering docs/design docs/superpowers/specs/2026-06-26-agent-canvas-workbench-design.md
```

Expected: identify any current doc wording that now incorrectly says the default Agent canvas is a flat domain projection.

- [ ] **Step 2: Update docs only if current docs are misleading**

If needed, add one short note to `docs/engineering/agent-multiagent-architecture.md` under current status:

```md
- Agent Canvas Workbench M1 后，Agent 默认画布使用 Scene / Shot 分组投影；旧 `domain_projection` 仅作为调试/兼容投影保留，不再是默认用户视图。
```

- [ ] **Step 3: Final verification**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected: all commands pass.

## M1 Acceptance Checklist

- [ ] `GET /api/agent/workspaces/:workspaceID/canvas/workbench` returns `overview + scenes + shots + counts`.
- [ ] Endpoint rejects non-owned or non-Agent workspaces through existing `agentWorkspaceForRequest`.
- [ ] Workbench projection groups shots under scenes.
- [ ] Preview and video slots are derived from Agent-owned media nodes by `shot_id` and `agent_artifact_kind`.
- [ ] Default Agent page renders `AgentWorkbenchCanvas`, not flat `DomainFlowNode` graph.
- [ ] A workspace with one scene and four shots shows one overview, one scene group, and four shot cards.
- [ ] RenderPlan / Review / Issue do not appear as default scattered nodes.
- [ ] Node count reflects visible workbench objects.
- [ ] WebSocket/canvas events refetch the workbench query.
- [ ] Focused backend tests pass.
- [ ] Frontend view-model/source tests pass.
- [ ] `make server-test`, web lint, web build, and `git diff --check` pass.

## Commit Guidance

When executing, commit only after Task 8 passes unless the user asks for smaller commits. Use explicit staging:

```bash
git add apps/server/internal/api/agent_workbench_projection.go \
  apps/server/internal/api/agent_workbench_projection_test.go \
  apps/server/internal/api/agent_handler.go \
  apps/server/internal/api/agent_handler_test.go \
  apps/server/cmd/server/main.go \
  apps/web/src/lib/agentWorkbench.ts \
  apps/web/src/lib/agentWorkbenchViewModel.ts \
  apps/web/src/lib/agentWorkbenchViewModel.test.mjs \
  apps/web/src/lib/agentApi.ts \
  apps/web/src/pages/AgentWorkspacePage.tsx \
  apps/web/src/components/agent-workbench/AgentWorkbenchCanvas.tsx \
  apps/web/src/components/agent-workbench/AgentProjectOverviewNode.tsx \
  apps/web/src/components/agent-workbench/AgentSceneGroupNode.tsx \
  apps/web/src/components/agent-workbench/AgentShotNode.tsx \
  apps/web/src/components/agent-workbench/AgentWorkbenchEdge.tsx \
  apps/web/src/lib/agentCanvas.test.mjs \
  apps/web/src/main.css \
  apps/web/package.json \
  docs/superpowers/plans/2026-06-26-agent-canvas-workbench-m1-implementation.md
git commit -m "feat: add agent canvas workbench m1"
```
