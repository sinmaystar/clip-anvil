# M6.5C Eino Native ToolNode Design

**Status**: Draft for review
**Date**: 2026-06-22
**Milestone**: M6 MultiAgent Agent Mode

## Goal

M6.5C replaces Producer's hand-written tool-call parsing and execution loop with Eino-native tool calling while keeping the existing custom Eino Graph and existing model provider path.

The current Producer model call already depends on Eino:

- `apps/server/internal/agent/producer/model_responder.go` creates Volcengine Ark models with `ark.NewChatModel`.
- It streams through `model.Stream(ctx, []*schema.Message)`.
- It concatenates stream chunks with `schema.ConcatMessages`.
- It reads Eino `schema.Message.Content` and `schema.Message.ReasoningContent`.

Therefore this phase does not rewrite model provider selection, thinking policy, streaming, or Volcengine Ark configuration. The core change is to stop asking the model to emit function calls as text and stop parsing `<|FunctionCallBegin|>` / JSON snippets in the normal path. Tool calls must come from Eino `schema.Message.ToolCalls` and be executed by Eino `compose.ToolsNode`.

## Current Problem

The current Graph already uses Eino composition:

```text
load_context -> draft_response -> finalize_response
```

But `draft_response` calls `runProducerLoop`, and the loop currently does this:

```text
Responder.Respond
-> ParseToolCall(out.AssistantText)
-> RegistryToolExecutor.ExecuteProducerTool
-> append same-turn assistant/tool messages
-> call model again
```

This has three structural problems:

1. Tool use depends on provider-specific or prompt-specific text formatting instead of model-native tool calls.
2. Tool arguments are less reliable because the model is not constrained by the Eino tool schema at inference time.
3. The implementation duplicates responsibilities Eino already provides: tool schema binding, tool-call extraction, tool execution dispatch, unknown-tool handling, and tool-call result messages.

The recent support for `<|FunctionCallBegin|>...<|FunctionCallEnd|>` is useful as a tactical compatibility patch, but it should not remain the primary Agent action protocol.

## Design Decision

Use **custom Eino Graph orchestration + Eino ToolCallingChatModel + Eino ToolsNode**.

Do not use `react.NewAgent` in this phase. The high-level ReAct Agent hides too much of the execution loop for ClipAnvil's needs:

- task/message/event persistence;
- WebSocket streaming and UI event timing;
- HITL interruption and resume;
- Eino checkpoint integration;
- max tool turn policy;
- per-tool audit records;
- later Producer / Craftsman / Worker / Composer graph branching.

The target shape is still a custom Graph:

```text
load_context
-> call_model
-> route_model_output
   -> finalize_response
   -> execute_tools
      -> persist_tool_results
      -> continue_model_loop
   -> interrupt_for_hitl
      -> persist_checkpoint
      -> finalize_waiting_for_user
```

The important change is inside the model/tool boundary:

```text
Eino ChatModel stream returns schema.Message
-> schema.Message.ToolCalls is inspected
-> compose.ToolsNode executes registered tools
-> tool result messages are appended to same-turn context
-> next model call receives assistant tool calls + tool results
```

## Scope

### In Scope

- Convert existing ClipAnvil `agenttools.Definition` into Eino `schema.ToolInfo`.
- Add an adapter from existing `agenttools.Executor` to Eino `tool.InvokableTool`.
- Bind all Producer-visible tools to the Eino Ark ChatModel call.
- Replace the normal `ParseToolCall` path with Eino `schema.Message.ToolCalls`.
- Execute tool calls through `compose.ToolsNode`.
- Preserve existing tool business implementations:
  - `read_workspace_context`;
  - `get_production_state`;
  - `update_storyboard`;
  - `request_user_decision`;
  - `create_agent_text_node`;
  - any already registered M6 tools.
- Preserve existing tool persistence semantics:
  - `agent_task(task_type='tool_call')`;
  - `agent_event(type='tool_call_started'/'tool_call_completed'/'tool_call_failed')`;
  - `agent_message(message_type='tool_call')`;
  - `agent_message(message_type='tool_result')`.
- Preserve model streaming to frontend:
  - `thinking` delta from `ReasoningContent`;
  - `markdown` delta from `Content`.
