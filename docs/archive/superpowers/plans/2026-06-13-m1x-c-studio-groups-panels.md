# M1.x-C Studio Groups and Panels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Studio organization and editing surfaces: persistent groups, a full resource tree, and a right-side property panel for nodes, edges, and groups.

**Architecture:** `media_group` and `media_node.group_id` become the source of truth for grouping. The frontend projects groups as custom `group-container` shapes and builds the resource tree and property panel from the canvas payload plus focused REST endpoints.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, goose, React 19, TypeScript 6, tldraw 5, TanStack Query.

---

## File Structure

- Create `apps/server/migrations/003_add_groups.sql`: group table and `media_node.group_id`.
- Create `apps/server/sqlc/queries/group.sql`: group CRUD queries.
- Modify `apps/server/sqlc/queries/node.sql`: group update queries and upstream input query.
- Generate `apps/server/internal/store/db/*`: sqlc output.
- Create `apps/server/internal/api/group_handler.go`: group APIs.
- Modify `apps/server/internal/api/node_handler.go`: `group_id` updates and `/inputs`.
- Modify `apps/server/internal/api/canvas_handler.go`: include `groups`.
- Modify `apps/server/cmd/server/main.go`: route registration.
- Create `apps/web/src/shapes/GroupContainerShapeUtil.tsx`: group container rendering.
- Create `apps/web/src/components/ResourceTree.tsx`: left tree.
- Create `apps/web/src/components/PropertyPanel.tsx`: right panel shell.
- Create `apps/web/src/components/NodePropertyPanel.tsx`: node details.
- Create `apps/web/src/components/EdgePropertyPanel.tsx`: edge details.
- Create `apps/web/src/components/GroupPropertyPanel.tsx`: group details.
- Modify `apps/web/src/lib/api.ts`: group DTOs and API calls.
- Modify `apps/web/src/lib/canvas.ts`: group shape mapping.
- Modify `apps/web/src/pages/WorkspaceDetailPage.tsx`: three-pane layout and selection wiring.
- Modify `apps/web/src/main.css`: resource tree, property panel, group container styles.

### Task 1: Group Schema and Queries

**Files:**
- Create: `apps/server/migrations/003_add_groups.sql`
- Create: `apps/server/sqlc/queries/group.sql`
- Modify: `apps/server/sqlc/queries/node.sql`
- Generate: `apps/server/internal/store/db/*`

- [ ] **Step 1: Create the group migration**

Create `apps/server/migrations/003_add_groups.sql`:

```sql
-- +goose Up
CREATE TABLE media_group (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_media_group_workspace ON media_group(workspace_id);

ALTER TABLE media_node
    ADD COLUMN group_id UUID REFERENCES media_group(id) ON DELETE SET NULL;

CREATE INDEX idx_media_node_group ON media_node(group_id);

-- +goose Down
DROP INDEX IF EXISTS idx_media_node_group;
ALTER TABLE media_node DROP COLUMN IF EXISTS group_id;
DROP INDEX IF EXISTS idx_media_group_workspace;
DROP TABLE IF EXISTS media_group;
```

- [ ] **Step 2: Add group queries**

Create `apps/server/sqlc/queries/group.sql`:

```sql
-- name: CreateMediaGroup :one
INSERT INTO media_group (workspace_id, name, sort_order)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListMediaGroupsByWorkspace :many
SELECT * FROM media_group
WHERE workspace_id = $1
ORDER BY sort_order, created_at;

-- name: GetMediaGroupByID :one
SELECT * FROM media_group
WHERE id = $1;

-- name: UpdateMediaGroupName :one
UPDATE media_group
SET name = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateMediaGroupSortOrder :one
UPDATE media_group
SET sort_order = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMediaGroup :exec
DELETE FROM media_group
WHERE id = $1;
```

- [ ] **Step 3: Extend node queries**

Append to `apps/server/sqlc/queries/node.sql`:

