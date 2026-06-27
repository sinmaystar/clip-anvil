package renderplan

import (
	"context"
	"strings"
	"testing"
)

func TestCompileSeedreamReferenceImagePrompt(t *testing.T) {
	compiler := NewPromptCompiler()
	out, err := compiler.Compile(context.Background(), UpsertInput{
		TargetPhase:        PhaseReferenceImage,
		ModelPromptProfile: ProfileSeedream5Image,
		Operation:          "text_to_image",
		PromptParts: PromptParts{
			Objective:      "生成现代机场出发大厅参考图。",
			Setting:        "清晨自然光，玻璃幕墙，干净开阔。",
			Composition:    "9:16 中景，留出人物拉行李箱路径。",
			Style:          "真实商业广告质感。",
			ConstraintPack: []string{"不要出现竞品 Logo"},
		},
		Params: Params{Ratio: "9:16", Resolution: "2K", MaxImages: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"现代机场出发大厅", "真实商业广告质感", "不要出现竞品 Logo"} {
		if !strings.Contains(out.CompiledPrompt, want) {
			t.Fatalf("compiled prompt missing %q: %s", want, out.CompiledPrompt)
		}
	}
}

func TestCompileSeedanceShotVideoRequiresActionOrSequence(t *testing.T) {
	compiler := NewPromptCompiler()
	_, err := compiler.Compile(context.Background(), UpsertInput{
		TargetPhase:        PhaseShotVideo,
		ModelPromptProfile: ProfileSeedance2Video,
		Operation:          "image_to_video_first_frame",
		PromptParts:        PromptParts{Objective: "生成分镜视频。"},
		Params:             Params{DurationSec: 6, Ratio: "9:16"},
	})
	if err == nil || !strings.Contains(err.Error(), "action 或 sequence") {
		t.Fatalf("error = %v, want action/sequence validation", err)
	}
}
