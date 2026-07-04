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

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
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

func TestProducerSystemPromptContainsDomainConcepts(t *testing.T) {
	prompt := ProducerSystemPrompt(ProducerContext{})
	for _, want := range []string{
		"ClipAnvil 领域概念",
		"ProjectMemory 是项目创作宪法",
		"KeyElement 是视频中需要保持一致或复用的关键元素",
		"Storyboard 不是一段纯文本脚本",
		"不要在 Producer 字段中写 Seedance",
		"## Skills Library",
		"load_agent_skill",
		"commerce-ad-producer",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"当前 Project Context",
		"已有 2 个分镜",
		"# Commerce Ad Producer",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt contains dynamic project state %q", forbidden)
		}
	}
}

func TestProducerPromptNoSeedanceKeepsDynamicStoryboard(t *testing.T) {
	prompt := ProducerSystemPrompt(ProducerContext{})
	for _, needle := range []string{
		"no-Seedance 不等于固定模板",
		"继续使用动态 Storyboard",
		"30 秒左右营销视频通常需要 4-9 个 shot",
		"no-Seedance low-cost final route should prefer Seedream stills plus remotion_timeline_v1 final Composer",
		"do not require every shot to become motion_shot_video",
		"dispatch_composer with template_key=remotion_timeline_v1",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("Producer prompt missing %q", needle)
		}
	}
}

func TestProducerPromptPrefersRemotionTimelineForNoSeedanceLowCostRoute(t *testing.T) {
	prompt := ProducerSystemPrompt(ProducerContext{})
	for _, needle := range []string{
		"no-Seedance low-cost final route should prefer Seedream stills plus remotion_timeline_v1 final Composer",
		"do not require every shot to become motion_shot_video",
		"dispatch_composer with template_key=remotion_timeline_v1",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("producer prompt missing %q", needle)
		}
	}
}

func TestProducerPromptDefinesMixedCostRemotionRoute(t *testing.T) {
	prompt := ProducerSystemPrompt(ProducerContext{})
	for _, needle := range []string{
		"mixed-cost 路线只在 hero shot、复杂真实运动",
		"其余分镜仍用 Seedream still",
		"最终统一交给 remotion_timeline_v1",
		"Seedance 使用数量和成本风险",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("producer prompt missing mixed-cost route wording %q", needle)
		}
	}
}

