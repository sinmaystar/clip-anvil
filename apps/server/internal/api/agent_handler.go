package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5/pgtype"

	agenthitl "github.com/sinmaystar/clip-anvil/internal/agent/hitl"
	"github.com/sinmaystar/clip-anvil/internal/agent/modelselection"
	agentoverview "github.com/sinmaystar/clip-anvil/internal/agent/overview"
	agentproducer "github.com/sinmaystar/clip-anvil/internal/agent/producer"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/storage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ProducerTaskRunner interface {
	RunTask(ctx context.Context, input agentproducer.RunTaskInput) error
}

type AgentHandler struct {
	queries        *db.Queries
	runtime        *agentruntime.Service
	hub            *AgentHub
	canvasHub      *CanvasHub
	broadcaster    *AgentBroadcaster
	storage        *storage.Service
	producerRunner ProducerTaskRunner
	modelSelection *modelselection.Service
	hitlService    *agenthitl.Service
}

func NewAgentHandler(queries *db.Queries, runtime *agentruntime.Service, hub *AgentHub, runners ...ProducerTaskRunner) *AgentHandler {
	var runner ProducerTaskRunner
	if len(runners) > 0 {
		runner = runners[0]
	}
	return &AgentHandler{
		queries:        queries,
		runtime:        runtime,
		hub:            hub,
		broadcaster:    NewAgentBroadcaster(hub),
		producerRunner: runner,
	}
}

func (h *AgentHandler) SetAttachmentStorage(storageService *storage.Service) {
	h.storage = storageService
}

func (h *AgentHandler) SetCanvasHub(canvasHub *CanvasHub) {
	h.canvasHub = canvasHub
}

func (h *AgentHandler) SetModelSelectionService(service *modelselection.Service) {
	h.modelSelection = service
}

func (h *AgentHandler) SetHITLService(service *agenthitl.Service) {
	h.hitlService = service
}

type getAgentThreadResponse struct {
	Thread agentThreadResponse `json:"thread"`
}

type listAgentMessagesResponse struct {
	Thread   agentThreadResponse    `json:"thread"`
	Messages []agentMessageResponse `json:"messages"`
}

type postAgentMessageResponse struct {
	Message       agentMessageResponse `json:"message"`
	Event         agentEventResponse   `json:"event"`
	Task          agentTaskResponse    `json:"task"`
	DecisionEvent *agentEventResponse  `json:"decision_event,omitempty"`
	ResolvedEvent *agentEventResponse  `json:"resolved_event,omitempty"`
}

type postAgentAttachmentResponse struct {
	Attachment agentMessageAttachment `json:"attachment"`
	Node       mediaNodeResponse      `json:"node"`
	Asset      assetResponse          `json:"asset"`
}

type postAgentDecisionResponse struct {
	Message       agentMessageResponse `json:"message"`
	DecisionEvent agentEventResponse   `json:"decision_event"`
	ResolvedEvent agentEventResponse   `json:"resolved_event"`
	Task          agentTaskResponse    `json:"task"`
}

func (h *AgentHandler) GetThread(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}

	thread, err := h.runtime.GetOrCreateProducerThread(ctx, workspace.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load agent thread")
		return
	}

	c.JSON(consts.StatusOK, getAgentThreadResponse{Thread: toAgentThreadResponse(thread)})
}

func (h *AgentHandler) GetProductionOverview(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}

	overview, err := agentoverview.NewBuilder(h.queries).Build(ctx, workspace.ID)
	if err != nil {
		slog.Error("failed to build agent production overview", "workspace_id", uuidToString(workspace.ID), "error", err)
		writeError(c, consts.StatusInternalServerError, "failed to load agent production overview")
		return
	}
	c.JSON(consts.StatusOK, overview)
}

