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
	for _, want := range []string{"model_prompt_profile", "reference_bindings", "prompt_parts"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("schema missing %s: %s", want, string(raw))
		}
	}
}

func TestUpsertRenderPlanToolReturnsNaturalValidationError(t *testing.T) {
	tool := NewUpsertRenderPlanNativeTool(nil)
	got, err := tool.InvokableRun(context.Background(), `{"mode":"create"}`)
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
		"scope":{"type":"shot","id":"04000000-0000-0000-0000-000000000000"},
		"target_phase":"preview_image",
		"task_type":"generate",
		"model_prompt_profile":"seedream_5_image",
		"operation":"image_to_image"
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
