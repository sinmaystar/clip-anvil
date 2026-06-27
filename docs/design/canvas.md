# ClipAnvil 画布设计和交互方案

## 1. 设计原则

**业务 DB 为唯一事实源，React Flow 只做可视化投影和交互层。** 画布上的节点、连线、分组和视口都能从业务表直接构建。不存前端画布 snapshot，也不维护独立的 `canvas_record` 表。

```
业务 DB（PostgreSQL）
  │
  ├── media_node (含画布坐标 x,y,w,h)  ──→  React Flow media node
  ├── media_edge                        ──→  React Flow dependency edge
  ├── media_group                       ──→  React Flow group node
  ├── canvas_document (camera)          ──→  React Flow viewport
  └── Agent 领域表 / production 表       ──→  Agent Workbench scene/shot/artifact 投影
```

| 方案 | 问题 |
|---|---|
| 全量 snapshot（JSONB 存所有前端节点） | 每次拖拽全量写入几十 KB，且业务字段与业务表脱节 |
| canvas_record（逐条存前端画布记录） | 业务表的副本，两套数据必然不一致 |
| **业务表直接存坐标**（当前方案） | 只有一个事实源，React Flow view-model 从业务表即时派生 |

## 2. React Flow 投影层

当前画布由 `apps/web/src/components/canvas-flow/` 承担：

| 文件 | 职责 |
|---|---|
| `flowTypes.ts` | 定义 `CanvasFlowNode`、`CanvasFlowEdge`、Studio/Agent mode |
| `flowModePolicy.ts` | 定义 Studio/Agent capability policy |
| `canvasViewModel.ts` | 从 `CanvasPayload` 派生 React Flow nodes/edges |
| `canvasViewport.ts` | 后端 camera 与 React Flow viewport 互转 |
| `CanvasFlowSurface.tsx` | Studio/Agent 共用画布宿主 |
| `MediaFlowNode.tsx` | 统一媒体节点卡片 |
| `GroupFlowNode.tsx` | 分组容器节点 |
| `DependencyFlowEdge.tsx` | dependency edge 渲染 |
| `NodeInspectorPopover.tsx` | 统一节点信息浮层 |

核心类型示意：

```typescript
type CanvasFlowMode = "studio" | "agent"

interface CanvasFlowNodeData {
  kind: "media"
  node: MediaNode
}

interface CanvasFlowGroupData {
  kind: "group"
  group: MediaGroup
  nodeIds: string[]
}

interface CanvasFlowEdgeData {
  edge: MediaEdge
}
```

React Flow node data 只保存画布渲染需要的业务投影。模型参数、版本列表、调用记录等重数据通过选中节点后的 API 按需加载。

## 2.1 Agent Workbench 投影层

Agent 模式当前不再只复用 Studio 的媒体节点平铺画布。`apps/web/src/components/agent-workbench/` 和 `apps/web/src/lib/agentWorkbench*.ts` 提供专门的制作工作台投影：

| 文件 | 职责 |
|---|---|
| `agentWorkbench.ts` | 定义 Agent 工作台投影数据：overview、scene、shot、artifact slot、review、issue |
| `agentWorkbenchViewModel.ts` | 把后端投影数据转换成 React Flow overview / scene / shot nodes 和 edges |
| `agentWorkbenchMediaLayout.ts` | 根据真实图片/视频尺寸计算媒体缩略图大小，支持竖图、横图和多产物 |
| `AgentWorkbenchCanvas.tsx` | Agent Workbench React Flow 宿主，支持选择 overview / scene / shot / artifact |
| `AgentSceneGroupNode.tsx` | 场景分组容器节点 |
| `AgentShotNode.tsx` | 分镜卡片，展示 shot 文案、预览图、视频、状态和问题入口 |
| `AgentCanvasDetailPanel.tsx` | 选中对象后的详情面板 |

Workbench 的事实源不是前端布局 snapshot，而是后端从 `creative_brief`、`project_memory`、`key_element`、`scene`、`shot`、`render_plan`、`media_node`、`artifact_version`、`review_record`、`artifact_issue` 等表构建的生产投影。

当前布局策略：

- overview 节点展示项目级 Brief / Memory 摘要。
- scene 节点作为分组容器，内部包含多个 shot。
- shot 使用两列瀑布流式自动布局，避免单个带视频的 shot 把整行其它 shot 拉高。
- shot 内按需展示 preview image、shot video 和 review/issue，不强行保留空槽位。
- 图片和视频按原始比例自适应尺寸，点击 artifact 可打开详情面板。

Studio 画布和 Agent Workbench 的关系：

| 维度 | Studio Canvas | Agent Workbench |
|---|---|---|
| 主要对象 | `media_node`、`media_edge`、`media_group` | scene、shot、artifact、review、issue、overview |
| 用户操作 | 创建、编辑、连线、运行、选择版本 | 浏览、选择、查看详情、通过对话要求 Producer 修改 |
| 事实源 | Studio 业务表和 production 表 | Agent 领域表 + production 表 |
| 布局 | 用户拖拽 + Dagre 自动整理 | 后端/前端根据 scene/shot 层级自动投影 |
| 目标 | 手动专业编辑器 | 让用户看见 Agent 制作过程和分镜状态 |

## 3. 节点卡片视觉规格

| nodeType | 内容区渲染 | 默认尺寸 |
|---|---|---|
| `text` | Markdown 文本摘要 | 200x120 |
| `image` | 图片预览，保持完整可见 | 200x160 |
| `video` | 可播放预览或封面 | 240x180 |
| `audio` | 音频占位和素材状态 | 200x80 |
| `reference_pack` | 成员数量和摘要 | 220x140 |

节点结构：

