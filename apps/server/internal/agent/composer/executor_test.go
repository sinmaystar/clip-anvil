package composer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

func TestComposerExecutorPassesDeterministicCheckpointID(t *testing.T) {
	runtime := &fakeComposerRuntime{}
	graph := &fakeComposerRunner{output: GraphOutput{Output: CompositionOutput{Status: "submitted"}}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})
	task := composerTaskWithInput(t, CompositionInput{VideoNodeRefs: []string{"shot-01 shot video"}})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}

	want := "agent:eino:composer_timeline:01000000-0000-0000-0000-000000000000:02000000-0000-0000-0000-000000000000:0a000000-0000-0000-0000-000000000000"
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

func TestComposerExecutorAcceptsDispatchComposerInput(t *testing.T) {
	runtime := &fakeComposerRuntime{}
	graph := &fakeComposerRunner{output: GraphOutput{Output: CompositionOutput{Status: "blocked"}}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})
	task := composerTaskWithRawInput(t, map[string]any{
		"source_storyboard_node_id": "21000000-0000-0000-0000-000000000000",
		"instructions":              "把已完成分镜拼成 20 秒营销视频。",
		"template_key":              "simple_concat",
	})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if graph.input.Input.SourceStoryboardNodeID != "21000000-0000-0000-0000-000000000000" {
		t.Fatalf("graph input = %#v", graph.input.Input)
	}
	if graph.input.Input.Instructions != "把已完成分镜拼成 20 秒营销视频。" || graph.input.Input.TemplateKey != "simple_concat" {
		t.Fatalf("graph input = %#v", graph.input.Input)
	}
}

func TestComposerExecutorSkipsAlreadyClaimedTask(t *testing.T) {
	runtime := &fakeComposerRuntime{claimErr: agentruntime.ErrTaskAlreadyClaimed}
	graph := &fakeComposerRunner{output: GraphOutput{Output: CompositionOutput{Status: "completed"}}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})
	task := composerTaskWithInput(t, CompositionInput{VideoNodeRefs: []string{"shot-01 shot video"}})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if graph.input.WorkspaceID.Valid {
		t.Fatalf("graph must not run for already claimed task: %#v", graph.input)
	}
	if runtime.succeeded || len(runtime.appendedMessages) != 0 {
		t.Fatalf("already claimed task must not persist output: succeeded=%v messages=%#v", runtime.succeeded, runtime.appendedMessages)
	}
}

func TestComposerExecutorPersistsThreadMessages(t *testing.T) {
	runtime := &fakeComposerRuntime{}
	graph := &fakeComposerRunner{output: GraphOutput{
		Output: CompositionOutput{
			Status: "completed",
			SameTurnMessages: []ComposerSameTurnMessage{
				{
					Role:          "assistant",
					MessageType:   "tool_call",
					ToolCallID:    "call_create_plan",
					ToolName:      "create_timeline_plan",
					ToolArguments: map[string]any{"template_key": "simple_concat"},
				},
				{
					Role:        "tool",
					MessageType: "tool_result",
					ToolCallID:  "call_create_plan",
					ToolName:    "create_timeline_plan",
					Content:     `{"timeline_plan_id":"timeline-1"}`,
				},
			},
		},
		AssistantText: "Composer completed.",
	}}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph})
	task := composerTaskWithInput(t, CompositionInput{VideoNodeRefs: []string{"shot-01 shot video"}})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.appendedMessages) != 3 {
		t.Fatalf("appended messages = %#v", runtime.appendedMessages)
	}
	if runtime.appendedMessages[0].MessageType != "tool_call" || runtime.appendedMessages[0].Role != "assistant" {
		t.Fatalf("first message = %#v", runtime.appendedMessages[0])
	}
	if runtime.appendedMessages[1].MessageType != "tool_result" || runtime.appendedMessages[1].Role != "tool" {
		t.Fatalf("second message = %#v", runtime.appendedMessages[1])
	}
	if runtime.appendedMessages[2].MessageType != "text" || runtime.appendedMessages[2].Role != "assistant" {
		t.Fatalf("third message = %#v", runtime.appendedMessages[2])
	}
	if !strings.Contains(string(runtime.appendedMessages[2].RawMessage), "Composer completed") {
		t.Fatalf("assistant raw message = %s", runtime.appendedMessages[2].RawMessage)
	}
	if len(runtime.updatedMessages) != 1 {
		t.Fatalf("updated messages = %#v", runtime.updatedMessages)
	}
}

