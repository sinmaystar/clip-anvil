package producer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/compose"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/attribute"

	"github.com/sinmaystar/clip-anvil/internal/agent/cozelooptrace"
	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Runtime interface {
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
	MarkTaskWaitingForUser(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	SetThreadCheckpoint(ctx context.Context, threadID pgtype.UUID, checkpointKey string) (db.AgentThread, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
	UpdateMessage(ctx context.Context, params agentruntime.UpdateMessageParams) (db.AgentMessage, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type producerSignalReleaser interface {
	ReleaseProducerPendingSignalsForTask(ctx context.Context, workspaceID, taskID pgtype.UUID, reason string) ([]db.ProducerPendingSignal, error)
}

type Runner interface {
	Run(ctx context.Context, input ProducerTurnInput, options ...agenteino.RunOptions) (ProducerTurnOutput, error)
}

type Broadcaster interface {
	BroadcastAgentMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent)
	BroadcastAgentMessageUpdated(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent)
	BroadcastAgentTask(workspaceID pgtype.UUID, task db.AgentTask)
	BroadcastAgentEvent(workspaceID pgtype.UUID, event db.AgentEvent)
	BroadcastAgentMessageDelta(workspaceID pgtype.UUID, delta ProducerStreamDelta)
}

type ExecutorConfig struct {
	Runtime        Runtime
	Graph          Runner
	Broadcaster    Broadcaster
	MaxToolCalls   int
	ToolTimeout    time.Duration
	Logger         *slog.Logger
	TraceCallbacks []callbacks.Handler
}

type Executor struct {
	runtime        Runtime
	graph          Runner
	broadcaster    Broadcaster
	maxToolCalls   int
	toolTimeout    time.Duration
	logger         *slog.Logger
	traceCallbacks []callbacks.Handler
}

type RunTaskInput struct {
	WorkspaceID        pgtype.UUID
	ThreadID           pgtype.UUID
	TaskID             pgtype.UUID
	TriggerMessageID   pgtype.UUID
	TriggerMessageSeq  int64
	RuntimeTriggerText string
	ResumeCheckpointID string
	ResumeData         map[string]any
	OriginalTaskID     pgtype.UUID
}

func NewExecutor(config ExecutorConfig) *Executor {
	maxToolCalls := config.MaxToolCalls
	if maxToolCalls <= 0 {
		maxToolCalls = defaultProducerMaxToolCalls
	}
	toolTimeout := config.ToolTimeout
	if toolTimeout <= 0 {
		toolTimeout = 300 * time.Second
	}
	return &Executor{
		runtime:        config.Runtime,
		graph:          config.Graph,
		broadcaster:    config.Broadcaster,
		maxToolCalls:   maxToolCalls,
		toolTimeout:    toolTimeout,
		logger:         config.Logger,
		traceCallbacks: config.TraceCallbacks,
	}
}

func (e *Executor) RunTask(ctx context.Context, input RunTaskInput) error {
	if !input.WorkspaceID.Valid || !input.ThreadID.Valid || !input.TaskID.Valid {
		return errors.New("invalid producer task input")
	}
	runningTask, err := e.runtime.MarkTaskRunning(ctx, input.TaskID)
	if err != nil {
		return err
	}
	applyProducerTaskTriggerInput(&input, runningTask.Input)
	e.broadcastTask(input.WorkspaceID, runningTask)

	started, err := e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "producer_turn_started",
		SourceRole:  "producer",
		TargetRole:  "user",
		Payload:     mustJSON(map[string]string{"task_id": uuidString(input.TaskID)}),
	})
	if err == nil {
		e.broadcastEvent(input.WorkspaceID, started)
	}

	graphInput := ProducerTurnInput{
		WorkspaceID:        input.WorkspaceID,
		ThreadID:           input.ThreadID,
		TaskID:             input.TaskID,
		TriggerMessageID:   input.TriggerMessageID,
		TriggerMessageSeq:  input.TriggerMessageSeq,
		RuntimeTriggerText: input.RuntimeTriggerText,
		MaxToolCalls:       e.maxToolCalls,
		ToolTimeout:        e.toolTimeout,
	}
	graphInput.EmitDelta = func(ctx context.Context, delta ProducerStreamDelta) error {
		if delta.Kind == "" {
			delta.Kind = "content"
		}
		if delta.WorkspaceID == "" {
			delta.WorkspaceID = uuidString(input.WorkspaceID)
		}
		if delta.ThreadID == "" {
			delta.ThreadID = uuidString(input.ThreadID)
		}
		if delta.TaskID == "" {
			delta.TaskID = uuidString(input.TaskID)
		}
		e.broadcastMessageDelta(input.WorkspaceID, delta)
		return nil
	}
	checkpointKey := agenteino.CheckpointKey("producer_turn", input.WorkspaceID, input.ThreadID, input.TaskID)
	if strings.TrimSpace(input.ResumeCheckpointID) != "" {
		checkpointKey = strings.TrimSpace(input.ResumeCheckpointID)
	}
	ctx = cozelooptrace.ContextWithAttributes(ctx,
		attribute.String("clipanvil.workspace_id", uuidString(input.WorkspaceID)),
		attribute.String("clipanvil.agent.thread_id", uuidString(input.ThreadID)),
		attribute.String("clipanvil.agent.task_id", uuidString(input.TaskID)),
		attribute.String("clipanvil.agent.role", "producer"),
		attribute.String("clipanvil.agent.task_type", "producer_turn"),
	)
	liveToolTrace := newProducerLiveToolTrace(e, input)
	ctx = agenttools.WithNativeToolTraceSink(ctx, liveToolTrace)
	output, err := e.graph.Run(ctx, graphInput, agenteino.RunOptions{
		CheckPointID: checkpointKey,
		ResumeData:   input.ResumeData,
		Callbacks:    e.traceCallbacks,
	})
	if err != nil {
		if interruptInfo, ok := compose.ExtractInterruptInfo(err); ok {
			return e.interruptTask(ctx, input, checkpointKey, interruptInfo)
		}
		return e.failTask(ctx, input, errorCode(err, "producer_turn_failed"), err.Error())
	}
	if liveToolTrace.count() == 0 {
		if err := e.persistNativeToolTrace(ctx, input, output.SameTurnMessages); err != nil {
			return e.failTask(ctx, input, "producer_tool_trace_persist_failed", err.Error())
		}
	}

	content, err := uimessage.BuildAssistantMessageContent(uimessage.AssistantMessageInput{
		Text:             output.AssistantText,
		ReasoningContent: stringFromMap(output.Metadata, "reasoning_content"),
		IncludeThinking:  boolFromMap(output.Metadata, "visible_thinking"),
		DefaultCollapsed: true,
	})
	if err != nil {
		return e.failTask(ctx, input, "producer_message_build_failed", err.Error())
	}

	msg, err := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        "assistant",
		MessageType: "text",
		Content:     content,
		RawMessage:  mustJSON(output.Metadata),
		TaskID:      input.TaskID,
	})
	if err != nil {
		return e.failTask(ctx, input, "producer_message_persist_failed", err.Error())
	}

	completed, err := e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "producer_turn_completed",
		SourceRole:  "producer",
		TargetRole:  "user",
		Payload:     mustJSON(map[string]string{"message_id": uuidString(msg.ID)}),
	})
	if err != nil {
		return e.failTask(ctx, input, "producer_event_persist_failed", err.Error())
	}

	if _, err := e.runtime.SetThreadCheckpoint(ctx, input.ThreadID, checkpointKey); err != nil {
		return e.failTask(ctx, input, "producer_checkpoint_update_failed", err.Error())
	}

	succeededTask, err := e.runtime.MarkTaskSucceeded(ctx, input.TaskID, mustJSON(output.Metadata))
	if err != nil {
		return err
	}
	if input.OriginalTaskID.Valid && input.OriginalTaskID != input.TaskID {
		_, _ = e.runtime.MarkTaskSucceeded(ctx, input.OriginalTaskID, mustJSON(map[string]any{
			"resumed_by_task_id": uuidString(input.TaskID),
			"checkpoint_key":     checkpointKey,
		}))
	}

	e.broadcastMessage(input.WorkspaceID, msg, completed)
	e.broadcastEvent(input.WorkspaceID, completed)
	e.broadcastTask(input.WorkspaceID, succeededTask)
	return nil
}

