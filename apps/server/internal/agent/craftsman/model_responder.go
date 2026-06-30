package craftsman

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	agentprompt "github.com/sinmaystar/clip-anvil/internal/agent/prompt"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type arkChatModel interface {
	Generate(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.Message, error)
	Stream(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.StreamReader[*schema.Message], error)
}

type arkChatModelFactory func(ctx context.Context, config *ark.ChatModelConfig) (arkChatModel, error)

type VolcengineModelResponderConfig struct {
	APIKey           string
	BaseURL          string
	Region           string
	Model            string
	MaxTokens        int
	Temperature      float32
	Factory          arkChatModelFactory
	ContextCompactor contextcompact.Middleware
}

type VolcengineModelResponder struct {
	cfg     VolcengineModelResponderConfig
	factory arkChatModelFactory
}

func NewVolcengineModelResponder(cfg VolcengineModelResponderConfig) VolcengineModelResponder {
	factory := cfg.Factory
	if factory == nil {
		factory = func(ctx context.Context, config *ark.ChatModelConfig) (arkChatModel, error) {
			return ark.NewChatModel(ctx, config)
		}
	}
	return VolcengineModelResponder{cfg: cfg, factory: factory}
}

func (r VolcengineModelResponder) Respond(ctx context.Context, craftsmanContext Context) (CraftsmanTurnOutput, error) {
	apiKey := strings.TrimSpace(r.cfg.APIKey)
	if apiKey == "" {
		return CraftsmanTurnOutput{}, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for Craftsman model")
	}
	modelID := strings.TrimSpace(r.cfg.Model)
	if modelID == "" {
		return CraftsmanTurnOutput{}, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL is required for Craftsman model")
	}
	config := &ark.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: strings.TrimSpace(r.cfg.BaseURL),
		Region:  strings.TrimSpace(r.cfg.Region),
		Model:   modelID,
		Timeout: durationPtr(10 * time.Minute),
	}
	if r.cfg.MaxTokens > 0 {
		config.MaxTokens = &r.cfg.MaxTokens
	}
	if r.cfg.Temperature > 0 {
		config.Temperature = &r.cfg.Temperature
	}
	model, err := r.factory(ctx, config)
	if err != nil {
		return CraftsmanTurnOutput{}, fmt.Errorf("create craftsman ark chat model: %w", err)
	}
	generator := model
	if len(craftsmanContext.ToolInfos) > 0 {
		toolCallingModel, ok := model.(einoModel.ToolCallingChatModel)
		if !ok {
			return CraftsmanTurnOutput{}, fmt.Errorf("selected Craftsman model does not support tool calling")
		}
		boundModel, err := toolCallingModel.WithTools(craftsmanContext.ToolInfos)
		if err != nil {
			return CraftsmanTurnOutput{}, fmt.Errorf("bind craftsman tools: %w", err)
		}
		generator = boundModel
	}
	prompt := craftsmanToolPromptMessagesWithBoundaries(craftsmanContext)
	messages := prompt.Messages
	facts, mediaCards := craftsmanContextCompactionFacts(craftsmanContext)
	var compacted contextcompact.ProjectionOutput
	if r.cfg.ContextCompactor != nil {
		compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
			WorkspaceID:       craftsmanContext.Input.WorkspaceID,
			ThreadID:          craftsmanContext.Input.ThreadID,
			TaskID:            craftsmanContext.Input.TaskID,
			Role:              "craftsman",
			ModelID:           modelID,
			Messages:          messages,
			MessageRefs:       prompt.MessageRefs,
			ToolInfos:         craftsmanContext.ToolInfos,
			MediaCards:        mediaCards,
			Facts:             facts,
			Trigger:           "craftsman_before_model",
			SameTurnFromIndex: prompt.SameTurnFromIndex,
			PendingFromIndex:  prompt.PendingFromIndex,
		})
		if err != nil {
			return CraftsmanTurnOutput{}, fmt.Errorf("compact craftsman context: %w", err)
		}
		messages = compacted.Messages
	}
	retriedContextOverflow := false
	final, err := generator.Generate(ctx, messages)
	if err != nil && contextcompact.IsContextOverflowError(err) && r.cfg.ContextCompactor != nil {
		retriedContextOverflow = true
		compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
			WorkspaceID:       craftsmanContext.Input.WorkspaceID,
			ThreadID:          craftsmanContext.Input.ThreadID,
			TaskID:            craftsmanContext.Input.TaskID,
			Role:              "craftsman",
			ModelID:           modelID,
			Messages:          prompt.Messages,
			MessageRefs:       prompt.MessageRefs,
			ToolInfos:         craftsmanContext.ToolInfos,
			MediaCards:        mediaCards,
			Facts:             facts,
			Trigger:           "model_error_context_overflow",
			SameTurnFromIndex: prompt.SameTurnFromIndex,
			PendingFromIndex:  prompt.PendingFromIndex,
			ForceFullCompact:  true,
		})
		if err != nil {
			return CraftsmanTurnOutput{}, fmt.Errorf("compact craftsman context after overflow: %w", err)
		}
		messages = compacted.Messages
		final, err = generator.Generate(ctx, messages)
	}
	if err != nil {
		return CraftsmanTurnOutput{}, fmt.Errorf("generate craftsman ark chat model: %w", err)
	}
	if final == nil {
		return CraftsmanTurnOutput{}, fmt.Errorf("generate craftsman ark chat model returned nil message")
	}
	metadata := map[string]any{
		"provider":               "volcengine",
		"model_id":               modelID,
		"native_tool_call_count": len(final.ToolCalls),
	}
	enrichCraftsmanContextCompactionMetadata(metadata, compacted)
	if retriedContextOverflow {
		metadata["context_compaction_retry"] = true
	}
	return CraftsmanTurnOutput{
		AssistantText: strings.TrimSpace(final.Content),
		Metadata:      metadata,
		ModelMessage:  final,
	}, nil
}

