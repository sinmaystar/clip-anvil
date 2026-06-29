package craftsman

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestContextLoaderBuildsShotScopedContext(t *testing.T) {
	store := &fakeContextStore{
		shot: db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", Title: "开场", Status: "planned"},
		nodes: []db.MediaNode{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), Title: "shot preview", NodeType: "image", Status: "queued"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(3), Title: "other shot", NodeType: "image", Status: "queued"},
		},
		jobs: map[pgtype.UUID][]db.GenerationJob{
			uuidWithByte(11): {{ID: uuidWithByte(21), TargetNodeID: uuidWithByte(11), OperationType: "text_to_image", Status: db.JobStatusQueued}},
		},
		versions: map[pgtype.UUID][]db.ArtifactVersion{
			uuidWithByte(11): {{ID: uuidWithByte(31), NodeID: uuidWithByte(11), Status: db.JobStatusQueued}},
		},
	}
	runtime := &fakeMessageRuntime{messages: []db.AgentMessage{{ID: uuidWithByte(41), Role: "assistant", MessageType: "text", Content: []byte(`{"text":"old strategy"}`)}}}
	loader := ContextLoader{Store: store, Runtime: runtime}

	out, err := loader.Load(context.Background(), GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(4),
		TaskID:      uuidWithByte(5),
		ShotID:      uuidWithByte(2),
		Mode:        "preview_image",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Shot.ClientKey != "shot-01" {
		t.Fatalf("shot = %#v", out.Shot)
	}
	if len(out.Nodes) != 1 {
		t.Fatalf("nodes = %#v", out.Nodes)
	}
	if !strings.Contains(out.Text, "shot-01") || !strings.Contains(out.Text, "shot preview") || !strings.Contains(out.Text, "queued") {
		t.Fatalf("context text = %q", out.Text)
	}
	if !strings.Contains(out.Text, "target_phase: preview_image") {
		t.Fatalf("context text missing current target phase: %q", out.Text)
	}
	if strings.Contains(out.Text, "other shot") {
		t.Fatalf("context leaked other shot: %q", out.Text)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("messages = %#v", out.Messages)
	}
}

func TestContextLoaderIncludesDependenciesAndSourceMaterials(t *testing.T) {
	store := &fakeContextStore{
		shot: db.Shot{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), ClientKey: "shot-02", Title: "卖点证明", Status: "planned"},
		nodes: []db.MediaNode{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), Title: "shot-02 preview", NodeType: "image", Status: "queued"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(3), Title: "other shot preview", NodeType: "image", Status: "queued"},
		},
		sourceNodes: []db.MediaNode{
			{ID: uuidWithByte(13), WorkspaceID: uuidWithByte(1), Title: "product.png", NodeType: "image", Status: "succeeded", Source: "agent", OperationType: "upload", AssetID: uuidWithByte(33)},
		},
		dependencies: []db.ShotDependency{
			{WorkspaceID: uuidWithByte(1), FromShotID: uuidWithByte(1), ToShotID: uuidWithByte(2), DependencyType: "continuity", BlockingPhase: "preview_generation", InjectionRole: "style_ref", Reason: "保持商品摆位连续"},
			{WorkspaceID: uuidWithByte(1), FromShotID: uuidWithByte(2), ToShotID: uuidWithByte(4), DependencyType: "handoff", BlockingPhase: "preview_generation", InjectionRole: "subject_ref", Reason: "下一镜头延续"},
			{WorkspaceID: uuidWithByte(1), FromShotID: uuidWithByte(5), ToShotID: uuidWithByte(6), DependencyType: "unrelated", BlockingPhase: "preview_generation", InjectionRole: "ignore", Reason: "不相关"},
		},
	}
	loader := ContextLoader{Store: store}

	out, err := loader.Load(context.Background(), GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(4),
		TaskID:      uuidWithByte(5),
		ShotID:      uuidWithByte(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Dependencies) != 2 {
		t.Fatalf("dependencies = %#v", out.Dependencies)
	}
	if len(out.SourceMaterials) != 1 || out.SourceMaterials[0].Node.Title != "product.png" {
		t.Fatalf("source materials = %#v", out.SourceMaterials)
	}
	if !strings.Contains(out.Text, "Dependencies") || !strings.Contains(out.Text, "保持商品摆位连续") {
		t.Fatalf("context text missing dependencies: %q", out.Text)
	}
	if !strings.Contains(out.Text, "Source Materials") || !strings.Contains(out.Text, "product.png") {
		t.Fatalf("context text missing source materials: %q", out.Text)
	}
	if strings.Contains(out.Text, "other shot preview") || strings.Contains(out.Text, "不相关") {
		t.Fatalf("context leaked unrelated data: %q", out.Text)
	}
}

