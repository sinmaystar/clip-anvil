package templatevideo

import (
	"strings"
	"testing"
)

func TestRenderStaticFallbackKenBurnsTemplateEscapesVariables(t *testing.T) {
	html, meta, err := Render(RenderInput{
		TemplateKey: "static_fallback_ken_burns_v1",
		DurationSec: 5,
		Ratio:       "9:16",
		Resolution:  "1080p",
		FPS:         24,
		Variables: map[string]any{
			"headline": `<script>alert("x")</script>`,
			"caption":  "轻装出发",
			"cta":      "现在了解",
		},
		Assets: []Asset{{
			ClientKey:     "product",
			WorkspacePath: "/workspace/input/template-video/job-1/product.png",
			Mime:          "image/png",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Width != 1080 || meta.Height != 1920 || meta.DurationFrames != 120 {
		t.Fatalf("meta = %#v", meta)
	}
	if strings.Contains(html, "<script>") || !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("html was not escaped: %s", html)
	}
	if !strings.Contains(html, `data-composition-id="static_fallback_ken_burns_v1"`) ||
		!strings.Contains(html, `data-duration="5"`) ||
		!strings.Contains(html, `data-fps="24"`) ||
		!strings.Contains(html, "totalDuration()") ||
		!strings.Contains(html, "window.__timelines") ||
		!strings.Contains(html, "/workspace/input/template-video/job-1/product.png") {
		t.Fatalf("html missing HyperFrames composition markers: %s", html)
	}
}

func TestRenderMarketingAdFourSceneTemplateUsesProductAndScenes(t *testing.T) {
	html, meta, err := Render(RenderInput{
		TemplateKey: "marketing_ad_4_scene_v1",
		DurationSec: 8,
		Ratio:       "9:16",
		Resolution:  "1080p",
		FPS:         24,
		Variables: map[string]any{
			"headline": "悦行行李箱",
			"cta":      "现在出发",
			"scenes": []any{
				map[string]any{"headline": "轻装短途出发", "caption": "周末和商务通勤，一箱刚刚好"},
				map[string]any{"headline": "顺滑万向轮", "caption": "转弯、进站、过安检都更省力"},
				map[string]any{"headline": "轻便硬壳", "caption": "好推好拿，旅途不被重量拖住"},
				map[string]any{"headline": "现在出发", "caption": "悦行行李箱，让短途更省心"},
			},
		},
		Assets: []Asset{{
			ClientKey:     "product",
			WorkspacePath: "assets/product.png",
			Mime:          "image/png",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.TemplateKey != "marketing_ad_4_scene_v1" || meta.DurationSec != 8 || meta.DurationFrames != 192 {
		t.Fatalf("meta = %#v", meta)
	}
	for _, want := range []string{
		`data-composition-id="marketing_ad_4_scene_v1"`,
		`src="assets/product.png"`,
		`data-scene-index="0"`,
		`data-scene-index="3"`,
		"轻装短途出发",
		"顺滑万向轮",
		"悦行行李箱",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q: %s", want, html)
		}
	}
}

func TestRenderMarketingAdFourSceneTemplateUsesExclusiveSceneWindows(t *testing.T) {
	html, _, err := Render(RenderInput{
		TemplateKey: "marketing_ad_4_scene_v1",
		DurationSec: 8,
		Ratio:       "9:16",
		Variables: map[string]any{
			"headline": "悦行行李箱",
		},
		Assets: []Asset{{
			ClientKey:     "product",
			WorkspacePath: "assets/product.png",
			Mime:          "image/png",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`animation: scene0`,
		`animation: scene1`,
		`animation: scene2`,
		`animation: scene3`,
		`@keyframes scene3`,
		`86%, 100% { opacity: 1; transform: translateY(0); }`,
		`copy-safe-panel`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing exclusive scene timing marker %q: %s", want, html)
		}
	}
	if strings.Contains(html, "animation-delay") {
		t.Fatalf("scene windows must not use delayed full-duration animations: %s", html)
	}
}

func TestRenderMarketingAdFourSceneTemplateRequiresProductImage(t *testing.T) {
	_, _, err := Render(RenderInput{
		TemplateKey: "marketing_ad_4_scene_v1",
		DurationSec: 8,
		Ratio:       "9:16",
		Variables:   map[string]any{"headline": "悦行行李箱"},
	})
	if err == nil || !strings.Contains(err.Error(), "product image") {
		t.Fatalf("error = %v", err)
	}
}

func TestRenderTemplateLibraryVariantsUseProductAndExposeCompositionID(t *testing.T) {
	variants := []string{
		"product_hero_v1",
		"benefit_cards_v1",
		"comparison_v1",
		"testimonial_v1",
		"price_cta_v1",
	}
	for _, key := range variants {
		t.Run(key, func(t *testing.T) {
			html, meta, err := Render(RenderInput{
				TemplateKey: key,
				DurationSec: 8,
				Ratio:       "9:16",
				Resolution:  "1080p",
				FPS:         24,
				Variables: map[string]any{
					"headline": "悦行行李箱",
					"caption":  "轻便好推，短途出行更省心",
					"cta":      "现在出发",
					"scenes": []any{
						map[string]any{"headline": "登机尺寸", "caption": "周末和商务通勤，一箱刚刚好"},
						map[string]any{"headline": "顺滑万向轮", "caption": "转弯、进站、过安检都更省力"},
						map[string]any{"headline": "轻便硬壳", "caption": "好推好拿，旅途不被重量拖住"},
					},
				},
				Assets: []Asset{{
					ClientKey:     "product",
					WorkspacePath: "assets/product.png",
					Mime:          "image/png",
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if meta.TemplateKey != key || meta.DurationSec != 8 || meta.DurationFrames != 192 {
				t.Fatalf("meta = %#v", meta)
			}
			for _, want := range []string{
				`data-composition-id="` + key + `"`,
				`src="assets/product.png"`,
				`data-template-variant="` + key + `"`,
				"悦行行李箱",
				"现在出发",
			} {
				if !strings.Contains(html, want) {
					t.Fatalf("html missing %q: %s", want, html)
				}
			}
		})
	}
}

func TestRenderTemplateLibraryVariantsRequireProductImage(t *testing.T) {
	for _, key := range []string{"product_hero_v1", "benefit_cards_v1", "comparison_v1", "testimonial_v1", "price_cta_v1"} {
		t.Run(key, func(t *testing.T) {
			_, _, err := Render(RenderInput{
				TemplateKey: key,
				DurationSec: 8,
				Ratio:       "9:16",
				Variables:   map[string]any{"headline": "悦行行李箱"},
			})
			if err == nil || !strings.Contains(err.Error(), "product image") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestRenderRejectsUnknownTemplateAndInvalidColor(t *testing.T) {
	_, _, err := Render(RenderInput{TemplateKey: "unknown", DurationSec: 5, Ratio: "9:16"})
	if err == nil || !strings.Contains(err.Error(), "unknown template_key") {
		t.Fatalf("error = %v", err)
	}

	_, _, err = Render(RenderInput{
		TemplateKey: "static_fallback_ken_burns_v1",
		DurationSec: 5,
		Ratio:       "9:16",
		Variables: map[string]any{
			"brand_colors": []any{"red;display:none"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "brand_colors") {
		t.Fatalf("error = %v", err)
	}
}
