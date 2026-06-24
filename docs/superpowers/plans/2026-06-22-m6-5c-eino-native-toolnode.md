# M6.5C Eino Native EdgeNode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Producer's hand-written tool-call parsing path with Eino-native `schema.Message.EdgeCalls` and `compose.EdgesNode`, while keeping the existing Eino Ark ChatModel model invocation.

**Architecture:** The custom Producer Eino Graph remains the top-level orchestrator. The Volcengine responder continues to call Eino `ark.NewChatModel` and `model.Stream`, but its output now retains the final Eino `schema.Message`; the graph routes native tool calls into an Eino `EdgesNode` backed by adapters around the existing ClipAnvil Edge Registry and `RegistryEdgeExecutor`.

**Tech Stack:** Go 1.26, CloudWeGo Eino v0.9.9, Eino Ark model extension v0.1.68, Hertz, pgx/sqlc, Vite/React for browser E2E.

---

## File Structure

- Modify `apps/server/internal/agent/producer/types.go`
  - Add `ModelMessage *schema.Message` and `UsedLegacyEdgeParser bool` to `ProducerTurnOutput`.
- Modify `apps/server/internal/agent/producer/model_responder.go`
  - Bind Eino tools with `EdgeCallingChatModel.WithEdges`.
  - Return final `schema.Message` in `ProducerTurnOutput`.
  - Keep existing streaming/thinking behavior.
- Create `apps/server/internal/agent/producer/eino_tool_adapter.go`
  - Convert existing `agenttools.Definition` to Eino `schema.EdgeInfo`.
  - Wrap existing `EdgeExecutor` as Eino `tool.InvokableEdge`.
- Modify `apps/server/internal/agent/producer/graph.go`
  - Use native `ModelMessage.EdgeCalls` in the normal loop.
  - Execute tool calls through `compose.EdgesNode`.
  - Keep legacy parser only as fallback.
- Modify `apps/server/internal/agent/producer/graph_test.go`
  - Replace JSON-text tool-call normal-path tests with native `schema.EdgeCall` tests.
  - Keep one explicit legacy fallback test.
- Modify `apps/server/internal/agent/producer/model_responder_test.go`
  - Verify tool edge and native tool-call output retention.
- Add `apps/server/internal/agent/producer/eino_tool_adapter_test.go`
  - Verify tool schema conversion, invokable execution, and EdgesNode invocation.

## Task 1: Prove Native Edge Calls In Producer Output

**Files:**
- Modify: `apps/server/internal/agent/producer/types.go`
- Modify: `apps/server/internal/agent/producer/model_responder_test.go`
- Modify: `apps/server/internal/agent/producer/model_responder.go`

- [ ] **Step 1: Write failing test for native tool call retention**

Add to `apps/server/internal/agent/producer/model_responder_test.go`:

```go
func TestVolcengineModelResponderReturnsNativeEdgeCalls(t *testing.T) {
	streamer := &fakeArkStreamer{
		chunks: []*schema.Message{
			{
				EdgeCalls: []schema.EdgeCall{
					{
						ID:   "call-update-storyboard",
						Type: "function",
						Function: schema.FunctionCall{
							Name:      "update_storyboard",
							Arguments: `{"intent":"replace","shots":[{"client_key":"shot-01","sort_order":1,"title":"开场"}]}`,
						},
					},
				},
			},
		},
	}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	out, err := responder.Respond(context.Background(), ProducerContext{LatestUserText: "拆分镜"})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelMessage == nil {
		t.Fatal("ModelMessage is nil")
	}
	if len(out.ModelMessage.EdgeCalls) != 1 {
		t.Fatalf("tool calls = %#v", out.ModelMessage.EdgeCalls)
	}
	if out.ModelMessage.EdgeCalls[0].Function.Name != "update_storyboard" {
		t.Fatalf("tool name = %q", out.ModelMessage.EdgeCalls[0].Function.Name)
	}
	if out.Metadata["native_tool_call_count"] != 1 {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run TestVolcengineModelResponderReturnsNativeEdgeCalls -count=1
```

Expected: fail because `ProducerTurnOutput` has no `ModelMessage`.

- [ ] **Step 3: Implement minimal output support**

In `types.go`, add:

```go
ModelMessage          *schema.Message
UsedLegacyEdgeParser  bool
```

to `ProducerTurnOutput`, and import `github.com/cloudwego/eino/schema`.

In `model_responder.go`, set:

