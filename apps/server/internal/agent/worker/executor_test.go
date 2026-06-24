package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

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

func TestWorkerCreatesShotVideoNodeAndSubmitsImageToVideoIntent(t *testing.T) {
	sourceNode := db.MediaNode{
		ID:            uuidWithByte(51),
		WorkspaceID:   uuidWithByte(1),
		NodeType:      db.NodeTypeImage,
		Title:         "shot-01 preview image",
		Status:        db.NodeStatusSucceeded,
		Source:        "agent",
		OperationType: "text_to_image",
		AssetID:       uuidWithByte(61),
	}
	store := &fakeWorkerStore{
		nodes: []db.MediaNode{sourceNode},
		assets: map[pgtype.UUID]db.MediaAsset{
			uuidWithByte(61): {
				ID:          uuidWithByte(61),
				WorkspaceID: uuidWithByte(1),
				Type:        db.AssetTypeImage,
				Mime:        "image/png",
				StorageUrl:  pgtype.Text{String: "workspace/shot-01-preview.png", Valid: true},
			},
		},
	}
	runtime := &fakeWorkerRuntime{}
	productionService := &fakeProductionSubmitter{
		result: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeVideo},
			Job:     db.GenerationJob{ID: uuidWithByte(30), TargetNodeID: uuidWithByte(20), OperationType: "image_to_video", Status: db.JobStatusQueued},
			Version: db.ArtifactVersion{ID: uuidWithByte(40), NodeID: uuidWithByte(20), JobID: uuidWithByte(30), Status: db.JobStatusQueued},
		},
	}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Store: store, Production: productionService})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:              "shot_video",
		ShotID:            uuidString(uuidWithByte(2)),
		ShotClientKey:     "shot-01",
		ShotSortOrder:     1,
		CraftsmanThreadID: uuidString(uuidWithByte(3)),
		CraftsmanTaskID:   uuidString(uuidWithByte(4)),
		Strategy:          "产品开场动态镜头",
		Prompt:            "Animate the accepted preview into a smooth 4-second product shot",
		InputNodeRefs:     []string{"shot-01 preview image"},
		Model:             ModelSpec{Provider: "mock", ModelID: "mock-video"},
		Params:            map[string]any{"duration_sec": float64(4)},
		MaxAttempts:       3,
	})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if store.createdNode.NodeType != db.NodeTypeVideo || store.createdNode.OperationType != "image_to_video" {
		t.Fatalf("created node = %#v", store.createdNode)
	}
	var metadata map[string]any
	if err := json.Unmarshal(store.createdNode.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["agent_artifact_kind"] != "shot_video" || metadata["source_phase"] != "preview_image" {
		t.Fatalf("metadata = %#v", metadata)
	}
	intent := productionService.intent
	if intent.OutputType != "video" || intent.OperationType != "image_to_video" {
		t.Fatalf("intent = %#v", intent)
	}
	if len(intent.InputRefs) != 1 || intent.InputRefs[0].NodeID != sourceNode.ID {
		t.Fatalf("input refs = %#v", intent.InputRefs)
	}
	if len(store.createdEdges) != 1 || store.createdEdges[0].FromNodeID != sourceNode.ID || store.createdEdges[0].ToNodeID != uuidWithByte(20) {
		t.Fatalf("created edges = %#v", store.createdEdges)
	}
	if !runtime.hasEvent("shot_video_submitted") {
		t.Fatalf("events = %#v", runtime.events)
	}
	if runtime.succeededOutput.OperationType != "image_to_video" {
		t.Fatalf("output = %#v", runtime.succeededOutput)
	}
}

