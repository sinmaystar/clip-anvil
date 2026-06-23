# M6 Eino Native Checkpoint / Resume Refactor Design

**Status**: Draft for review
**Date**: 2026-06-23
**Milestone**: M6 MultiAgent Agent Mode

## Goal

把当前 M6 MultiAgent runtime 从“业务代码手写 checkpoint 快照”重构为“Eino 原生 `CheckPointStore` / interrupt / resume 管理 Graph 运行态，ClipAnvil DB 管理长期会话事实”的架构。

重构后，Producer、Craftsman、Reviewer、Composer 这些 Eino Graph 必须通过 Eino 原生 checkpoint/resume 机制保存和恢复执行态。Agent thread、message、task、event、production facts 仍由 ClipAnvil 自己持久化，不交给 Eino checkpoint 代替。

## Current Code State

当前代码已经有 MultiAgent runtime 的业务持久化：

- `agent_thread`: Producer / Craftsman / Reviewer / Composer 的长期线程身份。
- `agent_message`: 用户消息、assistant 消息、tool status、review card、decision card。
- `agent_task`: 每次执行任务，包括 `producer_turn`、`craftsman_turn`、`worker_generation`、`reviewer_turn`、`composer_turn`。
- `agent_event`: Agent 事件和 UI/生产同步事件。
- `eino_checkpoint`: checkpoint blob 表。

当前代码也已经有 Graph：

- `ProducerGraph`: `load_context -> draft_response -> finalize_response`
- `CraftsmanGraph`: `load_shot_context -> draft_generation_strategy`
- `ReviewerGraph`: `load_review_context -> review_artifact`
- `ComposerGraph`: `load_composition_context -> create_final_node -> submit_composition_intent -> persist_checkpoint_and_events`

但这些 Graph 当前 compile 时只传了 `compose.WithGraphName(...)`，没有传 `compose.WithCheckPointStore(...)`。例如：

```go
runnable, err := g.Compile(context.Background(), compose.WithGraphName("producer_turn"))
```

部分 Graph 会在业务节点内部调用 `Runtime.UpsertCheckpoint(...)`，例如 Craftsman 把生成策略写入 `eino_checkpoint`。这类数据是业务结果快照，不是 Eino 原生 checkpoint。它不能让 Eino 自动恢复 node input、channel、subgraph state、interrupt state。

## Key Distinction

Eino 原生 checkpoint 保存的是 Graph run 运行态：

- graph channels
- node inputs
- graph state
- subgraph checkpoint
- interrupt address
- interrupt state

Eino 原生 checkpoint 不负责长期会话存储：

- 不负责按 workspace/thread 加载历史消息。
- 不负责管理 `agent_thread`。
- 不负责管理 `agent_message`。
- 不负责长期 memory / summary。
- 不负责 provider-specific thinking history policy。
- 不负责生产事实，如 `generation_job`、`artifact_version`、`review_record`。

因此，目标架构必须保持双层持久化：

```text
ClipAnvil long-term state
  agent_thread / agent_message / agent_task / agent_event
  shot / media_node / generation_job / artifact_version / review_record
  -> used by ContextLoader and UI

Eino execution state
  compose.CheckPointStore backed by eino_checkpoint
  WithCheckPointID(task checkpoint key)
  StatefulInterrupt / ResumeWithData
  -> used by Graph runtime resume
```

## Design Principles

1. **Eino checkpoint owns execution recovery**
   A Graph interruption or process restart must be recoverable through Eino's checkpoint blob, not by replaying ad hoc business snapshots.

2. **ClipAnvil owns conversation memory**
   Historical user/assistant/tool messages remain in `agent_message`. ContextLoader reads and filters them for each model call.

3. **Task checkpoint key is stable**
   Each `agent_task` gets one deterministic checkpoint ID. Re-running or resuming the same task uses the same `WithCheckPointID`.

4. **Business checkpoint snapshots should become metadata, not resume source**
   Existing manually written checkpoint payloads can remain as audit metadata if useful, but they must not be the only resume mechanism.

5. **HITL should be a real Graph interrupt**
   `request_user_decision` should interrupt the ProducerGraph through Eino, persist the interrupt state, and resume with the user's decision data.

6. **Worker remains deterministic**
   Worker is not an Eino Graph in this stage. It remains a persistent `agent_task` executor that translates validated strategy into `GenerationIntent`.

