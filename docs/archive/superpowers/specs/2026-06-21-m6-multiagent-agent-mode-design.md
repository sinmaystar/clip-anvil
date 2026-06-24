# M6 MultiAgent Agent Mode 设计方案

**状态**：待评审
**日期**：2026-06-21
**阶段目标**：完成基于 Eino Graph 编排的 MultiAgent Agent 模式。M6 覆盖 Agent 状态持久化、前端对话界面、WebSocket 同步、HITL、工具注册、Storyboard/PSS、Producer / Craftsman / Worker / Composer、评审重试、预览图生成、视频生成和成片合成。

## 1. 背景

M3-M5 已完成 Workspace 模式入口、共享生产底座和 Studio 手动生产闭环。当前 Agent Workspace 已有独立路由和只读画布壳子，但还没有真正的 Agent 对话、Agent 状态持久化、HITL、Agent 事件流或工具执行框架。

M6 里程碑目标是完整 Agent 自动生产模式，但实施顺序仍应先建立 Agent runtime foundation，再逐步接入完整 MultiAgent 生产能力：

```text
Agent 对话与状态持久化
  -> Agent WebSocket 事件同步
  -> HITL 决策卡片
  -> Edge registry 和工具调用审计
  -> Storyboard / PSS
  -> Producer / Craftsman / Worker / Composer Graph 编排
  -> 复用 M4/M5 生产底座执行预览图、视频和成片生成
```

这样可以避免工具调用、模型输出、前端状态和人工确认散落在临时代码里。

## 2. 设计原则

1. **先状态，后智能**：先让每条用户消息、Producer 回复、工具调用、UI 卡片和任务状态都可恢复、可审计，再接入复杂模型推理。
2. **Agent 看到生产工具，不看到画布原子操作**：Agent 调 `update_storyboard`、`generate_shot_preview` 这类生产语义工具；工具内部再调用节点、边、版本和生产服务。
3. **普通用户仍不能写 Agent 画布**：前端禁用编辑只是 UX；后端普通画布写接口继续在 Agent Workspace 返回 `403`。
4. **Agent 写入走内部工具层**：内部工具层校验 workspace 是 Agent 模式，写入 `source='agent'` 和 `requested_by_type/requested_by_id`，不开放给普通用户绕过权限。
5. **PSS 是 DB 事实投影**：Producer 和 Craftsman 每轮从 DB 事实构建自然语言状态，不把 LLM 总结当事实源。
6. **HITL 是工具，不是固定流程卡点**：Producer 在需要时调用 `request_user_decision`，用户可通过按钮或自然语言恢复流程。
7. **Eino Graph 是主编排层**：M6 需要细粒度控制 Agent 执行过程，因此 Producer turn、HITL、工具调用、Craftsman dispatch、Worker 任务、Review retry 和 Composer 都应落在 Eino Graph 或工程任务状态机可观察的节点上。Eino ADK Agent/middleware 可以作为局部实现方式，但不能遮蔽关键状态转移。

## 3. Eino 方案选择

Eino 官方文档同时提供低层编排能力和 ADK 高层 Agent 能力：Graph 是更灵活的有向图编排；ADK 提供 Agent 抽象，支持多 Agent 编排、HITL interrupt 和预置 Agent pattern。官方 `ChatModelAgent` 使用 ChatModel 作为决策器、Edges 作为行动空间，并通过 ReAct loop 执行 tool call。Middleware 可在 agent run、model call、tool call 周围扩展上下文、工具、日志、压缩、修复中断后的 tool call 等行为。

### 3.1 方案 A：Eino Graph 主编排

形态：

```text
ProducerGraph / CraftsmanGraph / ComposerGraph
  -> load_pss
  -> call_model
  -> route_tool_call
  -> execute_tool
  -> request_or_resume_hitl
  -> dispatch_task
  -> persist_messages
  -> decide_next
```

优点：

