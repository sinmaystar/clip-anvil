# M1.x-D3 Studio Auto Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add user-triggered DAG auto layout for Studio dependency graphs and persist the resulting node coordinates.

**Architecture:** Layout is computed entirely in the web app from the canvas payload. The backend remains a coordinate store through the existing `PATCH /api/nodes/batch-position`; group containers are recomputed from member node bounds after node positions change.

**Tech Stack:** React 19, TypeScript 6, tldraw 5, `@dagrejs/dagre`, TanStack Query.

---

## File Structure

- Modify `apps/web/package.json` and `pnpm-lock.yaml`: add `@dagrejs/dagre`.
- Create `apps/web/src/lib/layout.ts`: pure layout calculation.
- Create `apps/web/src/components/AutoLayoutControls.tsx`: direction switch and button.
- Modify `apps/web/src/pages/WorkspaceDetailPage.tsx`: apply layout and persist positions.
- Modify `apps/web/src/lib/canvas.ts`: expose group bounds helpers if useful.
- Modify `apps/web/src/main.css`: controls styling.

### Task 1: Add dagre and Pure Layout Function

**Files:**
- Modify: `apps/web/package.json`
- Modify: `pnpm-lock.yaml`
- Create: `apps/web/src/lib/layout.ts`

- [ ] **Step 1: Install dependency**

Run:

```bash
pnpm --filter @clip-anvil/web add @dagrejs/dagre
```

Expected: package and lockfile update.

- [ ] **Step 2: Create layout module**

Create `apps/web/src/lib/layout.ts`:

```ts
import dagre from "@dagrejs/dagre";
import type { MediaEdge, MediaGroup, MediaNode } from "./api";

export type LayoutDirection = "LR" | "TB";

export interface LayoutPosition {
  id: string;
  canvas_x: number;
  canvas_y: number;
}

export interface GroupBounds {
  groupId: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export function computeDagreLayout(input: {
  nodes: MediaNode[];
  edges: MediaEdge[];
  groups: MediaGroup[];
  direction: LayoutDirection;
}): { positions: LayoutPosition[]; groupBounds: GroupBounds[] } {
  const graph = new dagre.graphlib.Graph();
  graph.setDefaultEdgeLabel(() => ({}));
  graph.setGraph({
    rankdir: input.direction,
    nodesep: 40,
    ranksep: 80,
    marginx: 20,
    marginy: 20,
  });

  for (const node of input.nodes) {
    graph.setNode(node.id, {
      width: node.canvas_w,
      height: node.canvas_h,
    });
  }

  for (const edge of input.edges) {
    if (edge.edge_type === "dependency") {
      graph.setEdge(edge.from_node_id, edge.to_node_id);
    }
  }

  dagre.layout(graph);

  const positions = input.nodes.map((node) => {
    const layoutNode = graph.node(node.id) as { x: number; y: number };
    return {
      id: node.id,
      canvas_x: layoutNode.x - node.canvas_w / 2,
      canvas_y: layoutNode.y - node.canvas_h / 2,
    };
  });

  const positionedNodes = input.nodes.map((node) => {
    const position = positions.find((item) => item.id === node.id);
    return position ? { ...node, canvas_x: position.canvas_x, canvas_y: position.canvas_y } : node;
  });

  return {
    positions,
    groupBounds: input.groups.map((group) => boundsForGroup(group, positionedNodes)),
  };
}

function boundsForGroup(group: MediaGroup, nodes: MediaNode[]): GroupBounds {
  const members = nodes.filter((node) => group.node_ids.includes(node.id));
  if (members.length === 0) {
    return { groupId: group.id, x: 0, y: 0, w: 240, h: 120 };
  }
  const minX = Math.min(...members.map((node) => node.canvas_x));
  const minY = Math.min(...members.map((node) => node.canvas_y));
  const maxX = Math.max(...members.map((node) => node.canvas_x + node.canvas_w));
  const maxY = Math.max(...members.map((node) => node.canvas_y + node.canvas_h));
  return {
    groupId: group.id,
    x: minX - 20,
    y: minY - 44,
    w: Math.max(240, maxX - minX + 40),
    h: Math.max(120, maxY - minY + 64),
  };
}
```

- [ ] **Step 3: Build to verify dependency typings**

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS or a TypeScript import error for dagre. If import fails, use `import * as dagre from "@dagrejs/dagre";` and rerun.

### Task 2: Auto Layout Controls

**Files:**
- Create: `apps/web/src/components/AutoLayoutControls.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Create control component**

Create `AutoLayoutControls.tsx`:

```tsx
import type { LayoutDirection } from "../lib/layout";

interface AutoLayoutControlsProps {
  direction: LayoutDirection;
  disabled: boolean;
  onDirectionChange: (direction: LayoutDirection) => void;
  onRun: () => void;
}

