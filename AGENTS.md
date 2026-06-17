# ClipAnvil Agent Instructions

执行任务时优先遵守本文件，再参考 `docs/README.md` 指向的详细文档。

## Repo Snapshot

- 前端：`apps/web`，Vite 8 + React 19 + TypeScript 6 + tldraw 5 + TailwindCSS 4。
- 后端：`apps/server`，Go module `github.com/sinmaystar/clip-anvil`，Go 1.26 + Hertz + pgx v5 + sqlc + viper + slog。
- 中间件：PostgreSQL 16 / Redis 7 / MinIO，通过 Docker Compose 共享运行。
- 共享包：`packages/shared-types`、`packages/canvas-schema`、`packages/eslint-config`。
- 部署配置：`deploy/`，本地 NGINX 只适合默认单实例入口。
- 当前文档入口：`docs/README.md`；现行工程文档在 `docs/engineering/`，设计文档在 `docs/design/`，历史执行 spec/plan 在 `docs/archive/`。

## Local Runtime Rules

需要本地运行应用时，优先使用：

```bash
./scripts/dev-start.sh
```

不要手动拼 `go run` + `pnpm dev` 作为默认启动方式。`dev-start.sh` 会：

1. 拉起共享 Docker Compose 中间件和 NGINX。
2. 编译后端：`go build -o bin/server ./cmd/server`。
3. 运行当前 worktree 刚编译出的 `bin/server`。
4. 启动 Vite dev server。
5. 检查当前后端端口的 `/api/health`。

前端 dev 模式由 Vite 按需编译。需要生产构建验证时运行 `pnpm --filter @clip-anvil/web... build`。

首次初始化数据库或新增迁移后，先确认中间件已启动，再执行：

```bash
make migrate-up
```

停止脚本启动的前后端：

```bash
./scripts/dev-stop.sh
```

`dev-stop.sh` 不会停止 PostgreSQL / Redis / MinIO / NGINX 容器。

## Multi-Worktree Ports

多个 worktree / 多个 AI Coding Agent 并行时，共享中间件，只给每个 worktree 分配不同前后端端口。不要依赖 `http://localhost` 的 NGINX 固定入口；使用脚本输出的 Vite 地址。

默认情况下 agent 不需要手动猜端口：

- `dev-start.sh` 根据当前目录、git 分支和路径 hash 自动生成 profile 名。
- 后端端口从 `8888-8999` 查找可用端口。
- 前端端口从 `5173-5299` 查找可用端口。

只查看当前 worktree 会使用什么环境，不启动进程：

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

输出是可 `eval` 的 `export` 行。需要固定端口时再显式传：

```bash
CLIPANVIL_DEV_NAME=wt-a CLIPANVIL_SERVER_PORT=8888 CLIPANVIL_WEB_PORT=5173 ./scripts/dev-start.sh
CLIPANVIL_DEV_NAME=wt-b CLIPANVIL_SERVER_PORT=8889 CLIPANVIL_WEB_PORT=5174 ./scripts/dev-start.sh
```

如果显式指定的端口已被占用，脚本必须失败，不会静默换端口。

停止某个显式 profile：

```bash
CLIPANVIL_DEV_NAME=wt-b ./scripts/dev-stop.sh
```

如果没有手动设置 `CLIPANVIL_DEV_NAME`，在同一个 worktree 中直接运行 `./scripts/dev-stop.sh`，脚本会推导同一个默认 profile。

## Manual Commands

仅在需要拆分排查时使用手动命令：

```bash
docker compose -f deploy/docker-compose.yml up -d
make migrate-up
make server-dev
pnpm --filter @clip-anvil/web dev
```

注意：

- 默认后端端口是 `8888`，但多 worktree 模式下可能自动变为其他端口。
- 默认 Vite 端口是 `5173`，但多 worktree 模式下可能自动变为其他端口。
- 不要假设 `5178` 可用，除非当前会话显式启动了这个端口。
- Vite 读取 `CLIPANVIL_SERVER_PORT`，把 `/api` 和 `/ws` 代理到对应后端。
- 后端配置可被 `CLIPANVIL_SERVER_PORT` 等环境变量覆盖。

## Verification Commands

