# M6.3 Producer Turn Execution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first executable Producer turn: user message creates a `producer_turn` task, an Eino ProducerGraph runs, an assistant reply is persisted, and the UI receives it through `/ws/agent`.

**Architecture:** Keep M6.1 runtime as the persistence boundary and M6.2 API/WebSocket as the transport boundary. Add `internal/agent/producer` for Eino Graph, deterministic responder, and executor; extend the Agent REST handler to create and enqueue producer tasks; extend frontend Agent DTOs and chat state to display task progress and assistant replies.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, CloudWeGo Eino v0.9.9 `compose.NewGraph`, M6.1 Agent runtime service, M6.2 AgentHub, React 19, TanStack Query, TypeScript 6, Vite 8.

---

## Source Specs

- `docs/superpowers/specs/2026-06-21-m6-3-producer-turn-execution-design.md`
- `docs/superpowers/specs/2026-06-21-m6-2-agent-api-ws-chat-design.md`
- `docs/superpowers/specs/2026-06-21-m6-1-agent-runtime-persistence-design.md`
- `docs/superpowers/specs/2026-06-21-m6-multiagent-agent-mode-design.md`

## Scope Lock

M6.3 must produce real assistant replies, but it must not add real LLM credentials, HITL, tool registry, Storyboard/PSS, Craftsman/Worker/Composer, preview generation, video generation, retry rubric, or final composition. The default responder is deterministic so unit tests and browser e2e do not depend on external providers.

## File Structure

- Modify `apps/server/sqlc/queries/agent_task.sql`
  - Add oldest-first queued task lookup for workspace-scoped tests and startup recovery.
- Run `make sqlc-generate`
  - Updates `apps/server/internal/store/db/agent_task.sql.go`.
- Modify `apps/server/internal/agent/runtime/service.go`
  - Add `ListQueuedProducerTasks` and `ListQueuedProducerTasksAcrossWorkspaces`.
- Modify `apps/server/internal/agent/runtime/service_test.go`
  - Cover queued task lookup validation through fakes.
- Create `apps/server/internal/agent/producer/types.go`
  - Shared ProducerGraph input/output/context types.
- Create `apps/server/internal/agent/producer/responder.go`
  - `Responder` interface and deterministic responder.
- Create `apps/server/internal/agent/producer/graph.go`
  - Eino `compose.NewGraph` construction and invocation.
- Create `apps/server/internal/agent/producer/executor.go`
  - Task execution, message/event persistence, broadcasts.
- Create `apps/server/internal/agent/producer/graph_test.go`
  - Graph and deterministic responder tests.
- Create `apps/server/internal/agent/producer/executor_test.go`
  - Executor success/failure tests with fakes.
- Modify `apps/server/internal/api/agent_response.go`
  - Add `agentTaskResponse`.
- Modify `apps/server/internal/api/agent_handler.go`
  - Create producer task after user message and enqueue executor.
- Modify `apps/server/internal/api/agent_handler_test.go`
  - Cover response DTOs and task payload helper.
- Modify `apps/server/cmd/server/main.go`
  - Construct ProducerGraph/executor and run startup queued-task recovery.
- Modify `apps/web/src/lib/agentApi.ts`
  - Add `AgentTask` DTO and optional task in post response.
- Modify `apps/web/src/lib/agentWs.ts`
  - Add `agent.task.updated` typing.
- Create `apps/web/src/lib/agentTasks.ts`
  - Merge task updates and derive running producer state.
- Create `apps/web/src/lib/agentTasks.test.mjs`
  - Task merge/running-state tests.
- Modify `apps/web/src/pages/AgentWorkspacePage.tsx`
  - Display assistant replies, error messages, and Producer running status.
- Modify `apps/web/tsconfig.test.json`
  - Include `agentTasks.ts`.
- Modify `apps/web/package.json`
  - Add `agentTasks.test.mjs` to `test:connections`.

## Task 1: Add queued producer task lookup

**Files:**
- Modify: `apps/server/sqlc/queries/agent_task.sql`
- Generated: `apps/server/internal/store/db/agent_task.sql.go`
- Modify: `apps/server/internal/agent/runtime/service.go`
- Modify: `apps/server/internal/agent/runtime/service_test.go`

- [ ] **Step 1: Add failing runtime test**

In `apps/server/internal/agent/runtime/service_test.go`, add:

```go
func TestListQueuedProducerTasksRejectsMissingWorkspace(t *testing.T) {
	svc, err := NewService(&fakeBeginner{}, db.New(fakeDBTX{}))
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ListQueuedProducerTasks(context.Background(), pgtype.UUID{}, 10)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}
```

