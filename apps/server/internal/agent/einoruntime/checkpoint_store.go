package einoruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type CheckpointRuntime interface {
	UpsertCheckpoint(ctx context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error)
	GetCheckpoint(ctx context.Context, key string) (db.EinoCheckpoint, error)
	DeleteCheckpoint(ctx context.Context, key string) error
}

type CheckpointStore struct {
	runtime CheckpointRuntime
	logger  *slog.Logger
}

func NewCheckpointStore(runtime CheckpointRuntime, logger *slog.Logger) *CheckpointStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &CheckpointStore{runtime: runtime, logger: logger}
}

func (s *CheckpointStore) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	cp, err := s.runtime.GetCheckpoint(ctx, checkPointID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, agentruntime.ErrInvalidRequest) {
			return nil, false, nil
		}
		s.logger.ErrorContext(ctx, "read eino checkpoint failed",
			"checkpoint_key", checkPointID,
			"error", err,
		)
		return nil, false, err
	}
	return cp.Value, true, nil
}

func (s *CheckpointStore) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	scope, ok := ParseCheckpointKey(checkPointID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrInvalidCheckpointKey, checkPointID)
	}
	metadata := checkpointMetadata(scope, checkPointID)
	_, err := s.runtime.UpsertCheckpoint(ctx, agentruntime.UpsertCheckpointParams{
		Key:         checkPointID,
		WorkspaceID: scope.WorkspaceID,
		ThreadID:    scope.ThreadID,
		TaskID:      scope.TaskID,
		Value:       checkPoint,
		Metadata:    metadata,
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "write eino checkpoint failed",
			"graph_name", scope.GraphName,
			"checkpoint_key", checkPointID,
			"workspace_id", uuidString(scope.WorkspaceID),
			"thread_id", uuidString(scope.ThreadID),
			"task_id", uuidString(scope.TaskID),
			"error", err,
		)
	}
	return err
}

func (s *CheckpointStore) Delete(ctx context.Context, checkPointID string) error {
	return s.runtime.DeleteCheckpoint(ctx, checkPointID)
}

func checkpointMetadata(scope CheckpointScope, checkpointKey string) []byte {
	raw, err := json.Marshal(map[string]any{
		"source":             "eino_native",
		"graph_name":         scope.GraphName,
		"checkpoint_key":     checkpointKey,
		"checkpoint_version": 1,
		"workspace_id":       uuidString(scope.WorkspaceID),
		"thread_id":          uuidString(scope.ThreadID),
		"task_id":            uuidString(scope.TaskID),
	})
	if err != nil {
		return []byte("{}")
	}
	return raw
}
