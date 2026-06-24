# M3 Workspace Mode Entry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Studio / Agent workspace mode selection, route users into the correct workspace experience, and enforce that Agent workspaces cannot be manually edited through ordinary Studio canvas APIs.

**Architecture:** Store `workspace.mode` as the durable routing and permission fact. The backend accepts mode at workspace creation, returns it on workspace APIs, and rejects ordinary canvas mutations for non-Studio workspaces. The frontend uses mode to navigate into `/workspaces/:id/studio` or `/workspaces/:id/agent`; Studio reuses the current canvas page, while Agent starts as a minimal read-only workbench shell.

**Tech Stack:** PostgreSQL + goose migrations, sqlc + pgx, Go/Hertz API, React 19 + React Router 7 + TanStack Query, Vite 8, TypeScript 6, React Flow.

---

## Source Context

Primary milestone:

- `docs/milestones/m3-m6-studio-agent-roadmap.md`

Related design appendices:

- `docs/superpowers/specs/2026-06-18-multiagent-agent-mode-design.md`
- `docs/superpowers/specs/2026-06-18-studio-agent-shared-production-design.md`
- `docs/superpowers/specs/2026-06-18-production-database-technical-design.md`

Current implementation anchors:

- Backend workspace API: `apps/server/internal/api/workspace_handler.go`
- Backend workspace queries: `apps/server/sqlc/queries/workspace.sql`
- Backend schema: `apps/server/migrations/001_init_schema.sql` through `005_add_workspace_sandbox.sql`
- Backend ordinary canvas mutation handlers:
  - `apps/server/internal/api/node_handler.go`
  - `apps/server/internal/api/edge_handler.go`
  - `apps/server/internal/api/group_handler.go`
  - `apps/server/internal/api/canvas_handler.go`
- Frontend workspace list: `apps/web/src/pages/WorkspaceListPage.tsx`
- Frontend create dialog: `apps/web/src/components/CreateWorkspaceDialog.tsx`
- Frontend Studio canvas page: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Frontend API types: `apps/web/src/lib/api.ts`
- Frontend routes: `apps/web/src/App.tsx`
- Styles: `apps/web/src/main.css`

## Scope Decisions

- M3 does not implement real Agent chat, PSS, generation, or Producer tools.
- Agent workbench shell may show placeholders, but it must be a distinct page from Studio.
- Agent mode should still allow uploading raw素材 later; M3 only blocks ordinary manual canvas mutations such as node create/update/delete, edge create/delete, group mutation, node position mutation, and shared camera mutation.
- Internal Agent/Sandbox write paths are not blocked by this milestone. Existing `CreateAgentMediaNode` remains available to internal services.
- Existing workspaces should behave as Studio. The migration should default `workspace.mode` to `studio`.

---

## File Structure

### Backend

- Create: `apps/server/migrations/006_add_workspace_mode.sql`
  - Adds `workspace_mode` enum and `workspace.mode`.
- Modify: `apps/server/sqlc/queries/workspace.sql`
  - Inserts mode on create and returns it through existing `SELECT *`.
- Generated: `apps/server/internal/store/db/*.go`
  - Produced by `make sqlc-generate`.
- Modify: `apps/server/internal/api/workspace_handler.go`
  - Accepts `mode`, validates it, returns it.
- Create: `apps/server/internal/api/workspace_mode_guard.go`
  - Shared helpers for owner lookup and Studio-only mutation checks.
- Modify: `apps/server/internal/api/node_handler.go`
  - Uses Studio-only guard before create/update/delete/batch-position.
- Modify: `apps/server/internal/api/edge_handler.go`
  - Uses Studio-only guard before create/delete.
- Modify: `apps/server/internal/api/group_handler.go`
  - Uses Studio-only guard before create/update/delete/replace nodes.
- Modify: `apps/server/internal/api/canvas_handler.go`
  - Uses Studio-only guard before camera mutation.
- Modify tests:
  - `apps/server/internal/api/workspace_handler_test.go`
  - `apps/server/internal/api/node_handler_test.go`
  - Add focused tests for mode validation and guard behavior.

### Frontend

- Modify: `apps/web/src/lib/api.ts`
  - Adds `WorkspaceMode`, adds `Workspace.mode`, updates `createWorkspace`.
- Modify: `apps/web/src/components/CreateWorkspaceDialog.tsx`
  - Adds mode selection.
- Modify: `apps/web/src/pages/WorkspaceListPage.tsx`
  - Shows mode labels and navigates via mode route helper.
