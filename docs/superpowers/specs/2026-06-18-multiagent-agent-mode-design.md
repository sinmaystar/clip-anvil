# MultiAgent Agent Mode 设计方案

**状态**：待评审
**日期**：2026-06-18
**阶段目标**：定义影砧 Agent 模式的完整 MultiAgent 架构，让 Producer、Craftsman、Worker、Composer 以事件驱动方式协作，支撑分镜规划、预览图、视频生成、评审重试、成片合成和对话式干预。

## 1. 背景

当前 ClipAnvil 已具备 Studio M1.x 的核心画布能力：Workspace、媒体节点、dependency 连线、分组、资源树、属性面板、上传、MinIO、`/ws/canvas` 和 OpenSandbox 基础能力。Agent 模式尚未落地，现有设计文档中已有 Producer、Sub-Agent、PSS、Gate、Skill、生成任务和版本管理的目标态描述，但还缺少足够明确的运行时边界。

本方案收敛本阶段 Agent 模式的关键设计：

- 分镜必须成为可引用、可持久化、可审计的业务实体。
- Producer 是唯一面向用户的 Orchestrator，不再额外引入独立 Orchestrator Agent 或 Orchestrator Run 概念。
- Craftsman 是 shot 级有状态 Agent，负责单个分镜的创意策略、生成计划、评审反馈吸收和改写重试。
- Worker 是单次无状态执行器，负责一次图片、视频或音频生成调用。
- Composer 负责成片合成。
- PSS 从 DB 事实源构建，并以自然语言方式注入 Agent 上下文。
- Gate 不做固定 workflow 卡点，而是 Producer 可调用的用户决策工具。
- 异步执行、任务状态、重试、消息队列和 Producer 唤醒由工程层承担。

## 2. 产品定位

Agent 模式是一个懂电商营销的视频顾问。用户是甲方，提供素材、确认方向、审核成果；Agent 主动提问、建议创意、规划分镜、调度生产，并在用户干预时及时调整。

目标体验：

- 用户可以自然描述商品、目标平台、风格、素材和预算。
- Producer 能主动提问，也能在用户说“你来定”时继续推进。
- Agent 先规划故事版，再按分镜组织 Craftsman 和 Worker 生产。
- 用户可以说“第二个分镜重做”“第三个接着第二个动作”“预算内不用问我直接生成完”。
- 系统能恢复、审计和解释每个分镜为什么这样做、当前做到哪一步、哪个版本是 winner。

## 3. 范围

### 3.1 包含

- 单视频 Agent 模式，不包含 Campaign 批量策略层。
- Producer / Craftsman / Worker / Composer 的角色边界。
- 分镜实体、分镜依赖、Agent 会话、生产任务、HITL 决策事件、事件队列的概念模型。
- Producer 工具集合，包括统一的 `update_storyboard` 工具。
- Gate 的工具化设计：`request_user_decision`。
- PSS 与 Scoped PSS 的自然语言构建方式。
- Craftsman 的状态持久化、上下文范围和重试职责。
- 事件驱动执行：Producer 不同步等待长任务，由工程层事件唤醒。
- 跨分镜依赖的完整模型，不只支持一种依赖类型。

### 3.3 Eino 运行时口径

结合 Eino 官方文档，本方案采用以下边界：

- 对话消息由 ClipAnvil 自己存储。Eino 的 Memory / Session / Store 属于业务层概念，因此需要 `agent_thread` 和 `agent_message`。
- Interrupt / Resume 状态由 ClipAnvil 自己持久化。Eino 提供 `CheckPointStore` 接口，因此需要 `eino_checkpoint`。
- `request_user_decision` 使用 Eino Human-in-the-loop interrupt / stateful interrupt 语义实现，但 UI 卡片 schema 由 ClipAnvil 自己定义。
- A2UI 可以作为后续 UI streaming / card rendering 的参考，不作为首版强依赖。

### 3.2 不包含

- Campaign、Target Matrix、Strategist 和 Brief 批量生产层。
- Studio 手动编辑器增强。
- 具体模型供应商 API 参数细节。
- Skill 内容库的完整编写。
- 前端高保真 UI 设计。
- 多用户协作和权限系统。

## 4. 核心原则

### 4.1 Producer 是唯一 Orchestrator

不引入独立 Orchestrator Agent，也不引入 Orchestrator Run 作为新的智能角色。

