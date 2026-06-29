package overview

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestBuilderProjectsWaitingDecisionAndStoryboard(t *testing.T) {
	workspaceID := uuidWithByte(1)
	shotID := uuidWithByte(2)
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: workspaceID, Name: "agent", Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{{
			ID:          shotID,
			WorkspaceID: workspaceID,
			ClientKey:   "shot-01",
			SortOrder:   1,
			Title:       "开场",
			Status:      "preview_ready",
		}},
		events: []db.AgentEvent{{
			ID:          uuidWithByte(3),
			WorkspaceID: workspaceID,
			EventType:   "decision_requested",
			Status:      "pending",
		}},
		nodes: []db.MediaNode{{
			ID:          uuidWithByte(4),
			WorkspaceID: workspaceID,
			ShotID:      shotID,
			NodeType:    db.NodeTypeImage,
			Source:      "agent",
			Status:      db.NodeStatusSucceeded,
			Metadata:    []byte(`{"agent_artifact_kind":"preview_image"}`),
		}},
	})

	got, err := builder.Build(context.Background(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != PhaseWaitingConfirmation {
		t.Fatalf("phase = %q, want %q", got.Phase, PhaseWaitingConfirmation)
	}
	if got.Counts.WaitingDecisions != 1 || got.Counts.ShotsTotal != 1 || got.Counts.PreviewsReady != 1 {
		t.Fatalf("counts = %#v", got.Counts)
	}
	if len(got.Shots) != 1 || got.Shots[0].ClientKey != "shot-01" || got.Shots[0].PreviewStatus != StatusReady {
		t.Fatalf("shots = %#v", got.Shots)
	}
	if got.FinalOutputs == nil {
		t.Fatal("final_outputs must encode as an empty array, not null")
	}
}

func TestBuilderProjectsTimelineUserLabels(t *testing.T) {
	workspaceID := uuidWithByte(1)
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: workspaceID, Name: "agent", Mode: db.WorkspaceModeAgent},
		tasks: []db.AgentTask{{
			ID:          uuidWithByte(2),
			WorkspaceID: workspaceID,
			TaskType:    "composer_turn",
			Role:        "composer",
			Status:      "running",
		}},
		events: []db.AgentEvent{{
			ID:          uuidWithByte(3),
			WorkspaceID: workspaceID,
			EventType:   "composition_succeeded",
			Status:      "pending",
		}},
	})

	got, err := builder.Build(context.Background(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Timeline) < 2 {
		t.Fatalf("timeline = %#v", got.Timeline)
	}
	for _, item := range got.Timeline {
		if item.Label == "composer_turn" || item.Label == "composition_succeeded" {
			t.Fatalf("timeline leaked internal labels = %#v", got.Timeline)
		}
	}
}

