# ClipAnvil Codex 指令

本文件是给 Codex 读取的执行契约。保持简短、准确、可执行；面向人的长说明放到 `docs/`。

## 先读这里

- 文档入口：`docs/README.md`。
- 当前工程文档：`docs/engineering/`。
- 当前设计文档：`docs/design/`。
- 里程碑状态：`docs/milestones/`。
- 历史 spec/plan：`docs/archive/`；不要把 archive 当作当前实现口径。
- 当前事实优先级：代码和迁移 > `docs/engineering/` > `docs/design/` > `docs/milestones/` > `docs/archive/`。

## 仓库地图

- 前端：`apps/web` — Vite 8、React 19、TypeScript 6、`@xyflow/react` 12、TailwindCSS 4。
- 后端：`apps/server` — Go 1.26、Hertz、pgx v5、sqlc、viper、slog。
- 共享包：`packages/shared-types`、`packages/eslint-config`。
- 本地中间件：PostgreSQL 16、Redis 7、MinIO、NGINX、OpenSandbox，由 `deploy/docker-compose.yml` 管理。
- 运行脚本：`scripts/dev-start.sh`、`scripts/dev-stop.sh`。

## 任务路由

- 做 roadmap、milestone、架构或 docs 现状整理前，先对照代码、迁移、脚本和 smoke 面。
- 做 Studio/Agent 相关行为时，保持模式边界清晰。Studio 用户运行 API 不是 Agent 执行 API；Agent 应复用共享 production service/tool。
- 改数据库或 sqlc 时，同时看迁移和 `apps/server/sqlc/queries/`。
- 改前端画布时，记住 React Flow 是投影层；业务 DB/API 状态才是事实源。
- 只改文档时，不要新增长期执行计划文档，除非用户明确要求。

## 本地运行

默认使用仓库脚本：

```bash
./scripts/dev-start.sh
./scripts/dev-stop.sh
```

不要默认手动拼 `go run` + `pnpm dev`。只有拆分排查问题时才使用手动命令。

多个 worktree 并行时，不要猜端口，也不要依赖固定 NGINX 入口 `http://localhost`。使用脚本输出的 Vite URL；只查看当前环境时运行：

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

脚本会从 `8888-8999` 自动选择后端端口，从 `5173-5299` 自动选择前端端口。如果显式指定端口且端口被占用，启动必须失败，不要静默切换端口。

## 验证

按改动范围选择能证明结果的最小检查集：

```bash
make server-build
make server-test
make server-lint
make sqlc-generate
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

- 改 Go：运行 `gofmt`，再跑相关 server build/test/lint。
- 改迁移或 sqlc query：运行 `make sqlc-generate` 和 `make server-test`。
- 改前端：运行 web build 和 lint，除非明确只是文档改动。
- 改运行脚本：运行 `bash -n scripts/dev-start.sh`、`bash -n scripts/dev-stop.sh`、`CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh` 和 `git diff --check`。
- 只改文档：运行 `git diff --check`；如果改了链接或移动文件，再跑 Markdown 相对链接检查。

没有新鲜验证输出，不要声称完成。

## 编码规则

- 沿用现有模块边界和命名。
- Go：标准风格，显式处理 error，日志用 `slog`。
- TypeScript：严格模式，React 函数组件和 hooks。
- 有结构化 parser/API 时优先使用，不要写脆弱字符串拼接。
- 不要提前抽象；只有能消除真实重复或复杂度时才抽象。
- 不写注释，除非解释 why。

## 安全规则

- 不要改无关文件。
- 不要回滚用户已有改动。
- 不要使用破坏性 git 命令，除非用户明确要求。
- 本地浏览器/runtime 产物视为临时产物；不要把 smoke 生成物提交，除非用户要求。
- 如果命令被 sandbox、权限、认证或网络状态阻塞，报告阻塞原因和失败命令，不要编造替代状态。

## 发布和 PR

用户要求发布、提交或提 PR 时，先读 `docs/engineering/github-pr-flow.md`，按其中流程执行。
