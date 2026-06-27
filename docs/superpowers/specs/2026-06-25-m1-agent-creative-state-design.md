# M1 Agent 创作状态设计

**状态**：待评审
**日期**：2026-06-25
**里程碑**：M1，三 Agent v1 MultiAgent 重构

## 目标

M1 建立 Agent 模式下的视频创作事实源。完成后，Producer 可以把用户需求和上传素材转成结构化项目事实：创意简报、项目创作宪法、关键元素、元素状态、场景、分镜、分镜与元素引用关系、连续性依赖。

M1 不生成图片或视频。它只准备 M2 的 Craftsman / RenderPlan / PromptCompiler 所需要的事实层。

M1 的核心产品承诺是：

```text
Producer 掌控全局创作状态。
画布可以展示视频要做什么、哪些元素重要、哪些参考资源缺失。
Craftsman 和 Reviewer 不是 M1 完成条件。
```

## 当前状态

当前 Agent 系统已经具备：

- Agent 运行时表：`agent_thread`、`agent_task`、`agent_event`、`agent_message`、`eino_checkpoint`。
- 生产表：`media_node`、`media_asset`、`generation_job`、`artifact_version`、`model_capability`、`review_record`。
- 基础 Storyboard 表：`shot`、`shot_dependency`，以及可空的 `media_node.shot_id`。
- Producer 作为面向用户的 Eino tool-loop。
- 已有工具，例如 `read_workspace_context`、`get_production_state`、`update_storyboard`、`dispatch_craftsman`、`review_shot`、`request_user_decision`。

M1 缺失的是更完整的创作领域模型。当前 `shot.brief` JSONB 太松，无法表达可复用场景锚点、prompt 派生参考、项目级约束，也不适合把创作过程投影到画布。

本 spec 的实现设计可以完全忽略现有 Agent 工具和旧图结构。旧实现只用于识别迁移风险，不作为 M1 重构的代码风格或接口基线。

## 设计决策

### 三 Agent v1

M1 使用三 Agent v1 设计：

- Producer 是全局创作状态 owner。
- Craftsman 不参与 M1 执行；M1 只为后续 Craftsman 工作准备事实。
- Reviewer 不参与 M1 执行。
- StoryArchitect 在 v1 不作为独立 Agent。Producer 吸收轻量 StoryArchitect 职责：brief、memory、key elements、scenes、shots 和 continuity。

### 业务数据库是事实源

M1 表是业务事实，不是画布快照。React Flow / CanvasPayload 应该从这些对象派生画布投影。Agent 工具写领域对象；画布投影读取领域对象。

### 参考资源需求是状态，不是分镜临时 prompt

如果用户说“机场场景”或“柔光房间”，但没有上传对应参考图，Producer 必须创建 `KeyElement` 和 `KeyElementState`，并设置 `reference_status='needs_reference'`。M1 不生成这张参考图；M2 会派 Craftsman 生成。

这样可以避免未来每个 shot 级 Craftsman 任务各自发明一个不同的机场或房间。

### JSONB 只用于可扩展约束

需要检索、关联、索引或投影的字段，应该使用列或关系表。JSONB 只用于可扩展列表和低频结构化约束，例如 `source_refs`、`non_negotiables`、`audio_plan`。

## 范围

范围内：

- M1 创作状态表和 `shot` 扩展迁移。
- 新对象的 sqlc queries 和 server services。
- Producer 工具：
  - `upsert_project_brief`
  - `update_project_memory`
  - `upsert_key_elements`
  - `upsert_storyboard`
  - `read_project_context`
- Producer prompt / context 调整为三 Agent v1。
- PSS / Full Context Packet 支持新领域对象。
- M1 对象的只读画布投影。
- 单元测试、集成测试，以及一个基于悦行行李箱示例的 E2E / smoke 路径。

范围外：

- `render_plan`。
- PromptCompiler。
- Seedream / Seedance 执行。
- Craftsman 生成任务。
- Reviewer rubric 和 `artifact_issue`。
- `reference_bundle` 表。
- Timeline / Composer 升级。
- 领域节点的完整画布编辑能力。

## Eino-native 实现约束

M1 开始的新 Agent 图和工具实现必须使用 Eino-native 口径：

