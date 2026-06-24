# M6.6 Closure Preview Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the existing M6.6 Craftsman/Worker preview generation loop so Agent preview images have correct scoped context, resolved input refs, shot status lifecycle, durable async completion events, canvas websocket parity, and real browser E2E acceptance.

**Architecture:** Keep the current Producer -> `dispatch_craftsman` -> CraftsmanGraph -> Worker -> `production.Service.SubmitGenerationIntent` architecture. Add focused storage queries, a Worker input-ref resolver, a shot preview status reducer, and an Agent event bridge from production terminal events. Keep preview generation separate from M6.7 review/retry and M6.8 video/composer.

**Tech Stack:** Go 1.26, CloudWeGo Eino, Hertz, pgx/sqlc, PostgreSQL, ClipAnvil production service, Agent runtime service, Canvas/Agent WebSocket hubs, React 19/Vite/React Flow for browser E2E.

---

## Scope

This plan implements the M6.6 Closure section from `docs/superpowers/specs/2026-06-23-m6-agent-mode-completion-design.md`.

In scope:

- Expand Craftsman scoped context.
- Resolve Worker `input_node_refs` into `production.InputRef`.
- Create dependency edges for resolved preview inputs.
- Persist preview shot status transitions.
- Emit durable Agent events when preview generation terminal production events occur.
- Make Agent-created canvas websocket node payloads use the same UI-ready response node as canvas GET.
- Add explicit unit, integration, and browser E2E acceptance standards.

Out of scope:

- Review rubric and ReviewerGraph.
- Critique-driven retry.
- Video generation.
- Composer/final video.
- Workspace Memory.
- Studio / Agent import-export.

---

## File Structure

### SQL / sqlc

- Modify: `apps/server/sqlc/queries/shot.sql`
  - Add `UpdateShotStatus`.
- Modify: `apps/server/sqlc/queries/node.sql`
  - Add `ListSourceMaterialNodesByWorkspace`.
- Review: `apps/server/sqlc/queries/edge.sql`
  - Existing `GetDependencyEdgeByEndpoints` and `CreateMediaEdge` are sufficient for edge sync.
- Generated: `apps/server/internal/store/db/*.sql.go`
  - Regenerate with `make sqlc-generate`.

### Craftsman scoped context

- Modify: `apps/server/internal/agent/craftsman/types.go`
  - Add context fields for dependencies and source material node states.
- Modify: `apps/server/internal/agent/craftsman/context_loader.go`
  - Load related shot dependencies and source material candidates.
- Modify: `apps/server/internal/agent/craftsman/context_loader_test.go`
  - Assert context includes dependencies and source material without leaking unrelated shot generated nodes.

### Worker input refs and edge sync

- Create: `apps/server/internal/agent/worker/input_refs.go`
  - Resolve input refs by UUID or unambiguous title.
  - Convert source/generated nodes into `production.InputRef`.
  - Ensure dependency edge exists.
- Create: `apps/server/internal/agent/worker/input_refs_test.go`
  - Test source material, generated current winner, ambiguous title, missing ref, edge idempotency.
- Modify: `apps/server/internal/agent/worker/executor.go`
  - Inject/use resolver before `SubmitGenerationIntent`.
- Modify: `apps/server/internal/agent/worker/executor_test.go`
  - Assert `GenerationIntent.InputRefs` is populated when Worker input contains refs.
- Modify: `apps/server/internal/agent/worker/types.go`
  - Keep current string refs but document accepted values in comments or helper names. No schema expansion in this phase.

### Shot preview status

- Create: `apps/server/internal/agent/preview/status.go`
  - Define preview status reducer constants and helpers.
- Create: `apps/server/internal/agent/preview/status_test.go`
  - Cover dispatch, submitted, succeeded, failed, and force regeneration transitions.
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman.go`
  - Set selected shots to `preview_running` after successful task creation.
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman_test.go`
  - Assert dispatch updates selected shot status.
- Modify: `apps/server/internal/agent/worker/executor.go`
  - Mark shot `failed` on synchronous Worker failures when no submission happened.
- Modify: `apps/server/internal/agent/worker/executor_test.go`
  - Assert invalid input or submit failure can mark shot failed through store call.

### Production terminal event bridge

- Create: `apps/server/internal/agent/preview/events.go`
  - Build preview terminal event payloads from node/job/version facts.
- Create: `apps/server/internal/agent/preview/events_test.go`
  - Cover success/failure event payload and non-preview node filtering.
- Modify: `apps/server/internal/api/production_broadcaster.go`
  - When production terminal event targets an Agent preview node, update shot status and create/broadcast Agent event.
- Modify: `apps/server/internal/api/production_broadcaster_test.go`
  - Assert preview success/failure emits Agent event and updates shot status.
- Modify: `apps/server/cmd/server/main.go`
  - Wire `ProductionBroadcaster` with Agent runtime/broadcaster or a focused preview event sink.

### Canvas websocket payload parity

- Modify: `apps/server/internal/api/canvas_handler.go`
  - Extract a reusable single-node response builder if needed.
- Modify: `apps/server/internal/api/production_broadcaster.go`
  - Reuse that response builder for terminal `NodeUpdated`.
- Modify: `apps/server/cmd/server/main.go`
  - Replace raw `db.MediaNode` broadcast in `agentCanvasNodeBroadcaster`.
