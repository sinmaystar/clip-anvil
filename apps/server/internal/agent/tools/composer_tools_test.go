package tools

import (
	"context"
	"testing"
)

func TestComposerNativeToolsRegisterExpectedNames(t *testing.T) {
	registry, err := NewNativeRegistry(
		NewGetCompositionContextNativeTool(nil),
		NewStageMediaInputsNativeTool(nil),
		NewProbeMediaNativeTool(nil),
		NewCreateTimelinePlanNativeTool(nil),
		NewUpdateTimelinePlanStatusNativeTool(nil),
		NewRenderTimelineTemplateNativeTool(nil),
		NewRunFFmpegCommandNativeTool(nil),
		NewSubmitCompositionArtifactNativeTool(nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := registry.ToolInfos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, info := range infos {
		if info.ParamsOneOf == nil {
			t.Fatalf("%s ParamsOneOf is nil", info.Name)
		}
		got[info.Name] = true
	}
	for _, want := range []string{
		"get_composition_context",
		"stage_media_inputs",
		"probe_media",
		"create_timeline_plan",
		"update_timeline_plan_status",
		"render_timeline_template",
		"run_ffmpeg_command",
		"submit_composition_artifact",
	} {
		if !got[want] {
			t.Fatalf("missing composer native tool %q in %#v", want, got)
		}
	}
}
