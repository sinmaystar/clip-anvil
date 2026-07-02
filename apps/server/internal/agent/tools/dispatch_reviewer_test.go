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
	var taskInput map[string]any
	if err := json.Unmarshal(runtime.createdTask.Input, &taskInput); err != nil {
		t.Fatal(err)
	}
	if taskInput["producer_thread_id"] != "02000000-0000-0000-0000-000000000000" ||
		taskInput["producer_task_id"] != "03000000-0000-0000-0000-000000000000" {
		t.Fatalf("reviewer task missing producer linkage: %#v", taskInput)
	}
	if len(enqueuer.tasks) != 1 {
		t.Fatalf("enqueued tasks = %d", len(enqueuer.tasks))
	}
	if len(runtime.appended) != 1 || runtime.appended[0].Role != "user" || runtime.appended[0].MessageType != "text" || !strings.Contains(string(runtime.appended[0].Content), "Producer 派发 Reviewer 评审任务") {
		t.Fatalf("delegation message = %#v", runtime.appended)
	}
}

func TestDispatchReviewerAcceptsSemanticTargetRefs(t *testing.T) {
	workspaceID := uuidWithByte(1)
	shotID := uuidWithByte(4)
	nodeID := uuidWithByte(5)
	versionID := uuidWithByte(6)
	store := fakeDispatchReviewerStore{
		workspace: db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeAgent},
		node:      db.MediaNode{ID: nodeID, WorkspaceID: workspaceID, ShotID: shotID, NodeType: db.NodeTypeImage, SemanticKey: "shot_02.preview_image.r1.node"},
		version:   db.ArtifactVersion{ID: versionID, WorkspaceID: workspaceID, NodeID: nodeID, Status: db.JobStatusSucceeded, SemanticKey: "shot_02.preview_image.r1.output.v1"},
		objects: map[string]db.AgentObjectIndex{
			"shot/shot_02": {
				WorkspaceID: workspaceID,
				ObjectType:  "shot",
				ObjectID:    shotID,
				SemanticKey: "shot_02",
			},
			"media_node/shot_02.preview_image.r1.node": {
				WorkspaceID: workspaceID,
				ObjectType:  "media_node",
				ObjectID:    nodeID,
				SemanticKey: "shot_02.preview_image.r1.node",
			},
			"artifact_version/shot_02.preview_image.r1.output.v1": {
				WorkspaceID: workspaceID,
				ObjectType:  "artifact_version",
				ObjectID:    versionID,
				SemanticKey: "shot_02.preview_image.r1.output.v1",
			},
		},
	}
	runtime := &fakeDispatchReviewerRuntime{}
	tool := NewDispatchReviewerNativeTool(store, runtime, nil)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: workspaceID, ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3), ToolCallID: "call_1"})
	out, err := tool.InvokableRun(ctx, `{
		"brief":"评审 shot_02 预览图",
		"review_task":"preview_image_review",
		"target":{
			"workspace_scope":"shot",
			"shot_id":"shot_02",
			"node_id":"shot_02.preview_image.r1.node",
			"artifact_version_id":"shot_02.preview_image.r1.output.v1"
		},
		"reason":"预览图已生成，需要检查商品一致性"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "scope_ref：shot/shot_02") || strings.Contains(out, "scope：shot=") {
		t.Fatalf("unexpected output: %s", out)
	}
	if strings.Contains(out, "reviewer_task_id") ||
		strings.Contains(out, uuidString(runtime.createdTask.ID)) {
		t.Fatalf("output leaked reviewer task id: %s", out)
	}
	var taskInput dispatchReviewerTaskInput
	if err := json.Unmarshal(runtime.createdTask.Input, &taskInput); err != nil {
		t.Fatal(err)
	}
	if taskInput.Target.ShotID != uuidString(shotID) ||
		taskInput.Target.ShotRef.Key != "shot_02" ||
		taskInput.Target.NodeRef.Key != "shot_02.preview_image.r1.node" ||
		taskInput.Target.ArtifactVersionRef.Key != "shot_02.preview_image.r1.output.v1" {
		t.Fatalf("task target = %#v", taskInput.Target)
	}
	if len(runtime.appended) != 1 || !strings.Contains(string(runtime.appended[0].Content), "scope_ref: shot/shot_02") {
		t.Fatalf("delegation message = %#v", runtime.appended)
	}
}

func TestDispatchReviewerRejectsDuplicateFinalVideoReview(t *testing.T) {
	workspaceID := uuidWithByte(1)
	nodeID := uuidWithByte(5)
	versionID := uuidWithByte(6)
	store := fakeDispatchReviewerStore{
		workspace: db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeAgent},
		node:      db.MediaNode{ID: nodeID, WorkspaceID: workspaceID, NodeType: db.NodeTypeVideo, SemanticKey: "final_video.abc.node", ArtifactKind: "final_video"},
		version:   db.ArtifactVersion{ID: versionID, WorkspaceID: workspaceID, NodeID: nodeID, Status: db.JobStatusSucceeded, SemanticKey: "final_video.abc.compose.artifact.v1", ArtifactKind: "final_video"},
		objects: map[string]db.AgentObjectIndex{
			"media_node/final_video.abc.node": {
				WorkspaceID: workspaceID,
				ObjectType:  "media_node",
				ObjectID:    nodeID,
				SemanticKey: "final_video.abc.node",
			},
			"artifact_version/final_video.abc.compose.artifact.v1": {
				WorkspaceID: workspaceID,
				ObjectType:  "artifact_version",
				ObjectID:    versionID,
				SemanticKey: "final_video.abc.compose.artifact.v1",
			},
		},
		reviewsByVersion: []db.ReviewRecord{
			{
				ID:                uuidWithByte(12),
				WorkspaceID:       workspaceID,
				NodeID:            nodeID,
				ArtifactVersionID: versionID,
				ReviewTask:        reviewTaskFinalVideo,
				TargetPhase:       "final_video",
				Status:            reviewVerdictAcceptedWithWarnings,
				SemanticKey:       "final_video.abc.compose.review.v1",
			},
		},
	}
	runtime := &fakeDispatchReviewerRuntime{}
	enqueuer := &fakeReviewerTaskEnqueuer{}
	tool := NewDispatchReviewerNativeTool(store, runtime, enqueuer)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: workspaceID, ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3), ToolCallID: "call_1"})

	out, err := tool.InvokableRun(ctx, `{
		"brief":"复核最终成片",
		"review_task":"final_video_review",
		"target":{
			"workspace_scope":"final_video",
			"node_id":"final_video.abc.node",
			"artifact_version_id":"final_video.abc.compose.artifact.v1"
		},
		"reason":"成片已经有评审结果后不应重复派发"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "工具调用失败") ||
		!strings.Contains(out, "已有终态评审") ||
		!strings.Contains(out, "review_record/final_video.abc.compose.review.v1") {
		t.Fatalf("unexpected output: %s", out)
	}
	if runtime.createdTask.ID.Valid {
		t.Fatalf("unexpected task created: %#v", runtime.createdTask)
	}
	if len(enqueuer.tasks) != 0 {
		t.Fatalf("enqueued tasks = %d", len(enqueuer.tasks))
	}
}

