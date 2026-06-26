package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestDispatchReviewerNativeToolRequiresMatchingTarget(t *testing.T) {
	tool := NewDispatchReviewerNativeTool(fakeDispatchReviewerStore{}, &fakeDispatchReviewerRuntime{}, nil)
	out, err := tool.InvokableRun(context.Background(), `{
		"brief":"评审 shot video",
		"review_task":"shot_video_review",
		"target":{"workspace_scope":"shot"},
		"reason":"视频生成完成，需要评审"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "工具调用失败") || !strings.Contains(out, "target.") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestDispatchReviewerCreatesReviewerTask(t *testing.T) {
	workspaceID := uuidWithByte(1)
	shotID := uuidWithByte(4)
	nodeID := uuidWithByte(5)
	versionID := uuidWithByte(6)
	store := fakeDispatchReviewerStore{
		workspace: db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeAgent},
		node:      db.MediaNode{ID: nodeID, WorkspaceID: workspaceID, ShotID: shotID, NodeType: db.NodeTypeVideo},
		version:   db.ArtifactVersion{ID: versionID, WorkspaceID: workspaceID, NodeID: nodeID, Status: db.JobStatusSucceeded},
	}
	runtime := &fakeDispatchReviewerRuntime{}
	enqueuer := &fakeReviewerTaskEnqueuer{}
	tool := NewDispatchReviewerNativeTool(store, runtime, enqueuer)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: workspaceID, ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3), ToolCallID: "call_1"})
	out, err := tool.InvokableRun(ctx, `{
		"brief":"评审 shot_01 视频",
		"review_task":"shot_video_review",
		"target":{"workspace_scope":"shot","shot_id":"04000000-0000-0000-0000-000000000000","node_id":"05000000-0000-0000-0000-000000000000","artifact_version_id":"06000000-0000-0000-0000-000000000000"},
		"reason":"视频已生成，需要检查商品一致性和运动物理"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "已派发 Reviewer") {
		t.Fatalf("unexpected output: %s", out)
	}
	if runtime.createdTask.TaskType != "reviewer_turn" {
		t.Fatalf("task type = %s", runtime.createdTask.TaskType)
	}
	if len(enqueuer.tasks) != 1 {
		t.Fatalf("enqueued tasks = %d", len(enqueuer.tasks))
	}
	if len(runtime.appended) != 1 || runtime.appended[0].Role != "user" || runtime.appended[0].MessageType != "text" || !strings.Contains(string(runtime.appended[0].Content), "Producer 派发 Reviewer 评审任务") {
		t.Fatalf("delegation message = %#v", runtime.appended)
	}
}

type fakeDispatchReviewerStore struct {
	workspace db.Workspace
	node      db.MediaNode
	version   db.ArtifactVersion
	plan      db.RenderPlan
}

func (f fakeDispatchReviewerStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return f.workspace, nil
}

func (f fakeDispatchReviewerStore) GetMediaNodeByID(context.Context, pgtype.UUID) (db.MediaNode, error) {
	return f.node, nil
}

func (f fakeDispatchReviewerStore) GetArtifactVersionByID(context.Context, pgtype.UUID) (db.ArtifactVersion, error) {
	return f.version, nil
}

func (f fakeDispatchReviewerStore) GetRenderPlanByID(context.Context, db.GetRenderPlanByIDParams) (db.RenderPlan, error) {
	return f.plan, nil
}

type fakeDispatchReviewerRuntime struct {
	createdTask db.AgentTask
	appendSeq   int64
	appended    []db.AgentMessage
}

func (f *fakeDispatchReviewerRuntime) GetOrCreateReviewerThreadForScope(_ context.Context, workspaceID pgtype.UUID, scopeType string, scopeID pgtype.UUID) (db.AgentThread, error) {
	return db.AgentThread{ID: uuidWithByte(9), WorkspaceID: workspaceID, Role: "reviewer", ScopeType: scopeType, ScopeID: scopeID}, nil
}

func (f *fakeDispatchReviewerRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	f.createdTask = db.AgentTask{
		ID:          uuidWithByte(10),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		ScopeType:   params.ScopeType,
		ScopeID:     params.ScopeID,
		TaskType:    params.TaskType,
		Input:       params.Input,
	}
	return f.createdTask, nil
}

func (f *fakeDispatchReviewerRuntime) CreateEvent(context.Context, agentruntime.CreateEventParams) (db.AgentEvent, error) {
	return db.AgentEvent{ID: uuidWithByte(11), EventType: "review_queued"}, nil
}

func (f *fakeDispatchReviewerRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	f.appendSeq++
	msg := db.AgentMessage{
		ID:          uuidWithByte(byte(20 + f.appendSeq)),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		MessageType: params.MessageType,
		Content:     params.Content,
		RawMessage:  params.RawMessage,
		TaskID:      params.TaskID,
		Seq:         f.appendSeq,
	}
	f.appended = append(f.appended, msg)
	return msg, nil
}

type fakeReviewerTaskEnqueuer struct {
	tasks []db.AgentTask
}

func (f *fakeReviewerTaskEnqueuer) EnqueueReviewerTask(_ context.Context, task db.AgentTask) {
	f.tasks = append(f.tasks, task)
}
