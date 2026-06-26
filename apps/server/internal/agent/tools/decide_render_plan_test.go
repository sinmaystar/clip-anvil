package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestDecideRenderPlanAcceptSubmitsWorkerTask(t *testing.T) {
	store := &fakeRenderPlanDecisionStore{
		plan: db.RenderPlan{
			ID:                 uuidWithByte(21),
			WorkspaceID:        uuidWithByte(1),
			ScopeType:          "shot",
			ScopeID:            uuidWithByte(11),
			TargetPhase:        "preview_image",
			ModelPromptProfile: "seedream_5_image",
			Operation:          "text_to_image",
			Status:             "waiting_for_approval",
			CompiledPrompt:     "生成机场中的银色行李箱预览图。",
			Params:             []byte(`{"ratio":"9:16","max_images":1}`),
			Rationale:          "先确认第一轮视觉方向。",
			CreatedByThreadID:  uuidWithByte(12),
			CreatedByTaskID:    uuidWithByte(13),
		},
		shot: db.Shot{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01", SortOrder: 1},
	}
	runtime := &fakeRenderPlanDecisionRuntime{}
	enqueuer := &fakeWorkerTaskEnqueuer{}
	tool := NewDecideRenderPlanNativeTool(store, runtime, enqueuer)

	args := fmt.Sprintf(`{
		"brief":"接受 RenderPlan 并提交 Worker。",
		"render_plan_id":%q,
		"decision":"accept",
		"reason":"用户已确认出图",
		"next_action":"submit_worker"
	}`, uuidString(store.plan.ID))
	got, err := tool.InvokableRun(contextWithNativeRuntime(uuidWithByte(1), uuidWithByte(2), uuidWithByte(3)), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "已接受 RenderPlan") || !strings.Contains(got, "worker_generation") {
		t.Fatalf("result = %s", got)
	}
	if len(runtime.createdTasks) != 1 || len(enqueuer.tasks) != 1 {
		t.Fatalf("created tasks = %d, enqueued = %d", len(runtime.createdTasks), len(enqueuer.tasks))
	}
	task := runtime.createdTasks[0]
	if task.Role != "worker" || task.TaskType != "worker_generation" || task.RenderPlanID != store.plan.ID {
		t.Fatalf("task = %#v", task)
	}
	var input map[string]any
	if err := json.Unmarshal(task.Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["mode"] != "preview_image" || input["prompt"] != store.plan.CompiledPrompt {
		t.Fatalf("worker input = %#v", input)
	}
	if !store.submitted {
		t.Fatal("render plan was not marked submitted")
	}
	if len(runtime.processedRenderPlans) != 1 || runtime.processedRenderPlans[0] != store.plan.ID {
		t.Fatalf("processed render plans = %#v", runtime.processedRenderPlans)
	}
}

func TestDecideRenderPlanRejectDoesNotSubmitWorkerTask(t *testing.T) {
	store := &fakeRenderPlanDecisionStore{
		plan: db.RenderPlan{ID: uuidWithByte(21), WorkspaceID: uuidWithByte(1), ScopeType: "shot", ScopeID: uuidWithByte(11), TargetPhase: "preview_image", Status: "waiting_for_approval"},
		shot: db.Shot{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01"},
	}
	runtime := &fakeRenderPlanDecisionRuntime{}
	enqueuer := &fakeWorkerTaskEnqueuer{}
	tool := NewDecideRenderPlanNativeTool(store, runtime, enqueuer)

	args := fmt.Sprintf(`{
		"brief":"拒绝 RenderPlan 并要求修订。",
		"render_plan_id":%q,
		"decision":"reject",
		"reason":"需要先调整产品角度",
		"next_action":"revise_with_craftsman",
		"revision_instructions":"改成正面三分之二角度"
	}`, uuidString(store.plan.ID))
	got, err := tool.InvokableRun(contextWithNativeRuntime(uuidWithByte(1), uuidWithByte(2), uuidWithByte(3)), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "已拒绝 RenderPlan") {
		t.Fatalf("result = %s", got)
	}
	if len(runtime.createdTasks) != 0 || len(enqueuer.tasks) != 0 {
		t.Fatalf("created tasks = %d, enqueued = %d", len(runtime.createdTasks), len(enqueuer.tasks))
	}
	if !store.rejected {
		t.Fatal("render plan was not marked rejected")
	}
	if len(runtime.processedRenderPlans) != 1 || runtime.processedRenderPlans[0] != store.plan.ID {
		t.Fatalf("processed render plans = %#v", runtime.processedRenderPlans)
	}
}

type fakeRenderPlanDecisionStore struct {
	plan            db.RenderPlan
	shot            db.Shot
	keyElementState db.KeyElementState
	submitted       bool
	rejected        bool
}

func (f *fakeRenderPlanDecisionStore) GetRenderPlanByID(_ context.Context, params db.GetRenderPlanByIDParams) (db.RenderPlan, error) {
	if f.plan.ID == params.ID && f.plan.WorkspaceID == params.WorkspaceID {
		return f.plan, nil
	}
	return db.RenderPlan{}, errShotNotFound
}

func (f *fakeRenderPlanDecisionStore) GetShotByID(_ context.Context, id pgtype.UUID) (db.Shot, error) {
	if f.shot.ID == id {
		return f.shot, nil
	}
	return db.Shot{}, errShotNotFound
}

func (f *fakeRenderPlanDecisionStore) GetKeyElementStateByID(_ context.Context, params db.GetKeyElementStateByIDParams) (db.KeyElementState, error) {
	if f.keyElementState.ID == params.ID && f.keyElementState.WorkspaceID == params.WorkspaceID {
		return f.keyElementState, nil
	}
	return db.KeyElementState{}, errShotNotFound
}

func (f *fakeRenderPlanDecisionStore) MarkRenderPlanSubmitted(_ context.Context, params db.MarkRenderPlanSubmittedParams) (db.RenderPlan, error) {
	f.submitted = true
	f.plan.Status = "submitted"
	f.plan.SubmittedWorkerTaskID = params.SubmittedWorkerTaskID
	return f.plan, nil
}

func (f *fakeRenderPlanDecisionStore) MarkRenderPlanRejected(_ context.Context, params db.MarkRenderPlanRejectedParams) (db.RenderPlan, error) {
	f.rejected = true
	f.plan.Status = "rejected"
	f.plan.Blocker = params.Blocker
	return f.plan, nil
}

type fakeRenderPlanDecisionRuntime struct {
	createdTasks          []db.AgentTask
	events                []agentruntime.CreateEventParams
	processedRenderPlans  []pgtype.UUID
	processedByProducerID []pgtype.UUID
}

func (f *fakeRenderPlanDecisionRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	task := db.AgentTask{
		ID:           uuidWithByte(byte(90 + len(f.createdTasks))),
		WorkspaceID:  params.WorkspaceID,
		ThreadID:     params.ThreadID,
		Role:         params.Role,
		ScopeType:    params.ScopeType,
		ScopeID:      params.ScopeID,
		TaskType:     params.TaskType,
		Status:       "queued",
		MaxAttempts:  params.MaxAttempts,
		Input:        params.Input,
		RenderPlanID: params.RenderPlanID,
	}
	f.createdTasks = append(f.createdTasks, task)
	return task, nil
}

func (f *fakeRenderPlanDecisionRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	f.events = append(f.events, params)
	return db.AgentEvent{ID: uuidWithByte(100), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, TaskID: params.TaskID, EventType: params.EventType}, nil
}

func (f *fakeRenderPlanDecisionRuntime) MarkProducerPendingSignalsProcessedByRenderPlan(_ context.Context, workspaceID, renderPlanID, taskID pgtype.UUID) ([]db.ProducerPendingSignal, error) {
	f.processedRenderPlans = append(f.processedRenderPlans, renderPlanID)
	f.processedByProducerID = append(f.processedByProducerID, taskID)
	return []db.ProducerPendingSignal{{ID: uuidWithByte(120), WorkspaceID: workspaceID, RenderPlanID: renderPlanID, ProcessedByTaskID: taskID, Status: "processed"}}, nil
}

type fakeWorkerTaskEnqueuer struct {
	tasks []db.AgentTask
}

func (f *fakeWorkerTaskEnqueuer) EnqueueWorkerTask(_ context.Context, task db.AgentTask) {
	f.tasks = append(f.tasks, task)
}

func contextWithNativeRuntime(workspaceID pgtype.UUID, threadID pgtype.UUID, taskID pgtype.UUID) context.Context {
	return WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: workspaceID, ThreadID: threadID, TaskID: taskID, ToolCallID: "call_test"})
}
