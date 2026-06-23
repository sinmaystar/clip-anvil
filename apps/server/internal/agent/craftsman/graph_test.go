package craftsman

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/compose"
	"github.com/jackc/pgx/v5/pgtype"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	agentworker "github.com/sinmaystar/clip-anvil/internal/agent/worker"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestCraftsmanGraphPersistsStrategyAndCreatesWorkerTask(t *testing.T) {
	runtime := &fakeCraftsmanGraphRuntime{}
	workerEnqueuer := &fakeWorkerEnqueuer{}
	graph, err := NewGraph(GraphConfig{
		Loader: fakeGraphLoader{context: Context{
			Input: GraphInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(3), TaskID: uuidWithByte(4), ShotID: uuidWithByte(2), MaxAttempts: 3},
			Shot:  db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", Title: "开场"},
			Text:  "shot-01 开场",
		}},
		Responder: StaticResponder{Strategy: Strategy{
			Strategy:      "明亮商品特写",
			PreviewPrompt: "A bright product close-up",
			Params:        map[string]any{"size": "1024x1024"},
		}},
		Runtime:        runtime,
		WorkerEnqueuer: workerEnqueuer,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), GraphInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(3), TaskID: uuidWithByte(4), ShotID: uuidWithByte(2), MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	if out.WorkerTask.TaskType != "worker_generation" {
		t.Fatalf("worker task = %#v", out.WorkerTask)
	}
	if len(runtime.messages) != 1 || runtime.messages[0].Role != "assistant" {
		t.Fatalf("messages = %#v", runtime.messages)
	}
	if runtime.checkpointKey != "craftsman:01000000-0000-0000-0000-000000000000:02000000-0000-0000-0000-000000000000:04000000-0000-0000-0000-000000000000" {
		t.Fatalf("checkpoint key = %q", runtime.checkpointKey)
	}
	var workerInput agentworker.GenerationInput
	if err := json.Unmarshal(runtime.createdTask.Input, &workerInput); err != nil {
		t.Fatal(err)
	}
	if workerInput.Prompt != "A bright product close-up" || workerInput.ShotID == "" || workerInput.CraftsmanTaskID == "" {
		t.Fatalf("worker input = %#v", workerInput)
	}
	if len(workerEnqueuer.tasks) != 1 {
		t.Fatalf("enqueued = %d", len(workerEnqueuer.tasks))
	}
}

