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
			SemanticKey:        "scene_main.shot_01.preview_image.r1",
		},
		shot: db.Shot{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01", SemanticKey: "scene_main.shot_01", SortOrder: 1},
	}
	runtime := &fakeRenderPlanDecisionRuntime{}
	enqueuer := &fakeWorkerTaskEnqueuer{}
	tool := NewDecideRenderPlanNativeTool(store, runtime, enqueuer)

	args := fmt.Sprintf(`{
		"brief":"接受 RenderPlan 并提交 Worker。",
		"render_plan_ref":{"type":"render_plan","key":%q},
		"decision":"accept",
		"reason":"用户已确认出图",
		"next_action":"submit_worker"
	}`, store.plan.SemanticKey)
	got, err := tool.InvokableRun(contextWithNativeRuntime(uuidWithByte(1), uuidWithByte(2), uuidWithByte(3)), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "已接受 RenderPlan") || !strings.Contains(got, "worker_generation") {
		t.Fatalf("result = %s", got)
	}
	if strings.Contains(got, uuidString(uuidWithByte(90))) {
		t.Fatalf("result leaked worker task UUID: %s", got)
	}
	if !strings.Contains(got, "worker.scene_main.shot_01.worker_generation.t1") {
		t.Fatalf("result missing worker semantic key: %s", got)
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
	if input["render_plan_key"] != "scene_main.shot_01.preview_image.r1" || input["scope_key"] != "scene_main.shot_01" {
		t.Fatalf("worker input missing semantic keys = %#v", input)
	}
	if !store.submitted {
		t.Fatal("render plan was not marked submitted")
	}
	if len(runtime.processedRenderPlans) != 1 || runtime.processedRenderPlans[0] != store.plan.ID {
		t.Fatalf("processed render plans = %#v", runtime.processedRenderPlans)
	}
}

func TestDecideRenderPlanAcceptKeepsShotOutputInputRefs(t *testing.T) {
	store := &fakeRenderPlanDecisionStore{
		plan: db.RenderPlan{
			ID:                 uuidWithByte(21),
			WorkspaceID:        uuidWithByte(1),
			ScopeType:          "shot",
			ScopeID:            uuidWithByte(11),
			TargetPhase:        "shot_video",
			ModelPromptProfile: "seedance_2_video",
			Operation:          "image_to_video_first_frame",
			Status:             "waiting_for_approval",
			CompiledPrompt:     "把已确认预览图生成 5 秒行李箱广告视频。",
			Params:             []byte(`{"ratio":"9:16","duration_sec":5}`),
			ReferenceBindings:  []byte(`[{"client_key":"ref_shot_01_preview","source_type":"shot_output","source_id":"shot_01.preview_image.current","content_type":"image_url","model_role":"first_frame","required":true}]`),
			CreatedByThreadID:  uuidWithByte(12),
			CreatedByTaskID:    uuidWithByte(13),
			SemanticKey:        "shot_01.shot_video.r1",
		},
		shot: db.Shot{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01", SemanticKey: "shot_01", SortOrder: 1},
	}
	runtime := &fakeRenderPlanDecisionRuntime{}
	tool := NewDecideRenderPlanNativeTool(store, runtime, &fakeWorkerTaskEnqueuer{})

	args := fmt.Sprintf(`{
		"brief":"接受分镜视频 RenderPlan 并提交 Worker。",
		"render_plan_ref":{"type":"render_plan","key":%q},
		"decision":"accept",
		"reason":"预览图已成功，可以生成视频",
		"next_action":"submit_worker"
	}`, store.plan.SemanticKey)
	_, err := tool.InvokableRun(contextWithNativeRuntime(uuidWithByte(1), uuidWithByte(2), uuidWithByte(3)), args)
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.createdTasks) != 1 {
		t.Fatalf("created tasks = %d", len(runtime.createdTasks))
	}
	var input map[string]any
	if err := json.Unmarshal(runtime.createdTasks[0].Input, &input); err != nil {
		t.Fatal(err)
	}
	bindings, _ := input["input_bindings"].([]any)
	if len(bindings) != 1 {
		t.Fatalf("input_bindings = %#v", input["input_bindings"])
	}
	binding, _ := bindings[0].(map[string]any)
	if binding["source_id"] != "shot_01.preview_image.current" ||
		binding["content_type"] != "image_url" ||
		binding["model_role"] != "first_frame" {
		t.Fatalf("input_bindings[0] = %#v", binding)
	}
	if _, ok := binding["role"]; ok {
		t.Fatalf("input_bindings[0] must not contain legacy role: %#v", binding)
	}
}

