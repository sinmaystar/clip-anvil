package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/callbacks"
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
	ListActiveAgentTasksByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.AgentTask, error)
	SetThreadCheckpoint(ctx context.Context, threadID pgtype.UUID, checkpointKey string) (db.AgentThread, error)
}

type Runner interface {
	Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error)
}

type ExecutorConfig struct {
	Runtime          ExecutorRuntime
	Graph            Runner
	Broadcaster      Broadcaster
	ProducerEnqueuer ProducerTaskEnqueuer
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
	traceCallbacks   []callbacks.Handler
}

func NewExecutor(config ExecutorConfig) *Executor {
	return &Executor{
		runtime:          config.Runtime,
		graph:            config.Graph,
		broadcaster:      config.Broadcaster,
		producerEnqueuer: config.ProducerEnqueuer,
		traceCallbacks:   config.TraceCallbacks,
	}
}

func (e *Executor) RunTask(ctx context.Context, input RunTaskInput) error {
	task := input.Task
	if e == nil || e.runtime == nil || e.graph == nil || !task.ID.Valid || !task.WorkspaceID.Valid || !task.ThreadID.Valid {
		return ErrInvalidConfig
	}
	if task.TaskType != "reviewer_turn" {
		return fmt.Errorf("%w: unsupported task type %q", ErrInvalidInput, task.TaskType)
	}
	if _, err := e.runtime.MarkTaskRunning(ctx, task.ID); err != nil {
		return err
	}
	taskInput, err := parseTaskInput(task.Input)
	if err != nil {
		return e.fail(ctx, task.ID, "reviewer_invalid_input", err)
	}
	graphInput := GraphInput{
		WorkspaceID: task.WorkspaceID,
		ThreadID:    task.ThreadID,
		TaskID:      task.ID,
		Task:        taskInput,
	}
	checkpointKey := agenteino.CheckpointKey("reviewer_preview", task.WorkspaceID, task.ThreadID, task.ID)
	ctx = cozelooptrace.ContextWithAttributes(ctx,
		attribute.String("clipanvil.workspace_id", uuidString(task.WorkspaceID)),
		attribute.String("clipanvil.agent.thread_id", uuidString(task.ThreadID)),
		attribute.String("clipanvil.agent.task_id", uuidString(task.ID)),
		attribute.String("clipanvil.agent.role", "reviewer"),
		attribute.String("clipanvil.agent.task_type", task.TaskType),
		attribute.String("clipanvil.agent.scope_type", task.ScopeType),
		attribute.String("clipanvil.agent.scope_id", uuidString(task.ScopeID)),
	)
	liveToolTrace := newReviewerLiveToolTrace(e, task)
	ctx = agenttools.WithNativeToolTraceSink(ctx, liveToolTrace)
	out, err := e.graph.Run(ctx, graphInput, agenteino.RunOptions{
		CheckPointID: checkpointKey,
		Callbacks:    e.traceCallbacks,
	})
	if err != nil {
		return e.fail(ctx, task.ID, "reviewer_failed", err)
	}
	if liveToolTrace.count() == 0 {
		if err := e.persistNativeToolTrace(ctx, task, out.SameTurnMessages); err != nil {
			return e.fail(ctx, task.ID, "reviewer_tool_trace_persist_failed", err)
		}
	}
	if err := e.persistAssistantText(ctx, task, out.Result.Critique); err != nil {
		return e.fail(ctx, task.ID, "reviewer_message_persist_failed", err)
	}
	rawOutput := mustJSON(map[string]any{
		"review_record_id": uuidString(out.Record.ID),
		"status":           out.Decision.Status,
		"should_retry":     out.Decision.ShouldRetry,
	})
	if _, err := e.runtime.MarkTaskSucceeded(ctx, task.ID, rawOutput); err != nil {
		return err
	}
	if _, err := e.runtime.SetThreadCheckpoint(ctx, task.ThreadID, checkpointKey); err != nil {
		return e.fail(ctx, task.ID, "reviewer_checkpoint_update_failed", err)
	}
	if err := e.wakeProducerIfNeeded(ctx, task, taskInput, out); err != nil {
		return e.fail(ctx, task.ID, "reviewer_producer_signal_failed", err)
	}
	return nil
}