- Producer 图使用 Eino `compose.Graph` 构建，工具执行节点使用原生 `compose.NewToolNode` / `AddToolsNode`，不能再引入手写伪 tool loop 或文本 JSON 工具解析。
- 工具实现使用 Eino 标准工具接口，一个工具一个实现类。实现类至少提供 `Info(ctx) (*schema.ToolInfo, error)` 和 `InvokableRun(ctx, argumentsInJSON string, ...tool.Option) (string, error)` 这类 Eino 工具契约。
- `ToolInfo.ParamsOneOf` 优先通过 `github.com/cloudwego/eino/components/tool/utils.GoStruct2ParamsOneOf[T]` 从 Go struct 生成。入参 struct 使用 `json`、`jsonschema:"required"`、`jsonschema:"enum=a,enum=b"`、`jsonschema_description` 等 tag 描述字段。
- 工具函数体内仍必须做二次校验：必填字段、UUID 格式、枚举值、对象归属、workspace mode、数组长度和跨字段约束。模型侧 schema 只用于减少错误调用，不是安全边界。
- 每个工具正常返回值必须是字符串类型的自然语言观察。可以先由结构化结果拼接出中文摘要，但不能把裸 JSON 直接丢给模型。
- 每个工具都不能把业务错误直接抛给模型。参数错误、状态冲突、权限/归属错误、DB 错误和内部服务错误都要转成可理解、可重试的中文字符串，例如“工具调用失败：`mode` 只能是 create、patch、replace。请修正参数后重试。”只有图构建失败、依赖未注入这类不可恢复配置错误可以在启动/编译阶段返回 error。
- `request_user_decision` 仍依赖 Eino 原生 interrupt/resume，但工具返回给模型的内容也必须是自然语言；恢复后说明用户选择了什么，以及 Producer 下一步应该继续读取哪个状态。

推荐工具实现骨架：

```go
type UpsertProjectBriefTool struct {
    service CreativeStateService
}

type UpsertProjectBriefInput struct {
    Brief string `json:"brief" jsonschema:"required" jsonschema_description:"一句话说明本次调用要达成的业务目的，方便审计和模型自我纠错"`
    Mode  string `json:"mode" jsonschema:"required,enum=create,enum=patch,enum=archive" jsonschema_description:"create 创建新 brief；patch 局部更新当前 brief；archive 归档指定 brief"`
}
```

字段描述要面向模型写，不能只面向工程师写。好的字段描述应同时说明“这个字段是什么、什么时候填写、不能填什么、错了如何重试”。字段名必须稳定、语义具体、避免同义重复：例如用 `visual_intent` 表示高层视觉目标，用 `camera_intent` 表示创意级镜头意图；不要在 Producer 工具里使用 `prompt`、`model_prompt`、`provider_prompt` 这类会诱导 Producer 编写模型原生 prompt 的字段。

## Prompt 设计结论

本轮审阅了 Planora 的 system prompt、skills prompt、Seedance 2.0 prompt optimizer skill，以及外部整理的 system prompt 设计文档。可以采纳的不是通用 Agent 的全部能力，而是它的分层契约和工具描述方法。

Planora 值得吸收的模式：

- System prompt 按角色、语言、格式、核心原则、Agent loop、工具规则、错误处理、环境信息、关键规则分层，便于后续审阅和局部替换。
- Agent loop 明确“分析上下文 -> 规划 -> 选择工具 -> 执行动作 -> 观察 -> 迭代 -> 交付”，适合 Producer 这类 Full ReAct 角色。
- 工具描述需要写清支持动作、使用规则、推荐使用时机和不要做什么。工具描述不是给人看的注释，而是模型选择工具和修正参数的主要依据。
- 每次工具调用最好有 `brief` / `reason` 这类自然语言目的字段，帮助模型自我检查，也方便审计。
- 错误处理要禁止重复同一失败动作，工具应该返回可纠正的失败观察。

Seedance prompt optimizer 值得进入 ClipAnvil 的规则：

- Producer 必须知道“素材引用需要语义桥接”“关键元素需要稳定锚点”“复杂视频要分镜”“一镜一运镜”“非关键缺失可自动补全，关键歧义要问用户”这些规则。
- 但 Producer 不应该负责最终 Seedance / Seedream prompt 的工程化写法。Producer 只写创意级事实和一致性约束；Craftsman 把这些事实翻译成 `prompt_parts`；PromptCompiler 再按模型 profile 渲染最终 prompt。
- 对“机场场景”“柔光房间”这类用户提到但未上传素材的元素，Producer 必须先沉淀为 `KeyElementState(reference_status='needs_reference')`，而不是让每个分镜各自生成。

