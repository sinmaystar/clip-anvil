# ClipAnvil 画布设计和交互方案

## 1. 设计原则

**业务 DB 为唯一事实源，tldraw 只做投影。** 画布上的每一个元素都能从业务表直接构建。不存在 tldraw snapshot，不存在 canvas_record 表。

```
业务 DB（PostgreSQL）
  │
  ├── media_node (含画布坐标 x,y,w,h)  ──→  tldraw MediaShape
  ├── media_edge                        ──→  tldraw ArrowShape + Binding
  ├── media_group                       ──→  tldraw GroupShape
  └── canvas_document (camera)          ──→  tldraw Camera
```

**为什么不存 tldraw snapshot**：

| 方案 | 问题 |
|---|---|
| 全量 snapshot（JSONB 存所有 shape） | 每次拖拽全量写入几十 KB，且业务字段与业务表脱节 |
| canvas_record（逐条存 tldraw 记录） | 业务表的副本，两套数据必然不一致 |
| **业务表直接存坐标**（当前方案） | 只有一个事实源，不存在同步问题 |

## 2. tldraw 投影层类型

### 2.1 MediaShapeProps

```typescript
import type { TLBaseShape } from 'tldraw'

type MediaType = 'text' | 'image' | 'video' | 'audio'
type NodeStatus = 'draft' | 'ready' | 'queued' | 'running'
  | 'succeeded' | 'failed' | 'stale' | 'user_editing'

interface MediaShapeProps {
  nodeId: string
  nodeType: MediaType
  title: string
  status: NodeStatus
  thumbnailUrl?: string
  progress?: number
  reviewScore?: number
  isAgentCreated: boolean
}

type MediaShape = TLBaseShape<'media', MediaShapeProps>
```

这是 tldraw shape 需要的**最小渲染字段**。Prompt、模型参数、版本历史等通过 `nodeId` 从 API 按需加载，不放在 shape props 里。

### 2.2 从业务数据构建 shape

```typescript
function nodeToShape(node: MediaNodeDTO): MediaShape {
  return {
    id: createShapeId(node.id),
    type: 'media',
    x: node.canvasX,
    y: node.canvasY,
    props: {
      w: node.canvasW,
      h: node.canvasH,
      nodeId: node.id,
      nodeType: node.nodeType,
      title: node.title,
      status: node.status,
      thumbnailUrl: node.thumbnailUrl,
      isAgentCreated: node.source === 'agent',
    },
  }
}
```

## 3. 节点卡片视觉规格

### 3.1 四种媒体节点

| nodeType | 图标 | 内容区渲染 | 默认尺寸 |
|---|---|---|---|
| `text` | 📝 | 文本摘要预览（前 3 行） | 200×120 |
| `image` | 🖼 | 缩略图填充 | 200×160 |
| `video` | 🎬 | 缩略图 + 时长标签 + ▶ 播放按钮 | 240×180 |
| `audio` | 🔊 | 波形图预览 + 时长标签 | 200×80 |

### 3.2 卡片结构

```
┌─────────────────────────┐
│ [图标] 标题       [状态] │  ← 头部 32px：类型图标 + 可编辑标题 + 状态色块
├─────────────────────────┤
│                         │
│    [内容预览区域]        │  ← 内容区：按 nodeType 渲染
│                         │
├─────────────────────────┤
│ 输入: @素材A @素材B      │  ← 引用条 20px：上游依赖的缩略 chip
├─────────────────────────┤
│ [Prompt]    [模型▾][▶]  │  ← 操作栏 32px：Prompt 单行预览 + 模型 + 生成
└─────────────────────────┘
● 输入端口（左中）    输出端口（右中） ●
```

### 3.3 状态边框样式

| 状态 | 边框 | 额外视觉 |
|---|---|---|
| Draft | 1px 灰色虚线 | 内容区显示占位提示 |
| Ready | 1px 灰色实线 | — |
| Queued | 1px 灰色实线 | 右上角 ⏳ 图标 |
| Running | 2px 蓝色实线 | 外发光 + 底部进度条 |
| Succeeded | 2px 绿色实线 | 左下角评分徽标 |
| Failed | 2px 红色实线 | 右上角 ✗ + 失败原因 tooltip |
| Stale | 2px 黄色虚线 | 右上角 ⚠ + 黄色半透明遮罩 |
| UserEditing | 2px 橙色实线 | 右上角 ✎ + "编辑中" 标签 |

