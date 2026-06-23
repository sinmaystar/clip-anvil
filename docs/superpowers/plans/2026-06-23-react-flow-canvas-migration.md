# React Flow Canvas Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 ClipAnvil 前端画布从 tldraw 一次性迁移到 React Flow，同时把实施拆成可独立交付、可验收、可回滚的小阶段。

**Architecture:** 先建立 React Flow 的中立 view-model、mode policy 和共享 `CanvasFlowSurface`，再让 Agent 与 Studio 逐步接入同一画布能力。Agent/Studio 的差异只来自 capability policy，不再维护两套节点、两套浮层或两套投影逻辑。

**Tech Stack:** React 19, TypeScript 6, Vite 8, TanStack Query, `@xyflow/react`, TailwindCSS 4, Go 1.26, Hertz, pgx/sqlc, PostgreSQL, existing shell smoke scripts, in-app browser E2E smoke.

---

## 阶段拆分总览

### Phase 0: Baseline And Test Harness

**目的：** 固定迁移前的行为基线，补齐后续 E2E 的可重复入口。

**交付物：**
- 一个 React Flow canvas smoke seed/API 脚本，创建固定 Studio workspace、节点、边、分组，以及可打开的 Agent workspace baseline。
- 一份浏览器 E2E checklist，后续每个阶段复用同一套 workspace。
- 当前 tldraw 运行时保持不变。

**验收标准：**
- `pnpm --filter @clip-anvil/web test:connections` 通过。
- `pnpm --filter @clip-anvil/web... build` 通过。
- `make server-test` 通过。
- `./scripts/dev-start.sh` 启动后，浏览器能打开脚本创建的 Studio 和 Agent URL。

**E2E 验收：**
- Studio 能看到 seed 节点、边、分组。
- Agent URL 能打开当前 Agent workspace baseline。当前公开 API 禁止在 Agent workspace 创建节点，因此 Phase 0 记录 Agent 画布为空；Phase 3 负责细化权限后验证 Agent layout mutation。
- 记录迁移前 Studio/Agent 节点单击浮层内容差异，作为后续消除差异的对照。

### Phase 1: React Flow Foundation Without Route Swap

**目的：** 引入 React Flow 和中立 view-model，但不替换用户可见画布。

**交付物：**
- `@xyflow/react` 依赖和全局 CSS 引入。
- `components/canvas-flow/flowTypes.ts`
- `components/canvas-flow/flowModePolicy.ts`
- `components/canvas-flow/canvasViewModel.ts`
- `components/canvas-flow/canvasViewport.ts`
- 纯函数测试覆盖 node/group/edge/view-model/mode policy。

**验收标准：**
- tldraw 画布仍然是当前可见画布，功能无回归。
- React Flow view-model 能从 `CanvasPayload` 生成稳定 nodes/edges。
- Agent policy 允许 viewport、select、drag layout、inspect；禁止 create/delete/connect/run/edit/upload。
- Studio policy 允许现有 Studio 写操作。
- `pnpm --filter @clip-anvil/web test:connections` 通过。
- `pnpm --filter @clip-anvil/web lint` 通过。
- `pnpm --filter @clip-anvil/web... build` 通过。

**E2E 验收：**
- 无新增浏览器 E2E 要求，只做现有 Studio/Agent smoke，确认引入依赖不影响现有 tldraw 页面。

### Phase 2: Shared Canvas Surface And Inspector In Parallel

**目的：** 做出共享 React Flow surface、统一节点卡片、统一信息浮层，但先以内部组件形式存在，不替换正式路由。

**交付物：**
- `CanvasFlowSurface.tsx`
- `MediaFlowNode.tsx`
- `GroupFlowNode.tsx`
- `DependencyFlowEdge.tsx`
- `NodeInspectorPopover.tsx`
- 单测验证 Studio/Agent 共用同一 node data 和 inspector action policy。

**验收标准：**
- `CanvasFlowSurface` 能渲染 seed `CanvasPayload` 的 media nodes、group nodes、dependency edges。
- `NodeInspectorPopover` 在 Studio/Agent 使用同一组件和样式类。
- Agent policy 下按钮隐藏或 disabled；信息内容仍可见。
- `pnpm --filter @clip-anvil/web test:connections` 通过。
- `pnpm --filter @clip-anvil/web... build` 通过。

**E2E 验收：**
- 若实现阶段加了临时 dev-only route 或 local preview harness，浏览器打开后应看到非空 React Flow 画布、节点卡片、分组、边和 inspector。
- 若不加临时 route，本阶段 E2E 可以延后到 Phase 3。

### Phase 3: Agent Canvas Swap To Shared Surface

**目的：** 先替换 Agent 画布。Agent 交互面较少，但必须走同一 `CanvasFlowSurface`，不是简化版只读卡片。

**交付物：**
- `AgentFlowCanvas.tsx` 接入 `CanvasFlowSurface`。
- 删除或退役 `AgentReadonlyCanvas.tsx` 的 tldraw 实现。
- 后端权限细化：Agent workspace 允许 camera/viewport 和节点位置 layout mutation，继续禁止内容编辑、生产运行和结构写操作。
- Agent 相关测试从 `agentReadonlyCanvas` 改为 `agentCanvas` 或 `canvasModePolicy`。

