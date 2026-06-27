package craftsman

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	agentprompt "github.com/sinmaystar/clip-anvil/internal/agent/prompt"
)

type arkChatModel interface {
	Generate(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.Message, error)
	Stream(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.StreamReader[*schema.Message], error)
}

type arkChatModelFactory func(ctx context.Context, config *ark.ChatModelConfig) (arkChatModel, error)

type VolcengineModelResponderConfig struct {
	APIKey      string
	BaseURL     string
	Region      string
	Model       string
	MaxTokens   int
	Temperature float32
	Factory     arkChatModelFactory
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
	final, err := generator.Generate(ctx, craftsmanToolPromptMessages(craftsmanContext))
	if err != nil {
		return CraftsmanTurnOutput{}, fmt.Errorf("generate craftsman ark chat model: %w", err)
	}
	if final == nil {
		return CraftsmanTurnOutput{}, fmt.Errorf("generate craftsman ark chat model returned nil message")
	}
	return CraftsmanTurnOutput{
		AssistantText: strings.TrimSpace(final.Content),
		Metadata: map[string]any{
			"provider":               "volcengine",
			"model_id":               modelID,
			"native_tool_call_count": len(final.ToolCalls),
		},
		ModelMessage: final,
	}, nil
}

func craftsmanToolPromptMessages(craftsmanContext Context) []*schema.Message {
	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: SystemPrompt(),
		},
	}
	messages = append(messages, agentprompt.HistoryMessages(craftsmanContext.Messages)...)
	messages = append(messages,
		&schema.Message{
			Role:    schema.User,
			Content: craftsmanContext.Text,
		},
	)
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
	return agentprompt.AppendPendingReminders(messages, craftsmanContext.PendingReminders)
}

func durationPtr(value time.Duration) *time.Duration {
	return &value
}
