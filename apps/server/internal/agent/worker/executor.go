package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/sinmaystar/clip-anvil/internal/agent/preview"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Runtime interface {
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type Store interface {
	CreateAgentGenerationNode(ctx context.Context, params db.CreateAgentGenerationNodeParams) (db.MediaNode, error)
	GetMediaNodeByID(ctx context.Context, id pgtype.UUID) (db.MediaNode, error)
	ListMediaNodesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaNode, error)
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
	GetMediaAssetByID(ctx context.Context, id pgtype.UUID) (db.MediaAsset, error)
	GetKeyElementStateByID(ctx context.Context, params db.GetKeyElementStateByIDParams) (db.KeyElementState, error)
	GetDependencyEdgeByEndpoints(ctx context.Context, params db.GetDependencyEdgeByEndpointsParams) (db.MediaEdge, error)
	CreateMediaEdge(ctx context.Context, params db.CreateMediaEdgeParams) (db.MediaEdge, error)
	UpdateShotStatus(ctx context.Context, params db.UpdateShotStatusParams) (db.Shot, error)
	UpdateKeyElementState(ctx context.Context, params db.UpdateKeyElementStateParams) (db.KeyElementState, error)
	MarkRenderPlanCompleted(ctx context.Context, params db.MarkRenderPlanCompletedParams) (db.RenderPlan, error)
}

type NodeBroadcaster interface {
	BroadcastAgentNodeCreated(workspaceID pgtype.UUID, node db.MediaNode)
}

type ProductionSubmitter interface {
	SubmitGenerationIntent(ctx context.Context, intent production.GenerationIntent, options production.RunOptions) (production.RunResult, error)
}

type ExecutorConfig struct {
	Runtime     Runtime
	Store       Store
	Production  ProductionSubmitter
	Broadcaster NodeBroadcaster
	Logger      *slog.Logger
	Tracer      trace.Tracer
}

type Executor struct {
	runtime     Runtime
	store       Store
	production  ProductionSubmitter
	broadcaster NodeBroadcaster
	logger      *slog.Logger
	tracer      trace.Tracer
}

func NewExecutor(config ExecutorConfig) *Executor {
	return &Executor{
		runtime:     config.Runtime,
		store:       config.Store,
		production:  config.Production,
		broadcaster: config.Broadcaster,
		logger:      config.Logger,
		tracer:      config.Tracer,
	}
}