func (e *Executor) wakeProducerIfNeeded(ctx context.Context, task db.AgentTask, input TaskInput, out GraphOutput) error {
	if e == nil || e.runtime == nil || e.producerEnqueuer == nil || !task.WorkspaceID.Valid || !task.ID.Valid || !task.ThreadID.Valid {
		return nil
	}
	producerThreadID, ok := pgUUIDFromString(input.ProducerThreadID)
	if !ok {
		return nil
	}
	reviewRecordID := out.Record.ID
	if !reviewRecordID.Valid {
		return nil
	}
	scopeType := strings.TrimSpace(task.ScopeType)
	if scopeType == "" {
		scopeType = strings.TrimSpace(input.Target.WorkspaceScope)
	}
	scopeID := task.ScopeID
	if !scopeID.Valid {
		scopeID = reviewScopeID(input)
	}
	if scopeType == "" || !scopeID.Valid {
		return nil
	}
	renderPlanID, _ := pgUUIDFromString(input.Target.RenderPlanID)
	payload := mustJSON(map[string]any{
		"trigger":              "review_completed",
		"review_record_id":     uuidString(reviewRecordID),
		"review_task":          input.ReviewTask,
		"verdict":              out.Decision.Status,
		"should_retry":         out.Decision.ShouldRetry,
		"target_phase":         input.TargetPhase,
		"scope_type":           scopeType,
		"scope_id":             uuidString(scopeID),
		"shot_id":              input.ShotID,
		"node_id":              input.NodeID,
		"render_plan_id":       input.Target.RenderPlanID,
		"generation_job_id":    input.GenerationJobID,
		"artifact_version_id":  input.ArtifactVersionID,
		"reviewer_task_id":     uuidString(task.ID),
		"reviewer_thread_id":   uuidString(task.ThreadID),
		"producer_task_id":     input.ProducerTaskID,
		"parent_review_record": input.ParentReviewRecordID,
	})
	_, err := e.runtime.CreateProducerPendingSignal(ctx, agentruntime.CreateProducerPendingSignalParams{
		WorkspaceID:      task.WorkspaceID,
		ProducerThreadID: producerThreadID,
		SourceRole:       "reviewer",
		SourceTaskID:     task.ID,
		SourceThreadID:   task.ThreadID,
		SignalType:       "review_completed",
		ScopeType:        scopeType,
		ScopeID:          scopeID,
		RenderPlanID:     renderPlanID,
		Priority:         80,
		DedupeKey:        "review_completed:" + uuidString(reviewRecordID),
		Payload:          payload,
	})
	if err != nil {
		return err
	}
	activeTasks, err := e.runtime.ListActiveAgentTasksByWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return err
	}
	for _, activeTask := range activeTasks {
		if activeTask.Role == "producer" &&
			(activeTask.TaskType == "producer_turn" || activeTask.TaskType == "decision_resume") &&
			(activeTask.Status == "queued" || activeTask.Status == "running" || activeTask.Status == "waiting_for_user") {
			return nil
		}
	}
	wakeTask, err := e.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: task.WorkspaceID,
		ThreadID:    producerThreadID,
		Role:        "producer",
		ScopeType:   "workspace",
		TaskType:    "producer_turn",
		MaxAttempts: 1,
		Input:       payload,
	})
	if err != nil {
		return err
	}
	_, _ = e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: task.WorkspaceID,
		ThreadID:    producerThreadID,
		TaskID:      wakeTask.ID,
		EventType:   "producer_turn_queued",
		SourceRole:  "system",
		TargetRole:  "producer",
		Scope:       mustJSON(map[string]any{"trigger": "review_completed", "scope_type": scopeType, "scope_id": uuidString(scopeID)}),
		Payload:     payload,
	})
	e.producerEnqueuer.EnqueueProducerTask(ctx, wakeTask)
	return nil
}

func reviewScopeID(input TaskInput) pgtype.UUID {
	for _, candidate := range []string{
		input.Target.RenderPlanID,
		input.Target.ShotID,
		input.ShotID,
		input.Target.NodeID,
		input.NodeID,
	} {
		if id, ok := pgUUIDFromString(candidate); ok {
			return id
		}
	}
	return pgtype.UUID{}
}

type reviewerLiveToolTrace struct {
	executor     *Executor
	task         db.AgentTask
	callMessages map[string]db.AgentMessage
	startedCount int
}

func newReviewerLiveToolTrace(executor *Executor, task db.AgentTask) *reviewerLiveToolTrace {
	return &reviewerLiveToolTrace{
		executor:     executor,
		task:         task,
		callMessages: map[string]db.AgentMessage{},
	}
}

func (t *reviewerLiveToolTrace) count() int {
	if t == nil {
		return 0
	}
	return t.startedCount
}

