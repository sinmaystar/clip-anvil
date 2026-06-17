# M1 Studio 画布基础 — 里程碑规格

**状态**：✅ 已落地（2026-06-13，核心验收通过）
**前置**：M0 基建（✅ 已完成）
**目标**：打通注册登录 → 项目管理 → 画布渲染 → 节点创建/持久化的完整前后端链路

## 1. 里程碑范围

M1 在 M0 骨架基础上交付 4 个可独立验收的阶段：

| 阶段 | 交付 | 端到端验证 |
|---|---|---|
| 1. 基础设施 | goose 迁移 + sqlc 代码生成 + 配置扩展 | `make migrate-up && make sqlc-generate && make server-build` 全部通过 |
| 2. 注册登录 | 后端 auth API + 前端登录/注册页 + 路由守卫 | 注册账号 → 拿到 token → 访问受保护 API |
| 3. Workspace | 后端 CRUD API + 前端列表/创建页 | 创建项目 → 列表可见 → 点击进入 |
| 4. 画布 + 文本节点 | 后端 canvas/node API + 前端 MediaShape + 位置持久化 | 创建节点 → 拖拽 → 刷新后位置保持 |

**不在范围内**（留后续迭代）：
- WebSocket 事件推送
- image / video / audio 节点类型
- 连线（MediaEdge）、分组（MediaGroup）
- 完整左侧资源树、右侧属性面板、浮动工具栏
- 生成任务（GenerationJob）、版本管理（ArtifactVersion）
- Agent 模式

**M1 实际交互补充**：
- 画布页已实现轻量左侧栏，用于项目返回、主题切换、媒体节点列表和折叠展开；完整资源树仍留给 M1.x。
- 已隐藏 tldraw 原生顶部/底部/右侧工具 UI，并禁用绘图类快捷键；保留选择、删除、复制、撤销等必要快捷键。
- 创建节点入口改为画布右键菜单，不再使用固定“+ 文本节点”导航按钮。
- 节点编辑改为单击节点后在节点下方显示内联编辑面板，支持标题、引用占位、Prompt 和模型选择；标题/Prompt 自动保存，点击画布其他区域隐藏。
- 画布支持明亮/暗夜外观切换。

## 2. 阶段 1：基础设施

### 2.1 数据库迁移

工具：goose（SQL-first，与 sqlc 配合最自然）。

创建 `apps/server/migrations/001_init_schema.sql`，包含：

**枚举类型**：

```sql
CREATE TYPE media_type  AS ENUM ('text', 'image', 'video', 'audio');
CREATE TYPE node_status AS ENUM ('draft', 'ready', 'queued', 'running',
                                 'succeeded', 'failed', 'stale', 'user_editing');
```

**account 表**：

