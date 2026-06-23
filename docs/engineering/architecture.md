# ClipAnvil 影砧 — 架构文档

## 产品定位

面向中小电商商家的营销视频生成平台，提供两种模式：

- **Studio 模式**：用户主导的自由创作空间，在无限画布上创建媒体节点、建立依赖连线、提交生成
- **Agent 模式**：Agent 主导的自动化生产，画布只读，用户通过对话驱动修改。Agent 调用生产级工具（create_storyboard、generate_shot），系统自动翻译为画布状态

两种模式共享同一套业务数据层（PostgreSQL）和画布投影层（React Flow）。详见 [整体业务交互设计](../design/overview.md)。

## 技术选型

### 前端 `apps/web`

| 层 | 选型 |
|---|---|
| 构建 | Vite 8 |
| 框架 | React 19 + TypeScript 6 |
| 画布 | `@xyflow/react` 12 + 共享 CanvasFlowSurface |
| 样式 | TailwindCSS 4 + 自定义设计系统 |
| 状态 | Zustand（画布外 UI 状态） |
| 数据 | TanStack Query + Fetch |
| WebSocket | 原生 WebSocket + 轻量封装；当前已启用 `/ws/canvas` |
| 路由 | React Router v7 |

### 后端 `apps/server`

| 层 | 选型 |
|---|---|
| 语言 | Go 1.26+ |
| Web 框架 | Hertz (cloudwego/hertz) |
| DB | pgx v5 + sqlc |
| 迁移 | goose |
| 缓存 | go-redis v9 |
| 对象存储 | minio-go v7 |
| 沙箱 | OpenSandbox Go SDK；workspace 级沙箱绑定、执行和产物提交基础已接入 |
| Agent 编排 | 规划中：eino (cloudwego/eino + eino-ext) |
| 配置 | viper |
| 日志 | slog（标准库） |
| 鉴权 | JWT（注册/登录/token 签发校验） |
| AI 服务 | Mock provider（本地测试）+ Volcengine/Doubao 文本、图片、视频生成；音频生成暂 hold |

### 基础设施

| 组件 | 用途 |
|---|---|
| PostgreSQL 16 | 业务数据持久化 |
| Redis 7 | 缓存、会话 |
| MinIO | 对象存储（图片、视频、中间产物） |
| Nginx | 反向代理、静态托管（prod）、WS 升级 |
| OpenSandbox | workspace 级长生命周期沙箱，执行 ffmpeg、远程媒体下载和不可预测资源消耗任务 |
| Volcengine TOS | 真实 provider 输入暂存，供 Volcengine 下载图片/视频参考资源 |

## 模块边界

### 当前实现快照（M5 已完成）

截至 2026-06-21，当前代码已落地 M3-M5 的核心 Studio 生产能力，并已接入真实 Volcengine provider：

- 前端：登录/注册页、项目列表/创建弹窗、Studio/Agent mode 路由分流、Studio/Agent 共享 React Flow 画布、可折叠左侧资源树、明暗外观切换、统一媒体节点卡片、文本/图片/视频/音频/参考包节点、用户源素材节点、依赖连线、分组、Dagre 自动布局、浮层 Inspector、Prompt `@`、Reference Pack 成员管理、手动运行、版本预览/选择、调用记录详情、全屏素材查看、`/ws/canvas` 连接状态与重连。
- 后端：JWT 鉴权、Workspace API、Canvas API、MediaNode API、MediaEdge API、MediaGroup API、Upload/Storage API、Canvas WebSocket Hub、Production API、Prompt Reference API、Reference Pack API、goose 迁移、sqlc 查询生成、MinIO 上传与预签名访问 URL、OpenSandbox workspace manager、sandbox exec、artifact submit、sandbox-backed remote asset ingest、mock provider、Volcengine provider adapter、TOS staging。
- 数据库：`account`、`workspace`、`canvas_document`、`media_node`、`media_edge`、`media_group`、`media_asset`、`workspace_sandbox`、`generation_job`、`artifact_version`、`model_provider`、`model_capability`、`node_stale_reason`、`reference_pack_item`、`sandbox_job`。
- 尚未落地：完整 Agent 对话生产模式、`/ws/chat`、Producer/Craftsman/Worker/Composer、自动评审与成片 Composer 闭环。

### `apps/web/src/components/canvas-flow`（画布契约层）

Studio 和 Agent 模式共享的 React Flow 画布定义：

- `flowTypes.ts` 定义 media/group node data、dependency edge data 和 mode。
- `flowModePolicy.ts` 定义 Studio/Agent 能力差异；Agent 可浏览、选择、拖动布局和查看信息，但不能创建、删除、连线、编辑或运行。
- `canvasViewModel.ts` 从 `CanvasPayload` 派生 React Flow nodes/edges，边和节点 id 直接使用业务 id。
- `CanvasFlowSurface.tsx` 是 Studio/Agent 共用宿主；`MediaFlowNode`、`GroupFlowNode`、`DependencyFlowEdge` 和 `NodeInspectorPopover` 提供统一视觉与交互。
- 版本列表、调用记录和模型参数不放入 React Flow node data，通过 selected node 的 Production API 按需加载。

