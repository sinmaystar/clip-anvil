# Agent 子线程观察器设计 Spec

## 1. 背景

当前 Agent 对话框已经收敛为“只展示 Producer thread”。这是正确边界：Producer 是唯一直接面向用户的主 Agent，负责解释状态、请求决策和统领全局。

但 MultiAgent 重构后，Craftsman、Reviewer 都是持久化、有状态的 Agent。它们有自己的 `agent_thread`、`agent_task`、`agent_message` 和工具调用历史。用户在排查或审阅制作过程时，会自然想知道：

- Producer 派发了哪些 Craftsman / Reviewer？
- 每个子 Agent 当前是否 queued、running、failed、succeeded？
- 某个分镜的 Craftsman 到底写了什么 prompt、调用了哪些工具、为什么失败？
- Reviewer 的 10 轴评审过程和最终建议是什么？

目前主对话里曾出现过把 Craftsman 工具调用嵌套到 `dispatch_craftsman` 工具卡片里的趋势。这会让 Producer 主对话越来越重，也会混淆“主对话决策记录”和“子 Agent 执行轨迹”。因此需要一个专门的 **Agent 子线程观察器**：主对话保持干净，子 Agent 线程可实时只读查看。

## 2. 目标

- 在 Agent 对话区域中提供所有持久化子 Agent 的入口。
- 用户点击某个 Craftsman / Reviewer 后，能看到该 Agent 的只读完整对话和工具调用历史。
- 子 Agent 的新增消息能通过 `/ws/agent` 实时进入对应线程视图。
- Producer 主对话继续只展示 Producer thread，不再混入 Craftsman / Reviewer 原始消息。
- `dispatch_craftsman` / `dispatch_reviewer` 工具卡片只展示派发摘要和子 Agent 入口，不再嵌套渲染子 Agent 的完整工具调用。
- 刷新页面后，线程列表、选中线程和历史消息能从数据库恢复。

## 3. 非目标

- 不允许用户直接在子 Agent 线程里输入消息；第一版只读。
- 不改变 Producer / Craftsman / Reviewer 的角色分工。
- 不让 Craftsman / Reviewer 的消息参与 Producer 主对话渲染。
- 不新增业务事实表；第一版复用 `agent_thread`、`agent_task`、`agent_message`、`agent_event`。
- 不做用户级未读状态持久化。第一版可以做前端会话级 unread badge；后续需要跨设备已读再加 read cursor 表。
- 不把子 Agent 线程抽屉做成画布节点详情的替代品。画布详情看生产对象，线程抽屉看 Agent 执行过程。

## 4. 当前代码事实

- 后端已有 `agent_thread`，并且 sqlc 已有 `ListAgentThreadsByWorkspace`。
- 后端已有 `agent_message`，并且 `ListAgentMessagesByThread` 可按 thread 拉消息。
- `/ws/agent` 的 `agent.message.created`、`agent.message.updated`、`agent.message.delta`、`agent.task.updated`、`agent.event.created` payload 已带 `thread_id`。
- 前端 `AgentWorkspacePage` 已通过 `isProducerThreadMessage` 过滤主消息流。
- 当前 `/api/agent/workspaces/:workspaceID/messages` 默认只拉 Producer thread 消息。
- `dispatch_craftsman` 会创建或复用 scope-aware Craftsman thread，并向 Craftsman thread 写入 delegation user message。
- `dispatch_reviewer` 会创建或复用 Reviewer thread，并向 Reviewer thread 写入 delegation user message。

这些事实说明第一版不需要新 WebSocket 通道，也不需要改消息表结构。缺口主要是：

1. 缺少“列出可观察 Agent threads”的 API。
2. 缺少“按指定 thread 拉历史消息”的 API。
3. 前端缺少按 thread 分流和缓存子线程消息的状态模型。
4. dispatch 工具卡片还没有稳定的“跳转到子 Agent”入口形态。

## 5. 目标交互：B + C 方案

### 5.1 页面结构

Agent 页面右侧对话区域拆成三层：

