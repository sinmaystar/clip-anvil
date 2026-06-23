package producer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	arkModel "github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"

	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
)

type arkChatStreamer interface {
	Stream(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.StreamReader[*schema.Message], error)
}

func durationPtr(value time.Duration) *time.Duration {
	return &value
}

type arkChatModelFactory func(ctx context.Context, config *ark.ChatModelConfig) (arkChatStreamer, error)

type VolcengineModelResponderConfig struct {
	APIKey      string
	BaseURL     string
	Region      string
	Model       string
	MaxTokens   int
	Temperature float32
	Factory     arkChatModelFactory
	Logger      *slog.Logger
}

type VolcengineModelResponder struct {
	cfg     VolcengineModelResponderConfig
	factory arkChatModelFactory
}

func NewVolcengineModelResponder(cfg VolcengineModelResponderConfig) VolcengineModelResponder {
	factory := cfg.Factory
	if factory == nil {
		factory = func(ctx context.Context, config *ark.ChatModelConfig) (arkChatStreamer, error) {
			return ark.NewChatModel(ctx, config)
		}
	}
	return VolcengineModelResponder{cfg: cfg, factory: factory}
}

func (r VolcengineModelResponder) Respond(ctx context.Context, producerContext ProducerContext) (ProducerTurnOutput, error) {
	logger := r.logger()
	apiKey := strings.TrimSpace(r.cfg.APIKey)
	if apiKey == "" {
		return ProducerTurnOutput{}, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for Producer model")
	}
	selectedProvider := strings.TrimSpace(producerContext.Model.ProviderID)
	if selectedProvider != "" && selectedProvider != "volcengine" {
		return ProducerTurnOutput{}, NewAgentError("agent_model_unavailable", "selected Producer model provider is not supported")
	}
	modelID := strings.TrimSpace(producerContext.Model.ModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(r.cfg.Model)
	}
	if modelID == "" {
		return ProducerTurnOutput{}, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL is required for Producer model")
	}

	config := &ark.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: strings.TrimSpace(r.cfg.BaseURL),
		Region:  strings.TrimSpace(r.cfg.Region),
		Model:   modelID,
		Timeout: durationPtr(10 * time.Minute),
	}
	maxCompletionTokens := 0
	if producerContext.Model.SupportsThinking {
		policy := volcengineThinkingPolicy(producerContext.Model, r.cfg.MaxTokens)
		maxCompletionTokens = policy.MaxCompletionTokens
		if maxCompletionTokens > 0 {
			config.MaxCompletionTokens = &maxCompletionTokens
		}
		if policy.Thinking != nil {
			config.Thinking = policy.Thinking
		}
		if policy.ReasoningEffort != nil {
			effort := *policy.ReasoningEffort
			config.ReasoningEffort = &effort
		}
	} else if r.cfg.MaxTokens > 0 {
		config.MaxTokens = &r.cfg.MaxTokens
	}
	if r.cfg.Temperature > 0 {
		config.Temperature = &r.cfg.Temperature
	}
	model, err := r.factory(ctx, config)
	if err != nil {
		logProducerModelFailure(ctx, logger, "create_model", baseModelDiagnostics(producerContext, modelID, maxCompletionTokens), err)
		return ProducerTurnOutput{}, fmt.Errorf("create ark chat model: %w", err)
	}

	messages := producerPromptMessages(producerContext)
	diagnostics := baseModelDiagnostics(producerContext, modelID, maxCompletionTokens)
	diagnostics["message_count"] = len(messages)
	diagnostics["image_attachment_count"] = len(producerContext.ImageAttachments)
	diagnostics["tool_binding_count"] = len(producerContext.ToolInfos)
	enrichReasoningPassbackDiagnostics(diagnostics, messages)
	streamer := model
	if len(producerContext.ToolInfos) > 0 {
		toolCallingModel, ok := model.(einoModel.ToolCallingChatModel)
		if !ok {
			return ProducerTurnOutput{}, NewAgentError("agent_model_tool_calling_unsupported", "selected Producer model does not support tool calling")
		}
		boundModel, err := toolCallingModel.WithTools(producerContext.ToolInfos)
		if err != nil {
			logProducerModelFailure(ctx, logger, "bind_tools", diagnostics, err)
			return ProducerTurnOutput{}, fmt.Errorf("bind producer tools: %w", err)
		}
		streamer = boundModel
	}
	stream, err := streamer.Stream(ctx, messages)
	if err != nil {
		logProducerModelFailure(ctx, logger, "stream_start", diagnostics, err)
		return ProducerTurnOutput{}, fmt.Errorf("stream ark chat model: %w", err)
	}
	defer stream.Close()

	chunks := []*schema.Message{}
	deltaIndex := int64(0)
	streamChunkCount := 0
	thinkingChunkCount := 0
	contentChunkCount := 0
	showThinking := uimessage.ShouldShowThinking(producerContext.Model.SupportsThinking, producerContext.Model.ReasoningEffort)
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			diagnostics["stream_chunk_count"] = streamChunkCount
			diagnostics["thinking_chunk_count"] = thinkingChunkCount
			diagnostics["content_chunk_count"] = contentChunkCount
			logProducerModelFailure(ctx, logger, "stream_receive", diagnostics, err)
			return ProducerTurnOutput{}, fmt.Errorf("receive ark chat stream: %w", err)
		}
		if chunk == nil {
			continue
		}
		streamChunkCount++
		if chunk.ReasoningContent != "" {
			thinkingChunkCount++
		}
		if chunk.Content != "" {
			contentChunkCount++
		}
		chunks = append(chunks, chunk)
		if producerContext.EmitDelta != nil {
			if showThinking && chunk.ReasoningContent != "" {
				deltaIndex++
				if err := producerContext.EmitDelta(ctx, ProducerStreamDelta{
					WorkspaceID: uuidString(producerContext.Input.WorkspaceID),
					ThreadID:    uuidString(producerContext.Input.ThreadID),
					TaskID:      uuidString(producerContext.Input.TaskID),
					BlockID:     uimessage.BlockIDThinking,
					BlockType:   "thinking",
					Kind:        "thinking",
					Delta:       chunk.ReasoningContent,
					Index:       int(deltaIndex),
					Sequence:    deltaIndex,
				}); err != nil {
					return ProducerTurnOutput{}, fmt.Errorf("emit producer stream delta: %w", err)
				}
			}
			if chunk.Content != "" {
				deltaIndex++
				if err := producerContext.EmitDelta(ctx, ProducerStreamDelta{
					WorkspaceID: uuidString(producerContext.Input.WorkspaceID),
					ThreadID:    uuidString(producerContext.Input.ThreadID),
					TaskID:      uuidString(producerContext.Input.TaskID),
					BlockID:     uimessage.BlockIDAnswer,
					BlockType:   "markdown",
					Kind:        "content",
					Delta:       chunk.Content,
					Index:       int(deltaIndex),
					Sequence:    deltaIndex,
				}); err != nil {
					return ProducerTurnOutput{}, fmt.Errorf("emit producer stream delta: %w", err)
				}
			}
		}
	}

	final, err := schema.ConcatMessages(chunks)
	if err != nil {
		diagnostics["stream_chunk_count"] = streamChunkCount
		diagnostics["thinking_chunk_count"] = thinkingChunkCount
		diagnostics["content_chunk_count"] = contentChunkCount
		logProducerModelFailure(ctx, logger, "stream_concat", diagnostics, err)
		return ProducerTurnOutput{}, fmt.Errorf("concatenate ark chat stream: %w", err)
	}
	enrichModelDiagnostics(diagnostics, final, streamChunkCount, thinkingChunkCount, contentChunkCount)
	metadata := map[string]any{
		"provider":         "volcengine",
		"model_id":         modelID,
		"streaming":        true,
		"visible_thinking": showThinking,
		"diagnostics":      diagnostics,
	}
	if displayName := strings.TrimSpace(producerContext.Model.DisplayName); displayName != "" {
		metadata["model_display_name"] = displayName
	}
	if effort := strings.TrimSpace(producerContext.Model.ReasoningEffort); effort != "" {
		metadata["reasoning_effort"] = effort
	}
	if reasoningContent := strings.TrimSpace(final.ReasoningContent); reasoningContent != "" {
		metadata["reasoning_content"] = reasoningContent
	}
	if requestID := ark.GetArkRequestID(final); requestID != "" {
		metadata["request_id"] = requestID
	}
	if final.ResponseMeta != nil {
		metadata["finish_reason"] = final.ResponseMeta.FinishReason
		if final.ResponseMeta.Usage != nil {
			metadata["usage"] = final.ResponseMeta.Usage
		}
	}
	diagnostics["native_tool_call_count"] = len(final.ToolCalls)
	metadata["native_tool_call_count"] = len(final.ToolCalls)
	if strings.TrimSpace(final.Content) == "" {
		logProducerModelEmptyContent(ctx, logger, diagnostics)
	} else {
		logProducerModelCompleted(ctx, logger, diagnostics)
	}
	return ProducerTurnOutput{
		AssistantText: strings.TrimSpace(final.Content),
		Metadata:      metadata,
		ModelMessage:  final,
	}, nil
}

