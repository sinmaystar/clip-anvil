package composer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/callbacks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/attribute"

	"github.com/sinmaystar/clip-anvil/internal/agent/cozelooptrace"
	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Runtime interface {
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
	UpsertCheckpoint(ctx context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error)
	SetThreadCheckpoint(ctx context.Context, threadID pgtype.UUID, checkpointKey string) (db.AgentThread, error)
}

type producerSignalRuntime interface {
	GetOrCreateProducerThread(ctx context.Context, workspaceID pgtype.UUID) (db.AgentThread, error)
	CreateProducerPendingSignal(ctx context.Context, params agentruntime.CreateProducerPendingSignalParams) (db.ProducerPendingSignal, error)
}

type Store interface {
	CreateAgentGenerationNode(ctx context.Context, params db.CreateAgentGenerationNodeParams) (db.MediaNode, error)
	ListMediaNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
	ListActiveShotsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Shot, error)
	ListMediaNodesByShot(ctx context.Context, params db.ListMediaNodesByShotParams) ([]db.MediaNode, error)
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
	GetMediaAssetByID(ctx context.Context, id pgtype.UUID) (db.MediaAsset, error)
	GetDependencyEdgeByEndpoints(ctx context.Context, params db.GetDependencyEdgeByEndpointsParams) (db.MediaEdge, error)
	CreateMediaEdge(ctx context.Context, params db.CreateMediaEdgeParams) (db.MediaEdge, error)
}

type ProductionSubmitter interface {
	SubmitGenerationIntent(ctx context.Context, intent production.GenerationIntent, options production.RunOptions) (production.RunResult, error)
}

type NodeBroadcaster interface {
	BroadcastAgentNodeCreated(workspaceID pgtype.UUID, node db.MediaNode)
}

type Runner interface {
	Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error)
}

type ExecutorConfig struct {
	Runtime        Runtime
	Graph          Runner
	TraceCallbacks []callbacks.Handler
}

type Executor struct {
	runtime        Runtime
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
	if task.TaskType != "composer_turn" {
		return fmt.Errorf("%w: unsupported task type %q", ErrInvalidInput, task.TaskType)
	}
	if _, err := e.runtime.MarkTaskRunning(ctx, task.ID); err != nil {
		return err
	}
	compositionInput, err := parseCompositionInput(task.Input)
	if err != nil {
		return e.fail(ctx, task, "composer_invalid_input", err)
	}
	graphInput := GraphInput{
		WorkspaceID: task.WorkspaceID,
		ThreadID:    task.ThreadID,
		TaskID:      task.ID,
		Input:       compositionInput,
	}
	checkpointKey := agenteino.CheckpointKey("composer_final", task.WorkspaceID, task.ThreadID, task.ID)
	ctx = cozelooptrace.ContextWithAttributes(ctx,
		attribute.String("clipanvil.workspace_id", uuidString(task.WorkspaceID)),
		attribute.String("clipanvil.agent.thread_id", uuidString(task.ThreadID)),
		attribute.String("clipanvil.agent.task_id", uuidString(task.ID)),
		attribute.String("clipanvil.agent.role", "composer"),
		attribute.String("clipanvil.agent.task_type", task.TaskType),
		attribute.String("clipanvil.agent.scope_type", task.ScopeType),
		attribute.String("clipanvil.agent.scope_id", uuidString(task.ScopeID)),
	)
	out, err := e.graph.Run(ctx, graphInput, agenteino.RunOptions{
		CheckPointID: checkpointKey,
		Callbacks:    e.traceCallbacks,
	})
	if err != nil {
		return e.fail(ctx, task, "composer_failed", err)
	}
	rawOutput, _ := json.Marshal(out.Output)
	if _, err := e.runtime.MarkTaskSucceeded(ctx, task.ID, rawOutput); err != nil {
		return err
	}
	if err := e.signalProducer(ctx, task, out.Output); err != nil {
		return e.fail(ctx, task, "composer_signal_failed", err)
	}
	if _, err := e.runtime.SetThreadCheckpoint(ctx, task.ThreadID, checkpointKey); err != nil {
		return e.fail(ctx, task, "composer_checkpoint_update_failed", err)
	}
	return nil
}

func (e *Executor) signalProducer(ctx context.Context, task db.AgentTask, output CompositionOutput) error {
	signalType := composerSignalType(output.Status)
	if signalType == "" {
		return nil
	}
	runtime, ok := e.runtime.(producerSignalRuntime)
	if !ok {
		return nil
	}
	producerThread, err := runtime.GetOrCreateProducerThread(ctx, task.WorkspaceID)
	if err != nil {
		return err
	}
	scopeID, _ := pgUUIDFromString(output.NodeID)
	payload := mustJSON(map[string]any{
		"status":              output.Status,
		"timeline_plan_id":    output.TimelinePlanID,
		"output_node_id":      output.NodeID,
		"generation_job_id":   output.GenerationJobID,
		"artifact_version_id": output.ArtifactVersionID,
		"sandbox_job_id":      output.SandboxJobID,
		"operation_type":      output.OperationType,
	})
	_, err = runtime.CreateProducerPendingSignal(ctx, agentruntime.CreateProducerPendingSignalParams{
		WorkspaceID:      task.WorkspaceID,
		ProducerThreadID: producerThread.ID,
		SourceRole:       "composer",
		SourceTaskID:     task.ID,
		SourceThreadID:   task.ThreadID,
		SignalType:       signalType,
		ScopeType:        "final_output",
		ScopeID:          scopeID,
		Priority:         80,
		DedupeKey:        "composer:" + uuidString(task.ID) + ":" + output.Status,
		Payload:          payload,
	})
	return err
}

func composerSignalType(status string) string {
	switch status {
	case "completed":
		return "composition_completed"
	case "blocked":
		return "composition_blocked"
	case "failed":
		return "composition_failed"
	default:
		return ""
	}
}

func parseCompositionInput(raw []byte) (CompositionInput, error) {
	var input CompositionInput
	if err := json.Unmarshal(defaultJSON(raw), &input); err != nil {
		return CompositionInput{}, err
	}
	if len(input.VideoNodeRefs) == 0 && strings.TrimSpace(input.SourceStoryboardNodeID) == "" {
		return CompositionInput{}, ErrInvalidInput
	}
	if input.Strategy == "" {
		input.Strategy = input.Instructions
	}
	if input.TemplateKey == "" {
		input.TemplateKey = "simple_concat"
	}
	return input, nil
}

func (e *Executor) fail(ctx context.Context, task db.AgentTask, code string, err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	_, _ = e.runtime.MarkTaskFailed(ctx, task.ID, code, message)
	_, _ = e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: task.WorkspaceID,
		ThreadID:    task.ThreadID,
		TaskID:      task.ID,
		EventType:   "composition_failed",
		SourceRole:  "composer",
		Payload:     mustJSON(map[string]any{"error_code": code, "error": message}),
	})
	return fmt.Errorf("%s: %w", code, err)
}

func defaultParams(params map[string]any) map[string]any {
	if params == nil {
		return map[string]any{}
	}
	return params
}

func defaultJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

func pgUUIDFromString(value string) (pgtype.UUID, bool) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

func shortUUID(id pgtype.UUID) string {
	value := uuidString(id)
	if len(value) >= 8 {
		return value[:8]
	}
	return "unknown"
}