```text
┌─────────────────────────────────────────────┐
│ Producer 主对话                              │
│ - 只展示 Producer thread                     │
│ - 用户只在这里输入                           │
│ - HITL 决策卡仍在这里                        │
├─────────────────────────────────────────────┤
│ Agents 观察栏                                │
│ Craftsman shot_01 · running · 2m ago         │
│ Craftsman shot_02 · queued                   │
│ Reviewer shot_03 · failed                    │
└─────────────────────────────────────────────┘

点击某个 Agent：

┌──────────────────── Producer 主对话 ────────┬────────────── 子线程抽屉 ──────────────┐
│ 用户和 Producer 的主线对话                  │ Craftsman shot_01 · 只读               │
│ dispatch_craftsman 工具卡片只显示摘要       │ Producer 派发 Craftsman 任务            │
│ Producer 最终解释和下一步                   │ Craftsman assistant / tool_call / result │
│ 输入框仍只发给 Producer                     │ 错误、system-reminder、最终总结          │
└─────────────────────────────────────────────┴────────────────────────────────────────┘
```

### 5.2 Agents 观察栏

位置：Producer 对话框右侧或对话框内部右侧窄栏，桌面端常驻，窄屏可折叠。

每个 Agent item 展示：

| 字段 | 示例 | 来源 |
|---|---|---|
| 角色 | `Craftsman` / `Reviewer` | `agent_thread.role` |
| scope | `shot_01` / `key_element_state: multi_color` / `render_plan` | `agent_thread.scope_type/scope_id` + 领域对象查询 |
| 状态 | `active` / `failed` | `agent_thread.status` |
| 最新任务状态 | `queued` / `running` / `succeeded` / `failed` | 最新 `agent_task` |
| 最近消息时间 | `17:04:36` | 最新 `agent_message.created_at` |
| 摘要 | `预览图 RenderPlan 已创建` | thread summary 或最新 assistant/tool summary |

排序建议：

1. active running / queued 在前。
2. failed 其次。
3. 最近有消息更新的在前。
4. role 分组可选：Craftsman、Reviewer、Composer。

### 5.3 子线程抽屉

点击 Agent item 后打开只读抽屉：

- 标题：`Craftsman · shot_01 产品质感亮相`
- 副标题：`thread_id`、`scope`、最新 task 状态。
- 消息列表：复用 `AgentMessageRenderer`，但禁用决策操作和输入框。
- 支持 tool call / tool result 展开。
- 支持 system-reminder 完整展示，但空文本不渲染空气泡。
- 支持加载更多历史消息。
- 支持跟随滚动；如果用户向上翻看历史，新消息只显示 badge，不强制跳底。

抽屉中不提供“给 Craftsman 发送消息”的输入框。用户如果想干预，应回到 Producer 主输入框，对 Producer 说“让 shot_01 的 Craftsman 重做低机位版本”。

### 5.4 dispatch 工具卡片

Producer 主对话里的 `dispatch_craftsman` / `dispatch_reviewer` 工具卡片改为：

- 展示工具调用状态：running / succeeded / failed。
- 展示派发摘要：目标阶段、派发数量、execution policy。
- 展示子 Agent chips：
  - `Craftsman shot_01`
  - `Craftsman shot_02`
  - `Reviewer shot_03`
- 点击 chip 等价于在 Agents 观察栏选择该 thread。

不再把 Craftsman / Reviewer 的工具调用消息嵌套到 Producer 的 dispatch 工具卡片内部。完整执行过程只在子线程抽屉里看。

## 6. 后端设计

### 6.1 API：列出 Agent threads

新增：

```http
GET /api/agent/workspaces/:workspaceID/threads
```

返回：

```json
{
  "threads": [
    {
      "id": "uuid",
      "workspace_id": "uuid",
      "role": "craftsman",
      "scope_type": "shot",
      "scope_id": "uuid",
      "status": "active",
      "runtime_provider": "eino",
      "runtime_agent_name": "CraftsmanGraph",
      "summary": "",
      "display_name": "Craftsman · shot_01 产品质感亮相",
      "scope_label": "shot_01",
      "scope_title": "产品质感亮相",
      "latest_task": {
        "id": "uuid",
        "task_type": "craftsman_turn",
        "status": "running",
        "created_at": "2026-06-26T09:04:36Z",
        "completed_at": null
      },
      "latest_message_at": "2026-06-26T09:04:40Z",
      "latest_message_preview": "已为分镜创建预览图 RenderPlan",
      "created_at": "2026-06-26T09:03:11Z",
      "updated_at": "2026-06-26T09:04:40Z"
    }
  ]
}
```

