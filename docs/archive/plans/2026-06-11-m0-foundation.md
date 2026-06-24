# M0 基建 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Scaffold the ClipAnvil monorepo, bring up all middleware via docker Compose, deliver a Go+Hertz health endpoint and a React+React Flow empty canvas page, with full lint/format/commit hooks.

**Architecture:** Monorepo with `apps/web` (Vite+React+React Flow), `apps/server` (Go+Hertz), `packages/*` (TS shared libs), `deploy/` (compose+nginx). Self-bottom-up: skeleton → CLAUDE.md → hooks → infra → backend → frontend → packages.

**Tech Stack:** Go 1.26, Hertz 0.10, pgx v5, go-redis v9, minio-go v7, viper, slog | React 19, React Flow, Vite 8, TailwindCSS 4, TypeScript 6, ESLint 10 | docker Compose, Nginx, PostgreSQL 16, Redis 7, MinIO

**Version note:** The spec referenced React Flow / React 18 / Vite 5, but React Flow has no v3 release (went v2→v4→v5). This plan uses current stable versions: React Flow, React 19, Vite 8. The architecture doc should be updated after M0 lands.

---

### Task 1: Root Monorepo Skeleton

**Files:**
- Create: `package.json`
- Create: `pnpm-workspace.yaml`
- Create: `.gitignore`
- Create: `.editorconfig`

- [ ] **Step 1: Create root package.json**

```json
{
  "name": "clip-anvil",
  "private": true,
  "engines": {
    "node": ">=26"
  },
  "packageManager": "pnpm@11.5.3",
  "scripts": {
    "dev:web": "pnpm --filter @clip-anvil/web dev",
    "build:web": "pnpm --filter @clip-anvil/web... build",
    "lint:web": "pnpm --filter @clip-anvil/web lint"
  },
  "devDependencies": {
    "@commitlint/cli": "^21",
    "@commitlint/config-conventional": "^21",
    "prettier": "^3"
  }
}
```

- [ ] **Step 2: Create pnpm-workspace.yaml**

```yaml
packages:
  - "apps/web"
  - "packages/*"
```

- [ ] **Step 3: Create .gitignore**

```
node_modules/
dist/
.env
*.log
.DS_Store
bin/
tmp/
.vite/

# Go
apps/server/bin/
apps/server/tmp/
```

- [ ] **Step 4: Create .editorconfig**

```ini
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true
indent_style = space
indent_size = 2

[*.go]
indent_style = tab

[Makefile]
indent_style = tab
```

- [ ] **Step 5: Create directory structure with .gitkeep files**

```bash
mkdir -p apps/web/src
mkdir -p apps/server/cmd/server
mkdir -p apps/server/internal/{api,auth,config,workflow,agent,sandbox,media,dashscope,store}
mkdir -p packages/{shared-types/src,canvas-schema/src,eslint-config}
mkdir -p deploy/{nginx,init/postgres}
touch apps/server/internal/{api,auth,workflow,agent,sandbox,media,dashscope,store}/.gitkeep
touch deploy/init/postgres/.gitkeep
```

- [ ] **Step 6: Run pnpm install to generate lockfile**

Run: `pnpm install`
Expected: Creates `pnpm-lock.yaml`, installs commitlint and prettier

- [ ] **Step 7: Commit**

```bash
git add package.json pnpm-workspace.yaml pnpm-lock.yaml .gitignore .editorconfig apps/ packages/ deploy/
git commit -m "chore: scaffold monorepo directory structure"
```

---

### Task 2: Makefile

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Create Makefile**

```makefile
.PHONY: server-dev server-build server-test server-lint migrate

server-dev:
	cd apps/server && go run ./cmd/server

server-build:
	cd apps/server && go build -o ../../bin/server ./cmd/server

server-test:
	cd apps/server && go test ./...

server-lint:
	cd apps/server && golangci-lint run ./...

migrate:
	@echo "TODO: implement with goose or golang-migrate in M1+"
```

- [ ] **Step 2: Verify Makefile syntax**