func (t *reviewerLiveToolTrace) NativeToolCallStarted(ctx context.Context, runtime agenttools.NativeRuntimeContext, trace agenttools.NativeToolTrace) error {
	if t == nil || t.executor == nil || t.executor.runtime == nil {
		return nil
	}
	toolCallID := strings.TrimSpace(runtime.ToolCallID)
	if toolCallID == "" {
		toolCallID = strings.TrimSpace(trace.ToolName)
	}
	sameTurn := ReviewerSameTurnMessage{
		Role:          "assistant",
		MessageType:   "tool_call",
		ToolCallID:    toolCallID,
		ToolName:      trace.ToolName,
		ToolArguments: trace.Arguments,
	}
	msg, err := t.executor.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: t.task.WorkspaceID,
		ThreadID:    t.task.ThreadID,
		Role:        "assistant",
		MessageType: "tool_call",
		Content:     reviewerToolTraceContent(sameTurn),
		RawMessage:  reviewerToolTraceRaw(sameTurn),
		TaskID:      t.task.ID,
	})
	if err != nil {
		return err
	}
	t.callMessages[toolCallID] = msg
	t.startedCount++
	t.executor.broadcastMessage(t.task.WorkspaceID, msg, db.AgentEvent{})
	return nil
}

func (t *reviewerLiveToolTrace) NativeToolCallCompleted(ctx context.Context, runtime agenttools.NativeRuntimeContext, trace agenttools.NativeToolTrace) error {
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
	sameTurn := ReviewerSameTurnMessage{
		Role:        "tool",
		MessageType: "tool_result",
		Content:     resultText,
		ToolCallID:  toolCallID,
		ToolName:    trace.ToolName,
	}
	msg, err := t.executor.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: t.task.WorkspaceID,
		ThreadID:    t.task.ThreadID,
		Role:        "tool",
		MessageType: "tool_result",
		Content:     reviewerToolTraceContent(sameTurn),
		RawMessage:  reviewerToolTraceRaw(sameTurn),
		TaskID:      t.task.ID,
	})
	if err != nil {
		return err
	}
	t.executor.broadcastMessage(t.task.WorkspaceID, msg, db.AgentEvent{})
	if callMsg, ok := t.callMessages[toolCallID]; ok {
		updated, err := t.executor.runtime.UpdateMessage(ctx, agentruntime.UpdateMessageParams{
			ID:         callMsg.ID,
			Content:    reviewerCompletedToolTraceContentWithStatus(sameTurn, callMsg.RawMessage, status),
			RawMessage: reviewerCompletedToolTraceRawWithStatus(sameTurn, callMsg.RawMessage, status),
		})
		if err != nil {
			return err
		}
		t.executor.broadcastMessageUpdated(t.task.WorkspaceID, updated, db.AgentEvent{})
	}
	return nil
}

func parseTaskInput(raw []byte) (TaskInput, error) {
	var input TaskInput
	if err := json.Unmarshal(defaultJSON(raw), &input); err != nil {
		return TaskInput{}, err
	}
	if input.TargetPhase == "" {
		input.TargetPhase = targetPhaseFromReviewTask(input.ReviewTask)
	}
	if input.TargetPhase == "" {
		input.TargetPhase = TargetPhasePreviewImage
	}
	if input.ReviewTask == "" {
		input.ReviewTask = reviewTaskForTargetPhase(input.TargetPhase)
	}
	if input.ShotID == "" {
		input.ShotID = input.Target.ShotID
	}
	if input.NodeID == "" {
		input.NodeID = input.Target.NodeID
	}
	if input.ArtifactVersionID == "" {
		input.ArtifactVersionID = input.Target.ArtifactVersionID
	}
	if input.GenerationJobID == "" {
		input.GenerationJobID = input.Target.GenerationJobID
	}
	if input.ParentReviewRecordID == "" {
		input.ParentReviewRecordID = input.Target.ParentReviewRecordID
	}
	switch input.ReviewTask {
	case ReviewTaskPreRenderPlan:
		if input.Target.RenderPlanID == "" {
			return TaskInput{}, ErrInvalidInput
		}
	case ReviewTaskPreviewImage, ReviewTaskShotVideo:
		if input.ShotID == "" || input.NodeID == "" || input.ArtifactVersionID == "" {
			return TaskInput{}, ErrInvalidInput
		}
	case ReviewTaskFinalVideo:
		if input.NodeID == "" || input.ArtifactVersionID == "" {
			return TaskInput{}, ErrInvalidInput
		}
	default:
		return TaskInput{}, ErrInvalidInput
	}
	if input.AttemptNo <= 0 {
		input.AttemptNo = 1
	}
	if input.MaxAttempts <= 0 {
		input.MaxAttempts = DefaultReviewPolicy().MaxAttempts
	}
	if input.MaxAttempts > DefaultReviewPolicy().MaxAttempts {
		input.MaxAttempts = DefaultReviewPolicy().MaxAttempts
	}
	return input, nil
}

