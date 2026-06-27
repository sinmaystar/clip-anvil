package api

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestAgentWorkbenchSelectsBestArtifactNode(t *testing.T) {
	shotID := uuidWithByteForWorkbenchTest(1)
	oldSucceeded := db.MediaNode{
		ID:               uuidWithByteForWorkbenchTest(11),
		ShotID:           shotID,
		Source:           "agent",
		NodeType:         db.NodeTypeImage,
		Title:            "old preview",
		Status:           db.NodeStatusSucceeded,
		CurrentVersionID: uuidWithByteForWorkbenchTest(101),
		Metadata:         []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt:        pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true},
	}
	running := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(12),
		ShotID:    shotID,
		Source:    "agent",
		NodeType:  db.NodeTypeImage,
		Title:     "running preview",
		Status:    db.NodeStatusRunning,
		Metadata:  []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(200, 0), Valid: true},
	}
	got := bestAgentArtifactNode([]db.MediaNode{running, oldSucceeded}, "preview_image")
	if got == nil || got.ID != oldSucceeded.ID {
		t.Fatalf("best node = %#v, want succeeded current version node", got)
	}
}

func TestAgentWorkbenchSelectsNewestNodeWithinSameRank(t *testing.T) {
	older := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(21),
		Source:    "agent",
		NodeType:  db.NodeTypeImage,
		Status:    db.NodeStatusFailed,
		Metadata:  []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true},
	}
	newer := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(22),
		Source:    "agent",
		NodeType:  db.NodeTypeImage,
		Status:    db.NodeStatusFailed,
		Metadata:  []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(200, 0), Valid: true},
	}
	got := bestAgentArtifactNode([]db.MediaNode{older, newer}, "preview_image")
	if got == nil || got.ID != newer.ID {
		t.Fatalf("best node = %#v, want newest same-rank node", got)
	}
}

func TestAgentWorkbenchKeepsMultipleArtifactNodes(t *testing.T) {
	previewA := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(31),
		Source:    "agent",
		NodeType:  db.NodeTypeImage,
		Title:     "preview A",
		Status:    db.NodeStatusSucceeded,
		Metadata:  []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true},
	}
	previewB := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(32),
		Source:    "agent",
		NodeType:  db.NodeTypeImage,
		Title:     "preview B",
		Status:    db.NodeStatusRunning,
		Metadata:  []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(200, 0), Valid: true},
	}
	video := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(33),
		Source:    "agent",
		NodeType:  db.NodeTypeVideo,
		Title:     "video",
		Status:    db.NodeStatusQueued,
		Metadata:  []byte(`{"agent_artifact_kind":"shot_video"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(300, 0), Valid: true},
	}

	got := agentArtifactNodes([]db.MediaNode{video, previewB, previewA})
	if len(got) != 3 {
		t.Fatalf("artifact node count = %d, want 3", len(got))
	}
	if got[0].ID != previewB.ID || got[1].ID != previewA.ID || got[2].ID != video.ID {
		t.Fatalf("artifact order = %#v", got)
	}
}

func TestAgentWorkbenchMissingArtifactSlot(t *testing.T) {
	slot := agentWorkbenchArtifactSlotResponse{Kind: "shot_video", Status: "missing"}
	if slot.Kind != "shot_video" || slot.Status != "missing" || slot.NodeID != "" {
		t.Fatalf("slot = %#v", slot)
	}
}

func TestAgentWorkbenchArtifactSlotIncludesAssetDimensions(t *testing.T) {
	assetID := uuidWithByteForWorkbenchTest(41)
	versionID := uuidWithByteForWorkbenchTest(42)
	node := db.MediaNode{
		ID:               uuidWithByteForWorkbenchTest(43),
		Source:           "agent",
		NodeType:         db.NodeTypeImage,
		Title:            "vertical preview",
		Status:           db.NodeStatusSucceeded,
		CurrentVersionID: versionID,
		Metadata:         []byte(`{"agent_artifact_kind":"preview_image"}`),
	}
	version := db.ArtifactVersion{
		ID:      versionID,
		AssetID: assetID,
	}
	asset := db.MediaAsset{
		ID:          assetID,
		Type:        db.AssetTypeImage,
		Mime:        "image/png",
		Metadata:    []byte(`{"width":900,"height":1600}`),
		StorageUrl:  pgtype.Text{String: "s3://bucket/vertical.png", Valid: true},
		WorkspaceID: uuidWithByteForWorkbenchTest(44),
	}

	slot, err := agentWorkbenchArtifactSlotFromNode(
		context.Background(),
		nil,
		node,
		map[pgtype.UUID]db.MediaAsset{assetID: asset},
		map[pgtype.UUID]db.ArtifactVersion{versionID: version},
	)
	if err != nil {
		t.Fatalf("agentWorkbenchArtifactSlotFromNode error = %v", err)
	}
	if slot.Width != 900 || slot.Height != 1600 {
		t.Fatalf("slot dimensions = %dx%d, want 900x1600", slot.Width, slot.Height)
	}
}

func uuidWithByteForWorkbenchTest(value byte) pgtype.UUID {
	return pgtype.UUID{
		Bytes: [16]byte{value, value, value, value, value, value, value, value, value, value, value, value, value, value, value, value},
		Valid: true,
	}
}
