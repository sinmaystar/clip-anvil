package craftsman

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cloudwego/eino/callbacks"
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

type ExecutorRuntime interface {
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
	UpdateMessage(ctx context.Context, params agentruntime.UpdateMessageParams) (db.AgentMessage, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
	CreateProducerPendingSignal(ctx context.Context, params agentruntime.CreateProducerPendingSignalParams) (db.ProducerPendingSignal, error)
	GetLatestRenderPlanByTaskScopePhase(ctx context.Context, params db.GetLatestRenderPlanByTaskScopePhaseParams) (db.RenderPlan, error)
	ListActiveAgentTasksByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.AgentTask, error)
	SetThreadCheckpoint(ctx context.Context, threadID pgtype.UUID, checkpointKey string) (db.AgentThread, error)
	UpdateShotStatus(ctx context.Context, params db.UpdateShotStatusParams) (db.Shot, error)
}

type Runner interface {
	Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error)
}

type ExecutorConfig struct {
	Runtime          ExecutorRuntime
	Graph            Runner
	Broadcaster      Broadcaster
	ProducerEnqueuer ProducerTaskEnqueuer
	Logger           *slog.Logger
	TraceCallbacks   []callbacks.Handler
}

type Broadcaster interface {
	BroadcastAgentMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent)
	BroadcastAgentMessageUpdated(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent)
}

type ProducerTaskEnqueuer interface {
	EnqueueProducerTask(ctx context.Context, task db.AgentTask)
}

type Executor struct {
	runtime          ExecutorRuntime
	graph            Runner
	broadcaster      Broadcaster
	producerEnqueuer ProducerTaskEnqueuer
	logger           *slog.Logger
	traceCallbacks   []callbacks.Handler
}

func NewExecutor(config ExecutorConfig) *Executor {
	return &Executor{
		runtime:          config.Runtime,
		graph:            config.Graph,
		broadcaster:      config.Broadcaster,
		producerEnqueuer: config.ProducerEnqueuer,
		logger:           config.Logger,
		traceCallbacks:   config.TraceCallbacks,
	}
}

