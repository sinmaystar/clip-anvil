package craftsman

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

func TestCraftsmanExecutorRunsGraphAndMarksTaskSucceeded(t *testing.T) {
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
		Input:       []byte(`{"mode":"shot_video","input_node_refs":["shot-01 preview image"],"requested_max_attempts":2}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if graph.input.Mode != "shot_video" || graph.input.MaxAttempts != 2 {
		t.Fatalf("graph input = %#v", graph.input)
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
	output           []byte
	threadCheckpoint string
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

func (f *fakeCraftsmanExecutorRuntime) MarkTaskFailed(context.Context, pgtype.UUID, string, string) (db.AgentTask, error) {
	return db.AgentTask{}, nil
}

func (f *fakeCraftsmanExecutorRuntime) AppendMessage(context.Context, agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	return db.AgentMessage{}, nil
}

func (f *fakeCraftsmanExecutorRuntime) CreateEvent(context.Context, agentruntime.CreateEventParams) (db.AgentEvent, error) {
	return db.AgentEvent{}, nil
}

func (f *fakeCraftsmanExecutorRuntime) SetThreadCheckpoint(_ context.Context, _ pgtype.UUID, checkpointKey string) (db.AgentThread, error) {
	f.threadCheckpoint = checkpointKey
	return db.AgentThread{CurrentCheckpointKey: pgtype.Text{String: checkpointKey, Valid: checkpointKey != ""}}, nil
}
