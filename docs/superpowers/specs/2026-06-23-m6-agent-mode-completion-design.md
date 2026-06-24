# M6 Agent Mode Completion Design

**Status**: Draft for review
**Date**: 2026-06-23
**Milestone**: M6 MultiAgent Agent Mode

## Goal

定义从当前 M6 部分实现走到 roadmap 终态还需要补齐的能力。M6 终态不是“Agent 能聊天”，而是 Agent 能通过对话完成完整自动生产：

```text
用户需求和素材
-> Producer 规划 storyboard
-> Craftsman/Worker 生成预览图
-> Reviewer 评审并自动重试
-> Craftsman/Worker 生成视频片段
-> Composer 合成成片
-> 用户通过 HITL 确认或要求修改
```

这份 spec 是桥接文档：它不替代已有 M6.1-M6.6 阶段 spec，而是把 `M3-M6 roadmap` 的终态要求、当前代码现状和后续缺口统一到一个可审阅设计里。

## Current Code State

当前代码库已经具备这些 M6 基础：

- Agent runtime 持久化：
  - `agent_thread`
  - `agent_message`
  - `agent_task`
  - `agent_event`
  - `eino_checkpoint`
- Agent 对话和前端：
  - `/api/agent/workspaces/:workspaceID/...`
  - `/ws/agent`
  - 右侧悬浮 Agent 对话面板
  - 消息持久化和 WebSocket 同步
  - Markdown、thinking、tool status、decision card、attachment blocks
  - 模型选择和 thinking effort 选择
  - 附件上传并创建 source material node
- ProducerGraph：
  - Eino model call
  - streaming response
  - thinking stream
  - native tool calling through `schema.Message.EdgeCalls` and `compose.EdgesNode`
  - provider-specific thinking policy
- HITL foundation：
  - `request_user_decision`
  - decision UI card
  - checkpoint persistence
  - decision response API
  - 用户可通过按钮或自然语言回复 pending decision
- Storyboard / PSS：
  - `shot`
  - `shot_dependency`
  - `media_node.shot_id`
  - `update_storyboard`
  - `get_production_state`
  - Producer PSS 从 DB facts 派生
- Craftsman / Worker preview foundation：
  - `dispatch_craftsman`
  - shot-scoped Craftsman thread
  - `CraftsmanGraph`
  - `worker_generation`
  - Worker 创建 Agent-owned image node
  - Worker 调用 `production.Service.SubmitGenerationIntent`
- Agent read-only canvas：
  - Agent 复用 Studio canvas 渲染
  - Agent 页面接入 canvas websocket
  - production job events 会更新 Agent canvas node status
  - node detail drawer 只读展示 prompt、model、params、job、version、shot

M4/M5 的共享生产底座已经提供 M6 必须复用的能力：

- `GenerationIntent`
- provider bridge
- model capability check
- `generation_job`
- `artifact_version`
- current winner
- stale propagation
- sandbox job service
- text / image / video provider runtimes
- source material node
- production read APIs

## Confirmed Direction

1. M6 继续采用 Eino Graph 作为顶层编排，不用高层 ReAct Agent 黑盒遮蔽关键状态转移。
2. Producer 仍是用户对话入口和总协调者。
3. Craftsman 是 shot-scoped persistent graph。
4. Worker 是确定性执行任务，不是 LLM agent。
5. Reviewer 和 Composer 需要成为 first-class Agent roles，拥有 task/event/message/checkpoint 能力。
6. PSS 是 DB facts 的自然语言/结构化投影，不把 LLM summary 当事实源。
7. Agent tools 暴露生产语义，不暴露低层 canvas 原子操作。
8. Studio / Agent import-export 不放入当前 M6 terminal delivery；后续单独做。

## Remaining Work Blocks

后续应拆成四个大块：

```text
M6.6 Closure
  -> 补齐预览图生成的正确性、可观察性和 E2E
M6.7 Review / Retry / Dependency Scheduler
  -> 让质量评审、自动重试、跨 shot 阻塞调度可追踪
M6.8 Video / Composer
  -> 生成 shot video，并通过 sandbox-backed Composer 合成成片
M6 UX Completion
  -> 前端把 storyboard/review/video/final/task timeline 渲染成完整 Agent 工作台
```

