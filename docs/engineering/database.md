# ClipAnvil 数据库与前后端交互设计

## 1. 数据架构总览

### 1.1 核心原则

业务数据库是唯一事实源。tldraw 是纯渲染引擎——接收数据、渲染图形、捕获交互，不持有独立状态。

```
业务 DB（PostgreSQL）
  │
  ├── media_node (含画布坐标 x,y,w,h)  ──→  tldraw MediaShape
  ├── media_edge                        ──→  SVG connection overlay
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

当前 goose 迁移包含 `001_init_schema.sql` 到 `014_m5_version_lifecycle.sql`，已覆盖 M3-M5 的 Studio 生产链路：

- 枚举：`workspace_mode`、`node_type`、`asset_type`、`node_status`、`job_status`
- 核心表：`account`、`workspace`、`canvas_document`、`media_node`、`media_edge`、`media_group`、`media_asset`
- 沙箱表：`workspace_sandbox`、`sandbox_job`
- 生产表：`generation_job`、`artifact_version`、`model_provider`、`model_capability`、`node_stale_reason`、`reference_pack_item`
- 已收敛：`media_edge` 当前只表达 dependency，不再存 `edge_type` / transition 字段。
- 已扩展：`media_node` 当前包含 `operation_type`、`prompt_template`、`prompt_rich`、`prompt_refs`、`model_provider`、`model_id`、`model_params`、`current_version_id`、`metadata`。
- 已扩展：`artifact_version` 当前支持 queued/running/succeeded/failed/cancelled 生命周期，并通过 `job_id` 与 `generation_job` 一一绑定。

下面的 schema 记录当前已落地对象；Agent runtime、shot、review 等 M6 目标态对象在第 6 节单独列出。真实可执行结构以 `apps/server/migrations/` 和 sqlc 生成代码为准。

### 2.1 枚举类型

```sql
CREATE TYPE workspace_mode  AS ENUM ('studio', 'agent');
CREATE TYPE node_type       AS ENUM ('text', 'image', 'video', 'audio', 'reference_pack');
CREATE TYPE asset_type      AS ENUM ('text', 'image', 'video', 'audio', 'json');
CREATE TYPE node_status     AS ENUM ('draft', 'ready', 'queued', 'running',
                                     'succeeded', 'failed', 'stale', 'user_editing');
CREATE TYPE job_status      AS ENUM ('pending', 'queued', 'running', 'succeeded',
                                     'failed', 'cancelled');
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
    mode       workspace_mode NOT NULL DEFAULT 'studio',
    settings   JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

