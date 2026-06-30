package contextcompact

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/schema"
	tiktoken "github.com/pkoukk/tiktoken-go"
)

type TokenCounter interface {
	CountMessages(ctx context.Context, input CountMessagesInput) (CountMessagesResult, error)
}

type CountMessagesInput struct {
	ModelID    string
	Messages   []*schema.Message
	ToolInfos  []*schema.ToolInfo
	MediaCards []MediaCard
}

type CountMessagesResult struct {
	TotalTokens   int
	MessageTokens int
	ToolTokens    int
	MediaTokens   int
	EstimatedBy   string
}

type TiktokenCounter struct{}

func NewTokenCounter() TokenCounter {
	return TiktokenCounter{}
}

func (TiktokenCounter) CountMessages(_ context.Context, input CountMessagesInput) (CountMessagesResult, error) {
	estimate := estimatorForModel(input.ModelID)
	result := CountMessagesResult{EstimatedBy: estimate.name}
	for _, msg := range input.Messages {
		result.MessageTokens += estimate.count(messageText(msg))
	}
	for _, info := range input.ToolInfos {
		result.ToolTokens += estimate.count(toolInfoText(info))
	}
	for _, card := range input.MediaCards {
		if card.TokenWeight > 0 {
			result.MediaTokens += card.TokenWeight
			continue
		}
		result.MediaTokens += estimate.count(mediaCardText(card))
	}
	result.TotalTokens = result.MessageTokens + result.ToolTokens + result.MediaTokens
	return result, nil
}

type tokenEstimator struct {
	name  string
	count func(string) int
}

func estimatorForModel(modelID string) tokenEstimator {
	if enc, err := tiktoken.EncodingForModel(strings.TrimSpace(modelID)); err == nil {
		return tokenEstimator{
			name: "tiktoken",
			count: func(text string) int {
				if strings.TrimSpace(text) == "" {
					return 0
				}
				return len(enc.Encode(text, nil, nil))
			},
		}
	}
	return tokenEstimator{name: "heuristic", count: heuristicTokens}
}

func heuristicTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	count := len([]rune(text)) / 3
	if count < 1 {
		return 1
	}
	return count
}

func messageText(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(string(msg.Role))
	b.WriteByte('\n')
	b.WriteString(msg.Content)
	if len(msg.MultiContent) > 0 {
		data, _ := json.Marshal(msg.MultiContent)
		b.Write(data)
	}
	if len(msg.ToolCalls) > 0 {
		data, _ := json.Marshal(msg.ToolCalls)
		b.Write(data)
	}
	if msg.ToolCallID != "" {
		b.WriteString(msg.ToolCallID)
	}
	return b.String()
}

func toolInfoText(info *schema.ToolInfo) string {
	if info == nil {
		return ""
	}
	data, err := json.Marshal(info)
	if err != nil {
		return info.Name + "\n" + info.Desc
	}
	return string(data)
}

func mediaCardText(card MediaCard) string {
	return MediaCardPromptText(card)
}