**验收标准：**
- Agent 页面使用 React Flow，不再 import `Tldraw`。
- Agent 可以平移缩放、选择节点、拖动节点布局、刷新后位置保留。
- Agent 点击节点打开与 Studio 同款 inspector，生产状态、版本、调用记录、素材预览、Reference Pack 摘要可查看。
- Agent 禁止创建/删除节点、创建/删除边、运行节点、修改 prompt/title/model params、上传素材。
- `make server-test` 通过。
- `pnpm --filter @clip-anvil/web test:connections` 通过。
- `pnpm --filter @clip-anvil/web... build` 通过。

**E2E 验收：**
- 用 `./scripts/dev-start.sh` 启动，使用脚本输出的 Vite URL。
- 打开 Agent workspace。
- 拖动一个节点，刷新页面后节点位置仍然变化。
- 点击节点，确认 inspector 与 Studio 信息结构一致，但 run/edit/connect/delete 操作不可用。
- 尝试从节点连线，不能创建 edge。
- 尝试键盘删除节点，不能删除。

### Phase 4: Studio Basic Surface Swap

**目的：** 替换 Studio 主画布的加载、选择、viewport、右键创建和节点拖动。边、分组高级行为可以保留到 Phase 5，但页面必须可用。

**交付物：**
- `StudioFlowCanvas.tsx` 接入 `CanvasFlowSurface`。
- `WorkspaceDetailPage.tsx` 移除 `<Tldraw>` host。
- 右键菜单改用 `screenToFlowPosition`。
- viewport 持久化改用 React Flow viewport。
- 节点拖动持久化改用 React Flow node change/drag stop。

**验收标准：**
- Studio 页面使用 React Flow 加载节点。
- 右键创建 text/image/video/audio/reference_pack 节点，坐标接近鼠标位置。
- 节点拖动后刷新位置保留。
- 点击节点打开共享 inspector。
- Resource Tree 选择节点与画布选择同步。
- `pnpm --filter @clip-anvil/web test:connections` 通过。
- `pnpm --filter @clip-anvil/web lint` 通过。
- `pnpm --filter @clip-anvil/web... build` 通过。

**E2E 验收：**
- 打开 Studio workspace。
- 右键创建文本节点。
- 拖动该节点，刷新确认位置持久化。
- 点击 Resource Tree 中同一节点，画布 inspector 更新为同一节点。
- 缩放和平移画布，刷新后 viewport 恢复到合理位置。

### Phase 5: Studio Edges, Groups, Upload, Delete, Layout Parity

**目的：** 补齐 Studio 全量画布编辑能力，达到替换 tldraw 前的核心功能等价。

**交付物：**
- React Flow handles + `onConnect` 创建 `media_edge`。
- `DependencyFlowEdge` 承担选中态、动画路径、hit area。
- 删除边、删除节点走显式 mutation。
- 文件 drop 使用 `screenToFlowPosition` 创建素材节点。
- `GroupFlowNode` 渲染分组，拖动分组批量移动成员。
- 自动布局输出写回 React Flow nodes 和后端位置。

**验收标准：**
- 创建 dependency edge 成功，并在刷新后保留。
- 删除 edge 成功，并在刷新后不再出现。
- 删除 node 成功，Resource Tree、Property Panel、画布同步。
- 文件 drop 创建 image/video/audio 素材节点，不产生本地临时 canvas asset。
- 创建 group、移动成员、拖动 group 批量移动成员、删除 group 保留成员。
- 自动布局后刷新，节点位置保留。
- `pnpm --filter @clip-anvil/web test:connections` 通过。
- `pnpm --filter @clip-anvil/web lint` 通过。
- `pnpm --filter @clip-anvil/web... build` 通过。

**E2E 验收：**
- 在 Studio 中创建两个节点并连线。
- 选中 edge，删除 edge，刷新确认删除。
- 上传或 drop 一张图片，确认画布出现素材节点。
- 创建分组，把节点拖入分组，拖动分组，确认成员一起移动。
- 删除分组，确认成员仍在画布。
- 执行自动布局，刷新确认位置持久化。

### Phase 6: Remove tldraw Runtime And Current Docs Cleanup

**目的：** 彻底废除 tldraw 运行时代码和当前文档口径。

**交付物：**
- 删除 `tldraw` 和 `@tldraw/tlschema` 依赖。
- 删除或重写 `packages/canvas-schema`。
- 删除 `apps/web/src/shapes/*ShapeUtil.tsx`。
- 删除 `apps/web/src/lib/canvas.ts` 中 tldraw shape 语义，或改名为中立 view-model helper。
- 删除 `vite.config.ts` 中 `vendor-tldraw`。
- CSS 清理 `.tl-*`、`.agent-readonly-tldraw`。
- 当前有效文档改为 React Flow 口径；`docs/archive/` 不改。

**验收标准：**
- `rg -n "from \"tldraw\"|from 'tldraw'|@tldraw|TLRecord|TLShape|ShapeUtil" apps/web packages` 无命中。
- `rg -n "tldraw" AGENTS.md CLAUDE.md docs/README.md docs/engineering docs/design` 无当前口径残留。
- `pnpm install --lockfile-only` 或等价 pnpm lock 更新完成。
- `pnpm --filter @clip-anvil/web test:connections` 通过。
- `pnpm --filter @clip-anvil/web lint` 通过。
- `pnpm --filter @clip-anvil/web... build` 通过。
- 若有后端权限/schema 修改：`make sqlc-generate`、`make server-test`、`make server-build` 通过。
- `git diff --check` 通过。