```sql
-- name: UpdateMediaNodeGroup :one
UPDATE media_node
SET group_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ClearMediaNodeGroup :one
UPDATE media_node
SET group_id = NULL,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListMediaNodesByGroup :many
SELECT * FROM media_node
WHERE group_id = $1
ORDER BY created_at;

-- name: ListUpstreamDependencyNodes :many
SELECT media_node.*
FROM media_node
JOIN media_edge ON media_edge.from_node_id = media_node.id
WHERE media_edge.to_node_id = $1
  AND media_edge.edge_type = 'dependency'
ORDER BY media_edge.created_at;
```

- [ ] **Step 4: Generate sqlc code**

Run:

```bash
make sqlc-generate
```

Expected: generated group and new node query methods exist.

### Task 2: Backend Group and Inputs API

**Files:**
- Create: `apps/server/internal/api/group_handler.go`
- Modify: `apps/server/internal/api/node_handler.go`
- Modify: `apps/server/internal/api/canvas_handler.go`
- Modify: `apps/server/cmd/server/main.go`
- Create: `apps/server/internal/api/group_handler_test.go`
- Modify: `apps/server/internal/api/node_handler_test.go`
- Modify: `apps/server/internal/api/canvas_handler_test.go`

- [ ] **Step 1: Add failing group API tests**

Create tests in `group_handler_test.go` using this concrete pattern:

```go
func TestGroupHandlerCreateAssignsNodes(t *testing.T) {
	server, token, workspaceID := newGroupHandlerTestServer(t)
	nodeA := createTestNode(t, workspaceID, "text", 0, 0)
	nodeB := createTestNode(t, workspaceID, "image", 240, 0)
	body := strings.NewReader(fmt.Sprintf(
		`{"workspace_id":%q,"name":"分镜组","node_ids":[%q,%q]}`,
		workspaceID,
		nodeA.ID.String(),
		nodeB.ID.String(),
	))
	req := httptest.NewRequest(http.MethodPost, "/api/groups", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	canvas := getTestCanvas(t, server, token, workspaceID)
	require.Len(t, canvas.Groups, 1)
	require.ElementsMatch(t, []pgtype.UUID{nodeA.ID, nodeB.ID}, canvas.Groups[0].NodeIDs)
}

func TestGroupHandlerDeleteKeepsNodes(t *testing.T) {
	server, token, workspaceID := newGroupHandlerTestServer(t)
	nodeA := createTestNode(t, workspaceID, "text", 0, 0)
	groupID := createTestGroup(t, server, token, workspaceID, []string{nodeA.ID.String()})
	req := httptest.NewRequest(http.MethodDelete, "/api/groups/"+groupID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	require.Equal(t, http.StatusNoContent, resp.Code)
	canvas := getTestCanvas(t, server, token, workspaceID)
	require.Empty(t, canvas.Groups)
	require.Len(t, canvas.Nodes, 1)
	require.Equal(t, nodeA.ID, canvas.Nodes[0].ID)
}
```

Add `TestGroupHandlerReplaceNodes` with `PUT /api/groups/:id/nodes` and an assertion that only the replacement node remains in `groups[0].node_ids`. Add `TestGroupHandlerRejectsCrossWorkspaceNode` by creating a second workspace and asserting the create request returns a non-2xx status.

- [ ] **Step 2: Run group tests and verify failure**

Run:

```bash
cd apps/server && go test ./internal/api -run GroupHandler -count=1
```

Expected: FAIL because the group handler and routes do not exist.

- [ ] **Step 3: Implement group handler types**

Create `apps/server/internal/api/group_handler.go`:

```go
package api

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type GroupHandler struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewGroupHandler(pool *pgxpool.Pool, queries *db.Queries) *GroupHandler {
	return &GroupHandler{pool: pool, queries: queries}
}

type createGroupRequest struct {
	WorkspaceID string   `json:"workspace_id"`
	Name        string   `json:"name"`
	NodeIDs     []string `json:"node_ids"`
}

type updateGroupRequest struct {
	Name      *string `json:"name"`
	SortOrder *int32 `json:"sort_order"`
}

type replaceGroupNodesRequest struct {
	NodeIDs []string `json:"node_ids"`
}
```

