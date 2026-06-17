# ClipAnvil 影砧

营销视频生成平台，Studio（工作流画布）+ Agent（对话驱动）双模式。

## 技术栈

- 前端：Vite 8 + React 19 + TypeScript 6 + tldraw 5 + TailwindCSS 4
- 后端：Go 1.26 + Hertz + pgx v5 + sqlc + viper + slog
- 中间件：PostgreSQL 16 / Redis 7 / MinIO
- 容器：Docker Compose

## 项目结构

- `apps/web/` — 前端
- `apps/server/` — 后端（Go module: github.com/sinmaystar/clip-anvil）
- `packages/shared-types/` — TS 类型定义
- `packages/canvas-schema/` — 画布 Shape/Tool 契约
- `packages/eslint-config/` — 共享 ESLint 配置
- `deploy/` — compose + nginx 配置
- `docs/` — 架构方案 + 各阶段 spec

## 常用命令

### 推荐本地启动

- `./scripts/dev-start.sh` — 一键启动开发环境：拉起 Docker Compose 中间件和 NGINX，编译并启动宿主机后端，启动 Vite dev server，最后检查 `http://localhost/api/health`
- `./scripts/dev-stop.sh` — 停止脚本启动的前后端进程；PostgreSQL / Redis / MinIO / NGINX 容器会保留运行
- 首次初始化数据库或新增迁移后，先确认中间件已启动，再执行 `make migrate-up`

脚本启动后的默认访问入口：

- 应用入口：`http://localhost`（经 NGINX 反代）
- API 健康检查：`http://localhost/api/health`
- 后端直连：`http://localhost:8888/api/health`
- Vite 直连：`http://localhost:5173`
- MinIO Console：`http://localhost:9001`
- 后端日志：`tail -f /tmp/clipanvil-server.log`
- 前端日志：`tail -f /tmp/clipanvil-web.log`

注意：默认 Vite 端口是 `5173`。不要假设 `5178` 可用，除非当前会话显式用 `--port 5178` 启动了前端。

### 手动本地启动

- `docker compose -f deploy/docker-compose.yml up -d` — 拉起 PostgreSQL / Redis / MinIO / NGINX
- `make migrate-up` — 执行 goose 迁移；新数据库必须跑，否则 API 会因缺表或缺字段失败
- `make server-dev` — 启动宿主机 Go 后端（读取 `apps/server/config.yaml`，连接 localhost 中间件）
- `pnpm --filter @clip-anvil/web dev` — 启动前端 dev server（默认 `5173`）

### 构建与验证

- `make server-build` — 编译后端到 `bin/server`
- `make server-test` — 后端单测
- `make server-lint` — 后端 lint
- `pnpm --filter @clip-anvil/web... build` — 构建前端及依赖 workspace 包
- `pnpm --filter @clip-anvil/web lint` — 前端 lint
- `make sqlc-generate` — 修改 `apps/server/sqlc/queries/` 后重新生成 `internal/store/db`

### NGINX / 部署拓扑

- `deploy/docker-compose.yml` 是本地开发基础拓扑：中间件容器 + NGINX；NGINX 使用 `deploy/nginx/dev.conf`
- `deploy/nginx/dev.conf`：`/` 代理到宿主机 Vite `host.docker.internal:5173`，`/api/` 和 `/ws/` 代理到宿主机后端 `host.docker.internal:8888`
- `deploy/docker-compose.server.yml` 会额外构建后端容器，并将 NGINX 切到 `deploy/nginx/full.conf`
- `deploy/nginx/full.conf`：前端仍代理宿主机 Vite，`/api/` 和 `/ws/` 代理到后端容器 `backend:8888`
- `deploy/nginx/default.conf` 是生产静态前端模式：从 `/usr/share/nginx/html` 托管前端，`/api/` 和 `/ws/` 代理到 `backend:8888`
- 容器化验证命令：`docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.server.yml up -d --build`

## Hooks 行为

- 编辑 .go 文件后自动 gofmt
- 编辑 .ts/.tsx 文件后自动 eslint --fix
- git commit 时 pre-commit 跑 lint/format，commit-msg 跑 commitlint
- 提交信息遵循 conventional commits（feat: / fix: / chore: 等）

## 编码规范

- Go：遵循标准 Go 风格，slog 做日志，error 显式处理
- TypeScript：严格模式，ESLint + Prettier
- 不写注释除非解释 why
