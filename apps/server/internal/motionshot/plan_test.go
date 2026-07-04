package motionshot

import (
	"strings"
	"testing"
)

func TestNormalizeRequiresImageForMotionShot(t *testing.T) {
	_, err := Normalize(RenderInput{DurationSec: 5, Ratio: "9:16", Resolution: "1080p", FPS: 30})
	if err == nil || !strings.Contains(err.Error(), "requires at least one image") {
		t.Fatalf("expected missing image error, got %v", err)
	}
}

func TestNormalizeClampsSupportedMotionMeta(t *testing.T) {
	plan, err := Normalize(RenderInput{
		DurationSec: 5,
		Ratio:       "9:16",
		Resolution:  "1080p",
		FPS:         30,
		Assets:      []Asset{{WorkspacePath: "assets/product.png"}},
		Params:      map[string]any{"motion_style": "premium_product_ad"},
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if plan.Width != 1080 || plan.Height != 1920 || plan.DurationFrames != 150 {
		t.Fatalf("unexpected meta: %+v", plan)
	}
	if plan.MotionStyle != "premium_product_ad" || plan.VisualLayers[0].InputRef != "assets/product.png" {
		t.Fatalf("unexpected motion plan: %+v", plan)
	}
	if len(plan.TextLayers) != 0 {
		t.Fatalf("default motion shot should not invent screen text: %#v", plan.TextLayers)
	}
}

func TestNormalizeRejectsUnsupportedRatio(t *testing.T) {
	_, err := Normalize(RenderInput{
		DurationSec: 5,
		Ratio:       "4:5",
		Resolution:  "1080p",
		FPS:         30,
		Assets:      []Asset{{WorkspacePath: "assets/product.png"}},
	})
	if err == nil || !strings.Contains(err.Error(), "ratio") {
		t.Fatalf("expected ratio error, got %v", err)
	}
}

func TestNormalizeExplicitEmptyTextLayersDisablesScreenText(t *testing.T) {
	plan, err := Normalize(RenderInput{
		DurationSec: 5,
		Ratio:       "9:16",
		Resolution:  "1080p",
		FPS:         30,
		Assets:      []Asset{{WorkspacePath: "assets/product.png"}},
		Params:      map[string]any{"text_layers": []any{}},
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(plan.TextLayers) != 0 {
		t.Fatalf("explicit empty text_layers should stay empty: %#v", plan.TextLayers)
	}
}

func TestNormalizeUsesExplicitHeadlineAsScreenText(t *testing.T) {
	plan, err := Normalize(RenderInput{
		DurationSec: 5,
		Ratio:       "9:16",
		Resolution:  "1080p",
		FPS:         30,
		Assets:      []Asset{{WorkspacePath: "assets/product.png"}},
		Params:      map[string]any{"headline": "轻装出发"},
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(plan.TextLayers) != 1 || plan.TextLayers[0].Text != "轻装出发" {
		t.Fatalf("headline text layer not applied: %#v", plan.TextLayers)
	}
}

func TestNormalizeUsesExplicitVisualLayersAndClipsTextToDuration(t *testing.T) {
	plan, err := Normalize(RenderInput{
		DurationSec: 6,
		Ratio:       "9:16",
		Resolution:  "1080p",
		FPS:         30,
		Assets:      []Asset{{WorkspacePath: "assets/product.png"}},
		Params: map[string]any{
			"visual_layers": []any{
				map[string]any{"role": "background", "input_ref": "assets/bg.png", "fit": "cover", "motion": "slow_pan_left", "start_sec": float64(0), "end_sec": float64(6)},
				map[string]any{"role": "product", "input_ref": "primary_image", "fit": "contain", "motion": "float_up", "start_sec": float64(0.4), "end_sec": float64(6)},
			},
			"text_layers": []any{
				map[string]any{"role": "hook", "text": "轻装出发", "start_sec": float64(0.2), "end_sec": float64(9), "animation": "pop_slide_up", "position": "upper_third"},
			},
		},
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(plan.VisualLayers) != 2 {
		t.Fatalf("visual layers = %#v", plan.VisualLayers)
	}
	if plan.VisualLayers[0].Role != "background" || plan.VisualLayers[1].Motion != "float_up" {
		t.Fatalf("visual layers not preserved: %#v", plan.VisualLayers)
	}
	if len(plan.TextLayers) != 1 || plan.TextLayers[0].EndSec != 6 {
		t.Fatalf("text layer not clipped: %#v", plan.TextLayers)
	}
}

func TestNormalizeRejectsLongSingleMotionShotDuration(t *testing.T) {
	_, err := Normalize(RenderInput{
		DurationSec: 30,
		Ratio:       "9:16",
		Resolution:  "1080p",
		FPS:         30,
		Assets:      []Asset{{WorkspacePath: "assets/product.png"}},
	})
	if err == nil || !strings.Contains(err.Error(), "duration_sec 30 is not supported") {
		t.Fatalf("expected duration error, got %v", err)
	}
}
