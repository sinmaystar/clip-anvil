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
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ExecutorRuntime interface {
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
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
	if err := e.persistNativeToolTrace(ctx, task, out.SameTurnMessages); err != nil {
		return e.fail(ctx, task.ID, "reviewer_tool_trace_persist_failed", err)
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
		if _, err := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
			WorkspaceID: task.WorkspaceID,
			ThreadID:    task.ThreadID,
			Role:        role,
			MessageType: messageType,
			Content:     reviewerToolTraceContent(trace),
			RawMessage:  reviewerToolTraceRaw(trace),
			TaskID:      task.ID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func reviewerToolTraceContent(trace ReviewerSameTurnMessage) []byte {
	return mustJSON(map[string]any{
		"schema":       "clipanvil.agent.tool_trace.v1",
		"message_type": strings.TrimSpace(trace.MessageType),
		"tool_call_id": trace.ToolCallID,
		"tool_name":    trace.ToolName,
		"text":         strings.TrimSpace(trace.Content),
	})
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