func (h *AgentHandler) ListMessages(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}

	thread, err := h.runtime.GetOrCreateProducerThread(ctx, workspace.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load agent thread")
		return
	}

	messages, err := h.runtime.ListMessages(ctx, thread.ID, queryInt64(c, "after_seq", 0), queryInt32(c, "limit", 1000))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list agent messages")
		return
	}

	out := make([]agentMessageResponse, 0, len(messages))
	for _, msg := range messages {
		response := h.toAgentMessageResponse(ctx, msg)
		if msg.MessageType == "ui_card" && msg.EventID.Valid {
			if event, err := h.runtime.GetAgentEventForWorkspace(ctx, msg.EventID, workspace.ID); err == nil {
				hydrateDecisionCardFromEvent(response.Content, event)
			}
		}
		out = append(out, response)
	}

	c.JSON(consts.StatusOK, listAgentMessagesResponse{
		Thread:   toAgentThreadResponse(thread),
		Messages: out,
	})
}

func (h *AgentHandler) GetModelSelection(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}
	if h.modelSelection == nil {
		writeError(c, consts.StatusInternalServerError, "agent model selection is not configured")
		return
	}
	resolved, err := h.modelSelection.Resolve(ctx, workspace)
	if err != nil {
		status := consts.StatusInternalServerError
		if errors.Is(err, modelselection.ErrUnsupportedProducerModel) ||
			errors.Is(err, modelselection.ErrUnsupportedReasoningEffort) ||
			errors.Is(err, modelselection.ErrInvalidSelection) {
			status = consts.StatusBadRequest
		}
		writeError(c, status, "failed to resolve agent model selection")
		return
	}
	c.JSON(consts.StatusOK, toAgentModelSelectionResponse(resolved))
}

func (h *AgentHandler) PutModelSelection(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}
	if h.modelSelection == nil {
		writeError(c, consts.StatusInternalServerError, "agent model selection is not configured")
		return
	}
	if h.rejectIfAgentBusy(ctx, workspace.ID, c) {
		return
	}

	var req putAgentModelSelectionRequest
	if err := c.BindJSON(&req); err != nil || !req.valid() {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	selection := modelselection.Selection{
		Producer: modelselection.ModelRef{
			ProviderID:      req.Producer.ProviderID,
			ModelID:         req.Producer.ModelID,
			ReasoningEffort: req.Producer.ReasoningEffort,
		},
	}
	if _, err := h.modelSelection.ValidateProducerModel(ctx, selection.Producer); err != nil {
		writeError(c, consts.StatusBadRequest, "unsupported agent model")
		return
	}
	settings, err := modelselection.ApplyToWorkspaceSettings(workspace.Settings, selection)
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	updated, err := h.queries.UpdateWorkspaceSettings(ctx, db.UpdateWorkspaceSettingsParams{
		ID:       workspace.ID,
		Settings: settings,
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to update agent model selection")
		return
	}
	resolved, err := h.modelSelection.Resolve(ctx, updated)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to resolve agent model selection")
		return
	}
	c.JSON(consts.StatusOK, toAgentModelSelectionResponse(resolved))
}

