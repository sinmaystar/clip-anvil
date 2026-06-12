# M0 基建 — 设计文档

## 概述

ClipAnvil 影砧项目的第一个里程碑。搭建 monorepo 骨架、拉起全量中间件、实现前后端 hello world，配置完整的开发 hooks 体系。

## 验收标准

1. `docker compose up -d` 成功拉起 postgres、redis、minio、nginx 四个容器
2. `make server-dev` 启动后端，实际连接 postgres/redis/minio，连接成功打日志
3. `curl http://localhost/api/health` 通过 nginx 代理返回 `{"status":"ok","services":{"postgres":"connected","redis":"connected","minio":"connected"}}`
4. `open http://localhost` 通过 nginx 代理在浏览器看到 tldraw 空画布
5. git commit 时 pre-commit hook 自动跑 gofmt/golangci-lint/eslint/prettier
6. commit message 不符合 conventional commits 格式时被 commitlint 拒绝
7. Claude Code 编辑 .go 文件后自动 gofmt，编辑 .ts/.tsx 后自动 eslint --fix

## 1. Monorepo 骨架

### 目录结构

```
clip-anvil/
├── apps/
│   ├── web/
│   │   ├── package.json              # name: @clip-anvil/web
│   │   ├── tsconfig.json
│   │   ├── vite.config.ts
│   │   ├── index.html
│   │   ├── src/
│   │   │   ├── main.tsx
│   │   │   └── App.tsx               # 全屏 tldraw 空画布
│   │   └── .eslintrc.cjs
│   └── server/
│       ├── go.mod                    # module github.com/sinmaystar/clip-anvil
│       ├── go.sum
│       ├── cmd/
│       │   └── server/
│       │       └── main.go           # Hertz 入口 + /api/health
│       ├── internal/
│       │   ├── api/                  # .gitkeep
│       │   ├── auth/                 # .gitkeep
│       │   ├── config/               # viper 配置加载
│       │   ├── workflow/             # .gitkeep
│       │   ├── agent/                # .gitkeep
│       │   ├── sandbox/              # .gitkeep
│       │   ├── media/                # .gitkeep
│       │   ├── dashscope/            # .gitkeep
│       │   └── store/                # .gitkeep
│       └── config.yaml
├── packages/
│   ├── shared-types/
│   │   ├── package.json
│   │   └── src/index.ts              # 空导出
│   ├── canvas-schema/
│   │   ├── package.json
│   │   └── src/index.ts              # 空导出
│   └── eslint-config/
│       ├── package.json
│       └── index.cjs                 # 共享 ESLint 规则
├── deploy/
│   ├── docker-compose.yml
│   ├── .env.example
│   ├── nginx/
│   │   ├── default.conf              # prod 配置（后续里程碑）
│   │   └── dev.conf                  # dev 模式代理
│   └── init/
│       └── postgres/
│           └── 001_init.sql          # 占位
├── docs/
├── package.json                      # 根 package.json
├── pnpm-workspace.yaml
├── Makefile
├── CLAUDE.md
├── .gitignore
├── .editorconfig
├── lefthook.yml
└── commitlint.config.cjs
```

### 根 package.json

```json
{
  "name": "clip-anvil",
  "private": true,
  "engines": {
    "node": ">=26"
  },
  "packageManager": "pnpm@11.5.3",
  "scripts": {
    "dev:web": "pnpm --filter web dev",
    "build:web": "pnpm --filter web... build",
    "lint:web": "pnpm --filter web lint"
  },
  "devDependencies": {
    "@commitlint/cli": "^19",
    "@commitlint/config-conventional": "^19",
    "prettier": "^3"
  }
}
```

### pnpm-workspace.yaml

```yaml
packages:
  - "apps/web"
  - "packages/*"
```

### Go module

- module path: `github.com/sinmaystar/clip-anvil`
- `go.mod` 放在 `apps/server/`（Go 代码的根）
- 内部包 import 路径示例：`github.com/sinmaystar/clip-anvil/internal/config`
- M0 依赖：hertz、viper、slog（标准库）、pgx v5、go-redis v9、minio-go v7

## 2. Compose + Nginx

### docker-compose.yml

```yaml
services:
  postgres:
    image: postgres:16
    ports: ["5432:5432"]
    environment:
      POSTGRES_USER: clipanvil
      POSTGRES_PASSWORD: ${PG_PASSWORD:-clipanvil_dev}
      POSTGRES_DB: clipanvil
    volumes:
      - pg_data:/var/lib/postgresql/data
      - ./init/postgres:/docker-entrypoint-initdb.d

  redis:
    image: redis:7
    ports: ["6379:6379"]

  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      MINIO_ROOT_USER: ${MINIO_USER:-clipanvil}
      MINIO_ROOT_PASSWORD: ${MINIO_PASSWORD:-clipanvil_dev}
    volumes:
      - minio_data:/data

  nginx:
    image: nginx:alpine
    ports: ["80:80"]
    volumes:
      - ./nginx/dev.conf:/etc/nginx/conf.d/default.conf

volumes:
  pg_data:
  minio_data:
```

