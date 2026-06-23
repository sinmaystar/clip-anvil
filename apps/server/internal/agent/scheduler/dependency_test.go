package scheduler

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestPreviewReadinessBlocksWithoutUpstreamWinner(t *testing.T) {
	store := &fakeDependencyStore{
		dependencies: []db.ShotDependency{{
			WorkspaceID:   uuidWithByte(1),
			FromShotID:    uuidWithByte(2),
			ToShotID:      uuidWithByte(3),
			BlockingPhase: "preview",
			Reason:        "需要延续商品展示",
		}},
		nodesByShot: map[pgtype.UUID][]db.MediaNode{
			uuidWithByte(2): {{ID: uuidWithByte(10), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), NodeType: db.NodeTypeImage}},
		},
	}
	scheduler := NewDependencyScheduler(store)

	result, err := scheduler.Readiness(context.Background(), uuidWithByte(1), uuidWithByte(3), "preview")
	if err != nil {
		t.Fatal(err)
	}
	if result.Ready {
		t.Fatalf("result = %#v, want blocked", result)
	}
	if len(result.BlockedReasons) != 1 {
		t.Fatalf("blocked reasons = %#v", result.BlockedReasons)
	}
}

func TestPreviewReadinessAllowsUpstreamWinner(t *testing.T) {
	store := &fakeDependencyStore{
		dependencies: []db.ShotDependency{{
			WorkspaceID:   uuidWithByte(1),
			FromShotID:    uuidWithByte(2),
			ToShotID:      uuidWithByte(3),
			BlockingPhase: "preview",
		}},
		nodesByShot: map[pgtype.UUID][]db.MediaNode{
			uuidWithByte(2): {{ID: uuidWithByte(10), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), NodeType: db.NodeTypeImage, CurrentVersionID: uuidWithByte(11)}},
		},
	}
	scheduler := NewDependencyScheduler(store)

	result, err := scheduler.Readiness(context.Background(), uuidWithByte(1), uuidWithByte(3), "preview")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready {
		t.Fatalf("result = %#v, want ready", result)
	}
}

func TestReviewReadinessRequiresAcceptedUpstreamReview(t *testing.T) {
	store := &fakeDependencyStore{
		dependencies: []db.ShotDependency{{
			WorkspaceID:   uuidWithByte(1),
			FromShotID:    uuidWithByte(2),
			ToShotID:      uuidWithByte(3),
			BlockingPhase: "review",
		}},
		reviewsByShot: map[pgtype.UUID][]db.ReviewRecord{
			uuidWithByte(2): {{ID: uuidWithByte(20), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), TargetPhase: "preview_image", Status: "rejected"}},
		},
	}
	scheduler := NewDependencyScheduler(store)

	result, err := scheduler.Readiness(context.Background(), uuidWithByte(1), uuidWithByte(3), "review")
	if err != nil {
		t.Fatal(err)
	}
	if result.Ready {
		t.Fatalf("result = %#v, want blocked", result)
	}

	store.reviewsByShot[uuidWithByte(2)] = []db.ReviewRecord{{ID: uuidWithByte(21), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), TargetPhase: "preview_image", Status: "accepted"}}
	result, err = scheduler.Readiness(context.Background(), uuidWithByte(1), uuidWithByte(3), "review")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready {
		t.Fatalf("result = %#v, want ready", result)
	}
}

func TestDispatcherEmitsBlockedAndReadyEvents(t *testing.T) {
	store := &fakeDependencyStore{
		dependencies: []db.ShotDependency{{
			WorkspaceID:   uuidWithByte(1),
			FromShotID:    uuidWithByte(2),
			ToShotID:      uuidWithByte(3),
			BlockingPhase: "preview",
			Reason:        "需要前一镜头参考",
		}},
		nodesByShot: map[pgtype.UUID][]db.MediaNode{
			uuidWithByte(2): {{ID: uuidWithByte(10), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), NodeType: db.NodeTypeImage}},
		},
	}
	runtime := &fakeSchedulerRuntime{}
	dispatcher := NewDispatcher(NewDependencyScheduler(store), runtime)

	if err := dispatcher.NotifyShotUpdated(context.Background(), uuidWithByte(1), uuidWithByte(2), "preview"); err != nil {
		t.Fatal(err)
	}
	if got := runtime.eventTypes(); len(got) != 1 || got[0] != "shot_blocked" {
		t.Fatalf("events = %#v", got)
	}

	runtime.events = nil
	store.nodesByShot[uuidWithByte(2)] = []db.MediaNode{{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ShotID: uuidWithByte(2), NodeType: db.NodeTypeImage, CurrentVersionID: uuidWithByte(12)}}
	if err := dispatcher.NotifyShotUpdated(context.Background(), uuidWithByte(1), uuidWithByte(2), "preview"); err != nil {
		t.Fatal(err)
	}
	if got := runtime.eventTypes(); len(got) != 2 || got[0] != "shot_unblocked" || got[1] != "dependency_ready" {
		t.Fatalf("events = %#v", got)
	}
}

type fakeDependencyStore struct {
	dependencies  []db.ShotDependency
	nodesByShot   map[pgtype.UUID][]db.MediaNode
	reviewsByShot map[pgtype.UUID][]db.ReviewRecord
}

type fakeSchedulerRuntime struct {
	events []agentruntime.CreateEventParams
}

func (f *fakeSchedulerRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	f.events = append(f.events, params)
	return db.AgentEvent{ID: uuidWithByte(byte(40 + len(f.events))), EventType: params.EventType}, nil
}

func (f *fakeSchedulerRuntime) eventTypes() []string {
	out := make([]string, 0, len(f.events))
	for _, event := range f.events {
		out = append(out, event.EventType)
	}
	return out
}

func (f *fakeDependencyStore) ListShotDependenciesByWorkspace(_ context.Context, workspaceID pgtype.UUID) ([]db.ShotDependency, error) {
	out := []db.ShotDependency{}
	for _, dependency := range f.dependencies {
		if dependency.WorkspaceID == workspaceID {
			out = append(out, dependency)
		}
	}
	return out, nil
}

func (f *fakeDependencyStore) ListMediaNodesByShot(_ context.Context, params db.ListMediaNodesByShotParams) ([]db.MediaNode, error) {
	return f.nodesByShot[params.ShotID], nil
}

func (f *fakeDependencyStore) ListReviewRecordsByShotPhase(_ context.Context, params db.ListReviewRecordsByShotPhaseParams) ([]db.ReviewRecord, error) {
	return f.reviewsByShot[params.ShotID], nil
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}
