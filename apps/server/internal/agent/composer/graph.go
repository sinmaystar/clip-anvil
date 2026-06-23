package composer

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	agentworker "github.com/sinmaystar/clip-anvil/internal/agent/worker"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type GraphConfig struct {
	Runtime     Runtime
	Store       Store
	Production  ProductionSubmitter
	Broadcaster NodeBroadcaster
}

type Graph struct {
	runnable compose.Runnable[GraphInput, GraphOutput]
}

type graphNodeState struct {
	Input GraphInput
	Node  db.MediaNode
}

type graphSubmissionState struct {
	Input  GraphInput
	Node   db.MediaNode
	Output CompositionOutput
}

func NewGraph(config GraphConfig) (*Graph, error) {
	if config.Runtime == nil || config.Store == nil || config.Production == nil {
		return nil, ErrInvalidConfig
	}
	g := compose.NewGraph[GraphInput, GraphOutput]()
	if err := g.AddLambdaNode("load_composition_context", compose.InvokableLambda(func(ctx context.Context, input GraphInput) (GraphInput, error) {
		return loadCompositionContext(ctx, config, input)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("create_final_node", compose.InvokableLambda(func(ctx context.Context, input GraphInput) (graphNodeState, error) {
		return createGraphFinalNode(ctx, config, input)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("submit_composition_intent", compose.InvokableLambda(func(ctx context.Context, state graphNodeState) (graphSubmissionState, error) {
		return submitCompositionIntent(ctx, config, state)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("persist_checkpoint_and_events", compose.InvokableLambda(func(ctx context.Context, state graphSubmissionState) (GraphOutput, error) {
		return persistCompositionResult(ctx, config, state)
	})); err != nil {
		return nil, err
	}
	if err := g.AddEdge(compose.START, "load_composition_context"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("load_composition_context", "create_final_node"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("create_final_node", "submit_composition_intent"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("submit_composition_intent", "persist_checkpoint_and_events"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("persist_checkpoint_and_events", compose.END); err != nil {
		return nil, err
	}
	runnable, err := g.Compile(context.Background(), compose.WithGraphName("composer_final"))
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

func loadCompositionContext(ctx context.Context, config GraphConfig, input GraphInput) (GraphInput, error) {
	if !input.WorkspaceID.Valid || !input.ThreadID.Valid || !input.TaskID.Valid {
		return GraphInput{}, ErrInvalidConfig
	}
	if len(input.Input.VideoNodeRefs) == 0 {
		return GraphInput{}, ErrInvalidInput
	}
	_, _ = config.Runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "composer_started",
		SourceRole:  "composer",
		Payload:     mustJSON(map[string]any{"video_count": len(input.Input.VideoNodeRefs)}),
	})
	return input, nil
}

func createGraphFinalNode(ctx context.Context, config GraphConfig, input GraphInput) (graphNodeState, error) {
	modelParams, _ := json.Marshal(input.Input.Params)
	canvasX, canvasY := finalVideoNodePosition(len(input.Input.VideoNodeRefs))
	node, err := config.Store.CreateAgentGenerationNode(ctx, db.CreateAgentGenerationNodeParams{
		WorkspaceID:   input.WorkspaceID,
		NodeType:      db.NodeTypeVideo,
		Title:         "Agent final video",
		Prompt:        strings.TrimSpace(input.Input.Strategy),
		OperationType: "compose_final_video",
		CanvasX:       canvasX,
		CanvasY:       canvasY,
		CanvasW:       520,
		CanvasH:       300,
		ShotID:        pgtype.UUID{},
		ModelProvider: pgtype.Text{String: "internal_ffmpeg", Valid: true},
		ModelID:       pgtype.Text{String: "ffmpeg-compose", Valid: true},
		ModelParams:   defaultJSON(modelParams),
		Metadata: mustJSON(map[string]any{
			"agent_artifact_kind": "final_video",
			"source_phase":        "shot_video",
			"composer_task_id":    uuidString(input.TaskID),
		}),
	})
	if err != nil {
		return graphNodeState{}, err
	}
	if config.Broadcaster != nil {
		config.Broadcaster.BroadcastAgentNodeCreated(input.WorkspaceID, node)
	}
	return graphNodeState{Input: input, Node: node}, nil
}

func finalVideoNodePosition(videoCount int) (float32, float32) {
	const (
		startX  = 140
		startY  = 140
		columns = 3
		stepY   = 900
	)
	if videoCount <= 0 {
		videoCount = 1
	}
	rows := (videoCount + columns - 1) / columns
	return startX, float32(startY + rows*stepY)
}

func submitCompositionIntent(ctx context.Context, config GraphConfig, state graphNodeState) (graphSubmissionState, error) {
	inputRefs, err := agentworker.ResolveInputRefs(ctx, config.Store, state.Input.WorkspaceID, state.Node.ID, state.Input.Input.VideoNodeRefs)
	if err != nil {
		return graphSubmissionState{}, err
	}
	intent := production.GenerationIntent{
		WorkspaceID:    state.Input.WorkspaceID,
		TargetNodeID:   state.Node.ID,
		OutputType:     "video",
		OperationType:  "compose_final_video",
		PromptTemplate: strings.TrimSpace(state.Input.Input.Strategy),
		RenderedPrompt: strings.TrimSpace(state.Input.Input.Strategy),
		InputRefs:      inputRefs,
		Model:          production.ModelSpec{Provider: "internal_ffmpeg", ModelID: "ffmpeg-compose"},
		Params:         defaultParams(state.Input.Input.Params),
		RequestedBy: production.RequestedBy{
			Type: "agent_composer",
			ID:   uuidString(state.Input.TaskID),
		},
	}
	result, err := config.Production.SubmitGenerationIntent(ctx, intent, production.RunOptions{MaxAttempts: 1})
	if err != nil {
		return graphSubmissionState{}, err
	}
	output := CompositionOutput{
		Status:            "submitted",
		NodeID:            uuidString(result.Node.ID),
		GenerationJobID:   uuidString(result.Job.ID),
		ArtifactVersionID: uuidString(result.Version.ID),
		OperationType:     result.Job.OperationType,
	}
	return graphSubmissionState{Input: state.Input, Node: state.Node, Output: output}, nil
}

func persistCompositionResult(ctx context.Context, config GraphConfig, state graphSubmissionState) (GraphOutput, error) {
	checkpointKey := CheckpointKey(state.Input.WorkspaceID, state.Input.ThreadID, state.Input.TaskID)
	checkpointValue := mustJSON(map[string]any{
		"node_id":             state.Output.NodeID,
		"generation_job_id":   state.Output.GenerationJobID,
		"artifact_version_id": state.Output.ArtifactVersionID,
		"operation_type":      state.Output.OperationType,
		"status":              state.Output.Status,
	})
	if _, err := config.Runtime.UpsertCheckpoint(ctx, agentruntime.UpsertCheckpointParams{
		Key:         checkpointKey,
		WorkspaceID: state.Input.WorkspaceID,
		ThreadID:    state.Input.ThreadID,
		TaskID:      state.Input.TaskID,
		Value:       checkpointValue,
		Metadata:    mustJSON(map[string]any{"kind": "composer_result"}),
	}); err != nil {
		return GraphOutput{}, err
	}
	_, _ = config.Runtime.SetThreadCheckpoint(ctx, state.Input.ThreadID, checkpointKey)
	rawOutput := mustJSON(state.Output)
	_, _ = config.Runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: state.Input.WorkspaceID,
		ThreadID:    state.Input.ThreadID,
		TaskID:      state.Input.TaskID,
		EventType:   "composition_submitted",
		SourceRole:  "composer",
		TargetRole:  "producer",
		Payload:     rawOutput,
	})
	return GraphOutput{Output: state.Output, CheckpointKey: checkpointKey}, nil
}

func CheckpointKey(workspaceID, threadID, taskID pgtype.UUID) string {
	return "composer:" + uuidString(workspaceID) + ":" + uuidString(threadID) + ":" + uuidString(taskID)
}