- Modify: `apps/server/internal/api/canvas_handler_test.go`
  - Assert single-node broadcast response includes parsed metadata and production preview fields when present.
- Modify: `apps/web/src/lib/agentReadonlyCanvas.test.mjs`
  - Keep source-level guard that Agent workspace merges websocket node snapshots into cache.

### Browser E2E / smoke

- Create: `scripts/smoke-m6-6-preview-closure.sh`
  - API/database smoke for storyboard -> dispatch -> Worker submission with mock provider.
- Optional create if project pattern supports it: `apps/web/e2e/m6-6-preview-closure.spec.ts`
  - Browser flow against running Vite app.
- No production provider secrets are required for deterministic smoke; use existing mock provider path where possible.

---

## Task 1: Storage Queries And Generated Types

**Files:**

- Modify: `apps/server/sqlc/queries/shot.sql`
- Modify: `apps/server/sqlc/queries/node.sql`
- Generated: `apps/server/internal/store/db/*.sql.go`

- [ ] **Step 1: Add `UpdateShotStatus` query**

Append to `apps/server/sqlc/queries/shot.sql`:

```sql
-- name: UpdateShotStatus :one
UPDATE shot
SET status = $3,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND archived_at IS NULL
RETURNING *;
```

- [ ] **Step 2: Add source material query**

Append to `apps/server/sqlc/queries/node.sql`:

```sql
-- name: ListSourceMaterialNodesByWorkspace :many
SELECT *
FROM media_node
WHERE workspace_id = $1
  AND asset_id IS NOT NULL
  AND source = 'agent'
  AND operation_type = 'upload'
ORDER BY created_at;
```

- [ ] **Step 3: Generate sqlc code**

Run:

```bash
make sqlc-generate
```

Expected:

- `apps/server/internal/store/db/shot.sql.go` contains `UpdateShotStatus`.
- `apps/server/internal/store/db/node.sql.go` contains `ListSourceMaterialNodesByWorkspace`.

- [ ] **Step 4: Verify generated code compiles**

Run:

```bash
make server-build
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/server/sqlc/queries/shot.sql apps/server/sqlc/queries/node.sql apps/server/internal/store/db
git commit -m "feat: add m6 preview closure storage queries"
```

---

## Task 2: Expanded Craftsman Scoped Context

**Files:**

- Modify: `apps/server/internal/agent/craftsman/types.go`
- Modify: `apps/server/internal/agent/craftsman/context_loader.go`
- Modify: `apps/server/internal/agent/craftsman/context_loader_test.go`

- [ ] **Step 1: Write failing context test**

Add a test to `apps/server/internal/agent/craftsman/context_loader_test.go`:

```go
func TestContextLoaderIncludesDependenciesAndSourceMaterials(t *testing.T) {
	store := &fakeContextStore{
		shot: db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-02", Title: "卖点证明", Status: "planned"},
		nodes: []db.MediaNode{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), Title: "shot-02 preview", NodeType: "image", Status: "queued"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(3), Title: "other shot preview", NodeType: "image", Status: "queued"},
		},
		sourceNodes: []db.MediaNode{
			{ID: uuidWithByte(13), WorkspaceID: uuidWithByte(1), Title: "product.png", NodeType: "image", Status: "succeeded", Source: "agent", OperationType: "upload", AssetID: uuidWithByte(33)},
		},
		dependencies: []db.ShotDependency{
			{WorkspaceID: uuidWithByte(1), FromShotID: uuidWithByte(1), ToShotID: uuidWithByte(2), DependencyType: "continuity", BlockingPhase: "preview", InjectionRole: "style_ref", Reason: "保持商品摆位连续"},
		},
		jobs: map[pgtype.UUID][]db.GenerationJob{
			uuidWithByte(11): {{ID: uuidWithByte(21), TargetNodeID: uuidWithByte(11), OperationType: "text_to_image", Status: db.JobStatusQueued}},
		},
		versions: map[pgtype.UUID][]db.ArtifactVersion{
			uuidWithByte(11): {{ID: uuidWithByte(31), NodeID: uuidWithByte(11), Status: db.JobStatusQueued}},
		},
	}
	loader := ContextLoader{Store: store, Runtime: &fakeMessageRuntime{}}

	out, err := loader.Load(context.Background(), GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(4),
		TaskID:      uuidWithByte(5),
		ShotID:      uuidWithByte(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"shot-02", "product.png", "continuity", "blocking_phase=preview", "style_ref"} {
		if !strings.Contains(out.Text, want) {
			t.Fatalf("context text missing %q: %s", want, out.Text)
		}
	}
	if strings.Contains(out.Text, "other shot preview") {
		t.Fatalf("context leaked unrelated shot generated node: %s", out.Text)
	}
}
```

Extend `fakeContextStore` with:

```go
sourceNodes  []db.MediaNode
dependencies []db.ShotDependency
```

and methods:

```go
func (f *fakeContextStore) ListSourceMaterialNodesByWorkspace(_ context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error) {
	out := []db.MediaNode{}
	for _, node := range f.sourceNodes {
		if node.WorkspaceID == workspaceID {
			out = append(out, node)
		}
	}
	return out, nil
}

func (f *fakeContextStore) ListShotDependenciesByWorkspace(_ context.Context, workspaceID pgtype.UUID) ([]db.ShotDependency, error) {
	out := []db.ShotDependency{}
	for _, dep := range f.dependencies {
		if dep.WorkspaceID == workspaceID {
			out = append(out, dep)
		}
	}
	return out, nil
}
```

