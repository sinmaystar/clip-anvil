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

func TestParseCraftsmanStrategy(t *testing.T) {
	strategy, err := ParseStrategy(`{
		"strategy": "用明亮货架背景突出商品卖点。",
		"preview_prompt": "A clean commercial product shot, bright lighting",
		"negative_prompt": "blur, watermark",
		"style_notes": ["commercial", "clean"],
		"input_node_refs": ["node-1"],
		"model": {"provider": "", "model_id": ""},
		"params": {"size": "1024x1024"}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strategy.PreviewPrompt == "" {
		t.Fatal("preview prompt is empty")
	}
	if strategy.Params["size"] != "1024x1024" {
		t.Fatalf("params = %#v", strategy.Params)
	}
}

func TestParseCraftsmanStrategyRejectsEmptyPrompt(t *testing.T) {
	_, err := ParseStrategy(`{"strategy":"方向","preview_prompt":""}`)
	if err == nil {
		t.Fatal("expected invalid strategy error")
	}
}

func TestParseCraftsmanStrategyAcceptsStringStyleNotes(t *testing.T) {
	strategy, err := ParseStrategy(`{
		"strategy": "方向",
		"preview_prompt": "prompt",
		"style_notes": "commercial, clean"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(strategy.StyleNotes) != 1 || strategy.StyleNotes[0] != "commercial, clean" {
		t.Fatalf("style notes = %#v", strategy.StyleNotes)
	}
}

func TestParseCraftsmanStrategyAcceptsStringModel(t *testing.T) {
	strategy, err := ParseStrategy(`{
		"strategy": "方向",
		"preview_prompt": "prompt",
		"model": "doubao-seedream-5-0-260128"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strategy.Model.Provider != "" || strategy.Model.ModelID != "doubao-seedream-5-0-260128" {
		t.Fatalf("model = %#v", strategy.Model)
	}
}

func TestParseCraftsmanStrategyIgnoresLooseModelAlias(t *testing.T) {
	strategy, err := ParseStrategy(`{
		"strategy": "方向",
		"preview_prompt": "prompt",
		"model": {"provider": "volcengine", "model": "stable-diffusion-xl"}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strategy.Model.Provider != "volcengine" || strategy.Model.ModelID != "" {
		t.Fatalf("model = %#v", strategy.Model)
	}
}

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

func TestVolcengineCraftsmanResponderParsesStreamedStrategy(t *testing.T) {
	streamer := &fakeCraftsmanArkStreamer{chunks: []*schema.Message{{
		Content: `{"strategy":"明亮商品特写","preview_prompt":"A bright product close-up"}`,
	}}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	strategy, metadata, err := responder.DraftPreviewStrategy(context.Background(), Context{Text: "shot-01 开场"})
	if err != nil {
		t.Fatal(err)
	}
	if strategy.PreviewPrompt != "A bright product close-up" {
		t.Fatalf("strategy = %#v", strategy)
	}
	if metadata["model_id"] != "doubao-test" {
		t.Fatalf("metadata = %#v", metadata)
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
