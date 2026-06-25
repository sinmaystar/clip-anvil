package reviewer

import (
	"context"
	"encoding/json"
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
	graph := &fakeReviewerRunner{output: GraphOutput{Decision: ReviewDecision{Status: ReviewStatusAccepted}}}
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
}

type fakeReviewerExecutorRuntime struct {
	running          bool
	succeeded        bool
	failed           bool
	threadCheckpoint string
	appendSeq        int64
	appended         []db.AgentMessage
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
