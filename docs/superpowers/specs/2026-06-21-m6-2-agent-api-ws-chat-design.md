# M6.2 Agent API / WebSocket / Right Floating Chat 设计方案

**状态**：待评审
**日期**：2026-06-21
**所属里程碑**：M6 MultiAgent Agent Mode
**阶段目标**：在 M6.1 通用 Agent runtime 之上交付可用的 Agent 对话壳：后端对话 API、`/ws/agent` 实时同步、前端右侧悬浮 Producer 对话框，并保持 Agent Workspace 画布只读。

## 1. 背景

M6.1 已建立 `agent_thread`、`agent_message`、`agent_task`、`agent_event`、`eino_checkpoint` 以及最小 runtime service。M6.2 的职责是把这些持久化能力暴露给产品入口，让用户可以在 Agent Workspace 里打开 Producer 对话、发送消息、刷新后恢复消息、并让多个浏览器标签通过 WebSocket 同步。

M6.2 不实现 ProducerGraph、Eino Graph 执行、工具调用、HITL、Storyboard/PSS 或生产生成。它只把“对话事实源”和“实时同步通道”打通。

## 2. 设计结论

采用 **API + WebSocket + UI shell** 的窄实现。

M6.2 只做：

```text
Agent Workspace
  -> GET producer thread
  -> GET persisted messages
  -> POST user message
  -> persist agent_message(role=user)
  -> persist agent_event(type=message_created, status=pending)
  -> broadcast /ws/agent event
  -> frontend right-floating chat renders messages
```

M6.2 不自动创建 assistant message，也不创建 `producer_turn` task。这样 M6.3 可以用同一条用户消息和 pending `message_created` event 作为 ProducerGraph 的确定入口，不需要拆掉一个临时 mock reply。

这是对早期总计划里“临时 Producer 回复”想法的收缩：M6.2 用持久化 status/event 替代临时 assistant 回复；真正 Producer 回复从 M6.3 开始由 ProducerGraph 产生。

## 3. 范围

### 3.1 包含

- 后端 Agent HTTP handler。
- 后端 Agent WebSocket handler。
- 后端 AgentHub，用 workspace 维度广播 Agent 事件。
- DTO 转换层，输出稳定 JSON。
- Agent Workspace 权限和模式校验：
  - 必须登录。
  - 必须拥有 workspace。
  - workspace 必须是 `mode='agent'`。
- 前端 `agentApi`。
- 前端 `agentWs`。
- 前端右侧悬浮 Producer 对话框。
- 发送文本消息、显示消息列表、空状态、发送中、错误、WS 连接状态、折叠状态。
- 刷新后恢复历史消息。
- WS reconnect 后通过 REST 重新拉取消息。
- 回归保证：普通 canvas 写接口在 Agent Workspace 仍返回 `403`。

### 3.2 不包含

- ProducerGraph。
- 自动 assistant 回复。
- `producer_turn` task 创建。
- Eino Graph checkpoint/resume。
- Tool registry。
- HITL 决策卡。
- Storyboard/PSS。
- Craftsman / Worker / Reviewer / Composer。
- 自动生成预览图、视频或成片。

## 4. 后端 API

所有 API 都挂在 `/api/agent/workspaces/:workspaceID` 下，并使用现有 JWT middleware。

### 4.1 GET thread

```http
GET /api/agent/workspaces/:workspaceID/thread
```

行为：

- 校验 workspace 存在、归属当前 account、且 `mode='agent'`。
- 调用 `runtime.Service.GetOrCreateProducerThread`。
- 返回 active workspace scoped Producer thread。

响应：

```json
{
  "thread": {
    "id": "uuid",
    "workspace_id": "uuid",
    "role": "producer",
    "scope_type": "workspace",
    "scope_id": null,
    "runtime_provider": "eino",
    "runtime_agent_name": "",
    "current_checkpoint_key": null,
    "status": "active",
    "summary": "",
    "created_at": "2026-06-21T10:00:00Z",
    "updated_at": "2026-06-21T10:00:00Z"
  }
}
```

### 4.2 GET messages

```http
GET /api/agent/workspaces/:workspaceID/messages?after_seq=0&limit=50
```

行为：