- [ ] **Step 2: Run runtime tests and verify red**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/runtime
```

Expected: FAIL because `ListQueuedProducerTasks` does not exist.

- [ ] **Step 3: Add sqlc query**

Append to `apps/server/sqlc/queries/agent_task.sql`:

```sql
-- name: ListQueuedProducerTasks :many
SELECT *
FROM agent_task
WHERE workspace_id = $1
  AND role = 'producer'
  AND task_type = 'producer_turn'
  AND status = 'queued'
ORDER BY created_at ASC
LIMIT $2;

-- name: ListQueuedProducerTasksAcrossWorkspaces :many
SELECT *
FROM agent_task
WHERE role = 'producer'
  AND task_type = 'producer_turn'
  AND status = 'queued'
ORDER BY created_at ASC
LIMIT $1;
```

- [ ] **Step 4: Generate sqlc**

Run:

```bash
make sqlc-generate
```

Expected: `apps/server/internal/store/db/agent_task.sql.go` contains `ListQueuedProducerTasks`.

- [ ] **Step 5: Implement runtime wrapper**

In `apps/server/internal/agent/runtime/service.go`, add:

```go
func (s *Service) ListQueuedProducerTasks(ctx context.Context, workspaceID pgtype.UUID, limit int32) ([]db.AgentTask, error) {
	if !workspaceID.Valid {
		return nil, ErrInvalidRequest
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.queries.ListQueuedProducerTasks(ctx, db.ListQueuedProducerTasksParams{
		WorkspaceID: workspaceID,
		Limit:       limit,
	})
}

func (s *Service) ListQueuedProducerTasksAcrossWorkspaces(ctx context.Context, limit int32) ([]db.AgentTask, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.queries.ListQueuedProducerTasksAcrossWorkspaces(ctx, limit)
}
```

- [ ] **Step 6: Verify runtime tests**

Run:

```bash
cd apps/server && gofmt -w internal/agent/runtime/service.go internal/agent/runtime/service_test.go
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/runtime
```

Expected: PASS.

## Task 2: Implement deterministic Producer responder

**Files:**
- Create: `apps/server/internal/agent/producer/types.go`
- Create: `apps/server/internal/agent/producer/responder.go`
- Create: `apps/server/internal/agent/producer/graph_test.go`

- [ ] **Step 1: Write failing responder test**

Create `apps/server/internal/agent/producer/graph_test.go`:

```go
package producer

import (
	"context"
	"strings"
	"testing"
)

func TestDeterministicResponderUsesLatestUserText(t *testing.T) {
	responder := DeterministicResponder{}

	out, err := responder.Respond(context.Background(), ProducerContext{
		LatestUserText: "做一个咖啡广告",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.AssistantText, "做一个咖啡广告") {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if !strings.Contains(out.AssistantText, "后续阶段拆成分镜和生产任务") {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
}
```

- [ ] **Step 2: Run producer tests and verify red**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer
```

Expected: FAIL because package/types do not exist.

- [ ] **Step 3: Add producer types**

Create `apps/server/internal/agent/producer/types.go`:

```go
package producer

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ProducerTurnInput struct {
	WorkspaceID      pgtype.UUID
	ThreadID         pgtype.UUID
	TaskID           pgtype.UUID
	TriggerMessageID pgtype.UUID
}

type ProducerTurnOutput struct {
	AssistantText string
	Metadata      map[string]any
}

type ProducerContext struct {
	Input          ProducerTurnInput
	Messages       []db.AgentMessage
	LatestUserText string
}
```

- [ ] **Step 4: Add deterministic responder**

Create `apps/server/internal/agent/producer/responder.go`:

```go
package producer

import (
	"context"
	"fmt"
	"strings"
)

type Responder interface {
	Respond(ctx context.Context, context ProducerContext) (ProducerTurnOutput, error)
}

type DeterministicResponder struct{}

func (DeterministicResponder) Respond(_ context.Context, context ProducerContext) (ProducerTurnOutput, error) {
	text := strings.TrimSpace(context.LatestUserText)
	if text == "" {
		text = "你的需求"
	}
	return ProducerTurnOutput{
		AssistantText: fmt.Sprintf("我已收到你的需求：「%s」。\n下一步我会先整理创作目标，再在后续阶段拆成分镜和生产任务。", text),
		Metadata: map[string]any{
			"responder": "deterministic",
		},
	}, nil
}
```

- [ ] **Step 5: Verify producer tests**

Run:

```bash
cd apps/server && gofmt -w internal/agent/producer
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer
```

Expected: PASS.

## Task 3: Build Eino ProducerGraph

**Files:**
- Modify: `apps/server/internal/agent/producer/graph.go`
- Modify: `apps/server/internal/agent/producer/graph_test.go`

- [ ] **Step 1: Add failing graph test**

Append to `graph_test.go`:

```go
func TestGraphRunReturnsAssistantText(t *testing.T) {
	graph, err := NewGraph(GraphConfig{
		Loader: fakeContextLoader{
			context: ProducerContext{LatestUserText: "一条运动鞋短片"},
		},
		Responder: DeterministicResponder{},
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), ProducerTurnInput{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.AssistantText, "一条运动鞋短片") {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
}

type fakeContextLoader struct {
	context ProducerContext
	err     error
}

func (f fakeContextLoader) LoadProducerContext(context.Context, ProducerTurnInput) (ProducerContext, error) {
	return f.context, f.err
}
```

- [ ] **Step 2: Run producer tests and verify red**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer
```

Expected: FAIL because `NewGraph`, `GraphConfig`, and loader interface do not exist.

- [ ] **Step 3: Implement graph**

Create `apps/server/internal/agent/producer/graph.go`:

```go
package producer

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/eino/compose"
)

var ErrInvalidGraphConfig = errors.New("invalid producer graph config")

type ContextLoader interface {
	LoadProducerContext(ctx context.Context, input ProducerTurnInput) (ProducerContext, error)
}

type GraphConfig struct {
	Loader    ContextLoader
	Responder Responder
}

type Graph struct {
	runnable compose.Runnable[ProducerTurnInput, ProducerTurnOutput]
}

func NewGraph(config GraphConfig) (*Graph, error) {
	if config.Loader == nil || config.Responder == nil {
		return nil, ErrInvalidGraphConfig
	}

	g := compose.NewGraph[ProducerTurnInput, ProducerTurnOutput]()
	if err := g.AddLambdaNode("load_context", compose.InvokableLambda(func(ctx context.Context, input ProducerTurnInput) (ProducerContext, error) {
		return config.Loader.LoadProducerContext(ctx, input)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("draft_response", compose.InvokableLambda(func(ctx context.Context, input ProducerContext) (ProducerTurnOutput, error) {
		return config.Responder.Respond(ctx, input)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("finalize_response", compose.InvokableLambda(func(_ context.Context, input ProducerTurnOutput) (ProducerTurnOutput, error) {
		input.AssistantText = strings.TrimSpace(input.AssistantText)
		if input.AssistantText == "" {
			return ProducerTurnOutput{}, errors.New("producer returned empty response")
		}
		if input.Metadata == nil {
			input.Metadata = map[string]any{}
		}
		return input, nil
	})); err != nil {
		return nil, err
	}

	if err := g.AddEdge(compose.START, "load_context"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("load_context", "draft_response"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("draft_response", "finalize_response"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("finalize_response", compose.END); err != nil {
		return nil, err
	}

	runnable, err := g.Compile(context.Background(), compose.WithGraphName("producer_turn"))
	if err != nil {
		return nil, err
	}
	return &Graph{runnable: runnable}, nil
}

func (g *Graph) Run(ctx context.Context, input ProducerTurnInput) (ProducerTurnOutput, error) {
	return g.runnable.Invoke(ctx, input)
}
```

- [ ] **Step 4: Verify producer tests**

Run:

```bash
cd apps/server && gofmt -w internal/agent/producer
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer
```

Expected: PASS.

## Task 4: Load Producer context from runtime messages

**Files:**
- Create: `apps/server/internal/agent/producer/context_loader.go`
- Modify: `apps/server/internal/agent/producer/graph_test.go`

- [ ] **Step 1: Add context extraction unit test**

Append to `graph_test.go`:

```go
func TestLatestUserTextFromMessagesUsesLastUserText(t *testing.T) {
	messages := []db.AgentMessage{
		{Role: "user", MessageType: "text", Content: []byte(`{"text":"first"}`)},
		{Role: "assistant", MessageType: "text", Content: []byte(`{"text":"reply"}`)},
		{Role: "user", MessageType: "text", Content: []byte(`{"text":"second"}`)},
	}

	got := latestUserTextFromMessages(messages)

	if got != "second" {
		t.Fatalf("latest user text = %q, want second", got)
	}
}
```

Add `db` import:

```go
import "github.com/sinmaystar/clip-anvil/internal/store/db"
```

- [ ] **Step 2: Run producer tests and verify red**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer
```

Expected: FAIL because `latestUserTextFromMessages` does not exist.

- [ ] **Step 3: Implement context loader helpers**

Create `apps/server/internal/agent/producer/context_loader.go`:

```go
package producer

import (
	"context"
	"encoding/json"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type RuntimeContextLoader struct {
	Runtime *agentruntime.Service
}

func (l RuntimeContextLoader) LoadProducerContext(ctx context.Context, input ProducerTurnInput) (ProducerContext, error) {
	messages, err := l.Runtime.ListMessages(ctx, input.ThreadID, 0, 20)
	if err != nil {
		return ProducerContext{}, err
	}
	return ProducerContext{
		Input:          input,
		Messages:       messages,
		LatestUserText: latestUserTextFromMessages(messages),
	}, nil
}

func latestUserTextFromMessages(messages []db.AgentMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		var content struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(messages[i].Content, &content); err == nil && content.Text != "" {
			return content.Text
		}
	}
	return ""
}
```

- [ ] **Step 4: Verify producer tests**

Run:

```bash
cd apps/server && gofmt -w internal/agent/producer
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer
```

Expected: PASS.

## Task 5: Implement Producer executor

**Files:**
- Create: `apps/server/internal/agent/producer/executor.go`
- Create: `apps/server/internal/agent/producer/executor_test.go`

- [ ] **Step 1: Write executor success test with fakes**

Create `apps/server/internal/agent/producer/executor_test.go`:

```go
package producer

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestExecutorPersistsAssistantMessageOnSuccess(t *testing.T) {
	runtime := &fakeRuntime{}
	broadcaster := &fakeBroadcaster{}
	executor := NewExecutor(ExecutorConfig{
		Runtime:     runtime,
		Graph:       fakeGraph{output: ProducerTurnOutput{AssistantText: "assistant reply"}},
		Broadcaster: broadcaster,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	if runtime.runningTask != uuidWithByte(3) {
		t.Fatal("task was not marked running")
	}
	if runtime.assistantText != "assistant reply" {
		t.Fatalf("assistant text = %q", runtime.assistantText)
	}
	if runtime.succeededTask != uuidWithByte(3) {
		t.Fatal("task was not marked succeeded")
	}
	if broadcaster.messageCount != 1 || broadcaster.taskCount == 0 {
		t.Fatalf("broadcast counts = messages %d tasks %d", broadcaster.messageCount, broadcaster.taskCount)
	}
}

func TestExecutorPersistsErrorMessageOnFailure(t *testing.T) {
	runtime := &fakeRuntime{}
	broadcaster := &fakeBroadcaster{}
	executor := NewExecutor(ExecutorConfig{
		Runtime:     runtime,
		Graph:       fakeGraph{err: errors.New("model unavailable")},
		Broadcaster: broadcaster,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID:      uuidWithByte(1),
		ThreadID:         uuidWithByte(2),
		TaskID:           uuidWithByte(3),
		TriggerMessageID: uuidWithByte(4),
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if runtime.failedTask != uuidWithByte(3) {
		t.Fatal("task was not marked failed")
	}
	if runtime.assistantMessageType != "error" {
		t.Fatalf("assistant message type = %q, want error", runtime.assistantMessageType)
	}
	if broadcaster.messageCount != 1 || broadcaster.taskCount == 0 {
		t.Fatalf("broadcast counts = messages %d tasks %d", broadcaster.messageCount, broadcaster.taskCount)
	}
}
```

The fake types in this test should implement only the methods required by the executor interfaces defined in Step 3. Add `errors` to the test imports.

- [ ] **Step 2: Run producer tests and verify red**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer
```

Expected: FAIL because executor types do not exist.

- [ ] **Step 3: Implement executor with narrow interfaces**

Create `apps/server/internal/agent/producer/executor.go`:

```go
package producer

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Runtime interface {
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type Runner interface {
	Run(ctx context.Context, input ProducerTurnInput) (ProducerTurnOutput, error)
}

type Broadcaster interface {
	BroadcastAgentMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent)
	BroadcastAgentTask(workspaceID pgtype.UUID, task db.AgentTask)
	BroadcastAgentEvent(workspaceID pgtype.UUID, event db.AgentEvent)
}

type ExecutorConfig struct {
	Runtime     Runtime
	Graph       Runner
	Broadcaster Broadcaster
}

type Executor struct {
	runtime     Runtime
	graph       Runner
	broadcaster Broadcaster
}

type RunTaskInput struct {
	WorkspaceID      pgtype.UUID
	ThreadID         pgtype.UUID
	TaskID           pgtype.UUID
	TriggerMessageID pgtype.UUID
}

func NewExecutor(config ExecutorConfig) *Executor {
	return &Executor{runtime: config.Runtime, graph: config.Graph, broadcaster: config.Broadcaster}
}

func (e *Executor) RunTask(ctx context.Context, input RunTaskInput) error {
	if !input.WorkspaceID.Valid || !input.ThreadID.Valid || !input.TaskID.Valid {
		return errors.New("invalid producer task input")
	}
	runningTask, err := e.runtime.MarkTaskRunning(ctx, input.TaskID)
	if err != nil {
		return err
	}
	e.broadcaster.BroadcastAgentTask(input.WorkspaceID, runningTask)

	started, err := e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "producer_turn_started",
		SourceRole:  "producer",
		TargetRole:  "user",
		Payload:     mustJSON(map[string]string{"task_id": uuidString(input.TaskID)}),
	})
	if err == nil {
		e.broadcaster.BroadcastAgentEvent(input.WorkspaceID, started)
	}

	output, err := e.graph.Run(ctx, ProducerTurnInput(input))
	if err != nil {
		return e.failTask(ctx, input, "producer_turn_failed", err.Error())
	}

	msg, err := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        "assistant",
		MessageType: "text",
		Content:     mustJSON(map[string]any{"text": output.AssistantText}),
		RawMessage:  mustJSON(output.Metadata),
		TaskID:      input.TaskID,
	})
	if err != nil {
		return e.failTask(ctx, input, "producer_message_persist_failed", err.Error())
	}

	completed, err := e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "producer_turn_completed",
		SourceRole:  "producer",
		TargetRole:  "user",
		Payload:     mustJSON(map[string]string{"message_id": uuidString(msg.ID)}),
	})
	if err != nil {
		return e.failTask(ctx, input, "producer_event_persist_failed", err.Error())
	}

	succeededTask, err := e.runtime.MarkTaskSucceeded(ctx, input.TaskID, mustJSON(output.Metadata))
	if err != nil {
		return err
	}

	e.broadcaster.BroadcastAgentMessage(input.WorkspaceID, msg, completed)
	e.broadcaster.BroadcastAgentEvent(input.WorkspaceID, completed)
	e.broadcaster.BroadcastAgentTask(input.WorkspaceID, succeededTask)
	return nil
}
```

Add helper functions in the same file:

```go
func (e *Executor) failTask(ctx context.Context, input RunTaskInput, code string, message string) error {
	errorMsg, msgErr := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        "assistant",
		MessageType: "error",
		Content:     mustJSON(map[string]any{"text": "Producer 执行失败，请稍后再试。", "error_code": code}),
		RawMessage:  mustJSON(map[string]any{"error": message}),
		TaskID:      input.TaskID,
	})
	failedEvent, eventErr := e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "producer_turn_failed",
		SourceRole:  "producer",
		TargetRole:  "user",
		Payload:     mustJSON(map[string]string{"error_code": code}),
	})
	failedTask, err := e.runtime.MarkTaskFailed(ctx, input.TaskID, code, message)
	if msgErr == nil && eventErr == nil {
		e.broadcaster.BroadcastAgentMessage(input.WorkspaceID, errorMsg, failedEvent)
		e.broadcaster.BroadcastAgentEvent(input.WorkspaceID, failedEvent)
	}
	if err == nil {
		e.broadcaster.BroadcastAgentTask(input.WorkspaceID, failedTask)
	}
	return errors.New(message)
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
```

Import `github.com/google/uuid` for `uuidString`.

- [ ] **Step 4: Complete fake test types**

In `executor_test.go`, implement:

```go
type fakeGraph struct {
	output ProducerTurnOutput
	err    error
}

func (f fakeGraph) Run(context.Context, ProducerTurnInput) (ProducerTurnOutput, error) {
	return f.output, f.err
}

type fakeBroadcaster struct {
	messageCount int
	taskCount    int
	eventCount   int
}
```

Implement broadcaster methods by incrementing counters. Implement `fakeRuntime` methods by storing task IDs, assistant content from `AppendMessageParams.Content`, and `assistantMessageType` from `AppendMessageParams.MessageType`.

- [ ] **Step 5: Verify producer tests**

Run:

```bash
cd apps/server && gofmt -w internal/agent/producer
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer
```

Expected: PASS.

## Task 6: Add API task response and Agent broadcaster adapter

**Files:**
- Modify: `apps/server/internal/api/agent_response.go`
- Create: `apps/server/internal/api/agent_broadcaster.go`
- Modify: `apps/server/internal/api/agent_handler_test.go`

- [ ] **Step 1: Write task response test**

Add to `agent_handler_test.go`:

```go
func TestAgentTaskResponseMapsTask(t *testing.T) {
	task := db.AgentTask{
		ID:          testUUID(0x11),
		WorkspaceID: testUUID(0x12),
		ThreadID:    testUUID(0x13),
		Role:        "producer",
		ScopeType:   "workspace",
		TaskType:    "producer_turn",
		Status:      "queued",
		Attempt:     0,
		MaxAttempts: 1,
		Input:       []byte(`{"trigger_message_seq":3}`),
		Output:      []byte(`{}`),
	}

	got := toAgentTaskResponse(task)

	if got.ID != uuidToString(testUUID(0x11)) || got.TaskType != "producer_turn" || got.Status != "queued" {
		t.Fatalf("task response = %#v", got)
	}
	if got.Input["trigger_message_seq"].(float64) != 3 {
		t.Fatalf("task input = %#v", got.Input)
	}
}
```

- [ ] **Step 2: Run API tests and verify red**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: FAIL because `toAgentTaskResponse` does not exist.

- [ ] **Step 3: Add task response DTO**

In `agent_response.go`, add:

```go
type agentTaskResponse struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspace_id"`
	ThreadID      *string        `json:"thread_id"`
	Role          string         `json:"role"`
	ScopeType     string         `json:"scope_type"`
	ScopeID       *string        `json:"scope_id"`
	TaskType      string         `json:"task_type"`
	Status        string         `json:"status"`
	Attempt       int32          `json:"attempt"`
	MaxAttempts   int32          `json:"max_attempts"`
	Input         map[string]any `json:"input"`
	Output        map[string]any `json:"output"`
	ErrorCode     *string        `json:"error_code"`
	ErrorMessage  *string        `json:"error_message"`
	CreatedAt     string         `json:"created_at"`
	StartedAt     *string        `json:"started_at"`
	CompletedAt   *string        `json:"completed_at"`
}
```

Implement `toAgentTaskResponse` using existing nullable and JSON helpers.

- [ ] **Step 4: Add broadcaster adapter**

Create `apps/server/internal/api/agent_broadcaster.go`:

```go
package api

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type AgentBroadcaster struct {
	hub *AgentHub
}

func NewAgentBroadcaster(hub *AgentHub) *AgentBroadcaster {
	return &AgentBroadcaster{hub: hub}
}

func (b *AgentBroadcaster) BroadcastAgentMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent) {
	b.hub.Broadcast(workspaceID, AgentSocketEvent{
		Type: "agent.message.created",
		Payload: map[string]any{
			"workspace_id": uuidToString(workspaceID),
			"thread_id":    uuidToString(message.ThreadID),
			"message":      toAgentMessageResponse(message),
			"event":        toAgentEventResponse(event),
		},
	})
}

func (b *AgentBroadcaster) BroadcastAgentTask(workspaceID pgtype.UUID, task db.AgentTask) {
	b.hub.Broadcast(workspaceID, AgentSocketEvent{
		Type: "agent.task.updated",
		Payload: map[string]any{
			"workspace_id": uuidToString(workspaceID),
			"thread_id":    nullableUUIDString(task.ThreadID),
			"task":         toAgentTaskResponse(task),
		},
	})
}

func (b *AgentBroadcaster) BroadcastAgentEvent(workspaceID pgtype.UUID, event db.AgentEvent) {
	b.hub.Broadcast(workspaceID, AgentSocketEvent{
		Type: "agent.event.created",
		Payload: map[string]any{
			"workspace_id": uuidToString(workspaceID),
			"thread_id":    nullableUUIDString(event.ThreadID),
			"task_id":      nullableUUIDString(event.TaskID),
			"event":        toAgentEventResponse(event),
		},
	})
}
```

- [ ] **Step 5: Verify API tests**

Run:

```bash
cd apps/server && gofmt -w internal/api
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: PASS.

## Task 7: Create producer task from POST message

**Files:**
- Modify: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/server/internal/api/agent_handler_test.go`

- [ ] **Step 1: Add task input helper test**

Add to `agent_handler_test.go`:

```go
func TestProducerTurnTaskInputIncludesTriggerMessage(t *testing.T) {
	msg := db.AgentMessage{ID: testUUID(0x31), Seq: 9}

	body := producerTurnTaskInput(msg)

	if !strings.Contains(string(body), `"trigger_message_id":"31000000-0000-0000-0000-000000000000"`) {
		t.Fatalf("task input = %s", body)
	}
	if !strings.Contains(string(body), `"trigger_message_seq":9`) {
		t.Fatalf("task input = %s", body)
	}
}
```

Add `strings` import if missing.

- [ ] **Step 2: Run API tests and verify red**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: FAIL because `producerTurnTaskInput` does not exist.

- [ ] **Step 3: Extend handler types**

In `agent_handler.go`, change response:

```go
type postAgentMessageResponse struct {
	Message agentMessageResponse `json:"message"`
	Event   agentEventResponse   `json:"event"`
	Task    agentTaskResponse    `json:"task"`
}
```

Add enqueue dependency to `AgentHandler`:

```go
type ProducerTaskRunner interface {
	RunTask(ctx context.Context, input producer.RunTaskInput) error
}
```

Update constructor to receive runner. If nil in tests, handler still persists task and skips async dispatch.

- [ ] **Step 4: Create task in PostMessage**

After user message is created:

```go
task, err := h.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
	WorkspaceID: workspace.ID,
	ThreadID:    thread.ID,
	Role:        "producer",
	ScopeType:   "workspace",
	TaskType:    "producer_turn",
	MaxAttempts: 1,
	Input:       producerTurnTaskInput(msg),
})
if err != nil {
	writeError(c, consts.StatusInternalServerError, "failed to create producer task")
	return
}
```

Create queued event:

```go
event, err := h.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
	WorkspaceID: workspace.ID,
	ThreadID:    thread.ID,
	TaskID:      task.ID,
	EventType:   "producer_turn_queued",
	SourceRole:  "user",
	TargetRole:  "producer",
	Scope:       agentMessageEventScope(thread.ID),
	Payload:     producerTurnQueuedPayload(msg, task),
})
```

Broadcast user message, task, and event. Then:

```go
if h.producerRunner != nil {
	go func() {
		_ = h.producerRunner.RunTask(context.Background(), producer.RunTaskInput{
			WorkspaceID:      workspace.ID,
			ThreadID:         thread.ID,
			TaskID:           task.ID,
			TriggerMessageID: msg.ID,
		})
	}()
}
```

- [ ] **Step 5: Add helpers**

Add:

```go
func producerTurnTaskInput(msg db.AgentMessage) []byte {
	raw, err := json.Marshal(map[string]any{
		"trigger_message_id":  uuidToString(msg.ID),
		"trigger_message_seq": msg.Seq,
	})
	if err != nil {
		return []byte("{}")
	}
	return raw
}
```

- [ ] **Step 6: Verify API tests**

Run:

```bash
cd apps/server && gofmt -w internal/api
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: PASS.