func (h *AgentHandler) PostMessage(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}
	if h.rejectIfAgentProcessing(ctx, workspace.ID, c) {
		return
	}

	var req postAgentMessageRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if !req.valid() {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	thread, err := h.runtime.GetOrCreateProducerThread(ctx, workspace.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load agent thread")
		return
	}
	if handled := h.respondPendingDecisionWithMessage(ctx, c, workspace.ID, thread.ID, req); handled {
		return
	}
	if !h.agentMessageAttachmentsForWorkspace(ctx, workspace.ID, req.Attachments, c) {
		return
	}

	msg, err := h.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: workspace.ID,
		ThreadID:    thread.ID,
		Role:        "user",
		MessageType: "text",
		Content:     agentMessageContent(req.trimmedText(), req.ClientMessageID, req.Attachments),
		RawMessage:  []byte("{}"),
	})
	if err != nil {
		status := consts.StatusInternalServerError
		if errors.Is(err, agentruntime.ErrInvalidRequest) {
			status = consts.StatusBadRequest
		}
		writeError(c, status, "failed to append agent message")
		return
	}

	task, err := h.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: workspace.ID,
		ThreadID:    thread.ID,
		Role:        "producer",
		ScopeType:   "workspace",
		TaskType:    "producer_turn",
		MaxAttempts: 1,
		Input:       producerTurnTaskInput(msg),
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create producer task")
		return
	}

	event, err := h.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: workspace.ID,
		ThreadID:    thread.ID,
		TaskID:      task.ID,
		EventType:   "producer_turn_queued",
		SourceRole:  "user",
		TargetRole:  "producer",
		Scope:       agentMessageEventScope(thread.ID),
		Payload:     producerTurnQueuedPayload(msg, task),
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create agent event")
		return
	}

	messageResponse := h.toAgentMessageResponse(ctx, msg)
	eventResponse := toAgentEventResponse(event)
	taskResponse := toAgentTaskResponse(task)
	h.broadcastAgentMessage(ctx, workspace.ID, msg, event)
	h.broadcaster.BroadcastAgentTask(workspace.ID, task)
	h.broadcaster.BroadcastAgentEvent(workspace.ID, event)

	if h.producerRunner != nil {
		go func() {
			_ = h.producerRunner.RunTask(context.Background(), agentproducer.RunTaskInput{
				WorkspaceID:       workspace.ID,
				ThreadID:          thread.ID,
				TaskID:            task.ID,
				TriggerMessageID:  msg.ID,
				TriggerMessageSeq: msg.Seq,
			})
		}()
	}

	c.JSON(consts.StatusCreated, postAgentMessageResponse{
		Message: messageResponse,
		Event:   eventResponse,
		Task:    taskResponse,
	})
}

func (h *AgentHandler) PostDecision(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}
	if h.hitlService == nil {
		writeError(c, consts.StatusInternalServerError, "agent decision service is not configured")
		return
	}
	eventID, ok := uuidFromString(c.Param("eventID"))
	if !ok {
		writeError(c, consts.StatusNotFound, "decision not found")
		return
	}

	var req postAgentDecisionRequest
	if err := c.BindJSON(&req); err != nil || !req.valid() {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	output, err := h.hitlService.RespondDecision(ctx, agenthitl.RespondDecisionInput{
		WorkspaceID:      workspace.ID,
		EventID:          eventID,
		SelectedOptionID: strings.TrimSpace(req.SelectedOptionID),
		FreeText:         strings.TrimSpace(req.FreeText),
		ClientResponseID: strings.TrimSpace(req.ClientResponseID),
	})
	if err != nil {
		status := consts.StatusInternalServerError
		if errors.Is(err, agenthitl.ErrInvalidDecisionResponse) {
			status = consts.StatusBadRequest
		}
		writeError(c, status, "failed to submit decision")
		return
	}

	h.broadcastAgentMessage(ctx, workspace.ID, output.Message, output.DecisionEvent)
	h.broadcaster.BroadcastAgentEvent(workspace.ID, output.DecisionEvent)
	h.broadcaster.BroadcastAgentEvent(workspace.ID, output.ResolvedEvent)
	h.broadcaster.BroadcastAgentTask(workspace.ID, output.Task)

	if h.producerRunner != nil {
		go func() {
			_ = h.producerRunner.RunTask(context.Background(), producerDecisionResumeRunInput(workspace.ID, output.Task, output.Message.ID))
		}()
	}

	c.JSON(consts.StatusCreated, postAgentDecisionResponse{
		Message:       h.toAgentMessageResponse(ctx, output.Message),
		DecisionEvent: toAgentEventResponse(output.DecisionEvent),
		ResolvedEvent: toAgentEventResponse(output.ResolvedEvent),
		Task:          toAgentTaskResponse(output.Task),
	})
}

