# Producer Pending Signal 设计

## 背景

当前 Craftsman 并发完成 RenderPlan 后，会向 Producer thread 写入 `craftsman_render_plan_ready` 的系统生成 user message，并尝试唤醒 Producer。问题是如果当时已有 Producer task 处于 `queued` 或 `running`，唤醒逻辑会直接返回，不会再创建新的 Producer task。后续 ready message 虽然留在 `agent_message`，但正在运行的 Producer 不会自动读取运行期间新追加的 message，导致部分 RenderPlan 进入 `waiting_for_approval` 后无人处理，直到用户再次发消息触发 Producer。

设计目标是一步到位引入 Producer Pending Signal。`agent_message` 继续作为对话和上下文历史，Pending Signal 作为 Producer 必须消费的工程事件队列，避免把对话消息当任务队列使用。

## 目标

- Craftsman、Worker、Reviewer、Composer 等系统事件可以可靠唤醒 Producer。
- 多个 Craftsman 并发 ready 时，Producer 不会丢事件，也不会重复处理同一个 signal。
- Producer running 期间新增的 signal 必须在本轮结束前被发现并继续处理，或者在本轮结束后自动排队下一轮 Producer task。
- Producer 的 prompt 能看到待处理 signal 的自然语言摘要，但是否处理、如何处理仍由 Producer 决策并调用工具完成。
- Signal 的状态、归属、消费结果可追踪，便于排查“为什么 Producer 没继续执行”。

## 非目标

- 不把 `agent_message` 当作 pending queue 来 drain。
- 不做轻量版“有 Producer running 就再创建一个 task”的临时方案。
- 不改变主 Agent 对话框只展示 Producer thread 的口径。
- 不让 Craftsman 直接执行 Producer 审批逻辑；Craftsman 只发出 signal。

## 数据表

新增 `producer_pending_signal` 表。

| 字段 | 类型 | 可空 | 含义 |
|---|---|---:|---|
| `id` | `uuid` | 否 | Signal 主键 |
| `workspace_id` | `uuid` | 否 | 所属 workspace |
| `producer_thread_id` | `uuid` | 否 | 目标 Producer thread |
| `source_role` | `text` | 否 | 事件来源，取值如 `craftsman`、`worker`、`reviewer`、`composer`、`system` |
| `source_task_id` | `uuid` | 是 | 产生 signal 的任务 |
| `source_thread_id` | `uuid` | 是 | 来源 Agent thread |
| `signal_type` | `text` | 否 | 类型，如 `craftsman_render_plan_ready`、`worker_generation_completed` |
| `scope_type` | `text` | 否 | 业务范围，如 `workspace`、`shot`、`render_plan`、`final_output` |
| `scope_id` | `uuid` | 是 | 业务范围 ID |
| `render_plan_id` | `uuid` | 是 | 相关 RenderPlan；Craftsman ready 场景必须填 |
| `message_id` | `uuid` | 是 | 对应写入 Producer thread 的 system-reminder message |
| `status` | `text` | 否 | `pending`、`claimed`、`processed`、`ignored`、`failed` |
| `priority` | `integer` | 否 | 数字越小越优先，默认 100 |
| `dedupe_key` | `text` | 否 | 幂等键 |
| `payload` | `jsonb` | 否 | 结构化上下文，如 target_phase、shot_client_key、craftsman_output 摘要 |
| `claimed_by_task_id` | `uuid` | 是 | 当前认领 signal 的 Producer task |
| `claimed_at` | `timestamptz` | 是 | 认领时间 |
| `processed_by_task_id` | `uuid` | 是 | 最终处理 signal 的 Producer task |
| `processed_at` | `timestamptz` | 是 | 处理时间 |
| `last_error` | `text` | 是 | 处理失败原因 |
| `created_at` | `timestamptz` | 否 | 创建时间 |
| `updated_at` | `timestamptz` | 否 | 更新时间 |

约束：