```go
metadata["native_tool_call_count"] = len(final.EdgeCalls)
out := ProducerTurnOutput{
	AssistantText: strings.TrimSpace(final.Content),
	Metadata:      metadata,
	ModelMessage:  final,
}
return out, nil
```

- [ ] **Step 4: Run test and verify it passes**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run TestVolcengineModelResponderReturnsNativeEdgeCalls -count=1
```

Expected: PASS.

## Task 2: Add Eino Edge Adapter

**Files:**
- Create: `apps/server/internal/agent/producer/eino_tool_adapter.go`
- Add: `apps/server/internal/agent/producer/eino_tool_adapter_test.go`

- [ ] **Step 1: Write failing adapter tests**

Create `apps/server/internal/agent/producer/eino_tool_adapter_test.go` with tests that:

- convert `agenttools.Definition{Name:"update_storyboard", Description:"...", Parameters: map[string]any{"type":"object"}}` to `schema.EdgeInfo`;
- execute an adapter through `InvokableRun`;
- execute the same adapter through `compose.NewEdgeNode(...).Invoke(...)`.

The test tool should record the received tool name and arguments and return `{"ok":true}`.

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'TestEinoEdge' -count=1
```

Expected: fail because adapter types do not exist.

- [ ] **Step 3: Implement adapter**

Create `eino_tool_adapter.go` with:

- `type einoProducerEdge struct`.
- `func producerEdgeInfo(def agenttools.Definition) (*schema.EdgeInfo, error)`.
- `func newEinoProducerEdges(ctx context.Context, producerContext ProducerContext, registry *agenttools.Registry, executor EdgeExecutor) ([]tool.BaseEdge, map[string]agenttools.Definition, error)`.
- `func (t *einoProducerEdge) Info(ctx context.Context) (*schema.EdgeInfo, error)`.
- `func (t *einoProducerEdge) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error)`.

Adapter execution must:

```text
decode JSON arguments
-> call executor.ExecuteProducerEdge(ctx, producerContext, EdgeCall{ID: currentCallID, Name: def.Name, Arguments: args})
-> remember EdgeExecutionResult by call id
-> return result JSON string
```

Because Eino `InvokableRun` does not directly pass call id, add a `EdgeCallMiddlewares` path in Task 3 to attach the call id around execution.

- [ ] **Step 4: Run tests and verify they pass**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'TestEinoEdge' -count=1
```

Expected: PASS.

## Task 3: Route Native Edge Calls Through EdgesNode

**Files:**
- Modify: `apps/server/internal/agent/producer/graph.go`
- Modify: `apps/server/internal/agent/producer/graph_test.go`

- [ ] **Step 1: Write failing graph test for native tool call path**

Add a test where `sequenceResponder` returns:

```go
ProducerTurnOutput{
	ModelMessage: &schema.Message{
		Role: schema.Assistant,
		EdgeCalls: []schema.EdgeCall{
			{
				ID: "call-storyboard",
				Type: "function",
				Function: schema.FunctionCall{
					Name: "update_storyboard",
					Arguments: `{"intent":"replace","shots":[{"client_key":"shot-01","sort_order":1,"title":"开场"}]}`,
				},
			},
		},
	},
}
```

The second responder output should be `AssistantText: "已更新 storyboard。"` and the fake executor should record `update_storyboard`.

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run TestProducerGraphExecutesNativeEinoEdgeCall -count=1
```

Expected: fail because graph still parses assistant text.

- [ ] **Step 3: Implement native tool-call loop**

Change `runProducerLoop` so the first branch is:

```text
if out.ModelMessage != nil && len(out.ModelMessage.EdgeCalls) > 0:
  enforce max tool calls
  create EdgesNode using per-turn Eino tool adapters
  invoke EdgesNode with out.ModelMessage
  append assistant same-turn tool-call message
  append returned tool messages
  if any executed tool requires HITL, return interrupted output
  continue
```

Only if no native tool calls exist should the graph consider legacy fallback parsing.

- [ ] **Step 4: Run graph native tool test and existing graph tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'TestProducerGraph' -count=1
```

Expected: PASS.

## Task 4: Bind Edges To The Eino Ark Model

**Files:**
- Modify: `apps/server/internal/agent/producer/model_responder.go`
- Modify: `apps/server/internal/agent/producer/model_responder_test.go`

- [ ] **Step 1: Write failing test for tool edge**

Extend the fake streamer/factory path so tests can verify the responder calls `WithEdges` with at least `update_storyboard` when Producer context provides tool infos.

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run TestVolcengineModelResponderBindsProducerEdges -count=1
```

Expected: fail because responder does not bind tools.