func (e *Executor) persistNativeToolTrace(ctx context.Context, input RunTaskInput, messages []ProducerSameTurnMessage) error {
	callMessages := map[string]db.AgentMessage{}
	for _, trace := range messages {
		role := strings.TrimSpace(trace.Role)
		messageType := strings.TrimSpace(trace.MessageType)
		if messageType != "tool_call" && messageType != "tool_result" {
			continue
		}
		if role == "" {
			if messageType == "tool_result" {
				role = "tool"
			} else {
				role = "assistant"
			}
		}
		content := nativeToolTraceContent(trace)
		raw := nativeToolTraceRaw(trace)
		msg, err := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
			WorkspaceID: input.WorkspaceID,
			ThreadID:    input.ThreadID,
			Role:        role,
			MessageType: messageType,
			Content:     content,
			RawMessage:  raw,
			TaskID:      input.TaskID,
		})
		if err != nil {
			return err
		}
		e.broadcastMessage(input.WorkspaceID, msg, db.AgentEvent{})
		if messageType == "tool_call" {
			callMessages[trace.ToolCallID] = msg
		}
		if messageType == "tool_result" {
			if callMsg, ok := callMessages[trace.ToolCallID]; ok {
				updated, err := e.runtime.UpdateMessage(ctx, agentruntime.UpdateMessageParams{
					ID:         callMsg.ID,
					Content:    completedToolTraceContent(trace, callMsg.RawMessage, "succeeded"),
					RawMessage: completedToolTraceRaw(trace, callMsg.RawMessage),
				})
				if err != nil {
					return err
				}
				e.broadcastMessageUpdated(input.WorkspaceID, updated, db.AgentEvent{})
			}
		}
	}
	return nil
}