实现要点：

- 必须校验 workspace 属于当前账号，且 `workspace.mode = agent`。
- 默认返回 Producer 之外的持久化 Agent：`role IN ('craftsman', 'reviewer', 'composer')`。
- 如果后续需要调试，可用 query 参数 `include_producer=true`。
- `scope_label` 应尽量使用稳定业务标识，例如 shot `client_key`。
- `latest_task` 使用该 thread 最新一条 `agent_task`。
- `latest_message_preview` 从最新可展示消息提取纯文本摘要，避免直接返回大 JSON。

### 6.2 API：读取指定 thread 消息

新增：

```http
GET /api/agent/workspaces/:workspaceID/threads/:threadID/messages?after_created_at=&after_seq=&limit=1000
```

返回：

```json
{
  "thread": { "...": "agent thread response + display fields" },
  "messages": [
    { "...": "agent message response" }
  ]
}
```

实现要点：

- 必须校验 thread 属于 workspace。
- thread 不存在或不属于 workspace 返回 404。
- 默认不允许跨 workspace 查询。
- 支持 `after_seq` 更适合单 thread 增量拉取；`after_created_at` 可作为前端现有模式兼容。
- 返回内容使用现有 `toAgentMessageResponse`。
- 子线程消息允许包含 `role=user` 的 delegation message，因为它代表 Producer 给子 Agent 的委派，不是终端用户输入。

### 6.3 WebSocket 事件

第一版继续复用现有 `/ws/agent`：

- `agent.message.created`
- `agent.message.updated`
- `agent.message.delta`
- `agent.task.updated`
- `agent.event.created`

后端不需要新通道，但需要保证 Craftsman / Reviewer executor 持久化消息后都会广播。当前代码已有 broadcaster 接口，实施时应补测试确认三类 Agent 都会广播：

- Producer：主对话实时显示。
- Craftsman：观察栏状态更新，已打开抽屉实时追加。
- Reviewer：观察栏状态更新，已打开抽屉实时追加。

如果未来事件量过大，再考虑订阅 thread filter。第一版 workspace 级广播足够，前端按 `thread_id` 分流。

## 7. 前端设计

### 7.1 状态模型

新增前端状态：

```ts
interface AgentThreadListItem {
  id: string;
  role: "craftsman" | "reviewer" | "composer";
  scope_type: string;
  scope_id?: string | null;
  display_name: string;
  scope_label: string;
  scope_title?: string;
  status: string;
  latest_task?: AgentTask;
  latest_message_at?: string;
  latest_message_preview?: string;
}

interface AgentThreadMessageCache {
  [threadID: string]: {
    messages: AgentMessage[];
    lastLoadedAt?: string;
    lastSeq?: number;
    hasLoadedInitial: boolean;
  };
}
```

Producer 主消息仍用现有 `messages` state。子线程消息使用独立 cache，避免 `mergeAgentMessages` 把不同 thread 混进主流。

### 7.2 WebSocket 分流

收到 `agent.message.created/updated`：

1. 如果 `thread_id === producerThreadID`，进入 Producer 主消息流。
2. 否则进入 `subThreadMessageCache[thread_id]`。
3. 同步更新 thread item 的 `latest_message_at` 和 preview。
4. 如果 thread 不在列表中，触发 `threads` query refetch。

收到 `agent.message.delta`：

- 如果是 Producer thread，进入现有 streaming state。
- 如果是已打开的子 Agent thread，进入子线程 streaming state。
- 如果不是已打开的 thread，只更新 thread item running badge，不渲染完整 delta；等用户打开时从 DB 拉历史消息即可。

