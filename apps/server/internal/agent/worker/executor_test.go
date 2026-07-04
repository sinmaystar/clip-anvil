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
		ScopeKey:          "scene_main.shot_01",
		RenderPlanKey:     "scene_main.shot_01.preview_image.r1",
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
	task.RenderPlanID = uuidWithByte(77)

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
	if store.createdNode.SemanticKey != "scene_main.shot_01.preview_image.r1.node" || store.createdNode.SourceRenderPlanID != uuidWithByte(77) || store.createdNode.ArtifactKind != "preview_image" {
		t.Fatalf("created node semantic fields = %#v", store.createdNode)
	}
	intent := productionService.intent
	if intent.OutputType != "image" || intent.OperationType != "text_to_image" {
		t.Fatalf("intent = %#v", intent)
	}
	if intent.RequestedBy.Type != "agent_worker" || intent.RequestedBy.ID != uuidString(task.ID) {
		t.Fatalf("requested by = %#v", intent.RequestedBy)
	}
	if intent.Semantic.RenderPlanKey != "scene_main.shot_01.preview_image.r1" || intent.Semantic.ArtifactKind != "preview_image" || intent.Semantic.SourceRenderPlanID != task.RenderPlanID {
		t.Fatalf("intent semantic = %#v", intent.Semantic)
	}
	if runtime.succeededOutput.Status != "submitted" || runtime.succeededOutput.NodeID == "" {
		t.Fatalf("output = %#v", runtime.succeededOutput)
	}
	if len(store.renderPlanCompletions) != 0 {
		t.Fatalf("worker submit must not mark render plan terminal: %#v", store.renderPlanCompletions)
	}
}

func TestWorkerCreatesShotVideoNodeAndSubmitsImageToVideoIntent(t *testing.T) {
	sourceNode := db.MediaNode{
		ID:               uuidWithByte(51),
		WorkspaceID:      uuidWithByte(1),
		NodeType:         db.NodeTypeImage,
		Title:            "shot-01 preview image",
		Status:           db.NodeStatusSucceeded,
		Source:           "agent",
		OperationType:    "text_to_image",
		AssetID:          uuidWithByte(61),
		CurrentVersionID: uuidWithByte(62),
		SemanticKey:      "scene_main.shot_01.preview_image.r1.node",
		ArtifactKind:     "preview_image",
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
		versions: map[pgtype.UUID]db.ArtifactVersion{
			uuidWithByte(62): {
				ID:          uuidWithByte(62),
				WorkspaceID: uuidWithByte(1),
				NodeID:      sourceNode.ID,
				AssetID:     uuidWithByte(61),
				Status:      db.JobStatusSucceeded,
				InputHash:   "preview-hash",
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
		InputNodeRefs:     []string{"shot_01.preview_image.current"},
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

func TestWorkerDoesNotMarkRenderPlanSucceededOnAsyncSubmit(t *testing.T) {
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
		Mode:        "preview_image",
		ShotID:      uuidString(uuidWithByte(2)),
		Prompt:      "A bright product close-up",
		MaxAttempts: 3,
	})
	task.RenderPlanID = uuidWithByte(77)

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if len(store.renderPlanCompletions) != 0 {
		t.Fatalf("worker submit must not mark render plan terminal: %#v", store.renderPlanCompletions)
	}
}

func TestWorkerCreatesReferenceImageNodeAndMarksKeyElementStateReady(t *testing.T) {
	stateID := uuidWithByte(71)
	store := &fakeWorkerStore{
		keyElementState: db.KeyElementState{
			ID:                stateID,
			WorkspaceID:       uuidWithByte(1),
			ClientKey:         "element_airport_terminal_default",
			Label:             "机场航站楼",
			VisualDescription: "明亮现代机场航站楼",
			ReferenceStatus:   "needs_reference",
			IsDefault:         true,
			StateFacts:        []byte(`{"scene":"airport"}`),
			SourceRefs:        []byte(`[]`),
			Status:            "active",
		},
	}
	runtime := &fakeWorkerRuntime{}
	productionService := &fakeProductionSubmitter{
		result: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeImage},
			Job:     db.GenerationJob{ID: uuidWithByte(30), TargetNodeID: uuidWithByte(20), OperationType: "text_to_image", Status: db.JobStatusQueued},
			Version: db.ArtifactVersion{ID: uuidWithByte(40), NodeID: uuidWithByte(20), JobID: uuidWithByte(30), Status: db.JobStatusQueued},
		},
	}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Store: store, Production: productionService})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:                     "reference_image",
		TargetPhase:              "reference_image",
		ScopeType:                "key_element_state",
		ScopeID:                  uuidString(stateID),
		KeyElementStateClientKey: "element_airport_terminal_default",
		CraftsmanThreadID:        uuidString(uuidWithByte(3)),
		CraftsmanTaskID:          uuidString(uuidWithByte(4)),
		Strategy:                 "先生成统一场景参考图",
		Prompt:                   "A bright modern airport terminal reference image for a luggage ad",
		Model:                    ModelSpec{Provider: "volcengine", ModelID: "seedream-5"},
		Params:                   map[string]any{"size": "1024x1024"},
		MaxAttempts:              3,
	})
	task.ScopeType = "key_element_state"
	task.ScopeID = stateID

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if store.createdNode.NodeType != db.NodeTypeImage || store.createdNode.OperationType != "text_to_image" {
		t.Fatalf("created node = %#v", store.createdNode)
	}
	if store.createdNode.ShotID.Valid {
		t.Fatalf("reference image should not bind shot id: %#v", store.createdNode.ShotID)
	}
	var metadata map[string]any
	if err := json.Unmarshal(store.createdNode.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["agent_artifact_kind"] != "reference_image" || metadata["source_phase"] != "key_element_state" || metadata["key_element_state_client_key"] != "element_airport_terminal_default" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if productionService.intent.OutputType != "image" || productionService.intent.OperationType != "text_to_image" {
		t.Fatalf("intent = %#v", productionService.intent)
	}
	if len(store.keyElementStateUpdates) != 1 {
		t.Fatalf("state updates = %#v", store.keyElementStateUpdates)
	}
	update := store.keyElementStateUpdates[0]
	if update.ID != stateID || update.ReferenceStatus != "ready" || update.ReferenceNodeID != uuidWithByte(20) || update.ReferenceVersionID != uuidWithByte(40) {
		t.Fatalf("state update = %#v", update)
	}
	if len(store.statusUpdates) != 0 {
		t.Fatalf("shot status updates = %#v", store.statusUpdates)
	}
}

