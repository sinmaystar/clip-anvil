package pss

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestProducerPSSMentionsEmptyStoryboard(t *testing.T) {
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Name: "agent-ws", Mode: db.WorkspaceModeAgent},
	})

	pss, err := builder.BuildProducerPSS(context.Background(), uuidWithByte(1))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pss.Text, "当前还没有 storyboard") {
		t.Fatalf("PSS text = %s", pss.Text)
	}
	if pss.Structured["workspace"] == nil || pss.Structured["shots"] == nil {
		t.Fatalf("structured = %#v", pss.Structured)
	}
}

func TestProducerPSSListsShotsAndDependencies(t *testing.T) {
	shot1 := db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", SortOrder: 1, Title: "开场", Status: "planned", NarrativePurpose: "attention"}
	shot2 := db.Shot{ID: uuidWithByte(3), WorkspaceID: uuidWithByte(1), ClientKey: "shot-02", SortOrder: 2, Title: "产品", Status: "planned"}
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Name: "agent-ws", Mode: db.WorkspaceModeAgent},
		shots:     []db.Shot{shot1, shot2},
		dependencies: []db.ShotDependency{{
			FromShotID:     shot1.ID,
			ToShotID:       shot2.ID,
			DependencyType: "story_order",
			BlockingPhase:  "preview_generation",
			Reason:         "先介绍再展示",
		}},
		nodes: []db.MediaNode{{ID: uuidWithByte(4), Title: "design.png", NodeType: db.NodeTypeImage, Source: "agent", Status: "succeeded"}},
	})

	pss, err := builder.BuildProducerPSS(context.Background(), uuidWithByte(1))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[shot-01] 开场", "shot-01 -> shot-02", "design.png"} {
		if !strings.Contains(pss.Text, want) {
			t.Fatalf("PSS missing %q:\n%s", want, pss.Text)
		}
	}
	if strings.Contains(pss.Text, "http://") || strings.Contains(pss.Text, "api_key") {
		t.Fatalf("PSS leaks unsafe values: %s", pss.Text)
	}
}

func TestProducerPSSListsPreviewGenerationState(t *testing.T) {
	shot := db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", SortOrder: 1, Title: "开场", Status: "planned"}
	node := db.MediaNode{
		ID:            uuidWithByte(4),
		WorkspaceID:   uuidWithByte(1),
		ShotID:        shot.ID,
		Title:         "shot-01 preview image",
		NodeType:      db.NodeTypeImage,
		Source:        "agent",
		Status:        "queued",
		OperationType: "text_to_image",
		Metadata:      []byte(`{"agent_artifact_kind":"preview_image"}`),
	}
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Name: "agent-ws", Mode: db.WorkspaceModeAgent},
		shots:     []db.Shot{shot},
		nodes:     []db.MediaNode{node},
		jobs: map[pgtype.UUID][]db.GenerationJob{
			node.ID: {{ID: uuidWithByte(5), TargetNodeID: node.ID, OperationType: "text_to_image", Status: db.JobStatusQueued}},
		},
		versions: map[pgtype.UUID][]db.ArtifactVersion{
			node.ID: {{ID: uuidWithByte(6), NodeID: node.ID, Status: db.JobStatusQueued}},
		},
	})

	pss, err := builder.BuildProducerPSS(context.Background(), uuidWithByte(1))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Preview: shot-01 preview image", "job=queued", "version=queued"} {
		if !strings.Contains(pss.Text, want) {
			t.Fatalf("PSS missing %q:\n%s", want, pss.Text)
		}
	}
	shots := pss.Structured["shots"].([]map[string]any)
	previewNodes := shots[0]["preview_nodes"].([]map[string]any)
	if len(previewNodes) != 1 || previewNodes[0]["job_status"] != "queued" {
		t.Fatalf("preview_nodes = %#v", previewNodes)
	}
}

