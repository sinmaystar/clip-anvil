# React Flow Canvas Implementation Plan

> **For agentic workers:** This plan describes the production React Flow canvas implementation. Use it as the current-state checklist when continuing canvas work.

**Goal:** Deliver one shared React Flow canvas for Studio and Agent, with Studio enabled for editing and Agent enabled for read-only production review plus safe layout interaction.

**Architecture:** `CanvasFlowSurface` owns the shared React Flow runtime. `StudioFlowCanvas` and `AgentFlowCanvas` pass different mode policies into the same surface. The business database and production services remain the source of truth; React Flow state is derived view state.

**Tech Stack:** React 19, TypeScript 6, Vite 8, TanStack Query, `@xyflow/react`, TailwindCSS 4, Go 1.26, Hertz, pgx/sqlc, PostgreSQL, WebSocket canvas events, in-app browser smoke.

---

## Current Implementation Node

### Shared Surface

- `apps/web/src/components/canvas-flow/CanvasFlowSurface.tsx` renders the React Flow host, background, controls, MiniMap, custom nodes, custom edges, connection lifecycle, node selection, context menu, viewport persistence and file drop.
- `flowModePolicy.ts` defines Studio and Agent capabilities.
- `canvasViewModel.ts` converts `CanvasPayload` into React Flow nodes and edges.
- `canvasViewport.ts` converts backend viewport/camera payloads into React Flow viewport values.

### Studio

- `StudioFlowCanvas.tsx` enables create, connect, delete, upload, run, edit, group, layout and parameter editing.
- Studio node click opens the shared composer panel below the node.
- The composer panel shows dependency previews at the top, prompt in the body, model/operation/parameters at the bottom, and low-frequency details behind a more/details action.

### Agent

- `AgentFlowCanvas.tsx` reuses the same surface and components.
- Agent allows pan, zoom, node selection, node layout dragging and shared information viewing.
- Agent disables content edit, node run, dependency edits, upload, delete and structural mutations.

### Nodes And Edges

- `MediaFlowNode.tsx` renders icon + name without a visible type tab.
- Media previews preserve original aspect ratio and synchronize the outer React Flow node size with the visible content.
- `GroupFlowNode.tsx` renders flat business groups. Group/member movement updates together during drag.
- `DependencyFlowEdge.tsx` renders dependency edges with stable source/right and target/left anchors, continuous flow animation, selection hit area and delete support.

---

## Completion Checklist

### 1. Runtime And View Model

- [x] `@xyflow/react` is the canvas runtime dependency.
- [x] React Flow CSS is loaded in the web app.
- [x] Canvas payloads are projected through a React Flow view-model layer.
- [x] Studio and Agent share node/edge/group projection code.
- [x] WebSocket and polling updates reconcile through Query cache rather than ad hoc React Flow state writes.

### 2. Studio Editing

- [x] Studio loads persisted nodes, edges and groups.
- [x] Right-click creates text/image/video/audio/reference-pack nodes at flow coordinates.
- [x] Dragging nodes persists positions.
- [x] Dragging a group moves its members during the drag.
- [x] Dragging a member updates group bounds during the drag.
- [x] Drag-to-connect creates dependency edges by dropping on the target node.
- [x] Edge selection and edge deletion use explicit state and backend mutation.
- [x] File drop creates persisted source material nodes.
- [x] Auto layout writes positions back to the canvas state.

### 3. Agent Canvas

- [x] Agent renders through the shared React Flow surface.
- [x] Agent can pan, zoom, select nodes and inspect node details.
- [x] Agent can drag node layout when policy allows safe layout mutation.
- [x] Agent does not expose run/edit/connect/delete/upload controls.
- [x] Agent and Studio use the same node card and information panel structure.

### 4. Composer And Details UX

- [x] Node click opens a compact composer panel below the node with a fixed gap.
- [x] Dependency thumbnails/text summaries render at the top of the composer.
- [x] Prompt is the primary editable area.
- [x] Operation, model and supported parameters are directly selectable in the bottom bar.
- [x] Versions, stale reasons and provider details live behind a more/details action.
- [x] Dropdowns close when clicking outside.
- [x] Light and dark themes both keep the composer readable.
- [x] User-uploaded source material uses the same composer/details visual language while remaining read-only.

### 5. Visual Quality

- [x] Nodes use icon + name, not visible type tabs.
- [x] Success state avoids high-saturation full-card borders.
- [x] Image and video nodes preserve original aspect ratio.
- [x] React Flow outer node dimensions match visible media dimensions.
- [x] Edge anchors align with visible node boundaries.
- [x] Edge flow animation runs continuously along the full path.
- [x] React Flow controls are visible in light and dark themes.
- [x] MiniMap is available for large canvases.

---

## Verification

Use these checks when touching the canvas:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

If backend schema, sqlc queries or workspace-mode guards change, also run:

```bash
make sqlc-generate
make server-test
make server-build
```

Manual browser smoke:

1. Open a Studio workspace with image, text, video, dependency edges and a group.
2. Create a node, drag it, refresh and confirm position persistence.
3. Drag a dependency from source to target and confirm the edge remains after refresh.
4. Select and delete one edge; confirm unrelated edges remain visible and remain persisted.
5. Drag a group and confirm members move during the drag.
6. Drag a member and confirm group bounds resize during the drag.
7. Click image/text/video/source-material nodes and inspect the composer/details UI.
8. Switch light/dark theme and verify nodes, controls, MiniMap and composer readability.
9. Open Agent mode and confirm shared rendering with disabled editing actions.