func TestWorkerCreatesVoiceoverAudioNodeAndSubmitsTextToAudioIntent(t *testing.T) {
	audioPlanID := uuidWithByte(71)
	store := &fakeWorkerStore{}
	runtime := &fakeWorkerRuntime{}
	productionService := &fakeProductionSubmitter{
		result: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeAudio},
			Job:     db.GenerationJob{ID: uuidWithByte(30), TargetNodeID: uuidWithByte(20), OperationType: "text_to_audio", Status: db.JobStatusQueued},
			Version: db.ArtifactVersion{ID: uuidWithByte(40), NodeID: uuidWithByte(20), JobID: uuidWithByte(30), Status: db.JobStatusQueued},
		},
	}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Store: store, Production: productionService})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:              "voiceover_audio",
		TargetPhase:       "voiceover_audio",
		ScopeType:         "audio_plan",
		ScopeID:           uuidString(audioPlanID),
		ScopeKey:          "audio_plan.active",
		RenderPlanKey:     "audio_plan.active.voiceover_audio.r1",
		CraftsmanThreadID: uuidString(uuidWithByte(3)),
		CraftsmanTaskID:   uuidString(uuidWithByte(4)),
		Strategy:          "生成全片旁白音频",
		Prompt:            "zh, 12 seconds, warm female voice, script: 现在出发，让旅程更轻松。",
		OutputType:        "audio",
		OperationType:     "text_to_audio",
		Model:             ModelSpec{Provider: "volcengine", ModelID: "seed-audio-1.0"},
		Params:            map[string]any{"speaker": "warm_female", "format": "mp3"},
		MaxAttempts:       3,
	})
	task.ScopeType = "audio_plan"
	task.ScopeID = audioPlanID
	task.RenderPlanID = uuidWithByte(77)

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if store.createdNode.NodeType != db.NodeTypeAudio || store.createdNode.OperationType != "text_to_audio" {
		t.Fatalf("created node = %#v", store.createdNode)
	}
	if store.createdNode.ShotID.Valid {
		t.Fatalf("audio node should not bind shot id: %#v", store.createdNode.ShotID)
	}
	if store.createdNode.ArtifactKind != "voiceover_audio" || store.createdNode.SemanticKey != "audio_plan.active.voiceover_audio.r1.node" {
		t.Fatalf("created node semantic fields = %#v", store.createdNode)
	}
	var metadata map[string]any
	if err := json.Unmarshal(store.createdNode.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["agent_artifact_kind"] != "voiceover_audio" || metadata["source_phase"] != "voiceover_audio" || metadata["scope_type"] != "audio_plan" {
		t.Fatalf("metadata = %#v", metadata)
	}
	intent := productionService.intent
	if intent.OutputType != "audio" || intent.OperationType != "text_to_audio" || intent.Model.Provider != "volcengine" || intent.Model.ModelID != "seed-audio-1.0" {
		t.Fatalf("intent = %#v", intent)
	}
	if intent.Semantic.ArtifactKind != "voiceover_audio" || intent.Semantic.ScopeKey != "audio_plan.active" {
		t.Fatalf("intent semantic = %#v", intent.Semantic)
	}
	if len(store.voiceoverNodeLinks) != 1 || store.voiceoverNodeLinks[0].ID != audioPlanID || store.voiceoverNodeLinks[0].VoiceoverNodeID != uuidWithByte(20) {
		t.Fatalf("voiceover links = %#v", store.voiceoverNodeLinks)
	}
	if len(store.renderPlanSubmissions) != 1 || store.renderPlanSubmissions[0].OutputNodeID != uuidWithByte(20) {
		t.Fatalf("render plan submissions = %#v", store.renderPlanSubmissions)
	}
}

