# M6.4 Agent Tool / HITL / Production Bridge 设计方案

**状态**：待评审
**日期**：2026-06-21
**所属里程碑**：M6 MultiAgent Agent Mode
**阶段目标**：在已落地的 Agent runtime、右侧对话框、真实模型流式对话和附件源素材能力之上，建立 Producer 可控工具调用、HITL interrupt/resume、工具审计和 M4/M5 生产底座复用接口。M6.4 不直接完成 Storyboard、Craftsman、Worker、Review retry 或 Composer，但必须为后续完整 MultiAgent 架构提供稳定执行协议。

## 1. 背景

M3-M5 已完成：

- Workspace mode 区分 Studio / Agent。
- Agent Workspace 中普通用户画布写接口返回 `403`，画布读取保留。
- 共享生产底座：`GenerationIntent`、provider bridge、`generation_job`、`artifact_version`、winner、stale、retry、sandbox job。
- Studio 手动生产闭环：文本、图像、视频、源素材节点、Reference Pack、Prompt `@`、版本和调用记录。

M6.1-M6.3 及之后的聊天壳改动已经完成了 Agent 基础：

- `agent_thread`、`agent_message`、`agent_task`、`agent_event`、`eino_checkpoint`。
- `/api/agent/...` message API、attachment API 和 `/ws/agent`。
- 右侧悬浮 `ClipAnvil` 对话框、流式 assistant delta、消息刷新恢复、附件上传为 Agent-owned source material node。
- Eino ProducerGraph 当前形态：

```text
load_context -> draft_response -> finalize_response
```

当前系统已经能真实调用低成本 Volcengine text model 做对话，但 Agent 还不能可靠行动。它缺少：

- 明确的工具注册表和工具 schema。
- 工具调用消息、工具结果消息和工具事件审计。
- `request_user_decision` 的中断、持久化卡片、用户响应和 resume 协议。
- Eino Graph 中的 tool planning / execute / interrupt 节点。
- Agent 工具身份对 M4/M5 生产能力的安全复用边界。
- Agent 模式下的模型选择能力；当前 Producer 对话模型由环境变量固定，不利于用户按成本、速度、质量和上下文长度切换。

M6.4 的核心不是再做一个 UI 阶段，而是把“ClipAnvil 能对话”推进到“ClipAnvil 能在可审计、可恢复、可控权限下调用第一批工具”。

## 2. 设计结论

M6.4 采用 **Eino Graph 主编排 + ClipAnvil Tool Registry + 工程化 HITL primitive**。

Graph 仍是最高层编排，不采用 Eino ADK Agent 作为顶层黑盒。原因是 M6 后续需要对 Producer、Craftsman、Worker、Reviewer、Composer 的任务状态、并发调度、重试上限、HITL、checkpoint 和版本链路做细粒度控制。

M6.4 后 ProducerGraph 目标形态：

```text
ProducerGraph
  load_context
  -> build_tool_context
  -> call_model
  -> route_model_output
      -> finalize_text_response
      -> persist_tool_call
      -> execute_tool
          -> persist_tool_result
          -> continue_or_finalize
      -> request_hitl_interrupt
          -> persist_checkpoint
          -> persist_decision_card
          -> mark_task_waiting_for_user
```

M6.4 先支持一轮内最多有限次数工具调用，避免模型陷入无界 ReAct loop，但默认值不能过小。长程 Agent 任务会越来越普遍，`producer_turn` 的工具调用上限必须可配置。

- 单个 `producer_turn` 默认最多 50 次 tool call。
- 单次工具执行默认超时 300 秒。
- 两个值都必须通过后端配置覆盖，建议配置名：
  - `CLIPANVIL_AGENT_PRODUCER_MAX_TOOL_CALLS`
  - `CLIPANVIL_AGENT_TOOL_TIMEOUT_SECONDS`
- `request_user_decision` 一旦触发，本轮进入 `waiting_for_user`，不再继续调用其他工具。

## 3. 范围

### 3.1 包含

- Tool registry 基础结构。
- 工具定义暴露协议：name、description、JSON schema、safety、timeout、visibility。
- 工具调用和工具结果持久化：
  - `agent_message(message_type='tool_call')`
  - `agent_message(message_type='tool_result')`
  - `agent_task(task_type='tool_call')`
  - `agent_event(type='tool_call_started'/'tool_call_completed'/'tool_call_failed')`
- 第一批 Producer 必做工具：
  - `read_workspace_context`
  - `create_agent_text_node`
  - `request_user_decision`
- 条件实现工具：
  - `list_canvas_nodes`：默认不作为 M6.4 必做项；如果实现时证明 `read_workspace_context` 的节点摘要不足以完成可靠工具链路或 HITL 验收，则在本阶段一起实现，并严格定位为画布节点明细钻取工具。