- `status` 只能是 `pending`、`claimed`、`processed`、`ignored`、`failed`。
- `signal_type = 'craftsman_render_plan_ready'` 时，`render_plan_id` 必须非空。
- `(workspace_id, dedupe_key)` 唯一，避免同一个 RenderPlan ready 重复入队。
- 索引：
  - `(workspace_id, status, priority, created_at)`
  - `(producer_thread_id, status, priority, created_at)`
  - `(render_plan_id)`
  - `(source_task_id)`

`dedupe_key` 规则：

- Craftsman RenderPlan ready：`craftsman_render_plan_ready:<render_plan_id>`
- Worker 完成：`worker_generation_completed:<generation_job_id>`
- Reviewer 完成：`reviewer_result_ready:<review_record_id>`

## 写入流程

Craftsman 完成 RenderPlan 后：

1. Craftsman 持久化自己的 assistant/tool 消息到 Craftsman thread。
2. 工程代码写入 Producer thread 的 system-reminder message，用于 Producer 上下文和审计。
3. 工程代码调用 `CreateProducerPendingSignal`：
   - `signal_type = craftsman_render_plan_ready`
   - `scope_type = shot`
   - `scope_id = shot_id`
   - `render_plan_id = render_plan.id`
   - `message_id = producer system-reminder message id`
   - `status = pending`
   - `dedupe_key = craftsman_render_plan_ready:<render_plan_id>`
4. 工程代码调用 `EnsureProducerWakeTask(workspace_id, producer_thread_id)`。

`EnsureProducerWakeTask` 行为：

- 如果没有 Producer `queued/running/decision_resume running`，创建一个 `producer_turn` task。
- 如果已有 Producer active，不再创建重复 task，但 signal 已经在表里，不会丢。
- 如果只有 Producer `waiting_for_user`，不自动创建 task，signal 保持 pending；用户决策 resume 后由 Producer drain。

## Producer 消费流程

Producer task 启动时：

1. 加载 Producer thread messages。
2. 调用 `ClaimProducerPendingSignals(workspace_id, task_id, limit)`。
3. 使用 `FOR UPDATE SKIP LOCKED` 找到 `pending` 或过期 `claimed` signal，按 `priority, created_at` 排序。
4. 将 signal 标记为 `claimed`，写入 `claimed_by_task_id` 和 `claimed_at`。
5. 把 claimed signals 编译成自然语言 system reminder，注入 prompt，例如：

```text
<system-reminder>
你有 3 个待处理 Producer signal。
1. craftsman_render_plan_ready: shot_02 / render_plan=...
2. craftsman_render_plan_ready: shot_04 / render_plan=...
3. worker_generation_completed: shot_01 / job=...
请先读取项目上下文，然后按业务优先级处理这些 signal。处理 RenderPlan ready 时，应调用 decide_render_plan accept/reject 或派 Reviewer。
</system-reminder>
```

Producer ReAct loop 中：

- 每次工具结果 append 后、下一次 model 前，执行一次 `ClaimProducerPendingSignals`。
- 如果有新增 signal，追加新的 system reminder，让 Producer 在同一个 task 里继续处理。
- Producer finalize 前再检查一次 pending signal：
  - 如果没有，正常结束。
  - 如果有且本 task 尚未超过 max turns，继续进入下一轮 model。
  - 如果达到 max turns，调用 `EnsureProducerWakeTask` 创建下一轮 Producer task。

Signal 完成规则：

- Producer 成功调用 `decide_render_plan` accept/reject 后，将对应 `craftsman_render_plan_ready` signal 标记为 `processed`。
- 如果 Producer 明确决定不处理某 signal，标记为 `ignored`，必须写入 reason。
- 如果工具调用失败但可重试，signal 保持 `claimed`，task 失败或结束时释放回 `pending`。
- 如果 signal 对应对象已经不存在或状态已终态，标记为 `ignored`，reason 写清楚。

## 释放和恢复

需要处理进程崩溃或 Producer task 中断：

