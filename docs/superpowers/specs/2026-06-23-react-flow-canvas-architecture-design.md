# React Flow 画布架构设计

## 背景

ClipAnvil 的 Studio 和 Agent 共用 React Flow 画布能力。画布负责媒体节点、依赖边、分组、文件素材、生产预览、Reference Pack、节点详情与 Agent 只读查看体验；业务事实源仍然是后端数据库和生产服务，React Flow 只承担前端渲染和交互编排。

Studio 和 Agent 不维护两套画布实现。两种模式共享同一套 view-model、节点组件、边组件、分组组件、详情弹层和基础交互，差异通过 mode policy 控制。

## 目标

1. Studio 主画布支持加载、创建、拖动、连接、删除、分组、自动布局、文件上传、节点运行、版本预览和参数编辑。
2. Agent 画布复用 Studio 的画布能力，允许平移缩放、节点拖动布局、节点选择、同款信息面板和详情查看；禁止内容编辑、生产执行、连线、删除、上传和结构性修改。
3. React Flow 层不保存独立业务状态；所有节点、边、分组、资产、运行状态和版本信息都从业务 API 派生。
4. 节点详情、依赖展示、模型参数、版本信息和素材预览在 Studio/Agent 间保持同构，保证两种模式无损切换。

## 非目标

1. 不实现通用画布 undo/redo。
2. 不把 React Flow 节点/边状态作为业务事实源。
3. 不让 Agent 直接调用 Studio 面向用户的运行接口；Agent 生产能力继续走 shared production / Agent runtime。
4. 不为本地开发历史数据保留兼容层；必要时允许清空本地数据重新生成。

## 设计原则

业务模型优先于画布模型。`media_node`、`media_edge`、`media_group`、`canvas_document`、`artifact_version`、Reference Pack 和生产任务由后端定义，前端把这些数据投影为 React Flow nodes、edges 和 viewport。

画布交互必须显式调用业务 mutation。创建节点、创建边、删除节点、删除边、拖动持久化、分组变更、文件上传和运行节点都通过 API 完成，再由 TanStack Query 和 WebSocket 事件协调前端状态。

Studio/Agent 的差异只出现在权限策略。React Flow surface、节点卡片、边渲染、分组渲染、详情弹层、依赖摘要和只读状态都应复用同一套组件。

## 模块边界

React Flow 画布模块位于 `apps/web/src/components/canvas-flow/`：

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
  NodeComposerPopover.tsx
  NodeDetailsDialog.tsx
  flowModePolicy.ts
  flowTypes.ts