### Producer System Prompt 结构

Producer 的 system prompt 应采用以下结构：

1. **角色定义**：你是 ClipAnvil Producer，负责“从灵感到分镜，再到可生成的视频画布”的全局创作状态，不是模型 prompt 工程师，也不是实际生成执行者。
2. **语言与日期**：默认中文；工具入参里的自然语言字段也使用中文；当前日期动态注入。
3. **核心职责**：理解用户意图、分析素材、维护 `CreativeBrief`、维护 `ProjectMemory`、识别 `KeyElement` / `KeyElementState`、规划 scene / shot、设置连续性依赖、调度后续 Craftsman / Reviewer。
4. **职责边界**：不写 provider 原生 prompt，不直接提交 Seedream / Seedance job，不直接评审生成质量，不绕过工具写库，不把画布投影当事实源。
5. **创作状态原则**：先写稳定事实，再推进生产；结构化生产关键点，不结构化所有创意细节；简单请求快速建最小对象，复杂请求再展开完整 storyboard。
6. **一致性原则**：`ProjectMemory` 是创作宪法，`KeyElementState` 是视觉锚点，分镜必须引用锚点，不把一致性约束散落在聊天文本里。
7. **Seedream / Seedance 认知摘要**：只放 Producer 决策需要知道的能力边界，例如先图后视频、参考人物数量风险、首尾帧/尾帧串联、视频 4-15 秒、局部问题优先 edit/extend/bridge，不放完整 prompt 模板。
8. **Agent loop**：每轮先读当前项目上下文；判断用户是在更新创作事实、要求生成参考、修改分镜、确认结果，还是提出歧义；选择最少工具完成可审计状态变更；工具失败时修正参数重试。
9. **工具使用规则**：写工具前要有明确 `brief`；写 memory 前判断是否需要 HITL；修改全局约束后再派后续任务；chain group 内串行，跨组可并行；M1 不派 Craftsman。
10. **关键禁令**：不要把 `[asset-xxx]` 裸写进创意描述；不要在 `Shot.action_text` 里写 `<主体N>@图片N`、约束包或 provider prompt 语法；不要让多个 shot 各自发明同一个全局场景参考。

Producer prompt 中可以包含一个简短决策表：

| 用户意图 | Producer 行为 |
|---|---|
| “做一个行李箱机场广告” | 建 brief、memory、商品元素、机场派生场景元素，必要时建轻量 storyboard。 |
| “先生成机场场景图看看” | 确认或创建机场 `KeyElementState(needs_reference)`；M1 只标记需求，M2 再派 Craftsman 生成 reference image。 |
| “改第二个分镜脚本” | 读取当前 storyboard，patch 对应 shot，保留关联元素和连续性依赖。 |
| “这个品牌色必须是蓝色” | 判断是否属于核心约束；必要时 HITL；写入新 `ProjectMemory` 版本。 |
| “让分镜 2 接分镜 1 的尾帧” | 写 `shot_dependency(last_frame_chain)`，后续 M2/M3 由 scheduler 和 RenderPlan 使用。 |

### 工具描述写法

每个工具描述应采用固定结构，方便模型稳定选择：

```text
一句话说明这个工具改变或读取哪类项目事实。

<supported_actions>
- `create`: 什么时候创建新对象。
- `patch`: 什么时候局部更新已有对象。
- `replace`: 什么时候整体替换同一 scope 下的草稿对象。
</supported_actions>

<instructions>
- 调用前必须满足什么条件。
- 哪些字段必须引用已有对象。
- 哪些行为不要在这个工具里做。
- 工具失败时如何修正参数重试。
</instructions>

<recommended_usage>
- 用户说某类话时应该调用。
- 某类状态变化后应该调用。
</recommended_usage>
```

工具参数统一包含：

