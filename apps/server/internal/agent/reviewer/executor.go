package reviewer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/callbacks"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/attribute"

	"github.com/sinmaystar/clip-anvil/internal/agent/cozelooptrace"
	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ExecutorRuntime interface {
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
	SetThreadCheckpoint(ctx context.Context, threadID pgtype.UUID, checkpointKey string) (db.AgentThread, error)
}

type Runner interface {
	Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error)
}

type ExecutorConfig struct {
	Runtime        ExecutorRuntime
	Graph          Runner
	TraceCallbacks []callbacks.Handler
}

type Executor struct {
	runtime        ExecutorRuntime
	graph          Runner
	traceCallbacks []callbacks.Handler
}

func NewExecutor(config ExecutorConfig) *Executor {
	return &Executor{
		runtime:        config.Runtime,
		graph:          config.Graph,
		traceCallbacks: config.TraceCallbacks,
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
	out, err := e.graph.Run(ctx, graphInput, agenteino.RunOptions{
		CheckPointID: checkpointKey,
		Callbacks:    e.traceCallbacks,
	})
	if err != nil {
		return e.fail(ctx, task.ID, "reviewer_failed", err)
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
	return nil
}

func parseTaskInput(raw []byte) (TaskInput, error) {
	var input TaskInput
	if err := json.Unmarshal(defaultJSON(raw), &input); err != nil {
		return TaskInput{}, err
	}
	if input.TargetPhase == "" {
		input.TargetPhase = TargetPhasePreviewImage
	}
	if input.TargetPhase != TargetPhasePreviewImage || input.ShotID == "" || input.NodeID == "" || input.ArtifactVersionID == "" {
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

func (e *Executor) fail(ctx context.Context, taskID pgtype.UUID, code string, err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	_, _ = e.runtime.MarkTaskFailed(ctx, taskID, code, message)
	return fmt.Errorf("%s: %w", code, err)
}

func defaultJSON(value []byte) []byte {
	if len(value) == 0 {
		return []byte("{}")
	}
	return value
}
