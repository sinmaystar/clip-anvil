# React Flow 画布迁移设计

## 背景

ClipAnvil 当前 Studio 主画布和 Agent 画布都建立在 tldraw 之上。随着 Studio 节点、依赖边、分组、生产预览、Reference Pack、文件上传、Agent 画布等能力增加，前端已经形成了大量围绕 tldraw shape、store listener、camera API、Arrow binding、ShapeUtil 的代码。接下来需要把前端画布底座全面迁移到 React Flow，并彻底废除 tldraw 运行时代码。

本设计采用“一次性切完”的策略：Studio 画布和 Agent 画布同时迁移。迁移不是在 tldraw 概念上加适配层，而是围绕 React Flow 的 `nodes`、`edges`、`viewport`、custom node、custom edge、interaction handler 重新组织前端画布层。

## 目标

1. 前端运行时代码不再 import 或使用 `tldraw`、`@tldraw/tlschema`。
2. Studio 主画布迁移到 React Flow，并保持现有核心能力：加载、创建、拖动、连接、删除、分组、自动布局、文件上传、Property Panel、生产运行和版本预览。
3. Agent 画布迁移到同一套 React Flow 画布能力。Agent 模式不是简化版只读画布，而是同一画布在 mode policy 下禁用内容编辑、生产执行、连线和结构变更，同时保留平移缩放、节点拖动布局、节点选择、同款信息面板和详情查看。
4. 当前有效文档口径从 tldraw 同步改为 React Flow；历史 archive 文档保留。
5. 允许清理本地历史数据，不为旧 tldraw 行为建立兼容层。

## 非目标

1. 不实现通用画布 undo/redo。迁移后业务操作继续以服务端 API 和 Query cache 为事实源。
2. 不迁移历史 archive/spec/plan 文档中的 tldraw 叙述。
3. 不因为换库而重做 Resource Tree、Property Panel、生产服务、Agent runtime、后端生产任务模型。
4. 不保留 tldraw shape/store 语义命名作为兼容接口。

## 设计原则

业务数据仍然是画布唯一事实源。React Flow 只是前端交互和渲染层，不保存独立业务状态。`media_node`、`media_edge`、`media_group`、workspace mode、生产任务、artifact version、Reference Pack 等业务模型继续由后端和数据库负责。

后端不是冻结边界。如果实现过程中发现某些字段、接口或流程只是为了 tldraw shape/store 行为做出的妥协，可以直接重构。当前代码扫描显示后端没有 tldraw snapshot、shape、binding、arrow 表；`canvas_x/y/w/h`、`media_edge`、`media_group`、`canvas_document.camera_*` 更像中立业务画布模型，默认保留。是否把 API/docs 中的 `camera` 口径改成 `viewport`，以及 `canvas_w/h` 是否继续由后端持久化，应在实现时按 React Flow 的实际收益决定。

## 架构边界

迁移后的前端画布层分为三部分：

1. 业务 API 层：继续使用 `/api/workspaces/:id/canvas`、node/edge/group/camera 相关 API、Canvas WebSocket 事件、TanStack Query cache。
2. React Flow view-model 层：把 `CanvasPayload` 派生为 React Flow `Node[]`、`Edge[]`、viewport、bounds 和 selection 数据。
3. React Flow 运行层：渲染 custom node/custom edge，处理拖动、连线、右键菜单、文件 drop、viewport 更新和 mode policy。

旧的 `nodeToShape`、`groupToShape`、`edgeToArrow`、ShapeUtil、TL shape type、store listener 将被删除或重写为 React Flow 语义。

## 模块拆分

新增 `apps/web/src/components/canvas-flow/`，作为 React Flow 画布模块。建议结构如下：

```text
components/canvas-flow/
  canvasViewModel.ts
  canvasViewport.ts
  CanvasFlowSurface.tsx
  StudioFlowCanvas.tsx
  AgentFlowCanvas.tsx
  MediaFlowNode.tsx
  GroupFlowNode.tsx
  DependencyFlowEdge.tsx
  NodeInspectorPopover.tsx
  flowModePolicy.ts
  flowTypes.ts
```