func (h *AgentHandler) respondPendingDecisionWithMessage(ctx context.Context, c *app.RequestContext, workspaceID pgtype.UUID, threadID pgtype.UUID, req postAgentMessageRequest) bool {
	event, ok, err := h.pendingDecisionForThread(ctx, workspaceID, threadID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load pending decision")
		return true
	}
	if !ok {
		return false
	}
	if h.hitlService == nil {
		writeError(c, consts.StatusInternalServerError, "agent decision service is not configured")
		return true
	}
	if len(req.Attachments) > 0 {
		writeError(c, consts.StatusBadRequest, "decision response does not support attachments yet")
		return true
	}

	output, err := h.hitlService.RespondDecision(ctx, agenthitl.RespondDecisionInput{
		WorkspaceID:      workspaceID,
		EventID:          event.ID,
		FreeText:         req.trimmedText(),
		ClientResponseID: strings.TrimSpace(req.ClientMessageID),
		AllowNaturalText: true,
	})
	if err != nil {
		status := consts.StatusInternalServerError
		if errors.Is(err, agenthitl.ErrInvalidDecisionResponse) {
			status = consts.StatusBadRequest
		}
		writeError(c, status, "failed to submit decision")
		return true
	}

	h.broadcastAgentMessage(ctx, workspaceID, output.Message, output.DecisionEvent)
	h.broadcaster.BroadcastAgentEvent(workspaceID, output.DecisionEvent)
	h.broadcaster.BroadcastAgentEvent(workspaceID, output.ResolvedEvent)
	h.broadcaster.BroadcastAgentTask(workspaceID, output.Task)

	if h.producerRunner != nil {
		go func() {
			_ = h.producerRunner.RunTask(context.Background(), producerDecisionResumeRunInput(workspaceID, output.Task, output.Message.ID))
		}()
	}

	decisionEvent := toAgentEventResponse(output.DecisionEvent)
	resolvedEvent := toAgentEventResponse(output.ResolvedEvent)
	c.JSON(consts.StatusCreated, postAgentMessageResponse{
		Message:       h.toAgentMessageResponse(ctx, output.Message),
		Event:         resolvedEvent,
		Task:          toAgentTaskResponse(output.Task),
		DecisionEvent: &decisionEvent,
		ResolvedEvent: &resolvedEvent,
	})
	return true
}

func (h *AgentHandler) pendingDecisionForThread(ctx context.Context, workspaceID pgtype.UUID, threadID pgtype.UUID) (db.AgentEvent, bool, error) {
	events, err := h.queries.ListAgentEventsByWorkspaceStatus(ctx, db.ListAgentEventsByWorkspaceStatusParams{
		WorkspaceID: workspaceID,
		Status:      "pending",
	})
	if err != nil {
		return db.AgentEvent{}, false, err
	}
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.EventType == "decision_requested" && event.ThreadID == threadID {
			return event, true, nil
		}
	}
	return db.AgentEvent{}, false, nil
}