收到 `agent.task.updated`：

- 根据 `thread_id` 更新对应 Agent item 的 latest task。
- 如果是 Producer task，继续影响主输入框是否禁用。
- Craftsman / Reviewer / Worker 长任务不应锁死 Producer 主输入框。

### 7.3 组件建议

新增组件：

- `AgentThreadObserverPanel`
  - 渲染 Agents 观察栏。
  - 支持 role/status filter 的轻量 UI。
  - 点击 item 选择 thread。

- `AgentThreadDrawer`
  - 渲染只读子线程。
  - 复用 `AgentMessageRenderer`。
  - 禁用 `AgentDecisionActions`。
  - 支持空态、加载态、失败态。

- `AgentThreadListItem`
  - 角色、scope、任务状态、时间、preview。

- `AgentThreadLinkChip`
  - 用于 dispatch 工具卡片里的子 Agent 快捷入口。

### 7.4 视觉原则

- 观察栏要轻，不抢主对话注意力。
- running/queued/failed 用状态点和短标签表达，不做大卡片堆叠。
- 子线程抽屉可以信息密度更高，因为用户已经主动进入审计模式。
- 子线程消息气泡需要和主对话有视觉区分，例如抽屉标题和只读标识，但消息组件本身尽量复用。
- 不在主对话里重复渲染同一条子 Agent 消息，避免“刷新后少一条、实时多一条”的错觉。

## 8. 数据和权限边界

- `agent_thread` 是线程列表事实源。
- `agent_message` 是线程历史事实源。
- `agent_task` 是任务状态事实源。
- `agent_event` 是 HITL、dispatch、ready 等事件事实源。
- 前端 thread cache 只是展示缓存，不是事实源。
- 所有 API 必须通过 workspace/account 鉴权。
- 子 Agent 线程只读，不暴露 POST message API。
- Producer 对话输入框仍只创建 Producer thread 的 user message。

## 9. 与 Producer Pending Signal 的关系

Pending Signal 是 Producer 必须消费的工程事件队列；子线程观察器是用户查看子 Agent 执行过程的 UI。

二者不能混用：

- Producer 是否继续处理 RenderPlan ready，看 `producer_pending_signal`。
- 用户是否能看到 Craftsman 生成了什么，看 `agent_thread + agent_message`。
- 子线程消息不应该被当作 Producer 待办队列 drain。
- system-reminder 可以出现在 Producer thread，也可以作为子线程历史展示；但它不是 UI 里的任务调度源。

## 10. 里程碑

### M1：只读线程列表和抽屉

#### 落地事项

1. 后端新增 threads list API。
2. 后端新增 thread messages API。
3. 前端新增 Agent 观察栏。
4. 前端新增只读线程抽屉。
5. 前端 WebSocket 按 thread 分流消息和 task。

#### 可交付标准

- dispatch 多个 Craftsman 后，观察栏能出现多个 Craftsman item。
- 点击任意 Craftsman，抽屉能展示 delegation message、assistant message、tool call、tool result。
- Reviewer thread 也能以同一套 UI 展示。
- 刷新后观察栏能恢复，点击线程能从 DB 拉历史消息。
- 子 Agent 消息不会进入 Producer 主对话。

#### 验收标准

以行李箱广告 workspace 为例：

1. Producer 派发 4 个 preview image Craftsman。
2. 主对话只出现 Producer 用户消息、Producer assistant、Producer dispatch 工具卡片。
3. 右侧 Agents 观察栏出现 4 个 Craftsman。
4. 任意打开一个 Craftsman，可看到“Producer 派发 Craftsman 任务。”以及该 Craftsman 后续工具调用。
5. Craftsman 新增 tool result 时，如果抽屉打开，消息实时追加。
6. `pnpm --filter @clip-anvil/web... build` 和 `pnpm --filter @clip-anvil/web lint` 通过。
7. 如果改后端 API，`make server-test` 通过。

### M2：dispatch 工具卡片入口化

#### 落地事项