func (r VolcengineModelResponder) logger() *slog.Logger {
	if r.cfg.Logger != nil {
		return r.cfg.Logger
	}
	return slog.Default()
}

type volcenginePolicyResult struct {
	MaxCompletionTokens int
	Thinking            *arkModel.Thinking
	ReasoningEffort     *arkModel.ReasoningEffort
}

func volcengineThinkingPolicy(model ProducerModelSelection, fallbackMaxTokens int) volcenginePolicyResult {
	maxCompletionTokens := model.MaxCompletionTokens
	if maxCompletionTokens <= 0 {
		maxCompletionTokens = fallbackMaxTokens
	}
	result := volcenginePolicyResult{MaxCompletionTokens: maxCompletionTokens}
	switch strings.TrimSpace(model.ReasoningEffort) {
	case "", "minimal":
		result.Thinking = &arkModel.Thinking{Type: arkModel.ThinkingTypeDisabled}
	case "low", "medium", "high":
		result.Thinking = &arkModel.Thinking{Type: arkModel.ThinkingTypeEnabled}
		if effort, ok := arkReasoningEffort(model.ReasoningEffort); ok {
			result.ReasoningEffort = &effort
		}
	}
	return result
}

func arkReasoningEffort(value string) (arkModel.ReasoningEffort, bool) {
	switch strings.TrimSpace(value) {
	case "low":
		return arkModel.ReasoningEffortLow, true
	case "medium":
		return arkModel.ReasoningEffortMedium, true
	case "high":
		return arkModel.ReasoningEffortHigh, true
	default:
		return "", false
	}
}