Producer 负责：

- 面向用户对话。
- 理解需求和素材。
- 规划故事版。
- 调用工具持久化分镜。
- 决定是否请求用户决策。
- 调度 Craftsman、Worker 和 Composer。
- 响应用户中途干预。
- 处理工程层事件队列里的任务结果。

工程层负责：

- 任务队列。
- 任务状态。
- 运行中断、暂停、恢复。
- 确定性重试策略。
- 事件记录。
- Producer 唤醒。

### 4.2 分镜是稳定业务锚点

分镜不能只存在 Producer 的对话上下文中。分镜必须持久化，原因是：

- 用户会引用“第二个分镜”“刚才那个开场镜头”。
- PSS 需要从 DB 重新构建，不依赖长对话记忆。
- Craftsman 需要绑定到某个稳定分镜。
- Gate 1 需要展示结构化分镜列表。
- 分镜可以删除、归档、恢复、重排和重做。
- 跨分镜依赖和执行顺序需要明确事实源。

分镜实体是生产语义，不是画布节点。画布节点只是分镜相关媒体产物的可视化投影。

### 4.3 Craftsman 有状态，但不拥有全局编排权

Craftsman 是 shot 级持久 Agent。它需要知道自己负责哪个分镜、这个分镜的 brief、叙事目的、时长、品牌/风格约束、输入素材、相邻上下文和依赖约束。

Craftsman 不需要完整全局 PSS，不负责重排分镜，不负责全局依赖调度，也不直接面向用户。

### 4.4 PSS 是自然语言状态投影

PSS 不是 JSON dump，也不是 LLM 自己总结。它是后端从 DB 查询事实后，用确定性模板拼接出的自然语言状态描述。

PSS 目标是让 Agent 读起来像项目简报，同时保持事实来自 DB。

### 4.5 Gate 是工具，不是固定流程卡点

Gate 不应该做成 workflow 里的硬编码步骤。Producer 应通过 `request_user_decision` 工具在需要时发起用户决策。

用户可以设置自动推进偏好，例如“预算内不用问我，直接生成完”。Producer 和工程层的 decision policy 根据用户偏好、风险、成本、合规和不可逆动作决定是否需要确认。

### 4.6 长任务异步化，Producer 由事件唤醒

Producer 不应同步等待所有生成任务结束。它调度任务后可以回到空闲。工程层在任务完成、失败、重试耗尽、需要用户决策或成片完成时写入消息队列，并唤醒 Producer。

## 5. 角色模型

### 5.1 Producer

Producer 是面向用户的主 Agent，也是唯一编排者。

职责：

- 需求理解：主动提问、提取商品、平台、目标受众、预算和风格。
- Memory 写入：维护 workspace memory 中的产品、品牌、受众、创意方向、脚本策略和笔记。
- Storyboard 规划：通过 `update_storyboard` 创建和修改分镜。
- 决策请求：通过 `request_user_decision` 发起卡片式用户确认。
- 调度：调用 `dispatch_craftsman`、`dispatch_worker`、`dispatch_composer`。
- 事件处理：被任务事件唤醒后，读取 Producer PSS 和事件上下文，决定下一步。
- 用户干预：处理暂停、继续、替换素材、重做分镜、改变风格、切换模型、调整预算。

Producer 不直接调用底层媒体生成工具。它可以发起任务，但不自己执行 `generate_image`、`generate_video`、`generate_audio`。

### 5.2 Craftsman

Craftsman 是分镜级有状态 Agent。一个分镜可以绑定一个持久 Craftsman conversation。

职责：

- 读取当前 shot 的 Scoped PSS。
- 结合 Story Context Pack 理解该分镜在全片中的位置。
- 设计预览图生成策略。
- 设计视频生成策略。
- 调用 Worker 执行具体生成。
- 读取评审结果，判断改写方向。
- 在重试上限内自行完成 prompt 改写和再次生成。
- 记录决策历史和失败经验。

Craftsman 需要知道：

- 当前分镜 brief。
- 叙事目的。
- 时长。
- 输入素材。
- 当前任务阶段。
- 品牌和 mood_anchor。
- 相邻分镜摘要。
- 相关跨分镜依赖。

Craftsman 不需要知道：

- 所有分镜的完整版本历史。
- 画布坐标、布局和 UI 样式。
- 全局调度队列。
- 其他 Craftsman 的完整对话历史。