- [ ] **Step 2: Run failing test**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/craftsman -run TestContextLoaderIncludesDependenciesAndSourceMaterials -count=1
```

Expected: FAIL because `ContextStore` does not expose dependency/source material methods and context text lacks those sections.

- [ ] **Step 3: Extend Craftsman context types**

In `apps/server/internal/agent/craftsman/types.go`, add:

```go
type ShotDependencyState struct {
	Dependency db.ShotDependency
	Direction  string
}
```

and extend `Context`:

```go
Dependencies   []ShotDependencyState
SourceMaterials []NodeState
```

- [ ] **Step 4: Extend `ContextStore` and loader**

In `context_loader.go`, extend `ContextStore`:

```go
ListShotDependenciesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.ShotDependency, error)
ListSourceMaterialNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
```

Load dependencies where `FromShotID == input.ShotID || ToShotID == input.ShotID`.

Load source material nodes through `ListSourceMaterialNodesByWorkspace`, and load their jobs/versions through the same `NodeState` builder.

- [ ] **Step 5: Render context text**

Update `buildContextText` signature to:

```go
func buildContextText(shot db.Shot, nodes []NodeState, sourceMaterials []NodeState, dependencies []ShotDependencyState) string
```

Add sections:

```text
Source Materials
- product.png (image, succeeded) asset=<id>

Shot Dependencies
- incoming continuity from <shot-id> blocking_phase=preview role=style_ref reason=保持商品摆位连续
```

- [ ] **Step 6: Run craftsman tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/craftsman -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/server/internal/agent/craftsman/types.go apps/server/internal/agent/craftsman/context_loader.go apps/server/internal/agent/craftsman/context_loader_test.go
git commit -m "feat: expand craftsman preview context"
```

---

## Task 3: Worker Input Ref Resolver And Dependency Edge Sync

**Files:**

- Create: `apps/server/internal/agent/worker/input_refs.go`
- Create: `apps/server/internal/agent/worker/input_refs_test.go`
- Modify: `apps/server/internal/agent/worker/executor.go`
- Modify: `apps/server/internal/agent/worker/executor_test.go`

- [ ] **Step 1: Define resolver store interface and result**

Create `apps/server/internal/agent/worker/input_refs.go` with:

```go
package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type InputRefStore interface {
	GetMediaNodeByID(ctx context.Context, id pgtype.UUID) (db.MediaNode, error)
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
	GetMediaAssetByID(ctx context.Context, id pgtype.UUID) (db.MediaAsset, error)
	ListMediaNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
	GetDependencyEdgeByEndpoints(ctx context.Context, params db.GetDependencyEdgeByEndpointsParams) (db.MediaEdge, error)
	CreateMediaEdge(ctx context.Context, params db.CreateMediaEdgeParams) (db.MediaEdge, error)
}

type InputRefResolver struct {
	Store InputRefStore
}

func (r InputRefResolver) Resolve(ctx context.Context, workspaceID pgtype.UUID, targetNode db.MediaNode, refs []string) ([]production.InputRef, error) {
	// implementation added in later step
	return nil, nil
}

func normalizeRef(value string) string {
	return strings.TrimSpace(value)
}

func errInputRef(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, message)
}
```

- [ ] **Step 2: Write failing tests**

Create `apps/server/internal/agent/worker/input_refs_test.go` with tests:

```go
func TestInputRefResolverResolvesSourceMaterialByTitle(t *testing.T) {
	store := &fakeInputRefStore{
		nodes: []db.MediaNode{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), Title: "product.png", NodeType: db.NodeTypeImage, Source: "agent", OperationType: "upload", AssetID: uuidWithByte(21)},
		},
		assets: map[pgtype.UUID]db.MediaAsset{
			uuidWithByte(21): {ID: uuidWithByte(21), WorkspaceID: uuidWithByte(1), Type: db.AssetTypeImage, Mime: "image/png", StorageUrl: pgtype.Text{String: "workspace/input.png", Valid: true}},
		},
	}
	resolver := InputRefResolver{Store: store}

	refs, err := resolver.Resolve(context.Background(), uuidWithByte(1), db.MediaNode{ID: uuidWithByte(30), WorkspaceID: uuidWithByte(1)}, []string{"product.png"})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].AssetID != uuidString(uuidWithByte(21)) || refs[0].AssetType != "image" {
		t.Fatalf("refs = %#v", refs)
	}
	if len(store.createdEdges) != 1 || store.createdEdges[0].FromNodeID != uuidWithByte(11) || store.createdEdges[0].ToNodeID != uuidWithByte(30) {
		t.Fatalf("edges = %#v", store.createdEdges)
	}
}

func TestInputRefResolverResolvesGeneratedNodeCurrentWinner(t *testing.T) {
	store := &fakeInputRefStore{
		nodes: []db.MediaNode{
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), Title: "approved preview", NodeType: db.NodeTypeImage, Source: "agent", OperationType: "text_to_image", CurrentVersionID: uuidWithByte(22)},
		},
		versions: map[pgtype.UUID]db.ArtifactVersion{
			uuidWithByte(22): {ID: uuidWithByte(22), NodeID: uuidWithByte(12), AssetID: uuidWithByte(23), Status: db.JobStatusSucceeded},
		},
		assets: map[pgtype.UUID]db.MediaAsset{
			uuidWithByte(23): {ID: uuidWithByte(23), WorkspaceID: uuidWithByte(1), Type: db.AssetTypeImage, Mime: "image/png", StorageUrl: pgtype.Text{String: "workspace/generated.png", Valid: true}},
		},
	}
	resolver := InputRefResolver{Store: store}

	refs, err := resolver.Resolve(context.Background(), uuidWithByte(1), db.MediaNode{ID: uuidWithByte(30), WorkspaceID: uuidWithByte(1)}, []string{uuidString(uuidWithByte(12))})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].VersionID != uuidString(uuidWithByte(22)) || refs[0].StorageURL != "workspace/generated.png" {
		t.Fatalf("refs = %#v", refs)
	}
}

func TestInputRefResolverRejectsAmbiguousTitle(t *testing.T) {
	store := &fakeInputRefStore{nodes: []db.MediaNode{
		{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), Title: "product", AssetID: uuidWithByte(21)},
		{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), Title: "product", AssetID: uuidWithByte(22)},
	}}
	resolver := InputRefResolver{Store: store}
	_, err := resolver.Resolve(context.Background(), uuidWithByte(1), db.MediaNode{ID: uuidWithByte(30), WorkspaceID: uuidWithByte(1)}, []string{"product"})
	if err == nil {
		t.Fatal("expected ambiguous title error")
	}
}
```

