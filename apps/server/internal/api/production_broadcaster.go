package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/preview"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	agentvideo "github.com/sinmaystar/clip-anvil/internal/agent/video"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ProductionBroadcaster struct {
	hub              *CanvasHub
	queries          productionBroadcasterStore
	storage          assetURLSigner
	agentPreviewSink agentPreviewEventSink
}

type productionBroadcasterStore interface {
	GetMediaNodeByID(ctx context.Context, id pgtype.UUID) (db.MediaNode, error)
	GetGenerationJobByID(ctx context.Context, id pgtype.UUID) (db.GenerationJob, error)
	GetAgentTaskByID(ctx context.Context, id pgtype.UUID) (db.AgentTask, error)
	GetArtifactVersionByJobID(ctx context.Context, jobID pgtype.UUID) (db.ArtifactVersion, error)
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
	UpdateShotStatus(ctx context.Context, params db.UpdateShotStatusParams) (db.Shot, error)
	ListMediaAssetsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaAsset, error)
	ListActiveStaleReasonsByNode(ctx context.Context, nodeID pgtype.UUID) ([]db.NodeStaleReason, error)
	ListReferencePackItemNodes(ctx context.Context, packNodeID pgtype.UUID) ([]db.MediaNode, error)
}

type agentPreviewEventSink interface {
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
	BroadcastAgentEvent(workspaceID pgtype.UUID, event db.AgentEvent)
	GetOrCreateProducerThread(ctx context.Context, workspaceID pgtype.UUID) (db.AgentThread, error)
	CreateProducerPendingSignal(ctx context.Context, params agentruntime.CreateProducerPendingSignalParams) (db.ProducerPendingSignal, error)
	ListActiveAgentTasksByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.AgentTask, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	EnqueueProducerTask(ctx context.Context, task db.AgentTask)
}

func NewProductionBroadcaster(hub *CanvasHub, queries productionBroadcasterStore, storage assetURLSigner) *ProductionBroadcaster {
	return &ProductionBroadcaster{hub: hub, queries: queries, storage: storage}
}

func (b *ProductionBroadcaster) SetAgentPreviewEventSink(sink agentPreviewEventSink) {
	if b == nil {
		return
	}
	b.agentPreviewSink = sink
}

func (b *ProductionBroadcaster) PublishProductionEvent(event production.ProductionEvent) {
	if b == nil {
		return
	}
	payload := map[string]any{
		"workspace_id": uuidString(event.WorkspaceID),
		"node_id":      uuidString(event.TargetNodeID),
		"job_id":       uuidString(event.JobID),
		"status":       statusForProductionEvent(event.Type),
		"progress":     event.Progress,
	}
	for key, value := range event.Payload {
		payload[key] = value
	}
	if b.hub != nil {
		switch event.Type {
		case production.ProductionEventModelStreamDelta:
			b.hub.Broadcast(event.WorkspaceID, CanvasEvent{Type: "production.model.delta", Payload: payload})
		default:
			b.hub.Broadcast(event.WorkspaceID, CanvasEvent{Type: "production.job.updated", Payload: payload})
		}
	}
	if shouldBroadcastProductionNodeSnapshot(event.Type) {
		b.broadcastNodeSnapshot(event)
		b.publishAgentProductionTerminalEvent(event)
	}
}

func statusForProductionEvent(eventType string) string {
	switch eventType {
	case production.ProductionEventJobStarted:
		return "running"
	case production.ProductionEventJobSucceeded:
		return "succeeded"
	case production.ProductionEventJobFailed:
		return "failed"
	case production.ProductionEventJobCancelled:
		return "cancelled"
	default:
		return "running"
	}
}

func shouldBroadcastProductionNodeSnapshot(eventType string) bool {
	switch eventType {
	case production.ProductionEventJobSucceeded,
		production.ProductionEventJobFailed,
		production.ProductionEventJobCancelled:
		return true
	default:
		return false
	}
}

