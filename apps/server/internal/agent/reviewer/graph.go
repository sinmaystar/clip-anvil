package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/jackc/pgx/v5/pgtype"

	agenteino "github.com/sinmaystar/clip-anvil/internal/agent/einoruntime"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Runtime interface {
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
	UpsertCheckpoint(ctx context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error)
	SetThreadCheckpoint(ctx context.Context, threadID pgtype.UUID, checkpointKey string) (db.AgentThread, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type ReviewStore interface {
	CreateReviewRecord(ctx context.Context, params db.CreateReviewRecordParams) (db.ReviewRecord, error)
	CompleteReviewRecord(ctx context.Context, params db.CompleteReviewRecordParams) (db.ReviewRecord, error)
	FailReviewRecord(ctx context.Context, params db.FailReviewRecordParams) (db.ReviewRecord, error)
}

type VersionSelector interface {
	SelectArtifactVersion(ctx context.Context, nodeID, versionID pgtype.UUID) (production.ArtifactSelectionResult, error)
}

type GraphConfig struct {
	Loader           Loader
	Responder        ModelResponder
	Runtime          Runtime
	Store            ReviewStore
	Selector         VersionSelector
	RetryDispatcher  RetryDispatcher
	Dependency       DependencyNotifier
	Policy           ReviewPolicy
	CheckPointStore  compose.CheckPointStore
	CompileCallbacks []compose.GraphCompileCallback
}

type Graph struct {
	runnable compose.Runnable[GraphInput, GraphOutput]
}

func NewGraph(config GraphConfig) (*Graph, error) {
	if config.Loader == nil || config.Responder == nil || config.Runtime == nil || config.Store == nil {
		return nil, ErrInvalidConfig
	}
	g := compose.NewGraph[GraphInput, GraphOutput]()
	if err := g.AddLambdaNode("load_review_context", compose.InvokableLambda(func(ctx context.Context, input GraphInput) (Context, error) {
		return config.Loader.Load(ctx, input)
	})); err != nil {
		return nil, err
	}
	if err := g.AddLambdaNode("review_artifact", compose.InvokableLambda(func(ctx context.Context, reviewContext Context) (GraphOutput, error) {
		return runReview(ctx, config, reviewContext)
	})); err != nil {
		return nil, err
	}
	if err := g.AddEdge(compose.START, "load_review_context"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("load_review_context", "review_artifact"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("review_artifact", compose.END); err != nil {
		return nil, err
	}
	compileOptions := []compose.GraphCompileOption{compose.WithGraphName("reviewer_preview")}
	if config.CheckPointStore != nil {
		compileOptions = append(compileOptions, compose.WithCheckPointStore(config.CheckPointStore))
	}
	if len(config.CompileCallbacks) > 0 {
		compileOptions = append(compileOptions, compose.WithGraphCompileCallbacks(config.CompileCallbacks...))
	}
	runnable, err := g.Compile(context.Background(), compileOptions...)
	if err != nil {
		return nil, err
	}
	return &Graph{runnable: runnable}, nil
}

func (g *Graph) Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error) {
	if g == nil || g.runnable == nil {
		return GraphOutput{}, ErrInvalidConfig
	}
	runOptions := agenteino.RunOptions{}
	if len(options) > 0 {
		runOptions = options[0]
	}
	ctx, callOptions := agenteino.ApplyRunOptions(ctx, runOptions)
	return g.runnable.Invoke(ctx, input, callOptions...)
}

func runReview(ctx context.Context, config GraphConfig, reviewContext Context) (GraphOutput, error) {
	input := reviewContext.Input
	taskInput := input.Task
	attemptNo := taskInput.AttemptNo
	if attemptNo <= 0 {
		attemptNo = 1
	}
	maxAttempts := taskInput.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultReviewPolicy().MaxAttempts
	}
	metadata := map[string]any{}
	result, modelMeta, err := config.Responder.Review(ctx, reviewContext)
	if err != nil {
		return GraphOutput{}, err
	}
	for key, value := range modelMeta {
		metadata[key] = value
	}
	provider, _ := metadata["provider"].(string)
	modelID, _ := metadata["model_id"].(string)

	_, _ = config.Runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   "review_started",
		SourceRole:  "reviewer",
		Scope:       mustJSON(map[string]any{"shot_id": taskInput.ShotID, "node_id": taskInput.NodeID, "version_id": taskInput.ArtifactVersionID}),
	})
	record, err := config.Store.CreateReviewRecord(ctx, db.CreateReviewRecordParams{
		WorkspaceID:          input.WorkspaceID,
		ShotID:               reviewContext.Shot.ID,
		NodeID:               reviewContext.Node.ID,
		ArtifactVersionID:    reviewContext.Version.ID,
		GenerationJobID:      reviewContext.GenerationJob.ID,
		ReviewerThreadID:     input.ThreadID,
		ReviewerTaskID:       input.TaskID,
		ParentReviewRecordID: parentReviewID(taskInput.ParentReviewRecordID),
		TargetPhase:          taskInput.TargetPhase,
		AttemptNo:            attemptNo,
		MaxAttempts:          maxAttempts,
		ModelProvider:        strings.TrimSpace(provider),
		ModelID:              strings.TrimSpace(modelID),
	})
	if err != nil {
		return GraphOutput{}, err
	}
	policy := config.Policy
	if policy.OverallThreshold <= 0 {
		policy = DefaultReviewPolicy()
	}
	decision, err := ValidateRubric(result, policy)
	if err != nil {
		_, _ = config.Store.FailReviewRecord(ctx, db.FailReviewRecordParams{
			ID:           record.ID,
			ErrorCode:    "review_invalid_rubric",
			ErrorMessage: err.Error(),
		})
		return GraphOutput{}, err
	}
	rubricJSON := mustJSON(result.Rubric)
	retryJSON := mustJSON(result.RetryRecommendation)
	record, err = config.Store.CompleteReviewRecord(ctx, db.CompleteReviewRecordParams{
		ID:                  record.ID,
		Status:              decision.Status,
		OverallScore:        pgtype.Float4{Float32: float32(result.OverallScore), Valid: true},
		Rubric:              rubricJSON,
		Critique:            strings.TrimSpace(result.Critique),
		RetryRecommendation: retryJSON,
	})
	if err != nil {
		return GraphOutput{}, err
	}
	checkpointKey := CheckpointKey(input.WorkspaceID, input.ThreadID, input.TaskID)
	checkpointValue := mustJSON(map[string]any{
		"review_record_id": uuidString(record.ID),
		"decision":         decision.Status,
		"overall_score":    result.OverallScore,
	})
	if _, err := config.Runtime.UpsertCheckpoint(ctx, agentruntime.UpsertCheckpointParams{
		Key:         checkpointKey,
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		Value:       checkpointValue,
		Metadata:    mustJSON(map[string]any{"kind": "reviewer_result"}),
	}); err != nil {
		return GraphOutput{}, err
	}
	_, _ = config.Runtime.SetThreadCheckpoint(ctx, input.ThreadID, checkpointKey)
	if _, err := config.Runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		Role:        "assistant",
		MessageType: "ui_card",
		Content:     reviewMessageContent(reviewContext, record, result, decision),
		RawMessage:  mustJSON(result),
		TaskID:      input.TaskID,
	}); err != nil {
		return GraphOutput{}, err
	}
	eventType := "review_rejected"
	if decision.Status == ReviewStatusAccepted {
		eventType = "review_accepted"
		if config.Selector != nil {
			if _, err := config.Selector.SelectArtifactVersion(ctx, reviewContext.Node.ID, reviewContext.Version.ID); err != nil {
				return GraphOutput{}, err
			}
		}
	} else if decision.ShouldRetry && taskInput.AutoRetry && attemptNo < maxAttempts {
		_, _ = config.Runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
			WorkspaceID: input.WorkspaceID,
			ThreadID:    input.ThreadID,
			TaskID:      input.TaskID,
			EventType:   "retry_requested",
			SourceRole:  "reviewer",
			TargetRole:  "producer",
			Scope:       mustJSON(map[string]any{"shot_id": taskInput.ShotID, "review_record_id": uuidString(record.ID)}),
			Payload:     mustJSON(map[string]any{"fix_hints": decision.FixHints, "critique": result.Critique, "attempt_no": attemptNo, "max_attempts": maxAttempts}),
		})
		if config.RetryDispatcher != nil {
			if err := config.RetryDispatcher.DispatchRetry(ctx, RetryDispatchInput{
				WorkspaceID: input.WorkspaceID,
				ThreadID:    input.ThreadID,
				TaskID:      input.TaskID,
				ShotRef:     reviewContext.Shot.ClientKey,
				TargetPhase: taskInput.TargetPhase,
				ReviewID:    uuidString(record.ID),
				Critique:    strings.TrimSpace(result.Critique),
				FixHints:    decision.FixHints,
				AttemptNo:   attemptNo + 1,
				MaxAttempts: maxAttempts,
			}); err != nil {
				return GraphOutput{}, err
			}
		}
	} else if decision.Status == ReviewStatusRejected {
		_, _ = config.Runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
			WorkspaceID: input.WorkspaceID,
			ThreadID:    input.ThreadID,
			TaskID:      input.TaskID,
			EventType:   "retry_exhausted",
			SourceRole:  "reviewer",
			TargetRole:  "producer",
			Scope:       mustJSON(map[string]any{"shot_id": taskInput.ShotID, "review_record_id": uuidString(record.ID)}),
		})
	}
	_, _ = config.Runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		EventType:   eventType,
		SourceRole:  "reviewer",
		TargetRole:  "producer",
		Scope:       mustJSON(map[string]any{"shot_id": taskInput.ShotID, "node_id": taskInput.NodeID, "version_id": taskInput.ArtifactVersionID}),
		Payload:     mustJSON(map[string]any{"review_record_id": uuidString(record.ID), "overall_score": result.OverallScore, "status": decision.Status}),
	})
	if config.Dependency != nil {
		if err := config.Dependency.NotifyShotUpdated(ctx, input.WorkspaceID, reviewContext.Shot.ID, "review"); err != nil {
			return GraphOutput{}, err
		}
	}
	return GraphOutput{Record: record, Decision: decision, Result: result}, nil
}