## Task 8: Wire executor in server main

**Files:**
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Construct ProducerGraph and executor**

After `agentRuntime` and `agentHub` are created:

```go
producerGraph, err := producer.NewGraph(producer.GraphConfig{
	Loader:    producer.RuntimeContextLoader{Runtime: agentRuntime},
	Responder: producer.DeterministicResponder{},
})
if err != nil {
	slog.Error("failed to create producer graph", "error", err)
	os.Exit(1)
}
producerExecutor := producer.NewExecutor(producer.ExecutorConfig{
	Runtime:     agentRuntime,
	Graph:       producerGraph,
	Broadcaster: api.NewAgentBroadcaster(agentHub),
})
```

Pass `producerExecutor` into `api.NewAgentHandler`.

- [ ] **Step 2: Add startup queued recovery**

After routes are ready or before server start, run:

```go
go func() {
	tasks, err := agentRuntime.ListQueuedProducerTasksAcrossWorkspaces(context.Background(), 50)
	if err != nil {
		slog.Warn("skipping queued producer recovery", "error", err)
		return
	}
	for _, task := range tasks {
		_ = producerExecutor.RunTask(context.Background(), producer.RunTaskInput{
			WorkspaceID: task.WorkspaceID,
			ThreadID:    task.ThreadID,
			TaskID:      task.ID,
		})
	}
}()
```