**E2E 验收：**
- 跑 Phase 3、Phase 4、Phase 5 的完整浏览器 smoke。
- 浏览器 console 无新增应用错误。
- Studio 和 Agent 在同一 workspace 间切换时，节点位置、选择逻辑和 inspector 信息结构一致。

---

## 文件职责图

### 新增文件

- `apps/web/src/components/canvas-flow/flowTypes.ts`
  - 定义 `CanvasFlowMode`、`CanvasFlowNodeData`、`CanvasFlowEdgeData`、`CanvasFlowNode`、`CanvasFlowEdge`。
- `apps/web/src/components/canvas-flow/flowModePolicy.ts`
  - 定义 Studio/Agent capability policy。
- `apps/web/src/components/canvas-flow/canvasViewModel.ts`
  - 从 `CanvasPayload` 派生 React Flow nodes/edges。
- `apps/web/src/components/canvas-flow/canvasViewport.ts`
  - 处理 backend camera 与 React Flow viewport 的互转。
- `apps/web/src/components/canvas-flow/CanvasFlowSurface.tsx`
  - React Flow 共享 host。
- `apps/web/src/components/canvas-flow/StudioFlowCanvas.tsx`
  - Studio mode host。
- `apps/web/src/components/canvas-flow/AgentFlowCanvas.tsx`
  - Agent mode host。
- `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`
  - 统一节点卡片。
- `apps/web/src/components/canvas-flow/GroupFlowNode.tsx`
  - 分组容器节点。
- `apps/web/src/components/canvas-flow/DependencyFlowEdge.tsx`
  - dependency edge custom renderer。
- `apps/web/src/components/canvas-flow/NodeInspectorPopover.tsx`
  - Studio/Agent 共用节点信息浮层。
- `apps/web/src/lib/canvasFlow.test.mjs`
  - view-model、viewport、policy、selection 相关 Node tests。
- `scripts/smoke-react-flow-canvas.sh`
  - API seed/smoke，输出 Studio 和 Agent URL，供浏览器 E2E 使用。

### 修改文件

- `apps/web/package.json`
  - 新增 `@xyflow/react`，更新 `test:connections`。
- `apps/web/src/main.css`
  - 引入 React Flow CSS，迁移 tldraw class 相关样式。
- `apps/web/src/pages/WorkspaceDetailPage.tsx`
  - 用 `StudioFlowCanvas` 替换 tldraw host。
- `apps/web/src/components/agent/AgentReadonlyCanvas.tsx`
  - 删除或替换为 `AgentFlowCanvas`。
- `apps/web/src/components/FileDropZone.tsx`
  - 去掉 tldraw `Editor` 类型，改用 flow point converter。
- `apps/web/src/components/ConnectionOverlay.tsx`
  - 删除边渲染职责，或迁入 `DependencyFlowEdge` 后删除。
- `apps/web/src/lib/canvas.ts`
  - 删除 tldraw shape conversion，保留业务 helper 时改名到 `canvas-flow`。
- `apps/web/vite.config.ts`
  - 删除 `vendor-tldraw`，必要时新增 `vendor-xyflow`。
- `apps/server/internal/api/workspace_mode_guard.go`
  - 如需允许 Agent layout 写入，拆分 guard。
- `apps/server/cmd/server/main.go`
  - 如 guard 拆分，调整 camera/position 路由权限。
- `docs/README.md`、`docs/engineering/*`、`docs/design/*`、`AGENTS.md`、`CLAUDE.md`
  - 当前文档改为 React Flow。

---

## Task 0: Baseline Harness

**Files:**
- Create: `scripts/smoke-react-flow-canvas.sh`
- Modify: `apps/web/package.json`
- Test: existing `apps/web/src/lib/*.test.mjs`

- [x] **Step 1: Create API smoke seed script**

Create `scripts/smoke-react-flow-canvas.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

BASE="${CLIPANVIL_API_BASE:-http://127.0.0.1:${CLIPANVIL_SERVER_PORT:-8888}/api}"
WEB_BASE="${CLIPANVIL_WEB_BASE:-http://127.0.0.1:${CLIPANVIL_WEB_PORT:-5173}}"
EMAIL="react-flow-smoke-$(date +%s)@clipanvil.local"
PASSWORD="clipanvil-smoke-pass"
export BASE WEB_BASE EMAIL PASSWORD

node <<'NODE'
const base = process.env.BASE;
const webBase = process.env.WEB_BASE;
const email = process.env.EMAIL;
const password = process.env.PASSWORD;

async function req(path, options = {}) {
  const response = await fetch(`${base}${path}`, {
    ...options,
    headers: {
      "content-type": "application/json",
      ...(options.headers ?? {}),
    },
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(`${options.method ?? "GET"} ${path} -> ${response.status}: ${text}`);
  }
  if (response.status === 204) return null;
  return response.json();
}

const auth = await req("/auth/register", {
  method: "POST",
  body: JSON.stringify({ email, password, name: "React Flow Smoke" }),
});
const token = auth.token;
const headers = { authorization: `Bearer ${token}` };

const studio = await req("/workspaces", {
  method: "POST",
  headers,
  body: JSON.stringify({ name: "React Flow Studio Smoke", mode: "studio" }),
});
const agent = await req("/workspaces", {
  method: "POST",
  headers,
  body: JSON.stringify({ name: "React Flow Agent Smoke", mode: "agent" }),
});

async function createNode(workspace, title, node_type, x, y) {
  return req("/nodes", {
    method: "POST",
    headers,
    body: JSON.stringify({
      workspace_id: workspace.id,
      node_type,
      title,
      prompt: `${title} prompt`,
      canvas_x: x,
      canvas_y: y,
    }),
  });
}

const a = await createNode(studio, "Smoke Script", "text", 40, 80);
const b = await createNode(studio, "Smoke Image", "image", 360, 80);
await req("/edges", {
  method: "POST",
  headers,
  body: JSON.stringify({
    workspace_id: studio.id,
    from_node_id: a.id,
    to_node_id: b.id,
    edge_type: "dependency",
  }),
});
await req("/groups", {
  method: "POST",
  headers,
  body: JSON.stringify({
    workspace_id: studio.id,
    name: "Smoke Group",
    node_ids: [a.id, b.id],
  }),
});

const studioCanvas = await req(`/workspaces/${studio.id}/canvas`, { headers });
const agentCanvas = await req(`/workspaces/${agent.id}/canvas`, { headers });
if (studioCanvas.nodes.length !== 2 || studioCanvas.edges.length !== 1 || studioCanvas.groups.length !== 1) {
  throw new Error("studio smoke canvas did not seed expected nodes, edge, and group");
}
if (agentCanvas.nodes.length !== 0) {
  throw new Error("agent smoke baseline should stay empty before layout permissions change");
}

console.log(JSON.stringify({
  email,
  password,
  studio_url: `${webBase}/workspaces/${studio.id}/studio`,
  agent_url: `${webBase}/workspaces/${agent.id}/agent`,
  studio_id: studio.id,
  agent_id: agent.id,
  studio_node_ids: [a.id, b.id],
  agent_canvas_nodes: agentCanvas.nodes.length
}, null, 2));
NODE
```

