package craftsman

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestCraftsmanSystemPromptIncludesReviewerRepairRules(t *testing.T) {
	prompt := SystemPrompt()
	for _, required := range []string{
		"Reviewer",
		"artifact_issue",
		"retry_recommendation",
		"mode=fork_from",
		"不要直接问用户",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("craftsman prompt missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"M1 阶段",
		"M2 阶段",
		"TODO",
		"TBD",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("craftsman prompt contains stale placeholder wording %q", forbidden)
		}
	}
}

func TestVolcengineCraftsmanResponderReturnsNativeToolCallingMessage(t *testing.T) {
	streamer := &fakeCraftsmanArkStreamer{chunks: []*schema.Message{{
		Content: "已提交 RenderPlan。",
	}}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	out, err := responder.Respond(context.Background(), Context{Text: "shot-01 开场"})
	if err != nil {
		t.Fatal(err)
	}
	if out.AssistantText != "已提交 RenderPlan。" || out.Metadata["model_id"] != "doubao-test" || out.ModelMessage == nil {
		t.Fatalf("output = %#v", out)
	}
	if len(streamer.messages) != 2 || !strings.Contains(streamer.messages[1].Content, "shot-01") {
		t.Fatalf("messages = %#v", streamer.messages)
	}
}

type fakeCraftsmanArkStreamer struct {
	messages []*schema.Message
	chunks   []*schema.Message
}

func (f *fakeCraftsmanArkStreamer) Stream(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
	f.messages = messages
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		for _, chunk := range f.chunks {
			sw.Send(chunk, nil)
		}
		sw.Send(nil, io.EOF)
	}()
	return sr, nil
}
