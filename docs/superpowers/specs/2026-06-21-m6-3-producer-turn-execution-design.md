# M6.3 Producer Turn Execution 设计方案

**状态**：待评审
**日期**：2026-06-21
**所属里程碑**：M6 MultiAgent Agent Mode
**阶段目标**：在 M6.1 Agent runtime 和 M6.2 Agent 对话通道之上，交付首个可执行 ProducerGraph：用户消息进入 `producer_turn` task，后端执行 Eino Graph，生成 assistant 回复，持久化并通过 `/ws/agent` 同步到前端。

## 1. 背景

M6.2 已完成 Agent Workspace 右侧悬浮 Producer 对话框、用户消息持久化、`/ws/agent` 同步和刷新恢复。但当前系统还不能真正与 Agent 对话：用户消息只停留在 `agent_message(role=user)` 和 `agent_event(type=message_created,status=pending)`，没有进入 Agent 执行流程，也不会产生 assistant 回复。

M6.3 的目标是把“通道和事实源”升级为“最小可执行 Agent 回合”：

```text
POST user message
  -> persist user agent_message
  -> create producer_turn agent_task
  -> run ProducerGraph
  -> persist assistant agent_message
  -> mark task succeeded or failed
  -> emit task/message events
  -> /ws/agent broadcasts updates
```

M6.3 仍然不是完整 MultiAgent 生产阶段。它不做 Storyboard/PSS、工具调用、HITL、Craftsman、Worker、Composer、预览图、视频或成片生成。它的核心价值是确认后续所有 Agent 能力都会复用同一条 task/message/event/checkpoint 执行链路。

## 2. 设计结论

采用 **Eino Graph 主编排 + 后端内置 executor + deterministic Producer responder**。

首版 ProducerGraph 使用 Eino `compose.NewGraph` 建立真实 Graph 骨架，但节点先保持极窄：

```text
ProducerGraph
  load_context
  -> draft_response
  -> finalize_response
```

- `load_context` 从当前 Producer thread 读取最近消息，构造本轮上下文。
- `draft_response` 调用 `ProducerResponder`。M6.3 默认实现是 deterministic responder，保证本地测试、CI 和浏览器 e2e 不依赖外部 LLM。
- `finalize_response` 生成 assistant message content 和 task output。

这样 M6.4/M6.5 可以在同一个 Graph 上替换或扩展节点：

- deterministic responder -> real ChatModel responder。
- load_context -> PSS builder。
- draft_response -> tool planning。
- finalize_response -> HITL / tool dispatch / Craftsman dispatch。

## 3. 范围

### 3.1 包含

- `producer_turn` task 创建。
- 后端 Producer executor。
- Eino ProducerGraph 首版。
- deterministic Producer responder。
- task 状态转换：
  - `queued -> running -> succeeded`
  - `queued/running -> failed`
- assistant message 持久化。
- task 和 message WebSocket 事件广播。
- 前端对话面板展示 assistant 回复。
- 前端发送后展示执行中状态。
- 刷新后恢复 user + assistant 完整对话。
- 单元测试覆盖 Graph、executor、API task 创建和前端状态合并。
- 浏览器 e2e 覆盖真实 UI 发送后自动出现 assistant 回复。

### 3.2 不包含

- 真实 LLM API 接入。
- Eino checkpoint store 接入和 resume。
- HITL interrupt/resume。
- Edge registry。
- Edge call message 和 tool result message。
- Storyboard/PSS schema。
- Craftsman / Worker / Reviewer / Composer。
- 调用 M4/M5 production service 生成文本、图像、视频或成片。
- 自动 retry 策略。

## 4. 后端执行模型

### 4.1 POST message 行为调整

M6.2 的 `POST /api/agent/workspaces/:workspaceID/messages` 只写用户消息和 pending event。M6.3 调整为：

1. 校验 Agent Workspace。
2. 获取 Producer thread。
3. 写入 user message。
4. 创建 `agent_task`：

```json
{
  "role": "producer",
  "scope_type": "workspace",
  "task_type": "producer_turn",
  "max_attempts": 1,
  "input": {
    "trigger_message_id": "uuid",
    "trigger_message_seq": 12
  }
}
```

5. 创建 `agent_event(type='producer_turn_queued')`。
6. 广播：
   - `agent.message.created`
   - `agent.task.updated`
   - `agent.event.created`
7. 异步提交 task 给 executor。
8. HTTP 仍立即返回 user message、event 和 task，不等待 assistant 回复。

这样发送接口保持低延迟，后续真实 LLM 或工具调用变慢也不会阻塞 HTTP 请求。

### 4.2 Producer executor

新增 `internal/agent/producer.Executor`，负责执行单个 `producer_turn` task。

输入：

- `workspace_id`
- `thread_id`
- `task_id`
- `trigger_message_id`

执行：

```text
MarkTaskRunning
CreateEvent(producer_turn_started)
ProducerGraph.Invoke
AppendMessage(role=assistant,message_type=text,task_id=task_id)
CreateEvent(producer_turn_completed)
MarkTaskSucceeded
Broadcast assistant message and task status
```

失败：

```text
MarkTaskRunning
CreateEvent(producer_turn_started)
ProducerGraph.Invoke returns error
AppendMessage(role=assistant,message_type=error,task_id=task_id)
CreateEvent(producer_turn_failed)
MarkTaskFailed
Broadcast error message and failed task
```