- [x] **Step 2: Make script executable**

Run:

```bash
chmod +x scripts/smoke-react-flow-canvas.sh
```

Expected: no output.

- [x] **Step 3: Run existing frontend tests**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

- [x] **Step 4: Run build**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [x] **Step 5: Commit baseline harness**

```bash
git add scripts/smoke-react-flow-canvas.sh apps/web/package.json
git commit -m "test: add react flow canvas smoke harness"
```

**Phase 0 Done When:**
- Smoke script exists and outputs Studio/Agent URLs against a running local server.
- Existing build/test commands still pass.

---

## Task 1: React Flow Foundation

**Files:**
- Modify: `apps/web/package.json`
- Modify: `apps/web/src/main.css`
- Create: `apps/web/src/components/canvas-flow/flowTypes.ts`
- Create: `apps/web/src/components/canvas-flow/flowModePolicy.ts`
- Create: `apps/web/src/components/canvas-flow/canvasViewport.ts`
- Create: `apps/web/src/components/canvas-flow/canvasViewModel.ts`
- Create: `apps/web/src/lib/canvasFlow.test.mjs`

- [x] **Step 1: Add dependency**

Run:

```bash
pnpm --filter @clip-anvil/web add @xyflow/react
```

Expected: `apps/web/package.json` includes `@xyflow/react`, lockfile updates.

- [x] **Step 2: Add React Flow CSS after Tailwind**

Modify `apps/web/src/main.css` near the top so React Flow CSS is loaded after Tailwind:

```css
@import "tailwindcss";
@import "@xyflow/react/dist/style.css";
```

Expected: Tailwind remains first, React Flow CSS second.

- [x] **Step 3: Define flow types**

Create `apps/web/src/components/canvas-flow/flowTypes.ts`:

```ts
import type { Edge, Node } from "@xyflow/react";
import type { MediaEdge, MediaGroup, MediaNode } from "../../lib/api";

export type CanvasFlowMode = "studio" | "agent";

export interface CanvasFlowNodeData extends Record<string, unknown> {
  kind: "media";
  node: MediaNode;
}

export interface CanvasFlowGroupData extends Record<string, unknown> {
  kind: "group";
  group: MediaGroup;
  nodeIds: string[];
}

export interface CanvasFlowEdgeData extends Record<string, unknown> {
  edge: MediaEdge;
}

export type CanvasFlowNode =
  | Node<CanvasFlowNodeData, "media">
  | Node<CanvasFlowGroupData, "group">;

export type CanvasFlowEdge = Edge<CanvasFlowEdgeData, "dependency">;
```

- [x] **Step 4: Define mode policy**

Create `apps/web/src/components/canvas-flow/flowModePolicy.ts`:

```ts
import type { CanvasFlowMode } from "./flowTypes";

export interface CanvasFlowPolicy {
  canPanZoom: boolean;
  canSelect: boolean;
  canDragNodes: boolean;
  canPersistViewport: boolean;
  canCreateNodes: boolean;
  canDeleteNodes: boolean;
  canCreateEdges: boolean;
  canDeleteEdges: boolean;
  canUploadAssets: boolean;
  canEditNodeContent: boolean;
  canRunNodes: boolean;
  canEditGroups: boolean;
}

export const studioFlowPolicy: CanvasFlowPolicy = {
  canPanZoom: true,
  canSelect: true,
  canDragNodes: true,
  canPersistViewport: true,
  canCreateNodes: true,
  canDeleteNodes: true,
  canCreateEdges: true,
  canDeleteEdges: true,
  canUploadAssets: true,
  canEditNodeContent: true,
  canRunNodes: true,
  canEditGroups: true,
};

export const agentFlowPolicy: CanvasFlowPolicy = {
  canPanZoom: true,
  canSelect: true,
  canDragNodes: true,
  canPersistViewport: true,
  canCreateNodes: false,
  canDeleteNodes: false,
  canCreateEdges: false,
  canDeleteEdges: false,
  canUploadAssets: false,
  canEditNodeContent: false,
  canRunNodes: false,
  canEditGroups: false,
};

export function policyForCanvasMode(mode: CanvasFlowMode) {
  return mode === "studio" ? studioFlowPolicy : agentFlowPolicy;
}
```