func (e *Executor) RunTask(ctx context.Context, input RunTaskInput) (runErr error) {
	task := input.Task
	if e == nil || e.runtime == nil || e.store == nil || e.production == nil || !task.ID.Valid || !task.WorkspaceID.Valid {
		return ErrInvalidConfig
	}
	if _, err := e.runtime.MarkTaskRunning(ctx, task.ID); err != nil {
		return err
	}
	workerInput, err := parseGenerationInput(task.Input)
	if err != nil {
		return e.fail(ctx, task, "worker_generation_invalid_input", err)
	}
	ctx, span := e.startSpan(ctx, task, workerInput)
	if span != nil {
		defer func() {
			if runErr != nil {
				span.RecordError(runErr)
				span.SetStatus(codes.Error, runErr.Error())
			} else {
				span.SetStatus(codes.Ok, "")
			}
			span.End()
		}()
	}
	_, _ = e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: task.WorkspaceID,
		ThreadID:    task.ThreadID,
		TaskID:      task.ID,
		EventType:   "worker_generation_started",
		SourceRole:  "worker",
		Scope:       mustJSON(map[string]any{"scope_type": task.ScopeType, "scope_id": uuidString(task.ScopeID), "shot_id": workerInput.ShotID}),
	})
	node, created, err := e.resolveTargetNode(ctx, task, workerInput)
	if err != nil {
		return e.fail(ctx, task, "worker_generation_node_failed", err)
	}
	if created && e.broadcaster != nil {
		e.broadcaster.BroadcastAgentNodeCreated(task.WorkspaceID, node)
	}
	inputRefs, err := ResolveInputRefs(ctx, e.store, task.WorkspaceID, node.ID, workerInput.InputNodeRefs)
	if err != nil {
		return e.fail(ctx, task, "worker_generation_input_refs_failed", err)
	}
	intent := generationIntent(task, workerInput, node, inputRefs)
	options := production.RunOptions{MaxAttempts: effectiveMaxAttempts(task, workerInput)}
	var result production.RunResult
	for attempt := 1; attempt <= options.MaxAttempts; attempt++ {
		result, err = e.production.SubmitGenerationIntent(ctx, intent, options)
		if err == nil {
			break
		}
		e.loggerOrDefault().WarnContext(ctx, "worker generation submit failed",
			"workspace_id", uuidString(task.WorkspaceID),
			"worker_task_id", uuidString(task.ID),
			"node_id", uuidString(node.ID),
			"attempt", attempt,
			"max_attempts", options.MaxAttempts,
			"error", err,
		)
	}
	if err != nil {
		return e.fail(ctx, task, "worker_generation_submit_failed", err)
	}
	output := GenerationOutput{
		Status:            "submitted",
		NodeID:            uuidString(result.Node.ID),
		GenerationJobID:   uuidString(result.Job.ID),
		ArtifactVersionID: uuidString(result.Version.ID),
		OperationType:     result.Job.OperationType,
	}
	rawOutput, _ := json.Marshal(output)
	if err := e.markScopedKeyElementStateReady(ctx, task, result); err != nil {
		return e.fail(ctx, task, "worker_generation_state_update_failed", err)
	}
	if task.RenderPlanID.Valid {
		_, _ = e.store.MarkRenderPlanCompleted(ctx, db.MarkRenderPlanCompletedParams{
			ID:              task.RenderPlanID,
			WorkspaceID:     task.WorkspaceID,
			Status:          "succeeded",
			OutputVersionID: result.Version.ID,
			OutputNodeID:    result.Node.ID,
		})
	}
	if _, err := e.runtime.MarkTaskSucceeded(ctx, task.ID, rawOutput); err != nil {
		return err
	}
	_, _ = e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: task.WorkspaceID,
		ThreadID:    task.ThreadID,
		TaskID:      task.ID,
		EventType:   "worker_generation_submitted",
		SourceRole:  "worker",
		TargetRole:  "craftsman",
		Payload:     rawOutput,
	})
	if eventType := submittedEventType(workerInput.Mode); eventType != "" {
		_, _ = e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
			WorkspaceID: task.WorkspaceID,
			ThreadID:    task.ThreadID,
			TaskID:      task.ID,
			EventType:   eventType,
			SourceRole:  "worker",
			TargetRole:  "craftsman",
			Payload:     rawOutput,
		})
	}
	return nil
}

func (e *Executor) startSpan(ctx context.Context, task db.AgentTask, input GenerationInput) (context.Context, trace.Span) {
	if e == nil || e.tracer == nil {
		return ctx, nil
	}
	return e.tracer.Start(ctx, "worker_generation", trace.WithAttributes(
		attribute.String("clipanvil.workspace_id", uuidString(task.WorkspaceID)),
		attribute.String("clipanvil.agent.thread_id", uuidString(task.ThreadID)),
		attribute.String("clipanvil.agent.task_id", uuidString(task.ID)),
		attribute.String("clipanvil.agent.role", "worker"),
		attribute.String("clipanvil.agent.task_type", task.TaskType),
		attribute.String("clipanvil.agent.scope_type", task.ScopeType),
		attribute.String("clipanvil.agent.scope_id", uuidString(task.ScopeID)),
		attribute.String("clipanvil.agent.shot_id", input.ShotID),
		attribute.String("clipanvil.agent.shot_client_key", input.ShotClientKey),
		attribute.String("clipanvil.agent.mode", input.Mode),
		attribute.String("clipanvil.production.output_type", generationSpec(input).OutputType),
		attribute.String("clipanvil.production.operation_type", generationSpec(input).OperationType),
		attribute.String("clipanvil.production.model_provider", strings.TrimSpace(input.Model.Provider)),
		attribute.String("clipanvil.production.model_id", strings.TrimSpace(input.Model.ModelID)),
	))
}