func TestProducerPSSListsReviewState(t *testing.T) {
	shot := db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", SortOrder: 1, Title: "开场", Status: "preview_ready"}
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Name: "agent-ws", Mode: db.WorkspaceModeAgent},
		shots:     []db.Shot{shot},
		reviews: []db.ReviewRecord{{
			ID:           uuidWithByte(7),
			WorkspaceID:  uuidWithByte(1),
			ShotID:       shot.ID,
			TargetPhase:  "preview_image",
			Status:       "rejected",
			AttemptNo:    1,
			MaxAttempts:  3,
			OverallScore: pgtype.Float4{Float32: 0.52, Valid: true},
			Critique:     "商品不够清晰",
		}},
	})

	pss, err := builder.BuildProducerPSS(context.Background(), uuidWithByte(1))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Review: shot-01 preview_image rejected", "score=0.52", "商品不够清晰", "retry=1/3"} {
		if !strings.Contains(pss.Text, want) {
			t.Fatalf("PSS missing %q:\n%s", want, pss.Text)
		}
	}
	reviews := pss.Structured["reviews"].([]map[string]any)
	if len(reviews) != 1 || reviews[0]["status"] != "rejected" {
		t.Fatalf("reviews = %#v", reviews)
	}
}

func TestProducerPSSListsShotVideosAndFinalOutputs(t *testing.T) {
	shot := db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", SortOrder: 1, Title: "开场", Status: "video_ready"}
	shotVideo := db.MediaNode{
		ID:            uuidWithByte(4),
		WorkspaceID:   uuidWithByte(1),
		ShotID:        shot.ID,
		Title:         "shot-01 video",
		NodeType:      db.NodeTypeVideo,
		Source:        "agent",
		Status:        "succeeded",
		OperationType: "image_to_video",
		Metadata:      []byte(`{"agent_artifact_kind":"shot_video"}`),
	}
	finalVideo := db.MediaNode{
		ID:            uuidWithByte(7),
		WorkspaceID:   uuidWithByte(1),
		Title:         "final video",
		NodeType:      db.NodeTypeVideo,
		Source:        "agent",
		Status:        "succeeded",
		OperationType: "compose_final_video",
		Metadata:      []byte(`{"agent_artifact_kind":"final_video"}`),
	}
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Name: "agent-ws", Mode: db.WorkspaceModeAgent},
		shots:     []db.Shot{shot},
		nodes:     []db.MediaNode{shotVideo, finalVideo},
		jobs: map[pgtype.UUID][]db.GenerationJob{
			shotVideo.ID:  {{ID: uuidWithByte(5), TargetNodeID: shotVideo.ID, OperationType: "image_to_video", Status: db.JobStatusSucceeded}},
			finalVideo.ID: {{ID: uuidWithByte(8), TargetNodeID: finalVideo.ID, OperationType: "compose_final_video", Status: db.JobStatusSucceeded}},
		},
		versions: map[pgtype.UUID][]db.ArtifactVersion{
			shotVideo.ID:  {{ID: uuidWithByte(6), NodeID: shotVideo.ID, Status: db.JobStatusSucceeded}},
			finalVideo.ID: {{ID: uuidWithByte(9), NodeID: finalVideo.ID, Status: db.JobStatusSucceeded}},
		},
	})

	pss, err := builder.BuildProducerPSS(context.Background(), uuidWithByte(1))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"分镜视频", "ShotVideo: shot-01 video", "成片", "FinalVideo: final video"} {
		if !strings.Contains(pss.Text, want) {
			t.Fatalf("PSS missing %q:\n%s", want, pss.Text)
		}
	}
	shotVideos := pss.Structured["shot_videos"].([]map[string]any)
	if len(shotVideos) != 1 || shotVideos[0]["job_status"] != "succeeded" || shotVideos[0]["shot"] != "shot-01" {
		t.Fatalf("shot_videos = %#v", shotVideos)
	}
	finalOutputs := pss.Structured["final_outputs"].([]map[string]any)
	if len(finalOutputs) != 1 || finalOutputs[0]["version_status"] != "succeeded" {
		t.Fatalf("final_outputs = %#v", finalOutputs)
	}
}

