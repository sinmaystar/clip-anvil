package tools

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestReadWorkspaceContextDoesNotReadMessageHistory(t *testing.T) {
	store := &fakeContextStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Name: "Agent Space", Mode: db.WorkspaceModeAgent},
		nodes: []db.MediaNode{
			{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeImage, Title: "商品主图", Source: "agent", AssetID: uuidWithByte(3)},
		},
	}
	tool := NewReadWorkspaceContextTool(store)

	out, err := tool.Execute(context.Background(), ExecuteInput{WorkspaceID: uuidWithByte(1)})
	if err != nil {
		t.Fatal(err)
	}

	if store.messageHistoryReads != 0 {
		t.Fatal("read_workspace_context must not read message history")
	}
	refs, ok := out.Result["source_material_refs"].([]SourceMaterialRef)
	if !ok || len(refs) != 1 {
		t.Fatalf("source material refs = %#v", out.Result["source_material_refs"])
	}
}

func TestReadWorkspaceContextReturnsSourceMaterialRefs(t *testing.T) {
	store := &fakeContextStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Name: "Agent Space", Mode: db.WorkspaceModeAgent},
		nodes: []db.MediaNode{
			{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeImage, Title: "商品主图", Source: "agent", AssetID: uuidWithByte(3)},
			{ID: uuidWithByte(4), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeText, Title: "用户手写草稿", Source: "user", AssetID: uuidWithByte(5)},
		},
	}
	tool := NewReadWorkspaceContextTool(store)

	out, err := tool.Execute(context.Background(), ExecuteInput{WorkspaceID: uuidWithByte(1)})
	if err != nil {
		t.Fatal(err)
	}

	refs := out.Result["source_material_refs"].([]SourceMaterialRef)
	if len(refs) != 1 {
		t.Fatalf("refs = %#v, want only agent source refs", refs)
	}
	if refs[0].Title != "商品主图" || refs[0].Type != "image" {
		t.Fatalf("ref = %#v", refs[0])
	}
}

type fakeContextStore struct {
	workspace           db.Workspace
	nodes               []db.MediaNode
	messageHistoryReads int
}

func (f *fakeContextStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeContextStore) ListMediaNodesByWorkspace(context.Context, pgtype.UUID) ([]db.MediaNode, error) {
	return f.nodes, nil
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}
