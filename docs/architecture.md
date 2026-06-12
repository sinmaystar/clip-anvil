# ClipAnvil 影砧 — 架构文档

## 产品定位

面向中小电商商家的营销视频生成平台，提供两种模式：

- **Studio 模式**：用户主导的自由创作空间，在无限画布上创建媒体节点、建立依赖连线、提交生成
- **Agent 模式**：Agent 主导的自动化生产，画布只读，用户通过对话驱动修改。Agent 调用生产级工具（create_storyboard、generate_shot），系统自动翻译为画布状态

两种模式共享同一套业务数据层（PostgreSQL）和画布投影层（tldraw）。详见 [整体业务交互设计](design-overview.md)。

## 技术选型

### 前端 `apps/web`

| 层 | 选型 |
|---|---|
| 构建 | Vite 8 |
| 框架 | React 19 + TypeScript 6 |
| 画布 | tldraw v5 + 自定义 Shape/Tool |
| 样式 | TailwindCSS 4 + shadcn/ui (Radix) |
| 状态 | Zustand（画布外 UI 状态） |
| 数据 | TanStack Query + Fetch |
| WebSocket | 原生 + 轻量封装，双通道 |
| 路由 | React Router v6 |

### 后端 `apps/server`

| 层 | 选型 |
|---|---|
| 语言 | Go 1.26+ |
| Web 框架 | Hertz (cloudwego/hertz) |
| DB | pgx v5 + sqlc |
| 迁移 | goose 或 golang-migrate |
| 缓存 | go-redis v9 |
| 对象存储 | minio-go v7 |
| 沙箱 | OpenSandbox Go SDK（M3 引入） |
| Agent 编排 | eino (cloudwego/eino + eino-ext) |
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

### `packages/canvas-schema`（核心契约层）

Studio 和 Agent 模式共享的画布定义：

- 媒体节点类型：text / image / video / audio
- MediaShape props 接口（nodeId、nodeType、status、thumbnailUrl 等）
- ArrowShape 三种连线类型（dependency / reference / sequence）
- 节点状态枚举（draft → ready → queued → running → succeeded / failed / stale）
- WebSocket 事件类型定义（NodeCreated / NodeUpdated / JobProgress 等）

### 后端模块

```
apps/server/internal/
├── api/            HTTP/WS handler（REST + WebSocket 双通道）
├── auth/           注册/登录 + JWT 签发/校验/中间件
├── command/        业务命令 API（create_media_node / create_media_edge / submit_generation 等）
├── production/     生产翻译层：生产级工具 → 业务命令的映射（Agent 模式专用）
├── agent/          MultiAgent 编排（eino: Producer → Sub-Agents）
├── skill/          Skill 领域知识加载与匹配
├── sandbox/        OpenSandbox Go SDK 封装（M3 引入）
├── media/          对象存储/资产管理
├── dashscope/      DashScope API 封装（LLM / 文生图 / 图生视频）
├── config/         viper 配置加载
└── store/          sqlc 生成 + 仓储层
```

## 数据流

### Studio 模式

```
用户操作画布（创建节点 / 连线 / 编辑 Prompt / 提交生成）
  → REST API（POST /api/nodes, POST /api/edges, POST /api/generation 等）
  → command 模块执行业务命令 → 写 PostgreSQL
  → 广播 WebSocket 事件 → 前端更新 tldraw store
  → 生成任务异步执行 → DashScope API → 结果写回 DB
  → 中间产物存 MinIO，元数据存 Postgres
  → /ws/canvas 推送进度和完成状态
```

### Agent 模式

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

详见 [画布设计 — 前后端数据通路](design-canvas.md)。

## WebSocket 双通道设计

| 通道 | 路径 | 用途 | 可靠性要求 |
|---|---|---|---|
| 对话通道 | `/ws/chat` | Agent 模式流式文本输出、工具调用状态 | 丢消息影响不大 |
| 画布通道 | `/ws/canvas` | Shape 增删改、连线变更、节点执行状态 | 不能丢，否则状态不一致 |

前端通过 JWT token 认证 WebSocket 连接（query param 或首帧传递）。

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
| M0 Studio 画布 | 自定义 MediaShape + 连线 + 资源树 + 属性面板 + 生成 + WebSocket | 用户可手动创建节点、连线、提交生成 |
| M1 Agent 对话 | 对话面板 + 单 Agent + 生产级工具 + PSS + 画布只读 + Gate | 用户可通过对话让 Agent 创建节点和生成 |
| M2 MultiAgent + Skill | Producer + 5 Sub-Agent + 内置 Skill + 评审重试 + Stale 传播 | Agent 自动完成需求到成片全流程 |
| M3 一致性与质量 | 跨镜头一致性 + 多 Skill + 成本管理 + 审计 + 模式切换 | 生成视频可用性和可控性提升 |

详见 [整体业务交互设计 — 实施路线](design-overview.md)。