- HITL interrupt/resume：
  - `request_user_decision` tool。
  - `agent_message(message_type='ui_card')` 决策卡片。
  - `agent_event(type='decision_requested'/'decision_resolved')`。
  - `agent_task.status='waiting_for_user'`。
  - `eino_checkpoint` 写入和 `agent_thread.current_checkpoint_key` 更新。
  - 用户点击卡片后创建 `decision_resume` task 并 resume ProducerGraph。
- 前端右侧对话框渲染决策卡片、选项按钮、已选择状态和刷新恢复。
- WebSocket 同步 tool、task、event、ui_card message。
- Agent 对话模型选择：
  - 前端可以在 ClipAnvil 对话框中切换 Producer 模型。
  - 后端持久化 workspace 级默认 Agent 模型选择。
  - `producer_turn` 执行时读取该配置，而不是固定使用环境变量模型。
  - 每个 `producer_turn` task / assistant raw metadata 记录实际 provider、model id 和 model display name。
- Agent 工具身份写入画布的第一条内置路径：创建 `source='agent'` text source material node。
- 明确后续生产工具如何调用 M4/M5 production service 的接口边界。

### 3.2 不包含

- `shot` / `shot_dependency` schema。
- `update_storyboard` 真实落库。
- Workspace Memory / PSS 全量实现。
- Craftsman / Worker / Reviewer / Composer 完整实现。
- 自动生成预览图、视频和成片。
- 复杂 review rubric 和自动重试。
- Studio / Agent 导入导出。
- 多用户协作和团队权限。

这些能力继续放到 M6.5+，但 M6.4 的工具协议必须能承接它们。

## 4. 核心原则

1. **工具是业务语义，不是普通画布 API**
   Agent 不直接调用普通用户的 `/api/nodes`、`/api/edges`、`/api/runs` 路径。工具内部复用已有 store/service，但以 Agent 工具身份做权限、source、requested_by 和审计。

2. **HITL 是工具，不是固定 workflow 卡点**
   `request_user_decision` 由 Producer 判断何时调用。它不代表硬编码的“分镜确认阶段”，后续同一个工具可用于素材选择、预算确认、重做确认、成片确认。

3. **DB 是事实源，PSS 是事实投影**
   本阶段可以先做轻量 `read_workspace_context`，但不能把模型总结当成长期事实源。后续 PSS 必须从 workspace、node、asset、job、version、task、event、shot 等事实构建。

4. **普通用户仍不能写 Agent 画布**
   前端禁用编辑只是 UX。后端普通画布写接口继续保持 Agent Workspace `403`。内部 Agent tool 可以写，但必须走工具层。

5. **所有行动可恢复、可审计**
   每个工具调用都要有 tool_call message、tool_result message 或失败 event。HITL 必须能刷新后恢复卡片和待处理状态。

6. **测试不依赖模型随机输出**
   生产路径使用真实 Volcengine responder；自动化测试必须提供 deterministic responder 或 fixture model output 来触发 tool call / HITL，避免 CI 和本地验收依赖模型稳定性。

## 5. Tool Registry

### 5.1 ToolDefinition

后端新增内部包建议：

```text
apps/server/internal/agent/tools/
  registry.go
  context.go
  canvas.go
  decision.go
  text_node.go
```

核心结构：

```go
type Definition struct {
    Name        string
    Description string
    Parameters  JSONSchema
    Result      JSONSchema
    Safety      SafetySpec
    Timeout     time.Duration
    Visibility  VisibilitySpec
}

type Executor interface {
    Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error)
}
```

`SafetySpec`：

```json
{
  "read_only": false,
  "requires_hitl": false,
  "writes_canvas": true,
  "uses_production_service": false,
  "max_calls_per_turn": 1
}
```

`VisibilitySpec`：

```json
{
  "show_call_message": true,
  "show_result_message": false,
  "user_label": "创建文本素材"
}
```

M6.4 不要求把工具 schema 暴露给前端配置面板，但 Graph 的 model prompt / tool planner 必须使用同一份定义，避免 description 和执行参数漂移。

### 5.2 Tool Call Envelope

模型输出和工程层内部统一成 envelope：

```json
{
  "tool_call_id": "uuid-or-provider-call-id",
  "name": "request_user_decision",
  "arguments": {},
  "source": "producer_model",
  "turn_task_id": "producer_turn_task_id"
}
```

工具结果：

```json
{
  "tool_call_id": "same-id",
  "name": "request_user_decision",
  "status": "succeeded",
  "result": {},
  "error": null
}
```

持久化规则：