- Create: `apps/web/src/lib/workspaceRoutes.ts`
  - Central helper for mode-specific routes.
- Create: `apps/web/src/pages/WorkspaceModeGatePage.tsx`
  - Loads workspace and redirects `/workspaces/:id` to mode-specific route.
- Create: `apps/web/src/pages/AgentWorkspacePage.tsx`
  - Minimal Agent shell with conversation placeholder and read-only canvas summary.
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
  - Keeps Studio behavior; optionally redirects away if loaded for non-Studio workspace.
- Modify: `apps/web/src/App.tsx`
  - Adds `/workspaces/:id`, `/workspaces/:id/studio`, `/workspaces/:id/agent`.
- Modify: `apps/web/src/main.css`
  - Styles mode picker, mode badges, Agent shell.

---

## Task 1: Add Workspace Mode To Database And sqlc

**Files:**

- Create: `apps/server/migrations/006_add_workspace_mode.sql`
- Modify: `apps/server/sqlc/queries/workspace.sql`
- Generated after implementation: `apps/server/internal/store/db/*.go`

- [ ] **Step 1: Create migration**

Create `apps/server/migrations/006_add_workspace_mode.sql`:

```sql
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
```

- [ ] **Step 2: Update create query**

Change `apps/server/sqlc/queries/workspace.sql`:

```sql
-- name: CreateWorkspace :one
INSERT INTO workspace (name, owner_id, mode)
VALUES ($1, $2, $3)
RETURNING *;
```

Leave `ListWorkspacesByOwner` and `GetWorkspaceByID` as `SELECT *`; sqlc will include `mode` once generated.

- [ ] **Step 3: Generate sqlc**

Run:

```bash
make sqlc-generate
```

Expected:

- `db.Workspace` has a `Mode db.WorkspaceMode` field.
- `db.CreateWorkspaceParams` has a `Mode db.WorkspaceMode` field.
- `db.WorkspaceModeStudio` and `db.WorkspaceModeAgent` are generated constants.

- [ ] **Step 4: Verify generated code compiles**

Run:

```bash
make server-build
```

Expected: fail at this point only if handlers still need to pass the new `Mode` field. Continue to Task 2 before requiring a pass.

---

## Task 2: Add Workspace Mode To Backend API

**Files:**

- Modify: `apps/server/internal/api/workspace_handler.go`
- Modify: `apps/server/internal/api/workspace_handler_test.go`

- [ ] **Step 1: Write mode validation tests**

Add to `apps/server/internal/api/workspace_handler_test.go`:

```go
func TestCreateWorkspaceRequestDefaultsToStudioMode(t *testing.T) {
	req := createWorkspaceRequest{Name: "Demo"}

	mode, ok := req.workspaceMode()
	if !ok {
		t.Fatal("expected blank mode to default")
	}
	if mode != db.WorkspaceModeStudio {
		t.Fatalf("mode = %q, want studio", mode)
	}
}

func TestCreateWorkspaceRequestAcceptsAgentMode(t *testing.T) {
	req := createWorkspaceRequest{Name: "Demo", Mode: "agent"}

	mode, ok := req.workspaceMode()
	if !ok {
		t.Fatal("expected agent mode to be valid")
	}
	if mode != db.WorkspaceModeAgent {
		t.Fatalf("mode = %q, want agent", mode)
	}
}

func TestCreateWorkspaceRequestRejectsUnknownMode(t *testing.T) {
	req := createWorkspaceRequest{Name: "Demo", Mode: "manual"}

	if _, ok := req.workspaceMode(); ok {
		t.Fatal("unknown mode must be invalid")
	}
}
```

Also add the missing import if needed:

```go
import "github.com/sinmaystar/clip-anvil/internal/store/db"
```

- [ ] **Step 2: Run tests and confirm failure**

Run:

```bash
go test ./internal/api -run 'TestCreateWorkspaceRequest'
```

Expected: FAIL because `Mode` and `workspaceMode` do not exist yet.

- [ ] **Step 3: Update request/response structs**

In `apps/server/internal/api/workspace_handler.go`, change:

```go
type createWorkspaceRequest struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}
```

Add mode to response:

```go
type workspaceResponse struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Mode      string          `json:"mode"`
	OwnerID   string          `json:"owner_id"`
	Settings  json.RawMessage `json:"settings,omitempty"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}