func (e *Executor) resolveTargetNode(ctx context.Context, task db.AgentTask, input GenerationInput) (db.MediaNode, bool, error) {
	if id, ok := pgUUIDFromString(input.TargetNodeID); ok {
		node, err := e.store.GetMediaNodeByID(ctx, id)
		return node, false, err
	}
	shotID, ok := pgUUIDFromString(input.ShotID)
	if !ok {
		if task.ScopeType == "shot" && task.ScopeID.Valid {
			shotID = task.ScopeID
			ok = true
		}
	}
	if !ok && task.ScopeType == "shot" {
		return db.MediaNode{}, false, fmt.Errorf("%w: shot_id is required", ErrInvalidInput)
	}
	spec := generationSpec(input)
	modelParams, _ := json.Marshal(input.Params)
	canvasX, canvasY := nodePosition(input)
	metadata := mustJSON(map[string]any{
		"agent_artifact_kind":          spec.ArtifactKind,
		"source_phase":                 spec.SourcePhase,
		"scope_type":                   task.ScopeType,
		"scope_id":                     uuidString(task.ScopeID),
		"shot_client_key":              input.ShotClientKey,
		"shot_sort_order":              input.ShotSortOrder,
		"key_element_state_client_key": input.KeyElementStateClientKey,
		"craftsman_thread_id":          input.CraftsmanThreadID,
		"craftsman_task_id":            input.CraftsmanTaskID,
		"worker_task_id":               uuidString(task.ID),
	})
	node, err := e.store.CreateAgentGenerationNode(ctx, db.CreateAgentGenerationNodeParams{
		WorkspaceID:   task.WorkspaceID,
		NodeType:      spec.NodeType,
		Title:         nodeTitle(input),
		Prompt:        strings.TrimSpace(input.Prompt),
		OperationType: spec.OperationType,
		CanvasX:       canvasX,
		CanvasY:       canvasY,
		CanvasW:       spec.CanvasW,
		CanvasH:       220,
		ShotID:        shotID,
		ModelProvider: nullableText(input.Model.Provider),
		ModelID:       nullableText(input.Model.ModelID),
		ModelParams:   defaultJSON(modelParams),
		Metadata:      metadata,
	})
	return node, true, err
}

func generationIntent(task db.AgentTask, input GenerationInput, node db.MediaNode, inputRefs []production.InputRef) production.GenerationIntent {
	spec := generationSpec(input)
	return production.GenerationIntent{
		WorkspaceID:    task.WorkspaceID,
		TargetNodeID:   node.ID,
		OutputType:     spec.OutputType,
		OperationType:  spec.OperationType,
		PromptTemplate: strings.TrimSpace(input.Prompt),
		RenderedPrompt: strings.TrimSpace(input.Prompt),
		InputRefs:      inputRefs,
		Model: production.ModelSpec{
			Provider: strings.TrimSpace(input.Model.Provider),
			ModelID:  strings.TrimSpace(input.Model.ModelID),
		},
		Params: defaultParams(input.Params),
		RequestedBy: production.RequestedBy{
			Type: "agent_worker",
			ID:   uuidString(task.ID),
		},
	}
}

