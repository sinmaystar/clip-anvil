package api

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestReferencePackReplaceRequestRejectsDuplicateMembers(t *testing.T) {
	nodeID := "11111111-1111-1111-1111-111111111111"
	req := replaceReferencePackItemsRequest{MemberNodeIDs: []string{nodeID, nodeID}}

	_, err := req.memberUUIDs()
	if err == nil {
		t.Fatal("duplicate member ids should be rejected")
	}
}

func TestCanAddReferencePackMemberRejectsNestedPack(t *testing.T) {
	pack := db.MediaNode{
		ID:          pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true},
		NodeType:    db.NodeTypeReferencePack,
		WorkspaceID: pgtype.UUID{Bytes: [16]byte{0x09}, Valid: true},
	}
	member := db.MediaNode{
		ID:          pgtype.UUID{Bytes: [16]byte{0x02}, Valid: true},
		NodeType:    db.NodeTypeReferencePack,
		WorkspaceID: pack.WorkspaceID,
	}

	if err := validateReferencePackMember(pack, member); err == nil {
		t.Fatal("reference pack nesting should be rejected")
	}
}

func TestToReferencePackItemResponsesUsesPositionOrder(t *testing.T) {
	item := db.ReferencePackItem{
		ID:           pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true},
		PackNodeID:   pgtype.UUID{Bytes: [16]byte{0x02}, Valid: true},
		MemberNodeID: pgtype.UUID{Bytes: [16]byte{0x03}, Valid: true},
		Position:     7,
	}

	resp := toReferencePackItemResponse(item)
	if resp.Position != 7 || resp.MemberNodeID == "" {
		t.Fatalf("response = %#v", resp)
	}
}