export function AutoLayoutControls({ direction, disabled, onDirectionChange, onRun }: AutoLayoutControlsProps) {
  return (
    <div className="auto-layout-controls">
      <select aria-label="布局方向" onChange={(event) => onDirectionChange(event.target.value as LayoutDirection)} value={direction}>
        <option value="LR">从左到右</option>
        <option value="TB">从上到下</option>
      </select>
      <button disabled={disabled} onClick={onRun} type="button">自动整理</button>
    </div>
  );
}
```

- [ ] **Step 2: Add CSS**

Append:

```css
.auto-layout-controls {
  position: absolute;
  left: 50%;
  bottom: 16px;
  z-index: 12;
  display: flex;
  gap: 8px;
  align-items: center;
  transform: translateX(-50%);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--color-panel-elevated);
  padding: 6px;
  box-shadow: var(--shadow-popover);
}

.auto-layout-controls select,
.auto-layout-controls button {
  height: 30px;
  border-radius: 6px;
  padding: 0 10px;
  font-size: 12px;
}
```

### Task 3: Apply Layout in WorkspaceDetailPage

**Files:**
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`

- [ ] **Step 1: Add state and imports**

Import:

```ts
import { AutoLayoutControls } from "../components/AutoLayoutControls";
import { computeDagreLayout, type LayoutDirection } from "../lib/layout";
```

Add state:

```ts
const [layoutDirection, setLayoutDirection] = useState<LayoutDirection>("LR");
const [isLayouting, setIsLayouting] = useState(false);
```

- [ ] **Step 2: Implement layout handler**

Add:

```ts
const runAutoLayout = useCallback(() => {
  if (!id || !canvasQuery.data || !editorRef.current) {
    return;
  }
  setIsLayouting(true);
  const result = computeDagreLayout({
    nodes: canvasQuery.data.nodes,
    edges: canvasQuery.data.edges,
    groups: canvasQuery.data.groups,
    direction: layoutDirection,
  });

  const editor = editorRef.current;
  editor.store.mergeRemoteChanges(() => {
    editor.updateShapes(
      result.positions.map((position) => ({
        id: shapeIdForNode(position.id),
        type: "media",
        x: position.canvas_x,
        y: position.canvas_y,
      })),
    );
    editor.updateShapes(
      result.groupBounds.map((bounds) => ({
        id: shapeIdForGroup(bounds.groupId),
        type: "group-container",
        x: bounds.x,
        y: bounds.y,
        props: {
          w: bounds.w,
          h: bounds.h,
        },
      })),
    );
  });

  queryClient.setQueryData<CanvasPayload>(["workspace", id, "canvas"], (current) => {
    if (!current) {
      return current;
    }
    return {
      ...current,
      nodes: current.nodes.map((node) => {
        const position = result.positions.find((item) => item.id === node.id);
        return position ? { ...node, canvas_x: position.canvas_x, canvas_y: position.canvas_y } : node;
      }),
    };
  });

  void batchUpdateNodePositions(result.positions)
    .catch(() => {
      void queryClient.invalidateQueries({ queryKey: ["workspace", id, "canvas"] });
    })
    .finally(() => setIsLayouting(false));
}, [canvasQuery.data, id, layoutDirection, queryClient]);
```

- [ ] **Step 3: Render controls**

Inside the canvas frame:

```tsx
<AutoLayoutControls
  direction={layoutDirection}
  disabled={isLayouting || nodes.length === 0}
  onDirectionChange={setLayoutDirection}
  onRun={runAutoLayout}
/>
```

- [ ] **Step 4: Build**

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

### Task 4: Manual Layout Verification

**Files:**
- No new files.

- [ ] **Step 1: Run app**

```bash
docker compose -f deploy/docker-compose.yml up -d
make server-dev
pnpm --filter @clip-anvil/web dev
```

Expected: backend and frontend start.

- [ ] **Step 2: Create layout test data**

In browser:

1. Create four nodes.
2. Connect them with dependency edges: A -> B, A -> C, B -> D.
3. Create a group around B and C if M1.x-C is present.

Expected: graph is visible before layout.

- [ ] **Step 3: Run LR layout**

Click `自动整理` with `从左到右`.

Expected:

- A appears left of B/C.
- D appears right of B/C.
- Nodes do not overlap.
- Group container still wraps B/C.

- [ ] **Step 4: Verify persistence**

Refresh the browser.

Expected: nodes keep the layout positions because `PATCH /api/nodes/batch-position` persisted them.

- [ ] **Step 5: Run TB layout**

Change direction to `从上到下`, click `自动整理`, refresh.

Expected: A appears above B/C, D below B/C, and positions persist.

### Task 5: Final D3 Verification and Commit

**Files:**
- All files changed in Tasks 1-3.

- [ ] **Step 1: Run frontend build**

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [ ] **Step 2: Run backend tests to ensure no API regression**

```bash
make server-test
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/web/package.json pnpm-lock.yaml apps/web/src/lib/layout.ts apps/web/src/components/AutoLayoutControls.tsx apps/web/src/pages/WorkspaceDetailPage.tsx apps/web/src/main.css
git commit -m "feat: add studio auto layout"
```