func TestBuilderMarksFailedFinalOutputAsNeedsAttention(t *testing.T) {
	workspaceID := uuidWithByte(1)
	finalNodeID := uuidWithByte(2)
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: workspaceID, Name: "agent", Mode: db.WorkspaceModeAgent},
		nodes: []db.MediaNode{{
			ID:            finalNodeID,
			WorkspaceID:   workspaceID,
			NodeType:      db.NodeTypeVideo,
			OperationType: "compose_final_video",
			Status:        db.NodeStatusFailed,
			Source:        "agent",
			Metadata:      []byte(`{"agent_artifact_kind":"final_video"}`),
		}},
	})

	got, err := builder.Build(context.Background(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != PhaseNeedsAttention {
		t.Fatalf("phase = %q, want %q", got.Phase, PhaseNeedsAttention)
	}
	if got.Counts.FailedTasks != 1 {
		t.Fatalf("failed count = %d, want 1", got.Counts.FailedTasks)
	}
}

func TestBuilderProjectsAudioPlanAndFinalReviewCounts(t *testing.T) {
	workspaceID := uuidWithByte(1)
	voiceNodeID := uuidWithByte(2)
	bgmNodeID := uuidWithByte(3)
	timelinePlanID := uuidWithByte(4)
	finalNodeID := uuidWithByte(5)
	builder := NewBuilder(fakeStore{
		workspace: db.Workspace{ID: workspaceID, Name: "agent", Mode: db.WorkspaceModeAgent},
		audioPlan: db.AudioPlan{
			ID:                uuidWithByte(6),
			WorkspaceID:       workspaceID,
			Status:            "composing",
			Title:             "Audio plan",
			Language:          "zh",
			VoiceoverScript:   "新品上线，轻松完成创意短片。",
			VoiceProfile:      []byte(`{"speaker":"zh_female"}`),
			BgmPlan:           []byte(`{"mood":"bright"}`),
			VoiceoverNodeID:   voiceNodeID,
			BgmNodeID:         bgmNodeID,
			TimelinePlanID:    timelinePlanID,
			TargetDurationSec: pgtype.Float8{Float64: 12, Valid: true},
		},
		nodes: []db.MediaNode{
			{ID: voiceNodeID, WorkspaceID: workspaceID, NodeType: db.NodeTypeAudio, Status: db.NodeStatusSucceeded},
			{ID: bgmNodeID, WorkspaceID: workspaceID, NodeType: db.NodeTypeAudio, Status: db.NodeStatusRunning},
			{ID: finalNodeID, WorkspaceID: workspaceID, NodeType: db.NodeTypeVideo, OperationType: "compose_final_video", Status: db.NodeStatusSucceeded},
		},
		reviews: []db.ReviewRecord{{
			ID:          uuidWithByte(7),
			WorkspaceID: workspaceID,
			NodeID:      finalNodeID,
			ReviewTask:  "final_video_review",
			TargetPhase: "final_video",
			Status:      "accepted_with_warnings",
		}},
	})

	got, err := builder.Build(context.Background(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AudioPlan == nil || got.AudioPlan.Status != "composing" || got.AudioPlan.VoiceoverStatus != StatusReady || got.AudioPlan.BGMStatus != StatusRunning {
		t.Fatalf("audio plan = %#v", got.AudioPlan)
	}
	if got.AudioPlan.TimelinePlanID != uuidString(timelinePlanID) || got.AudioPlan.TargetDurationSec == nil || *got.AudioPlan.TargetDurationSec != 12 {
		t.Fatalf("audio plan timeline/duration = %#v", got.AudioPlan)
	}
	if got.Counts.AudioReady != 1 || got.Counts.AudioMissing != 1 || got.Counts.FinalReviews != 1 {
		t.Fatalf("audio/final review counts = %#v", got.Counts)
	}
}

type fakeStore struct {
	workspace db.Workspace
	shots     []db.Shot
	nodes     []db.MediaNode
	tasks     []db.AgentTask
	events    []db.AgentEvent
	reviews   []db.ReviewRecord
	sandboxes []db.SandboxJob
	audioPlan db.AudioPlan
	jobs      map[pgtype.UUID][]db.GenerationJob
	versions  map[pgtype.UUID][]db.ArtifactVersion
}

func (s fakeStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return s.workspace, nil
}

func (s fakeStore) ListActiveShotsByWorkspace(context.Context, pgtype.UUID) ([]db.Shot, error) {
	return s.shots, nil
}

func (s fakeStore) ListMediaNodesByWorkspace(context.Context, pgtype.UUID) ([]db.MediaNode, error) {
	return s.nodes, nil
}

func (s fakeStore) ListAgentTasksByWorkspace(context.Context, db.ListAgentTasksByWorkspaceParams) ([]db.AgentTask, error) {
	return s.tasks, nil
}

func (s fakeStore) ListAgentEventsByWorkspace(context.Context, db.ListAgentEventsByWorkspaceParams) ([]db.AgentEvent, error) {
	return s.events, nil
}

func (s fakeStore) ListReviewRecordsByWorkspace(context.Context, db.ListReviewRecordsByWorkspaceParams) ([]db.ReviewRecord, error) {
	return s.reviews, nil
}

func (s fakeStore) ListSandboxJobsByWorkspace(context.Context, pgtype.UUID) ([]db.SandboxJob, error) {
	return s.sandboxes, nil
}

func (s fakeStore) GetActiveAudioPlanByWorkspace(context.Context, pgtype.UUID) (db.AudioPlan, error) {
	if !s.audioPlan.ID.Valid {
		return db.AudioPlan{}, pgx.ErrNoRows
	}
	return s.audioPlan, nil
}

func (s fakeStore) ListGenerationJobsByNode(_ context.Context, nodeID pgtype.UUID) ([]db.GenerationJob, error) {
	if s.jobs == nil {
		return nil, nil
	}
	return s.jobs[nodeID], nil
}

func (s fakeStore) ListArtifactVersionsByNode(_ context.Context, nodeID pgtype.UUID) ([]db.ArtifactVersion, error) {
	if s.versions == nil {
		return nil, nil
	}
	return s.versions[nodeID], nil
}

func uuidWithByte(b byte) pgtype.UUID {
	var id pgtype.UUID
	id.Valid = true
	id.Bytes[0] = b
	return id
}
