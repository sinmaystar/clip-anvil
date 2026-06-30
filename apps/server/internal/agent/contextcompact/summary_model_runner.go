package contextcompact

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fullSummaryChatModel interface {
	Generate(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.Message, error)
}

type VolcengineFullSummarizerConfig struct {
	APIKey      string
	BaseURL     string
	Region      string
	Model       string
	MaxTokens   int
	Temperature float32
	Factory     func(context.Context, *ark.ChatModelConfig) (fullSummaryChatModel, error)
}

type VolcengineFullSummarizer struct {
	cfg     VolcengineFullSummarizerConfig
	factory func(context.Context, *ark.ChatModelConfig) (fullSummaryChatModel, error)
}

func NewVolcengineFullSummarizer(cfg VolcengineFullSummarizerConfig) VolcengineFullSummarizer {
	factory := cfg.Factory
	if factory == nil {
		factory = func(ctx context.Context, config *ark.ChatModelConfig) (fullSummaryChatModel, error) {
			return ark.NewChatModel(ctx, config)
		}
	}
	return VolcengineFullSummarizer{cfg: cfg, factory: factory}
}

func (s VolcengineFullSummarizer) Summarize(ctx context.Context, input FullSummaryInput) (FullSummaryOutput, error) {
	apiKey := strings.TrimSpace(s.cfg.APIKey)
	if apiKey == "" {
		return FullSummaryOutput{}, fmt.Errorf("volcengine full summarizer api key is required")
	}
	modelID := strings.TrimSpace(s.cfg.Model)
	if modelID == "" {
		return FullSummaryOutput{}, fmt.Errorf("volcengine full summarizer model is required")
	}
	config := &ark.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: strings.TrimSpace(s.cfg.BaseURL),
		Region:  strings.TrimSpace(s.cfg.Region),
		Model:   modelID,
		Timeout: durationPointer(5 * time.Minute),
	}
	if s.cfg.MaxTokens > 0 {
		config.MaxTokens = &s.cfg.MaxTokens
	}
	if s.cfg.Temperature > 0 {
		config.Temperature = &s.cfg.Temperature
	}
	model, err := s.factory(ctx, config)
	if err != nil {
		return FullSummaryOutput{}, fmt.Errorf("create full summary model: %w", err)
	}
	final, err := model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(fullSummarySystemPrompt()),
		schema.UserMessage(BuildFullSummaryPrompt(input)),
	})
	if err != nil {
		return FullSummaryOutput{}, fmt.Errorf("generate full summary: %w", err)
	}
	if final == nil {
		return FullSummaryOutput{}, fmt.Errorf("generate full summary returned nil message")
	}
	summary := strings.TrimSpace(final.Content)
	if err := ValidateFullSummaryMarkdown(summary); err != nil {
		return FullSummaryOutput{}, err
	}
	return FullSummaryOutput{Summary: summary, ModelID: modelID}, nil
}

func fullSummarySystemPrompt() string {
	return strings.TrimSpace(`You write ClipAnvil Agent context compaction handoff summaries.
Return only structured Markdown.
Do not invent visual or audio media facts.
Use semantic refs and DB facts as authoritative.
If a fact is uncertain, write "未确认".
Include every required heading exactly as requested.`)
}

func durationPointer(value time.Duration) *time.Duration {
	return &value
}