### 后端模块

```
apps/server/internal/
├── api/            REST handler + /ws/canvas WebSocket
├── auth/           注册/登录 + JWT 签发/校验/中间件
├── config/         viper 配置加载
├── store/          sqlc 生成 + 仓储层
├── production/     GenerationIntent、模型能力校验、异步 runner、provider 适配、版本和 stale
├── promptrefs/     Prompt @ 引用解析、渲染和校验
├── agent/          规划中：MultiAgent 编排（eino: Producer → Sub-Agents）
├── sandbox/        OpenSandbox SDK 封装、workspace sandbox、exec、文件预置、artifact submit
├── storage/        MinIO 上传、预签名 URL、对象访问
└── media/          媒体资产和生成产物管理边界
```

## 数据流

### Studio 模式（当前）

```
用户操作画布（创建节点 / 连线 / 编辑 Prompt / 提交生成）
  → REST API（POST /api/nodes, POST /api/edges, POST /api/nodes/:id/run 等）
  → api / production 模块执行业务命令 → 写 PostgreSQL
  → 广播 /ws/canvas 事件 → 前端刷新/合并 React Flow nodes 和 edges
  → 生成任务异步执行
  → mock provider / Volcengine provider / sandbox-backed internal provider
  → 远程图片和视频通过 sandbox 下载并存 MinIO，元数据存 PostgreSQL
  → generation_job + artifact_version 更新状态、进度、provider request/response
  → /ws/canvas 推送节点状态和预览更新
```

Prompt `@` 渲染时，文本输入会内联进 `rendered_prompt`；图片/视频等媒体输入会以 `图1`、`图2` 等可读占位进入 prompt，同时作为 provider input refs 传入模型请求。上游节点默认读取其 `current_version_id` 指向的版本；用户源素材节点则读取自身 `asset_id` 或手动文本内容。

### Agent 模式（目标态）

```
用户对话 → /ws/chat
  → agent 模块（eino: Producer Agent → Sub-Agents）
  → Agent 调用生产级工具（create_storyboard / generate_shot / ...）
  → production 模块翻译为多个业务命令
  → command 模块执行 → 写 DB → 广播事件
  → /ws/chat 推送对话内容
  → /ws/canvas 推送画布变更（节点创建/状态更新/连线等）
  → Agent 不直接操作画布，画布为只读投影
```

详见 [画布设计 — 前后端数据通路](../design/canvas.md)。

## WebSocket 通道设计

| 通道 | 路径 | 用途 | 可靠性要求 |
|---|---|---|---|
| 画布通道 | `/ws/canvas` | 节点、连线、分组事件；后续承载节点执行状态 | 已落地，前端断线后重连并重新拉取画布 |
| 对话通道 | `/ws/chat` | Agent 模式流式文本输出、工具调用状态 | 目标态 |

前端通过 query param 携带 JWT token 认证 `/ws/canvas` 连接。后端当前尚未注册 `/ws/chat`。

## 设计原则

- 业务 DB 为唯一事实源，React Flow 只做投影（不存前端画布 snapshot）
- Agent 调用生产级工具，不直接操作画布；生产翻译层自动将生产操作映射为业务命令
- Studio 和 Agent 共用同一套业务命令层（command 模块），数据一致性有保障
- eino 只负责推理 + 工具选择 + 状态机，业务能力不耦合到框架
- 代码层无状态，未来可平滑切到多副本/云端
- 本地优先，单机 docker Compose 一键启动

## 迭代里程碑

当前里程碑以 `docs/milestones/` 为准；下表保留工程视角的当前状态摘要。

| 里程碑 | 范围 | 关键交付 |
|---|---|---|
| M0 基建 | Monorepo 骨架 + compose + 前后端 hello world | ✅ 已完成 |
| M1 Studio 画布基础 | 注册登录 + Workspace + 文本节点画布 + 坐标持久化 | 用户可创建项目、创建文本节点、拖拽后刷新保持位置 |
| M1.x Studio 增量 | image/video/audio 节点 + 连线 + 分组 + 资源树 + 属性面板 + WebSocket + 上传 + 自动布局 | ✅ 已完成核心 DAG 编辑能力 |
| M2 OpenSandbox 工作区沙箱 | OpenSandbox Server + workspace_sandbox + sandbox exec + MinIO 传输 + artifact submit | ✅ 基础链路已部分落地；端到端 Agent 集成仍后续 |
| M3 Workspace 模式入口 | Studio / Agent Workspace 入口、路由分流和权限边界 | ✅ 已完成 |
| M4 共享生产底座 | GenerationIntent、Provider Bridge、Sandbox Job Service、版本、stale、失败重试、Production Read API | ✅ 已完成 |
| M5 Studio 专业手动模式 | 浮层 Inspector、Prompt `@`、Reference Pack、手动运行、版本/调用记录、真实 Volcengine 文本/图片/视频、源素材节点 | ✅ 已完成 |
| M6 Agent 自动生产模式 | Producer / Craftsman / Worker / Composer 复用 M4 生产底座完成自动生产 | 待实施 |

详见 [整体业务交互设计 — 实施路线](../design/overview.md)。
