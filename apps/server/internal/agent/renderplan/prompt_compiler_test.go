package renderplan

import (
	"context"
	"encoding/json"
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

func TestCompileTemplateShotVideoPromptIncludesInternalProvider(t *testing.T) {
	compiler := NewPromptCompiler()
	out, err := compiler.Compile(context.Background(), UpsertInput{
		TargetPhase:        PhaseShotVideo,
		ModelPromptProfile: ProfileTemplateVideo,
		Operation:          "template_to_video",
		PromptParts: PromptParts{
			Objective:     "生成低成本模板视频：商品卖点卡片、产品图轻微推进、价格利益点、结尾 CTA。",
			Subject:       "银灰色硬壳行李箱。",
			TextRendering: "轻松登机｜现在出发",
		},
		Params: Params{DurationSec: 5, Ratio: "9:16", Resolution: "1080p"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.CompiledPrompt, "结尾 CTA") || !strings.Contains(out.CompiledPrompt, "轻松登机") {
		t.Fatalf("compiled prompt = %s", out.CompiledPrompt)
	}
	var request map[string]any
	if err := json.Unmarshal(out.CompiledRequest, &request); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"provider":  "internal_template_video",
		"model":     "hyperframes-html",
		"profile":   ProfileTemplateVideo,
		"operation": "template_to_video",
	} {
		if request[key] != want {
			t.Fatalf("request[%s] = %#v, want %q; request=%s", key, request[key], want, string(out.CompiledRequest))
		}
	}
}
