package craftsman

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ExecutorRuntime interface {
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type Runner interface {
	Run(ctx context.Context, input GraphInput) (GraphOutput, error)
}

type ExecutorConfig struct {
	Runtime ExecutorRuntime
	Graph   Runner
	Logger  *slog.Logger
}

type Executor struct {
	runtime ExecutorRuntime
	graph   Runner
	logger  *slog.Logger
}

func NewExecutor(config ExecutorConfig) *Executor {
	return &Executor{runtime: config.Runtime, graph: config.Graph, logger: config.Logger}
}

func (e *Executor) RunTask(ctx context.Context, input RunTaskInput) error {
	if e == nil || e.runtime == nil || e.graph == nil || !input.WorkspaceID.Valid || !input.ThreadID.Valid || !input.TaskID.Valid || !input.ShotID.Valid {
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
		Scope:       mustJSON(map[string]any{"shot_id": uuidString(input.ShotID)}),
	})
	out, err := e.graph.Run(ctx, GraphInput{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		ShotID:      input.ShotID,
		MaxAttempts: 3,
	})
	if err != nil {
		return e.fail(ctx, input, "craftsman_failed", err)
	}
	rawOutput, _ := json.Marshal(map[string]any{
		"strategy":         out.Strategy.Strategy,
		"preview_prompt":   out.Strategy.PreviewPrompt,
		"worker_task_id":   uuidString(out.WorkerTask.ID),
		"checkpoint_key":   out.Metadata["checkpoint_key"],
		"worker_task_type": out.WorkerTask.TaskType,
	})
	if _, err := e.runtime.MarkTaskSucceeded(ctx, input.TaskID, rawOutput); err != nil {
		return err
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
	return nil
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
		"shot_id", uuidString(input.ShotID),
		"error_code", code,
		"error", message,
	)
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

func (e *Executor) loggerOrDefault() *slog.Logger {
	if e != nil && e.logger != nil {
		return e.logger
	}
	return slog.Default()
}