type producerLiveToolTrace struct {
	executor     *Executor
	input        RunTaskInput
	callMessages map[string]db.AgentMessage
	startedCount int
}

func newProducerLiveToolTrace(executor *Executor, input RunTaskInput) *producerLiveToolTrace {
	return &producerLiveToolTrace{
		executor:     executor,
		input:        input,
		callMessages: map[string]db.AgentMessage{},
	}
}

func (t *producerLiveToolTrace) count() int {
	if t == nil {
		return 0
	}
	return t.startedCount
}

func (t *producerLiveToolTrace) NativeToolCallStarted(ctx context.Context, runtime agenttools.NativeRuntimeContext, trace agenttools.NativeToolTrace) error {
	if t == nil || t.executor == nil || t.executor.runtime == nil {
		return nil
	}
	toolCallID := strings.TrimSpace(runtime.ToolCallID)
	if toolCallID == "" {
		toolCallID = strings.TrimSpace(trace.ToolName)
	}
	sameTurn := ProducerSameTurnMessage{
		Role:          "assistant",
		MessageType:   "tool_call",
		ToolCallID:    toolCallID,
		ToolName:      trace.ToolName,
		ToolArguments: trace.Arguments,
	}
	msg, err := t.executor.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: t.input.WorkspaceID,
		ThreadID:    t.input.ThreadID,
		Role:        "assistant",
		MessageType: "tool_call",
		Content:     nativeToolTraceContent(sameTurn),
		RawMessage:  nativeToolTraceRaw(sameTurn),
		TaskID:      t.input.TaskID,
	})
	if err != nil {
		return err
	}
	t.callMessages[toolCallID] = msg
	t.startedCount++
	t.executor.broadcastMessage(t.input.WorkspaceID, msg, db.AgentEvent{})
	return nil
}

