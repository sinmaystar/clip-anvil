package hitl

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type CheckpointRuntime interface {
	UpsertCheckpoint(ctx context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error)
	GetCheckpoint(ctx context.Context, key string) (db.EinoCheckpoint, error)
	DeleteCheckpoint(ctx context.Context, key string) error
}

type CheckpointScope struct {
	WorkspaceID   pgtype.UUID
	ThreadID      pgtype.UUID
	TaskID        pgtype.UUID
	InterruptType string
}

type CheckpointStore struct {
	runtime CheckpointRuntime
	scope   CheckpointScope
}

func NewCheckpointStore(runtime CheckpointRuntime, scope CheckpointScope) *CheckpointStore {
	return &CheckpointStore{runtime: runtime, scope: scope}
}

func (s *CheckpointStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	cp, err := s.runtime.GetCheckpoint(ctx, checkPointID)
	if err != nil {
		if errors.Is(err, agentruntime.ErrInvalidRequest) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return cp.Value, true, nil
}

func (s *CheckpointStore) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	_, err := s.runtime.UpsertCheckpoint(ctx, agentruntime.UpsertCheckpointParams{
		Key:         checkPointID,
		WorkspaceID: s.scope.WorkspaceID,
		ThreadID:    s.scope.ThreadID,
		TaskID:      s.scope.TaskID,
		Value:       checkPoint,
		Metadata:    checkpointMetadata(s.scope),
	})
	return err
}

func (s *CheckpointStore) Delete(ctx context.Context, checkPointID string) error {
	return s.runtime.DeleteCheckpoint(ctx, checkPointID)
}

func checkpointMetadata(scope CheckpointScope) []byte {
	raw, err := json.Marshal(map[string]any{
		"workspace_id":   uuidString(scope.WorkspaceID),
		"thread_id":      uuidString(scope.ThreadID),
		"task_id":        uuidString(scope.TaskID),
		"interrupt_type": scope.InterruptType,
	})
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}
