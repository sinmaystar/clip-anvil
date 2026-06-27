# ClipAnvil 数据库与前后端交互设计

## 1. 数据架构总览

### 1.1 核心原则

业务数据库是唯一事实源。React Flow 是纯画布交互层：接收业务数据、渲染节点/连线/分组、捕获选择和拖拽，不持有独立事实源。

```
业务 DB（PostgreSQL）
  │
  ├── media_node (含画布坐标 x,y,w,h)  ──→  React Flow media node
  ├── media_edge                        ──→  React Flow dependency edge
  ├── media_group                       ──→  React Flow group node
  └── canvas_document (camera)          ──→  React Flow viewport
```

不存在前端画布 snapshot、不存在 canvas_record 表。画布上的每一个元素都能从业务表直接构建，不可能出现"画布和业务数据不一致"的问题。

### 1.2 为什么不存前端画布 snapshot

| 方案 | 问题 |
|---|---|
| 全量 snapshot（一个 JSONB 存所有前端节点） | 每次拖拽都要全量写入几十 KB，且 snapshot 中的业务字段（status、title）会与业务表脱节 |
| canvas_record（逐条存前端画布记录） | 本质上是业务表的副本，两套数据必然不一致——Agent 后台创建节点、网络断开、Bug 都会导致两边分叉 |
| **业务表直接存坐标**（当前方案） | 只有一个事实源，React Flow view-model 从业务表构建，不存在同步问题 |

## 2. 数据库表结构

### 2.0 当前迁移快照

当前 goose 迁移包含 `001_init_schema.sql` 到 `030_agent_semantic_identity.sql`，已覆盖 Studio 生产底座、Agent 三角色主链路、RenderPlan、Reviewer gate、Producer signal 和语义身份层：

- 枚举：`workspace_mode`、`node_type`、`asset_type`、`node_status`、`job_status`
- 核心表：`account`、`workspace`、`canvas_document`、`media_node`、`media_edge`、`media_group`、`media_asset`
- 沙箱表：`workspace_sandbox`、`sandbox_job`
- 生产表：`generation_job`、`artifact_version`、`model_provider`、`model_capability`、`node_stale_reason`、`reference_pack_item`
- Agent runtime 表：`agent_thread`、`agent_message`、`agent_task`、`agent_event`、`eino_checkpoint`、`producer_pending_signal`
- Agent 创作事实表：`creative_brief`、`project_memory`、`key_element`、`key_element_state`、`scene`、`shot`、`shot_key_element`、`shot_dependency`
- Agent 生产计划与评审表：`render_plan`、`review_record`、`artifact_issue`
- Agent 语义身份视图：`agent_object_index`
- 已收敛：`media_edge` 当前只表达 dependency，不再存 `edge_type` / transition 字段。
- 已扩展：`media_node` 当前包含 `operation_type`、`prompt_template`、`prompt_rich`、`prompt_refs`、`model_provider`、`model_id`、`model_params`、`current_version_id`、`metadata`。
- 已扩展：`artifact_version` 当前支持 queued/running/succeeded/failed/cancelled 生命周期，并通过 `job_id` 与 `generation_job` 一一绑定。

下面的 schema 记录当前已落地对象。真实可执行结构以 `apps/server/migrations/` 和 sqlc 生成代码为准。

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

扁平分组，不支持嵌套。对应画布上的 React Flow group node 和左侧资源树的文件夹。分组是纯组织工具，不影响依赖关系。

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
    -- 画布坐标（React Flow 渲染位置）
    canvas_x           REAL NOT NULL DEFAULT 0,
    canvas_y           REAL NOT NULL DEFAULT 0,
    canvas_w           REAL NOT NULL DEFAULT 200,
    canvas_h           REAL NOT NULL DEFAULT 120,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**关键设计**：画布坐标（`canvas_x/y/w/h`）直接存在业务节点表上，不存在独立的画布状态表。React Flow media node 从这张表构建，不需要前端画布 snapshot。

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

当前 provider 包括 `mock`、`volcengine` 和 `internal_ffmpeg`。Volcengine 当前启用文本 `doubao-seed-2-0-mini-260428`、`doubao-seed-2-0-pro-260215`、`doubao-seed-2-0-lite-260428`、`doubao-seed-2-1-turbo-260628`、`doubao-seed-2-1-pro-260628`，图片 `doubao-seedream-5-0-260128`，视频 `doubao-seedance-1-0-pro-fast-251015`；音频模型记录为 hold/disabled。

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

