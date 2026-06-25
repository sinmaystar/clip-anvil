package craftsman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func ParseStrategy(raw string) (Strategy, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var decoded struct {
		Strategy       string         `json:"strategy"`
		PreviewPrompt  string         `json:"preview_prompt"`
		VideoPrompt    string         `json:"video_prompt"`
		NegativePrompt string         `json:"negative_prompt"`
		StyleNotes     any            `json:"style_notes"`
		InputNodeRefs  any            `json:"input_node_refs"`
		OutputType     string         `json:"output_type"`
		OperationType  string         `json:"operation_type"`
		Model          any            `json:"model"`
		Params         map[string]any `json:"params"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return Strategy{}, fmt.Errorf("%w: parse strategy json: %v", ErrInvalidStrategy, err)
	}
	strategy := Strategy{
		Strategy:       decoded.Strategy,
		PreviewPrompt:  decoded.PreviewPrompt,
		VideoPrompt:    decoded.VideoPrompt,
		NegativePrompt: decoded.NegativePrompt,
		StyleNotes:     flexibleStringSlice(decoded.StyleNotes),
		InputNodeRefs:  flexibleStringSlice(decoded.InputNodeRefs),
		OutputType:     decoded.OutputType,
		OperationType:  decoded.OperationType,
		Model:          flexibleModelSpec(decoded.Model),
		Params:         decoded.Params,
	}
	if err := ValidateStrategy(strategy); err != nil {
		return Strategy{}, err
	}
	if strategy.Params == nil {
		strategy.Params = map[string]any{}
	}
	return strategy, nil
}

func ValidateStrategy(strategy Strategy) error {
	if strings.TrimSpace(strategy.Strategy) == "" || (strings.TrimSpace(strategy.PreviewPrompt) == "" && strings.TrimSpace(strategy.VideoPrompt) == "") {
		return fmt.Errorf("%w: strategy and generation prompt are required", ErrInvalidStrategy)
	}
	for _, ref := range strategy.InputNodeRefs {
		if strings.TrimSpace(ref) == "" {
			return fmt.Errorf("%w: input_node_refs cannot contain empty values", ErrInvalidStrategy)
		}
	}
	return nil
}

func ValidateStrategyForMode(strategy Strategy, mode string) error {
	if err := ValidateStrategy(strategy); err != nil {
		return err
	}
	if mode == "shot_video" && strings.TrimSpace(strategyPrompt(strategy, mode)) == "" {
		return fmt.Errorf("%w: video prompt is required", ErrInvalidStrategy)
	}
	return nil
}

func flexibleStringSlice(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func flexibleModelSpec(value any) ModelSpec {
	switch typed := value.(type) {
	case nil:
		return ModelSpec{}
	case string:
		return ModelSpec{ModelID: flexibleModelID(typed)}
	case map[string]any:
		return ModelSpec{
			Provider: stringFromAny(typed["provider"]),
			ModelID:  flexibleModelID(firstNonEmptyString(stringFromAny(typed["model_id"]), stringFromAny(typed["model"]))),
		}
	default:
		return ModelSpec{}
	}
}

func flexibleModelID(value string) string {
	text := strings.TrimSpace(value)
	switch strings.ToLower(text) {
	case "", "default", "auto", "production_default":
		return ""
	}
	if strings.HasPrefix(strings.ToLower(text), "doubao-") {
		return text
	}
	return ""
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type StaticResponder struct {
	Strategy Strategy
	Metadata map[string]any
	Err      error
	Turns    []CraftsmanTurnOutput
}

func (r StaticResponder) DraftPreviewStrategy(context.Context, Context) (Strategy, map[string]any, error) {
	if r.Err != nil {
		return Strategy{}, nil, r.Err
	}
	return r.Strategy, r.Metadata, nil
}

func (r StaticResponder) Respond(_ context.Context, craftsmanContext Context) (CraftsmanTurnOutput, error) {
	if r.Err != nil {
		return CraftsmanTurnOutput{}, r.Err
	}
	index := 0
	for _, message := range craftsmanContext.SameTurnMessages {
		if message.Role == "tool" {
			index++
		}
	}
	if index >= 0 && index < len(r.Turns) {
		return r.Turns[index], nil
	}
	return CraftsmanTurnOutput{
		AssistantText: "Craftsman 已完成 RenderPlan。",
		Metadata:      r.Metadata,
	}, nil
}

type arkChatStreamer interface {
	Stream(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.StreamReader[*schema.Message], error)
}

type arkChatModelFactory func(ctx context.Context, config *ark.ChatModelConfig) (arkChatStreamer, error)

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
		factory = func(ctx context.Context, config *ark.ChatModelConfig) (arkChatStreamer, error) {
			return ark.NewChatModel(ctx, config)
		}
	}
	return VolcengineModelResponder{cfg: cfg, factory: factory}
}

func (r VolcengineModelResponder) DraftPreviewStrategy(ctx context.Context, craftsmanContext Context) (Strategy, map[string]any, error) {
	apiKey := strings.TrimSpace(r.cfg.APIKey)
	if apiKey == "" {
		return Strategy{}, nil, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for Craftsman model")
	}
	modelID := strings.TrimSpace(r.cfg.Model)
	if modelID == "" {
		return Strategy{}, nil, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL is required for Craftsman model")
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
		return Strategy{}, nil, fmt.Errorf("create craftsman ark chat model: %w", err)
	}
	stream, err := model.Stream(ctx, craftsmanPromptMessages(craftsmanContext))
	if err != nil {
		return Strategy{}, nil, fmt.Errorf("stream craftsman ark chat model: %w", err)
	}
	defer stream.Close()

	chunks := []*schema.Message{}
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Strategy{}, nil, fmt.Errorf("receive craftsman ark chat stream: %w", err)
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
	final, err := schema.ConcatMessages(chunks)
	if err != nil {
		return Strategy{}, nil, fmt.Errorf("concatenate craftsman ark chat stream: %w", err)
	}
	strategy, err := ParseStrategy(final.Content)
	if err != nil {
		return Strategy{}, nil, err
	}
	return strategy, map[string]any{
		"provider": "volcengine",
		"model_id": modelID,
	}, nil
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
	streamer := model
	if len(craftsmanContext.ToolInfos) > 0 {
		toolCallingModel, ok := model.(einoModel.ToolCallingChatModel)
		if !ok {
			return CraftsmanTurnOutput{}, fmt.Errorf("selected Craftsman model does not support tool calling")
		}
		boundModel, err := toolCallingModel.WithTools(craftsmanContext.ToolInfos)
		if err != nil {
			return CraftsmanTurnOutput{}, fmt.Errorf("bind craftsman tools: %w", err)
		}
		streamer = boundModel
	}
	stream, err := streamer.Stream(ctx, craftsmanToolPromptMessages(craftsmanContext))
	if err != nil {
		return CraftsmanTurnOutput{}, fmt.Errorf("stream craftsman ark chat model: %w", err)
	}
	defer stream.Close()

	chunks := []*schema.Message{}
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return CraftsmanTurnOutput{}, fmt.Errorf("receive craftsman ark chat stream: %w", err)
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
	final, err := schema.ConcatMessages(chunks)
	if err != nil {
		return CraftsmanTurnOutput{}, fmt.Errorf("concatenate craftsman ark chat stream: %w", err)
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

func craftsmanPromptMessages(craftsmanContext Context) []*schema.Message {
	return []*schema.Message{
		{
			Role: schema.System,
			Content: strings.TrimSpace(`You are ClipAnvil CraftsmanGraph for one video shot.
Create one preview image generation strategy for the target shot.
Return JSON only. Do not wrap the response in Markdown.
Required JSON fields: strategy, preview_prompt.
Optional JSON fields: negative_prompt, style_notes, input_node_refs, model, params.`),
		},
		{
			Role:    schema.User,
			Content: craftsmanContext.Text,
		},
	}
}

func craftsmanToolPromptMessages(craftsmanContext Context) []*schema.Message {
	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: SystemPrompt(),
		},
		{
			Role:    schema.User,
			Content: craftsmanContext.Text,
		},
	}
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
	return messages
}

func durationPtr(value time.Duration) *time.Duration {
	return &value
}
