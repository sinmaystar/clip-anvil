# ClipAnvil 数据库与前后端交互设计

## 1. 数据架构总览

### 1.1 核心原则

业务数据库是唯一事实源。tldraw 是纯渲染引擎——接收数据、渲染图形、捕获交互，不持有独立状态。

```
业务 DB（PostgreSQL）
  │
  ├── media_node (含画布坐标 x,y,w,h)  ──→  tldraw MediaShape
  ├── media_edge                        ──→  tldraw ArrowShape + Binding
  ├── media_group                       ──→  tldraw GroupShape
  └── canvas_document (camera)          ──→  tldraw Camera
```

不存在 tldraw snapshot、不存在 canvas_record 表。画布上的每一个元素都能从业务表直接构建，不可能出现"画布和业务数据不一致"的问题。

### 1.2 为什么不存 tldraw snapshot

| 方案 | 问题 |
|---|---|
| 全量 snapshot（一个 JSONB 存所有 shape） | 每次拖拽都要全量写入几十 KB，且 snapshot 中的业务字段（status、title）会与业务表脱节 |
| canvas_record（逐条存 tldraw 记录） | 本质上是业务表的副本，两套数据必然不一致——Agent 后台创建节点、网络断开、Bug 都会导致两边分叉 |
| **业务表直接存坐标**（当前方案） | 只有一个事实源，tldraw 从业务表构建，不存在同步问题 |

## 2. 数据库表结构

### 2.0 当前迁移快照

当前 goose 迁移包含 `001_init_schema.sql` 到 `004_add_assets.sql`，已覆盖 Studio M1.x 的核心编辑数据：

- 枚举：`media_type`、`node_status`、`edge_type`、`transition_type`
- 表：`account`、`workspace`、`canvas_document`、`media_node`、`media_edge`、`media_group`、`media_asset`
- `media_node` 当前包含 `group_id`、`asset_id`，但不包含 `model_provider`、`model_name`、`model_params`、`current_version_id`、`sort_order`
- `canvas_w/canvas_h` 默认值为 `200/120`

下面的完整 schema 同时记录当前已落地对象和后续 Agent/生成目标态。真实可执行结构以 `apps/server/migrations/` 和 sqlc 生成代码为准。

### 2.1 枚举类型

```sql
CREATE TYPE media_type      AS ENUM ('text', 'image', 'video', 'audio');
CREATE TYPE node_status     AS ENUM ('draft', 'ready', 'queued', 'running',
                                     'succeeded', 'failed', 'stale', 'user_editing');
CREATE TYPE edge_type       AS ENUM ('dependency', 'reference', 'sequence');
CREATE TYPE transition_type AS ENUM ('cut', 'crossfade', 'dissolve', 'wipe');
CREATE TYPE job_status      AS ENUM ('pending', 'running', 'succeeded', 'failed', 'cancelled');
CREATE TYPE review_verdict  AS ENUM ('approved', 'rejected', 'needs_revision');
```

### 2.2 account — 用户账号

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

邮箱 + 密码注册登录。`password_hash` 存 bcrypt 哈希，明文密码不落库。`name` 是展示用昵称。后续如需第三方登录（微信/Google），在此表基础上扩展 OAuth 关联表。

### 2.3 鉴权方式：JWT

不设 session 表。登录成功后签发 JWT access token（含 account_id、过期时间），前端存 localStorage。请求时通过 `Authorization: Bearer <token>` 携带，后端中间件校验签名和过期。WebSocket 连接时通过 query param `?token=xxx` 传递。

如后续需要 token 撤销（如修改密码后踢出所有端），可在 Redis 中维护黑名单，不需要 DB 表。

### 2.4 workspace — 顶层项目容器

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

一个 Workspace 对应一个项目（如"咖啡广告"）。`settings` 存项目级配置（默认模型、质量阈值等）。`owner_id` 关联 account 表。

### 2.5 canvas_document — 画布视口状态

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

一个 workspace 对应一张画布。只存视口信息（用户上次在看画布的什么位置、什么缩放级别），刷新后恢复。`layout_version` 用于乐观并发控制。

### 2.6 media_asset — 文件级资产

