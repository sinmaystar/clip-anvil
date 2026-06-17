# 开发指南

## 项目结构

```
clip-anvil/
├── apps/
│   ├── web/                          前端应用
│   │   ├── src/
│   │   │   ├── App.tsx               RouterProvider 路由入口
│   │   │   ├── main.tsx              入口
│   │   │   ├── main.css              Tailwind 入口 + 视觉 token + 页面样式
│   │   │   ├── components/           Layout、路由守卫、资源树、属性面板、上传、自动布局控件
│   │   │   ├── pages/                Login/Register/Workspace/Studio 页面
│   │   │   ├── lib/                  API client、canvas 映射、布局、WebSocket
│   │   │   ├── shapes/               tldraw 自定义 MediaShape、GroupContainerShape
│   │   │   └── stores/               auth、appearance 状态
│   │   ├── package.json              @clip-anvil/web
│   │   ├── vite.config.ts
│   │   └── tsconfig.json
│   └── server/                       后端应用
│       ├── cmd/server/main.go        Hertz 启动入口
│       ├── internal/
│       │   ├── api/                  REST handler + /ws/canvas
│       │   ├── auth/                 JWT 鉴权
│       │   ├── config/              viper 配置加载
│       │   └── store/               sqlc + 仓储层
│       ├── migrations/               goose SQL 迁移
│       ├── sqlc/queries/             sqlc 查询定义
│       ├── Dockerfile                多阶段构建（容器化部署用）
│       ├── config.yaml               运行配置
│       ├── go.mod
│       └── go.sum
├── packages/
│   ├── shared-types/                 前后端共享 TS 类型
│   ├── canvas-schema/                画布 Shape/Tool 契约（Studio + Agent 共用）
│   └── eslint-config/                共享 ESLint 配置
├── deploy/
│   ├── docker-compose.yml            中间件容器编排
│   ├── docker-compose.server.yml     容器化部署 override（添加后端容器）
│   ├── config-container.yaml         容器内后端配置（用容器名连接中间件）
│   ├── nginx/                        Nginx 配置（dev/full/prod）
│   └── init/postgres/                数据库初始化脚本
├── scripts/
│   ├── dev-start.sh                  一键启动开发环境
│   └── dev-stop.sh                   一键停止
├── docs/                             项目文档（README 索引，engineering/design/milestones/archive 分层）
├── AGENTS.md                         Codex/Agent 项目上下文
├── CLAUDE.md                         AI Agent 项目上下文（精简版）
├── Makefile                          Go 构建/测试命令
├── package.json                      根 package.json（workspaces 配置）
├── pnpm-workspace.yaml
├── lefthook.yml                      Git hooks 配置
└── commitlint.config.cjs             提交信息规范
```

## 构建与运行命令

### 本地开发 profiles

`./scripts/dev-start.sh` 默认启动一组前后端：后端优先使用 `8888`、Vite 优先使用 `5173`。如果这些端口已被其他 worktree 占用，脚本会自动向后寻找可用端口。多个 worktree 并行时，每个 worktree 使用独立 profile 和端口，PostgreSQL / Redis / MinIO / NGINX 仍共享同一套中间件容器。

查看当前 worktree 将使用的 profile 和端口：

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

直接启动时也会自动推导，不需要 agent 手动判断自己是否在 worktree 中：

```bash
./scripts/dev-start.sh
./scripts/dev-stop.sh
```

需要固定端口时再显式传：

```bash
CLIPANVIL_DEV_NAME=agent-a CLIPANVIL_SERVER_PORT=8888 CLIPANVIL_WEB_PORT=5173 ./scripts/dev-start.sh
CLIPANVIL_DEV_NAME=agent-b CLIPANVIL_SERVER_PORT=8889 CLIPANVIL_WEB_PORT=5174 ./scripts/dev-start.sh

CLIPANVIL_DEV_NAME=agent-b ./scripts/dev-stop.sh
```

