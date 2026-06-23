package api

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type agentCanvasSnapshotStore interface {
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
	ListMediaAssetsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.MediaAsset, error)
	ListActiveStaleReasonsByNode(ctx context.Context, nodeID pgtype.UUID) ([]db.NodeStaleReason, error)
	ListReferencePackItemNodes(ctx context.Context, packNodeID pgtype.UUID) ([]db.MediaNode, error)
}

type AgentCanvasNodeBroadcaster struct {
	hub     *CanvasHub
	queries agentCanvasSnapshotStore
	storage assetURLSigner
}

func NewAgentCanvasNodeBroadcaster(hub *CanvasHub, queries agentCanvasSnapshotStore, storage assetURLSigner) *AgentCanvasNodeBroadcaster {
	return &AgentCanvasNodeBroadcaster{hub: hub, queries: queries, storage: storage}
}

func (b *AgentCanvasNodeBroadcaster) BroadcastAgentNodeCreated(workspaceID pgtype.UUID, node db.MediaNode) {
	if b == nil || b.hub == nil {
		return
	}
	response, err := b.canvasNodeResponse(context.Background(), workspaceID, node)
	if err != nil {
		b.hub.Broadcast(workspaceID, CanvasEvent{Type: "NodeCreated", Payload: map[string]any{"node": node}})
		return
	}
	b.hub.Broadcast(workspaceID, CanvasEvent{Type: "NodeCreated", Payload: map[string]any{"node": response}})
}

func (b *AgentCanvasNodeBroadcaster) canvasNodeResponse(ctx context.Context, workspaceID pgtype.UUID, node db.MediaNode) (canvasNodeResponse, error) {
	if b.queries == nil {
		responses := toCanvasNodeResponses([]db.MediaNode{node}, nil, nil, nil, nil)
		if len(responses) == 0 {
			return canvasNodeResponse{}, nil
		}
		return responses[0], nil
	}
	assets, err := b.queries.ListMediaAssetsByWorkspace(ctx, workspaceID)
	if err != nil {
		return canvasNodeResponse{}, err
	}
	assetsByID := make(map[pgtype.UUID]db.MediaAsset, len(assets))
	for _, asset := range assets {
		assetsByID[asset.ID] = asset
	}
	versionsByID := map[pgtype.UUID]db.ArtifactVersion{}
	if node.CurrentVersionID.Valid {
		version, err := b.queries.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil {
			return canvasNodeResponse{}, err
		}
		versionsByID[node.CurrentVersionID] = version
	}
	staleReasons, err := b.queries.ListActiveStaleReasonsByNode(ctx, node.ID)
	if err != nil {
		return canvasNodeResponse{}, err
	}
	packMembers := map[pgtype.UUID][]db.MediaNode{}
	if node.NodeType == db.NodeTypeReferencePack {
		members, err := b.queries.ListReferencePackItemNodes(ctx, node.ID)
		if err != nil {
			return canvasNodeResponse{}, err
		}
		packMembers[node.ID] = members
	}
	responses, err := toCanvasNodeResponsesWithSigner(ctx, b.storage, []db.MediaNode{node}, assetsByID, versionsByID, map[pgtype.UUID]int{node.ID: len(staleReasons)}, packMembers)
	if err != nil {
		return canvasNodeResponse{}, err
	}
	if len(responses) == 0 {
		return canvasNodeResponse{}, nil
	}
	return responses[0], nil
}