这些块不是完全并行关系。正确顺序应优先让生产事实和事件闭环稳定，再补 UI。

---

# M6.6 Closure: Craftsman / Worker Preview Generation

## Purpose

M6.6 的主体代码已存在，但还不是终态闭环。本阶段只补齐 preview image generation，不引入 review、video、Composer。

## Problems To Solve

### 1. Craftsman Context 仍然偏窄

当前 Craftsman context 主要读取目标 shot 和已经绑定该 shot 的 nodes。终态 scoped PSS 还需要包含：

- 与目标 shot 相关的 incoming / outgoing `shot_dependency`；
- 可能被该 shot 使用的 source material nodes；
- storyboard brief、prompt refs、用户附件中显式引用的 nodes/assets；
- 相关 nodes 的 latest generation jobs 和 artifact versions；
- 触发本次 dispatch 的 Producer instruction / reason。

Craftsman 默认不应该读取完整 Producer chat history。Producer 历史对话应先沉淀成 storyboard、memory、PSS facts。只有后续上下文压缩策略明确选择时，才把部分对话带入 Craftsman。

### 2. Worker 还没有解析 `input_node_refs`

当前 Worker 提交 `GenerationIntent` 时 `InputRefs` 仍为空。这会破坏 M6 复用 M4/M5 输入链路的目标。

Worker 必须把 Craftsman 输出的 `input_node_refs` 解析为真实 `GenerationIntent.InputRefs`：

- 支持 node UUID；
- 支持 source material title / stable label，但只在唯一匹配时接受；
- 对歧义引用返回结构化失败；
- 对 generated upstream node 使用 current winner version；
- 对 source material node 使用 direct asset input；
- 后续支持 role，例如 `product`、`logo`、`style_ref`、`continuity_frame`。

解析成功后，系统应确保 referenced nodes 到 target preview node 的 dependency edges 存在。这里的 edge 是生产输入事实，不是 Agent 随意创建画布线。

### 3. Shot Status 需要成为稳定生产摘要

`shot.status` 已有 `planned`、`draft`、`preview_running`、`preview_ready`、`failed` 等值，但还没有完整流转口径。

M6.6 Closure 定义：

- `dispatch_craftsman` 成功排队后，把 selected shots 标为 `preview_running`。
- Worker submission 成功后保持 `preview_running`。
- preview generation job 成功，并且 preview image node 有 current winner 后，shot 标为 `preview_ready`。
- Worker 同步失败或异步 generation job failed 后，如果没有其他 active preview job，shot 标为 `failed`。
- `force=true` 重新生成时，可从 `preview_ready` 回到 `preview_running`，但保留历史版本。

`shot.status` 是派生摘要，但需要持久化，便于 PSS、UI 和调度器稳定读取。

### 4. 异步生产完成需要回流 Agent 事件

Worker succeeded 只表示 job/version 已提交，不表示图片已经生成完成。Agent 编排不能依赖用户刷新，也不能依赖 Producer 盲目轮询。

当 Agent-created shot preview node 的 `generation_job` 进入 terminal state 时，production broadcaster 应创建 Agent-side event：

- `preview_generation_succeeded`
- `preview_generation_failed`

事件 payload 至少包含：

- `workspace_id`
- `shot_id`
- `node_id`
- `generation_job_id`
- `artifact_version_id`，如果有
- `status`
- `error_code` / `error_message`，失败时

这些事件要持久化，并通过 websocket 推送。后续 ReviewGraph 以这些事件作为触发源。

### 5. Canvas WebSocket Payload 需要收敛

Worker 创建 preview node 后不应广播 raw `db.MediaNode`。Canvas websocket 的 `NodeCreated` / `NodeUpdated` payload 应与 canvas read API 的 UI-ready node response 一致。

原因：

- 前端需要 `production_preview`；
- 前端需要 parsed `metadata`；
- 前端需要 signed asset preview URLs；
- Agent 和 Studio canvas consumer 应使用同一协议；
- 避免“刷新后才显示完整字段”的体验问题。

## Deliverables

