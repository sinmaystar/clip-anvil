package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/creative"
)

func TestM1NativeToolInfosUseTypedSchemasAndChineseDescriptions(t *testing.T) {
	service := creative.NewService(nil)
	registry, err := NewNativeRegistry(
		NewReadProjectContextNativeTool(service),
		NewUpsertProjectBriefNativeTool(service),
		NewUpdateProjectMemoryNativeTool(service),
		NewUpsertKeyElementsNativeTool(service),
		NewUpsertStoryboardNativeTool(service),
	)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := registry.ToolInfos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 5 {
		t.Fatalf("infos len = %d", len(infos))
	}
	for _, info := range infos {
		if info.ParamsOneOf == nil {
			t.Fatalf("%s has nil params schema", info.Name)
		}
		if strings.TrimSpace(info.Desc) == "" || !strings.Contains(info.Desc, "ClipAnvil") {
			t.Fatalf("%s desc is too weak: %q", info.Name, info.Desc)
		}
	}
}

func TestM1NativeToolReturnsNaturalValidationError(t *testing.T) {
	tool := NewUpsertProjectBriefNativeTool(creative.NewService(nil))
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: pgtype.UUID{Valid: true},
	})
	got, err := tool.InvokableRun(ctx, `{"brief":"测试","mode":"rewrite"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "工具调用失败") || !strings.Contains(got, "mode") {
		t.Fatalf("unexpected result: %s", got)
	}
}

func TestUpsertStoryboardSkipsSelfDependenciesBeforeService(t *testing.T) {
	got, skipped := toCreativeShotDependencies([]ShotDependencyInput{
		{FromShotClientKey: "shot_01", ToShotClientKey: "shot_01", DependencyType: "same_product_consistency", Reason: "单分镜不需要自依赖"},
		{FromShotClientKey: "shot_01", ToShotClientKey: "shot_02", DependencyType: "same_product_consistency", Reason: "保持商品一致"},
	})
	if skipped != 1 {
		t.Fatalf("skipped = %d", skipped)
	}
	if len(got) != 1 || got[0].FromShotClientKey != "shot_01" || got[0].ToShotClientKey != "shot_02" {
		t.Fatalf("dependencies = %#v", got)
	}
}