func targetPhaseFromReviewTask(reviewTask string) string {
	switch reviewTask {
	case ReviewTaskPreRenderPlan:
		return TargetPhasePreRenderPlan
	case ReviewTaskShotVideo:
		return TargetPhaseShotVideo
	case ReviewTaskFinalVideo:
		return TargetPhaseFinalVideo
	case ReviewTaskPreviewImage:
		return TargetPhasePreviewImage
	default:
		return ""
	}
}

func (e *Executor) fail(ctx context.Context, taskID pgtype.UUID, code string, err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	_, _ = e.runtime.MarkTaskFailed(ctx, taskID, code, message)
	return fmt.Errorf("%s: %w", code, err)
}

func (e *Executor) persistNativeToolTrace(ctx context.Context, task db.AgentTask, messages []ReviewerSameTurnMessage) error {
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
			WorkspaceID: task.WorkspaceID,
			ThreadID:    task.ThreadID,
			Role:        role,
			MessageType: messageType,
			Content:     reviewerToolTraceContent(trace),
			RawMessage:  reviewerToolTraceRaw(trace),
			TaskID:      task.ID,
		})
		if err != nil {
			return err
		}
		e.broadcastMessage(task.WorkspaceID, msg, db.AgentEvent{})
		if messageType == "tool_call" {
			callMessages[trace.ToolCallID] = msg
		}
		if messageType == "tool_result" {
			if callMsg, ok := callMessages[trace.ToolCallID]; ok {
				updated, err := e.runtime.UpdateMessage(ctx, agentruntime.UpdateMessageParams{
					ID:         callMsg.ID,
					Content:    reviewerCompletedToolTraceContent(trace, callMsg.RawMessage),
					RawMessage: reviewerCompletedToolTraceRaw(trace, callMsg.RawMessage),
				})
				if err != nil {
					return err
				}
				e.broadcastMessageUpdated(task.WorkspaceID, updated, db.AgentEvent{})
			}
		}
	}
	return nil
}

func (e *Executor) persistAssistantText(ctx context.Context, task db.AgentTask, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	content, err := uimessage.BuildAssistantMessageContent(uimessage.AssistantMessageInput{Text: text})
	if err != nil {
		return err
	}
	msg, err := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: task.WorkspaceID,
		ThreadID:    task.ThreadID,
		Role:        "assistant",
		MessageType: "text",
		Content:     content,
		RawMessage:  mustJSON(map[string]any{"schema": "clipanvil.agent.assistant_text.v1", "text": text}),
		TaskID:      task.ID,
	})
	if err != nil {
		return err
	}
	e.broadcastMessage(task.WorkspaceID, msg, db.AgentEvent{})
	return nil
}

func reviewerToolTraceContent(trace ReviewerSameTurnMessage) []byte {
	if trace.MessageType == "tool_call" {
		return toolStatusContent(trace.ToolCallID, trace.ToolName, trace.ToolName, "running", toolTraceSummary(trace.ToolArguments, trace.Content), "", trace.ToolArguments, nil)
	}
	return mustJSON(map[string]any{"schema": "clipanvil.agent.tool_trace.v1", "message_type": strings.TrimSpace(trace.MessageType), "tool_call_id": trace.ToolCallID, "tool_name": trace.ToolName, "text": strings.TrimSpace(trace.Content)})
}

func reviewerCompletedToolTraceContent(trace ReviewerSameTurnMessage, previousRaw []byte) []byte {
	return reviewerCompletedToolTraceContentWithStatus(trace, previousRaw, "succeeded")
}

func reviewerCompletedToolTraceContentWithStatus(trace ReviewerSameTurnMessage, previousRaw []byte, status string) []byte {
	args := toolTraceArgumentsFromRaw(previousRaw)
	result := map[string]any{}
	if text := strings.TrimSpace(trace.Content); text != "" {
		result["text"] = text
	}
	return toolStatusContent(trace.ToolCallID, trace.ToolName, trace.ToolName, status, toolTraceSummary(args, trace.Content), "", args, result)
}

func reviewerCompletedToolTraceRaw(trace ReviewerSameTurnMessage, previousRaw []byte) []byte {
	return reviewerCompletedToolTraceRawWithStatus(trace, previousRaw, "succeeded")
}

func reviewerCompletedToolTraceRawWithStatus(trace ReviewerSameTurnMessage, previousRaw []byte, status string) []byte {
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

func reviewerToolTraceRaw(trace ReviewerSameTurnMessage) []byte {
	payload := map[string]any{
		"schema":       "clipanvil.agent.tool_trace.v1",
		"role":         trace.Role,
		"message_type": trace.MessageType,
		"tool_call_id": trace.ToolCallID,
		"tool_name":    trace.ToolName,
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

func defaultJSON(value []byte) []byte {
	if len(value) == 0 {
		return []byte("{}")
	}
	return value
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