```

`canvasViewModel.ts` 从 `CanvasPayload` 派生 React Flow nodes/edges。节点 data 使用业务字段，不暴露运行时内部状态语义。

`CanvasFlowSurface.tsx` 承载共享 React Flow 实例，负责节点、边、viewport、selection、拖动、右键菜单、文件 drop、连线命中、节点浮层定位、主题和辅助控件。

`StudioFlowCanvas.tsx` 提供 Studio policy 和 Studio mutation，包括创建、连接、删除、上传、运行、编辑、分组和自动布局。

`AgentFlowCanvas.tsx` 提供 Agent policy。Agent 可以查看和调整画布布局，但不能修改节点内容、依赖结构、素材集合或生产状态。

`MediaFlowNode.tsx` 渲染统一节点卡片。节点标题使用 icon + name，媒体预览按原始比例自适应，状态通过低干扰样式表达，不渲染显式类型 tab。

`DependencyFlowEdge.tsx` 渲染 dependency edge。边的出度锚点在 source 右侧中点，入度锚点在 target 左侧中点；流动光效沿整条路径连续运动，选中和删除使用显式 edge state 与后端 mutation。

`NodeComposerPopover.tsx` 是节点单击后的轻量输入面板。面板放在节点下方，主体是 prompt，顶部展示依赖摘要，底部平铺 operation、model 和模型支持的关键参数。版本、stale reason、provider request/response 等低频信息放入更多入口。

`NodeDetailsDialog.tsx` 承载低频详情，包括版本列表、stale reason、调用记录和调试信息。

`flowModePolicy.ts` 定义 mode capability matrix，组件不直接判断页面模式。

## 数据流

页面加载时，`fetchCanvas` 返回 viewport、nodes、edges、groups、assets 和生产摘要。React Flow view-model 派生：

1. media node：`id=node.id`，`type="media"`，`position={ x: canvas_x, y: canvas_y }`，`data` 保存节点渲染和交互需要的业务字段。
2. group node：`id=group.id`，`type="group"`，位置和尺寸从成员节点 bounds 推导，data 保存 group id、name、成员数量和布局状态。
3. dependency edge：`id=edge.id`，`source=from_node_id`，`target=to_node_id`，`type="dependency"`。

WebSocket 事件不直接写 React Flow internals。事件更新 Query cache 或触发 invalidate，然后由 view-model 重新派生 nodes/edges。这样 Studio 和 Agent 共享同一数据流，也能避免轮询或 WebSocket reconcile 时出现局部状态丢失。

图片、视频、音频等资产 URL 需要稳定化处理。接口刷新时如果业务 asset 没有变化，前端应复用稳定 URL 或稳定 cache key，避免 presigned URL 变化造成画布闪烁。

## 交互

节点拖动使用 React Flow `onNodesChange` 和 drag stop。拖动过程中更新本地 nodes；停止后批量持久化位置。Agent 模式允许布局拖动，视为画布浏览能力，不视为内容编辑。

连线使用 React Flow connection lifecycle。用户从 source handle 拖出线，拖到 target node 命中区域即可创建依赖；成功后调用 edge API，失败时回滚本地预览。

删除节点和删除边都走显式 mutation。选中态只影响当前 UI，不允许通过未确认的 React Flow selection state 推导业务删除。

分组保持 flat 业务语义。拖动分组时成员节点实时跟随；拖动成员节点时分组 bounds 实时重算；停止后批量持久化。删除分组只删除分组，不删除成员节点。

文件 drop 使用 `screenToFlowPosition` 计算落点。上传先创建 `media_asset`，再创建持久化素材节点。

viewport 使用 React Flow `{ x, y, zoom }`，映射到当前 canvas document 的 viewport/camera 字段，并节流持久化。

节点名称通过双击节点标题编辑。回车、失焦和点击画布都应提交；Escape 取消。

## 后端和数据库

默认保留现有业务模型：

1. `media_node.canvas_x/y/w/h` 记录画布布局。
2. `media_edge` 记录业务依赖。
3. `media_group` 和 `media_node.group_id` 记录分组关系。
4. `canvas_document` 记录 workspace viewport。
5. `artifact_version`、`node_stale_reason`、生产任务和 provider audit 记录节点产物与运行状态。

如果某些字段只服务于前端投影且会制造状态不同步，可以改为前端推导；本地历史数据可清理重建。

Agent workspace 权限需要区分布局写入和业务写入：允许 viewport 与节点位置更新；禁止创建/删除节点、创建/删除边、运行节点、修改 prompt/title/model/params、修改 Reference Pack、上传素材。

## 视觉和体验

画布背景支持亮色和暗色主题，React Flow controls、MiniMap、selection、edge、node card 和 popover 都必须随主题切换。

节点不渲染显式类型 tab。标题行使用 icon + name，状态用低干扰文字或标记表达。成功状态不使用高饱和粗边框。

媒体节点按原始比例自适应尺寸。横图保持横向，竖图保持竖向，外层 React Flow node 尺寸必须和内层预览尺寸同步，避免连线锚点与视觉边界脱节。

节点单击面板尺寸应紧凑，放在节点下方并保留间距。即使面板超出当前视窗，也保持节点下方定位，让用户通过画布滚动/平移查看完整内容。

底部参数栏只显示用户需要直接选择的控件。operation、model、温度、时长、比例、数量等关键参数平铺显示；低频调试信息放入更多入口。

## 验收

1. Studio 能加载节点、边、分组，并支持创建、拖动、连线、删除、上传、分组和自动布局。
2. Agent 能加载同一 workspace 的画布，支持平移缩放、节点拖动布局、选择和同款详情查看，并禁止编辑与结构修改。
3. 图片/视频节点尺寸与原始比例一致，连线锚点贴合视觉边界。
4. 轮询 canvas 或收到 WebSocket 事件时，未变化的媒体资源不闪烁。
5. 节点输入面板在亮色/暗色主题下都可读、可操作。
6. 删除边、创建边和 WebSocket reconcile 不会导致其他边临时消失。
7. 当前文档和代码命名使用 React Flow 终态口径。