- [ ] **Step 3: Verify server compile**

Run:

```bash
cd apps/server && gofmt -w cmd/server/main.go
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./cmd/server ./internal/agent/producer ./internal/api
```

Expected: PASS.

## Task 9: Add frontend task DTOs and merge helper

**Files:**
- Modify: `apps/web/src/lib/agentApi.ts`
- Modify: `apps/web/src/lib/agentWs.ts`
- Create: `apps/web/src/lib/agentTasks.ts`
- Create: `apps/web/src/lib/agentTasks.test.mjs`
- Modify: `apps/web/tsconfig.test.json`
- Modify: `apps/web/package.json`

- [ ] **Step 1: Write failing task helper test**

Create `apps/web/src/lib/agentTasks.test.mjs`:

```js
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  mergeAgentTasks,
  hasRunningProducerTask,
} from "../../dist-test/lib/agentTasks.js";

describe("agent tasks", () => {
  it("dedupes task updates by id", () => {
    const tasks = mergeAgentTasks(
      [{ id: "task-1", status: "queued", task_type: "producer_turn" }],
      [{ id: "task-1", status: "running", task_type: "producer_turn" }],
    );

    assert.deepEqual(tasks, [
      { id: "task-1", status: "running", task_type: "producer_turn" },
    ]);
  });

  it("detects running producer turns", () => {
    assert.equal(
      hasRunningProducerTask([
        { id: "task-1", status: "running", task_type: "producer_turn" },
      ]),
      true,
    );
    assert.equal(
      hasRunningProducerTask([
        { id: "task-1", status: "succeeded", task_type: "producer_turn" },
      ]),
      false,
    );
  });
});
```