## 3. React Flow 投影层类型

### 3.1 CanvasFlowNodeData — React Flow node data

```typescript
import type { Edge, Node } from '@xyflow/react'

type MediaType = 'text' | 'image' | 'video' | 'audio' | 'reference_pack'
type NodeStatus = 'draft' | 'ready' | 'queued' | 'running'
  | 'succeeded' | 'failed' | 'stale' | 'user_editing'

interface CanvasFlowNodeData {
  kind: 'media'
  node: MediaNode
}

interface CanvasFlowGroupData {
  kind: 'group'
  group: MediaGroup
  nodeIds: string[]
}

interface CanvasFlowEdgeData {
  edge: MediaEdge
}

type CanvasFlowNode =
  | Node<CanvasFlowNodeData, 'media'>
  | Node<CanvasFlowGroupData, 'group'>

type CanvasFlowEdge = Edge<CanvasFlowEdgeData, 'dependency'>
```

React Flow node data 只保存画布渲染需要的业务投影。模型参数、版本列表、调用记录等重数据通过 selected node 的 API 按需加载，不塞进前端画布状态。

### 3.2 从业务数据构建 React Flow nodes/edges

页面加载时，从后端 API 获取 workspace 的完整 `CanvasPayload`，映射为 React Flow nodes/edges：

```typescript
function nodeToFlowNode(node: MediaNode): CanvasFlowNode {
  return {
    id: node.id,
    type: 'media',
    position: { x: node.canvas_x, y: node.canvas_y },
    width: mediaNodeDisplaySize(node).w,
    height: mediaNodeDisplaySize(node).h,
    data: { kind: 'media', node },
  }
}

function edgeToFlowEdge(edge: MediaEdge): CanvasFlowEdge {
  return {
    id: edge.id,
    type: 'dependency',
    source: edge.from_node_id,
    target: edge.to_node_id,
    data: { edge },
  }
}
```

## 4. 前后端交互流程

### 4.1 三条数据通路

```
                    ┌────────────────────────────────────┐
                    │         React Flow（浏览器内存）         │
                    │  nodes/edges/viewport → 即时渲染     │
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
1. `canvasToFlowNodes(canvas)` 派生 media/group nodes
2. `canvasToFlowEdges(canvas)` 派生 dependency edges
3. `cameraToViewport(camera)` 恢复视口
4. `CanvasFlowSurface` 根据 Studio/Agent policy 启用或禁用编辑能力

加载完成后 React Flow 可即时响应拖拽、选择和平移缩放；业务事实仍以后端 API 和 DB 为准。

### 4.3 通路 ②：用户操作 → 异步持久化

用户操作分两类，处理策略不同：

**位置类操作（拖拽、缩放、移动视口）— 容忍丢失**

```
用户拖拽节点
  → React Flow 内存 nodes 立即更新（0ms）
  → 画布立即响应
  → 防抖 2s 后批量写回
  → PATCH /api/nodes/batch-position [{ id, canvasX, canvasY }, ...]
  → 失败？静默重试一次，仍失败则忽略
  → 最坏结果：下次刷新位置回退
```

```
用户缩放/平移画布
  → React Flow viewport 立即更新（0ms）
  → 防抖 2s 后写回
  → PATCH /api/workspaces/:id/camera { x, y, zoom }
  → 失败？忽略
```

**业务操作（创建节点、建连线、编辑 Prompt、提交生成）— 必须成功**

```
用户点击"创建视频节点"
  → 调用 POST /api/nodes { nodeType: 'video', canvasX: 100, canvasY: 200 }
  → 等后端返回（~200ms）
  → 成功 → canvas payload 增加 node，React Flow 渲染节点
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

**为什么业务操作不做乐观更新**：200ms 的等待在创建节点、建连线、编辑保存这些操作中用户几乎察觉不到。乐观更新需要处理回滚逻辑（删除已渲染节点、撤销状态变更），增加复杂度但收益极低。只有高频交互（拖拽）才需要本地即时响应，而那恰恰是丢失了也无所谓的位置数据。

### 4.4 通路 ③：后端事件 → WebSocket 推送

当前 `/ws/canvas` 已用于节点、连线、分组和节点状态/预览更新。Agent 对话、工具调用、HITL 决策和任务进度通过 `/ws/agent` 推送：