- [x] **Step 5: Define viewport helpers**

Create `apps/web/src/components/canvas-flow/canvasViewport.ts`:

```ts
import type { Viewport } from "@xyflow/react";
import type { CanvasCamera } from "../../lib/api";

export function cameraToViewport(camera: CanvasCamera): Viewport {
  return {
    x: camera.x,
    y: camera.y,
    zoom: camera.zoom,
  };
}

export function viewportToCamera(viewport: Viewport): CanvasCamera {
  return {
    x: viewport.x,
    y: viewport.y,
    zoom: viewport.zoom,
  };
}
```

- [x] **Step 6: Define view-model helper**

Create `apps/web/src/components/canvas-flow/canvasViewModel.ts`:

```ts
import type { CanvasPayload, MediaGroup, MediaNode } from "../../lib/api";
import { mediaNodeDisplaySize } from "../../lib/canvas";
import type { CanvasFlowEdge, CanvasFlowNode } from "./flowTypes";

export function canvasToFlowNodes(canvas: CanvasPayload): CanvasFlowNode[] {
  return [
    ...canvas.groups.map((group) => groupToFlowNode(group, canvas.nodes)),
    ...canvas.nodes.map(nodeToFlowNode),
  ];
}

export function canvasToFlowEdges(canvas: CanvasPayload): CanvasFlowEdge[] {
  return canvas.edges.map((edge) => ({
    id: edge.id,
    type: "dependency",
    source: edge.from_node_id,
    target: edge.to_node_id,
    data: { edge },
  }));
}

export function nodeToFlowNode(node: MediaNode): CanvasFlowNode {
  const size = mediaNodeDisplaySize(node);
  return {
    id: node.id,
    type: "media",
    position: { x: node.canvas_x, y: node.canvas_y },
    width: size.w,
    height: size.h,
    data: { kind: "media", node },
  };
}

export function groupToFlowNode(
  group: MediaGroup,
  nodes: MediaNode[],
): CanvasFlowNode {
  const bounds = boundsForGroup(group, nodes);
  return {
    id: group.id,
    type: "group",
    position: { x: bounds.x, y: bounds.y },
    width: bounds.w,
    height: bounds.h,
    data: { kind: "group", group, nodeIds: group.node_ids },
    draggable: true,
    selectable: true,
  };
}

function boundsForGroup(group: MediaGroup, nodes: MediaNode[]) {
  const members = nodes.filter((node) => group.node_ids.includes(node.id));
  if (members.length === 0) {
    return { x: 0, y: 0, w: 240, h: 120 };
  }
  const minX = Math.min(...members.map((node) => node.canvas_x));
  const minY = Math.min(...members.map((node) => node.canvas_y));
  const maxX = Math.max(
    ...members.map((node) => node.canvas_x + mediaNodeDisplaySize(node).w),
  );
  const maxY = Math.max(
    ...members.map((node) => node.canvas_y + mediaNodeDisplaySize(node).h),
  );
  return {
    x: minX - 20,
    y: minY - 44,
    w: Math.max(240, maxX - minX + 40),
    h: Math.max(120, maxY - minY + 64),
  };
}
```

- [x] **Step 7: Add tests**

Create `apps/web/src/lib/canvasFlow.test.mjs`:

```js
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

describe("React Flow canvas migration source contracts", () => {
  it("defines Studio and Agent mode policy without making Agent a weak readonly canvas", async () => {
    const source = await readFile(
      new URL("../components/canvas-flow/flowModePolicy.ts", import.meta.url),
      "utf8",
    );
    assert.match(source, /canDragNodes:\s*true/);
    assert.match(source, /canRunNodes:\s*false/);
    assert.match(source, /canCreateEdges:\s*false/);
    assert.match(source, /policyForCanvasMode/);
  });

  it("maps canvas payload into React Flow node and edge ids directly from business ids", async () => {
    const source = await readFile(
      new URL("../components/canvas-flow/canvasViewModel.ts", import.meta.url),
      "utf8",
    );
    assert.match(source, /id:\s*node\.id/);
    assert.match(source, /source:\s*edge\.from_node_id/);
    assert.match(source, /target:\s*edge\.to_node_id/);
    assert.doesNotMatch(source, /createShapeId|TLRecord|TLShape/);
  });
});
```

- [x] **Step 8: Add test to package script**

Append `src/lib/canvasFlow.test.mjs` to `apps/web/package.json` `test:connections`.