func TestComposerExecutorSignalsProducerWhenCompositionCompletes(t *testing.T) {
	runtime := &fakeComposerRuntime{producerThread: db.AgentThread{ID: uuidWithByte(90), WorkspaceID: uuidWithByte(1), Role: "producer"}}
	graph := &fakeComposerRunner{output: GraphOutput{Output: CompositionOutput{
		Status:            "completed",
		NodeID:            "32000000-0000-0000-0000-000000000000",
		GenerationJobID:   "33000000-0000-0000-0000-000000000000",
		ArtifactVersionID: "34000000-0000-0000-0000-000000000000",
		SandboxJobID:      "35000000-0000-0000-0000-000000000000",
		TimelinePlanID:    "36000000-0000-0000-0000-000000000000",
	}}}
	enqueuer := &fakeComposerProducerEnqueuer{}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph, ProducerEnqueuer: enqueuer})
	task := composerTaskWithInput(t, CompositionInput{VideoNodeRefs: []string{"shot-01 shot video"}})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.signals) != 1 {
		t.Fatalf("signals = %#v", runtime.signals)
	}
	signal := runtime.signals[0]
	if signal.SignalType != "composition_completed" || signal.ScopeType != "final_output" {
		t.Fatalf("signal = %#v", signal)
	}
	if signal.ProducerThreadID != runtime.producerThread.ID {
		t.Fatalf("producer thread id = %s", uuidString(signal.ProducerThreadID))
	}
	if !strings.Contains(string(signal.Payload), "artifact_version_id") || signal.DedupeKey == "" {
		t.Fatalf("signal payload/dedupe = %q %q", string(signal.Payload), signal.DedupeKey)
	}
	if len(runtime.createdTasks) != 1 {
		t.Fatalf("created wake tasks = %#v", runtime.createdTasks)
	}
	wakeTask := runtime.createdTasks[0]
	if wakeTask.Role != "producer" || wakeTask.TaskType != "producer_turn" || wakeTask.ThreadID != runtime.producerThread.ID {
		t.Fatalf("wake task = %#v", wakeTask)
	}
	if !strings.Contains(string(wakeTask.Input), "composition_completed") {
		t.Fatalf("wake task input = %s", wakeTask.Input)
	}
	if len(enqueuer.tasks) != 1 || enqueuer.tasks[0].ID != uuidWithByte(111) {
		t.Fatalf("enqueued producer tasks = %#v", enqueuer.tasks)
	}
}

func TestComposerExecutorDoesNotQueueProducerWakeWhenProducerAlreadyActive(t *testing.T) {
	runtime := &fakeComposerRuntime{
		producerThread: db.AgentThread{ID: uuidWithByte(90), WorkspaceID: uuidWithByte(1), Role: "producer"},
		activeTasks: []db.AgentTask{{
			ID:          uuidWithByte(91),
			Role:        "producer",
			TaskType:    "producer_turn",
			Status:      "running",
			ThreadID:    uuidWithByte(90),
			WorkspaceID: uuidWithByte(1),
		}},
	}
	graph := &fakeComposerRunner{output: GraphOutput{Output: CompositionOutput{
		Status: "completed",
		NodeID: "32000000-0000-0000-0000-000000000000",
	}}}
	enqueuer := &fakeComposerProducerEnqueuer{}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Graph: graph, ProducerEnqueuer: enqueuer})
	task := composerTaskWithInput(t, CompositionInput{VideoNodeRefs: []string{"shot-01 shot video"}})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.signals) != 1 {
		t.Fatalf("signals = %#v", runtime.signals)
	}
	if len(runtime.createdTasks) != 0 || len(enqueuer.tasks) != 0 {
		t.Fatalf("created/enqueued wake tasks = %#v / %#v", runtime.createdTasks, enqueuer.tasks)
	}
}

func composerTaskWithInput(t *testing.T, input CompositionInput) db.AgentTask {
	t.Helper()
	return composerTaskWithRawInput(t, input)
}

func composerTaskWithRawInput(t *testing.T, input any) db.AgentTask {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return db.AgentTask{ID: uuidWithByte(10), WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), Role: "composer", ScopeType: "final_output", TaskType: "composer_turn", Status: "queued", MaxAttempts: 1, Input: raw}
}

type fakeComposerRuntime struct {
	claimErr         error
	succeeded        bool
	checkpointKey    string
	checkpointValue  []byte
	threadCheckpoint string
	eventsByType     map[string][]agentruntime.CreateEventParams
	producerThread   db.AgentThread
	signals          []agentruntime.CreateProducerPendingSignalParams
	activeTasks      []db.AgentTask
	createdTasks     []agentruntime.CreateTaskParams
	appendedMessages []agentruntime.AppendMessageParams
	updatedMessages  []agentruntime.UpdateMessageParams
}

type fakeComposerProducerEnqueuer struct {
	tasks []db.AgentTask
}