1. `dispatch_craftsman` 工具结果返回可被前端识别的 `craftsman_thread_id` 列表。
2. `dispatch_reviewer` 工具结果返回 `reviewer_thread_id`。
3. `AgentToolStatusBlock` 对 dispatch 工具渲染 thread chips。
4. 移除或关闭主对话中基于 `parent_tool_call_id` 的子 Agent 嵌套排序展示。

#### 可交付标准

- dispatch 工具卡片可以点击跳转到对应子 Agent 抽屉。
- dispatch 工具卡片不再展开显示子 Agent 完整工具调用。
- `parent_tool_call_id` 可以继续保存在 raw_message 里用于审计，但不驱动主对话嵌套 UI。

#### 验收标准

1. Producer dispatch 4 个 Craftsman。
2. dispatch 工具卡片展示 4 个 thread chip。
3. 点击 chip 打开对应 Craftsman 抽屉。
4. Producer 主对话中不会出现 Craftsman 的 `upsert_render_plan`、`read_project_memory` 等工具卡片。

### M3：体验 polish 和诊断能力

#### 落地事项

1. 观察栏支持按 role/status 筛选。
2. 观察栏显示前端会话级 unread badge。
3. 抽屉支持复制 thread id / task id。
4. 抽屉支持跳转到关联画布对象，例如 shot、render_plan、review_record。
5. 对失败线程展示错误摘要和“让 Producer 处理这个问题”的引导文案，但仍不直接给子 Agent 发消息。

#### 可交付标准

- 用户能从失败的 Reviewer 或 Craftsman 快速定位到对应 shot/render_plan。
- 用户能回到 Producer 主输入框发起干预。
- 子线程观察器成为调试和审阅 MultiAgent 过程的稳定入口。

## 11. 测试计划

### 后端单测

- threads list API 只返回当前 workspace 的子 Agent threads。
- thread messages API 拒绝跨 workspace thread。
- thread messages API 能返回 delegation user message、assistant、tool_call、tool_result。
- response 中 `display_name`、`scope_label`、`latest_task`、`latest_message_preview` 正确。

### 前端单测

- WebSocket message created 按 `thread_id` 分流，Producer 消息进入主流，子 Agent 消息进入 cache。
- `agent.task.updated` 不会因为 Craftsman running 禁用 Producer 输入框。
- `AgentThreadObserverPanel` 正确渲染 running/failed/succeeded。
- `AgentThreadDrawer` 只读渲染消息，不出现输入框或 HITL 操作按钮。
- dispatch 工具卡片能渲染 thread chips。

### E2E

- 浏览器进入 Agent workspace。
- 发送用户消息触发 Producer dispatch 多个 Craftsman。
- 验证主对话只显示 Producer thread。
- 验证右侧观察栏实时出现 Craftsman。
- 打开一个 Craftsman 抽屉，等待其 tool call / tool result 实时追加。
- 刷新页面后重新打开同一线程，历史消息仍在。
- 查询数据库确认消息分别属于 Producer thread 和 Craftsman thread，没有被错误写入同一 thread。

## 12. 风险和约束

- 如果子 Agent 高频流式输出很多 delta，workspace 级广播可能增加前端负担。第一版只对已打开子线程渲染 delta，未打开线程只更新状态。
- 如果 thread list API 每次都 join 太多领域表，可能变慢。第一版可以先只补 shot/key_element_state/render_plan 的 label，后续再优化。
- 如果 `parent_tool_call_id` 仍参与主消息排序，可能继续导致嵌套显示。M2 必须明确改掉主对话嵌套渲染逻辑。
- 子线程只读会让用户少一个直接干预入口，这是有意设计。所有干预都应回到 Producer，否则全局一致性会被破坏。

## 13. 开放问题

1. 观察栏在桌面端是固定在对话框右侧，还是作为对话框内部的可折叠窄栏，需要前端实现时结合现有布局宽度确定。
2. 是否需要在 URL 上表达选中的子线程，例如 `?agentThread=...`，方便刷新恢复和分享排查链接。
3. 是否要在 Agent Canvas 的 Shot 详情里也放一个“查看 Craftsman 线程”入口。建议 M1 先只在对话区实现，M3 再和画布详情打通。