```

Add helper:

```go
func (r createWorkspaceRequest) workspaceMode() (db.WorkspaceMode, bool) {
	switch strings.TrimSpace(r.Mode) {
	case "", string(db.WorkspaceModeStudio):
		return db.WorkspaceModeStudio, true
	case string(db.WorkspaceModeAgent):
		return db.WorkspaceModeAgent, true
	default:
		return "", false
	}
}
```

- [ ] **Step 4: Use mode in Create**

In `Create`, after validating name:

```go
mode, ok := req.workspaceMode()
if !ok {
	writeError(c, consts.StatusBadRequest, "invalid workspace mode")
	return
}
```

Pass mode to sqlc:

```go
workspace, err := qtx.CreateWorkspace(ctx, db.CreateWorkspaceParams{
	Name:    strings.TrimSpace(req.Name),
	OwnerID: accountID,
	Mode:    mode,
})
```

- [ ] **Step 5: Return mode**

Update `toWorkspaceResponse`:

```go
func toWorkspaceResponse(workspace db.Workspace) workspaceResponse {
	return workspaceResponse{
		ID:        uuidToString(workspace.ID),
		Name:      workspace.Name,
		Mode:      string(workspace.Mode),
		OwnerID:   uuidToString(workspace.OwnerID),
		Settings:  json.RawMessage(workspace.Settings),
		CreatedAt: workspace.CreatedAt.Time.Format(timeFormatRFC3339),
		UpdatedAt: workspace.UpdatedAt.Time.Format(timeFormatRFC3339),
	}
}
```

- [ ] **Step 6: Verify tests**

Run:

```bash
go test ./internal/api -run 'TestCreateWorkspaceRequest|TestValidWorkspaceName'
```

Expected: PASS.

---

## Task 3: Enforce Studio-Only Ordinary Canvas Mutations

**Files:**

- Create: `apps/server/internal/api/workspace_mode_guard.go`
- Modify: `apps/server/internal/api/node_handler.go`
- Modify: `apps/server/internal/api/edge_handler.go`
- Modify: `apps/server/internal/api/group_handler.go`
- Modify: `apps/server/internal/api/canvas_handler.go`
- Modify: `apps/server/internal/api/node_handler_test.go`

- [ ] **Step 1: Add guard unit tests**

Append to `apps/server/internal/api/node_handler_test.go`:

```go
func TestIsStudioWorkspaceMode(t *testing.T) {
	if !isStudioWorkspaceMode(db.WorkspaceModeStudio) {
		t.Fatal("studio mode should allow ordinary canvas edits")
	}
	if isStudioWorkspaceMode(db.WorkspaceModeAgent) {
		t.Fatal("agent mode should block ordinary canvas edits")
	}
}
```

- [ ] **Step 2: Run test and confirm failure**

Run:

```bash
go test ./internal/api -run TestIsStudioWorkspaceMode
```

Expected: FAIL because `isStudioWorkspaceMode` does not exist.

- [ ] **Step 3: Create shared guard helper**

Create `apps/server/internal/api/workspace_mode_guard.go`:

```go
package api

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func isStudioWorkspaceMode(mode db.WorkspaceMode) bool {
	return mode == db.WorkspaceModeStudio
}

func workspaceForAccount(
	ctx context.Context,
	queries *db.Queries,
	workspaceID pgtype.UUID,
	accountID pgtype.UUID,
	c *app.RequestContext,
) (db.Workspace, bool) {
	workspace, err := queries.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(c, consts.StatusNotFound, "workspace not found")
			return db.Workspace{}, false
		}
		writeError(c, consts.StatusInternalServerError, "failed to load workspace")
		return db.Workspace{}, false
	}
	if workspace.OwnerID != accountID {
		writeError(c, consts.StatusForbidden, "forbidden")
		return db.Workspace{}, false
	}
	return workspace, true
}

func workspaceBelongsToAccount(
	ctx context.Context,
	queries *db.Queries,
	workspaceID pgtype.UUID,
	accountID pgtype.UUID,
	c *app.RequestContext,
) bool {
	_, ok := workspaceForAccount(ctx, queries, workspaceID, accountID, c)
	return ok
}

