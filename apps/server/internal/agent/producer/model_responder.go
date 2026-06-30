package producer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"
	arkModel "github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	agentprompt "github.com/sinmaystar/clip-anvil/internal/agent/prompt"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type arkChatStreamer interface {
	Stream(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.StreamReader[*schema.Message], error)
}

func durationPtr(value time.Duration) *time.Duration {
	return &value
}

type arkChatModelFactory func(ctx context.Context, config *ark.ChatModelConfig) (arkChatStreamer, error)

type VolcengineModelResponderConfig struct {
	APIKey           string
	BaseURL          string
	Region           string
	Model            string
	MaxTokens        int
	Temperature      float32
	Factory          arkChatModelFactory
	Logger           *slog.Logger
	ContextCompactor contextcompact.Middleware
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

	diagnostics := baseModelDiagnostics(producerContext, modelID, maxCompletionTokens)
	prompt := producerPromptMessagesWithBoundaries(producerContext)
	messages := prompt.Messages
	facts, mediaCards := producerContextCompactionFacts(producerContext)
	var compacted contextcompact.ProjectionOutput
	if r.cfg.ContextCompactor != nil {
		compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
			WorkspaceID:       producerContext.Input.WorkspaceID,
			ThreadID:          producerContext.Input.ThreadID,
			TaskID:            producerContext.Input.TaskID,
			Role:              "producer",
			ModelID:           modelID,
			Messages:          messages,
			MessageRefs:       prompt.MessageRefs,
			ToolInfos:         producerContext.ToolInfos,
			MediaCards:        mediaCards,
			Facts:             facts,
			Trigger:           "producer_before_model",
			SameTurnFromIndex: prompt.SameTurnFromIndex,
			PendingFromIndex:  prompt.PendingFromIndex,
		})
		if err != nil {
			logProducerModelFailure(ctx, logger, "context_compaction", diagnostics, err)
			return ProducerTurnOutput{}, fmt.Errorf("compact producer context: %w", err)
		}
		messages = compacted.Messages
		enrichContextCompactionDiagnostics(diagnostics, compacted)
	}
	diagnostics["message_count"] = len(messages)
	diagnostics["image_attachment_count"] = len(producerContext.ImageAttachments)
	diagnostics["tool_binding_count"] = len(producerContext.ToolInfos)
	enrichReasoningPassbackDiagnostics(diagnostics, messages)
	logProducerModelInputIfEnabled(ctx, logger, diagnostics, messages, producerContext.ToolInfos)
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
	retriedContextOverflow := false
	stream, err := streamer.Stream(ctx, messages)
	if err != nil && contextcompact.IsContextOverflowError(err) && r.cfg.ContextCompactor != nil {
		retriedContextOverflow = true
		compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
			WorkspaceID:       producerContext.Input.WorkspaceID,
			ThreadID:          producerContext.Input.ThreadID,
			TaskID:            producerContext.Input.TaskID,
			Role:              "producer",
			ModelID:           modelID,
			Messages:          prompt.Messages,
			MessageRefs:       prompt.MessageRefs,
			ToolInfos:         producerContext.ToolInfos,
			MediaCards:        mediaCards,
			Facts:             facts,
			Trigger:           "model_error_context_overflow",
			SameTurnFromIndex: prompt.SameTurnFromIndex,
			PendingFromIndex:  prompt.PendingFromIndex,
			ForceFullCompact:  true,
		})
		if err != nil {
			logProducerModelFailure(ctx, logger, "context_compaction_retry", diagnostics, err)
			return ProducerTurnOutput{}, fmt.Errorf("compact producer context after overflow: %w", err)
		}
		messages = compacted.Messages
		enrichContextCompactionDiagnostics(diagnostics, compacted)
		stream, err = streamer.Stream(ctx, messages)
	}
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
	enrichContextCompactionMetadata(metadata, compacted)
	if retriedContextOverflow {
		metadata["context_compaction_retry"] = true
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
	if strings.TrimSpace(final.Content) == "" && len(final.ToolCalls) == 0 {
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

func enrichContextCompactionDiagnostics(diagnostics map[string]any, output contextcompact.ProjectionOutput) {
	if len(output.Applied) == 0 {
		return
	}
	diagnostics["context_compaction_applied"] = true
	diagnostics["context_compaction_mode"] = output.CompactionMode
	diagnostics["context_compaction_count"] = len(output.Applied)
	diagnostics["context_compaction_token_before"] = output.TokenBefore
	diagnostics["context_compaction_token_after"] = output.TokenAfter
	diagnostics["context_compaction_refs"] = output.CompactionRefs
	diagnostics["context_compaction_detail_files"] = output.DetailFiles
}

func enrichContextCompactionMetadata(metadata map[string]any, output contextcompact.ProjectionOutput) {
	if len(output.Applied) == 0 {
		return
	}
	metadata["context_compaction_applied"] = true
	metadata["context_compaction_mode"] = output.CompactionMode
	metadata["context_compaction_count"] = len(output.Applied)
	metadata["context_compaction_refs"] = output.CompactionRefs
	metadata["context_compaction_detail_files"] = output.DetailFiles
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

func logProducerModelInputIfEnabled(ctx context.Context, logger *slog.Logger, diagnostics map[string]any, messages []*schema.Message, tools []*schema.ToolInfo) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CLIPANVIL_AGENT_LOG_MODEL_INPUT")))
	if value != "1" && value != "true" && value != "yes" {
		return
	}
	payload := map[string]any{
		"diagnostics": diagnostics,
		"messages":    producerModelInputMessageDiagnostics(messages),
		"tools":       producerModelInputToolDiagnostics(tools),
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		logger.WarnContext(ctx, "producer model input diagnostic marshal failed", "error", err)
		return
	}
	logger.WarnContext(ctx, "producer model input diagnostic", "payload", string(raw))
}

func producerModelInputMessageDiagnostics(messages []*schema.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for index, message := range messages {
		if message == nil {
			out = append(out, map[string]any{"index": index, "nil": true})
			continue
		}
		item := map[string]any{
			"index":                          index,
			"role":                           string(message.Role),
			"name":                           message.Name,
			"content":                        message.Content,
			"content_chars":                  utf8.RuneCountInString(message.Content),
			"reasoning_content_chars":        utf8.RuneCountInString(message.ReasoningContent),
			"tool_call_id":                   message.ToolCallID,
			"tool_name":                      message.ToolName,
			"tool_calls":                     message.ToolCalls,
			"user_input_multi_content":       message.UserInputMultiContent,
			"user_input_multi_content_count": len(message.UserInputMultiContent),
		}
		out = append(out, item)
	}
	return out
}

func producerModelInputToolDiagnostics(tools []*schema.ToolInfo) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for index, tool := range tools {
		if tool == nil {
			out = append(out, map[string]any{"index": index, "nil": true})
			continue
		}
		item := map[string]any{
			"index": index,
			"name":  tool.Name,
			"desc":  tool.Desc,
			"extra": tool.Extra,
		}
		if tool.ParamsOneOf != nil {
			item["params_one_of"] = tool.ParamsOneOf
		}
		out = append(out, item)
	}
	return out
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

type producerPromptBoundary struct {
	Messages          []*schema.Message
	MessageRefs       []contextcompact.SourceMessageRef
	SameTurnFromIndex int
	PendingFromIndex  int
}

func producerPromptMessages(producerContext ProducerContext) []*schema.Message {
	return producerPromptMessagesWithBoundaries(producerContext).Messages
}

func producerPromptMessagesWithBoundaries(producerContext ProducerContext) producerPromptBoundary {
	systemPrompt := ProducerSystemPrompt(producerContext)
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
	}
	messageRefs := make([]contextcompact.SourceMessageRef, 0, len(producerContext.Messages))
	for _, msg := range producerContext.Messages {
		switch msg.Role {
		case "user":
			text := agentMessageText(msg.Content)
			if text == "" {
				continue
			}
			messages = append(messages, userPromptMessage(msg.Content, text, producerContext.ImageAttachments))
			messageRefs = appendPromptMessageRef(messageRefs, len(messages)-1, msg.ID)
		case "assistant":
			if msg.MessageType == "text" {
				text := agentMessageText(msg.Content)
				if text == "" {
					continue
				}
				messages = append(messages, schema.AssistantMessage(text, nil))
				messageRefs = appendPromptMessageRef(messageRefs, len(messages)-1, msg.ID)
			} else if msg.MessageType == "tool_call" {
				if toolMessage := historicalToolCallPromptMessage(msg); toolMessage != nil {
					messages = append(messages, toolMessage)
					messageRefs = appendPromptMessageRef(messageRefs, len(messages)-1, msg.ID)
				}
			}
		case "tool":
			if msg.MessageType == "tool_result" {
				if toolMessage := historicalToolResultPromptMessage(msg); toolMessage != nil {
					messages = append(messages, toolMessage)
					messageRefs = appendPromptMessageRef(messageRefs, len(messages)-1, msg.ID)
				}
			}
		}
	}
	sameTurnFromIndex := 0
	if len(producerContext.SameTurnMessages) > 0 {
		sameTurnFromIndex = len(messages)
	}
	for _, msg := range producerContext.SameTurnMessages {
		next := sameTurnPromptMessage(msg)
		if next != nil {
			messages = append(messages, next)
		}
	}
	if trigger := strings.TrimSpace(producerContext.RuntimeTriggerText); trigger != "" && !hasSameTurnToolExchange(producerContext.SameTurnMessages) {
		messages = append(messages, schema.UserMessage(runtimeTriggerPromptText(trigger)))
	}
	if !hasNonSystemPromptMessage(messages) {
		text := strings.TrimSpace(producerContext.LatestUserText)
		if text == "" {
			text = "请开始一次 Producer 对话。"
		}
		messages = append(messages, schema.UserMessage(text))
	}
	pendingFromIndex := contextcompact.PendingReminderTargetIndex(messages, producerContext.PendingReminders)
	messages = agentprompt.AppendPendingReminders(messages, producerContext.PendingReminders)
	return producerPromptBoundary{Messages: messages, MessageRefs: messageRefs, SameTurnFromIndex: sameTurnFromIndex, PendingFromIndex: pendingFromIndex}
}

func appendPromptMessageRef(refs []contextcompact.SourceMessageRef, index int, id pgtype.UUID) []contextcompact.SourceMessageRef {
	if !id.Valid {
		return refs
	}
	return append(refs, contextcompact.SourceMessageRef{MessageIndex: index, MessageID: id})
}

func hasSameTurnToolExchange(messages []ProducerSameTurnMessage) bool {
	for _, message := range messages {
		switch strings.TrimSpace(message.MessageType) {
		case "tool_call", "tool_result":
			return true
		}
	}
	return false
}

func hasNonSystemPromptMessage(messages []*schema.Message) bool {
	for _, message := range messages {
		if message != nil && message.Role != schema.System {
			return true
		}
	}
	return false
}

func runtimeTriggerPromptText(trigger string) string {
	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		return ""
	}
	return "请根据以下系统触发事件继续推进 Producer 工作。你需要读取项目上下文，确认真实状态，再决定调用 decide_render_plan、dispatch_reviewer 或 request_user_decision；如果存在多条 craftsman_render_plan_ready RenderPlan，请优先用 decide_render_plan 的 decisions 批量参数一次处理每条决策。\n\n" + trigger
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

func historicalToolCallPromptMessage(msg db.AgentMessage) *schema.Message {
	raw := jsonObject(msg.RawMessage)
	toolCallID := stringFromAny(raw["tool_call_id"])
	toolName := firstNonEmpty(stringFromAny(raw["tool_name"]), stringFromAny(raw["name"]))
	if toolCallID == "" || toolName == "" {
		return nil
	}
	arguments := "{}"
	if rawArgs, ok := raw["arguments"]; ok {
		if encoded, err := json.Marshal(rawArgs); err == nil {
			arguments = string(encoded)
		}
	}
	return schema.AssistantMessage(historicalToolText(msg), []schema.ToolCall{{
		ID:   toolCallID,
		Type: "function",
		Function: schema.FunctionCall{
			Name:      toolName,
			Arguments: arguments,
		},
	}})
}

func historicalToolResultPromptMessage(msg db.AgentMessage) *schema.Message {
	raw := jsonObject(msg.RawMessage)
	toolCallID := stringFromAny(raw["tool_call_id"])
	toolName := firstNonEmpty(stringFromAny(raw["tool_name"]), stringFromAny(raw["name"]))
	content := firstNonEmpty(stringFromAny(raw["result_text"]), historicalToolText(msg))
	if content == "" {
		if rawResult, ok := raw["result"]; ok {
			if encoded, err := json.Marshal(rawResult); err == nil {
				content = string(encoded)
			}
		}
	}
	if content == "" {
		return nil
	}
	if toolCallID == "" {
		return schema.UserMessage("工具返回：" + content)
	}
	return schema.ToolMessage(content, toolCallID, schema.WithToolName(toolName))
}

func historicalToolText(msg db.AgentMessage) string {
	if text := strings.TrimSpace(agentMessageText(msg.Content)); text != "" {
		return text
	}
	content := jsonObject(msg.Content)
	return strings.TrimSpace(stringFromAny(content["text"]))
}

func jsonObject(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
