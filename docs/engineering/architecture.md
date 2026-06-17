# ClipAnvil 影砧 — 架构文档

## 产品定位

面向中小电商商家的营销视频生成平台，提供两种模式：

- **Studio 模式**：用户主导的自由创作空间，在无限画布上创建媒体节点、建立依赖连线、提交生成
- **Agent 模式**：Agent 主导的自动化生产，画布只读，用户通过对话驱动修改。Agent 调用生产级工具（create_storyboard、generate_shot），系统自动翻译为画布状态

两种模式共享同一套业务数据层（PostgreSQL）和画布投影层（tldraw）。详见 [整体业务交互设计](../design/overview.md)。

## 技术选型

### 前端 `apps/web`

| 层 | 选型 |
|---|---|
| 构建 | Vite 8 |
| 框架 | React 19 + TypeScript 6 |
| 画布 | tldraw v5 + 自定义 Shape/Tool |
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
| 沙箱 | 规划中：OpenSandbox Go SDK（M3 引入） |
| Agent 编排 | 规划中：eino (cloudwego/eino + eino-ext) |
| 配置 | viper |
| 日志 | slog（标准库） |
| 鉴权 | JWT（注册/登录/token 签发校验） |
| AI 服务 | 阿里 DashScope（LLM + 文生图/图生视频） |

### 基础设施

| 组件 | 用途 |
|---|---|
| PostgreSQL 16 | 业务数据持久化 |
| Redis 7 | 缓存、会话 |
| MinIO | 对象存储（图片、视频、中间产物） |
| Nginx | 反向代理、静态托管（prod）、WS 升级 |
| OpenSandbox | 沙箱执行 ffmpeg 等（M3 引入） |

## 模块边界

### 当前实现快照（M1.x）

截至 2026-06-17，当前代码已落地 Studio M1.x 的核心 DAG 编辑能力：

- 前端：登录/注册页、项目列表/创建弹窗、Studio 画布页、可折叠左侧资源树、明暗外观切换、tldraw 自定义 `MediaShape` 和 `GroupContainerShape`、文本/图片/视频/音频节点、节点编辑面板、右侧属性面板、文件拖拽上传、依赖连线、分组、Dagre 自动布局、`/ws/canvas` 连接状态与重连。
- 后端：JWT 鉴权、Workspace API、Canvas API、MediaNode API、MediaEdge API、MediaGroup API、Upload API、Canvas WebSocket Hub、goose 迁移、sqlc 查询生成、MinIO 上传与预签名访问 URL。
- 数据库：`account`、`workspace`、`canvas_document`、`media_node`、`media_edge`、`media_group`、`media_asset`。
- 尚未落地：Agent 模式、生成任务、产物版本、自动评审、`/ws/chat`、reference/sequence 的完整前端配置、模型供应商调用。

### `packages/canvas-schema`（核心契约层）

Studio 和 Agent 模式共享的画布定义：

- 当前 M1.x：媒体节点类型枚举、节点状态枚举、`MediaShape` props（含 `thumbnailUrl`）、`GroupContainerShape` props。
- tldraw ArrowShape 使用内置 arrow + binding 表达 `media_edge`，边数据仍以业务表为事实源。
- 后续 M2/M3：进度、评审、版本、Agent 事件等字段。

### 后端模块

```
apps/server/internal/
├── api/            REST handler + /ws/canvas WebSocket
├── auth/           注册/登录 + JWT 签发/校验/中间件
├── config/         viper 配置加载
├── store/          sqlc 生成 + 仓储层
├── agent/          规划中：MultiAgent 编排（eino: Producer → Sub-Agents）
├── sandbox/        规划中：OpenSandbox Go SDK 封装（M3 引入）
├── media/          规划中：对象存储/资产管理
└── dashscope/      规划中：DashScope API 封装（LLM / 文生图 / 图生视频）
```

## 数据流

### Studio 模式（目标态）

```
用户操作画布（创建节点 / 连线 / 编辑 Prompt / 提交生成）
  → REST API（POST /api/nodes, POST /api/edges, POST /api/generation 等）
  → command 模块执行业务命令 → 写 PostgreSQL
  → 广播 /ws/canvas 事件 → 前端更新 tldraw store
  → 生成任务异步执行（目标态）→ DashScope API → 结果写回 DB
  → 中间产物存 MinIO，元数据存 Postgres
  → /ws/canvas 推送进度和完成状态（生成阶段目标态）
```

> 当前 Studio 数据流是 REST + `/ws/canvas`：用户操作先经 HTTP API 写库，后端广播节点、连线、分组事件；前端以 React Query 数据和 tldraw store 做投影同步。Camera 和批量坐标更新仍走 HTTP。

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

- 业务 DB 为唯一事实源，tldraw 只做投影（不存 tldraw snapshot）
- Agent 调用生产级工具，不直接操作画布；生产翻译层自动将生产操作映射为业务命令
- Studio 和 Agent 共用同一套业务命令层（command 模块），数据一致性有保障
- eino 只负责推理 + 工具选择 + 状态机，业务能力不耦合到框架
- 代码层无状态，未来可平滑切到多副本/云端
- 本地优先，单机 docker Compose 一键启动

## 迭代里程碑

| 里程碑 | 范围 | 关键交付 |
|---|---|---|
| M0 基建 | Monorepo 骨架 + compose + 前后端 hello world | ✅ 已完成 |
| M1 Studio 画布基础 | 注册登录 + Workspace + 文本节点画布 + 坐标持久化 | 用户可创建项目、创建文本节点、拖拽后刷新保持位置 |
| M1.x Studio 增量 | image/video/audio 节点 + 连线 + 分组 + 资源树 + 属性面板 + WebSocket + 上传 + 自动布局 | ✅ 已完成核心 DAG 编辑能力；生成/版本仍后续 |
| M2 Agent 对话 | 对话面板 + 单 Agent + 生产级工具 + PSS + 画布只读 + Gate | 用户可通过对话让 Agent 创建节点和生成 |
| M3 MultiAgent + Skill | Producer + 5 Sub-Agent + 内置 Skill + 评审重试 + Stale 传播 | Agent 自动完成需求到成片全流程 |
| M4 一致性与质量 | 跨镜头一致性 + 多 Skill + 成本管理 + 审计 + 模式切换 | 生成视频可用性和可控性提升 |

详见 [整体业务交互设计 — 实施路线](../design/overview.md)。
