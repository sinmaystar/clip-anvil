package composer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/cozelooptrace"
	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestComposerCreatesFinalVideoNodeAndSubmitsComposeIntent(t *testing.T) {
	sourceNode := db.MediaNode{ID: uuidWithByte(21), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeVideo, Title: "shot-01 shot video", CurrentVersionID: uuidWithByte(31), AssetID: uuidWithByte(41)}
	store := &fakeComposerStore{
		nodes: []db.MediaNode{sourceNode},
		versions: map[pgtype.UUID]db.ArtifactVersion{
			uuidWithByte(31): {ID: uuidWithByte(31), WorkspaceID: uuidWithByte(1), NodeID: uuidWithByte(21), AssetID: uuidWithByte(41), InputHash: "hash-1", Status: db.JobStatusSucceeded},
		},
		assets: map[pgtype.UUID]db.MediaAsset{
			uuidWithByte(41): {ID: uuidWithByte(41), WorkspaceID: uuidWithByte(1), Type: db.AssetTypeVideo, Mime: "video/mp4", StorageUrl: pgtype.Text{String: "workspace/final-input.mp4", Valid: true}},
		},
	}
	runtime := &fakeComposerRuntime{}
	productionService := &fakeComposerProduction{
		result: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(50), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeVideo},
			Job:     db.GenerationJob{ID: uuidWithByte(60), TargetNodeID: uuidWithByte(50), OperationType: "compose_final_video", Status: db.JobStatusQueued},
			Version: db.ArtifactVersion{ID: uuidWithByte(70), NodeID: uuidWithByte(50), JobID: uuidWithByte(60), Status: db.JobStatusQueued},
		},
	}
	graph, err := NewGraph(GraphConfig{Runtime: runtime, Store: store, Production: productionService, CheckPointStore: fakeEinoCheckpointStore{}})
	if err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})
	task := composerTaskWithInput(t, CompositionInput{VideoNodeRefs: []string{"shot-01 shot video"}})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if store.createdNode.NodeType != db.NodeTypeVideo || store.createdNode.OperationType != "compose_final_video" {
		t.Fatalf("created node = %#v", store.createdNode)
	}
	var metadata map[string]any
	if err := json.Unmarshal(store.createdNode.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["agent_artifact_kind"] != "final_video" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if productionService.intent.OutputType != "video" || productionService.intent.OperationType != "compose_final_video" {
		t.Fatalf("intent = %#v", productionService.intent)
	}
	if len(productionService.intent.InputRefs) != 1 || productionService.intent.InputRefs[0].NodeID != sourceNode.ID {
		t.Fatalf("input refs = %#v", productionService.intent.InputRefs)
	}
	if !runtime.succeeded {
		t.Fatal("composer task was not marked succeeded")
	}
}

func TestComposerExecutorPassesDeterministicCheckpointID(t *testing.T) {
	runtime := &fakeComposerRuntime{}
	graph := &fakeComposerRunner{output: GraphOutput{Output: CompositionOutput{Status: "submitted"}}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})
	task := composerTaskWithInput(t, CompositionInput{VideoNodeRefs: []string{"shot-01 shot video"}})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}

	want := "agent:eino:composer_final:01000000-0000-0000-0000-000000000000:02000000-0000-0000-0000-000000000000:0a000000-0000-0000-0000-000000000000"
	if graph.runOptions.CheckPointID != want {
		t.Fatalf("checkpoint id = %q, want %q", graph.runOptions.CheckPointID, want)
	}
	if runtime.threadCheckpoint != want {
		t.Fatalf("thread checkpoint = %q, want %q", runtime.threadCheckpoint, want)
	}
}

func TestComposerExecutorPassesTraceCallbacksToGraph(t *testing.T) {
	runtime := &fakeComposerRuntime{}
	graph := &fakeComposerRunner{output: GraphOutput{Output: CompositionOutput{Status: "submitted"}}}
	traceCallback := callbacks.NewHandlerBuilder().Build()
	executor := NewExecutor(ExecutorConfig{
		Runtime:        runtime,
		Graph:          graph,
		TraceCallbacks: []callbacks.Handler{traceCallback},
	})
	task := composerTaskWithInput(t, CompositionInput{VideoNodeRefs: []string{"shot-01 shot video"}})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}

	if len(graph.runOptions.Callbacks) != 1 {
		t.Fatalf("callbacks len = %d, want 1", len(graph.runOptions.Callbacks))
	}
	if got := traceAttribute(graph.ctx, "clipanvil.agent.role"); got != "composer" {
		t.Fatalf("trace role = %q, want composer", got)
	}
}

func composerTaskWithInput(t *testing.T, input CompositionInput) db.AgentTask {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return db.AgentTask{ID: uuidWithByte(10), WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), Role: "composer", ScopeType: "final_output", TaskType: "composer_turn", Status: "queued", MaxAttempts: 1, Input: raw}
}

