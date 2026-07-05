package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
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
	if len(runtime.appendedMessages) != 1 {
		t.Fatalf("appended messages = %#v", runtime.appendedMessages)
	}
	message := runtime.appendedMessages[0]
	if message.ThreadID != uuidWithByte(44) || message.Role != "user" || message.MessageType != "text" {
		t.Fatalf("delegation message = %#v", message)
	}
	if !strings.Contains(string(message.RawMessage), `"target_role":"composer"`) ||
		!strings.Contains(string(message.RawMessage), `"schema":"clipanvil.agent.delegation.v1"`) {
		t.Fatalf("delegation raw message = %s", message.RawMessage)
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

func TestDispatchComposerNativeToolReusesActiveComposerTask(t *testing.T) {
	runtime := &fakeComposeRuntime{
		activeTasks: []db.AgentTask{{
			ID:          uuidWithByte(77),
			WorkspaceID: uuidWithByte(1),
			ThreadID:    uuidWithByte(44),
			Role:        "composer",
			ScopeType:   "final_output",
			ScopeID:     uuidWithByte(33),
			TaskType:    "composer_turn",
			Status:      "running",
			SemanticKey: "composer.final_output.source.composer_turn.active",
		}},
	}
	resolver := fakeComposerSourceResolver{
		object: db.AgentObjectIndex{
			WorkspaceID: uuidWithByte(1),
			ObjectType:  "media_node",
			ObjectID:    uuidWithByte(33),
			SemanticKey: "workspace.final_storyboard.v1",
		},
	}
	enqueuer := &fakeComposerEnqueuer{}
	tool := NewDispatchComposerNativeTool(runtime, enqueuer, resolver)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ToolCallID:  "call_composer",
	})

	out, err := tool.InvokableRun(ctx, `{
		"source_storyboard_ref":{"type":"media_node","key":"workspace.final_storyboard.v1"},
		"instructions":"重复派发同一个 final output",
		"template_key":"agent_remotion_code_v1"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.createdTasks) != 0 || len(runtime.appendedMessages) != 0 || len(enqueuer.tasks) != 0 {
		t.Fatalf("active composer task should be reused without new writes: created=%#v messages=%#v enqueued=%#v", runtime.createdTasks, runtime.appendedMessages, enqueuer.tasks)
	}
	if !strings.Contains(out, "agent_task/composer.final_output.source.composer_turn.active") {
		t.Fatalf("output should reference active task: %s", out)
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

type fakeComposeRuntime struct {
	createdTasks     []db.AgentTask
	appendedMessages []agentruntime.AppendMessageParams
	activeTasks      []db.AgentTask
}

func (f *fakeComposeRuntime) GetOrCreateComposerThread(_ context.Context, workspaceID pgtype.UUID) (db.AgentThread, error) {
	return db.AgentThread{ID: uuidWithByte(44), WorkspaceID: workspaceID, Role: "composer", ScopeType: "final_output"}, nil
}

func (f *fakeComposeRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	task := db.AgentTask{ID: uuidWithByte(byte(70 + len(f.createdTasks))), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, Role: params.Role, ScopeType: params.ScopeType, ScopeID: params.ScopeID, TaskType: params.TaskType, Status: "queued", MaxAttempts: params.MaxAttempts, Input: params.Input}
	f.createdTasks = append(f.createdTasks, task)
	return task, nil
}

func (f *fakeComposeRuntime) CreateEvent(context.Context, agentruntime.CreateEventParams) (db.AgentEvent, error) {
	return db.AgentEvent{}, nil
}

func (f *fakeComposeRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	f.appendedMessages = append(f.appendedMessages, params)
	return db.AgentMessage{
		ID:          uuidWithByte(byte(80 + len(f.appendedMessages))),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		MessageType: params.MessageType,
		Content:     params.Content,
		RawMessage:  params.RawMessage,
		TaskID:      params.TaskID,
	}, nil
}

func (f *fakeComposeRuntime) ListActiveAgentTasksByWorkspace(_ context.Context, workspaceID pgtype.UUID) ([]db.AgentTask, error) {
	out := []db.AgentTask{}
	for _, task := range f.activeTasks {
		if task.WorkspaceID == workspaceID {
			out = append(out, task)
		}
	}
	return out, nil
}

type fakeComposerEnqueuer struct {
	tasks []db.AgentTask
}

func (f *fakeComposerEnqueuer) EnqueueComposerTask(_ context.Context, task db.AgentTask) {
	f.tasks = append(f.tasks, task)
}
