package craftsman

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
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
	model := &fakeCraftsmanArkModel{message: &schema.Message{
		Content: "已提交 RenderPlan。",
	}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatModel, error) {
			return model, nil
		},
	})

	out, err := responder.Respond(context.Background(), Context{Text: "shot-01 开场"})
	if err != nil {
		t.Fatal(err)
	}
	if out.AssistantText != "已提交 RenderPlan。" || out.Metadata["model_id"] != "doubao-test" || out.ModelMessage == nil {
		t.Fatalf("output = %#v", out)
	}
	if len(model.messages) != 2 || !strings.Contains(model.messages[1].Content, "shot-01") {
		t.Fatalf("messages = %#v", model.messages)
	}
}

func TestVolcengineCraftsmanResponderReturnsNativeToolCallFromGenerate(t *testing.T) {
	model := &fakeCraftsmanArkModel{message: &schema.Message{
		ToolCalls: []schema.ToolCall{{
			ID:   "call-plan",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "upsert_render_plan",
				Arguments: `{"brief":"创建预览图计划","generation_text":"机场晨光中的银色行李箱"}`,
			},
		}},
	}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatModel, error) {
			return model, nil
		},
	})

	out, err := responder.Respond(context.Background(), Context{
		Text:      "Current Task\n- target_phase: preview_image\n- shot: shot_01",
		ToolInfos: []*schema.ToolInfo{{Name: "upsert_render_plan", Desc: "创建 RenderPlan"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelMessage == nil || len(out.ModelMessage.ToolCalls) != 1 || out.ModelMessage.ToolCalls[0].Function.Name != "upsert_render_plan" {
		t.Fatalf("model message = %#v", out.ModelMessage)
	}
	if model.generateCalls != 1 {
		t.Fatalf("generate calls = %d, want 1", model.generateCalls)
	}
}

func TestCraftsmanPromptIncludesHistoricalThreadMessages(t *testing.T) {
	userContent, err := uimessage.BuildUserMessageContent(uimessage.UserMessageInput{Text: "用户要求把机场改成清晨"})
	if err != nil {
		t.Fatal(err)
	}
	assistantContent, err := uimessage.BuildAssistantMessageContent(uimessage.AssistantMessageInput{Text: "已记录清晨机场设定"})
	if err != nil {
		t.Fatal(err)
	}
	messages := craftsmanToolPromptMessages(Context{
		Text: "Current Task\n- target_phase: preview_image\n- shot: shot_01",
		Messages: []db.AgentMessage{
			{Role: "user", MessageType: "text", Content: userContent},
			{Role: "assistant", MessageType: "text", Content: assistantContent},
			{
				Role:        "assistant",
				MessageType: "tool_call",
				RawMessage:  []byte(`{"tool_call_id":"call_old","tool_name":"upsert_render_plan","arguments":{"brief":"旧的清晨机场预览图计划"}}`),
			},
			{
				Role:        "tool",
				MessageType: "tool_result",
				Content:     []byte(`{"text":"工具返回：RenderPlan 已保存"}`),
				RawMessage:  []byte(`{"tool_call_id":"call_old","tool_name":"upsert_render_plan","result_text":"RenderPlan 已保存"}`),
			},
		},
	})

	if len(messages) != 6 {
		t.Fatalf("message count = %d, messages = %#v", len(messages), messages)
	}
	if messages[1].Role != schema.User || !strings.Contains(messages[1].Content, "用户要求把机场改成清晨") {
		t.Fatalf("historical user message missing: %#v", messages[1])
	}
	if messages[2].Role != schema.Assistant || !strings.Contains(messages[2].Content, "已记录清晨机场设定") {
		t.Fatalf("historical assistant message missing: %#v", messages[2])
	}
	if messages[3].Role != schema.Assistant || len(messages[3].ToolCalls) != 1 || messages[3].ToolCalls[0].ID != "call_old" || messages[3].ToolCalls[0].Function.Name != "upsert_render_plan" {
		t.Fatalf("historical tool call missing: %#v", messages[3])
	}
	if messages[4].Role != schema.Tool || messages[4].ToolCallID != "call_old" || !strings.Contains(messages[4].Content, "RenderPlan 已保存") {
		t.Fatalf("historical tool result missing: %#v", messages[4])
	}
	if messages[5].Role != schema.User || !strings.Contains(messages[5].Content, "target_phase: preview_image") {
		t.Fatalf("current task context should be last user message before same-turn tools: %#v", messages[5])
	}
}

type fakeCraftsmanArkModel struct {
	messages      []*schema.Message
	message       *schema.Message
	chunks        []*schema.Message
	generateCalls int
	toolInfos     []*schema.ToolInfo
}

func (f *fakeCraftsmanArkModel) WithTools(tools []*schema.ToolInfo) (einoModel.ToolCallingChatModel, error) {
	f.toolInfos = append([]*schema.ToolInfo(nil), tools...)
	return f, nil
}

func (f *fakeCraftsmanArkModel) Generate(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.Message, error) {
	f.messages = messages
	f.generateCalls++
	return f.message, nil
}

func (f *fakeCraftsmanArkModel) Stream(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
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