func TestDispatchReviewerRejectsDuplicatePreRenderPlanReview(t *testing.T) {
	workspaceID := uuidWithByte(1)
	renderPlanID := uuidWithByte(7)
	store := fakeDispatchReviewerStore{
		workspace: db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeAgent},
		plan: db.RenderPlan{
			ID:            renderPlanID,
			WorkspaceID:   workspaceID,
			ScopeType:     "shot",
			ScopeID:       uuidWithByte(4),
			SemanticKey:   "shot_01.shot_video.r1",
			RenderPlanKey: "shot_01.shot_video.r1",
		},
		objects: map[string]db.AgentObjectIndex{
			"render_plan/shot_01.shot_video.r1": {
				WorkspaceID: workspaceID,
				ObjectType:  "render_plan",
				ObjectID:    renderPlanID,
				SemanticKey: "shot_01.shot_video.r1",
			},
		},
		reviewsByRenderPlan: []db.ReviewRecord{
			{
				ID:           uuidWithByte(12),
				WorkspaceID:  workspaceID,
				RenderPlanID: renderPlanID,
				ReviewTask:   reviewTaskPreRenderPlan,
				TargetPhase:  "pre_render_plan",
				Status:       reviewVerdictAcceptedWithWarnings,
				SemanticKey:  "shot_01.shot_video.r1.review.v1",
			},
		},
	}
	runtime := &fakeDispatchReviewerRuntime{}
	enqueuer := &fakeReviewerTaskEnqueuer{}
	tool := NewDispatchReviewerNativeTool(store, runtime, enqueuer)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: workspaceID, ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3), ToolCallID: "call_1"})

	out, err := tool.InvokableRun(ctx, `{
		"brief":"复核 RenderPlan",
		"review_task":"pre_render_plan_review",
		"target":{"workspace_scope":"render_plan","render_plan_ref":{"type":"render_plan","key":"shot_01.shot_video.r1"}},
		"reason":"RenderPlan 已有终态预评审后不应重复派发"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "工具调用失败") ||
		!strings.Contains(out, "已有终态评审") ||
		!strings.Contains(out, "review_record/shot_01.shot_video.r1.review.v1") {
		t.Fatalf("unexpected output: %s", out)
	}
	if runtime.createdTask.ID.Valid {
		t.Fatalf("unexpected task created: %#v", runtime.createdTask)
	}
	if len(enqueuer.tasks) != 0 {
		t.Fatalf("enqueued tasks = %d", len(enqueuer.tasks))
	}
}

func TestDispatchReviewerRejectsActiveReviewerTaskForSameScope(t *testing.T) {
	workspaceID := uuidWithByte(1)
	renderPlanID := uuidWithByte(7)
	store := fakeDispatchReviewerStore{
		workspace: db.Workspace{ID: workspaceID, Mode: db.WorkspaceModeAgent},
		plan: db.RenderPlan{
			ID:            renderPlanID,
			WorkspaceID:   workspaceID,
			ScopeType:     "shot",
			ScopeID:       uuidWithByte(4),
			SemanticKey:   "shot_01.shot_video.r2",
			RenderPlanKey: "shot_01.shot_video.r2",
		},
		objects: map[string]db.AgentObjectIndex{
			"render_plan/shot_01.shot_video.r2": {
				WorkspaceID: workspaceID,
				ObjectType:  "render_plan",
				ObjectID:    renderPlanID,
				SemanticKey: "shot_01.shot_video.r2",
			},
		},
	}
	runtime := &fakeDispatchReviewerRuntime{
		activeTasks: []db.AgentTask{
			{
				ID:          uuidWithByte(10),
				WorkspaceID: workspaceID,
				Role:        "reviewer",
				ScopeType:   "render_plan",
				ScopeID:     renderPlanID,
				TaskType:    "reviewer_turn",
				Status:      "running",
				SemanticKey: "reviewer.active.pre_render",
			},
		},
	}
	enqueuer := &fakeReviewerTaskEnqueuer{}
	tool := NewDispatchReviewerNativeTool(store, runtime, enqueuer)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: workspaceID, ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3), ToolCallID: "call_1"})

	out, err := tool.InvokableRun(ctx, `{
		"brief":"复核 RenderPlan",
		"review_task":"pre_render_plan_review",
		"target":{"workspace_scope":"render_plan","render_plan_ref":{"type":"render_plan","key":"shot_01.shot_video.r2"}},
		"reason":"已有相同 scope 的 Reviewer 正在运行，不应重复派发"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "工具调用失败") ||
		!strings.Contains(out, "active_reviewer_task_exists") ||
		!strings.Contains(out, "agent_task/reviewer.active.pre_render") {
		t.Fatalf("unexpected output: %s", out)
	}
	if runtime.createdTask.ID.Valid {
		t.Fatalf("unexpected task created: %#v", runtime.createdTask)
	}
	if len(enqueuer.tasks) != 0 {
		t.Fatalf("enqueued tasks = %d", len(enqueuer.tasks))
	}
}

