package remotiontimeline

import (
	"strings"
	"testing"
)

func TestValidateAcceptsFixturePlan(t *testing.T) {
	plan := minimalValidPlan()
	if err := Validate(plan); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDecodeAcceptsMapPlan(t *testing.T) {
	plan, err := Decode(map[string]any{
		"schema":      SchemaV1,
		"composition": CompositionMarketingTimeline,
		"output": map[string]any{
			"width":        float64(1080),
			"height":       float64(1920),
			"fps":          float64(30),
			"duration_sec": float64(10),
		},
		"segments": []any{map[string]any{
			"id":        "seg-1",
			"start_sec": float64(0),
			"end_sec":   float64(10),
			"layout":    "hero_packshot",
			"assets": []any{map[string]any{
				"role":           "primary",
				"type":           "image",
				"workspace_path": "/workspace/input/product.png",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if err := Validate(plan); err != nil {
		t.Fatalf("Validate(decoded) error = %v", err)
	}
}

func TestDecodeAcceptsNumericMotionIntensityAndCaptionAlias(t *testing.T) {
	plan, err := Decode(map[string]any{
		"schema":      SchemaV1,
		"composition": CompositionMarketingTimeline,
		"output": map[string]any{
			"width":        float64(1080),
			"height":       float64(1920),
			"fps":          float64(30),
			"duration_sec": float64(10),
		},
		"segments": []any{map[string]any{
			"id":        "seg-1",
			"start_sec": float64(0),
			"end_sec":   float64(10),
			"layout":    "hero_packshot",
			"assets": []any{map[string]any{
				"role":           "primary",
				"type":           "image",
				"workspace_path": "/workspace/input/product.png",
			}},
			"motion": map[string]any{
				"preset":    "push_in",
				"intensity": float64(0.55),
			},
			"captions": []any{map[string]any{
				"source":    "audio_cue",
				"text":      "顺滑万向轮",
				"start_sec": float64(0),
				"end_sec":   float64(10),
				"position":  "subtitle_bottom",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if err := Validate(plan); err != nil {
		t.Fatalf("Validate(decoded) error = %v", err)
	}
	if got, ok := plan.Segments[0].Motion.Intensity.(float64); !ok || got != 0.55 {
		t.Fatalf("numeric intensity was not preserved: %#v", plan.Segments[0].Motion.Intensity)
	}
	if plan.Segments[0].Caption.Text != "顺滑万向轮" {
		t.Fatalf("caption alias not normalized: %#v", plan.Segments[0].Caption)
	}
}

func TestValidateRejectsBadWorkspacePath(t *testing.T) {
	plan := minimalValidPlan()
	plan.Segments[0].Assets[0].WorkspacePath = "/tmp/product.png"
	if err := Validate(plan); err == nil {
		t.Fatalf("expected bad workspace path to be rejected")
	}
}

func TestValidateAcceptsImageAndVideoAssetsOnly(t *testing.T) {
	plan := minimalValidPlan()
	plan.Segments[0].Assets[0].Type = "video"
	plan.Segments[0].Assets[0].WorkspacePath = "/workspace/input/hero.mp4"
	if err := Validate(plan); err != nil {
		t.Fatalf("Validate(video asset) error = %v", err)
	}

	plan = minimalValidPlan()
	plan.Segments[0].Assets[0].Type = "lottie"
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "assets[0].type") {
		t.Fatalf("Validate invalid asset type error = %v, want assets[0].type error", err)
	}
}

func TestValidateRejectsInvalidTiming(t *testing.T) {
	plan := minimalValidPlan()
	plan.Segments[0].EndSec = plan.Segments[0].StartSec
	if err := Validate(plan); err == nil {
		t.Fatalf("expected invalid timing to be rejected")
	}
}

func TestValidateRejectsMissingSegments(t *testing.T) {
	plan := minimalValidPlan()
	plan.Segments = nil
	if err := Validate(plan); err == nil {
		t.Fatalf("expected missing segments to be rejected")
	}
}

func TestValidateRejectsBadAudioRole(t *testing.T) {
	plan := minimalValidPlan()
	plan.AudioTracks = []AudioTrack{{Role: "dialogue", WorkspacePath: "/workspace/input/dialogue.mp3"}}
	if err := Validate(plan); err == nil {
		t.Fatalf("expected bad audio role to be rejected")
	}
}

func TestValidateRejectsUnknownLayoutMotionAndTransition(t *testing.T) {
	plan := validMarketingPlanForTest()
	plan.Segments[0].Layout = "freeform_layout"
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "layout") {
		t.Fatalf("Validate unknown layout error = %v, want layout error", err)
	}

	plan = validMarketingPlanForTest()
	plan.Segments[0].Motion.Preset = "spin_forever"
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "motion.preset") {
		t.Fatalf("Validate unknown motion error = %v, want motion.preset error", err)
	}

	plan = validMarketingPlanForTest()
	plan.Segments[0].TransitionIn.Type = "flashbang"
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "transition_in.type") {
		t.Fatalf("Validate unknown transition error = %v, want transition_in.type error", err)
	}
}

func TestValidateRejectsInternalCaptionSourcesAndText(t *testing.T) {
	plan := validMarketingPlanForTest()
	plan.Captions.SingleLane = true
	plan.Segments[0].Caption = Caption{
		Source:   "visual_intent",
		Text:     "前三秒抓住短途出行用户注意",
		StartSec: 0,
		EndSec:   4,
		Position: "subtitle_bottom",
	}
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "caption.source") {
		t.Fatalf("Validate internal caption source error = %v, want caption.source error", err)
	}

	plan = validMarketingPlanForTest()
	plan.Segments[0].Caption.Text = "短途出行痛点钩子"
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "caption.text") {
		t.Fatalf("Validate internal caption text error = %v, want caption.text error", err)
	}
}

func TestValidateRejectsRepeatedVisualsLayoutsAndCueMismatch(t *testing.T) {
	plan := validMarketingPlanForTest()
	plan.Segments = []Segment{
		testSegment("shot_01", 0, 4, "hero_packshot", "短途出行", "/workspace/input/hero.png"),
		testSegment("shot_02", 4, 8, "hero_packshot", "顺滑万向轮", "/workspace/input/hero.png"),
		testSegment("shot_03", 8, 12, "hero_packshot", "大周出行收纳", "/workspace/input/hero.png"),
	}
	plan.Output.DurationSec = 12
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "repeated visual") {
		t.Fatalf("Validate repeated visual error = %v, want repeated visual error", err)
	}

	plan = validMarketingPlanForTest()
	plan.Segments = []Segment{
		testSegment("shot_01", 0, 4, "benefit_card", "轻便好推", "/workspace/input/hero.png"),
		testSegment("shot_02", 4, 8, "benefit_card", "顺滑万向轮", "/workspace/input/wheel.png"),
		testSegment("shot_03", 8, 12, "benefit_card", "收纳分区", "/workspace/input/storage.png"),
	}
	plan.Output.DurationSec = 12
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "repeated layout") {
		t.Fatalf("Validate repeated layout error = %v, want repeated layout error", err)
	}

	plan = validMarketingPlanForTest()
	plan.Segments[0] = testSegment("shot_02", 0, 4, "detail_focus", "顺滑万向轮", "/workspace/input/open-storage.png")
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "cue/asset mismatch") {
		t.Fatalf("Validate mismatch error = %v, want cue/asset mismatch error", err)
	}
}

func minimalValidPlan() Plan {
	return Plan{
		Schema:      SchemaV1,
		Composition: CompositionMarketingTimeline,
		Output: Output{
			Width:       1080,
			Height:      1920,
			FPS:         30,
			DurationSec: 10,
			Codec:       "h264",
			AudioCodec:  "aac",
		},
		Segments: []Segment{{
			ID:       "seg-1",
			ShotRef:  "shot_01",
			StartSec: 0,
			EndSec:   10,
			Layout:   "hero_packshot",
			Assets: []Asset{{
				Role:          "primary",
				Type:          "image",
				WorkspacePath: "/workspace/input/product.png",
			}},
			Caption: Caption{
				Source:   "audio_cue",
				Text:     "轻松出发",
				StartSec: 0,
				EndSec:   10,
				Position: "subtitle_bottom",
			},
		}},
		AudioTracks: []AudioTrack{{
			ID:            "voiceover",
			Role:          "voiceover",
			WorkspacePath: "/workspace/input/voiceover.mp3",
			Volume:        1,
		}},
	}
}

func validMarketingPlanForTest() Plan {
	return minimalValidPlan()
}

func testSegment(shotRef string, start, end float64, layout, caption, workspacePath string) Segment {
	return Segment{
		ID:          shotRef,
		ShotRef:     shotRef,
		StartSec:    start,
		EndSec:      end,
		Layout:      layout,
		VisualFocus: caption,
		Assets: []Asset{{
			Role:          "primary",
			Type:          "image",
			WorkspacePath: workspacePath,
		}},
		Motion: Motion{Preset: "push_in", Intensity: 0.55},
		Caption: Caption{
			Source:   "audio_cue",
			Text:     caption,
			StartSec: start,
			EndSec:   end,
			Position: "subtitle_bottom",
		},
		TransitionIn: Transition{Type: "crossfade", DurationSec: 0.25},
	}
}