```sql
CREATE TABLE media_asset (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    type          media_type NOT NULL,
    mime          TEXT NOT NULL,
    storage_url   TEXT NOT NULL,
    thumbnail_url TEXT,
    duration_ms   INT,
    size_bytes    BIGINT,
    metadata      JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

所有文件级资源：用户上传的图片、Agent 生成的视频、BGM 音频等。实际文件存 MinIO，这里存元数据和访问路径。多个节点/版本可以引用同一个 asset。

### 2.7 media_group — 扁平分组

```sql
CREATE TABLE media_group (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

扁平分组，不支持嵌套。对应画布上的 tldraw GroupShape 和左侧资源树的文件夹。分组是纯组织工具，不影响依赖关系。

### 2.8 media_node — 业务节点（含画布坐标）

```sql
CREATE TABLE media_node (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id       UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    group_id           UUID REFERENCES media_group(id) ON DELETE SET NULL,
    asset_id           UUID REFERENCES media_asset(id) ON DELETE SET NULL,
    node_type          media_type NOT NULL,
    title              TEXT NOT NULL DEFAULT '',
    status             node_status NOT NULL DEFAULT 'draft',
    prompt             TEXT NOT NULL DEFAULT '',
    model_provider     TEXT,
    model_name         TEXT,
    model_params       JSONB NOT NULL DEFAULT '{}',
    current_version_id UUID,
    source             TEXT NOT NULL DEFAULT 'user',
    sort_order         INT NOT NULL DEFAULT 0,
    -- 画布坐标（tldraw 渲染位置）
    canvas_x           REAL NOT NULL DEFAULT 0,
    canvas_y           REAL NOT NULL DEFAULT 0,
    canvas_w           REAL NOT NULL DEFAULT 240,
    canvas_h           REAL NOT NULL DEFAULT 180,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**关键设计**：画布坐标（`canvas_x/y/w/h`）直接存在业务节点表上，不存在独立的画布状态表。tldraw 的 MediaShape 从这张表构建，不需要 snapshot。

> 当前实现使用分阶段迁移后的 `media_node`：默认尺寸为 `200 × 120`，支持 `text/image/video/audio`，并已通过后续迁移增加 `group_id` 和 `asset_id`。上方完整表结构中的模型、版本和排序字段仍属于目标态。

- `group_id`：所属分组，一个节点最多属于一个分组
- `asset_id`：关联的文件资产（Draft 节点此字段为 NULL）
- `current_version_id`：当前 winner 版本（FK 延迟添加，见 2.11）
- `source`：`'user'`（用户创建）或 `'agent'`（Agent 创建），决定画布上的视觉区分（实线 vs 蓝色虚线边框）
- `canvas_w/h` 默认值对应视频节点的标准卡片尺寸，创建时可按 node_type 设置不同默认值

### 2.9 media_edge — 节点间关系

```sql
CREATE TABLE media_edge (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    from_node_id        UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    to_node_id          UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    edge_type           edge_type NOT NULL,
    transition_type     transition_type,
    transition_duration REAL,
    source              TEXT NOT NULL DEFAULT 'user',
    metadata            JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT no_self_loop CHECK (from_node_id != to_node_id),
    CONSTRAINT unique_edge UNIQUE (from_node_id, to_node_id, edge_type)
);
```

三种语义的连线共用一张表。`transition_type` 和 `transition_duration` 仅 sequence 类型使用，其他类型为 NULL。

**DAG 约束**：`no_self_loop` 在 SQL 层禁止自连接。环检测在应用层实现——创建 dependency 类型的 edge 前，从目标节点沿 dependency 出边做 BFS，如果能到达源节点则拒绝。`reference` 和 `sequence` 类型不参与依赖传播，不做环检测。

### 2.10 generation_job — 生成任务

```sql
CREATE TABLE generation_job (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id   UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    target_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    provider       TEXT NOT NULL,
    model          TEXT NOT NULL,
    prompt         TEXT NOT NULL,
    params         JSONB NOT NULL DEFAULT '{}',
    status         job_status NOT NULL DEFAULT 'pending',
    progress       INT NOT NULL DEFAULT 0,
    cost_cents     INT,
    error_message  TEXT,
    started_at     TIMESTAMPTZ,
    completed_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

每次调用 `submit_generation` 创建一条记录。`prompt` 和 `params` 是提交时的快照——后续修改节点的 Prompt 不影响已提交的任务。`cost_cents` 以分为单位记录费用，用于 Stale 影响分析时展示重算成本。

### 2.11 artifact_version — 产物版本

```sql
CREATE TABLE artifact_version (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id      UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    job_id       UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    asset_id     UUID REFERENCES media_asset(id) ON DELETE SET NULL,
    version_no   INT NOT NULL,
    winner       BOOLEAN NOT NULL DEFAULT false,
    review_score REAL,
    input_hash   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_version_per_node UNIQUE (node_id, version_no)
);

ALTER TABLE media_node ADD CONSTRAINT fk_current_version
    FOREIGN KEY (current_version_id) REFERENCES artifact_version(id)
    ON DELETE SET NULL;
```

一个节点可以有多个版本（多次生成的候选结果）。`winner = true` 标记当前选中的版本，`media_node.current_version_id` 指向它。`input_hash` 是上游依赖 winner + Prompt + 模型参数的哈希，用于 Stale 检测时判断当前版本是否仍有效。

### 2.12 review_record — 评审记录

```sql
CREATE TABLE review_record (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id UUID NOT NULL REFERENCES artifact_version(id) ON DELETE CASCADE,
    axes       JSONB NOT NULL DEFAULT '{}',
    score      REAL NOT NULL,
    critique   TEXT,
    verdict    review_verdict NOT NULL,
    reviewer   TEXT NOT NULL DEFAULT 'auto',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`axes` 存储多维度评分（如 `{"visual_quality": 8.5, "cast_match": 7.0, "content_safety": 9.0}`）。`reviewer` 为 `'auto'`（Clip Review Skill 自动评审）或 `'user'`（用户手动评审）。

### 2.13 agent_step — Agent 操作审计日志

```sql
CREATE TABLE agent_step (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id     UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    step_type        TEXT NOT NULL,
    input            JSONB NOT NULL DEFAULT '{}',
    output           JSONB NOT NULL DEFAULT '{}',
    created_node_ids UUID[] DEFAULT '{}',
    created_edge_ids UUID[] DEFAULT '{}',
    duration_ms      INT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Agent 的每一步操作记录。`step_type` 对应 Skill 名称（如 `'screenwriter'`、`'director'`）或命令名称（如 `'create_media_node'`）。用于操作回溯和调试。

### 2.14 索引

```sql
-- 账号
CREATE INDEX idx_account_email ON account(email);

-- workspace 维度（最常用的过滤条件）
CREATE INDEX idx_workspace_owner ON workspace(owner_id);
CREATE INDEX idx_media_node_workspace ON media_node(workspace_id);
CREATE INDEX idx_media_asset_workspace ON media_asset(workspace_id);
CREATE INDEX idx_media_edge_workspace ON media_edge(workspace_id);
CREATE INDEX idx_media_group_workspace ON media_group(workspace_id);

-- DAG 遍历（Stale 传播、拓扑排序、环检测）
CREATE INDEX idx_media_edge_from ON media_edge(from_node_id);
CREATE INDEX idx_media_edge_to ON media_edge(to_node_id);

-- 节点状态筛选（资源树按状态过滤）
CREATE INDEX idx_media_node_status ON media_node(workspace_id, status);
CREATE INDEX idx_media_node_group ON media_node(group_id);

-- 版本查询
CREATE INDEX idx_artifact_version_node ON artifact_version(node_id);
CREATE INDEX idx_artifact_version_winner ON artifact_version(node_id) WHERE winner = true;
CREATE INDEX idx_review_record_version ON review_record(version_id);

-- 生成任务
CREATE INDEX idx_generation_job_node ON generation_job(target_node_id);
CREATE INDEX idx_generation_job_status ON generation_job(workspace_id, status);

-- 审计日志（按时间倒序查最近操作）
CREATE INDEX idx_agent_step_workspace ON agent_step(workspace_id, created_at DESC);
```

## 3. tldraw 投影层类型

### 3.1 MediaShapeProps — tldraw shape 的 props

```typescript
import type { TLBaseShape } from 'tldraw'

type MediaType = 'text' | 'image' | 'video' | 'audio'
type NodeStatus = 'draft' | 'ready' | 'queued' | 'running'
  | 'succeeded' | 'failed' | 'stale' | 'user_editing'

interface MediaShapeProps {
  nodeId: string
  nodeType: MediaType
  title: string
  status: NodeStatus
  thumbnailUrl?: string
  progress?: number
  reviewScore?: number
  isAgentCreated: boolean
}

type MediaShape = TLBaseShape<'media', MediaShapeProps>
```

目标态下这是 tldraw shape 需要的最小渲染字段。M1 当前为了节点预览、自动保存和撤销恢复，把 `prompt` 也放入 `MediaShapeProps`，并额外包含 `w/h`。

### 3.2 从业务数据构建 shape

页面加载时，从后端 API 获取 workspace 的所有 media_node，逐个映射为 tldraw shape：

```typescript
function nodeToShape(node: MediaNodeDTO): MediaShape {
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
      thumbnailUrl: node.thumbnailUrl,
      isAgentCreated: node.source === 'agent',
    },
  }
}
```

## 4. 前后端交互流程

### 4.1 三条数据通路

```
                    ┌────────────────────────────────────┐
                    │         tldraw（浏览器内存）         │
                    │  内存 store → 即时渲染，零延迟       │
                    └──────┬────────────────┬─────────────┘
                           │                │
              ② 异步持久化   │                │  ③ 事件推送
              （前端→后端）  │                │ （后端→前端）
                           ▼                │
                    ┌──────────────┐        │
  ① 初始加载         │   后端 API    │        │
  （一次性）  ──────→│  + 业务 DB   │────────┘
                    └──────────────┘
                                    WebSocket 事件流
```

### 4.2 通路 ①：初始加载

页面打开时一次性从后端拉取 workspace 的全部画布数据：

```
GET /api/workspaces/:id/canvas

响应：
{
  camera: { x, y, zoom },
  nodes: [ { id, nodeType, title, status, ..., canvasX, canvasY, canvasW, canvasH } ],
  edges: [ { id, fromNodeId, toNodeId, edgeType, transitionType, ... } ],
  groups: [ { id, name, nodeIds: [...] } ]
}
```

前端收到后：
1. `editor.createShapes(nodes.map(nodeToShape))` — 批量创建节点 shape
2. 为每条 edge 创建 ArrowShape + Binding
3. 为每个 group 创建 GroupShape
4. `editor.setCamera({ x, y, z: zoom })` — 恢复视口

加载完成后 tldraw 在内存中独立工作，后续交互不依赖后端返回才能渲染。

### 4.3 通路 ②：用户操作 → 异步持久化

用户操作分两类，处理策略不同：

**位置类操作（拖拽、缩放、移动视口）— 容忍丢失**

```
用户拖拽节点
  → tldraw 内存 store 立即更新（0ms）
  → 画布立即响应
  → 防抖 2s 后批量写回
  → PATCH /api/nodes/batch-position [{ id, canvasX, canvasY }, ...]
  → 失败？静默重试一次，仍失败则忽略
  → 最坏结果：下次刷新位置回退
```

```
用户缩放/平移画布
  → tldraw camera 立即更新（0ms）
  → 防抖 2s 后写回
  → PATCH /api/workspaces/:id/camera { x, y, zoom }
  → 失败？忽略
```

**业务操作（创建节点、建连线、编辑 Prompt、提交生成）— 必须成功**

```
用户点击"创建视频节点"
  → 调用 POST /api/nodes { nodeType: 'video', canvasX: 100, canvasY: 200 }
  → 等后端返回（~200ms）
  → 成功 → editor.createShape(nodeToShape(response))  ← 画布出现节点
  → 失败 → toast 提示"创建失败，请重试"                ← 画布无变化
```

```
用户从 A 拖连线到 B
  → 调用 POST /api/edges { fromNodeId: A, toNodeId: B, edgeType: 'dependency' }
  → 后端做 DAG 环检测
  → 成功 → editor.createShape(arrowShape) + binding
  → 失败（成环）→ toast 提示"不能形成循环依赖"
  → 失败（其他）→ toast 提示"连线失败，请重试"
```

**为什么业务操作不做乐观更新**：200ms 的等待在创建节点、建连线、编辑保存这些操作中用户几乎察觉不到。乐观更新需要处理回滚逻辑（删除已渲染的 shape、撤销状态变更），增加复杂度但收益极低。只有高频交互（拖拽）才需要本地即时响应，而那恰恰是丢失了也无所谓的位置数据。

### 4.4 通路 ③：后端事件 → WebSocket 推送

Agent 操作、生成任务状态变更等后端事件通过 WebSocket 推送到前端：

```
WebSocket /ws/canvas?workspaceId=xxx

事件格式：
{ type: "NodeCreated",   payload: { node: {...} } }
{ type: "NodeUpdated",   payload: { nodeId, changes: { status, thumbnailUrl, ... } } }
{ type: "NodeDeleted",   payload: { nodeId } }
{ type: "EdgeCreated",   payload: { edge: {...} } }
{ type: "EdgeDeleted",   payload: { edgeId } }
{ type: "JobProgress",   payload: { nodeId, progress: 45 } }
{ type: "JobCompleted",  payload: { nodeId, status, thumbnailUrl, reviewScore } }
{ type: "JobFailed",     payload: { nodeId, errorMessage } }
{ type: "GateRequested", payload: { gateType, message, options } }
```

前端按事件类型调用对应的 Editor API：

| 事件 | 前端响应 |
|---|---|
| NodeCreated | `editor.createShape(nodeToShape(node))` |
| NodeUpdated | `editor.updateShape({ id, props: changes })` |
| NodeDeleted | `editor.deleteShape(shapeId)` |
| EdgeCreated | 创建 ArrowShape + Binding |
| EdgeDeleted | 删除 ArrowShape |
| JobProgress | `editor.updateShape({ id, props: { progress } })` |
| JobCompleted | 更新 status + thumbnailUrl + reviewScore |
| JobFailed | 更新 status 为 failed |
| GateRequested | 对话面板展示确认卡片 |

### 4.5 冲突处理

当用户正在编辑某个节点，同时 Agent 也在修改它时：

1. 用户点击节点编辑 → 节点状态变为 `user_editing` → 后端广播 `NodeUpdated { status: 'user_editing' }`
2. Agent 收到状态变更 → 暂停该节点的分支，转而处理其他节点
3. 用户编辑完毕，点击"交还 Agent" → 节点状态恢复 → Agent 继续

如果 Agent 推送的 `NodeUpdated` 与用户当前正在编辑的节点冲突（用户已经在修改 Prompt 但还没保存）：
- 前端检查该节点是否处于本地编辑状态
- 如果是 → 忽略 Agent 的 props 更新，保留用户本地的编辑内容
- 用户保存时以用户版本为准（用户操作优先级高于 Agent）

## 5. 迁移与工具链

### 5.1 迁移工具：goose

```
apps/server/migrations/
└── 001_init_schema.sql    ← goose 格式：-- +goose Up / -- +goose Down
```

goose 的 SQL-first 方式与 sqlc 配合最自然——sqlc 直接读迁移文件作为 schema 源，无需维护两份 schema 定义。

常用命令：

```bash
make migrate-up       # 执行所有未应用的迁移
make migrate-down     # 回滚最近一次迁移
make migrate-create name=add_xxx   # 创建新迁移文件
```

当前 Makefile 暂未提供 `migrate-status`。需要查看状态时可在 `apps/server` 下直接运行 goose CLI。

### 5.2 类型安全查询：sqlc

```yaml
# apps/server/sqlc.yaml
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

sqlc 从迁移文件读取 schema，从 `sqlc/queries/*.sql` 读取查询，生成类型安全的 Go 代码到 `internal/store/db/`。

```bash
make sqlc-generate    # 生成 Go 代码
```

## 6. v2 业务交互设计增补

以下表和字段在 [业务交互设计 v2](../design/overview.md) 中引入，作为 Agent 模式和 Skill 体系的数据支撑。

### 6.1 workspace 表增加 mode 字段

当前代码仍通过 `workspace.settings` 承载项目级配置，未增加显式 `mode` 字段。后续如果 Agent/Studio 模式切换需要查询和约束，再引入显式字段。

```sql
ALTER TABLE workspace ADD COLUMN mode TEXT NOT NULL DEFAULT 'studio';
-- 值: 'studio' | 'agent'
```

### 6.2 media_node 表增加分镜相关字段

```sql
ALTER TABLE media_node ADD COLUMN duration_sec REAL;
-- 视频节点的目标时长（秒），分镜规划时设置

ALTER TABLE media_node ADD COLUMN narrative_purpose TEXT NOT NULL DEFAULT '';
-- 叙事目的（借鉴 spark-video），分镜节点必填

ALTER TABLE media_node ADD COLUMN use_prev_last_frame BOOLEAN NOT NULL DEFAULT false;
-- 是否使用前一个镜头的最后一帧作为本镜头的首帧（帧连续性）
```

### 6.3 skill 表（MVP 后期）

```sql
CREATE TABLE skill (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description  TEXT NOT NULL,
    config       JSONB NOT NULL,  -- Skill YAML 的 JSON 表示
    is_builtin   BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

内置 Skill 在系统初始化时写入。`config` 存储完整定义（phases、review_rubric、gates、style_constraints 等）。

### 6.4 agent_session 表

```sql
CREATE TABLE agent_session (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    skill_name    TEXT,
    brief         JSONB NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL DEFAULT 'running',
    -- 值: 'running' | 'paused' | 'waiting_gate' | 'completed' | 'failed'
    current_phase TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

记录 Agent 模式下的一次完整工作会话。一个 workspace 同时只能有一个 running 的 agent_session。

## 相关文档

- [整体设计](../design/overview.md) — 架构、原则、路线图
- [画布设计](../design/canvas.md) — tldraw 投影层和数据通路
- [Studio 模式](../design/studio-mode.md) — 用户主导的创作交互
- [Agent 模式](../design/agent-mode.md) — Agent 驱动的生产交互
