package pss

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
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

func TestProducerPSSListsM1CreativeState(t *testing.T) {
	brief := db.CreativeBrief{ID: uuidWithByte(10), WorkspaceID: uuidWithByte(1), Title: "悦行行李箱机场广告", Status: "active", Concept: "机场轻松出行广告"}
	memory := db.ProjectMemory{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), Version: 1, Status: "active", CoreIntent: "突出短途商务出行", Soul: "轻松出门，行程有掌控感"}
	element := db.KeyElement{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "product_yuexing_luggage", ElementType: "product", Name: "悦行行李箱"}
	state := db.KeyElementState{ID: uuidWithByte(13), WorkspaceID: uuidWithByte(1), KeyElementID: element.ID, ClientKey: "state_uploaded_front", Label: "用户上传素材状态", ReferenceStatus: "ready", IsDefault: true}
	scene := db.Scene{ID: uuidWithByte(14), WorkspaceID: uuidWithByte(1), ClientKey: "scene_airport_departure_hall", SortOrder: 1, Title: "机场出发大厅", Location: "机场出发大厅", Mood: "明亮、轻快"}
	builder := NewBuilder(fakeStore{
		workspace:     db.Workspace{ID: uuidWithByte(1), Name: "agent-ws", Mode: db.WorkspaceModeAgent},
		creativeBrief: &brief,
		projectMemory: &memory,
		elements:      []db.KeyElement{element},
		elementStates: []db.KeyElementState{state},
		scenes:        []db.Scene{scene},
	})

	pss, err := builder.BuildProducerPSS(context.Background(), uuidWithByte(1))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CreativeBrief", "悦行行李箱机场广告", "ProjectMemory", "轻松出门", "关键元素", "product_yuexing_luggage", "机场出发大厅"} {
		if !strings.Contains(pss.Text, want) {
			t.Fatalf("PSS missing %q:\n%s", want, pss.Text)
		}
	}
	if pss.Structured["creative_brief"] == nil || pss.Structured["project_memory"] == nil || pss.Structured["key_elements"] == nil || pss.Structured["scenes"] == nil {
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
		SemanticKey:   "shot-01.preview_image.r1.node",
	}
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Name: "agent-ws", Mode: db.WorkspaceModeAgent},
		shots:     []db.Shot{shot},
		nodes:     []db.MediaNode{node},
		jobs: map[pgtype.UUID][]db.GenerationJob{
			node.ID: {{ID: uuidWithByte(5), TargetNodeID: node.ID, OperationType: "text_to_image", Status: db.JobStatusQueued, SemanticKey: "shot-01.preview_image.r1.job.a1"}},
		},
		versions: map[pgtype.UUID][]db.ArtifactVersion{
			node.ID: {{ID: uuidWithByte(6), NodeID: node.ID, Status: db.JobStatusQueued, SemanticKey: "shot-01.preview_image.r1.artifact.v1"}},
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

func TestProducerPSSDoesNotExposeExecutableUUIDs(t *testing.T) {
	shot := db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01", SortOrder: 1, Title: "开场", Status: "planned", SemanticKey: "shot_01"}
	node := db.MediaNode{
		ID:            uuidWithByte(4),
		WorkspaceID:   uuidWithByte(1),
		ShotID:        shot.ID,
		Title:         "shot_01 preview image",
		NodeType:      db.NodeTypeImage,
		Source:        "agent",
		Status:        "queued",
		OperationType: "text_to_image",
		Metadata:      []byte(`{"agent_artifact_kind":"preview_image"}`),
		SemanticKey:   "shot_01.preview_image.r1.node",
	}
	task := db.AgentTask{
		ID:          uuidWithByte(9),
		WorkspaceID: uuidWithByte(1),
		Role:        "producer",
		TaskType:    "producer_turn",
		Status:      "running",
		SemanticKey: "producer.workspace.agent-ws.producer_turn.t1",
	}
	event := db.AgentEvent{
		ID:         uuidWithByte(10),
		EventType:  "producer_turn_queued",
		SourceRole: "system",
		Status:     "pending",
	}
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Name: "agent-ws", Mode: db.WorkspaceModeAgent},
		shots:     []db.Shot{shot},
		nodes:     []db.MediaNode{node},
		tasks:     []db.AgentTask{task},
		events:    []db.AgentEvent{event},
		jobs: map[pgtype.UUID][]db.GenerationJob{
			node.ID: {{ID: uuidWithByte(5), TargetNodeID: node.ID, OperationType: "text_to_image", Status: db.JobStatusQueued, SemanticKey: "shot_01.preview_image.r1.job.a1"}},
		},
		versions: map[pgtype.UUID][]db.ArtifactVersion{
			node.ID: {{ID: uuidWithByte(6), NodeID: node.ID, Status: db.JobStatusQueued, SemanticKey: "shot_01.preview_image.r1.artifact.v1"}},
		},
	})

	pss, err := builder.BuildProducerPSS(context.Background(), uuidWithByte(1))
	if err != nil {
		t.Fatal(err)
	}
	structured, err := json.Marshal(pss.Structured)
	if err != nil {
		t.Fatal(err)
	}
	combined := pss.Text + "\n" + string(structured)
	for _, leaked := range []string{
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000004",
		"00000000-0000-0000-0000-000000000005",
		"00000000-0000-0000-0000-000000000006",
		"00000000-0000-0000-0000-000000000009",
		"00000000-0000-0000-0000-000000000010",
	} {
		if strings.Contains(combined, leaked) {
			t.Fatalf("PSS leaked executable UUID %s:\n%s", leaked, combined)
		}
	}
	for _, want := range []string{"shot_01.preview_image.r1.node", "shot_01.preview_image.r1.job.a1", "producer.workspace.agent-ws.producer_turn.t1", "producer_turn_queued"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("PSS missing semantic ref %q:\n%s", want, combined)
		}
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
		issues: []db.ArtifactIssue{{
			ID:               uuidWithByte(8),
			WorkspaceID:      uuidWithByte(1),
			ReviewRecordID:   uuidWithByte(7),
			Dimension:        "subject_consistency",
			Severity:         "blocking",
			Status:           "open",
			TargetObjectType: "artifact_version",
			TargetObjectID:   uuidWithByte(9),
			Title:            "商品外观漂移",
			SuggestedFix:     "revise_render_plan",
			FixHint:          "强化悦行银灰色硬壳行李箱 reference binding。",
		}},
	})

	pss, err := builder.BuildProducerPSS(context.Background(), uuidWithByte(1))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Review: shot-01 preview_image rejected", "score=0.52", "商品不够清晰", "retry=1/3", "Issue: subject_consistency blocking 商品外观漂移", "强化悦行银灰色硬壳行李箱"} {
		if !strings.Contains(pss.Text, want) {
			t.Fatalf("PSS missing %q:\n%s", want, pss.Text)
		}
	}
	reviews := pss.Structured["reviews"].([]map[string]any)
	if len(reviews) != 1 || reviews[0]["status"] != "rejected" {
		t.Fatalf("reviews = %#v", reviews)
	}
	issues := pss.Structured["open_issues"].([]map[string]any)
	if len(issues) != 1 || issues[0]["severity"] != "blocking" {
		t.Fatalf("open_issues = %#v", issues)
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

func TestProducerPSSListsShotVideoMissingAndFailedPerShot(t *testing.T) {
	shotMissing := db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", SortOrder: 1, Title: "开场", Status: "preview_ready"}
	shotFailed := db.Shot{ID: uuidWithByte(3), WorkspaceID: uuidWithByte(1), ClientKey: "shot-02", SortOrder: 2, Title: "卖点", Status: "failed"}
	shotRunning := db.Shot{ID: uuidWithByte(4), WorkspaceID: uuidWithByte(1), ClientKey: "shot-03", SortOrder: 3, Title: "行动", Status: "video_running"}
	failedVideo := db.MediaNode{
		ID:            uuidWithByte(5),
		WorkspaceID:   uuidWithByte(1),
		ShotID:        shotFailed.ID,
		Title:         "shot-02 shot video",
		NodeType:      db.NodeTypeVideo,
		Source:        "agent",
		Status:        "failed",
		OperationType: "image_to_video",
		Metadata:      []byte(`{"agent_artifact_kind":"shot_video"}`),
	}
	runningVideo := db.MediaNode{
		ID:            uuidWithByte(6),
		WorkspaceID:   uuidWithByte(1),
		ShotID:        shotRunning.ID,
		Title:         "shot-03 shot video",
		NodeType:      db.NodeTypeVideo,
		Source:        "agent",
		Status:        "queued",
		OperationType: "image_to_video",
		Metadata:      []byte(`{"agent_artifact_kind":"shot_video"}`),
	}
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Name: "agent-ws", Mode: db.WorkspaceModeAgent},
		shots:     []db.Shot{shotMissing, shotFailed, shotRunning},
		nodes:     []db.MediaNode{failedVideo, runningVideo},
		jobs: map[pgtype.UUID][]db.GenerationJob{
			failedVideo.ID:  {{ID: uuidWithByte(7), TargetNodeID: failedVideo.ID, OperationType: "image_to_video", Status: db.JobStatusFailed}},
			runningVideo.ID: {{ID: uuidWithByte(8), TargetNodeID: runningVideo.ID, OperationType: "image_to_video", Status: db.JobStatusRunning}},
		},
		versions: map[pgtype.UUID][]db.ArtifactVersion{
			failedVideo.ID:  {{ID: uuidWithByte(9), NodeID: failedVideo.ID, Status: db.JobStatusFailed}},
			runningVideo.ID: {{ID: uuidWithByte(10), NodeID: runningVideo.ID, Status: db.JobStatusQueued}},
		},
	})

	pss, err := builder.BuildProducerPSS(context.Background(), uuidWithByte(1))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[shot-01] 开场",
		"ShotVideo: missing",
		"shot-02 ShotVideo: shot-02 shot video, state=failed",
		"shot-03 ShotVideo: shot-03 shot video, state=running",
	} {
		if !strings.Contains(pss.Text, want) {
			t.Fatalf("PSS missing %q:\n%s", want, pss.Text)
		}
	}
	shots := pss.Structured["shots"].([]map[string]any)
	if shots[0]["shot_video_state"] != "missing" || shots[1]["shot_video_state"] != "failed" || shots[2]["shot_video_state"] != "running" {
		t.Fatalf("structured shots = %#v", shots)
	}
	shotVideos := pss.Structured["shot_videos"].([]map[string]any)
	if len(shotVideos) != 3 || shotVideos[0]["state"] != "missing" || shotVideos[1]["state"] != "failed" || shotVideos[2]["state"] != "running" {
		t.Fatalf("shot_videos = %#v", shotVideos)
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
	workspace     db.Workspace
	nodes         []db.MediaNode
	shots         []db.Shot
	dependencies  []db.ShotDependency
	creativeBrief *db.CreativeBrief
	projectMemory *db.ProjectMemory
	elements      []db.KeyElement
	elementStates []db.KeyElementState
	scenes        []db.Scene
	tasks         []db.AgentTask
	events        []db.AgentEvent
	jobs          map[pgtype.UUID][]db.GenerationJob
	versions      map[pgtype.UUID][]db.ArtifactVersion
	reviews       []db.ReviewRecord
	issues        []db.ArtifactIssue
}

