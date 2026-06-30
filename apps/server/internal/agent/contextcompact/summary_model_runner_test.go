package contextcompact

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestVolcengineFullSummarizerBuildsPromptAndValidatesSummary(t *testing.T) {
	model := &fakeFullSummaryChatModel{message: schema.AssistantMessage(validFullSummaryForTest("agent_context_compaction/full"), nil)}
	summarizer := NewVolcengineFullSummarizer(VolcengineFullSummarizerConfig{
		APIKey: "test-key",
		Model:  "doubao-summary",
		Factory: func(context.Context, *ark.ChatModelConfig) (fullSummaryChatModel, error) {
			return model, nil
		},
	})

	out, err := summarizer.Summarize(context.Background(), FullSummaryInput{
		Role:                   "producer",
		Facts:                  []FullSummaryFact{{Ref: "shot/shot_01", Kind: "shot", Source: "db", Summary: "preview ready"}},
		RecentUserInstructions: []string{"保持箱体颜色一致"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelID != "doubao-summary" || !strings.Contains(out.Summary, "# Compacted Agent Handoff Summary") {
		t.Fatalf("output = %#v", out)
	}
	if len(model.messages) != 2 || model.messages[0].Role != schema.System || model.messages[1].Role != schema.User {
		t.Fatalf("messages = %#v", model.messages)
	}
	if !strings.Contains(model.messages[1].Content, "shot/shot_01") || !strings.Contains(model.messages[1].Content, "保持箱体颜色一致") {
		t.Fatalf("summary prompt missing facts or recent user instruction: %s", model.messages[1].Content)
	}
}

type fakeFullSummaryChatModel struct {
	messages []*schema.Message
	message  *schema.Message
}

func (f *fakeFullSummaryChatModel) Generate(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.Message, error) {
	f.messages = messages
	return f.message, nil
}
