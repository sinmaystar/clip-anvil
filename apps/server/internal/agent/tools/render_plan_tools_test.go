package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestRenderPlanToolInfosUseTypedSchemasAndChineseDescriptions(t *testing.T) {
	renderTool := NewUpsertRenderPlanNativeTool(nil)
	info, err := renderTool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "upsert_render_plan" {
		t.Fatalf("name = %q", info.Name)
	}
	if !strings.Contains(info.Desc, "RenderPlan") || !strings.Contains(info.Desc, "Seedream") {
		t.Fatalf("description not specific enough: %s", info.Desc)
	}
	schema, err := info.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(schema)
	for _, want := range []string{"generation_text", "model_prompt_profile", "reference_bindings", "prompt_parts"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("schema missing %s: %s", want, string(raw))
		}
	}
	for _, want := range []string{"主体", "场景", "光线", "镜头", "动作", "风格", "音频", "避免"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("generation_text description missing %q: %s", want, string(raw))
		}
	}
}

func TestUpsertRenderPlanToolReturnsNaturalValidationError(t *testing.T) {
	tool := NewUpsertRenderPlanNativeTool(nil)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ScopeType:   "shot",
		ScopeID:     uuidWithByte(4),
		TargetPhase: "preview_image",
		ToolCallID:  "call_render_plan",
	})
	got, err := tool.InvokableRun(ctx, `{"mode":"create"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "工具调用失败") || !strings.Contains(got, "brief") {
		t.Fatalf("result = %s", got)
	}
}

func TestUpsertRenderPlanToolAcceptsMinimalPlanArguments(t *testing.T) {
	tool := NewUpsertRenderPlanNativeTool(nil)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ScopeType:   "shot",
		ScopeID:     uuidWithByte(4),
		TargetPhase: "preview_image",
		ToolCallID:  "call_render_plan",
	})

	got, err := tool.InvokableRun(ctx, `{
		"brief":"为 shot_01 创建预览图 RenderPlan。",
		"mode":"create",
		"generation_text":"生成一张 9:16 分镜预览图：银灰色硬壳竖条纹行李箱位于现代机场出发大厅中央，柔和晨光从落地窗照入，镜头平视略低机位，主体清晰完整，商业广告质感，避免改变箱体颜色、条纹、护角和万向轮结构。"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "render plan service 未配置") {
		t.Fatalf("result = %s", got)
	}
	if strings.Contains(got, "prompt_parts.objective") || strings.Contains(got, "rationale") {
		t.Fatalf("minimal plan should not fail long-field validation: %s", got)
	}
}

func TestUpsertRenderPlanToolRequiresGenerationTextForWritablePlan(t *testing.T) {
	tool := NewUpsertRenderPlanNativeTool(nil)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ScopeType:   "shot",
		ScopeID:     uuidWithByte(4),
		TargetPhase: "preview_image",
		ToolCallID:  "call_render_plan",
	})

	got, err := tool.InvokableRun(ctx, `{
		"brief":"为 shot_01 创建预览图 RenderPlan。",
		"mode":"create"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "generation_text 必填") {
		t.Fatalf("result = %s", got)
	}
}

func TestUpsertRenderPlanRuntimeDefaultsInferVideoOperationFromFirstFrame(t *testing.T) {
	input := UpsertRenderPlanToolInput{
		Brief:          "为 shot_02 创建视频计划。",
		Mode:           "create",
		GenerationText: "生成 5 秒 9:16 分镜视频：银灰色行李箱从首帧画面开始被人物平稳拉动，镜头轻微跟拍，万向轮顺滑滚动，光线干净高级，避免产品变形。",
		ReferenceBindings: []ReferenceBindingInput{{
			ClientKey:      "ref_shot_02_preview",
			SourceType:     "artifact_version",
			SourceID:       "05000000-0000-0000-0000-000000000000",
			Role:           "first_frame",
			SemanticTarget: "shot_02 首帧预览图",
		}},
		Params: RenderPlanParamsInput{DurationSec: 5, Ratio: "9:16"},
	}

	got := applyUpsertRenderPlanRuntimeDefaults(input, NativeRuntimeContext{
		ScopeType:   "shot",
		ScopeID:     uuidWithByte(4),
		ScopeKey:    "scene_main.shot_02",
		TargetPhase: "shot_video",
	})

	if got.Scope.Type != "shot" || got.Scope.ID != "04000000-0000-0000-0000-000000000000" || got.Scope.Key != "scene_main.shot_02" {
		t.Fatalf("scope defaults = %#v", got.Scope)
	}
	if got.TargetPhase != "shot_video" || got.TaskType != "generate" || got.ModelPromptProfile != "seedance_2_video" || got.Operation != "image_to_video_first_frame" {
		t.Fatalf("inferred fields = target=%q task=%q profile=%q operation=%q", got.TargetPhase, got.TaskType, got.ModelPromptProfile, got.Operation)
	}
	if got.PromptParts.Objective != got.GenerationText || got.PromptParts.Action != got.GenerationText {
		t.Fatalf("prompt parts should be backed by generation_text: %#v", got.PromptParts)
	}
}

func TestUpsertRenderPlanToolRejectsRuntimeTargetPhaseMismatch(t *testing.T) {
	tool := NewUpsertRenderPlanNativeTool(nil)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ScopeType:   "shot",
		ScopeID:     uuidWithByte(4),
		TargetPhase: "preview_image",
		ToolCallID:  "call_render_plan",
	})

	got, err := tool.InvokableRun(ctx, `{
		"brief":"错误地创建视频计划。",
		"mode":"create",
		"generation_text":"生成一段错误的分镜视频计划，用来验证 target_phase 与当前 Craftsman task 不一致时会被拒绝。",
		"scope":{"type":"shot","id":"04000000-0000-0000-0000-000000000000"},
		"target_phase":"shot_video",
		"task_type":"generate",
		"model_prompt_profile":"seedance_2_video",
		"operation":"image_to_video_first_frame"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "target_phase 必须与当前 Craftsman 任务一致") {
		t.Fatalf("result = %s", got)
	}
}

