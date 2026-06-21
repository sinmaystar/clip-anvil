package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/storage"
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
		Edges:  toMediaEdgeResponses([]db.MediaEdge{{FromNodeID: fromNodeID, ToNodeID: toNodeID}}),
	}

	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal canvas response: %v", err)
	}

	var payload struct {
		Edges []mediaEdgeResponse `json:"edges"`
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
	if payload.Edges[0].EdgeType != "dependency" {
		t.Fatalf("edge type = %q, want dependency", payload.Edges[0].EdgeType)
	}
}

func TestCanvasResponseIncludesGroups(t *testing.T) {
	groupID := pgtype.UUID{Bytes: [16]byte{0x03}, Valid: true}
	nodeID := pgtype.UUID{Bytes: [16]byte{0x04}, Valid: true}
	response := canvasResponse{
		Camera: cameraResponse{X: 1, Y: 2, Zoom: 1},
		Nodes:  []canvasNodeResponse{},
		Edges:  []mediaEdgeResponse{},
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

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, assets, nil, nil, nil)

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

func TestCanvasNodeResponsesIncludeCurrentVersionPreview(t *testing.T) {
	assetID := pgtype.UUID{Bytes: [16]byte{0x07}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{0x08}, Valid: true}
	node := db.MediaNode{
		ID:               pgtype.UUID{Bytes: [16]byte{0x09}, Valid: true},
		CurrentVersionID: versionID,
	}
	versions := map[pgtype.UUID]db.ArtifactVersion{
		versionID: {
			ID:        versionID,
			AssetID:   assetID,
			VersionNo: 2,
			Winner:    true,
			InputHash: "sha256:abcdef1234567890",
		},
	}
	assets := map[pgtype.UUID]db.MediaAsset{
		assetID: {
			ID:          assetID,
			Type:        db.AssetTypeText,
			TextContent: pgtype.Text{String: "Generated winner text", Valid: true},
		},
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, assets, versions, nil, nil)

	if len(nodes) != 1 {
		t.Fatalf("nodes len = %d, want 1", len(nodes))
	}
	if nodes[0].ProductionPreview == nil {
		t.Fatal("production preview should be set")
	}
	if nodes[0].ProductionPreview.VersionNo != 2 {
		t.Fatalf("version no = %d, want 2", nodes[0].ProductionPreview.VersionNo)
	}
	if nodes[0].ProductionPreview.Text != "Generated winner text" {
		t.Fatalf("preview text = %q, want generated text", nodes[0].ProductionPreview.Text)
	}
}

func TestCanvasNodeResponsesIncludeDirectAssetPreview(t *testing.T) {
	workspaceID := pgtype.UUID{Bytes: [16]byte{0x31}, Valid: true}
	assetID := pgtype.UUID{Bytes: [16]byte{0x32}, Valid: true}
	node := db.MediaNode{
		ID:            pgtype.UUID{Bytes: [16]byte{0x33}, Valid: true},
		WorkspaceID:   workspaceID,
		NodeType:      db.NodeTypeImage,
		AssetID:       assetID,
		OperationType: "upload",
		Status:        db.NodeStatusSucceeded,
	}
	assets := map[pgtype.UUID]db.MediaAsset{
		assetID: {
			ID:          assetID,
			WorkspaceID: workspaceID,
			Type:        db.AssetTypeImage,
			Mime:        "image/png",
			StorageUrl:  pgtype.Text{String: storage.StorageURL(workspaceID, "uploads/product.png"), Valid: true},
			Metadata:    []byte(`{"width":1024,"height":1024}`),
		},
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, assets, nil, nil, nil)

	if nodes[0].ProductionPreview == nil {
		t.Fatal("direct asset source node should have a production preview")
	}
	if got := nodes[0].ProductionPreview.AssetID; got != uuidToString(assetID) {
		t.Fatalf("asset id = %q, want %s", got, uuidToString(assetID))
	}
	if got := nodes[0].ProductionPreview.AssetType; got != "image" {
		t.Fatalf("asset type = %q, want image", got)
	}
	if nodes[0].ProductionPreview.Width != 1024 || nodes[0].ProductionPreview.Height != 1024 {
		t.Fatalf("dimensions = %dx%d, want 1024x1024", nodes[0].ProductionPreview.Width, nodes[0].ProductionPreview.Height)
	}
}

func TestCanvasNodeResponsesIncludePreviewDimensions(t *testing.T) {
	assetID := pgtype.UUID{Bytes: [16]byte{0x21}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{0x22}, Valid: true}
	node := db.MediaNode{
		ID:               pgtype.UUID{Bytes: [16]byte{0x23}, Valid: true},
		CurrentVersionID: versionID,
	}
	versions := map[pgtype.UUID]db.ArtifactVersion{
		versionID: {
			ID:        versionID,
			AssetID:   assetID,
			VersionNo: 1,
			Winner:    true,
			InputHash: "sha256:image",
		},
	}
	assets := map[pgtype.UUID]db.MediaAsset{
		assetID: {
			ID:       assetID,
			Type:     db.AssetTypeImage,
			Mime:     "image/png",
			Metadata: []byte(`{"width":1024,"height":576}`),
		},
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, assets, versions, nil, nil)

	if nodes[0].ProductionPreview == nil {
		t.Fatal("production preview should be set")
	}
	if nodes[0].ProductionPreview.Width != 1024 {
		t.Fatalf("width = %d, want 1024", nodes[0].ProductionPreview.Width)
	}
	if nodes[0].ProductionPreview.Height != 576 {
		t.Fatalf("height = %d, want 576", nodes[0].ProductionPreview.Height)
	}
}

func TestCanvasNodeResponsesIncludePreviewDuration(t *testing.T) {
	assetID := pgtype.UUID{Bytes: [16]byte{0x24}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{0x25}, Valid: true}
	node := db.MediaNode{
		ID:               pgtype.UUID{Bytes: [16]byte{0x26}, Valid: true},
		CurrentVersionID: versionID,
	}
	versions := map[pgtype.UUID]db.ArtifactVersion{
		versionID: {
			ID:        versionID,
			AssetID:   assetID,
			VersionNo: 1,
			Winner:    true,
			InputHash: "sha256:video",
		},
	}
	assets := map[pgtype.UUID]db.MediaAsset{
		assetID: {
			ID:         assetID,
			Type:       db.AssetTypeVideo,
			Mime:       "video/mp4",
			DurationMs: pgtype.Int4{Int32: 5000, Valid: true},
			Metadata:   []byte(`{"width":1280,"height":720}`),
		},
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, assets, versions, nil, nil)

	if nodes[0].ProductionPreview == nil {
		t.Fatal("production preview should be set")
	}
	if nodes[0].ProductionPreview.DurationMS != 5000 {
		t.Fatalf("duration = %d, want 5000", nodes[0].ProductionPreview.DurationMS)
	}
	if nodes[0].ProductionPreview.Width != 1280 || nodes[0].ProductionPreview.Height != 720 {
		t.Fatalf("dimensions = %dx%d, want 1280x720", nodes[0].ProductionPreview.Width, nodes[0].ProductionPreview.Height)
	}
}

func TestCanvasNodeResponsesSignProductionPreviewAssetURL(t *testing.T) {
	workspaceID := pgtype.UUID{Bytes: [16]byte{0x07}, Valid: true}
	assetID := pgtype.UUID{Bytes: [16]byte{0x08}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{0x09}, Valid: true}
	node := db.MediaNode{
		ID:               pgtype.UUID{Bytes: [16]byte{0x0a}, Valid: true},
		WorkspaceID:      workspaceID,
		CurrentVersionID: versionID,
	}
	versions := map[pgtype.UUID]db.ArtifactVersion{
		versionID: {
			ID:        versionID,
			AssetID:   assetID,
			VersionNo: 1,
		},
	}
	assets := map[pgtype.UUID]db.MediaAsset{
		assetID: {
			ID:          assetID,
			WorkspaceID: workspaceID,
			Type:        db.AssetTypeImage,
			Mime:        "image/jpeg",
			StorageUrl:  pgtype.Text{String: storage.StorageURL(workspaceID, "production/image.jpg"), Valid: true},
		},
	}
	signer := &fakeCanvasAssetSigner{url: "http://signed.local/production/image.jpg"}

	nodes, err := toCanvasNodeResponsesWithSigner(context.Background(), signer, []db.MediaNode{node}, assets, versions, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if got := nodes[0].ProductionPreview.AccessURL; got != signer.url {
		t.Fatalf("access url = %q, want signed url", got)
	}
	if signer.key != "production/image.jpg" {
		t.Fatalf("signed key = %q, want production/image.jpg", signer.key)
	}
}

func TestCanvasNodeResponsesIncludeActiveStaleReasonCount(t *testing.T) {
	nodeID := pgtype.UUID{Bytes: [16]byte{0x0a}, Valid: true}
	node := db.MediaNode{ID: nodeID, Status: db.NodeStatusStale}
	reasonsByNode := map[pgtype.UUID]int{nodeID: 2}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, nil, nil, reasonsByNode, nil)

	if nodes[0].ActiveStaleReasonCount != 2 {
		t.Fatalf("stale reason count = %d, want 2", nodes[0].ActiveStaleReasonCount)
	}
}

func TestCanvasNodeResponsesIncludeReferencePackPreview(t *testing.T) {
	packID := pgtype.UUID{Bytes: [16]byte{0x21}, Valid: true}
	memberAID := pgtype.UUID{Bytes: [16]byte{0x22}, Valid: true}
	memberBID := pgtype.UUID{Bytes: [16]byte{0x23}, Valid: true}
	pack := db.MediaNode{
		ID:       packID,
		NodeType: db.NodeTypeReferencePack,
		Title:    "商品参考包",
	}
	packMembers := map[pgtype.UUID][]db.MediaNode{
		packID: {
			{
				ID:            memberAID,
				NodeType:      db.NodeTypeImage,
				Title:         "主图",
				Status:        db.NodeStatusSucceeded,
				OperationType: "upload",
				AssetID:       pgtype.UUID{Bytes: [16]byte{0x25}, Valid: true},
			},
			{ID: memberBID, NodeType: db.NodeTypeVideo, Title: "动作参考", Status: db.NodeStatusSucceeded},
		},
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{pack}, nil, nil, nil, packMembers)

	if nodes[0].ReferencePackPreview == nil {
		t.Fatal("reference pack preview should be set")
	}
	if nodes[0].ReferencePackPreview.MemberCount != 2 {
		t.Fatalf("member count = %d, want 2", nodes[0].ReferencePackPreview.MemberCount)
	}
	if len(nodes[0].ReferencePackPreview.Members) != 2 {
		t.Fatalf("members len = %d, want 2", len(nodes[0].ReferencePackPreview.Members))
	}
	if nodes[0].ReferencePackPreview.Members[0].Title != "主图" {
		t.Fatalf("first member = %#v", nodes[0].ReferencePackPreview.Members[0])
	}
	if nodes[0].ReferencePackPreview.Members[0].OperationType != "upload" ||
		nodes[0].ReferencePackPreview.Members[0].AssetID == "" {
		t.Fatalf("first member source fields = %#v", nodes[0].ReferencePackPreview.Members[0])
	}
}

func TestCanvasNodeResponsesDoNotAttachReferencePackPreviewToNormalNodes(t *testing.T) {
	node := db.MediaNode{
		ID:       pgtype.UUID{Bytes: [16]byte{0x24}, Valid: true},
		NodeType: db.NodeTypeImage,
		Title:    "普通图片",
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, nil, nil, nil, nil)

	if nodes[0].ReferencePackPreview != nil {
		t.Fatalf("normal node preview = %#v, want nil", nodes[0].ReferencePackPreview)
	}
}

type fakeCanvasAssetSigner struct {
	url string
	key string
}

func (f *fakeCanvasAssetSigner) PresignedGetURL(_ context.Context, _ pgtype.UUID, key string, _ time.Duration) (string, error) {
	f.key = key
	return f.url, nil
}
