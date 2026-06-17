package sandbox

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusCreating   = "creating"
	StatusRunning    = "running"
	StatusUnhealthy  = "unhealthy"
	StatusTerminated = "terminated"

	DefaultWorkdir = "/workspace"
)

func VolumeName(workspaceID pgtype.UUID) string {
	return "sandbox-ws-" + uuidString(workspaceID)
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}