func TestWorkerCreatesBGMAudioNodeAndLinksAudioPlan(t *testing.T) {
	audioPlanID := uuidWithByte(71)
	store := &fakeWorkerStore{}
	runtime := &fakeWorkerRuntime{}
	productionService := &fakeProductionSubmitter{
		result: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeAudio},
			Job:     db.GenerationJob{ID: uuidWithByte(30), TargetNodeID: uuidWithByte(20), OperationType: "text_to_audio", Status: db.JobStatusQueued},
			Version: db.ArtifactVersion{ID: uuidWithByte(40), NodeID: uuidWithByte(20), JobID: uuidWithByte(30), Status: db.JobStatusQueued},
		},
	}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Store: store, Production: productionService})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:              "bgm_audio",
		TargetPhase:       "bgm_audio",
		ScopeType:         "audio_plan",
		ScopeID:           uuidString(audioPlanID),
		ScopeKey:          "audio_plan.active",
		RenderPlanKey:     "audio_plan.active.bgm_audio.r1",
		CraftsmanThreadID: uuidString(uuidWithByte(3)),
		CraftsmanTaskID:   uuidString(uuidWithByte(4)),
		Strategy:          "生成全片 BGM",
		Prompt:            "12 seconds bright electronic pop BGM, no vocals, duck under voiceover.",
		OutputType:        "audio",
		OperationType:     "text_to_audio",
		Model:             ModelSpec{Provider: "volcengine", ModelID: "seed-audio-1.0"},
		MaxAttempts:       3,
	})
	task.ScopeType = "audio_plan"
	task.ScopeID = audioPlanID
	task.RenderPlanID = uuidWithByte(78)

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	if store.createdNode.NodeType != db.NodeTypeAudio || store.createdNode.ArtifactKind != "bgm_audio" {
		t.Fatalf("created node = %#v", store.createdNode)
	}
	if len(store.bgmNodeLinks) != 1 || store.bgmNodeLinks[0].ID != audioPlanID || store.bgmNodeLinks[0].BgmNodeID != uuidWithByte(20) {
		t.Fatalf("bgm links = %#v", store.bgmNodeLinks)
	}
	if !runtime.hasEvent("audio_generation_submitted") {
		t.Fatalf("events = %#v", runtime.events)
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

func TestGenerationSpecUsesImageOperationFromInput(t *testing.T) {
	spec := generationSpec(GenerationInput{
		Mode:          "preview_image",
		OutputType:    "image",
		OperationType: "multi_image_to_image",
	})
	if spec.OutputType != "image" || spec.OperationType != "multi_image_to_image" {
		t.Fatalf("spec = %#v", spec)
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

func TestWorkerResolvesInputBindingsIntoRoleAwareGenerationIntent(t *testing.T) {
	sourceNode := db.MediaNode{
		ID:               uuidWithByte(52),
		WorkspaceID:      uuidWithByte(1),
		NodeType:         db.NodeTypeImage,
		Title:            "shot_01 preview image",
		Status:           db.NodeStatusSucceeded,
		Source:           "agent",
		OperationType:    "text_to_image",
		AssetID:          uuidWithByte(62),
		CurrentVersionID: uuidWithByte(72),
		ArtifactKind:     "preview_image",
		SemanticKey:      "shot_01.preview_image.r1.node",
	}
	store := &fakeWorkerStore{
		nodes: []db.MediaNode{sourceNode},
		assets: map[pgtype.UUID]db.MediaAsset{
			uuidWithByte(62): {
				ID:          uuidWithByte(62),
				WorkspaceID: uuidWithByte(1),
				Type:        db.AssetTypeImage,
				Mime:        "image/png",
				StorageUrl:  pgtype.Text{String: "workspace/shot_01_preview.png", Valid: true},
			},
		},
		versions: map[pgtype.UUID]db.ArtifactVersion{
			uuidWithByte(72): {
				ID:          uuidWithByte(72),
				WorkspaceID: uuidWithByte(1),
				NodeID:      uuidWithByte(52),
				AssetID:     uuidWithByte(62),
				Status:      db.JobStatusSucceeded,
				InputHash:   "preview-hash",
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
		Mode:          "shot_video",
		ShotID:        uuidString(uuidWithByte(2)),
		ShotClientKey: "shot-01",
		Prompt:        "turn the preview image into a video",
		InputBindings: []InputBinding{{
			ClientKey:   "shot_01_preview_as_first_frame",
			SourceType:  "shot_output",
			SourceID:    "shot_01.preview_image.current",
			ContentType: "image_url",
			ModelRole:   "first_frame",
			Required:    true,
		}},
		MaxAttempts: 3,
	})

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err != nil {
		t.Fatal(err)
	}
	refs := productionService.intent.InputRefs
	if len(refs) != 1 {
		t.Fatalf("input refs = %#v", refs)
	}
	if refs[0].ContentType != "image_url" || refs[0].ModelRole != "first_frame" || !refs[0].Required {
		t.Fatalf("input ref role binding = %#v", refs[0])
	}
	if refs[0].NodeID != sourceNode.ID || refs[0].StorageURL != "workspace/shot_01_preview.png" {
		t.Fatalf("input ref source = %#v", refs[0])
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

func TestWorkerMarksRenderPlanFailedWhenSynchronousSubmitFails(t *testing.T) {
	store := &fakeWorkerStore{}
	runtime := &fakeWorkerRuntime{}
	productionService := &fakeProductionSubmitter{failuresBeforeSuccess: 3}
	executor := NewExecutor(ExecutorConfig{Runtime: runtime, Store: store, Production: productionService})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:        "preview_image",
		ShotID:      uuidString(uuidWithByte(2)),
		Prompt:      "prompt",
		MaxAttempts: 3,
	})
	task.RenderPlanID = uuidWithByte(77)

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err == nil {
		t.Fatal("RunTask succeeded, want failure")
	}
	if len(store.renderPlanCompletions) != 1 {
		t.Fatalf("render plan completions = %#v", store.renderPlanCompletions)
	}
	got := store.renderPlanCompletions[0]
	if got.ID != task.RenderPlanID || got.Status != "failed" {
		t.Fatalf("render plan completion = %#v", got)
	}
}

func TestWorkerBroadcastsAndSignalsSynchronousSubmitFailure(t *testing.T) {
	store := &fakeWorkerStore{
		existingNode: db.MediaNode{
			ID:          uuidWithByte(20),
			WorkspaceID: uuidWithByte(1),
			NodeType:    db.NodeTypeImage,
			Status:      db.NodeStatusFailed,
			ShotID:      uuidWithByte(2),
		},
	}
	runtime := &fakeWorkerRuntime{producerThread: db.AgentThread{ID: uuidWithByte(90), WorkspaceID: uuidWithByte(1), Role: "producer"}}
	nodeBroadcaster := &fakeWorkerNodeBroadcaster{}
	agentBroadcaster := &fakeWorkerAgentBroadcaster{}
	producerEnqueuer := &fakeWorkerProducerEnqueuer{}
	productionService := &fakeProductionSubmitter{
		failuresBeforeSuccess: 3,
		failureResult: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeImage, Status: db.NodeStatusFailed, ShotID: uuidWithByte(2)},
			Job:     db.GenerationJob{ID: uuidWithByte(30), TargetNodeID: uuidWithByte(20), OperationType: "text_to_image", Status: db.JobStatusFailed},
			Version: db.ArtifactVersion{ID: uuidWithByte(40), NodeID: uuidWithByte(20), JobID: uuidWithByte(30), Status: db.JobStatusFailed},
		},
	}
	executor := NewExecutor(ExecutorConfig{
		Runtime:          runtime,
		Store:            store,
		Production:       productionService,
		Broadcaster:      nodeBroadcaster,
		AgentBroadcaster: agentBroadcaster,
		ProducerEnqueuer: producerEnqueuer,
	})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:          "preview_image",
		ShotID:        uuidString(uuidWithByte(2)),
		ShotClientKey: "shot-01",
		Prompt:        "prompt",
		MaxAttempts:   3,
	})
	task.RenderPlanID = uuidWithByte(77)

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err == nil {
		t.Fatal("RunTask succeeded, want failure")
	}
	if agentBroadcaster.event.EventType != "worker_generation_failed" {
		t.Fatalf("broadcast event = %#v", agentBroadcaster.event)
	}
	if nodeBroadcaster.updatedNode.ID != uuidWithByte(20) || nodeBroadcaster.updatedNode.Status != db.NodeStatusFailed {
		t.Fatalf("updated node = %#v", nodeBroadcaster.updatedNode)
	}
	if len(runtime.signals) != 1 {
		t.Fatalf("signals = %#v", runtime.signals)
	}
	signal := runtime.signals[0]
	if signal.SignalType != "worker_generation_completed" ||
		signal.ProducerThreadID != uuidWithByte(90) ||
		signal.SourceRole != "worker" ||
		signal.SourceTaskID != task.ID ||
		signal.RenderPlanID != uuidWithByte(77) ||
		signal.DedupeKey != "worker_generation_completed:1e000000-0000-0000-0000-000000000000" {
		t.Fatalf("signal = %#v", signal)
	}
	var payload map[string]any
	if err := json.Unmarshal(signal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["render_plan_status"] != "failed" ||
		payload["generation_job_id"] != uuidString(uuidWithByte(30)) ||
		payload["artifact_version_id"] != uuidString(uuidWithByte(40)) {
		t.Fatalf("payload = %#v", payload)
	}
	if len(producerEnqueuer.tasks) != 1 {
		t.Fatalf("producer wake tasks = %#v", producerEnqueuer.tasks)
	}
	if len(store.renderPlanCompletions) != 1 || store.renderPlanCompletions[0].OutputNodeID != uuidWithByte(20) || store.renderPlanCompletions[0].OutputVersionID != uuidWithByte(40) {
		t.Fatalf("render plan completions = %#v", store.renderPlanCompletions)
	}
}

func TestWorkerSignalsSeedanceFailureFallbackGuidance(t *testing.T) {
	store := &fakeWorkerStore{
		existingNode: db.MediaNode{
			ID:          uuidWithByte(20),
			WorkspaceID: uuidWithByte(1),
			NodeType:    db.NodeTypeVideo,
			Status:      db.NodeStatusFailed,
			ShotID:      uuidWithByte(2),
		},
		nodes: []db.MediaNode{
			{
				ID:               uuidWithByte(21),
				WorkspaceID:      uuidWithByte(1),
				NodeType:         db.NodeTypeImage,
				Title:            "shot_01 preview image",
				Status:           db.NodeStatusSucceeded,
				CurrentVersionID: uuidWithByte(41),
			},
		},
		versions: map[pgtype.UUID]db.ArtifactVersion{
			uuidWithByte(41): {
				ID:          uuidWithByte(41),
				WorkspaceID: uuidWithByte(1),
				NodeID:      uuidWithByte(21),
				AssetID:     uuidWithByte(51),
				Status:      db.JobStatusSucceeded,
				InputHash:   "preview-hash",
			},
		},
		assets: map[pgtype.UUID]db.MediaAsset{
			uuidWithByte(51): {
				ID:          uuidWithByte(51),
				WorkspaceID: uuidWithByte(1),
				Type:        db.AssetTypeImage,
				Mime:        "image/png",
				StorageUrl:  pgtype.Text{String: "workspace/preview.png", Valid: true},
			},
		},
	}
	runtime := &fakeWorkerRuntime{producerThread: db.AgentThread{ID: uuidWithByte(90), WorkspaceID: uuidWithByte(1), Role: "producer"}}
	producerEnqueuer := &fakeWorkerProducerEnqueuer{}
	productionService := &fakeProductionSubmitter{
		failuresBeforeSuccess: 3,
		failureResult: production.RunResult{
			Node: db.MediaNode{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeVideo, Status: db.NodeStatusFailed, ShotID: uuidWithByte(2)},
			Job: db.GenerationJob{
				ID:            uuidWithByte(30),
				TargetNodeID:  uuidWithByte(20),
				OperationType: "image_to_video_first_frame",
				Provider:      "volcengine",
				ModelID:       "doubao-seedance-2-0-pro-260428",
				Status:        db.JobStatusFailed,
				ErrorCode:     pgtype.Text{String: "provider_error", Valid: true},
				ErrorMessage:  pgtype.Text{String: "ark video task failed: safety rejection", Valid: true},
			},
			Version: db.ArtifactVersion{ID: uuidWithByte(40), NodeID: uuidWithByte(20), JobID: uuidWithByte(30), Status: db.JobStatusFailed},
		},
	}
	executor := NewExecutor(ExecutorConfig{
		Runtime:          runtime,
		Store:            store,
		Production:       productionService,
		ProducerEnqueuer: producerEnqueuer,
	})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:          "shot_video",
		TargetPhase:   "shot_video",
		ScopeKey:      "scene_main.shot_01",
		RenderPlanKey: "scene_main.shot_01.shot_video.r1",
		ShotID:        uuidString(uuidWithByte(2)),
		ShotClientKey: "shot_01",
		Prompt:        "真实动态展示商品被人物推行",
		OperationType: "image_to_video_first_frame",
		Model:         ModelSpec{Provider: "volcengine", ModelID: "doubao-seedance-2-0-pro-260428"},
		InputBindings: []InputBinding{{
			SourceType:  "media_node",
			SourceID:    "shot_01 preview image",
			ContentType: "image_url",
			ModelRole:   "first_frame",
			Required:    true,
		}},
		MaxAttempts: 3,
	})
	task.RenderPlanID = uuidWithByte(77)

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err == nil {
		t.Fatal("RunTask succeeded, want failure")
	}
	if len(runtime.signals) != 1 {
		t.Fatalf("signals = %#v", runtime.signals)
	}
	var payload map[string]any
	if err := json.Unmarshal(runtime.signals[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"model_provider":               "volcengine",
		"model_id":                     "doubao-seedance-2-0-pro-260428",
		"operation_type":               "image_to_video_first_frame",
		"fallback_strategy":            "template_fallback_or_hitl",
		"recommended_next_action":      "route_to_template_fallback_or_request_user_confirmation",
		"should_stop_same_route_retry": true,
		"cost_risk":                    true,
	} {
		if got := payload[key]; got != want {
			t.Fatalf("payload[%s] = %#v, want %#v; payload=%#v", key, got, want, payload)
		}
	}
	if len(producerEnqueuer.tasks) != 1 {
		t.Fatalf("producer wake tasks = %#v", producerEnqueuer.tasks)
	}
}