M6.3 不实现后台队列和多 worker 扫描。首版 executor 由 POST message 后 `go executor.RunTask(...)` 触发。为避免进程重启时 queued task 永久卡住，服务启动时查询最近 queued `producer_turn` 并补跑一次，限制数量为 50。

### 4.3 Eino ProducerGraph

Graph 输入：

```go
type ProducerTurnInput struct {
	WorkspaceID      pgtype.UUID
	ThreadID         pgtype.UUID
	TaskID           pgtype.UUID
	TriggerMessageID pgtype.UUID
}
```

Graph 输出：

```go
type ProducerTurnOutput struct {
	AssistantText string
	Metadata      map[string]any
}
```

Graph 节点：

- `load_context`：读取最近 20 条 thread messages，并定位 trigger user message。
- `draft_response`：调用 `ProducerResponder.Respond(ctx, ProducerContext)`。
- `finalize_response`：trim assistant text，空文本视为错误。

默认 deterministic response：

```text
我已收到你的需求：「<latest user text>」。
下一步我会先整理创作目标，再在后续阶段拆成分镜和生产任务。
```

这不是产品最终文案，只是 M6.3 的可测试 Agent 回复。真实模型和工具决策从后续阶段替换。

## 5. WebSocket 事件

M6.2 已有 `agent.message.created`。M6.3 扩展事件类型：

```ts
type AgentSocketEvent =
  | {
      type: "agent.message.created";
      payload: {
        workspace_id: string;
        thread_id: string;
        message: AgentMessage;
        event?: AgentEvent;
      };
    }
  | {
      type: "agent.task.updated";
      payload: {
        workspace_id: string;
        thread_id?: string | null;
        task: AgentTask;
      };
    }
  | {
      type: "agent.event.created";
      payload: {
        workspace_id: string;
        thread_id?: string | null;
        task_id?: string | null;
        event: AgentEvent;
      };
    };
```

前端必须允许未知 event type，不应断开连接或抛错。后续 HITL 和工具调用会继续扩展事件。

## 6. 前端行为

### 6.1 对话展示

现有 `AgentWorkspacePage` 已渲染 `role=user` message。M6.3 增加：

- `role=assistant` 左侧或中性色消息气泡。
- `message_type=error` 显示错误样式。
- task running 时显示“Producer 正在思考”状态行。
- 第二标签通过 websocket 收到 assistant message 后自动显示。
- 刷新后通过 `GET messages` 恢复 user + assistant。

### 6.2 发送体验

发送按钮行为保持不变。POST 成功后：

- user message 立即出现。
- 若 response 里包含 queued task，显示执行中。
- assistant 回复通过 websocket 或后续 REST reload 进入消息列表。

如果 websocket 断开，前端 reconnect 后继续按 M6.2 的 `after_seq` 拉取 message，task 状态不作为消息恢复的唯一事实源。

## 7. 错误处理

- Graph 编译失败：服务启动失败，避免运行到半可用状态。
- task 执行失败：写入 `agent_task.status='failed'`，写入 assistant error message，广播 task 和 message。
- deterministic responder 返回空文本：按 `producer_empty_response` 失败处理。
- trigger message 不存在或不是 user message：task failed，error code `producer_invalid_trigger_message`。
- workspace 非 Agent mode：沿用 M6.2 的 forbidden 行为，不创建 task。

## 8. 可交付标准

M6.3 完成时必须满足：

- 用户在 Agent 对话框发送消息后，不只看到自己的消息，还能自动看到 Producer assistant 回复。
- 数据库中存在：
  - user `agent_message`
  - `agent_task(task_type='producer_turn')`
  - assistant `agent_message(task_id=<producer_turn>)`
  - `producer_turn_queued/started/completed` events
- `agent_task` 最终为 `succeeded`，失败路径能落 `failed`。
- 前端刷新后仍显示完整对话。
- 第二浏览器标签能通过 websocket 收到 assistant 回复。
- 不依赖真实 LLM key。
- 不破坏 Agent Workspace 只读画布策略。

## 9. 验收测试标准

### 9.1 自动化命令

必须全部通过：

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
make server-lint
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web test:connections
git diff --check
```

### 9.2 浏览器 e2e

必须使用 `./scripts/dev-start.sh` 启动当前 worktree，并使用脚本输出的 Vite URL。

验收步骤：

1. 注册或登录测试账号。
2. 创建 Agent Workspace。
3. 打开第二个同 workspace 标签页。
4. 在第一个标签发送 `M6.3 e2e producer turn`。
5. 第一个标签看到 user message 和 assistant reply。
6. 第二个标签自动看到 assistant reply。
7. 刷新第一个标签，完整对话仍存在。
8. 查询数据库确认 `producer_turn` task 为 `succeeded`，assistant message 绑定 task。

### 9.3 数据库抽查

使用 compose Postgres 容器执行：

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select task_type,status from agent_task order by created_at desc limit 5;"
```

预期最近一条 `producer_turn` 为 `succeeded`。

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select role,message_type,task_id is not null as has_task from agent_message order by created_at desc limit 5;"
```

预期能看到 `assistant/text/true`。

## 10. 后续阶段接口

M6.3 留给后续阶段的明确接点：

- M6.4：把 `ProducerResponder` 替换为真实 ChatModel responder，并接入 HITL interrupt/resume。
- M6.5：在 `load_context` 中加入 PSS builder，在 `draft_response` 后加入 tool planning。
- M6.6：`producer_turn` 可创建 Craftsman/Worker tasks。
- M6.7：失败 task 进入 review rubric 和 retry graph。
- M6.8：ComposerGraph 复用同一 executor/task/event/message 模式。