func requireStudioWorkspace(
	ctx context.Context,
	queries *db.Queries,
	workspaceID pgtype.UUID,
	accountID pgtype.UUID,
	c *app.RequestContext,
) (db.Workspace, bool) {
	workspace, ok := workspaceForAccount(ctx, queries, workspaceID, accountID, c)
	if !ok {
		return db.Workspace{}, false
	}
	if !isStudioWorkspaceMode(workspace.Mode) {
		writeError(c, consts.StatusForbidden, "workspace is read-only in agent mode")
		return db.Workspace{}, false
	}
	return workspace, true
}
```

- [ ] **Step 4: Remove duplicate package-level helper from upload handler**

`apps/server/internal/api/upload_handler.go` currently defines package-level `workspaceBelongsToAccount`. Delete that duplicate function from the bottom of the file and remove any now-unused imports. It will use the shared helper created above.

- [ ] **Step 5: Add guards to NodeHandler**

In `NodeHandler.Create`, replace:

```go
if !h.workspaceBelongsToAccount(ctx, workspaceID, accountID, c) {
	return
}
```

with:

```go
if _, ok := requireStudioWorkspace(ctx, h.queries, workspaceID, accountID, c); !ok {
	return
}
```

In `NodeHandler.Update`, after loading `node`:

```go
if _, ok := requireStudioWorkspace(ctx, h.queries, node.WorkspaceID, accountID, c); !ok {
	return
}
```

In `NodeHandler.Delete`, after loading `node`:

```go
if _, ok := requireStudioWorkspace(ctx, h.queries, node.WorkspaceID, accountID, c); !ok {
	return
}
```

In `BatchUpdatePosition`, when iterating nodes, after `nodeForAccount` returns:

```go
if _, ok := requireStudioWorkspace(ctx, h.queries, node.WorkspaceID, accountID, c); !ok {
	return
}
```

Then delete the `NodeHandler.workspaceBelongsToAccount` method and update `nodeForAccount` to call the shared helper:

```go
if !workspaceBelongsToAccount(ctx, h.queries, node.WorkspaceID, accountID, c) {
	return db.MediaNode{}, false
}
```

- [ ] **Step 6: Add guards to EdgeHandler**

In `EdgeHandler.Delete`, replace the workspace ownership check with:

```go
if _, ok := requireStudioWorkspace(ctx, h.queries, edge.WorkspaceID, accountID, c); !ok {
	return
}
```

In `validateEdgeEndpoints`, after loading workspace and checking owner:

```go
if !isStudioWorkspaceMode(workspace.Mode) {
	return consts.StatusForbidden, "workspace is read-only in agent mode"
}
```

Delete `EdgeHandler.workspaceBelongsToAccount` if it is unused after the change.

- [ ] **Step 7: Add guards to GroupHandler**

In `GroupHandler.Create`, replace workspace ownership check with:

```go
if _, ok := requireStudioWorkspace(ctx, h.queries, workspaceID, accountID, c); !ok {
	return
}
```

In `Update`, `Delete`, and `ReplaceNodes`, after `groupForAccount` returns:

```go
if _, ok := requireStudioWorkspace(ctx, h.queries, group.WorkspaceID, accountID, c); !ok {
	return
}
```

Delete `GroupHandler.workspaceBelongsToAccount` and update `groupForAccount` to call the shared `workspaceBelongsToAccount`.

- [ ] **Step 8: Guard shared camera mutation**

In `CanvasHandler.UpdateCamera`, replace manual workspace lookup/owner check with:

```go
if _, ok := requireStudioWorkspace(ctx, h.queries, workspaceID, accountID, c); !ok {
	return
}
```

Keep `GetCanvas` readable for both Studio and Agent.

- [ ] **Step 9: Verify backend package**

Run:

```bash
go test ./internal/api
```

Expected: PASS.

- [ ] **Step 10: Verify backend build**

Run:

```bash
make server-build
```

Expected: PASS.

---

## Task 4: Add Frontend Mode Types, Create Dialog, And Workspace Cards

**Files:**

- Modify: `apps/web/src/lib/api.ts`
- Modify: `apps/web/src/components/CreateWorkspaceDialog.tsx`
- Modify: `apps/web/src/pages/WorkspaceListPage.tsx`
- Create: `apps/web/src/lib/workspaceRoutes.ts`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Update frontend API types**

In `apps/web/src/lib/api.ts`, add:

```ts
export type WorkspaceMode = "studio" | "agent";
```

Update `Workspace`:

```ts
export interface Workspace {
  id: string;
  name: string;
  mode: WorkspaceMode;
  owner_id: string;
  settings?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}
```

Update `createWorkspace`:

```ts
export function createWorkspace(input: { name: string; mode: WorkspaceMode }) {
  return apiFetch<Workspace>("/workspaces", {
    method: "POST",
    body: JSON.stringify(input),
  });
}
```

- [ ] **Step 2: Add route helper**

Create `apps/web/src/lib/workspaceRoutes.ts`:

```ts
import type { Workspace, WorkspaceMode } from "./api";