### 5.3 Worker

Worker 是一次性、无状态执行器。

职责：

- 执行一次 `generate_image`、`generate_video` 或 `generate_audio`。
- 调用模型供应商或内部工具。
- 写入 generation job、asset、artifact version。
- 返回结构化结果或失败原因。

Worker 完成后即归档，不保留长期对话上下文。

### 5.4 Composer

Composer 负责成片合成。

职责：

- 读取已确认的视频 winner。
- 读取 BGM、旁白、字幕和转场要求。
- 通过 Sandbox Job Service 在 sandbox 内调用 ffmpeg 合成成片。
- 生成最终 artifact version。
- 写入合成日志、耗时、成本和错误。

### 5.5 工程层

工程层不是 Agent，但承担异步运行能力。

职责：

- 任务队列。
- 全局消息队列。
- agent task 状态机。
- job 状态机。
- retry policy。
- 事件广播。
- WebSocket 推送。
- 唤醒 Producer。

## 6. 数据模型

以下为目标态概念模型，不要求一次迁移全部实现，但当前设计应按这些边界落位。

### 6.1 workspace.memory

Workspace Memory 存储 Agent 的长期战略认知，建议放在 `workspace.settings` 的 `memory` 字段，或后续显式增加 `workspace.memory JSONB` 字段。