func (t *producerLiveToolTrace) NativeToolCallCompleted(ctx context.Context, runtime agenttools.NativeRuntimeContext, trace agenttools.NativeToolTrace) error {
	if t == nil || t.executor == nil || t.executor.runtime == nil {
		return nil
	}
	toolCallID := strings.TrimSpace(runtime.ToolCallID)
	resultText := strings.TrimSpace(trace.Result)
	status := "succeeded"
	if strings.TrimSpace(trace.Error) != "" {
		status = "failed"
		resultText = strings.TrimSpace(trace.Error)
	}
	sameTurn := ProducerSameTurnMessage{
		Role:        "tool",
		MessageType: "tool_result",
		Content:     resultText,
		ToolCallID:  toolCallID,
		ToolName:    trace.ToolName,
	}
	msg, err := t.executor.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: t.input.WorkspaceID,
		ThreadID:    t.input.ThreadID,
		Role:        "tool",
		MessageType: "tool_result",
		Content:     nativeToolTraceContent(sameTurn),
		RawMessage:  nativeToolTraceRaw(sameTurn),
		TaskID:      t.input.TaskID,
	})
	if err != nil {
		return err
	}
	t.executor.broadcastMessage(t.input.WorkspaceID, msg, db.AgentEvent{})
	if callMsg, ok := t.callMessages[toolCallID]; ok {
		updated, err := t.executor.runtime.UpdateMessage(ctx, agentruntime.UpdateMessageParams{
			ID:         callMsg.ID,
			Content:    completedToolTraceContent(sameTurn, callMsg.RawMessage, status),
			RawMessage: completedToolTraceRawWithStatus(sameTurn, callMsg.RawMessage, status),
		})
		if err != nil {
			return err
		}
		t.executor.broadcastMessageUpdated(t.input.WorkspaceID, updated, db.AgentEvent{})
	}
	return nil
}

func nativeToolTraceContent(trace ProducerSameTurnMessage) []byte {
	if trace.MessageType == "tool_call" {
		return toolStatusContent(trace.ToolCallID, trace.ToolName, trace.ToolName, "running", toolTraceSummary(trace.ToolArguments, trace.Content), "", trace.ToolArguments, nil)
	}
	return mustJSON(map[string]any{
		"schema":       "clipanvil.agent.tool_trace.v1",
		"message_type": "tool_result",
		"tool_call_id": trace.ToolCallID,
		"tool_name":    trace.ToolName,
		"text":         strings.TrimSpace(trace.Content),
	})
}

func completedToolTraceContent(trace ProducerSameTurnMessage, previousRaw []byte, status string) []byte {
	args := toolTraceArgumentsFromRaw(previousRaw)
	result := map[string]any{}
	if text := strings.TrimSpace(trace.Content); text != "" {
		result["text"] = text
	}
	return toolStatusContent(trace.ToolCallID, trace.ToolName, trace.ToolName, status, toolTraceSummary(args, trace.Content), "", args, result)
}

func completedToolTraceRaw(trace ProducerSameTurnMessage, previousRaw []byte) []byte {
	return completedToolTraceRawWithStatus(trace, previousRaw, "succeeded")
}

func completedToolTraceRawWithStatus(trace ProducerSameTurnMessage, previousRaw []byte, status string) []byte {
	raw := map[string]any{}
	_ = json.Unmarshal(defaultJSON(previousRaw), &raw)
	raw["result_text"] = strings.TrimSpace(trace.Content)
	raw["message_type"] = "tool_call"
	raw["status"] = status
	return mustJSON(raw)
}

func toolTraceArgumentsFromRaw(raw []byte) map[string]any {
	payload := map[string]any{}
	_ = json.Unmarshal(defaultJSON(raw), &payload)
	if args, ok := payload["arguments"].(map[string]any); ok {
		return args
	}
	return map[string]any{}
}

func toolTraceSummary(args map[string]any, fallback string) string {
	if args != nil {
		if brief, _ := args["brief"].(string); strings.TrimSpace(brief) != "" {
			return strings.TrimSpace(brief)
		}
	}
	return strings.TrimSpace(fallback)
}

func nativeToolTraceRaw(trace ProducerSameTurnMessage) []byte {
	raw := map[string]any{
		"schema":       "clipanvil.agent.tool_trace.v1",
		"role":         trace.Role,
		"message_type": trace.MessageType,
		"tool_call_id": trace.ToolCallID,
		"tool_name":    trace.ToolName,
	}
	if len(trace.ToolArguments) > 0 {
		raw["arguments"] = trace.ToolArguments
	}
	if text := strings.TrimSpace(trace.Content); text != "" {
		raw["result_text"] = text
	}
	if reasoning := strings.TrimSpace(trace.ReasoningContent); reasoning != "" {
		raw["reasoning_content"] = reasoning
	}
	return mustJSON(raw)
}