type fakeComposerRuntime struct {
	succeeded        bool
	checkpointKey    string
	checkpointValue  []byte
	threadCheckpoint string
	eventsByType     map[string][]agentruntime.CreateEventParams
}

type fakeComposerRunner struct {
	input      GraphInput
	output     GraphOutput
	runOptions agenteino.RunOptions
	ctx        context.Context
}

type fakeEinoCheckpointStore struct{}

func (fakeEinoCheckpointStore) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, nil
}

func (fakeEinoCheckpointStore) Set(context.Context, string, []byte) error {
	return nil
}

func (f *fakeComposerRunner) Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error) {
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

func (f *fakeComposerRuntime) MarkTaskRunning(context.Context, pgtype.UUID) (db.AgentTask, error) {
	return db.AgentTask{}, nil
}

func (f *fakeComposerRuntime) MarkTaskSucceeded(context.Context, pgtype.UUID, []byte) (db.AgentTask, error) {
	f.succeeded = true
	return db.AgentTask{}, nil
}

func (f *fakeComposerRuntime) MarkTaskFailed(context.Context, pgtype.UUID, string, string) (db.AgentTask, error) {
	return db.AgentTask{}, nil
}

func (f *fakeComposerRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	if f.eventsByType == nil {
		f.eventsByType = map[string][]agentruntime.CreateEventParams{}
	}
	f.eventsByType[params.EventType] = append(f.eventsByType[params.EventType], params)
	return db.AgentEvent{}, nil
}

func (f *fakeComposerRuntime) UpsertCheckpoint(_ context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error) {
	f.checkpointKey = params.Key
	f.checkpointValue = params.Value
	return db.EinoCheckpoint{Key: params.Key, WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, TaskID: params.TaskID, Value: params.Value}, nil
}

func (f *fakeComposerRuntime) SetThreadCheckpoint(_ context.Context, _ pgtype.UUID, checkpointKey string) (db.AgentThread, error) {
	f.threadCheckpoint = checkpointKey
	return db.AgentThread{CurrentCheckpointKey: pgtype.Text{String: checkpointKey, Valid: checkpointKey != ""}}, nil
}

type fakeComposerStore struct {
	createdNode   db.CreateAgentGenerationNodeParams
	nodes         []db.MediaNode
	versions      map[pgtype.UUID]db.ArtifactVersion
	assets        map[pgtype.UUID]db.MediaAsset
	createdEdges  []db.CreateMediaEdgeParams
	existingEdges map[[2]pgtype.UUID]db.MediaEdge
}

func (f *fakeComposerStore) CreateAgentGenerationNode(_ context.Context, params db.CreateAgentGenerationNodeParams) (db.MediaNode, error) {
	f.createdNode = params
	return db.MediaNode{ID: uuidWithByte(50), WorkspaceID: params.WorkspaceID, NodeType: params.NodeType, Title: params.Title, OperationType: params.OperationType, Status: db.NodeStatusQueued, Source: "agent", Metadata: params.Metadata}, nil
}

func (f *fakeComposerStore) ListMediaNodesByWorkspace(_ context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error) {
	out := []db.MediaNode{}
	for _, node := range f.nodes {
		if node.WorkspaceID == workspaceID {
			out = append(out, node)
		}
	}
	return out, nil
}

func (f *fakeComposerStore) GetArtifactVersionByID(_ context.Context, id pgtype.UUID) (db.ArtifactVersion, error) {
	version, ok := f.versions[id]
	if !ok {
		return db.ArtifactVersion{}, errors.New("version not found")
	}
	return version, nil
}

func (f *fakeComposerStore) GetMediaAssetByID(_ context.Context, id pgtype.UUID) (db.MediaAsset, error) {
	asset, ok := f.assets[id]
	if !ok {
		return db.MediaAsset{}, errors.New("asset not found")
	}
	return asset, nil
}

func (f *fakeComposerStore) GetDependencyEdgeByEndpoints(_ context.Context, params db.GetDependencyEdgeByEndpointsParams) (db.MediaEdge, error) {
	if f.existingEdges != nil {
		if edge, ok := f.existingEdges[[2]pgtype.UUID{params.FromNodeID, params.ToNodeID}]; ok {
			return edge, nil
		}
	}
	return db.MediaEdge{}, pgx.ErrNoRows
}

func (f *fakeComposerStore) CreateMediaEdge(_ context.Context, params db.CreateMediaEdgeParams) (db.MediaEdge, error) {
	f.createdEdges = append(f.createdEdges, params)
	return db.MediaEdge{WorkspaceID: params.WorkspaceID, FromNodeID: params.FromNodeID, ToNodeID: params.ToNodeID}, nil
}

type fakeComposerProduction struct {
	intent production.GenerationIntent
	result production.RunResult
}

func (f *fakeComposerProduction) SubmitGenerationIntent(_ context.Context, intent production.GenerationIntent, _ production.RunOptions) (production.RunResult, error) {
	f.intent = intent
	return f.result, nil
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}