```sql
CREATE TABLE account (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL,
    avatar_url    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**workspace 表**：

```sql
CREATE TABLE workspace (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    owner_id   UUID NOT NULL REFERENCES account(id),
    settings   JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**canvas_document 表**：

```sql
CREATE TABLE canvas_document (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id   UUID NOT NULL UNIQUE REFERENCES workspace(id) ON DELETE CASCADE,
    camera_x       REAL NOT NULL DEFAULT 0,
    camera_y       REAL NOT NULL DEFAULT 0,
    camera_zoom    REAL NOT NULL DEFAULT 1,
    layout_version INT NOT NULL DEFAULT 1,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**media_node 表**（M1 精简版，不含 group_id / asset_id / current_version_id 等 FK）：

```sql
CREATE TABLE media_node (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    node_type    media_type NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    status       node_status NOT NULL DEFAULT 'draft',
    prompt       TEXT NOT NULL DEFAULT '',
    source       TEXT NOT NULL DEFAULT 'user',
    canvas_x     REAL NOT NULL DEFAULT 0,
    canvas_y     REAL NOT NULL DEFAULT 0,
    canvas_w     REAL NOT NULL DEFAULT 200,
    canvas_h     REAL NOT NULL DEFAULT 120,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**索引**：

```sql
CREATE INDEX idx_account_email ON account(email);
CREATE INDEX idx_workspace_owner ON workspace(owner_id);
CREATE INDEX idx_media_node_workspace ON media_node(workspace_id);
CREATE INDEX idx_media_node_status ON media_node(workspace_id, status);
```

### 2.2 sqlc 配置

创建 `apps/server/sqlc.yaml`：

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "sqlc/queries"
    schema: "migrations"
    gen:
      go:
        package: "db"
        out: "internal/store/db"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_empty_slices: true
```

Query 文件：

- `sqlc/queries/account.sql`：
  - `CreateAccount` — INSERT 返回完整行
  - `GetAccountByEmail` — SELECT WHERE email
  - `GetAccountByID` — SELECT WHERE id

- `sqlc/queries/workspace.sql`：
  - `CreateWorkspace` — INSERT 返回完整行
  - `ListWorkspacesByOwner` — SELECT WHERE owner_id ORDER BY created_at DESC
  - `GetWorkspaceByID` — SELECT WHERE id

- `sqlc/queries/canvas.sql`：
  - `CreateCanvasDocument` — INSERT 返回完整行
  - `GetCanvasDocumentByWorkspace` — SELECT WHERE workspace_id
  - `UpdateCamera` — UPDATE camera_x/y/zoom WHERE workspace_id

- `sqlc/queries/node.sql`：
  - `CreateMediaNode` — INSERT 返回完整行
  - `ListMediaNodesByWorkspace` — SELECT WHERE workspace_id ORDER BY created_at
  - `GetMediaNodeByID` — SELECT WHERE id
  - `UpdateMediaNode` — UPDATE title/prompt/status WHERE id（部分更新，sqlc 用 CASE/COALESCE 或多个 query）
  - `UpdateMediaNodePosition` — UPDATE canvas_x/y WHERE id
  - `DeleteMediaNode` — DELETE WHERE id

### 2.3 配置扩展

`config.yaml` 新增：

```yaml
jwt:
  secret: "dev-secret-change-in-prod"
  expire_hours: 72
```

`config.go` 新增：

```go
type JWTConfig struct {
    Secret      string
    ExpireHours int `mapstructure:"expire_hours"`
}
```

Config struct 加 `JWT JWTConfig` 字段。

### 2.4 Makefile 补充

```makefile
migrate-up:
	cd apps/server && goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	cd apps/server && goose -dir migrations postgres "$$DATABASE_URL" down

migrate-create:
	cd apps/server && goose -dir migrations create $(name) sql

sqlc-generate:
	cd apps/server && sqlc generate
```

`DATABASE_URL` 默认值与 `config.yaml` 中的 DSN 一致。

### 2.5 Go 依赖新增

```
golang-jwt/jwt/v5   — JWT 签发/校验
golang.org/x/crypto — bcrypt（标准库已含，确认引入）
pressly/goose/v3    — 迁移工具 CLI（已在 go.sum 中，需确认 go.mod require）
sqlc                — 开发工具，不进 go.mod
```

### 2.6 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 1.1 | 迁移执行成功 | `make migrate-up` | 退出码 0，4 张表 + 2 个枚举创建 |
| 1.2 | sqlc 生成成功 | `make sqlc-generate` | `internal/store/db/` 下生成 `.go` 文件 |
| 1.3 | 编译通过 | `make server-build` | `bin/server` 生成，退出码 0 |
| 1.4 | 迁移回滚成功 | `make migrate-down` | 退出码 0，表被删除 |

## 3. 阶段 2：注册登录

### 3.1 后端 auth 模块

文件位于 `internal/auth/`：

**`jwt.go`**：
- `SignToken(accountID uuid.UUID, secret string, expireHours int) (string, error)` — 签发 JWT，Claims: `{sub: accountID, exp: now+expireHours}`
- `VerifyToken(tokenStr string, secret string) (uuid.UUID, error)` — 校验并返回 account_id

依赖 `golang-jwt/jwt/v5`，签名算法 HS256。

**`password.go`**：
- `HashPassword(password string) (string, error)` — bcrypt hash，cost 默认 10
- `CheckPassword(password, hash string) bool` — bcrypt compare

**`middleware.go`**：
- Hertz 中间件函数，从 `Authorization: Bearer <token>` 提取 token
- 校验成功 → `c.Set("account_id", accountID)`，继续
- 校验失败 / 无 header → 返回 `401 {"error": "unauthorized"}`

### 3.2 后端 API 端点

| 端点 | 方法 | 鉴权 | 请求体 | 成功响应 | 错误响应 |
|---|---|---|---|---|---|
| `/api/auth/register` | POST | 无 | `{email, password, name}` | `200 {token, account: {id, email, name}}` | `400` 参数无效 / `409` 邮箱已注册 |
| `/api/auth/login` | POST | 无 | `{email, password}` | `200 {token, account: {id, email, name}}` | `401` 邮箱或密码错误 |
| `/api/auth/me` | GET | JWT | — | `200 {id, email, name, avatar_url}` | `401` 未认证 |

**校验规则**：
- `email`：非空 + 合法格式（contains `@` and `.`）
- `password`：最少 6 位
- `name`：非空
- 注册成功后直接签发 token 返回，无需二次登录

**Handler 实现**（`internal/api/auth_handler.go`）：
- 接收请求 → 参数校验 → 调 sqlc 生成的 store 方法 → 返回 JSON
- 注册：hash 密码 → CreateAccount → SignToken → 返回
- 登录：GetAccountByEmail → CheckPassword → SignToken → 返回

### 3.3 后端路由注册

`main.go` 中注册路由，结构：

```go
authGroup := h.Group("/api/auth")
authGroup.POST("/register", authHandler.Register)
authGroup.POST("/login", authHandler.Login)
authGroup.GET("/me", authMiddleware, authHandler.Me)
```

后续阶段的路由（workspace、nodes）都套 `authMiddleware`。

### 3.4 前端基础设施

**安装依赖**：

```bash
pnpm --filter @clip-anvil/web add react-router zustand @tanstack/react-query
pnpm --filter @clip-anvil/web add -D tailwindcss @tailwindcss/vite
```

**TailwindCSS 4 配置**：
- `vite.config.ts` 添加 `@tailwindcss/vite` 插件
- `src/main.css` 添加 `@import "tailwindcss"`
- 删除 `App.css`

**TanStack Query 配置**：
- `src/main.tsx` 中创建 `QueryClient` 并包裹 `<QueryClientProvider>`
- 封装 `src/lib/api.ts`：基础 fetch 封装，自动注入 `Authorization` header

**Zustand auth store**（`src/stores/auth.ts`）：

```typescript
interface AuthState {
  token: string | null
  account: { id: string; email: string; name: string } | null
  login: (token: string, account: Account) => void
  logout: () => void
}
```

- `login()` — 写 state + localStorage
- `logout()` — 清 state + localStorage + 跳转 /login
- 初始化时从 localStorage 恢复 token

### 3.5 前端路由

使用 React Router v7：

```
/login        — 登录页（未认证时默认页）
/register     — 注册页
/workspaces   — 项目列表（需认证，阶段 3 实现）
/workspaces/:id — 画布页（需认证，阶段 4 实现）
```

**路由守卫**（`src/components/ProtectedRoute.tsx`）：
- 检查 auth store 的 token
- 无 token → `<Navigate to="/login" />`
- 有 token → 渲染子路由

**反向守卫**（`src/components/GuestRoute.tsx`）：
- 有 token → `<Navigate to="/workspaces" />`
- 无 token → 渲染登录/注册页

### 3.6 前端页面

**登录页** `/login`：
- 表单：邮箱输入框 + 密码输入框 + "登录"按钮
- 底部链接："没有账号？注册"→ `/register`
- 提交：POST `/api/auth/login` → 成功：store.login() + navigate('/workspaces') → 失败：表单下方红色错误提示
- 页面居中布局，简洁卡片风格

**注册页** `/register`：
- 表单：邮箱 + 密码 + 昵称 + "注册"按钮
- 底部链接："已有账号？登录"→ `/login`
- 提交：POST `/api/auth/register` → 成功：store.login() + navigate('/workspaces') → 失败：表单下方红色错误提示
- 布局与登录页一致

**样式**：遵循 `docs/design-frontend.md` 视觉契约，使用 `apps/web/src/main.css` 中的 token 和 `auth-*` class；不引入 shadcn/ui。

### 3.7 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 2.1 | 注册返回 token | `curl -s -X POST localhost:8888/api/auth/register -H 'Content-Type: application/json' -d '{"email":"t@t.com","password":"123456","name":"T"}' \| jq -e '.token'` | 输出非空字符串，退出码 0 |
| 2.2 | 重复注册返回 409 | `curl -s -o /dev/null -w '%{http_code}' -X POST localhost:8888/api/auth/register -H 'Content-Type: application/json' -d '{"email":"t@t.com","password":"123456","name":"T"}'` | 输出 `409` |
| 2.3 | 登录返回 token | `curl -s -X POST localhost:8888/api/auth/login -H 'Content-Type: application/json' -d '{"email":"t@t.com","password":"123456"}' \| jq -e '.token'` | 输出非空字符串，退出码 0 |
| 2.4 | 错误密码返回 401 | `curl -s -o /dev/null -w '%{http_code}' -X POST localhost:8888/api/auth/login -H 'Content-Type: application/json' -d '{"email":"t@t.com","password":"wrong"}'` | 输出 `401` |
| 2.5 | /me 带 token 返回用户 | `TOKEN=$(...login...); curl -s localhost:8888/api/auth/me -H "Authorization: Bearer $TOKEN" \| jq -e '.email'` | 输出 `"t@t.com"` |
| 2.6 | /me 无 token 返回 401 | `curl -s -o /dev/null -w '%{http_code}' localhost:8888/api/auth/me` | 输出 `401` |
| 2.7 | 参数校验 | `curl -s -o /dev/null -w '%{http_code}' -X POST localhost:8888/api/auth/register -H 'Content-Type: application/json' -d '{"email":"","password":"12","name":""}'` | 输出 `400` |
| 2.8 | 后端单测通过 | `make server-test` | 退出码 0 |
| 2.9 | 前端编译通过 | `pnpm --filter @clip-anvil/web build` | 退出码 0 |
| 2.10 | 前端路由守卫 | 浏览器打开 `localhost:5173/workspaces` 未登录状态 | 自动跳转到 `/login` |

## 4. 阶段 3：Workspace 管理

### 4.1 后端 API 端点

| 端点 | 方法 | 鉴权 | 请求体 | 成功响应 | 错误响应 |
|---|---|---|---|---|---|
| `/api/workspaces` | POST | JWT | `{name}` | `200 {id, name, owner_id, created_at}` | `400` name 为空 |
| `/api/workspaces` | GET | JWT | — | `200 [{id, name, owner_id, created_at, updated_at}]` | — |
| `/api/workspaces/:id` | GET | JWT | — | `200 {id, name, owner_id, settings, created_at, updated_at}` | `403` 非所有者 / `404` 不存在 |

**业务规则**：
- 创建 workspace 在单个事务中完成：`INSERT workspace` + `INSERT canvas_document`（初始 camera 0,0,1）
- GET 列表只返回 `owner_id = 当前用户` 的 workspace
- GET 详情校验 `owner_id = 当前用户`，不匹配返回 403

**Handler 实现**（`internal/api/workspace_handler.go`）：
- 从 request context 取 `account_id`（中间件写入）
- 创建：参数校验 → 事务（CreateWorkspace + CreateCanvasDocument）→ 返回
- 列表：ListWorkspacesByOwner(account_id) → 返回
- 详情：GetWorkspaceByID → 校验 owner → 返回

### 4.2 后端路由注册

```go
wsGroup := h.Group("/api/workspaces", authMiddleware)
wsGroup.POST("/", wsHandler.Create)
wsGroup.GET("/", wsHandler.List)
wsGroup.GET("/:id", wsHandler.Get)
```

### 4.3 前端页面

**Workspace 列表页** `/workspaces`：

布局：
```
┌──────────────────────────────────────────┐
│  影砧                    [用户名] [登出]  │
├──────────────────────────────────────────┤
│                                          │
│  我的项目                   [+ 新建项目]  │
│                                          │
│  ┌──────────┐ ┌──────────┐              │
│  │ 咖啡广告  │ │ 品牌故事  │              │
│  │ 6/12创建  │ │ 6/11创建  │              │
│  └──────────┘ └──────────┘              │
│                                          │
│  （空状态：还没有项目，点击上方按钮创建）   │
└──────────────────────────────────────────┘
```

- 数据获取：`useQuery(['workspaces'], fetchWorkspaces)`
- 卡片网格（`grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4`）
- 每张卡片显示名称 + 创建时间
- 点击卡片 → `navigate(/workspaces/:id)`
- 空状态居中提示

**新建项目 Dialog**：
- 点击"新建项目"按钮 → 弹出 modal overlay
- 输入项目名称 + 确认/取消按钮
- `useMutation(createWorkspace, { onSuccess: () => navigate(新 workspace 的路径) })`

**顶部导航条**（全局 layout 组件）：
- 左侧：产品名"影砧"
- 右侧：用户昵称 + 登出按钮（调 store.logout()）

### 4.4 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 3.1 | 创建 workspace | `curl -s -X POST localhost:8888/api/workspaces -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"name":"咖啡广告"}' \| jq -e '.id'` | 输出 UUID |
| 3.2 | 自动创建 canvas_document | `WID=$(...); curl -s localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq -e '.camera'` | 输出 camera 对象 |
| 3.3 | 列表返回当前用户的 workspace | `curl -s localhost:8888/api/workspaces -H "Authorization: Bearer $TOKEN" \| jq 'length'` | 输出 >= 1 |
| 3.4 | 详情返回完整数据 | `curl -s localhost:8888/api/workspaces/$WID -H "Authorization: Bearer $TOKEN" \| jq -e '.name'` | 输出 `"咖啡广告"` |
| 3.5 | 非所有者访问返回 403 | `curl -s -o /dev/null -w '%{http_code}' localhost:8888/api/workspaces/$WID -H "Authorization: Bearer $OTHER_TOKEN"` | 输出 `403` |
| 3.6 | name 为空返回 400 | `curl -s -o /dev/null -w '%{http_code}' -X POST localhost:8888/api/workspaces -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"name":""}'` | 输出 `400` |
| 3.7 | 后端单测通过 | `make server-test` | 退出码 0 |
| 3.8 | 前端编译通过 | `pnpm --filter @clip-anvil/web build` | 退出码 0 |

## 5. 阶段 4：画布 + 文本节点

### 5.1 后端 API 端点

| 端点 | 方法 | 鉴权 | 请求体 | 成功响应 | 错误响应 |
|---|---|---|---|---|---|
| `/api/workspaces/:id/canvas` | GET | JWT | — | `200 {camera, nodes}` | `403` / `404` |
| `/api/nodes` | POST | JWT | `{workspace_id, node_type, title, canvas_x, canvas_y}` | `200 {完整节点}` | `400` / `403` |
| `/api/nodes/:id` | GET | JWT | — | `200 {完整节点}` | `403` / `404` |
| `/api/nodes/:id` | PATCH | JWT | `{title?, prompt?, status?}` | `200 {更新后节点}` | `403` / `404` |
| `/api/nodes/:id` | DELETE | JWT | — | `204` | `403` / `404` |
| `/api/nodes/batch-position` | PATCH | JWT | `{positions: [{id, canvas_x, canvas_y}]}` | `204` | `403` |
| `/api/workspaces/:id/camera` | PATCH | JWT | `{x, y, zoom}` | `204` | `403` / `404` |

**业务规则**：
- Canvas GET 返回该 workspace 的 camera + 所有 media_node（M1 不返回 edges/groups）
- POST 创建节点：`node_type` M1 只允许 `text`，`status` 默认 `draft`，`source` 默认 `user`
- `canvas_w/h` 按 node_type 设默认值（text: 200×120）
- 所有节点操作校验节点所属 workspace 归属当前用户
- batch-position 只更新 canvas_x/y
- 删除为硬删除（M1 无连线依赖，不需级联）

**Handler 实现**（`internal/api/canvas_handler.go` + `internal/api/node_handler.go`）：
- canvas GET：GetCanvasDocumentByWorkspace + ListMediaNodesByWorkspace → 组装返回
- node PATCH（部分更新）：只更新请求体中包含的字段，未传字段保持不变

**路由注册**：

```go
wsGroup.GET("/:id/canvas", canvasHandler.GetCanvas)
wsGroup.PATCH("/:id/camera", canvasHandler.UpdateCamera)

nodeGroup := h.Group("/api/nodes", authMiddleware)
nodeGroup.POST("/", nodeHandler.Create)
nodeGroup.GET("/:id", nodeHandler.Get)
nodeGroup.PATCH("/batch-position", nodeHandler.BatchUpdatePosition)
nodeGroup.PATCH("/:id", nodeHandler.Update)
nodeGroup.DELETE("/:id", nodeHandler.Delete)
```

注意 `batch-position` 路由注册在 `/:id` 之前，避免路由冲突。

### 5.2 前端自定义 MediaShape

**类型定义**（`packages/canvas-schema/src/index.ts`）：

```typescript
export type MediaType = 'text' | 'image' | 'video' | 'audio'
export type NodeStatus = 'draft' | 'ready' | 'queued' | 'running'
  | 'succeeded' | 'failed' | 'stale' | 'user_editing'

export interface MediaShapeProps {
  w: number
  h: number
  nodeId: string
  nodeType: MediaType
  title: string
  status: NodeStatus
  prompt: string
}
```

**ShapeUtil 实现**（`apps/web/src/shapes/MediaShapeUtil.tsx`）：

- `extends ShapeUtil<MediaShape>`
- `static type = 'media'`
- `getDefaultProps()` — `{ w: 200, h: 120, nodeId: '', nodeType: 'text', title: '', status: 'draft', prompt: '' }`
- `component()` — 渲染节点卡片：
  - 头部：文本类型标识 + 标题文本 + 右侧状态色块（圆点 + 状态颜色）
  - 内容区：prompt 文本预览（前 3 行，overflow hidden）
  - 状态色块颜色：draft 灰、ready 灰、running 蓝、succeeded 绿、failed 红、stale 黄
- `getIndicatorPath()` — 返回矩形选中边框路径
- 尺寸固定 200×120（M1 不支持 resize）
- 不使用 tldraw 内置文本编辑模式（M1 用节点下方内联编辑面板）

**nodeToShape 映射函数**（`apps/web/src/lib/canvas.ts`）：

```typescript
function nodeToShape(node: MediaNodeDTO): TLShapePartial<MediaShape> {
  return {
    id: createShapeId(node.id),
    type: 'media',
    x: node.canvasX,
    y: node.canvasY,
    props: {
      w: node.canvasW,
      h: node.canvasH,
      nodeId: node.id,
      nodeType: node.nodeType,
      title: node.title,
      status: node.status,
      prompt: node.prompt,
    },
  }
}
```

> 说明：API JSON 字段按 Go 项目惯例使用 `snake_case`（如 `canvas_x`、`canvas_w`、`node_type`）。前端 `api.ts` 负责把响应映射成 TypeScript 侧 camelCase DTO，`nodeToShape` 只消费 DTO。

### 5.3 前端画布页面

**页面** `/workspaces/:id`：

布局：
```
┌───────────────┬──────────────────────────┐
│ Studio 侧栏    │        tldraw 画布        │
│ 返回项目列表   │                          │
│ 外观切换       │   [右键菜单: 创建文本节点] │
│ 媒体节点列表   │                          │
│ 折叠/展开      │                          │
└───────────────┴──────────────────────────┘
```

**初始加载**：
1. `GET /api/workspaces/:id` → 获取 workspace 名称显示在顶部
2. `GET /api/workspaces/:id/canvas` → 获取 camera + nodes
3. `editor.createShapes(nodes.map(nodeToShape))`
4. `editor.setCamera({ x: camera.x, y: camera.y, z: camera.zoom })`

**创建节点交互**：
- 右键画布空白处 → 自定义 context menu（覆盖 tldraw 默认菜单）→ "文本节点"
- 点击 → `POST /api/nodes { workspace_id, node_type: 'text', title: '未命名文本', canvas_x: 右键位置.x, canvas_y: 右键位置.y }`
- 成功 → `editor.createShape(nodeToShape(response))`
- 失败 → toast 提示

**编辑标题/Prompt 交互**：
- 单击节点 → 节点下方显示内联编辑面板
- 面板上部展示引用资源占位，中部编辑 Prompt，底部选择模型
- 标题和 Prompt 自动保存：失焦或输入防抖后 `PATCH /api/nodes/:id { title, prompt }`
- 保存成功后同步 shape props，确保删除后 `Cmd+Z` 撤销能恢复标题和 Prompt
- 点击画布空白处或选择其他节点时隐藏编辑面板

**删除节点交互**：
- 选中节点 → 按 Delete/Backspace
- tldraw 删除 shape 后触发后端 `DELETE /api/nodes/:id`
- `Cmd+Z` 恢复节点时重新向后端创建/更新对应节点数据，保持左侧列表、画布和后端一致

### 5.4 位置持久化

监听 `editor.store.listen('change')`，过滤 MediaShape 的 x/y 变化：

- 收集变化的节点 ID + 新坐标
- 短防抖批量提交（当前实现约 500ms）
- `PATCH /api/nodes/batch-position { positions: [{id, canvas_x, canvas_y}, ...] }`
- 失败静默重试一次，仍失败忽略

Camera 变更同理：

- 监听 camera 变化
- 节流/短间隔持久化（当前实现约 800ms）
- `PATCH /api/workspaces/:id/camera { x, y, zoom }`
- 失败忽略

### 5.5 tldraw 配置

传入 `<Tldraw>` 的配置：

- `shapeUtils: [MediaShapeUtil]` — 注册自定义 shape
- `tools: []` — M1 不注册自定义工具
- 使用 `TLUiComponents` 将原生顶部工具栏、底部工具、右侧样式面板等置空
- 右键菜单由页面层自定义渲染，位置按画布坐标转换
- `options={{ enableToolbarKeyboardShortcuts: false }}` 并拦截 D/E 等绘图快捷键，避免用户进入画笔/橡皮模式

### 5.6 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 4.1 | 空画布加载 | `curl -s localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq '.nodes \| length'` | 输出 `0` |
| 4.2 | 创建文本节点 | `curl -s -X POST localhost:8888/api/nodes -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"workspace_id":"'$WID'","node_type":"text","title":"脚本","canvas_x":100,"canvas_y":200}' \| jq -e '.id'` | 输出 UUID |
| 4.3 | 画布包含节点 | `curl -s localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq '.nodes \| length'` | 输出 `1` |
| 4.4 | 节点默认值 | `curl -s localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq '.nodes[0].status'` | 输出 `"draft"` |
| 4.5 | 节点默认尺寸 | `curl -s localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq '.nodes[0].canvas_w'` | 输出 `200` |
| 4.6 | 更新节点 | `curl -s -X PATCH localhost:8888/api/nodes/$NID -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"title":"终稿","prompt":"内容"}' \| jq '.title'` | 输出 `"终稿"` |
| 4.7 | 部分更新不覆盖 | `curl -s -X PATCH localhost:8888/api/nodes/$NID -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"title":"新标题"}' \| jq '.prompt'` | 输出 `"内容"`（未被清空） |
| 4.8 | 批量更新位置 | `curl -s -o /dev/null -w '%{http_code}' -X PATCH localhost:8888/api/nodes/batch-position -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"positions":[{"id":"'$NID'","canvas_x":300,"canvas_y":400}]}'` | 输出 `204` |
| 4.9 | 位置已持久化 | `curl -s localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq '.nodes[0].canvas_x'` | 输出 `300` |
| 4.10 | 更新 camera | `curl -s -o /dev/null -w '%{http_code}' -X PATCH localhost:8888/api/workspaces/$WID/camera -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"x":50,"y":100,"zoom":1.5}'` | 输出 `204` |
| 4.11 | camera 已持久化 | `curl -s localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq '.camera.zoom'` | 输出 `1.5` |
| 4.12 | 删除节点 | `curl -s -o /dev/null -w '%{http_code}' -X DELETE localhost:8888/api/nodes/$NID -H "Authorization: Bearer $TOKEN"` | 输出 `204` |
| 4.13 | 删除后画布为空 | `curl -s localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq '.nodes \| length'` | 输出 `0` |
| 4.14 | 非 text 类型拒绝 | `curl -s -o /dev/null -w '%{http_code}' -X POST localhost:8888/api/nodes -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"workspace_id":"'$WID'","node_type":"video","title":"x","canvas_x":0,"canvas_y":0}'` | 输出 `400` |
| 4.15 | 后端单测通过 | `make server-test` | 退出码 0 |
| 4.16 | 前端编译通过 | `pnpm --filter @clip-anvil/web build` | 退出码 0 |

## 6. 端到端集成验收

以下是完整的端到端验收脚本，Coding Agent 可按顺序执行：

```bash
#!/bin/bash
set -e

BASE="http://localhost:8888"

echo "=== 阶段 1: 基础设施 ==="
make migrate-up
make sqlc-generate
make server-build
echo "✅ 基础设施通过"

echo "=== 阶段 2: 注册登录 ==="
# 注册
REGISTER=$(curl -sf -X POST $BASE/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"e2e@test.com","password":"123456","name":"E2E"}')
TOKEN=$(echo $REGISTER | jq -r '.token')
[ -n "$TOKEN" ] && [ "$TOKEN" != "null" ] && echo "✅ 2.1 注册成功"

# 重复注册 409
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X POST $BASE/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"e2e@test.com","password":"123456","name":"E2E"}')
[ "$HTTP" = "409" ] && echo "✅ 2.2 重复注册拒绝"

# 登录
LOGIN=$(curl -sf -X POST $BASE/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"e2e@test.com","password":"123456"}')
TOKEN=$(echo $LOGIN | jq -r '.token')
[ -n "$TOKEN" ] && [ "$TOKEN" != "null" ] && echo "✅ 2.3 登录成功"

# 错误密码 401
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X POST $BASE/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"e2e@test.com","password":"wrong"}')
[ "$HTTP" = "401" ] && echo "✅ 2.4 错误密码拒绝"

# /me 带 token
EMAIL=$(curl -sf $BASE/api/auth/me -H "Authorization: Bearer $TOKEN" | jq -r '.email')
[ "$EMAIL" = "e2e@test.com" ] && echo "✅ 2.5 /me 返回用户"

# /me 无 token 401
HTTP=$(curl -s -o /dev/null -w '%{http_code}' $BASE/api/auth/me)
[ "$HTTP" = "401" ] && echo "✅ 2.6 未认证拒绝"

echo "=== 阶段 3: Workspace ==="
# 创建
WID=$(curl -sf -X POST $BASE/api/workspaces \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"E2E项目"}' | jq -r '.id')
[ -n "$WID" ] && [ "$WID" != "null" ] && echo "✅ 3.1 创建 workspace"

# 列表
COUNT=$(curl -sf $BASE/api/workspaces \
  -H "Authorization: Bearer $TOKEN" | jq 'length')
[ "$COUNT" -ge 1 ] && echo "✅ 3.2 列表返回"

# 详情
WNAME=$(curl -sf $BASE/api/workspaces/$WID \
  -H "Authorization: Bearer $TOKEN" | jq -r '.name')
[ "$WNAME" = "E2E项目" ] && echo "✅ 3.3 详情正确"

echo "=== 阶段 4: 画布 + 节点 ==="
# 空画布
NCOUNT=$(curl -sf $BASE/api/workspaces/$WID/canvas \
  -H "Authorization: Bearer $TOKEN" | jq '.nodes | length')
[ "$NCOUNT" = "0" ] && echo "✅ 4.1 空画布"

# 创建节点
NID=$(curl -sf -X POST $BASE/api/nodes \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","node_type":"text","title":"脚本","canvas_x":100,"canvas_y":200}' \
  | jq -r '.id')
[ -n "$NID" ] && [ "$NID" != "null" ] && echo "✅ 4.2 创建节点"

# 画布包含节点
NCOUNT=$(curl -sf $BASE/api/workspaces/$WID/canvas \
  -H "Authorization: Bearer $TOKEN" | jq '.nodes | length')
[ "$NCOUNT" = "1" ] && echo "✅ 4.3 画布包含节点"

# 更新节点
TITLE=$(curl -sf -X PATCH $BASE/api/nodes/$NID \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"终稿","prompt":"内容"}' | jq -r '.title')
[ "$TITLE" = "终稿" ] && echo "✅ 4.4 更新节点"

# 部分更新不覆盖
PROMPT=$(curl -sf -X PATCH $BASE/api/nodes/$NID \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"新标题"}' | jq -r '.prompt')
[ "$PROMPT" = "内容" ] && echo "✅ 4.5 部分更新不覆盖"

# 批量更新位置
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X PATCH $BASE/api/nodes/batch-position \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"positions":[{"id":"'$NID'","canvas_x":300,"canvas_y":400}]}')
[ "$HTTP" = "204" ] && echo "✅ 4.6 批量更新位置"

# 位置已持久化
CX=$(curl -sf $BASE/api/workspaces/$WID/canvas \
  -H "Authorization: Bearer $TOKEN" | jq '.nodes[0].canvas_x')
[ "$CX" = "300" ] && echo "✅ 4.7 位置持久化"

# 更新 camera
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X PATCH $BASE/api/workspaces/$WID/camera \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"x":50,"y":100,"zoom":1.5}')
[ "$HTTP" = "204" ] && echo "✅ 4.8 更新 camera"

# camera 持久化
ZOOM=$(curl -sf $BASE/api/workspaces/$WID/canvas \
  -H "Authorization: Bearer $TOKEN" | jq '.camera.zoom')
[ "$ZOOM" = "1.5" ] && echo "✅ 4.9 camera 持久化"

# 删除节点
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE $BASE/api/nodes/$NID \
  -H "Authorization: Bearer $TOKEN")
[ "$HTTP" = "204" ] && echo "✅ 4.10 删除节点"

# 删除后为空
NCOUNT=$(curl -sf $BASE/api/workspaces/$WID/canvas \
  -H "Authorization: Bearer $TOKEN" | jq '.nodes | length')
[ "$NCOUNT" = "0" ] && echo "✅ 4.11 删除后画布为空"

# 后端单测 + 前端编译
make server-test && echo "✅ 后端单测通过"
pnpm --filter @clip-anvil/web build && echo "✅ 前端编译通过"

echo "=== 全部通过 ==="
```

## 7. 技术备注

### 7.1 后端代码组织

```
apps/server/
├── cmd/server/main.go          — 入口，初始化依赖，注册路由
├── config.yaml                 — 开发配置
├── migrations/
│   └── 001_init_schema.sql     — goose 迁移
├── sqlc/queries/
│   ├── account.sql
│   ├── workspace.sql
│   ├── canvas.sql
│   └── node.sql
├── sqlc.yaml
└── internal/
    ├── auth/
    │   ├── jwt.go              — 签发/校验
    │   ├── password.go         — bcrypt
    │   └── middleware.go       — Hertz JWT 中间件
    ├── api/
    │   ├── auth_handler.go     — register/login/me
    │   ├── workspace_handler.go
    │   ├── canvas_handler.go   — GET canvas, PATCH camera
    │   └── node_handler.go     — CRUD + batch-position
    ├── config/
    │   └── config.go           — 扩展 JWT 配置
    └── store/
        └── db/                 — sqlc 生成（不手动编辑）
```

### 7.2 前端代码组织

```
apps/web/src/
├── main.tsx                    — 入口，QueryClientProvider + RouterProvider
├── main.css                    — TailwindCSS 入口
├── lib/
│   ├── api.ts                  — fetch 封装，自动注入 Authorization
│   └── canvas.ts               — nodeToShape 映射
├── stores/
│   ├── appearance.ts           — 明亮/暗夜外观
│   └── auth.ts                 — Zustand auth store
├── components/
│   ├── ProtectedRoute.tsx
│   ├── GuestRoute.tsx
│   ├── Layout.tsx              — 顶部导航条 + Outlet
│   └── CreateWorkspaceDialog.tsx
├── pages/
│   ├── LoginPage.tsx
│   ├── RegisterPage.tsx
│   ├── WorkspaceListPage.tsx
│   └── WorkspaceDetailPage.tsx — Studio shell + tldraw + MediaShape + 右键菜单 + 内联编辑
└── shapes/
    └── MediaShapeUtil.tsx      — 自定义 ShapeUtil
```

### 7.3 依赖变更汇总

**Go（go.mod 新增）**：
- `github.com/golang-jwt/jwt/v5`

**前端（package.json 新增）**：
- `react-router`、`zustand`、`@tanstack/react-query`
- devDependencies: `tailwindcss`、`@tailwindcss/vite`

**开发工具（不进 go.mod）**：
- `goose` CLI（通过 `go install`）
- `sqlc` CLI（通过 `go install`）

## 8. 与现有设计文档的关系

本 Spec 是 Studio Canvas 基础链路的第一步实施拆解。当前项目以 [architecture.md](../../../engineering/architecture.md) 的里程碑表为准：M1 = Studio 画布基础，M1.x = Studio 增量，M2 = Agent 对话基础。

完成 M1 后，下一步迭代方向：

- image/video/audio 节点类型
- 连线（MediaEdge）+ DAG 环检测
- 分组（MediaGroup）
- 左侧资源树 + 右侧属性面板 + 浮动工具栏
- 生成任务（GenerationJob）+ 版本管理
- WebSocket 事件推送

这些内容对应目标 Studio 画布的完整交付，在 M1 之后作为 M1.x 增量迭代。
