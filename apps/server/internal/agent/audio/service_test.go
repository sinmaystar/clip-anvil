package audio

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestServiceCreateReplacesExistingActivePlan(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	workspaceID := uuidWithByte(1)
	store.workspace = db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeAgent}
	store.active = db.AudioPlan{ID: uuidWithByte(2), WorkspaceID: workspaceID, Status: "draft"}

	created, err := service.Upsert(context.Background(), UpsertInput{
		WorkspaceID:      workspaceID,
		TaskID:           uuidWithByte(3),
		Mode:             "replace_draft",
		Title:            "营销短视频音频方案",
		Language:         "zh",
		VoiceoverScript:  "现在出发，让旅程更轻松。",
		VoiceProfile:     map[string]any{"speaker": "marketing_female_clear"},
		BGMPlan:          map[string]any{"source": "generated", "model": "seed-audio-1.0"},
		CuePlan:          []CueInput{{ShotRef: "shot_01", StartSec: 0, EndSec: 3.2, Text: "现在出发，让旅程更轻松。"}},
		GenerationParams: map[string]any{"format": "mp3", "sample_rate": 48000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.archivedActive {
		t.Fatal("expected existing active audio plans to be archived")
	}
	if created.Status != "waiting_for_user" {
		t.Fatalf("status = %q, want waiting_for_user", created.Status)
	}
	if created.SemanticKey != "audio_plan.active" {
		t.Fatalf("semantic_key = %q, want audio_plan.active", created.SemanticKey)
	}
}

func TestServiceApproveRequiresExistingActivePlan(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	workspaceID := uuidWithByte(1)
	store.workspace = db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeAgent}
	store.activeErr = pgx.ErrNoRows

	_, err := service.Upsert(context.Background(), UpsertInput{WorkspaceID: workspaceID, Mode: "approve"})
	if !errors.Is(err, ErrAudioPlanNotFound) {
		t.Fatalf("error = %v, want ErrAudioPlanNotFound", err)
	}
}

func TestServiceRejectsStudioWorkspace(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	workspaceID := uuidWithByte(1)
	store.workspace = db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeStudio}

	_, err := service.Upsert(context.Background(), UpsertInput{WorkspaceID: workspaceID, Mode: "replace_draft"})
	if !errors.Is(err, ErrAgentWorkspaceRequired) {
		t.Fatalf("error = %v, want ErrAgentWorkspaceRequired", err)
	}
}

type fakeStore struct {
	workspace      db.Workspace
	active         db.AudioPlan
	activeErr      error
	archivedActive bool
	created        []db.CreateAudioPlanParams
	updated        []db.UpdateAudioPlanParams
	statusUpdates  []db.UpdateAudioPlanStatusParams
}

func newFakeStore() *fakeStore {
	return &fakeStore{}
}

func (f *fakeStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeStore) GetActiveAudioPlanByWorkspace(context.Context, pgtype.UUID) (db.AudioPlan, error) {
	if f.activeErr != nil {
		return db.AudioPlan{}, f.activeErr
	}
	return f.active, nil
}

func (f *fakeStore) ArchiveActiveAudioPlansByWorkspace(context.Context, pgtype.UUID) error {
	f.archivedActive = true
	return nil
}

func (f *fakeStore) CreateAudioPlan(_ context.Context, arg db.CreateAudioPlanParams) (db.AudioPlan, error) {
	f.created = append(f.created, arg)
	return db.AudioPlan{
		ID:                uuidWithByte(byte(10 + len(f.created))),
		WorkspaceID:       arg.WorkspaceID,
		Status:            arg.Status,
		Title:             arg.Title,
		PlanKind:          arg.PlanKind,
		Language:          arg.Language,
		TargetDurationSec: arg.TargetDurationSec,
		VoiceoverScript:   arg.VoiceoverScript,
		VoiceProfile:      arg.VoiceProfile,
		BgmPlan:           arg.BgmPlan,
		CuePlan:           arg.CuePlan,
		GenerationParams:  arg.GenerationParams,
		CreatedByTaskID:   arg.CreatedByTaskID,
		SemanticKey:       arg.SemanticKey,
		DisplayName:       arg.DisplayName,
	}, nil
}

func (f *fakeStore) UpdateAudioPlan(_ context.Context, arg db.UpdateAudioPlanParams) (db.AudioPlan, error) {
	f.updated = append(f.updated, arg)
	return db.AudioPlan{
		ID:                arg.ID,
		WorkspaceID:       arg.WorkspaceID,
		Status:            arg.Status,
		Title:             arg.Title,
		Language:          arg.Language,
		TargetDurationSec: arg.TargetDurationSec,
		VoiceoverScript:   arg.VoiceoverScript,
		VoiceProfile:      arg.VoiceProfile,
		BgmPlan:           arg.BgmPlan,
		CuePlan:           arg.CuePlan,
		GenerationParams:  arg.GenerationParams,
		DisplayName:       arg.DisplayName,
	}, nil
}

func (f *fakeStore) UpdateAudioPlanStatus(_ context.Context, arg db.UpdateAudioPlanStatusParams) (db.AudioPlan, error) {
	f.statusUpdates = append(f.statusUpdates, arg)
	return db.AudioPlan{ID: arg.ID, WorkspaceID: arg.WorkspaceID, Status: arg.Status}, nil
}

func uuidWithByte(value byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{15: value}, Valid: true}
}