func TestWorkerDoesNotWakeProducerWhenProducerRunning(t *testing.T) {
	store := &fakeWorkerStore{
		existingNode: db.MediaNode{
			ID:          uuidWithByte(20),
			WorkspaceID: uuidWithByte(1),
			NodeType:    db.NodeTypeImage,
			Status:      db.NodeStatusFailed,
			ShotID:      uuidWithByte(2),
		},
	}
	runtime := &fakeWorkerRuntime{
		producerThread: db.AgentThread{ID: uuidWithByte(90), WorkspaceID: uuidWithByte(1), Role: "producer"},
		activeTasks: []db.AgentTask{
			{ID: uuidWithByte(91), Role: "producer", TaskType: "producer_turn", Status: "running"},
		},
	}
	producerEnqueuer := &fakeWorkerProducerEnqueuer{}
	productionService := &fakeProductionSubmitter{
		failuresBeforeSuccess: 3,
		failureResult: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeImage, Status: db.NodeStatusFailed, ShotID: uuidWithByte(2)},
			Job:     db.GenerationJob{ID: uuidWithByte(30), TargetNodeID: uuidWithByte(20), OperationType: "text_to_image", Status: db.JobStatusFailed},
			Version: db.ArtifactVersion{ID: uuidWithByte(40), NodeID: uuidWithByte(20), JobID: uuidWithByte(30), Status: db.JobStatusFailed},
		},
	}
	executor := NewExecutor(ExecutorConfig{
		Runtime:          runtime,
		Store:            store,
		Production:       productionService,
		ProducerEnqueuer: producerEnqueuer,
	})
	task := workerTaskWithInput(t, GenerationInput{
		Mode:        "preview_image",
		ShotID:      uuidString(uuidWithByte(2)),
		Prompt:      "prompt",
		MaxAttempts: 3,
	})
	task.RenderPlanID = uuidWithByte(77)

	if err := executor.RunTask(context.Background(), RunTaskInput{Task: task}); err == nil {
		t.Fatal("RunTask succeeded, want failure")
	}
	if len(runtime.signals) != 1 {
		t.Fatalf("signals = %#v", runtime.signals)
	}
	if len(runtime.createdTasks) != 0 || len(producerEnqueuer.tasks) != 0 {
		t.Fatalf("created tasks = %#v, enqueued = %#v", runtime.createdTasks, producerEnqueuer.tasks)
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
	producerThread  db.AgentThread
	signals         []agentruntime.CreateProducerPendingSignalParams
	activeTasks     []db.AgentTask
	createdTasks    []agentruntime.CreateTaskParams
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
	return db.AgentEvent{ID: uuidWithByte(byte(70 + len(f.events))), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, TaskID: params.TaskID, EventType: params.EventType}, nil
}

