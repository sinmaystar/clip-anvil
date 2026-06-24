package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type fakeDependencyEdgeLister map[pgtype.UUID][]db.MediaEdge

func (f fakeDependencyEdgeLister) ListOutgoingDependencyEdges(_ context.Context, fromNodeID pgtype.UUID) ([]db.MediaEdge, error) {
	return f[fromNodeID], nil
}

func TestWouldCreateCycleDetectsReachableSource(t *testing.T) {
	nodeA := testUUID(0x01)
	nodeB := testUUID(0x02)
	nodeC := testUUID(0x03)
	lister := fakeDependencyEdgeLister{
		nodeB: {
			{FromNodeID: nodeB, ToNodeID: nodeC},
		},
		nodeC: {
			{FromNodeID: nodeC, ToNodeID: nodeA},
		},
	}

	hasCycle, err := wouldCreateCycle(context.Background(), lister, nodeA, nodeB)
	if err != nil {
		t.Fatalf("wouldCreateCycle returned error: %v", err)
	}
	if !hasCycle {
		t.Fatal("expected B -> C -> A path to make A -> B a cycle")
	}
}

func TestWouldCreateCycleAllowsAcyclicEdge(t *testing.T) {
	nodeA := testUUID(0x01)
	nodeB := testUUID(0x02)
	nodeC := testUUID(0x03)
	lister := fakeDependencyEdgeLister{
		nodeB: {
			{FromNodeID: nodeB, ToNodeID: nodeC},
		},
	}

	hasCycle, err := wouldCreateCycle(context.Background(), lister, nodeA, nodeB)
	if err != nil {
		t.Fatalf("wouldCreateCycle returned error: %v", err)
	}
	if hasCycle {
		t.Fatal("did not expect B -> C path to make A -> B a cycle")
	}
}

func TestReferencePackContainsMemberDetectsDirectMember(t *testing.T) {
	packID := testUUID(0x04)
	memberID := testUUID(0x05)
	items := []db.ReferencePackItem{
		{PackNodeID: packID, MemberNodeID: memberID},
	}

	if !referencePackContainsMember(items, memberID) {
		t.Fatal("expected pack member dependency to be detected")
	}
	if referencePackContainsMember(items, testUUID(0x06)) {
		t.Fatal("unexpected non-member dependency detection")
	}
}

func TestNormalizedEdgeMetadataDefaultsToObject(t *testing.T) {
	metadata, ok := normalizedEdgeMetadata(nil)
	if !ok {
		t.Fatal("expected empty metadata to be accepted")
	}
	if string(metadata) != "{}" {
		t.Fatalf("metadata = %s, want {}", metadata)
	}
}

func TestNormalizedEdgeMetadataAcceptsObject(t *testing.T) {
	metadata, ok := normalizedEdgeMetadata(json.RawMessage(`{"anchors":{"target":{"x":0,"y":0.25}}}`))
	if !ok {
		t.Fatal("expected object metadata to be accepted")
	}
	if !json.Valid(metadata) {
		t.Fatalf("metadata is invalid JSON: %s", metadata)
	}
}

func TestNormalizedEdgeMetadataRejectsNonObject(t *testing.T) {
	if _, ok := normalizedEdgeMetadata(json.RawMessage(`[{"x":0}]`)); ok {
		t.Fatal("expected array metadata to be rejected")
	}
	if _, ok := normalizedEdgeMetadata(json.RawMessage(`null`)); ok {
		t.Fatal("expected null metadata to be rejected")
	}
}

func testUUID(value byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{value}, Valid: true}
}
