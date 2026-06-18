# Studio / Agent 生产底层数据库技术方案

**状态**：待评审
**日期**：2026-06-18
**阶段目标**：定义 Studio 模式和 Agent 模式共享的生产底层数据库结构，覆盖节点、引用、Reference Pack、生成任务、版本、评审、Stale、模型能力、Agent 分镜和事件队列，并用真实 Studio/Agent 场景推演设计是否满足需求。

## 1. 设计目标

本方案回答三个问题：

1. Studio 和 Agent 如何复用同一套底层存储？
2. 每张表负责什么事实，哪些状态不能重复存？
3. Studio 手动运行和 Agent 自动运行是否能走同一套生成、版本、评审和 Stale 链路？

核心结论：

- `media_node` 是画布上的可运行产物单元。
- `media_asset` 是文件级存储，不是主要交互对象。
- `artifact_version` 是节点每次运行的产物版本。
- `generation_job` 记录一次模型调用或内部媒体处理任务。
- `sandbox_job` 记录一次沙箱执行事实；FFmpeg、Agent shell、Composer、脚本类任务必须落这张表。
- `media_edge` 只记录节点之间的 dependency 输入候选关系，不再保留多种边类型。
- `media_node.prompt_refs` 记录 Prompt 中 `@` 显式引用了哪些输入候选。
- `reference_pack` 作为一种 node 存在，成员关系由 `reference_pack_item` 表显式记录。
- Agent 的 `shot` 是生产语义，不是 Studio 必备节点。
- Studio 和 Agent 都提交 `GenerationIntent`，由 Provider Bridge 转为具体供应商请求。
- Provider Bridge 可以转向外部模型供应商，也可以转向 sandbox-backed internal provider；应用进程不本地执行不可预测资源命令。

## 2. 当前状态

当前已落地迁移包含：

- `account`
- `workspace`
- `canvas_document`
- `media_node`
- `media_edge`
- `media_group`
- `media_asset`
- `workspace_sandbox`

当前 schema 能支撑 Studio M1.x：

- 登录注册。
- Workspace。
- 文本/图片/视频/音频节点。
- dependency 连线。
- 分组。
- 上传和 MinIO 存储。
- 画布坐标和 camera。
- OpenSandbox workspace 绑定。
- OpenSandbox Sandbox Job Service 是 M4.S 的生产执行基础，负责会话、volume、输入输出传输、命令执行和失败归因。

当前缺口：

- `reference_pack` 节点类型。
- 节点 operation、prompt template、模型参数和 current version。
- 生成任务和版本表的真实迁移。
- Prompt `@` 引用的结构化记录，存储在 `media_node.prompt_refs`。
- Reference Pack membership。
- 模型能力表。
- Agent shot、shot_dependency、thread/message、Eino checkpoint、task、event、HITL decision。
- `sandbox_job` 和 generation job 的关联。
- Workspace 模式复制/导入来源追踪。

## 2.1 Eino 调研结论

本方案按 Eino 官方文档的当前能力边界设计 Agent 存储：

- Memory / Session / Store 是业务层概念，不是 Eino 内置数据库能力。ClipAnvil 需要自行实现 `agent_thread` 和 `agent_message`，并把 Eino 原始消息快照保存在 `raw_message` 中，便于恢复、审计和调试。
- Interrupt / Resume 依赖 `CheckPointStore`。Eino 提供接口和运行时语义，但 checkpoint 的持久化需要业务自己实现，因此需要 `eino_checkpoint` 表。
- Human-in-the-loop 可以用 Eino ADK 的 interrupt / stateful interrupt 语义承载。ClipAnvil 的 `request_user_decision` 应该是一个 HITL 工具：触发 interrupt、写 checkpoint、推送 UI 卡片，用户选择后 resume。
- A2UI 可以作为“Agent 事件转 UI 卡片”的参考协议，但首版不直接引入 A2UI 表结构。ClipAnvil 先用 `agent_event` + `agent_message(message_type=ui_card)` + 前端自有 card schema 实现。

参考：

- Eino Memory and Session: https://www.cloudwego.io/docs/eino/quick_start/chapter_03_memory_and_session/
- Eino CheckPoint / Interrupt: https://www.cloudwego.io/docs/eino/core_modules/chain_and_graph_orchestration/checkpoint_interrupt/
- Eino ADK Human-in-the-loop: https://www.cloudwego.io/docs/eino/core_modules/eino_adk/agent_hitl/
- Eino Interrupt / Resume Quickstart: https://www.cloudwego.io/docs/eino/quick_start/chapter_07_interrupt_resume/
- Eino A2UI Protocol: https://www.cloudwego.io/docs/eino/quick_start/chapter_10_a2ui_protocol/

## 3. 命名和兼容策略

当前数据库都是测试数据，可以清空并重新创建，因此不需要为了历史数据保留复杂兼容层。当前 `media_node.node_type` 使用 `media_type` enum，值为 `text/image/video/audio`。但目标态需要 `reference_pack`，并且 `media_asset.type` 与 `media_node.node_type` 语义开始分离。

建议采用重建策略：

1. 新增 `node_type` enum，包含 `text/image/video/audio/reference_pack`。
2. 新增 `asset_type` enum，包含 `text/image/video/audio/json`.
3. 重建 `media_node.node_type` 为新的 `node_type`。
4. 重建 `media_asset.type` 为新的 `asset_type`。
5. `media_edge` 直接收敛为 dependency-only 表，不再保留 `edge_type`。

目标态中：

```text
node_type 表示节点输出类型和画布渲染类型。
asset_type 表示实际存储文件或文本/JSON 资产类型。
operation_type 表示节点如何产生输出。
```

示例：

```text
Image Node:
  node_type = image
  operation_type = text_to_image
  output asset_type = image

Reference Pack Node:
  node_type = reference_pack
  operation_type = collect_references
  output asset_type = json 或直接存在 artifact_version.output
```

## 4. 枚举类型

```sql
CREATE TYPE workspace_mode AS ENUM ('studio', 'agent');

CREATE TYPE node_type AS ENUM (
    'text',
    'image',
    'video',
    'audio',
    'reference_pack'
);

CREATE TYPE asset_type AS ENUM (
    'text',
    'image',
    'video',
    'audio',
    'json'
);

CREATE TYPE node_status AS ENUM (
    'draft',
    'ready',
    'queued',
    'running',
    'succeeded',
    'failed',
    'stale',
    'user_editing'
);

CREATE TYPE job_status AS ENUM (
    'pending',
    'queued',
    'running',
    'succeeded',
    'failed',
    'cancelled'
);

CREATE TYPE review_verdict AS ENUM (
    'approved',
    'rejected',
    'needs_revision'
);
```