export function workspaceRoute(workspace: Pick<Workspace, "id" | "mode">) {
  return workspaceModeRoute(workspace.id, workspace.mode);
}

export function workspaceModeRoute(id: string, mode: WorkspaceMode) {
  return `/workspaces/${id}/${mode}`;
}
```

- [ ] **Step 3: Update create dialog props**

In `apps/web/src/components/CreateWorkspaceDialog.tsx`, import `WorkspaceMode`:

```ts
import type { WorkspaceMode } from "../lib/api";
```

Change props:

```ts
onSubmit: (input: { name: string; mode: WorkspaceMode }) => void;
```

Add state:

```ts
const [mode, setMode] = useState<WorkspaceMode>("studio");
```

Reset mode when opened:

```ts
setMode("studio");
```

Submit both values:

```ts
onSubmit({ name, mode });
```

Add a segmented mode control above the name input:

```tsx
<fieldset className="workspace-mode-picker">
  <legend>项目模式</legend>
  <label className="workspace-mode-option" data-selected={mode === "studio"}>
    <input
      checked={mode === "studio"}
      name="workspace-mode"
      onChange={() => setMode("studio")}
      type="radio"
      value="studio"
    />
    <span>
      <strong>Studio 手动模式</strong>
      <small>专业用户手动搭建节点、连线和运行。</small>
    </span>
  </label>
  <label className="workspace-mode-option" data-selected={mode === "agent"}>
    <input
      checked={mode === "agent"}
      name="workspace-mode"
      onChange={() => setMode("agent")}
      type="radio"
      value="agent"
    />
    <span>
      <strong>Agent 自动模式</strong>
      <small>通过对话驱动 Agent 规划和生产，画布只读。</small>
    </span>
  </label>
</fieldset>
```

- [ ] **Step 4: Update workspace list create mutation**

In `apps/web/src/pages/WorkspaceListPage.tsx`, import:

```ts
import type { WorkspaceMode } from "../lib/api";
import { workspaceRoute } from "../lib/workspaceRoutes";
```

Change success navigation:

```ts
navigate(workspaceRoute(workspace));
```

Change create handler:

```ts
const handleCreate = (input: { name: string; mode: WorkspaceMode }) => {
  setError("");
  createMutation.mutate(input);
};
```

- [ ] **Step 5: Add mode badge to cards**

In the workspace card body, render:

```tsx
<span className="workspace-card-title-row">
  <span className="workspace-card-title">{workspace.name}</span>
  <span className="workspace-mode-badge" data-mode={workspace.mode}>
    {workspace.mode === "agent" ? "Agent" : "Studio"}
  </span>
</span>
```

Change card click:

```tsx
onClick={() => navigate(workspaceRoute(workspace))}
```

- [ ] **Step 6: Add CSS**

In `apps/web/src/main.css`, add styles near workspace/modal styles:

```css
.workspace-mode-picker {
  display: grid;
  gap: 8px;
  border: 0;
  padding: 0;
  margin: 0;
}

.workspace-mode-picker legend {
  margin-bottom: 8px;
  font-size: 12px;
  font-weight: 700;
  color: var(--fg-secondary);
}

.workspace-mode-option {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 10px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--surface-panel);
  cursor: pointer;
}