func TestUpsertRenderPlanToolRejectsRuntimeScopeMismatch(t *testing.T) {
	tool := NewUpsertRenderPlanNativeTool(nil)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ScopeType:   "shot",
		ScopeID:     uuidWithByte(4),
		TargetPhase: "preview_image",
		ToolCallID:  "call_render_plan",
	})

	got, err := tool.InvokableRun(ctx, `{
		"brief":"错误地写入另一个分镜。",
		"mode":"create",
		"generation_text":"生成一张错误 scope 的分镜预览图，用来验证 scope 与当前 Craftsman task 不一致时会被拒绝。",
		"scope":{"type":"shot","id":"05000000-0000-0000-0000-000000000000"},
		"target_phase":"preview_image",
		"task_type":"generate",
		"model_prompt_profile":"seedream_5_image",
		"operation":"image_to_image"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "scope 必须与当前 Craftsman 任务一致") {
		t.Fatalf("result = %s", got)
	}
}

func TestUpsertRenderPlanToolRejectsMissingMediaNodeReferenceBeforeService(t *testing.T) {
	tool := NewUpsertRenderPlanNativeTool(nil).WithReferenceStore(fakeRenderPlanReferenceStore{
		nodes: []db.MediaNode{{
			ID:          uuidWithByte(9),
			WorkspaceID: uuidWithByte(1),
			Title:       "box.png",
			NodeType:    db.NodeTypeImage,
		}},
	})
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		ScopeType:   "shot",
		ScopeID:     uuidWithByte(4),
		TargetPhase: "preview_image",
		ToolCallID:  "call_render_plan",
	})

	got, err := tool.InvokableRun(ctx, `{
		"brief":"为 shot_01 创建预览图 RenderPlan。",
		"mode":"create",
		"generation_text":"生成一张 9:16 分镜预览图：银灰色硬壳竖条纹行李箱位于现代机场出发大厅中央，柔和晨光从落地窗照入，镜头平视略低机位，主体清晰完整，商业广告质感，避免改变箱体颜色、条纹、护角和万向轮结构。",
		"scope":{"type":"shot","id":"04000000-0000-0000-0000-000000000000"},
		"target_phase":"preview_image",
		"task_type":"generate",
		"model_prompt_profile":"seedream_5_image",
		"operation":"image_to_image",
		"reference_bindings":[{
			"client_key":"ref_product_luggage",
			"source_type":"media_node",
			"source_id":"123e4567-e89b-12d3-a456-426614174000",
			"role":"product_reference",
			"semantic_target":"悦行行李箱",
			"priority":1,
			"required":true
		}]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "工具调用失败") ||
		!strings.Contains(got, "reference_bindings[0].source_id") ||
		!strings.Contains(got, "123e4567-e89b-12d3-a456-426614174000") ||
		!strings.Contains(got, "不存在") ||
		!strings.Contains(got, "box.png") {
		t.Fatalf("result = %s", got)
	}
	if strings.Contains(got, "render plan service 未配置") {
		t.Fatalf("missing reference should be rejected before service access: %s", got)
	}
}

