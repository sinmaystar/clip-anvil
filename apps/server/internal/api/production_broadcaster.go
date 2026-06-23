package api

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ProductionBroadcaster struct {
	hub     *CanvasHub
	queries *db.Queries
	storage assetURLSigner
}

func NewProductionBroadcaster(hub *CanvasHub, queries *db.Queries, storage assetURLSigner) *ProductionBroadcaster {
	return &ProductionBroadcaster{hub: hub, queries: queries, storage: storage}
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
	if shouldBroadcastProductionNodeSnapshot(event.Type) {
		b.broadcastNodeSnapshot(event)
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
	if b.queries == nil {
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