```
WebSocket /ws/canvas?workspaceId=xxx

事件格式：
{ type: "NodeCreated",   payload: { node: {...} } }
{ type: "NodeUpdated",   payload: { nodeId, changes: { status, thumbnailUrl, ... } } }
{ type: "NodeDeleted",   payload: { nodeId } }
{ type: "EdgeCreated",   payload: { edge: {...} } }
{ type: "EdgeDeleted",   payload: { edgeId } }
{ type: "NodeUpdated",   payload: { nodeId, changes: { status, productionPreview, ... } } }
WebSocket /ws/agent?workspaceId=xxx

事件包括：
{ type: "message_created", payload: { message: {...} } }
{ type: "message_delta",   payload: { messageId, delta } }
{ type: "task_updated",    payload: { task: {...} } }
{ type: "event_created",   payload: { event: {...} } }
{ type: "decision_updated", payload: { eventId, status } }
```

前端按事件类型重新合并或局部更新 canvas payload：

| 事件 | 前端响应 |
|---|---|
| NodeCreated | 添加 media node，重新派生 React Flow node |
| NodeUpdated | 合并节点状态、尺寸、预览、参考包摘要等字段 |
| NodeDeleted | 移除 media node |
| EdgeCreated | 添加 dependency edge |
| EdgeDeleted | 移除 dependency edge |
| Agent message/event/task | 对话面板合并消息、流式增量、任务时间线和决策卡状态 |

### 4.5 冲突处理

当前选择是 Studio / Agent mode 分离：Studio Workspace 可手工编辑，Agent Workspace 普通画布写接口会被拒绝。Agent 通过后端生产工具写业务事实，用户通过对话、附件和决策卡干预。

如果后续引入同一 workspace 内的更强手工编辑能力，应按以下策略处理：

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
└── 030_agent_semantic_identity.sql
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

## 6. Agent 三角色与语义身份层

以下对象已经通过迁移 `015` 到 `030` 落地，支撑当前 Producer / Craftsman / Reviewer 三角色主链路。Composer 代码仍保留，但当前 Agent 主路径以创作事实源、RenderPlan、Worker、Reviewer gate 和 Producer pending signal 为核心。

### 6.1 workspace 表 mode 字段

```sql
CREATE TYPE workspace_mode AS ENUM ('studio', 'agent');
ALTER TABLE workspace ADD COLUMN mode workspace_mode NOT NULL DEFAULT 'studio';
```

`workspace.mode` 已在 M3 落地，用于 Studio / Agent 路由分流和普通画布写接口权限校验。

### 6.2 agent_thread / agent_task / agent_event / agent_message / eino_checkpoint

```sql
agent_thread (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    scope_type TEXT NOT NULL DEFAULT 'workspace',
    scope_id UUID,
    runtime_provider TEXT NOT NULL DEFAULT 'eino',
    runtime_agent_name TEXT NOT NULL DEFAULT '',
    current_checkpoint_key TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    summary TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT ''
);

agent_task (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    role TEXT NOT NULL,
    scope_type TEXT NOT NULL DEFAULT 'workspace',
    scope_id UUID,
    task_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    attempt INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 1,
    input JSONB NOT NULL DEFAULT '{}',
    output JSONB NOT NULL DEFAULT '{}',
    error_code TEXT,
    error_message TEXT,
    render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

agent_event (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    source_role TEXT NOT NULL DEFAULT 'system',
    target_role TEXT,
    scope JSONB NOT NULL DEFAULT '{}',
    payload JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    handled_at TIMESTAMPTZ
);

agent_message (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID NOT NULL REFERENCES agent_thread(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    role TEXT NOT NULL,
    message_type TEXT NOT NULL DEFAULT 'text',
    content JSONB NOT NULL DEFAULT '{}',
    raw_message JSONB NOT NULL DEFAULT '{}',
    task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    event_id UUID REFERENCES agent_event(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

eino_checkpoint (
    key TEXT PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    value BYTEA NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`agent_thread` 表示 Producer/Craftsman/Reviewer 的持久运行上下文；`agent_task` 记录 Producer turn、Craftsman/Reviewer turn、Worker generation、tool call、decision resume 等任务；`agent_event` 支撑 HITL 决策、工具事件和任务事件；`agent_message` 是对话历史和工具调用/工具结果历史；`eino_checkpoint` 是 Eino native checkpoint/resume 的 DB 存储。

消息持久化口径：

- 用户消息、assistant 文本、assistant tool_call、tool_result、decision card 都写入 `agent_message`。
- 前端流式 delta 不重复落库为完整 assistant 消息；最终消息以 DB 为准。
- Agent 历史消息最大轮次当前放大到开发期足够大，避免多轮生产链路丢上下文。

### 6.3 创作事实源

```sql
creative_brief (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '',
    video_type TEXT NOT NULL DEFAULT '',
    target_audience TEXT NOT NULL DEFAULT '',
    tone TEXT NOT NULL DEFAULT '',
    visual_style TEXT NOT NULL DEFAULT '',
    duration_sec DOUBLE PRECISION,
    aspect_ratio TEXT NOT NULL DEFAULT '',
    language TEXT NOT NULL DEFAULT '',
    objective TEXT NOT NULL DEFAULT '',
    concept TEXT NOT NULL DEFAULT '',
    constraints JSONB NOT NULL DEFAULT '[]',
    metadata JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'draft',
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT ''
);

