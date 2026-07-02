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
