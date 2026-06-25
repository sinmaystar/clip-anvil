package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
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