说明：

- Studio 和 Agent 共享的节点边只保留 dependency。多种边类型会增加用户理解成本，也会让 Studio 和 Agent 的运行依赖变得不清楚。
- Agent 的分镜顺序和跨分镜依赖使用 `shot_dependency`，不复用 `media_edge` 的边类型。
- 如果 PostgreSQL enum 后续变更成本太高，可以把 `operation_type`、`task_type`、`dependency_type` 等保留为 `TEXT + CHECK` 或配置校验。

## 4.1 sandbox_job

`sandbox_job` 是沙箱执行事实表，不替代 `generation_job`。一个 `generation_job` 可以委托一个或多个 `sandbox_job`，M4 首版先支持内部媒体 provider 写入一个关联 sandbox job。

目标态：

```sql
CREATE TABLE sandbox_job (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    target_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    generation_job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    job_type TEXT NOT NULL,
    operation_type TEXT NOT NULL,
    status job_status NOT NULL DEFAULT 'pending',
    sandbox_id TEXT,
    command TEXT NOT NULL DEFAULT '',
    cwd TEXT NOT NULL DEFAULT '/workspace',
    input JSONB NOT NULL DEFAULT '{}',
    output JSONB NOT NULL DEFAULT '{}',
    exit_code INT,
    stdout TEXT NOT NULL DEFAULT '',
    stderr TEXT NOT NULL DEFAULT '',
    duration_ms INT NOT NULL DEFAULT 0,
    error_code TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

规则：

- `sandbox_job` 可以记录 Agent shell、内部 FFmpeg、Composer 合成、输入准备和后续脚本任务。
- `sandbox_id`、workspace volume、临时路径、presigned URL 不参与 `artifact_version.input_hash`。
- `generation_job.provider_response.sandbox_job_id` 是生产任务追踪 sandbox 执行的第一版轻量关联。

## 5. Workspace 层

### 5.1 workspace

目标态：

```sql
CREATE TABLE workspace (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    owner_id UUID NOT NULL REFERENCES account(id),
    mode workspace_mode NOT NULL DEFAULT 'studio',
    source_workspace_id UUID REFERENCES workspace(id) ON DELETE SET NULL,
    created_from_mode workspace_mode,
    settings JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

字段说明：

| 字段 | 说明 |
|---|---|
| `mode` | 当前 workspace 是 Studio 还是 Agent |
| `source_workspace_id` | 复制/导入来源 workspace |
| `created_from_mode` | 来源模式，用于区分 Agent -> Studio 和 Studio -> Agent |
| `settings` | UI 偏好、默认模型、预算策略等配置 |

设计取舍：

- `mode` 建议成为显式字段，而不是只放在 `settings`，因为权限、PSS、复制/导入流程都依赖它。
- Agent 长期记忆不放在 `workspace` 行内，使用独立 `memory_document` 表。这样未来 Campaign 可以拥有更高层全局记忆，每个 Workspace 也可以拥有自己的记忆，并且二者可以用同一套读写、审计和 PSS 注入逻辑。

### 5.2 memory_document

统一记忆表。Campaign 模式尚未落地，但表结构先保留 scope 抽象。

```sql
CREATE TABLE memory_document (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope_type TEXT NOT NULL,
    scope_id UUID NOT NULL,
    memory JSONB NOT NULL DEFAULT '{}',
    summary TEXT NOT NULL DEFAULT '',
    version INT NOT NULL DEFAULT 1,
    updated_by_type TEXT NOT NULL DEFAULT 'system',
    updated_by_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_memory_scope UNIQUE (scope_type, scope_id)
);
```

scope 示例：

| scope_type | scope_id | 说明 |
|---|---|---|
| `workspace` | workspace.id | 单条视频/单个 workspace 的记忆 |
| `campaign` | campaign.id | 未来 Campaign 级全局记忆 |

Workspace memory 示例：

```json
{
  "product": {
    "name": "燕麦拿铁",
    "category": "即饮咖啡",
    "selling_points": ["低糖", "健康", "顺滑"]
  },
  "brand": {
    "tone": "年轻、干净、轻盈",
    "forbidden": ["竞品 Logo", "医疗化承诺"]
  },
  "creative_direction": {
    "mood_anchor": "clean lifestyle, warm morning light, commercial quality",
    "strategy": "AIDA"
  },
  "notes": []
}
```

### 5.3 memory_revision

记忆变更需要审计，尤其是 Agent 自动写入时。

```sql
CREATE TABLE memory_revision (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_document_id UUID NOT NULL REFERENCES memory_document(id) ON DELETE CASCADE,
    old_memory JSONB NOT NULL DEFAULT '{}',
    new_memory JSONB NOT NULL DEFAULT '{}',
    reason TEXT NOT NULL DEFAULT '',
    changed_by_type TEXT NOT NULL DEFAULT 'system',
    changed_by_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 5.4 workspace_import_analysis

Studio -> Agent 需要一次大模型理解过程，建议单独记录。

```sql
CREATE TABLE workspace_import_analysis (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    target_workspace_id UUID REFERENCES workspace(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    input_summary JSONB NOT NULL DEFAULT '{}',
    proposed_memory JSONB NOT NULL DEFAULT '{}',
    proposed_shots JSONB NOT NULL DEFAULT '[]',
    proposed_node_mappings JSONB NOT NULL DEFAULT '[]',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);
```

用途：

- 保存 Studio DAG 被理解成 Agent 结构的过程。
- 用户确认前不直接污染 Agent workspace。
- 失败后可以重新分析。

## 6. Canvas 和组织层

### 6.1 canvas_document

沿用当前设计：

```sql
CREATE TABLE canvas_document (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL UNIQUE REFERENCES workspace(id) ON DELETE CASCADE,
    camera_x REAL NOT NULL DEFAULT 0,
    camera_y REAL NOT NULL DEFAULT 0,
    camera_zoom REAL NOT NULL DEFAULT 1,
    layout_version INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.2 media_group

`media_group` 只做布局组织，不做模型引用语义。

```sql
CREATE TABLE media_group (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    collapsed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

规则：

- Group 可以包含任意 node。
- Group 不产生输出。
- Group 不参与 GenerationIntent。
- 删除 Group 不删除节点。

## 7. 资源和节点层

### 7.1 media_asset

文件级资源表。

```sql
CREATE TABLE media_asset (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    type asset_type NOT NULL,
    mime TEXT NOT NULL,
    storage_url TEXT,
    text_content TEXT,
    thumbnail_url TEXT,
    duration_ms INT,
    size_bytes BIGINT,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

字段说明：

| 字段 | 说明 |
|---|---|
| `storage_url` | 图片、视频、音频、长文本、JSON 文件在 MinIO 的地址 |
| `text_content` | 短文本输出可直接存储，避免每次读对象存储 |
| `thumbnail_url` | 图片缩略图、视频封面、音频波形图等 |
| `metadata` | 尺寸、帧率、分辨率、provider 原始信息等 |

约束建议：

```sql
CHECK (
    storage_url IS NOT NULL
    OR text_content IS NOT NULL
)
```

说明：

- Asset 是底层存储对象，不直接作为 Studio/Agent 主要交互对象。
- 用户上传文件时，系统创建 asset，同时创建对应 node。
- Reference Pack 不直接收裸 asset，必须通过 node 间接引用。

### 7.2 media_node

目标态核心表。

```sql
CREATE TABLE media_node (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    group_id UUID REFERENCES media_group(id) ON DELETE SET NULL,
    shot_id UUID,
    node_type node_type NOT NULL,
    operation_type TEXT NOT NULL DEFAULT 'manual',
    title TEXT NOT NULL DEFAULT '',
    status node_status NOT NULL DEFAULT 'draft',
    prompt_template TEXT NOT NULL DEFAULT '',
    prompt_rich JSONB NOT NULL DEFAULT '{}',
    prompt_refs JSONB NOT NULL DEFAULT '[]',
    model_provider TEXT,
    model_id TEXT,
    model_params JSONB NOT NULL DEFAULT '{}',
    current_version_id UUID,
    source TEXT NOT NULL DEFAULT 'user',
    sort_order INT NOT NULL DEFAULT 0,
    canvas_x REAL NOT NULL DEFAULT 0,
    canvas_y REAL NOT NULL DEFAULT 0,
    canvas_w REAL NOT NULL DEFAULT 240,
    canvas_h REAL NOT NULL DEFAULT 180,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

后续添加 FK：

```sql
ALTER TABLE media_node
    ADD CONSTRAINT fk_media_node_current_version
    FOREIGN KEY (current_version_id) REFERENCES artifact_version(id)
    ON DELETE SET NULL;
```

字段说明：

| 字段 | 说明 |
|---|---|
| `shot_id` | Agent 模式可选关联 shot；Studio 通常为空 |
| `node_type` | 节点输出类型和渲染类型 |
| `operation_type` | 节点如何产生输出，如 `text_to_image`、`extract_last_frame` |
| `prompt_template` | 带 `{{node:id.output}}` 占位符的文本模板 |
| `prompt_rich` | 富文本编辑器结构，保存 chip、缩略图、文本 spans |
| `prompt_refs` | Prompt 中显式 `@` 引用的输入节点列表 |
| `model_provider` | 如 `volcengine`、`internal` |
| `model_id` | provider 下的模型或内部工具 ID |
| `model_params` | 分辨率、时长、比例、seed、质量等 |
| `current_version_id` | 当前 winner 版本 |
| `source` | `user` 或 `agent` |
| `metadata` | reference_pack role、text subtype、UI 扩展等 |

典型 metadata：

```json
{
  "text_subtype": "storyboard_text",
  "reference_pack_role": "product_identity",
  "locked": false
}
```

### 7.3 media_edge

节点输入候选关系。目标态只有 dependency 一种边关系，因此不需要 `edge_type` 字段。

```sql
CREATE TABLE media_edge (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    from_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    to_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    source TEXT NOT NULL DEFAULT 'user',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT no_self_loop CHECK (from_node_id != to_node_id),
    CONSTRAINT unique_edge UNIQUE (from_node_id, to_node_id)
);
```

规则：

- `to_node` 可以使用 `from_node` 的 selected output 作为输入候选。
- Studio 和 Agent 的画布运行 DAG 只使用这一种关系。
- 手动连线创建 `media_edge`。
- Prompt `@` 引用未连线节点时自动创建 `media_edge`。
- 有 `media_edge` 但未在 Prompt 中 `@` 的输入是隐式参考输入，UI 需要提示“未在 Prompt 中引用”。
- Reference Pack membership 不使用 media_edge。
- 分镜顺序、首尾帧连续性等 Agent 生产语义使用 `shot_dependency`，不放进 `media_edge`。

### 7.4 prompt_refs

`prompt_refs` 存在 `media_node` 行内，记录 Prompt 中显式 `@` 引用了哪些输入候选。

示例：

```json
[
  {
    "placeholder_key": "ref_1",
    "source_node_id": "node-product-pack",
    "display_label": "商品身份包",
    "kind": "reference_pack",
    "sort_order": 1
  },
  {
    "placeholder_key": "ref_2",
    "source_node_id": "node-script",
    "display_label": "视频脚本",
    "kind": "text",
    "sort_order": 2
  }
]
```

为什么需要结构化保存：

- `media_edge` 表示“输入候选”：B 运行时可以使用 A 的输出。
- `prompt_refs` 表示“Prompt 里的显式引用”：用户在 B 的 Prompt 哪个位置写了 `@A`。
- 富文本编辑器需要稳定 placeholder。
- Provider Bridge 需要按引用顺序展开输入。

交互删除规则：

| 用户操作 | 数据变化 |
|---|---|
| 在 Prompt 中删除 `@A` chip | 从 `prompt_refs` 删除 A，但保留 A -> B 的 `media_edge` |
| 用户手动画布删除 A -> B edge | 删除 `media_edge`，并从 B 的 `prompt_refs` 删除 A 或将 chip 标记失效 |
| 用户在 Prompt 中 `@` 未连线节点 C | 自动创建 C -> B 的 `media_edge`，并写入 `prompt_refs` |

Provider Bridge 组装输入：

```text
connected_inputs = media_edge 上游节点
explicit_refs = prompt_refs 中显式 @ 的节点
implicit_refs = connected_inputs - explicit_refs
```

规则：

- `explicit_refs` 按 Prompt 中 placeholder 位置渲染。
- `implicit_refs` 作为普通参考输入传给支持参考输入的模型。
- 如果模型不支持隐式参考输入，则运行前提示用户移除连线、换模型，或在 Prompt 中改成纯文本描述。

### 7.5 reference_pack_item

Reference Pack membership。

```sql
CREATE TABLE reference_pack_item (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    pack_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    member_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT '',
    label TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_reference_pack_member UNIQUE (pack_node_id, member_node_id),
    CONSTRAINT no_pack_self_member CHECK (pack_node_id != member_node_id)
);
```

规则：

- `pack_node_id` 必须指向 `node_type = reference_pack` 的节点。
- `member_node_id` 可以是 text/image/video/audio 等普通产物节点；不建议允许 pack 嵌套 pack，避免引用展开和 Stale 传播复杂化。
- Pack 不自动收纳 member 的上游依赖。
- Pack 被引用时，只展开直接 member 的 selected outputs。

## 8. 生成、版本和评审

### 8.1 generation_job

一次运行记录。既可以是模型生成，也可以是内部 ffmpeg/media processing。

```sql
CREATE TABLE generation_job (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    target_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    parent_job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    operation_type TEXT NOT NULL,
    provider TEXT NOT NULL,
    model_id TEXT NOT NULL,
    intent JSONB NOT NULL DEFAULT '{}',
    rendered_prompt TEXT NOT NULL DEFAULT '',
    provider_request JSONB NOT NULL DEFAULT '{}',
    provider_response JSONB NOT NULL DEFAULT '{}',
    status job_status NOT NULL DEFAULT 'pending',
    progress INT NOT NULL DEFAULT 0,
    attempt INT NOT NULL DEFAULT 1,
    max_attempts INT NOT NULL DEFAULT 1,
    retry_policy JSONB NOT NULL DEFAULT '{}',
    cost_cents INT,
    error_code TEXT,
    error_message TEXT,
    requested_by_type TEXT NOT NULL DEFAULT 'user',
    requested_by_id TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

字段说明：

| 字段 | 说明 |
|---|---|
| `intent` | ClipAnvil 稳定 GenerationIntent 快照 |
| `rendered_prompt` | Provider Bridge 渲染后的最终文本 |
| `provider_request` | 供应商请求快照，便于审计和复现 |
| `provider_response` | 供应商原始响应摘要 |
| `requested_by_type` | `user`、`producer`、`craftsman`、`system` |
| `provider` | `volcengine`、`internal_ffmpeg` 等 |
| `parent_job_id` | 如果这是一次重试，指向被重试的上一条 job |
| `attempt/max_attempts` | 功能级自动重试或 Agent 重试的次数记录 |
| `retry_policy` | 重试条件、退避、是否允许 Agent 改写 Prompt |

内部处理示例：

```text
extract_last_frame:
  provider = internal_ffmpeg
  model_id = ffmpeg
  operation_type = extract_last_frame
```

失败记录规则：

- 模型能力校验失败、provider 返回失败、内部 ffmpeg 失败都需要落库。
- `error_code` 和 `error_message` 必填，便于 UI、Agent 和审计解释。
- 功能级自动重试创建新的 `generation_job`，通过 `parent_job_id` 串联。
- Agent 改写 Prompt 后重试同样创建新的 `generation_job`，并在 `agent_task.output` 或 review 记录中说明改写原因。

### 8.2 artifact_version

节点产物版本。

```sql
CREATE TABLE artifact_version (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    job_id UUID REFERENCES generation_job(id) ON DELETE SET NULL,
    asset_id UUID REFERENCES media_asset(id) ON DELETE SET NULL,
    version_no INT NOT NULL,
    winner BOOLEAN NOT NULL DEFAULT false,
    output JSONB NOT NULL DEFAULT '{}',
    review_score REAL,
    input_hash TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_version_per_node UNIQUE (node_id, version_no)
);
```

`output` 用于非文件或补充产物：

```json
{
  "text_preview": "0-3 秒展示燕麦拿铁拉花，3-8 秒呈现低糖卖点，结尾出现品牌 CTA。",
  "reference_pack_members": ["node-a", "node-b"],
  "last_frame_asset_id": "asset-last-frame",
  "provider_artifact_id": "remote-task-output-id"
}
```

winner 约束建议使用部分唯一索引：

```sql
CREATE UNIQUE INDEX idx_artifact_version_one_winner
ON artifact_version(node_id)
WHERE winner = true;
```

### 8.3 review_record

评审记录。

```sql
CREATE TABLE review_record (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    version_id UUID NOT NULL REFERENCES artifact_version(id) ON DELETE CASCADE,
    axes JSONB NOT NULL DEFAULT '{}',
    score REAL NOT NULL,
    critique TEXT NOT NULL DEFAULT '',
    verdict review_verdict NOT NULL,
    reviewer TEXT NOT NULL DEFAULT 'auto',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Studio 可以选择不开自动评审，也可以手动运行评审。Agent 默认使用自动评审驱动 retry。

### 8.4 input_hash

`artifact_version.input_hash` 用于判断下游是否 Stale。

建议 hash 输入：

- target node prompt_template。
- target node model_provider/model_id/model_params。
- prompt_refs 中显式引用节点的 current_version_id。
- media_edge 上游隐式输入节点的 current_version_id。
- reference_pack 的 member_node_id 和 member current_version_id。
- operation_type。
- Provider Bridge 关键输入规范版本。

当上游 winner 或 pack membership 改变时，下游重新计算 input_hash，若不匹配则标记 stale。

## 9. 模型能力和 Provider Bridge

### 9.1 model_provider

```sql
CREATE TABLE model_provider (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

示例：

```text
id = volcengine
provider_type = media_generation
```

Agent LLM provider 由 Eino 管理，但也可以在这里记录可选配置。

### 9.2 model_capability

```sql
CREATE TABLE model_capability (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id TEXT NOT NULL REFERENCES model_provider(id),
    model_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    output_types JSONB NOT NULL DEFAULT '[]',
    supported_operations JSONB NOT NULL DEFAULT '[]',
    supported_input_node_types JSONB NOT NULL DEFAULT '[]',
    limits JSONB NOT NULL DEFAULT '{}',
    pricing JSONB NOT NULL DEFAULT '{}',
    defaults JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_model_capability UNIQUE (provider_id, model_id)
);
```

示例：

```json
{
  "provider_id": "volcengine",
  "model_id": "seedance-video-standard",
  "output_types": ["video"],
  "supported_operations": ["text_to_video", "image_to_video", "multi_reference_to_video"],
  "supported_input_node_types": ["text", "image", "video", "audio", "reference_pack"],
  "limits": {
    "max_reference_images": 9,
    "max_reference_videos": 3,
    "durations_sec": [4, 5, 8, 10, 15],
    "aspect_ratios": ["9:16", "16:9", "1:1"]
  },
  "pricing": {
    "tier": "premium"
  }
}
```

用途：

- 前端模型下拉过滤和置灰。
- 后端 GenerationIntent 校验。
- Agent 根据预算选择模型。
- Provider Bridge 决定如何映射参数。

## 10. Agent 生产语义

### 10.1 shot

Agent 模式分镜实体。

```sql
CREATE TABLE shot (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    sort_order INT NOT NULL,
    title TEXT NOT NULL,
    brief JSONB NOT NULL DEFAULT '{}',
    duration_sec REAL,
    narrative_purpose TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'planned',
    craftsman_thread_id UUID,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

说明：

- Studio 模式不需要 shot。
- Agent 模式使用 shot 做稳定分镜锚点。
- `media_node.shot_id` 可选关联到 shot，用于投影某个分镜的图片、视频、文本或参考节点。

### 10.2 shot_dependency

跨分镜生产依赖。

```sql
CREATE TABLE shot_dependency (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    from_shot_id UUID NOT NULL REFERENCES shot(id) ON DELETE CASCADE,
    to_shot_id UUID NOT NULL REFERENCES shot(id) ON DELETE CASCADE,
    dependency_type TEXT NOT NULL,
    required_artifact TEXT,
    injection_role TEXT,
    blocking_phase TEXT,
    stale_policy TEXT NOT NULL DEFAULT 'mark_downstream_stale',
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT no_shot_self_dependency CHECK (from_shot_id != to_shot_id)
);
```

示例：

```text
dependency_type = last_frame_continuity
required_artifact = video_winner.last_frame_asset
injection_role = first_frame
blocking_phase = video_generation
```

### 10.3 agent_thread

Producer 和 Craftsman 的运行时会话绑定。

Eino 官方文档将 Memory / Session / Store 定义为业务层概念：Eino 负责处理消息，业务层负责保存和恢复消息历史。因此 ClipAnvil 需要自行持久化 Agent 对话，而不是期待 Eino 内置数据库表。

`agent_thread` 表记录 Producer、Craftsman 等 Agent 会话的业务作用域和 Eino runtime 绑定。

```sql
CREATE TABLE agent_thread (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    scope_type TEXT NOT NULL,
    scope_id UUID,
    runtime_provider TEXT NOT NULL DEFAULT 'eino',
    runtime_thread_id TEXT,
    current_checkpoint_id TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    summary TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Craftsman：

```text
role = craftsman
scope_type = shot
scope_id = shot.id
```

Producer:

```text
role = producer
scope_type = workspace
scope_id = workspace.id
```

说明：

- `agent_thread` 是持久 Agent 身份和业务作用域的索引，不一定存完整消息。
- Craftsman 是有状态的，状态体现在 `agent_message`、Eino checkpoint 和 `summary` 上。
- `agent_task` 是一次次异步执行记录，可以指向同一个 Craftsman thread。

### 10.4 agent_message

Agent 对话、工具调用、工具结果、Interrupt 和 UI 卡片消息流水。

```sql
CREATE TABLE agent_message (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID NOT NULL REFERENCES agent_thread(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    role TEXT NOT NULL,
    message_type TEXT NOT NULL DEFAULT 'text',
    content JSONB NOT NULL DEFAULT '{}',
    raw_message JSONB NOT NULL DEFAULT '{}',
    task_id UUID,
    event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_agent_message_seq UNIQUE (thread_id, seq)
);
```

字段说明：

| 字段 | 说明 |
|---|---|
| `role` | `user`、`assistant`、`tool`、`system` |
| `message_type` | `text`、`tool_call`、`tool_result`、`interrupt`、`ui_card` |
| `content` | ClipAnvil 业务层消息内容 |
| `raw_message` | Eino/schema 原始消息快照，便于重放和排查 |
| `task_id` | 关联某次 agent_task，可为空 |
| `event_id` | 关联某条 agent_event，可为空 |

说明：

- Producer 对话面板主要读写 Producer thread 的 `agent_message`。
- Craftsman 的分镜级历史写在对应 shot thread 下。
- PSS 不直接等于消息历史；PSS 仍从 DB 事实构建，消息历史用于恢复对话和上下文。

### 10.5 eino_checkpoint

Eino Interrupt/Resume 需要 CheckPointStore。官方 CheckPointStore 是 KV 接口，业务需要自行实现持久化。ClipAnvil 用 Postgres 表承载。

```sql
CREATE TABLE eino_checkpoint (
    key TEXT PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    task_id UUID,
    value BYTEA NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

用途：

- 保存 Eino interrupt/resume 的中间执行状态。
- 支持进程重启后继续 resume。
- 支持 Human-in-the-loop 卡片等待用户选择后继续执行。

### 10.6 agent_task

异步 Agent 任务。它记录的是一次性执行，不等于 Craftsman 本身。

示例：

- Producer 收到用户消息：可以创建一个 producer task。
- shot-03 Craftsman 生成预览图：创建一个 craftsman task，复用 shot-03 的 `craftsman_thread_id`。
- Worker 调一次生图 API：创建一个 worker task。
- Composer 合成成片：创建一个 composer task。

Craftsman 是持久 thread；Craftsman 的每次行动是 `agent_task`。

```sql
CREATE TABLE agent_task (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    parent_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    role TEXT NOT NULL,
    scope_type TEXT NOT NULL,
    scope_id UUID,
    task_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    attempt INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    input JSONB NOT NULL DEFAULT '{}',
    output JSONB NOT NULL DEFAULT '{}',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);
```

### 10.7 agent_event

工程层消息队列和唤醒机制。

`agent_event` 不是 Agent 的长期记忆，而是“发生了什么，需要谁处理”的事实流。Producer 可以被 pending event 唤醒，读取 PSS 后继续决策。

```sql
CREATE TABLE agent_event (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    source_role TEXT NOT NULL,
    target_role TEXT,
    scope JSONB NOT NULL DEFAULT '{}',
    payload JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    handled_at TIMESTAMPTZ
);
```

事件类型示例：

```text
task_completed
task_failed
retry_exhausted
decision_requested
decision_resolved
asset_uploaded
shot_stale
final_video_ready
```

### 10.8 Decision / HITL 事件

`request_user_decision` 是 Producer 的工具，也应实现为 Eino Human-in-the-loop interrupt 工具，而不是简单地写事件后阻塞业务代码。

首版不单独建 `decision_request` 表。使用：

- `agent_event(event_type=decision_requested)` 承载前端待渲染卡片。
- `agent_message(message_type=ui_card)` 记录对话流中的卡片消息。
- `eino_checkpoint` 保存 Eino resume 所需中间状态。

示例 payload：

```json
{
  "title": "确认分镜计划",
  "message": "我规划了 5 个分镜，总时长 30 秒，是否开始生成预览图？",
  "options": [
    {"id": "approve", "label": "确认并生成预览图"},
    {"id": "revise", "label": "修改分镜"},
    {"id": "auto", "label": "预算内自动继续"}
  ],
  "default_option": "approve",
  "risk_level": "cost_increasing",
  "scope": {
    "shot_ids": ["shot-01", "shot-02", "shot-03", "shot-04", "shot-05"]
  }
}
```

当用户点击卡片或自然语言回复时：

- 更新原 `decision_requested` event 为 `handled`。
- 写入新的 `decision_resolved` event。
- 写入用户选择对应的 `agent_message`。
- 调用 Eino resume，将选择结果传回 interrupt ID。
- 唤醒 Producer。

如果未来需要复杂查询、过期策略、权限或统计，再把 decision materialize 成独立表。

## 11. 推荐索引

```sql
CREATE INDEX idx_workspace_owner ON workspace(owner_id);
CREATE INDEX idx_workspace_mode ON workspace(owner_id, mode);

CREATE INDEX idx_memory_document_scope ON memory_document(scope_type, scope_id);
CREATE INDEX idx_memory_revision_document ON memory_revision(memory_document_id, created_at DESC);

CREATE INDEX idx_media_asset_workspace ON media_asset(workspace_id);

CREATE INDEX idx_media_node_workspace ON media_node(workspace_id);
CREATE INDEX idx_media_node_status ON media_node(workspace_id, status);
CREATE INDEX idx_media_node_group ON media_node(group_id);
CREATE INDEX idx_media_node_shot ON media_node(shot_id);
CREATE INDEX idx_media_node_type ON media_node(workspace_id, node_type);

CREATE INDEX idx_media_edge_workspace ON media_edge(workspace_id);
CREATE INDEX idx_media_edge_from ON media_edge(from_node_id);
CREATE INDEX idx_media_edge_to ON media_edge(to_node_id);

CREATE INDEX idx_reference_pack_item_pack ON reference_pack_item(pack_node_id);
CREATE INDEX idx_reference_pack_item_member ON reference_pack_item(member_node_id);

CREATE INDEX idx_generation_job_node ON generation_job(target_node_id);
CREATE INDEX idx_generation_job_status ON generation_job(workspace_id, status);

CREATE INDEX idx_artifact_version_node ON artifact_version(node_id);
CREATE UNIQUE INDEX idx_artifact_version_one_winner
ON artifact_version(node_id)
WHERE winner = true;

CREATE INDEX idx_review_record_version ON review_record(version_id);

CREATE INDEX idx_shot_workspace ON shot(workspace_id, sort_order);
CREATE INDEX idx_shot_dependency_from ON shot_dependency(from_shot_id);
CREATE INDEX idx_shot_dependency_to ON shot_dependency(to_shot_id);

CREATE INDEX idx_agent_thread_scope ON agent_thread(workspace_id, role, scope_type, scope_id);
CREATE INDEX idx_agent_message_thread ON agent_message(thread_id, seq);
CREATE INDEX idx_eino_checkpoint_workspace ON eino_checkpoint(workspace_id, updated_at DESC);
CREATE INDEX idx_agent_task_status ON agent_task(workspace_id, status);
CREATE INDEX idx_agent_event_pending ON agent_event(workspace_id, status, created_at);
```

## 12. 场景推演：Studio 从想法到脚本和分镜文本

目标：用户在 Studio 里手动创建文本创作链路。

### 12.1 创建想法节点

用户创建 Text Node A：

```text
node_type = text
operation_type = text_generation
prompt_template = 帮我想一个燕麦拿铁广告创意
model_provider = volcengine
model_id = doubao-text
```

运行后：

- 创建 `generation_job`。
- Provider Bridge 调 LLM 文本生成。
- 写 `media_asset(type=text, text_content='燕麦拿铁广告创意方向：清爽晨光、低糖轻负担、办公室场景')`。
- 写 `artifact_version(node_id=A, asset_id=text_asset, version_no=1, winner=true)`。
- 更新 `media_node.current_version_id` 和 `status=succeeded`。

### 12.2 基于想法生成脚本

用户创建 Text Node B，在 Prompt 中 @A：

```text
prompt_template = 基于 {{node:A.output}}，把它写成 30 秒视频脚本
```

系统写：

- `media_edge(A -> B, source=prompt_ref)`
- 更新 B 的 `prompt_refs`，加入 `{source_node_id: A, placeholder_key: 'node:A.output'}`

运行 B 时：

- Provider Bridge 读取 A 的 current winner text。
- 渲染最终 prompt。
- 创建 B 的 job、asset、version。

### 12.3 基于脚本拆分分镜文本

用户创建 Text Node C，在 Prompt 中 @B：

```text
prompt_template = 基于 {{node:B.output}}，拆成 5 个分镜，每个包含时长、画面和旁白
```

结果：

```text
A -> B -> C
```

数据库满足需求：

- Studio 不需要 shot 表。
- 用户可以在画布上自由搭文本 DAG。
- 每一步都有版本和可追溯输入。
- C 的输出未来可被 Studio -> Agent import analysis 理解成 shots。

## 13. 场景推演：Studio 商品 Reference Pack

目标：用户上传商品图，生成不同视角，再把选中的图加入商品身份包。

### 13.1 上传商品主图

用户上传图片：

- 写 `media_asset(type=image, storage_url='s3://clip-anvil/workspace-123/product-main.png')`。
- 创建 `media_node(node_type=image, operation_type=upload, current_version_id=version-product-main-1)`。
- 创建 `artifact_version(winner=true)` 指向上传 asset。

### 13.2 基于主图生成九宫格视角

用户创建 Image Node B，Prompt 中 @A：

```text
prompt_template = 基于 {{node:A.output}}，生成 9 张不同视角的商品参考图
operation_type = image_to_image
```

系统：

- 写 A -> B dependency。
- 更新 B 的 `prompt_refs`，记录 Prompt 中对 A 的显式引用。
- 校验模型支持 `image_to_image`。
- 生成 B 的 image version。

### 13.3 创建 Reference Pack

用户创建 Reference Pack Node P：

```text
node_type = reference_pack
operation_type = collect_references
metadata.reference_pack_role = product_identity
title = 商品身份包
```

用户把 A 和 B 拖入 P：

```text
reference_pack_item(pack_node_id=P, member_node_id=A, label='产品主图')
reference_pack_item(pack_node_id=P, member_node_id=B, label='九宫格视角')
```

不创建 A -> P 或 B -> P 的 generation dependency edge，因为 pack membership 不是生成依赖。

### 13.4 引用 Reference Pack 生成广告主视觉

用户创建 Image Node C，在 Prompt 中 @P：

```text
prompt_template = 基于 {{node:P.output}}，生成适合小红书封面的广告主视觉
operation_type = multi_image_to_image
```

系统：

- 创建 P -> C dependency edge。
- 更新 C 的 `prompt_refs`，记录 Prompt 中对 P 的显式引用。
- Provider Bridge 展开 P 的直接成员 A、B 的 current winner。
- 不展开 A 或 B 的上游依赖。

数据库满足需求：

- Reference Pack 是可渲染、可拖动、可整体引用的 node。
- Pack 只显式收纳 node。
- Pack 整体被下游引用。
- 上游 provenance 可查，但不自动进入 pack 输入。

## 14. 场景推演：Studio 提取尾帧并续写视频

目标：用户生成一段视频后，用尾帧续接下一段视频。

### 14.1 Video Node A 生成视频

```text
node_type = video
operation_type = image_to_video
prompt_template = 基于 {{node:product_pack.output}}，生成 5 秒产品展示视频
```

运行后：

- 生成 video asset。
- 写 artifact_version winner。

### 14.2 Image Node B 提取尾帧

用户点击 A 的“提取尾帧”，系统创建 Image Node B：

```text
node_type = image
operation_type = extract_last_frame
model_provider = internal
model_id = ffmpeg
prompt_template = ''
```

系统写：

- `media_edge(A -> B, source=system)`
- `generation_job(provider=internal_ffmpeg, model_id=ffmpeg, operation_type=extract_last_frame)`
- B 的 artifact_version 指向尾帧 image asset。

### 14.3 Video Node C 引用尾帧续写

用户创建 Video Node C：

```text
prompt_template = 从 {{node:B.output}} 的画面继续，让杯子被拿起并转到办公桌场景
operation_type = image_to_video
```

Provider Bridge：

- 读取 B 的 image winner。
- 判断模型是否支持 image_to_video。
- 提交火山视频生成 API。

数据库满足需求：

- Studio 不需要特殊“首帧/尾帧 edge role”。
- 首帧/尾帧是普通 image node 输出。
- Worker 不必是 Agent，可以是内部 media processing job。
- Agent 模式也可自动创建同样的 B 节点。

## 15. 场景推演：模型能力不兼容

目标：用户 @ 引用图片，但手动选择了只支持文生图的模型。

状态：

```text
Image Node B:
operation_type = image_to_image
prompt_template = 基于 {{node:A.output}} 改成更高级的商业摄影风格
model_id = text-only-image-model
```

提交前：

- 前端根据 `model_capability` 置灰或警告。
- 用户仍尝试运行时，后端执行 capability validate。

后端发现：

```text
operation_type=image_to_image
input_refs include image node
selected_model.supported_operations only contains text_to_image
```

结果：

- 拒绝创建 running job。
- 可创建 failed generation_job，或直接返回 400。
- 错误信息写明模型不支持图片引用。

数据库满足需求：

- 用户可以自由选模型。
- 系统不会把不支持的输入悄悄丢掉。
- 能力校验由配置驱动。

## 16. 场景推演：手动连线但 Prompt 未 @

目标：用户希望先通过画布连线选择输入候选，但暂时不在 Prompt 中明确引用。

用户手动画线：

```text
Image Node A -> Video Node B
```

系统写：

```text
media_edge:
from_node_id = A
to_node_id = B
source = user_line

media_node B:
prompt_refs = []
```

UI 展示：

```text
输入资源：
- Image Node A：已连接，未在 Prompt 中引用
```

运行 B 时：

```text
connected_inputs = [A]
explicit_refs = []
implicit_refs = [A]
```

Provider Bridge 行为：

- 如果模型支持参考图或参考视频，A 作为普通参考输入传入。
- 如果模型要求 Prompt 中出现图序，Bridge 可以自动追加中性说明，或提示用户在 Prompt 中 `@A` 后再运行。
- 如果模型不支持参考输入，拒绝运行并提示移除连线或换模型。

Stale 行为：

- A 的 winner 变化时，B 仍然 stale。
- 因为 `media_edge` 表示 B 的输入候选，输入候选变化会影响 B 的 input_hash。

用户后来在 Prompt 中 `@A`：

```text
prompt_template = 从 {{ref_1}} 的画面继续生成一个 5 秒视频
prompt_refs = [{source_node_id: A, placeholder_key: "ref_1"}]
```

数据库满足需求：

- 手动连线仍然是专业 Studio 的主交互。
- `@` 只表达 Prompt 中的明确语义位置。
- 没有 `@` 的输入不会丢失，也不会隐藏；UI 会提示。

## 17. 场景推演：Agent 规划分镜并生成预览图

目标：Producer 规划 5 个分镜，Craftsman 处理其中一个 shot。

### 17.1 Producer 创建 shots

Producer 调 `update_storyboard`，写：

```text
shot-01: 开场钩子
shot-02: 倒入燕麦拿铁
shot-03: 低糖卖点视觉化
shot-04: 饮用场景
shot-05: 品牌收尾
```

数据库：

- 插入 5 行 `shot(status=planned)`。
- 如果用户确认，更新为 `approved`。

### 17.2 Producer 创建跨 shot 依赖

如果 shot-03 需要接 shot-02 末帧：

```text
shot_dependency:
from_shot_id = shot-02
to_shot_id = shot-03
dependency_type = last_frame_continuity
required_artifact = video_winner.last_frame_asset
blocking_phase = video_generation
```

### 17.3 Craftsman 创建预览图节点

Producer dispatch shot-03 Craftsman。

Craftsman 创建或使用 Image Node P3：

```text
media_node:
node_type = image
operation_type = text_to_image 或 multi_image_to_image
shot_id = shot-03
source = agent
prompt_template = 用清爽的视觉隐喻表现燕麦拿铁低糖卖点，保持明亮晨光和商业摄影质感
```

如果引用商品 Reference Pack：

- 写 P -> P3 dependency。
- 写 `prompt_refs`，记录 Prompt 中对商品 Reference Pack 的显式引用。

运行后：

- 写 generation_job。
- 写 artifact_version。
- 写 review_record。
- 通过后 winner=true。
- shot 状态更新为 `preview_ready`。

数据库满足需求：

- Agent shot 和 media_node 分离。
- Craftsman 可以围绕 shot 持久工作。
- 画布仍展示普通 image node。
- Studio/Agent 共用 generation/version/review。

## 18. 场景推演：Agent 视频生成和 Stale

目标：shot-03 视频依赖 shot-02 末帧，shot-02 重跑后 downstream stale。

### 18.1 shot-02 生成视频并抽取末帧

shot-02 Video Node V2 成功：

- `artifact_version(V2, winner=true)`
- `output.last_frame_asset_id = asset-last-frame-02`

系统可自动创建 Image Node LF2：

```text
node_type = image
operation_type = extract_last_frame
shot_id = shot-02
source = agent
```

### 18.2 shot-03 视频引用 LF2

shot-03 Video Node V3：

```text
prompt_template = 从 {{node:LF2.output}} 延续，表现低糖卖点
```

写：

- LF2 -> V3 dependency。
- prompt_refs。

### 18.3 shot-02 重跑

如果 V2 winner 改变：

- LF2 stale 或重新生成。
- V3 input_hash 失效。
- V3 标记 stale。
- Producer 在 PSS 中看到 shot-03 video stale。

Studio 行为：

- 用户看到 V3 stale，决定是否手动重跑。

Agent 行为：

- Producer 根据预算和用户偏好决定自动重跑，或调用 `request_user_decision`。

数据库满足需求：

- Stale engine 共享。
- Studio 和 Agent 的响应策略不同。
- 跨 shot 依赖通过 shot_dependency 表达，实际生成输入通过 media_edge 和 prompt_refs 落到节点 DAG。

## 19. 场景推演：Agent -> Studio 复制

目标：用户想把 Agent 成果复制到 Studio 手动精修。

复制内容：

- workspace: 新建 `mode=studio`, `source_workspace_id=agent_workspace_id`, `created_from_mode=agent`
- canvas_document: 复制 camera 或重置。
- media_asset: 复制记录或共享对象存储路径，按权限策略决定。
- media_node: 复制节点、坐标、operation、prompt_template、model 配置。
- media_edge: 复制 dependency。
- reference_pack_item: 复制 pack membership。
- artifact_version: 复制版本和 winner。
- media_group: 复制布局分组。

不复制为活跃状态：

- shot。
- shot_dependency。
- agent_task。
- agent_event。
- pending decision event。
- agent_thread。
- agent_message。
- eino_checkpoint。

原因：

- Studio 副本只允许用户手动编辑。
- Agent 上下文不能继续跟随 Studio 副本。

数据库满足需求：

- 底层媒体生产结果完整保留。
- Agent 上下文隔离。
- 用户明确进入 Studio 工作方式。

## 20. 场景推演：Studio -> Agent 导入

目标：用户在 Studio 做了自由 DAG，希望交给 Agent 继续自动生产。

流程：

1. 新建 `workspace_import_analysis`。
2. 收集 Studio 节点：
   - text nodes。
   - reference_pack nodes。
   - current winners。
   - dependency graph。
   - prompt templates。
3. 调用 Producer/LLM 分析。
4. 生成 proposed memory 和 proposed shots。
5. 用户确认。
6. 创建 Agent workspace。
7. 复制必要 nodes/assets/versions/reference packs。
8. 写 `shot` 和 `media_node.shot_id` 映射。

数据库满足需求：

- Studio 不需要提前符合 Agent shot 结构。
- Agent 不直接接管原 Studio workspace。
- 导入分析结果可审计、可失败重试、可用户确认。

## 21. 关键约束和实现顺序

### 21.1 必须保持的约束

- Node 是 Studio 和 Agent 的共同生产单元。
- Asset 不直接暴露为主要交互对象。
- Reference Pack 只显式收纳 node。
- Pack membership 不等于 dependency edge。
- Prompt `@` 引用必须结构化落库。
- Provider Bridge 不允许静默丢弃模型不支持的输入。
- Stale 传播由 DB 事实和 input_hash 驱动。
- Studio 和 Agent 通过复制/导入互通，不原地切换。

### 21.2 建议实施顺序

Phase 1: 生成和版本基础

- `generation_job`
- `artifact_version`
- `review_record`
- `media_node.current_version_id`
- `media_node.operation_type`
- `media_node.prompt_template`
- `media_node.model_provider/model_id/model_params`

Phase 2: Prompt @ 引用

- `media_node.prompt_refs`
- `media_edge.source=prompt_ref`
- Prompt rich JSON。
- Provider Bridge 展开输入。

Phase 3: Reference Pack

- `reference_pack` node_type。
- `reference_pack_item`。
- Pack 渲染和 membership。
- Pack 展开到 GenerationIntent。

Phase 4: Model capabilities 和 Provider Bridge

- `model_provider`
- `model_capability`
- capability validate。
- 火山图片/视频 adapter。
- internal ffmpeg adapter。

Phase 5: Agent 生产语义

- `shot`
- `shot_dependency`
- `agent_thread`
- `agent_message`
- `eino_checkpoint`
- `agent_task`
- `agent_event`
- Producer PSS builder。

Phase 6: Workspace 复制/导入

- `workspace.mode`
- `source_workspace_id`
- Agent -> Studio copy。
- Studio -> Agent import analysis。

## 22. 已收敛决策

1. 当前数据库都是测试数据，可以清空并重建，不需要为旧 schema 保留复杂兼容策略。
2. Text asset 允许长文本只存在 MinIO，DB 可以只存摘要或短文本；短文本可直接存在 `media_asset.text_content`。
3. `prompt_refs` 表示 Prompt 中的 `@` 显式引用，`media_edge` 表示输入候选依赖。删除 `@` 只删除 `prompt_refs`，不自动删除手动连线。
4. Reference Pack 不支持嵌套，避免展开、Stale 和 UI 心智复杂化。
5. Pack membership 变化需要让下游依赖节点 stale。
6. 失败需要落库，包括模型能力校验失败、provider 失败、内部处理失败；失败原因、错误码和重试关系都要可追踪。
7. Provider 原始请求和响应需要完整记录。为避免表过大，超大 payload 后续可以转存对象存储，但 `generation_job.provider_request/provider_response` 中必须保留索引、摘要和对象地址。
8. Model capability 首版手动配置，后续再考虑管理后台或 provider API 同步。
9. Studio -> Agent import analysis 使用专门导入向导，不用普通对话卡片承载；该功能优先级靠后。

## 23. 剩余开放问题

1. `media_asset.text_content` 的直接存储上限是多少，超过多长转存 MinIO？
2. 是否需要 `artifact_version` 级别的 Pack 快照，保证历史 generation job 复现时使用当时的 pack membership？
3. `agent_message.raw_message` 保存 Eino 原始消息的具体 schema 需要在实现时跟当前 Eino 版本对齐。
4. `agent_event` 作为 decision 承载时，是否需要过期时间和自动取消策略？
5. `generation_job.provider_request/provider_response` 的大字段归档策略如何做，避免长期拖慢查询？
6. `eino_checkpoint.value` 使用 BYTEA 直接存储，还是超过阈值后转存对象存储？