func (e *Executor) RunTask(ctx context.Context, input RunTaskInput) error {
	if input.ScopeType == "" {
		input.ScopeType = "shot"
	}
	if !input.ScopeID.Valid && input.ShotID.Valid {
		input.ScopeID = input.ShotID
	}
	if input.ScopeType == "shot" && !input.ShotID.Valid {
		input.ShotID = input.ScopeID
	}
	if e == nil || e.runtime == nil || e.graph == nil || !input.WorkspaceID.Valid || !input.ThreadID.Valid || !input.TaskID.Valid || !input.ScopeID.Valid {
		return ErrInvalidConfig
	}
	if _, err := e.runtime.MarkTaskRunning(ctx, input.TaskID); err != nil {
		return err
	}
	_, _ = e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "craftsman_started",
		SourceRole:  "craftsman",
		Scope:       mustJSON(map[string]any{"scope_type": input.ScopeType, "scope_id": uuidString(input.ScopeID), "shot_id": uuidString(input.ShotID)}),
	})
	taskInput, err := parseTaskInput(input.Input)
	if err != nil {
		return e.fail(ctx, input, "craftsman_invalid_input", err)
	}
	graphInput := GraphInput{
		WorkspaceID:      input.WorkspaceID,
		ThreadID:         input.ThreadID,
		TaskID:           input.TaskID,
		ScopeType:        input.ScopeType,
		ScopeID:          input.ScopeID,
		ShotID:           input.ShotID,
		Mode:             taskInput.Mode,
		ExecutionPolicy:  taskInput.ExecutionPolicy,
		ParentToolCallID: taskInput.ParentToolCallID,
		MaxAttempts:      taskInput.MaxAttempts,
		WorkerParams:     taskInput.WorkerParams,
	}
	checkpointKey := agenteino.CheckpointKey("craftsman_generation", input.WorkspaceID, input.ThreadID, input.TaskID)
	ctx = cozelooptrace.ContextWithAttributes(ctx,
		attribute.String("clipanvil.workspace_id", uuidString(input.WorkspaceID)),
		attribute.String("clipanvil.agent.thread_id", uuidString(input.ThreadID)),
		attribute.String("clipanvil.agent.task_id", uuidString(input.TaskID)),
		attribute.String("clipanvil.agent.role", "craftsman"),
		attribute.String("clipanvil.agent.task_type", "craftsman_turn"),
		attribute.String("clipanvil.agent.scope_type", input.ScopeType),
		attribute.String("clipanvil.agent.scope_id", uuidString(input.ScopeID)),
		attribute.String("clipanvil.agent.shot_id", uuidString(input.ShotID)),
	)
	liveToolTrace := newCraftsmanLiveToolTrace(e, input, taskInput.ParentToolCallID)
	ctx = agenttools.WithNativeToolTraceSink(ctx, liveToolTrace)
	out, err := e.graph.Run(ctx, graphInput, agenteino.RunOptions{
		CheckPointID: checkpointKey,
		Callbacks:    e.traceCallbacks,
	})
	if err != nil {
		return e.fail(ctx, input, "craftsman_failed", err)
	}
	if liveToolTrace.count() == 0 {
		if err := e.persistNativeToolTrace(ctx, input, out.SameTurnMessages, taskInput.ParentToolCallID); err != nil {
			return e.fail(ctx, input, "craftsman_tool_trace_persist_failed", err)
		}
	}
	if err := e.persistAssistantText(ctx, input, out.AssistantText); err != nil {
		return e.fail(ctx, input, "craftsman_message_persist_failed", err)
	}
	outputPayload := map[string]any{
		"assistant_text": out.AssistantText,
		"metadata":       out.Metadata,
	}
	if strings.TrimSpace(out.Strategy.Strategy) != "" {
		outputPayload["strategy"] = out.Strategy.Strategy
		outputPayload["preview_prompt"] = out.Strategy.PreviewPrompt
	}
	if out.WorkerTask.ID.Valid {
		outputPayload["worker_task_id"] = uuidString(out.WorkerTask.ID)
		outputPayload["worker_task_type"] = out.WorkerTask.TaskType
	}
	if out.Metadata != nil {
		outputPayload["checkpoint_key"] = out.Metadata["checkpoint_key"]
	}
	rawOutput, _ := json.Marshal(outputPayload)
	if _, err := e.runtime.SetThreadCheckpoint(ctx, input.ThreadID, checkpointKey); err != nil {
		return e.fail(ctx, input, "craftsman_checkpoint_update_failed", err)
	}
	_, _ = e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "craftsman_strategy_created",
		SourceRole:  "craftsman",
		TargetRole:  "worker",
		Payload:     rawOutput,
	})
	if err := e.wakeProducerIfNeeded(ctx, input, taskInput, rawOutput); err != nil {
		return e.fail(ctx, input, "craftsman_ready_missing_render_plan", err)
	}
	if _, err := e.runtime.MarkTaskSucceeded(ctx, input.TaskID, rawOutput); err != nil {
		return err
	}
	return nil
}

type craftsmanLiveToolTrace struct {
	executor         *Executor
	input            RunTaskInput
	parentToolCallID string
	callMessages     map[string]db.AgentMessage
	startedCount     int
}

func newCraftsmanLiveToolTrace(executor *Executor, input RunTaskInput, parentToolCallID string) *craftsmanLiveToolTrace {
	return &craftsmanLiveToolTrace{
		executor:         executor,
		input:            input,
		parentToolCallID: strings.TrimSpace(parentToolCallID),
		callMessages:     map[string]db.AgentMessage{},
	}
}

func (t *craftsmanLiveToolTrace) count() int {
	if t == nil {
		return 0
	}
	return t.startedCount
}

