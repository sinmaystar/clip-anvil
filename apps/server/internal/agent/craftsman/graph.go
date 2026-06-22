package craftsman

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/compose"
	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	agentworker "github.com/sinmaystar/clip-anvil/internal/agent/worker"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Loader interface {
	Load(ctx context.Context, input GraphInput) (Context, error)
}

type Runtime interface {
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
	UpsertCheckpoint(ctx context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error)
	SetThreadCheckpoint(ctx context.Context, threadID pgtype.UUID, checkpointKey string) (db.AgentThread, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type WorkerTaskEnqueuer interface {
	EnqueueWorkerTask(ctx context.Context, task db.AgentTask)
}

type GraphConfig struct {
	Loader         Loader
	Responder      ModelResponder
	Runtime        Runtime
	WorkerEnqueuer WorkerTaskEnqueuer
}

type Graph struct {
	runnable compose.Runnable[GraphInput, GraphOutput]
}

func NewGraph(config GraphConfig) (*Graph, error) {
	if config.Loader == nil || config.Responder == nil || config.Runtime == nil {
		return nil, ErrInvalidConfig
	}
	g := compose.NewGraph[GraphInput, GraphOutput]()
	if err := g.AddLambdaNode("load_shot_context", compose.InvokableLambda(func(ctx context.Context, input GraphInput) (Context, error) {
		return config.Loader.Load(ctx, input)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("draft_preview_strategy", compose.InvokableLambda(func(ctx context.Context, craftsmanContext Context) (GraphOutput, error) {
		return runCraftsmanStrategy(ctx, config, craftsmanContext)
	})); err != nil {
		return nil, err
	}
	if err := g.AddEdge(compose.START, "load_shot_context"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("load_shot_context", "draft_preview_strategy"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("draft_preview_strategy", compose.END); err != nil {
		return nil, err
	}
	runnable, err := g.Compile(context.Background(), compose.WithGraphName("craftsman_preview"))
	if err != nil {
		return nil, err
	}
	return &Graph{runnable: runnable}, nil
}

func (g *Graph) Run(ctx context.Context, input GraphInput) (GraphOutput, error) {
	if g == nil || g.runnable == nil {
		return GraphOutput{}, ErrInvalidConfig
	}
	return g.runnable.Invoke(ctx, input)
}

func runCraftsmanStrategy(ctx context.Context, config GraphConfig, craftsmanContext Context) (GraphOutput, error) {
	input := craftsmanContext.Input
	attempts := maxAttempts(input.MaxAttempts)
	var strategy Strategy
	var metadata map[string]any
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		strategy, metadata, err = config.Responder.DraftPreviewStrategy(ctx, craftsmanContext)
		if err == nil {
			err = ValidateStrategy(strategy)
		}
		if err == nil {
			break
		}
	}
	if err != nil {
		return GraphOutput{}, fmt.Errorf("craftsman_strategy_invalid: %w", err)
	}
	rawStrategy := mustJSON(strategy)
	if _, err := config.Runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        "assistant",
		MessageType: "text",
		Content: mustJSON(map[string]any{
			"text":           strategy.Strategy,
			"preview_prompt": strategy.PreviewPrompt,
		}),
		RawMessage: rawStrategy,
		TaskID:     input.TaskID,
	}); err != nil {
		return GraphOutput{}, err
	}
	checkpointKey := CheckpointKey(input.WorkspaceID, input.ShotID, input.TaskID)
	if _, err := config.Runtime.UpsertCheckpoint(ctx, agentruntime.UpsertCheckpointParams{
		Key:         checkpointKey,
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		Value:       rawStrategy,
		Metadata:    mustJSON(map[string]any{"kind": "craftsman_preview_strategy"}),
	}); err != nil {
		return GraphOutput{}, err
	}
	_, _ = config.Runtime.SetThreadCheckpoint(ctx, input.ThreadID, checkpointKey)
	workerInput := agentworker.GenerationInput{
		Mode:              "preview_image",
		ShotID:            uuidString(input.ShotID),
		ShotClientKey:     craftsmanContext.Shot.ClientKey,
		CraftsmanThreadID: uuidString(input.ThreadID),
		CraftsmanTaskID:   uuidString(input.TaskID),
		Strategy:          strategy.Strategy,
		Prompt:            strategy.PreviewPrompt,
		NegativePrompt:    strategy.NegativePrompt,
		InputNodeRefs:     strategy.InputNodeRefs,
		Model:             agentworker.ModelSpec{Provider: strategy.Model.Provider, ModelID: strategy.Model.ModelID},
		Params:            strategy.Params,
		MaxAttempts:       attempts,
	}
	task, err := config.Runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        "worker",
		ScopeType:   "shot",
		ScopeID:     input.ShotID,
		TaskType:    "worker_generation",
		MaxAttempts: int32(attempts),
		Input:       mustJSON(workerInput),
	})
	if err != nil {
		return GraphOutput{}, err
	}
	if config.WorkerEnqueuer != nil {
		config.WorkerEnqueuer.EnqueueWorkerTask(ctx, task)
	}
	outMeta := map[string]any{
		"checkpoint_key": checkpointKey,
		"worker_task_id": uuidString(task.ID),
	}
	for key, value := range metadata {
		outMeta[key] = value
	}
	return GraphOutput{Strategy: strategy, WorkerTask: task, Metadata: outMeta}, nil
}

func CheckpointKey(workspaceID, shotID, taskID pgtype.UUID) string {
	return fmt.Sprintf("craftsman:%s:%s:%s", uuidString(workspaceID), uuidString(shotID), uuidString(taskID))
}

func maxAttempts(value int) int {
	if value <= 0 {
		return 3
	}
	if value > 3 {
		return 3
	}
	return value
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