Add fake store methods for all `InputRefStore` methods.

- [ ] **Step 3: Run failing tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/worker -run TestInputRefResolver -count=1
```

Expected: FAIL because resolver is incomplete.

- [ ] **Step 4: Implement resolver**

Implement in `input_refs.go`:

- Empty refs return empty slice.
- UUID ref resolves via `GetMediaNodeByID`.
- Non-UUID ref scans `ListMediaNodesByWorkspace` for case-insensitive exact `Title` match.
- 0 matches: `invalid worker input: input ref <ref> not found`.
- 2+ matches: `invalid worker input: input ref <ref> is ambiguous`.
- Source material node uses `node.AssetID`.
- Generated node requires `node.CurrentVersionID`.
- For every resolved node, call `ensureDependencyEdge`.

For each node, build:

```go
production.InputRef{
	NodeID:     uuidString(node.ID),
	NodeType:   string(node.NodeType),
	VersionID:  uuidString(version.ID), // generated nodes only
	AssetID:    uuidString(asset.ID),
	AssetType:  string(asset.Type),
	Mime:       asset.Mime,
	StorageURL: textString(asset.StorageUrl),
	TextContent: textString(asset.TextContent),
	Kind:       production.InputKindExplicit,
}
```

Use the existing `uuidString` helper in `executor.go`; if duplicate helper conflicts, move shared helpers to a package-local `util.go`.

- [ ] **Step 5: Wire resolver into executor**

In `ExecutorConfig`, add:

```go
InputRefs InputRefResolver
```

In `RunTask`, after `node` is resolved:

```go
resolver := e.inputRefs
if resolver.Store == nil {
	resolver = InputRefResolver{Store: e.store}
}
inputRefs, err := resolver.Resolve(ctx, task.WorkspaceID, node, workerInput.InputNodeRefs)
if err != nil {
	return e.fail(ctx, task, "worker_generation_input_ref_failed", err)
}
intent := generationIntent(task, workerInput, node, inputRefs)
```

Change `generationIntent` signature:

```go
func generationIntent(task db.AgentTask, input GenerationInput, node db.MediaNode, inputRefs []production.InputRef) production.GenerationIntent
```

and set `InputRefs: inputRefs`.

- [ ] **Step 6: Add executor regression test**

In `executor_test.go`, add:

```go
func TestWorkerPassesResolvedInputRefsToGenerationIntent(t *testing.T) {
	store := &fakeWorkerStore{
		nodes: []db.MediaNode{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), Title: "product.png", NodeType: db.NodeTypeImage, Source: "agent", OperationType: "upload", AssetID: uuidWithByte(21)},
		},
		assets: map[pgtype.UUID]db.MediaAsset{
			uuidWithByte(21): {ID: uuidWithByte(21), WorkspaceID: uuidWithByte(1), Type: db.AssetTypeImage, Mime: "image/png", StorageUrl: pgtype.Text{String: "workspace/input.png", Valid: true}},
		},
	}
	productionService := &fakeProductionSubmitter{result: production.RunResult{Node: db.MediaNode{ID: uuidWithByte(20)}, Job: db.GenerationJob{ID: uuidWithByte(30)}, Version: db.ArtifactVersion{ID: uuidWithByte(40)}}}
	executor := NewExecutor(ExecutorConfig{Runtime: &fakeWorkerRuntime{}, Store: store, Production: productionService})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:          "preview_image",
		ShotID:        uuidString(uuidWithByte(2)),
		Prompt:        "prompt",
		InputNodeRefs: []string{"product.png"},
		MaxAttempts:   3,
	})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if len(productionService.intent.InputRefs) != 1 || productionService.intent.InputRefs[0].AssetID != uuidString(uuidWithByte(21)) {
		t.Fatalf("input refs = %#v", productionService.intent.InputRefs)
	}
}
```

Extend `fakeWorkerStore` with the methods and maps required by `InputRefStore`.

- [ ] **Step 7: Run worker tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/worker -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/server/internal/agent/worker/input_refs.go apps/server/internal/agent/worker/input_refs_test.go apps/server/internal/agent/worker/executor.go apps/server/internal/agent/worker/executor_test.go apps/server/internal/agent/worker/types.go
git commit -m "feat: resolve agent worker preview inputs"
```