- `claimed` 超过租约时间，例如 10 分钟，视为 stale，可以被新的 Producer task 重新 claim。
- Producer task `failed/cancelled` 时，将其 `claimed_by_task_id = task_id` 且未处理的 signal 释放回 `pending`。
- 应用重启时不自动恢复 queued task 的开发期设置不影响 pending signal。下一次用户消息、HITL resume、或显式调度触发 Producer 时会重新 drain pending signal。

## Agent 消息关系

- Producer thread message 是对话上下文和用户可见主线。
- Craftsman/Reviewer thread message 是子 Agent 历史和调试轨迹，不渲染到主 Agent 对话框。
- `producer_pending_signal.message_id` 只引用 Producer thread 中的 system-reminder message，用于审计和 prompt 可追溯。
- Producer 判断和执行结果仍通过工具调用、tool result、assistant text 持久化到 Producer thread。

## 工具与服务边界

新增 runtime/service 能力：

- `CreateProducerPendingSignal(ctx, params)`
- `ClaimProducerPendingSignals(ctx, workspaceID, taskID, limit)`
- `MarkProducerPendingSignalProcessed(ctx, signalID, taskID)`
- `MarkProducerPendingSignalIgnored(ctx, signalID, taskID, reason)`
- `ReleaseProducerPendingSignalsForTask(ctx, taskID)`
- `ListPendingProducerSignals(ctx, workspaceID)`

Producer 工具实现需要在完成关键业务动作后回写 signal 状态：

- `decide_render_plan`：处理 `craftsman_render_plan_ready`
- 后续 `review_shot` / `compose_final` / worker completion 相关工具按 signal_type 扩展

## 并发语义

- 多个 Craftsman 可并发写 signal。
- 同一 `dedupe_key` 只允许一条 active signal。
- 多个 Producer task 理论上不应并发运行；即使出现并发，`FOR UPDATE SKIP LOCKED` 也保证同一 signal 只被一个 task claim。
- Producer 每轮 drain 不是 drain message，而是 claim signal。
- Signal 处理是幂等的：如果 RenderPlan 已经不是 `waiting_for_approval`，Producer 或工具应把 signal 标为 `ignored/processed`，不要重复提交 worker。

## 画布和前端影响

- 主 Agent 对话框只展示 Producer thread messages。
- 子 Agent 消息可在后续节点详情/任务详情里展示，不依赖本 spec 实施。
- Pending signal 不直接渲染成用户气泡；如果需要展示，应作为“系统进度/待处理事项”进入任务日志或画布详情。
- Producer 处理 signal 后产生的工具调用和 assistant 总结仍会出现在主对话框。

## 可交付标准

- 新增 migration、sqlc query、runtime service 方法。
- Craftsman ready 写入 pending signal，并保留 Producer thread system-reminder message。
- Producer 启动、工具循环中、finalize 前都能 claim/drain pending signal。
- `decide_render_plan` 成功后会标记对应 signal processed。
- 主 Agent 对话框不会展示 Craftsman/Reviewer/Worker thread messages。
- 有单元测试覆盖 signal 创建、去重、claim、释放、processed 标记。
- 有 E2E 覆盖 5 个 Craftsman 并发 ready，Producer 最终处理全部 5 个 RenderPlan，不依赖用户再次发消息。

## 验收标准

以行李箱广告 5 分镜为验收场景：

1. Producer 派发 5 个 Craftsman 并发创建预览图 RenderPlan。
2. 5 个 Craftsman 完成后，DB 中出现 5 条 `craftsman_render_plan_ready` signal。
3. 即使前 3 个 ready 触发的 Producer 正在运行，后 2 个 signal 也保持 pending 或被后续同一 Producer task claim，不会丢。
4. 最终 5 个 RenderPlan 都被 Producer accept/reject，signal 全部进入 `processed` 或 `ignored` 终态。
5. 刷新 Agent 页面后，主对话框只展示 Producer thread 的用户消息、Producer assistant、Producer tool call/tool result、HITL card，不展示 “Producer 派发 Craftsman 任务。” 或 Craftsman assistant 原始总结。