- 调用前写 `agent_message(role='assistant', message_type='tool_call')`。
- 工具执行创建 `agent_task(role='producer', task_type='tool_call')`。
- 成功写 `agent_message(role='tool', message_type='tool_result')`。
- 失败写 `tool_result(status='failed')`，并写 `agent_event(type='tool_call_failed')`。
- 不直接把工具结果塞进 assistant text。

### 5.3 Agent 模型选择

Agent 模型选择是运行配置，不是 tool。Producer 不应通过工具自己切换模型；模型选择由用户或 workspace 默认配置决定，ProducerGraph 每轮执行时读取配置。

首期目标：

- 支持 Agent Workspace 的 Producer 对话模型切换。
- 默认模型仍来自 `CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL`。
- 用户选择后持久化到 `workspace.settings`。
- 每次 `producer_turn` 记录实际使用的 provider/model，方便调试、成本核算和回放。
- 为后续 Craftsman / Reviewer / Composer 独立模型选择预留 role scoped 结构。

建议持久化结构：

```json
{
  "agent": {
    "model_selection": {
      "producer": {
        "provider_id": "volcengine",
        "model_id": "doubao-seed-2-0-mini-260428"
      }
    }
  }
}
```

候选模型来源：

- 复用现有 `model_capability`。
- 前端可以先使用已有模型能力列表 API 展示候选项。
- Agent Producer 候选模型必须过滤为可用于文本/对话的 enabled model。
- 如果模型 capability 无法准确表达 “chat/text Agent model”，M6.4 需要补一个后端过滤 helper，避免把 image/video/audio 模型暴露给 Producer 对话选择。

API 建议：

```text
GET /api/agent/workspaces/:workspaceID/model-selection
PUT /api/agent/workspaces/:workspaceID/model-selection
```

`GET` response:

```json
{
  "selection": {
    "producer": {
      "provider_id": "volcengine",
      "model_id": "doubao-seed-2-0-mini-260428"
    }
  },
  "defaults": {
    "producer": {
      "provider_id": "volcengine",
      "model_id": "env-default-model"
    }
  },
  "options": [
    {
      "provider_id": "volcengine",
      "model_id": "doubao-seed-2-0-mini-260428",
      "display_name": "Doubao Mini",
      "limits": {},
      "pricing": {}
    }
  ]
}
```

`PUT` request:

```json
{
  "producer": {
    "provider_id": "volcengine",
    "model_id": "doubao-seed-2-0-mini-260428"
  }
}
```

校验：

- JWT required。
- workspace 必须属于当前 account。
- workspace mode 必须是 `agent`。
- provider/model 必须在 enabled capability 中。
- provider/model 必须支持 Producer 对话所需的文本或 chat operation。

ProducerGraph 执行规则：

- `load_context` 或独立 `load_model_config` 节点读取 workspace model selection。
- 如果 workspace 没有选择，使用环境变量默认模型。
- 如果用户选择的模型后来被禁用：
  - 本轮不静默降级。
  - `producer_turn` failed，error code 建议为 `agent_model_unavailable`。
  - 前端提示用户重新选择模型。
- assistant `raw_message` / task output 必须包含：

```json
{
  "provider": "volcengine",
  "model_id": "doubao-seed-2-0-mini-260428",
  "model_display_name": "Doubao Mini"
}
```

前端：

- 在右侧 ClipAnvil 对话框 header 或 composer 附近提供模型选择入口。
- 不暴露 Producer 角色名，用户看到的是“模型”或“对话模型”。
- 运行中的 Agent task 期间禁用模型切换，避免同一回合中途换模型。
- 切换成功后下一轮 `producer_turn` 生效，不回写历史消息。

## 6. 第一批工具

### 6.1 read_workspace_context

用途：让 Producer 读取当前 Agent Workspace 的项目事实摘要，而不是读取历史对话。历史消息由每轮 ProducerGraph 的 context loader 默认带入完整可用对话窗口；后续做上下文压缩时，也应在 context loader 层处理压缩摘要，而不是让模型通过工具主动读取聊天记录。

Description:

```text
Read the current ClipAnvil Agent workspace context, including workspace metadata,
source material summary, canvas summary, running tasks, and production status summary.
Use this before making decisions that depend on existing assets or task state.
```

参数：

```json
{
  "type": "object",
  "properties": {
    "include_assets": {
      "type": "boolean",
      "description": "Whether to include source material nodes and assets."
    },
    "include_canvas_summary": {
      "type": "boolean",
      "description": "Whether to include a compact canvas summary."
    },
    "include_tasks": {
      "type": "boolean",
      "description": "Whether to include recent agent tasks and generation jobs."
    }
  },
  "additionalProperties": false
}
```