func toolStatusContent(toolCallID string, toolName string, label string, status string, summary string, errorMessage string, arguments map[string]any, result map[string]any) []byte {
	content, err := uimessage.BuildToolStatusMessageContent(uimessage.ToolStatusInput{
		ToolCallID:   toolCallID,
		ToolName:     toolName,
		Label:        label,
		Status:       status,
		Summary:      summary,
		ErrorMessage: errorMessage,
		Arguments:    arguments,
		Result:       result,
	})
	if err != nil {
		return mustJSON(map[string]any{
			"schema":       "clipanvil.agent.tool_trace.v1",
			"message_type": "tool_call",
			"tool_call_id": toolCallID,
			"tool_name":    toolName,
			"text":         summary,
		})
	}
	return content
}

func (e *Executor) interruptTask(ctx context.Context, input RunTaskInput, checkpointKey string, interruptInfo *compose.InterruptInfo) error {
	if _, err := e.runtime.SetThreadCheckpoint(ctx, input.ThreadID, checkpointKey); err != nil {
		return e.failTask(ctx, input, "producer_checkpoint_update_failed", err.Error())
	}
	event, err := e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "graph_interrupted",
		SourceRole:  "producer",
		TargetRole:  "user",
		Payload: mustJSON(map[string]any{
			"checkpoint_key":  checkpointKey,
			"interrupt_count": len(interruptInfo.InterruptContexts),
			"interrupt_ids":   interruptIDs(interruptInfo),
			"rerun_nodes":     interruptInfo.RerunNodes,
		}),
	})
	if err != nil {
		return e.failTask(ctx, input, "producer_interrupt_event_failed", err.Error())
	}
	waitingTask, err := e.runtime.MarkTaskWaitingForUser(ctx, input.TaskID)
	if err != nil {
		return e.failTask(ctx, input, "producer_waiting_state_failed", err.Error())
	}
	e.broadcastEvent(input.WorkspaceID, event)
	e.broadcastTask(input.WorkspaceID, waitingTask)
	return nil
}

func interruptIDs(info *compose.InterruptInfo) []string {
	if info == nil {
		return nil
	}
	out := make([]string, 0, len(info.InterruptContexts))
	for _, interruptCtx := range info.InterruptContexts {
		if interruptCtx != nil && interruptCtx.ID != "" {
			out = append(out, interruptCtx.ID)
		}
	}
	return out
}

func (e *Executor) failTask(ctx context.Context, input RunTaskInput, code string, message string) error {
	e.loggerOrDefault().ErrorContext(ctx, "producer task failed",
		"workspace_id", uuidString(input.WorkspaceID),
		"thread_id", uuidString(input.ThreadID),
		"task_id", uuidString(input.TaskID),
		"trigger_message_id", uuidString(input.TriggerMessageID),
		"error_code", code,
		"error", message,
	)
	if releaser, ok := e.runtime.(producerSignalReleaser); ok {
		_, _ = releaser.ReleaseProducerPendingSignalsForTask(ctx, input.WorkspaceID, input.TaskID, code+": "+message)
	}
	errorMsg, msgErr := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        "assistant",
		MessageType: "error",
		Content: mustJSON(map[string]any{
			"text":       "ClipAnvil 暂时没有生成回复，请稍后再试。",
			"error_code": code,
		}),
		RawMessage: mustJSON(map[string]any{"error": message}),
		TaskID:     input.TaskID,
	})
	failedEvent, eventErr := e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "producer_turn_failed",
		SourceRole:  "producer",
		TargetRole:  "user",
		Payload:     mustJSON(map[string]string{"error_code": code}),
	})
	failedTask, err := e.runtime.MarkTaskFailed(ctx, input.TaskID, code, message)
	if msgErr == nil && eventErr == nil {
		e.broadcastMessage(input.WorkspaceID, errorMsg, failedEvent)
		e.broadcastEvent(input.WorkspaceID, failedEvent)
	}
	if err == nil {
		e.broadcastTask(input.WorkspaceID, failedTask)
	}
	return fmt.Errorf("%s: %s", code, message)
}