- [ ] **Step 2: Run frontend tests and verify red**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL because `agentTasks.ts` is not included.

- [ ] **Step 3: Add task DTOs**

In `agentApi.ts`, add:

```ts
export interface AgentTask {
  id: string;
  workspace_id: string;
  thread_id?: string | null;
  role: "producer" | "craftsman" | "reviewer" | "composer" | "worker" | "system";
  scope_type: "workspace" | "shot" | "node" | "job" | "final_output";
  scope_id?: string | null;
  task_type: "producer_turn" | "tool_call" | "decision_resume";
  status: "queued" | "running" | "succeeded" | "failed" | "cancelled" | "waiting_for_user";
  attempt: number;
  max_attempts: number;
  input: Record<string, unknown>;
  output: Record<string, unknown>;
  error_code?: string | null;
  error_message?: string | null;
  created_at: string;
  started_at?: string | null;
  completed_at?: string | null;
}
```

Update `PostAgentMessageResponse`:

```ts
export interface PostAgentMessageResponse {
  message: AgentMessage;
  event: AgentEvent;
  task: AgentTask;
}
```

- [ ] **Step 4: Add websocket task event type**

In `agentWs.ts`, extend `AgentSocketEvent` with:

```ts
| {
    type: "agent.task.updated";
    payload: {
      workspace_id: string;
      thread_id?: string | null;
      task: AgentTask;
    };
  }
```