- [ ] **Step 4: Implement create, update, delete, replace members**

Implement these methods:

```go
func (h *GroupHandler) Create(ctx context.Context, c *app.RequestContext) {}
func (h *GroupHandler) Update(ctx context.Context, c *app.RequestContext) {}
func (h *GroupHandler) Delete(ctx context.Context, c *app.RequestContext) {}
func (h *GroupHandler) ReplaceNodes(ctx context.Context, c *app.RequestContext) {}
```

Use the same ownership style as `NodeHandler`: parse UUIDs, load workspace, compare `OwnerID`, return `403` for wrong owner and `404` for missing group. In `Create` and `ReplaceNodes`, open a transaction and validate every node belongs to the same workspace before setting `group_id`.

- [ ] **Step 5: Extend node PATCH with group_id**

In `node_handler.go`, update `updateNodeRequest`:

```go
type updateNodeRequest struct {
	Title   *string `json:"title"`
	Prompt  *string `json:"prompt"`
	Status  *string `json:"status"`
	GroupID *string `json:"group_id"`
}
```

Update `hasChanges`:

```go
return r.Title != nil || r.Prompt != nil || r.Status != nil || r.GroupID != nil
```

When `GroupID != nil`:

- If empty string, call `ClearMediaNodeGroup`.
- If UUID, load group and verify same workspace, then call `UpdateMediaNodeGroup`.

- [ ] **Step 6: Add node inputs route**

Add method:

```go
func (h *NodeHandler) Inputs(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	node, ok := h.nodeForAccount(ctx, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	inputs, err := h.queries.ListUpstreamDependencyNodes(ctx, node.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load inputs")
		return
	}
	c.JSON(consts.StatusOK, inputs)
}
```

- [ ] **Step 7: Extend canvas response with groups**

In `canvas_handler.go`, add:

```go
type canvasGroupResponse struct {
	ID          pgtype.UUID   `json:"id"`
	WorkspaceID pgtype.UUID  `json:"workspace_id"`
	Name        string       `json:"name"`
	SortOrder   int32        `json:"sort_order"`
	NodeIDs     []pgtype.UUID `json:"node_ids"`
}
```

Load groups and build `NodeIDs` by iterating nodes with valid `GroupID`.

- [ ] **Step 8: Register routes**

In `cmd/server/main.go`:

```go
groupHandler := api.NewGroupHandler(pgPool, queries)
h.POST("/api/groups", authMiddleware, groupHandler.Create)
h.PATCH("/api/groups/:id", authMiddleware, groupHandler.Update)
h.DELETE("/api/groups/:id", authMiddleware, groupHandler.Delete)
h.PUT("/api/groups/:id/nodes", authMiddleware, groupHandler.ReplaceNodes)
h.GET("/api/nodes/:id/inputs", authMiddleware, nodeHandler.Inputs)
```

Register `/api/nodes/:id/inputs` before generic `PATCH` and `DELETE` are unaffected because methods differ; keep it near node routes for readability.

- [ ] **Step 9: Run backend tests**

Run:

```bash
make server-test
```

Expected: PASS.

### Task 3: Frontend DTOs and Group Mapping

**Files:**
- Modify: `apps/web/src/lib/api.ts`
- Modify: `apps/web/src/lib/canvas.ts`
- Create: `apps/web/src/shapes/GroupContainerShapeUtil.tsx`

- [ ] **Step 1: Add DTOs and API functions**

In `api.ts`, add:

```ts
export interface MediaGroup {
  id: string;
  workspace_id: string;
  name: string;
  sort_order: number;
  node_ids: string[];
}

export interface CanvasPayload {
  camera: CanvasCamera;
  nodes: MediaNode[];
  edges: MediaEdge[];
  groups: MediaGroup[];
}

export function createMediaGroup(input: {
  workspace_id: string;
  name: string;
  node_ids?: string[];
}) {
  return apiFetch<{ group: MediaGroup; node_ids: string[] }>("/groups", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateMediaGroup(id: string, input: Partial<Pick<MediaGroup, "name" | "sort_order">>) {
  return apiFetch<MediaGroup>(`/groups/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function deleteMediaGroup(id: string) {
  return apiFetch<void>(`/groups/${id}`, { method: "DELETE" });
}