func TestWorkerTracesGenerationSubmit(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	tracer := tracerProvider.Tracer("clipanvil-test")
	store := &fakeWorkerStore{}
	productionService := &fakeProductionSubmitter{
		result: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1)},
			Job:     db.GenerationJob{ID: uuidWithByte(30), TargetNodeID: uuidWithByte(20), OperationType: "text_to_image", Status: db.JobStatusQueued},
			Version: db.ArtifactVersion{ID: uuidWithByte(40), NodeID: uuidWithByte(20), JobID: uuidWithByte(30), Status: db.JobStatusQueued},
		},
	}
	executor := NewExecutor(ExecutorConfig{
		Runtime:    &fakeWorkerRuntime{},
		Store:      store,
		Production: productionService,
		Tracer:     tracer,
	})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:          "preview_image",
		ShotID:        uuidString(uuidWithByte(2)),
		ShotClientKey: "shot-01",
		Prompt:        "prompt",
		MaxAttempts:   3,
	})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}

	if !productionService.submitSpanContext.IsValid() {
		t.Fatal("submit context did not carry a valid trace span")
	}
	for _, span := range recorder.Ended() {
		if span.Name() != "worker_generation" {
			continue
		}
		if got := spanAttribute(span, "clipanvil.agent.role"); got != "worker" {
			t.Fatalf("worker role attr = %q, want worker", got)
		}
		if got := spanAttribute(span, "clipanvil.production.operation_type"); got != "text_to_image" {
			t.Fatalf("operation attr = %q, want text_to_image", got)
		}
		return
	}
	t.Fatalf("worker_generation span not found; spans=%v", recorder.Ended())
}

func TestWorkerBroadcastsCreatedPreviewNode(t *testing.T) {
	store := &fakeWorkerStore{}
	broadcaster := &fakeWorkerNodeBroadcaster{}
	executor := NewExecutor(ExecutorConfig{
		Runtime:     &fakeWorkerRuntime{},
		Store:       store,
		Production:  &fakeProductionSubmitter{result: production.RunResult{Node: db.MediaNode{ID: uuidWithByte(20)}, Job: db.GenerationJob{ID: uuidWithByte(30)}, Version: db.ArtifactVersion{ID: uuidWithByte(40)}}},
		Broadcaster: broadcaster,
	})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:          "preview_image",
		ShotID:        uuidString(uuidWithByte(2)),
		ShotClientKey: "shot-01",
		Prompt:        "prompt",
		MaxAttempts:   3,
	})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if broadcaster.node.ID != uuidWithByte(20) {
		t.Fatalf("broadcasted node = %#v", broadcaster.node)
	}
}

func TestWorkerLaysOutPreviewNodesByShotOrder(t *testing.T) {
	for _, input := range []struct {
		key       string
		sortOrder int
		wantX     float32
		wantY     float32
	}{
		{key: "shot-01", sortOrder: 1, wantX: 140, wantY: 140},
		{key: "shot-02", sortOrder: 2, wantX: 660, wantY: 140},
		{key: "shot-03", sortOrder: 3, wantX: 1180, wantY: 140},
		{key: "shot-04", sortOrder: 4, wantX: 140, wantY: 1040},
		{key: "shot-05", sortOrder: 5, wantX: 660, wantY: 1040},
	} {
		t.Run(input.key, func(t *testing.T) {
			store := &fakeWorkerStore{}
			executor := NewExecutor(ExecutorConfig{
				Runtime:    &fakeWorkerRuntime{},
				Store:      store,
				Production: &fakeProductionSubmitter{result: production.RunResult{Node: db.MediaNode{ID: uuidWithByte(20)}, Job: db.GenerationJob{ID: uuidWithByte(30)}, Version: db.ArtifactVersion{ID: uuidWithByte(40)}}},
			})
			task := workerTaskWithInput(t, GenerationInput{
				Mode:          "preview_image",
				ShotID:        uuidString(uuidWithByte(2)),
				ShotClientKey: input.key,
				ShotSortOrder: input.sortOrder,
				Prompt:        "prompt",
				MaxAttempts:   3,
			})

			if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
				t.Fatal(err)
			}
			if store.createdNode.CanvasX != input.wantX || store.createdNode.CanvasY != input.wantY {
				t.Fatalf("position = (%v,%v), want (%v,%v)", store.createdNode.CanvasX, store.createdNode.CanvasY, input.wantX, input.wantY)
			}
		})
	}
}

