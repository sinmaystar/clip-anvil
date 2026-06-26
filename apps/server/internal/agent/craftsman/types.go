package craftsman

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/schema"
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
	Input       []byte
}

type GraphInput struct {
	WorkspaceID      pgtype.UUID
	ThreadID         pgtype.UUID
	TaskID           pgtype.UUID
	ShotID           pgtype.UUID
	Mode             string
	ExecutionPolicy  string
	ParentToolCallID string
	MaxAttempts      int
	WorkerParams     map[string]any
}

type GraphOutput struct {
	AssistantText    string
	Strategy         Strategy
	WorkerTask       db.AgentTask
	Metadata         map[string]any
	SameTurnMessages []CraftsmanSameTurnMessage
}

type Context struct {
	Input            GraphInput
	Shot             db.Shot
	Messages         []db.AgentMessage
	Nodes            []NodeState
	Dependencies     []db.ShotDependency
	SourceMaterials  []NodeState
	Text             string
	Structured       map[string]any
	ToolInfos        []*schema.ToolInfo
	SameTurnMessages []CraftsmanSameTurnMessage
}

type CraftsmanSameTurnMessage struct {
	Role          string
	MessageType   string
	Content       string
	ToolCallID    string
	ToolName      string
	ToolArguments map[string]any
}

type NodeState struct {
	Node     db.MediaNode
	Jobs     []db.GenerationJob
	Versions []db.ArtifactVersion
}

type Strategy struct {
	Strategy       string         `json:"strategy"`
	PreviewPrompt  string         `json:"preview_prompt"`
	VideoPrompt    string         `json:"video_prompt,omitempty"`
	NegativePrompt string         `json:"negative_prompt,omitempty"`
	StyleNotes     []string       `json:"style_notes,omitempty"`
	InputNodeRefs  []string       `json:"input_node_refs,omitempty"`
	OutputType     string         `json:"output_type,omitempty"`
	OperationType  string         `json:"operation_type,omitempty"`
	Model          ModelSpec      `json:"model,omitempty"`
	Params         map[string]any `json:"params,omitempty"`
}

type ModelSpec struct {
	Provider string `json:"provider,omitempty"`
	ModelID  string `json:"model_id,omitempty"`
}

type ToolCallingResponder interface {
	Respond(ctx context.Context, context Context) (CraftsmanTurnOutput, error)
}

type CraftsmanTurnOutput struct {
	AssistantText string
	Metadata      map[string]any
	ModelMessage  *schema.Message
}
