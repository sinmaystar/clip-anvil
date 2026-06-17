package api

import (
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestDefaultNodeSizeForText(t *testing.T) {
	w, h := defaultNodeSize(db.MediaTypeText)
	if w != 200 {
		t.Fatalf("width = %v, want 200", w)
	}
	if h != 120 {
		t.Fatalf("height = %v, want 120", h)
	}
}

func TestDefaultNodeSizeForM1xNodeTypes(t *testing.T) {
	testCases := []struct {
		nodeType db.MediaType
		width    float32
		height   float32
	}{
		{nodeType: db.MediaTypeText, width: 200, height: 120},
		{nodeType: db.MediaTypeImage, width: 200, height: 160},
		{nodeType: db.MediaTypeVideo, width: 240, height: 180},
		{nodeType: db.MediaTypeAudio, width: 200, height: 80},
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
	for _, nodeType := range []db.MediaType{
		db.MediaTypeText,
		db.MediaTypeImage,
		db.MediaTypeVideo,
		db.MediaTypeAudio,
	} {
		t.Run(string(nodeType), func(t *testing.T) {
			if !isAllowedNodeType(nodeType) {
				t.Fatalf("%s should be allowed in M1.x", nodeType)
			}
		})
	}
}

func TestIsAllowedNodeTypeRejectsUnknownTypes(t *testing.T) {
	if isAllowedNodeType(db.MediaType("model")) {
		t.Fatal("unknown node type should not be allowed")
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