func TestProducerSystemPromptEnablesCurrentGenerationAndReviewerGate(t *testing.T) {
	prompt := ProducerSystemPrompt(ProducerContext{})
	for _, forbidden := range []string{
		"M1 阶段只记录需求",
		"M1 可用工具",
		"M1 阶段不要调度 Craftsman",
		"在 M2 调度",
		"然后在 M2 调用",
		"M2 可用生成调度工具",
		"## M2 生成调度能力",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt still contains milestone-only wording %q", forbidden)
		}
	}
	for _, required := range []string{
		"当前生成调度能力",
		"dispatch_craftsman",
		"decide_render_plan",
		"execute_immediately",
		"wait_for_producer",
		"dispatch_reviewer",
		"Reviewer 是质量 gate",
		"Reviewer 不直接重跑生成",
		"select_artifact_version",
		"不要虚构 compile_render_plan",
		"reference_generation_succeeded",
		"audio_generation_succeeded",
		"worker_generation_completed",
		"composition_completed",
		"fallback_strategy",
		"不要继续同一路线自动重试",
		"motion shot fallback",
		"cost_risk",
		"用户明确授权自动推进",
		"shot_04.preview_image.r1.node",
		"media_node",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt missing current capability wording %q", required)
		}
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
	if strings.Contains(logs.String(), "producer model response empty content") {
		t.Fatalf("tool-call-only response should not be logged as empty content: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "producer model response completed") {
		t.Fatalf("logs = %s", logs.String())
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

func TestVolcengineModelResponderAppliesContextCompactionProjection(t *testing.T) {
	longResult := strings.Repeat("ffmpeg stderr line with frame details\n", 180)
	streamer := &fakeArkStreamer{chunks: []*schema.Message{{Content: "ok"}}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:     compactResponderTestConfig(),
			Store:      contextcompactTestStore(),
			FileWriter: contextcompactTestFileWriter(),
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})
	originalContent := []byte(`{"text":"` + longResult + `"}`)

	out, err := responder.Respond(context.Background(), ProducerContext{
		Input: ProducerTurnInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3)},
		Messages: []db.AgentMessage{
			{Role: "user", MessageType: "text", Content: mustUserContent(t, uimessage.UserMessageInput{Text: "继续"})},
			{
				Role:        "tool",
				MessageType: "tool_result",
				Content:     originalContent,
				RawMessage:  []byte(`{"tool_call_id":"call-ffmpeg","tool_name":"run_ffmpeg_command","result_text":` + mustJSONString(t, longResult) + `}`),
			},
			{Role: "user", MessageType: "text", Content: mustUserContent(t, uimessage.UserMessageInput{Text: "请继续推进"})},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Metadata["context_compaction_applied"] != true {
		t.Fatalf("metadata missing compaction applied: %#v", out.Metadata)
	}
	if out.Metadata["context_compaction_mode"] != "micro" || out.Metadata["context_compaction_count"] != 1 {
		t.Fatalf("metadata missing compaction mode/count: %#v", out.Metadata)
	}
	if refs, ok := out.Metadata["context_compaction_refs"].([]string); !ok || len(refs) == 0 {
		t.Fatalf("metadata missing compaction refs: %#v", out.Metadata)
	}
	if files, ok := out.Metadata["context_compaction_detail_files"].([]string); !ok || len(files) == 0 {
		t.Fatalf("metadata missing compaction detail files: %#v", out.Metadata)
	}
	if len(streamer.messages) < 3 {
		t.Fatalf("messages = %#v", streamer.messages)
	}
	tool := streamer.messages[2]
	if tool.Role != schema.Tool || tool.ToolCallID != "call-ffmpeg" || tool.ToolName != "run_ffmpeg_command" {
		t.Fatalf("tool message identity changed: %#v", tool)
	}
	if !strings.Contains(tool.Content, "compact_ref:") || !strings.Contains(tool.Content, "detail_file: /workspace/.clipanvil/context/") {
		t.Fatalf("tool message was not compacted: %s", tool.Content)
	}
	if strings.Contains(tool.Content, "ffmpeg stderr line with frame details") {
		t.Fatal("provider input still contains original long tool result")
	}
	if string(originalContent) != `{"text":"`+longResult+`"}` {
		t.Fatal("original agent_message content was mutated")
	}
}

func TestVolcengineModelResponderRetriesOnceWithFullCompactOnContextOverflow(t *testing.T) {
	streamer := &fakeArkStreamer{
		chunks:     []*schema.Message{{Content: "ok"}},
		streamErrs: []error{errors.New("prompt is too long")},
	}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:         compactResponderTestConfig(),
			Store:          contextcompactTestStore(),
			FileWriter:     contextcompactTestFileWriter(),
			FullSummarizer: contextcompact.StaticFullSummarizer{},
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	out, err := responder.Respond(context.Background(), ProducerContext{
		Input:          ProducerTurnInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3)},
		LatestUserText: "继续生成营销视频",
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

func TestProducerPromptMessagesKeepsSystemPromptStable(t *testing.T) {
	messages := producerPromptMessages(ProducerContext{
		LatestUserText: "创建分镜",
	})
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if strings.Contains(messages[0].Content, "当前 Project Context") ||
		strings.Contains(messages[0].Content, "当前还没有 storyboard") {
		t.Fatalf("system prompt = %q", messages[0].Content)
	}
	if !strings.Contains(messages[0].Content, "upsert_storyboard") {
		t.Fatalf("system prompt missing tool guidance = %q", messages[0].Content)
	}
	for _, want := range []string{"AudioPlan", "upsert_audio_plan", "seed-audio-1.0", "request_user_decision"} {
		if !strings.Contains(messages[0].Content, want) {
			t.Fatalf("system prompt missing audio guidance %q", want)
		}
	}
	for _, want := range []string{"scope.type=audio_plan", "target_phase=voiceover_audio", "target_phase=bgm_audio", "shot_refs=[]", "不要使用 mode=preview_image 或 mode=shot_video", "等 voiceover_audio 和 bgm_audio 媒体资产都成功"} {
		if !strings.Contains(messages[0].Content, want) {
			t.Fatalf("system prompt missing audio dispatch guidance %q", want)
		}
	}
	for _, want := range []string{"final video", "audio_sync", "platform selling power", "final_video_review"} {
		if !strings.Contains(messages[0].Content, want) {
			t.Fatalf("system prompt missing final review guidance %q", want)
		}
	}
	for _, want := range []string{"reference_generation_succeeded", "audio_generation_succeeded", "shot_04.preview_image.r1.node", "media_node"} {
		if !strings.Contains(messages[0].Content, want) {
			t.Fatalf("system prompt missing signal/ref guidance %q", want)
		}
	}
}

func TestProducerPromptMessagesAppendsPendingRemindersToUserMessage(t *testing.T) {
	messages := producerPromptMessages(ProducerContext{
		LatestUserText:   "继续",
		PendingReminders: []string{"<system-reminder>你已连续调用 read_project_context 5 次，请反思策略。</system-reminder>"},
	})
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if messages[1].Role != schema.User ||
		!strings.Contains(messages[1].Content, "继续") ||
		!strings.Contains(messages[1].Content, "运行时提醒") ||
		!strings.Contains(messages[1].Content, "read_project_context") {
		t.Fatalf("user message with reminder = %#v", messages[1])
	}
	for _, message := range messages[1:] {
		if message.Role == schema.System {
			t.Fatalf("pending reminder must not create extra system message: %#v", messages)
		}
	}
}

func TestProducerPromptMessagesAppendsPendingRemindersToLatestToolResult(t *testing.T) {
	messages := producerPromptMessages(ProducerContext{
		SameTurnMessages: []ProducerSameTurnMessage{
			{
				Role:        "tool",
				MessageType: "tool_result",
				Content:     "已读取项目上下文",
				ToolCallID:  "call-read",
				ToolName:    "read_project_context",
			},
		},
		PendingReminders: []string{"<system-reminder>你有 2 个待处理 Producer signal。</system-reminder>"},
	})
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if messages[1].Role != schema.Tool ||
		!strings.Contains(messages[1].Content, "已读取项目上下文") ||
		!strings.Contains(messages[1].Content, "运行时提醒") ||
		!strings.Contains(messages[1].Content, "待处理 Producer signal") {
		t.Fatalf("tool result with reminder = %#v", messages[1])
	}
}

func TestProducerPromptMessagesIncludesRuntimeTrigger(t *testing.T) {
	trigger := strings.Join([]string{
		"系统事件：Craftsman 已完成 RenderPlan 编译。",
		"触发原因：craftsman_render_plan_ready。",
		"下一步：请读取项目上下文，检查 waiting_for_approval RenderPlan，并决定 accept/reject 或派 Reviewer。",
	}, "\n")
	messages := producerPromptMessages(ProducerContext{
		Messages: []db.AgentMessage{
			{
				Role:        "assistant",
				MessageType: "text",
				Content:     mustAssistantContent(t, uimessage.AssistantMessageInput{Text: "我已派发 Craftsman。"}),
			},
		},
		RuntimeTriggerText: trigger,
	})
	if len(messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(messages))
	}
	if messages[0].Role != schema.System ||
		strings.Contains(messages[0].Content, "当前触发事件") ||
		strings.Contains(messages[0].Content, "Craftsman 已完成 RenderPlan 编译") {
		t.Fatalf("system prompt = %q", messages[0].Content)
	}
	if messages[1].Role != schema.Assistant {
		t.Fatalf("history assistant message = %#v", messages[1])
	}
	if messages[2].Role != schema.User ||
		!strings.Contains(messages[2].Content, "系统事件：Craftsman 已完成 RenderPlan 编译") ||
		!strings.Contains(messages[2].Content, "decide") {
		t.Fatalf("runtime trigger user message = %#v", messages[2])
	}
}

func TestProducerPromptMessagesKeepsRuntimeTriggerAfterSystemReminderBeforeToolLoop(t *testing.T) {
	trigger := "<system-reminder>系统事件：Craftsman 已完成 RenderPlan 编译，请处理 pending signal。</system-reminder>"
	messages := producerPromptMessages(ProducerContext{
		Messages: []db.AgentMessage{
			{
				Role:        "assistant",
				MessageType: "text",
				Content:     mustAssistantContent(t, uimessage.AssistantMessageInput{Text: "我已派发 Craftsman。"}),
			},
		},
		PendingReminders: []string{"<system-reminder>你有 2 个待处理 Producer signal。</system-reminder>"},
		SameTurnMessages: []ProducerSameTurnMessage{
			{
				Role:        "system",
				MessageType: "system_reminder",
				Content:     "<system-reminder>你有 2 个待处理 Producer signal。</system-reminder>",
			},
		},
		RuntimeTriggerText: trigger,
	})

	last := messages[len(messages)-1]
	if last.Role != schema.User ||
		!strings.Contains(last.Content, "系统事件：Craftsman 已完成 RenderPlan 编译") ||
		!strings.Contains(last.Content, "decide_render_plan") {
		t.Fatalf("last runtime trigger message = %#v", last)
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
				Content:          "调用 read_workspace_context",
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

func TestProducerPromptMessagesUsesRuntimeTriggerOnlyBeforeToolLoop(t *testing.T) {
	userContent := mustUserContent(t, uimessage.UserMessageInput{Text: "继续"})
	messages := producerPromptMessages(ProducerContext{
		RuntimeTriggerText: "<system-reminder>shot_05 ready</system-reminder>",
		Messages: []db.AgentMessage{
			{Role: "user", MessageType: "text", Content: userContent},
		},
		SameTurnMessages: []ProducerSameTurnMessage{
			{
				Role:          "assistant",
				MessageType:   "tool_call",
				ToolCallID:    "call-read",
				ToolName:      "read_project_context",
				ToolArguments: map[string]any{"scope": map[string]any{"type": "workspace", "id": ""}},
			},
			{
				Role:        "tool",
				MessageType: "tool_result",
				ToolCallID:  "call-read",
				ToolName:    "read_project_context",
				Content:     "还有 4 个 waiting_for_approval RenderPlan",
			},
		},
	})

	for _, message := range messages {
		if message.Role == schema.User && strings.Contains(message.Content, "shot_05 ready") {
			t.Fatalf("runtime trigger leaked after tool loop started: %#v", message)
		}
	}
}

func TestProducerPromptMessagesRebuildsHistoricalToolCallAndResult(t *testing.T) {
	userContent := mustUserContent(t, uimessage.UserMessageInput{Text: "继续检查状态"})
	messages := producerPromptMessages(ProducerContext{
		Messages: []db.AgentMessage{
			{Role: "user", MessageType: "text", Content: userContent},
			{
				Role:        "assistant",
				MessageType: "tool_call",
				Content:     []byte(`{"text":"调用 read_project_context"}`),
				RawMessage:  []byte(`{"tool_call_id":"call-read","tool_name":"read_project_context","arguments":{"include":["memory"]}}`),
			},
			{
				Role:        "tool",
				MessageType: "tool_result",
				Content:     []byte(`{"text":"项目上下文：机场场景 needs_reference"}`),
				RawMessage:  []byte(`{"tool_call_id":"call-read","tool_name":"read_project_context","result_text":"项目上下文：机场场景 needs_reference"}`),
			},
		},
	})

	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(messages))
	}
	assistant := messages[2]
	if assistant.Role != schema.Assistant || len(assistant.ToolCalls) != 1 {
		t.Fatalf("historical tool call = %#v", assistant)
	}
	if assistant.ToolCalls[0].ID != "call-read" || assistant.ToolCalls[0].Function.Name != "read_project_context" {
		t.Fatalf("historical tool call function = %#v", assistant.ToolCalls[0])
	}
	tool := messages[3]
	if tool.Role != schema.Tool || tool.ToolCallID != "call-read" || tool.ToolName != "read_project_context" {
		t.Fatalf("historical tool result = %#v", tool)
	}
	if !strings.Contains(tool.Content, "needs_reference") {
		t.Fatalf("historical tool result content = %q", tool.Content)
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
			{Role: "assistant", MessageType: "tool_call", Content: "调用 read_workspace_context", ReasoningContent: "需要先读取上下文"},
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
	messages    []*schema.Message
	chunks      []*schema.Message
	boundTools  []*schema.ToolInfo
	streamErrs  []error
	streamCalls int
}

func (f *fakeArkStreamer) Stream(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
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