type craftsmanPromptBoundary struct {
	Messages          []*schema.Message
	MessageRefs       []contextcompact.SourceMessageRef
	SameTurnFromIndex int
	PendingFromIndex  int
}

func craftsmanToolPromptMessages(craftsmanContext Context) []*schema.Message {
	return craftsmanToolPromptMessagesWithBoundaries(craftsmanContext).Messages
}

func craftsmanToolPromptMessagesWithBoundaries(craftsmanContext Context) craftsmanPromptBoundary {
	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: SystemPrompt(),
		},
	}
	messageRefs := []contextcompact.SourceMessageRef{}
	for _, source := range craftsmanContext.Messages {
		for _, message := range agentprompt.HistoryMessages([]db.AgentMessage{source}) {
			messages = append(messages, message)
			messageRefs = appendCraftsmanPromptMessageRef(messageRefs, len(messages)-1, source.ID)
		}
	}
	messages = append(messages,
		&schema.Message{
			Role:    schema.User,
			Content: craftsmanContext.Text,
		},
	)
	sameTurnFromIndex := contextcompact.CurrentToolLoopFromIndex(len(messages), len(craftsmanContext.SameTurnMessages))
	for _, message := range craftsmanContext.SameTurnMessages {
		switch message.Role {
		case "assistant":
			messages = append(messages, &schema.Message{
				Role:    schema.Assistant,
				Content: message.Content,
				ToolCalls: []schema.ToolCall{{
					ID:   message.ToolCallID,
					Type: "function",
					Function: schema.FunctionCall{
						Name:      message.ToolName,
						Arguments: string(mustJSON(message.ToolArguments)),
					},
				}},
			})
		case "tool":
			messages = append(messages, &schema.Message{
				Role:       schema.Tool,
				Content:    message.Content,
				ToolCallID: message.ToolCallID,
				ToolName:   message.ToolName,
			})
		}
	}
	pendingFromIndex := contextcompact.PendingReminderTargetIndex(messages, craftsmanContext.PendingReminders)
	messages = agentprompt.AppendPendingReminders(messages, craftsmanContext.PendingReminders)
	return craftsmanPromptBoundary{
		Messages:          messages,
		MessageRefs:       messageRefs,
		SameTurnFromIndex: sameTurnFromIndex,
		PendingFromIndex:  pendingFromIndex,
	}
}

func appendCraftsmanPromptMessageRef(refs []contextcompact.SourceMessageRef, index int, id pgtype.UUID) []contextcompact.SourceMessageRef {
	if !id.Valid {
		return refs
	}
	return append(refs, contextcompact.SourceMessageRef{MessageIndex: index, MessageID: id})
}

func enrichCraftsmanContextCompactionMetadata(metadata map[string]any, output contextcompact.ProjectionOutput) {
	if len(output.Applied) == 0 {
		return
	}
	metadata["context_compaction_applied"] = true
	metadata["context_compaction_mode"] = output.CompactionMode
	metadata["context_compaction_count"] = len(output.Applied)
	metadata["context_compaction_refs"] = output.CompactionRefs
	metadata["context_compaction_detail_files"] = output.DetailFiles
}

func durationPtr(value time.Duration) *time.Duration {
	return &value
}