执行：

- 读取 workspace。
- 不读取历史对话；历史对话由 ProducerGraph context loader 负责。
- 可选读取 `source='agent'` / source material nodes 摘要。
- 可选读取 canvas 节点数量、类型分布、关键节点标题和当前生产状态摘要。
- 可选读取最近 `agent_task` 和 generation job 摘要。
- 不返回大体积文本、完整 asset 内容或完整节点列表。

结果：

```json
{
  "workspace": {"id": "...", "title": "...", "mode": "agent"},
  "source_material_summary": "已有 2 个图片素材、1 个文本 brief。",
  "source_material_refs": [
    {"node_id": "...", "asset_id": "...", "type": "image", "title": "商品主图"}
  ],
  "canvas_summary": "当前画布包含 3 个 source material node，暂无生成节点。",
  "recent_tasks": [],
  "summary": "当前项目已有 2 个素材，暂无分镜和生成任务。"
}
```

### 6.2 条件工具：list_canvas_nodes

默认决策：M6.4 不把 `list_canvas_nodes` 作为必做工具。`read_workspace_context` 必须返回足够完成首期链路的画布摘要和少量关键节点引用，例如最近 source material 的 node id、asset id、类型、标题。

条件纳入：如果实现时发现 `read_workspace_context` 的摘要不足以稳定完成工具链路、HITL 验收或精确节点引用，则 M6.4 同步实现 `list_canvas_nodes`。它的定位必须是 `read_workspace_context` 的详细钻取工具，而不是第二个上下文摘要工具。

用途：只读列出 Agent 画布节点详情。`read_workspace_context` 只给摘要，避免每轮上下文塞入完整画布；当 Producer 需要解析用户“这个素材”“刚才上传的图”、精确选择 node id、查看节点标题/类型/版本状态时，再调用 `list_canvas_nodes`。

Description:

```text
List read-only canvas nodes in the current Agent workspace. Use this to inspect
source materials and generated outputs before deciding what to create or ask next.
```

参数：

```json
{
  "type": "object",
  "properties": {
    "node_types": {
      "type": "array",
      "items": {"type": "string", "enum": ["text", "image", "video", "audio", "reference_pack"]},
      "description": "Optional node type filter."
    },
    "source_only": {
      "type": "boolean",
      "description": "Only return source material nodes."
    },
    "limit": {
      "type": "integer",
      "minimum": 1,
      "maximum": 100
    }
  },
  "additionalProperties": false
}
```

执行：

- 调用现有 canvas/node query，不走普通用户 mutation API。
- 支持分页、过滤和 source-only 查询。
- 不返回大段文本或二进制内容；文本素材只返回摘要。
- 与 `read_workspace_context` 的边界是：`read_workspace_context` 返回项目摘要和少量关键节点引用，`list_canvas_nodes` 返回可被后续工具继续引用的节点详情。

### 6.3 create_agent_text_node

用途：Producer 把用户需求整理成一个 Agent-owned text source material node，例如商品 brief、创意方向、脚本草稿。它是 M6.4 打通工具写入链路的最小写工具；后续 Storyboard / Memory / PSS 落地后，它不会成为主编排工具，但仍可保留用于创建持久文本素材和用户认可的 brief/script/note。

Description:

```text
Create an Agent-owned text source material node in the current Agent workspace.
Use this for durable briefs, scripts, notes, and user-approved creative direction.
Do not use it for transient chat replies.
```

参数：

```json
{
  "type": "object",
  "required": ["title", "text"],
  "properties": {
    "title": {
      "type": "string",
      "minLength": 1,
      "maxLength": 120,
      "description": "User-readable node title."
    },
    "text": {
      "type": "string",
      "minLength": 1,
      "maxLength": 12000,
      "description": "Text content to persist as a source material asset."
    },
    "placement": {
      "type": "object",
      "properties": {
        "x": {"type": "number"},
        "y": {"type": "number"}
      },
      "additionalProperties": false
    }
  },
  "additionalProperties": false
}
```

执行：

- 校验 workspace 是 Agent mode。
- 创建 text `media_asset`。
- 调用现有 `CreateAgentMediaNode` 或等价 internal query。
- 节点 `source='agent'`，`node_type='text'`。
- 广播 canvas `NodeCreated`。
- 写入 tool result，包含 node id / asset id。

此工具是 M6.4 的最小写工具，用来验证 Agent 内部写入路径，而不是完整 storyboard 工具。M6.5 之后，结构化分镜应走 `update_storyboard`，长期项目记忆应走 Memory/PSS 相关工具；`create_agent_text_node` 只负责把确实需要出现在画布上的文本素材落为 node。