func parseGenerationInput(raw []byte) (GenerationInput, error) {
	var input GenerationInput
	if err := json.Unmarshal(defaultJSON(raw), &input); err != nil {
		return GenerationInput{}, err
	}
	if strings.TrimSpace(input.TargetPhase) != "" {
		input.Mode = strings.TrimSpace(input.TargetPhase)
	}
	if input.Mode == "" {
		input.Mode = "preview_image"
	}
	if input.Mode != "reference_image" && input.Mode != "preview_image" && input.Mode != "shot_video" {
		return GenerationInput{}, ErrInvalidInput
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return GenerationInput{}, ErrInvalidInput
	}
	if input.Mode == "shot_video" && len(input.InputNodeRefs) == 0 {
		return GenerationInput{}, ErrInvalidInput
	}
	if input.MaxAttempts <= 0 {
		input.MaxAttempts = 3
	}
	if input.MaxAttempts > 3 {
		input.MaxAttempts = 3
	}
	return input, nil
}

func effectiveMaxAttempts(task db.AgentTask, input GenerationInput) int {
	maxAttempts := int(task.MaxAttempts)
	if input.MaxAttempts > 0 && input.MaxAttempts < maxAttempts {
		maxAttempts = input.MaxAttempts
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if maxAttempts > 3 {
		maxAttempts = 3
	}
	return maxAttempts
}

func (e *Executor) fail(ctx context.Context, task db.AgentTask, code string, err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	e.markScopedShotFailed(ctx, task)
	if e != nil && e.store != nil && task.RenderPlanID.Valid {
		_, _ = e.store.MarkRenderPlanCompleted(ctx, db.MarkRenderPlanCompletedParams{
			ID:              task.RenderPlanID,
			WorkspaceID:     task.WorkspaceID,
			Status:          "failed",
			OutputVersionID: pgtype.UUID{},
			OutputNodeID:    pgtype.UUID{},
		})
	}
	_, _ = e.runtime.MarkTaskFailed(ctx, task.ID, code, message)
	_, _ = e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: task.WorkspaceID,
		ThreadID:    task.ThreadID,
		TaskID:      task.ID,
		EventType:   "worker_generation_failed",
		SourceRole:  "worker",
		Payload:     mustJSON(map[string]any{"error_code": code, "error": message}),
	})
	return fmt.Errorf("%s: %w", code, err)
}

func (e *Executor) markScopedShotFailed(ctx context.Context, task db.AgentTask) {
	if e == nil || e.store == nil || task.ScopeType != "shot" || !task.ScopeID.Valid || !task.WorkspaceID.Valid {
		return
	}
	status, ok := preview.ShotStatusForEvent(preview.EventFailed)
	if !ok {
		return
	}
	_, _ = e.store.UpdateShotStatus(ctx, db.UpdateShotStatusParams{
		ID:          task.ScopeID,
		WorkspaceID: task.WorkspaceID,
		Status:      status,
	})
}

func (e *Executor) markScopedKeyElementStateReady(ctx context.Context, task db.AgentTask, result production.RunResult) error {
	if e == nil || e.store == nil || task.ScopeType != "key_element_state" || !task.ScopeID.Valid || !task.WorkspaceID.Valid {
		return nil
	}
	state, err := e.store.GetKeyElementStateByID(ctx, db.GetKeyElementStateByIDParams{
		ID:          task.ScopeID,
		WorkspaceID: task.WorkspaceID,
	})
	if err != nil {
		return err
	}
	_, err = e.store.UpdateKeyElementState(ctx, db.UpdateKeyElementStateParams{
		ID:                 state.ID,
		WorkspaceID:        state.WorkspaceID,
		Label:              state.Label,
		VisualDescription:  state.VisualDescription,
		ReferenceStatus:    "ready",
		ReferenceNodeID:    result.Node.ID,
		ReferenceVersionID: result.Version.ID,
		IsDefault:          state.IsDefault,
		StateFacts:         state.StateFacts,
		SourceRefs:         state.SourceRefs,
		Status:             state.Status,
	})
	if err != nil {
		return err
	}
	return nil
}

type generationSpecValue struct {
	NodeType      db.NodeType
	OutputType    string
	OperationType string
	ArtifactKind  string
	SourcePhase   string
	CanvasW       float32
}