---

## Task 4: Preview Shot Status Lifecycle

**Files:**

- Create: `apps/server/internal/agent/preview/status.go`
- Create: `apps/server/internal/agent/preview/status_test.go`
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman.go`
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman_test.go`
- Modify: `apps/server/internal/agent/worker/executor.go`
- Modify: `apps/server/internal/agent/worker/executor_test.go`

- [ ] **Step 1: Add preview status reducer tests**

Create `apps/server/internal/agent/preview/status_test.go`:

```go
package preview

import "testing"

func TestStatusAfterDispatch(t *testing.T) {
	if got := StatusAfterDispatch("planned", false); got != "preview_running" {
		t.Fatalf("status = %q", got)
	}
	if got := StatusAfterDispatch("preview_ready", false); got != "preview_ready" {
		t.Fatalf("status = %q", got)
	}
	if got := StatusAfterDispatch("preview_ready", true); got != "preview_running" {
		t.Fatalf("status = %q", got)
	}
}

func TestStatusAfterTerminalPreviewJob(t *testing.T) {
	if got := StatusAfterTerminalPreviewJob("succeeded", true); got != "preview_ready" {
		t.Fatalf("status = %q", got)
	}
	if got := StatusAfterTerminalPreviewJob("failed", false); got != "failed" {
		t.Fatalf("status = %q", got)
	}
	if got := StatusAfterTerminalPreviewJob("cancelled", false); got != "failed" {
		t.Fatalf("status = %q", got)
	}
}
```

- [ ] **Step 2: Implement reducer**

Create `apps/server/internal/agent/preview/status.go`:

```go
package preview

func StatusAfterDispatch(current string, force bool) string {
	switch current {
	case "preview_ready":
		if !force {
			return current
		}
	case "archived":
		return current
	}
	return "preview_running"
}

func StatusAfterTerminalPreviewJob(jobStatus string, hasWinner bool) string {
	if jobStatus == "succeeded" && hasWinner {
		return "preview_ready"
	}
	return "failed"
}
```

- [ ] **Step 3: Run preview tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/preview -count=1
```

Expected: PASS.

- [ ] **Step 4: Extend dispatch store interface**

In `apps/server/internal/agent/tools/dispatch_craftsman.go`, extend `CraftsmanDispatcherStore`:

```go
UpdateShotStatus(ctx context.Context, params db.UpdateShotStatusParams) (db.Shot, error)
```

After `SetShotCraftsmanThread`, call:

```go
_, _ = t.store.UpdateShotStatus(ctx, db.UpdateShotStatusParams{
	ID:          shot.ID,
	WorkspaceID: input.WorkspaceID,
	Status:      agentpreview.StatusAfterDispatch(shot.Status, args.Force),
})
```

Import:

```go
agentpreview "github.com/sinmaystar/clip-anvil/internal/agent/preview"
```

- [ ] **Step 5: Update dispatch test**

In `dispatch_craftsman_test.go`, extend fake store to record status updates and assert:

```go
if len(store.updatedStatuses) != 3 || store.updatedStatuses[0].status != "preview_running" {
	t.Fatalf("status updates = %#v", store.updatedStatuses)
}
```

- [ ] **Step 6: Worker synchronous failure updates shot failed**

Extend Worker `Store` interface:

```go
UpdateShotStatus(ctx context.Context, params db.UpdateShotStatusParams) (db.Shot, error)
```

In `fail`, if `task.ScopeType == "shot"` and `task.ScopeID.Valid`, call:

```go
_, _ = e.store.UpdateShotStatus(ctx, db.UpdateShotStatusParams{
	ID:          task.ScopeID,
	WorkspaceID: task.WorkspaceID,
	Status:      "failed",
})
```

Only call this for failure codes before async production owns status:

- `worker_generation_invalid_input`
- `worker_generation_node_failed`
- `worker_generation_input_ref_failed`
- `worker_generation_submit_failed`

- [ ] **Step 7: Worker failure test**

In `executor_test.go`, add:

```go
func TestWorkerMarksShotFailedOnSynchronousSubmitFailure(t *testing.T) {
	store := &fakeWorkerStore{}
	executor := NewExecutor(ExecutorConfig{
		Runtime:    &fakeWorkerRuntime{},
		Store:      store,
		Production: &fakeProductionSubmitter{err: errors.New("provider unavailable")},
	})
	task := workerTaskWithInput(t, GenerationInput{Mode: "preview_image", ShotID: uuidString(uuidWithByte(2)), Prompt: "prompt", MaxAttempts: 1})
	_ = executor.RunTask(context.Background(), RunTaskInput{Task: task})
	if store.updatedShotStatus != "failed" {
		t.Fatalf("shot status = %q", store.updatedShotStatus)
	}
}
```

- [ ] **Step 8: Run affected tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/preview ./internal/agent/tools ./internal/agent/worker -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/server/internal/agent/preview/status.go apps/server/internal/agent/preview/status_test.go apps/server/internal/agent/tools/dispatch_craftsman.go apps/server/internal/agent/tools/dispatch_craftsman_test.go apps/server/internal/agent/worker/executor.go apps/server/internal/agent/worker/executor_test.go
git commit -m "feat: track preview shot status"
```

