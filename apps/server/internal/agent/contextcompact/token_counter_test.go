package contextcompact

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestTokenCounterCountsMessagesAndTools(t *testing.T) {
	counter := NewTokenCounter()
	result, err := counter.CountMessages(context.Background(), CountMessagesInput{
		ModelID: "gpt-4o",
		Messages: []*schema.Message{
			schema.SystemMessage("You are Producer."),
			schema.UserMessage("Create a 15s suitcase ad."),
		},
		ToolInfos: []*schema.ToolInfo{{Name: "read_project_context", Desc: "读取项目上下文。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalTokens <= 0 {
		t.Fatalf("TotalTokens = %d, want > 0", result.TotalTokens)
	}
	if result.MessageTokens <= 0 || result.ToolTokens <= 0 {
		t.Fatalf("message/tool tokens = %d/%d, want both > 0", result.MessageTokens, result.ToolTokens)
	}
}

func TestTokenCounterFallsBackForUnknownModel(t *testing.T) {
	counter := NewTokenCounter()
	result, err := counter.CountMessages(context.Background(), CountMessagesInput{
		ModelID:  "doubao-seed-2-0-mini-260428",
		Messages: []*schema.Message{schema.UserMessage(strings.Repeat("长", 200))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalTokens <= 0 {
		t.Fatal("fallback counter returned zero tokens")
	}
	if result.EstimatedBy != "heuristic" && result.EstimatedBy != "tiktoken" {
		t.Fatalf("EstimatedBy = %q, want heuristic or tiktoken", result.EstimatedBy)
	}
}
