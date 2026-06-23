package composer

import (
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrInvalidConfig = errors.New("invalid composer config")
	ErrInvalidInput  = errors.New("invalid composer input")
)

type RunTaskInput struct {
	Task db.AgentTask
}

type GraphInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	Input       CompositionInput
}

type GraphOutput struct {
	Output        CompositionOutput
	CheckpointKey string
}

type CompositionInput struct {
	VideoNodeRefs []string       `json:"video_node_refs"`
	Strategy      string         `json:"strategy,omitempty"`
	Params        map[string]any `json:"params,omitempty"`
}

type CompositionOutput struct {
	Status            string `json:"status"`
	NodeID            string `json:"node_id"`
	GenerationJobID   string `json:"generation_job_id"`
	ArtifactVersionID string `json:"artifact_version_id"`
	OperationType     string `json:"operation_type"`
}
