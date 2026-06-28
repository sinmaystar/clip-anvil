package composer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/compose"
	"github.com/jackc/pgx/v5/pgtype"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestComposerGraphSubmitsCompositionAndPersistsCheckpoint(t *testing.T) {
	sourceNode := db.MediaNode{ID: uuidWithByte(21), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeVideo, Title: "shot-01 shot video", CurrentVersionID: uuidWithByte(31), AssetID: uuidWithByte(41)}
	store := composerStoreWithSourceVideo(sourceNode)
	runtime := &fakeComposerRuntime{}
	productionService := &fakeComposerProduction{
		result: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(50), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeVideo},
			Job:     db.GenerationJob{ID: uuidWithByte(60), TargetNodeID: uuidWithByte(50), OperationType: "compose_final_video", Status: db.JobStatusQueued},
			Version: db.ArtifactVersion{ID: uuidWithByte(70), NodeID: uuidWithByte(50), JobID: uuidWithByte(60), Status: db.JobStatusQueued},
		},
	}
	graph, err := NewGraph(GraphConfig{
		Runtime:    runtime,
		Store:      store,
		Production: productionService,
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := graph.Run(context.Background(), GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(10),
		Input:       CompositionInput{VideoNodeRefs: []string{"shot-01 shot video"}, Strategy: "cut fast"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if out.Output.OperationType != "compose_final_video" || out.Output.Status != "submitted" {
		t.Fatalf("output = %#v", out.Output)
	}
	if runtime.checkpointKey == "" {
		t.Fatal("checkpoint was not persisted")
	}
	if runtime.threadCheckpoint != runtime.checkpointKey {
		t.Fatalf("thread checkpoint = %q, want %q", runtime.threadCheckpoint, runtime.checkpointKey)
	}
	var checkpoint map[string]any
	if err := json.Unmarshal(runtime.checkpointValue, &checkpoint); err != nil {
		t.Fatal(err)
	}
	if checkpoint["node_id"] == "" || checkpoint["generation_job_id"] == "" {
		t.Fatalf("checkpoint = %#v", checkpoint)
	}
	if len(runtime.eventsByType["composition_submitted"]) != 1 {
		t.Fatalf("events = %#v", runtime.eventsByType)
	}
}

func TestComposerGraphCompileCapturesGraphInfo(t *testing.T) {
	registry := agenteino.NewGraphInfoRegistry()
	_, err := NewGraph(GraphConfig{
		Runtime:    &fakeComposerRuntime{},
		Store:      composerStoreWithSourceVideo(sourceShotVideoNode(21, "shot-01 shot video")),
		Production: &fakeComposerProduction{result: production.RunResult{Node: db.MediaNode{ID: uuidWithByte(50)}, Job: db.GenerationJob{ID: uuidWithByte(60)}, Version: db.ArtifactVersion{ID: uuidWithByte(70)}}},
		CompileCallbacks: []compose.GraphCompileCallback{
			registry.CompileCallback(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	info, ok := registry.Get("composer_final")
	if !ok {
		t.Fatal("composer graph info was not captured")
	}
	for _, node := range []string{"load_composition_context", "create_final_node", "submit_composition_intent", "persist_checkpoint_and_events"} {
		if _, ok := info.Nodes[node]; !ok {
			t.Fatalf("node %q missing from graph info", node)
		}
	}
}

func TestComposerGraphPlacesFinalVideoBelowShotRows(t *testing.T) {
	sourceNodes := []db.MediaNode{
		sourceShotVideoNode(21, "shot-01 shot video"),
		sourceShotVideoNode(22, "shot-02 shot video"),
		sourceShotVideoNode(23, "shot-03 shot video"),
		sourceShotVideoNode(24, "shot-04 shot video"),
		sourceShotVideoNode(25, "shot-05 shot video"),
	}
	store := composerStoreWithSourceVideo(sourceNodes...)
	graph, err := NewGraph(GraphConfig{
		Runtime:    &fakeComposerRuntime{},
		Store:      store,
		Production: &fakeComposerProduction{result: production.RunResult{Node: db.MediaNode{ID: uuidWithByte(50)}, Job: db.GenerationJob{ID: uuidWithByte(60)}, Version: db.ArtifactVersion{ID: uuidWithByte(70)}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Run(context.Background(), GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(10),
		Input: CompositionInput{VideoNodeRefs: []string{
			"shot-01 shot video",
			"shot-02 shot video",
			"shot-03 shot video",
			"shot-04 shot video",
			"shot-05 shot video",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.createdNode.CanvasY < 1700 {
		t.Fatalf("final video y = %v, want below all shot rows", store.createdNode.CanvasY)
	}
}

func TestComposerGraphDerivesVideoRefsFromShotWinners(t *testing.T) {
	shot1 := db.Shot{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), SortOrder: 2, Title: "shot 2"}
	shot2 := db.Shot{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), SortOrder: 1, Title: "shot 1"}
	node1 := sourceShotVideoNode(21, "shot-02 shot video")
	node1.SemanticKey = "shot_02.shot_video.r3.node"
	node1.ShotID = shot1.ID
	node1.Metadata = mustJSON(map[string]any{"agent_artifact_kind": "shot_video"})
	node2 := sourceShotVideoNode(22, "shot-01 shot video")
	node2.SemanticKey = "shot_01.shot_video.r1.node"
	node2.ShotID = shot2.ID
	node2.Metadata = mustJSON(map[string]any{"agent_artifact_kind": "shot_video"})
	staleDuplicate := sourceShotVideoNode(23, "shot-02 shot video")
	staleDuplicate.SemanticKey = "shot_02.shot_video.r1.node"
	staleDuplicate.ShotID = shot1.ID
	staleDuplicate.CurrentVersionID = pgtype.UUID{}
	staleDuplicate.Metadata = mustJSON(map[string]any{"agent_artifact_kind": "shot_video"})
	store := composerStoreWithSourceVideo(node1, node2, staleDuplicate)
	store.shots = []db.Shot{shot1, shot2}
	store.nodesByShot = map[pgtype.UUID][]db.MediaNode{
		shot1.ID: {staleDuplicate, node1},
		shot2.ID: {node2},
	}
	productionService := &fakeComposerProduction{
		result: production.RunResult{
			Node:    db.MediaNode{ID: uuidWithByte(50), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeVideo},
			Job:     db.GenerationJob{ID: uuidWithByte(60), TargetNodeID: uuidWithByte(50), OperationType: "compose_final_video", Status: db.JobStatusQueued},
			Version: db.ArtifactVersion{ID: uuidWithByte(70), NodeID: uuidWithByte(50), JobID: uuidWithByte(60), Status: db.JobStatusQueued},
		},
	}
	graph, err := NewGraph(GraphConfig{
		Runtime:    &fakeComposerRuntime{},
		Store:      store,
		Production: productionService,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = graph.Run(context.Background(), GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(10),
		Input: CompositionInput{
			SourceStoryboardNodeID: "21000000-0000-0000-0000-000000000000",
			Instructions:           "把已完成分镜拼成 20 秒营销视频。",
			Strategy:               "把已完成分镜拼成 20 秒营销视频。",
			TemplateKey:            "simple_concat",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(productionService.intent.InputRefs) != 2 {
		t.Fatalf("input refs = %#v", productionService.intent.InputRefs)
	}
	if productionService.intent.InputRefs[0].NodeID != node2.ID || productionService.intent.InputRefs[1].NodeID != node1.ID {
		t.Fatalf("input refs order = %#v", productionService.intent.InputRefs)
	}
	if productionService.intent.Semantic.ArtifactKind != "final_video" {
		t.Fatalf("intent semantic = %#v", productionService.intent.Semantic)
	}
	var metadata map[string]any
	if err := json.Unmarshal(store.createdNode.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["composer_template_key"] != "simple_concat" || metadata["source_storyboard_node_id"] == "" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if store.createdNode.SemanticKey == "" || store.createdNode.ArtifactKind != "final_video" {
		t.Fatalf("created final node semantic = key %q kind %q", store.createdNode.SemanticKey, store.createdNode.ArtifactKind)
	}
}

func composerStoreWithSourceVideo(sourceNodes ...db.MediaNode) *fakeComposerStore {
	versions := map[pgtype.UUID]db.ArtifactVersion{}
	assets := map[pgtype.UUID]db.MediaAsset{}
	for _, sourceNode := range sourceNodes {
		versions[sourceNode.CurrentVersionID] = db.ArtifactVersion{ID: sourceNode.CurrentVersionID, WorkspaceID: sourceNode.WorkspaceID, NodeID: sourceNode.ID, AssetID: sourceNode.AssetID, InputHash: "hash-1", Status: db.JobStatusSucceeded}
		assets[sourceNode.AssetID] = db.MediaAsset{ID: sourceNode.AssetID, WorkspaceID: sourceNode.WorkspaceID, Type: db.AssetTypeVideo, Mime: "video/mp4", StorageUrl: pgtypeText("workspace/final-input.mp4")}
	}
	return &fakeComposerStore{
		nodes:    sourceNodes,
		versions: versions,
		assets:   assets,
	}
}

func sourceShotVideoNode(idByte byte, title string) db.MediaNode {
	return db.MediaNode{ID: uuidWithByte(idByte), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeVideo, Title: title, CurrentVersionID: uuidWithByte(idByte + 20), AssetID: uuidWithByte(idByte + 40)}
}

func pgtypeText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

var _ = agentruntime.UpsertCheckpointParams{}
