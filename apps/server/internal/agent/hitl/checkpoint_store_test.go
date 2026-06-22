package hitl

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestCheckpointStorePutGetDelete(t *testing.T) {
	runtime := &fakeCheckpointRuntime{}
	store := NewCheckpointStore(runtime, CheckpointScope{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
	})

	if err := store.Set(context.Background(), "cp-1", []byte("checkpoint")); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(context.Background(), "cp-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || string(got) != "checkpoint" {
		t.Fatalf("checkpoint = %q ok=%v", got, ok)
	}
	if err := store.Delete(context.Background(), "cp-1"); err != nil {
		t.Fatal(err)
	}
	_, ok, err = store.Get(context.Background(), "cp-1")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("checkpoint should be deleted")
	}
}

func TestCheckpointStorePersistsMetadata(t *testing.T) {
	runtime := &fakeCheckpointRuntime{}
	store := NewCheckpointStore(runtime, CheckpointScope{
		WorkspaceID:   uuidWithByte(1),
		ThreadID:      uuidWithByte(2),
		TaskID:        uuidWithByte(3),
		InterruptType: "request_user_decision",
	})

	if err := store.Set(context.Background(), "cp-1", []byte("checkpoint")); err != nil {
		t.Fatal(err)
	}

	var metadata map[string]any
	if err := json.Unmarshal(runtime.metadata, &metadata); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if metadata["interrupt_type"] != "request_user_decision" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata["workspace_id"] != "01000000-0000-0000-0000-000000000000" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

type fakeCheckpointRuntime struct {
	values   map[string][]byte
	metadata []byte
}

func (f *fakeCheckpointRuntime) UpsertCheckpoint(_ context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error) {
	if f.values == nil {
		f.values = map[string][]byte{}
	}
	f.values[params.Key] = params.Value
	f.metadata = params.Metadata
	return db.EinoCheckpoint{Key: params.Key, Value: params.Value, Metadata: params.Metadata}, nil
}

func (f *fakeCheckpointRuntime) GetCheckpoint(_ context.Context, key string) (db.EinoCheckpoint, error) {
	if value, ok := f.values[key]; ok {
		return db.EinoCheckpoint{Key: key, Value: value, Metadata: f.metadata}, nil
	}
	return db.EinoCheckpoint{}, agentruntime.ErrInvalidRequest
}

func (f *fakeCheckpointRuntime) DeleteCheckpoint(_ context.Context, key string) error {
	delete(f.values, key)
	return nil
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}