Run: `make -n server-dev`
Expected: Prints the `cd apps/server && go run ./cmd/server` command without executing

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile with server build targets"
```

---

### Task 3: CLAUDE.md

**Files:**
- Create: `CLAUDE.md`

- [ ] **Step 1: Create CLAUDE.md**

```markdown
# ClipAnvil 影砧

营销视频生成平台，Studio（工作流画布）+ Agent（对话驱动）双模式。

## 技术栈

- 前端：Vite 8 + React 19 + TypeScript 6 + React Flow + TailwindCSS 4
- 后端：Go 1.26 + Hertz + pgx v5 + sqlc + viper + slog
- 中间件：PostgreSQL 16 / Redis 7 / MinIO
- 容器：Docker Compose

## 项目结构

- `apps/web/` — 前端
- `apps/server/` — 后端（Go module: github.com/sinmaystar/clip-anvil）
- `packages/shared-types/` — TS 类型定义
- `apps/web/src/components/canvas-flow/` — 画布 Node/Edge 契约
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
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "chore: add CLAUDE.md project context for AI agents"
```

---

### Task 4: Hooks — lefthook + commitlint + Claude Code

**Files:**
- Create: `lefthook.yml`
- Create: `commitlint.config.cjs`
- Create: `.claude/settings.json`

- [ ] **Step 1: Create lefthook.yml**

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

commit-msg:
  commands:
    commitlint:
      run: pnpm commitlint --edit {1}
```

- [ ] **Step 2: Create commitlint.config.cjs**

```js
module.exports = { extends: ['@commitlint/config-conventional'] };
```

- [ ] **Step 3: Install lefthook git hooks**

Run: `lefthook install`
Expected: Output like `SYNCED` indicating hooks are registered in `.git/hooks/`

- [ ] **Step 4: Create .claude/settings.json**

Merge into existing `.claude/settings.json` (preserving existing permissions):

