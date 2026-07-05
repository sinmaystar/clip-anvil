package sandbox

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestPersistAgentRemotionValidationUpdatesAttemptSnapshot(t *testing.T) {
	repo := &fakeAgentRemotionAttemptRepo{}
	attemptID := pgtype.UUID{Bytes: [16]byte{0x42}, Valid: true}
	snapshot := validAgentRemotionSnapshot(t, map[string]string{
		"GeneratedComposition.tsx": `export default function Video(){ return null; }`,
	}, []byte(`{"output":{"fps":30}}`))
	result := ValidateAgentRemotionSnapshot(snapshot)

	_, err := PersistAgentRemotionValidation(context.Background(), repo, attemptID, snapshot, result)
	if err != nil {
		t.Fatal(err)
	}
	arg := repo.arg
	if arg.ID != attemptID || arg.SourceHash != snapshot.SourceHash || arg.PropsHash != snapshot.PropsHash {
		t.Fatalf("unexpected update arg: %#v", arg)
	}
	if arg.Status != "validated" {
		t.Fatalf("status = %q, want validated", arg.Status)
	}
	if len(arg.SourceSnapshot) == 0 || len(arg.PropsJson) == 0 || len(arg.ValidationResult) == 0 || len(arg.CompileResult) == 0 {
		t.Fatalf("missing persisted payloads: %#v", arg)
	}
}

func TestPersistAgentRemotionValidationStoresValidationFailureStatus(t *testing.T) {
	repo := &fakeAgentRemotionAttemptRepo{}
	attemptID := pgtype.UUID{Bytes: [16]byte{0x24}, Valid: true}
	snapshot := validAgentRemotionSnapshot(t, map[string]string{
		"GeneratedComposition.tsx": `import fs from "fs"; export default function Video(){ return null; }`,
	}, []byte(`{"output":{"fps":30}}`))
	result := ValidateAgentRemotionSnapshot(snapshot)

	_, err := PersistAgentRemotionValidation(context.Background(), repo, attemptID, snapshot, result)
	if err != nil {
		t.Fatal(err)
	}
	if repo.arg.Status != "validation_failed" {
		t.Fatalf("status = %q, want validation_failed", repo.arg.Status)
	}
}

type fakeAgentRemotionAttemptRepo struct {
	arg db.UpdateRemotionRendererAttemptSnapshotParams
}

func (f *fakeAgentRemotionAttemptRepo) UpdateRemotionRendererAttemptSnapshot(_ context.Context, arg db.UpdateRemotionRendererAttemptSnapshotParams) (db.RemotionRendererAttempt, error) {
	f.arg = arg
	return db.RemotionRendererAttempt{ID: arg.ID, Status: arg.Status}, nil
}
