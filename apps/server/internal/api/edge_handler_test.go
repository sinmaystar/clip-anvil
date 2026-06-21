package api

import (
	"context"
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

func testUUID(value byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{value}, Valid: true}
}