```json
{
  "permissions": {
    "allow": [
      "Bash(docker --version)",
      "Bash(docker machine *)"
    ]
  },
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

- [ ] **Step 5: Verify lefthook is active**

Run: `lefthook run pre-commit`
Expected: Runs without error (no staged files, so commands skip gracefully)

- [ ] **Step 6: Verify commitlint**

Run: `echo "bad message" | pnpm commitlint`
Expected: Error output indicating the message doesn't match conventional commits format

- [ ] **Step 7: Commit**

```bash
git add lefthook.yml commitlint.config.cjs .claude/settings.json
git commit -m "chore: configure lefthook, commitlint, and Claude Code hooks"
```

---

### Task 5: Deploy — Compose + Nginx

**Files:**
- Create: `deploy/docker-compose.yml`
- Create: `deploy/.env.example`
- Create: `deploy/.env`
- Create: `deploy/nginx/dev.conf`
- Create: `deploy/nginx/default.conf`
- Create: `deploy/init/postgres/001_init.sql`

- [ ] **Step 1: Create deploy/docker-compose.yml**

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

- [ ] **Step 2: Create deploy/.env.example and deploy/.env**

Both files with the same content:

```
PG_PASSWORD=clipanvil_dev
MINIO_USER=clipanvil
MINIO_PASSWORD=clipanvil_dev
```

- [ ] **Step 3: Create deploy/nginx/dev.conf**

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

- [ ] **Step 4: Create deploy/nginx/default.conf (prod placeholder)**

```nginx
server {
    listen 80;

    root /usr/share/nginx/html;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location /api/ {
        proxy_pass http://server:8888;
    }

    location /ws/ {
        proxy_pass http://server:8888;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

- [ ] **Step 5: Create deploy/init/postgres/001_init.sql**

```sql
-- M0: placeholder init script
-- Tables will be added in subsequent milestones via migration tool
```

- [ ] **Step 6: Start containers**

Run: `docker compose -f deploy/docker-compose.yml up -d`
Expected: All 4 containers start (postgres, redis, minio, nginx)

- [ ] **Step 7: Verify all containers are healthy**

Run: `docker compose -f deploy/docker-compose.yml ps`
Expected: 4 containers listed, all in "Up" state

Run: `docker exec deploy-postgres-1 pg_isready -U clipanvil`
Expected: "accepting connections"

Run: `docker exec deploy-redis-1 redis-cli ping`
Expected: "PONG"

Run: `curl -s http://localhost:9000/minio/health/live`
Expected: HTTP 200

Run: `curl -s -o /dev/null -w "%{http_code}" http://localhost`
Expected: 502 (nginx is up but upstream not yet running — this is correct for now)

- [ ] **Step 8: Commit**

```bash
git add deploy/
git commit -m "chore: add docker compose with postgres, redis, minio, nginx"
```

---

### Task 6: Go Module Init + Config Loader

**Files:**
- Create: `apps/server/go.mod`
- Create: `apps/server/internal/config/config.go`
- Create: `apps/server/config.yaml`

- [ ] **Step 1: Initialize Go module**

Run: `cd apps/server && go mod init github.com/sinmaystar/clip-anvil`
Expected: Creates `go.mod` with `module github.com/sinmaystar/clip-anvil`

- [ ] **Step 2: Create apps/server/config.yaml**

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

- [ ] **Step 3: Create apps/server/internal/config/config.go**

```go
package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	MinIO    MinIOConfig
}

type ServerConfig struct {
	Port int
}

type PostgresConfig struct {
	DSN string
}

type RedisConfig struct {
	Addr string
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	UseSSL    bool   `mapstructure:"use_ssl"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("../..")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
```

- [ ] **Step 4: Install Go dependencies**

Run: `cd apps/server && go get github.com/spf13/viper`
Expected: go.mod updated with viper dependency

- [ ] **Step 5: Verify compilation**

Run: `cd apps/server && go build ./internal/config/`
Expected: Builds without error

- [ ] **Step 6: Commit**

```bash
git add apps/server/go.mod apps/server/go.sum apps/server/config.yaml apps/server/internal/config/
git commit -m "feat(server): init go module with viper config loader"
```

---

### Task 7: Hertz Server + /api/health with Middleware Connections

**Files:**
- Create: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Install Go dependencies**

Run:
```bash
cd apps/server && go get github.com/cloudwego/hertz/pkg/app \
  github.com/cloudwego/hertz/pkg/app/server \
  github.com/cloudwego/hertz/pkg/protocol/consts \
  github.com/jackc/pgx/v5/pgxpool \
  github.com/redis/go-redis/v9 \
  github.com/minio/minio-go/v7
```
Expected: go.mod updated with all dependencies

- [ ] **Step 2: Create apps/server/cmd/server/main.go**

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"

	"github.com/sinmaystar/clip-anvil/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pgPool, err := pgxpool.New(ctx, cfg.Postgres.DSN)
	if err != nil {
		slog.Error("failed to create postgres pool", "error", err)
		os.Exit(1)
	}
	if err := pgPool.Ping(ctx); err != nil {
		slog.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	slog.Info("postgres connected")
	defer pgPool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	slog.Info("redis connected")
	defer rdb.Close()

	minioClient, err := minio.New(cfg.MinIO.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIO.AccessKey, cfg.MinIO.SecretKey, ""),
		Secure: cfg.MinIO.UseSSL,
	})
	if err != nil {
		slog.Error("failed to create minio client", "error", err)
		os.Exit(1)
	}
	// Verify minio connectivity by listing buckets
	if _, err := minioClient.ListBuckets(ctx); err != nil {
		slog.Error("failed to connect to minio", "error", err)
		os.Exit(1)
	}
	slog.Info("minio connected")

	h := server.Default(server.WithHostPorts(fmt.Sprintf(":%d", cfg.Server.Port)))

	h.GET("/api/health", func(ctx context.Context, c *app.RequestContext) {
		pgStatus := "connected"
		if err := pgPool.Ping(ctx); err != nil {
			pgStatus = "disconnected"
		}

		redisStatus := "connected"
		if err := rdb.Ping(ctx).Err(); err != nil {
			redisStatus = "disconnected"
		}

		minioStatus := "connected"
		if _, err := minioClient.ListBuckets(ctx); err != nil {
			minioStatus = "disconnected"
		}

		status := "ok"
		if pgStatus != "connected" || redisStatus != "connected" || minioStatus != "connected" {
			status = "degraded"
		}

		c.JSON(consts.StatusOK, map[string]any{
			"status": status,
			"services": map[string]string{
				"postgres": pgStatus,
				"redis":    redisStatus,
				"minio":    minioStatus,
			},
		})
	})

	slog.Info("server starting", "port", cfg.Server.Port)
	h.Spin()
}
```

- [ ] **Step 3: Tidy Go modules**

Run: `cd apps/server && go mod tidy`
Expected: go.sum updated, no errors

- [ ] **Step 4: Verify compilation**

Run: `cd apps/server && go build ./cmd/server/`
Expected: Builds without error

- [ ] **Step 5: Start server and verify health endpoint**

Prerequisite: Containers from Task 5 must be running (`docker compose -f deploy/docker-compose.yml ps`)

Run: `make server-dev` (in background, or in a separate terminal)

Run (in another terminal): `curl -s http://localhost:8888/api/health | python3 -m json.tool`
Expected:
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

Run: `curl -s http://localhost/api/health | python3 -m json.tool`
Expected: Same response (through nginx proxy)

- [ ] **Step 6: Stop the server (Ctrl+C) and commit**

```bash
git add apps/server/
git commit -m "feat(server): add Hertz server with health endpoint and middleware connections"
```

---

### Task 8: Frontend — Vite + React + React Flow

**Files:**
- Create: `apps/web/package.json`
- Create: `apps/web/tsconfig.json`
- Create: `apps/web/vite.config.ts`
- Create: `apps/web/index.html`
- Create: `apps/web/src/main.tsx`
- Create: `apps/web/src/App.tsx`
- Create: `apps/web/src/App.css`

- [ ] **Step 1: Create apps/web/package.json**

```json
{
  "name": "@clip-anvil/web",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview",
    "lint": "eslint ."
  },
  "dependencies": {
    "react": "^19",
    "react-dom": "^19",
    "/react": "^12"
  },
  "devDependencies": {
    "@types/react": "^19",
    "@types/react-dom": "^19",
    "@vitejs/plugin-react": "^4",
    "eslint": "^10",
    "typescript": "^6",
    "vite": "^8"
  }
}
```

- [ ] **Step 2: Create apps/web/tsconfig.json**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true
  },
  "include": ["src"]
}
```

- [ ] **Step 3: Create apps/web/vite.config.ts**

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    host: true,
  },
});
```

