package einoruntime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestCheckpointKeyRoundTripScope(t *testing.T) {
	workspaceID := uuidWithByte(1)
	threadID := uuidWithByte(2)
	taskID := uuidWithByte(3)

	key := CheckpointKey("producer_turn", workspaceID, threadID, taskID)
	scope, ok := ParseCheckpointKey(key)
	if !ok {
		t.Fatalf("ParseCheckpointKey(%q) returned false", key)
	}

	if scope.GraphName != "producer_turn" {
		t.Fatalf("GraphName = %q", scope.GraphName)
	}
	if scope.WorkspaceID != workspaceID {
		t.Fatalf("WorkspaceID = %v", scope.WorkspaceID)
	}
	if scope.ThreadID != threadID {
		t.Fatalf("ThreadID = %v", scope.ThreadID)
	}
	if scope.TaskID != taskID {
		t.Fatalf("TaskID = %v", scope.TaskID)
	}
}

func TestParseCheckpointKeyRejectsMalformedKeys(t *testing.T) {
	values := []string{
		"",
		"agent:eino",
		"agent:eino:producer_turn",
		"agent:old:producer_turn:01000000-0000-0000-0000-000000000000:02000000-0000-0000-0000-000000000000:03000000-0000-0000-0000-000000000000",
		"agent:eino:producer_turn:not-a-uuid:02000000-0000-0000-0000-000000000000:03000000-0000-0000-0000-000000000000",
	}
	for _, value := range values {
		if _, ok := ParseCheckpointKey(value); ok {
			t.Fatalf("ParseCheckpointKey(%q) returned true", value)
		}
	}
}

func TestCheckpointStoreSetParsesScopeAndStoresRawBlob(t *testing.T) {
	runtime := &fakeCheckpointRuntime{}
	store := NewCheckpointStore(runtime, slog.Default())
	workspaceID := uuidWithByte(1)
	threadID := uuidWithByte(2)
	taskID := uuidWithByte(3)
	key := CheckpointKey("producer_turn", workspaceID, threadID, taskID)

	if err := store.Set(context.Background(), key, []byte("raw-eino-checkpoint")); err != nil {
		t.Fatal(err)
	}

	got := runtime.upserts[0]
	if got.Key != key {
		t.Fatalf("Key = %q", got.Key)
	}
	if got.WorkspaceID != workspaceID {
		t.Fatalf("WorkspaceID = %v", got.WorkspaceID)
	}
	if got.ThreadID != threadID {
		t.Fatalf("ThreadID = %v", got.ThreadID)
	}
	if got.TaskID != taskID {
		t.Fatalf("TaskID = %v", got.TaskID)
	}
	if string(got.Value) != "raw-eino-checkpoint" {
		t.Fatalf("Value = %q", got.Value)
	}

	var metadata map[string]any
	if err := json.Unmarshal(got.Metadata, &metadata); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if metadata["source"] != "eino_native" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata["graph_name"] != "producer_turn" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata["checkpoint_key"] != key {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata["checkpoint_version"].(float64) != 1 {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestCheckpointStoreRejectsInvalidCheckpointKey(t *testing.T) {
	store := NewCheckpointStore(&fakeCheckpointRuntime{}, slog.Default())
	err := store.Set(context.Background(), "bad-key", []byte("raw"))
	if !errors.Is(err, ErrInvalidCheckpointKey) {
		t.Fatalf("Set error = %v", err)
	}
}

func TestCheckpointStoreGetReturnsRawBlob(t *testing.T) {
	runtime := &fakeCheckpointRuntime{
		values: map[string]db.EinoCheckpoint{
			"cp-1": {Key: "cp-1", Value: []byte("raw")},
		},
	}
	store := NewCheckpointStore(runtime, slog.Default())

	got, ok, err := store.Get(context.Background(), "cp-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || string(got) != "raw" {
		t.Fatalf("Get = %q ok=%v", got, ok)
	}
}

func TestCheckpointStoreGetMissingReturnsFalse(t *testing.T) {
	runtime := &fakeCheckpointRuntime{getErr: pgx.ErrNoRows}
	store := NewCheckpointStore(runtime, slog.Default())

	_, ok, err := store.Get(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("Get returned ok for missing checkpoint")
	}
}

func TestCheckpointStoreDeleteDelegatesRuntime(t *testing.T) {
	runtime := &fakeCheckpointRuntime{}
	store := NewCheckpointStore(runtime, slog.Default())

	if err := store.Delete(context.Background(), "cp-1"); err != nil {
		t.Fatal(err)
	}
	if runtime.deleted != "cp-1" {
		t.Fatalf("deleted = %q", runtime.deleted)
	}
}

type fakeCheckpointRuntime struct {
	upserts []agentruntime.UpsertCheckpointParams
	values  map[string]db.EinoCheckpoint
	getErr  error
	deleted string
}

func (f *fakeCheckpointRuntime) UpsertCheckpoint(_ context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error) {
	f.upserts = append(f.upserts, params)
	if f.values == nil {
		f.values = map[string]db.EinoCheckpoint{}
	}
	cp := db.EinoCheckpoint{
		Key:         params.Key,
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		TaskID:      params.TaskID,
		Value:       params.Value,
		Metadata:    params.Metadata,
	}
	f.values[params.Key] = cp
	return cp, nil
}

func (f *fakeCheckpointRuntime) GetCheckpoint(_ context.Context, key string) (db.EinoCheckpoint, error) {
	if f.getErr != nil {
		return db.EinoCheckpoint{}, f.getErr
	}
	if cp, ok := f.values[key]; ok {
		return cp, nil
	}
	return db.EinoCheckpoint{}, pgx.ErrNoRows
}

func (f *fakeCheckpointRuntime) DeleteCheckpoint(_ context.Context, key string) error {
	f.deleted = key
	return nil
}

func uuidWithByte(value byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{value}, Valid: true}
}