一个 Workspace 对应一个项目（如"咖啡广告"）。`mode` 是 Studio / Agent 的路由和权限边界。`settings` 存项目级配置（默认模型、质量阈值等）。`owner_id` 关联 account 表。

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
    type          asset_type NOT NULL,
    mime          TEXT NOT NULL,
    storage_url   TEXT,
    text_content  TEXT,
    thumbnail_url TEXT,
    duration_ms   INT,
    size_bytes    BIGINT,
    metadata      JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT media_asset_has_content
      CHECK (storage_url IS NOT NULL OR text_content IS NOT NULL)
);
```

所有资源级资产：用户上传的图片/视频/音频、模型生成的图片/视频、文本生成结果等。二进制文件存 MinIO，文本内容可直接存 `text_content`。多个节点/版本可以引用同一个 asset。

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
    node_type          node_type NOT NULL,
    title              TEXT NOT NULL DEFAULT '',
    status             node_status NOT NULL DEFAULT 'draft',
    prompt             TEXT NOT NULL DEFAULT '',
    operation_type     TEXT NOT NULL DEFAULT 'manual',
    prompt_template    TEXT NOT NULL DEFAULT '',
    prompt_rich        JSONB NOT NULL DEFAULT '{}',
    prompt_refs        JSONB NOT NULL DEFAULT '[]',
    model_provider     TEXT,
    model_id           TEXT,
    model_params       JSONB NOT NULL DEFAULT '{}',
    current_version_id UUID,
    metadata           JSONB NOT NULL DEFAULT '{}',
    source             TEXT NOT NULL DEFAULT 'user',
    -- 画布坐标（tldraw 渲染位置）
    canvas_x           REAL NOT NULL DEFAULT 0,
    canvas_y           REAL NOT NULL DEFAULT 0,
    canvas_w           REAL NOT NULL DEFAULT 200,
    canvas_h           REAL NOT NULL DEFAULT 120,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**关键设计**：画布坐标（`canvas_x/y/w/h`）直接存在业务节点表上，不存在独立的画布状态表。tldraw 的 MediaShape 从这张表构建，不需要 snapshot。

- `group_id`：所属分组，一个节点最多属于一个分组
- `asset_id`：用户源素材节点直接关联的资产，或 legacy 上传资产引用
- `operation_type`：生成或素材语义，例如 `manual`、`upload`、`text_generation`、`text_to_image`、`image_to_video`、`collect_references`
- `prompt_template`：用户编写的原始 Prompt 模板，可包含 `@节点名`
- `prompt_refs`：结构化 Prompt 引用，运行前用于校验和渲染
- `current_version_id`：当前选中的产物版本
- `source`：`'user'`（用户创建）或 `'agent'`（Agent 创建），决定画布上的视觉区分（实线 vs 蓝色虚线边框）
- `operation_type='manual'` 的文本节点和 `operation_type='upload'` 或有 `asset_id` 的媒体节点是用户源素材，不展示模型运行入口，但可作为下游输入或参考包成员。

### 2.9 media_edge — 节点间关系

```sql
CREATE TABLE media_edge (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    from_node_id        UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    to_node_id          UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    source              TEXT NOT NULL DEFAULT 'user',
    metadata            JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT no_self_loop CHECK (from_node_id != to_node_id),
    CONSTRAINT unique_edge UNIQUE (from_node_id, to_node_id)
);
```

当前 Studio 用户只创建 dependency 连线。reference / sequence / transition 不在 `media_edge` 当前 schema 中表达，后续 Agent storyboard 需要时应在独立 Agent/shot 表里建模，避免污染 Studio dependency 语义。

**DAG 约束**：`no_self_loop` 在 SQL 层禁止自连接。环检测在应用层实现——创建 dependency 类型的 edge 前，从目标节点沿 dependency 出边做 BFS，如果能到达源节点则拒绝。

### 2.10 workspace_sandbox — 工作区沙箱绑定

```sql
CREATE TABLE workspace_sandbox (
    workspace_id          UUID PRIMARY KEY REFERENCES workspace(id) ON DELETE CASCADE,
    sandbox_id            TEXT,
    volume_name           TEXT NOT NULL UNIQUE,
    status                TEXT NOT NULL CHECK (status IN ('creating', 'running', 'unhealthy', 'terminated')),
    last_health_check_at  TIMESTAMPTZ,
    last_seen_at          TIMESTAMPTZ,
    error_message         TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

OpenSandbox 容器本身可替换，`workspace_sandbox` 是 workspace 到稳定 sandbox volume 的事实源。后端通过这张表创建、复用、替换或终止 workspace sandbox，内存状态不作为恢复依据。

### 2.11 generation_job — 生成任务

```sql
CREATE TABLE generation_job (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id      UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    target_node_id    UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    parent_job_id     UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    operation_type    TEXT NOT NULL,
    provider          TEXT NOT NULL,
    model_id          TEXT NOT NULL,
    intent            JSONB NOT NULL DEFAULT '{}',
    rendered_prompt   TEXT NOT NULL DEFAULT '',
    provider_request  JSONB NOT NULL DEFAULT '{}',
    provider_response JSONB NOT NULL DEFAULT '{}',
    status            job_status NOT NULL DEFAULT 'pending',
    progress          INT NOT NULL DEFAULT 0,
    attempt           INT NOT NULL DEFAULT 1,
    max_attempts      INT NOT NULL DEFAULT 1,
    retry_policy      JSONB NOT NULL DEFAULT '{}',
    cost_cents        INT,
    error_code        TEXT,
    error_message     TEXT,
    requested_by_type TEXT NOT NULL DEFAULT 'user',
    requested_by_id   TEXT,
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

每次节点运行创建一条记录。`intent`、`rendered_prompt`、`provider_request` 和 `provider_response` 是提交和调用时的审计快照；后续修改节点 Prompt 不影响已提交任务。异步 provider 会先进入 `queued/running`，完成或失败后更新状态。`parent_job_id`、`attempt` 和 `max_attempts` 用于失败重试链路。

### 2.12 artifact_version — 产物版本

```sql
CREATE TABLE artifact_version (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id      UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    node_id           UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    job_id            UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    asset_id          UUID REFERENCES media_asset(id) ON DELETE SET NULL,
    version_no        INT NOT NULL,
    winner            BOOLEAN NOT NULL DEFAULT false,
    output            JSONB NOT NULL DEFAULT '{}',
    review_score      REAL,
    input_hash        TEXT NOT NULL DEFAULT '',
    status            job_status NOT NULL DEFAULT 'succeeded',
    progress          INT NOT NULL DEFAULT 100,
    error_code        TEXT,
    error_message     TEXT,
    provider_request  JSONB NOT NULL DEFAULT '{}',
    provider_response JSONB NOT NULL DEFAULT '{}',
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_version_per_node UNIQUE (node_id, version_no)
);

ALTER TABLE media_node ADD CONSTRAINT fk_current_version
    FOREIGN KEY (current_version_id) REFERENCES artifact_version(id)
    ON DELETE SET NULL;
```

一个节点可以有多个版本（多次生成的候选结果）。`winner = true` 标记当前选中的版本，`media_node.current_version_id` 指向它。`input_hash` 是上游依赖 winner + Prompt + 模型参数的哈希，用于 Stale 检测时判断当前版本是否仍有效。

M5 之后，用户点击运行时会立即创建一个 queued `artifact_version`，并通过唯一索引保证 production-generated version 与 `generation_job` 一一绑定。版本详情里展示该次运行的调用记录，而不是把所有 job 平铺在 Inspector 主界面。

### 2.13 model_provider / model_capability — 模型能力

```sql
CREATE TABLE model_provider (
    id            TEXT PRIMARY KEY,
    display_name  TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    config        JSONB NOT NULL DEFAULT '{}',
    enabled       BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE model_capability (
    id                         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id                TEXT NOT NULL REFERENCES model_provider(id) ON DELETE CASCADE,
    model_id                   TEXT NOT NULL,
    display_name               TEXT NOT NULL,
    output_types               JSONB NOT NULL DEFAULT '[]',
    supported_operations       JSONB NOT NULL DEFAULT '[]',
    supported_input_node_types JSONB NOT NULL DEFAULT '[]',
    limits                     JSONB NOT NULL DEFAULT '{}',
    pricing                    JSONB NOT NULL DEFAULT '{}',
    defaults                   JSONB NOT NULL DEFAULT '{}',
    enabled                    BOOLEAN NOT NULL DEFAULT true,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_model_capability UNIQUE (provider_id, model_id)
);
```

当前 provider 包括 `mock`、`volcengine` 和 `internal_ffmpeg`。Volcengine 当前启用文本 `doubao-seed-2-0-mini-260428`、图片 `doubao-seedream-5-0-260128`、视频 `doubao-seedance-1-0-pro-fast-251015`；音频模型记录为 hold/disabled。

### 2.14 node_stale_reason — Stale 原因

```sql
CREATE TABLE node_stale_reason (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    node_id             UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    upstream_node_id    UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    upstream_version_id UUID REFERENCES artifact_version(id) ON DELETE SET NULL,
    reason_code         TEXT NOT NULL,
    reason_message      TEXT NOT NULL DEFAULT '',
    details             JSONB NOT NULL DEFAULT '{}',
    resolved_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

上游 current winner、参考包成员、参考包成员 winner 等实质输入变化会写入 active stale reason。用户重跑并产生新版本后，相关 stale reason 会被清理或解析。

### 2.15 reference_pack_item — 参考包成员

```sql
CREATE TABLE reference_pack_item (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id   UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    pack_node_id   UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    member_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    position       INT NOT NULL DEFAULT 0,
    metadata       JSONB NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT reference_pack_item_no_self_member CHECK (pack_node_id <> member_node_id),
    CONSTRAINT unique_reference_pack_member UNIQUE (pack_node_id, member_node_id)
);
```

Reference Pack 是一种 `media_node(node_type='reference_pack')`，成员通过这张表管理。Pack membership 不等于 dependency edge；其他节点可以 dependency 到整个 Pack。

### 2.16 sandbox_job — 沙箱任务

```sql
CREATE TABLE sandbox_job (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id      UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    target_node_id    UUID REFERENCES media_node(id) ON DELETE SET NULL,
    generation_job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    job_type          TEXT NOT NULL,
    operation_type    TEXT NOT NULL,
    status            job_status NOT NULL DEFAULT 'pending',
    sandbox_id        TEXT,
    command           TEXT NOT NULL DEFAULT '',
    cwd               TEXT NOT NULL DEFAULT '/workspace',
    input             JSONB NOT NULL DEFAULT '{}',
    output            JSONB NOT NULL DEFAULT '{}',
    exit_code         INT,
    stdout            TEXT NOT NULL DEFAULT '',
    stderr            TEXT NOT NULL DEFAULT '',
    duration_ms       INT NOT NULL DEFAULT 0,
    error_code        TEXT,
    error_message     TEXT,
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

FFmpeg 首帧/尾帧提取、远程生成图片/视频下载入库等不可预测资源消耗任务必须记录为 `sandbox_job`，应用容器不直接执行这些命令。

### 2.17 索引

```sql
-- 账号
CREATE INDEX idx_account_email ON account(email);

-- workspace 维度（最常用的过滤条件）
CREATE INDEX idx_workspace_owner ON workspace(owner_id);
CREATE INDEX idx_workspace_owner_mode ON workspace(owner_id, mode);
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
CREATE INDEX idx_workspace_sandbox_status ON workspace_sandbox(status);

-- 版本查询
CREATE INDEX idx_artifact_version_node ON artifact_version(node_id);
CREATE UNIQUE INDEX idx_artifact_version_one_winner ON artifact_version(node_id) WHERE winner = true;
CREATE UNIQUE INDEX idx_artifact_version_job_unique ON artifact_version(job_id) WHERE job_id IS NOT NULL;
CREATE INDEX idx_artifact_version_node_status ON artifact_version(node_id, status, created_at DESC);

-- 生成任务
CREATE INDEX idx_generation_job_node ON generation_job(target_node_id);
CREATE INDEX idx_generation_job_status ON generation_job(workspace_id, status);

-- 模型能力、stale、参考包、沙箱任务
CREATE INDEX idx_model_capability_provider ON model_capability(provider_id);
CREATE INDEX idx_model_capability_enabled ON model_capability(provider_id, enabled);
CREATE INDEX idx_node_stale_reason_node_active ON node_stale_reason(node_id, created_at) WHERE resolved_at IS NULL;
CREATE INDEX idx_reference_pack_item_pack ON reference_pack_item(pack_node_id, position, created_at);
CREATE INDEX idx_reference_pack_item_member ON reference_pack_item(member_node_id);
CREATE INDEX idx_sandbox_job_workspace_status ON sandbox_job(workspace_id, status);
CREATE INDEX idx_sandbox_job_generation_job ON sandbox_job(generation_job_id);
```

## 3. tldraw 投影层类型

### 3.1 MediaShapeProps — tldraw shape 的 props

```typescript
import type { TLBaseShape } from 'tldraw'

type MediaType = 'text' | 'image' | 'video' | 'audio' | 'reference_pack'
type NodeStatus = 'draft' | 'ready' | 'queued' | 'running'
  | 'succeeded' | 'failed' | 'stale' | 'user_editing'

interface MediaShapeProps {
  nodeId: string
  nodeType: MediaType
  title: string
  prompt: string
  status: NodeStatus
  w: number
  h: number
  thumbnailUrl?: string
  productionPreview?: ProductionPreview
  referencePackPreview?: ReferencePackPreview
}

type MediaShape = TLBaseShape<'media', MediaShapeProps>
```

shape props 只保存画布渲染需要的业务投影。模型参数、版本列表、调用记录等重数据通过 selected node 的 API 按需加载，不塞进 tldraw store。

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
      prompt: node.prompt,
      status: node.status,
      productionPreview: node.productionPreview,
      referencePackPreview: node.referencePackPreview,
      thumbnailUrl: node.thumbnailUrl,
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
  edges: [ { id, fromNodeId, toNodeId, ... } ],
  groups: [ { id, name, nodeIds: [...] } ]
}
```

前端收到后：
1. `editor.createShapes(nodes.map(nodeToShape))` — 批量创建节点 shape
2. SVG connection overlay 根据 `edges + nodes` 渲染 dependency 连线
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
  → 调用 POST /api/edges { fromNodeId: A, toNodeId: B }
  → 后端做 DAG 环检测
  → 成功 → canvas payload 增加 edge，SVG overlay 渲染动效连线
  → 失败（成环）→ toast 提示"这条线会形成循环"
  → 失败（其他）→ toast 提示"连线失败，请重试"
```

**为什么业务操作不做乐观更新**：200ms 的等待在创建节点、建连线、编辑保存这些操作中用户几乎察觉不到。乐观更新需要处理回滚逻辑（删除已渲染的 shape、撤销状态变更），增加复杂度但收益极低。只有高频交互（拖拽）才需要本地即时响应，而那恰恰是丢失了也无所谓的位置数据。

### 4.4 通路 ③：后端事件 → WebSocket 推送

当前 `/ws/canvas` 已用于节点、连线、分组和节点状态/预览更新。M6 Agent 操作、Gate 和更细粒度任务进度也会走事件流，但还没有完整落地：

```
WebSocket /ws/canvas?workspaceId=xxx

事件格式：
{ type: "NodeCreated",   payload: { node: {...} } }
{ type: "NodeUpdated",   payload: { nodeId, changes: { status, thumbnailUrl, ... } } }
{ type: "NodeDeleted",   payload: { nodeId } }
{ type: "EdgeCreated",   payload: { edge: {...} } }
{ type: "EdgeDeleted",   payload: { edgeId } }
{ type: "NodeUpdated",   payload: { nodeId, changes: { status, productionPreview, ... } } }
{ type: "GateRequested", payload: { gateType, message, options } } // M6 目标态
```

前端按事件类型调用对应的 Editor API：

| 事件 | 前端响应 |
|---|---|
| NodeCreated | `editor.createShape(nodeToShape(node))` |
| NodeUpdated | `editor.updateShape({ id, props: changes })` |
| NodeDeleted | `editor.deleteShape(shapeId)` |
| EdgeCreated | 更新 canvas edges，SVG overlay 渲染连线 |
| EdgeDeleted | 更新 canvas edges，SVG overlay 移除连线 |
| NodeUpdated | 更新 status、尺寸、预览、参考包摘要等 props |
| GateRequested | 对话面板展示确认卡片（M6 目标态） |

### 4.5 冲突处理

当前 M3 的选择是 Studio / Agent mode 分离：Studio Workspace 可手工编辑，Agent Workspace 普通画布写接口会被拒绝。因此 M5 不处理“同一 workspace 内用户和 Agent 同时编辑同一节点”的冲突。

M6 如果引入 Agent 内部工具写画布，同时允许用户通过对话干预，应按以下目标态处理：

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
├── 001_init_schema.sql
├── ...
└── 014_m5_version_lifecycle.sql
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

### 6.1 workspace 表 mode 字段

```sql
CREATE TYPE workspace_mode AS ENUM ('studio', 'agent');
ALTER TABLE workspace ADD COLUMN mode workspace_mode NOT NULL DEFAULT 'studio';
```

`workspace.mode` 已在 M3 落地，用于 Studio / Agent 路由分流和普通画布写接口权限校验。

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