func TestDecideRenderPlanAcceptSubmitsAudioPlanWorkerTask(t *testing.T) {
	store := &fakeRenderPlanDecisionStore{
		plan: db.RenderPlan{
			ID:                 uuidWithByte(21),
			WorkspaceID:        uuidWithByte(1),
			ScopeType:          "audio_plan",
			ScopeID:            uuidWithByte(11),
			TargetPhase:        "voiceover_audio",
			ModelPromptProfile: "seed_audio_1",
			Operation:          "text_to_audio",
			Status:             "waiting_for_approval",
			CompiledPrompt:     "zh, 12 seconds, warm female voice, script: 现在出发，让旅程更轻松。",
			Params:             []byte(`{"model":"seed-audio-1.0","speaker":"warm_female","format":"mp3"}`),
			Rationale:          "基于已确认 AudioPlan 生成全片旁白。",
			CreatedByThreadID:  uuidWithByte(12),
			CreatedByTaskID:    uuidWithByte(13),
			SemanticKey:        "audio_plan.active.voiceover_audio.r1",
		},
	}
	runtime := &fakeRenderPlanDecisionRuntime{}
	enqueuer := &fakeWorkerTaskEnqueuer{}
	tool := NewDecideRenderPlanNativeTool(store, runtime, enqueuer)

	args := fmt.Sprintf(`{
		"brief":"接受旁白音频 RenderPlan 并提交 Worker。",
		"render_plan_ref":{"type":"render_plan","key":%q},
		"decision":"accept",
		"reason":"用户已确认 AudioPlan",
		"next_action":"submit_worker"
	}`, store.plan.SemanticKey)
	got, err := tool.InvokableRun(contextWithNativeRuntime(uuidWithByte(1), uuidWithByte(2), uuidWithByte(3)), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "已接受 RenderPlan") {
		t.Fatalf("result = %s", got)
	}
	if len(runtime.createdTasks) != 1 || len(enqueuer.tasks) != 1 {
		t.Fatalf("created tasks = %d, enqueued = %d", len(runtime.createdTasks), len(enqueuer.tasks))
	}
	task := runtime.createdTasks[0]
	if task.ScopeType != "audio_plan" || task.ScopeID != uuidWithByte(11) || task.RenderPlanID != store.plan.ID {
		t.Fatalf("task = %#v", task)
	}
	var input map[string]any
	if err := json.Unmarshal(task.Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["mode"] != "voiceover_audio" || input["scope_type"] != "audio_plan" || input["scope_key"] != "audio_plan.active" {
		t.Fatalf("worker input = %#v", input)
	}
	if input["output_type"] != "audio" || input["operation_type"] != "text_to_audio" {
		t.Fatalf("worker input generation fields = %#v", input)
	}
	model, _ := input["model"].(map[string]any)
	if model["provider"] != "volcengine" || model["model_id"] != "seed-audio-1.0" {
		t.Fatalf("worker input model = %#v", model)
	}
	if input["shot_id"] != "" {
		t.Fatalf("audio worker input must not require shot_id: %#v", input)
	}
}

func TestDecideRenderPlanBatchAcceptsAudioPlanRenderPlans(t *testing.T) {
	audioPlanID := uuidWithByte(11)
	voiceoverPlan := db.RenderPlan{
		ID:                 uuidWithByte(21),
		WorkspaceID:        uuidWithByte(1),
		ScopeType:          "audio_plan",
		ScopeID:            audioPlanID,
		TargetPhase:        "voiceover_audio",
		ModelPromptProfile: "seed_audio_1",
		Operation:          "text_to_audio",
		Status:             "waiting_for_approval",
		CompiledPrompt:     "生成全片中文旁白。",
		Params:             []byte(`{"model":"seed-audio-1.0","speaker":"warm_female","format":"mp3"}`),
		SemanticKey:        "audio_plan.active.voiceover_audio.r1",
	}
	bgmPlan := db.RenderPlan{
		ID:                 uuidWithByte(22),
		WorkspaceID:        uuidWithByte(1),
		ScopeType:          "audio_plan",
		ScopeID:            audioPlanID,
		TargetPhase:        "bgm_audio",
		ModelPromptProfile: "seed_audio_1",
		Operation:          "text_to_audio",
		Status:             "waiting_for_approval",
		CompiledPrompt:     "生成 12 秒轻快电子流行 BGM。",
		Params:             []byte(`{"model":"seed-audio-1.0","format":"mp3"}`),
		SemanticKey:        "audio_plan.active.bgm_audio.r1",
	}
	store := &fakeRenderPlanDecisionStore{
		plans: map[string]db.RenderPlan{
			uuidString(voiceoverPlan.ID): voiceoverPlan,
			uuidString(bgmPlan.ID):       bgmPlan,
		},
	}
	runtime := &fakeRenderPlanDecisionRuntime{}
	enqueuer := &fakeWorkerTaskEnqueuer{}
	tool := NewDecideRenderPlanNativeTool(store, runtime, enqueuer)

	args := fmt.Sprintf(`{
		"brief":"接受已确认 AudioPlan 的旁白和 BGM 音频 RenderPlan。",
		"decisions":[
			{"render_plan_id":%q,"decision":"accept","reason":"旁白方案可执行","next_action":"submit_worker"},
			{"render_plan_id":%q,"decision":"accept","reason":"BGM 方案可执行","next_action":"submit_worker"}
		]
	}`, uuidString(voiceoverPlan.ID), uuidString(bgmPlan.ID))
	got, err := tool.InvokableRun(contextWithNativeRuntime(uuidWithByte(1), uuidWithByte(2), uuidWithByte(3)), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "批量 RenderPlan 决策完成") {
		t.Fatalf("result = %s", got)
	}
	if len(runtime.createdTasks) != 2 || len(enqueuer.tasks) != 2 {
		t.Fatalf("created tasks = %d, enqueued = %d", len(runtime.createdTasks), len(enqueuer.tasks))
	}
	if len(store.voiceoverLinks) != 1 || store.voiceoverLinks[0].VoiceoverRenderPlanID != voiceoverPlan.ID {
		t.Fatalf("voiceover links = %#v", store.voiceoverLinks)
	}
	if len(store.bgmLinks) != 1 || store.bgmLinks[0].BgmRenderPlanID != bgmPlan.ID {
		t.Fatalf("bgm links = %#v", store.bgmLinks)
	}
	for _, task := range runtime.createdTasks {
		if task.ScopeType != "audio_plan" || task.ScopeID != audioPlanID {
			t.Fatalf("task = %#v", task)
		}
		var input map[string]any
		if err := json.Unmarshal(task.Input, &input); err != nil {
			t.Fatal(err)
		}
		if input["output_type"] != "audio" || input["operation_type"] != "text_to_audio" {
			t.Fatalf("worker input = %#v", input)
		}
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

func TestDecideRenderPlanBatchDecisionsProcessEachRenderPlan(t *testing.T) {
	acceptPlan := db.RenderPlan{
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
		CreatedByThreadID:  uuidWithByte(12),
		CreatedByTaskID:    uuidWithByte(13),
		SemanticKey:        "shot_01.preview_image.r1",
	}
	rejectPlan := db.RenderPlan{
		ID:          uuidWithByte(22),
		WorkspaceID: uuidWithByte(1),
		ScopeType:   "shot",
		ScopeID:     uuidWithByte(12),
		TargetPhase: "preview_image",
		Status:      "waiting_for_approval",
		SemanticKey: "shot_02.preview_image.r1",
	}
	store := &fakeRenderPlanDecisionStore{
		plans: map[string]db.RenderPlan{
			uuidString(acceptPlan.ID): acceptPlan,
			uuidString(rejectPlan.ID): rejectPlan,
		},
		shots: map[string]db.Shot{
			uuidString(acceptPlan.ScopeID): {ID: acceptPlan.ScopeID, WorkspaceID: uuidWithByte(1), ClientKey: "shot_01"},
			uuidString(rejectPlan.ScopeID): {ID: rejectPlan.ScopeID, WorkspaceID: uuidWithByte(1), ClientKey: "shot_02"},
		},
	}
	runtime := &fakeRenderPlanDecisionRuntime{}
	enqueuer := &fakeWorkerTaskEnqueuer{}
	tool := NewDecideRenderPlanNativeTool(store, runtime, enqueuer)

	args := fmt.Sprintf(`{
		"brief":"批量处理 Craftsman 提交的预览图 RenderPlan。",
		"decisions":[
			{
				"render_plan_id":%q,
				"decision":"accept",
				"reason":"符合全局视觉方向",
				"next_action":"submit_worker"
			},
			{
				"render_plan_id":%q,
				"decision":"reject",
				"reason":"产品占比太低",
				"next_action":"revise_with_craftsman",
				"revision_instructions":"放大行李箱并减少背景人物"
			}
		]
	}`, uuidString(acceptPlan.ID), uuidString(rejectPlan.ID))
	got, err := tool.InvokableRun(contextWithNativeRuntime(uuidWithByte(1), uuidWithByte(2), uuidWithByte(3)), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "批量 RenderPlan 决策完成") ||
		!strings.Contains(got, "shot_01.preview_image.r1") ||
		!strings.Contains(got, "shot_02.preview_image.r1") {
		t.Fatalf("result = %s", got)
	}
	if strings.Contains(got, uuidString(acceptPlan.ID)) || strings.Contains(got, uuidString(rejectPlan.ID)) {
		t.Fatalf("result = %s", got)
	}
	if len(runtime.createdTasks) != 1 || len(enqueuer.tasks) != 1 {
		t.Fatalf("created tasks = %d, enqueued = %d", len(runtime.createdTasks), len(enqueuer.tasks))
	}
	if len(store.rejectedPlans) != 1 || store.rejectedPlans[0] != rejectPlan.ID {
		t.Fatalf("rejected plans = %#v", store.rejectedPlans)
	}
	if len(runtime.processedRenderPlans) != 2 ||
		runtime.processedRenderPlans[0] != acceptPlan.ID ||
		runtime.processedRenderPlans[1] != rejectPlan.ID {
		t.Fatalf("processed render plans = %#v", runtime.processedRenderPlans)
	}
}

type fakeRenderPlanDecisionStore struct {
	plan            db.RenderPlan
	plans           map[string]db.RenderPlan
	shot            db.Shot
	shots           map[string]db.Shot
	keyElementState db.KeyElementState
	submitted       bool
	submittedPlans  []pgtype.UUID
	voiceoverLinks  []db.SetAudioPlanVoiceoverRenderPlanParams
	bgmLinks        []db.SetAudioPlanBGMRenderPlanParams
	rejected        bool
	rejectedPlans   []pgtype.UUID
}

func (f *fakeRenderPlanDecisionStore) GetRenderPlanByID(_ context.Context, params db.GetRenderPlanByIDParams) (db.RenderPlan, error) {
	if f.plans != nil {
		plan, ok := f.plans[uuidString(params.ID)]
		if ok && plan.WorkspaceID == params.WorkspaceID {
			return plan, nil
		}
		return db.RenderPlan{}, errShotNotFound
	}
	if f.plan.ID == params.ID && f.plan.WorkspaceID == params.WorkspaceID {
		return f.plan, nil
	}
	return db.RenderPlan{}, errShotNotFound
}

func (f *fakeRenderPlanDecisionStore) GetRenderPlanBySemanticKey(_ context.Context, params db.GetRenderPlanBySemanticKeyParams) (db.RenderPlan, error) {
	if f.plans != nil {
		for _, plan := range f.plans {
			if plan.WorkspaceID == params.WorkspaceID && plan.SemanticKey == params.SemanticKey {
				return plan, nil
			}
		}
		return db.RenderPlan{}, errShotNotFound
	}
	if f.plan.WorkspaceID == params.WorkspaceID && f.plan.SemanticKey == params.SemanticKey {
		return f.plan, nil
	}
	return db.RenderPlan{}, errShotNotFound
}

func (f *fakeRenderPlanDecisionStore) GetShotByID(_ context.Context, id pgtype.UUID) (db.Shot, error) {
	if f.shots != nil {
		shot, ok := f.shots[uuidString(id)]
		if ok {
			return shot, nil
		}
		return db.Shot{}, errShotNotFound
	}
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
	f.submittedPlans = append(f.submittedPlans, params.ID)
	if f.plans != nil {
		plan := f.plans[uuidString(params.ID)]
		plan.Status = "submitted"
		plan.SubmittedWorkerTaskID = params.SubmittedWorkerTaskID
		f.plans[uuidString(params.ID)] = plan
		return plan, nil
	}
	f.plan.Status = "submitted"
	f.plan.SubmittedWorkerTaskID = params.SubmittedWorkerTaskID
	return f.plan, nil
}

func (f *fakeRenderPlanDecisionStore) SetAudioPlanVoiceoverRenderPlan(_ context.Context, params db.SetAudioPlanVoiceoverRenderPlanParams) (db.AudioPlan, error) {
	f.voiceoverLinks = append(f.voiceoverLinks, params)
	return db.AudioPlan{ID: params.ID, WorkspaceID: params.WorkspaceID, VoiceoverRenderPlanID: params.VoiceoverRenderPlanID}, nil
}

func (f *fakeRenderPlanDecisionStore) SetAudioPlanBGMRenderPlan(_ context.Context, params db.SetAudioPlanBGMRenderPlanParams) (db.AudioPlan, error) {
	f.bgmLinks = append(f.bgmLinks, params)
	return db.AudioPlan{ID: params.ID, WorkspaceID: params.WorkspaceID, BgmRenderPlanID: params.BgmRenderPlanID}, nil
}

func (f *fakeRenderPlanDecisionStore) MarkRenderPlanRejected(_ context.Context, params db.MarkRenderPlanRejectedParams) (db.RenderPlan, error) {
	f.rejected = true
	f.rejectedPlans = append(f.rejectedPlans, params.ID)
	if f.plans != nil {
		plan := f.plans[uuidString(params.ID)]
		plan.Status = "rejected"
		plan.Blocker = params.Blocker
		f.plans[uuidString(params.ID)] = plan
		return plan, nil
	}
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
		SemanticKey:  "worker.scene_main.shot_01.worker_generation.t1",
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
