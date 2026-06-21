package api

import (
	"encoding/json"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestDefaultNodeSizeForText(t *testing.T) {
	w, h := defaultNodeSize(db.NodeTypeText)
	if w != 360 {
		t.Fatalf("width = %v, want 360", w)
	}
	if h != 300 {
		t.Fatalf("height = %v, want 300", h)
	}
}

func TestDefaultNodeSizeForM1xNodeTypes(t *testing.T) {
	testCases := []struct {
		nodeType db.NodeType
		width    float32
		height   float32
	}{
		{nodeType: db.NodeTypeText, width: 360, height: 300},
		{nodeType: db.NodeTypeImage, width: 380, height: 280},
		{nodeType: db.NodeTypeVideo, width: 420, height: 280},
		{nodeType: db.NodeTypeAudio, width: 320, height: 120},
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

func TestNodePatchRequestDetectsPromptRefsFields(t *testing.T) {
	promptRefs := json.RawMessage(`{"version":1,"refs":[]}`)
	promptRich := json.RawMessage(`{"version":1,"source":"textarea-at","text":"hello"}`)
	req := updateNodeRequest{PromptRefs: &promptRefs, PromptRich: &promptRich}

	if !req.hasChanges() {
		t.Fatal("expected request with prompt refs to have changes")
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

func TestCreateNodeRequestAcceptsUploadSourceMaterial(t *testing.T) {
	req := createNodeRequest{
		WorkspaceID:   "018f6ef0-5f4f-7e86-9f8c-100000000001",
		NodeType:      "image",
		Title:         "商品主图",
		Status:        "succeeded",
		AssetID:       "018f6ef0-5f4f-7e86-9f8c-100000000002",
		OperationType: "upload",
		CanvasX:       120,
		CanvasY:       160,
	}

	status, ok := req.nodeStatus()
	if !ok || status != db.NodeStatusSucceeded {
		t.Fatalf("status = %q, %v, want succeeded", status, ok)
	}
	if !req.hasProductionConfig() {
		t.Fatal("upload source node should persist production config")
	}
}

func TestCreateNodeRequestAcceptsManualTextSourceMaterial(t *testing.T) {
	req := createNodeRequest{
		WorkspaceID:   "018f6ef0-5f4f-7e86-9f8c-100000000001",
		NodeType:      "text",
		Title:         "视频脚本",
		Prompt:        "第一幕：机场大厅。",
		Status:        "succeeded",
		OperationType: "manual",
	}

	status, ok := req.nodeStatus()
	if !ok || status != db.NodeStatusSucceeded {
		t.Fatalf("status = %q, %v, want succeeded", status, ok)
	}
	if !req.hasProductionConfig() {
		t.Fatal("manual text source node should persist production config")
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
