package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestCreateAgentTextNodeRejectsStudioWorkspace(t *testing.T) {
	store := &fakeTextNodeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeStudio},
	}
	tool := NewCreateAgentTextNodeTool(store, nil)

	_, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		Arguments:   map[string]any{"title": "brief", "text": "hello"},
	})
	if !errors.Is(err, ErrAgentWorkspaceRequired) {
		t.Fatalf("error = %v, want ErrAgentWorkspaceRequired", err)
	}
}

func TestCreateAgentTextNodeCreatesAgentSourceMaterial(t *testing.T) {
	store := &fakeTextNodeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		asset:     db.MediaAsset{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), Type: db.AssetTypeText},
		node:      db.MediaNode{ID: uuidWithByte(3), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeText, Title: "brief", Source: "agent", AssetID: uuidWithByte(2)},
	}
	tool := NewCreateAgentTextNodeTool(store, nil)

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		Arguments:   map[string]any{"title": "brief", "text": "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.createdText != "hello" {
		t.Fatalf("created text = %q", store.createdText)
	}
	if store.createNodeParams.NodeType != db.NodeTypeText || store.createNodeParams.Title != "brief" {
		t.Fatalf("create node params = %#v", store.createNodeParams)
	}
	if out.Result["node_id"] == "" || out.Result["asset_id"] == "" {
		t.Fatalf("result = %#v", out.Result)
	}
}

func TestCreateAgentTextNodeBroadcastsCanvasNode(t *testing.T) {
	store := &fakeTextNodeStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		asset:     db.MediaAsset{ID: uuidWithByte(2), WorkspaceID: uuidWithByte(1), Type: db.AssetTypeText},
		node:      db.MediaNode{ID: uuidWithByte(3), WorkspaceID: uuidWithByte(1), NodeType: db.NodeTypeText, Source: "agent", AssetID: uuidWithByte(2)},
	}
	broadcaster := &fakeNodeBroadcaster{}
	tool := NewCreateAgentTextNodeTool(store, broadcaster)

	_, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		Arguments:   map[string]any{"title": "brief", "text": "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if broadcaster.count != 1 {
		t.Fatalf("broadcast count = %d, want 1", broadcaster.count)
	}
}

type fakeTextNodeStore struct {
	workspace        db.Workspace
	asset            db.MediaAsset
	node             db.MediaNode
	createdText      string
	createNodeParams db.CreateAgentMediaNodeParams
}

func (f *fakeTextNodeStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeTextNodeStore) CreateTextMediaAsset(_ context.Context, params db.CreateTextMediaAssetParams) (db.MediaAsset, error) {
	f.createdText = params.TextContent.String
	return f.asset, nil
}

func (f *fakeTextNodeStore) CreateAgentMediaNode(_ context.Context, params db.CreateAgentMediaNodeParams) (db.MediaNode, error) {
	f.createNodeParams = params
	return f.node, nil
}

type fakeNodeBroadcaster struct {
	count int
}

func (f *fakeNodeBroadcaster) BroadcastAgentNodeCreated(pgtype.UUID, db.MediaNode) {
	f.count++
}