### 6.4 request_user_decision

用途：Producer 请求用户做选择或确认，并暂停当前 Graph。

Description:

```text
Ask the user to make a decision before continuing. This creates a decision card,
persists an interrupt checkpoint, marks the Producer turn as waiting_for_user,
and resumes after the user selects an option or provides free text.
Use this only when the next action needs user confirmation, missing information,
cost approval, or a preference choice.
```

参数：

```json
{
  "type": "object",
  "required": ["title", "message"],
  "properties": {
    "title": {
      "type": "string",
      "minLength": 1,
      "maxLength": 120
    },
    "message": {
      "type": "string",
      "minLength": 1,
      "maxLength": 2000
    },
    "options": {
      "type": "array",
      "minItems": 0,
      "maxItems": 6,
      "items": {
        "type": "object",
        "required": ["id", "label"],
        "properties": {
          "id": {"type": "string", "minLength": 1, "maxLength": 64},
          "label": {"type": "string", "minLength": 1, "maxLength": 80},
          "description": {"type": "string", "maxLength": 240}
        },
        "additionalProperties": false
      }
    },
    "allow_free_text": {
      "type": "boolean"
    },
    "default_option_id": {
      "type": "string"
    },
    "metadata": {
      "type": "object",
      "additionalProperties": true
    }
  },
  "additionalProperties": false
}
```

执行：

1. 写入 checkpoint：

```json
{
  "graph_name": "producer_turn",
  "interrupt_type": "request_user_decision",
  "tool_call_id": "...",
  "state": "opaque-eino-or-clipanvil-state-bytes"
}
```

2. 更新 `agent_thread.current_checkpoint_key`。
3. 创建 `agent_event(type='decision_requested', status='pending')`。
4. 创建 `agent_message(role='assistant', message_type='ui_card')`。
5. 标记当前 `producer_turn` task 为 `waiting_for_user`。
6. 广播 message / event / task。
7. Graph 返回 interrupt，不写最终 assistant text。

`ui_card` content：

```json
{
  "card_type": "decision_request",
  "decision_id": "event-id",
  "checkpoint_key": "agent:workspace:thread:task",
  "title": "确认创意方向",
  "message": "你希望这条视频更偏转化还是品牌质感？",
  "options": [
    {"id": "conversion", "label": "更偏转化"},
    {"id": "brand", "label": "更偏品牌"}
  ],
  "allow_free_text": true,
  "status": "pending"
}
```

## 7. HITL Resume API

新增：

```text
POST /api/agent/workspaces/:workspaceID/decisions/:eventID/respond
```

Request:

```json
{
  "selected_option_id": "conversion",
  "free_text": "偏转化，但不要太硬广",
  "client_response_id": "optional-idempotency-key"
}
```

校验：

- JWT required。
- workspace 属于当前 account。
- workspace mode 必须是 `agent`。
- `eventID` 必须属于该 workspace。
- event type 必须是 `decision_requested`。
- event status 必须是 `pending`。
- 选项必须存在，除非 `allow_free_text=true` 且提供 `free_text`。
- 重复提交同一个 `client_response_id` 必须幂等返回已处理结果。

行为：

1. 创建 user message 或 system message 记录用户选择：

```json
{
  "text": "选择：更偏转化。补充：偏转化，但不要太硬广",
  "decision_id": "event-id",
  "selected_option_id": "conversion",
  "free_text": "偏转化，但不要太硬广"
}
```

2. 更新原 decision event 为 `handled`。
3. 创建 `decision_resolved` event。
4. 创建 `agent_task(task_type='decision_resume', status='queued')`。
5. 启动 resume runner。
6. 广播 card resolved、user decision message、resume task。

响应：

```json
{
  "decision_event": {},
  "resolved_event": {},
  "message": {},
  "task": {}
}
```

## 8. ProducerGraph Resume

M6.4 需要定义两类执行入口：

```go
RunTask(ctx, ProducerTurnInput)
ResumeDecision(ctx, ProducerResumeInput)
```

`ProducerResumeInput`：

```go
type ProducerResumeInput struct {
    WorkspaceID   pgtype.UUID
    ThreadID      pgtype.UUID
    TaskID        pgtype.UUID // decision_resume task
    DecisionEventID pgtype.UUID
    CheckpointKey string
    UserResponse  DecisionResponse
    EmitDelta     ProducerDeltaHandler
}
```

Resume 流程：

```text
load_checkpoint
  -> restore_graph_state
  -> inject_decision_result
  -> continue_graph
  -> persist_followup_tool_result_or_assistant_message
  -> mark_decision_resume_succeeded
```