.workspace-mode-option[data-selected="true"] {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.workspace-mode-option input {
  margin-top: 3px;
}

.workspace-mode-option strong,
.workspace-mode-option small {
  display: block;
}

.workspace-mode-option small {
  margin-top: 3px;
  color: var(--fg-tertiary);
  font-size: 12px;
}

.workspace-card-title-row {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.workspace-mode-badge {
  flex: 0 0 auto;
  border-radius: 999px;
  padding: 3px 7px;
  font-size: 11px;
  font-weight: 700;
  color: var(--fg-secondary);
  background: var(--surface-panel);
  border: 1px solid var(--border-subtle);
}

.workspace-mode-badge[data-mode="agent"] {
  color: var(--accent);
}
```

- [ ] **Step 7: Verify TypeScript catches remaining call sites**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: fail only if old `createWorkspace(name)` call sites remain. Fix those before proceeding.

---

## Task 5: Add Mode-Aware Routes And Agent Workbench Shell

**Files:**

- Modify: `apps/web/src/App.tsx`
- Create: `apps/web/src/pages/WorkspaceModeGatePage.tsx`
- Create: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Create route gate page**

Create `apps/web/src/pages/WorkspaceModeGatePage.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { Navigate, useParams } from "react-router";
import { fetchWorkspace } from "../lib/api";
import { workspaceRoute } from "../lib/workspaceRoutes";

export function WorkspaceModeGatePage() {
  const { id } = useParams();
  const workspaceQuery = useQuery({
    queryKey: ["workspace", id],
    queryFn: () => fetchWorkspace(id ?? ""),
    enabled: Boolean(id),
  });

  if (workspaceQuery.isLoading) {
    return <div className="app-route-loading" role="status" aria-label="正在加载" />;
  }

  if (workspaceQuery.isError || !workspaceQuery.data) {
    return (
      <main className="workspace-route-state">
        <p>项目加载失败</p>
      </main>
    );
  }

  return <Navigate to={workspaceRoute(workspaceQuery.data)} replace />;
}
```

- [ ] **Step 2: Create Agent workspace page**

Create `apps/web/src/pages/AgentWorkspacePage.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { Navigate, useNavigate, useParams } from "react-router";
import { fetchCanvas, fetchWorkspace } from "../lib/api";
import { workspaceModeRoute } from "../lib/workspaceRoutes";

export function AgentWorkspacePage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const workspaceQuery = useQuery({
    queryKey: ["workspace", id],
    queryFn: () => fetchWorkspace(id ?? ""),
    enabled: Boolean(id),
  });
  const canvasQuery = useQuery({
    queryKey: ["workspace", id, "canvas"],
    queryFn: () => fetchCanvas(id ?? ""),
    enabled: Boolean(id),
  });

  if (workspaceQuery.isLoading) {
    return <div className="app-route-loading" role="status" aria-label="正在加载" />;
  }

  if (workspaceQuery.isError || !workspaceQuery.data) {
    return (
      <main className="agent-workspace-shell">
        <p className="agent-empty-text">项目加载失败</p>
      </main>
    );
  }

  if (workspaceQuery.data.mode !== "agent") {
    return (
      <Navigate
        to={workspaceModeRoute(workspaceQuery.data.id, workspaceQuery.data.mode)}
        replace
      />
    );
  }

  const canvas = canvasQuery.data;

  return (
    <main className="agent-workspace-shell">
      <header className="agent-topbar">
        <button
          className="studio-secondary-button"
          onClick={() => navigate("/workspaces")}
          type="button"
        >
          返回
        </button>
        <div>
          <p className="workspace-kicker">Agent Workspace</p>
          <h1>{workspaceQuery.data.name}</h1>
        </div>
      </header>

      <section className="agent-workbench">
        <aside className="agent-chat-panel">
          <div className="agent-panel-header">
            <h2>Producer</h2>
            <span>即将接入</span>
          </div>
          <div className="agent-chat-placeholder">
            <p>Agent 对话将在 M6 接入。当前工作区已进入 Agent 模式，画布由 Agent 工具写入。</p>
          </div>
        </aside>

        <section className="agent-canvas-panel" aria-label="只读画布">
          <div className="agent-panel-header">
            <h2>只读画布</h2>
            <span>{canvas?.nodes.length ?? 0} 个节点</span>
          </div>
          {canvasQuery.isLoading ? (
            <p className="agent-empty-text">正在加载画布</p>
          ) : canvas && canvas.nodes.length > 0 ? (
            <div className="agent-node-list">
              {canvas.nodes.map((node) => (
                <article className="agent-node-card" key={node.id}>
                  <strong>{node.title || "未命名节点"}</strong>
                  <span>{node.node_type}</span>
                </article>
              ))}
            </div>
          ) : (
            <p className="agent-empty-text">Agent 尚未创建画布节点。</p>
          )}
        </section>
      </section>
    </main>
  );
}
```

- [ ] **Step 3: Add routes**

In `apps/web/src/App.tsx`, add lazy imports:

```tsx
const WorkspaceModeGatePage = lazy(() =>
  import("./pages/WorkspaceModeGatePage").then((module) => ({
    default: module.WorkspaceModeGatePage,
  })),
);

const AgentWorkspacePage = lazy(() =>
  import("./pages/AgentWorkspacePage").then((module) => ({
    default: module.AgentWorkspacePage,
  })),
);
```

Update protected routes:

```tsx
{
  path: "/workspaces/:id",
  element: (
    <Suspense fallback={<RouteFallback />}>
      <WorkspaceModeGatePage />
    </Suspense>
  ),
},
{
  path: "/workspaces/:id/studio",
  element: (
    <Suspense fallback={<RouteFallback />}>
      <WorkspaceDetailPage />
    </Suspense>
  ),
},
{
  path: "/workspaces/:id/agent",
  element: (
    <Suspense fallback={<RouteFallback />}>
      <AgentWorkspacePage />
    </Suspense>
  ),
},
```

- [ ] **Step 4: Guard Studio route against Agent workspaces**

In `apps/web/src/pages/WorkspaceDetailPage.tsx`, import:

```ts
import { Navigate } from "react-router";
import { workspaceModeRoute } from "../lib/workspaceRoutes";
```

Because it already imports `useNavigate` and `useParams` from `react-router`, merge the import:

```ts
import { Navigate, useNavigate, useParams } from "react-router";
```

After `workspaceQuery` is defined and before the main return:

```tsx
if (workspaceQuery.data && workspaceQuery.data.mode !== "studio") {
  return (
    <Navigate
      to={workspaceModeRoute(workspaceQuery.data.id, workspaceQuery.data.mode)}
      replace
    />
  );
}
```

- [ ] **Step 5: Add Agent shell CSS**

In `apps/web/src/main.css`, add:

```css
.workspace-route-state,
.agent-workspace-shell {
  min-height: 100vh;
  background: var(--color-canvas);
  color: var(--fg-primary);
}

.workspace-route-state {
  display: grid;
  place-items: center;
  font-size: 13px;
  color: var(--fg-tertiary);
}

.agent-workspace-shell {
  display: grid;
  grid-template-rows: auto 1fr;
}

.agent-topbar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--surface-panel);
}

