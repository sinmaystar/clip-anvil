package reviewer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/cozelooptrace"
	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestReviewerExecutorRunsReviewerTurnTask(t *testing.T) {
	runtime := &fakeReviewerExecutorRuntime{}
	graph := &fakeReviewerRunner{output: GraphOutput{
		Decision: ReviewDecision{Status: ReviewStatusAccepted},
		Result:   ReviewResult{Critique: "Reviewer 评审通过，产品一致性可接受。"},
	}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})
	input := TaskInput{
		TargetPhase:       TargetPhasePreviewImage,
		ShotID:            uuidString(uuidWithByte(2)),
		NodeID:            uuidString(uuidWithByte(3)),
		ArtifactVersionID: uuidString(uuidWithByte(4)),
		AttemptNo:         1,
		MaxAttempts:       3,
	}
	raw, _ := json.Marshal(input)
	task := db.AgentTask{
		ID:          uuidWithByte(9),
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(8),
		Role:        "reviewer",
		TaskType:    "reviewer_turn",
		Input:       raw,
	}

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if !runtime.running || !runtime.succeeded {
		t.Fatalf("runtime running=%v succeeded=%v", runtime.running, runtime.succeeded)
	}
	if graph.input.Task.NodeID != input.NodeID {
		t.Fatalf("graph input = %#v", graph.input)
	}
	wantCheckpoint := "agent:eino:reviewer_preview:01000000-0000-0000-0000-000000000000:08000000-0000-0000-0000-000000000000:09000000-0000-0000-0000-000000000000"
	if graph.runOptions.CheckPointID != wantCheckpoint {
		t.Fatalf("checkpoint id = %q, want %q", graph.runOptions.CheckPointID, wantCheckpoint)
	}
	if runtime.threadCheckpoint != wantCheckpoint {
		t.Fatalf("thread checkpoint = %q, want %q", runtime.threadCheckpoint, wantCheckpoint)
	}
	if len(runtime.appended) != 1 || runtime.appended[0].Role != "assistant" || runtime.appended[0].MessageType != "text" || !strings.Contains(string(runtime.appended[0].Content), "Reviewer 评审通过") {
		t.Fatalf("assistant message not persisted: %#v", runtime.appended)
	}
}

func TestReviewerExecutorSignalsProducerWhenReviewCompleted(t *testing.T) {
	runtime := &fakeReviewerExecutorRuntime{}
	producerEnqueuer := &fakeProducerTaskEnqueuer{}
	graph := &fakeReviewerRunner{output: GraphOutput{
		Record:   db.ReviewRecord{ID: uuidWithByte(40)},
		Decision: ReviewDecision{Status: ReviewStatusRejected, ShouldRetry: true},
		Result:   ReviewResult{Critique: "Reviewer 发现阻塞问题，需要 Producer 决策。"},
	}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph, ProducerEnqueuer: producerEnqueuer})
	raw, _ := json.Marshal(TaskInput{
		ReviewTask:        ReviewTaskPreviewImage,
		TargetPhase:       TargetPhasePreviewImage,
		ShotID:            uuidString(uuidWithByte(2)),
		NodeID:            uuidString(uuidWithByte(3)),
		ArtifactVersionID: uuidString(uuidWithByte(4)),
		ProducerThreadID:  uuidString(uuidWithByte(7)),
		ProducerTaskID:    uuidString(uuidWithByte(6)),
		AttemptNo:         1,
		MaxAttempts:       3,
	})
	task := db.AgentTask{
		ID:          uuidWithByte(9),
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(8),
		Role:        "reviewer",
		ScopeType:   "shot",
		ScopeID:     uuidWithByte(2),
		TaskType:    "reviewer_turn",
		Input:       raw,
	}

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}

	if len(runtime.signals) != 1 {
		t.Fatalf("signals = %#v", runtime.signals)
	}
	signal := runtime.signals[0]
	if signal.SignalType != "review_completed" ||
		signal.ProducerThreadID != uuidWithByte(7) ||
		signal.SourceRole != "reviewer" ||
		signal.SourceTaskID != uuidWithByte(9) ||
		signal.ScopeType != "shot" ||
		signal.ScopeID != uuidWithByte(2) ||
		signal.DedupeKey != "review_completed:28000000-0000-0000-0000-000000000000" {
		t.Fatalf("signal = %#v", signal)
	}
	if len(runtime.createdTasks) != 1 || len(producerEnqueuer.tasks) != 1 {
		t.Fatalf("created tasks = %#v enqueued=%#v", runtime.createdTasks, producerEnqueuer.tasks)
	}
}

