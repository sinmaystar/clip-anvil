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

func TestIsAllowedM1NodeTypeOnlyAcceptsText(t *testing.T) {
	if !isAllowedM1NodeType(db.MediaTypeText) {
		t.Fatal("text should be allowed in M1")
	}
	if isAllowedM1NodeType(db.MediaTypeVideo) {
		t.Fatal("video should not be allowed in M1")
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
