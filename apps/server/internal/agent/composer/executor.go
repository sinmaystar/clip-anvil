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
	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Runtime interface {
	MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
	MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
	MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code, message string) (db.AgentTask, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
	UpdateMessage(ctx context.Context, params agentruntime.UpdateMessageParams) (db.AgentMessage, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
	UpsertCheckpoint(ctx context.Context, params agentruntime.UpsertCheckpointParams) (db.EinoCheckpoint, error)
	SetThreadCheckpoint(ctx context.Context, threadID pgtype.UUID, checkpointKey string) (db.AgentThread, error)
}

type producerSignalRuntime interface {
	GetOrCreateProducerThread(ctx context.Context, workspaceID pgtype.UUID) (db.AgentThread, error)
	CreateProducerPendingSignal(ctx context.Context, params agentruntime.CreateProducerPendingSignalParams) (db.ProducerPendingSignal, error)
}

type producerWakeRuntime interface {
	ListActiveAgentTasksByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.AgentTask, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
}

type ProducerTaskEnqueuer interface {
	EnqueueProducerTask(ctx context.Context, task db.AgentTask)
}

type Store interface {
	CreateAgentGenerationNode(ctx context.Context, params db.CreateAgentGenerationNodeParams) (db.MediaNode, error)
	GetActiveAudioPlanByWorkspace(ctx context.Context, workspaceID pgtype.UUID) (db.AudioPlan, error)
	GetMediaNodeByID(ctx context.Context, id pgtype.UUID) (db.MediaNode, error)
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

type Broadcaster interface {
	BroadcastAgentMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent)
	BroadcastAgentMessageUpdated(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent)
}

type Runner interface {
	Run(ctx context.Context, input GraphInput, options ...agenteino.RunOptions) (GraphOutput, error)
}

type ExecutorConfig struct {
	Runtime          Runtime
	Graph            Runner
	Broadcaster      Broadcaster
	ProducerEnqueuer ProducerTaskEnqueuer
	TraceCallbacks   []callbacks.Handler
}

type Executor struct {
	runtime          Runtime
	graph            Runner
	broadcaster      Broadcaster
	producerEnqueuer ProducerTaskEnqueuer
	traceCallbacks   []callbacks.Handler
}

func NewExecutor(config ExecutorConfig) *Executor {
	return &Executor{
		runtime:          config.Runtime,
		graph:            config.Graph,
		broadcaster:      config.Broadcaster,
		producerEnqueuer: config.ProducerEnqueuer,
		traceCallbacks:   config.TraceCallbacks,
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
	checkpointKey := agenteino.CheckpointKey("composer_timeline", task.WorkspaceID, task.ThreadID, task.ID)
	ctx = cozelooptrace.ContextWithAttributes(ctx,
		attribute.String("clipanvil.workspace_id", uuidString(task.WorkspaceID)),
		attribute.String("clipanvil.agent.thread_id", uuidString(task.ThreadID)),
		attribute.String("clipanvil.agent.task_id", uuidString(task.ID)),
		attribute.String("clipanvil.agent.role", "composer"),
		attribute.String("clipanvil.agent.task_type", task.TaskType),
		attribute.String("clipanvil.agent.scope_type", task.ScopeType),
		attribute.String("clipanvil.agent.scope_id", uuidString(task.ScopeID)),
	)
	liveToolTrace := newComposerLiveToolTrace(e, task, compositionInput.ParentToolCallID)
	ctx = agenttools.WithNativeToolTraceSink(ctx, liveToolTrace)
	out, err := e.graph.Run(ctx, graphInput, agenteino.RunOptions{
		CheckPointID: checkpointKey,
		Callbacks:    e.traceCallbacks,
	})
	if err != nil {
		return e.fail(ctx, task, "composer_failed", err)
	}
	if liveToolTrace.count() == 0 {
		if err := e.persistNativeToolTrace(ctx, task, out.Output.SameTurnMessages, compositionInput.ParentToolCallID); err != nil {
			return e.fail(ctx, task, "composer_tool_trace_persist_failed", err)
		}
	}
	if err := e.persistAssistantText(ctx, task, out.AssistantText); err != nil {
		return e.fail(ctx, task, "composer_message_persist_failed", err)
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

type composerLiveToolTrace struct {
	executor         *Executor
	task             db.AgentTask
	parentToolCallID string
	callMessages     map[string]db.AgentMessage
	startedCount     int
}

func newComposerLiveToolTrace(executor *Executor, task db.AgentTask, parentToolCallID string) *composerLiveToolTrace {
	return &composerLiveToolTrace{
		executor:         executor,
		task:             task,
		parentToolCallID: strings.TrimSpace(parentToolCallID),
		callMessages:     map[string]db.AgentMessage{},
	}
}

func (t *composerLiveToolTrace) count() int {
	if t == nil {
		return 0
	}
	return t.startedCount
}

func (t *composerLiveToolTrace) NativeToolCallStarted(ctx context.Context, runtime agenttools.NativeRuntimeContext, trace agenttools.NativeToolTrace) error {
	if t == nil || t.executor == nil || t.executor.runtime == nil {
		return nil
	}
	toolCallID := strings.TrimSpace(runtime.ToolCallID)
	if toolCallID == "" {
		toolCallID = strings.TrimSpace(trace.ToolName)
	}
	sameTurn := ComposerSameTurnMessage{
		Role:          "assistant",
		MessageType:   "tool_call",
		ToolCallID:    toolCallID,
		ToolName:      trace.ToolName,
		ToolArguments: trace.Arguments,
	}
	msg, err := t.executor.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: t.task.WorkspaceID,
		ThreadID:    t.task.ThreadID,
		Role:        "assistant",
		MessageType: "tool_call",
		Content:     composerToolTraceContent(sameTurn),
		RawMessage:  composerToolTraceRaw(sameTurn, t.parentToolCallID),
		TaskID:      t.task.ID,
	})
	if err != nil {
		return err
	}
	t.callMessages[toolCallID] = msg
	t.startedCount++
	t.executor.broadcastMessage(t.task.WorkspaceID, msg, db.AgentEvent{})
	return nil
}

func (t *composerLiveToolTrace) NativeToolCallCompleted(ctx context.Context, runtime agenttools.NativeRuntimeContext, trace agenttools.NativeToolTrace) error {
	if t == nil || t.executor == nil || t.executor.runtime == nil {
		return nil
	}
	toolCallID := strings.TrimSpace(runtime.ToolCallID)
	resultText := strings.TrimSpace(trace.Result)
	status := "succeeded"
	if strings.TrimSpace(trace.Error) != "" {
		status = "failed"
		resultText = strings.TrimSpace(trace.Error)
	}
	sameTurn := ComposerSameTurnMessage{
		Role:        "tool",
		MessageType: "tool_result",
		Content:     resultText,
		ToolCallID:  toolCallID,
		ToolName:    trace.ToolName,
	}
	msg, err := t.executor.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: t.task.WorkspaceID,
		ThreadID:    t.task.ThreadID,
		Role:        "tool",
		MessageType: "tool_result",
		Content:     composerToolTraceContent(sameTurn),
		RawMessage:  composerToolTraceRaw(sameTurn, t.parentToolCallID),
		TaskID:      t.task.ID,
	})
	if err != nil {
		return err
	}
	t.executor.broadcastMessage(t.task.WorkspaceID, msg, db.AgentEvent{})
	if callMsg, ok := t.callMessages[toolCallID]; ok {
		updated, err := t.executor.runtime.UpdateMessage(ctx, agentruntime.UpdateMessageParams{
			ID:         callMsg.ID,
			Content:    composerCompletedToolTraceContentWithStatus(sameTurn, callMsg.RawMessage, status),
			RawMessage: composerCompletedToolTraceRawWithStatus(sameTurn, callMsg.RawMessage, status),
		})
		if err != nil {
			return err
		}
		t.executor.broadcastMessageUpdated(t.task.WorkspaceID, updated, db.AgentEvent{})
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
		"trigger":             signalType,
		"scope_type":          "final_output",
		"scope_id":            output.NodeID,
		"scope_key":           finalOutputKey(output),
		"scope_ref":           objectRef("media_node", finalOutputKey(output)),
		"status":              output.Status,
		"timeline_plan_id":    output.TimelinePlanID,
		"output_node_id":      output.NodeID,
		"node_id":             output.NodeID,
		"node_key":            finalOutputKey(output),
		"node_ref":            objectRef("media_node", finalOutputKey(output)),
		"generation_job_id":   output.GenerationJobID,
		"artifact_version_id": output.ArtifactVersionID,
		"sandbox_job_id":      output.SandboxJobID,
		"operation_type":      output.OperationType,
		"composer_task_id":    uuidString(task.ID),
		"composer_thread_id":  uuidString(task.ThreadID),
	})
	if _, err = runtime.CreateProducerPendingSignal(ctx, agentruntime.CreateProducerPendingSignalParams{
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
	}); err != nil {
		return err
	}
	return e.ensureProducerWakeTask(ctx, task.WorkspaceID, producerThread.ID, payload)
}