func generationSpec(input GenerationInput) generationSpecValue {
	switch input.Mode {
	case "reference_image":
		operationType := strings.TrimSpace(input.OperationType)
		if operationType == "" {
			operationType = "text_to_image"
		}
		outputType := strings.TrimSpace(input.OutputType)
		if outputType == "" {
			outputType = "image"
		}
		return generationSpecValue{
			NodeType:      db.NodeTypeImage,
			OutputType:    outputType,
			OperationType: operationType,
			ArtifactKind:  "reference_image",
			SourcePhase:   "key_element_state",
			CanvasW:       320,
		}
	case "shot_video":
		operationType := strings.TrimSpace(input.OperationType)
		if operationType == "" {
			operationType = "image_to_video"
		}
		outputType := strings.TrimSpace(input.OutputType)
		if outputType == "" {
			outputType = "video"
		}
		return generationSpecValue{
			NodeType:      db.NodeTypeVideo,
			OutputType:    outputType,
			OperationType: operationType,
			ArtifactKind:  "shot_video",
			SourcePhase:   "preview_image",
			CanvasW:       360,
		}
	default:
		operationType := strings.TrimSpace(input.OperationType)
		if operationType == "" {
			operationType = "text_to_image"
		}
		outputType := strings.TrimSpace(input.OutputType)
		if outputType == "" {
			outputType = "image"
		}
		return generationSpecValue{
			NodeType:      db.NodeTypeImage,
			OutputType:    outputType,
			OperationType: operationType,
			ArtifactKind:  "preview_image",
			CanvasW:       320,
		}
	}
}

func nodeTitle(input GenerationInput) string {
	if input.Mode == "reference_image" {
		if strings.TrimSpace(input.KeyElementStateClientKey) != "" {
			return input.KeyElementStateClientKey + " reference image"
		}
		return "Agent reference image"
	}
	if input.Mode == "shot_video" {
		if strings.TrimSpace(input.ShotClientKey) != "" {
			return input.ShotClientKey + " shot video"
		}
		return "Agent shot video"
	}
	if strings.TrimSpace(input.ShotClientKey) != "" {
		return input.ShotClientKey + " preview image"
	}
	return "Agent preview image"
}

func nodePosition(input GenerationInput) (float32, float32) {
	if input.Mode == "reference_image" {
		return 140, 140
	}
	x, y := previewNodePosition(input)
	if input.Mode == "shot_video" {
		return x, y + 500
	}
	return x, y
}

func previewNodePosition(input GenerationInput) (float32, float32) {
	const (
		startX = 140
		startY = 140
		stepX  = 520
		stepY  = 900
	)
	order := input.ShotSortOrder
	if order <= 0 {
		order = shotOrderFromClientKey(input.ShotClientKey)
	}
	if order <= 0 {
		order = 1
	}
	index := order - 1
	column := index % 3
	row := index / 3
	return float32(startX + column*stepX), float32(startY + row*stepY)
}

func submittedEventType(mode string) string {
	switch mode {
	case "shot_video":
		return "shot_video_submitted"
	default:
		return ""
	}
}

func shotOrderFromClientKey(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	lastDash := strings.LastIndex(value, "-")
	if lastDash < 0 || lastDash == len(value)-1 {
		return 0
	}
	order, err := strconv.Atoi(value[lastDash+1:])
	if err != nil {
		return 0
	}
	return order
}

func defaultParams(params map[string]any) map[string]any {
	if params == nil {
		return map[string]any{}
	}
	return params
}

func nullableText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	return pgtype.Text{String: value, Valid: value != ""}
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

func pgUUIDFromString(value string) (pgtype.UUID, bool) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

func (e *Executor) loggerOrDefault() *slog.Logger {
	if e != nil && e.logger != nil {
		return e.logger
	}
	return slog.Default()
}