export function replaceMediaGroupNodes(id: string, node_ids: string[]) {
  return apiFetch<void>(`/groups/${id}/nodes`, {
    method: "PUT",
    body: JSON.stringify({ node_ids }),
  });
}

export function fetchNodeInputs(id: string) {
  return apiFetch<MediaNode[]>(`/nodes/${id}/inputs`);
}
```

- [ ] **Step 2: Add group shape schema**

Create `GroupContainerShapeUtil.tsx` with:

```tsx
import { HTMLContainer, Rectangle2d, ShapeUtil, T, type Geometry2d, type RecordProps } from "tldraw";
import type { TLBaseShape } from "@tldraw/tlschema";

export const GROUP_CONTAINER_SHAPE_TYPE = "group-container" as const;

export interface GroupContainerShapeProps {
  groupId: string;
  name: string;
  nodeCount: number;
  collapsed: boolean;
  w: number;
  h: number;
}

export type GroupContainerShape = TLBaseShape<typeof GROUP_CONTAINER_SHAPE_TYPE, GroupContainerShapeProps>;

export class GroupContainerShapeUtil extends ShapeUtil<GroupContainerShape> {
  static override type = GROUP_CONTAINER_SHAPE_TYPE;
  static override props: RecordProps<GroupContainerShape> = {
    groupId: T.string,
    name: T.string,
    nodeCount: T.number,
    collapsed: T.boolean,
    w: T.number,
    h: T.number,
  };

  override getDefaultProps(): GroupContainerShapeProps {
    return { groupId: "", name: "未命名分组", nodeCount: 0, collapsed: false, w: 320, h: 220 };
  }

  override getGeometry(shape: GroupContainerShape): Geometry2d {
    return new Rectangle2d({ width: shape.props.w, height: shape.props.h, isFilled: false });
  }

  override component(shape: GroupContainerShape) {
    return (
      <HTMLContainer>
        <div className="group-container-shape" data-collapsed={shape.props.collapsed} style={{ width: shape.props.w, height: shape.props.h }}>
          <div className="group-container-title">
            <span>{shape.props.name}</span>
            <span>{shape.props.nodeCount}</span>
          </div>
        </div>
      </HTMLContainer>
    );
  }

  override getIndicatorPath(shape: GroupContainerShape) {
    const path = new Path2D();
    path.rect(0, 0, shape.props.w, shape.props.h);
    return path;
  }
}
```

- [ ] **Step 3: Add group mapping helpers**

In `canvas.ts`, add `shapeIdForGroup` and `groupToShape`:

```ts
export function shapeIdForGroup(groupId: string) {
  return createShapeId(`group-${groupId}`);
}

export function groupToShape(group: MediaGroup, nodes: MediaNode[]): TLShapePartial {
  const groupNodes = nodes.filter((node) => group.node_ids.includes(node.id));
  const bounds = boundsForNodes(groupNodes);
  return {
    id: shapeIdForGroup(group.id),
    type: "group-container",
    x: bounds.x - 20,
    y: bounds.y - 44,
    props: {
      groupId: group.id,
      name: group.name,
      nodeCount: group.node_ids.length,
      collapsed: false,
      w: Math.max(240, bounds.w + 40),
      h: Math.max(120, bounds.h + 64),
    },
  };
}
```

Add a local helper that returns a stable empty bounds value when a group has no nodes:

```ts
function boundsForNodes(nodes: MediaNode[]) {
  if (nodes.length === 0) {
    return { x: 0, y: 0, w: 240, h: 120 };
  }
  const minX = Math.min(...nodes.map((node) => node.canvas_x));
  const minY = Math.min(...nodes.map((node) => node.canvas_y));
  const maxX = Math.max(...nodes.map((node) => node.canvas_x + node.canvas_w));
  const maxY = Math.max(...nodes.map((node) => node.canvas_y + node.canvas_h));
  return { x: minX, y: minY, w: maxX - minX, h: maxY - minY };
}
```

- [ ] **Step 4: Build**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS after imports are adjusted.

### Task 4: Resource Tree

**Files:**
- Create: `apps/web/src/components/ResourceTree.tsx`
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Create ResourceTree component**

Create `ResourceTree.tsx`:

```tsx
import { useMemo, useState } from "react";
import type { MediaGroup, MediaNode, MediaType } from "../lib/api";