func (h *AgentHandler) PostAttachment(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}
	if h.storage == nil {
		writeError(c, consts.StatusInternalServerError, "agent attachment storage is not configured")
		return
	}

	header, err := c.FormFile("file")
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if header.Size > maxUploadBytes {
		writeError(c, consts.StatusBadRequest, "file too large")
		return
	}
	file, err := header.Open()
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	defer func() { _ = file.Close() }()

	mime, err := detectMultipartMIME(file)
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	kind, ok := agentAttachmentKindForMIME(mime)
	if !ok {
		writeError(c, consts.StatusBadRequest, "unsupported attachment type")
		return
	}

	title := safeFilename(header.Filename)
	var asset db.MediaAsset
	var accessURL string
	if kind == "text" {
		raw, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
		if err != nil || len(raw) > maxUploadBytes {
			writeError(c, consts.StatusBadRequest, "invalid request")
			return
		}
		asset, err = h.queries.CreateTextMediaAsset(ctx, db.CreateTextMediaAssetParams{
			WorkspaceID: workspace.ID,
			TextContent: pgtype.Text{String: string(raw), Valid: true},
			SizeBytes:   pgtype.Int8{Int64: int64(len(raw)), Valid: true},
			Metadata:    jsonBytes(map[string]any{"filename": title}),
		})
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to create attachment")
			return
		}
	} else {
		objectName := fmt.Sprintf("assets/%d/%s", time.Now().UnixNano(), title)
		object, err := h.storage.Upload(ctx, workspace.ID, objectName, file, header.Size, mime)
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to store attachment")
			return
		}
		mediaType, _ := mediaTypeForMIME(mime)
		asset, err = h.queries.CreateMediaAsset(ctx, db.CreateMediaAssetParams{
			WorkspaceID: workspace.ID,
			Type:        mediaType,
			Mime:        mime,
			StorageUrl:  pgtype.Text{String: object.StorageURL, Valid: true},
			SizeBytes:   pgtype.Int8{Int64: header.Size, Valid: true},
			Metadata:    jsonBytes(map[string]any{"filename": title}),
		})
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to create attachment")
			return
		}
		accessURL, err = h.storage.PresignedGetURL(ctx, workspace.ID, objectName, 15*time.Minute)
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to create attachment")
			return
		}
	}

	nodeType := db.NodeType(kind)
	node, err := h.queries.CreateAgentMediaNode(ctx, db.CreateAgentMediaNodeParams{
		WorkspaceID: workspace.ID,
		NodeType:    nodeType,
		Title:       title,
		Prompt:      agentAttachmentPrompt(kind, title, asset),
		AssetID:     asset.ID,
		CanvasX:     120,
		CanvasY:     120,
		CanvasW:     280,
		CanvasH:     180,
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create attachment node")
		return
	}
	if h.canvasHub != nil {
		h.canvasHub.Broadcast(workspace.ID, CanvasEvent{Type: "NodeCreated", Payload: map[string]any{"node": node}})
	}

	attachment := agentMessageAttachment{
		AssetID:   uuidToString(asset.ID),
		NodeID:    uuidToString(node.ID),
		Kind:      kind,
		Name:      title,
		Mime:      asset.Mime,
		SizeBytes: assetSizeBytes(asset),
	}
	if accessURL != "" {
		attachment.URL = accessURL
		attachment.ThumbnailURL = accessURL
	}
	c.JSON(consts.StatusOK, postAgentAttachmentResponse{
		Attachment: attachment,
		Node:       toMediaNodeResponse(node),
		Asset:      assetResponse{MediaAsset: asset, AccessURL: accessURL},
	})
}

func (h *AgentHandler) broadcastAgentMessage(ctx context.Context, workspaceID pgtype.UUID, message db.AgentMessage, event db.AgentEvent) {
	h.hub.Broadcast(workspaceID, AgentSocketEvent{
		Type: "agent.message.created",
		Payload: map[string]any{
			"workspace_id": uuidToString(workspaceID),
			"thread_id":    uuidToString(message.ThreadID),
			"message":      h.toAgentMessageResponse(ctx, message),
			"event":        toAgentEventResponse(event),
		},
	})
}

func (h *AgentHandler) agentWorkspaceForRequest(ctx context.Context, c *app.RequestContext) (db.Workspace, bool) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return db.Workspace{}, false
	}
	workspaceID, ok := uuidFromString(c.Param("workspaceID"))
	if !ok {
		writeError(c, consts.StatusNotFound, "workspace not found")
		return db.Workspace{}, false
	}
	workspace, ok := workspaceForAccount(ctx, h.queries, workspaceID, accountID, c)
	if !ok {
		return db.Workspace{}, false
	}
	if workspace.Mode != db.WorkspaceModeAgent {
		writeError(c, consts.StatusForbidden, "workspace is not in agent mode")
		return db.Workspace{}, false
	}
	return workspace, true
}