func (t *craftsmanLiveToolTrace) NativeToolCallStarted(ctx context.Context, runtime agenttools.NativeRuntimeContext, trace agenttools.NativeToolTrace) error {
	if t == nil || t.executor == nil || t.executor.runtime == nil {
		return nil
	}
	toolCallID := strings.TrimSpace(runtime.ToolCallID)
	if toolCallID == "" {
		toolCallID = strings.TrimSpace(trace.ToolName)
	}
	sameTurn := CraftsmanSameTurnMessage{
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
		Content:     craftsmanToolTraceContent(sameTurn),
		RawMessage:  craftsmanToolTraceRaw(sameTurn, t.parentToolCallID),
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

func (t *craftsmanLiveToolTrace) NativeToolCallCompleted(ctx context.Context, runtime agenttools.NativeRuntimeContext, trace agenttools.NativeToolTrace) error {
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
	sameTurn := CraftsmanSameTurnMessage{
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
		Content:     craftsmanToolTraceContent(sameTurn),
		RawMessage:  craftsmanToolTraceRaw(sameTurn, t.parentToolCallID),
		TaskID:      t.input.TaskID,
	})
	if err != nil {
		return err
	}
	t.executor.broadcastMessage(t.input.WorkspaceID, msg, db.AgentEvent{})
	if callMsg, ok := t.callMessages[toolCallID]; ok {
		updated, err := t.executor.runtime.UpdateMessage(ctx, agentruntime.UpdateMessageParams{
			ID:         callMsg.ID,
			Content:    craftsmanCompletedToolTraceContentWithStatus(sameTurn, callMsg.RawMessage, status),
			RawMessage: craftsmanCompletedToolTraceRawWithStatus(sameTurn, callMsg.RawMessage, status),
		})
		if err != nil {
			return err
		}
		t.executor.broadcastMessageUpdated(t.input.WorkspaceID, updated, db.AgentEvent{})
	}
	return nil
}

type parsedTaskInput struct {
	Mode             string
	ExecutionPolicy  string
	ParentToolCallID string
	ProducerThreadID pgtype.UUID
	ProducerTaskID   pgtype.UUID
	MaxAttempts      int
	WorkerParams     map[string]any
}

func parseTaskInput(raw []byte) (parsedTaskInput, error) {
	out := parsedTaskInput{
		Mode:            "preview_image",
		ExecutionPolicy: "wait_for_producer",
		MaxAttempts:     3,
		WorkerParams:    map[string]any{},
	}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out.WorkerParams); err != nil {
		return parsedTaskInput{}, err
	}
	if mode, ok := out.WorkerParams["mode"].(string); ok && strings.TrimSpace(mode) != "" {
		out.Mode = strings.TrimSpace(mode)
	}
	if targetPhase, ok := out.WorkerParams["target_phase"].(string); ok && strings.TrimSpace(targetPhase) != "" {
		out.Mode = strings.TrimSpace(targetPhase)
	}
	if executionPolicy, ok := out.WorkerParams["execution_policy"].(string); ok && strings.TrimSpace(executionPolicy) != "" {
		out.ExecutionPolicy = strings.TrimSpace(executionPolicy)
	}
	if parentToolCallID, ok := out.WorkerParams["parent_tool_call_id"].(string); ok {
		out.ParentToolCallID = strings.TrimSpace(parentToolCallID)
	}
	if producerThreadID, ok := stringUUIDValue(out.WorkerParams["producer_thread_id"]); ok {
		out.ProducerThreadID = producerThreadID
	}
	if producerTaskID, ok := stringUUIDValue(out.WorkerParams["producer_task_id"]); ok {
		out.ProducerTaskID = producerTaskID
	}
	if out.ExecutionPolicy != "execute_immediately" && out.ExecutionPolicy != "wait_for_producer" {
		out.ExecutionPolicy = "wait_for_producer"
	}
	if refs := taskInputStringSlice(out.WorkerParams["input_node_refs"]); len(refs) > 0 {
		out.WorkerParams["input_node_refs"] = refs
	}
	if maxAttempts := taskInputInt(out.WorkerParams["requested_max_attempts"]); maxAttempts > 0 {
		out.MaxAttempts = maxAttempts
	}
	if out.MaxAttempts > 3 {
		out.MaxAttempts = 3
	}
	if out.MaxAttempts < 1 {
		out.MaxAttempts = 1
	}
	return out, nil
}

