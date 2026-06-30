package reviewer

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
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

func TestVolcengineReviewerResponderRetriesOnceWithFullCompactOnContextOverflow(t *testing.T) {
	streamer := &fakeReviewArkStreamer{
		chunks:     []*schema.Message{{Content: "ok"}},
		streamErrs: []error{errors.New("tokens exceed context window")},
	}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-reviewer",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:         compactReviewerResponderTestConfig(),
			Store:          reviewerContextcompactTestStore(),
			FileWriter:     reviewerContextcompactTestFileWriter(),
			FullSummarizer: contextcompact.StaticFullSummarizer{},
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	out, err := responder.Respond(context.Background(), Context{
		Input: GraphInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3)},
		Text:  "Review Target\n- shot: shot_01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Metadata["context_compaction_retry"] != true {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
	if streamer.streamCalls != 2 {
		t.Fatalf("streamCalls = %d, want 2", streamer.streamCalls)
	}
	if !strings.Contains(streamer.messages[1].Content, "# Compacted Agent Handoff Summary") {
		t.Fatalf("retry prompt missing full summary: %#v", streamer.messages)
	}
}

func TestReviewerPromptIncludesHistoricalThreadMessages(t *testing.T) {
	userContent, err := uimessage.BuildUserMessageContent(uimessage.UserMessageInput{Text: "用户要求 Reviewer 重点检查产品一致性"})
	if err != nil {
		t.Fatal(err)
	}
	assistantContent, err := uimessage.BuildAssistantMessageContent(uimessage.AssistantMessageInput{Text: "已将产品一致性设为评审重点"})
	if err != nil {
		t.Fatal(err)
	}
	messages := reviewToolPromptMessages(Context{
		Text: "Review Target\n- shot: shot_01\n- phase: preview_image",
		Messages: []db.AgentMessage{
			{Role: "user", MessageType: "text", Content: userContent},
			{Role: "assistant", MessageType: "text", Content: assistantContent},
			{
				Role:        "assistant",
				MessageType: "tool_call",
				RawMessage:  []byte(`{"tool_call_id":"call_review_old","tool_name":"submit_review_result","arguments":{"verdict":"accepted_with_warnings"}}`),
			},
			{
				Role:        "tool",
				MessageType: "tool_result",
				Content:     []byte(`{"text":"工具返回：Reviewer 结果已提交"}`),
				RawMessage:  []byte(`{"tool_call_id":"call_review_old","tool_name":"submit_review_result","result_text":"Reviewer 结果已提交"}`),
			},
		},
	})

	if len(messages) != 6 {
		t.Fatalf("message count = %d, messages = %#v", len(messages), messages)
	}
	if messages[1].Role != schema.User || !strings.Contains(messages[1].Content, "重点检查产品一致性") {
		t.Fatalf("historical user message missing: %#v", messages[1])
	}
	if messages[2].Role != schema.Assistant || !strings.Contains(messages[2].Content, "产品一致性设为评审重点") {
		t.Fatalf("historical assistant message missing: %#v", messages[2])
	}
	if messages[3].Role != schema.Assistant || len(messages[3].ToolCalls) != 1 || messages[3].ToolCalls[0].ID != "call_review_old" || messages[3].ToolCalls[0].Function.Name != "submit_review_result" {
		t.Fatalf("historical tool call missing: %#v", messages[3])
	}
	if messages[4].Role != schema.Tool || messages[4].ToolCallID != "call_review_old" || !strings.Contains(messages[4].Content, "Reviewer 结果已提交") {
		t.Fatalf("historical tool result missing: %#v", messages[4])
	}
	if messages[5].Role != schema.User || !strings.Contains(messages[5].Content, "phase: preview_image") {
		t.Fatalf("current review target should be last user message before same-turn tools: %#v", messages[5])
	}
}

func TestReviewerPendingRemindersAppendToLatestUserMessage(t *testing.T) {
	messages := reviewToolPromptMessages(Context{
		Text:             "Review Target\n- shot: shot_01\n- phase: preview_image",
		PendingReminders: []string{"<system-reminder>你已连续调用 read_project_context 5 次，请反思策略。</system-reminder>"},
	})

	if len(messages) != 2 {
		t.Fatalf("message count = %d, messages = %#v", len(messages), messages)
	}
	if messages[0].Role != schema.System {
		t.Fatalf("first message = %#v", messages[0])
	}
	if messages[1].Role != schema.User ||
		!strings.Contains(messages[1].Content, "phase: preview_image") ||
		!strings.Contains(messages[1].Content, "运行时提醒") ||
		!strings.Contains(messages[1].Content, "read_project_context") {
		t.Fatalf("latest user message should include pending reminder: %#v", messages[1])
	}
}

type fakeReviewArkStreamer struct {
	messages    []*schema.Message
	chunks      []*schema.Message
	boundTools  []*schema.ToolInfo
	streamErrs  []error
	streamCalls int
}

func (f *fakeReviewArkStreamer) Stream(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
	f.messages = messages
	if f.streamCalls < len(f.streamErrs) && f.streamErrs[f.streamCalls] != nil {
		f.streamCalls++
		return nil, f.streamErrs[f.streamCalls-1]
	}
	f.streamCalls++
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