- Keep HITL behavior for `request_user_decision`: once triggered, the turn finishes as `waiting_for_user` / interrupted instead of continuing tool execution.
- Keep max tool calls configurable and defaulting to 50.
- Keep per-tool timeout defaulting to 300 seconds.
- Add structured logs for:
  - model tool-call count;
  - tool name;
  - tool call id;
  - argument validation failure;
  - Eino ToolsNode failure;
  - fallback parser usage.
- Add deterministic tests that do not depend on live model output.
- Add browser E2E for a real prompt that creates a 3-shot storyboard through native tool calling.

### Out of Scope

- Replacing `ark.NewChatModel` with `agenticark`.
- Migrating the whole message stack to Eino `AgenticMessage`.
- Using Eino `react.NewAgent`.
- Rewriting model selection, thinking depth policy, or Volcengine provider config.
- Implementing Craftsman / Worker / Reviewer / Composer.
- Implementing image/video/final composition tools.
- Removing the legacy text parser completely in this phase.

## Why Not AgenticMessage Now

Eino `AgenticMessage` is a richer protocol for Responses-style models and agentic providers. It can represent reasoning blocks, function tools, server tools, MCP tools, and structured content blocks.

However, Eino `compose.ToolsNode` currently consumes assistant `schema.Message` values with `ToolCalls`. The current ClipAnvil model path already uses `schema.Message` and the Ark ChatModel already implements Eino's tool-calling chat model interface.

For this phase, the lower-risk migration is:

```text
schema.Message + ToolCalls + ToolsNode
```

instead of:

```text
AgenticMessage + agenticark + new response protocol + new tool bridge
```

`AgenticMessage` should be evaluated in a later provider-specific phase if ClipAnvil needs Responses API semantics, richer reasoning passback, provider-managed tools, or MCP/server tools.

## Tool Adapter Design

### Existing Tool Definition

Current tools already expose:

```go
type Definition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Result      map[string]any
	Safety      SafetySpec
	Timeout     time.Duration
	Visibility  VisibilitySpec
}
```

This remains the single source of truth for tool name, description, parameters, result shape, safety, timeout, and UI visibility.

### Eino ToolInfo Conversion

Add a conversion layer:

```text
agenttools.Definition
-> schema.ToolInfo
```

Rules:

- `Definition.Name` maps to Eino tool name.
- `Definition.Description` maps to Eino tool description.
- `Definition.Parameters` maps to Eino parameter schema.
- `Definition.Result` is retained in ClipAnvil metadata and logs, but does not need to be forced into model prompt text unless Eino requires it.
- `Definition.Safety` remains ClipAnvil runtime policy, not model-facing authority.
- `Definition.Visibility` remains UI rendering policy.

The adapter must not duplicate descriptions in the system prompt. Tool descriptions should be supplied through Eino's tool binding so the provider receives actual tool schema.

### Eino InvokableTool Adapter

Add a Producer-scoped adapter around each existing `agenttools.Executor`.

Behavior:

```text
InvokableRun(ctx, argumentsJSON)
-> decode arguments JSON into map[string]any
-> build agenttools.ExecuteInput using current ProducerContext
-> call existing tool.Execute
-> encode ExecuteOutput.Result as JSON string
```

This preserves the existing service reuse boundary. Tools still call existing store/service code internally; Eino only becomes the calling protocol.

### Context Injection

Eino tools only receive JSON arguments. ClipAnvil tools also need workspace/thread/task context. The adapter should hold immutable per-turn context:

- `workspace_id`;
- `thread_id`;
- `producer_turn_task_id`;
- `tool_timeout`;
- runtime persistence handles;
- broadcaster;
- logger.

The adapter must be rebuilt per Producer turn so tool calls cannot accidentally use stale workspace or task IDs.

## Graph Execution Design

### Model Node

`call_model` should:

1. Build `[]*schema.Message` from:
   - system prompt;
   - persisted `agent_message` history;
   - same-turn assistant tool-call messages;
   - same-turn tool result messages;
   - PSS / production state context;
   - attachments.
2. Create or reuse the current Eino Ark ChatModel configuration.
3. Bind Producer tools to the model using Eino's tool-calling interface.
4. Stream the model response.
5. Emit frontend deltas for reasoning/content.
6. Return the final `*schema.Message` and model metadata.

