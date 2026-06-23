package producer

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ToolRuntime interface {
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
	UpdateMessage(ctx context.Context, params agentruntime.UpdateMessageParams) (db.AgentMessage, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
}

type RegistryToolExecutorConfig struct {
	Registry    *agenttools.Registry
	Runtime     ToolRuntime
	Broadcaster Broadcaster
}

type RegistryToolExecutor struct {
	registry    *agenttools.Registry
	runtime     ToolRuntime
	broadcaster Broadcaster
}

func NewRegistryToolExecutor(config RegistryToolExecutorConfig) *RegistryToolExecutor {
	return &RegistryToolExecutor{
		registry:    config.Registry,
		runtime:     config.Runtime,
		broadcaster: config.Broadcaster,
	}
}

func (e *RegistryToolExecutor) ExecuteProducerTool(ctx context.Context, producerContext ProducerContext, call ToolCall) (ToolExecutionResult, error) {
	if e == nil || e.registry == nil || e.runtime == nil {
		return ToolExecutionResult{}, NewAgentError("agent_tool_executor_missing", "producer tool executor is not configured")
	}
	tool, ok := e.registry.Executor(call.Name)
	if !ok {
		return ToolExecutionResult{}, NewAgentError("agent_tool_not_found", "agent tool is not registered")
	}
	definition := tool.Definition()
	toolCallID := call.ID
	if toolCallID == "" {
		toolCallID = uuid.NewString()
	}
	toolTask, err := e.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: producerContext.Input.WorkspaceID,
		ThreadID:    producerContext.Input.ThreadID,
		Role:        "producer",
		ScopeType:   "workspace",
		TaskType:    "tool_call",
		MaxAttempts: 1,
		Input: mustJSON(map[string]any{
			"tool_call_id": toolCallID,
			"name":         call.Name,
			"arguments":    call.Arguments,
			"turn_task_id": uuidString(producerContext.Input.TaskID),
		}),
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	e.broadcastTask(producerContext.Input.WorkspaceID, toolTask)

	started, err := e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: producerContext.Input.WorkspaceID,
		ThreadID:    producerContext.Input.ThreadID,
		TaskID:      toolTask.ID,
		EventType:   "tool_call_started",
		SourceRole:  "producer",
		TargetRole:  "system",
		Payload:     toolEnvelope(toolCallID, call, "started", nil),
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	e.broadcastEvent(producerContext.Input.WorkspaceID, started)

	callMessage, err := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: producerContext.Input.WorkspaceID,
		ThreadID:    producerContext.Input.ThreadID,
		Role:        "assistant",
		MessageType: "tool_call",
		Content:     toolStatusContent(toolCallID, call.Name, definition.Visibility.UserLabel, "running", "", "", call.Arguments, nil),
		RawMessage:  toolEnvelope(toolCallID, call, "started", nil),
		TaskID:      toolTask.ID,
		EventID:     started.ID,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	e.broadcastMessage(producerContext.Input.WorkspaceID, callMessage, started)

	running, err := e.runtime.MarkTaskRunning(ctx, toolTask.ID)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	e.broadcastTask(producerContext.Input.WorkspaceID, running)

	toolCtx := ctx
	cancel := func() {}
	if producerContext.Input.ToolTimeout > 0 {
		toolCtx, cancel = context.WithTimeout(ctx, producerContext.Input.ToolTimeout)
	}
	defer cancel()

	output, err := tool.Execute(toolCtx, agenttools.ExecuteInput{
		WorkspaceID: producerContext.Input.WorkspaceID,
		ThreadID:    producerContext.Input.ThreadID,
		TaskID:      producerContext.Input.TaskID,
		Arguments:   call.Arguments,
	})
	if err != nil {
		return ToolExecutionResult{}, e.failTool(ctx, producerContext, toolTask, callMessage, toolCallID, call, definition.Visibility.UserLabel, err)
	}

	completed, err := e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: producerContext.Input.WorkspaceID,
		ThreadID:    producerContext.Input.ThreadID,
		TaskID:      toolTask.ID,
		EventType:   "tool_call_completed",
		SourceRole:  "system",
		TargetRole:  "producer",
		Payload:     toolEnvelope(toolCallID, call, "succeeded", output.Result),
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	e.broadcastEvent(producerContext.Input.WorkspaceID, completed)

	resultMessage, err := e.runtime.UpdateMessage(ctx, agentruntime.UpdateMessageParams{
		ID:         callMessage.ID,
		Content:    toolStatusContent(toolCallID, call.Name, definition.Visibility.UserLabel, "succeeded", "", "", call.Arguments, output.Result),
		RawMessage: toolEnvelope(toolCallID, call, "succeeded", output.Result),
		EventID:    completed.ID,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	e.broadcastMessageUpdated(producerContext.Input.WorkspaceID, resultMessage, completed)

	succeeded, err := e.runtime.MarkTaskSucceeded(ctx, toolTask.ID, toolEnvelope(toolCallID, call, "succeeded", output.Result))
	if err != nil {
		return ToolExecutionResult{}, err
	}
	e.broadcastTask(producerContext.Input.WorkspaceID, succeeded)

	return ToolExecutionResult{
		Result:      output.Result,
		Summary:     strings.TrimSpace(output.Summary),
		Interrupted: definition.Safety.RequiresHITL,
		ToolCallID:  toolCallID,
		ToolName:    call.Name,
	}, nil
}

func (e *RegistryToolExecutor) failTool(ctx context.Context, producerContext ProducerContext, toolTask db.AgentTask, callMessage db.AgentMessage, toolCallID string, call ToolCall, label string, cause error) error {
	failedEvent, eventErr := e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: producerContext.Input.WorkspaceID,
		ThreadID:    producerContext.Input.ThreadID,
		TaskID:      toolTask.ID,
		EventType:   "tool_call_failed",
		SourceRole:  "system",
		TargetRole:  "producer",
		Payload: mustJSON(map[string]any{
			"tool_call_id": toolCallID,
			"name":         call.Name,
			"status":       "failed",
			"error":        cause.Error(),
		}),
	})
	if eventErr == nil {
		e.broadcastEvent(producerContext.Input.WorkspaceID, failedEvent)
		if updated, updateErr := e.runtime.UpdateMessage(ctx, agentruntime.UpdateMessageParams{
			ID:         callMessage.ID,
			Content:    toolStatusContent(toolCallID, call.Name, label, "failed", "", cause.Error(), call.Arguments, nil),
			RawMessage: toolEnvelope(toolCallID, call, "failed", map[string]any{"error": cause.Error()}),
			EventID:    failedEvent.ID,
		}); updateErr == nil {
			e.broadcastMessageUpdated(producerContext.Input.WorkspaceID, updated, failedEvent)
		} else {
			eventErr = errors.Join(eventErr, updateErr)
		}
	}
	failed, err := e.runtime.MarkTaskFailed(ctx, toolTask.ID, errorCode(cause, "agent_tool_failed"), cause.Error())
	if err == nil {
		e.broadcastTask(producerContext.Input.WorkspaceID, failed)
	}
	return errors.Join(cause, eventErr, err)
}

func (e *RegistryToolExecutor) broadcastMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent) {
	if e.broadcaster != nil {
		e.broadcaster.BroadcastAgentMessage(workspaceID, message, event)
	}
}