---

## Task 5: Production Terminal Event Bridge And Canvas Payload Parity

**Files:**

- Create: `apps/server/internal/agent/preview/events.go`
- Create: `apps/server/internal/agent/preview/events_test.go`
- Modify: `apps/server/internal/api/canvas_handler.go`
- Modify: `apps/server/internal/api/production_broadcaster.go`
- Modify: `apps/server/internal/api/production_broadcaster_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Add preview event payload tests**

Create `apps/server/internal/agent/preview/events_test.go`:

```go
package preview

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestIsPreviewNode(t *testing.T) {
	node := db.MediaNode{Source: "agent", OperationType: "text_to_image", ShotID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, Metadata: []byte(`{"agent_artifact_kind":"preview_image"}`)}
	if !IsPreviewNode(node) {
		t.Fatal("expected preview node")
	}
	if IsPreviewNode(db.MediaNode{Source: "user", OperationType: "text_to_image", Metadata: []byte(`{"agent_artifact_kind":"preview_image"}`)}) {
		t.Fatal("user node must not be treated as Agent preview")
	}
}

func TestTerminalEventType(t *testing.T) {
	if TerminalEventType("succeeded") != "preview_generation_succeeded" {
		t.Fatalf("success event = %q", TerminalEventType("succeeded"))
	}
	if TerminalEventType("failed") != "preview_generation_failed" {
		t.Fatalf("failure event = %q", TerminalEventType("failed"))
	}
}
```

- [ ] **Step 2: Implement preview event helpers**

Create `apps/server/internal/agent/preview/events.go`:

```go
package preview