func (f *fakeWorkerRuntime) GetOrCreateProducerThread(context.Context, pgtype.UUID) (db.AgentThread, error) {
	return f.producerThread, nil
}

func (f *fakeWorkerRuntime) CreateProducerPendingSignal(_ context.Context, params agentruntime.CreateProducerPendingSignalParams) (db.ProducerPendingSignal, error) {
	f.signals = append(f.signals, params)
	return db.ProducerPendingSignal{ID: uuidWithByte(byte(80 + len(f.signals))), WorkspaceID: params.WorkspaceID, ProducerThreadID: params.ProducerThreadID, SignalType: params.SignalType}, nil
}

func (f *fakeWorkerRuntime) ListActiveAgentTasksByWorkspace(context.Context, pgtype.UUID) ([]db.AgentTask, error) {
	return f.activeTasks, nil
}

func (f *fakeWorkerRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	f.createdTasks = append(f.createdTasks, params)
	return db.AgentTask{ID: uuidWithByte(byte(90 + len(f.createdTasks))), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, Role: params.Role, TaskType: params.TaskType, Status: "queued", Input: params.Input}, nil
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
	createdNode            db.CreateAgentGenerationNodeParams
	createNodeCalls        int
	existingNode           db.MediaNode
	nodes                  []db.MediaNode
	assets                 map[pgtype.UUID]db.MediaAsset
	versions               map[pgtype.UUID]db.ArtifactVersion
	keyElementState        db.KeyElementState
	keyElementStateUpdates []db.UpdateKeyElementStateParams
	existingEdges          map[[2]pgtype.UUID]db.MediaEdge
	createdEdges           []db.CreateMediaEdgeParams
	statusUpdates          []db.UpdateShotStatusParams
	renderPlanCompletions  []db.MarkRenderPlanCompletedParams
	renderPlanSubmissions  []db.MarkRenderPlanSubmittedParams
	voiceoverNodeLinks     []db.SetAudioPlanVoiceoverNodeParams
	bgmNodeLinks           []db.SetAudioPlanBGMNodeParams
}