`canvasViewModel.ts` 负责把 `CanvasPayload` 转换成 React Flow nodes/edges。节点 data 使用业务字段，不再出现 `props`、`shape`、`TLRecord` 等 tldraw 概念。

`CanvasFlowSurface.tsx` 承载共享 React Flow 实例。Studio 和 Agent 都调用这个组件，并传入 mode policy。这个组件负责节点/边渲染、viewport、selection、拖动、右键、drop、edge hit area、节点信息浮层定位等通用能力。

`StudioFlowCanvas.tsx` 是 Studio mode host，提供允许创建、连接、删除、文件上传、运行节点、编辑节点内容、调整分组等 policy。

`AgentFlowCanvas.tsx` 是 Agent mode host，不复制画布能力，只提供 Agent policy。Agent 默认允许平移缩放、节点选择、节点拖动布局、查看与 Studio 同款信息面板和详情；禁止创建/删除节点、创建/删除边、运行节点、修改 prompt/title/model 参数、修改 Reference Pack 成员和上传素材。

`MediaFlowNode.tsx` 承载统一节点卡片。现有 `.media-node` 视觉可以保留，但组件不再继承 `ShapeUtil`，也不再依赖 `HTMLContainer`。节点卡片根据 mode policy 显示或隐藏连接 handle、运行入口、编辑入口等交互，而不是拆成两套不同节点。

`NodeInspectorPopover.tsx` 承载节点单击后的浮层。Studio 和 Agent 使用同一个组件、同一套样式和信息结构；差异只来自 action permission。Agent 可以查看生产状态、版本、调用记录、素材预览、Reference Pack 摘要等信息，但 action 按钮会被隐藏或 disabled。

`GroupFlowNode.tsx` 渲染 flat `media_group` 容器。默认不使用 React Flow parent-child 作为业务事实源，避免破坏“删除分组保留节点”的语义。实现时如果确认 React Flow parent extent 能显著简化且不会污染业务模型，可以局部采用，但数据库仍以 `group_id` 和 `node_ids` 为准。

`DependencyFlowEdge.tsx` 替代 tldraw arrow 和现有 `ConnectionOverlay` 的边渲染职责。动画路径、选中态和 hit area 放进 custom edge。

`flowModePolicy.ts` 定义模式能力矩阵，避免在组件中散落 `if (mode === "agent")`。React Flow 层按 policy 控制 `nodesDraggable`、`nodesConnectable`、pane context menu、delete shortcut、drop、run action、edit action、edge action 等。

## 数据流

页面加载时，`fetchCanvas` 返回 camera、nodes、edges、groups。React Flow view-model 根据业务数据生成：

1. media node：`id=node.id`，`type="media"`，`position={ x: canvas_x, y: canvas_y }`，`data` 保存节点渲染和交互需要的业务字段。
2. group node：`id=group.id`，`type="group"`，位置和尺寸从成员节点 bounds 推导，data 保存 group id/name/node count。
3. dependency edge：`id=edge.id`，`source=from_node_id`，`target=to_node_id`，type 为 custom dependency edge。

WebSocket 事件不直接操作 React Flow internals。事件先更新或 invalidates Query cache，React Flow nodes/edges 再由 view-model 重新派生。这样 Studio 和 Agent 画布共享同一份数据流。

Studio 和 Agent 不应有两套 data projection 或两套 inspector。Agent 画布可以配置为更少 mutation，但渲染、选择、节点详情和信息层级必须与 Studio 保持同构，这样未来用户在 Studio/Agent 之间切换时不会丢失上下文，也不会看到两个不同产品。

## 交互替换

