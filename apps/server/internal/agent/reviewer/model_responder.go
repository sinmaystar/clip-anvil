package reviewer

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

func (r VolcengineModelResponder) Review(ctx context.Context, reviewContext Context) (ReviewResult, map[string]any, error) {
	apiKey := strings.TrimSpace(r.cfg.APIKey)
	if apiKey == "" {
		return ReviewResult{}, nil, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for Reviewer model")
	}
	modelID := strings.TrimSpace(r.cfg.Model)
	if modelID == "" {
		return ReviewResult{}, nil, fmt.Errorf("reviewer model is required")
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
		return ReviewResult{}, nil, fmt.Errorf("create reviewer ark chat model: %w", err)
	}
	stream, err := model.Stream(ctx, reviewPromptMessages(reviewContext))
	if err != nil {
		return ReviewResult{}, nil, fmt.Errorf("stream reviewer ark chat model: %w", err)
	}
	defer stream.Close()

	chunks := []*schema.Message{}
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ReviewResult{}, nil, fmt.Errorf("receive reviewer ark chat stream: %w", err)
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
	final, err := schema.ConcatMessages(chunks)
	if err != nil {
		return ReviewResult{}, nil, fmt.Errorf("concatenate reviewer ark chat stream: %w", err)
	}
	result, err := ParseReviewResult(final.Content)
	if err != nil {
		return ReviewResult{}, nil, err
	}
	return result, map[string]any{"provider": "volcengine", "model_id": modelID}, nil
}

func (r VolcengineModelResponder) Respond(ctx context.Context, reviewContext Context) (ReviewerTurnOutput, error) {
	apiKey := strings.TrimSpace(r.cfg.APIKey)
	if apiKey == "" {
		return ReviewerTurnOutput{}, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for Reviewer model")
	}
	modelID := strings.TrimSpace(r.cfg.Model)
	if modelID == "" {
		return ReviewerTurnOutput{}, fmt.Errorf("reviewer model is required")
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
		return ReviewerTurnOutput{}, fmt.Errorf("create reviewer ark chat model: %w", err)
	}
	streamer := model
	if len(reviewContext.ToolInfos) > 0 {
		toolCallingModel, ok := model.(einoModel.ToolCallingChatModel)
		if !ok {
			return ReviewerTurnOutput{}, fmt.Errorf("selected Reviewer model does not support tool calling")
		}
		boundModel, err := toolCallingModel.WithTools(reviewContext.ToolInfos)
		if err != nil {
			return ReviewerTurnOutput{}, fmt.Errorf("bind reviewer tools: %w", err)
		}
		streamer = boundModel
	}
	stream, err := streamer.Stream(ctx, reviewToolPromptMessages(reviewContext))
	if err != nil {
		return ReviewerTurnOutput{}, fmt.Errorf("stream reviewer ark chat model: %w", err)
	}
	defer stream.Close()

	chunks := []*schema.Message{}
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ReviewerTurnOutput{}, fmt.Errorf("receive reviewer ark chat stream: %w", err)
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
	final, err := schema.ConcatMessages(chunks)
	if err != nil {
		return ReviewerTurnOutput{}, fmt.Errorf("concatenate reviewer ark chat stream: %w", err)
	}
	return ReviewerTurnOutput{
		AssistantText: strings.TrimSpace(final.Content),
		Metadata: map[string]any{
			"provider":               "volcengine",
			"model_id":               modelID,
			"native_tool_call_count": len(final.ToolCalls),
		},
		ModelMessage: final,
	}, nil
}

func ParseReviewResult(raw string) (ReviewResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ReviewResult{}, fmt.Errorf("%w: empty review result", ErrInvalidRubric)
	}
	var result ReviewResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return ReviewResult{}, fmt.Errorf("%w: %v", ErrInvalidRubric, err)
	}
	if _, err := ValidateRubric(result, DefaultReviewPolicy()); err != nil {
		return ReviewResult{}, err
	}
	return result, nil
}

func reviewPromptMessages(reviewContext Context) []*schema.Message {
	system := schema.SystemMessage(strings.TrimSpace(`You are ClipAnvil ReviewerGraph for generated short-video production artifacts.
Return strict JSON only. Do not include markdown.
Required JSON fields: overall_score, rubric, critique, retry_recommendation.
Rubric must include: proportion, physics, style, visual_quality, product_visibility, selling_power, platform_fit.
Each rubric axis must include score, pass, reason, fix_hint.`))
	user := reviewUserMessage(reviewContext)
	return []*schema.Message{system, user}
}

func reviewToolPromptMessages(reviewContext Context) []*schema.Message {
	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: SystemPrompt(),
		},
		reviewUserMessage(reviewContext),
	}
	for _, message := range reviewContext.SameTurnMessages {
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

func reviewUserMessage(reviewContext Context) *schema.Message {
	text := strings.TrimSpace(reviewContext.Text)
	if text == "" {
		text = "Review the attached generated artifact."
	}
	if strings.TrimSpace(reviewContext.AssetURL) == "" {
		return schema.UserMessage(text)
	}
	url := strings.TrimSpace(reviewContext.AssetURL)
	mime := strings.TrimSpace(reviewContext.AssetMime)
	if mime == "" && strings.HasPrefix(url, "data:image/") {
		mime = strings.TrimPrefix(strings.SplitN(strings.TrimPrefix(url, "data:"), ";", 2)[0], "data:")
	}
	if mime != "" && !strings.HasPrefix(mime, "image/") {
		return schema.UserMessage(strings.TrimSpace(text + "\n\nArtifact URL: " + url + "\nArtifact MIME: " + mime))
	}
	parts := []schema.MessageInputPart{
		{Type: schema.ChatMessagePartTypeText, Text: text},
		{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{MessagePartCommon: schema.MessagePartCommon{
				URL:      &url,
				MIMEType: mime,
			}},
		},
	}
	return &schema.Message{
		Role:                  schema.User,
		UserInputMultiContent: parts,
	}
}

func durationPtr(value time.Duration) *time.Duration {
	return &value
}