func (f fakeStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return f.workspace, nil
}

func (f fakeStore) ListMediaNodesByWorkspace(context.Context, pgtype.UUID) ([]db.MediaNode, error) {
	return f.nodes, nil
}

func (f fakeStore) GetActiveCreativeBriefByWorkspace(context.Context, pgtype.UUID) (db.CreativeBrief, error) {
	if f.creativeBrief == nil {
		return db.CreativeBrief{}, pgx.ErrNoRows
	}
	return *f.creativeBrief, nil
}

func (f fakeStore) GetActiveProjectMemoryByWorkspace(context.Context, pgtype.UUID) (db.ProjectMemory, error) {
	if f.projectMemory == nil {
		return db.ProjectMemory{}, pgx.ErrNoRows
	}
	return *f.projectMemory, nil
}

func (f fakeStore) ListActiveKeyElementsByWorkspace(context.Context, pgtype.UUID) ([]db.KeyElement, error) {
	return f.elements, nil
}

func (f fakeStore) ListActiveKeyElementStatesByWorkspace(context.Context, pgtype.UUID) ([]db.KeyElementState, error) {
	return f.elementStates, nil
}

func (f fakeStore) ListActiveScenesByWorkspace(context.Context, pgtype.UUID) ([]db.Scene, error) {
	return f.scenes, nil
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

func (f fakeStore) ListOpenArtifactIssuesByWorkspace(context.Context, db.ListOpenArtifactIssuesByWorkspaceParams) ([]db.ArtifactIssue, error) {
	return f.issues, nil
}

func uuidWithByte(b byte) pgtype.UUID {
	var id pgtype.UUID
	id.Valid = true
	id.Bytes[15] = b
	return id
}
