package reviewer

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestParseReviewResult(t *testing.T) {
	result, err := ParseReviewResult(`{
		"review_task": "preview_image_review",
		"verdict": "accepted",
		"overall_score": 0.82,
		"rubric": {
			"faithfulness": {"score": 0.8, "pass": true, "reason": "ok", "fix_hint": ""},
			"subject_consistency": {"score": 0.8, "pass": true, "reason": "ok", "fix_hint": ""},
			"product_visibility": {"score": 0.8, "pass": true, "reason": "ok", "fix_hint": ""},
			"brand_style_consistency": {"score": 0.8, "pass": true, "reason": "ok", "fix_hint": ""},
			"composition_proportion": {"score": 0.8, "pass": true, "reason": "ok", "fix_hint": ""},
			"visual_quality": {"score": 0.8, "pass": true, "reason": "ok", "fix_hint": ""}
		},
		"critique": "画面可用",
		"retry_recommendation": {"should_retry": false}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.OverallScore != 0.82 || result.Critique != "画面可用" {
		t.Fatalf("result = %#v", result)
	}
}

func TestVolcengineReviewResponderParsesStreamedResult(t *testing.T) {
	streamer := &fakeReviewArkStreamer{chunks: []*schema.Message{{
		Content: `{"review_task":"preview_image_review","verdict":"accepted","overall_score":0.82,"rubric":{"faithfulness":{"score":0.8,"pass":true,"reason":"ok"},"subject_consistency":{"score":0.8,"pass":true,"reason":"ok"},"product_visibility":{"score":0.8,"pass":true,"reason":"ok"},"brand_style_consistency":{"score":0.8,"pass":true,"reason":"ok"},"composition_proportion":{"score":0.8,"pass":true,"reason":"ok"},"visual_quality":{"score":0.8,"pass":true,"reason":"ok"}},"critique":"画面可用","retry_recommendation":{"should_retry":false}}`,
	}}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-reviewer",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	result, metadata, err := responder.Review(context.Background(), Context{
		Text:     "Review Target\n- shot: shot-01",
		AssetURL: "data:image/png;base64,iVBORw0KGgo=",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Critique != "画面可用" || metadata["model_id"] != "doubao-reviewer" {
		t.Fatalf("result=%#v metadata=%#v", result, metadata)
	}
	if len(streamer.messages) != 2 || !strings.Contains(streamer.messages[1].UserInputMultiContent[0].Text, "shot-01") {
		t.Fatalf("messages = %#v", streamer.messages)
	}
	if len(streamer.messages[1].UserInputMultiContent) != 2 {
		t.Fatalf("multi content = %#v", streamer.messages[1].UserInputMultiContent)
	}
}

type fakeReviewArkStreamer struct {
	messages []*schema.Message
	chunks   []*schema.Message
}

func (f *fakeReviewArkStreamer) Stream(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
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