- `brief`：必填。说明本次工具调用的业务目的，不超过 160 字。
- `mode` 或 `action`：必填。用于表达 create / patch / replace / archive 等变更语义。
- `scope`：涉及局部对象时必填，显式标明 `workspace`、`scene`、`shot` 或 `key_element`。
- 稳定 `client_key`：批量 upsert 时必填，方便模型在同一轮或重试时引用新对象，不依赖尚未知道的 UUID。
- `reason`：可选。用于记录为什么这样改，适合 memory、storyboard 和连续性变更。

工具字段描述要避免含糊词。比如 `description` 可以存在，但核心字段应更具体：

- `creative_text`：给用户和 Producer 看的创意级画面描述。
- `visual_intent`：这一镜的视觉目标，例如“突出银灰色箱体质感”。
- `action_text`：主体动作和事件，不写模型语法。
- `camera_intent`：高层镜头意图，可以写“中景跟拍”，但不写 Seedance 三段论。
- `reference_status`：参考资源状态，只能是 `none`、`needs_reference`、`ready`、`approved`、`rejected`。

## 数据模型

使用当前 worktree 迁移后的下一个迁移号，目前是：

```text
apps/server/migrations/024_m1_agent_creative_state.sql
```

### `creative_brief`

用途：记录 workspace 级当前创意方向。一个 workspace 同一时间应只有一个 active brief。

```sql
CREATE TABLE creative_brief (
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
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT creative_brief_duration_positive CHECK (duration_sec IS NULL OR duration_sec > 0),
    CONSTRAINT creative_brief_status_check CHECK (status IN ('draft', 'active', 'approved', 'archived'))
);
```

索引：

- `(workspace_id, archived_at, updated_at DESC)`。
- 对每个 workspace 建立部分唯一 active brief 索引，条件是 `archived_at IS NULL AND status <> 'archived'`。

字段说明：

- `concept` 是自然语言创意概念。
- `constraints` 存储用户明确提出、但尚未提升进 `ProjectMemory` 的约束。
- `metadata` 存储低频扩展数据，不能成为主要 brief 载体。

### `project_memory`

用途：版本化项目级创作宪法。M1 中只有 Producer 可以写。

```sql
CREATE TABLE project_memory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    version INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    core_intent TEXT NOT NULL DEFAULT '',
    soul TEXT NOT NULL DEFAULT '',
    brand_facts JSONB NOT NULL DEFAULT '{}',
    non_negotiables JSONB NOT NULL DEFAULT '[]',
    visual_anchors JSONB NOT NULL DEFAULT '{}',
    allowed JSONB NOT NULL DEFAULT '[]',
    forbidden JSONB NOT NULL DEFAULT '[]',
    prompt_injection_hints JSONB NOT NULL DEFAULT '[]',
    source_refs JSONB NOT NULL DEFAULT '[]',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT project_memory_status_check CHECK (status IN ('draft', 'active', 'archived'))
);
```

索引：

- unique `(workspace_id, version)`。
- 对每个 workspace 建立部分唯一 active memory 索引，条件是 `status='active'`。

M1 不创建 `project_memory_rule`。如果后续单条 memory rule 需要评论、确认或独立生命周期，再在后续里程碑拆表。

### `key_element`

用途：稳定身份锚点，例如商品、角色、场景、道具或风格。

```sql
CREATE TABLE key_element (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    client_key TEXT NOT NULL DEFAULT '',
    element_type TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT '',
    source_refs JSONB NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'active',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT key_element_type_check CHECK (element_type IN ('product', 'character', 'scene', 'prop', 'style')),
    CONSTRAINT key_element_source_type_check CHECK (source_type IN ('', 'user_asset', 'material_analysis', 'prompt_derived', 'agent_created')),
    CONSTRAINT key_element_status_check CHECK (status IN ('active', 'archived'))
);
```

索引：

- `(workspace_id, element_type, archived_at)`。
- partial unique `(workspace_id, client_key)` where `archived_at IS NULL AND client_key <> ''`。

### `key_element_state`

用途：关键元素的具体视觉状态。未来 RenderPlan 应引用 state，而不是只引用抽象 element。

```sql
CREATE TABLE key_element_state (
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
    state_facts JSONB NOT NULL DEFAULT '{}',
    source_refs JSONB NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'active',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT key_element_state_reference_status_check CHECK (reference_status IN ('none', 'needs_reference', 'ready', 'approved', 'rejected')),
    CONSTRAINT key_element_state_status_check CHECK (status IN ('active', 'archived'))
);
```