- 对每个状态转移、并发调度、错误分支、HITL 暂停/恢复和事件落库有最高控制力。
- 适合 Producer -> Craftsman -> Worker -> Review -> Retry -> Composer 的完整确定性编排。
- 更容易把业务状态机、agent_task、agent_event 和 Graph 节点一一对应。
- 更适合跨 shot 阻塞依赖、并行调度、失败重试上限、用户中途干预和 Composer 合成。

缺点：

- 首期成本高，需要先定义 Graph state、节点输入输出、checkpoint 和错误分支。
- 需要自己处理 tool call 消息修补、上下文压缩、HITL resume、事件流映射等通用问题。
- Graph 结构一旦过细，可能让早期迭代变慢，需要控制首期节点数量。

### 3.2 方案 B：Eino ADK ChatModelAgent + Middleware 主导

形态：

```text
Producer ChatModelAgent
  + PSS middleware
  + ClipAnvil tool registry
  + message persistence middleware
  + event emission middleware
  + HITL middleware
  + summarization / reduction middleware
```

优点：

- 更贴合 M6.1：先跑通对话、工具、HITL、事件同步。
- 可以把 ClipAnvil 工具作为普通 tools 暴露给 Producer，不需要先写完整 Graph。
- Middleware 天然适合接入 PSS、消息持久化、工具审计、WebSocket event、checkpoint。
- 后续 Producer、Craftsman 可以各自是一个 ChatModelAgent，先形成 MultiAgent 形态。

缺点：

- 对复杂跨 Agent 并行、阻塞依赖调度、重试策略的确定性控制弱于 Graph。
- 如果所有流程都塞进一个 Agent loop，后期可能出现工具调用路径不够清晰。
- 需要明确哪些逻辑由 Agent 决策，哪些由工程层状态机决定。
- 对 M6 要求的完整 review retry、跨 shot 依赖、Composer 合成和中途干预来说，关键执行过程可能被 ReAct loop 隐藏，不利于精细控制。

### 3.3 推荐

M6 采用 **方案 A：Eino Graph 主编排**。

理由：

- M6 不是只做对话壳子，而是要完成完整 MultiAgent 架构，需要细粒度控制 Agent 执行过程。
- Producer、Craftsman、Worker、Review retry、Composer 都会产生持久任务、事件、消息和版本状态。Graph 节点能自然对应这些可审计状态。
- HITL 不是普通 tool result，而是会暂停和恢复执行；Graph + checkpoint 更适合表达可恢复边界。
- ADK ChatModelAgent 可以作为某些 Graph 节点内部的模型调用/工具调用封装，但不作为最高层编排。

推荐演进：

```text
M6.1: Agent runtime schema/service、message/event/task/checkpoint 持久化
M6.2: Agent API、/ws/agent、右侧悬浮 Producer 对话界面
M6.3: ProducerGraph 首版，支持 message -> PSS -> model -> persist -> response
M6.4: HITL interrupt/resume 和 request_user_decision
M6.5: Storyboard tools、shot/shot_dependency、PSS
M6.6: CraftsmanGraph + Worker task，生成预览图和视频
M6.7: Review rubric、自动改写重试、跨 shot 依赖调度
M6.8: ComposerGraph，生成成片并通过 HITL 确认
```

## 4. M6 范围

### 4.1 包含

- Agent runtime 数据表。
- Agent thread 和 message 持久化，供 Producer、Craftsman、Reviewer、Composer 等需要持久上下文的 Agent 共用。
- Agent WebSocket hub 和事件协议。
- 前端 Agent 对话工作台，右侧悬浮在只读画布之上。
- HITL 决策卡片基础协议。
- Edge registry 的定义、工具调用落库和事件同步。
- Eino Graph 编排、checkpoint、interrupt/resume 和 Graph 节点状态映射。
- PSS builder 的首版输入/输出口径。
- 完整 Producer / Craftsman / Worker / Reviewer / Composer MultiAgent 架构。
- 自动生成预览图、视频和成片。
- 完整 review rubric、critique 记录和自动重试策略。
- 跨 shot 依赖调度。
- 复用 M4/M5 生产底座的 Agent 生产工具。

### 4.2 不包含

- Studio / Agent 导入导出。

## 5. 数据模型

### 5.1 agent_thread