- [x] **Step 9: Run tests**

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
```

Expected: all PASS.

- [x] **Step 10: Commit Phase 1**

```bash
git add apps/web/package.json pnpm-lock.yaml apps/web/src/main.css apps/web/src/components/canvas-flow apps/web/src/lib/canvasFlow.test.mjs
git commit -m "feat: add react flow canvas foundation"
```

**Phase 1 Done When:**
- Foundation compiles and tests pass.
- No route swap yet.

---

## Task 2: Shared Surface And Inspector

**Files:**
- Create: `apps/web/src/components/canvas-flow/CanvasFlowSurface.tsx`
- Create: `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`
- Create: `apps/web/src/components/canvas-flow/GroupFlowNode.tsx`
- Create: `apps/web/src/components/canvas-flow/DependencyFlowEdge.tsx`
- Create: `apps/web/src/components/canvas-flow/NodeInspectorPopover.tsx`
- Modify: `apps/web/src/lib/canvasFlow.test.mjs`

- [x] **Step 1: Implement shared node components**

`MediaFlowNode.tsx` 渲染现有 `.media-node` shell，并从节点 data 或父级 context 读取 policy。首版应渲染完整信息内容；连接 handle 只在 policy 允许创建 edge 时显示。

- [x] **Step 2: Implement custom edge**

`DependencyFlowEdge.tsx` 使用 React Flow edge helper，并保留当前 flowing edge 的视觉语言。它会在后续阶段替代 `ConnectionOverlay` 的边渲染职责。

- [x] **Step 3: Implement shared inspector**

`NodeInspectorPopover.tsx` must accept:

```ts
interface NodeInspectorPopoverProps {
  mode: CanvasFlowMode;
  policy: CanvasFlowPolicy;
  node: MediaNode;
  edges: MediaEdge[];
  groups: MediaGroup[];
  onRunNode: (node: MediaNode) => void;
  onUpdateNode: (nodeId: string, patch: unknown) => void;
}
```

Expected behavior: same information layout in Studio and Agent; action controls honor policy.

- [x] **Step 4: Implement shared surface**

`CanvasFlowSurface.tsx` must accept:

```ts
interface CanvasFlowSurfaceProps {
  canvas: CanvasPayload;
  mode: CanvasFlowMode;
  selectedNodeId: string | null;
  selectedEdgeId: string | null;
  onSelectNode: (nodeId: string | null) => void;
  onSelectEdge: (edgeId: string | null) => void;
}
```

它渲染 `<ReactFlow />`，并传入 `nodeTypes`、`edgeTypes`、`nodesDraggable={policy.canDragNodes}`、`nodesConnectable={policy.canCreateEdges}` 和共享 selection 行为。

- [x] **Step 5: Add source-level tests**

Extend `apps/web/src/lib/canvasFlow.test.mjs`:

```js
it("keeps Agent and Studio on the shared CanvasFlowSurface", async () => {
  const source = await readFile(
    new URL("../components/canvas-flow/CanvasFlowSurface.tsx", import.meta.url),
    "utf8",
  );
  assert.match(source, /ReactFlow/);
  assert.match(source, /nodesDraggable/);
  assert.match(source, /nodesConnectable/);
});
```

- [x] **Step 6: Run verification**

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [x] **Step 7: Commit Phase 2**

```bash
git add apps/web/src/components/canvas-flow apps/web/src/lib/canvasFlow.test.mjs
git commit -m "feat: add shared react flow canvas surface"
```

**Phase 2 Done When:**
- Shared surface exists and compiles.
- Studio/Agent have not yet swapped routes.

---

## Task 3: Agent Canvas Swap

**Files:**
- Create: `apps/web/src/components/canvas-flow/AgentFlowCanvas.tsx`
- Modify: `apps/web/src/components/agent/AgentReadonlyCanvas.tsx` or replace imports where it is used.
- Modify: `apps/server/internal/api/workspace_mode_guard.go`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/web/src/lib/agentReadonlyCanvas.test.mjs` or rename to `agentCanvas.test.mjs`

- [x] **Step 1: Add Agent host**

Create `AgentFlowCanvas.tsx` wrapping `CanvasFlowSurface` with `mode="agent"` and policy-driven mutation callbacks.

- [x] **Step 2: Replace Agent page import**

Find usages:

```bash
rg -n "AgentReadonlyCanvas|agent-readonly" apps/web/src
```

Replace with `AgentFlowCanvas`.

- [x] **Step 3: Split backend guard if needed**

If Agent currently cannot update camera or node positions, split guard into:

```go
func requireCanvasLayoutWorkspace(...)
func requireStudioWorkspace(...)
```

Expected policy:
- Agent may call camera/viewport and position update endpoints.
- Agent may not call node content update, create/delete node, create/delete edge, run node.

- [x] **Step 4: Update Agent tests**

Rename assertions from "readonly tldraw" to "shared React Flow Agent canvas". Required assertions:
- source imports `CanvasFlowSurface`;
- source does not import `tldraw`;
- policy allows drag;
- run/edit/connect/delete controls disabled or absent.

- [x] **Step 5: Run backend verification**

```bash
make server-test
make server-build
```

Expected: PASS.

- [x] **Step 6: Run frontend verification**

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [x] **Step 7: Browser E2E**