索引：

- `(workspace_id, key_element_id, archived_at)`。
- partial unique `(key_element_id, client_key)` where `archived_at IS NULL AND client_key <> ''`。
- partial unique default state per key element where `archived_at IS NULL AND is_default`。

M1 可以创建 `reference_status='needs_reference'` 的 state。M2 负责生成并绑定参考图。

### `scene`

用途：分镜的逻辑分组。

```sql
CREATE TABLE scene (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    client_key TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    mood TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'planned',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT scene_status_check CHECK (status IN ('planned', 'draft', 'approved', 'archived'))
);
```

索引：

- `(workspace_id, archived_at, sort_order)`。
- partial unique `(workspace_id, client_key)` where `archived_at IS NULL AND client_key <> ''`。

### `shot` 扩展

现有 `shot` 表继续作为主要 ShotPlan 表。M1 扩展 `shot`，不新建 `shot_plan`。

```sql
ALTER TABLE shot
    ADD COLUMN scene_id UUID REFERENCES scene(id) ON DELETE SET NULL,
    ADD COLUMN shot_kind TEXT NOT NULL DEFAULT '',
    ADD COLUMN creative_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN visual_intent TEXT NOT NULL DEFAULT '',
    ADD COLUMN action_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN camera_intent TEXT NOT NULL DEFAULT '',
    ADD COLUMN dialogue TEXT NOT NULL DEFAULT '',
    ADD COLUMN narration TEXT NOT NULL DEFAULT '',
    ADD COLUMN audio_plan JSONB NOT NULL DEFAULT '{}';
```

字段说明：

- 保留现有 `brief JSONB` 以兼容旧实现。
- 新代码应优先使用显式字段。
- `camera_intent` 是高层视觉意图，不是 provider prompt 语法。
- `audio_plan` 是可选字段，M1 可以为空。

索引：

- `(workspace_id, scene_id, archived_at, sort_order)`。

### `shot_key_element`

用途：结构化表达分镜和关键元素状态之间的关系。

```sql
CREATE TABLE shot_key_element (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    shot_id UUID NOT NULL REFERENCES shot(id) ON DELETE CASCADE,
    key_element_id UUID NOT NULL REFERENCES key_element(id) ON DELETE CASCADE,
    key_element_state_id UUID REFERENCES key_element_state(id) ON DELETE SET NULL,
    role TEXT NOT NULL DEFAULT '',
    required BOOLEAN NOT NULL DEFAULT true,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

索引：

- `(workspace_id, shot_id, sort_order)`。
- unique `(shot_id, key_element_id, key_element_state_id, role)`。

### `shot_dependency` 用法

M1 复用现有 `shot_dependency` 表表达连续性关系。

推荐的 `dependency_type`：

- `story_order`
- `last_frame_chain`
- `same_subject_consistency`
- `same_product_consistency`
- `same_scene_consistency`
- `visual_reference`
- `asset_reuse`

推荐的 `blocking_phase`：

- `planning`
- `reference_generation`
- `preview_generation`
- `video_generation`
- `review`
- `composition`

M1 不需要单独的 `continuity_link` 表。

## Producer 工具

所有写工具都必须校验 workspace mode 和对象归属。工具通过服务写领域对象；Producer 绝不直接写 SQL。

M1 Producer 工具的 `ToolInfo.Desc` 必须使用中文，且描述工具的事实边界、何时使用、何时不要使用。工具返回给模型的字符串应该包含：执行是否成功、实际写入或读取了什么、生成了哪些 `client_key` 或对象 ID、是否有可重试错误、下一步建议。

### ToolInfo 文案设计

#### `read_project_context`

描述：

```text
读取当前 ClipAnvil Agent workspace 的创作事实源，用于 Producer 在行动前理解 brief、memory、关键元素、场景、分镜、连续性依赖和只读画布投影。这个工具只读，不会修改任何对象，也不会读取完整聊天历史。

<instructions>
- 每轮开始或关键决策前优先调用，避免基于过期上下文写入。
- `detail_level=summary` 适合普通规划，`detail_level=full` 适合写 storyboard、修复冲突或解释当前状态。
- 如果 scope 指向 scene、shot 或 key_element，工具只返回该 scope 相关上下文和必要的全局约束。
- 不要用这个工具读取模型生成日志或 artifact 原始内容；那是后续生产/评审工具的职责。
</instructions>
```

#### `upsert_project_brief`

描述：

```text
创建、局部更新或归档当前 workspace 的创意简报 CreativeBrief。CreativeBrief 描述这条视频要做什么、给谁看、整体调性、风格、比例、语言、时长和创意概念。

