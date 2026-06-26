package api

import (
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

func uuidWithByteForWorkbenchTest(value byte) pgtype.UUID {
	return pgtype.UUID{
		Bytes: [16]byte{value, value, value, value, value, value, value, value, value, value, value, value, value, value, value, value},
		Valid: true,
	}
}