interface ResourceTreeProps {
  nodes: MediaNode[];
  groups: MediaGroup[];
  selectedNodeId: string | null;
  onSelectNode: (nodeId: string) => void;
  onSelectGroup: (groupId: string) => void;
}

const filters: Array<{ value: "all" | MediaType; label: string }> = [
  { value: "all", label: "全部" },
  { value: "text", label: "文本" },
  { value: "image", label: "图片" },
  { value: "video", label: "视频" },
  { value: "audio", label: "音频" },
];

export function ResourceTree({ nodes, groups, selectedNodeId, onSelectNode, onSelectGroup }: ResourceTreeProps) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<"all" | MediaType>("all");
  const [collapsedGroups, setCollapsedGroups] = useState(new Set<string>());

  const visibleNodes = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return nodes.filter((node) => {
      const matchesType = filter === "all" || node.node_type === filter;
      const matchesQuery = normalized === "" || node.title.toLowerCase().includes(normalized);
      return matchesType && matchesQuery;
    });
  }, [filter, nodes, query]);

  const ungroupedNodes = visibleNodes.filter((node) => !node.group_id);

  return (
    <div className="resource-tree">
      <input aria-label="搜索资源" onChange={(event) => setQuery(event.target.value)} placeholder="搜索" value={query} />
      <div className="resource-tree-filters">
        {filters.map((item) => (
          <button data-active={filter === item.value} key={item.value} onClick={() => setFilter(item.value)} type="button">
            {item.label}
          </button>
        ))}
      </div>
      <div className="resource-tree-list">
        {groups.map((group) => {
          const groupNodes = visibleNodes.filter((node) => group.node_ids.includes(node.id));
          const collapsed = collapsedGroups.has(group.id);
          return (
            <section className="resource-tree-group" key={group.id}>
              <button onClick={() => onSelectGroup(group.id)} type="button">{group.name} ({groupNodes.length})</button>
              <button onClick={() => setCollapsedGroups(toggleSet(collapsedGroups, group.id))} type="button">{collapsed ? "展开" : "收起"}</button>
              {!collapsed && groupNodes.map((node) => (
                <ResourceNodeRow key={node.id} node={node} selected={node.id === selectedNodeId} onSelectNode={onSelectNode} />
              ))}
            </section>
          );
        })}
        <section className="resource-tree-group">
          <p>未分组</p>
          {ungroupedNodes.map((node) => (
            <ResourceNodeRow key={node.id} node={node} selected={node.id === selectedNodeId} onSelectNode={onSelectNode} />
          ))}
        </section>
      </div>
    </div>
  );
}

function ResourceNodeRow({ node, selected, onSelectNode }: { node: MediaNode; selected: boolean; onSelectNode: (nodeId: string) => void }) {
  return (
    <button className="resource-tree-node" data-selected={selected} onClick={() => onSelectNode(node.id)} type="button">
      <span>{node.node_type}</span>
      <span>{node.title || "未命名资源"}</span>
      <span>{node.status}</span>
    </button>
  );
}

function toggleSet(source: Set<string>, value: string) {
  const next = new Set(source);
  if (next.has(value)) {
    next.delete(value);
  } else {
    next.add(value);
  }
  return next;
}
```

- [ ] **Step 2: Add `group_id` to frontend MediaNode**

In `api.ts`:

```ts
group_id?: string | null;
```

- [ ] **Step 3: Replace sidebar node list with ResourceTree**

In `WorkspaceDetailPage.tsx`, import `ResourceTree` and render:

```tsx
<ResourceTree
  groups={canvasQuery.data?.groups ?? []}
  nodes={nodes}
  onSelectGroup={selectGroup}
  onSelectNode={selectNode}
  selectedNodeId={selectedNodeId}