func TestContextLoaderBuildsAudioPlanScopedContext(t *testing.T) {
	audioPlanID := uuidWithByte(6)
	store := &fakeContextStore{
		audioPlan: db.AudioPlan{
			ID:                audioPlanID,
			WorkspaceID:       uuidWithByte(1),
			Status:            "approved",
			Title:             "全片旁白与 BGM",
			Language:          "zh",
			TargetDurationSec: pgtype.Float8{Float64: 12, Valid: true},
			VoiceoverScript:   "现在出发，让旅程更轻松。",
			VoiceProfile:      []byte(`{"style":"warm female voice"}`),
			BgmPlan:           []byte(`{"source":"generated","model":"seed-audio-1.0","style":"bright electronic pop"}`),
			CuePlan:           []byte(`[{"shot_ref":"shot_01","start_sec":0,"end_sec":4,"text":"现在出发"},{"shot_ref":"shot_02","start_sec":4,"end_sec":8,"text":"让旅程更轻松"}]`),
			SemanticKey:       "audio_plan.active",
		},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01", Title: "开场", Status: "preview_ready", SortOrder: 1},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot_02", Title: "收尾", Status: "preview_ready", SortOrder: 2},
		},
		renderPlans: []db.RenderPlan{
			{ID: uuidWithByte(21), WorkspaceID: uuidWithByte(1), ScopeType: "audio_plan", ScopeID: audioPlanID, TargetPhase: "voiceover_audio", Status: "waiting_for_producer", ModelPromptProfile: "seed_audio_1", Operation: "text_to_audio"},
		},
	}
	runtime := &fakeMessageRuntime{messages: []db.AgentMessage{{ID: uuidWithByte(41), Role: "assistant", MessageType: "text", Content: []byte(`{"text":"old audio strategy"}`)}}}
	loader := ContextLoader{Store: store, Runtime: runtime}

	out, err := loader.Load(context.Background(), GraphInput{
		WorkspaceID:     uuidWithByte(1),
		ThreadID:        uuidWithByte(4),
		TaskID:          uuidWithByte(5),
		ScopeType:       "audio_plan",
		ScopeID:         audioPlanID,
		ScopeKey:        "audio_plan.active",
		Mode:            "voiceover_audio",
		ExecutionPolicy: "wait_for_producer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.AudioPlan.ID != audioPlanID {
		t.Fatalf("audio plan = %#v", out.AudioPlan)
	}
	if len(out.Shots) != 2 || out.Shots[0].ClientKey != "shot_01" {
		t.Fatalf("shots = %#v", out.Shots)
	}
	if len(out.RenderPlans) != 1 || out.RenderPlans[0].TargetPhase != "voiceover_audio" {
		t.Fatalf("render plans = %#v", out.RenderPlans)
	}
	audioStructured, ok := out.Structured["audio_plan"].(map[string]any)
	if !ok || audioStructured["status"] != "approved" {
		t.Fatalf("structured = %#v", out.Structured)
	}
	for _, want := range []string{"AudioPlan", "全片旁白与 BGM", "voiceover_audio", "shot_01", "现在出发", "seed-audio-1.0", "waiting_for_producer"} {
		if !strings.Contains(out.Text, want) {
			t.Fatalf("context text missing %q: %q", want, out.Text)
		}
	}
	if len(out.Messages) != 1 {
		t.Fatalf("messages = %#v", out.Messages)
	}
}

func TestContextLoaderRejectsUnapprovedAudioPlan(t *testing.T) {
	audioPlanID := uuidWithByte(6)
	store := &fakeContextStore{
		audioPlan: db.AudioPlan{
			ID:          audioPlanID,
			WorkspaceID: uuidWithByte(1),
			Status:      "waiting_for_user",
			Title:       "待确认音频方案",
		},
	}
	loader := ContextLoader{Store: store}

	_, err := loader.Load(context.Background(), GraphInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(4),
		TaskID:      uuidWithByte(5),
		ScopeType:   "audio_plan",
		ScopeID:     audioPlanID,
		Mode:        "bgm_audio",
	})
	if err == nil || !strings.Contains(err.Error(), "approved") {
		t.Fatalf("err = %v", err)
	}
}