- [ ] **Step 5: Add task helper**

Create `apps/web/src/lib/agentTasks.ts`:

```ts
import type { AgentTask } from "./agentApi";

export function mergeAgentTasks(
  current: Pick<AgentTask, "id" | "status" | "task_type">[],
  incoming: Pick<AgentTask, "id" | "status" | "task_type">[],
) {
  const byId = new Map<string, Pick<AgentTask, "id" | "status" | "task_type">>();
  for (const task of current) {
    byId.set(task.id, task);
  }
  for (const task of incoming) {
    byId.set(task.id, task);
  }
  return [...byId.values()];
}

export function hasRunningProducerTask(
  tasks: Pick<AgentTask, "status" | "task_type">[],
) {
  return tasks.some(
    (task) =>
      task.task_type === "producer_turn" &&
      (task.status === "queued" || task.status === "running"),
  );
}
```

- [ ] **Step 6: Include tests**

Add `src/lib/agentTasks.ts` to `apps/web/tsconfig.test.json`.

Add `src/lib/agentTasks.test.mjs` to `test:connections` in `apps/web/package.json`.

- [ ] **Step 7: Verify frontend tests**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

## Task 10: Show Producer execution state in Agent UI

**Files:**
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`

- [ ] **Step 1: Wire task state**

Add state:

```ts
const [tasks, setTasks] = useState<AgentTask[]>([]);
```

On POST success:

```ts
setTasks((current) => mergeAgentTasks(current, [response.task]));
```

On WebSocket event:

```ts
if (
  event.type === "agent.task.updated" &&
  event.payload.workspace_id === id
) {
  setTasks((current) => mergeAgentTasks(current, [event.payload.task]));
}
```

- [ ] **Step 2: Render running state**

In the message list, after existing messages:

```tsx
{hasRunningProducerTask(tasks) ? (
  <p className="agent-empty-text">Producer 正在思考</p>
) : null}
```

Keep this as a small status row; do not add a blocking modal.

- [ ] **Step 3: Verify TypeScript build**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

## Task 11: Full automated verification

**Files:**
- All changed files.

- [ ] **Step 1: Run migration and sqlc checks**

Run:

```bash
make migrate-up
make sqlc-generate
```

Expected: no migration errors; sqlc generation is clean.

- [ ] **Step 2: Run backend checks**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
make server-lint
```