func (h *AgentHandler) agentMessageAttachmentsForWorkspace(ctx context.Context, workspaceID pgtype.UUID, attachments []agentMessageAttachment, c *app.RequestContext) bool {
	for _, attachment := range attachments {
		assetID, ok := uuidFromString(attachment.AssetID)
		if !ok {
			writeError(c, consts.StatusBadRequest, "invalid attachment")
			return false
		}
		nodeID, ok := uuidFromString(attachment.NodeID)
		if !ok {
			writeError(c, consts.StatusBadRequest, "invalid attachment")
			return false
		}
		asset, err := h.queries.GetMediaAssetByID(ctx, assetID)
		if err != nil || asset.WorkspaceID != workspaceID {
			writeError(c, consts.StatusBadRequest, "invalid attachment")
			return false
		}
		node, err := h.queries.GetMediaNodeByID(ctx, nodeID)
		if err != nil || node.WorkspaceID != workspaceID || node.AssetID != assetID {
			writeError(c, consts.StatusBadRequest, "invalid attachment")
			return false
		}
	}
	return true
}

func (h *AgentHandler) rejectIfAgentBusy(ctx context.Context, workspaceID pgtype.UUID, c *app.RequestContext) bool {
	tasks, err := h.runtime.ListActiveAgentTasksByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load agent task state")
		return true
	}
	if reason := agentBusyReason(tasks); reason != "" {
		writeError(c, consts.StatusConflict, reason)
		return true
	}
	return false
}

func (h *AgentHandler) rejectIfAgentProcessing(ctx context.Context, workspaceID pgtype.UUID, c *app.RequestContext) bool {
	tasks, err := h.runtime.ListActiveAgentTasksByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load agent task state")
		return true
	}
	if reason := agentProcessingReason(tasks); reason != "" {
		writeError(c, consts.StatusConflict, reason)
		return true
	}
	return false
}

func agentBusyReason(tasks []db.AgentTask) string {
	for _, task := range tasks {
		switch task.Status {
		case "queued", "running":
			return "ClipAnvil 正在处理上一条消息，请稍后再试"
		case "waiting_for_user":
			return "ClipAnvil 正在等待你的选择，请先完成当前决策"
		}
	}
	return ""
}

func agentProcessingReason(tasks []db.AgentTask) string {
	for _, task := range tasks {
		switch task.Status {
		case "queued", "running":
			return "ClipAnvil 正在处理上一条消息，请稍后再试"
		}
	}
	return ""
}

func hydrateDecisionCardStatus(content map[string]any, status string) {
	hydrateDecisionCardFromEvent(content, db.AgentEvent{Status: status})
}

func hydrateDecisionCardFromEvent(content map[string]any, event db.AgentEvent) {
	var payload struct {
		Title         string `json:"title"`
		Message       string `json:"message"`
		Options       any    `json:"options"`
		AllowFreeText *bool  `json:"allow_free_text"`
	}
	if len(event.Payload) > 0 {
		_ = json.Unmarshal(event.Payload, &payload)
	}
	status := event.Status
	if status == "" {
		status = "pending"
	}
	blocks, ok := content["blocks"].([]any)
	if !ok {
		return
	}
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok || block["type"] != "decision_card" {
			continue
		}
		block["status"] = status
		if title := strings.TrimSpace(payload.Title); title != "" {
			block["title"] = title
		}
		if message := strings.TrimSpace(payload.Message); message != "" {
			block["message"] = message
		}
		if payload.AllowFreeText != nil {
			block["allow_free_text"] = *payload.AllowFreeText
		}
		if options := hydratedDecisionOptions(payload.Options); len(options) > 0 {
			block["options"] = options
		}
	}
}

