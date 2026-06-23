package composer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

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

type Store interface {
	CreateAgentGenerationNode(ctx context.Context, params db.CreateAgentGenerationNodeParams) (db.MediaNode, error)
	ListMediaNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
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
	Runtime Runtime
	Graph   Runner
}

type Executor struct {
	runtime Runtime
	graph   Runner
}

func NewExecutor(config ExecutorConfig) *Executor {
	return &Executor{runtime: config.Runtime, graph: config.Graph}
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
	out, err := e.graph.Run(ctx, graphInput, agenteino.RunOptions{CheckPointID: checkpointKey})
	if err != nil {
		return e.fail(ctx, task, "composer_failed", err)
	}
	rawOutput, _ := json.Marshal(out.Output)
	if _, err := e.runtime.MarkTaskSucceeded(ctx, task.ID, rawOutput); err != nil {
		return err
	}
	if _, err := e.runtime.SetThreadCheckpoint(ctx, task.ThreadID, checkpointKey); err != nil {
		return e.fail(ctx, task, "composer_checkpoint_update_failed", err)
	}
	return nil
}

func parseCompositionInput(raw []byte) (CompositionInput, error) {
	var input CompositionInput
	if err := json.Unmarshal(defaultJSON(raw), &input); err != nil {
		return CompositionInput{}, err
	}
	if len(input.VideoNodeRefs) == 0 {
		return CompositionInput{}, ErrInvalidInput
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