并行模式下优先访问脚本输出的 Vite 地址，例如 `http://localhost:5174`。Vite 会读取 `CLIPANVIL_SERVER_PORT`，把 `/api` 和 `/ws` 代理到对应后端。

### OpenSandbox

本地开发栈包含 OpenSandbox Server，默认监听 `http://localhost:8080`。它是 Go 后端使用的内部基础设施服务，不直接暴露给前端。

健康检查：

```bash
curl http://127.0.0.1:8080/health
```

修改 `sandbox-image/` 后构建本地沙箱镜像：

```bash
docker build -t clipanvil-sandbox:dev sandbox-image
```

### 前端

```bash
pnpm --filter @clip-anvil/web dev       # 开发服务器
pnpm --filter @clip-anvil/web build     # 生产构建
pnpm --filter @clip-anvil/web lint      # ESLint 检查
pnpm --filter @clip-anvil/web... build  # 构建（含依赖包）
```

### 后端

```bash
make server-dev      # go run 启动
make server-build    # 编译到 bin/server
make server-test     # 运行单测
make server-lint     # golangci-lint
make migrate-up      # 执行 goose 迁移
make migrate-down    # 回滚最近一次 goose 迁移
make migrate-create name=add_xxx
make sqlc-generate   # 重新生成 internal/store/db
```

### 全局

```bash
pnpm install         # 安装所有前端依赖
```

## Git Hooks

三层 hooks 自动执行，开发者无需手动操作。

### 第 1 层：pre-commit（lefthook）

提交时自动运行，仅处理 staged 文件：

| 命令 | 文件类型 | 行为 |
|---|---|---|
| `gofmt -l -w` + `golangci-lint run --fix` | `*.go` | 格式化 + 静态分析 |
| `eslint --fix` | `*.ts, *.tsx` | Lint 修复 |
| `prettier --write` | `*.ts, *.tsx, *.json, *.css` | 格式化 |

### 第 2 层：commit-msg（lefthook）

校验提交信息格式，必须符合 [Conventional Commits](https://www.conventionalcommits.org/)：

```
feat: 新功能
fix: 修复 bug
chore: 构建/工具变更
docs: 文档更新
refactor: 重构
test: 测试
```

示例：`feat(server): add JWT authentication`

### 第 3 层：Claude Code afterEdit hooks

AI Agent 编辑文件后自动触发。Codex 环境同样遵循根目录 `AGENTS.md` 中的 hooks 约定：

| 文件类型 | 行为 |
|---|---|
| `*.go` | `gofmt -w` |
| `*.ts, *.tsx` | `eslint --fix` |

## 编码规范

### Go

- 遵循标准 Go 风格（`gofmt` 强制）
- 日志使用 `slog`（标准库），不用第三方日志库
- error 显式处理，不忽略返回的 error
- 内部包 import 路径：`github.com/sinmaystar/clip-anvil/internal/<module>`
- 配置通过 viper 从 `config.yaml` 加载，不硬编码

### TypeScript

- 严格模式（`"strict": true`）
- ESLint + Prettier 强制格式
- 组件使用函数式组件 + hooks
- 状态管理：画布状态由 tldraw 管理，UI 状态用 zustand

### 通用

- 不写注释，除非解释 why（非 what）
- 不提前抽象，三次重复再提取
- 不加 feature flag 或向后兼容 shim
- 文件保持聚焦，一个文件一个职责

## 依赖管理

### 前端

- 包管理器：pnpm 11.5.3（`package.json` 中 `packageManager` 字段锁定）
- Node 要求：26.x（根 `package.json` 锁定 `>=26 <27`，`.nvmrc` / `.node-version` 提供本地版本提示）
- workspace 包以 `@clip-anvil/` 为命名空间

### 后端

- Go module path：`github.com/sinmaystar/clip-anvil`
- `go.mod` 位于 `apps/server/`

## 全局工具要求

开发前确保已安装：

```bash
brew install golangci-lint    # Go 静态分析
brew install lefthook         # Git hooks 管理
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

Node、pnpm、Go 通过各自版本管理器安装。