func hydratedDecisionOptions(value any) []any {
	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "null" {
		return nil
	}
	var objectOptions []struct {
		ID          string `json:"id"`
		Label       string `json:"label"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &objectOptions); err == nil {
		out := make([]any, 0, len(objectOptions))
		for index, option := range objectOptions {
			label := strings.TrimSpace(option.Label)
			if label == "" {
				label = strings.TrimSpace(option.ID)
			}
			if label == "" {
				continue
			}
			id := strings.TrimSpace(option.ID)
			if id == "" {
				id = fmt.Sprintf("option_%d", index+1)
			}
			item := map[string]any{"id": id, "label": label}
			if description := strings.TrimSpace(option.Description); description != "" {
				item["description"] = description
			}
			out = append(out, item)
		}
		return out
	}

	var labels []string
	if err := json.Unmarshal(raw, &labels); err != nil {
		return nil
	}
	out := make([]any, 0, len(labels))
	for index, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		out = append(out, map[string]any{
			"id":    fmt.Sprintf("option_%d", index+1),
			"label": label,
		})
	}
	return out
}

type agentMessageAttachment struct {
	AssetID      string `json:"asset_id"`
	NodeID       string `json:"node_id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Mime         string `json:"mime"`
	SizeBytes    int64  `json:"size_bytes"`
	URL          string `json:"url,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

func agentMessageContent(text string, clientMessageID string, attachments ...[]agentMessageAttachment) []byte {
	var messageAttachments []agentMessageAttachment
	if len(attachments) > 0 && len(attachments[0]) > 0 {
		messageAttachments = attachments[0]
	}
	raw, err := uimessage.BuildUserMessageContent(uimessage.UserMessageInput{
		Text:            text,
		ClientMessageID: clientMessageID,
		Attachments:     toUIMessageAttachments(messageAttachments),
	})
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func toUIMessageAttachments(attachments []agentMessageAttachment) []uimessage.Attachment {
	out := make([]uimessage.Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, uimessage.Attachment{
			AssetID:   attachment.AssetID,
			NodeID:    attachment.NodeID,
			Kind:      attachment.Kind,
			Name:      attachment.Name,
			Mime:      attachment.Mime,
			SizeBytes: attachment.SizeBytes,
		})
	}
	return out
}

func (h *AgentHandler) toAgentMessageResponse(ctx context.Context, msg db.AgentMessage) agentMessageResponse {
	response := toAgentMessageResponse(msg)
	response.Content = h.hydrateAgentAttachmentURLs(ctx, msg.WorkspaceID, msg.ID, response.Content)
	return response
}

func (h *AgentHandler) hydrateAgentAttachmentURLs(ctx context.Context, workspaceID pgtype.UUID, messageID pgtype.UUID, content map[string]any) map[string]any {
	if h.storage == nil || len(content) == 0 {
		return content
	}
	blocks, ok := content["blocks"].([]any)
	if !ok {
		return content
	}
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok || block["type"] != "attachment" {
			continue
		}
		attachments, ok := block["attachments"].([]any)
		if !ok {
			continue
		}
		for _, rawAttachment := range attachments {
			attachment, ok := rawAttachment.(map[string]any)
			if !ok {
				continue
			}
			assetIDValue, _ := attachment["asset_id"].(string)
			assetID, ok := uuidFromString(assetIDValue)
			if !ok {
				continue
			}
			url, err := h.presignedAssetURL(ctx, workspaceID, assetID)
			if err != nil {
				slog.WarnContext(ctx, "failed to hydrate agent attachment url",
					"workspace_id", uuidToString(workspaceID),
					"message_id", uuidToString(messageID),
					"asset_id", assetIDValue,
					"error", err,
				)
				continue
			}
			if url != "" {
				attachment["url"] = url
				attachment["thumbnail_url"] = url
			}
		}
	}
	return content
}

func (h *AgentHandler) presignedAssetURL(ctx context.Context, workspaceID pgtype.UUID, assetID pgtype.UUID) (string, error) {
	asset, err := h.queries.GetMediaAssetByID(ctx, assetID)
	if err != nil {
		return "", err
	}
	if asset.WorkspaceID != workspaceID {
		return "", fmt.Errorf("asset workspace mismatch")
	}
	if !asset.StorageUrl.Valid || strings.TrimSpace(asset.StorageUrl.String) == "" {
		return "", nil
	}
	key, err := storage.KeyFromStorageURL(workspaceID, asset.StorageUrl.String)
	if err != nil {
		return "", err
	}
	return h.storage.PresignedGetURL(ctx, workspaceID, key, 15*time.Minute)
}

func agentAttachmentKindForMIME(mime string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.HasPrefix(normalized, "text/plain"):
		return "text", true
	case strings.HasPrefix(normalized, "image/"):
		if _, ok := mediaTypeForMIME(normalized); ok {
			return "image", true
		}
	case strings.HasPrefix(normalized, "video/"):
		if _, ok := mediaTypeForMIME(normalized); ok {
			return "video", true
		}
	}
	return "", false
}

func agentAttachmentPrompt(kind string, name string, asset db.MediaAsset) string {
	if kind == "text" && asset.TextContent.Valid {
		return asset.TextContent.String
	}
	return name
}

func assetSizeBytes(asset db.MediaAsset) int64 {
	if asset.SizeBytes.Valid {
		return asset.SizeBytes.Int64
	}
	return 0
}

func agentMessageEventScope(threadID pgtype.UUID) []byte {
	raw, err := json.Marshal(map[string]string{"thread_id": uuidToString(threadID)})
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func producerTurnTaskInput(msg db.AgentMessage) []byte {
	raw, err := json.Marshal(map[string]any{
		"trigger_message_id":  uuidToString(msg.ID),
		"trigger_message_seq": msg.Seq,
	})
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func producerDecisionResumeRunInput(workspaceID pgtype.UUID, task db.AgentTask, triggerMessageID pgtype.UUID) agentproducer.RunTaskInput {
	out := agentproducer.RunTaskInput{
		WorkspaceID:      workspaceID,
		ThreadID:         task.ThreadID,
		TaskID:           task.ID,
		TriggerMessageID: triggerMessageID,
	}
	var input struct {
		DecisionEventID  string   `json:"decision_event_id"`
		ResolvedEventID  string   `json:"resolved_event_id"`
		OriginalTaskID   string   `json:"original_task_id"`
		CheckpointKey    string   `json:"checkpoint_key"`
		InterruptIDs     []string `json:"interrupt_ids"`
		SelectedOptionID string   `json:"selected_option_id"`
		FreeText         string   `json:"free_text"`
	}
	if err := json.Unmarshal(task.Input, &input); err != nil {
		return out
	}
	out.ResumeCheckpointID = strings.TrimSpace(input.CheckpointKey)
	if originalTaskID, ok := uuidFromString(input.OriginalTaskID); ok {
		out.OriginalTaskID = originalTaskID
	}
	if len(input.InterruptIDs) > 0 {
		data := map[string]any{
			"decision_event_id":       input.DecisionEventID,
			"resolved_event_id":       input.ResolvedEventID,
			"selected_option_id":      input.SelectedOptionID,
			"free_text":               strings.TrimSpace(input.FreeText),
			"decision_resume_task_id": uuidToString(task.ID),
		}
		out.ResumeData = map[string]any{}
		for _, interruptID := range input.InterruptIDs {
			interruptID = strings.TrimSpace(interruptID)
			if interruptID != "" {
				out.ResumeData[interruptID] = data
			}
		}
	}
	return out
}

func producerTurnQueuedPayload(msg db.AgentMessage, task db.AgentTask) []byte {
	raw, err := json.Marshal(map[string]any{
		"message_id": uuidToString(msg.ID),
		"seq":        msg.Seq,
		"task_id":    uuidToString(task.ID),
	})
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func jsonBytes(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func queryInt64(c *app.RequestContext, key string, fallback int64) int64 {
	raw := string(c.Query(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func queryInt32(c *app.RequestContext, key string, fallback int32) int32 {
	raw := string(c.Query(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return fallback
	}
	return int32(value)
}