M6.4 必须接入 Eino 原生 CheckPointStore 适配器，这是 HITL 的基础能力，不作为后续替换项。ClipAnvil 的 `eino_checkpoint` 表是持久化后端，ProducerGraph 的 interrupt/resume 必须通过该适配器读写 checkpoint，而不是用临时 GraphState 绕过 Eino checkpoint 语义。

关键要求：

- checkpoint 落在 `eino_checkpoint`。
- `agent_thread.current_checkpoint_key` 指向它。
- resume 入口从 checkpoint 恢复，而不是只根据 event 临时重跑。
- CheckPointStore adapter 必须覆盖写入、读取、删除和 metadata 透传。
- 自动化测试必须验证进程内 Graph 中断后，使用持久化 checkpoint 可以恢复执行。

Checkpoint metadata 建议包含 ClipAnvil 可读信息，方便调试和权限校验：

```json
{
  "workspace_id": "...",
  "thread_id": "...",
  "producer_turn_task_id": "...",
  "messages_window": [],
  "tool_call_count": 1,
  "pending_tool_call": {
    "tool_call_id": "...",
    "name": "request_user_decision",
    "arguments": {}
  },
  "model_context": {},
  "resume_policy": {
    "after_decision": "continue_model"
  }
}
```

## 9. WebSocket 事件协议

现有事件：

- `agent.message.created`
- `agent.message.delta`
- `agent.task.updated`
- `agent.event.created`

M6.4 继续复用这四类，不新增前端必须识别的新顶层类型。变化在 payload 内容：

- tool call message：`message_type='tool_call'`
- tool result message：`message_type='tool_result'`
- decision card：`message_type='ui_card'`
- waiting task：`task.status='waiting_for_user'`
- decision events：`event.event_type='decision_requested'/'decision_resolved'`

前端必须：

- 对未知 `message_type` 有 fallback。
- 对 `ui_card.card_type='decision_request'` 渲染决策卡片。
- 对已 resolved 的 decision 禁用按钮并展示选择。
- WebSocket 断开重连后通过 `GET messages` 恢复卡片，通过后续 task/event 查询或 message content 恢复 resolved 状态。

M6.4 可以先不新增 `GET events`，但如果仅靠 messages 无法稳定恢复卡片状态，就必须新增：

```text
GET /api/agent/workspaces/:workspaceID/events?after=<timestamp-or-id>&limit=100
```

优先方案：respond API 在原 `ui_card` message 后追加 resolved status message 或通过 `decision_resolved` event 广播让当前页面更新；刷新后通过 `ui_card.content.status` 的更新需要支持 message update。如果当前 message append-only 不支持更新，则采用“追加 resolved status message + 前端按 decision_id 合并”的方式，避免改写 append-only 语义。

## 10. 前端设计

在 `AgentWorkspacePage` 的右侧悬浮对话框中新增：

- `AgentDecisionCard` 组件。
- 对话模型选择控件：
  - 展示 enabled text/chat model options。
  - 读取并保存 Agent Workspace 的 Producer 模型选择。
  - Agent 运行中禁用切换。
  - 切换后下一轮消息生效。
- `respondAgentDecision(workspaceId, eventId, response)` API。
- `agentDecision.ts` helper：
  - parse card content。
  - merge resolved state。
  - validate option。
- `hasRunningProducerTask` 调整为：
  - `queued` / `running` 显示思考动效。
  - `waiting_for_user` 不显示“正在思考”，而显示卡片等待用户。
- 首期交互规则：
  - 当存在 `producer_turn` / `decision_resume` task 处于 `queued`、`running` 或 `waiting_for_user` 时，普通文本 composer 禁止再次发送消息。
  - `waiting_for_user` 时只允许用户通过 pending decision card 响应。
  - 后续可以迭代消息队列：用户在 Agent 忙碌时输入的下一句先进入 queued user intent，当前 Agent 回合结束后自动触发下一轮。
- message renderer 支持：
  - `text`
  - `error`
  - `status`
  - `tool_call`
  - `tool_result`
  - `ui_card`

UI 原则：

- 继续使用 `ClipAnvil`，不暴露 Producer。
- 模型选择入口使用用户语言，例如“模型”或“对话模型”，不要显示内部 `producer` 字样。
- 决策卡片是对话内容的一部分，不做模态弹窗。
- 卡片按钮应紧凑、现代、可禁用。
- 已选择后必须明确显示用户选择，防止重复点击造成误判。
- 刷新页面后 pending card 仍可继续响应。
- 禁用发送时输入框应展示运行状态，但不要把 Producer 等内部角色名暴露给用户。

## 11. 与 M4/M5 生产底座的复用边界

M6.4 的生产桥接只定义接口和最小写工具，不直接完成完整生产。

