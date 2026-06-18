package api

import (
	"encoding/json"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestDefaultNodeSizeForText(t *testing.T) {
	w, h := defaultNodeSize(db.NodeTypeText)
	if w != 200 {
		t.Fatalf("width = %v, want 200", w)
	}
	if h != 120 {
		t.Fatalf("height = %v, want 120", h)
	}
}

func TestDefaultNodeSizeForM1xNodeTypes(t *testing.T) {
	testCases := []struct {
		nodeType db.NodeType
		width    float32
		height   float32
	}{
		{nodeType: db.NodeTypeText, width: 200, height: 120},
		{nodeType: db.NodeTypeImage, width: 200, height: 160},
		{nodeType: db.NodeTypeVideo, width: 240, height: 180},
		{nodeType: db.NodeTypeAudio, width: 200, height: 80},
	}

	for _, tc := range testCases {
		t.Run(string(tc.nodeType), func(t *testing.T) {
			w, h := defaultNodeSize(tc.nodeType)
			if w != tc.width {
				t.Fatalf("width = %v, want %v", w, tc.width)
			}
			if h != tc.height {
				t.Fatalf("height = %v, want %v", h, tc.height)
			}
		})
	}
}

func TestIsAllowedNodeTypeAcceptsM1xTypes(t *testing.T) {
	for _, nodeType := range []db.NodeType{
		db.NodeTypeText,
		db.NodeTypeImage,
		db.NodeTypeVideo,
		db.NodeTypeAudio,
	} {
		t.Run(string(nodeType), func(t *testing.T) {
			if !isAllowedNodeType(nodeType) {
				t.Fatalf("%s should be allowed in M1.x", nodeType)
			}
		})
	}
}

func TestIsAllowedNodeTypeRejectsUnknownTypes(t *testing.T) {
	if isAllowedNodeType(db.NodeType("model")) {
		t.Fatal("unknown node type should not be allowed")
	}
}

func TestIsStudioWorkspaceMode(t *testing.T) {
	if !isStudioWorkspaceMode(db.WorkspaceModeStudio) {
		t.Fatal("studio mode should allow ordinary canvas edits")
	}
	if isStudioWorkspaceMode(db.WorkspaceModeAgent) {
		t.Fatal("agent mode should block ordinary canvas edits")
	}
}

func TestNodePatchRequestDetectsProvidedFields(t *testing.T) {
	title := "新标题"
	req := updateNodeRequest{Title: &title}

	if !req.hasChanges() {
		t.Fatal("expected request with title to have changes")
	}
	if req.Prompt != nil {
		t.Fatal("prompt should remain omitted")
	}
}

func TestNodePatchRequestDetectsGroupIDField(t *testing.T) {
	groupID := ""
	req := updateNodeRequest{GroupID: &groupID}

	if !req.hasChanges() {
		t.Fatal("expected request with group_id to have changes")
	}
}

func TestNodePatchRequestRejectsNoFields(t *testing.T) {
	req := updateNodeRequest{}
	if req.hasChanges() {
		t.Fatal("empty update request should not have changes")
	}
}

func TestCreateNodeRequestAcceptsRestoreFields(t *testing.T) {
	req := createNodeRequest{
		ID:     "018f6ef0-5f4f-7e86-9f8c-100000000001",
		Prompt: "保留撤销前的 prompt",
		Status: "draft",
	}

	nodeID, ok := req.nodeID()
	if !ok {
		t.Fatal("expected restore id to parse")
	}
	if nodeID.Valid != true {
		t.Fatal("expected parsed restore id to be valid")
	}

	status, ok := req.nodeStatus()
	if !ok {
		t.Fatal("expected restore status to parse")
	}
	if status != db.NodeStatusDraft {
		t.Fatalf("status = %q, want draft", status)
	}
}

func TestCreateNodeRequestModelParamsJSON(t *testing.T) {
	raw := json.RawMessage(`{"temperature":0.2}`)
	req := createNodeRequest{ModelParams: raw}
	got := req.modelParamsJSON()
	if string(got) != `{"temperature":0.2}` {
		t.Fatalf("model params = %s", got)
	}
}

func TestCreateNodeRequestModelParamsDefaultsToObject(t *testing.T) {
	req := createNodeRequest{}
	got := req.modelParamsJSON()
	if string(got) != `{}` {
		t.Fatalf("model params = %s", got)
	}
}