type fakeWorkerNodeBroadcaster struct {
	node        db.MediaNode
	updatedNode db.MediaNode
}

func (f *fakeWorkerNodeBroadcaster) BroadcastAgentNodeCreated(_ pgtype.UUID, node db.MediaNode) {
	f.node = node
}

func (f *fakeWorkerNodeBroadcaster) BroadcastAgentNodeUpdated(_ pgtype.UUID, node db.MediaNode) {
	f.updatedNode = node
}

type fakeWorkerAgentBroadcaster struct {
	event db.AgentEvent
}

func (f *fakeWorkerAgentBroadcaster) BroadcastAgentEvent(_ pgtype.UUID, event db.AgentEvent) {
	f.event = event
}

type fakeWorkerProducerEnqueuer struct {
	tasks []db.AgentTask
}

func (f *fakeWorkerProducerEnqueuer) EnqueueProducerTask(_ context.Context, task db.AgentTask) {
	f.tasks = append(f.tasks, task)
}

func (f *fakeWorkerStore) CreateAgentGenerationNode(_ context.Context, params db.CreateAgentGenerationNodeParams) (db.MediaNode, error) {
	f.createdNode = params
	f.createNodeCalls++
	return db.MediaNode{
		ID:                 uuidWithByte(20),
		WorkspaceID:        params.WorkspaceID,
		NodeType:           params.NodeType,
		Title:              params.Title,
		Prompt:             params.Prompt,
		PromptTemplate:     params.Prompt,
		OperationType:      params.OperationType,
		Status:             db.NodeStatusQueued,
		Source:             "agent",
		ShotID:             params.ShotID,
		Metadata:           params.Metadata,
		SemanticKey:        params.SemanticKey,
		DisplayName:        params.DisplayName,
		ArtifactKind:       params.ArtifactKind,
		SourceRenderPlanID: params.SourceRenderPlanID,
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

func (f *fakeWorkerStore) GetArtifactVersionByID(_ context.Context, id pgtype.UUID) (db.ArtifactVersion, error) {
	if f.versions != nil {
		version, ok := f.versions[id]
		if ok {
			return version, nil
		}
	}
	return db.ArtifactVersion{}, errors.New("version not found")
}

func (f *fakeWorkerStore) GetKeyElementStateByID(_ context.Context, params db.GetKeyElementStateByIDParams) (db.KeyElementState, error) {
	if f.keyElementState.ID == params.ID && f.keyElementState.WorkspaceID == params.WorkspaceID {
		return f.keyElementState, nil
	}
	return db.KeyElementState{}, errors.New("key element state not found")
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

func (f *fakeWorkerStore) UpdateKeyElementState(_ context.Context, params db.UpdateKeyElementStateParams) (db.KeyElementState, error) {
	f.keyElementStateUpdates = append(f.keyElementStateUpdates, params)
	f.keyElementState.ReferenceStatus = params.ReferenceStatus
	f.keyElementState.ReferenceNodeID = params.ReferenceNodeID
	f.keyElementState.ReferenceVersionID = params.ReferenceVersionID
	return f.keyElementState, nil
}

func (f *fakeWorkerStore) MarkRenderPlanCompleted(_ context.Context, params db.MarkRenderPlanCompletedParams) (db.RenderPlan, error) {
	f.renderPlanCompletions = append(f.renderPlanCompletions, params)
	return db.RenderPlan{ID: params.ID, WorkspaceID: params.WorkspaceID, Status: params.Status, OutputNodeID: params.OutputNodeID, OutputVersionID: params.OutputVersionID}, nil
}

func (f *fakeWorkerStore) MarkRenderPlanSubmitted(_ context.Context, params db.MarkRenderPlanSubmittedParams) (db.RenderPlan, error) {
	f.renderPlanSubmissions = append(f.renderPlanSubmissions, params)
	return db.RenderPlan{ID: params.ID, WorkspaceID: params.WorkspaceID, Status: "submitted", SubmittedWorkerTaskID: params.SubmittedWorkerTaskID, OutputNodeID: params.OutputNodeID}, nil
}

func (f *fakeWorkerStore) SetAudioPlanVoiceoverNode(_ context.Context, params db.SetAudioPlanVoiceoverNodeParams) (db.AudioPlan, error) {
	f.voiceoverNodeLinks = append(f.voiceoverNodeLinks, params)
	return db.AudioPlan{ID: params.ID, WorkspaceID: params.WorkspaceID, VoiceoverNodeID: params.VoiceoverNodeID}, nil
}

func (f *fakeWorkerStore) SetAudioPlanBGMNode(_ context.Context, params db.SetAudioPlanBGMNodeParams) (db.AudioPlan, error) {
	f.bgmNodeLinks = append(f.bgmNodeLinks, params)
	return db.AudioPlan{ID: params.ID, WorkspaceID: params.WorkspaceID, BgmNodeID: params.BgmNodeID}, nil
}

type fakeProductionSubmitter struct {
	intent                production.GenerationIntent
	options               production.RunOptions
	result                production.RunResult
	failureResult         production.RunResult
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
		return f.failureResult, errors.New("temporary submit failure")
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
