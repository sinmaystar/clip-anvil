package craftsman

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

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

type fakeCraftsmanRunner struct {
	output GraphOutput
	err    error
	input  GraphInput
}

func (f *fakeCraftsmanRunner) Run(_ context.Context, input GraphInput) (GraphOutput, error) {
	f.input = input
	return f.output, f.err
}

type fakeCraftsmanExecutorRuntime struct {
	running   bool
	succeeded bool
	output    []byte
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
