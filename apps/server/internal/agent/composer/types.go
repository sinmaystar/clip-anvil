package composer

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/schema"
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

type Request struct {
	WorkspaceID            pgtype.UUID
	TaskID                 pgtype.UUID
	ThreadID               pgtype.UUID
	SourceStoryboardNodeID pgtype.UUID
	Instructions           string
}

type Result struct {
	Status            Status
	TimelinePlanID    pgtype.UUID
	OutputNodeID      pgtype.UUID
	ArtifactVersionID pgtype.UUID
	SandboxJobID      pgtype.UUID
	Summary           string
	ErrorMessage      string
}

type Status string

const (
	StatusCompleted Status = "completed"
	StatusBlocked   Status = "blocked"
	StatusFailed    Status = "failed"
)

type TimelinePlan struct {
	TemplateKey string         `json:"template_key"`
	Segments    []Segment      `json:"segments"`
	Transitions []Transition   `json:"transitions,omitempty"`
	Output      OutputSettings `json:"output"`
}

type Segment struct {
	ID            string  `json:"id"`
	AssetID       string  `json:"asset_id"`
	WorkspacePath string  `json:"workspace_path,omitempty"`
	StartSec      float64 `json:"start_sec,omitempty"`
	DurationSec   float64 `json:"duration_sec,omitempty"`
}

type Transition struct {
	FromSegmentID string  `json:"from_segment_id"`
	ToSegmentID   string  `json:"to_segment_id"`
	Type          string  `json:"type"`
	DurationSec   float64 `json:"duration_sec"`
}

type OutputSettings struct {
	WorkspacePath string `json:"workspace_path"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	FPS           int    `json:"fps,omitempty"`
	Format        string `json:"format"`
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
	AssistantText string
}

type CompositionInput struct {
	VideoNodeRefs          []string       `json:"video_node_refs"`
	Strategy               string         `json:"strategy,omitempty"`
	Params                 map[string]any `json:"params,omitempty"`
	SourceStoryboardNodeID string         `json:"source_storyboard_node_id,omitempty"`
	Instructions           string         `json:"instructions,omitempty"`
	TemplateKey            string         `json:"template_key,omitempty"`
	ProducerThreadID       string         `json:"producer_thread_id,omitempty"`
	ProducerTaskID         string         `json:"producer_task_id,omitempty"`
	ParentToolCallID       string         `json:"parent_tool_call_id,omitempty"`
}

type CompositionOutput struct {
	Status            string                    `json:"status"`
	TimelinePlanID    string                    `json:"timeline_plan_id,omitempty"`
	NodeID            string                    `json:"node_id"`
	GenerationJobID   string                    `json:"generation_job_id"`
	ArtifactVersionID string                    `json:"artifact_version_id"`
	SandboxJobID      string                    `json:"sandbox_job_id,omitempty"`
	OperationType     string                    `json:"operation_type"`
	SameTurnMessages  []ComposerSameTurnMessage `json:"same_turn_messages,omitempty"`
}

type ToolResponder interface {
	Respond(ctx context.Context, context Context) (ComposerTurnOutput, error)
}

type ComposerTurnOutput struct {
	AssistantText string
	Result        CompositionOutput
	Metadata      map[string]any
	ModelMessage  *schema.Message
}

type ComposerSameTurnMessage struct {
	Role          string         `json:"role"`
	MessageType   string         `json:"message_type"`
	Content       string         `json:"content"`
	ToolCallID    string         `json:"tool_call_id,omitempty"`
	ToolName      string         `json:"tool_name,omitempty"`
	ToolArguments map[string]any `json:"tool_arguments,omitempty"`
}
