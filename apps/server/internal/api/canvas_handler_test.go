package api

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestToCameraResponseMapsCanvasDocument(t *testing.T) {
	camera := toCameraResponse(db.CanvasDocument{
		CameraX:    12.5,
		CameraY:    -8,
		CameraZoom: 1.25,
	})

	if camera.X != 12.5 {
		t.Fatalf("camera x = %v, want %v", camera.X, 12.5)
	}
	if camera.Y != -8 {
		t.Fatalf("camera y = %v, want %v", camera.Y, -8)
	}
	if camera.Zoom != 1.25 {
		t.Fatalf("camera zoom = %v, want %v", camera.Zoom, 1.25)
	}
}

func TestCanvasResponseIncludesEdges(t *testing.T) {
	fromNodeID := pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true}
	toNodeID := pgtype.UUID{Bytes: [16]byte{0x02}, Valid: true}
	response := canvasResponse{
		Camera: cameraResponse{X: 1, Y: 2, Zoom: 1},
		Nodes:  []canvasNodeResponse{},
		Edges: []db.MediaEdge{
			{
				FromNodeID: fromNodeID,
				ToNodeID:   toNodeID,
				EdgeType:   db.EdgeTypeDependency,
			},
		},
	}

	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal canvas response: %v", err)
	}

	var payload struct {
		Edges []db.MediaEdge `json:"edges"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal canvas response: %v", err)
	}
	if len(payload.Edges) != 1 {
		t.Fatalf("edges len = %d, want 1", len(payload.Edges))
	}
	if payload.Edges[0].FromNodeID != fromNodeID {
		t.Fatalf("from node id = %v, want %v", payload.Edges[0].FromNodeID, fromNodeID)
	}
	if payload.Edges[0].ToNodeID != toNodeID {
		t.Fatalf("to node id = %v, want %v", payload.Edges[0].ToNodeID, toNodeID)
	}
}

func TestCanvasResponseIncludesGroups(t *testing.T) {
	groupID := pgtype.UUID{Bytes: [16]byte{0x03}, Valid: true}
	nodeID := pgtype.UUID{Bytes: [16]byte{0x04}, Valid: true}
	response := canvasResponse{
		Camera: cameraResponse{X: 1, Y: 2, Zoom: 1},
		Nodes:  []canvasNodeResponse{},
		Edges:  []db.MediaEdge{},
		Groups: []canvasGroupResponse{
			{
				ID:      groupID,
				Name:    "分镜组",
				NodeIDs: []pgtype.UUID{nodeID},
			},
		},
	}

	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal canvas response: %v", err)
	}

	var payload struct {
		Groups []canvasGroupResponse `json:"groups"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal canvas response: %v", err)
	}
	if len(payload.Groups) != 1 {
		t.Fatalf("groups len = %d, want 1", len(payload.Groups))
	}
	if payload.Groups[0].ID != groupID {
		t.Fatalf("group id = %v, want %v", payload.Groups[0].ID, groupID)
	}
	if len(payload.Groups[0].NodeIDs) != 1 || payload.Groups[0].NodeIDs[0] != nodeID {
		t.Fatalf("group node ids = %v, want [%v]", payload.Groups[0].NodeIDs, nodeID)
	}
}

func TestCanvasNodeResponsesIncludeAssetThumbnailURL(t *testing.T) {
	assetID := pgtype.UUID{Bytes: [16]byte{0x05}, Valid: true}
	node := db.MediaNode{
		ID:      pgtype.UUID{Bytes: [16]byte{0x06}, Valid: true},
		AssetID: assetID,
	}
	assets := map[pgtype.UUID]db.MediaAsset{
		assetID: {
			ID:           assetID,
			ThumbnailUrl: pgtype.Text{String: "https://assets.local/thumb.png", Valid: true},
		},
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, assets)

	if len(nodes) != 1 {
		t.Fatalf("nodes len = %d, want 1", len(nodes))
	}
	if nodes[0].ThumbnailURL == nil {
		t.Fatal("thumbnail url should be set")
	}
	if *nodes[0].ThumbnailURL != "https://assets.local/thumb.png" {
		t.Fatalf("thumbnail url = %q, want thumb url", *nodes[0].ThumbnailURL)
	}
}
