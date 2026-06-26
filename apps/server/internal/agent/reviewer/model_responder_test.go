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

func TestVolcengineReviewerResponderReturnsNativeToolCallingMessage(t *testing.T) {
	streamer := &fakeReviewArkStreamer{chunks: []*schema.Message{{
		Content: "已提交 Reviewer 评审结果。",
	}}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-reviewer",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	out, err := responder.Respond(context.Background(), Context{
		Text:     "Review Target\n- shot: shot-01",
		AssetURL: "data:image/png;base64,iVBORw0KGgo=",
		ToolInfos: []*schema.ToolInfo{{
			Name: "submit_review_result",
			Desc: "提交 Reviewer 评审结果。",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.AssistantText != "已提交 Reviewer 评审结果。" || out.Metadata["model_id"] != "doubao-reviewer" || out.ModelMessage == nil {
		t.Fatalf("output = %#v", out)
	}
	if len(streamer.boundTools) != 1 || streamer.boundTools[0].Name != "submit_review_result" {
		t.Fatalf("bound tools = %#v", streamer.boundTools)
	}
	if len(streamer.messages) != 2 || !strings.Contains(streamer.messages[1].UserInputMultiContent[0].Text, "shot-01") {
		t.Fatalf("messages = %#v", streamer.messages)
	}
	if len(streamer.messages[1].UserInputMultiContent) != 2 {
		t.Fatalf("multi content = %#v", streamer.messages[1].UserInputMultiContent)
	}
}

type fakeReviewArkStreamer struct {
	messages   []*schema.Message
	chunks     []*schema.Message
	boundTools []*schema.ToolInfo
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

func (f *fakeReviewArkStreamer) WithTools(tools []*schema.ToolInfo) (einoModel.ToolCallingChatModel, error) {
	f.boundTools = tools
	return f, nil
}

func (f *fakeReviewArkStreamer) Generate(context.Context, []*schema.Message, ...einoModel.Option) (*schema.Message, error) {
	if len(f.chunks) == 0 {
		return &schema.Message{}, nil
	}
	return schema.ConcatMessages(f.chunks)
}