func (e *Executor) loggerOrDefault() *slog.Logger {
	if e != nil && e.logger != nil {
		return e.logger
	}
	return slog.Default()
}

func (e *Executor) broadcastMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent) {
	if e.broadcaster != nil {
		e.broadcaster.BroadcastAgentMessage(workspaceID, message, event)
	}
}

func (e *Executor) broadcastMessageUpdated(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent) {
	if e.broadcaster != nil {
		e.broadcaster.BroadcastAgentMessageUpdated(workspaceID, message, event)
	}
}

func (e *Executor) broadcastTask(workspaceID pgtype.UUID, task db.AgentTask) {
	if e.broadcaster != nil {
		e.broadcaster.BroadcastAgentTask(workspaceID, task)
	}
}

func (e *Executor) broadcastEvent(workspaceID pgtype.UUID, event db.AgentEvent) {
	if e.broadcaster != nil {
		e.broadcaster.BroadcastAgentEvent(workspaceID, event)
	}
}

func (e *Executor) broadcastMessageDelta(workspaceID pgtype.UUID, delta ProducerStreamDelta) {
	if e.broadcaster != nil {
		e.broadcaster.BroadcastAgentMessageDelta(workspaceID, delta)
	}
}

func applyProducerTaskTriggerInput(input *RunTaskInput, raw []byte) {
	var payload producerTaskTriggerPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}
	if !input.TriggerMessageID.Valid {
		if id, ok := pgUUIDFromString(payload.TriggerMessageID); ok {
			input.TriggerMessageID = id
		}
	}
	if input.TriggerMessageSeq <= 0 {
		input.TriggerMessageSeq = payload.TriggerMessageSeq
	}
	if strings.TrimSpace(input.RuntimeTriggerText) == "" && !input.TriggerMessageID.Valid && input.TriggerMessageSeq <= 0 {
		input.RuntimeTriggerText = producerRuntimeTriggerText(payload)
	}
}

type producerTaskTriggerPayload struct {
	Trigger           string `json:"trigger"`
	CraftsmanTaskID   string `json:"craftsman_task_id"`
	CraftsmanThreadID string `json:"craftsman_thread_id"`
	ScopeType         string `json:"scope_type"`
	ScopeID           string `json:"scope_id"`
	ShotID            string `json:"shot_id"`
	TargetPhase       string `json:"target_phase"`
	TriggerMessageID  string `json:"trigger_message_id"`
	TriggerMessageSeq int64  `json:"trigger_message_seq"`
}

func producerRuntimeTriggerText(payload producerTaskTriggerPayload) string {
	switch strings.TrimSpace(payload.Trigger) {
	case "craftsman_render_plan_ready":
		lines := []string{
			"<system-reminder>",
			"系统事件：Craftsman 已完成 RenderPlan 编译，当前轮次由工程自动唤醒 Producer。",
			"触发原因：craftsman_render_plan_ready。",
			"下一步：请读取项目上下文，检查 waiting_for_approval RenderPlan，并决定调用 decide_render_plan accept/reject，或先派 Reviewer 评审。",
		}
		if targetPhase := strings.TrimSpace(payload.TargetPhase); targetPhase != "" {
			lines = append(lines, "目标阶段："+targetPhase+"。")
		}
		if scopeType := strings.TrimSpace(payload.ScopeType); scopeType != "" {
			lines = append(lines, "目标范围："+scopeType+" "+strings.TrimSpace(payload.ScopeID)+"。")
		}
		if shotID := strings.TrimSpace(payload.ShotID); shotID != "" {
			lines = append(lines, "关联分镜："+shotID+"。")
		}
		if craftsmanTaskID := strings.TrimSpace(payload.CraftsmanTaskID); craftsmanTaskID != "" {
			lines = append(lines, "Craftsman 任务："+craftsmanTaskID+"。")
		}
		lines = append(lines, "</system-reminder>")
		return strings.Join(lines, "\n")
	default:
		return ""
	}
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func defaultJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

func boolFromMap(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	value, ok := values[key].(bool)
	return ok && value
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