- Expanded Craftsman scoped PSS。
- Worker input ref resolver。
- Resolved input refs -> dependency edge sync。
- Preview shot status reducer。
- Production terminal event -> Agent preview event bridge。
- Canvas websocket node payload convergence。
- Browser E2E：真实 Agent 对话触发 `dispatch_craftsman`，生成 preview node/job/version，canvas 无刷新更新。

## Acceptance

- 用户能创建 storyboard 并要求生成预览图。
- Producer 通过 native tool call 调用 `dispatch_craftsman`。
- Craftsman 能看到目标 shot、相关依赖、相关 source material。
- Worker 至少能把一个 source material ref 解析进 `GenerationIntent.InputRefs`。
- Preview image node 有：
  - `source='agent'`
  - `node_type='image'`
  - `operation_type='text_to_image'`
  - `shot_id`
  - Agent trace metadata
- `generation_job.requested_by_type='agent_worker'`。
- `shot.status` 能从 `preview_running` 进入 terminal preview state。
- Agent canvas 不刷新即可看到新节点和生成完成后的 preview。
- PSS 能展示 preview node、job、version 和 shot status。

---

# M6.7: Review / Retry / Dependency Scheduler

## Purpose

M6.7 让生成结果进入可审计的质量控制闭环，并让跨 shot 依赖真正参与调度。

## Review Data Model

不要把完整评审信息塞进 `artifact_version.review_score`。该字段只能作为兼容性摘要。需要新增 review record。

建议表：

```sql
CREATE TABLE review_record (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    shot_id UUID REFERENCES shot(id) ON DELETE SET NULL,
    node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    artifact_version_id UUID NOT NULL REFERENCES artifact_version(id) ON DELETE CASCADE,
    reviewer_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    reviewer_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    target_phase TEXT NOT NULL,
    status TEXT NOT NULL,
    overall_score REAL,
    rubric JSONB NOT NULL DEFAULT '{}',
    critique TEXT NOT NULL DEFAULT '',
    retry_recommendation JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);
```

`target_phase`：

- `preview_image`
- `shot_video`
- `final_video`

`status`：

- `running`
- `accepted`
- `rejected`
- `failed`

## Review Rubric

默认评审轴：

- `proportion`：主体比例、构图和画面关系是否合理。
- `physics`：光影、材质、动作、运动逻辑是否可信。
- `style`：是否匹配品牌和创意方向。
- `visual_quality`：清晰度、畸变、伪影、可用性。
- `product_visibility`：商品是否清楚可识别。
- `selling_power`：画面是否支持卖点表达。
- `platform_fit`：是否适合目标平台和短视频节奏。

每个轴结构：

```json
{
  "score": 0.0,
  "pass": false,
  "reason": "...",
  "fix_hint": "..."
}
```

默认 reject 规则：

- 任一 required axis 低于阈值；
- overall score 低于阈值；
- review model 多次输出 invalid rubric。

阈值后续可进入 workspace settings。M6.7 首期可使用服务端默认值。

## ReviewerGraph

ReviewerGraph 是 first-class Eino Graph：

```text
load_review_context
-> call_review_model
-> validate_rubric
-> persist_review_record
-> route_accept_or_retry
```

Review context 包含：

- target shot brief；
- target artifact preview URL / video metadata / text output；
- generation prompt 和 params；
- source inputs；
- prior review records；
- retry count 和 max retry count；
- memory 中与品牌、商品、平台相关的摘要。

Reviewer 不直接调用生成服务。Reviewer 只产生 review record 和 retry recommendation。

## Retry Flow

Reject 且 retry attempts 未耗尽：

```text
review rejected
-> create retry instruction from critique
-> dispatch Craftsman with critique context
-> Craftsman writes revised strategy/prompt
-> Worker submits new GenerationIntent
-> new artifact_version
-> Reviewer reviews again
```

规则：

- 默认每个 shot / phase 最多 3 次；
- 不删除历史版本；
- 重试链路必须能从 `review_record`、`agent_task`、`generation_job` 追溯；
- 同步失败和异步失败都要有错误码；
- retry exhausted 后写 `retry_exhausted` event，并让 Producer 能向用户解释。

## Version Selection Edge

新增 Agent production tool：

```text
select_version
```

职责：

