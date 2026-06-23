package craftsman

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ExecutorRuntime interface {
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
	SetThreadCheckpoint(ctx context.Context, threadID pgtype.UUID, checkpointKey string) (db.AgentThread, error)
}

type Runner interface {
	Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error)
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
	taskInput, err := parseTaskInput(input.Input)
	if err != nil {
		return e.fail(ctx, input, "craftsman_invalid_input", err)
	}
	graphInput := GraphInput{
		WorkspaceID:  input.WorkspaceID,
		ThreadID:     input.ThreadID,
		TaskID:       input.TaskID,
		ShotID:       input.ShotID,
		Mode:         taskInput.Mode,
		MaxAttempts:  taskInput.MaxAttempts,
		WorkerParams: taskInput.WorkerParams,
	}
	checkpointKey := agenteino.CheckpointKey("craftsman_generation", input.WorkspaceID, input.ThreadID, input.TaskID)
	out, err := e.graph.Run(ctx, graphInput, agenteino.RunOptions{CheckPointID: checkpointKey})
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
	return nil
}

type parsedTaskInput struct {
	Mode         string
	MaxAttempts  int
	WorkerParams map[string]any
}

func parseTaskInput(raw []byte) (parsedTaskInput, error) {
	out := parsedTaskInput{
		Mode:         "preview_image",
		MaxAttempts:  3,
		WorkerParams: map[string]any{},
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
