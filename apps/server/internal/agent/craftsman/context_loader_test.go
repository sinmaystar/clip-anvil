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

type fakeContextStore struct {
	shot            db.Shot
	keyElementState db.KeyElementState
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