import (
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func IsPreviewNode(node db.MediaNode) bool {
	if node.Source != "agent" || node.OperationType != "text_to_image" || !node.ShotID.Valid {
		return false
	}
	var metadata map[string]any
	if err := json.Unmarshal(node.Metadata, &metadata); err != nil {
		return false
	}
	return metadata["agent_artifact_kind"] == "preview_image"
}

func TerminalEventType(status string) string {
	if status == "succeeded" {
		return "preview_generation_succeeded"
	}
	return "preview_generation_failed"
}

func HasWinner(node db.MediaNode) bool {
	return node.CurrentVersionID.Valid
}

func UUIDValid(id pgtype.UUID) bool {
	return id.Valid
}
```

- [ ] **Step 3: Extract single canvas node response builder**

In `apps/server/internal/api/canvas_handler.go`, add:

```go
func buildCanvasNodeResponse(ctx context.Context, queries *db.Queries, signer assetURLSigner, node db.MediaNode) (canvasNodeResponse, error) {
	assets, err := queries.ListMediaAssetsByWorkspace(ctx, node.WorkspaceID)
	if err != nil {
		return canvasNodeResponse{}, err
	}
	assetsByID := make(map[pgtype.UUID]db.MediaAsset, len(assets))
	for _, asset := range assets {
		assetsByID[asset.ID] = asset
	}
	versionsByID := map[pgtype.UUID]db.ArtifactVersion{}
	if node.CurrentVersionID.Valid {
		version, err := queries.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil {
			return canvasNodeResponse{}, err
		}
		versionsByID[node.CurrentVersionID] = version
	}
	staleReasons, err := queries.ListActiveStaleReasonsByNode(ctx, node.ID)
	if err != nil {
		return canvasNodeResponse{}, err
	}
	responses, err := toCanvasNodeResponsesWithSigner(ctx, signer, []db.MediaNode{node}, assetsByID, versionsByID, map[pgtype.UUID]int{node.ID: len(staleReasons)}, map[pgtype.UUID][]db.MediaNode{})
	if err != nil {
		return canvasNodeResponse{}, err
	}
	if len(responses) == 0 {
		return canvasNodeResponse{}, err
	}
	return responses[0], nil
}
```

Then use this helper in `ProductionBroadcaster.broadcastNodeSnapshot`.

- [ ] **Step 4: Extend `ProductionBroadcaster` with Agent preview event sink**

In `apps/server/internal/api/production_broadcaster.go`, add interfaces:

```go
type agentPreviewRuntime interface {
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type agentPreviewBroadcaster interface {
	BroadcastAgentEvent(workspaceID pgtype.UUID, event db.AgentEvent)
}
```

Extend struct:

```go
agentRuntime agentPreviewRuntime
agentEvents  agentPreviewBroadcaster
```

Add constructor options or new constructor:

```go
func NewProductionBroadcaster(hub *CanvasHub, queries *db.Queries, storage assetURLSigner, agentRuntime agentPreviewRuntime, agentEvents agentPreviewBroadcaster) *ProductionBroadcaster
```

Update call sites in `cmd/server/main.go`.

- [ ] **Step 5: Bridge terminal event**

In `broadcastNodeSnapshot`, after loading `node` and broadcasting `NodeUpdated`, if `agentpreview.IsPreviewNode(node)`:

```go
status := statusForProductionEvent(event.Type)
nextShotStatus := agentpreview.StatusAfterTerminalPreviewJob(status, node.CurrentVersionID.Valid)
_, _ = b.queries.UpdateShotStatus(ctx, db.UpdateShotStatusParams{
	ID:          node.ShotID,
	WorkspaceID: event.WorkspaceID,
	Status:      nextShotStatus,
})
if b.agentRuntime != nil {
	agentEvent, err := b.agentRuntime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: event.WorkspaceID,
		EventType:   agentpreview.TerminalEventType(status),
		SourceRole:  "system",
		TargetRole:  "producer",
		Scope:       mustJSON(map[string]any{"shot_id": uuidString(node.ShotID), "node_id": uuidString(node.ID)}),
		Payload: mustJSON(map[string]any{
			"shot_id": uuidString(node.ShotID),
			"node_id": uuidString(node.ID),
			"generation_job_id": uuidString(event.JobID),
			"artifact_version_id": uuidString(node.CurrentVersionID),
			"status": status,
		}),
	})
	if err == nil && b.agentEvents != nil {
		b.agentEvents.BroadcastAgentEvent(event.WorkspaceID, agentEvent)
	}
}
```

Use existing local JSON helper or add a private `mustJSON` in this file.

- [ ] **Step 6: Update Agent node creation broadcaster**

In `apps/server/cmd/server/main.go`, change `agentCanvasNodeBroadcaster` to include `queries *db.Queries` and `storage assetURLSigner`:

```go
type agentCanvasNodeBroadcaster struct {
	hub     *api.CanvasHub
	queries *db.Queries
	storage api.AssetURLSignerForInternalUse // if no exported type exists, create an exported interface in api package
}
```

If exporting the signer type is too invasive, move the broadcaster into `internal/api` as `NewAgentCanvasNodeBroadcaster`. The broadcaster must build the same `canvasNodeResponse` as canvas GET before broadcasting:

```go
b.hub.Broadcast(workspaceID, api.CanvasEvent{Type: "NodeCreated", Payload: map[string]any{"node": response}})
```

- [ ] **Step 7: Add broadcaster tests**

In `production_broadcaster_test.go`, add tests:

- `TestProductionBroadcasterEmitsPreviewAgentEventOnSuccess`
- `TestProductionBroadcasterEmitsPreviewAgentEventOnFailure`
- `TestProductionBroadcasterSkipsNonPreviewNode`

Each fake should assert:

- `UpdateShotStatus` receives `preview_ready` for success with current winner.
- `UpdateShotStatus` receives `failed` for failure.
- `CreateEvent` receives `preview_generation_succeeded` or `preview_generation_failed`.
- `BroadcastAgentEvent` is called once.

- [ ] **Step 8: Run api tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestProductionBroadcaster|TestCanvas' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/server/internal/agent/preview/events.go apps/server/internal/agent/preview/events_test.go apps/server/internal/api/canvas_handler.go apps/server/internal/api/production_broadcaster.go apps/server/internal/api/production_broadcaster_test.go apps/server/cmd/server/main.go
git commit -m "feat: emit preview completion events"
```

---

## Task 6: E2E Smoke Script And Browser Acceptance

**Files:**

- Create: `scripts/smoke-m6-6-preview-closure.sh`
- Optional Modify: `docs/superpowers/specs/2026-06-23-m6-agent-mode-completion-design.md`
  - Add a short note if implementation reveals a clarified behavior.

- [ ] **Step 1: Create smoke script**

Create `scripts/smoke-m6-6-preview-closure.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${CLIPANVIL_PUBLIC_BASE_URL:-http://localhost:${CLIPANVIL_WEB_PORT:-5173}}"
API_URL="${CLIPANVIL_API_BASE_URL:-http://localhost:${CLIPANVIL_SERVER_PORT:-8888}}"

email="m66-preview-$(date +%s)@example.test"
password="Password123!"

register_payload=$(jq -n --arg email "$email" --arg password "$password" '{email:$email,password:$password,name:"M66 Preview"}')
curl -fsS -X POST "$API_URL/api/auth/register" -H 'content-type: application/json' -d "$register_payload" >/tmp/m66-register.json
token=$(jq -r '.token' /tmp/m66-register.json)

workspace_payload='{"name":"m6-6-preview-closure","mode":"agent"}'
curl -fsS -X POST "$API_URL/api/workspaces" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$workspace_payload" >/tmp/m66-workspace.json
workspace_id=$(jq -r '.workspace.id // .id' /tmp/m66-workspace.json)

message_payload='{"text":"创建一个 3 个分镜的 15 秒口播种草短视频 storyboard，然后为所有分镜生成预览图。","client_message_id":"m66-smoke-1"}'
curl -fsS -X POST "$API_URL/api/agent/workspaces/$workspace_id/messages" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$message_payload" >/tmp/m66-message.json

echo "workspace_id=$workspace_id"
echo "agent_url=$BASE_URL/workspaces/$workspace_id/agent"
echo "Wait for Agent tasks to finish, then run DB spot checks from the plan."
```

Make executable:

```bash
chmod +x scripts/smoke-m6-6-preview-closure.sh
```