<instructions>
- 当用户给出新视频目标、营销诉求、目标受众或整体风格时使用。
- `concept` 写自然语言创意概念，不写分镜细节，不写 Seedream 或 Seedance prompt。
- `constraints` 只放用户在 brief 层提出但尚未提升为 ProjectMemory 的约束。
- 如果用户只是修改某个分镜，不要改 brief，改 storyboard。
</instructions>
```

#### `update_project_memory`

描述：

```text
写入新的项目创作宪法 ProjectMemory 版本，记录全片必须遵守的核心意图、创作灵魂、品牌事实、视觉锚点、允许项和禁止项。ProjectMemory 会影响后续分镜、RenderPlan、PromptCompiler 和 Reviewer。

<instructions>
- 只有 Producer 可以写。
- 修改 `core_intent`、`soul`、`brand_facts`、`non_negotiables` 或重要 `visual_anchors` 前，若会改变用户已确认方向，应先请求用户决策。
- 每次成功写入都会创建新版本，不要把多个互相冲突的规则写进同一版本。
- `prompt_injection_hints` 只能放短约束，用于后续注入每个 shot prompt；不要放长剧本或完整 prompt。
</instructions>
```

#### `upsert_key_elements`

描述：

```text
创建、局部更新或替换关键元素 KeyElement 及其视觉状态 KeyElementState。关键元素是视频一致性的锚点，包括商品、人物、场景、道具和风格参考。

<instructions>
- 用户上传素材后，必须把可复用主体沉淀为 key element，而不是只写在聊天回复里。
- 用户 prompt 中提到但没有上传素材的稳定元素，例如“机场出发大厅”或“柔光房间”，也要创建 key element，并把 state 标记为 `reference_status=needs_reference`。
- 同一元素的不同视觉状态要拆成多个 state，例如白天机场和夜晚机场、行李箱开合状态。
- 不要在这里创建分镜；分镜使用 `upsert_storyboard`。
</instructions>
```

#### `upsert_storyboard`

描述：

```text
创建、局部更新、替换或归档 storyboard 事实，包括 Scene、Shot、分镜引用的关键元素状态，以及分镜之间的连续性依赖。这个工具写的是创意级 storyboard，不写模型原生 prompt。

