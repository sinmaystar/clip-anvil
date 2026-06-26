package producer

import (
	"context"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ProducerDeltaHandler func(ctx context.Context, delta ProducerStreamDelta) error

type ProducerTurnInput struct {
	WorkspaceID        pgtype.UUID
	ThreadID           pgtype.UUID
	TaskID             pgtype.UUID
	TriggerMessageID   pgtype.UUID
	TriggerMessageSeq  int64
	RuntimeTriggerText string
	EmitDelta          ProducerDeltaHandler
	MaxToolCalls       int
	ToolTimeout        time.Duration
}

type ProducerTurnOutput struct {
	AssistantText    string
	Metadata         map[string]any
	ModelMessage     *schema.Message
	SameTurnMessages []ProducerSameTurnMessage
}

type ProducerStreamDelta struct {
	WorkspaceID string `json:"workspace_id"`
	ThreadID    string `json:"thread_id"`
	TaskID      string `json:"task_id"`
	MessageID   string `json:"message_id,omitempty"`
	BlockID     string `json:"block_id"`
	BlockType   string `json:"block_type"`
	Kind        string `json:"kind"`
	Delta       string `json:"delta"`
	Index       int    `json:"index"`
	Sequence    int64  `json:"sequence"`
}

type ProducerContext struct {
	Input               ProducerTurnInput
	Messages            []db.AgentMessage
	SameTurnMessages    []ProducerSameTurnMessage
	LatestUserText      string
	RuntimeTriggerText  string
	Model               ProducerModelSelection
	ToolInfos           []*schema.ToolInfo
	ImageAttachments    map[string]ProducerImageAttachment
	ProductionStateText string
	ProductionState     map[string]any
	EmitDelta           ProducerDeltaHandler
}

type ProducerSameTurnMessage struct {
	Role             string
	MessageType      string
	Content          string
	ReasoningContent string
	ToolCallID       string
	ToolName         string
	ToolArguments    map[string]any
}

type ProducerModelSelection struct {
	ProviderID          string
	ModelID             string
	DisplayName         string
	ReasoningEffort     string
	SupportsThinking    bool
	MaxCompletionTokens int
}

type ProducerImageAttachment struct {
	AssetID string
	NodeID  string
	Name    string
	URL     string
	Mime    string
}