func TestReadProjectContextSummarizesRenderPlansByTargetPhase(t *testing.T) {
	got := summarizeRenderPlans([]db.RenderPlan{
		{ID: uuidWithByte(11), ScopeType: "shot", ScopeID: uuidWithByte(21), TargetPhase: "preview_image", Operation: "text_to_image", Status: "succeeded"},
		{ID: uuidWithByte(12), ScopeType: "shot", ScopeID: uuidWithByte(22), TargetPhase: "shot_video", Operation: "image_to_video_first_frame", Status: "running"},
		{ID: uuidWithByte(13), ScopeType: "shot", ScopeID: uuidWithByte(23), TargetPhase: "shot_video", Operation: "image_to_video_first_frame", Status: "failed"},
	})
	for _, want := range []string{"3 个", "preview_image: succeeded=1", "shot_video: running=1 failed=1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q: %s", want, got)
		}
	}
}

func TestReadProjectContextRenderPlanSummaryUsesSemanticKeys(t *testing.T) {
	got := summarizeRenderPlans([]db.RenderPlan{{
		ID:          uuidWithByte(13),
		ScopeType:   "shot",
		ScopeID:     uuidWithByte(23),
		TargetPhase: "preview_image",
		Operation:   "text_to_image",
		Status:      "waiting_for_approval",
		SemanticKey: "shot_03.preview_image.rp1",
	}})
	if !strings.Contains(got, "shot_03.preview_image.rp1") {
		t.Fatalf("summary missing semantic key: %s", got)
	}
	if strings.Contains(got, "00000000-0000-0000-0000-00000000000d") || strings.Contains(got, "00000000-0000-0000-0000-000000000017") {
		t.Fatalf("summary leaked uuid: %s", got)
	}
}

func TestReadProjectContextAcceptsLenientScopeRefs(t *testing.T) {
	cases := []ReadProjectContextToolInput{
		{Brief: "读取全局上下文", ScopeRef: ToolObjectRef{Type: "shot"}},
		{Brief: "读取 brief 周边上下文", ScopeRef: ToolObjectRef{Type: "creative_brief", Key: "creative_brief.main"}},
		{Brief: "读取产物上下文", ScopeRef: ToolObjectRef{Type: "artifact_version", Key: "shot_01.preview_image.current"}},
	}
	for _, input := range cases {
		if err := validateReadProjectContextInput(input); err != nil {
			t.Fatalf("validateReadProjectContextInput(%#v) error = %v", input, err)
		}
	}
}

func TestProductionStateDecisionTextDoesNotExposeExecutableUUIDs(t *testing.T) {
	got := productionStateDecisionText(map[string]any{
		"shots": []any{map[string]any{
			"id":         "shot-uuid",
			"client_key": "shot_03",
			"preview_nodes": []any{map[string]any{
				"node_id":        "node-uuid",
				"version_id":     "version-uuid",
				"version_status": "succeeded",
			}},
			"shot_video_nodes": []any{map[string]any{
				"node_id":        "video-node-uuid",
				"version_id":     "video-version-uuid",
				"version_status": "succeeded",
			}},
		}},
	})
	for _, forbidden := range []string{"shot_id", "node_id", "version_id", "node-uuid", "version-uuid"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("production state leaked executable id %q: %s", forbidden, got)
		}
	}
}

func TestUpsertRenderPlanNormalizesMediaNodeTitleReference(t *testing.T) {
	input := UpsertRenderPlanToolInput{ReferenceBindings: []ReferenceBindingInput{{
		ClientKey:  "ref_product_luggage",
		SourceType: "media_node",
		SourceID:   "box.png",
		Role:       "product_reference",
	}}}
	got, msg, ok := normalizeMediaNodeReferenceBindings(input, []db.MediaNode{{
		ID:          uuidWithByte(9),
		WorkspaceID: uuidWithByte(1),
		Title:       "box.png",
		NodeType:    db.NodeTypeImage,
	}})
	if !ok {
		t.Fatalf("unexpected validation failure: %s", msg)
	}
	if got.ReferenceBindings[0].SourceID != "09000000-0000-0000-0000-000000000000" {
		t.Fatalf("source_id = %q", got.ReferenceBindings[0].SourceID)
	}
}

type fakeRenderPlanReferenceStore struct {
	nodes []db.MediaNode
}

func (f fakeRenderPlanReferenceStore) ListMediaNodesByWorkspace(context.Context, pgtype.UUID) ([]db.MediaNode, error) {
	return f.nodes, nil
}
