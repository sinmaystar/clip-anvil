package api

import (
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestStudioWorkspaceModeOnlyAllowsStudio(t *testing.T) {
	if !isStudioWorkspaceMode(db.WorkspaceModeStudio) {
		t.Fatal("studio mode should be writable by Studio APIs")
	}
	if isStudioWorkspaceMode(db.WorkspaceModeAgent) {
		t.Fatal("agent mode must remain blocked from Studio-only APIs")
	}
}

func TestCanvasLayoutWorkspaceModeAllowsStudioAndAgent(t *testing.T) {
	if !isCanvasLayoutWorkspaceMode(db.WorkspaceModeStudio) {
		t.Fatal("studio mode should allow canvas layout writes")
	}
	if !isCanvasLayoutWorkspaceMode(db.WorkspaceModeAgent) {
		t.Fatal("agent mode should allow canvas layout writes")
	}
}