Expected: all pass; `server-lint` reports `0 issues`.

- [ ] **Step 3: Run frontend checks**

Run:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web test:connections
```

Expected: all pass.

- [ ] **Step 4: Run diff whitespace check**

Run:

```bash
git diff --check
```

Expected: no output and exit 0.

## Task 12: Browser e2e

**Files:**
- No code edits unless e2e exposes a bug.

- [ ] **Step 1: Start current worktree**

Run:

```bash
./scripts/dev-start.sh
```

Use the Vite URL printed by the script. In the in-app browser, use the printed Network URL if `localhost` is not reachable from the browser surface.

- [ ] **Step 2: Register/login and create Agent workspace**

In browser:

1. Register a new test user.
2. Create a workspace with `Agent 自动模式`.
3. Confirm the route is `/workspaces/<id>/agent`.
4. Confirm the Producer panel shows `已连接`.

- [ ] **Step 3: Verify assistant reply**

Open a second tab to the same Agent workspace.

In the first tab send:

```text
M6.3 e2e producer turn
```

Expected:

- First tab shows the user message.
- First tab shows `Producer 正在思考` briefly or receives task updates.
- First tab shows assistant text containing `M6.3 e2e producer turn`.
- Second tab receives the same assistant text without manual refresh.

- [ ] **Step 4: Verify refresh persistence**

Reload the first tab.

Expected:

- User message still appears.
- Assistant reply still appears.

- [ ] **Step 5: Verify database facts**

Run:

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select task_type,status from agent_task order by created_at desc limit 5;"
```

Expected: latest `producer_turn` is `succeeded`.

Run:

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select role,message_type,task_id is not null as has_task from agent_message order by created_at desc limit 5;"
```

Expected: one recent row is `assistant | text | true`.

- [ ] **Step 6: Stop dev profile**

Run the exact stop command printed by `dev-start.sh`, for example:

```bash
CLIPANVIL_DEV_NAME=<printed-profile> ./scripts/dev-stop.sh
```

Confirm frontend/backend ports no longer listen:

```bash
lsof -nP -iTCP:<printed-web-port> -sTCP:LISTEN || true
lsof -nP -iTCP:<printed-server-port> -sTCP:LISTEN || true
```

Expected: no listener output for either port.
