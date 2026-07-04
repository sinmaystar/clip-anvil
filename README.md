# ClipAnvil 影砧

ClipAnvil 是一个面向电商营销视频生产的本地优先创作系统。它把多媒体画布、模型生成、Agent 编排、素材版本管理和最终视频合成放在同一套工作流里，让用户可以用两种方式制作营销资产：

- **Studio 模式**：用户在 React Flow 画布上手动创建文本、图片、视频、音频、参考包和依赖连线，按节点提交生成。
- **Agent 模式**：用户通过对话给出商品、目标和约束，Producer / Craftsman / Reviewer / Worker / Composer 协同生成分镜、图片、音频、字幕和最终视频。

当前低成本视频主线已经支持：Seedream 生成多张商品图，火山引擎生成口播和 BGM，Composer 生成 `remotion_timeline_v1`，由 Remotion 在 sandbox 中渲染完整营销视频。Seedance 视频模型仍可作为更高成本路线中的局部 motion/hero shot 使用，但不会在用户禁止 Seedance 时被调用。

## 功能快照

- Workspace、登录注册、Studio / Agent 模式分流。
- Studio React Flow 画布：媒体节点、分组、依赖连线、资源树、上传、Prompt `@` 引用、Reference Pack、手动运行、版本查看和 winner 选择。
- Agent Workbench：对话驱动生产、HITL 决策卡、任务时间线、分镜/产物/问题投影。
- MultiAgent 后端：Producer、Craftsman、Reviewer、Worker、Composer，基于 Eino tool loop 和数据库 checkpoint。
- 生产底座：`generation_job`、`artifact_version`、模型能力、失败重试、stale reason、MinIO 持久化。
- Provider：mock provider、本地 internal provider、Volcengine/Doubao 文本/图片/视频/音频，TOS 暂存真实 provider 输入。
- Sandbox：OpenSandbox workspace、远程媒体下载、FFmpeg、Remotion final timeline renderer。
- Remotion final composer：多段图片/视频 timeline、layout/motion/transition、字幕安全区、voiceover/BGM 合成、最终 MP4 artifact。

## 技术栈

| 模块 | 技术 |
|---|---|
| 前端 | Vite 8、React 19、TypeScript 6、React Router、TanStack Query、Zustand、TailwindCSS 4、`@xyflow/react` |
| 后端 | Go 1.26、Hertz、pgx v5、sqlc、goose、viper、slog、Eino |
| 存储 | PostgreSQL 16、Redis 7、MinIO、Volcengine TOS |
| 沙箱 | OpenSandbox、FFmpeg、Remotion |
| 包管理 | pnpm 11.5.3、Node 26 |

## 仓库结构

```text
clip-anvil/
├── apps/
│   ├── web/                 # 前端应用
│   └── server/              # Go 后端、迁移、sqlc、Agent、生产服务
├── packages/
│   ├── shared-types/        # 前后端共享 TS 类型
│   └── eslint-config/       # 共享 ESLint 配置
├── sandbox-image/           # OpenSandbox 镜像与 Remotion timeline renderer
├── deploy/                  # 本地 compose、Nginx、容器配置
├── scripts/                 # dev-start/dev-stop、smoke 脚本
├── docs/                    # 工程、设计、里程碑、spec/plan
├── AGENTS.md                # Codex 执行契约
└── Makefile                 # 后端构建、测试、迁移、sqlc
```

## 环境要求

- Docker Desktop
- Go 1.26+
- Node 26.x
- pnpm 11.5.3
- 可选工具：`goose`、`sqlc`、`golangci-lint`

安装常用 Go 工具：

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
brew install golangci-lint
```

## 快速开始

安装依赖：

```bash
pnpm install
```

复制本地配置。默认 mock provider 可跑本地开发；真实 Volcengine / TOS key 只放在本地 `.env`，不要提交：

```bash
cp .env.example .env
```

启动完整本地开发环境：

```bash
./scripts/dev-start.sh
```

脚本会自动：

- 启动 `deploy/docker-compose.yml` 中的 PostgreSQL、Redis、MinIO、Nginx、OpenSandbox。
- 编译后端到 `bin/server`。
- 从 `8888-8999` 选择可用后端端口。
- 从 `5173-5299` 选择可用 Vite 端口。
- 启动前后端并打印浏览器访问地址。

停止当前 profile：

```bash
./scripts/dev-stop.sh
```

多个 worktree 并行时，先查看当前 worktree 将使用的 profile 和端口：

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

如果需要停止指定 profile，使用脚本输出的 `CLIPANVIL_DEV_NAME`：

```bash
CLIPANVIL_DEV_NAME=<profile> ./scripts/dev-stop.sh
```

## 常用命令

后端：

```bash
make server-build
make server-test
make server-lint
make migrate-up
make migrate-down
make sqlc-generate
```

前端：

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm dev:web
```

Sandbox / Remotion：

```bash
docker build -t clipanvil-sandbox:dev sandbox-image
node --check sandbox-image/remotion-timeline/src/render.mjs
./scripts/smoke-m13-1-remotion-timeline.sh
./scripts/smoke-m13-3-remotion-layouts.sh
./scripts/smoke-m13-4-remotion-mixed-media.sh
```

提交前的最小检查按改动范围选择：

```bash
git diff --check
GOCACHE=/private/tmp/clipanvil-go-build make server-build
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

## 真实模型配置

本地默认使用 mock provider。需要真实模型时，在 `.env` 中配置：

- `CLIPANVIL_PRODUCTION_PROVIDER_MODE`
- `CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY`
- `CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_API_KEY`
- `CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL`
- `CLIPANVIL_PRODUCTION_VOLCENGINE_IMAGE_MODEL`
- `CLIPANVIL_PRODUCTION_VOLCENGINE_VIDEO_MODEL`
- `CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_MODEL`
- `CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_*`

注意：Seedance 视频生成成本较高。Agent / Producer / Composer prompt 和 route policy 已区分 `no-seedance`、`mixed-cost`、`premium` 路线；用户明确禁止 Seedance 时，应只走 Seedream still + audio + Remotion final timeline。

## 文档入口

- [docs/README.md](docs/README.md)：文档总入口和当前事实口径。
- [docs/engineering/architecture.md](docs/engineering/architecture.md)：工程架构。
- [docs/engineering/development.md](docs/engineering/development.md)：开发指南。
- [docs/engineering/agent-multiagent-architecture.md](docs/engineering/agent-multiagent-architecture.md)：Agent 多角色架构。
- [docs/design/overview.md](docs/design/overview.md)：产品和交互总览。
- [docs/milestones/m13-remotion-timeline-composer.md](docs/milestones/m13-remotion-timeline-composer.md)：Remotion timeline final composer 里程碑。

当前实现口径优先级：代码和迁移 > `docs/engineering/` > `docs/design/` > `docs/milestones/` > `docs/archive/`。

## 开发约定

- Studio 和 Agent 共享 production substrate，但 Studio 用户运行 API 与 Agent 执行 API 保持边界清晰。
- 业务数据库是事实源；React Flow 只是投影层。
- 改数据库或 sqlc query 时，同时更新迁移和生成代码。
- 本地浏览器、runtime、smoke 产物不要提交。
- 提交信息使用 Conventional Commits，例如 `feat: add remotion timeline composer`。