.agent-topbar h1 {
  margin: 2px 0 0;
  font-size: 20px;
  font-weight: 760;
}

.agent-workbench {
  display: grid;
  grid-template-columns: minmax(320px, 420px) minmax(0, 1fr);
  min-height: 0;
}

.agent-chat-panel,
.agent-canvas-panel {
  min-height: 0;
  padding: 16px;
}

.agent-chat-panel {
  border-right: 1px solid var(--border-subtle);
  background: var(--surface-panel);
}

.agent-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.agent-panel-header h2 {
  margin: 0;
  font-size: 14px;
  font-weight: 760;
}

.agent-panel-header span,
.agent-empty-text,
.agent-chat-placeholder {
  color: var(--fg-tertiary);
  font-size: 13px;
}

.agent-chat-placeholder,
.agent-node-card {
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--surface-raised);
  padding: 12px;
}

.agent-node-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 10px;
}

.agent-node-card {
  display: grid;
  gap: 6px;
}

.agent-node-card span {
  color: var(--fg-tertiary);
  font-size: 12px;
}

@media (max-width: 860px) {
  .agent-workbench {
    grid-template-columns: 1fr;
  }

  .agent-chat-panel {
    border-right: 0;
    border-bottom: 1px solid var(--border-subtle);
  }
}
```

- [ ] **Step 6: Verify frontend build**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

---

## Task 6: Add API Smoke Coverage For Mode Behavior

**Files:**

- Modify or create backend tests under `apps/server/internal/api/`
- Optional create: `apps/server/internal/api/workspace_mode_guard_test.go`

- [ ] **Step 1: Add pure helper tests**

If not already covered in Task 3, add `apps/server/internal/api/workspace_mode_guard_test.go`:

```go
package api