This script validates API reachability and creates a real Agent workspace/message. Browser and DB checks remain explicit manual/E2E steps because model/provider behavior can vary locally.

- [ ] **Step 2: Run final backend verification**

Run:

```bash
make sqlc-generate
make server-build
make server-test
make server-lint
```

Expected: PASS.

- [ ] **Step 3: Run frontend verification**

Run:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web test:connections
git diff --check
```

Expected: PASS.

- [ ] **Step 4: Start runtime for E2E**

Run:

```bash
./scripts/dev-start.sh
```

Expected:

- Script prints `CLIPANVIL_PUBLIC_BASE_URL`.
- `/api/health` returns ok.

- [ ] **Step 5: Run smoke script**

Use the printed environment if needed:

```bash
CLIPANVIL_SERVER_PORT=<printed-server-port> CLIPANVIL_WEB_PORT=<printed-web-port> ./scripts/smoke-m6-6-preview-closure.sh
```

Expected:

- Script prints `workspace_id=<uuid>`.
- Script prints Agent URL.

- [ ] **Step 6: Browser E2E acceptance**

Open the printed Agent URL in browser automation. Verify:

1. Agent page loads and websocket connection indicator is green.
2. The user message is visible after refresh.
3. Agent creates or updates storyboard through `update_storyboard`.
4. Agent calls `dispatch_craftsman` with `mode=preview_image`.
5. Edge status card updates from running to completed in one card, not duplicate cards.
6. Canvas shows one preview image node per dispatched shot without manual refresh.
7. At least one preview node detail drawer shows:
   - `Shot ID`
   - prompt
   - model provider/model when selected
   - operation `text_to_image`
   - version/job information after generation starts
8. PSS from `get_production_state` includes preview node/job/version and shot status.

- [ ] **Step 7: Database spot checks**

Run with the workspace id from smoke:

```sql
SELECT id, client_key, status, craftsman_thread_id
FROM shot
WHERE workspace_id = '<workspace-id>'
ORDER BY sort_order;

SELECT id, node_type, source, operation_type, status, shot_id, current_version_id, metadata
FROM media_node
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;

SELECT id, target_node_id, operation_type, status, requested_by_type, requested_by_id
FROM generation_job
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;

SELECT event_type, source_role, target_role, scope, payload
FROM agent_event
WHERE workspace_id = '<workspace-id>'
  AND event_type IN ('preview_generation_succeeded', 'preview_generation_failed', 'worker_generation_submitted', 'craftsman_strategy_created')
ORDER BY created_at;

SELECT from_node_id, to_node_id, source, metadata
FROM media_edge
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;
```

Expected:

- Shots move through `preview_running` and terminal preview state.
- Preview nodes are `source='agent'`.
- Generation jobs are `requested_by_type='agent_worker'`.
- Preview terminal events exist after provider completion/failure.
- Input ref dependency edges exist when the test used source material refs.

- [ ] **Step 8: Stop runtime**

Run:

```bash
./scripts/dev-stop.sh
```

Expected: frontend/backend processes for this profile stop.

- [ ] **Step 9: Commit**

```bash
git add scripts/smoke-m6-6-preview-closure.sh docs/superpowers/specs/2026-06-23-m6-agent-mode-completion-design.md
git commit -m "test: add m6 preview closure smoke"
```

---

## Overall Acceptance Criteria

M6.6 Closure is complete only when all of these are true:

- Craftsman scoped context includes target shot, related shot dependencies, source material candidates, and latest job/version facts.
- Worker resolves `input_node_refs` into `GenerationIntent.InputRefs`.
- Worker creates dependency edges from resolved input nodes to preview target nodes.
- `dispatch_craftsman` moves selected shots to `preview_running`.
- Production terminal events move preview shots to `preview_ready` or `failed`.
- Agent preview terminal events are persisted and websocket-broadcast.
- Canvas websocket `NodeCreated`/`NodeUpdated` payloads use UI-ready canvas response node.
- Agent read-only canvas updates without browser refresh.
- PSS shows preview node, job, version, and shot status.
- Backend and frontend verification commands pass.
- Browser E2E has been run and its workspace id is recorded in the final implementation report.

## Strict Verification Command Set

Run before claiming completion:

```bash
make sqlc-generate
make server-build
make server-test
make server-lint
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web test:connections
git diff --check
```

Runtime:

```bash
./scripts/dev-start.sh
CLIPANVIL_SERVER_PORT=<printed-server-port> CLIPANVIL_WEB_PORT=<printed-web-port> ./scripts/smoke-m6-6-preview-closure.sh
```

Browser:

- Use the printed Vite URL.
- Complete the Browser E2E acceptance checklist above.

Database:

- Run the five SQL spot checks above against the E2E workspace.

Stop:

```bash
./scripts/dev-stop.sh
```

## Self-Review

- Spec coverage: covers all M6.6 Closure requirements from `2026-06-23-m6-agent-mode-completion-design.md`: context, input refs, dependency edge sync, shot status, async completion events, websocket payload parity, E2E.
- Placeholder scan: no placeholder tasks; each task names files, tests, commands, and expected results.
- Type consistency: `UpdateShotStatus`, `ListSourceMaterialNodesByWorkspace`, `InputRefResolver`, preview event names, and Worker `GenerationIntent.InputRefs` are consistently named across tasks.