节点拖动使用 React Flow `onNodesChange`、`onNodeDragStop` 或等价 handler。拖动过程中更新本地 nodes 和 Query cache，停止拖动后批量调用 `batchUpdateNodePositions`。失败时 invalidates canvas query。Agent 模式允许节点拖动布局；这属于画布布局能力，不等同于节点内容编辑。若后端当前 Agent workspace guard 禁止位置写入，需要细化权限，允许安全的 canvas layout mutation，同时继续禁止内容、生产和结构性写操作。

连线使用 React Flow handles 和 `onConnect`。创建成功后调用 `createMediaEdge`，更新 Query cache，并等待 WebSocket 或 query reconciliation。旧的 tldraw ArrowShape、binding、全局 DOM output-port click/drag 和 `ConnectionOverlay` 边渲染职责删除。

右键创建使用 pane context menu 和 `screenToFlowPosition`，将屏幕坐标转换为 flow 坐标后调用 `createMediaNode`。不再使用 tldraw `getShapeAtPoint`。

文件 drop 使用 window/pane drop 事件和 `screenToFlowPosition`。上传仍然先创建 `media_asset`，再创建持久化 media node。禁止引入本地临时 canvas asset。

分组保持 flat 语义。拖动 group node 时批量移动成员节点，并持久化成员 `canvas_x/y`。节点拖入 group bounds 时更新 `group_id`。删除 group 只删除分组，不删除成员节点。

删除和选择改为显式 UI state 与后端 mutation。删除节点/边调用对应 API，不再依赖 “store record removed -> 业务删除” 的隐式链路。

viewport 持久化使用 React Flow viewport `{ x, y, zoom }` 映射现有 camera payload。更新节流或 debounce 后调用现有 camera API；如果实现时重命名为 viewport API，需要同步 sqlc、handler、前端 client 和当前文档。

Agent 画布不再按“只读弱化版”实现。Agent policy 至少允许平移缩放、节点拖动布局、节点点击选择、信息面板查看和素材预览；禁止节点创建/删除、边创建/删除、分组结构修改、文件上传、生产运行和节点内容编辑。这样 Agent 和 Studio 可以共享同一画布体验，只在动作权限上不同。

## 后端和数据库

默认保留现有后端业务模型。允许在实现中做以下清理：

1. 如果 `camera` 命名继续造成 React Flow 语义混乱，可以改为 `viewport`，同时修改迁移、sqlc、handler、前端 API 类型和文档。
2. 如果 `canvas_w/h` 不再适合作为持久字段，可以改为前端 view-model 推导尺寸；本地历史数据可以清理重来。
3. 删除由 tldraw undo/store 行为引出的 client-created node restore flow。节点创建应走显式 API mutation。
4. 细化 Agent mode 写权限。当前 Agent 普通写 API 的 403 策略需要保留在内容和生产写操作上，但允许画布浏览体验所需的安全布局写入，例如 camera/viewport 和节点位置更新。
5. 若 schema 变更能明显简化 React Flow 语义，可新增迁移并执行 `make sqlc-generate`。不要求兼容旧本地数据。

## 依赖和代码清理

移除：

1. `apps/web` 的 `tldraw` 依赖。
2. `packages/canvas-schema` 的 `@tldraw/tlschema` 依赖。
3. 如果 `packages/canvas-schema` 只剩 tldraw shape 类型，则删除该包；若需要中立共享类型，则重命名或重写为非 tldraw schema。
4. `apps/web/src/shapes/*ShapeUtil.tsx`。
5. `vite.config.ts` 中的 `vendor-tldraw` 分包。
6. CSS 中 `.tl-*`、`.agent-readonly-tldraw` 等 tldraw 口径。

新增：

1. `@xyflow/react` 依赖。
2. React Flow CSS。按照 React Flow 官方建议，在全局 CSS 中于 Tailwind 之后引入 `@xyflow/react/dist/style.css` 或等价样式，避免 Tailwind 4 顺序问题。
3. React Flow view-model、node、edge、shared surface、Studio host、Agent host 和 mode policy。

## 当前文档更新

需要同步更新当前有效文档：