func (b *ProductionBroadcaster) broadcastNodeSnapshot(event production.ProductionEvent) {
	if b.queries == nil || b.hub == nil {
		return
	}
	ctx := context.Background()
	node, err := b.queries.GetMediaNodeByID(ctx, event.TargetNodeID)
	if err != nil {
		return
	}
	assets, err := b.queries.ListMediaAssetsByWorkspace(ctx, event.WorkspaceID)
	if err != nil {
		return
	}
	assetsByID := make(map[pgtype.UUID]db.MediaAsset, len(assets))
	for _, asset := range assets {
		assetsByID[asset.ID] = asset
	}
	versionsByID := map[pgtype.UUID]db.ArtifactVersion{}
	if node.CurrentVersionID.Valid {
		version, err := b.queries.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil {
			return
		}
		versionsByID[node.CurrentVersionID] = version
	}
	staleReasons, err := b.queries.ListActiveStaleReasonsByNode(ctx, node.ID)
	if err != nil {
		return
	}
	packMembers := map[pgtype.UUID][]db.MediaNode{}
	if node.NodeType == db.NodeTypeReferencePack {
		members, err := b.queries.ListReferencePackItemNodes(ctx, node.ID)
		if err != nil {
			return
		}
		packMembers[node.ID] = members
	}
	responses, err := toCanvasNodeResponsesWithSigner(ctx, b.storage, []db.MediaNode{node}, assetsByID, versionsByID, map[pgtype.UUID]int{node.ID: len(staleReasons)}, packMembers)
	if err != nil || len(responses) == 0 {
		return
	}
	b.hub.Broadcast(event.WorkspaceID, CanvasEvent{Type: "NodeUpdated", Payload: map[string]any{"node": responses[0]}})
}

func (b *ProductionBroadcaster) publishAgentProductionTerminalEvent(event production.ProductionEvent) {
	if b.queries == nil || b.agentPreviewSink == nil {
		return
	}
	ctx := context.Background()
	node, err := b.queries.GetMediaNodeByID(ctx, event.TargetNodeID)
	if err != nil {
		return
	}
	artifactKind, ok := agentArtifactKind(node)
	if !ok {
		return
	}
	shotStatus, agentEventType, ok := terminalAgentEventForProductionEvent(artifactKind, event.Type)
	if !ok {
		return
	}
	job, err := b.queries.GetGenerationJobByID(ctx, event.JobID)
	if err != nil || (job.RequestedByType != "agent_worker" && job.RequestedByType != "agent_composer") {
		return
	}
	if node.ShotID.Valid && shotStatus != "" {
		_, _ = b.queries.UpdateShotStatus(ctx, db.UpdateShotStatusParams{
			ID:          node.ShotID,
			WorkspaceID: event.WorkspaceID,
			Status:      shotStatus,
		})
	}
	task := db.AgentTask{}
	if taskID, ok := uuidFromText(job.RequestedByID); ok {
		if loaded, err := b.queries.GetAgentTaskByID(ctx, taskID); err == nil {
			task = loaded
		}
	}
	versionID := ""
	if version, err := b.queries.GetArtifactVersionByJobID(ctx, event.JobID); err == nil {
		versionID = uuidString(version.ID)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return
	}
	payload := map[string]any{
		"node_id":             uuidString(node.ID),
		"shot_id":             uuidString(node.ShotID),
		"job_id":              uuidString(event.JobID),
		"artifact_version_id": versionID,
		"status":              statusForProductionEvent(event.Type),
		"progress":            event.Progress,
	}
	if event.Err != nil {
		payload["error"] = event.Err.Error()
	}
	agentEvent, err := b.agentPreviewSink.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: event.WorkspaceID,
		ThreadID:    task.ThreadID,
		TaskID:      task.ID,
		EventType:   agentEventType,
		SourceRole:  "system",
		TargetRole:  "producer",
		Scope:       productionBroadcastJSON(map[string]any{"shot_id": uuidString(node.ShotID), "node_id": uuidString(node.ID), "artifact_kind": artifactKind}),
		Payload:     productionBroadcastJSON(payload),
	})
	if err != nil {
		return
	}
	b.agentPreviewSink.BroadcastAgentEvent(event.WorkspaceID, agentEvent)
	b.publishProducerWorkerCompletionSignal(ctx, event, node, artifactKind, job, task, versionID)
}

func (b *ProductionBroadcaster) publishProducerWorkerCompletionSignal(ctx context.Context, event production.ProductionEvent, node db.MediaNode, artifactKind string, job db.GenerationJob, task db.AgentTask, versionID string) {
	if b == nil || b.agentPreviewSink == nil || job.RequestedByType != "agent_worker" || !task.ID.Valid || !event.JobID.Valid || !event.WorkspaceID.Valid {
		return
	}
	thread, err := b.agentPreviewSink.GetOrCreateProducerThread(ctx, event.WorkspaceID)
	if err != nil || !thread.ID.Valid {
		return
	}
	status := statusForProductionEvent(event.Type)
	targetPhase := targetPhaseForAgentArtifactKind(artifactKind)
	payload := productionBroadcastJSON(map[string]any{
		"trigger":             "worker_generation_completed",
		"target_phase":        targetPhase,
		"render_plan_id":      uuidString(task.RenderPlanID),
		"render_plan_status":  status,
		"scope_type":          task.ScopeType,
		"scope_id":            uuidString(task.ScopeID),
		"shot_id":             uuidString(node.ShotID),
		"node_id":             uuidString(node.ID),
		"generation_job_id":   uuidString(event.JobID),
		"artifact_version_id": versionID,
		"artifact_kind":       artifactKind,
		"worker_task_id":      uuidString(task.ID),
		"worker_thread_id":    uuidString(task.ThreadID),
	})
	_, err = b.agentPreviewSink.CreateProducerPendingSignal(ctx, agentruntime.CreateProducerPendingSignalParams{
		WorkspaceID:      event.WorkspaceID,
		ProducerThreadID: thread.ID,
		SourceRole:       "worker",
		SourceTaskID:     task.ID,
		SourceThreadID:   task.ThreadID,
		SignalType:       "worker_generation_completed",
		ScopeType:        defaultSignalScopeType(task.ScopeType),
		ScopeID:          task.ScopeID,
		RenderPlanID:     task.RenderPlanID,
		Priority:         70,
		DedupeKey:        "worker_generation_completed:" + uuidString(event.JobID),
		Payload:          payload,
	})
	if err != nil {
		return
	}
	b.ensureProducerWakeTask(ctx, event.WorkspaceID, thread.ID, payload)
}