func TestCraftsmanGraphCompileCapturesGraphInfo(t *testing.T) {
	registry := agenteino.NewGraphInfoRegistry()
	_, err := NewGraph(GraphConfig{
		Loader: fakeGraphLoader{context: Context{
			Input: GraphInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(3), TaskID: uuidWithByte(4), ShotID: uuidWithByte(2), MaxAttempts: 3},
			Shot:  db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01"},
		}},
		Responder: StaticResponder{Strategy: Strategy{Strategy: "方向", PreviewPrompt: "prompt"}},
		Runtime:   &fakeCraftsmanGraphRuntime{},
		CompileCallbacks: []compose.GraphCompileCallback{
			registry.CompileCallback(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	info, ok := registry.Get("craftsman_generation")
	if !ok {
		t.Fatal("craftsman graph info was not captured")
	}
	for _, node := range []string{"load_shot_context", "draft_generation_strategy"} {
		if _, ok := info.Nodes[node]; !ok {
			t.Fatalf("node %q missing from graph info", node)
		}
	}
}

func TestCraftsmanGraphCreatesShotVideoWorkerTask(t *testing.T) {
	runtime := &fakeCraftsmanGraphRuntime{}
	graph, err := NewGraph(GraphConfig{
		Loader: fakeGraphLoader{context: Context{
			Input: GraphInput{
				WorkspaceID: uuidWithByte(1),
				ThreadID:    uuidWithByte(3),
				TaskID:      uuidWithByte(4),
				ShotID:      uuidWithByte(2),
				MaxAttempts: 3,
				WorkerParams: map[string]any{
					"mode":            "shot_video",
					"input_node_refs": []string{"shot-01 preview image"},
				},
			},
			Shot: db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", SortOrder: 1, Title: "开场"},
			Text: "shot-01 开场",
		}},
		Responder: StaticResponder{Strategy: Strategy{
			Strategy:      "产品开场动态镜头",
			PreviewPrompt: "Animate the accepted preview into a smooth product shot",
			Model:         ModelSpec{Provider: "mock", ModelID: "mock-video"},
			Params:        map[string]any{"duration_sec": float64(4)},
		}},
		Runtime: runtime,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Run(context.Background(), GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(3),
		TaskID:      uuidWithByte(4),
		ShotID:      uuidWithByte(2),
		MaxAttempts: 3,
		WorkerParams: map[string]any{
			"mode":            "shot_video",
			"input_node_refs": []string{"shot-01 preview image"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var workerInput agentworker.GenerationInput
	if err := json.Unmarshal(runtime.createdTask.Input, &workerInput); err != nil {
		t.Fatal(err)
	}
	if workerInput.Mode != "shot_video" || workerInput.OutputType != "video" || workerInput.OperationType != "image_to_video" {
		t.Fatalf("worker input = %#v", workerInput)
	}
	if workerInput.Prompt != "Animate the accepted preview into a smooth product shot" {
		t.Fatalf("prompt = %q", workerInput.Prompt)
	}
	if len(workerInput.InputNodeRefs) != 1 || workerInput.InputNodeRefs[0] != "shot-01 preview image" {
		t.Fatalf("input refs = %#v", workerInput.InputNodeRefs)
	}
}

func TestCraftsmanGraphRetriesInvalidStrategy(t *testing.T) {
	responder := &sequenceResponder{strategies: []Strategy{
		{Strategy: "方向"},
		{Strategy: "方向", PreviewPrompt: "prompt"},
	}}
	graph, err := NewGraph(GraphConfig{
		Loader: fakeGraphLoader{context: Context{
			Input: GraphInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(3), TaskID: uuidWithByte(4), ShotID: uuidWithByte(2), MaxAttempts: 3},
			Shot:  db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01"},
			Text:  "shot",
		}},
		Responder:      responder,
		Runtime:        &fakeCraftsmanGraphRuntime{},
		WorkerEnqueuer: &fakeWorkerEnqueuer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graph.Run(context.Background(), GraphInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(3), TaskID: uuidWithByte(4), ShotID: uuidWithByte(2), MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	if responder.calls != 2 {
		t.Fatalf("calls = %d", responder.calls)
	}
}

type fakeGraphLoader struct {
	context Context
}

func (f fakeGraphLoader) Load(context.Context, GraphInput) (Context, error) {
	return f.context, nil
}

type sequenceResponder struct {
	strategies []Strategy
	calls      int
}

func (r *sequenceResponder) DraftPreviewStrategy(context.Context, Context) (Strategy, map[string]any, error) {
	defer func() { r.calls++ }()
	return r.strategies[r.calls], nil, nil
}

type fakeCraftsmanGraphRuntime struct {
	messages      []agentruntime.AppendMessageParams
	checkpointKey string
	createdTask   db.AgentTask
}

func (f *fakeCraftsmanGraphRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	f.messages = append(f.messages, params)
	return db.AgentMessage{ID: uuidWithByte(50), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, Role: params.Role, MessageType: params.MessageType, Content: params.Content}, nil
}

func (f *fakeCraftsmanGraphRuntime) UpsertCheckpoint(_ context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error) {
	f.checkpointKey = params.Key
	return db.EinoCheckpoint{Key: params.Key, WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, TaskID: params.TaskID}, nil
}

func (f *fakeCraftsmanGraphRuntime) SetThreadCheckpoint(context.Context, pgtype.UUID, string) (db.AgentThread, error) {
	return db.AgentThread{}, nil
}

func (f *fakeCraftsmanGraphRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	task := db.AgentTask{
		ID:          uuidWithByte(60),
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
	f.createdTask = task
	return task, nil
}

func (f *fakeCraftsmanGraphRuntime) CreateEvent(context.Context, agentruntime.CreateEventParams) (db.AgentEvent, error) {
	return db.AgentEvent{}, nil
}

type fakeWorkerEnqueuer struct {
	tasks []db.AgentTask
}

func (f *fakeWorkerEnqueuer) EnqueueWorkerTask(context.Context, db.AgentTask) {
	f.tasks = append(f.tasks, db.AgentTask{})
}