## Target Architecture

### CheckPointStore Adapter

Create or refactor a store implementing Eino's interface:

```go
type CheckPointStore interface {
    Get(ctx context.Context, checkPointID string) ([]byte, bool, error)
    Set(ctx context.Context, checkPointID string, checkPoint []byte) error
}
```

It should be backed by `eino_checkpoint`:

- `key`: checkpoint ID.
- `workspace_id`: required for ClipAnvil ownership and cleanup.
- `thread_id`: optional but should be present for Agent task execution.
- `task_id`: optional but should be present for Agent task execution.
- `value`: raw Eino checkpoint blob.
- `metadata`: graph name, role, scope, version, updated reason.

The current `agent/hitl.CheckpointStore` is close but scoped around HITL. The target should be a general Agent Eino checkpoint store, with HITL using the same adapter.

### Checkpoint Scope

Use deterministic keys:

```text
agent:eino:<graph_name>:<workspace_id>:<thread_id>:<task_id>
```

Examples:

```text
agent:eino:producer_turn:<workspace>:<producer_thread>:<producer_task>
agent:eino:craftsman_generation:<workspace>:<craftsman_thread>:<craftsman_task>
agent:eino:reviewer_preview:<workspace>:<reviewer_thread>:<reviewer_task>
agent:eino:composer_final:<workspace>:<composer_thread>:<composer_task>
```

`agent_thread.current_checkpoint_key` should point at the latest active checkpoint for that thread. For shot-scoped Agents this means the latest task checkpoint for that shot's Craftsman or Reviewer thread.

### Graph Compile

Each Graph constructor should accept compile/runtime checkpoint options instead of hardcoding only graph name:

```go
type GraphConfig struct {
    ...
    CheckPointStore compose.CheckPointStore
    CompileCallbacks []compose.GraphCompileCallback
}
```

Each Graph compile should include:

```go
opts := []compose.GraphCompileOption{
    compose.WithGraphName("producer_turn"),
}
if config.CheckPointStore != nil {
    opts = append(opts, compose.WithCheckPointStore(config.CheckPointStore))
}
if len(config.CompileCallbacks) > 0 {
    opts = append(opts, compose.WithGraphCompileCallbacks(config.CompileCallbacks...))
}
runnable, err := g.Compile(context.Background(), opts...)
```

This keeps graph visualization and checkpointing extensible without coupling Graph code to server startup details.

### Graph Invocation

Executors should invoke Graphs with `compose.WithCheckPointID(...)`. The current `Runner` interface hides Eino call options:

```go
type Runner interface {
    Run(ctx context.Context, input GraphInput) (GraphOutput, error)
}
```

Refactor each Graph `Run` method to accept runtime options through a typed struct:

```go
type RunOptions struct {
    CheckPointID string
    ForceNewRun bool
    ResumeData map[string]any
}
```

Graph implementation maps those to Eino call options:

```go
opts := []compose.Option{}
if inputOptions.CheckPointID != "" {
    opts = append(opts, compose.WithCheckPointID(inputOptions.CheckPointID))
}
if inputOptions.ForceNewRun {
    opts = append(opts, compose.WithForceNewRun())
}
if len(inputOptions.ResumeData) > 0 {
    ctx = compose.BatchResumeWithData(ctx, inputOptions.ResumeData)
}
return g.runnable.Invoke(ctx, input, opts...)
```

Executors compute the checkpoint ID from task/thread/workspace and pass it into `Run`.

### HITL Interrupt / Resume

Current HITL creates a decision card and marks task `waiting_for_user`, but the ProducerGraph returns a normal output like "等待你的选择。". The target is:

1. `request_user_decision` tool detects it must pause.
2. Tool returns or raises an Eino `StatefulInterrupt` containing:
   - decision request ID
   - message/card ID if already persisted
   - tool call ID
   - allowed options
   - original tool arguments
   - any same-turn model/tool state needed to continue
3. Producer task is marked `waiting_for_user`.
4. `eino_checkpoint` contains the Eino checkpoint blob with interrupt state.
5. User clicks decision card or sends natural language response.
6. Server creates or reuses a `decision_resume` task linked to the original task.
7. ProducerGraph resumes with `compose.ResumeWithData(ctx, interruptID, decisionData)` or `BatchResumeWithData`.
8. The resumed Graph continues from the interrupted point rather than starting a new model turn from scratch.