<instructions>
- `creative_text`、`visual_intent`、`action_text` 和 `camera_intent` 必须保持创意级表达。
- 每个 shot 应通过 `shot_key_elements` 引用已有 key element / state；不要把商品、人物或场景一致性只写进自然语言。
- 需要尾帧接续、同商品一致、同场景一致时，写 `dependencies`，不要等待 Craftsman 自己猜。
- 如果用户只是要求先生成一个场景参考图，可以不创建完整 storyboard，只创建 key element state。
</instructions>
```

### `read_project_context`

用途：返回 M1 Full Context Packet。

入参：

```json
{
  "brief": "读取当前 workspace 的完整 M1 创作状态，判断是否需要创建 brief、memory、关键元素或 storyboard。",
  "scope": {"type": "workspace|scene|shot|key_element", "id": "uuid"},
  "include": ["brief", "memory", "elements", "scenes", "shots", "dependencies", "canvas_projection"],
  "detail_level": "summary|full"
}
```

M1 默认行为：

- Producer 可以请求完整 workspace context。
- Craftsman / Reviewer 不是 M1 必需调用方，但后续调用时应获得 scope-limited context。

### `upsert_project_brief`

用途：创建或 patch `creative_brief`。

入参：

```json
{
  "brief": "为悦行行李箱机场广告创建当前 active 创意简报。",
  "mode": "create|patch|archive",
  "brief_id": "uuid",
  "title": "悦行行李箱机场广告",
  "video_type": "marketing_ad",
  "target_audience": "短途商务出行用户",
  "tone": "轻快、可靠、高级",
  "visual_style": "现代机场、清晨自然光、商业质感",
  "duration_sec": 18,
  "aspect_ratio": "9:16",
  "language": "zh-CN",
  "objective": "突出悦行行李箱适合短途商务出行",
  "concept": "在机场拉箱的轻松出行广告",
  "constraints": [],
  "metadata": {}
}
```

### `update_project_memory`

用途：写入新的 `project_memory` 版本，或创建第一个版本。

入参：

```json
{
  "brief": "记录悦行行李箱广告的项目创作宪法，保证后续所有分镜保持商品外观和机场商务氛围一致。",
  "mode": "create|patch|replace",
  "core_intent": "展示悦行行李箱适合机场商务出行",
  "soul": "轻松出门，行程有掌控感",
  "brand_facts": {},
  "non_negotiables": [],
  "visual_anchors": {},
  "allowed": [],
  "forbidden": [],
  "prompt_injection_hints": [],
  "source_refs": [],
  "reason": "用户上传行李箱素材并要求机场广告"
}
```

规则：

- 仅 Producer 可调用。
- 每次成功写入都会创建新版本。
- 每个 workspace 只能有一个 active 版本。

### `upsert_key_elements`

用途：创建或 patch 关键元素及其状态。

入参：

```json
{
  "brief": "把用户上传的行李箱和用户需求中的机场出发大厅沉淀为可复用关键元素。",
  "mode": "create|patch|replace",
  "elements": [
    {
      "client_key": "product_yuexing_luggage",
      "element_type": "product",
      "name": "悦行行李箱",
      "description": "用户上传的银灰色悦行行李箱",
      "source_type": "user_asset",
      "source_refs": [],
      "states": [
        {
          "client_key": "state_uploaded_front",
          "label": "用户上传素材状态",
          "visual_description": "银灰色登机箱，正面可见 Logo",
          "reference_node_id": "uuid",
          "reference_status": "ready",
          "is_default": true,
          "state_facts": {}
        }
      ]
    }
  ]
}
```

### `upsert_storyboard`

用途：写入 scenes、shots、shot-key-element links 和 shot dependencies。

入参：

```json
{
  "brief": "创建机场广告的第一版场景和分镜，并把分镜绑定到悦行行李箱关键元素。",
  "mode": "create|patch|replace|archive",
  "scope": {"type": "workspace|scene|shot", "id": "uuid"},
  "scenes": [
    {
      "client_key": "scene_airport",
      "sort_order": 1,
      "title": "机场出发大厅",
      "description": "现代机场出发大厅中的轻松拉箱场景",
      "location": "机场出发大厅",
      "mood": "明亮、商务、轻快"
    }
  ],
  "shots": [
    {
      "client_key": "shot_01",
      "scene_client_key": "scene_airport",
      "sort_order": 1,
      "title": "机场拉箱开场",
      "shot_kind": "lifestyle",
      "creative_text": "商务女性拉着悦行行李箱穿过机场出发大厅",
      "narrative_purpose": "建立机场出行场景和轻便印象",
      "duration_sec": 5,
      "visual_intent": "干净明亮，突出行李箱质感",
      "action_text": "人物单手拉箱，步伐轻快",
      "camera_intent": "中景跟拍",
      "dialogue": "",
      "narration": "短途出差，一个箱子就够了",
      "audio_plan": {}
    }
  ],
  "shot_key_elements": [
    {
      "shot_client_key": "shot_01",
      "element_client_key": "product_yuexing_luggage",
      "state_client_key": "state_uploaded_front",
      "role": "hero_product",
      "required": true
    }
  ],
  "dependencies": [
    {
      "from_shot_client_key": "shot_01",
      "to_shot_client_key": "shot_02",
      "dependency_type": "same_product_consistency",
      "required_artifact": "",
      "injection_role": "product_reference",
      "blocking_phase": "preview_generation",
      "reason": "保持悦行行李箱外观一致"
    }
  ]
}
```

## 示例数据：悦行行李箱机场广告

当用户说“做一个悦行行李箱机场广告，我只上传了行李箱图片”后，M1 应能创建：

```json
{
  "creative_brief": {
    "title": "悦行行李箱机场广告",
    "video_type": "marketing_ad",
    "target_audience": "短途商务出行用户",
    "tone": "轻快、可靠、高级",
    "visual_style": "现代机场、清晨自然光、商业质感",
    "aspect_ratio": "9:16",
    "language": "zh-CN",
    "objective": "突出悦行行李箱适合短途商务出行",
    "concept": "在机场拉箱的轻松出行广告"
  },
  "project_memory": {
    "core_intent": "展示悦行行李箱适合机场商务出行",
    "soul": "轻松出门，行程有掌控感",
    "non_negotiables": [
      {"rule": "行李箱外观必须和用户上传素材一致", "severity": "blocking"}
    ],
    "prompt_injection_hints": [
      "悦行行李箱始终保持同一外观",
      "现代机场商务氛围，干净高级"
    ]
  },
  "key_elements": [
    {
      "client_key": "product_yuexing_luggage",
      "element_type": "product",
      "name": "悦行行李箱",
      "states": [
        {
          "client_key": "state_uploaded_front",
          "reference_status": "ready"
        }
      ]
    },
    {
      "client_key": "scene_airport_departure_hall",
      "element_type": "scene",
      "name": "机场出发大厅",
      "states": [
        {
          "client_key": "state_modern_morning",
          "visual_description": "现代机场出发大厅，明亮自然光，干净商务感",
          "reference_status": "needs_reference"
        }
      ]
    }
  ]
}
```

## 画布投影

M1 的画布投影是只读的，可以保持简单：

- `CreativeBrief` -> domain node。
- `ProjectMemory` -> 靠近项目起点的 domain node。
- `KeyElement` -> domain node。
- `KeyElementState` -> 子节点、domain node 或展开行。
- `Scene` -> scene lane / group。
- `Shot` -> shot card。
- `shot_dependency` -> typed continuity edge。

M1 不应该把这些投影节点存成业务事实。如果需要持久化布局，可以按 domain object refs 存布局 metadata，但事实源仍然是业务表。

## Producer 行为

Producer 必须能够：

- 判断用户请求是创作状态请求、参考资源需求，还是后续生成请求。
- 当用户给出新的视频目标时，先写 brief 和 memory，再做更深层规划。
- 创建 prompt 派生关键元素，例如“airport departure hall”。
- 用 `reference_status='needs_reference'` 标记缺失参考。
- 当用户要求 storyboard 时，创建轻量 scene 和 shot。
- M1 中避免派发 Craftsman。
- 向用户解释缺失参考已经准备好，后续可以进入生成。

## 测试与验证

后端测试：

- M1 表 migration up/down。
- sqlc query 测试：create / list / update / archive，按实际对象覆盖。
- 每个 Producer tool 的 service 测试。
- 校验测试：非法 enum、缺失 workspace、跨 workspace ID、重复 active client key、非法 patch payload。
- PSS / Full Context Packet 测试，覆盖 brief、memory、key elements、scenes、shots 和 dependencies。

Agent 测试：

- Producer deterministic / model-responder 测试可以调用新工具。
- Eino tool loop 能持久化 tool call 和 tool result。
- 悦行行李箱 prompt 能创建预期领域对象。

前端 / 画布测试：

- API 返回投影后的 domain nodes 和 edges。
- 画布能只读渲染 brief、memory、key element、key element state、scene、shot 和 continuity edge。
- Agent-owned domain projections 不暴露 Studio 写控件。

建议验证命令：

```bash
make sqlc-generate
make server-test
pnpm --filter @clip-anvil/web... build
git diff --check
```

如果只是评审本 spec，`git diff --check` 即可。

## 验收标准

M1 完成条件：

- Producer 可以通过 typed tools 创建或更新 `CreativeBrief`、`ProjectMemory`、`KeyElement`、`KeyElementState`、`Scene`、`Shot`、`shot_key_element` 和 `shot_dependency`。
- 悦行行李箱机场广告示例可以创建：
  - 一个 active creative brief。
  - 一个 active project memory。
  - 一个已关联用户上传图片节点的 ready 行李箱商品状态。
  - 一个 prompt 派生的机场场景状态，`reference_status='needs_reference'`。
  - 用户要求时，可以创建 scene / shot facts。
- `read_project_context` 以确定性结构返回这些对象，适合注入 Producer context。
- Agent 画布能只读展示 M1 domain objects。
- 不要求图片/视频生成。
- 现有 Studio mode 继续保持分离，不能通过 Agent tools 修改 Agent-only domain facts。
- 当前改动范围内的自动化测试和 whitespace 检查通过。