结构：

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
  "audience": {
    "persona": "关注健康的年轻白领女性",
    "platform": "douyin"
  },
  "script": {
    "storyline": "从注意力钩子到低糖卖点，再到生活方式和 CTA"
  },
  "notes": []
}
```

Memory 提供“为什么这样做”。它不是实时状态，不记录当前生成进度。

### 6.2 storyboard_item / shot

建议命名为 `shot` 或 `storyboard_item`。本文使用 `shot`。

Shot 是单条视频里的稳定分镜槽位。

核心字段：

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
    craftsman_conversation_id UUID,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

状态：

| 状态 | 含义 |
|---|---|
| `planned` | Producer 已规划，等待用户确认或后续修改 |
| `approved` | 已确认，可进入生产 |
| `in_progress` | Craftsman/Worker 正在处理 |
| `preview_ready` | 预览图已有 winner |
| `video_ready` | 视频已有 winner |
| `stale` | 上游或 brief 变更导致结果过期 |
| `revising` | 正在按用户反馈修改 |
| `archived` | 已归档，不参与当前成片 |

Shot 不重复存储画布状态、版本列表、评审记录和任务状态。这些从 `media_node`、`artifact_version`、`generation_job`、`review_record` 和 agent task 派生。

### 6.3 shot_dependency

跨分镜依赖需要一次设计为通用模型，不只支持上一镜末帧。

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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

依赖类型：

| 类型 | 含义 | 是否阻塞 |
|---|---|---|
| `sequence_order` | 成片顺序，仅表达剪辑顺序 | 否 |
| `last_frame_continuity` | 上游视频末帧作为下游首帧 | 是 |
| `visual_reference` | 上游结果作为视觉参考 | 可选 |
| `same_subject_consistency` | 保持同一商品、人物或场景一致 | 可选 |
| `transition_match` | 为转场匹配构图、运动方向或节奏 | 可选 |
| `audio_timing` | 受旁白、音乐或节奏点约束 | 可选 |

示例：

```json
{
  "dependency_type": "last_frame_continuity",
  "required_artifact": "video_winner.last_frame_asset",
  "injection_role": "first_frame",
  "blocking_phase": "video_generation",
  "stale_policy": "mark_downstream_video_stale",
  "reason": "shot-03 需要承接 shot-02 中杯子被拿起的动作"
}
```

Producer 负责规划这些依赖。工程层负责按依赖确定哪些任务可运行、哪些任务必须等待。

### 6.4 media_node 与 shot 的关系

`media_node` 是画布上的媒体投影，shot 是生产语义。建议给 `media_node` 增加可选 `shot_id`。

```sql
ALTER TABLE media_node ADD COLUMN shot_id UUID REFERENCES shot(id) ON DELETE SET NULL;
```

一个 shot 可以关联多个 media node：

- 预览图节点。
- 视频节点。
- 参考图节点。
- 分镜说明节点，如果未来需要。

一个 media node 可以为空 shot_id，例如用户上传的全局素材、Logo、BGM、最终成片。

### 6.5 artifact_version

版本记录仍挂在 media node 上。每次生成产生 artifact version。

关键字段：

- `node_id`
- `job_id`
- `asset_id`
- `version_no`
- `winner`
- `review_score`
- `input_hash`

Shot 当前使用哪个预览图或视频 winner，可从与该 shot 关联的 media node + artifact version 派生。若查询复杂，可后续加只读缓存字段，但不作为第一事实源。

### 6.6 agent_thread / agent_message / eino_checkpoint

Craftsman 的状态要持久化。Eino 官方把 Memory / Session / Store 视为业务层概念，因此 ClipAnvil 需要自行存储对话历史，并实现 Eino CheckPointStore。

概念字段：

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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

对于 Craftsman：

- `role = 'craftsman'`
- `scope_type = 'shot'`
- `scope_id = shot.id`

消息表：

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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Eino checkpoint 表：

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

Craftsman 会话需要保存：

- 生成策略。
- 评审反馈。
- 改写原因。
- 已尝试方向。
- 被用户否定的方向。
- 阶段摘要。

### 6.7 decision event

Gate 工具化后不单独建 `decision_request` 表。`request_user_decision` 应实现为 Eino Human-in-the-loop interrupt 工具：

- Eino `StatefulInterrupt` 保存中间执行状态。
- `eino_checkpoint` 持久化 checkpoint。
- `agent_event(event_type=decision_requested)` 承载前端待渲染卡片。
- `agent_message(message_type=ui_card)` 写入对话流。
- 用户选择后写 `decision_resolved` event，并调用 Eino resume。

UI 收到 decision event 后，在对话面板渲染卡片。用户可以点按钮，也可以自然语言回复。

### 6.8 agent_task

工程层异步任务，不是新的 Agent 角色。

```sql
CREATE TABLE agent_task (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
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

示例：

- `role = craftsman`, `task_type = create_preview`
- `role = worker`, `task_type = generate_image`
- `role = worker`, `task_type = generate_video`
- `role = composer`, `task_type = compose_final`

### 6.9 agent_event

全局消息队列的持久事件。

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

事件示例：

- `shot_preview_ready`
- `shot_video_ready`
- `task_failed`
- `retry_exhausted`
- `decision_requested`
- `decision_resolved`
- `user_interrupted`
- `asset_uploaded`
- `final_video_ready`

当事件需要 Agent 决策或用户解释时，工程层唤醒 Producer。

## 7. PSS 设计

### 7.1 PSS 构建原则

PSS 从 DB 事实构建，输出自然语言。

流程：

```text
DB 查询 workspace、memory、shots、dependencies、nodes、versions、jobs、tasks、decisions、events
  -> 后端确定性聚合
  -> 模板拼接成自然语言
  -> 注入 Agent 上下文
```

PSS 不长期保存为事实源。每轮 Agent 调用前重新构建或按需构建。

### 7.2 Producer PSS

Producer 需要全局状态。

示例：

```text
当前 Workspace 正在制作一条 30 秒抖音商品广告。
用户偏好：预算 50 元以内可自动继续；视频生成前需要确认预览图。

Workspace Memory：
商品是燕麦拿铁，卖点是低糖、健康、顺滑。
品牌调性是年轻、干净、轻盈。
创意方向使用 AIDA，mood_anchor 是 clean lifestyle, warm morning light, commercial quality。

分镜计划：
- [shot-01] 开场钩子，3 秒。状态：preview_ready。预览图 winner 是 image-v2。
- [shot-02] 倒入燕麦拿铁，5 秒。状态：video_ready。视频 winner 是 video-v1。
- [shot-03] 低糖卖点视觉化，6 秒。状态：approved。依赖 shot-02 的末帧作为首帧，等待上游末帧资产。
- [shot-04] 饮用场景，8 秒。状态：approved。可并行生成预览图。
- [shot-05] 品牌收尾 CTA，5 秒。状态：planned，等待用户确认。

待处理决策：
- 无。

正在运行：
- worker-task-17 正在为 shot-04 生成预览图，预计 1 分钟。

最近事件：
- shot-02 视频生成完成，评分 8.1，已设为 winner。
```

### 7.3 Craftsman Scoped PSS

Craftsman 只看当前 shot 的 scoped context。

示例：

```text
你是 shot-03 的 Craftsman。

当前分镜：
标题：低糖卖点视觉化
时长：6 秒
叙事目的：让用户理解燕麦拿铁的低糖健康卖点，同时保持画面轻盈。
画面 brief：用清爽的动态图形或真实饮品细节表达低糖，不要出现医疗化表达。

全片策略：
这是一条 30 秒抖音广告，采用 AIDA 结构。
shot-03 位于中段卖点解释位置，前面已经完成产品吸引，后面会进入生活方式场景。

相邻上下文：
上一镜 shot-02：倒入燕麦拿铁，结尾是杯子被拿起。
下一镜 shot-04：年轻女性在明亮办公室饮用，传递轻负担生活方式。

依赖约束：
本 shot 的视频生成需要使用 shot-02 的末帧作为首帧，以保持动作连续。

输入素材：
产品主图、品牌 Logo、shot-02 末帧资产。

当前任务：
生成预览图候选，目标是先确认构图和卖点表达，不生成视频。
```

### 7.4 Worker PSS

Worker 不需要 PSS，只需要结构化 task input：

```json
{
  "task_type": "generate_image",
  "prompt": "A clean commercial product shot of oat latte with warm morning light",
  "model": "dashscope-image-default",
  "reference_assets": [],
  "output_contract": {
    "type": "image",
    "workspace_id": "workspace-123",
    "shot_id": "shot-03"
  }
}
```

### 7.5 Composer PSS

Composer 需要 selected video winners 和合成规则。

```text
你是 Composer。
目标：合成 30 秒抖音成片。

已确认视频：
- shot-01: video-v2, 3 秒
- shot-02: video-v1, 5 秒
- shot-03: video-v3, 6 秒
- shot-04: video-v1, 8 秒
- shot-05: video-v2, 5 秒

转场：
- shot-01 -> shot-02: cut
- shot-02 -> shot-03: continuity match
- shot-03 -> shot-04: crossfade 0.5 秒
- shot-04 -> shot-05: cut

BGM：asset-bgm-01
字幕：无
输出：9:16, 1080x1920
```

## 8. Producer 工具

### 8.1 update_storyboard

统一分镜规划工具，支持多种动作。

```json
{
  "operations": [
    {
      "type": "create",
      "client_ref": "shot-01",
      "title": "开场钩子",
      "sort_order": 1,
      "duration_sec": 3,
      "narrative_purpose": "抓住注意力",
      "brief": {}
    },
    {
      "type": "update",
      "shot_id": "shot-03",
      "title": "低糖卖点视觉化",
      "brief": {}
    },
    {
      "type": "archive",
      "shot_id": "shot-04",
      "reason": "用户要求删掉重来"
    },
    {
      "type": "reorder",
      "shot_ids": ["shot-01", "shot-02", "shot-03"]
    },
    {
      "type": "approve",
      "shot_ids": ["shot-01", "shot-02", "shot-03"]
    },
    {
      "type": "restore",
      "shot_id": "shot-04"
    }
  ],
  "dependencies": [
    {
      "from_client_ref": "shot-02",
      "to_client_ref": "shot-03",
      "dependency_type": "last_frame_continuity",
      "required_artifact": "video_winner.last_frame_asset",
      "injection_role": "first_frame",
      "blocking_phase": "video_generation",
      "reason": "承接杯子被拿起的动作"
    }
  ]
}
```

工具行为：

- 写入 shot。
- 写入 shot_dependency。
- 更新 sort_order。
- 归档而不是硬删除。
- 标记受影响的下游 stale。
- 写 agent_event。
- 广播 `/ws/canvas` 和 `/ws/chat` 需要的事件。

### 8.2 request_user_decision

用户决策工具，不是固定 Gate。实现上它是 Eino Human-in-the-loop interrupt 工具。

```json
{
  "title": "确认分镜计划",
  "message": "我规划了 5 个分镜，总时长 30 秒，预计预览图成本 0.5 元，视频生成成本 12-20 元。是否继续？",
  "options": [
    {"id": "approve", "label": "确认并生成预览图"},
    {"id": "revise", "label": "修改分镜"},
    {"id": "auto", "label": "预算内自动继续"}
  ],
  "default_option": "approve",
  "risk_level": "cost_increasing",
  "cost_estimate": {
    "preview": "0.5",
    "video": "12-20"
  },
  "scope": {
    "shot_ids": ["shot-01", "shot-02", "shot-03", "shot-04", "shot-05"]
  }
}
```

执行行为：

- Producer 调用工具时触发 Eino `StatefulInterrupt`。
- interrupt info 使用下面的卡片 schema。
- Eino checkpoint 写入 `eino_checkpoint`。
- 后端写 `agent_event(event_type=decision_requested)`。
- 后端写 `agent_message(message_type=ui_card)`。
- 用户选择后，后端调用 Eino resume，把选择结果传回 interrupt ID。
- 后端写 `agent_event(event_type=decision_resolved)`，唤醒 Producer。

UI 行为：

- 在对话面板插入卡片。
- 按钮对应 options。
- 用户自然语言回复也可以 resolve decision。

卡片 schema 建议：

```json
{
  "type": "choice_card",
  "title": "确认分镜计划",
  "message": "我规划了 5 个分镜，总时长 30 秒。建议先生成预览图。",
  "options": [
    {"id": "approve", "label": "确认并生成预览图", "recommended": true},
    {"id": "revise", "label": "修改分镜"},
    {"id": "auto", "label": "预算内自动继续"}
  ],
  "allow_free_text": true
}
```

### 8.3 dispatch_craftsman

调度某个 shot 的 Craftsman。

```json
{
  "shot_id": "shot-03",
  "task_type": "create_preview",
  "instructions": "先生成 2 张预览图候选，重点验证构图和卖点表达。",
  "max_attempts": 3
}
```

后端路由：

- 如果 shot 已有 `craftsman_conversation_id`，复用。
- 如果没有，创建新的 Craftsman conversation 并绑定 shot。
- 创建 agent_task。
- 注入 Craftsman Scoped PSS。

### 8.4 dispatch_worker

Producer 通常不直接调用 Worker，Craftsman 可以调用 Worker。保留 Producer 调用能力用于简单任务。

```json
{
  "task_type": "generate_image",
  "shot_id": "shot-03",
  "prompt": "A clean visual metaphor for low sugar oat latte, commercial product lighting",
  "reference_assets": ["asset-product-main", "asset-brand-logo"],
  "model": "dashscope-image-default"
}
```

### 8.5 dispatch_composer

当所有需要的 shot 视频 winner 就绪后，Producer 调用 Composer。

```json
{
  "shot_ids": ["shot-01", "shot-02", "shot-03", "shot-04", "shot-05"],
  "bgm_asset_id": "asset-bgm-01",
  "output_format": "9:16"
}
```

### 8.6 cancel_task / pause_agent / resume_agent

流程控制工具：

- `cancel_task(task_id)`
- `pause_workspace_agent(workspace_id, reason)`
- `resume_workspace_agent(workspace_id)`

暂停不会取消已提交给模型供应商且无法撤销的生成任务，但会阻止新任务继续派发。

## 9. 工作流

### 9.1 需求理解

```text
用户：帮我做一个 30 秒燕麦拿铁广告，投放抖音。
Producer：
1. 读取 Producer PSS。
2. 检查素材是否足够。
3. 必要时提问；如果用户说“你来定”，继续。
4. 写入 workspace memory。
```

### 9.2 分镜规划

```text
Producer：
1. 基于 Skill、Memory、用户目标生成故事版。
2. 对话中展示分镜文案。
3. 调用 update_storyboard 创建 planned shots 和 shot_dependencies。
4. PSS 刷新后包含 shot 列表。
5. 调用 request_user_decision 请求确认，或根据用户偏好自动继续。
```

### 9.3 预览图生成

```text
Producer：
1. 查询 PSS 中 approved shots。
2. 根据 shot_dependency 判断哪些 shot 可先生成预览图。
3. 调用 dispatch_craftsman。

Craftsman：
1. 读取 Scoped PSS。
2. 设计预览图 prompt。
3. 调用 Worker generate_image。
4. 评审结果。
5. 不通过则改写重试。
6. 成功后设置 preview winner，并写 agent_event。
```

### 9.4 图片确认

图片确认不是强制固定 Gate。Producer 根据用户偏好和 decision policy 判断：

- 用户要求“每一步都给我看”：调用 `request_user_decision`。
- 用户要求“预算内自动继续”：自动进入视频生成。
- 评审低分或版本差异大：建议询问用户。
- 成本高或不可逆动作：强制询问。

### 9.5 视频生成

```text
Producer 被 preview_ready event 唤醒：
1. 读取 PSS。
2. 找到可进入视频生成的 shots。
3. 对有 blocking dependency 的 shot，等待上游 required artifact。
4. dispatch_craftsman(task_type=generate_video)。

Craftsman：
1. 使用 preview winner、shot brief、必要的上游产物。
2. 调 Worker generate_video。
3. 自动评审。
4. 失败时改写并重试。
5. 成功后设置 video winner，抽取 last_frame_asset，写 event。
```

### 9.6 成片合成

```text
Producer：
1. 看到所有 required shots 的 video winner ready。
2. 根据用户偏好决定是否请求确认。
3. dispatch_composer。

Composer：
1. 拉取 selected video winners。
2. 通过 Sandbox Job Service 执行 ffmpeg 合成。
3. 写 final asset 和 artifact version。
4. 写 final_video_ready event。
```

### 9.7 成片确认

Producer 被 `final_video_ready` 唤醒后：

- 在对话中总结成片。
- UI 展示视频播放器卡片。
- 用户可以确认、要求局部修改、重剪、替换 BGM 或导出。

## 10. 跨分镜依赖和调度

### 10.1 依赖判断

Producer 负责判断是否需要跨分镜依赖。判断来源：

- Skill：例如剧情连续、产品动作连续、转场匹配。
- 用户要求：例如“第三个接着第二个动作”。
- 脚本策略：例如 AIDA 中不同镜头承担不同叙事目的。
- Craftsman 建议：Craftsman 可以提出“本镜最好接上一镜末帧”，但最终由 Producer 更新 storyboard dependency。

### 10.2 调度执行

工程层根据 `shot_dependency` 和 task 状态确定哪些任务可运行。

规则：

- `sequence_order` 不阻塞预览图和视频生成，只影响 Composer 顺序。
- `last_frame_continuity` 阻塞下游视频生成，直到上游 video winner 和 last_frame_asset 就绪。
- `visual_reference` 如果 required artifact 已存在则注入；不存在时可由 Producer 决定等待或降级。
- `same_subject_consistency` 主要影响 prompt 和参考素材，不一定阻塞。
- `transition_match` 可能影响下游 prompt、首尾帧选择或 Composer 参数。
- `audio_timing` 在旁白或 BGM 节奏确定前可能阻塞 Composer。

Producer 不需要在上下文里手动记住所有等待关系。Producer 规划依赖，工程层执行等待和唤醒。

### 10.3 Stale 传播

当上游变更时：

- 上游 shot brief 变更：相关 preview/video 可能 stale。
- 上游 preview winner 变更：下游 visual_reference 可能 stale。
- 上游 video winner 或 last_frame_asset 变更：下游 last_frame_continuity 的 video stale。
- 转场依赖变更：final composition stale。

Stale 传播由工程层确定性执行，并写入 PSS。

## 11. 评审与重试

### 11.1 评审维度

评审包含通用轴和营销轴。

通用轴：

- proportion
- physics
- style
- visual_quality

营销轴：

- product_visibility
- selling_power
- platform_fit

规则：

- 任一轴小于等于 5：reject。
- 7 轴均分大于等于 7.0：accept。
- 其他情况：reject，并由 Craftsman 判断性改写。

### 11.2 重试职责

Craftsman 负责 shot 内重试。

流程：

```text
Worker 生成结果
  -> review
  -> accept: set winner
  -> reject: Craftsman 读取 critique
  -> 改写 prompt
  -> 再 dispatch Worker
  -> 最多 3 次
  -> 仍失败：写 retry_exhausted event，唤醒 Producer
```

Producer 只处理重试耗尽、方向性调整、用户介入和预算超限。

## 12. 用户干预

用户随时可以打断。

示例：

| 用户说 | Producer 行为 |
|---|---|
| “停一下” | 调 `pause_workspace_agent`，阻止新任务派发 |
| “继续” | 调 `resume_workspace_agent` |
| “第二个分镜重新生成” | 从 PSS 匹配 shot-02，决定重做 preview/video 或更新 brief |
| “第三个接着第二个动作” | 更新 shot_dependency，标记相关视频 stale |
| “产品图换成这张” | 导入 asset，更新引用，标记受影响 shot stale |
| “预算内不用问我” | 更新 decision policy 或 memory preference |
| “这个风格太冷了” | 更新 mood_anchor，询问是否重新生成受影响 shot |

## 13. 前端呈现

### 13.1 对话面板

对话面板承载：

- Producer 回复。
- 用户消息。
- 任务进度摘要。
- Decision 卡片。
- 预览图候选卡片。
- 成片播放器卡片。

### 13.2 画布

Agent 模式下画布只读，但可以：

- 平移、缩放。
- 点击节点查看详情。
- 引用节点到对话输入。
- 查看 shot 相关媒体节点。
- 查看 dependency 和 sequence/continuity 的可视化投影。

Studio 当前只暴露 dependency；Agent 模式可内部使用 shot_dependency。是否把 sequence/continuity 可视化给用户，需要另行设计 UI，不能混同 Studio 手动连线语义。

## 14. 与现有文档的调整关系

后续如果本方案通过评审，需要更新：

- `docs/design/agent-mode.md`
- `docs/design/overview.md`
- `docs/engineering/database.md`
- `docs/engineering/architecture.md`

重点同步：

- 将 Gate 改为 `request_user_decision` 工具。
- 将 `create_storyboard` / `modify_storyboard` 收敛为 `update_storyboard`。
- 明确 Producer 是唯一 Orchestrator。
- 删除独立 Orchestrator Run/Agent 概念。
- 增加 shot、shot_dependency、agent_thread、agent_message、eino_checkpoint、agent_task、agent_event。
- 明确 PSS 是 DB 到自然语言的确定性模板投影。

## 15. 首版落地切片

建议分阶段落地。

### Phase 1: 分镜实体和 PSS

交付：

- `shot` 表。
- `shot_dependency` 表。
- `update_storyboard` 后端工具/API。
- Producer PSS builder。
- docs 和基础测试。

验收：

- Producer 可以创建 5 个 planned shots。
- PSS 能自然语言列出分镜、状态和依赖。
- 用户说“第二个分镜”时能解析到稳定 shot id。

### Phase 2: HITL 工具和对话卡片

交付：

- `agent_thread` / `agent_message` 基础消息存储。
- `eino_checkpoint` CheckPointStore 实现。
- `request_user_decision` HITL 工具。
- `/ws/chat` 或现有事件通道的 `decision_requested` 推送。
- 前端对话卡片。

验收：

- Producer 能发起分镜确认卡片。
- 卡片消息能作为 `agent_message(message_type=ui_card)` 出现在对话流。
- 用户按钮或自然语言回复能写入 `decision_resolved` event。
- 后端能通过 Eino resume 让 Producer 从 interrupt 位置继续执行。

### Phase 3: Craftsman 持久会话和预览图链路

交付：

- Craftsman `agent_thread` 生命周期管理。
- `agent_task` 表。
- `dispatch_craftsman`。
- Craftsman Scoped PSS。
- Worker image generation 适配。

验收：

- 每个 shot 可复用 Craftsman conversation。
- Craftsman 能生成预览图并记录评审/改写摘要。
- 重试耗尽能唤醒 Producer。

### Phase 4: 视频生成和跨 shot 依赖

交付：

- video generation task。
- last_frame_asset 抽取。
- shot_dependency 调度。
- stale 传播。

验收：

- 无依赖 shots 可并行生成视频。
- `last_frame_continuity` 下游视频等待上游末帧。
- 上游 winner 变更能标记下游 stale。

### Phase 5: Composer 和成片确认

交付：

- Composer task。
- Sandbox Job Service / ffmpeg 合成。
- final video artifact。
- 成片确认卡片。

验收：

- 所有 video winners 就绪后可合成成片。
- 用户可确认或要求局部修改。

## 16. 开放问题

1. `shot` 命名是否使用 `storyboard_item`，避免和视频文件中的 shot 概念混淆？
2. `workspace.memory` 是放在 `workspace.settings.memory`，还是新增显式字段？
3. PSS 模板是否按角色版本化，便于后续 A/B 调整？
4. Decision policy 哪些情况必须确认，哪些可以按用户偏好自动继续？
5. Craftsman 是否允许主动建议更新 shot_dependency，还是只能通过 Producer 修改？
6. 自动评审模型如何选择，是否需要多模型投票？
7. 生成任务的并发配额按 workspace、account 还是 provider 维度控制？
8. Craftsman conversation 的压缩和归档策略是什么？
9. Agent 模式下的 sequence/continuity 可视化是否进入画布，还是只在对话和分镜面板展示？