func TestProducerPSSListsDependencyStatus(t *testing.T) {
	shot1 := db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", SortOrder: 1, Title: "开场"}
	shot2 := db.Shot{ID: uuidWithByte(3), WorkspaceID: uuidWithByte(1), ClientKey: "shot-02", SortOrder: 2, Title: "演示"}
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Name: "agent-ws", Mode: db.WorkspaceModeAgent},
		shots:     []db.Shot{shot1, shot2},
		events: []db.AgentEvent{{
			ID:        uuidWithByte(8),
			EventType: "shot_blocked",
			Status:    "pending",
			Scope:     []byte(`{"from_shot_id":"00000000-0000-0000-0000-000000000002","to_shot_id":"00000000-0000-0000-0000-000000000003","phase":"review"}`),
			Payload:   []byte(`{"phase":"review","ready":false,"blocked_reasons":[{"code":"upstream_review_accepted_missing","reason":"需要先通过评审"}]}`),
		}},
	})

	pss, err := builder.BuildProducerPSS(context.Background(), uuidWithByte(1))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"依赖状态", "shot_blocked", "shot-01 -> shot-02", "ready=false"} {
		if !strings.Contains(pss.Text, want) {
			t.Fatalf("PSS missing %q:\n%s", want, pss.Text)
		}
	}
	status := pss.Structured["dependency_status"].([]map[string]any)
	if len(status) != 1 || status[0]["event_type"] != "shot_blocked" || status[0]["from"] != "shot-01" {
		t.Fatalf("dependency_status = %#v", status)
	}
}

type fakeStore struct {
	workspace    db.Workspace
	nodes        []db.MediaNode
	shots        []db.Shot
	dependencies []db.ShotDependency
	tasks        []db.AgentTask
	events       []db.AgentEvent
	jobs         map[pgtype.UUID][]db.GenerationJob
	versions     map[pgtype.UUID][]db.ArtifactVersion
	reviews      []db.ReviewRecord
}

func (f fakeStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return f.workspace, nil
}

func (f fakeStore) ListMediaNodesByWorkspace(context.Context, pgtype.UUID) ([]db.MediaNode, error) {
	return f.nodes, nil
}

func (f fakeStore) ListActiveShotsByWorkspace(context.Context, pgtype.UUID) ([]db.Shot, error) {
	return f.shots, nil
}

func (f fakeStore) ListShotDependenciesByWorkspace(context.Context, pgtype.UUID) ([]db.ShotDependency, error) {
	return f.dependencies, nil
}

func (f fakeStore) ListActiveAgentTasksByWorkspace(context.Context, pgtype.UUID) ([]db.AgentTask, error) {
	return f.tasks, nil
}

func (f fakeStore) ListAgentEventsByWorkspaceStatus(context.Context, db.ListAgentEventsByWorkspaceStatusParams) ([]db.AgentEvent, error) {
	return f.events, nil
}

func (f fakeStore) ListGenerationJobsByNode(_ context.Context, nodeID pgtype.UUID) ([]db.GenerationJob, error) {
	return f.jobs[nodeID], nil
}

func (f fakeStore) ListArtifactVersionsByNode(_ context.Context, nodeID pgtype.UUID) ([]db.ArtifactVersion, error) {
	return f.versions[nodeID], nil
}

func (f fakeStore) ListReviewRecordsByWorkspace(context.Context, db.ListReviewRecordsByWorkspaceParams) ([]db.ReviewRecord, error) {
	return f.reviews, nil
}

func uuidWithByte(b byte) pgtype.UUID {
	var id pgtype.UUID
	id.Valid = true
	id.Bytes[15] = b
	return id
}