- 同样校验 Agent Workspace。
- 获取或创建 Producer thread。
- 调用 `runtime.Service.ListMessages(threadID, afterSeq, limit)`。
- `limit` 后端限制为 `1..200`，默认 `50`。

响应：

```json
{
  "thread": { "id": "uuid", "role": "producer" },
  "messages": [
    {
      "id": "uuid",
      "workspace_id": "uuid",
      "thread_id": "uuid",
      "seq": 1,
      "role": "user",
      "message_type": "text",
      "content": { "text": "请帮我做一个咖啡广告" },
      "raw_message": {},
      "task_id": null,
      "event_id": null,
      "created_at": "2026-06-21T10:01:00Z"
    }
  ]
}
```

### 4.3 POST message

```http
POST /api/agent/workspaces/:workspaceID/messages
Content-Type: application/json
```

请求：

```json
{
  "text": "请帮我做一个咖啡广告",
  "client_message_id": "optional-client-id"
}
```

验证：

- `text` trim 后不能为空。
- `text` 最大 8000 字符。
- `client_message_id` 可选，最大 128 字符，仅用于客户端去重和调试。

行为：

- 获取或创建 Producer thread。
- 调用 `runtime.Service.AppendMessage` 写入：
  - `role='user'`
  - `message_type='text'`
  - `content={"text": "...", "client_message_id": "..."}`
  - `raw_message={}`
- 调用 `runtime.Service.CreateEvent` 写入：
  - `event_type='message_created'`
  - `source_role='user'`
  - `target_role='producer'`
  - `scope={"thread_id":"..."}`
  - `payload={"message_id":"...","seq":1}`
  - `status='pending'`
- 通过 AgentHub 广播 `agent.message.created`。

响应：

```json
{
  "message": {
    "id": "uuid",
    "seq": 1,
    "role": "user",
    "message_type": "text",
    "content": { "text": "请帮我做一个咖啡广告" }
  },
  "event": {
    "id": "uuid",
    "event_type": "message_created",
    "status": "pending"
  }
}
```

## 5. WebSocket

### 5.1 Endpoint

```http
GET /ws/agent?workspaceId=<uuid>&token=<jwt>
```

行为与 `/ws/canvas` 保持一致：

- query 里传 `workspaceId` 和 `token`。
- 连接前校验 token、workspace owner、workspace mode。
- 只接受 Agent Workspace；Studio Workspace 返回 forbidden。
- 当前阶段客户端不需要向 socket 写业务消息；服务端读循环只用于感知断开。

### 5.2 Event shape

```ts
type AgentSocketEvent =
  | {
      type: "agent.message.created";
      payload: {
        workspace_id: string;
        thread_id: string;
        message: AgentMessage;
        event: AgentEvent;
      };
    }
  | {
      type: "agent.event.created";
      payload: {
        workspace_id: string;
        thread_id?: string;
        event: AgentEvent;
      };
    };
```

M6.2 只要求广播 `agent.message.created`。`agent.event.created` 在 M6.3/M6.4 给 task、tool、HITL 事件复用。

### 5.3 Reconnect

前端 WebSocket reconnect 成功后不依赖服务端补发历史消息，而是调用：

```http
GET /api/agent/workspaces/:workspaceID/messages?after_seq=<lastSeenSeq>&limit=200
```

这样恢复逻辑只依赖 DB 事实源，不依赖 AgentHub 是否保存离线队列。

## 6. 前端交互

### 6.1 页面布局

`AgentWorkspacePage` 调整为：

```text
agent-workspace-shell
  agent-topbar
  agent-canvas-stage
    agent-readonly-canvas
    agent-chat-float (right, above canvas)
```

桌面端：

- Canvas 区域占满剩余视口。
- Producer 对话框固定在右侧，宽度约 `380px..440px`。
- 对话框悬浮在 canvas 上方，有明确边框和阴影。
- 对话框可以折叠为右侧小按钮；折叠后不遮挡 canvas。

移动端：

- Producer 对话框变成底部抽屉或全宽浮层。
- 输入框和发送按钮不可溢出。
- 折叠按钮仍可恢复对话框。

### 6.2 对话体验