func (b *ProductionBroadcaster) ensureProducerWakeTask(ctx context.Context, workspaceID, producerThreadID pgtype.UUID, input []byte) {
	if b == nil || b.agentPreviewSink == nil || !workspaceID.Valid || !producerThreadID.Valid {
		return
	}
	activeTasks, err := b.agentPreviewSink.ListActiveAgentTasksByWorkspace(ctx, workspaceID)
	if err != nil {
		return
	}
	for _, task := range activeTasks {
		if task.Role == "producer" &&
			(task.TaskType == "producer_turn" || task.TaskType == "decision_resume") &&
			(task.Status == "queued" || task.Status == "waiting_for_user") {
			return
		}
	}
	task, err := b.agentPreviewSink.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: workspaceID,
		ThreadID:    producerThreadID,
		Role:        "producer",
		ScopeType:   "workspace",
		TaskType:    "producer_turn",
		MaxAttempts: 1,
		Input:       input,
	})
	if err != nil {
		return
	}
	b.agentPreviewSink.EnqueueProducerTask(ctx, task)
}

func productionBroadcastJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func previewEventForProductionEvent(eventType string) (string, string, bool) {
	switch eventType {
	case production.ProductionEventJobSucceeded:
		return preview.EventSucceeded, "preview_generation_succeeded", true
	case production.ProductionEventJobFailed, production.ProductionEventJobCancelled:
		return preview.EventFailed, "preview_generation_failed", true
	default:
		return "", "", false
	}
}

func terminalAgentEventForProductionEvent(artifactKind string, eventType string) (string, string, bool) {
	switch artifactKind {
	case "preview_image":
		statusEvent, agentEventType, ok := previewEventForProductionEvent(eventType)
		if !ok {
			return "", "", false
		}
		shotStatus, ok := preview.ShotStatusForEvent(statusEvent)
		return shotStatus, agentEventType, ok
	case "shot_video":
		switch eventType {
		case production.ProductionEventJobSucceeded:
			shotStatus, ok := agentvideo.ShotStatusForEvent(agentvideo.EventSucceeded)
			return shotStatus, agentvideo.EventSucceeded, ok
		case production.ProductionEventJobFailed, production.ProductionEventJobCancelled:
			shotStatus, ok := agentvideo.ShotStatusForEvent(agentvideo.EventFailed)
			return shotStatus, agentvideo.EventFailed, ok
		default:
			return "", "", false
		}
	case "final_video":
		switch eventType {
		case production.ProductionEventJobSucceeded:
			return "", "composition_succeeded", true
		case production.ProductionEventJobFailed, production.ProductionEventJobCancelled:
			return "", "composition_failed", true
		default:
			return "", "", false
		}
	default:
		return "", "", false
	}
}

func targetPhaseForAgentArtifactKind(artifactKind string) string {
	switch artifactKind {
	case "preview_image":
		return "preview_image"
	case "shot_video":
		return "shot_video"
	default:
		return artifactKind
	}
}

func defaultSignalScopeType(scopeType string) string {
	if strings.TrimSpace(scopeType) == "" {
		return "workspace"
	}
	return scopeType
}

func agentArtifactKind(node db.MediaNode) (string, bool) {
	if node.Source != "" && node.Source != "agent" {
		return "", false
	}
	var metadata map[string]any
	if len(node.Metadata) > 0 {
		_ = json.Unmarshal(node.Metadata, &metadata)
	}
	kind, _ := metadata["agent_artifact_kind"].(string)
	switch kind {
	case "preview_image", "shot_video", "final_video":
		return kind, true
	default:
		return "", false
	}
}

func uuidFromText(value pgtype.Text) (pgtype.UUID, bool) {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return pgtype.UUID{}, false
	}
	parsed, err := uuid.Parse(strings.TrimSpace(value.String))
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}