The final message must retain:

- `Content`;
- `ReasoningContent`;
- `ToolCalls`;
- response metadata and usage;
- provider request id when available.

### Route Node

`route_model_output` branches:

- no tool calls and non-empty content: finalize assistant response;
- no tool calls and only reasoning: use the existing empty-content fallback policy;
- tool calls present: execute tools;
- tool calls present but `ToolExecutor` or registry is missing: fail with `agent_tool_executor_missing`;
- tool call count exceeds configured turn limit: fail with `agent_tool_loop_exhausted`.

### Tool Node

`execute_tools` should use Eino `compose.ToolsNode`.

The ToolsNode should receive the assistant message containing `ToolCalls`, not parsed text. It should return Eino tool result messages.

ClipAnvil must still persist tool events. There are two acceptable implementation patterns:

1. **Adapter-owned persistence**: each Eino tool adapter wraps the existing `RegistryToolExecutor` and returns the executor result JSON.
2. **ToolsNode middleware persistence**: Eino `ToolCallMiddlewares` handle started/completed/failed events around each tool call.

Use adapter-owned persistence first because it reuses the current `RegistryToolExecutor` behavior and avoids duplicating message/task/event semantics. Middleware can be introduced later if ClipAnvil needs cross-cutting instrumentation independent of the existing executor.

### Same-Turn Context

After ToolsNode returns result messages, Producer must append same-turn messages before the next model call:

```text
assistant message:
  role=assistant
  message_type=tool_call
  tool_calls=<provider/eino tool calls>
  reasoning_content=<optional>

tool message:
  role=tool
  message_type=tool_result
  tool_call_id=<same id>
  content=<JSON result>
```

These same-turn messages are used for the next model call without waiting for a full thread reload.

Persisted `agent_message` rows remain the durable UI/audit layer. Same-turn messages are the immediate model context layer.

## HITL Semantics

`request_user_decision` remains a tool.

When this tool executes successfully:

1. The tool creates the decision card / checkpoint through existing HITL service.
2. The Eino adapter returns a normal tool result JSON so persistence remains complete.
3. Producer detects `Definition.Safety.RequiresHITL`.
4. Producer stops the current loop and returns an interrupted output:

```json
{
  "interrupted": true,
  "tool_name": "request_user_decision"
}
```

The model must not continue after a HITL tool in the same turn. Later resume should reconstruct context from persisted messages and checkpoint data.

## Legacy Parser Policy

`ParseToolCall` should no longer be used in the normal path.

Keep it temporarily as a compatibility fallback only when all conditions are true:

1. Model returns no native `ToolCalls`.
2. Model returns content containing a recognized legacy tool-call envelope.
3. Backend config enables fallback parsing.

When fallback parsing is used, log a warning with:

- workspace id;
- thread id;
- task id;
- model id;
- parser format;
- tool name;
- message length.

Fallback usage is a bug signal, not the intended architecture.

## Provider and Model Compatibility

This phase assumes Volcengine Ark ChatModel supports Eino tool calling through the existing Eino Ark extension. If a selected model does not support tool calling, Producer should fail before the model call with a clear error:

```text
agent_model_tool_calling_unsupported
```

The model selection metadata should eventually include tool-calling capability. Until that is represented in the database, the Volcengine provider adapter may maintain a conservative allowlist or perform a startup/runtime capability check.

Switching between mini and pro models inside one thread is supported as long as both models consume the same Eino message representation. Historical assistant/tool messages must be rebuilt into provider-valid `schema.Message` values each turn. Reasoning content should follow the provider-specific thinking passback policy already established for Volcengine and must not be blindly sent back when the provider says not to.

## Error Handling

### Model Errors

Keep the existing model failure diagnostics:

- `create_model`;
- `stream_start`;
- `stream_receive`;
- `stream_concat`;
- empty content with reasoning.

Add tool-call related fields when applicable:

- `native_tool_call_count`;
- `tool_names`;
- `tool_call_ids`;
- `model_supports_tool_calling`;
- `tool_binding_count`.

### Tool Errors

Tool failures must still create:

- `tool_call_failed` event;
- failed tool task;
- `tool_result` message with error content;
- structured slog entry.

The Producer turn should fail unless the tool explicitly returns a recoverable result. Silent tool failure is not allowed.

### Unknown Tool

Unknown native tool calls should be handled by Eino ToolsNode unknown-tool handling and mapped to a ClipAnvil error:

```text
agent_tool_not_found
```

This must be persisted and logged with the requested tool name.

## Deliverables

- Producer model path confirmed as existing Eino Ark ChatModel path.
- New Eino tool adapter package or file near Producer/tool execution code.
- Producer Graph revised to route native `schema.Message.ToolCalls`.
- Eino `compose.ToolsNode` used for normal tool execution.
- Existing tool registry and tool business logic reused.
- Existing tool-call persistence semantics preserved.
- Legacy parser gated as fallback, not default.
- Deterministic tests for native tool-call path.
- Browser E2E showing a storyboard prompt creates durable shots through native tool calls.

## Acceptance Criteria

### Functional

- A prompt like "把一个 15 秒口播种草短视频拆成 3 个分镜" causes the model/tool path to call `update_storyboard` natively.
- The assistant UI does not display raw `<|FunctionCallBegin|>` text or raw JSON tool-call envelopes.
- The database contains 3 `shot` rows after successful execution.
- `agent_message` contains visible tool call/result records for `update_storyboard`.
- A follow-up `get_production_state` call returns the created storyboard in PSS.
- If `request_user_decision` is called, the turn stops and waits for the user.
- If the model does not produce tool calls, normal text-only conversation still works.

### Technical

- Current Eino Ark ChatModel model invocation remains in place.
- No high-level `react.NewAgent` is introduced.
- Normal path does not call `ParseToolCall`.
- Eino `compose.ToolsNode` is used to execute native tool calls.
- Tool definitions come from the existing ClipAnvil Tool Registry.
- Existing tool implementations are reused without duplicating production/canvas services.
- Tool call IDs are stable across assistant tool call, persisted task/event/message, and tool result.
- Tool loop limit defaults to 50.
- Tool execution timeout defaults to 300 seconds.

### Observability

- Logs include whether native tool calling or fallback parsing was used.
- Logs include model id, tool binding count, native tool-call count, tool names, and provider request id when available.
- Tool failures include the underlying error message and tool call id.
- Empty model content after reasoning still uses the existing diagnostic path.

## Required Verification Commands

Run backend tests for Producer and Agent tools:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer ./internal/agent/tools ./internal/agent/storyboard ./internal/agent/pss -count=1
```

Run full server tests:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Run server build:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Run frontend build if message rendering or E2E helpers change:

```bash
pnpm --filter @clip-anvil/web... build
```

Run lint when frontend files change:

```bash
pnpm --filter @clip-anvil/web lint
```

Run diff whitespace check:

```bash
git diff --check
```

Run browser E2E against the script-started dev server:

```bash
./scripts/dev-start.sh
```

Then in the browser:

1. Open the Vite URL printed by `dev-start.sh`.
2. Create or open an Agent workspace.
3. Send: `我想要创作一个15s请把一个 15 秒的口播种草短视频拆成 3 个分镜`.
4. Confirm the UI shows tool activity instead of raw function-call text.
5. Confirm the assistant returns a human-readable response.
6. Confirm DB state contains 3 shots for the workspace.
7. Confirm logs show native Eino tool calls, not fallback parser usage.

Stop local app processes after verification:

```bash
./scripts/dev-stop.sh
```

## Implementation Notes For Later Plan

The implementation plan should split work into these tasks:

1. Add tests proving current model path can return native Eino `ToolCalls` through a fake streamer.
2. Add `agenttools` to Eino ToolInfo conversion tests.
3. Add an Eino `InvokableTool` adapter around the existing registry executor.
4. Refactor Producer responder output so it can return the final `schema.Message`, not only assistant text.
5. Replace `runProducerLoop` normal path with native tool call routing and ToolsNode execution.
6. Keep `ParseToolCall` behind an explicit fallback gate with warning logs.
7. Add integration tests for `update_storyboard` and `get_production_state`.
8. Run browser E2E and inspect logs/DB.