func baseModelDiagnostics(producerContext ProducerContext, modelID string, maxCompletionTokens int) map[string]any {
	return map[string]any{
		"provider":                 "volcengine",
		"model_id":                 modelID,
		"reasoning_history_policy": "normal_omit_same_turn_tool_resume",
		"workspace_id":             uuidString(producerContext.Input.WorkspaceID),
		"thread_id":                uuidString(producerContext.Input.ThreadID),
		"task_id":                  uuidString(producerContext.Input.TaskID),
		"trigger_message_id":       uuidString(producerContext.Input.TriggerMessageID),
		"reasoning_effort":         strings.TrimSpace(producerContext.Model.ReasoningEffort),
		"supports_thinking":        producerContext.Model.SupportsThinking,
		"max_completion_tokens":    maxCompletionTokens,
	}
}

func enrichModelDiagnostics(diagnostics map[string]any, final *schema.Message, streamChunkCount, thinkingChunkCount, contentChunkCount int) {
	diagnostics["stream_chunk_count"] = streamChunkCount
	diagnostics["thinking_chunk_count"] = thinkingChunkCount
	diagnostics["content_chunk_count"] = contentChunkCount
	diagnostics["reasoning_chars"] = utf8.RuneCountInString(final.ReasoningContent)
	diagnostics["content_chars"] = utf8.RuneCountInString(final.Content)
	if requestID := ark.GetArkRequestID(final); requestID != "" {
		diagnostics["request_id"] = requestID
	}
	if final.ResponseMeta == nil {
		return
	}
	diagnostics["finish_reason"] = final.ResponseMeta.FinishReason
	if final.ResponseMeta.Usage == nil {
		return
	}
	usage := final.ResponseMeta.Usage
	diagnostics["prompt_tokens"] = usage.PromptTokens
	diagnostics["cached_tokens"] = usage.PromptTokenDetails.CachedTokens
	diagnostics["completion_tokens"] = usage.CompletionTokens
	diagnostics["total_tokens"] = usage.TotalTokens
	diagnostics["reasoning_tokens"] = usage.CompletionTokensDetails.ReasoningTokens
}