/>
```

Add `selectGroup`:

```ts
const selectGroup = useCallback((groupId: string) => {
  const editor = editorRef.current;
  if (!editor) {
    return;
  }
  const shapeId = shapeIdForGroup(groupId);
  if (editor.getShape(shapeId)) {
    editor.setSelectedShapes([shapeId]);
    editor.zoomToSelection();
  }
}, []);
```

- [ ] **Step 4: Build**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

### Task 5: Property Panel

**Files:**
- Create: `apps/web/src/components/PropertyPanel.tsx`
- Create: `apps/web/src/components/NodePropertyPanel.tsx`
- Create: `apps/web/src/components/EdgePropertyPanel.tsx`
- Create: `apps/web/src/components/GroupPropertyPanel.tsx`
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Create panel shell**

Create `PropertyPanel.tsx`:

```tsx
import type { MediaEdge, MediaGroup, MediaNode } from "../lib/api";
import { EdgePropertyPanel } from "./EdgePropertyPanel";
import { GroupPropertyPanel } from "./GroupPropertyPanel";
import { NodePropertyPanel } from "./NodePropertyPanel";

interface PropertyPanelProps {
  selectedNode?: MediaNode;
  selectedEdge?: MediaEdge;
  selectedGroup?: MediaGroup;
  nodes: MediaNode[];
  onSelectNode: (nodeId: string) => void;
}

export function PropertyPanel(props: PropertyPanelProps) {
  if (props.selectedNode) {
    return <NodePropertyPanel node={props.selectedNode} onSelectNode={props.onSelectNode} />;
  }
  if (props.selectedEdge) {
    return <EdgePropertyPanel edge={props.selectedEdge} nodes={props.nodes} onSelectNode={props.onSelectNode} />;
  }
  if (props.selectedGroup) {
    return <GroupPropertyPanel group={props.selectedGroup} />;
  }
  return null;
}
```

- [ ] **Step 2: Create node panel**

Create `NodePropertyPanel.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { fetchNodeInputs, updateMediaNode, type MediaNode } from "../lib/api";

export function NodePropertyPanel({ node, onSelectNode }: { node: MediaNode; onSelectNode: (nodeId: string) => void }) {
  const inputsQuery = useQuery({
    queryKey: ["node", node.id, "inputs"],
    queryFn: () => fetchNodeInputs(node.id),
  });

  return (
    <aside className="property-panel">
      <h2>节点属性</h2>
      <label>
        <span>标题</span>
        <input defaultValue={node.title} onBlur={(event) => void updateMediaNode(node.id, { title: event.target.value })} />
      </label>
      <p>类型：{node.node_type}</p>
      <p>状态：{node.status}</p>
      <section>
        <h3>输入引用</h3>
        {(inputsQuery.data ?? []).map((input) => (
          <button key={input.id} onClick={() => onSelectNode(input.id)} type="button">{input.title || input.node_type}</button>
        ))}
      </section>
      <label>
        <span>Prompt</span>
        <textarea defaultValue={node.prompt} onBlur={(event) => void updateMediaNode(node.id, { prompt: event.target.value })} rows={8} />
      </label>
    </aside>
  );
}
```

- [ ] **Step 3: Create edge panel**

Create `EdgePropertyPanel.tsx`:

```tsx
import { deleteMediaEdge, type MediaEdge, type MediaNode } from "../lib/api";