后续工具应统一走 `Tool Registry -> internal service`：

```text
generate_node
  -> production.Service.SubmitNodeRun
  -> generation_job
  -> artifact_version
  -> production runner / provider bridge / sandbox
```

Agent requested_by：

```json
{
  "type": "agent",
  "id": "agent_task_id",
  "role": "producer|craftsman|worker|composer"
}
```

若现有 `requested_by_type` 只接受 `user` 或 string 未约束，则 M6.4 不必迁移；若 DB enum 约束不支持 `agent`，后续 M6.5/M6.6 必须补迁移。

M6.4 先验收 `create_agent_text_node`，因为它能验证：

- Agent 内部写 canvas 的路径独立于普通用户写接口。
- Agent-created node 能出现在只读画布。
- tool call/result/message/event/task 能完整落库和同步。

## 12. 数据模型是否需要迁移

优先不新增迁移。当前表已包含 M6.4 所需基础：

- `agent_message.message_type` 支持 `tool_call`、`tool_result`、`ui_card`。
- `agent_task.status` 支持 `waiting_for_user`。
- `agent_task.task_type` 支持 `tool_call`、`decision_resume`。
- `eino_checkpoint` 可持久化 checkpoint。
- `workspace.settings` 可持久化 Agent model selection。

可能需要补充的 sqlc 查询：

- 更新 workspace settings 中的 `agent.model_selection`。
- 按 `event_id` 获取 decision event。
- 标记 event handled。
- 读取 event payload。
- 追加 decision response message。
- 查找同一 event 的 resolved message，用于幂等。
- 创建 / 查询 tool_call task。

如果 append-only message 无法表达 card resolved 状态，不要在 M6.4 修改 `agent_message` 为可更新；优先追加 `status` message 或 `decision_resolved` message，并在前端合并展示。

## 13. 错误处理

- 未知工具名：写 `tool_result(status='failed', error_code='agent_tool_unknown')`，本轮 ProducerGraph 失败或交给模型解释。
- 参数 schema 校验失败：写 `agent_tool_invalid_arguments`。
- 工具执行超时：写 `agent_tool_timeout`。
- 普通用户访问 decision respond 但 workspace 不属于自己：`403`。
- decision 已处理：幂等 key 命中则返回成功；非幂等重复提交返回 `409` 或 `400 decision already resolved`。
- checkpoint 丢失：`decision_resume` task failed，写 assistant error message，提示用户重新发起请求。
- `request_user_decision` 无 options 且不允许 free text：schema 校验失败。
- Graph resume 后模型再次要求同一个 decision：必须产生新 decision event，不能复用旧 event。

## 14. 可交付标准

M6.4 完成时必须满足：

- Agent Workspace 支持 Producer 对话模型选择，并持久化到 `workspace.settings`。
- `producer_turn` 使用 workspace 选择的模型；未选择时使用环境变量默认模型。
- 每条 assistant message raw metadata 或 task output 记录实际 provider/model。
- Tool registry 能返回第一批工具定义，包含 name、description、parameters、safety 和 visibility。
- ProducerGraph 能识别模型 tool call，并持久化 tool_call message。
- `create_agent_text_node` 能通过工具创建 Agent-owned text source material node，普通用户画布写接口仍在 Agent Workspace 返回 `403`。
- `request_user_decision` 能创建 ui_card、decision_requested event、checkpoint，并把当前 producer_turn 标记为 `waiting_for_user`。
- 前端能在右侧 ClipAnvil 对话框渲染 pending decision card。
- 用户点击选项后，后端创建 decision_resolved event 和 decision_resume task。
- ProducerGraph 能从 checkpoint resume，并输出后续 assistant message。
- 刷新页面后，历史 tool call、tool result、decision card、用户选择和 assistant follow-up 都能恢复。
- WebSocket 能同步 tool/message/task/event 状态到第二个浏览器标签。
- 自动化测试不依赖真实 LLM 的随机 tool call；真实 LLM 路径可以用于本地浏览器 smoke。

## 15. 验收测试标准

### 15.1 自动化命令

如果没有新增迁移：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
make server-lint
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

如果新增或修改 SQL / sqlc：

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
make server-lint
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

### 15.2 Server 单测

必须覆盖：

- Tool registry 注册和重复 name 拒绝。
- Tool definition schema 包含 description 和 parameters。
- Agent model selection GET/PUT 权限、mode 校验和 capability 校验。
- ProducerGraph 使用 workspace model selection 创建 model responder。
- 已禁用或不支持文本/对话的模型会阻断 `producer_turn`，并返回 `agent_model_unavailable`。
- `create_agent_text_node` 只允许 Agent Workspace。
- `create_agent_text_node` 写入 `source='agent'` source material node。
- tool call 成功写 task/message/event。
- tool call 失败写 failed tool_result 和 failed event。
- `request_user_decision` 写 checkpoint、ui_card、decision_requested event、waiting task。
- decision respond 权限、选项校验、重复提交、event handled。
- resume checkpoint 不存在时 decision_resume task failed。