func TestPreviewNodeLayoutKeepsReadableGap(t *testing.T) {
	firstX, firstY := previewNodePosition(GenerationInput{ShotSortOrder: 1})
	secondX, secondY := previewNodePosition(GenerationInput{ShotSortOrder: 2})
	fourthX, fourthY := previewNodePosition(GenerationInput{ShotSortOrder: 4})
	_, firstVideoY := nodePosition(GenerationInput{Mode: "shot_video", ShotSortOrder: 1})

	const maxRenderedImageWidth = 380
	const maxRenderedImageHeight = 420
	if horizontalGap := secondX - firstX - maxRenderedImageWidth; horizontalGap < 96 {
		t.Fatalf("horizontal gap = %v, want at least 96", horizontalGap)
	}
	if previewToVideoGap := firstVideoY - firstY - maxRenderedImageHeight; previewToVideoGap < 48 {
		t.Fatalf("preview to video gap = %v, want at least 48", previewToVideoGap)
	}
	if verticalGap := fourthY - firstY - maxRenderedImageHeight; verticalGap < 96 {
		t.Fatalf("vertical gap = %v, want at least 96", verticalGap)
	}
	if secondY != firstY || fourthX != firstX {
		t.Fatalf("grid alignment broken: first=(%v,%v) second=(%v,%v) fourth=(%v,%v)", firstX, firstY, secondX, secondY, fourthX, fourthY)
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

func TestWorkerResolvesInputNodeRefsIntoGenerationIntentAndEdges(t *testing.T) {
	sourceNode := db.MediaNode{
		ID:            uuidWithByte(51),
		WorkspaceID:   uuidWithByte(1),
		NodeType:      db.NodeTypeImage,
		Title:         "product.png",
		Status:        db.NodeStatusSucceeded,
		Source:        "agent",
		OperationType: "upload",
		AssetID:       uuidWithByte(61),
	}
	store := &fakeWorkerStore{
		nodes: []db.MediaNode{sourceNode},
		assets: map[pgtype.UUID]db.MediaAsset{
			uuidWithByte(61): {
				ID:          uuidWithByte(61),
				WorkspaceID: uuidWithByte(1),
				Type:        db.AssetTypeImage,
				Mime:        "image/png",
				StorageUrl:  pgtype.Text{String: "workspace/product.png", Valid: true},
			},
		},
	}
	productionService := &fakeProductionSubmitter{result: production.RunResult{
		Node:    db.MediaNode{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1)},
		Job:     db.GenerationJob{ID: uuidWithByte(30)},
		Version: db.ArtifactVersion{ID: uuidWithByte(40)},
	}}
	executor := NewExecutor(ExecutorConfig{Runtime: &fakeWorkerRuntime{}, Store: store, Production: productionService})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:          "preview_image",
		ShotID:        uuidString(uuidWithByte(2)),
		ShotClientKey: "shot-01",
		Prompt:        "use the product reference",
		InputNodeRefs: []string{"product.png"},
		MaxAttempts:   3,
	})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	refs := productionService.intent.InputRefs
	if len(refs) != 1 {
		t.Fatalf("input refs = %#v", refs)
	}
	if refs[0].NodeID != sourceNode.ID || refs[0].Kind != production.InputKindExplicit || refs[0].AssetID != uuidString(uuidWithByte(61)) {
		t.Fatalf("input ref = %#v", refs[0])
	}
	if len(store.createdEdges) != 1 {
		t.Fatalf("created edges = %#v", store.createdEdges)
	}
	if store.createdEdges[0].FromNodeID != sourceNode.ID || store.createdEdges[0].ToNodeID != uuidWithByte(20) {
		t.Fatalf("created edge = %#v", store.createdEdges[0])
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

func TestWorkerMarksShotFailedWhenSynchronousSubmitFails(t *testing.T) {
	store := &fakeWorkerStore{}
	runtime := &fakeWorkerRuntime{}
	productionService := &fakeProductionSubmitter{
		failuresBeforeSuccess: 3,
	}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Store: store, Production: productionService})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:        "preview_image",
		ShotID:      uuidString(uuidWithByte(2)),
		Prompt:      "prompt",
		MaxAttempts: 3,
	})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err == nil {
		t.Fatal("RunTask succeeded, want failure")
	}
	if len(store.statusUpdates) != 1 {
		t.Fatalf("status updates = %#v", store.statusUpdates)
	}
	if store.statusUpdates[0].ID != uuidWithByte(2) || store.statusUpdates[0].Status != "failed" {
		t.Fatalf("status update = %#v", store.statusUpdates[0])
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
	events          []agentruntime.CreateEventParams
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

func (f *fakeWorkerRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	f.events = append(f.events, params)
	return db.AgentEvent{}, nil
}

func (f *fakeWorkerRuntime) hasEvent(eventType string) bool {
	for _, event := range f.events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

type fakeWorkerStore struct {
	createdNode     db.CreateAgentGenerationNodeParams
	createNodeCalls int
	existingNode    db.MediaNode
	nodes           []db.MediaNode
	assets          map[pgtype.UUID]db.MediaAsset
	existingEdges   map[[2]pgtype.UUID]db.MediaEdge
	createdEdges    []db.CreateMediaEdgeParams
	statusUpdates   []db.UpdateShotStatusParams
}

type fakeWorkerNodeBroadcaster struct {
	node db.MediaNode
}

func (f *fakeWorkerNodeBroadcaster) BroadcastAgentNodeCreated(_ pgtype.UUID, node db.MediaNode) {
	f.node = node
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

func (f *fakeWorkerStore) ListMediaNodesByWorkspace(_ context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error) {
	out := []db.MediaNode{}
	for _, node := range f.nodes {
		if node.WorkspaceID == workspaceID {
			out = append(out, node)
		}
	}
	return out, nil
}

func (f *fakeWorkerStore) GetMediaAssetByID(_ context.Context, id pgtype.UUID) (db.MediaAsset, error) {
	asset, ok := f.assets[id]
	if !ok {
		return db.MediaAsset{}, errors.New("asset not found")
	}
	return asset, nil
}

func (f *fakeWorkerStore) GetArtifactVersionByID(context.Context, pgtype.UUID) (db.ArtifactVersion, error) {
	return db.ArtifactVersion{}, errors.New("version not found")
}

func (f *fakeWorkerStore) GetDependencyEdgeByEndpoints(_ context.Context, params db.GetDependencyEdgeByEndpointsParams) (db.MediaEdge, error) {
	if f.existingEdges != nil {
		edge, ok := f.existingEdges[[2]pgtype.UUID{params.FromNodeID, params.ToNodeID}]
		if ok {
			return edge, nil
		}
	}
	return db.MediaEdge{}, pgx.ErrNoRows
}

func (f *fakeWorkerStore) CreateMediaEdge(_ context.Context, params db.CreateMediaEdgeParams) (db.MediaEdge, error) {
	f.createdEdges = append(f.createdEdges, params)
	return db.MediaEdge{WorkspaceID: params.WorkspaceID, FromNodeID: params.FromNodeID, ToNodeID: params.ToNodeID}, nil
}

func (f *fakeWorkerStore) UpdateShotStatus(_ context.Context, params db.UpdateShotStatusParams) (db.Shot, error) {
	f.statusUpdates = append(f.statusUpdates, params)
	return db.Shot{ID: params.ID, WorkspaceID: params.WorkspaceID, Status: params.Status}, nil
}

type fakeProductionSubmitter struct {
	intent                production.GenerationIntent
	options               production.RunOptions
	result                production.RunResult
	calls                 int
	failuresBeforeSuccess int
	submitSpanContext     trace.SpanContext
}

func (f *fakeProductionSubmitter) SubmitGenerationIntent(ctx context.Context, intent production.GenerationIntent, options production.RunOptions) (production.RunResult, error) {
	f.calls++
	f.intent = intent
	f.options = options
	f.submitSpanContext = trace.SpanContextFromContext(ctx)
	if f.calls <= f.failuresBeforeSuccess {
		return production.RunResult{}, errors.New("temporary submit failure")
	}
	return f.result, nil
}

func spanAttribute(span sdktrace.ReadOnlySpan, key string) string {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
}