The user-facing card/message storage remains in `agent_message`; Eino checkpoint only stores execution state required to resume the Graph.

### ToolNode Interaction

Producer currently uses Eino `ToolsNode` inside `draft_response`. For native Eino HITL, interrupt support should be implemented at the tool adapter layer:

- Normal tools return Eino tool result messages.
- HITL tools use Eino interrupt primitives and preserve tool call state.
- Tool execution events still persist to `agent_event`.
- Tool call/result UI remains driven by `agent_message` and WebSocket events.

This preserves current frontend protocol while making ProducerGraph resume real.

### GraphInfo Export

Eino supports graph compile callbacks via `compose.WithGraphCompileCallbacks(...)`. Add a lightweight compile callback that captures `compose.GraphInfo` and converts it to:

- JSON for exact inspection.
- Mermaid for human review.

Target output path for dev/test:

```text
docs/superpowers/graphs/
  producer_turn.mmd
  craftsman_generation.mmd
  reviewer_preview.mmd
  composer_final.mmd
  graph-info.json
```

Graph export should be deterministic and testable. It should not require the server to run.

## Per-Graph Refactor Scope

### ProducerGraph

Must use Eino checkpoint/resume.

Responsibilities:

- Load long-term context from `agent_message`, PSS, workspace facts.
- Stream model deltas.
- Execute native tool calls through `ToolsNode`.
- Persist tool status messages and task events.
- Interrupt on HITL.
- Resume same Graph run after user decision.

Producer's historical messages are not loaded from Eino checkpoint. They continue to be loaded by `RuntimeContextLoader`.

### CraftsmanGraph

Must use Eino checkpoint/resume.

Responsibilities:

- Load shot-scoped context.
- Ask model for strategy.
- Validate strategy.
- Persist strategy message/audit facts.
- Create Worker task.

If Craftsman ever introduces HITL or multi-step review of strategy, the Graph should use the same Eino interrupt/resume primitives. In the first refactor, it only needs checkpointed execution state and stable GraphInfo export.

### ReviewerGraph

Must use Eino checkpoint/resume.

Responsibilities:

- Load artifact/version context.
- Ask model for rubric review.
- Validate rubric.
- Write `review_record`.
- Select accepted version or dispatch retry.

Reviewer should checkpoint the review execution state, but durable review facts remain in `review_record`.

### ComposerGraph

Must use Eino checkpoint/resume.

Responsibilities:

- Load selected shot video facts.
- Create final video node.
- Submit `compose_final_video` production intent.
- Persist composition event/output.

Durable final video facts remain in `media_node`, `generation_job`, `artifact_version`.

### Worker

Worker remains outside Eino Graph in this refactor.

Reason:

- Worker performs deterministic side effects.
- It owns production submission retries and node creation.
- It is already persisted as `agent_task(role='worker')`.

Future work may wrap Worker as an Eino node only if it needs to participate in a larger graph's checkpointed state. That is not required for this refactor.

## Data Model Impact

No mandatory migration is expected because `eino_checkpoint` already exists and stores:

- key
- workspace_id
- thread_id
- task_id
- value
- metadata

Possible optional additions if implementation reveals a need:

- `metadata->graph_name`
- `metadata->checkpoint_version`
- `metadata->interrupt_ids`
- `metadata->source="eino_native"`

These can be stored in existing JSONB metadata.

## Execution Flow

### Normal Producer Turn

```text
user message persisted
-> producer_turn task created
-> checkpoint_id computed
-> ProducerGraph.Invoke(... WithCheckPointID(checkpoint_id))
-> Graph completes
-> assistant message persisted
-> task succeeded
-> thread.current_checkpoint_key updated
```

### HITL Producer Turn

```text
user message persisted
-> producer_turn task created
-> ProducerGraph.Invoke(... WithCheckPointID(checkpoint_id))
-> request_user_decision triggers StatefulInterrupt
-> Eino writes checkpoint blob
-> decision card persisted
-> task waiting_for_user
-> user decision persisted
-> decision_resume task created
-> ProducerGraph.Invoke(... WithCheckPointID(checkpoint_id), ResumeWithData(...))
-> Graph resumes after interrupt
-> assistant/tool result persisted
-> original or resume task succeeded
```

