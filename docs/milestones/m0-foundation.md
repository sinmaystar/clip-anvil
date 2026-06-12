# M0 基建 — 交付总结

**状态**：✅ 已完成（2026-06-11）

## 交付内容

1. **Monorepo 骨架** — pnpm workspaces + Makefile，前端 `apps/web`，后端 `apps/server`，共享包 `packages/*`
2. **中间件容器** — docker Compose 拉起 PostgreSQL 16、Redis 7、MinIO、Nginx，全部健康运行
3. **Go 后端** — Hertz 框架，启动时连接 postgres/redis/minio，`/api/health` 返回各服务连接状态
4. **React 前端** — Vite 8 + React 19 + tldraw v5 全屏空画布
5. **Hooks 体系** — lefthook pre-commit（gofmt/golangci-lint/eslint/prettier）+ commitlint + Claude Code afterEdit hooks
6. **开发脚本** — `dev-start.sh` / `dev-stop.sh` 一键启停
7. **CLAUDE.md** — AI Agent 项目上下文

## 验收结果

| 验收标准 | 结果 |
|---|---|
| `docker compose up -d` 成功拉起 4 个容器 | ✅ |
| `make server-dev` 启动后端，连接所有中间件 | ✅ |
| `curl http://localhost/api/health` 通过 nginx 返回 ok | ✅ |
| `http://localhost` 看到 tldraw 画布 | ✅ |
| pre-commit hook 自动跑 lint/format | ✅ |
| commitlint 拒绝不合规提交信息 | ✅ |
| Claude Code afterEdit hooks 配置 | ✅ |

## 实施中的关键决策

| 决策 | 原因 |
|---|---|
| tldraw v5（非原定 v3） | tldraw 无 v3 发布，从 v2 直接跳到 v4→v5 |
| React 19 + Vite 8 + TypeScript 6 | 跟随当前最新稳定版，tldraw v5 支持 React 19 |
| Vite 配置 `allowedHosts: true` | Vite 8 默认拒绝非 localhost Host 头，nginx 容器通过 `host.docker.internal` 代理时需放行 |
| lefthook go-fmt 添加 `root: "apps/server/"` | golangci-lint 需从 go.mod 所在目录运行 |
| 根目录添加 `eslint.config.js` | ESLint 10 要求 flat config，pre-commit hook 需要 |
| 启动脚本用 `go build` 替代 `go run` | `go run` 的子进程 PID 不可控，无法正确 kill |

## 遗留项

- 数据库迁移工具（goose/golang-migrate）未选定，Makefile `migrate` 为占位
- Nginx prod 配置（`default.conf`）为占位，未经验证
- 全局 `core.hooksPath` 与 lefthook 存在冲突提示（不影响功能，lefthook 通过 `--force` 安装）
