package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestDispatchComposerNativeToolCreatesComposerTask(t *testing.T) {
	runtime := &fakeComposeRuntime{}
	enqueuer := &fakeComposerEnqueuer{}
	tool := NewDispatchComposerNativeTool(runtime, enqueuer)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ToolCallID:  "call_composer",
	})

	out, err := tool.InvokableRun(ctx, `{
		"source_storyboard_node_id":"21000000-0000-0000-0000-000000000000",
		"instructions":"把已完成分镜拼成 20 秒营销视频，使用淡入淡出。",
		"template_key":"concat_with_fades"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Composer 派发结果") {
		t.Fatalf("unexpected output: %s", out)
	}
	if len(runtime.createdTasks) != 1 || len(enqueuer.tasks) != 1 {
		t.Fatalf("created=%d enqueued=%d", len(runtime.createdTasks), len(enqueuer.tasks))
	}
	task := runtime.createdTasks[0]
	if task.Role != "composer" || task.TaskType != "composer_turn" || task.ScopeType != "final_output" {
		t.Fatalf("task = %#v", task)
	}
	var input map[string]any
	if err := json.Unmarshal(task.Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["source_storyboard_node_id"] != "21000000-0000-0000-0000-000000000000" ||
		input["template_key"] != "concat_with_fades" ||
		input["producer_thread_id"] != "02000000-0000-0000-0000-000000000000" {
		t.Fatalf("task input = %#v", input)
	}
}

func TestDispatchComposerNativeToolAcceptsSemanticSourceRef(t *testing.T) {
	runtime := &fakeComposeRuntime{}
	resolver := fakeComposerSourceResolver{
		object: db.AgentObjectIndex{
			WorkspaceID: uuidWithByte(1),
			ObjectType:  "media_node",
			ObjectID:    uuidWithByte(33),
			SemanticKey: "workspace.final_storyboard.v1",
		},
	}
	tool := NewDispatchComposerNativeTool(runtime, nil, resolver)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ToolCallID:  "call_composer",
	})

	out, err := tool.InvokableRun(ctx, `{
		"source_storyboard_ref":{"type":"media_node","key":"workspace.final_storyboard.v1"},
		"instructions":"把已完成分镜拼成 20 秒营销视频，使用淡入淡出。",
		"template_key":"concat_with_fades"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "source_ref：media_node/workspace.final_storyboard.v1") {
		t.Fatalf("unexpected output: %s", out)
	}
	if len(runtime.createdTasks) != 1 || runtime.createdTasks[0].ScopeID != uuidWithByte(33) {
		t.Fatalf("task = %#v", runtime.createdTasks)
	}
	var input map[string]any
	if err := json.Unmarshal(runtime.createdTasks[0].Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["source_storyboard_node_id"] != "21000000-0000-0000-0000-000000000000" {
		t.Fatalf("task input = %#v", input)
	}
}

type fakeComposerSourceResolver struct {
	object db.AgentObjectIndex
}

func (f fakeComposerSourceResolver) GetAgentObjectBySemanticKey(_ context.Context, params db.GetAgentObjectBySemanticKeyParams) (db.AgentObjectIndex, error) {
	if params.ObjectType != f.object.ObjectType || params.SemanticKey != f.object.SemanticKey {
		return db.AgentObjectIndex{}, pgx.ErrNoRows
	}
	return f.object, nil
}