### 3.4 创建来源视觉区分

- 用户创建：实线边框
- Agent 创建：虚线边框 + 蓝色色调

两者交互能力完全一致，视觉区分仅用于信任透明。

## 4. 连线视觉规格

### 4.1 三种连线类型

| edgeType | 线型 | 颜色 | 箭头 | 标签 |
|---|---|---|---|---|
| `dependency` | 2px 实线 | `#3b82f6` 蓝 | 实心三角 | 无 |
| `reference` | 2px 虚线 | `#a855f7` 紫 | 空心三角 | "参考" |
| `sequence` | 2px 实线 | `#22c55e` 绿 | 实心三角 | 转场类型 + 时长 |

### 4.2 端口规则

- 每个节点左侧中点为输入端口，右侧中点为输出端口
- 端口在鼠标悬停节点时显示为实心圆点（半径 4px）
- 拖拽端口时高亮所有合法目标端口
- 禁止自连接和环形依赖（前端校验 + 后端二次校验）

### 4.3 连线交互

- 点击连线 → 选中高亮（线宽变 3px）
- 选中 dependency/reference edge → 右侧属性面板显示连线详情
- 选中 sequence edge → 右侧属性面板显示转场配置（类型选择 + 时长滑块）
- sequence edge 的标签可点击，弹出转场快捷配置浮层

## 5. 分组视觉规格

使用 tldraw 的 GroupShapeUtil，视觉表现为虚线容器。

**展开态**：

```
┌ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┐
  📁 广告分镜 (4)    [▼折叠]     ← 标题栏：名称 + 成员计数 + 折叠按钮
├ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┤
│                              │
│  [节点A]  [节点B]  [节点C]    │  ← 内部节点自由布局
│           [节点D]             │
│                              │
└ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘
```

**折叠态**：

```
┌ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┐
  📁 广告分镜 (4)   [▶展开]
└ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘
```

**行为规则**：

- 分组是纯组织工具，不影响依赖关系和生成逻辑
- 依赖连线可自由跨越分组边界
- 拖拽节点进入分组容器 → 自动加入分组
- 拖拽节点移出分组容器 → 自动脱离分组
- 移动分组 → 内部节点跟随移动
- 删除分组 → 内部节点保留，仅解除分组关系
- 画布分组与左侧资源树文件夹双向同步

## 6. 自动布局

### 6.1 布局策略

Agent 模式下节点创建后必须自动布局。Studio 模式下用户可手动调整后调用"自动整理"。

**布局规则**：

```
┌─────────────────────────────────────────────────────┐
│ 📁 参考素材                                         │
│ [产品主图] [品牌Logo] [BGM] [参考图]                 │
├─────────────────────────────────────────────────────┤
│                                                     │
│ 📁 广告分镜                                         │
│ [01特写] →→ [02场景] →→ [03卖点] →→ [04情感] →→ [05品牌] │
│                                                     │
├─────────────────────────────────────────────────────┤
│ [📼 成片 30s]                                       │
└─────────────────────────────────────────────────────┘
```

**算法**：
1. 同一 group 的节点排列在一起
2. 有 sequence 连线的节点按顺序从左到右
3. 被依赖的节点（素材）在上方，依赖者（分镜）在下方
4. 成片节点在最下方
5. 节点间距：水平 40px，垂直 60px，分组间 80px

### 6.2 布局实现分工

**前端计算布局，后端存坐标**。

| 角色 | 职责 |
|---|---|
| 后端（生产翻译层） | 创建节点时写入简单初始坐标（分镜水平排开 `x = i * (cardWidth + gap)`，素材在上方，成片在下方）。这不是布局算法，是十行算术 |
| 前端（tldraw） | 用户点击"自动整理"时运行完整布局算法（dagre/elkjs），计算最优坐标，写回后端 |
| 前端 → 后端 | 坐标写回通过 `PATCH /api/nodes/batch-position` 防抖 2s |

后端不需要 dagre/elkjs 等布局库。初始坐标让节点不堆在 (0,0) 即可，真正的布局是前端的事。

### 6.3 增量布局

