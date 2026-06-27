package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestResolverResolvesObjectBySemanticKey(t *testing.T) {
	workspaceID := uuidWithByte(1)
	store := &fakeStore{object: db.AgentObjectIndex{WorkspaceID: workspaceID, ObjectType: ObjectShot, ObjectID: uuidWithByte(2), SemanticKey: "shot_03", DisplayName: "箱体细节"}}
	resolver := NewResolver(store)
	got, err := resolver.ResolveObject(context.Background(), workspaceID, ObjectRef{Type: ObjectShot, Key: "shot_03"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ObjectID != uuidWithByte(2) || got.SemanticKey != "shot_03" {
		t.Fatalf("resolved object = %#v", got)
	}
}

func TestResolverRejectsMissingKey(t *testing.T) {
	workspaceID := uuidWithByte(1)
	resolver := NewResolver(&fakeStore{})
	_, err := resolver.ResolveObject(context.Background(), workspaceID, ObjectRef{Type: ObjectShot})
	if !errors.Is(err, ErrInvalidRef) {
		t.Fatalf("err = %v, want ErrInvalidRef", err)
	}
}

func TestResolverResolvesCurrentArtifactByShotAndKind(t *testing.T) {
	workspaceID := uuidWithByte(1)
	version := db.ArtifactVersion{ID: uuidWithByte(9), WorkspaceID: workspaceID, SemanticKey: "shot_03.preview_image.rp1.output.v1", ArtifactKind: "preview_image"}
	resolver := NewResolver(&fakeStore{artifact: version})
	got, err := resolver.ResolveArtifact(context.Background(), workspaceID, ArtifactSelectorRef{
		Scope:        ObjectRef{Type: ObjectShot, Key: "shot_03"},
		ArtifactKind: "preview_image",
		Selector:     "current",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != version.ID {
		t.Fatalf("artifact = %#v", got)
	}
}

type fakeStore struct {
	object   db.AgentObjectIndex
	artifact db.ArtifactVersion
	err      error
}

func (f *fakeStore) GetAgentObjectBySemanticKey(context.Context, db.GetAgentObjectBySemanticKeyParams) (db.AgentObjectIndex, error) {
	if f.err != nil {
		return db.AgentObjectIndex{}, f.err
	}
	return f.object, nil
}

func (f *fakeStore) GetArtifactVersionBySemanticKey(context.Context, db.GetArtifactVersionBySemanticKeyParams) (db.ArtifactVersion, error) {
	if f.err != nil {
		return db.ArtifactVersion{}, f.err
	}
	return f.artifact, nil
}

func (f *fakeStore) GetCurrentArtifactVersionByShotKeyAndKind(context.Context, db.GetCurrentArtifactVersionByShotKeyAndKindParams) (db.ArtifactVersion, error) {
	if f.err != nil {
		return db.ArtifactVersion{}, f.err
	}
	return f.artifact, nil
}

func (f *fakeStore) GetLatestArtifactVersionByShotKeyAndKind(context.Context, db.GetLatestArtifactVersionByShotKeyAndKindParams) (db.ArtifactVersion, error) {
	if f.err != nil {
		return db.ArtifactVersion{}, f.err
	}
	return f.artifact, nil
}

func (f *fakeStore) GetWinnerArtifactVersionByShotKeyAndKind(context.Context, db.GetWinnerArtifactVersionByShotKeyAndKindParams) (db.ArtifactVersion, error) {
	if f.err != nil {
		return db.ArtifactVersion{}, f.err
	}
	return f.artifact, nil
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{15: b}, Valid: true}
}