- 将某个 artifact version 设为 current winner；
- 更新 `media_node.current_version_id`；
- 更新 shot phase state；
- 触发 downstream stale / final composition stale；
- 写 agent event，供 PSS 和 UI 展示。

`select_version` 可由 Producer 根据用户指令调用，也可由 Reviewer policy 在 auto-accept 允许时调用。

## Cross-Shot Dependency Scheduler

`shot_dependency` 必须影响调度，不只是 PSS 展示。

`blocking_phase` 语义：

- `preview`：下游 preview 等上游 preview winner。
- `video`：下游 video 等上游 video winner。
- `review`：下游 review 等上游 accepted review。
- `composer`：final composition 等相关 dependency chain ready。

调度器行为：

- ready shots 并行 dispatch；
- blocked shots 保持 waiting/queued，并写 blocked reason；
- 上游 terminal event 到达后，重新评估下游 readiness；
- 写事件：
  - `shot_blocked`
  - `shot_unblocked`
  - `dependency_ready`

连续性依赖可触发 derived input：

- 上游视频首帧/尾帧提取；
- 通过 M4 sandbox-backed internal media operation 执行；
- 产物作为下游 `continuity_frame` input ref。

## Deliverables

- `review_record` schema / sqlc / API projection。
- ReviewerGraph。
- review dispatcher。
- `review_shot` tool。
- `select_version` tool。
- `retry_generation` tool。
- retry orchestration back into Craftsman/Worker。
- dependency readiness scheduler。
- PSS 展示 review、retry、blocked shots、accepted winners。
- 前端 review cards 和 blocked shot indicators。

## Acceptance

- Preview generation success can trigger review.
- Review record stores rubric, critique, score, target version.
- Rejected output creates a traceable retry chain.
- Accepted version becomes current winner.
- Blocked shot does not dispatch before upstream dependency is ready.
- User says “第二个分镜重做”时，Producer resolves `shot-02` and starts a traceable retry.

---

# Workspace Memory

## Purpose

Workspace Memory 存长期创意认知，不存实时生产状态。

区别：

- PSS：当前生产事实，例如 shots、nodes、jobs、versions、review、blocked state。
- Memory：项目级认知，例如商品、品牌、受众、平台、风格、禁忌、用户偏好、脚本 notes。

## Data Model

