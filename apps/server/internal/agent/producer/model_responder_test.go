package producer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	arkModel "github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"

	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestVolcengineModelResponderStreamsDeltasAndReturnsFinalText(t *testing.T) {
	streamer := &fakeArkStreamer{
		chunks: []*schema.Message{
			{Content: "第一段"},
			{Content: "，第二段"},
		},
	}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})
	deltas := []string{}

	out, err := responder.Respond(context.Background(), ProducerContext{
		LatestUserText: "做一条新品短片",
		EmitDelta: func(_ context.Context, delta ProducerStreamDelta) error {
			deltas = append(deltas, delta.Delta)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if out.AssistantText != "第一段，第二段" {
		t.Fatalf("assistant text = %q", out.AssistantText)
	}
	if strings.Join(deltas, "") != out.AssistantText {
		t.Fatalf("deltas = %#v, assistant = %q", deltas, out.AssistantText)
	}
	if out.Metadata["provider"] != "volcengine" || out.Metadata["model_id"] != "doubao-test" {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
	if len(streamer.messages) < 2 {
		t.Fatalf("messages = %#v", streamer.messages)
	}
	if streamer.messages[1].Content != "做一条新品短片" {
		t.Fatalf("user prompt = %q", streamer.messages[1].Content)
	}
}

func TestVolcengineModelResponderUsesSelectedModel(t *testing.T) {
	streamer := &fakeArkStreamer{chunks: []*schema.Message{{Content: "ok"}}}
	var gotModel string
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "env-default",
		Factory: func(_ context.Context, cfg *ark.ChatModelConfig) (arkChatStreamer, error) {
			gotModel = cfg.Model
			return streamer, nil
		},
	})

	out, err := responder.Respond(context.Background(), ProducerContext{
		LatestUserText: "hello",
		Model: ProducerModelSelection{
			ProviderID:  "volcengine",
			ModelID:     "workspace-model",
			DisplayName: "Workspace Model",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotModel != "workspace-model" {
		t.Fatalf("factory model = %q, want workspace-model", gotModel)
	}
	if out.Metadata["model_id"] != "workspace-model" || out.Metadata["model_display_name"] != "Workspace Model" {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
}

func TestVolcengineModelResponderReturnsNativeToolCalls(t *testing.T) {
	streamer := &fakeArkStreamer{
		chunks: []*schema.Message{
			{
				ToolCalls: []schema.ToolCall{
					{
						ID:   "call-update-storyboard",
						Type: "function",
						Function: schema.FunctionCall{
							Name:      "update_storyboard",
							Arguments: `{"intent":"replace","shots":[{"client_key":"shot-01","sort_order":1,"title":"开场"}]}`,
						},
					},
				},
			},
		},
	}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	out, err := responder.Respond(context.Background(), ProducerContext{LatestUserText: "拆分镜"})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelMessage == nil {
		t.Fatal("ModelMessage is nil")
	}
	if len(out.ModelMessage.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", out.ModelMessage.ToolCalls)
	}
	if out.ModelMessage.ToolCalls[0].Function.Name != "update_storyboard" {
		t.Fatalf("tool name = %q", out.ModelMessage.ToolCalls[0].Function.Name)
	}
	if out.Metadata["native_tool_call_count"] != 1 {
		t.Fatalf("metadata = %#v", out.Metadata)
	}
}

func TestVolcengineModelResponderBindsProducerTools(t *testing.T) {
	streamer := &fakeArkStreamer{chunks: []*schema.Message{{Content: "ok"}}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	_, err := responder.Respond(context.Background(), ProducerContext{
		LatestUserText: "拆分镜",
		ToolInfos: []*schema.ToolInfo{
			{Name: "update_storyboard", Desc: "Update storyboard."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(streamer.boundTools) != 1 || streamer.boundTools[0].Name != "update_storyboard" {
		t.Fatalf("bound tools = %#v", streamer.boundTools)
	}
}

func TestVolcengineModelResponderConfiguresReasoningEffort(t *testing.T) {
	streamer := &fakeArkStreamer{chunks: []*schema.Message{{Content: "ok"}}}
	var gotConfig *ark.ChatModelConfig
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey:    "test-key",
		Model:     "env-default",
		MaxTokens: 2048,
		Factory: func(_ context.Context, cfg *ark.ChatModelConfig) (arkChatStreamer, error) {
			gotConfig = cfg
			return streamer, nil
		},
	})

	_, err := responder.Respond(context.Background(), ProducerContext{
		LatestUserText: "hello",
		Model: ProducerModelSelection{
			ProviderID:          "volcengine",
			ModelID:             "thinking-model",
			SupportsThinking:    true,
			ReasoningEffort:     "high",
			MaxCompletionTokens: 4096,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotConfig.ReasoningEffort == nil || *gotConfig.ReasoningEffort != arkModel.ReasoningEffortHigh {
		t.Fatalf("ReasoningEffort = %#v, want high", gotConfig.ReasoningEffort)
	}
	if gotConfig.Thinking == nil || gotConfig.Thinking.Type != arkModel.ThinkingTypeEnabled {
		t.Fatalf("Thinking = %#v, want enabled", gotConfig.Thinking)
	}
	if gotConfig.MaxCompletionTokens == nil || *gotConfig.MaxCompletionTokens != 4096 {
		t.Fatalf("MaxCompletionTokens = %#v, want 4096", gotConfig.MaxCompletionTokens)
	}
	if gotConfig.MaxTokens != nil {
		t.Fatalf("MaxTokens = %#v, want nil for thinking-capable model", gotConfig.MaxTokens)
	}
}

func TestVolcengineModelResponderMinimalReasoningDisablesThinking(t *testing.T) {
	streamer := &fakeArkStreamer{chunks: []*schema.Message{{Content: "ok"}}}
	var gotConfig *ark.ChatModelConfig
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey:    "test-key",
		Model:     "env-default",
		MaxTokens: 2048,
		Factory: func(_ context.Context, cfg *ark.ChatModelConfig) (arkChatStreamer, error) {
			gotConfig = cfg
			return streamer, nil
		},
	})

	_, err := responder.Respond(context.Background(), ProducerContext{
		LatestUserText: "hello",
		Model: ProducerModelSelection{
			ProviderID:          "volcengine",
			ModelID:             "thinking-model",
			SupportsThinking:    true,
			ReasoningEffort:     "minimal",
			MaxCompletionTokens: 4096,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotConfig.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort = %#v, want nil for minimal", gotConfig.ReasoningEffort)
	}
	if gotConfig.Thinking == nil || gotConfig.Thinking.Type != arkModel.ThinkingTypeDisabled {
		t.Fatalf("Thinking = %#v, want disabled", gotConfig.Thinking)
	}
	if gotConfig.MaxCompletionTokens == nil || *gotConfig.MaxCompletionTokens != 4096 {
		t.Fatalf("MaxCompletionTokens = %#v, want 4096", gotConfig.MaxCompletionTokens)
	}
	if gotConfig.MaxTokens != nil {
		t.Fatalf("MaxTokens = %#v, want nil for thinking-capable model", gotConfig.MaxTokens)
	}
}

func TestVolcengineModelResponderStreamsVisibleReasoningDeltas(t *testing.T) {
	streamer := &fakeArkStreamer{
		chunks: []*schema.Message{
			{ReasoningContent: "先分析"},
			{Content: "结论"},
		},
	}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})
	deltas := []ProducerStreamDelta{}

	out, err := responder.Respond(context.Background(), ProducerContext{
		LatestUserText: "hello",
		Model: ProducerModelSelection{
			SupportsThinking: true,
			ReasoningEffort:  "high",
		},
		EmitDelta: func(_ context.Context, delta ProducerStreamDelta) error {
			deltas = append(deltas, delta)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 2 {
		t.Fatalf("deltas = %#v", deltas)
	}
	if deltas[0].BlockID != uimessage.BlockIDThinking || deltas[0].BlockType != "thinking" || deltas[0].Delta != "先分析" {
		t.Fatalf("thinking delta = %#v", deltas[0])
	}
	if deltas[1].BlockID != uimessage.BlockIDAnswer || deltas[1].BlockType != "markdown" || deltas[1].Delta != "结论" {
		t.Fatalf("content delta = %#v", deltas[1])
	}
	if out.Metadata["reasoning_content"] != "先分析" {
		t.Fatalf("metadata reasoning_content = %#v", out.Metadata["reasoning_content"])
	}
	if out.Metadata["visible_thinking"] != true {
		t.Fatalf("visible_thinking = %#v, want true", out.Metadata["visible_thinking"])
	}
}

func TestVolcengineModelResponderSuppressesUnsupportedReasoningDeltas(t *testing.T) {
	streamer := &fakeArkStreamer{
		chunks: []*schema.Message{
			{ReasoningContent: "隐藏思考"},
			{Content: "结论"},
		},
	}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})
	deltas := []ProducerStreamDelta{}

	out, err := responder.Respond(context.Background(), ProducerContext{
		LatestUserText: "hello",
		Model: ProducerModelSelection{
			SupportsThinking: false,
			ReasoningEffort:  "high",
		},
		EmitDelta: func(_ context.Context, delta ProducerStreamDelta) error {
			deltas = append(deltas, delta)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 1 {
		t.Fatalf("deltas = %#v", deltas)
	}
	if deltas[0].BlockID != uimessage.BlockIDAnswer || deltas[0].BlockType != "markdown" || deltas[0].Delta != "结论" {
		t.Fatalf("content delta = %#v", deltas[0])
	}
	if out.Metadata["reasoning_content"] != "隐藏思考" {
		t.Fatalf("metadata reasoning_content = %#v", out.Metadata["reasoning_content"])
	}
	if out.Metadata["visible_thinking"] != false {
		t.Fatalf("visible_thinking = %#v, want false", out.Metadata["visible_thinking"])
	}
}

func TestProducerPromptMessagesOmitsCompletedHistoricalReasoning(t *testing.T) {
	assistantContent := mustAssistantContent(t, uimessage.AssistantMessageInput{
		Text:             "最终正文",
		ReasoningContent: "历史思考不应回传",
		IncludeThinking:  true,
		DefaultCollapsed: true,
	})
	userContent := mustUserContent(t, uimessage.UserMessageInput{Text: "继续"})
	messages := producerPromptMessages(ProducerContext{
		Messages: []db.AgentMessage{
			{
				Role:        "assistant",
				MessageType: "text",
				Content:     assistantContent,
				RawMessage:  []byte(`{"reasoning_content":"历史思考不应回传"}`),
			},
			{
				Role:        "user",
				MessageType: "text",
				Content:     userContent,
			},
		},
	})

	if len(messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(messages))
	}
	if messages[1].Role != schema.Assistant || messages[1].ReasoningContent != "" {
		t.Fatalf("assistant history = %#v", messages[1])
	}
	if strings.Contains(messages[1].Content, "历史思考不应回传") {
		t.Fatalf("assistant content leaked thinking = %#v", messages[1])
	}
}

func TestProducerPromptMessagesUseMarkdownBlocks(t *testing.T) {
	userContent := mustUserContent(t, uimessage.UserMessageInput{Text: "用户需求"})
	assistantContent := mustAssistantContent(t, uimessage.AssistantMessageInput{Text: "助手回复"})
	messages := producerPromptMessages(ProducerContext{
		Messages: []db.AgentMessage{
			{Role: "user", MessageType: "text", Content: userContent},
			{Role: "assistant", MessageType: "text", Content: assistantContent},
		},
	})
	if len(messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(messages))
	}
	if messages[1].Role != schema.User || messages[1].Content != "用户需求" {
		t.Fatalf("user prompt = %#v", messages[1])
	}
	if messages[2].Role != schema.Assistant || messages[2].Content != "助手回复" {
		t.Fatalf("assistant prompt = %#v", messages[2])
	}
}

func TestProducerPromptMessagesIncludesProductionStateSummary(t *testing.T) {
	messages := producerPromptMessages(ProducerContext{
		LatestUserText:      "创建分镜",
		ProductionStateText: "Storyboard\n- 当前还没有 storyboard。",
	})
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if !strings.Contains(messages[0].Content, "Production State Summary") ||
		!strings.Contains(messages[0].Content, "当前还没有 storyboard") ||
		!strings.Contains(messages[0].Content, "update_storyboard") {
		t.Fatalf("system prompt = %q", messages[0].Content)
	}
}

func TestProducerPromptMessagesSkipThinkingBlocks(t *testing.T) {
	content := []byte(`{
	  "schema":"clipanvil.agent.message.v1",
	  "blocks":[
	    {"id":"blk_thinking","type":"thinking","text":"不要进 prompt","status":"done","default_collapsed":true},
	    {"id":"blk_answer","type":"markdown","text":"只保留正文"}
	  ]
	}`)
	messages := producerPromptMessages(ProducerContext{
		Messages: []db.AgentMessage{{Role: "assistant", MessageType: "text", Content: content}},
	})
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if strings.Contains(messages[1].Content, "不要进 prompt") {
		t.Fatalf("prompt leaked thinking: %#v", messages[1])
	}
}

func TestProducerPromptMessagesIncludesSameTurnToolReasoningPassback(t *testing.T) {
	userContent := mustUserContent(t, uimessage.UserMessageInput{Text: "读取画布"})
	messages := producerPromptMessages(ProducerContext{
		Messages: []db.AgentMessage{
			{
				Role:        "user",
				MessageType: "text",
				Content:     userContent,
			},
		},
		SameTurnMessages: []ProducerSameTurnMessage{
			{
				Role:             "assistant",
				MessageType:      "tool_call",
				Content:          `{"tool_call":{"name":"read_workspace_context","arguments":{}}}`,
				ReasoningContent: "需要先读取上下文",
			},
			{
				Role:        "tool",
				MessageType: "tool_result",
				ToolCallID:  "call-1",
				ToolName:    "read_workspace_context",
				Content:     `{"ok":true}`,
			},
		},
	})

	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(messages))
	}
	assistant := messages[2]
	if assistant.Role != schema.Assistant || assistant.ReasoningContent != "需要先读取上下文" {
		t.Fatalf("same-turn assistant = %#v", assistant)
	}
	tool := messages[3]
	if tool.Role != schema.Tool || tool.ToolCallID != "call-1" || tool.ToolName != "read_workspace_context" {
		t.Fatalf("tool result = %#v", tool)
	}
}

func TestVolcengineModelResponderPersistsDiagnosticsForReasoningOnlyResponse(t *testing.T) {
	streamer := &fakeArkStreamer{
		chunks: []*schema.Message{
			{
				ReasoningContent: "只返回思考",
				ResponseMeta: &schema.ResponseMeta{
					FinishReason: "length",
					Usage: &schema.TokenUsage{
						PromptTokens:     12,
						CompletionTokens: 4096,
						TotalTokens:      4108,
						CompletionTokensDetails: schema.CompletionTokensDetails{
							ReasoningTokens: 4096,
						},
					},
				},
			},
		},
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Logger: logger,
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	out, err := responder.Respond(context.Background(), ProducerContext{
		LatestUserText: "hello",
		Model: ProducerModelSelection{
			ProviderID:          "volcengine",
			ModelID:             "doubao-test",
			SupportsThinking:    true,
			ReasoningEffort:     "high",
			MaxCompletionTokens: 4096,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.AssistantText != "" {
		t.Fatalf("assistant text = %q, want empty", out.AssistantText)
	}
	diagnostics, ok := out.Metadata["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("diagnostics = %#v", out.Metadata["diagnostics"])
	}
	if diagnostics["finish_reason"] != "length" || diagnostics["content_chars"] != 0 || diagnostics["reasoning_chars"] != utf8.RuneCountInString("只返回思考") {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if diagnostics["reasoning_tokens"] != 4096 || diagnostics["max_completion_tokens"] != 4096 {
		t.Fatalf("diagnostics token fields = %#v", diagnostics)
	}
	if !strings.Contains(logs.String(), "producer model response empty content") ||
		!strings.Contains(logs.String(), `"finish_reason":"length"`) ||
		!strings.Contains(logs.String(), `"content_chars":0`) {
		t.Fatalf("logs = %s", logs.String())
	}
}

func TestVolcengineModelResponderDiagnosticsTrackReasoningPassback(t *testing.T) {
	streamer := &fakeArkStreamer{chunks: []*schema.Message{{Content: "ok"}}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	out, err := responder.Respond(context.Background(), ProducerContext{
		Messages: []db.AgentMessage{
			{Role: "user", MessageType: "text", Content: mustUserContent(t, uimessage.UserMessageInput{Text: "读取画布"})},
		},
		SameTurnMessages: []ProducerSameTurnMessage{
			{Role: "assistant", MessageType: "tool_call", Content: `{"tool_call":{"name":"read_workspace_context","arguments":{}}}`, ReasoningContent: "需要先读取上下文"},
			{Role: "tool", MessageType: "tool_result", ToolCallID: "call-1", ToolName: "read_workspace_context", Content: `{"ok":true}`},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, ok := out.Metadata["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("diagnostics = %#v", out.Metadata["diagnostics"])
	}
	if diagnostics["reasoning_passback_enabled"] != true ||
		diagnostics["reasoning_passback_messages"] != 1 ||
		diagnostics["reasoning_passback_chars"] != utf8.RuneCountInString("需要先读取上下文") {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestProducerPromptMessagesBuildsImageInputParts(t *testing.T) {
	userContent := mustUserContent(t, uimessage.UserMessageInput{
		Text: "看这张图写脚本",
		Attachments: []uimessage.Attachment{
			{AssetID: "asset-1", NodeID: "node-1", Kind: "image", Name: "product.png", Mime: "image/png"},
		},
	})
	messages := producerPromptMessages(ProducerContext{
		Messages: []db.AgentMessage{
			{
				Role:        "user",
				MessageType: "text",
				Content:     userContent,
			},
		},
		ImageAttachments: map[string]ProducerImageAttachment{
			"asset-1": {AssetID: "asset-1", URL: "https://example.com/product.png", Mime: "image/png"},
		},
	})

	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	user := messages[1]
	if len(user.UserInputMultiContent) != 2 {
		t.Fatalf("UserInputMultiContent = %#v", user.UserInputMultiContent)
	}
	if user.UserInputMultiContent[0].Type != schema.ChatMessagePartTypeText {
		t.Fatalf("first part = %#v", user.UserInputMultiContent[0])
	}
	image := user.UserInputMultiContent[1]
	if image.Type != schema.ChatMessagePartTypeImageURL || image.Image == nil || image.Image.URL == nil || *image.Image.URL != "https://example.com/product.png" {
		t.Fatalf("image part = %#v", image)
	}
}

func TestVolcengineModelResponderRejectsNonVolcengineSelection(t *testing.T) {
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "env-default",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			t.Fatal("factory must not be called for unsupported provider")
			return nil, nil
		},
	})

	_, err := responder.Respond(context.Background(), ProducerContext{
		LatestUserText: "hello",
		Model: ProducerModelSelection{
			ProviderID: "mock",
			ModelID:    "mock-text",
		},
	})
	if !errors.Is(err, ErrAgentModelUnavailable) {
		t.Fatalf("error = %v, want ErrAgentModelUnavailable", err)
	}
}

func mustUIContent(t *testing.T, raw []byte, err error) []byte {
	t.Helper()
	if err != nil {
		t.Fatalf("build ui content: %v", err)
	}
	return raw
}

func mustUserContent(t *testing.T, input uimessage.UserMessageInput) []byte {
	t.Helper()
	raw, err := uimessage.BuildUserMessageContent(input)
	return mustUIContent(t, raw, err)
}

func mustAssistantContent(t *testing.T, input uimessage.AssistantMessageInput) []byte {
	t.Helper()
	raw, err := uimessage.BuildAssistantMessageContent(input)
	return mustUIContent(t, raw, err)
}

func TestVolcengineModelResponderRejectsMissingConfigBeforeNetwork(t *testing.T) {
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		Model: "doubao-test",
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			t.Fatal("factory must not be called without api key")
			return nil, nil
		},
	})

	_, err := responder.Respond(context.Background(), ProducerContext{LatestUserText: "hello"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY") {
		t.Fatalf("error = %v", err)
	}
}

type fakeArkStreamer struct {
	messages   []*schema.Message
	chunks     []*schema.Message
	boundTools []*schema.ToolInfo
}

func (f *fakeArkStreamer) Stream(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
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

func (f *fakeArkStreamer) WithTools(tools []*schema.ToolInfo) (einoModel.ToolCallingChatModel, error) {
	f.boundTools = tools
	return f, nil
}

func (f *fakeArkStreamer) Generate(context.Context, []*schema.Message, ...einoModel.Option) (*schema.Message, error) {
	if len(f.chunks) == 0 {
		return &schema.Message{}, nil
	}
	return schema.ConcatMessages(f.chunks)
}
