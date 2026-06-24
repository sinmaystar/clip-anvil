package api

import (
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type mediaEdgeResponse struct {
	ID          pgtype.UUID        `json:"id"`
	WorkspaceID pgtype.UUID        `json:"workspace_id"`
	FromNodeID  pgtype.UUID        `json:"from_node_id"`
	ToNodeID    pgtype.UUID        `json:"to_node_id"`
	EdgeType    string             `json:"edge_type"`
	Source      string             `json:"source"`
	Metadata    json.RawMessage    `json:"metadata"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
}

func toMediaEdgeResponse(edge db.MediaEdge) mediaEdgeResponse {
	return mediaEdgeResponse{
		ID:          edge.ID,
		WorkspaceID: edge.WorkspaceID,
		FromNodeID:  edge.FromNodeID,
		ToNodeID:    edge.ToNodeID,
		EdgeType:    "dependency",
		Source:      edge.Source,
		Metadata:    jsonMetadata(edge.Metadata),
		CreatedAt:   edge.CreatedAt,
	}
}

func toMediaEdgeResponses(edges []db.MediaEdge) []mediaEdgeResponse {
	responses := make([]mediaEdgeResponse, 0, len(edges))
	for _, edge := range edges {
		responses = append(responses, toMediaEdgeResponse(edge))
	}
	return responses
}

func jsonMetadata(metadata []byte) json.RawMessage {
	if len(metadata) == 0 || !json.Valid(metadata) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(metadata)
}