func (e *Executor) wakeProducerIfNeeded(ctx context.Context, input RunTaskInput, taskInput parsedTaskInput, craftsmanOutput []byte) error {
	if taskInput.ExecutionPolicy != "wait_for_producer" || e == nil || e.runtime == nil || e.producerEnqueuer == nil || !taskInput.ProducerThreadID.Valid {
		return nil
	}
	renderPlan, err := e.runtime.GetLatestRenderPlanByTaskScopePhase(ctx, db.GetLatestRenderPlanByTaskScopePhaseParams{
		WorkspaceID:     input.WorkspaceID,
		ScopeType:       input.ScopeType,
		ScopeID:         input.ScopeID,
		TargetPhase:     taskInput.Mode,
		CreatedByTaskID: input.TaskID,
	})
	if err != nil {
		return fmt.Errorf("craftsman ready missing render plan: %w", err)
	}
	_, err = e.runtime.CreateProducerPendingSignal(ctx, agentruntime.CreateProducerPendingSignalParams{
		WorkspaceID:      input.WorkspaceID,
		ProducerThreadID: taskInput.ProducerThreadID,
		SourceRole:       "craftsman",
		SourceTaskID:     input.TaskID,
		SourceThreadID:   input.ThreadID,
		SignalType:       "craftsman_render_plan_ready",
		ScopeType:        input.ScopeType,
		ScopeID:          input.ScopeID,
		RenderPlanID:     renderPlan.ID,
		Priority:         50,
		DedupeKey:        "craftsman_render_plan_ready:" + uuidString(renderPlan.ID),
		Payload: mustJSON(map[string]any{
			"target_phase":        taskInput.Mode,
			"render_plan_id":      uuidString(renderPlan.ID),
			"render_plan_status":  renderPlan.Status,
			"scope_type":          input.ScopeType,
			"scope_id":            uuidString(input.ScopeID),
			"shot_id":             uuidString(input.ShotID),
			"craftsman_task_id":   uuidString(input.TaskID),
			"craftsman_thread_id": uuidString(input.ThreadID),
			"craftsman_output":    json.RawMessage(craftsmanOutput),
		}),
	})
	if err != nil {
		return err
	}
	activeTasks, err := e.runtime.ListActiveAgentTasksByWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return err
	}
	for _, task := range activeTasks {
		if task.Role == "producer" &&
			(task.TaskType == "producer_turn" || task.TaskType == "decision_resume") &&
			(task.Status == "queued" || task.Status == "running" || task.Status == "waiting_for_user") {
			return nil
		}
	}
	wakeInput := mustJSON(map[string]any{
		"trigger":             "craftsman_render_plan_ready",
		"craftsman_task_id":   uuidString(input.TaskID),
		"craftsman_thread_id": uuidString(input.ThreadID),
		"producer_task_id":    uuidString(taskInput.ProducerTaskID),
		"scope_type":          input.ScopeType,
		"scope_id":            uuidString(input.ScopeID),
		"shot_id":             uuidString(input.ShotID),
		"target_phase":        taskInput.Mode,
		"render_plan_id":      uuidString(renderPlan.ID),
	})
	task, err := e.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    taskInput.ProducerThreadID,
		Role:        "producer",
		ScopeType:   "workspace",
		TaskType:    "producer_turn",
		MaxAttempts: 1,
		Input:       wakeInput,
	})
	if err != nil {
		return err
	}
	_, _ = e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    taskInput.ProducerThreadID,
		TaskID:      task.ID,
		EventType:   "producer_turn_queued",
		SourceRole:  "system",
		TargetRole:  "producer",
		Scope:       mustJSON(map[string]any{"trigger": "craftsman_render_plan_ready", "scope_type": input.ScopeType, "scope_id": uuidString(input.ScopeID)}),
		Payload:     mustJSON(map[string]any{"craftsman_task_id": uuidString(input.TaskID), "target_phase": taskInput.Mode, "render_plan_id": uuidString(renderPlan.ID), "craftsman_output": json.RawMessage(craftsmanOutput)}),
	})
	e.producerEnqueuer.EnqueueProducerTask(ctx, task)
	return nil
}

func stringUUIDValue(value any) (pgtype.UUID, bool) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return pgtype.UUID{}, false
	}
	parsed, err := uuid.Parse(strings.TrimSpace(text))
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}

func taskInputStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	default:
		return nil
	}
}

func taskInputInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func (e *Executor) fail(ctx context.Context, input RunTaskInput, code string, err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	e.loggerOrDefault().ErrorContext(ctx, "craftsman task failed",
		"workspace_id", uuidString(input.WorkspaceID),
		"thread_id", uuidString(input.ThreadID),
		"task_id", uuidString(input.TaskID),
		"scope_type", input.ScopeType,
		"scope_id", uuidString(input.ScopeID),
		"shot_id", uuidString(input.ShotID),
		"error_code", code,
		"error", message,
	)
	e.markScopedShotFailed(ctx, input)
	_, _ = e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        "assistant",
		MessageType: "error",
		Content:     mustJSON(map[string]any{"error_code": code, "text": "分镜预览图策略生成失败。"}),
		RawMessage:  mustJSON(map[string]any{"error": message}),
		TaskID:      input.TaskID,
	})
	_, _ = e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "craftsman_failed",
		SourceRole:  "craftsman",
		Payload:     mustJSON(map[string]any{"error_code": code, "error": message}),
	})
	_, _ = e.runtime.MarkTaskFailed(ctx, input.TaskID, code, message)
	return fmt.Errorf("%s: %w", code, err)
}