- 初次进入时加载 thread 和 message。
- 空消息时显示短空状态：“还没有 Producer 对话”。
- 发送按钮只在非空文本、非发送中、已加载 workspace 时可用。
- Enter 发送；Shift+Enter 换行。
- 发送成功后清空输入并追加消息。
- 发送失败保留输入，显示错误文本。
- WS 收到同一 message 时按 `message.id` 去重。
- 消息按 `seq` 排序。
- `role=user` 右对齐；后续 `role=assistant/system/tool` 左对齐或中性样式。
- 当前没有 assistant 回复时，不显示假回复；可以显示轻量状态：“ProducerGraph 将在 M6.3 接入”。

### 6.3 只读 canvas

M6.2 不重建 Studio tldraw 编辑器。Agent 页面继续使用当前只读节点概览，但视觉结构从左侧 chat + 右侧 canvas 改成 canvas 底层 + 右侧浮层 chat。

普通用户对 Agent Workspace 的节点、边、分组、相机写操作仍由后端返回 `403`；前端不暴露编辑控件。

## 7. 后端实现边界

新增 package 依赖方向：

```text
cmd/server
  -> api.AgentHandler
  -> agent/runtime.Service
  -> store/db

cmd/server
  -> api.AgentWSHandler
  -> api.AgentHub
```

M6.2 不允许：

- `api.AgentHandler` 调用 Eino。
- `api.AgentHandler` 调用 production service。
- `api.AgentHandler` 创建 `producer_turn` task。
- WebSocket handler 直接写 DB。

## 8. DTO 规则

后端不要直接把 sqlc row 暴露给前端。需要新增转换函数：

- `toAgentThreadResponse(db.AgentThread)`
- `toAgentMessageResponse(db.AgentMessage)`
- `toAgentEventResponse(db.AgentEvent)`

JSONB 字段必须输出 object：

- 空值输出 `{}`。
- 非法 JSON 不应 panic；返回 `{}` 并保留服务端日志即可。

UUID 空值：

- nullable UUID 输出 `null`。
- valid UUID 输出字符串。

## 9. 验收标准

### 9.1 后端验收

- Agent Workspace 调用 `GET thread` 会创建或返回 active Producer thread。
- Studio Workspace 调用 Agent API 返回 `403`。
- 非 owner 调用 Agent API 返回 `403`。
- `POST message` 会持久化 user text message。
- `POST message` 会创建 pending `message_created` event。
- `POST message` 会广播 `agent.message.created` 给同 workspace 的 WS 连接。
- `GET messages` 按 `seq` 升序返回消息，支持 `after_seq`。
- `/ws/agent` 校验 token 和 workspace owner。
- AgentHub register/unregister/broadcast 无连接时不 panic。
- 现有 Agent Workspace canvas write guard 仍生效。

### 9.2 前端验收

- 进入 Agent Workspace 后右侧浮层 Producer 对话框显示在 canvas 之上。
- 输入文本并发送后，消息立即出现在列表中。
- 刷新页面后，历史消息仍显示。
- 第二个浏览器标签收到第一标签发送的消息。
- WebSocket 重连后会重新拉取缺失消息。
- 折叠对话框后 canvas 仍可阅读。
- 移动端没有按钮文字溢出或 UI 重叠。

### 9.3 严格验收命令

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
make server-lint
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web test:connections
git diff --check
```

如果实现过程中修改 migration 或 sqlc query，还必须运行：

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

## 10. 风险与约束

- WebSocket 事件不是事实源；DB message 是事实源。
- M6.2 的 pending `message_created` event 会在 M6.3 变成 ProducerGraph 消费入口，因此不能在 M6.2 自动标记 handled。
- 前端不要把发送成功等同于 Producer 已处理；发送成功只表示消息已持久化。
- 右侧浮层不能重新引入可编辑 canvas 能力。
- 如果 M6.1 runtime service 缺少某个列表方法，可以在 M6.2 小幅补 runtime wrapper，但不要新增表。

## 11. 完成定义

M6.2 完成时，用户能在 Agent Workspace 右侧 Producer 对话框发送消息、刷新恢复、跨标签实时同步；后端已有稳定 API/WS 契约供 M6.3 ProducerGraph 接入。系统仍不具备智能回复、工具调用和生产生成能力，这些从 M6.3 起逐步实现。