project_memory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    version INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    core_intent TEXT NOT NULL DEFAULT '',
    soul TEXT NOT NULL DEFAULT '',
    brand_facts JSONB NOT NULL DEFAULT '[]',
    non_negotiables JSONB NOT NULL DEFAULT '[]',
    visual_anchors JSONB NOT NULL DEFAULT '[]',
    allowed JSONB NOT NULL DEFAULT '[]',
    forbidden JSONB NOT NULL DEFAULT '[]',
    prompt_injection_hints JSONB NOT NULL DEFAULT '[]',
    source_refs JSONB NOT NULL DEFAULT '[]',
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT ''
);

key_element (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    client_key TEXT NOT NULL DEFAULT '',
    element_type TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT '',
    source_refs JSONB NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'active',
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT ''
);

key_element_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    key_element_id UUID NOT NULL REFERENCES key_element(id) ON DELETE CASCADE,
    client_key TEXT NOT NULL DEFAULT '',
    label TEXT NOT NULL DEFAULT 'default',
    visual_description TEXT NOT NULL DEFAULT '',
    reference_status TEXT NOT NULL DEFAULT 'none',
    reference_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    reference_version_id UUID REFERENCES artifact_version(id) ON DELETE SET NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    state_facts JSONB NOT NULL DEFAULT '[]',
    source_refs JSONB NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'active',
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT ''
);

scene (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    client_key TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    mood TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'planned',
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT ''
);