Agent 添加新节点时不全局重排（避免打乱用户已调整过的位置）：
- 新节点插入到合理位置（时间线末尾、分组内部）
- 已有节点位置不变
- 全局自动整理只在用户主动调用时执行

## 7. 前后端数据通路

### 7.1 三条通路

```
                ┌────────────────────────────────────┐
                │         tldraw（浏览器内存）         │
                │  内存 store → 即时渲染，零延迟       │
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

### 7.2 通路 ①：初始加载

页面打开时一次性拉取 workspace 全部画布数据：

```
GET /api/workspaces/:id/canvas

响应：
{
  camera: { x, y, zoom },
  nodes: [ { id, nodeType, title, status, ..., canvasX, canvasY, canvasW, canvasH } ],
  edges: [ { id, fromNodeId, toNodeId, edgeType, transitionType, ... } ],
  groups: [ { id, name, nodeIds: [...] } ]
}
```

前端收到后：
1. `editor.createShapes(nodes.map(nodeToShape))` — 批量创建节点 shape
2. 为每条 edge 创建 ArrowShape + Binding
3. 为每个 group 创建 GroupShape
4. `editor.setCamera({ x, y, z: zoom })` — 恢复视口

加载完成后 tldraw 在内存中独立工作，后续交互不依赖后端返回才能渲染。

### 7.3 通路 ②：用户操作 → 异步持久化

**位置类操作（拖拽、缩放、移动视口）— 容忍丢失**：

```
用户拖拽节点
  → tldraw 内存立即更新（0ms）→ 画布立即响应
  → 防抖 2s → PATCH /api/nodes/batch-position [{ id, canvasX, canvasY }, ...]
  → 失败？静默重试一次，仍失败忽略
  → 最坏结果：刷新后位置回退
```

**业务操作（创建节点、建连线、编辑 Prompt、提交生成）— 必须成功**：

```
用户点击"创建视频节点"
  → POST /api/nodes { nodeType: 'video', canvasX: 100, canvasY: 200 }
  → 等后端返回（~200ms）
  → 成功 → editor.createShape(nodeToShape(response))
  → 失败 → toast 提示"创建失败，请重试"
```

**为什么业务操作不做乐观更新**：200ms 延迟在创建节点、建连线这类操作中几乎无感知。乐观更新需要回滚逻辑（删除已渲染的 shape），增加复杂度但收益极低。只有高频交互（拖拽）才需要本地即时响应。

### 7.4 通路 ③：后端事件 → WebSocket 推送

Agent 操作、生成任务状态变更等后端事件通过 WebSocket 推送到前端：

```
WebSocket /ws/canvas?workspaceId=xxx
```

| 事件 | Payload | 前端响应 |
|---|---|---|
| `NodeCreated` | node 完整数据 | `editor.createShape(nodeToShape(node))` |
| `NodeUpdated` | nodeId + changes | `editor.updateShape({ id, props: changes })` |
| `NodeDeleted` | nodeId | `editor.deleteShape(shapeId)` |
| `EdgeCreated` | edge 完整数据 | 创建 ArrowShape + Binding |
| `EdgeDeleted` | edgeId | 删除 ArrowShape |
| `JobProgress` | nodeId, progress | 更新节点进度条 |
| `JobCompleted` | nodeId, status, thumbnailUrl, reviewScore | 更新状态 + 缩略图 + 评分 |
| `JobFailed` | nodeId, errorMessage | 更新状态为 failed |
| `GateRequested` | gateType, message, options | 对话面板展示确认卡片 |

### 7.5 冲突处理

当 Agent 推送 `NodeUpdated` 与用户当前正在编辑的节点冲突（仅 Studio 模式）：

1. 前端检查该节点是否处于本地编辑状态
2. 如果是 → 忽略 Agent 的 props 更新，保留用户本地内容
3. 用户保存时以用户版本为准（用户操作优先级高于 Agent）

Agent 模式下无此问题——画布只读，无并发编辑。

## 相关文档

- [整体设计](design-overview.md) — 架构、原则、路线图
- [Studio 模式](design-studio-mode.md) — 用户主导的创作交互
- [Agent 模式](design-agent-mode.md) — Agent 驱动的生产交互
- [数据库设计](database-design.md) — 完整 schema 和数据通路