func (e *Executor) ensureProducerWakeTask(ctx context.Context, workspaceID pgtype.UUID, producerThreadID pgtype.UUID, input []byte) error {
	if e == nil || e.runtime == nil || e.producerEnqueuer == nil || !workspaceID.Valid || !producerThreadID.Valid {
		return nil
	}
	runtime, ok := e.runtime.(producerWakeRuntime)
	if !ok {
		return nil
	}
	activeTasks, err := runtime.ListActiveAgentTasksByWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, task := range activeTasks {
		if task.Role == "producer" &&
			(task.TaskType == "producer_turn" || task.TaskType == "decision_resume") &&
			(task.Status == "queued" || task.Status == "running" || task.Status == "waiting_for_user") {
			return nil
		}
	}
	task, err := runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: workspaceID,
		ThreadID:    producerThreadID,
		Role:        "producer",
		ScopeType:   "workspace",
		TaskType:    "producer_turn",
		MaxAttempts: 1,
		Input:       input,
	})
	if err != nil {
		return err
	}
	e.producerEnqueuer.EnqueueProducerTask(ctx, task)
	return nil
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

func (e *Executor) persistNativeToolTrace(ctx context.Context, task db.AgentTask, messages []ComposerSameTurnMessage, parentToolCallID string) error {
	callMessages := map[string]db.AgentMessage{}
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
		msg, err := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
			WorkspaceID: task.WorkspaceID,
			ThreadID:    task.ThreadID,
			Role:        role,
			MessageType: messageType,
			Content:     composerToolTraceContent(trace),
			RawMessage:  composerToolTraceRaw(trace, parentToolCallID),
			TaskID:      task.ID,
		})
		if err != nil {
			return err
		}
		e.broadcastMessage(task.WorkspaceID, msg, db.AgentEvent{})
		if messageType == "tool_call" {
			callMessages[trace.ToolCallID] = msg
		}
		if messageType == "tool_result" {
			if callMsg, ok := callMessages[trace.ToolCallID]; ok {
				updated, err := e.runtime.UpdateMessage(ctx, agentruntime.UpdateMessageParams{
					ID:         callMsg.ID,
					Content:    composerCompletedToolTraceContent(trace, callMsg.RawMessage),
					RawMessage: composerCompletedToolTraceRaw(trace, callMsg.RawMessage),
				})
				if err != nil {
					return err
				}
				e.broadcastMessageUpdated(task.WorkspaceID, updated, db.AgentEvent{})
			}
		}
	}
	return nil
}