### Nginx dev.conf

```nginx
server {
    listen 80;

    location / {
        proxy_pass http://host.docker.internal:5173;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    location /api/ {
        proxy_pass http://host.docker.internal:8888;
    }

    location /ws/ {
        proxy_pass http://host.docker.internal:8888;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### .env.example

```
PG_PASSWORD=clipanvil_dev
MINIO_USER=clipanvil
MINIO_PASSWORD=clipanvil_dev
```

`.env` 加入 `.gitignore`，提供 `.env.example` 作为模板。

## 3. 后端

### 启动流程

1. viper 读取 `config.yaml`
2. pgx 连接 postgres — 成功打日志，失败退出
3. go-redis 连接 redis — 成功打日志，失败退出
4. minio-go 连接 minio — 成功打日志，失败退出
5. 注册 `GET /api/health` — 返回各中间件连接状态
6. Hertz 监听 `:8888`

### config.yaml

```yaml
server:
  port: 8888

postgres:
  dsn: "postgres://clipanvil:clipanvil_dev@localhost:5432/clipanvil?sslmode=disable"

redis:
  addr: "localhost:6379"

minio:
  endpoint: "localhost:9000"
  access_key: "clipanvil"
  secret_key: "clipanvil_dev"
  use_ssl: false
```

### /api/health 响应

```json
{
  "status": "ok",
  "services": {
    "postgres": "connected",
    "redis": "connected",
    "minio": "connected"
  }
}
```

### Makefile targets

```makefile
server-dev:       # go run ./apps/server/cmd/server
server-build:     # go build -o bin/server ./apps/server/cmd/server
server-test:      # go test ./apps/server/...
server-lint:      # golangci-lint run ./apps/server/...
migrate:          # goose/golang-migrate（M0 占位，后续实现）
```

## 4. 前端

### 页面

`App.tsx` 渲染一个全屏 tldraw Editor，使用 tldraw 默认工具集，无自定义 Shape。

### 依赖

```json
{
  "dependencies": {
    "react": "^18",
    "react-dom": "^18",
    "tldraw": "^5"
  },
  "devDependencies": {
    "@types/react": "^18",
    "@types/react-dom": "^18",
    "typescript": "^5",
    "vite": "^5",
    "@vitejs/plugin-react": "^4",
    "tailwindcss": "^4",
    "eslint": "^9"
  }
}
```

M0 不装 shadcn/ui、zustand、react-router、tanstack-query。

### packages/* 空壳

三个包只有 `package.json` + `src/index.ts`（空导出）。`apps/web` 不引用它们，等后续里程碑需要时再加。

## 5. Hooks

### 第 1 层：lefthook pre-commit

```yaml
pre-commit:
  parallel: true
  commands:
    go-fmt:
      glob: "*.go"
      run: gofmt -l -w {staged_files} && golangci-lint run --fix {staged_files}
    ts-lint:
      glob: "*.{ts,tsx}"
      run: pnpm eslint --fix {staged_files}
    ts-format:
      glob: "*.{ts,tsx,json,css}"
      run: pnpm prettier --write {staged_files}
```

### 第 2 层：lefthook commit-msg

```yaml
commit-msg:
  commands:
    commitlint:
      run: pnpm commitlint --edit {1}
```

### 第 3 层：Claude Code settings.json

```json
{
  "hooks": {
    "afterEdit": [
      {
        "glob": "*.go",
        "command": "gofmt -w $FILE"
      },
      {
        "glob": "*.{ts,tsx}",
        "command": "pnpm eslint --fix $FILE"
      }
    ]
  }
}
```

### commitlint.config.cjs

```js
module.exports = { extends: ['@commitlint/config-conventional'] };
```

### 前置全局工具

- `brew install golangci-lint`
- `brew install lefthook`

## 6. CLAUDE.md

项目级指令文件，Agent 每次启动自动读取。内容包含：项目简介、技术栈、项目结构、常用命令、hooks 行为说明、编码规范。随项目演进逐步更新。

## 实施顺序

按自底向上方案，每步可独立验证：

1. Monorepo 骨架（目录 + 配置文件）
2. CLAUDE.md
3. Hooks 配置（lefthook + commitlint + Claude Code settings.json）
4. deploy/（compose + nginx）→ `docker compose up -d` 验证中间件健康
5. apps/server（Hertz + /api/health + 连接中间件）→ `make server-dev` + `curl` 验证
6. apps/web（Vite + tldraw 空画布）→ `pnpm --filter web dev` + 浏览器验证
7. packages/*（三个空壳包）

## 不在 M0 范围内

- 登录/注册/JWT（M1+）
- 自定义 tldraw Shape/Tool（M1）
- DashScope 接入（M2）
- OpenSandbox 沙箱（M3）
- 路由、状态管理、UI 组件库（按需引入）
- 单测和 E2E 测试框架搭建（M0 只有 hooks，测试框架在第一个有业务逻辑的里程碑引入）
