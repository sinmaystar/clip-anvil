package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestGenerateShotVideoQueuesShotVideoCraftsmanTask(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", Title: "开场", Status: "preview_ready"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	enqueuer := &fakeCraftsmanEnqueuer{}
	tool := NewGenerateShotVideoTool(store, runtime, enqueuer)

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments: map[string]any{
			"shot_refs":    []any{"shot-01"},
			"max_attempts": float64(2),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.createdTasks) != 1 || len(enqueuer.tasks) != 1 {
		t.Fatalf("created=%d enqueued=%d", len(runtime.createdTasks), len(enqueuer.tasks))
	}
	var input map[string]any
	if err := json.Unmarshal(runtime.createdTasks[0].Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["mode"] != "shot_video" {
		t.Fatalf("task input = %#v", input)
	}
	refs, _ := input["input_node_refs"].([]any)
	if len(refs) != 1 || refs[0] != "shot-01 preview image" {
		t.Fatalf("input_node_refs = %#v", input["input_node_refs"])
	}
	if runtime.createdTasks[0].MaxAttempts != 2 {
		t.Fatalf("max attempts = %d", runtime.createdTasks[0].MaxAttempts)
	}
	if len(store.statusUpdates) != 0 {
		t.Fatalf("shot video dispatch should not reuse preview status updates: %#v", store.statusUpdates)
	}
	if !strings.Contains(out.Summary, "分镜视频") || !strings.Contains(out.Summary, "不表示视频已经生成完成") {
		t.Fatalf("summary = %q", out.Summary)
	}
}
