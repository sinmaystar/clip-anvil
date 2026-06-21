package api

import "github.com/sinmaystar/clip-anvil/internal/production"

type ProductionBroadcaster struct {
	hub *CanvasHub
}

func NewProductionBroadcaster(hub *CanvasHub) *ProductionBroadcaster {
	return &ProductionBroadcaster{hub: hub}
}

func (b *ProductionBroadcaster) PublishProductionEvent(event production.ProductionEvent) {
	if b == nil || b.hub == nil {
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
	switch event.Type {
	case production.ProductionEventModelStreamDelta:
		b.hub.Broadcast(event.WorkspaceID, CanvasEvent{Type: "production.model.delta", Payload: payload})
	default:
		b.hub.Broadcast(event.WorkspaceID, CanvasEvent{Type: "production.job.updated", Payload: payload})
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
