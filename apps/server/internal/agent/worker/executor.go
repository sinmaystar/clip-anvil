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
}

type Executor struct {
	runtime     Runtime
	store       Store
	production  ProductionSubmitter
	broadcaster NodeBroadcaster
	logger      *slog.Logger
}

func NewExecutor(config ExecutorConfig) *Executor {
	return &Executor{
		runtime:     config.Runtime,
		store:       config.Store,
		production:  config.Production,
		broadcaster: config.Broadcaster,
		logger:      config.Logger,
	}
}

func (e *Executor) RunTask(ctx context.Context, input RunTaskInput) error {
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
	_, _ = e.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: task.WorkspaceID,
		ThreadID:    task.ThreadID,
		TaskID:      task.ID,
		EventType:   "worker_generation_started",
		SourceRole:  "worker",
		Scope:       mustJSON(map[string]any{"shot_id": workerInput.ShotID}),
	})
	node, created, err := e.resolveTargetNode(ctx, task, workerInput)
	if err != nil {
		return e.fail(ctx, task, "worker_generation_node_failed", err)
	}
	if created && e.broadcaster != nil {
		e.broadcaster.BroadcastAgentNodeCreated(task.WorkspaceID, node)
	}
	intent := generationIntent(task, workerInput, node)
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
	return nil
}

func (e *Executor) resolveTargetNode(ctx context.Context, task db.AgentTask, input GenerationInput) (db.MediaNode, bool, error) {
	if id, ok := pgUUIDFromString(input.TargetNodeID); ok {
		node, err := e.store.GetMediaNodeByID(ctx, id)
		return node, false, err
	}
	shotID, ok := pgUUIDFromString(input.ShotID)
	if !ok {
		if task.ScopeID.Valid {
			shotID = task.ScopeID
			ok = true
		}
	}
	if !ok {
		return db.MediaNode{}, false, fmt.Errorf("%w: shot_id is required", ErrInvalidInput)
	}
	modelParams, _ := json.Marshal(input.Params)
	canvasX, canvasY := previewNodePosition(input)
	metadata := mustJSON(map[string]any{
		"agent_artifact_kind": "preview_image",
		"shot_client_key":     input.ShotClientKey,
		"shot_sort_order":     input.ShotSortOrder,
		"craftsman_thread_id": input.CraftsmanThreadID,
		"craftsman_task_id":   input.CraftsmanTaskID,
		"worker_task_id":      uuidString(task.ID),
	})
	node, err := e.store.CreateAgentGenerationNode(ctx, db.CreateAgentGenerationNodeParams{
		WorkspaceID:   task.WorkspaceID,
		NodeType:      db.NodeTypeImage,
		Title:         previewNodeTitle(input),
		Prompt:        strings.TrimSpace(input.Prompt),
		OperationType: "text_to_image",
		CanvasX:       canvasX,
		CanvasY:       canvasY,
		CanvasW:       320,
		CanvasH:       220,
		ShotID:        shotID,
		ModelProvider: nullableText(input.Model.Provider),
		ModelID:       nullableText(input.Model.ModelID),
		ModelParams:   defaultJSON(modelParams),
		Metadata:      metadata,
	})
	return node, true, err
}

func generationIntent(task db.AgentTask, input GenerationInput, node db.MediaNode) production.GenerationIntent {
	return production.GenerationIntent{
		WorkspaceID:    task.WorkspaceID,
		TargetNodeID:   node.ID,
		OutputType:     "image",
		OperationType:  "text_to_image",
		PromptTemplate: strings.TrimSpace(input.Prompt),
		RenderedPrompt: strings.TrimSpace(input.Prompt),
		InputRefs:      nil,
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
	if input.Mode != "preview_image" || strings.TrimSpace(input.Prompt) == "" {
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

func previewNodeTitle(input GenerationInput) string {
	if strings.TrimSpace(input.ShotClientKey) != "" {
		return input.ShotClientKey + " preview image"
	}
	return "Agent preview image"
}

func previewNodePosition(input GenerationInput) (float32, float32) {
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
	return float32(140 + column*380), float32(140 + row*300)
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