func TestReviewerExecutorPassesTraceCallbacksToGraph(t *testing.T) {
	runtime := &fakeReviewerExecutorRuntime{}
	graph := &fakeReviewerRunner{output: GraphOutput{Decision: ReviewDecision{Status: ReviewStatusAccepted}}}
	traceCallback := callbacks.NewHandlerBuilder().Build()
	executor := NewExecutor(ExecutorConfig{
		Runtime:        runtime,
		Graph:          graph,
		TraceCallbacks: []callbacks.Handler{traceCallback},
	})
	input := TaskInput{
		TargetPhase:       TargetPhasePreviewImage,
		ShotID:            uuidString(uuidWithByte(2)),
		NodeID:            uuidString(uuidWithByte(3)),
		ArtifactVersionID: uuidString(uuidWithByte(4)),
		AttemptNo:         1,
		MaxAttempts:       3,
	}
	raw, _ := json.Marshal(input)
	task := db.AgentTask{
		ID:          uuidWithByte(9),
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(8),
		Role:        "reviewer",
		TaskType:    "reviewer_turn",
		Input:       raw,
	}

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}

	if len(graph.runOptions.Callbacks) != 1 {
		t.Fatalf("callbacks len = %d, want 1", len(graph.runOptions.Callbacks))
	}
	if got := traceAttribute(graph.ctx, "clipanvil.agent.role"); got != "reviewer" {
		t.Fatalf("trace role = %q, want reviewer", got)
	}
}

func TestReviewerExecutorPersistsNativeToolTrace(t *testing.T) {
	runtime := &fakeReviewerExecutorRuntime{}
	graph := &fakeReviewerRunner{output: GraphOutput{
		Decision: ReviewDecision{Status: ReviewStatusAccepted},
		SameTurnMessages: []ReviewerSameTurnMessage{
			{
				Role:          "assistant",
				MessageType:   "tool_call",
				Content:       "需要提交评审结果。",
				ToolCallID:    "call-review",
				ToolName:      "submit_review_result",
				ToolArguments: map[string]any{"verdict": ReviewStatusAccepted},
			},
			{
				Role:        "tool",
				MessageType: "tool_result",
				Content:     "已提交 Reviewer 评审结果。",
				ToolCallID:  "call-review",
				ToolName:    "submit_review_result",
			},
		},
	}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})
	raw, _ := json.Marshal(TaskInput{
		TargetPhase:       TargetPhasePreviewImage,
		ShotID:            uuidString(uuidWithByte(2)),
		NodeID:            uuidString(uuidWithByte(3)),
		ArtifactVersionID: uuidString(uuidWithByte(4)),
		AttemptNo:         1,
		MaxAttempts:       3,
	})
	task := db.AgentTask{
		ID:          uuidWithByte(9),
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(8),
		Role:        "reviewer",
		TaskType:    "reviewer_turn",
		Input:       raw,
	}

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}

	want := []string{"tool_call", "tool_result"}
	if got := runtime.appendedMessageTypes(); !equalStringSlices(got, want) {
		t.Fatalf("appended message types = %#v, want %#v", got, want)
	}
	if runtime.appended[0].Role != "assistant" || runtime.appended[1].Role != "tool" {
		t.Fatalf("roles = %q/%q", runtime.appended[0].Role, runtime.appended[1].Role)
	}
	if len(runtime.updated) != 1 || !strings.Contains(string(runtime.updated[0].Content), `"status":"succeeded"`) {
		t.Fatalf("updated tool status = %#v", runtime.updated)
	}
}