func (e *Executor) persistAssistantText(ctx context.Context, task db.AgentTask, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	content, err := uimessage.BuildAssistantMessageContent(uimessage.AssistantMessageInput{Text: text})
	if err != nil {
		return err
	}
	msg, err := e.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: task.WorkspaceID,
		ThreadID:    task.ThreadID,
		Role:        "assistant",
		MessageType: "text",
		Content:     content,
		RawMessage:  mustJSON(map[string]any{"schema": "clipanvil.agent.assistant_text.v1", "text": text}),
		TaskID:      task.ID,
	})
	if err != nil {
		return err
	}
	e.broadcastMessage(task.WorkspaceID, msg, db.AgentEvent{})
	return nil
}

func (e *Executor) broadcastMessage(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent) {
	if e == nil || e.broadcaster == nil {
		return
	}
	e.broadcaster.BroadcastAgentMessage(workspaceID, message, event)
}

func (e *Executor) broadcastMessageUpdated(workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent) {
	if e == nil || e.broadcaster == nil {
		return
	}
	e.broadcaster.BroadcastAgentMessageUpdated(workspaceID, message, event)
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

func defaultJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

func composerToolTraceContent(trace ComposerSameTurnMessage) []byte {
	if trace.MessageType == "tool_call" {
		return composerToolStatusContent(trace.ToolCallID, trace.ToolName, trace.ToolName, "running", composerToolTraceSummary(trace.ToolArguments, trace.Content), "", trace.ToolArguments, nil)
	}
	return mustJSON(map[string]any{
		"schema":       "clipanvil.agent.tool_trace.v1",
		"message_type": "tool_result",
		"tool_call_id": trace.ToolCallID,
		"tool_name":    trace.ToolName,
		"text":         strings.TrimSpace(trace.Content),
	})
}

func composerCompletedToolTraceContent(trace ComposerSameTurnMessage, previousRaw []byte) []byte {
	return composerCompletedToolTraceContentWithStatus(trace, previousRaw, "succeeded")
}

func composerCompletedToolTraceContentWithStatus(trace ComposerSameTurnMessage, previousRaw []byte, status string) []byte {
	args := composerToolTraceArgumentsFromRaw(previousRaw)
	result := map[string]any{}
	if text := strings.TrimSpace(trace.Content); text != "" {
		result["text"] = text
	}
	return composerToolStatusContent(trace.ToolCallID, trace.ToolName, trace.ToolName, status, composerToolTraceSummary(args, trace.Content), "", args, result)
}

func composerCompletedToolTraceRaw(trace ComposerSameTurnMessage, previousRaw []byte) []byte {
	return composerCompletedToolTraceRawWithStatus(trace, previousRaw, "succeeded")
}

func composerCompletedToolTraceRawWithStatus(trace ComposerSameTurnMessage, previousRaw []byte, status string) []byte {
	raw := map[string]any{}
	_ = json.Unmarshal(defaultJSON(previousRaw), &raw)
	raw["result_text"] = strings.TrimSpace(trace.Content)
	raw["message_type"] = "tool_call"
	raw["status"] = status
	return mustJSON(raw)
}

func composerToolStatusContent(toolCallID string, toolName string, label string, status string, summary string, errorMessage string, arguments map[string]any, result map[string]any) []byte {
	content, err := uimessage.BuildToolStatusMessageContent(uimessage.ToolStatusInput{
		ToolCallID:   toolCallID,
		ToolName:     toolName,
		Label:        label,
		Status:       status,
		Summary:      summary,
		ErrorMessage: errorMessage,
		Arguments:    arguments,
		Result:       result,
	})
	if err != nil {
		return mustJSON(map[string]any{
			"schema":       "clipanvil.agent.tool_trace.v1",
			"message_type": "tool_call",
			"tool_call_id": toolCallID,
			"tool_name":    toolName,
			"text":         summary,
		})
	}
	return content
}

func composerToolTraceArgumentsFromRaw(raw []byte) map[string]any {
	payload := map[string]any{}
	_ = json.Unmarshal(defaultJSON(raw), &payload)
	if args, ok := payload["arguments"].(map[string]any); ok {
		return args
	}
	return map[string]any{}
}

func composerToolTraceSummary(args map[string]any, fallback string) string {
	if args != nil {
		if instructions, _ := args["instructions"].(string); strings.TrimSpace(instructions) != "" {
			return strings.TrimSpace(instructions)
		}
		if templateKey, _ := args["template_key"].(string); strings.TrimSpace(templateKey) != "" {
			return strings.TrimSpace(templateKey)
		}
	}
	return strings.TrimSpace(fallback)
}

func composerToolTraceRaw(trace ComposerSameTurnMessage, parentToolCallID string) []byte {
	return mustJSON(map[string]any{
		"schema":              "clipanvil.agent.tool_trace.v1",
		"role":                trace.Role,
		"message_type":        trace.MessageType,
		"tool_call_id":        trace.ToolCallID,
		"tool_name":           trace.ToolName,
		"arguments":           trace.ToolArguments,
		"result_text":         strings.TrimSpace(trace.Content),
		"parent_tool_call_id": strings.TrimSpace(parentToolCallID),
	})
}

func pgUUIDFromString(value string) (pgtype.UUID, bool) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}

func finalOutputKey(output CompositionOutput) string {
	if value := strings.TrimSpace(output.NodeID); value != "" {
		return value
	}
	return strings.TrimSpace(output.TimelinePlanID)
}

func objectRef(objectType string, key string) string {
	objectType = strings.TrimSpace(objectType)
	key = strings.TrimSpace(key)
	if objectType == "" || key == "" {
		return ""
	}
	return objectType + "/" + key
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
