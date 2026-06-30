package composer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	agentprompt "github.com/sinmaystar/clip-anvil/internal/agent/prompt"
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

func (r VolcengineModelResponder) Respond(ctx context.Context, composerContext Context) (ComposerTurnOutput, error) {
	apiKey := strings.TrimSpace(r.cfg.APIKey)
	if apiKey == "" {
		return ComposerTurnOutput{}, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for Composer model")
	}
	modelID := strings.TrimSpace(r.cfg.Model)
	if modelID == "" {
		return ComposerTurnOutput{}, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL is required for Composer model")
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
		return ComposerTurnOutput{}, fmt.Errorf("create composer ark chat model: %w", err)
	}
	generator := model
	if len(composerContext.ToolInfos) > 0 {
		toolCallingModel, ok := model.(einoModel.ToolCallingChatModel)
		if !ok {
			return ComposerTurnOutput{}, fmt.Errorf("selected Composer model does not support tool calling")
		}
		boundModel, err := toolCallingModel.WithTools(composerContext.ToolInfos)
		if err != nil {
			return ComposerTurnOutput{}, fmt.Errorf("bind composer tools: %w", err)
		}
		generator = boundModel
	}
	prompt := composerPromptMessagesWithBoundaries(composerContext)
	messages := prompt.Messages
	facts, mediaCards := composerContextCompactionFacts(composerContext)
	var compacted contextcompact.ProjectionOutput
	if r.cfg.ContextCompactor != nil {
		compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
			WorkspaceID:       composerContext.Input.WorkspaceID,
			ThreadID:          composerContext.Input.ThreadID,
			TaskID:            composerContext.Input.TaskID,
			Role:              "composer",
			ModelID:           modelID,
			Messages:          messages,
			ToolInfos:         composerContext.ToolInfos,
			MediaCards:        mediaCards,
			Facts:             facts,
			Trigger:           "composer_before_model",
			SameTurnFromIndex: prompt.SameTurnFromIndex,
			PendingFromIndex:  prompt.PendingFromIndex,
		})
		if err != nil {
			return ComposerTurnOutput{}, fmt.Errorf("compact composer context: %w", err)
		}
		messages = compacted.Messages
	}
	retriedContextOverflow := false
	final, err := generator.Generate(ctx, messages)
	if err != nil && contextcompact.IsContextOverflowError(err) && r.cfg.ContextCompactor != nil {
		retriedContextOverflow = true
		compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
			WorkspaceID:       composerContext.Input.WorkspaceID,
			ThreadID:          composerContext.Input.ThreadID,
			TaskID:            composerContext.Input.TaskID,
			Role:              "composer",
			ModelID:           modelID,
			Messages:          prompt.Messages,
			ToolInfos:         composerContext.ToolInfos,
			MediaCards:        mediaCards,
			Facts:             facts,
			Trigger:           "model_error_context_overflow",
			SameTurnFromIndex: prompt.SameTurnFromIndex,
			PendingFromIndex:  prompt.PendingFromIndex,
			ForceFullCompact:  true,
		})
		if err != nil {
			return ComposerTurnOutput{}, fmt.Errorf("compact composer context after overflow: %w", err)
		}
		messages = compacted.Messages
		final, err = generator.Generate(ctx, messages)
	}
	if err != nil {
		return ComposerTurnOutput{}, fmt.Errorf("generate composer ark chat model: %w", err)
	}
	if final == nil {
		return ComposerTurnOutput{}, fmt.Errorf("generate composer ark chat model returned nil message")
	}
	metadata := map[string]any{
		"provider":               "volcengine",
		"model_id":               modelID,
		"native_tool_call_count": len(final.ToolCalls),
	}
	enrichComposerContextCompactionMetadata(metadata, compacted)
	if retriedContextOverflow {
		metadata["context_compaction_retry"] = true
	}
	return ComposerTurnOutput{
		AssistantText: strings.TrimSpace(final.Content),
		Metadata:      metadata,
		ModelMessage:  final,
	}, nil
}

type DeterministicResponder struct{}

func NewDeterministicResponder() DeterministicResponder {
	return DeterministicResponder{}
}

func (DeterministicResponder) Respond(_ context.Context, composerContext Context) (ComposerTurnOutput, error) {
	text := "Composer Agent 已接入 native tool loop；当前非 real 模式不会沿用旧线性合成逻辑自动伪造成片。"
	return ComposerTurnOutput{
		AssistantText: text,
		Result: CompositionOutput{
			Status:        "blocked",
			OperationType: "compose_final_video",
		},
		Metadata: map[string]any{"provider": "deterministic"},
		ModelMessage: &schema.Message{
			Role:    schema.Assistant,
			Content: strings.TrimSpace(text + "\n\nContext: " + composerContext.Summary),
		},
	}, nil
}

type composerPromptBoundary struct {
	Messages          []*schema.Message
	SameTurnFromIndex int
	PendingFromIndex  int
}

func composerPromptMessagesWithBoundaries(composerContext Context) composerPromptBoundary {
	messages := []*schema.Message{
		{Role: schema.System, Content: SystemPrompt},
		{Role: schema.User, Content: composerUserMessage(composerContext)},
	}
	sameTurnFromIndex := contextcompact.CurrentToolLoopFromIndex(len(messages), len(composerContext.SameTurnMessages))
	for _, message := range composerContext.SameTurnMessages {
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
	pendingFromIndex := contextcompact.PendingReminderTargetIndex(messages, composerContext.PendingReminders)
	messages = agentprompt.AppendPendingReminders(messages, composerContext.PendingReminders)
	return composerPromptBoundary{Messages: messages, SameTurnFromIndex: sameTurnFromIndex, PendingFromIndex: pendingFromIndex}
}

func enrichComposerContextCompactionMetadata(metadata map[string]any, output contextcompact.ProjectionOutput) {
	if len(output.Applied) == 0 {
		return
	}
	metadata["context_compaction_applied"] = true
	metadata["context_compaction_mode"] = output.CompactionMode
	metadata["context_compaction_count"] = len(output.Applied)
	metadata["context_compaction_refs"] = output.CompactionRefs
	metadata["context_compaction_detail_files"] = output.DetailFiles
}

func composerUserMessage(composerContext Context) string {
	lines := []string{
		"Run a Composer final-output turn.",
		"Workspace summary: " + strings.TrimSpace(composerContext.Summary),
		"Instructions: " + strings.TrimSpace(composerContext.Input.Input.Instructions),
		"Template: " + strings.TrimSpace(composerContext.Input.Input.TemplateKey),
	}
	if composerContext.SourceStoryboardNodeID.Valid {
		lines = append(lines, "Source storyboard node id: "+uuidString(composerContext.SourceStoryboardNodeID))
	}
	if strings.TrimSpace(composerContext.SourceNodeTitle) != "" {
		lines = append(lines, "Source node title: "+strings.TrimSpace(composerContext.SourceNodeTitle))
	}
	return strings.Join(lines, "\n")
}

func durationPtr(value time.Duration) *time.Duration {
	return &value
}