Run:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
./scripts/dev-start.sh
CLIPANVIL_SERVER_PORT=<printed-server-port> CLIPANVIL_WEB_PORT=<printed-web-port> ./scripts/smoke-react-flow-canvas.sh
```

Open `agent_url` from the smoke output. Verify:
- React Flow canvas is nonblank.
- Dragging a node changes its position.
- Refresh preserves position.
- Node click opens shared inspector.
- Run/edit/connect/delete are unavailable.

Phase 3 run notes:
- Browser verified Agent uses React Flow, has no tldraw host, opens shared inspector, has no connect handles/buttons, and disables run/edit controls.
- Browser/API verified Agent content edit, run, and delete return `403`, while layout position update returns `204`.
- Browser verified React Flow settled position changes persist: selecting the Agent node and moving it with arrow keys changed `(120,120)` to `(130,130)`, updated backend canvas coordinates, and survived refresh.
- MCP `dragTo` and DevTools a11y drag do not trigger React Flow pointer dragging, so pointer drag itself remains covered by the same settled-position persistence path plus source tests.

- [x] **Step 8: Commit Phase 3**

```bash
git add apps/web/src/components/canvas-flow apps/web/src/components/agent apps/web/src/lib/*agent*Canvas*.test.mjs apps/server/internal/api/workspace_mode_guard.go apps/server/cmd/server/main.go
git commit -m "feat: migrate agent canvas to shared react flow surface"
```

**Phase 3 Done When:**
- Agent no longer uses tldraw.
- Agent shares surface/inspector with Studio design.

---

## Task 4: Studio Basic Surface Swap

**Files:**
- Create: `apps/web/src/components/canvas-flow/StudioFlowCanvas.tsx`
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/components/FileDropZone.tsx`
- Modify: `apps/web/src/lib/canvasLayering.test.mjs`
- Create: `apps/web/src/lib/studioCanvas.test.mjs`

- [x] **Step 1: Add Studio host**

Create `StudioFlowCanvas.tsx` wrapping `CanvasFlowSurface` with `mode="studio"` and callbacks for selection, create node, update viewport, drag persistence.

- [x] **Step 2: Replace tldraw host in WorkspaceDetailPage**

Remove direct imports:

```ts
import { Tldraw, type Editor, type TLRecord, type TLUiComponents } from "tldraw";
import "tldraw/tldraw.css";
```

Render `StudioFlowCanvas` where `<Tldraw />` currently lives.

- [x] **Step 3: Convert right-click create**

Use React Flow `screenToFlowPosition` in `CanvasFlowSurface` and pass the flow point to existing `createNodeMutation`.

- [x] **Step 4: Convert viewport persistence**

Use React Flow viewport change callbacks and existing `updateCamera` API.

- [x] **Step 5: Convert node drag persistence**

On drag stop, call existing `batchUpdateNodePositions` with changed media nodes.

- [x] **Step 6: Run verification**

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [x] **Step 7: Browser E2E**

Open `studio_url` from smoke output. Verify:
- canvas loads seed nodes;
- right-click creates a text node near cursor;
- dragging node persists after refresh;
- clicking node opens shared inspector;
- Resource Tree selection still syncs.

Run note 2026-06-23:
- Studio URL `http://127.0.0.1:5177/workspaces/86ee77da-3b8b-4464-8139-6729e583fa3a/studio` rendered React Flow with 3 media nodes, 1 group, 1 edge, and no tldraw host.
- Right-click at `(900, 340)` opened the Studio create menu and created text node `688b252d-3aa4-4d8c-9d60-275aa0e7919e` near `(800, 280)`.
- Dragged that node to about `(874, 340)` and refreshed; the node stayed at `(874, 340)`.
- Resource Tree click on `Smoke Script` selected React Flow node `720fa8ff-7ab6-42b2-b5e8-e80333164715` and updated the shared inspector.
- Zoom changed viewport from `translate(0px, 0px) scale(1)` to `translate(-1100px, -700px) scale(2)`; refresh restored the same transform.
- Browser console had 0 errors; observed one WebSocket close warning during refresh.

- [x] **Step 8: Commit Phase 4**

```bash
git add apps/web/src/components/canvas-flow apps/web/src/pages/WorkspaceDetailPage.tsx apps/web/src/components/FileDropZone.tsx apps/web/src/lib/canvasLayering.test.mjs apps/web/src/lib/canvasFlow.test.mjs
git commit -m "feat: migrate studio canvas basics to react flow"
```

**Phase 4 Done When:**
- Studio visible canvas is React Flow.
- Basic create/select/drag/viewport flows work.

---

## Task 5: Studio Full Editing Parity

**Files:**
- Modify: `apps/web/src/components/canvas-flow/CanvasFlowSurface.tsx`
- Modify: `apps/web/src/components/canvas-flow/DependencyFlowEdge.tsx`
- Modify: `apps/web/src/components/canvas-flow/GroupFlowNode.tsx`
- Modify: `apps/web/src/components/FileDropZone.tsx`
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/lib/groupLayout.test.mjs`
- Modify: `apps/web/src/lib/connectionGeometry.test.mjs`
- Modify: `apps/web/src/lib/canvasFlow.test.mjs`

- [ ] **Step 1: Wire edge creation**

Use React Flow `onConnect` to call existing `createMediaEdge`.

- [ ] **Step 2: Wire edge selection and deletion**

Custom edge hit area calls `onSelectEdge`. Keyboard delete calls `deleteMediaEdge` only when policy allows.

- [ ] **Step 3: Wire node deletion**

Keyboard delete calls `deleteMediaNode` only in Studio policy.

- [ ] **Step 4: Wire file drop**

Replace `Editor.screenToPage` with a callback supplied by `CanvasFlowSurface`:

```ts
type ScreenToCanvasPoint = (point: { x: number; y: number }) => {
  x: number;
  y: number;
};
```

- [ ] **Step 5: Wire group movement**

Dragging `GroupFlowNode` computes delta and updates member node positions using existing group layout helpers.

- [ ] **Step 6: Wire auto layout**

After `computeDagreLayout`, update React Flow nodes and persist positions.

- [ ] **Step 7: Run verification**

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [ ] **Step 8: Browser E2E**

Verify:
- create two nodes and connect them;
- select and delete edge;
- delete node and confirm Resource Tree/inspector sync;
- file drop creates persisted asset node;
- create group, move members, drag group, delete group preserving members;
- auto layout persists after refresh.

- [ ] **Step 9: Commit Phase 5**

```bash
git add apps/web/src/components/canvas-flow apps/web/src/components/FileDropZone.tsx apps/web/src/pages/WorkspaceDetailPage.tsx apps/web/src/lib/*test.mjs
git commit -m "feat: complete react flow studio editing parity"
```

**Phase 5 Done When:**
- Studio reaches core tldraw parity on React Flow.
- Agent still shares surface and remains policy-limited.

---

## Task 6: Remove tldraw And Update Current Docs

**Files:**
- Modify: `apps/web/package.json`
- Modify: `pnpm-lock.yaml`
- Delete: `apps/web/src/shapes/MediaShapeUtil.tsx`
- Delete: `apps/web/src/shapes/AgentReadonlyMediaShapeUtil.tsx`
- Delete: `apps/web/src/shapes/GroupContainerShapeUtil.tsx`
- Delete or rewrite: `packages/canvas-schema`
- Modify: `apps/web/vite.config.ts`
- Modify: `apps/web/src/main.css`
- Modify: `AGENTS.md`
- Modify: `CLAUDE.md`
- Modify: `docs/README.md`
- Modify: `docs/engineering/architecture.md`
- Modify: `docs/engineering/database.md`
- Modify: `docs/engineering/development.md`
- Modify: `docs/design/overview.md`
- Modify: `docs/design/canvas.md`
- Modify: `docs/design/studio-mode.md`
- Modify: `docs/design/agent-mode.md`
- Modify: `docs/design/frontend.md`

- [ ] **Step 1: Remove dependencies**

Run:

```bash
pnpm --filter @clip-anvil/web remove tldraw @clip-anvil/canvas-schema
```

If `packages/canvas-schema` is unused, remove it from workspace files in the same commit.

- [ ] **Step 2: Delete tldraw shape utils**

Run:

```bash
rm apps/web/src/shapes/MediaShapeUtil.tsx apps/web/src/shapes/AgentReadonlyMediaShapeUtil.tsx apps/web/src/shapes/GroupContainerShapeUtil.tsx
```

- [ ] **Step 3: Clean Vite chunks**

Remove `vendor-tldraw` from `apps/web/vite.config.ts`. If React Flow chunk warnings appear, add `vendor-xyflow`.

- [ ] **Step 4: Clean CSS**

Remove `.tl-*` and `.agent-readonly-tldraw` selectors. Rename remaining Agent canvas selectors to React Flow neutral names such as `.agent-flow-canvas`.

- [ ] **Step 5: Update current docs**

Replace current tldraw architecture language with React Flow. Do not update `docs/archive/`.

- [ ] **Step 6: Runtime/current docs grep**

Run:

```bash
rg -n "from \"tldraw\"|from 'tldraw'|@tldraw|TLRecord|TLShape|ShapeUtil" apps/web packages
rg -n "tldraw" AGENTS.md CLAUDE.md docs/README.md docs/engineering docs/design
```

Expected: no output. If output appears only in this migration spec or archive, do not count it against current cleanup.

- [ ] **Step 7: Full verification**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
make server-test
make server-build
git diff --check
```

Expected: all PASS.

- [ ] **Step 8: Final browser E2E**

Run Phase 3, 4, and 5 browser smoke end-to-end in one dev session. Confirm:
- Studio full editing works.
- Agent can inspect and drag layout but cannot mutate content/structure.
- Studio/Agent share visual node cards and inspector content.
- Console has no new application errors.

- [ ] **Step 9: Commit Phase 6**

```bash
git add apps/web package.json pnpm-lock.yaml packages docs AGENTS.md CLAUDE.md
git commit -m "refactor: remove tldraw canvas runtime"
```

**Phase 6 Done When:**
- Runtime tldraw dependencies and code are gone.
- Current docs describe React Flow.
- Full verification and browser E2E pass.

---

## Final Acceptance Gate

Before opening a PR or calling the migration complete, run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
make server-test
make server-build
rg -n "from \"tldraw\"|from 'tldraw'|@tldraw|TLRecord|TLShape|ShapeUtil" apps/web packages
rg -n "tldraw" AGENTS.md CLAUDE.md docs/README.md docs/engineering docs/design
git diff --check
```

Expected:
- all test/build commands pass;
- grep commands have no runtime/current-doc hits;
- browser E2E for Studio and Agent passes;
- no untracked `.superpowers/`, `.playwright-mcp/`, Vite cache, or local runtime artifacts are staged.

## Execution Recommendation

Use subagent-driven execution by phase:

1. Phase 0 and Phase 1 can be one subagent because they are low-risk foundation work.
2. Phase 2 建议单独执行，因为 shared surface 边界需要 review。
3. Phase 3 建议单独执行，因为 Agent 权限会同时触达前端和后端 guard。
4. Phase 4 建议单独执行，因为 Studio route swap 风险最高。
5. Phase 5 建议单独执行，因为 edge/group/upload/delete parity 覆盖很多交互路径。
6. Phase 6 建议单独执行，因为 cleanup 容易误删或漏改当前文档。

每个 phase 单独提交，并在进入下一阶段前停下来 review。
