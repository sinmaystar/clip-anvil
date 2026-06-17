# ClipAnvil 影砧 - 架构方案 v1

## 一、设计目标与原则

- 本地个人项目优先，单机 Docker Compose 一键启动
- Monorepo + AI Coding Agent 友好（清晰边界、最小心智负担）
- Studio / Agent 双模式共用同一个 tldraw 画布抽象
- 代码层无状态，未来可平滑切到多副本/云端，但本地不预先做副本

## 二、Monorepo 目录结构

```
clip-anvil/
├── apps/
│   ├── web/                       前端 (Vite + React + TS + tldraw)
│   └── server/                    后端 (Go + Hertz)
├── packages/
│   ├── shared-types/              TS 类型 (从 Go 生成 OpenAPI / 手写)
│   ├── canvas-schema/             画布 Shape/Tool 定义 (Studio + Agent 共用)
│   └── eslint-config/             共享前端配置
├── sandbox/
│   └── code-interpreter-ffmpeg/   基于 OpenSandbox 镜像的扩展 (装 ffmpeg/yt-dlp 等)
├── deploy/
│   ├── docker-compose.yml         全栈一键启动
│   ├── nginx/
│   │   └── default.conf           反代 + 静态托管 + WS 升级
│   └── init/
│       ├── postgres/              建表/迁移初始化
│       ├── minio/                 桶初始化
│       └── opensandbox/           sandbox.toml 配置
├── docs/
└── pnpm-workspace.yaml / Makefile
```

## 三、容器拓扑（docker compose / docker compose）

```
                ┌──────────┐
   浏览器  ───▶ │  nginx   │ ── 静态前端（prod）/ 代理 Vite dev server（dev）
                │  :80     │ ── /api       ──▶ server
                │          │ ── /ws/chat   ──▶ server (WebSocket: Agent 对话)
                │          │ ── /ws/canvas ──▶ server (WebSocket: 画布同步)
                └──────────┘

                  server (Go)
                     │
   ┌─────────────────┼──────────────────┬──────────────────┐
   ▼                 ▼                  ▼                  ▼
postgres:16       redis:7         minio:latest    opensandbox-server
                                                        │
                                                        ▼ (容器 socket)
                                              code-interpreter-ffmpeg 沙箱
                                              (按会话动态拉起/销毁)
```

要点：
- 当前阶段使用 **docker**（`docker compose`），后期切换回 Docker。compose 文件保持兼容，不使用 Docker 专有特性
- OpenSandbox Server 挂载容器 socket（docker: `$XDG_RUNTIME_DIR/docker/docker.sock`，Docker: `/var/run/docker.sock`），需确认 OpenSandbox 支持自定义 socket 路径
- 每个中间件独立容器，自定义 network（不打多进程镜像）
- web 服务先单实例，nginx upstream 预留扩展位
- Nginx 在本地开发阶段也通过 compose 拉起，dev 模式下代理到 Vite dev server（:5173），保证开发/部署环境一致

## 四、技术选型

### 前端 `apps/web`

| 层 | 选型 |
|---|---|
| 构建 | Vite 8 |
| 框架 | React 19 + TypeScript 6 |
| 画布 | `tldraw` v5 + 自定义 Shape/Tool |
| 样式 | TailwindCSS + shadcn/ui (Radix) |
| 状态 | Zustand（画布外 UI 状态） |
| 数据 | TanStack Query + Axios/Fetch |
| WebSocket | 原生 + 轻量封装，双通道：`/ws/chat`（Agent 对话流式交互）+ `/ws/canvas`（画布状态同步） |
| 路由 | React Router v6 |

### 后端 `apps/server`

| 层 | 选型 |
|---|---|
| 语言 | Go 1.26+ |
| Web | Hertz (cloudwego/hertz) |
| DB driver | pgx v5 |
| SQL | sqlc（生成类型安全代码） |
| 迁移 | goose 或 golang-migrate |
| Redis | `github.com/redis/go-redis/v9` |
| MinIO | `github.com/minio/minio-go/v7` |
| Sandbox | `github.com/alibaba/OpenSandbox/sdks/sandbox/go` |
| Agent 编排 | **eino** (`github.com/cloudwego/eino` + `eino-ext`) |
| 配置 | viper |
| 日志 | slog（标准库） |
| 鉴权 | JWT（注册/登录，token 签发与校验） |
| LLM / AIGC | 通过 eino 接入阿里 DashScope（通义千问 LLM + 文生图/图生图/文生视频等） |

### Monorepo 工具
- 包管理：pnpm + workspaces（`pnpm --filter web... build` 按依赖拓扑顺序构建）
- Go 构建/测试/迁移：Makefile
- 提交规范：commitlint + lefthook（轻量替代 husky）

## 五、关键模块边界

### 5.1 `packages/canvas-schema`（最重要）
两种模式都依赖的契约层。定义：
- 节点类型：`text-to-image` / `image-to-image` / `image-to-video` / `text-to-video` / `composite` / `output` 等
- 节点 IO Schema（输入输出端口类型）
- 工作流 DAG 序列化格式（JSON）
- Agent 操作画布的工具定义（与 tldraw editor API 一一对应：createShape / connect / setProps 等）