func (e *RegistryToolExecutor) broadcastMessageUpdated(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent) {
	if e.broadcaster != nil {
		e.broadcaster.BroadcastAgentMessageUpdated(workspaceID, message, event)
	}
}

func (e *RegistryToolExecutor) broadcastTask(workspaceID pgtype.UUID, task db.AgentTask) {
	if e.broadcaster != nil {
		e.broadcaster.BroadcastAgentTask(workspaceID, task)
	}
}

func (e *RegistryToolExecutor) broadcastEvent(workspaceID pgtype.UUID, event db.AgentEvent) {
	if e.broadcaster != nil {
		e.broadcaster.BroadcastAgentEvent(workspaceID, event)
	}
}

func toolEnvelope(toolCallID string, call ToolCall, status string, result map[string]any) []byte {
	payload := map[string]any{
		"tool_call_id": toolCallID,
		"name":         call.Name,
		"arguments":    call.Arguments,
		"status":       status,
		"source":       "producer_model",
	}
	if result != nil {
		payload["result"] = result
	}
	return mustJSON(payload)
}

func toolStatusContent(toolCallID string, toolName string, label string, status string, summary string, errorMessage string, arguments map[string]any, result map[string]any) []byte {
	raw, err := uimessage.BuildToolStatusMessageContent(uimessage.ToolStatusInput{
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
		return []byte("{}")
	}
	return raw
}
