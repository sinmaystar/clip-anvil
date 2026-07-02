package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestDispatchCraftsmanDefinition(t *testing.T) {
	tool := NewDispatchCraftsmanTool(&fakeCraftsmanDispatchStore{}, &fakeCraftsmanRuntime{}, &fakeCraftsmanEnqueuer{})
	def := tool.Definition()
	if def.Name != "dispatch_craftsman" {
		t.Fatalf("Name = %q", def.Name)
	}
	targetPhase, ok := def.Parameters["properties"].(map[string]any)["target_phase"].(map[string]any)
	if !ok {
		t.Fatalf("target_phase schema missing: %#v", def.Parameters)
	}
	enum, ok := targetPhase["enum"].([]string)
	if !ok || len(enum) != 5 || enum[0] != "reference_image" || enum[1] != "preview_image" || enum[2] != "shot_video" || enum[3] != "voiceover_audio" || enum[4] != "bgm_audio" {
		t.Fatalf("target_phase enum = %#v", targetPhase["enum"])
	}
	scope, ok := def.Parameters["properties"].(map[string]any)["scope"].(map[string]any)
	if !ok {
		t.Fatalf("scope schema missing: %#v", def.Parameters)
	}
	if scope["type"] != "object" {
		t.Fatalf("scope schema = %#v", scope)
	}
	policy, ok := def.Parameters["properties"].(map[string]any)["execution_policy"].(map[string]any)
	if !ok {
		t.Fatalf("execution_policy schema missing: %#v", def.Parameters)
	}
	policyEnum, ok := policy["enum"].([]string)
	if !ok || len(policyEnum) != 2 || policyEnum[0] != "execute_immediately" || policyEnum[1] != "wait_for_producer" {
		t.Fatalf("execution_policy enum = %#v", policy["enum"])
	}
	if !def.Safety.UsesProductionService || def.Safety.WritesCanvas {
		t.Fatalf("Safety = %#v", def.Safety)
	}
	if def.Visibility.UserLabel != "派发生成任务" {
		t.Fatalf("UserLabel = %q", def.Visibility.UserLabel)
	}
}

