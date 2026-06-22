package craftsman

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrInvalidConfig   = errors.New("invalid craftsman config")
	ErrInvalidInput    = errors.New("invalid craftsman input")
	ErrInvalidStrategy = errors.New("invalid craftsman strategy")
)

type RunTaskInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	ShotID      pgtype.UUID
}

type GraphInput struct {
	WorkspaceID  pgtype.UUID
	ThreadID     pgtype.UUID
	TaskID       pgtype.UUID
	ShotID       pgtype.UUID
	MaxAttempts  int
	WorkerParams map[string]any
}

type GraphOutput struct {
	Strategy   Strategy
	WorkerTask db.AgentTask
	Metadata   map[string]any
}

type Context struct {
	Input      GraphInput
	Shot       db.Shot
	Messages   []db.AgentMessage
	Nodes      []NodeState
	Text       string
	Structured map[string]any
}

type NodeState struct {
	Node     db.MediaNode
	Jobs     []db.GenerationJob
	Versions []db.ArtifactVersion
}

type Strategy struct {
	Strategy       string         `json:"strategy"`
	PreviewPrompt  string         `json:"preview_prompt"`
	NegativePrompt string         `json:"negative_prompt,omitempty"`
	StyleNotes     []string       `json:"style_notes,omitempty"`
	InputNodeRefs  []string       `json:"input_node_refs,omitempty"`
	Model          ModelSpec      `json:"model,omitempty"`
	Params         map[string]any `json:"params,omitempty"`
}

type ModelSpec struct {
	Provider string `json:"provider,omitempty"`
	ModelID  string `json:"model_id,omitempty"`
}

type ModelResponder interface {
	DraftPreviewStrategy(ctx context.Context, context Context) (Strategy, map[string]any, error)
}