### 15.3 Web 单测

必须覆盖：

- `ui_card.decision_request` 解析。
- pending / resolved card 合并。
- option click request payload。
- model selection options 过滤和保存 payload。
- Agent task running/waiting 时模型选择控件禁用。
- `waiting_for_user` 不触发 thinking indicator。
- unknown `message_type` fallback。
- tool_call / tool_result 渲染不会破坏消息列表滚动。

### 15.4 浏览器 E2E

必须使用当前 worktree 的脚本启动：

```bash
./scripts/dev-start.sh
```

并使用脚本输出的 Vite URL。

验收步骤：

1. 注册或登录测试账号。
2. 创建 Agent Workspace。
3. 上传或发送一个简单素材说明。
4. 在对话框中切换 Producer 对话模型，刷新后确认选择保留。
5. 发送一条普通消息，确认 assistant raw metadata 或数据库 task output 记录刚选择的 provider/model。
6. 发送会触发 `create_agent_text_node` fixture 的测试消息，确认：
   - 对话中出现 tool call / tool result 或可观察状态。
   - 只读画布出现 Agent-created text source node。
   - 普通用户 node mutation API 对该 Agent Workspace 仍返回 `403`。
7. 发送会触发 `request_user_decision` fixture 的测试消息，确认：
   - 对话中出现决策卡片。
   - task 状态进入 `waiting_for_user`。
   - thinking indicator 停止。
8. 打开第二个同 workspace 标签页，确认卡片同步出现。
9. 在第一个标签选择一个选项并提交。
10. 确认卡片变为 resolved，第二个标签同步 resolved 状态。
11. 确认 Producer resume 后出现后续 assistant message。
12. 刷新页面，确认 model selection、tool call、tool result、decision card、用户选择和 assistant follow-up 全部恢复。

### 15.5 数据库抽查

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select settings->'agent'->'model_selection' as agent_model_selection from workspace order by updated_at desc limit 5;"
```

预期能看到 Producer model selection。

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select message_type, role, content from agent_message order by created_at desc limit 10;"
```

预期能看到：

- `tool_call`
- `tool_result`
- `ui_card`
- user decision response
- assistant follow-up

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select task_type,status,input,output from agent_task order by created_at desc limit 10;"
```

预期能看到：

- `producer_turn` 曾进入 `waiting_for_user`。
- `tool_call` succeeded。
- `decision_resume` succeeded。

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select event_type,status,payload from agent_event order by created_at desc limit 10;"
```

预期能看到：

- `tool_call_started`
- `tool_call_completed`
- `decision_requested`
- `decision_resolved`

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select key, metadata from eino_checkpoint order by updated_at desc limit 5;"
```

预期能看到 latest decision checkpoint。

## 16. 与后续阶段的接点

M6.5 Storyboard / PSS：

- 在本阶段 Tool Registry 上新增 `update_storyboard`、`get_production_state`。
- 新增 `shot` / `shot_dependency`。
- `read_workspace_context` 升级为 PSS builder 的输入之一。

M6.6 Craftsman / Worker：

- 新增 Craftsman scoped thread。
- `dispatch_craftsman` 创建 shot scoped task。
- Worker tool 调用 `production.Service.SubmitGenerationIntent` 或 `SubmitNodeRun`。

M6.7 Review retry：

- 工具结果和 generation job 事件进入 review graph。
- retry policy 使用 agent_task / generation_job attempt 上限，不依赖模型自己记忆。

M6.8 Composer：

- ComposerGraph 复用 tool registry、task/event/message、checkpoint 和 sandbox job。
- 成片确认继续使用 `request_user_decision`，不是新的固定 gate。

## 17. 开放问题

1. `tool_call` 是否采用模型供应商原生 tool calling，还是先用 JSON fenced block 解析？
   推荐：自动化测试走 deterministic JSON；真实 Volcengine 路径优先使用 Eino/schema tool call 能力，若该低成本模型 tool calling 不稳定，再加严格 JSON fallback。

2. `ui_card` resolved 状态是否更新原 message？
   推荐：保持 message append-only，先追加 `decision_resolved` event / status message，前端按 `decision_id` 合并。

3. 是否需要取消 decision？
   推荐：M6.4 可以先不做显式 cancel，但 task/event schema 已支持 cancelled；M6.5 再补用户取消和自动过期策略。