export function EdgePropertyPanel({ edge, nodes, onSelectNode }: { edge: MediaEdge; nodes: MediaNode[]; onSelectNode: (nodeId: string) => void }) {
  const fromNode = nodes.find((node) => node.id === edge.from_node_id);
  const toNode = nodes.find((node) => node.id === edge.to_node_id);
  return (
    <aside className="property-panel">
      <h2>连线属性</h2>
      <button onClick={() => fromNode && onSelectNode(fromNode.id)} type="button">起点：{fromNode?.title ?? "未知节点"}</button>
      <button onClick={() => toNode && onSelectNode(toNode.id)} type="button">终点：{toNode?.title ?? "未知节点"}</button>
      <p>类型：{edge.edge_type}</p>
      <button onClick={() => void deleteMediaEdge(edge.id)} type="button">删除连线</button>
    </aside>
  );
}
```

- [ ] **Step 4: Create group panel**

Create `GroupPropertyPanel.tsx`:

```tsx
import { deleteMediaGroup, updateMediaGroup, type MediaGroup } from "../lib/api";

export function GroupPropertyPanel({ group }: { group: MediaGroup }) {
  return (
    <aside className="property-panel">
      <h2>分组属性</h2>
      <label>
        <span>名称</span>
        <input defaultValue={group.name} onBlur={(event) => void updateMediaGroup(group.id, { name: event.target.value })} />
      </label>
      <p>成员：{group.node_ids.length}</p>
      <button onClick={() => void deleteMediaGroup(group.id)} type="button">删除分组</button>
    </aside>
  );
}
```

- [ ] **Step 5: Wire panel selection in page**

Track selected shape IDs and derive selected node/edge/group from `canvasQuery.data`. Render:

```tsx
<PropertyPanel
  nodes={nodes}
  onSelectNode={selectNode}
  selectedEdge={selectedEdge}
  selectedGroup={selectedGroup}
  selectedNode={selectedNode}
/>
```

- [ ] **Step 6: Build**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

### Task 6: Group Creation and Canvas Projection

**Files:**
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Register group shape util**

Change shape utils:

```ts
const shapeUtils = useMemo(() => [MediaShapeUtil, GroupContainerShapeUtil], []);
```

- [ ] **Step 2: Create group shapes on initial load**

After node and edge shape creation:

```ts
const groupShapes = canvasQuery.data.groups.map((group) => groupToShape(group, canvasQuery.data.nodes));
if (groupShapes.length > 0) {
  editor.store.mergeRemoteChanges(() => {
    editor.createShapes(groupShapes);
  });
}
```

- [ ] **Step 3: Add create group command**

Add a context menu action when two or more media nodes are selected:

```ts
const createGroupFromSelection = () => {
  const editor = editorRef.current;
  if (!id || !editor) {
    return;
  }
  const nodeIds = editor
    .getSelectedShapes()
    .filter(isMediaShape)
    .map((shape) => shape.props.nodeId);
  if (nodeIds.length < 2) {
    return;
  }
  void createMediaGroup({ workspace_id: id, name: "新分组", node_ids: nodeIds }).then(() => {
    void queryClient.invalidateQueries({ queryKey: ["workspace", id, "canvas"] });
  });
};
```

- [ ] **Step 4: Add group CSS**

```css
.group-container-shape {
  border: 1px dashed color-mix(in_srgb, var(--fg-tertiary) 70%, transparent);
  border-radius: 8px;
  background: color-mix(in_srgb, var(--color-panel) 35%, transparent);
}

.group-container-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  height: 32px;
  padding: 0 10px;
  font-size: 12px;
  color: var(--fg-secondary);
}
```

- [ ] **Step 5: Build and manually verify**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

Manual check: create two nodes, select both, create group, refresh, group container and resource tree membership remain.

### Task 7: Final M1.x-C Verification and Commit

**Files:**
- All files changed in Tasks 1-6.

- [ ] **Step 1: Run migrations and generation**

```bash
make migrate-up
make sqlc-generate
```

Expected: migration and generation succeed.

- [ ] **Step 2: Run backend tests**

```bash
make server-test
```

Expected: PASS.

- [ ] **Step 3: Run frontend build**

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/server/migrations/003_add_groups.sql apps/server/sqlc/queries/group.sql apps/server/sqlc/queries/node.sql apps/server/internal/store/db apps/server/internal/api apps/server/cmd/server/main.go apps/web/src
git commit -m "feat: add studio groups and panels"
```