import (
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestIsStudioWorkspaceModeOnlyAllowsStudio(t *testing.T) {
	if !isStudioWorkspaceMode(db.WorkspaceModeStudio) {
		t.Fatal("studio mode should be editable")
	}
	if isStudioWorkspaceMode(db.WorkspaceModeAgent) {
		t.Fatal("agent mode should be read-only for ordinary canvas APIs")
	}
}
```

- [ ] **Step 2: Add integration-style manual API smoke note**

M3 does not currently have a full HTTP integration harness for workspace mode. Add a short manual smoke section to the final implementation report rather than introducing a large new test harness in this milestone.

Use this smoke script after `./scripts/dev-start.sh` and `make migrate-up`:

```bash
node -e '
const base = "http://localhost:8888/api";
async function req(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  if (!res.ok) throw new Error(`${init.method || "GET"} ${path} -> ${res.status}: ${text}`);
  return text ? JSON.parse(text) : null;
}
(async () => {
  const email = `m3-${Date.now()}@clip.test`;
  const auth = await req("/auth/register", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({email, password: "password123", name: "M3 Smoke"})
  });
  const headers = {Authorization: `Bearer ${auth.token}`, "Content-Type": "application/json"};
  const studio = await req("/workspaces", {
    method: "POST",
    headers,
    body: JSON.stringify({name: "Studio M3", mode: "studio"})
  });
  const agent = await req("/workspaces", {
    method: "POST",
    headers,
    body: JSON.stringify({name: "Agent M3", mode: "agent"})
  });
  await req("/nodes", {
    method: "POST",
    headers,
    body: JSON.stringify({workspace_id: studio.id, node_type: "text", title: "ok", canvas_x: 0, canvas_y: 0})
  });
  const blocked = await fetch(`${base}/nodes`, {
    method: "POST",
    headers,
    body: JSON.stringify({workspace_id: agent.id, node_type: "text", title: "blocked", canvas_x: 0, canvas_y: 0})
  });
  if (blocked.status !== 403) throw new Error(`agent node create status ${blocked.status}, want 403`);
  console.log(JSON.stringify({studioMode: studio.mode, agentMode: agent.mode, blockedStatus: blocked.status}, null, 2));
})().catch((err) => {
  console.error(err);
  process.exit(1);
});
'
```

If the active worktree uses a non-8888 backend port, get it with:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

Then replace `base` in the smoke script with the printed backend port.

- [ ] **Step 3: Run backend tests**

Run:

```bash
make server-test
```

Expected: PASS.

---

## Task 7: Full M3 Verification

**Files:**

- No new files unless fixing issues found by verification.

- [ ] **Step 1: Run database/codegen verification**

Run:

```bash
make migrate-up
make sqlc-generate
```

Expected: migrations apply cleanly and generated code is up to date.

- [ ] **Step 2: Run backend verification**

Run:

```bash
make server-build
make server-test
```

Expected: PASS.

- [ ] **Step 3: Run frontend verification**

Run:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Expected: PASS.

- [ ] **Step 4: Run docs/whitespace verification**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 5: Manual browser smoke**

Start the app using repo tooling:

```bash
./scripts/dev-start.sh
```

Use the Vite URL printed by the script, not a guessed localhost port.

Smoke steps:

1. Register or log in.
2. Create a Studio workspace.
3. Confirm the app navigates to `/workspaces/:id/studio`.
4. Create a text node and move it.
5. Refresh and confirm the node remains.
6. Go back to workspace list.
7. Create an Agent workspace.
8. Confirm the app navigates to `/workspaces/:id/agent`.
9. Confirm the Agent workbench shell renders.
10. Confirm there is no Studio toolbar/context menu/manual node creation affordance.
11. Directly visit `/workspaces/:agentId/studio`; confirm redirect to `/workspaces/:agentId/agent`.
12. Directly visit `/workspaces/:studioId/agent`; confirm redirect to `/workspaces/:studioId/studio`.

- [ ] **Step 6: Manual API smoke**

Run the Node smoke script from Task 6 using the active backend port.

Expected output:

```json
{
  "studioMode": "studio",
  "agentMode": "agent",
  "blockedStatus": 403
}
```

---

## Completion Criteria

M3 is complete when:

- `workspace.mode` is stored in Postgres and returned by API.
- Workspace creation supports `studio` and `agent`.
- Existing/blank mode defaults to `studio`.
- Workspace list shows mode labels.
- `/workspaces/:id` routes to the correct mode-specific page.
- Studio workspaces keep existing manual canvas behavior.
- Agent workspaces render a distinct Agent shell.
- Ordinary user canvas mutation APIs reject Agent workspaces.
- All verification commands in Task 7 pass, or any skipped command is explicitly documented with the reason.

## Plan Self-Review

- M3 milestone coverage: covered mode field, creation entry, route split, Agent shell, Studio edit preservation, backend ordinary edit blocking, E2E smoke.
- Out of scope: real Agent chat, Agent tools, generation jobs, Provider Bridge, Reference Pack, PSS.
- Risk: `WorkspaceDetailPage.tsx` is large. Keep M3 edits minimal: add only mode redirect and leave Studio internals intact.
- Risk: duplicate ownership helpers exist in several handlers. Centralize only the helper needed for M3; avoid broad handler refactors.
- Risk: upload semantics for Agent workspaces are nuanced. M3 leaves raw upload allowed and blocks only ordinary canvas mutations.