1. `AGENTS.md`、`CLAUDE.md` 的前端技术栈。
2. `docs/README.md` 当前能力描述。
3. `docs/engineering/architecture.md`、`docs/engineering/database.md`、`docs/engineering/development.md`。
4. `docs/design/overview.md`、`docs/design/canvas.md`、`docs/design/studio-mode.md`、`docs/design/agent-mode.md`、`docs/design/frontend.md`。
5. 活跃 `docs/superpowers/specs/` 和 `docs/superpowers/plans/` 中描述当前实现或近期计划的 tldraw 口径。

`docs/archive/` 保留历史叙述，不做批量改写。

## 测试和验收

前端验证：

1. `pnpm --filter @clip-anvil/web lint`
2. `pnpm --filter @clip-anvil/web... build`
3. 更新并运行 web node tests，至少覆盖 canvas view-model、mode policy、Agent/Studio 共享 surface、connection geometry/edge rendering、file drop、layering 和删除/选择行为。

后端验证：

1. 如果修改迁移或 sqlc queries，运行 `make sqlc-generate`。
2. 如果修改后端 handler/schema，运行 `make server-test` 和 `make server-build`。

仓库验证：

1. `rg -n "tldraw|@tldraw|TLRecord|TLShape|ShapeUtil" apps/web packages docs/README.md docs/engineering docs/design AGENTS.md CLAUDE.md` 不应出现当前 runtime 或当前文档口径残留。迁移 spec 自身和 archive 不纳入清零要求。
2. `git diff --check`

浏览器 smoke：

1. Studio 画布加载已有节点、边、分组。
2. 右键创建文本/图片/视频/音频/参考包节点。
3. 拖动节点后刷新页面，位置仍然正确。
4. 创建依赖边、选择边、删除边。
5. 删除节点后 Resource Tree、Property Panel、画布同步。
6. 文件 drop 创建素材节点。
7. 分组创建、成员移动、拖动分组批量移动成员、删除分组保留成员。
8. 自动布局后节点位置持久化。
9. Property Panel 运行节点、版本预览、全屏查看仍可用。
10. Agent 画布可加载、可平移缩放、可拖动节点布局、可点击节点打开与 Studio 同款信息面板；不可创建/删除节点，不可连线/删线，不可运行节点，不可编辑内容。

## 风险

React Flow 和 tldraw 的坐标/viewport 语义不同，`camera_x/y` 到 React Flow viewport 的映射需要用浏览器 smoke 验证。右键创建、文件 drop 和 viewport persistence 是最容易出现偏移的地方。

React Flow group node 和当前 flat `media_group` 语义不同。如果直接使用 React Flow parent-child，需要谨慎处理删除分组保留节点、拖动分组批量移动成员、资源树成员关系等行为。

旧代码的 tldraw store listener 同时承担创建恢复、拖动持久化、删除业务对象等职责。迁移时必须把这些隐式副作用拆成显式 mutation，否则容易漏掉删除或位置保存。

Agent 画布如果继续拆成弱化版只读实现，会延续现在“点击浮层内容和样式不一致”的问题。迁移后应避免复制一套 Agent 投影或 Agent inspector，必须复用 Studio 的 React Flow surface、node component 和 inspector，只通过 mode policy 禁用不允许的动作。

## 实施顺序建议

1. 建立 React Flow 依赖、CSS 和中立 view-model 类型。
2. 实现 shared media/group/edge React Flow 组件和纯函数测试。
3. 建立 shared `CanvasFlowSurface` 和 mode policy，先让 Studio/Agent 使用同一渲染和同一 inspector。
4. 替换 Agent 画布，验证平移缩放、节点拖动布局、节点选择和同款信息面板。
5. 替换 Studio 画布加载、selection、viewport 和右键创建。
6. 迁移拖动持久化、连线、删除、文件 drop、分组移动。
7. 删除 tldraw 依赖、ShapeUtil、canvas-schema 旧包和旧测试断言。
8. 更新当前文档，跑完整验证和浏览器 smoke。