### 5.2 后端模块（Go）
```
apps/server/internal/
├── api/            HTTP/WS handler
├── auth/           注册/登录 + JWT 签发/校验/中间件
├── workflow/       Studio 模式 DAG 编排执行器
├── agent/          Agent 模式编排 (基于 eino: ChatModel / Tool / Graph)
├── sandbox/        OpenSandbox Go SDK 封装（创建/复用/回收沙箱）
├── media/          ffmpeg/对象存储/资产管理
├── dashscope/      DashScope API 封装（LLM / 文生图 / 图生视频等）
├── store/          sqlc 生成 + 仓储层
└── config/
```

### 5.3 沙箱镜像 `sandbox/code-interpreter-ffmpeg/`
基于官方 `opensandbox/code-interpreter:v1.1.0` 加一层：
```Dockerfile
FROM sandbox-registry.cn-zhangjiakou.cr.aliyuncs.com/opensandbox/code-interpreter:v1.1.0
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg yt-dlp imagemagick && rm -rf /var/lib/apt/lists/*
```

## 六、数据流

### Studio 模式
```
用户在 tldraw 画布拖拽节点 → 前端序列化为 DAG
  → POST /api/workflows/run
  → server 拓扑排序 → 逐节点执行
       ├─ LLM 节点：调用 llm 模块
       ├─ 图像/视频节点：调用模型 API（或本地推理）
       └─ ffmpeg 合成节点：通过 sandbox 模块在沙箱里执行
  → 中间产物存 MinIO，元数据存 Postgres
  → /ws/canvas 推送节点执行进度 → 前端刷新画布节点状态
```

### Agent 模式
```
用户对话 → /ws/chat 建立连接
  → server.agent 模块（eino Graph 编排）
  → ChatModel 循环推理 + Tool 调用
     · Tool 集 = canvas-schema 暴露的画布操作 + Studio 节点能力
  → 对话流式输出通过 /ws/chat 推送给前端
  → 画布变更通过 /ws/canvas 推送给前端 tldraw store
  → 直到产出最终视频
```

关键：
- Agent 工具集与 Studio 节点能力 **同一套底层**，复用 workflow 执行器；eino 只负责”推理 + 工具选择 + 状态机”，业务能力不耦合到框架
- WebSocket 双通道分离关注点：`/ws/chat` 承载对话交互（流式文本、工具调用状态），`/ws/canvas` 承载画布状态同步（Shape 增删改、连线变更、节点执行状态），两者可靠性要求不同

## 七、本地启动流程

```bash
# 一次性
pnpm install
make sandbox-image           # 构建自定义 sandbox 镜像
docker compose -f deploy/docker-compose.yml up -d postgres redis minio opensandbox-server nginx
make migrate                 # 数据库迁移

# 日常开发
pnpm --filter web dev        # 前端 Vite dev server（nginx dev 模式代理到 :5173）
make server-dev              # 后端 air/CompileDaemon 热重载
# nginx 始终通过 compose 运行，dev profile 下代理到本地开发端口
```

也提供完整容器化模式：`docker compose --profile prod up`，前后端都打镜像跑，用于演示。
> 注：当前阶段使用 docker compose，后期切换 Docker 后将命令改为 docker compose，compose 文件保持兼容。

## 八、迭代里程碑（建议）

| 里程碑 | 范围 | 关键交付 |
|---|---|---|
| M0 基建 | Monorepo 骨架 + compose 跑通 + 前后端 hello world | `docker compose up` 后浏览器能看到带 tldraw 空画布的页面，调通一个 `/api/health` |
| M1 Studio 静态 | 自定义节点 1-2 个 + 画布序列化 | 文生图节点 + 输出节点，可手动连线但不执行 |
| M2 Studio 执行 | DAG 执行器 + LLM/图像 API 接入 + MinIO 存储 | 一条最简单链路（文字→图片）端到端跑通 |
| M3 沙箱集成 | OpenSandbox Go SDK 接入 + ffmpeg 节点 | 多图合成短视频成功输出 |
| M4 Agent 模式 | LLM 工具调用 → 画布操作 → 自动构图 | Agent 能从对话生成完整工作流 |
| M5 体验打磨 | 错误处理 / 重试 / 资产管理 / 模板 | 可对外展示版本 |

## 九、与你最初设想的差异（已采纳的调整）

1. 中间件**不打到一个容器里**，独立容器 + compose 编排（你已同意"全上"，按 Postgres/Redis/MinIO 三件套独立部署）
2. nginx 保留，但**本地只起单实例 web 服务**，不真起多副本（代码无状态化，留扩展位即可）
3. Sandbox 用阿里 **OpenSandbox**（已确认），自定义镜像加 ffmpeg
4. 前端定 **Vite + React + tldraw + Tailwind + shadcn/ui** 主流栈

## 十、已确认的细节

- **鉴权**：简单的注册 + 登录，JWT 保存 token。后端签发/校验 JWT，前端请求头带 `Authorization: Bearer <token>`，WebSocket 连接时通过 query param 或首帧传递 token
- **LLM / 图像 / 视频生成**：统一使用阿里 **DashScope** 云服务，通过 eino 接入。覆盖 LLM 对话（通义千问）、文生图、图生图、文生视频等能力，不做本地推理

—— 以上为整体架构蓝图。如果方向 OK，下一步进入 M0 基建落地，我会按此目录结构生成 monorepo 骨架、docker-compose、初始化脚本以及前后端最小可运行 demo。