func enrichReasoningPassbackDiagnostics(diagnostics map[string]any, messages []*schema.Message) {
	count := 0
	chars := 0
	for _, msg := range messages {
		if msg == nil || strings.TrimSpace(msg.ReasoningContent) == "" {
			continue
		}
		count++
		chars += utf8.RuneCountInString(msg.ReasoningContent)
	}
	diagnostics["reasoning_passback_enabled"] = count > 0
	diagnostics["reasoning_passback_messages"] = count
	diagnostics["reasoning_passback_chars"] = chars
}

func logProducerModelFailure(ctx context.Context, logger *slog.Logger, stage string, diagnostics map[string]any, cause error) {
	values := diagnosticsLogValues(diagnostics)
	values = append(values, "stage", stage, "error", cause)
	logger.ErrorContext(ctx, "producer model response failed", values...)
}

func logProducerModelEmptyContent(ctx context.Context, logger *slog.Logger, diagnostics map[string]any) {
	logger.WarnContext(ctx, "producer model response empty content", diagnosticsLogValues(diagnostics)...)
}

func logProducerModelCompleted(ctx context.Context, logger *slog.Logger, diagnostics map[string]any) {
	logger.InfoContext(ctx, "producer model response completed", diagnosticsLogValues(diagnostics)...)
}

func diagnosticsLogValues(diagnostics map[string]any) []any {
	keys := []string{
		"provider",
		"model_id",
		"workspace_id",
		"thread_id",
		"task_id",
		"trigger_message_id",
		"reasoning_effort",
		"reasoning_history_policy",
		"supports_thinking",
		"max_completion_tokens",
		"message_count",
		"image_attachment_count",
		"tool_binding_count",
		"native_tool_call_count",
		"stream_chunk_count",
		"thinking_chunk_count",
		"content_chunk_count",
		"reasoning_chars",
		"content_chars",
		"finish_reason",
		"request_id",
		"prompt_tokens",
		"cached_tokens",
		"completion_tokens",
		"total_tokens",
		"reasoning_tokens",
		"reasoning_passback_enabled",
		"reasoning_passback_messages",
		"reasoning_passback_chars",
	}
	values := make([]any, 0, len(keys)*2)
	for _, key := range keys {
		value, ok := diagnostics[key]
		if !ok {
			continue
		}
		values = append(values, key, value)
	}
	return values
}

func producerPromptMessages(producerContext ProducerContext) []*schema.Message {
	systemPrompt := strings.TrimSpace(`你是 ClipAnvil 的 Producer Agent。
你的职责是理解用户的视频创作需求，维护 storyboard，并推动后续 Craftsman / Worker / Composer 阶段。
当前 M6.5 阶段只能通过工具保存 storyboard 和读取生产状态；不要声称已经生成图片、视频、评审或成片。
需要持久化分镜时必须调用 update_storyboard；需要确认时调用 request_user_decision；需要刷新项目状态时调用 get_production_state。
每次回复最多调用一个工具；不要把多个 FunctionCall 拼在同一条回复里。
推荐工具调用格式：{"tool_call":{"name":"update_storyboard","arguments":{"intent":"replace","shots":[{"client_key":"shot-01","sort_order":1,"title":"开场","duration_sec":4,"brief":{"summary":"画面内容","voice_over":"口播","ui_overlay":"字幕或贴片"}}]}}}。
回答使用中文，简洁、具体，并在工具成功后总结已保存的 shot。`)
	if pss := strings.TrimSpace(producerContext.ProductionStateText); pss != "" {
		systemPrompt += "\n\n当前 Production State Summary (PSS):\n" + pss
	}
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
	}
	for _, msg := range producerContext.Messages {
		text := agentMessageText(msg.Content)
		if text == "" {
			continue
		}
		switch msg.Role {
		case "user":
			messages = append(messages, userPromptMessage(msg.Content, text, producerContext.ImageAttachments))
		case "assistant":
			if msg.MessageType == "text" {
				messages = append(messages, schema.AssistantMessage(text, nil))
			}
		}
	}
	for _, msg := range producerContext.SameTurnMessages {
		next := sameTurnPromptMessage(msg)
		if next != nil {
			messages = append(messages, next)
		}
	}
	if len(messages) == 1 {
		text := strings.TrimSpace(producerContext.LatestUserText)
		if text == "" {
			text = "请开始一次 Producer 对话。"
		}
		messages = append(messages, schema.UserMessage(text))
	}
	return messages
}