Agent thread 表示一个持久对话线程，是通用 Agent runtime 能力，不只属于 Producer。Producer 首期每个 Agent Workspace 一个 active thread；后续每个 shot 可有一个 Craftsman thread，Reviewer 和 Composer 也可以按 workspace 或 final output scope 持久化线程。

```sql
CREATE TABLE agent_thread (
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
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

约束：

- `role` 支持 `producer`、`craftsman`、`reviewer`、`composer`，后续可扩展。
- `scope_type='workspace'` 表示 Producer；`scope_type='shot'` 表示 Craftsman。
- 同一 workspace 首期只需要一个 active Producer thread。

### 5.2 agent_message

消息是对话 UI 和 Agent resume 的事实源。

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

`role`：

- `user`
- `assistant`
- `tool`
- `system`

`message_type`：

- `text`
- `tool_call`
- `tool_result`
- `ui_card`
- `error`
- `status`

### 5.3 agent_task

Agent task 是工程层异步任务，不是新的智能角色。

```sql
CREATE TABLE agent_task (
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);
```

首期 `task_type`：

- `producer_turn`
- `tool_call`
- `decision_resume`

后续：

- `dispatch_craftsman`
- `generate_preview`
- `generate_video`
- `compose_final`

### 5.4 agent_event

Agent event 是前端同步和工程层唤醒 Producer 的事件源。

```sql
CREATE TABLE agent_event (
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
```

首期事件：

- `message_created`
- `producer_turn_started`
- `producer_turn_completed`
- `producer_turn_failed`
- `tool_started`
- `tool_succeeded`
- `tool_failed`
- `decision_requested`
- `decision_resolved`

### 5.5 eino_checkpoint

Eino interrupt/resume 的 checkpoint 持久化表。首期可以只建表，M6.2 开始接入 HITL。

```sql
CREATE TABLE eino_checkpoint (
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

### 5.6 memory_document / memory_revision

Workspace Memory 存长期创意认知，不存实时执行状态。

```sql
CREATE TABLE memory_document (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    doc_type TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    current_revision_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE memory_revision (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    document_id UUID NOT NULL REFERENCES memory_document(id) ON DELETE CASCADE,
    content JSONB NOT NULL DEFAULT '{}',
    summary TEXT NOT NULL DEFAULT '',
    created_by_type TEXT NOT NULL DEFAULT 'agent',
    created_by_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

首期可只支持一个 `doc_type='workspace_memory'`。

### 5.7 shot / shot_dependency

Storyboard 是 M6 的生产语义锚点。shot 不重复存储版本/job/画布状态。

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
    craftsman_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

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

同时给 `media_node` 增加：

```sql
ALTER TABLE media_node ADD COLUMN shot_id UUID REFERENCES shot(id) ON DELETE SET NULL;
```

## 6. Agent WebSocket

当前系统只有 `/ws/canvas`，用于画布节点、边、生产任务状态更新。M6 需要新增 `/ws/agent`，用于对话、HITL 和 Agent task/event。

### 6.1 路由

```text
GET /ws/agent?workspaceId=<uuid>&token=<jwt>
```

鉴权逻辑复用 canvas websocket：

- token 必须有效。
- workspace 必须属于当前 account。
- workspace mode 必须是 `agent`。

### 6.2 事件 envelope

```json
{
  "type": "agent.message.created",
  "workspace_id": "uuid",
  "thread_id": "uuid",
  "event_id": "uuid",
  "task_id": "uuid",
  "seq": 12,
  "payload": {}
}
```

首期事件类型：

- `agent.message.created`
- `agent.task.updated`
- `agent.tool.started`
- `agent.tool.completed`
- `agent.tool.failed`
- `agent.decision.requested`
- `agent.decision.resolved`
- `agent.error`

### 6.3 与 Canvas WebSocket 的关系

不合并 `/ws/canvas` 和 `/ws/agent`。

原因：

- Canvas 事件是画布投影和 production job 的同步。
- Agent 事件是对话、任务、工具和 HITL 的同步。
- 前端 Agent 页面可以同时订阅两条通道：右侧悬浮对话框用 `/ws/agent`，底层只读画布用 `/ws/canvas` 或 canvas query invalidation。

## 7. 后端 API

### 7.1 Thread API

```text
GET /api/agent/workspaces/:workspaceID/thread
```

返回当前 workspace 的 Producer thread；不存在则创建。

### 7.2 Message API

```text
GET /api/agent/workspaces/:workspaceID/messages?thread_id=<uuid>&after_seq=<n>
POST /api/agent/workspaces/:workspaceID/messages
```

POST input：

```json
{
  "thread_id": "uuid",
  "content": {
    "text": "帮我做一条 15 秒商品视频"
  }
}
```

行为：

1. 校验 workspace 是 agent mode。
2. 写 user `agent_message`。
3. 写 `agent_event(message_created)`。
4. 创建 `agent_task(task_type='producer_turn')`。
5. 创建 `agent_task(task_type='producer_turn')` 后交给 ProducerGraph 执行。开发初期可以用 mock graph node 生成 assistant 回复，但任务、消息和事件路径必须与真实 Graph 一致。

### 7.3 Decision API

```text
POST /api/agent/decisions/:eventID/resolve
```

input：

```json
{
  "option_id": "approve_storyboard",
  "free_text": ""
}
```

行为：

1. 将 `decision_requested` event 标记 handled。
2. 写 `decision_resolved` event。
3. 写用户选择消息。
4. 创建 `agent_task(task_type='decision_resume')`。
5. 后续恢复 Eino checkpoint。

## 8. 前端 Agent 工作台

### 8.1 布局

Agent Workspace 首屏应是只读画布为底、右侧悬浮 Producer 对话框的实际工作台。对话框不是左侧固定面板，避免把画布压缩成列表视图。

```text
┌──────────────────────────────────────────────────────────┐
│ Topbar: workspace name / status / back                   │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  Read-only production canvas                             │
│  - React Flow/canvas projection                              │
│  - nodes / shots / current versions / stale/job badges   │
│                                                          │
│                              ┌────────────────────────┐  │
│                              │ Producer Chat          │  │
│                              │ - messages             │  │
│                              │ - tool status          │  │
│                              │ - HITL cards           │  │
│                              │ - composer input       │  │
│                              └────────────────────────┘  │
└──────────────────────────────────────────────────────────┘
```

布局规则：

- 画布占满工作台主体，用户不可直接编辑。
- Producer 对话框固定在右侧，悬浮在画布之上，桌面端默认宽度约 380-440px。
- 对话框可折叠为窄条，避免遮挡画布关键区域。
- 移动端对话框作为底部抽屉或全屏面板，画布保持可查看。
- 对话框内的 HITL 卡片和工具状态应能引用并高亮画布上的 shot/node。

### 8.2 对话流

需要渲染：

- user text message
- assistant text message
- tool call collapsed row
- tool result collapsed row
- ui_card decision
- task/error/status row

### 8.3 输入框

首期支持：

- 多行文本输入。
- Enter 发送，Shift+Enter 换行。
- 发送中禁用。
- WebSocket 断开时仍允许通过 HTTP 发送，但显示离线/重连状态。

### 8.4 HITL 卡片

卡片字段：

```ts
interface DecisionCard {
  title: string;
  summary: string;
  options: Array<{
    id: string;
    label: string;
    description?: string;
  }>;
  blocking: boolean;
  status: "pending" | "resolved" | "cancelled";
}
```

卡片交互：

- pending 时可点击选项。
- resolved 后显示已选择项。
- 用户也可以通过自然语言回复，Producer 解析后 resolve。

## 9. Edge Registry

Edge registry 是 Eino 和 ClipAnvil 业务服务之间的边界。

```go
type EdgeContext struct {
    WorkspaceID pgtype.UUID
    AccountID   pgtype.UUID
    ThreadID    pgtype.UUID
    TaskID      pgtype.UUID
    Role        string
}

type EdgeDefinition struct {
    Name        string
    Description string
    InputSchema  json.RawMessage
    Handler      func(ctx context.Context, toolCtx EdgeContext, input json.RawMessage) (EdgeResult, error)
}
```

每次 tool call 都必须：

1. 写 `agent_message(message_type='tool_call')`。
2. 写 `agent_event(tool_started)`。
3. 执行 handler。
4. 成功写 `tool_result` 和 `tool_succeeded`。
5. 失败写 `tool_result(error)` 和 `tool_failed`。
6. 通过 `/ws/agent` 推送。

### 9.1 基础工具

#### get_production_state

```json
{
  "workspace_id": "uuid"
}
```

Description:

> 获取当前 Agent Workspace 的生产状态摘要。用于 Producer 理解 workspace、素材、storyboard、节点、版本、任务、stale 和待处理决策。返回自然语言 PSS 和结构化摘要。

内部复用：

- workspace queries
- canvas read queries
- production read queries
- shot / agent_task / agent_event queries

#### request_user_decision

```json
{
  "workspace_id": "uuid",
  "title": "确认分镜方向",
  "summary": "我准备按 5 个分镜生成短视频。",
  "options": [
    {
      "id": "approve",
      "label": "确认并继续",
      "description": "进入预览图生成"
    }
  ],
  "blocking": true
}
```

Description:

> 请求用户做确认、选择或补充。该工具会创建 HITL 卡片并暂停当前 Producer turn，直到用户通过卡片或自然语言回复。

内部复用：

- `agent_event`
- `agent_message`
- `eino_checkpoint`（M6.2+）

#### update_storyboard

```json
{
  "workspace_id": "uuid",
  "intent": "replace",
  "shots": [
    {
      "client_key": "shot-01",
      "title": "开场钩子",
      "brief": "用商品主图和强卖点吸引注意",
      "duration_sec": 3,
      "narrative_purpose": "attention"
    }
  ],
  "dependencies": []
}
```

Description:

> 创建或修改 Agent Workspace 的 storyboard。该工具只写分镜语义和分镜依赖，不直接提交模型生成。

内部复用：

- shot queries
- shot_dependency queries
- 可选 `CreateAgentMediaNode` 创建画布投影
- `production.Service.MarkDownstreamStale` 标记受影响节点

### 9.2 生产工具

这些工具属于 M6 完整交付范围，但应在 runtime、HITL、Graph 和 storyboard 基础可用后接入。

- `generate_asset`
- `generate_shot_preview`
- `generate_shot_video`
- `select_version`
- `retry_generation`
- `compose_final`

内部复用：

- `CreateAgentMediaNode`
- `CreateMediaEdge`
- `UpdateMediaNodePrompt`
- `UpdateMediaNodeProductionConfig`
- `production.Service.SubmitNodeRun`
- `production.Service.SelectArtifactVersion`
- `production.Service.RetryJob`
- Sandbox Job Service

## 10. Eino Graph 编排

M6 以 Eino Graph 作为主编排层。Graph 节点负责显式状态转移；middleware 或 ADK Agent 可作为局部节点实现，但不能替代 Graph 对流程的控制。

### 10.1 ProducerGraph

首版 ProducerGraph：

```text
load_thread_and_messages
  -> build_producer_pss
  -> call_producer_model
  -> parse_model_output
  -> execute_or_schedule_tool
  -> persist_assistant_message
  -> emit_agent_events
```

职责：

- 处理用户消息。
- 读取 PSS。
- 调用 Producer 模型。
- 选择工具或输出自然语言回复。
- 调度 Craftsman / Worker / Composer task。
- 在需要用户决策时进入 HITL interrupt。

### 10.2 CraftsmanGraph

每个 shot 可以绑定一个 Craftsman thread。CraftsmanGraph 只处理 shot scoped context。

```text
load_shot_scoped_pss
  -> plan_generation_strategy
  -> create_or_update_preview_node
  -> dispatch_worker
  -> wait_worker_result
  -> dispatch_review
  -> revise_or_accept
```

职责：

- 为 shot 生成预览图策略。
- 生成视频策略。
- 根据 review critique 改写 prompt。
- 在重试上限内继续尝试。
- 把关键决策写回 agent_message 和 agent_event。

### 10.3 Worker Task

Worker 不需要长期对话上下文，首期作为确定性 task executor。

职责：

- 接收结构化 generation input。
- 调用 Agent production tool。
- 复用 `production.Service.SubmitNodeRun` 或 `SubmitGenerationIntent`。
- 等待 generation job/version 结果。
- 将结果事件写回 `agent_event`，唤醒上游 Graph。

### 10.4 ReviewGraph

ReviewGraph 对预览图或视频版本执行质量评审。

输入：

- shot brief
- current version
- rendered prompt
- provider request/response
- rubric

输出：

- rubric scores
- critique
- accept/reject
- suggested_revision

评审轴：

- `proportion`
- `physics`
- `style`
- `visual_quality`
- `product_visibility`
- `selling_power`
- `platform_fit`

### 10.5 ComposerGraph

ComposerGraph 读取已确认视频片段、音频、字幕和转场配置，生成最终成片。

职责：

- 校验所有 required shot video winner 已存在。
- 创建 final video node。
- 通过 Sandbox Job Service 执行 FFmpeg 合成。
- 写入 generation job / artifact version / sandbox job。
- 通过 HITL 请求用户确认成片或发起修改。

### 10.6 Middleware 使用边界

Middleware 仍然有用，但只承担横切能力：

- message persistence
- tool audit
- event emission
- PSS injection
- context summarization/reduction
- patch dangling tool calls after interrupt/resume

业务流程判断放在 Graph 节点和工程状态机中。

## 11. PSS 首版

Producer PSS 从以下事实构建：

- workspace name/mode
- workspace memory
- uploaded/source material nodes
- storyboard shots
- shot dependencies
- canvas nodes and dependency edges
- current versions
- latest jobs
- active stale reasons
- pending decisions
- running agent tasks

输出包含：

```text
当前项目：
这是一个 Agent Workspace，正在制作一条商品短视频。

用户素材：
- 产品主图：image node，source material，可作为输入。

Storyboard：
- 如果暂无分镜：当前还没有 storyboard。
- 如果已有分镜：[shot-01] 开场钩子，3 秒，状态 planned，目标是先吸引注意。

生产节点：
- image node「产品主图」是用户上传素材，可作为参考输入。
- text node「脚本文案」当前 winner 是 v2，最近一次 job succeeded。

待处理决策：
- decision-12 正在等待用户确认 storyboard 是否进入预览图生成。

正在运行：
- agent_task producer_turn-18 正在处理用户最新反馈。
```

PSS 每次 Agent turn 重新构建，不作为长期事实源保存。

## 12. 分阶段实施

### M6.1 Agent Runtime And Chat

交付：

- migrations: `agent_thread`、`agent_message`、`agent_task`、`agent_event`、`eino_checkpoint`。
- sqlc queries。
- Agent thread/message HTTP API。
- `/ws/agent` hub。
- Agent Workspace 前端右侧悬浮对话界面。
- 用户消息持久化和 ProducerGraph mock node 回复。

验收：

- 创建 Agent Workspace。
- 发送消息。
- 刷新页面后消息仍在。
- 另一个浏览器窗口能通过 WebSocket 收到新消息。
- 普通画布写接口在 Agent Workspace 仍返回 403。

### M6.2 ProducerGraph And HITL

交付：

- ProducerGraph 首版。
- PSS Graph node。
- model call Graph node。
- tool dispatch Graph node。
- `request_user_decision` 工具。
- `ui_card` message。
- decision resolve API。
- 前端决策卡片。
- Eino checkpoint interrupt/resume 接入。

验收：

- ProducerGraph 可从用户消息进入 PSS、模型调用和 assistant 回复。
- ProducerGraph 可生成确认卡片。
- 用户点击选项。
- 卡片状态变 resolved。
- Graph 可从 checkpoint 恢复。
- 事件和消息可恢复。

### M6.3 Edge Registry, Storyboard And PSS

交付：

- Edge registry。
- Edge call/result 落库。
- `get_production_state`。
- `update_storyboard`。
- `shot` / `shot_dependency`。
- PSS 包含 storyboard。
- 前端只读画布支持显示 shot/node 关联摘要。

验收：

- Producer 通过工具创建 storyboard。
- 刷新页面后 storyboard 仍在。
- PSS 能描述 shots。
- 工具调用可在对话中展开查看。

### M6.4 CraftsmanGraph And Preview Generation

交付：

- CraftsmanGraph。
- Worker task executor。
- `generate_shot_preview`。
- `generate_asset`。
- 复用 M4/M5 generation job/version/stale。
- 预览图生成后的 review task。

验收：

- Producer 可 dispatch Craftsman。
- Craftsman 可为 shot 创建或更新预览图节点。
- Worker 提交生成后产生 queued job/version。
- 预览图成功后 current winner 和只读画布更新。
- 失败 job 可追溯。

### M6.5 Video Generation, Review And Retry

交付：

- `generate_shot_video`。
- `select_version`。
- `retry_generation`。
- ReviewGraph。
- review rubric 和 critique 记录。
- Craftsman 根据 critique 自动改写，最多重试 3 次。
- 跨 shot 依赖调度，支持阻塞依赖等待上游完成。

验收：

- 预览图确认后可生成视频。
- review reject 时自动重试并记录原因。
- 用户说“第二个分镜重做”时复用该 shot 的 Craftsman thread。
- 有阻塞依赖的 shot 不会提前执行。
- 新版本成为 winner 后下游相关节点 stale。

### M6.6 ComposerGraph And Final Video

交付：

- ComposerGraph。
- `compose_final`。
- final video node。
- Sandbox Job Service FFmpeg 合成。
- 成片 HITL 确认卡片。

验收：

- 所有 shot video winner ready 后可合成成片。
- Composer 的 FFmpeg 命令只在 sandbox 内执行。
- 成片生成 generation job / artifact version / sandbox job。
- 用户可确认成片或要求修改。

## 13. 风险与取舍

### 13.1 Graph 过细导致迭代变慢

M6 需要 Graph 主编排，但首期 Graph 节点不能切得过碎。建议先按业务边界划分：Producer turn、HITL、storyboard、Craftsman、Worker、Review、Composer。节点内部可以先用普通 Go service 封装细节，等流程稳定后再拆更细。

### 13.2 工具太原子

如果暴露 create_node/create_edge/update_node，Agent 会变成画布脚本生成器。工具应保持生产语义。

### 13.3 前端只禁用编辑不够

前端禁用只是体验。后端必须继续拒绝普通用户在 Agent Workspace 的画布写接口。

### 13.4 HITL 与消息一致性

HITL 卡片、event、message、checkpoint 必须同事务或可恢复地写入，否则刷新后会出现“Agent 等待用户，但 UI 没卡片”的坏状态。

### 13.5 PSS 事实污染

PSS 必须从 DB 构建。LLM 可以写 memory revision，但不能直接改写当前任务状态。

## 14. 完成定义

M6 完成时：

- Agent Workspace 有真实右侧悬浮对话界面，底层画布只读。
- 消息、任务、事件、HITL 卡片可持久化并通过 WebSocket 同步。
- ProducerGraph 可完整处理用户消息、PSS、工具调用、HITL 和任务调度。
- Agent thread/message 持久化是通用能力，Producer、Craftsman、Reviewer、Composer 都可复用。
- Edge registry 暴露 `get_production_state`、`request_user_decision`、`update_storyboard`、`generate_asset`、`generate_shot_preview`、`generate_shot_video`、`select_version`、`retry_generation`、`compose_final`。
- Storyboard、shot_dependency、PSS、Memory 可持久化并可恢复。
- CraftsmanGraph 可为 shot 生成预览图和视频。
- ReviewGraph 可按 rubric 评审并触发自动改写重试。
- Worker task 复用 M4/M5 generation job / artifact version / stale / provider / sandbox 能力。
- ComposerGraph 可通过 Sandbox Job Service 生成成片。
- 普通用户仍不能直接编辑 Agent 画布。
- Studio / Agent 导入导出不属于本里程碑。