The implementation must choose whether the resumed execution marks the original task succeeded or records success on the `decision_resume` task while linking back to the original. The recommended choice is:

- original `producer_turn` remains the owner of the Graph run and final output;
- `decision_resume` is an audit task that records who/what resumed it;
- both are visible in task timeline.

## Failure Handling

### Checkpoint Write Failure

If Eino cannot write checkpoint:

- task must fail with a specific error code, such as `eino_checkpoint_write_failed`;
- user should see a recoverable assistant error;
- no HITL card should be shown unless the checkpoint exists.

### Resume Missing Checkpoint

If user responds to a decision but checkpoint is missing:

- do not start a fresh Producer turn silently;
- mark resume task failed with `eino_checkpoint_missing`;
- create assistant error explaining the task can no longer be resumed and should be retried.

### Resume Type Mismatch

If Graph schema changed and checkpoint cannot deserialize:

- mark task failed with `eino_checkpoint_incompatible`;
- log graph name, checkpoint key, serializer error;
- keep the checkpoint row for debugging;
- future migration can use Eino `MigrateCheckpointState`.

### Duplicate Resume

If a decision was already resolved:

- return idempotent success if the same decision is submitted again;
- reject conflicting decisions with `decision_already_resolved`;
- do not resume the Graph twice.

## Observability

Add structured logs for:

- graph name
- checkpoint key
- task id
- thread id
- interrupt id
- resume data shape, not full sensitive payload
- checkpoint read/write errors
- resume result

Add Agent events:

- `graph_checkpoint_written`
- `graph_interrupted`
- `graph_resumed`
- `graph_resume_failed`

These events are for debugging and timeline inspection. They should not replace task status.

## Testing Requirements

### Unit Tests

Add tests proving:

- Eino checkpoint store adapter writes and reads raw checkpoint bytes from `eino_checkpoint`.
- Graph compile uses `WithCheckPointStore`.
- ProducerGraph interrupted by a fake HITL tool can resume through Eino checkpoint.
- Resume without checkpoint fails with a structured error.
- Historical messages are still loaded from `agent_message`, not from checkpoint.
- GraphInfo export produces deterministic Mermaid containing expected node names.

### Integration Tests

Add tests for:

- producer task enters `waiting_for_user` after `request_user_decision`;
- decision response resumes the same checkpoint ID;
- resumed Producer produces final assistant output without replaying the whole turn as a new conversation;
- server restart recovery can resume queued/waiting tasks using DB state.

### Manual / E2E Acceptance

Browser E2E should validate:

1. Start Agent conversation that triggers `request_user_decision`.
2. Decision card appears.
3. Refresh browser.
4. Decision card is still interactive.
5. Choose an option.
6. Agent continues from the previous Graph run.
7. Task timeline shows interruption and resume.
8. `eino_checkpoint` row exists for the task.
9. Graph export files show Producer/Craftsman/Reviewer/Composer topology.

## Acceptance Criteria

- Producer, Craftsman, Reviewer and Composer compile with Eino native `CheckPointStore`.
- Executors invoke Graphs with deterministic checkpoint IDs.
- HITL uses Eino interrupt/resume for Producer, not only manual `waiting_for_user` state.
- `agent_message` remains the source of historical conversation context.
- `eino_checkpoint` contains Eino checkpoint blobs, not only business JSON snapshots.
- GraphInfo export exists and can print Mermaid for all four Graphs.
- Existing Worker production flow still works and remains outside Eino Graph.
- Existing Agent WebSocket UI behavior remains compatible.

## Out Of Scope

- Replacing `agent_message` with Eino memory.
- Replacing `agent_task` lifecycle with Eino runtime callbacks.
- Turning Worker into an Eino Graph.
- Moving Studio production APIs into Agent tools.
- Adding new Agent UI features beyond what is required to show real HITL resume state.
- Introducing Eino high-level ReAct Agent or MultiAgent host as the top-level runtime.

## Review Questions

1. Should `decision_resume` be a separate task, or should user decision directly resume the original `producer_turn` task?
2. Should manual business checkpoint rows be retained as audit snapshots after native Eino checkpoint is introduced, or should they be removed once GraphInfo and task output are sufficient?
3. Should GraphInfo Mermaid export be dev/test only, or should there be an authenticated debug API for runtime inspection?