type fakeDispatchReviewerStore struct {
	workspace           db.Workspace
	node                db.MediaNode
	version             db.ArtifactVersion
	plan                db.RenderPlan
	objects             map[string]db.AgentObjectIndex
	reviewsByVersion    []db.ReviewRecord
	reviewsByRenderPlan []db.ReviewRecord
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

func (f fakeDispatchReviewerStore) ListReviewRecordsByArtifactVersion(context.Context, pgtype.UUID) ([]db.ReviewRecord, error) {
	return f.reviewsByVersion, nil
}

func (f fakeDispatchReviewerStore) ListReviewRecordsByRenderPlan(context.Context, pgtype.UUID) ([]db.ReviewRecord, error) {
	return f.reviewsByRenderPlan, nil
}

func (f fakeDispatchReviewerStore) GetAgentObjectBySemanticKey(_ context.Context, params db.GetAgentObjectBySemanticKeyParams) (db.AgentObjectIndex, error) {
	if f.objects == nil {
		return db.AgentObjectIndex{}, pgx.ErrNoRows
	}
	object, ok := f.objects[params.ObjectType+"/"+params.SemanticKey]
	if !ok {
		return db.AgentObjectIndex{}, pgx.ErrNoRows
	}
	return object, nil
}

type fakeDispatchReviewerRuntime struct {
	createdTask db.AgentTask
	appendSeq   int64
	appended    []db.AgentMessage
	activeTasks []db.AgentTask
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

func (f *fakeDispatchReviewerRuntime) ListActiveAgentTasksByWorkspace(context.Context, pgtype.UUID) ([]db.AgentTask, error) {
	return f.activeTasks, nil
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
