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

type fakeStore struct {
	workspace    db.Workspace
	nodes        []db.MediaNode
	shots        []db.Shot
	dependencies []db.ShotDependency
	tasks        []db.AgentTask
	events       []db.AgentEvent
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

func uuidWithByte(b byte) pgtype.UUID {
	var id pgtype.UUID
	id.Valid = true
	id.Bytes[15] = b
	return id
}