```
┌─────────────────────────┐
│ 类型标签 标题      状态  │
├─────────────────────────┤
│                         │
│       内容预览区域       │
│                         │
└─────────────────────────┘
                  +        ← Studio 下 hover / selected 时显示连接按钮
```

视觉规则：

- 节点本体使用小圆角，接近方形。
- 文本节点展示 Markdown 摘要。
- 图片和视频使用 `.media-node-media-frame`，不裁剪关键内容。
- 用户源素材节点使用素材身份标签，不展示模型运行入口。
- Stale、Running、Failed 等状态通过边线和 badge 表达，不使用大面积发光。

## 4. 连线视觉规格

当前 Studio 只暴露 dependency 连线：

| 关系 | 线型 | 颜色 | 箭头 | 标签 |
|---|---|---|---|---|
| dependency | React Flow custom edge + 轻量流动样式 | `--accent` | 实心三角 | 无 |

交互规则：

- Studio 中节点右侧显示 source handle，可拖拽创建 dependency。
- Agent 中 `canCreateEdges=false`，节点不可连线。
- 禁止自连接和环形依赖，前端校验后端二次校验。
- 选中 edge 后可通过显式删除逻辑删除，刷新后以 DB 为准。

## 5. 分组视觉规格

分组使用 React Flow group node 渲染，视觉表现为虚线容器。

```
┌ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┐
  广告分镜 (4)
│  [节点A]  [节点B]  │
│  [节点C]  [节点D]  │
└ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘
```

行为规则：

- 分组是纯组织工具，不影响依赖关系和生成逻辑。
- 依赖连线可自由跨越分组边界。
- Studio 支持创建分组、拖动节点进入/移出分组、拖动分组批量移动成员、删除分组保留成员。
- Agent 可查看分组并拖动布局，但不能创建、删除或编辑分组。
- 画布分组与左侧资源树文件夹双向同步。

## 6. 自动布局

自动整理使用 `computeDagreLayout`，前端计算布局，后端保存坐标。

| 角色 | 职责 |
|---|---|
| 后端 | 创建节点时写入初始坐标；保存用户拖拽和自动布局后的坐标 |
| 前端 | 用户点击“自动整理”时运行 Dagre，计算节点坐标和分组边界 |
| 前端 → 后端 | 通过 `PATCH /api/nodes/batch-position` 写回坐标 |

Studio 自动整理会考虑左侧浮层安全区。安全起点从屏幕坐标通过 React Flow `screenToFlowPosition` 转成画布坐标。

## 7. 前后端数据通路

```
                ┌────────────────────────────────────┐
                │       React Flow（浏览器内存）       │
                │  nodes/edges/viewport → 即时渲染     │
                └──────┬────────────────┬─────────────┘
                       │                │
          ② 异步持久化   │                │  ③ 事件推送
          （前端→后端）  │                │ （后端→前端）
                       ▼                │
                ┌──────────────┐        │
  ① 初始加载     │   后端 API    │        │
  （一次性）────→│  + 业务 DB   │────────┘
                └──────────────┘
                                WebSocket 事件流
```

### 7.1 初始加载

页面打开时拉取 workspace 全部画布数据：

```
GET /api/workspaces/:id/canvas

响应：
{
  camera: { x, y, zoom },
  nodes: [ { id, node_type, title, status, canvas_x, canvas_y, canvas_w, canvas_h } ],
  edges: [ { id, from_node_id, to_node_id, edge_type } ],
  groups: [ { id, name, node_ids } ]
}
```

前端收到后：

1. `canvasToFlowNodes(canvas)` 派生 media/group nodes。
2. `canvasToFlowEdges(canvas)` 派生 dependency edges。
3. `cameraToViewport(camera)` 恢复 viewport。
4. `CanvasFlowSurface` 根据 mode policy 启用或禁用编辑能力。

### 7.2 用户操作持久化

位置类操作：

```
用户拖拽节点
  → React Flow 内存 nodes 立即更新
  → onNodeDragStop / onNodePositionsChange
  → PATCH /api/nodes/batch-position
  → 刷新后位置以 DB 为准
```

视口操作：

```
用户缩放/平移
  → React Flow viewport 更新
  → onMoveEnd
  → PATCH /api/workspaces/:id/camera
```

业务操作：

```
用户创建节点 / 建连线 / 删除 / 运行
  → REST API
  → 后端写 PostgreSQL
  → /ws/canvas 广播
  → 前端重新合并 canvas payload
```

创建节点、建连线、提交生成等业务操作以后端成功为准；标题、Prompt 和 Inspector 配置采用自动保存或显式 API 更新；运行状态和预览以后端 job/version 状态为准。

### 7.3 WebSocket 推送

当前画布变更和节点生产状态通过 `/ws/canvas?workspaceId=xxx` 推送。节点、连线、分组、节点状态和预览变更会广播；前端断线重连后重新拉取画布。

## 8. Studio / Agent Mode Policy

Studio 与 Agent 复用同一 `CanvasFlowSurface`。差异只来自 `flowModePolicy.ts`。

| 能力 | Studio | Agent |
|---|---|---|
| 平移缩放 | 允许 | 允许 |
| 选择节点/边/分组 | 允许 | 允许 |
| 拖动节点布局 | 允许 | 允许 |
| 持久化 viewport | 允许 | 允许 |
| 创建/删除节点 | 允许 | 禁止 |
| 创建/删除连线 | 允许 | 禁止 |
| 上传素材 | 允许 | 禁止 |
| 编辑标题/Prompt/参数 | 允许 | 禁止 |
| 运行节点 | 允许 | 禁止 |
| 编辑分组 | 允许 | 禁止 |

这保证 Agent 模式不是简化版画布，而是同一画布能力的只读生产视图：可浏览、可拖布局、可查看完整信息，但不能改变内容结构或执行生产。
