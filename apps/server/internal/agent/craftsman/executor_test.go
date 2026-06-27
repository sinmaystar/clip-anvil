package craftsman

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/cozelooptrace"
	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestCraftsmanExecutorRunsGraphAndMarksTaskSucceeded(t *testing.T) {
	runtime := &fakeCraftsmanExecutorRuntime{}
	graph := fakeCraftsmanRunner{output: GraphOutput{
		AssistantText: "Craftsman 已完成 RenderPlan。",
		Strategy:      Strategy{Strategy: "方向", PreviewPrompt: "prompt"},
		WorkerTask:    db.AgentTask{ID: uuidWithByte(20), TaskType: "worker_generation"},
		Metadata:      map[string]any{"checkpoint_key": "craftsman:key"},
	}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: &graph})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(3),
		TaskID:      uuidWithByte(4),
		ShotID:      uuidWithByte(2),
		Input:       []byte(`{"parent_tool_call_id":"producer-dispatch-call"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !runtime.running || !runtime.succeeded {
		t.Fatalf("running=%v succeeded=%v", runtime.running, runtime.succeeded)
	}
	var output map[string]any
	if err := json.Unmarshal(runtime.output, &output); err != nil {
		t.Fatal(err)
	}
	if output["worker_task_id"] == "" {
		t.Fatalf("output = %#v", output)
	}
	wantCheckpoint := "agent:eino:craftsman_generation:01000000-0000-0000-0000-000000000000:03000000-0000-0000-0000-000000000000:04000000-0000-0000-0000-000000000000"
	if graph.runOptions.CheckPointID != wantCheckpoint {
		t.Fatalf("checkpoint id = %q, want %q", graph.runOptions.CheckPointID, wantCheckpoint)
	}
	if runtime.threadCheckpoint != wantCheckpoint {
		t.Fatalf("thread checkpoint = %q, want %q", runtime.threadCheckpoint, wantCheckpoint)
	}
	if len(runtime.appended) != 1 || runtime.appended[0].Role != "assistant" || runtime.appended[0].MessageType != "text" || !strings.Contains(string(runtime.appended[0].Content), "Craftsman 已完成 RenderPlan") {
		t.Fatalf("assistant message not persisted: %#v", runtime.appended)
	}
}

func TestCraftsmanExecutorWakesProducerWhenWaitingForProducer(t *testing.T) {
	runtime := &fakeCraftsmanExecutorRuntime{}
	producerEnqueuer := &fakeProducerTaskEnqueuer{}
	graph := fakeCraftsmanRunner{output: GraphOutput{
		AssistantText: "RenderPlan 已创建，等待 Producer 审批。",
		Metadata:      map[string]any{"checkpoint_key": "craftsman:key"},
	}}
	executor := NewExecutor(ExecutorConfig{
		Runtime:          runtime,
		Graph:            &graph,
		ProducerEnqueuer: producerEnqueuer,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(30),
		TaskID:      uuidWithByte(4),
		ShotID:      uuidWithByte(2),
		Input:       []byte(`{"target_phase":"preview_image","execution_policy":"wait_for_producer","producer_thread_id":"03000000-0000-0000-0000-000000000000","producer_task_id":"04000000-0000-0000-0000-000000000000"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.createdTasks) != 1 {
		t.Fatalf("created producer tasks = %d", len(runtime.createdTasks))
	}
	task := runtime.createdTasks[0]
	if task.Role != "producer" || task.TaskType != "producer_turn" {
		t.Fatalf("producer wake task = %#v", task)
	}
	if task.ThreadID != uuidWithByte(3) || task.ScopeType != "workspace" {
		t.Fatalf("producer wake task scope/thread = %#v", task)
	}
	var input map[string]any
	if err := json.Unmarshal(task.Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["trigger"] != "craftsman_render_plan_ready" || input["craftsman_task_id"] != "04000000-0000-0000-0000-000000000000" {
		t.Fatalf("producer wake input = %#v", input)
	}
	if input["render_plan_id"] != "5a000000-0000-0000-0000-000000000000" {
		t.Fatalf("producer wake input missing render_plan_id: %#v", input)
	}
	if _, ok := input["trigger_message_id"]; ok {
		t.Fatalf("producer wake input should not depend on a synthetic producer user message: %#v", input)
	}
	if _, ok := input["trigger_message_seq"]; ok {
		t.Fatalf("producer wake input should not depend on a synthetic producer user message seq: %#v", input)
	}
	if len(runtime.appended) != 1 {
		t.Fatalf("appended messages = %#v", runtime.appended)
	}
	if runtime.appended[0].ThreadID == uuidWithByte(3) || runtime.appended[0].Role == "user" {
		t.Fatalf("craftsman should not append synthetic user messages to producer thread: %#v", runtime.appended[0])
	}
	if len(runtime.signals) != 1 {
		t.Fatalf("signals = %#v", runtime.signals)
	}
	signal := runtime.signals[0]
	if signal.SignalType != "craftsman_render_plan_ready" ||
		signal.RenderPlanID != uuidWithByte(90) ||
		signal.MessageID.Valid ||
		signal.DedupeKey != "craftsman_render_plan_ready:5a000000-0000-0000-0000-000000000000" {
		t.Fatalf("signal = %#v", signal)
	}
	if len(producerEnqueuer.tasks) != 1 || producerEnqueuer.tasks[0].ID != task.ID {
		t.Fatalf("enqueued producer tasks = %#v", producerEnqueuer.tasks)
	}
}

func TestCraftsmanExecutorDoesNotWakeProducerWhenProducerWaitingForUser(t *testing.T) {
	runtime := &fakeCraftsmanExecutorRuntime{
		activeTasks: []db.AgentTask{
			{ID: uuidWithByte(91), Role: "producer", TaskType: "producer_turn", Status: "waiting_for_user"},
		},
	}
	producerEnqueuer := &fakeProducerTaskEnqueuer{}
	graph := fakeCraftsmanRunner{output: GraphOutput{
		AssistantText: "RenderPlan 已创建，等待 Producer 审批。",
		Metadata:      map[string]any{"checkpoint_key": "craftsman:key"},
	}}
	executor := NewExecutor(ExecutorConfig{
		Runtime:          runtime,
		Graph:            &graph,
		ProducerEnqueuer: producerEnqueuer,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(30),
		TaskID:      uuidWithByte(4),
		ShotID:      uuidWithByte(2),
		Input:       []byte(`{"target_phase":"preview_image","execution_policy":"wait_for_producer","producer_thread_id":"03000000-0000-0000-0000-000000000000","producer_task_id":"04000000-0000-0000-0000-000000000000"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.signals) != 1 {
		t.Fatalf("signals = %#v", runtime.signals)
	}
	if len(runtime.createdTasks) != 0 || len(producerEnqueuer.tasks) != 0 {
		t.Fatalf("created tasks = %#v, enqueued = %#v", runtime.createdTasks, producerEnqueuer.tasks)
	}
}

func TestCraftsmanExecutorDoesNotWakeProducerWhenProducerRunning(t *testing.T) {
	runtime := &fakeCraftsmanExecutorRuntime{
		activeTasks: []db.AgentTask{
			{ID: uuidWithByte(91), Role: "producer", TaskType: "producer_turn", Status: "running"},
		},
	}
	producerEnqueuer := &fakeProducerTaskEnqueuer{}
	graph := fakeCraftsmanRunner{output: GraphOutput{
		AssistantText: "RenderPlan 已创建，等待 Producer 审批。",
		Metadata:      map[string]any{"checkpoint_key": "craftsman:key"},
	}}
	executor := NewExecutor(ExecutorConfig{
		Runtime:          runtime,
		Graph:            &graph,
		ProducerEnqueuer: producerEnqueuer,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(30),
		TaskID:      uuidWithByte(4),
		ShotID:      uuidWithByte(2),
		Input:       []byte(`{"target_phase":"preview_image","execution_policy":"wait_for_producer","producer_thread_id":"03000000-0000-0000-0000-000000000000","producer_task_id":"04000000-0000-0000-0000-000000000000"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.signals) != 1 {
		t.Fatalf("signals = %#v", runtime.signals)
	}
	if len(runtime.createdTasks) != 0 || len(producerEnqueuer.tasks) != 0 {
		t.Fatalf("created tasks = %#v, enqueued = %#v", runtime.createdTasks, producerEnqueuer.tasks)
	}
}

func TestCraftsmanExecutorFailsWhenWaitingForProducerWithoutRenderPlan(t *testing.T) {
	runtime := &fakeCraftsmanExecutorRuntime{renderPlanErr: errors.New("render plan not found")}
	producerEnqueuer := &fakeProducerTaskEnqueuer{}
	graph := fakeCraftsmanRunner{output: GraphOutput{
		AssistantText: "我已经准备好让 Producer 处理。",
		Metadata:      map[string]any{"checkpoint_key": "craftsman:key"},
	}}
	executor := NewExecutor(ExecutorConfig{
		Runtime:          runtime,
		Graph:            &graph,
		ProducerEnqueuer: producerEnqueuer,
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(30),
		TaskID:      uuidWithByte(4),
		ShotID:      uuidWithByte(2),
		Input:       []byte(`{"target_phase":"shot_video","execution_policy":"wait_for_producer","producer_thread_id":"03000000-0000-0000-0000-000000000000","producer_task_id":"04000000-0000-0000-0000-000000000000"}`),
	})
	if err == nil {
		t.Fatal("RunTask succeeded, want failure")
	}
	if runtime.succeeded {
		t.Fatal("task was marked succeeded even though no RenderPlan was created")
	}
	if !runtime.failed || runtime.failedCode != "craftsman_ready_missing_render_plan" {
		t.Fatalf("failed=%v code=%q", runtime.failed, runtime.failedCode)
	}
	if len(runtime.createdTasks) != 0 || len(runtime.signals) != 0 || len(producerEnqueuer.tasks) != 0 {
		t.Fatalf("producer wake leaked: created=%#v signals=%#v enqueued=%#v", runtime.createdTasks, runtime.signals, producerEnqueuer.tasks)
	}
	if len(runtime.statusUpdates) != 1 || runtime.statusUpdates[0].Status != "failed" {
		t.Fatalf("shot status updates = %#v", runtime.statusUpdates)
	}
}

func TestCraftsmanExecutorPassesTaskInputToGraph(t *testing.T) {
	runtime := &fakeCraftsmanExecutorRuntime{}
	graph := fakeCraftsmanRunner{output: GraphOutput{
		Strategy:   Strategy{Strategy: "方向", PreviewPrompt: "prompt"},
		WorkerTask: db.AgentTask{ID: uuidWithByte(20), TaskType: "worker_generation"},
		Metadata:   map[string]any{"checkpoint_key": "craftsman:key"},
	}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: &graph})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(3),
		TaskID:      uuidWithByte(4),
		ShotID:      uuidWithByte(2),
		Input:       []byte(`{"mode":"shot_video","parent_tool_call_id":"producer-dispatch-call","input_node_refs":["shot-01 preview image"],"requested_max_attempts":2}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if graph.input.Mode != "shot_video" || graph.input.MaxAttempts != 2 {
		t.Fatalf("graph input = %#v", graph.input)
	}
	if graph.input.ParentToolCallID != "producer-dispatch-call" {
		t.Fatalf("parent tool call = %q", graph.input.ParentToolCallID)
	}
	refs, _ := graph.input.WorkerParams["input_node_refs"].([]string)
	if len(refs) != 1 || refs[0] != "shot-01 preview image" {
		t.Fatalf("worker params = %#v", graph.input.WorkerParams)
	}
}

func TestCraftsmanExecutorPassesTraceCallbacksToGraph(t *testing.T) {
	runtime := &fakeCraftsmanExecutorRuntime{}
	graph := fakeCraftsmanRunner{output: GraphOutput{
		Strategy:   Strategy{Strategy: "方向", PreviewPrompt: "prompt"},
		WorkerTask: db.AgentTask{ID: uuidWithByte(20), TaskType: "worker_generation"},
		Metadata:   map[string]any{"checkpoint_key": "craftsman:key"},
	}}
	traceCallback := callbacks.NewHandlerBuilder().Build()
	executor := NewExecutor(ExecutorConfig{
		Runtime:        runtime,
		Graph:          &graph,
		TraceCallbacks: []callbacks.Handler{traceCallback},
	})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(3),
		TaskID:      uuidWithByte(4),
		ShotID:      uuidWithByte(2),
		Input:       []byte(`{"parent_tool_call_id":"producer-dispatch-call"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(graph.runOptions.Callbacks) != 1 {
		t.Fatalf("callbacks len = %d, want 1", len(graph.runOptions.Callbacks))
	}
	if got := traceAttribute(graph.ctx, "clipanvil.agent.role"); got != "craftsman" {
		t.Fatalf("trace role = %q, want craftsman", got)
	}
}

func TestCraftsmanExecutorPersistsNativeToolTrace(t *testing.T) {
	runtime := &fakeCraftsmanExecutorRuntime{}
	graph := fakeCraftsmanRunner{output: GraphOutput{
		Strategy: Strategy{Strategy: "方向", PreviewPrompt: "prompt"},
		Metadata: map[string]any{"checkpoint_key": "craftsman:key"},
		SameTurnMessages: []CraftsmanSameTurnMessage{
			{
				Role:          "assistant",
				MessageType:   "tool_call",
				Content:       "需要先提交 render plan。",
				ToolCallID:    "call-render-plan",
				ToolName:      "upsert_render_plan",
				ToolArguments: map[string]any{"mode": "create", "shot_id": "shot-01"},
			},
			{
				Role:        "tool",
				MessageType: "tool_result",
				Content:     "已创建 RenderPlan，等待 Producer 审批。",
				ToolCallID:  "call-render-plan",
				ToolName:    "upsert_render_plan",
			},
		},
	}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: &graph})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(3),
		TaskID:      uuidWithByte(4),
		ShotID:      uuidWithByte(2),
		Input:       []byte(`{"parent_tool_call_id":"producer-dispatch-call"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"tool_call", "tool_result"}
	if got := runtime.appendedMessageTypes(); !equalStringSlices(got, want) {
		t.Fatalf("appended message types = %#v, want %#v", got, want)
	}
	if string(runtime.appended[0].RawMessage) == "" || string(runtime.appended[1].RawMessage) == "" {
		t.Fatalf("raw messages were not persisted: %#v", runtime.appended)
	}
	if !strings.Contains(string(runtime.appended[0].RawMessage), `"parent_tool_call_id":"producer-dispatch-call"`) {
		t.Fatalf("raw message missing parent tool call: %s", runtime.appended[0].RawMessage)
	}
	if runtime.appended[0].Role != "assistant" || runtime.appended[1].Role != "tool" {
		t.Fatalf("roles = %q/%q", runtime.appended[0].Role, runtime.appended[1].Role)
	}
	if len(runtime.updated) != 1 || !strings.Contains(string(runtime.updated[0].Content), `"status":"succeeded"`) {
		t.Fatalf("updated tool status = %#v", runtime.updated)
	}
}

func TestCraftsmanExecutorMarksShotFailedWhenGraphFails(t *testing.T) {
	runtime := &fakeCraftsmanExecutorRuntime{}
	graph := fakeCraftsmanRunner{err: errors.New("model rejected tool calling")}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: &graph})

	err := executor.RunTask(context.Background(), RunTaskInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(3),
		TaskID:      uuidWithByte(4),
		ShotID:      uuidWithByte(2),
		Input:       []byte(`{"parent_tool_call_id":"producer-dispatch-call"}`),
	})
	if err == nil {
		t.Fatal("RunTask succeeded, want failure")
	}
	if len(runtime.statusUpdates) != 1 {
		t.Fatalf("status updates = %#v", runtime.statusUpdates)
	}
	update := runtime.statusUpdates[0]
	if update.ID != uuidWithByte(2) || update.WorkspaceID != uuidWithByte(1) || update.Status != "failed" {
		t.Fatalf("status update = %#v", update)
	}
}

type fakeCraftsmanRunner struct {
	output     GraphOutput
	err        error
	input      GraphInput
	runOptions agenteino.RunOptions
	ctx        context.Context
}

func (f *fakeCraftsmanRunner) Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error) {
	f.ctx = ctx
	f.input = input
	if len(options) > 0 {
		f.runOptions = options[0]
	}
	return f.output, f.err
}

func traceAttribute(ctx context.Context, key string) string {
	for _, attr := range cozelooptrace.AttributesFromContext(ctx) {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
}

type fakeCraftsmanExecutorRuntime struct {
	running          bool
	succeeded        bool
	failed           bool
	failedCode       string
	failedMessage    string
	output           []byte
	threadCheckpoint string
	appendSeq        int64
	appended         []db.AgentMessage
	updated          []db.AgentMessage
	statusUpdates    []db.UpdateShotStatusParams
	createdTasks     []db.AgentTask
	activeTasks      []db.AgentTask
	signals          []db.ProducerPendingSignal
	renderPlanErr    error
}

func (f *fakeCraftsmanExecutorRuntime) MarkTaskRunning(context.Context, pgtype.UUID) (db.AgentTask, error) {
	f.running = true
	return db.AgentTask{}, nil
}

func (f *fakeCraftsmanExecutorRuntime) MarkTaskSucceeded(_ context.Context, _ pgtype.UUID, output []byte) (db.AgentTask, error) {
	f.succeeded = true
	f.output = output
	return db.AgentTask{}, nil
}

func (f *fakeCraftsmanExecutorRuntime) MarkTaskFailed(_ context.Context, _ pgtype.UUID, code, message string) (db.AgentTask, error) {
	f.failed = true
	f.failedCode = code
	f.failedMessage = message
	return db.AgentTask{}, nil
}

func (f *fakeCraftsmanExecutorRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	f.appendSeq++
	msg := db.AgentMessage{
		ID:          uuidWithByte(byte(40 + f.appendSeq)),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		MessageType: params.MessageType,
		Content:     params.Content,
		RawMessage:  params.RawMessage,
		TaskID:      params.TaskID,
		Seq:         f.appendSeq,
	}
	f.appended = append(f.appended, msg)
	return msg, nil
}

func (f *fakeCraftsmanExecutorRuntime) UpdateMessage(_ context.Context, params agentruntime.UpdateMessageParams) (db.AgentMessage, error) {
	for index, msg := range f.appended {
		if msg.ID == params.ID {
			msg.Content = params.Content
			msg.RawMessage = params.RawMessage
			msg.EventID = params.EventID
			f.appended[index] = msg
			f.updated = append(f.updated, msg)
			return msg, nil
		}
	}
	return db.AgentMessage{}, ErrInvalidInput
}

func (f *fakeCraftsmanExecutorRuntime) CreateEvent(context.Context, agentruntime.CreateEventParams) (db.AgentEvent, error) {
	return db.AgentEvent{}, nil
}

func (f *fakeCraftsmanExecutorRuntime) CreateProducerPendingSignal(_ context.Context, params agentruntime.CreateProducerPendingSignalParams) (db.ProducerPendingSignal, error) {
	signal := db.ProducerPendingSignal{
		ID:               uuidWithByte(byte(80 + len(f.signals))),
		WorkspaceID:      params.WorkspaceID,
		ProducerThreadID: params.ProducerThreadID,
		SourceRole:       params.SourceRole,
		SourceTaskID:     params.SourceTaskID,
		SourceThreadID:   params.SourceThreadID,
		SignalType:       params.SignalType,
		ScopeType:        params.ScopeType,
		ScopeID:          params.ScopeID,
		RenderPlanID:     params.RenderPlanID,
		MessageID:        params.MessageID,
		Status:           "pending",
		Priority:         params.Priority,
		DedupeKey:        params.DedupeKey,
		Payload:          params.Payload,
	}
	f.signals = append(f.signals, signal)
	return signal, nil
}

func (f *fakeCraftsmanExecutorRuntime) GetLatestRenderPlanByTaskScopePhase(_ context.Context, params db.GetLatestRenderPlanByTaskScopePhaseParams) (db.RenderPlan, error) {
	if f.renderPlanErr != nil {
		return db.RenderPlan{}, f.renderPlanErr
	}
	return db.RenderPlan{
		ID:              uuidWithByte(90),
		WorkspaceID:     params.WorkspaceID,
		ScopeType:       params.ScopeType,
		ScopeID:         params.ScopeID,
		TargetPhase:     params.TargetPhase,
		Status:          "waiting_for_approval",
		CreatedByTaskID: params.CreatedByTaskID,
	}, nil
}

func (f *fakeCraftsmanExecutorRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	task := db.AgentTask{
		ID:          uuidWithByte(byte(70 + len(f.createdTasks))),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		ScopeType:   params.ScopeType,
		ScopeID:     params.ScopeID,
		TaskType:    params.TaskType,
		Status:      "queued",
		MaxAttempts: params.MaxAttempts,
		Input:       params.Input,
	}
	f.createdTasks = append(f.createdTasks, task)
	return task, nil
}

func (f *fakeCraftsmanExecutorRuntime) ListActiveAgentTasksByWorkspace(context.Context, pgtype.UUID) ([]db.AgentTask, error) {
	return f.activeTasks, nil
}

func (f *fakeCraftsmanExecutorRuntime) SetThreadCheckpoint(_ context.Context, _ pgtype.UUID, checkpointKey string) (db.AgentThread, error) {
	f.threadCheckpoint = checkpointKey
	return db.AgentThread{CurrentCheckpointKey: pgtype.Text{String: checkpointKey, Valid: checkpointKey != ""}}, nil
}

func (f *fakeCraftsmanExecutorRuntime) UpdateShotStatus(_ context.Context, params db.UpdateShotStatusParams) (db.Shot, error) {
	f.statusUpdates = append(f.statusUpdates, params)
	return db.Shot{ID: params.ID, WorkspaceID: params.WorkspaceID, Status: params.Status}, nil
}

func (f *fakeCraftsmanExecutorRuntime) appendedMessageTypes() []string {
	out := make([]string, 0, len(f.appended))
	for _, msg := range f.appended {
		out = append(out, msg.MessageType)
	}
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type fakeProducerTaskEnqueuer struct {
	tasks []db.AgentTask
}

func (f *fakeProducerTaskEnqueuer) EnqueueProducerTask(_ context.Context, task db.AgentTask) {
	f.tasks = append(f.tasks, task)
}