func TestDispatchCraftsmanDispatchesAllActiveShotsByDefault(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", SemanticKey: "scene_main.shot_01", Title: "开场", Status: "planned"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot-02", SemanticKey: "scene_main.shot_02", Title: "演示", Status: "draft"},
			{ID: uuidWithByte(13), WorkspaceID: uuidWithByte(1), ClientKey: "shot-03", SemanticKey: "scene_main.shot_03", Title: "收尾", Status: "failed"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	enqueuer := &fakeCraftsmanEnqueuer{}
	tool := NewDispatchCraftsmanTool(store, runtime, enqueuer)

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments:   map[string]any{"target_phase": "preview_image", "execution_policy": "execute_immediately", "scope": map[string]any{"type": "shot"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatched := out.Result["dispatched"].([]map[string]any)
	if len(dispatched) != 3 {
		t.Fatalf("dispatched = %#v", out.Result["dispatched"])
	}
	if len(runtime.createdTasks) != 3 || len(enqueuer.tasks) != 3 {
		t.Fatalf("created tasks = %d, enqueued = %d", len(runtime.createdTasks), len(enqueuer.tasks))
	}
	if len(runtime.appended) != 3 {
		t.Fatalf("delegation messages = %#v", runtime.appended)
	}
	for _, msg := range runtime.appended {
		if msg.Role != "user" || msg.MessageType != "text" || !strings.Contains(string(msg.Content), "Producer 派发 Craftsman 任务") {
			t.Fatalf("delegation message = %#v", msg)
		}
	}
	if len(store.statusUpdates) != 0 {
		t.Fatalf("dispatch should not update shot status before render plan exists: %#v", store.statusUpdates)
	}
	summary, _ := out.Result["summary"].(string)
	if !strings.Contains(summary, "加入队列") || !strings.Contains(summary, "不表示图片已经生成完成") {
		t.Fatalf("summary = %q", summary)
	}
	for _, task := range runtime.createdTasks {
		if task.Role != "craftsman" || task.TaskType != "craftsman_turn" || task.ScopeType != "shot" {
			t.Fatalf("task = %#v", task)
		}
		if task.MaxAttempts != 3 {
			t.Fatalf("max attempts = %d", task.MaxAttempts)
		}
		var input map[string]any
		if err := json.Unmarshal(task.Input, &input); err != nil {
			t.Fatal(err)
		}
		if input["target_phase"] != "preview_image" {
			t.Fatalf("task input = %#v", input)
		}
		if !strings.HasPrefix(input["scope_key"].(string), "scene_main.shot_") {
			t.Fatalf("task input missing semantic scope_key: %#v", input)
		}
		if input["execution_policy"] != "execute_immediately" {
			t.Fatalf("execution_policy = %#v, want execute_immediately", input["execution_policy"])
		}
	}
}

func TestDispatchCraftsmanNativeCarriesParentToolCallID(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", SemanticKey: "scene_main.shot_01", Title: "开场", Status: "planned"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanNativeTool(store, runtime, &fakeCraftsmanEnqueuer{})
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ToolCallID:  "producer-dispatch-call",
	})

	got, err := tool.InvokableRun(ctx, `{
		"brief":"直接生成所有分镜预览图。",
		"scope":{"type":"shot"},
		"target_phase":"preview_image",
		"execution_policy":"execute_immediately"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Craftsman 派发结果") {
		t.Fatalf("result = %s", got)
	}
	if len(runtime.createdTasks) != 1 {
		t.Fatalf("created tasks = %d", len(runtime.createdTasks))
	}
	var input map[string]any
	if err := json.Unmarshal(runtime.createdTasks[0].Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["parent_tool_call_id"] != "producer-dispatch-call" {
		t.Fatalf("task input = %#v", input)
	}
	if input["producer_thread_id"] != "02000000-0000-0000-0000-000000000000" {
		t.Fatalf("task input missing producer_thread_id: %#v", input)
	}
	if input["producer_task_id"] != "03000000-0000-0000-0000-000000000000" {
		t.Fatalf("task input missing producer_task_id: %#v", input)
	}
	if input["execution_policy"] != "execute_immediately" {
		t.Fatalf("task input = %#v", input)
	}
	if input["scope_key"] != "scene_main.shot_01" {
		t.Fatalf("task input missing semantic scope_key: %#v", input)
	}
}

func TestDispatchCraftsmanNativeLimitsDispatchToShotRefs(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01", SemanticKey: "scene_main.shot_01", Title: "开场", Status: "preview_ready"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot_02", SemanticKey: "scene_main.shot_02", Title: "细节", Status: "preview_ready"},
			{ID: uuidWithByte(13), WorkspaceID: uuidWithByte(1), ClientKey: "shot_03", SemanticKey: "scene_main.shot_03", Title: "推行", Status: "preview_ready"},
			{ID: uuidWithByte(14), WorkspaceID: uuidWithByte(1), ClientKey: "shot_04", SemanticKey: "scene_main.shot_04", Title: "转场", Status: "preview_ready"},
			{ID: uuidWithByte(15), WorkspaceID: uuidWithByte(1), ClientKey: "shot_05", SemanticKey: "scene_main.shot_05", Title: "收尾", Status: "preview_ready"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanNativeTool(store, runtime, &fakeCraftsmanEnqueuer{})
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ToolCallID:  "producer-dispatch-call",
	})

	got, err := tool.InvokableRun(ctx, `{
		"brief":"只重生成 shot_03 和 shot_04。",
		"scope":{"type":"shot"},
		"shot_refs":["scene_main.shot_03","scene_main.shot_04"],
		"target_phase":"preview_image",
		"execution_policy":"execute_immediately",
		"force":true
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "已将 2 个分镜的预览图") {
		t.Fatalf("result = %s", got)
	}
	if len(runtime.createdTasks) != 2 {
		t.Fatalf("created tasks = %d, want 2", len(runtime.createdTasks))
	}
	gotKeys := []string{}
	for _, task := range runtime.createdTasks {
		var input map[string]any
		if err := json.Unmarshal(task.Input, &input); err != nil {
			t.Fatal(err)
		}
		gotKeys = append(gotKeys, input["shot_client_key"].(string))
		if !strings.HasPrefix(input["scope_key"].(string), "scene_main.shot_") {
			t.Fatalf("task input missing semantic scope_key: %#v", input)
		}
	}
	if strings.Join(gotKeys, ",") != "shot_03,shot_04" {
		t.Fatalf("shot keys = %#v", gotKeys)
	}
}

func TestDispatchCraftsmanRecommendsMotionRouteForNonHeroShotVideos(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01", SemanticKey: "scene_main.shot_01", Title: "开场 hero", Status: "preview_ready"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot_02", SemanticKey: "scene_main.shot_02", Title: "三点卖点卡", Status: "preview_ready"},
			{ID: uuidWithByte(13), WorkspaceID: uuidWithByte(1), ClientKey: "shot_03", SemanticKey: "scene_main.shot_03", Title: "CTA 收尾", Status: "preview_ready"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanTool(store, runtime, &fakeCraftsmanEnqueuer{})

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments: map[string]any{
			"brief":            "生成完整营销视频的分镜视频，控制 Seedance 成本。",
			"target_phase":     "shot_video",
			"execution_policy": "execute_immediately",
			"scope":            map[string]any{"type": "shot"},
			"force":            true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Result["status"] != "queued" {
		t.Fatalf("result = %#v", out.Result)
	}
	if len(runtime.createdTasks) != 3 {
		t.Fatalf("created tasks = %d, want 3", len(runtime.createdTasks))
	}
	profiles := []string{}
	operations := []string{}
	for _, task := range runtime.createdTasks {
		var input map[string]any
		if err := json.Unmarshal(task.Input, &input); err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, input["recommended_model_prompt_profile"].(string))
		operations = append(operations, input["recommended_operation"].(string))
		if input["recommended_route_reason"] == "" {
			t.Fatalf("missing route reason: %#v", input)
		}
	}
	if strings.Join(profiles, ",") != "seedance_2_video,motion_shot_video,motion_shot_video" {
		t.Fatalf("profiles = %#v", profiles)
	}
	if strings.Join(operations, ",") != "image_to_video_first_frame,image_to_motion_video,image_to_motion_video" {
		t.Fatalf("operations = %#v", operations)
	}
}

func TestDispatchCraftsmanMotionOnlyPolicyAvoidsSeedanceForAllShotVideos(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01", SemanticKey: "scene_main.shot_01", Title: "悦行行李箱开场", Status: "preview_ready"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot_02", SemanticKey: "scene_main.shot_02", Title: "卖点口播卡", Status: "preview_ready"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanTool(store, runtime, &fakeCraftsmanEnqueuer{})

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments: map[string]any{
			"brief":              "生成悦行行李箱口播广告，不要调用 Seedance，只使用 Remotion motion shot。",
			"target_phase":       "shot_video",
			"execution_policy":   "execute_immediately",
			"scope":              map[string]any{"type": "shot"},
			"video_route_policy": "motion_only",
			"force":              true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Result["status"] != "queued" {
		t.Fatalf("result = %#v", out.Result)
	}
	if len(runtime.createdTasks) != 2 {
		t.Fatalf("created tasks = %d, want 2", len(runtime.createdTasks))
	}
	for _, task := range runtime.createdTasks {
		var input map[string]any
		if err := json.Unmarshal(task.Input, &input); err != nil {
			t.Fatal(err)
		}
		if input["video_route_policy"] != "motion_only" {
			t.Fatalf("missing video_route_policy: %#v", input)
		}
		if input["recommended_model_prompt_profile"] != "motion_shot_video" ||
			input["recommended_operation"] != "image_to_motion_video" {
			t.Fatalf("policy did not force motion route: %#v", input)
		}
		if strings.Contains(strings.ToLower(input["recommended_route_reason"].(string)), "seedance") &&
			!strings.Contains(strings.ToLower(input["recommended_route_reason"].(string)), "no-seedance") {
			t.Fatalf("route reason should explain no-Seedance policy: %#v", input)
		}
	}
}

func TestDispatchCraftsmanMotionOnlyPolicyDispatchesEveryDynamicShotWithFacts(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01_hook", SemanticKey: "scene_intro.shot_01_hook", Title: "开场钩子", Status: "preview_ready", DurationSec: pgtype.Float8{Float64: 6, Valid: true}, NarrativePurpose: "用短途出行痛点吸引注意", VisualIntent: "行李箱居中，背景留白", ActionText: "产品图轻推近", CameraIntent: "缓慢推进", Narration: "短途出行，行李箱别再拖后腿。"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot_02_product", SemanticKey: "scene_intro.shot_02_product", Title: "产品展示", Status: "preview_ready", DurationSec: pgtype.Float8{Float64: 8, Valid: true}, NarrativePurpose: "建立悦行行李箱主体", VisualIntent: "展示银灰硬壳和轮子", ActionText: "商品细节分层出现", CameraIntent: "轻微视差", Narration: "悦行行李箱，轻便好推。"},
			{ID: uuidWithByte(13), WorkspaceID: uuidWithByte(1), ClientKey: "shot_03_benefits", SemanticKey: "scene_benefit.shot_03_benefits", Title: "卖点卡", Status: "preview_ready", DurationSec: pgtype.Float8{Float64: 8, Valid: true}, NarrativePurpose: "解释万向轮和托运安心", VisualIntent: "卖点文字分组", ActionText: "三点卖点依次入场", CameraIntent: "稳定信息卡", Narration: "顺滑万向轮，转向更稳，安心托运。"},
			{ID: uuidWithByte(14), WorkspaceID: uuidWithByte(1), ClientKey: "shot_04_cta", SemanticKey: "scene_outro.shot_04_cta", Title: "CTA", Status: "preview_ready", DurationSec: pgtype.Float8{Float64: 6, Valid: true}, NarrativePurpose: "收束购买行动", VisualIntent: "按钮和品牌口号清晰", ActionText: "CTA 弹出", CameraIntent: "轻微拉远", Narration: "现在出发。"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanTool(store, runtime, &fakeCraftsmanEnqueuer{})

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments: map[string]any{
			"brief":              "生成 30 秒悦行行李箱广告的所有 Remotion 分镜视频，不调用 Seedance。",
			"target_phase":       "shot_video",
			"execution_policy":   "execute_immediately",
			"scope":              map[string]any{"type": "shot"},
			"shot_refs":          []string{"scene_intro.shot_01_hook", "scene_intro.shot_02_product", "scene_benefit.shot_03_benefits", "scene_outro.shot_04_cta"},
			"video_route_policy": "motion_only",
			"force":              true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Result["status"] != "queued" {
		t.Fatalf("result = %#v", out.Result)
	}
	if len(runtime.createdTasks) != 4 {
		t.Fatalf("created tasks = %d, want 4", len(runtime.createdTasks))
	}
	for _, task := range runtime.createdTasks {
		var input map[string]any
		if err := json.Unmarshal(task.Input, &input); err != nil {
			t.Fatal(err)
		}
		if input["recommended_model_prompt_profile"] != "motion_shot_video" || input["recommended_operation"] != "image_to_motion_video" {
			t.Fatalf("task did not force motion route: %#v", input)
		}
		if input["video_route_policy"] != "motion_only" {
			t.Fatalf("task missing motion_only: %#v", input)
		}
		facts, ok := input["shot_facts"].(map[string]any)
		if !ok {
			t.Fatalf("shot_facts missing: %#v", input)
		}
		for _, key := range []string{"duration_sec", "narrative_purpose", "visual_intent", "action_text", "camera_intent", "narration"} {
			if facts[key] == nil || facts[key] == "" {
				t.Fatalf("shot_facts missing %s: %#v", key, facts)
			}
		}
		params := input["recommended_params"].(map[string]any)
		if params["duration_sec"] != facts["duration_sec"] {
			t.Fatalf("recommended duration does not inherit shot duration: params=%#v facts=%#v", params, facts)
		}
		if strings.Contains(strings.ToLower(mustString(input["recommended_route_reason"])), "seedance") &&
			!strings.Contains(strings.ToLower(mustString(input["recommended_route_reason"])), "no-seedance") {
			t.Fatalf("route reason should only mention Seedance as prohibited policy: %#v", input)
		}
	}
}

func TestDispatchCraftsmanMotionOnlyPolicyAllowsPlannedShotVideo(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01", SemanticKey: "scene_main.shot_01", Title: "悦行行李箱口播卖点卡", Status: "planned"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanTool(store, runtime, &fakeCraftsmanEnqueuer{})

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments: map[string]any{
			"brief":              "用上传产品图生成 Remotion 图片动效口播广告，不要调用 Seedance。",
			"target_phase":       "shot_video",
			"execution_policy":   "execute_immediately",
			"scope":              map[string]any{"type": "shot"},
			"shot_refs":          []string{"shot_01"},
			"input_node_refs":    []string{"box.png"},
			"video_route_policy": "motion_only",
			"force":              true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Result["status"] != "queued" || len(runtime.createdTasks) != 1 {
		t.Fatalf("result=%#v created=%d", out.Result, len(runtime.createdTasks))
	}
	var input map[string]any
	if err := json.Unmarshal(runtime.createdTasks[0].Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["recommended_model_prompt_profile"] != "motion_shot_video" || input["video_route_policy"] != "motion_only" {
		t.Fatalf("task input = %#v", input)
	}
	if got := input["input_node_refs"].([]any); len(got) != 1 || got[0] != "box.png" {
		t.Fatalf("input_node_refs = %#v", input["input_node_refs"])
	}
}

func mustString(value any) string {
	if value == nil {
		return ""
	}
	return value.(string)
}

func TestDispatchCraftsmanNativeLimitsDispatchToExplicitShotScopeRef(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01", SemanticKey: "scene_main.shot_01", Title: "开场", Status: "preview_ready"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot_02", SemanticKey: "scene_main.shot_02", Title: "细节", Status: "preview_ready"},
			{ID: uuidWithByte(13), WorkspaceID: uuidWithByte(1), ClientKey: "shot_03", SemanticKey: "scene_main.shot_03", Title: "推行", Status: "preview_ready"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanNativeTool(store, runtime, &fakeCraftsmanEnqueuer{})
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ToolCallID:  "producer-dispatch-call",
	})

	got, err := tool.InvokableRun(ctx, `{
		"brief":"只重生成 shot_03。",
		"scope":{"type":"shot","id":"shot_03"},
		"shot_refs":[],
		"target_phase":"shot_video",
		"execution_policy":"execute_immediately",
		"force":true
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "已将 1 个分镜视频 RenderPlan 任务加入队列") {
		t.Fatalf("result = %s", got)
	}
	if len(runtime.createdTasks) != 1 {
		t.Fatalf("created tasks = %d, want 1", len(runtime.createdTasks))
	}
	var input map[string]any
	if err := json.Unmarshal(runtime.createdTasks[0].Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["shot_client_key"] != "shot_03" || input["scope_key"] != "scene_main.shot_03" {
		t.Fatalf("task input = %#v", input)
	}
}

func TestDispatchCraftsmanSkipsActiveSameScopePhaseTask(t *testing.T) {
	shotID := uuidWithByte(11)
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: shotID, WorkspaceID: uuidWithByte(1), ClientKey: "shot_01", SemanticKey: "scene_main.shot_01", Title: "开场", Status: "planned"},
		},
	}
	runtime := &fakeCraftsmanRuntime{
		activeTasks: []db.AgentTask{
			{
				ID:          uuidWithByte(71),
				WorkspaceID: uuidWithByte(1),
				Role:        "craftsman",
				ScopeType:   "shot",
				ScopeID:     shotID,
				TaskType:    "craftsman_turn",
				Status:      "running",
				SemanticKey: "craftsman.shot.shot_01.preview_image.active",
				Input:       []byte(`{"target_phase":"preview_image","scope_type":"shot"}`),
			},
		},
	}
	tool := NewDispatchCraftsmanTool(store, runtime, &fakeCraftsmanEnqueuer{})

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments: map[string]any{
			"brief":            "生成 shot_01 预览图。",
			"target_phase":     "preview_image",
			"execution_policy": "execute_immediately",
			"force":            true,
			"scope":            map[string]any{"type": "shot"},
			"shot_refs":        []any{"scene_main.shot_01"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Result["status"] != "skipped" {
		t.Fatalf("status = %#v", out.Result["status"])
	}
	if len(runtime.createdTasks) != 0 {
		t.Fatalf("created duplicate tasks = %#v", runtime.createdTasks)
	}
	skipped := out.Result["skipped"].([]map[string]any)
	if len(skipped) != 1 || skipped[0]["reason"] != "active_craftsman_task_exists" {
		t.Fatalf("skipped = %#v", skipped)
	}
	summary := out.Result["summary"].(string)
	if !strings.Contains(summary, "已存在同一 target_phase") || !strings.Contains(summary, "不要重复派发") {
		t.Fatalf("summary = %q", summary)
	}
}

func TestDispatchCraftsmanDispatchesKeyElementStateReferenceTask(t *testing.T) {
	stateID := uuidWithByte(31)
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		keyElementStates: []db.KeyElementState{
			{ID: stateID, WorkspaceID: uuidWithByte(1), ClientKey: "state_airport_morning", SemanticKey: "element_airport.state_morning", ReferenceStatus: "needs_reference", Status: "active"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	enqueuer := &fakeCraftsmanEnqueuer{}
	tool := NewDispatchCraftsmanTool(store, runtime, enqueuer)

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments: map[string]any{
			"brief":            "生成机场晨光统一参考图。",
			"target_phase":     "reference_image",
			"execution_policy": "execute_immediately",
			"scope":            map[string]any{"type": "key_element_state", "id": uuidString(stateID)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatched := out.Result["dispatched"].([]map[string]any)
	if len(dispatched) != 1 ||
		dispatched[0]["scope_type"] != "key_element_state" ||
		dispatched[0]["scope_ref"] != "key_element_state/element_airport.state_morning" {
		t.Fatalf("dispatched = %#v", dispatched)
	}
	if len(runtime.createdTasks) != 1 || len(enqueuer.tasks) != 1 {
		t.Fatalf("created tasks = %d, enqueued = %d", len(runtime.createdTasks), len(enqueuer.tasks))
	}
	task := runtime.createdTasks[0]
	if task.ScopeType != "key_element_state" || task.ScopeID != stateID {
		t.Fatalf("task scope = %s/%s", task.ScopeType, uuidString(task.ScopeID))
	}
	var input map[string]any
	if err := json.Unmarshal(task.Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["target_phase"] != "reference_image" || input["scope_type"] != "key_element_state" || input["scope_id"] != uuidString(stateID) {
		t.Fatalf("task input = %#v", input)
	}
	if input["scope_key"] != "element_airport.state_morning" {
		t.Fatalf("task input missing semantic scope_key: %#v", input)
	}
	if len(store.statusUpdates) != 0 {
		t.Fatalf("reference dispatch should not touch shot status: %#v", store.statusUpdates)
	}
}

func TestDispatchCraftsmanResolvesKeyElementStateByClientKey(t *testing.T) {
	stateID := uuidWithByte(32)
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		keyElementStates: []db.KeyElementState{
			{ID: stateID, WorkspaceID: uuidWithByte(1), ClientKey: "state_airport_morning", SemanticKey: "element_airport.state_morning", ReferenceStatus: "needs_reference", Status: "active"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanTool(store, runtime, &fakeCraftsmanEnqueuer{})

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments: map[string]any{
			"brief":            "生成机场晨光统一参考图。",
			"target_phase":     "reference_image",
			"execution_policy": "execute_immediately",
			"scope":            map[string]any{"type": "key_element_state", "id": "state_airport_morning"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatched := out.Result["dispatched"].([]map[string]any)
	if len(dispatched) != 1 || dispatched[0]["scope_ref"] != "key_element_state/element_airport.state_morning" {
		t.Fatalf("dispatched = %#v", dispatched)
	}
	if len(runtime.createdTasks) != 1 || runtime.createdTasks[0].ScopeID != stateID {
		t.Fatalf("created tasks = %#v", runtime.createdTasks)
	}
	var input map[string]any
	if err := json.Unmarshal(runtime.createdTasks[0].Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["scope_key"] != "element_airport.state_morning" {
		t.Fatalf("task input missing semantic scope_key: %#v", input)
	}
}

func TestDispatchCraftsmanNativeDispatchesVoiceoverAudioPlan(t *testing.T) {
	audioPlanID := uuidWithByte(41)
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		audioPlan: &db.AudioPlan{
			ID:          audioPlanID,
			WorkspaceID: uuidWithByte(1),
			Status:      "approved",
			Title:       "营销短视频音频方案",
			SemanticKey: "audio_plan.active",
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanNativeTool(store, runtime, &fakeCraftsmanEnqueuer{})
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ToolCallID:  "producer-dispatch-audio",
	})

	got, err := tool.InvokableRun(ctx, `{
		"brief":"为已确认 AudioPlan 生成旁白音频 RenderPlan。",
		"scope":{"type":"audio_plan","id":"audio_plan.active"},
		"target_phase":"voiceover_audio",
		"execution_policy":"wait_for_producer"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "voiceover_audio") {
		t.Fatalf("result = %s", got)
	}
	if len(runtime.createdTasks) != 1 {
		t.Fatalf("created tasks = %d", len(runtime.createdTasks))
	}
	task := runtime.createdTasks[0]
	if task.ScopeType != "audio_plan" || task.ScopeID != audioPlanID {
		t.Fatalf("task scope = %s/%s", task.ScopeType, uuidString(task.ScopeID))
	}
	var input map[string]any
	if err := json.Unmarshal(task.Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["target_phase"] != "voiceover_audio" || input["scope_key"] != "audio_plan.active" {
		t.Fatalf("task input = %#v", input)
	}
}

func TestDispatchCraftsmanNativeDispatchesBGMAudioPlan(t *testing.T) {
	audioPlanID := uuidWithByte(42)
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		audioPlan: &db.AudioPlan{
			ID:          audioPlanID,
			WorkspaceID: uuidWithByte(1),
			Status:      "approved",
			Title:       "营销短视频音频方案",
			SemanticKey: "audio_plan.active",
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanNativeTool(store, runtime, &fakeCraftsmanEnqueuer{})
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
	})

	got, err := tool.InvokableRun(ctx, `{
		"brief":"为已确认 AudioPlan 生成 BGM 音频 RenderPlan。",
		"scope":{"type":"audio_plan","id":"audio_plan.active"},
		"target_phase":"bgm_audio",
		"execution_policy":"wait_for_producer"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "bgm_audio") {
		t.Fatalf("result = %s", got)
	}
	if len(runtime.createdTasks) != 1 || runtime.createdTasks[0].ScopeID != audioPlanID {
		t.Fatalf("created tasks = %#v", runtime.createdTasks)
	}
}

func TestDispatchCraftsmanResolvesShotRefsAndCapsAttempts(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot-01", SemanticKey: "scene_main.shot_01", Title: "开场", Status: "planned"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot-02", SemanticKey: "scene_main.shot_02", Title: "演示", Status: "planned"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanTool(store, runtime, &fakeCraftsmanEnqueuer{})

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments: map[string]any{
			"target_phase":     "preview_image",
			"execution_policy": "wait_for_producer",
			"scope":            map[string]any{"type": "shot"},
			"shot_refs":        []any{"shot-02"},
			"max_attempts":     99.0,
			"review_record_id": "review-1",
			"critique":         "商品太小",
			"fix_hints":        []any{"拉近主体"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatched := out.Result["dispatched"].([]map[string]any)
	if len(dispatched) != 1 || dispatched[0]["client_key"] != "shot-02" {
		t.Fatalf("dispatched = %#v", dispatched)
	}
	if len(runtime.createdTasks) != 1 {
		t.Fatalf("created tasks = %d", len(runtime.createdTasks))
	}
	if runtime.createdTasks[0].MaxAttempts != 3 {
		t.Fatalf("max attempts = %d", runtime.createdTasks[0].MaxAttempts)
	}
	var input map[string]any
	if err := json.Unmarshal(runtime.createdTasks[0].Input, &input); err != nil {
		t.Fatal(err)
	}
	if input["review_record_id"] != "review-1" || input["review_critique"] != "商品太小" {
		t.Fatalf("task input = %#v", input)
	}
	if input["execution_policy"] != "wait_for_producer" {
		t.Fatalf("execution_policy = %#v, want wait_for_producer", input["execution_policy"])
	}
	hints, _ := input["review_fix_hints"].([]any)
	if len(hints) != 1 || hints[0] != "拉近主体" {
		t.Fatalf("review_fix_hints = %#v", input["review_fix_hints"])
	}
}

type fakeCraftsmanDispatchStore struct {
	workspace        db.Workspace
	shots            []db.Shot
	keyElementStates []db.KeyElementState
	audioPlan        *db.AudioPlan
	linked           []db.SetShotCraftsmanThreadParams
	statusUpdates    []db.UpdateShotStatusParams
}

func (f *fakeCraftsmanDispatchStore) GetWorkspaceByID(context.Context, pgtype.UUID) (db.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeCraftsmanDispatchStore) ListActiveShotsByWorkspace(context.Context, pgtype.UUID) ([]db.Shot, error) {
	return f.shots, nil
}

func (f *fakeCraftsmanDispatchStore) GetShotByID(_ context.Context, id pgtype.UUID) (db.Shot, error) {
	for _, shot := range f.shots {
		if shot.ID == id {
			return shot, nil
		}
	}
	return db.Shot{}, errShotNotFound
}

func (f *fakeCraftsmanDispatchStore) GetShotByClientKey(_ context.Context, params db.GetShotByClientKeyParams) (db.Shot, error) {
	for _, shot := range f.shots {
		if shot.WorkspaceID == params.WorkspaceID && shot.ClientKey == params.ClientKey {
			return shot, nil
		}
	}
	return db.Shot{}, errShotNotFound
}

func (f *fakeCraftsmanDispatchStore) GetKeyElementStateByID(_ context.Context, params db.GetKeyElementStateByIDParams) (db.KeyElementState, error) {
	for _, state := range f.keyElementStates {
		if state.WorkspaceID == params.WorkspaceID && state.ID == params.ID {
			return state, nil
		}
	}
	return db.KeyElementState{}, errScopeNotFound
}

func (f *fakeCraftsmanDispatchStore) ListActiveKeyElementStatesByWorkspace(context.Context, pgtype.UUID) ([]db.KeyElementState, error) {
	return f.keyElementStates, nil
}

func (f *fakeCraftsmanDispatchStore) GetActiveAudioPlanByWorkspace(context.Context, pgtype.UUID) (db.AudioPlan, error) {
	if f.audioPlan == nil {
		return db.AudioPlan{}, errScopeNotFound
	}
	return *f.audioPlan, nil
}

func (f *fakeCraftsmanDispatchStore) GetAudioPlan(_ context.Context, params db.GetAudioPlanParams) (db.AudioPlan, error) {
	if f.audioPlan == nil || f.audioPlan.ID != params.ID || f.audioPlan.WorkspaceID != params.WorkspaceID {
		return db.AudioPlan{}, errScopeNotFound
	}
	return *f.audioPlan, nil
}

func (f *fakeCraftsmanDispatchStore) SetShotCraftsmanThread(_ context.Context, params db.SetShotCraftsmanThreadParams) (db.Shot, error) {
	f.linked = append(f.linked, params)
	for _, shot := range f.shots {
		if shot.ID == params.ID {
			shot.CraftsmanThreadID = params.CraftsmanThreadID
			return shot, nil
		}
	}
	return db.Shot{}, errShotNotFound
}

func (f *fakeCraftsmanDispatchStore) UpdateShotStatus(_ context.Context, params db.UpdateShotStatusParams) (db.Shot, error) {
	f.statusUpdates = append(f.statusUpdates, params)
	for index, shot := range f.shots {
		if shot.ID == params.ID && shot.WorkspaceID == params.WorkspaceID {
			f.shots[index].Status = params.Status
			return f.shots[index], nil
		}
	}
	return db.Shot{}, errShotNotFound
}

type fakeCraftsmanRuntime struct {
	createdTasks []db.AgentTask
	activeTasks  []db.AgentTask
	events       []agentruntime.CreateEventParams
	appendSeq    int64
	appended     []db.AgentMessage
}

func (f *fakeCraftsmanRuntime) GetOrCreateCraftsmanThread(_ context.Context, workspaceID, shotID pgtype.UUID) (db.AgentThread, error) {
	return db.AgentThread{ID: pgtype.UUID{Bytes: shotID.Bytes, Valid: true}, WorkspaceID: workspaceID, Role: "craftsman", ScopeType: "shot", ScopeID: shotID}, nil
}

func (f *fakeCraftsmanRuntime) GetOrCreateCraftsmanThreadForScope(_ context.Context, workspaceID pgtype.UUID, scopeType string, scopeID pgtype.UUID) (db.AgentThread, error) {
	return db.AgentThread{ID: pgtype.UUID{Bytes: scopeID.Bytes, Valid: true}, WorkspaceID: workspaceID, Role: "craftsman", ScopeType: scopeType, ScopeID: scopeID}, nil
}

func (f *fakeCraftsmanRuntime) ListActiveAgentTasksByWorkspace(context.Context, pgtype.UUID) ([]db.AgentTask, error) {
	return f.activeTasks, nil
}

func (f *fakeCraftsmanRuntime) CreateTask(_ context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error) {
	task := db.AgentTask{
		ID:          uuidWithByte(byte(80 + len(f.createdTasks))),
		WorkspaceID: params.WorkspaceID,
		ThreadID:    params.ThreadID,
		Role:        params.Role,
		ScopeType:   params.ScopeType,
		ScopeID:     params.ScopeID,
		TaskType:    params.TaskType,
		Status:      "queued",
		MaxAttempts: params.MaxAttempts,
		Input:       params.Input,
	}
	f.createdTasks = append(f.createdTasks, task)
	return task, nil
}

func (f *fakeCraftsmanRuntime) CreateEvent(_ context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error) {
	f.events = append(f.events, params)
	return db.AgentEvent{ID: uuidWithByte(90), WorkspaceID: params.WorkspaceID, ThreadID: params.ThreadID, TaskID: params.TaskID, EventType: params.EventType}, nil
}

func (f *fakeCraftsmanRuntime) AppendMessage(_ context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error) {
	f.appendSeq++
	msg := db.AgentMessage{
		ID:          uuidWithByte(byte(100 + f.appendSeq)),
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

type fakeCraftsmanEnqueuer struct {
	tasks []db.AgentTask
}

func (f *fakeCraftsmanEnqueuer) EnqueueCraftsmanTask(_ context.Context, task db.AgentTask) {
	f.tasks = append(f.tasks, task)
}