func reviewMessageContent(reviewContext Context, record db.ReviewRecord, result ReviewResult, decision ReviewDecision) []byte {
	envelope := uimessage.Envelope{
		Schema: uimessage.SchemaV1,
		Blocks: []uimessage.Block{uimessage.ReviewCardBlock{
			BaseBlock:    uimessage.NewBaseBlock("blk_review", "review_card"),
			ReviewID:     uuidString(record.ID),
			Status:       decision.Status,
			TargetPhase:  reviewContext.Input.Task.TargetPhase,
			ShotRef:      reviewContext.Shot.ClientKey,
			NodeID:       uuidString(reviewContext.Node.ID),
			VersionID:    uuidString(reviewContext.Version.ID),
			OverallScore: result.OverallScore,
			Rubric:       result.Rubric,
			Critique:     strings.TrimSpace(result.Critique),
			RetryCount:   int(record.AttemptNo),
			MaxAttempts:  int(record.MaxAttempts),
			FixHints:     decision.FixHints,
		}},
	}
	return mustJSON(envelope)
}

func parentReviewID(value string) pgtype.UUID {
	id, _ := pgUUIDFromString(value)
	return id
}

func CheckpointKey(workspaceID, threadID, taskID pgtype.UUID) string {
	return fmt.Sprintf("reviewer:%s:%s:%s", uuidString(workspaceID), uuidString(threadID), uuidString(taskID))
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