func sameTurnPromptMessage(msg ProducerSameTurnMessage) *schema.Message {
	content := strings.TrimSpace(msg.Content)
	switch strings.TrimSpace(msg.Role) {
	case "assistant":
		if content == "" && strings.TrimSpace(msg.ReasoningContent) == "" && strings.TrimSpace(msg.ToolCallID) == "" {
			return nil
		}
		assistant := schema.AssistantMessage(content, sameTurnToolCalls(msg))
		assistant.ReasoningContent = strings.TrimSpace(msg.ReasoningContent)
		return assistant
	case "tool":
		if content == "" {
			return nil
		}
		toolCallID := strings.TrimSpace(msg.ToolCallID)
		if toolCallID == "" {
			return schema.UserMessage("工具返回：" + content)
		}
		return schema.ToolMessage(content, toolCallID, schema.WithToolName(strings.TrimSpace(msg.ToolName)))
	default:
		return nil
	}
}

func sameTurnToolCalls(msg ProducerSameTurnMessage) []schema.ToolCall {
	toolCallID := strings.TrimSpace(msg.ToolCallID)
	toolName := strings.TrimSpace(msg.ToolName)
	if toolCallID == "" || toolName == "" {
		return nil
	}
	arguments := "{}"
	if msg.ToolArguments != nil {
		if raw, err := json.Marshal(msg.ToolArguments); err == nil {
			arguments = string(raw)
		}
	}
	return []schema.ToolCall{{
		ID:   toolCallID,
		Type: "function",
		Function: schema.FunctionCall{
			Name:      toolName,
			Arguments: arguments,
		},
	}}
}

func userPromptMessage(raw []byte, text string, images map[string]ProducerImageAttachment) *schema.Message {
	parts := []schema.MessageInputPart{}
	for _, attachment := range uimessage.ExtractAttachments(raw) {
		if strings.TrimSpace(attachment.Kind) != "image" {
			continue
		}
		image, ok := images[strings.TrimSpace(attachment.AssetID)]
		if !ok || strings.TrimSpace(image.URL) == "" {
			continue
		}
		if len(parts) == 0 {
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeText,
				Text: text,
			})
		}
		url := strings.TrimSpace(image.URL)
		parts = append(parts, schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{MessagePartCommon: schema.MessagePartCommon{
				URL:      &url,
				MIMEType: strings.TrimSpace(image.Mime),
			}},
		})
	}
	if len(parts) == 0 {
		return schema.UserMessage(text)
	}
	return &schema.Message{
		Role:                  schema.User,
		UserInputMultiContent: parts,
	}
}

func agentMessageText(raw []byte) string {
	text := strings.TrimSpace(strings.Join(uimessage.ExtractMarkdownTexts(raw), "\n\n"))
	attachments := uimessage.ExtractAttachments(raw)
	if len(attachments) == 0 {
		return text
	}
	lines := []string{text, "用户附加素材："}
	for _, attachment := range attachments {
		kind := strings.TrimSpace(attachment.Kind)
		name := strings.TrimSpace(attachment.Name)
		if kind == "" || name == "" {
			continue
		}
		lines = append(lines, "- "+kind+": "+name)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