- [ ] **Step 4: Create apps/web/index.html**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>ClipAnvil 影砧</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 5: Create apps/web/src/main.tsx**

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

- [ ] **Step 6: Create apps/web/src/App.tsx**

```tsx
import { ReactFlow } from "/react";
import "/react/dist/style.css";

export default function App() {
  return (
    <div style={{ position: "fixed", inset: 0 }}>
      <React Flow />
    </div>
  );
}
```

- [ ] **Step 7: Create apps/web/src/App.css (empty, reserved for global styles)**

```css
/* global styles */
```

- [ ] **Step 8: Install dependencies**

Run: `pnpm install`
Expected: All dependencies installed, no peer dependency errors

- [ ] **Step 9: Verify TypeScript compilation**

Run: `cd apps/web && npx tsc -b`
Expected: No errors

- [ ] **Step 10: Start dev server and verify**

Run: `pnpm --filter @clip-anvil/web dev` (background or separate terminal)

Run: `curl -s -o /dev/null -w "%{http_code}" http://localhost:5173`
Expected: 200

Run: `curl -s -o /dev/null -w "%{http_code}" http://localhost`
Expected: 200 (through nginx proxy)

Open `http://localhost` in browser — should see a full-screen React Flow canvas with default tools (select, draw, nodes, etc.)

