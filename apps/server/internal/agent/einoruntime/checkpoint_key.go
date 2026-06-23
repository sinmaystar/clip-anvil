package einoruntime

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const CheckpointKeyPrefix = "agent:eino"

var ErrInvalidCheckpointKey = errors.New("invalid_eino_checkpoint_key")

type CheckpointScope struct {
	GraphName   string
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
}

func CheckpointKey(graphName string, workspaceID, threadID, taskID pgtype.UUID) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s",
		CheckpointKeyPrefix,
		graphName,
		uuidString(workspaceID),
		uuidString(threadID),
		uuidString(taskID),
	)
}

func ParseCheckpointKey(key string) (CheckpointScope, bool) {
	parts := strings.Split(key, ":")
	if len(parts) != 6 {
		return CheckpointScope{}, false
	}
	if parts[0]+":"+parts[1] != CheckpointKeyPrefix {
		return CheckpointScope{}, false
	}
	if parts[2] == "" {
		return CheckpointScope{}, false
	}
	workspaceID, ok := parsePGUUID(parts[3])
	if !ok {
		return CheckpointScope{}, false
	}
	threadID, ok := parsePGUUID(parts[4])
	if !ok {
		return CheckpointScope{}, false
	}
	taskID, ok := parsePGUUID(parts[5])
	if !ok {
		return CheckpointScope{}, false
	}
	return CheckpointScope{
		GraphName:   parts[2],
		WorkspaceID: workspaceID,
		ThreadID:    threadID,
		TaskID:      taskID,
	}, true
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return "00000000-0000-0000-0000-000000000000"
	}
	return uuid.UUID(id.Bytes).String()
}

func parsePGUUID(value string) (pgtype.UUID, bool) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}
