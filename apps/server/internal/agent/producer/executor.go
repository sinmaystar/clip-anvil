package producer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Runtime interface {
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type Runner interface {
	Run(ctx context.Context, input ProducerTurnInput) (ProducerTurnOutput, error)
}

type Broadcaster interface {
	BroadcastAgentMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent)
	BroadcastAgentMessageUpdated(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent)
	BroadcastAgentTask(workspaceID pgtype.UUID, task db.AgentTask)
	BroadcastAgentEvent(workspaceID pgtype.UUID, event db.AgentEvent)
	BroadcastAgentMessageDelta(workspaceID pgtype.UUID, delta ProducerStreamDelta)
}

type ExecutorConfig struct {
	Runtime      Runtime
	Graph        Runner
	Broadcaster  Broadcaster
	MaxToolCalls int
	ToolTimeout  time.Duration
	Logger       *slog.Logger
}

type Executor struct {
	runtime      Runtime
	graph        Runner
	broadcaster  Broadcaster
	maxToolCalls int
	toolTimeout  time.Duration
	logger       *slog.Logger
}

type RunTaskInput struct {
	WorkspaceID      pgtype.UUID
	ThreadID         pgtype.UUID
	TaskID           pgtype.UUID
	TriggerMessageID pgtype.UUID
}

func NewExecutor(config ExecutorConfig) *Executor {
	maxToolCalls := config.MaxToolCalls
	if maxToolCalls <= 0 {
		maxToolCalls = 50
	}
	toolTimeout := config.ToolTimeout
	if toolTimeout <= 0 {
		toolTimeout = 300 * time.Second
	}
	return &Executor{
		runtime:      config.Runtime,
		graph:        config.Graph,
		broadcaster:  config.Broadcaster,
		maxToolCalls: maxToolCalls,
		toolTimeout:  toolTimeout,
		logger:       config.Logger,
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
		WorkspaceID:      input.WorkspaceID,
		ThreadID:         input.ThreadID,
		TaskID:           input.TaskID,
		TriggerMessageID: input.TriggerMessageID,
		MaxToolCalls:     e.maxToolCalls,
		ToolTimeout:      e.toolTimeout,
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
	output, err := e.graph.Run(ctx, graphInput)
	if err != nil {
		return e.failTask(ctx, input, errorCode(err, "producer_turn_failed"), err.Error())
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

	succeededTask, err := e.runtime.MarkTaskSucceeded(ctx, input.TaskID, mustJSON(output.Metadata))
	if err != nil {
		return err
	}

	e.broadcastMessage(input.WorkspaceID, msg, completed)
	e.broadcastEvent(input.WorkspaceID, completed)
	e.broadcastTask(input.WorkspaceID, succeededTask)
	return nil
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

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
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