type fakeComposerRunner struct {
	input      GraphInput
	output     GraphOutput
	runOptions agenteino.RunOptions
	ctx        context.Context
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
	if f.claimErr != nil {
		return db.AgentTask{}, f.claimErr
	}
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

func (f *fakeComposerRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	f.appendedMessages = append(f.appendedMessages, params)
	return db.AgentMessage{
		ID:          uuidWithByte(byte(80 + len(f.appendedMessages))),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		MessageType: params.MessageType,
		Content:     params.Content,
		RawMessage:  params.RawMessage,
		TaskID:      params.TaskID,
	}, nil
}

func (f *fakeComposerRuntime) UpdateMessage(_ context.Context, params agentruntime.UpdateMessageParams) (db.AgentMessage, error) {
	f.updatedMessages = append(f.updatedMessages, params)
	return db.AgentMessage{
		ID:         params.ID,
		Content:    params.Content,
		RawMessage: params.RawMessage,
	}, nil
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

func (f *fakeComposerRuntime) GetOrCreateProducerThread(_ context.Context, workspaceID pgtype.UUID) (db.AgentThread, error) {
	if !f.producerThread.ID.Valid {
		f.producerThread = db.AgentThread{ID: uuidWithByte(90), WorkspaceID: workspaceID, Role: "producer"}
	}
	return f.producerThread, nil
}

func (f *fakeComposerRuntime) CreateProducerPendingSignal(_ context.Context, params agentruntime.CreateProducerPendingSignalParams) (db.ProducerPendingSignal, error) {
	f.signals = append(f.signals, params)
	return db.ProducerPendingSignal{ID: uuidWithByte(byte(100 + len(f.signals))), WorkspaceID: params.WorkspaceID, SignalType: params.SignalType}, nil
}

func (f *fakeComposerRuntime) ListActiveAgentTasksByWorkspace(_ context.Context, workspaceID pgtype.UUID) ([]db.AgentTask, error) {
	out := []db.AgentTask{}
	for _, task := range f.activeTasks {
		if task.WorkspaceID == workspaceID {
			out = append(out, task)
		}
	}
	return out, nil
}

func (f *fakeComposerRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	f.createdTasks = append(f.createdTasks, params)
	return db.AgentTask{
		ID:          uuidWithByte(111),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		ScopeType:   params.ScopeType,
		TaskType:    params.TaskType,
		Status:      "queued",
		Input:       params.Input,
	}, nil
}

func (f *fakeComposerProducerEnqueuer) EnqueueProducerTask(_ context.Context, task db.AgentTask) {
	f.tasks = append(f.tasks, task)
}

type fakeComposerStore struct {
	createdNode   db.CreateAgentGenerationNodeParams
	audioPlan     *db.AudioPlan
	nodes         []db.MediaNode
	shots         []db.Shot
	nodesByShot   map[pgtype.UUID][]db.MediaNode
	versions      map[pgtype.UUID]db.ArtifactVersion
	assets        map[pgtype.UUID]db.MediaAsset
	createdEdges  []db.CreateMediaEdgeParams
	existingEdges map[[2]pgtype.UUID]db.MediaEdge
}

func (f *fakeComposerStore) CreateAgentGenerationNode(_ context.Context, params db.CreateAgentGenerationNodeParams) (db.MediaNode, error) {
	f.createdNode = params
	return db.MediaNode{ID: uuidWithByte(50), WorkspaceID: params.WorkspaceID, NodeType: params.NodeType, Title: params.Title, OperationType: params.OperationType, Status: db.NodeStatusQueued, Source: "agent", Metadata: params.Metadata, SemanticKey: params.SemanticKey, DisplayName: params.DisplayName, ArtifactKind: params.ArtifactKind}, nil
}

func (f *fakeComposerStore) GetActiveAudioPlanByWorkspace(_ context.Context, workspaceID pgtype.UUID) (db.AudioPlan, error) {
	if f.audioPlan == nil || f.audioPlan.WorkspaceID != workspaceID {
		return db.AudioPlan{}, pgx.ErrNoRows
	}
	return *f.audioPlan, nil
}

func (f *fakeComposerStore) GetMediaNodeByID(_ context.Context, id pgtype.UUID) (db.MediaNode, error) {
	for _, node := range f.nodes {
		if node.ID == id {
			return node, nil
		}
	}
	return db.MediaNode{}, pgx.ErrNoRows
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

func (f *fakeComposerStore) ListActiveShotsByWorkspace(_ context.Context, workspaceID pgtype.UUID) ([]db.Shot, error) {
	out := []db.Shot{}
	for _, shot := range f.shots {
		if shot.WorkspaceID == workspaceID {
			out = append(out, shot)
		}
	}
	return out, nil
}

func (f *fakeComposerStore) ListMediaNodesByShot(_ context.Context, params db.ListMediaNodesByShotParams) ([]db.MediaNode, error) {
	if f.nodesByShot != nil {
		return append([]db.MediaNode(nil), f.nodesByShot[params.ShotID]...), nil
	}
	out := []db.MediaNode{}
	for _, node := range f.nodes {
		if node.WorkspaceID == params.WorkspaceID && node.ShotID == params.ShotID {
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