- [ ] **Step 3: Implement tool edge**

Update responder config/context to accept tool infos:

```go
EdgeInfos []*schema.EdgeInfo
```

Before `model.Stream`, if tool infos are present:

```go
toolCallingModel, ok := model.(einoModel.EdgeCallingChatModel)
if !ok {
	return ProducerTurnOutput{}, NewAgentError("agent_model_tool_calling_unsupported", "selected Producer model does not support tool calling")
}
boundModel, err := toolCallingModel.WithEdges(producerContext.EdgeInfos)
stream, err := boundModel.Stream(ctx, messages)
```

Use `WithEdges`, not `BindEdges`.

- [ ] **Step 4: Run model responder tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'TestVolcengineModelResponder' -count=1
```

Expected: PASS.

## Task 5: Legacy Parser Fallback Gate

**Files:**
- Modify: `apps/server/internal/agent/producer/graph.go`
- Modify: `apps/server/internal/agent/producer/graph_test.go`

- [ ] **Step 1: Add fallback test**

Add one explicit test where the model returns no native `EdgeCalls`, but text contains the old JSON envelope. The test should enable fallback and assert `UsedLegacyEdgeParser == true`.

- [ ] **Step 2: Run fallback test and verify it fails**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run TestProducerGraphUsesLegacyEdgeParserOnlyWhenEnabled -count=1
```

Expected: fail until fallback gate exists.

- [ ] **Step 3: Implement fallback gate**

Add a `GraphConfig.EnableLegacyEdgeParserFallback bool` or equivalent config path. Normal runtime should default this to false unless explicitly enabled.

- [ ] **Step 4: Run parser and graph tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'TestProducerGraph|TestParseEdgeCall' -count=1
```

Expected: PASS.

## Task 6: Focused Backend Verification

**Files:**
- No new files unless tests reveal a gap.

- [ ] **Step 1: Run focused Agent tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer ./internal/agent/tools ./internal/agent/storyboard ./internal/agent/pss -count=1
```

Expected: PASS.

- [ ] **Step 2: Run server build**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: PASS.

- [ ] **Step 3: Run diff whitespace check**

Run:

```bash
git diff --check
```

Expected: no output.

## Task 7: Browser E2E With Real Agent Conversation

**Files:**
- No source files expected.

- [ ] **Step 1: Start dev server**

Run:

```bash
./scripts/dev-start.sh
```

Expected: script prints the actual Vite URL and backend health is OK.

- [ ] **Step 2: Open browser to Agent workspace**

Use the printed Vite URL. Create or open an Agent workspace.

- [ ] **Step 3: Non-thinking + non-tool call**

Select a non-thinking or minimal-thinking model/mode. Send:

```text
请用一句话介绍 ClipAnvil 是什么，不要调用工具。
```

Expected:

- assistant streams a normal markdown/text answer;
- no tool call UI appears;
- logs do not show fallback parser usage.

- [ ] **Step 4: Thinking + non-tool call**

Select a thinking-capable model and high thinking depth. Send:

```text
请先思考如何做一个 15 秒短视频，再用三句话回答，但不要调用工具。
```

Expected:

- thinking block streams and then collapses/settles according to current UI behavior;
- final answer appears;
- no tool call UI appears.

- [ ] **Step 5: Non-thinking + tool call**

Select minimal/disabled thinking. Send:

```text
我想要创作一个15s请把一个 15 秒的口播种草短视频拆成 3 个分镜，并保存到 storyboard。
```

Expected:

- `update_storyboard` tool activity appears;
- raw `<|FunctionCallBegin|>` or JSON envelope is not visible;
- assistant returns a human-readable response;
- DB contains 3 shots for the workspace.

- [ ] **Step 6: Thinking + tool call / state read**

Select high thinking depth. Send:

```text
查询现在的 production state，并指出刚才三个分镜各自的作用。
```

Expected:

- thinking output appears if provider streams reasoning;
- `get_production_state` tool activity appears;
- assistant answer references the 3 shots;
- logs show native Eino tool calls.

- [ ] **Step 7: Inspect logs and DB**

Check backend logs for:

```text
native_tool_call_count
tool_binding_count
fallback parser usage absent
```

Check DB for:

```sql
SELECT sort_order, title, duration_sec, brief
FROM shot
WHERE workspace_id = '<workspace-id>'
ORDER BY sort_order;
```

Expected: 3 rows.

- [ ] **Step 8: Stop dev server**

Run:

```bash
./scripts/dev-stop.sh
```

Expected: current worktree frontend/backend processes stop cleanly.