```sql
CREATE TABLE memory_document (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    current_revision_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE memory_revision (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    document_id UUID NOT NULL REFERENCES memory_document(id) ON DELETE CASCADE,
    source_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    source_message_id UUID REFERENCES agent_message(id) ON DELETE SET NULL,
    content JSONB NOT NULL DEFAULT '{}',
    summary TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Memory kinds：

- `brand`
- `product`
- `audience`
- `platform`
- `creative_direction`
- `constraints`
- `script_notes`

## Edges

新增：

- `read_workspace_memory`
- `update_workspace_memory`

Memory 不应在每轮对话后静默更新。只有用户明确提供长期项目事实，或 Producer 明确调用工具时，才写 memory revision。

Producer context 使用 memory summaries。Craftsman、Reviewer、Composer 只接收与当前 phase 相关的 selected memory。

## Acceptance

- 用户只需说一次品牌/商品/平台事实，后续 turn 可以使用。
- Memory revision 可追溯到 source message 或 tool call。
- PSS 不存 memory；memory 不存实时 job/version 状态。

---

# M6.8: Video Generation / Composer / Final Video

## Purpose

完成从 accepted previews 到 shot videos，再到 final video 的生产路径。

## Shot Video Generation

新增 Agent production tool：

```text
generate_shot_video
```

Producer 不直接调用 provider。该工具应通过 Craftsman/Worker 调度新 Worker mode：

```text
mode = shot_video
```

Worker video behavior：

- 创建或复用 Agent-owned video node；
- 绑定 `shot_id`；
- 使用 accepted preview image winner 作为输入；
- 带入 source material refs 和 continuity refs；
- 使用 model capability 选择 `text_to_video` / `image_to_video` 等 operation；
- 通过 `GenerationIntent` 创建 `generation_job` / `artifact_version`；
- 写 `shot_video_submitted` / `shot_video_succeeded` / `shot_video_failed` events。

shot status：

- `video_running`
- `video_ready`
- `failed`

ReviewGraph 也要支持 `target_phase='shot_video'`。

## ComposerGraph

ComposerGraph 是 final-output scoped Eino Graph：

```text
load_composition_context
-> draft_composition_plan
-> validate_assets_ready
-> create_final_video_node
-> submit_composer_worker
-> request_final_review_or_hitl
```

Composer context：

- ordered shots；
- selected video winners；
- transitions；
- BGM / audio nodes；
- subtitle / voice-over metadata；
- platform constraints；
- review outcomes；
- stale status。

## Final Composition Worker

Final composition 必须通过 Sandbox Job Service：

- stage selected video/audio assets into sandbox；
- run FFmpeg composition command inside sandbox job；
- upload final output through storage service；
- create `media_asset`；
- create final video `generation_job` / `artifact_version`；
- persist `sandbox_job_id` in provider response or job metadata。

应用进程不得直接运行 FFmpeg。

## Final HITL

Final video ready 后：

- Producer 或 Composer 调用 `request_user_decision`；
- UI 展示 final video card；
- 用户可确认成片或要求修改；
- 修改请求进入新的 Producer turn，并带 final-output context。

## Deliverables

- `generate_shot_video` tool。
- Worker `shot_video` mode。
- video status propagation into shot/PSS/canvas。
- ComposerGraph。
- final composition Worker through Sandbox Job Service。
- final video node and version lifecycle。
- final video HITL card。
- PSS final output section。

## Acceptance

- Accepted preview can generate shot video.
- Shot video output can be reviewed and selected.
- Composer waits until required shot video winners are ready.
- Composer creates final video through sandbox-backed execution.
- Final video appears as Agent-created video node with job/version/sandbox trace.
- User can confirm final video through Agent UI.

---

# Frontend Completion

## Current Strengths

当前 Agent UI 已有：

- floating right panel；
- collapsible / draggable panel；
- typed message renderer；
- Markdown rendering；
- thinking blocks；
- tool status blocks；
- decision card blocks；
- attachment previews；
- model / reasoning controls；
- Agent read-only canvas；
- node detail drawer。

## Missing Surfaces

### 1. Agent Production Status Bar

Agent 面板需要一个总状态区：

- current phase；
- active task count；
- preview/video/review/final progress；
- waiting decision count；
- failed task count；
- websocket connection status。

数据来自 `agent_task`、`agent_event`、PSS 和 websocket events。

### 2. Storyboard View

用户需要结构化查看 storyboard，不应只能看画布节点。

展示：

- shot key / title / duration；
- phase status；
- preview/video/review winner status；
- blocked reason；
- quick action：要求修改该 shot。

可以先作为 message block，后续再做 panel 内 persistent section。

### 3. Review Cards

Review result block 展示：

- accepted / rejected；
- overall score；
- rubric axis scores；
- critique；
- retry count；
- linked node/version；
- actions：view node、retry、accept anyway，具体是否允许由后端 policy 控制。

### 4. Final Video Card

Final output block 展示：

- playable final video；
- duration / format / size；
- source shots；
- version number；
- confirmation actions。

### 5. Task Timeline

需要可读的后台任务轨迹：

- Producer action；
- Craftsman strategy；
- Worker submission；
- generation job progress；
- review result；
- retry；
- Composer run。

默认用户文案不暴露 Producer/Craftsman/Worker 内部名。展开诊断信息时可以显示 role、task id、job id。

### 6. Canvas Detail Enhancements

Node detail drawer 增加只读区：

- review records；
- retry chain；
- shot dependency context；
- final composition trace；
- sandbox job trace。

## Acceptance

- 用户不看日志也能理解 Agent 正在做什么。
- 长任务通过 websocket 更新，不依赖刷新。
- Review/retry/final output 是结构化卡片，不是 raw JSON。
- 内部角色名不作为主要用户-facing 文案。
- 诊断展开区仍能看到 task/job/event IDs。

---

# Event And Resume Model

## Principle

Agent 编排不能依赖用户刷新、模型记忆或 Producer 盲目轮询。生产终态必须写 durable event，再由对应 dispatcher 或 graph 消费。

## Required Event Families

Preview：

- `preview_generation_submitted`
- `preview_generation_succeeded`
- `preview_generation_failed`

Review：

- `review_started`
- `review_accepted`
- `review_rejected`
- `review_failed`

Retry：

- `retry_requested`
- `retry_submitted`
- `retry_exhausted`

Dependency：

- `shot_blocked`
- `shot_unblocked`
- `dependency_ready`

Video：

- `shot_video_submitted`
- `shot_video_succeeded`
- `shot_video_failed`

Composer：

- `composition_started`
- `composition_succeeded`
- `composition_failed`
- `final_review_requested`
- `final_accepted`

## Consumers

Events are consumed by:

- PSS builder；
- Agent websocket；
- dependency scheduler；
- Reviewer dispatcher；
- Composer dispatcher；
- Producer system-reminder / resume flow when user-facing summary is needed。

WebSocket 只是 delivery，不是 storage。所有关键事件必须可恢复。

---

# Edge Registry Target State

当前已有或部分已有：

- `read_workspace_context`
- `get_production_state`
- `update_storyboard`
- `request_user_decision`
- `dispatch_craftsman`

需要新增或补齐：

- `read_workspace_memory`
- `update_workspace_memory`
- `generate_shot_preview`，或保留 `dispatch_craftsman(mode=preview_image)` 作为内部工具，并提供用户友好的 label；
- `generate_shot_video`
- `review_shot`
- `select_version`
- `retry_generation`
- `compose_final`
- `get_shot_detail`
- `get_asset_detail`

工具 description 必须 model-facing 精确；用户-facing label 默认隐藏内部角色名。

---

# Testing Strategy

## Unit Tests

必须覆盖：

- memory document revision creation；
- PSS memory selection；
- Worker input ref resolution；
- shot status reducer；
- review rubric validation；
- retry cap and retry chain；
- dependency readiness calculation；
- video Worker intent construction；
- Composer sandbox command planning；
- frontend block parsers/renderers for review/final video/task timeline。

## Integration Tests

必须覆盖：

- dispatch -> Craftsman -> Worker -> generation job accepted；
- production terminal event -> preview event -> shot status update；
- rejected review -> retry generation；
- dependency blocks downstream dispatch；
- accepted video winners -> Composer task creation；
- Composer Worker creates final artifact version。

## Browser E2E

Terminal M6 E2E：

1. Create Agent workspace.
2. Upload product image.
3. Ask for a 3-shot short video storyboard.
4. Confirm storyboard through HITL card.
5. Generate preview images.
6. Review and retry at least one rejected preview using deterministic/mock review mode.
7. Generate shot videos.
8. Compose final video.
9. Confirm final video through HITL.
10. Verify canvas contains source material, preview nodes, video nodes, and final video node.
11. Verify node detail exposes prompt/model/job/version/review/sandbox trace.
12. Verify database rows for threads, tasks, events, shots, reviews, jobs, versions, and sandbox jobs.

## Verification Commands

```bash
make migrate-up
make sqlc-generate
make server-build
make server-test
make server-lint
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Runtime E2E must use:

```bash
./scripts/dev-start.sh
```

Use the Vite URL printed by the script. Stop with:

```bash
./scripts/dev-stop.sh
```

---

# Out Of Scope

- Studio / Agent import-export。
- Multi-user collaborative editing inside one Agent workspace。
- User direct editing of Agent canvas。
- A separate non-ClipAnvil workflow engine。
- Running FFmpeg or arbitrary composition commands in the app process。
- Treating LLM summaries as production truth。

---

# Recommended Execution Order

1. M6.6 Closure：preview generation correctness、input refs、shot status、async completion events、E2E。
2. Workspace Memory：schema 和 tools，因为 review/video prompt 需要稳定品牌/商品/平台事实。
3. M6.7 Review：review records、ReviewerGraph、review cards。
4. M6.7 Retry：retry tools 和 critique-driven Craftsman regeneration。
5. M6.7 Dependency Scheduler：readiness calculation 和 event-driven unblocking。
6. M6.8 Shot Video：video Worker mode 和 video review。
7. M6.8 Composer：通过 Sandbox Job Service 合成成片。
8. M6 UX Completion：status bar、storyboard card、review cards、final video card、task timeline、detail enhancements。

这个顺序让每一阶段都能独立验收，同时保持 M6 终态架构一致。