shot (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    scene_id UUID REFERENCES scene(id) ON DELETE SET NULL,
    client_key TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL,
    title TEXT NOT NULL,
    brief JSONB NOT NULL DEFAULT '{}',
    shot_kind TEXT NOT NULL DEFAULT '',
    creative_text TEXT NOT NULL DEFAULT '',
    visual_intent TEXT NOT NULL DEFAULT '',
    action_text TEXT NOT NULL DEFAULT '',
    camera_intent TEXT NOT NULL DEFAULT '',
    dialogue TEXT NOT NULL DEFAULT '',
    narration TEXT NOT NULL DEFAULT '',
    audio_plan JSONB NOT NULL DEFAULT '{}',
    duration_sec DOUBLE PRECISION,
    narrative_purpose TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'planned',
    craftsman_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

shot_key_element (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    shot_id UUID NOT NULL REFERENCES shot(id) ON DELETE CASCADE,
    key_element_id UUID NOT NULL REFERENCES key_element(id) ON DELETE CASCADE,
    key_element_state_id UUID REFERENCES key_element_state(id) ON DELETE SET NULL,
    role TEXT NOT NULL DEFAULT '',
    required BOOLEAN NOT NULL DEFAULT true,
    sort_order INT NOT NULL DEFAULT 0
);

shot_dependency (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    from_shot_id UUID NOT NULL REFERENCES shot(id) ON DELETE CASCADE,
    to_shot_id UUID NOT NULL REFERENCES shot(id) ON DELETE CASCADE,
    dependency_type TEXT NOT NULL,
    required_artifact TEXT NOT NULL DEFAULT '',
    injection_role TEXT NOT NULL DEFAULT '',
    blocking_phase TEXT NOT NULL DEFAULT '',
    stale_policy TEXT NOT NULL DEFAULT 'mark_downstream_stale',
    reason TEXT NOT NULL DEFAULT '',
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

这些表是 Producer 写入的全局创作事实源。`ProjectMemory` 是项目级创作宪法；`KeyElement` / `KeyElementState` 是一致性锚点；`Scene` / `Shot` / `shot_dependency` 是分镜结构和连续性；`shot_key_element` 记录分镜使用哪些关键元素或状态。

### 6.4 RenderPlan / Worker 生产链路

```sql
render_plan (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    scope_type TEXT NOT NULL,          -- key_element_state / shot
    scope_id UUID NOT NULL,
    target_phase TEXT NOT NULL,        -- reference_image / preview_image / shot_video
    task_type TEXT NOT NULL DEFAULT 'generate',
    model_prompt_profile TEXT NOT NULL,
    operation TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    revision INT NOT NULL DEFAULT 1,
    forked_from_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    render_plan_key TEXT NOT NULL DEFAULT '',
    reference_bindings JSONB NOT NULL DEFAULT '[]',
    subject_bindings JSONB NOT NULL DEFAULT '[]',
    prompt_parts JSONB NOT NULL DEFAULT '{}',
    params JSONB NOT NULL DEFAULT '{}',
    compiled_prompt TEXT NOT NULL DEFAULT '',
    compiled_request JSONB NOT NULL DEFAULT '{}',
    prompt_audit JSONB NOT NULL DEFAULT '{}',
    blocker JSONB NOT NULL DEFAULT '{}',
    rationale TEXT NOT NULL DEFAULT '',
    submitted_worker_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    output_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    output_version_id UUID REFERENCES artifact_version(id) ON DELETE SET NULL,
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT ''
);

media_node (
    -- 既有字段省略
    shot_id UUID REFERENCES shot(id) ON DELETE SET NULL,
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    artifact_kind TEXT NOT NULL DEFAULT '',
    source_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL
);

generation_job (
    -- 既有字段省略
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    source_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL
);

artifact_version (
    -- 既有字段省略
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    artifact_kind TEXT NOT NULL DEFAULT '',
    source_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL
);
```

Craftsman 只写 `render_plan`。Producer 通过 `decide_render_plan` accept 后，工程代码创建 `worker_generation` task，Worker 构造 `GenerationIntent` 并复用共享 production service 写 `generation_job` / `artifact_version`。`media_node.source_render_plan_id`、`generation_job.source_render_plan_id` 和 `artifact_version.source_render_plan_id` 让画布、日志和 Agent 上下文能追溯到具体 RenderPlan。

### 6.5 review_record / artifact_issue

```sql
review_record (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    shot_id UUID REFERENCES shot(id) ON DELETE SET NULL,
    node_id UUID REFERENCES media_node(id) ON DELETE CASCADE,
    artifact_version_id UUID REFERENCES artifact_version(id) ON DELETE CASCADE,
    generation_job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    reviewer_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    reviewer_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    parent_review_record_id UUID REFERENCES review_record(id) ON DELETE SET NULL,
    target_phase TEXT NOT NULL,
    review_task TEXT NOT NULL DEFAULT 'preview_image_review',
    target_object_type TEXT NOT NULL DEFAULT 'artifact_version',
    target_object_id UUID,
    render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    status TEXT NOT NULL,
    attempt_no INT NOT NULL DEFAULT 1,
    max_attempts INT NOT NULL DEFAULT 3,
    overall_score REAL,
    required_axes JSONB NOT NULL DEFAULT '[]',
    rubric JSONB NOT NULL DEFAULT '{}',
    critique TEXT NOT NULL DEFAULT '',
    retry_recommendation JSONB NOT NULL DEFAULT '{}',
    escalation JSONB NOT NULL DEFAULT '{}',
    model_provider TEXT NOT NULL DEFAULT '',
    model_id TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

artifact_issue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    review_record_id UUID NOT NULL REFERENCES review_record(id) ON DELETE CASCADE,
    dimension TEXT NOT NULL,
    severity TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    target_object_type TEXT NOT NULL,
    target_object_id UUID NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    evidence TEXT NOT NULL DEFAULT '',
    suggested_fix TEXT NOT NULL DEFAULT 'none',
    fix_hint TEXT NOT NULL DEFAULT '',
    requires_user_confirmation BOOLEAN NOT NULL DEFAULT false,
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT ''
);
```

Reviewer 只写 `review_record` 和 `artifact_issue`，不直接重跑、不直接选择 winner。Producer 读取评审结果后决定 accept、派 Craftsman fork 新 RenderPlan、请求用户确认或停止自动重试。`ArtifactIssue.dimension` 覆盖 10 轴 rubric 和 pre-render 专用维度，例如 `faithfulness`、`subject_consistency`、`motion_physics`、`continuity`、`audio_sync`、`model_capability`、`prompt_validity`。

### 6.6 producer_pending_signal

```sql
producer_pending_signal (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    producer_thread_id UUID NOT NULL REFERENCES agent_thread(id) ON DELETE CASCADE,
    source_role TEXT NOT NULL,
    source_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    source_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    signal_type TEXT NOT NULL,
    scope_type TEXT NOT NULL,
    scope_id UUID,
    render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    message_id UUID REFERENCES agent_message(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    priority INT NOT NULL DEFAULT 100,
    dedupe_key TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    claimed_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    claimed_at TIMESTAMPTZ,
    processed_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    processed_at TIMESTAMPTZ,
    last_error TEXT,
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT ''
);
```

后台任务不会直接继续“冒充 Producer”做决策。Craftsman、Reviewer、Worker 或系统服务完成后写 `producer_pending_signal`，再创建或唤醒 Producer task。Producer 在模型轮次里读取 pending signal 文本，并通过 `read_project_context` 读取 DB 事实源做下一步决策。

信号语义：

- `craftsman_render_plan_ready`：Craftsman 已提交等待 Producer 决策的 RenderPlan。
- `worker_generation_completed` / `worker_generation_failed`：Worker 或 production broadcaster 通知产物成功/失败。
- `reviewer_result_submitted`：Reviewer 已提交 `review_record` / `artifact_issue`。
- dedupe key 保证同一生产事实不会重复唤醒 Producer。

### 6.7 semantic_key / agent_object_index

`030_agent_semantic_identity.sql` 给 Agent 相关领域对象、生产对象、消息线程和信号补充 `semantic_key` / `display_name`，并创建统一视图：

```sql
CREATE VIEW agent_object_index AS
SELECT workspace_id, 'shot' AS object_type, id AS object_id, semantic_key, display_name, ...
UNION ALL
SELECT workspace_id, 'render_plan' AS object_type, id AS object_id, semantic_key, display_name, ...
UNION ALL
SELECT workspace_id, 'artifact_version' AS object_type, id AS object_id, semantic_key, display_name, ...
-- creative_brief / project_memory / key_element / key_element_state / scene /
-- shot_dependency / media_node / generation_job / review_record /
-- artifact_issue / agent_thread / agent_task / producer_pending_signal
```

长期原则：提供给 Agent 的上下文和工具入参优先使用语义键，不要求模型记 UUID。工具内部通过 `agent_object_index`、sqlc 查询和对象归属校验把语义键解析回真实 UUID。

常见语义键示例：

| 对象 | 示例 |
|---|---|
| creative brief | `creative_brief.main` |
| project memory | `project_memory.v1` |
| key element | `product_luggage` |
| key element state | `product_luggage.state_silver_reference` |
| scene | `scene_airport_departure` |
| shot | `shot_01` |
| render plan | `shot_01.preview_image.rp1.ab12cd34` |
| artifact | `shot_01.preview_image.rp1.ab12cd34.output.ef56ab78.v1` |
| producer signal | `signal.shot_01.preview_image.rp1.ab12cd34.worker_generation_completed...` |

这层解决的是 Agent 工具参数容易幻觉 UUID 的问题。`read_project_context` 会返回 ObjectIndex 和 production state 里的语义引用；`dispatch_craftsman`、`decide_render_plan`、`dispatch_reviewer`、`upsert_render_plan` 等工具优先接受这些语义引用。

### 6.8 旧 agent_session 方案

早期设计曾提过 `agent_session`，当前实现没有采用这张表。实际运行态由 `agent_thread`、`agent_task`、`agent_event`、`agent_message` 和 `eino_checkpoint` 组合表达。

## 相关文档

- [整体设计](../design/overview.md) — 架构、原则、路线图
- [画布设计](../design/canvas.md) — React Flow 投影层和数据通路
- [Studio 模式](../design/studio-mode.md) — 用户主导的创作交互
- [Agent 模式](../design/agent-mode.md) — Agent 驱动的生产交互