type fakeReviewerExecutorRuntime struct {
	running          bool
	succeeded        bool
	failed           bool
	threadCheckpoint string
	appendSeq        int64
	appended         []db.AgentMessage
	updated          []db.AgentMessage
	signals          []agentruntime.CreateProducerPendingSignalParams
	activeTasks      []db.AgentTask
	createdTasks     []db.AgentTask
	createdEvents    []agentruntime.CreateEventParams
}

func (f *fakeReviewerExecutorRuntime) MarkTaskRunning(context.Context, pgtype.UUID) (db.AgentTask, error) {
	f.running = true
	return db.AgentTask{}, nil
}

func (f *fakeReviewerExecutorRuntime) MarkTaskSucceeded(context.Context, pgtype.UUID, []byte) (db.AgentTask, error) {
	f.succeeded = true
	return db.AgentTask{}, nil
}

func (f *fakeReviewerExecutorRuntime) MarkTaskFailed(context.Context, pgtype.UUID, string, string) (db.AgentTask, error) {
	f.failed = true
	return db.AgentTask{}, nil
}

func (f *fakeReviewerExecutorRuntime) SetThreadCheckpoint(_ context.Context, _ pgtype.UUID, checkpointKey string) (db.AgentThread, error) {
	f.threadCheckpoint = checkpointKey
	return db.AgentThread{CurrentCheckpointKey: pgtype.Text{String: checkpointKey, Valid: checkpointKey != ""}}, nil
}

func (f *fakeReviewerExecutorRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	f.appendSeq++
	msg := db.AgentMessage{
		ID:          uuidWithByte(byte(60 + f.appendSeq)),
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

func (f *fakeReviewerExecutorRuntime) UpdateMessage(_ context.Context, params agentruntime.UpdateMessageParams) (db.AgentMessage, error) {
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

func (f *fakeReviewerExecutorRuntime) CreateProducerPendingSignal(_ context.Context, params agentruntime.CreateProducerPendingSignalParams) (db.ProducerPendingSignal, error) {
	f.signals = append(f.signals, params)
	return db.ProducerPendingSignal{
		ID:               uuidWithByte(byte(90 + len(f.signals))),
		WorkspaceID:      params.WorkspaceID,
		ProducerThreadID: params.ProducerThreadID,
		SourceRole:       params.SourceRole,
		SourceTaskID:     params.SourceTaskID,
		SourceThreadID:   params.SourceThreadID,
		SignalType:       params.SignalType,
		ScopeType:        params.ScopeType,
		ScopeID:          params.ScopeID,
		RenderPlanID:     params.RenderPlanID,
		Priority:         params.Priority,
		DedupeKey:        params.DedupeKey,
		Payload:          params.Payload,
	}, nil
}

func (f *fakeReviewerExecutorRuntime) ListActiveAgentTasksByWorkspace(context.Context, pgtype.UUID) ([]db.AgentTask, error) {
	return f.activeTasks, nil
}

func (f *fakeReviewerExecutorRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	task := db.AgentTask{
		ID:          uuidWithByte(byte(100 + len(f.createdTasks))),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		ScopeType:   params.ScopeType,
		ScopeID:     params.ScopeID,
		TaskType:    params.TaskType,
		MaxAttempts: params.MaxAttempts,
		Input:       params.Input,
	}
	f.createdTasks = append(f.createdTasks, task)
	return task, nil
}

func (f *fakeReviewerExecutorRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	f.createdEvents = append(f.createdEvents, params)
	return db.AgentEvent{ID: uuidWithByte(byte(110 + len(f.createdEvents))), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, TaskID: params.TaskID, EventType: params.EventType}, nil
}

func (f *fakeReviewerExecutorRuntime) appendedMessageTypes() []string {
	out := make([]string, 0, len(f.appended))
	for _, msg := range f.appended {
		out = append(out, msg.MessageType)
	}
	return out
}

type fakeReviewerRunner struct {
	input      GraphInput
	output     GraphOutput
	runOptions agenteino.RunOptions
	ctx        context.Context
}

func (f *fakeReviewerRunner) Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error) {
	f.ctx = ctx
	f.input = input
	if len(options) > 0 {
		f.runOptions = options[0]
	}
	return f.output, nil
}

func traceAttribute(ctx context.Context, key string) string {
	for _, attr := range cozelooptrace.AttributesFromContext(ctx) {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
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