func (e *Executor) markScopedShotFailed(ctx context.Context, input RunTaskInput) {
	if e == nil || e.runtime == nil || input.ScopeType != "shot" || !input.ScopeID.Valid || !input.WorkspaceID.Valid {
		return
	}
	_, _ = e.runtime.UpdateShotStatus(ctx, db.UpdateShotStatusParams{
		ID:          input.ScopeID,
		WorkspaceID: input.WorkspaceID,
		Status:      "failed",
	})
}

func (e *Executor) persistNativeToolTrace(ctx context.Context, input RunTaskInput, messages []CraftsmanSameTurnMessage, parentToolCallID string) error {
	callMessages := map[string]db.AgentMessage{}
	for _, trace := range messages {
		messageType := strings.TrimSpace(trace.MessageType)
		if messageType != "tool_call" && messageType != "tool_result" {
			continue
		}
		role := strings.TrimSpace(trace.Role)
		if role == "" {
			role = "assistant"
			if messageType == "tool_result" {
				role = "tool"
			}
		}
		msg, err := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
			WorkspaceID: input.WorkspaceID,
			ThreadID:    input.ThreadID,
			Role:        role,
			MessageType: messageType,
			Content:     craftsmanToolTraceContent(trace),
			RawMessage:  craftsmanToolTraceRaw(trace, parentToolCallID),
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
					Content:    craftsmanCompletedToolTraceContent(trace, callMsg.RawMessage),
					RawMessage: craftsmanCompletedToolTraceRaw(trace, callMsg.RawMessage),
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

func (e *Executor) persistAssistantText(ctx context.Context, input RunTaskInput, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	content, err := uimessage.BuildAssistantMessageContent(uimessage.AssistantMessageInput{Text: text})
	if err != nil {
		return err
	}
	msg, err := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        "assistant",
		MessageType: "text",
		Content:     content,
		RawMessage:  mustJSON(map[string]any{"schema": "clipanvil.agent.assistant_text.v1", "text": text}),
		TaskID:      input.TaskID,
	})
	if err != nil {
		return err
	}
	e.broadcastMessage(input.WorkspaceID, msg, db.AgentEvent{})
	return nil
}

func craftsmanToolTraceContent(trace CraftsmanSameTurnMessage) []byte {
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

func craftsmanCompletedToolTraceContent(trace CraftsmanSameTurnMessage, previousRaw []byte) []byte {
	return craftsmanCompletedToolTraceContentWithStatus(trace, previousRaw, "succeeded")
}

func craftsmanCompletedToolTraceContentWithStatus(trace CraftsmanSameTurnMessage, previousRaw []byte, status string) []byte {
	args := toolTraceArgumentsFromRaw(previousRaw)
	result := map[string]any{}
	if text := strings.TrimSpace(trace.Content); text != "" {
		result["text"] = text
	}
	return toolStatusContent(trace.ToolCallID, trace.ToolName, trace.ToolName, status, toolTraceSummary(args, trace.Content), "", args, result)
}

func craftsmanCompletedToolTraceRaw(trace CraftsmanSameTurnMessage, previousRaw []byte) []byte {
	return craftsmanCompletedToolTraceRawWithStatus(trace, previousRaw, "succeeded")
}

func craftsmanCompletedToolTraceRawWithStatus(trace CraftsmanSameTurnMessage, previousRaw []byte, status string) []byte {
	raw := map[string]any{}
	_ = json.Unmarshal(defaultJSON(previousRaw), &raw)
	raw["result_text"] = strings.TrimSpace(trace.Content)
	raw["message_type"] = "tool_call"
	raw["status"] = status
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

func defaultJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

func craftsmanToolTraceRaw(trace CraftsmanSameTurnMessage, parentToolCallID string) []byte {
	payload := map[string]any{
		"schema":       "clipanvil.agent.tool_trace.v1",
		"role":         trace.Role,
		"message_type": trace.MessageType,
		"tool_call_id": trace.ToolCallID,
		"tool_name":    trace.ToolName,
	}
	if strings.TrimSpace(parentToolCallID) != "" {
		payload["parent_tool_call_id"] = strings.TrimSpace(parentToolCallID)
	}
	if len(trace.ToolArguments) > 0 {
		payload["arguments"] = trace.ToolArguments
	}
	if strings.TrimSpace(trace.Content) != "" {
		if trace.MessageType == "tool_result" {
			payload["result_text"] = strings.TrimSpace(trace.Content)
		} else {
			payload["text"] = strings.TrimSpace(trace.Content)
		}
	}
	return mustJSON(payload)
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
