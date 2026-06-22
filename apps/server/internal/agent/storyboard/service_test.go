package storyboard

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestUpdateStoryboardReplaceCreatesShotsAndDependencies(t *testing.T) {
	store := &fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
	}
	service := NewService(nil, store)

	out, err := service.UpdateStoryboard(context.Background(), UpdateInput{
		WorkspaceID: uuidWithByte(1),
		Intent:      "replace",
		Shots: []ShotInput{
			{ClientKey: "shot-01", SortOrder: 1, Title: "开场", Brief: map[string]any{"summary": "开场钩子"}},
			{ClientKey: "shot-02", SortOrder: 2, Title: "产品", Brief: map[string]any{"summary": "产品细节"}},
		},
		Dependencies: []DependencyInput{
			{From: "shot-01", To: "shot-02", DependencyType: "story_order", Reason: "顺序播放"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ShotsCreated != 2 {
		t.Fatalf("ShotsCreated = %d, want 2", out.ShotsCreated)
	}
	if len(out.Shots) != 2 {
		t.Fatalf("shots len = %d, want 2", len(out.Shots))
	}
	if len(store.dependencies) != 1 {
		t.Fatalf("dependencies len = %d, want 1", len(store.dependencies))
	}
	if store.dependencies[0].FromShotID != out.Shots[0].ID || store.dependencies[0].ToShotID != out.Shots[1].ID {
		t.Fatalf("dependency = %#v shots=%#v", store.dependencies[0], out.Shots)
	}
}

func TestUpdateStoryboardRejectsUnresolvedDependencyWithoutWriting(t *testing.T) {
	store := &fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
	}
	service := NewService(nil, store)

	_, err := service.UpdateStoryboard(context.Background(), UpdateInput{
		WorkspaceID: uuidWithByte(1),
		Intent:      "replace",
		Shots:       []ShotInput{{ClientKey: "shot-01", SortOrder: 1, Title: "开场"}},
		Dependencies: []DependencyInput{
			{From: "shot-01", To: "shot-missing", DependencyType: "story_order"},
		},
	})
	if !errors.Is(err, ErrShotReferenceNotFound) {
		t.Fatalf("error = %v, want ErrShotReferenceNotFound", err)
	}
	if len(store.dependencies) != 0 {
		t.Fatalf("dependencies should not be written: %#v", store.dependencies)
	}
}

func TestUpdateStoryboardRejectsStudioWorkspace(t *testing.T) {
	store := &fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeStudio},
	}
	service := NewService(nil, store)

	_, err := service.UpdateStoryboard(context.Background(), UpdateInput{
		WorkspaceID: uuidWithByte(1),
		Intent:      "replace",
		Shots:       []ShotInput{{ClientKey: "shot-01", Title: "开场"}},
	})
	if !errors.Is(err, ErrAgentWorkspaceRequired) {
		t.Fatalf("error = %v, want ErrAgentWorkspaceRequired", err)
	}
}

func TestUpdateStoryboardArchivesOmittedShotsOnReplace(t *testing.T) {
	existingID := uuidWithByte(7)
	store := &fakeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: existingID, WorkspaceID: uuidWithByte(1), ClientKey: "shot-old", Title: "旧分镜"},
		},
	}
	service := NewService(nil, store)

	_, err := service.UpdateStoryboard(context.Background(), UpdateInput{
		WorkspaceID: uuidWithByte(1),
		Intent:      "replace",
		Shots:       []ShotInput{{ClientKey: "shot-new", SortOrder: 1, Title: "新分镜"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.archived) != 1 || store.archived[0] != existingID {
		t.Fatalf("archived = %#v, want old shot", store.archived)
	}
}

type fakeStore struct {
	workspace    db.Workspace
	shots        []db.Shot
	dependencies []db.ShotDependency
	archived     []pgtype.UUID
	linkedNodes  []db.UpdateMediaNodeShotParams
	nextID       byte
}

func (f *fakeStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeStore) ListActiveShotsByWorkspace(context.Context, pgtype.UUID) ([]db.Shot, error) {
	return append([]db.Shot{}, f.shots...), nil
}

func (f *fakeStore) CreateShot(_ context.Context, params db.CreateShotParams) (db.Shot, error) {
	f.nextID++
	shot := db.Shot{
		ID:               uuidWithByte(f.nextID),
		WorkspaceID:      params.WorkspaceID,
		ClientKey:        params.ClientKey,
		SortOrder:        params.SortOrder,
		Title:            params.Title,
		Brief:            params.Brief,
		DurationSec:      params.DurationSec,
		NarrativePurpose: params.NarrativePurpose,
		Status:           params.Status,
	}
	f.shots = append(f.shots, shot)
	return shot, nil
}

func (f *fakeStore) UpdateShot(_ context.Context, params db.UpdateShotParams) (db.Shot, error) {
	for i, shot := range f.shots {
		if shot.ID == params.ID {
			shot.ClientKey = params.ClientKey
			shot.SortOrder = params.SortOrder
			shot.Title = params.Title
			shot.Brief = params.Brief
			shot.DurationSec = params.DurationSec
			shot.NarrativePurpose = params.NarrativePurpose
			shot.Status = params.Status
			f.shots[i] = shot
			return shot, nil
		}
	}
	return db.Shot{}, errors.New("missing shot")
}

func (f *fakeStore) ArchiveShot(_ context.Context, params db.ArchiveShotParams) (db.Shot, error) {
	f.archived = append(f.archived, params.ID)
	for _, shot := range f.shots {
		if shot.ID == params.ID {
			return shot, nil
		}
	}
	return db.Shot{}, nil
}

func (f *fakeStore) DeleteShotDependenciesByWorkspace(context.Context, pgtype.UUID) error {
	f.dependencies = nil
	return nil
}

func (f *fakeStore) DeleteShotDependenciesForShot(context.Context, db.DeleteShotDependenciesForShotParams) error {
	return nil
}

func (f *fakeStore) CreateShotDependency(_ context.Context, params db.CreateShotDependencyParams) (db.ShotDependency, error) {
	dep := db.ShotDependency{
		ID:               uuidWithByte(byte(len(f.dependencies) + 50)),
		WorkspaceID:      params.WorkspaceID,
		FromShotID:       params.FromShotID,
		ToShotID:         params.ToShotID,
		DependencyType:   params.DependencyType,
		RequiredArtifact: params.RequiredArtifact,
		InjectionRole:    params.InjectionRole,
		BlockingPhase:    params.BlockingPhase,
		StalePolicy:      params.StalePolicy,
		Reason:           params.Reason,
	}
	f.dependencies = append(f.dependencies, dep)
	return dep, nil
}

func (f *fakeStore) UpdateMediaNodeShot(_ context.Context, params db.UpdateMediaNodeShotParams) (db.MediaNode, error) {
	f.linkedNodes = append(f.linkedNodes, params)
	return db.MediaNode{ID: params.ID, WorkspaceID: params.WorkspaceID, ShotID: params.ShotID}, nil
}

func uuidWithByte(b byte) pgtype.UUID {
	var id pgtype.UUID
	id.Valid = true
	id.Bytes[15] = b
	return id
}