按改动范围选择验证，不能声称完成而未运行相关检查。

```bash
make server-build
make server-test
make server-lint
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
make sqlc-generate
git diff --check
```

修改 `apps/server/sqlc/queries/` 或迁移后，运行：

```bash
make sqlc-generate
make server-test
```

修改启动脚本后，至少运行：

```bash
bash -n scripts/dev-start.sh
bash -n scripts/dev-stop.sh
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
git diff --check
```

修改文档后，至少运行 `git diff --check`；如果改了相对链接，检查 Markdown 链接是否仍指向存在文件。

## Hooks And Formatting

- 编辑 `.go` 文件后确保 `gofmt`。
- 编辑 `.ts` / `.tsx` 文件后遵循 ESLint + Prettier。
- git commit 时 pre-commit 跑 lint/format，commit-msg 跑 commitlint。
- 提交信息遵循 Conventional Commits：`feat:` / `fix:` / `docs:` / `chore:` / `refactor:` / `test:`。

## Coding Rules

- Go：标准 Go 风格，`slog` 日志，error 显式处理。
- TypeScript：严格模式，React 函数组件 + hooks。
- 不写注释除非解释 why。
- 不提前抽象；优先沿用现有模块边界和命名。
- 不要把历史归档文档当作当前实现口径。
- 当前事实优先级：代码和迁移 > `docs/engineering/` > `docs/design/` > `docs/archive/`。

## Safety Rules

- 不要改动用户未要求的无关文件。
- 不要回滚用户已有改动。
- 不要使用破坏性 git 命令，除非用户明确要求。
- 并行 worktree 下，报告访问地址时必须使用脚本输出的 Vite URL，不要默认写 `http://localhost`。
- 启动失败时先查对应 profile 的日志路径，日志路径由 `dev-start.sh` 输出。

## GitHub PR Flow

用户要求“提 PR”时，按下面路线执行，不要跳过 scope 检查：

1. 先确认当前状态和范围：

   ```bash
   git status --short --branch
   git diff --stat
   git diff --name-only
   git ls-files --others --exclude-standard
   ```

   如果工作区包含明显无关改动，先向用户确认哪些文件进 PR；不要默认 `git add -A`。

2. 处理分支：

   - 如果当前是 detached HEAD 或在默认分支，先创建 `codex/<short-topic>` 分支。
   - Codex worktree 的实际 `.git` 可能在桌面 checkout，例如 `/Users/wanwan/Desktop/clip-anvil/.git`。创建分支、stage、commit 等写 git metadata 的操作如果被 sandbox 拒绝，使用提权重跑同一条 git 命令。

3. Stage 和 commit：

   - 用显式文件列表 stage 这次 PR 需要的文件。
   - 提交前至少确认 `git diff --cached --stat`。
   - commit message 用简短英文，例如 `polish canvas groups and connections`；如果项目钩子要求 Conventional Commits，再使用 `feat:` / `fix:` / `docs:` 等前缀。

4. 推送分支：

   ```bash
   git push -u origin <branch>
   ```

   如果沙箱里出现 `Could not resolve host: github.com`，这是网络权限问题，使用提权重跑同一条 push。

5. 创建 draft PR：

   - 优先尝试 GitHub App / connector 创建 PR。
   - 如果 GitHub App 返回 `403 Resource not accessible by integration`，改用 `gh pr create` fallback。
   - `gh auth status` 在受限沙箱里可能显示 token invalid 或 API 连接失败；实际 `gh pr create` 仍可能在提权后读取本机有效凭据。遇到网络/API 失败时，先提权重跑 `gh pr create`，不要直接放弃。
   - PR 默认创建 draft，除非用户明确要求 ready for review。

   推荐命令：

   ```bash
   gh pr create \
     --draft \
     --base main \
     --head <branch> \
     --title "[codex] <summary>" \
     --body-file /tmp/<repo>-pr-body.md
   ```

   PR body 至少包含 `Summary` 和 `Validation`，必要时补 `Root Cause Notes`。

6. 完成后汇报：

   - PR URL 和编号。
   - 分支名、commit SHA。
   - 本地 `git status --short --branch` 是否干净。
   - 已运行的验证命令。
