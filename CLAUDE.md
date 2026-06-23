# ClipAnvil 影砧

营销视频生成平台，Studio（工作流画布）+ Agent（对话驱动）双模式。

## 技术栈

- 前端：Vite 8 + React 19 + TypeScript 6 + `@xyflow/react` 12 + TailwindCSS 4
- 后端：Go 1.26 + Hertz + pgx v5 + sqlc + viper + slog
- 中间件：PostgreSQL 16 / Redis 7 / MinIO
- 容器：Docker Compose

## 项目结构

- `apps/web/` — 前端
- `apps/server/` — 后端（Go module: github.com/sinmaystar/clip-anvil）
- `packages/shared-types/` — TS 类型定义
- `packages/eslint-config/` — 共享 ESLint 配置
- `deploy/` — compose + nginx 配置
- `docs/` — 架构方案 + 各阶段 spec

## 常用命令

- `pnpm --filter @clip-anvil/web dev` — 启动前端 dev server
- `make server-dev` — 启动后端
- `make server-test` — 后端单测
- `make server-lint` — 后端 lint
- `pnpm --filter @clip-anvil/web... build` — 构建前端（含依赖包）
- `docker compose -f deploy/docker-compose.yml up -d` — 拉起中间件

## Hooks 行为

- 编辑 .go 文件后自动 gofmt
- 编辑 .ts/.tsx 文件后自动 eslint --fix
- git commit 时 pre-commit 跑 lint/format，commit-msg 跑 commitlint
- 提交信息遵循 conventional commits（feat: / fix: / chore: 等）

## 编码规范

- Go：遵循标准 Go 风格，slog 做日志，error 显式处理
- TypeScript：严格模式，ESLint + Prettier
- 不写注释除非解释 why