- [ ] **Step 11: Stop dev server and commit**

```bash
git add apps/web/
git commit -m "feat(web): add Vite + React + React Flow empty canvas"
```

---

### Task 9: Shared Packages (Empty Shells)

**Files:**
- Create: `packages/shared-types/package.json`
- Create: `packages/shared-types/src/index.ts`
- Create: `apps/web/src/components/canvas-flow/package.json`
- Create: `apps/web/src/components/canvas-flow/flowTypes.ts`
- Create: `packages/eslint-config/package.json`
- Create: `packages/eslint-config/index.cjs`

- [ ] **Step 1: Create packages/shared-types/package.json**

```json
{
  "name": "@clip-anvil/shared-types",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "main": "./src/index.ts",
  "types": "./src/index.ts"
}
```

- [ ] **Step 2: Create packages/shared-types/src/index.ts**

```ts
export {};
```

- [ ] **Step 3: Create apps/web/src/components/canvas-flow/package.json**

```json
{
  "name": "@clip-anvil/canvas-schema",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "main": "./src/index.ts",
  "types": "./src/index.ts"
}
```

- [ ] **Step 4: Create apps/web/src/components/canvas-flow/flowTypes.ts**

```ts
export {};
```

- [ ] **Step 5: Create packages/eslint-config/package.json**

```json
{
  "name": "@clip-anvil/eslint-config",
  "private": true,
  "version": "0.0.0",
  "main": "./index.cjs"
}
```

- [ ] **Step 6: Create packages/eslint-config/index.cjs**

```js
module.exports = {
  rules: {},
};
```

- [ ] **Step 7: Run pnpm install to register workspace packages**

Run: `pnpm install`
Expected: Lockfile updated, workspace packages linked

- [ ] **Step 8: Commit**

```bash
git add packages/
git commit -m "chore: add shared-types, canvas-schema, eslint-config package shells"
```

---

### Task 10: Final End-to-End Verification

No new files. This task verifies all acceptance criteria from the spec.

- [ ] **Step 1: Verify containers are running**

Run: `docker compose -f deploy/docker-compose.yml ps`
Expected: 4 containers (postgres, redis, minio, nginx) all in "Up" state

- [ ] **Step 2: Start backend**

Run: `make server-dev` (background or separate terminal)
Expected: Logs show "postgres connected", "redis connected", "minio connected", "server starting"

- [ ] **Step 3: Verify /api/health through nginx**

Run: `curl -s http://localhost/api/health | python3 -m json.tool`
Expected:
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

- [ ] **Step 4: Start frontend**

Run: `pnpm --filter @clip-anvil/web dev` (background or separate terminal)
Expected: Vite dev server starts on :5173

- [ ] **Step 5: Verify React Flow canvas through nginx**

Open `http://localhost` in browser.
Expected: Full-screen React Flow canvas with default tools visible.

- [ ] **Step 6: Verify pre-commit hook**

Create a test Go file with bad formatting, stage it, and attempt to commit:

```bash
echo 'package main
func main(){fmt.Println("test")}' > apps/server/test_hook.go
git add apps/server/test_hook.go
git commit -m "test: verify hooks"
```

Expected: pre-commit hook runs gofmt and golangci-lint. The file should be auto-formatted. If lint passes after formatting, commit succeeds. Clean up:

```bash
rm apps/server/test_hook.go
git checkout -- . 2>/dev/null; git reset HEAD -- apps/server/test_hook.go 2>/dev/null
```

- [ ] **Step 7: Verify commitlint hook**

```bash
touch /tmp/test-commitlint
git add /tmp/test-commitlint 2>/dev/null || true
echo "bad message" | pnpm commitlint
```

Expected: commitlint rejects the message with an error about format.

- [ ] **Step 8: Stop all services, clean up**

Stop the Go server (Ctrl+C).
Stop the Vite dev server (Ctrl+C).
Containers stay running in the background (as designed).

All 7 acceptance criteria verified. M0 is complete.
