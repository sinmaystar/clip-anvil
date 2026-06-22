package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestWorkerCreatesPreviewNodeAndSubmitsGenerationIntent(t *testing.T) {
	store := &fakeWorkerStore{}
	runtime := &fakeWorkerRuntime{}
	productionService := &fakeProductionSubmitter{
		result: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1)},
			Job:     db.GenerationJob{ID: uuidWithByte(30), TargetNodeID: uuidWithByte(20), OperationType: "text_to_image", Status: db.JobStatusQueued},
			Version: db.ArtifactVersion{ID: uuidWithByte(40), NodeID: uuidWithByte(20), JobID: uuidWithByte(30), Status: db.JobStatusQueued},
		},
	}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Store: store, Production: productionService})

	task := workerTaskWithInput(t, GenerationInput{
		Mode:              "preview_image",
		ShotID:            uuidString(uuidWithByte(2)),
		ShotClientKey:     "shot-01",
		CraftsmanThreadID: uuidString(uuidWithByte(3)),
		CraftsmanTaskID:   uuidString(uuidWithByte(4)),
		Strategy:          "明亮商品特写",
		Prompt:            "A bright product close-up",
		Model:             ModelSpec{Provider: "volcengine", ModelID: "test-image"},
		Params:            map[string]any{"size": "1024x1024"},
		MaxAttempts:       3,
	})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if store.createdNode.Prompt != "A bright product close-up" {
		t.Fatalf("created node = %#v", store.createdNode)
	}
	if store.createdNode.NodeType != db.NodeTypeImage || store.createdNode.OperationType != "text_to_image" {
		t.Fatalf("created node = %#v", store.createdNode)
	}
	if !store.createdNode.ShotID.Valid || store.createdNode.ShotID != uuidWithByte(2) {
		t.Fatalf("shot id = %#v", store.createdNode.ShotID)
	}
	var metadata map[string]any
	if err := json.Unmarshal(store.createdNode.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["agent_artifact_kind"] != "preview_image" || metadata["worker_task_id"] == "" {
		t.Fatalf("metadata = %#v", metadata)
	}
	intent := productionService.intent
	if intent.OutputType != "image" || intent.OperationType != "text_to_image" {
		t.Fatalf("intent = %#v", intent)
	}
	if intent.RequestedBy.Type != "agent_worker" || intent.RequestedBy.ID != uuidString(task.ID) {
		t.Fatalf("requested by = %#v", intent.RequestedBy)
	}
	if runtime.succeededOutput.Status != "submitted" || runtime.succeededOutput.NodeID == "" {
		t.Fatalf("output = %#v", runtime.succeededOutput)
	}
}

func TestWorkerUsesExistingTargetNodeWhenProvided(t *testing.T) {
	store := &fakeWorkerStore{existingNode: db.MediaNode{ID: uuidWithByte(22), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeImage, OperationType: "text_to_image", ShotID: uuidWithByte(2)}}
	runtime := &fakeWorkerRuntime{}
	productionService := &fakeProductionSubmitter{result: production.RunResult{
		Node:    store.existingNode,
		Job:     db.GenerationJob{ID: uuidWithByte(30)},
		Version: db.ArtifactVersion{ID: uuidWithByte(40)},
	}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Store: store, Production: productionService})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:         "preview_image",
		ShotID:       uuidString(uuidWithByte(2)),
		TargetNodeID: uuidString(uuidWithByte(22)),
		Strategy:     "方向",
		Prompt:       "prompt",
		MaxAttempts:  3,
	})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if store.createNodeCalls != 0 {
		t.Fatalf("create node calls = %d", store.createNodeCalls)
	}
	if productionService.intent.TargetNodeID != uuidWithByte(22) {
		t.Fatalf("target node = %#v", productionService.intent.TargetNodeID)
	}
}

func TestWorkerRetriesSynchronousSubmitFailure(t *testing.T) {
	store := &fakeWorkerStore{}
	runtime := &fakeWorkerRuntime{}
	productionService := &fakeProductionSubmitter{
		failuresBeforeSuccess: 2,
		result: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1)},
			Job:     db.GenerationJob{ID: uuidWithByte(30)},
			Version: db.ArtifactVersion{ID: uuidWithByte(40)},
		},
	}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Store: store, Production: productionService})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:        "preview_image",
		ShotID:      uuidString(uuidWithByte(2)),
		Strategy:    "方向",
		Prompt:      "prompt",
		MaxAttempts: 3,
	})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if productionService.calls != 3 {
		t.Fatalf("calls = %d", productionService.calls)
	}
	if !runtime.succeeded {
		t.Fatal("task not marked succeeded")
	}
}

func workerTaskWithInput(t *testing.T, input GenerationInput) db.AgentTask {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return db.AgentTask{
		ID:          uuidWithByte(10),
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(3),
		Role:        "worker",
		ScopeType:   "shot",
		ScopeID:     uuidWithByte(2),
		TaskType:    "worker_generation",
		Status:      "queued",
		MaxAttempts: 3,
		Input:       raw,
	}
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}

type fakeWorkerRuntime struct {
	succeeded       bool
	succeededOutput GenerationOutput
}

func (f *fakeWorkerRuntime) MarkTaskRunning(context.Context, pgtype.UUID) (db.AgentTask, error) {
	return db.AgentTask{}, nil
}

func (f *fakeWorkerRuntime) MarkTaskSucceeded(_ context.Context, _ pgtype.UUID, output []byte) (db.AgentTask, error) {
	f.succeeded = true
	_ = json.Unmarshal(output, &f.succeededOutput)
	return db.AgentTask{}, nil
}

func (f *fakeWorkerRuntime) MarkTaskFailed(context.Context, pgtype.UUID, string, string) (db.AgentTask, error) {
	return db.AgentTask{}, nil
}

func (f *fakeWorkerRuntime) CreateEvent(context.Context, agentruntime.CreateEventParams) (db.AgentEvent, error) {
	return db.AgentEvent{}, nil
}

type fakeWorkerStore struct {
	createdNode     db.CreateAgentGenerationNodeParams
	createNodeCalls int
	existingNode    db.MediaNode
}

func (f *fakeWorkerStore) CreateAgentGenerationNode(_ context.Context, params db.CreateAgentGenerationNodeParams) (db.MediaNode, error) {
	f.createdNode = params
	f.createNodeCalls++
	return db.MediaNode{
		ID:             uuidWithByte(20),
		WorkspaceID:    params.WorkspaceID,
		NodeType:       params.NodeType,
		Title:          params.Title,
		Prompt:         params.Prompt,
		PromptTemplate: params.Prompt,
		OperationType:  params.OperationType,
		Status:         db.NodeStatusQueued,
		Source:         "agent",
		ShotID:         params.ShotID,
		Metadata:       params.Metadata,
	}, nil
}

func (f *fakeWorkerStore) GetMediaNodeByID(context.Context, pgtype.UUID) (db.MediaNode, error) {
	return f.existingNode, nil
}

type fakeProductionSubmitter struct {
	intent                production.GenerationIntent
	options               production.RunOptions
	result                production.RunResult
	calls                 int
	failuresBeforeSuccess int
}

func (f *fakeProductionSubmitter) SubmitGenerationIntent(_ context.Context, intent production.GenerationIntent, options production.RunOptions) (production.RunResult, error) {
	f.calls++
	f.intent = intent
	f.options = options
	if f.calls <= f.failuresBeforeSuccess {
		return production.RunResult{}, errors.New("temporary submit failure")
	}
	return f.result, nil
}