type fakeContextStore struct {
	shot            db.Shot
	keyElementState db.KeyElementState
	audioPlan       db.AudioPlan
	shots           []db.Shot
	renderPlans     []db.RenderPlan
	nodes           []db.MediaNode
	sourceNodes     []db.MediaNode
	dependencies    []db.ShotDependency
	jobs            map[pgtype.UUID][]db.GenerationJob
	versions        map[pgtype.UUID][]db.ArtifactVersion
}

func (f *fakeContextStore) GetShotByID(context.Context, pgtype.UUID) (db.Shot, error) {
	return f.shot, nil
}

func (f *fakeContextStore) GetKeyElementStateByID(context.Context, db.GetKeyElementStateByIDParams) (db.KeyElementState, error) {
	return f.keyElementState, nil
}

func (f *fakeContextStore) GetAudioPlan(_ context.Context, params db.GetAudioPlanParams) (db.AudioPlan, error) {
	if f.audioPlan.ID != params.ID || f.audioPlan.WorkspaceID != params.WorkspaceID {
		return db.AudioPlan{}, ErrInvalidInput
	}
	return f.audioPlan, nil
}

func (f *fakeContextStore) GetActiveAudioPlanByWorkspace(_ context.Context, workspaceID pgtype.UUID) (db.AudioPlan, error) {
	if f.audioPlan.WorkspaceID != workspaceID || f.audioPlan.ArchivedAt.Valid {
		return db.AudioPlan{}, ErrInvalidInput
	}
	return f.audioPlan, nil
}

func (f *fakeContextStore) ListActiveShotsByWorkspace(_ context.Context, workspaceID pgtype.UUID) ([]db.Shot, error) {
	out := []db.Shot{}
	for _, shot := range f.shots {
		if shot.WorkspaceID == workspaceID {
			out = append(out, shot)
		}
	}
	return out, nil
}

func (f *fakeContextStore) ListRenderPlansByScope(_ context.Context, params db.ListRenderPlansByScopeParams) ([]db.RenderPlan, error) {
	out := []db.RenderPlan{}
	for _, plan := range f.renderPlans {
		if plan.WorkspaceID == params.WorkspaceID && plan.ScopeType == params.ScopeType && plan.ScopeID == params.ScopeID {
			out = append(out, plan)
		}
	}
	return out, nil
}

func (f *fakeContextStore) ListMediaNodesByShot(_ context.Context, params db.ListMediaNodesByShotParams) ([]db.MediaNode, error) {
	out := []db.MediaNode{}
	for _, node := range f.nodes {
		if node.WorkspaceID == params.WorkspaceID && node.ShotID == params.ShotID {
			out = append(out, node)
		}
	}
	return out, nil
}

func (f *fakeContextStore) ListGenerationJobsByNode(_ context.Context, nodeID pgtype.UUID) ([]db.GenerationJob, error) {
	return f.jobs[nodeID], nil
}

func (f *fakeContextStore) ListArtifactVersionsByNode(_ context.Context, nodeID pgtype.UUID) ([]db.ArtifactVersion, error) {
	return f.versions[nodeID], nil
}

func (f *fakeContextStore) ListSourceMaterialNodesByWorkspace(_ context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error) {
	out := []db.MediaNode{}
	for _, node := range f.sourceNodes {
		if node.WorkspaceID == workspaceID {
			out = append(out, node)
		}
	}
	return out, nil
}

func (f *fakeContextStore) ListShotDependenciesByWorkspace(_ context.Context, workspaceID pgtype.UUID) ([]db.ShotDependency, error) {
	out := []db.ShotDependency{}
	for _, dependency := range f.dependencies {
		if dependency.WorkspaceID == workspaceID {
			out = append(out, dependency)
		}
	}
	return out, nil
}

type fakeMessageRuntime struct {
	messages []db.AgentMessage
}

func (f *fakeMessageRuntime) ListMessages(context.Context, pgtype.UUID, int64, int32) ([]db.AgentMessage, error) {
	return f.messages, nil
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}
