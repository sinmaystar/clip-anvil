package motionshot

import (
	"fmt"
	"strings"
)

type RenderInput struct {
	DurationSec int
	Ratio       string
	Resolution  string
	FPS         int
	Assets      []Asset
	Params      map[string]any
}

type Asset struct {
	AssetID       string `json:"asset_id,omitempty"`
	WorkspacePath string `json:"workspace_path"`
	Mime          string `json:"mime,omitempty"`
}

type Plan struct {
	DurationSec    int            `json:"duration_sec"`
	Ratio          string         `json:"ratio"`
	Resolution     string         `json:"resolution"`
	FPS            int            `json:"fps"`
	Width          int            `json:"width"`
	Height         int            `json:"height"`
	DurationFrames int            `json:"duration_frames"`
	MotionStyle    string         `json:"motion_style"`
	SafeArea       string         `json:"safe_area"`
	VisualLayers   []VisualLayer  `json:"visual_layers"`
	TextLayers     []TextLayer    `json:"text_layers,omitempty"`
	Transitions    map[string]any `json:"transitions,omitempty"`
	BrandColors    []string       `json:"brand_colors,omitempty"`
}

type VisualLayer struct {
	Role     string  `json:"role"`
	InputRef string  `json:"input_ref"`
	Fit      string  `json:"fit"`
	Motion   string  `json:"motion"`
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
}

type TextLayer struct {
	Role      string  `json:"role"`
	Text      string  `json:"text"`
	StartSec  float64 `json:"start_sec"`
	EndSec    float64 `json:"end_sec"`
	Animation string  `json:"animation"`
	Position  string  `json:"position"`
}

func Normalize(input RenderInput) (Plan, error) {
	if len(input.Assets) == 0 || strings.TrimSpace(input.Assets[0].WorkspacePath) == "" {
		return Plan{}, fmt.Errorf("motion shot requires at least one image")
	}
	duration := input.DurationSec
	if duration == 0 {
		duration = 5
	}
	if !supportedInt(duration, 3, 4, 5, 6, 8) {
		return Plan{}, fmt.Errorf("duration_sec %d is not supported", duration)
	}
	fps := input.FPS
	if fps == 0 {
		fps = 30
	}
	if !supportedInt(fps, 24, 30) {
		return Plan{}, fmt.Errorf("fps %d is not supported", fps)
	}
	ratio := firstNonEmptyString(input.Ratio, "9:16")
	resolution := firstNonEmptyString(input.Resolution, "1080p")
	width, height, ok := dimensions(ratio, resolution)
	if !ok {
		return Plan{}, fmt.Errorf("ratio %q with resolution %q is not supported", ratio, resolution)
	}
	motionStyle := stringParam(input.Params, "motion_style", "premium_product_ad")
	safeArea := stringParam(input.Params, "safe_area", "caption_safe_bottom")
	return Plan{
		DurationSec:    duration,
		Ratio:          ratio,
		Resolution:     resolution,
		FPS:            fps,
		Width:          width,
		Height:         height,
		DurationFrames: duration * fps,
		MotionStyle:    motionStyle,
		SafeArea:       safeArea,
		VisualLayers:   visualLayers(input.Params, input.Assets, duration),
		TextLayers:     textLayers(input.Params, duration),
		Transitions:    mapParam(input.Params, "transitions"),
		BrandColors:    stringSliceParam(input.Params, "brand_colors"),
	}, nil
}

func supportedInt(value int, allowed ...int) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func dimensions(ratio string, resolution string) (int, int, bool) {
	longSide := 1080
	if resolution == "720p" {
		longSide = 720
	} else if resolution != "1080p" {
		return 0, 0, false
	}
	switch ratio {
	case "9:16":
		return longSide, longSide * 16 / 9, true
	case "16:9":
		return longSide * 16 / 9, longSide, true
	case "1:1":
		return longSide, longSide, true
	default:
		return 0, 0, false
	}
}

func textLayers(params map[string]any, duration int) []TextLayer {
	layers := []TextLayer{{
		Role:      "hook",
		Text:      stringParam(params, "headline", "轻松出发"),
		StartSec:  0.2,
		EndSec:    minFloat(float64(duration), 2.4),
		Animation: "pop_slide_up",
		Position:  "upper_third",
	}}
	if raw, ok := params["text_layers"].([]any); ok && len(raw) > 0 {
		out := make([]TextLayer, 0, len(raw))
		for _, item := range raw {
			values, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text := strings.TrimSpace(stringParam(values, "text", ""))
			if text == "" {
				continue
			}
			out = append(out, TextLayer{
				Role:      stringParam(values, "role", "copy"),
				Text:      text,
				StartSec:  clampFloat(floatParam(values, "start_sec", 0), 0, float64(duration)),
				EndSec:    clampFloat(floatParam(values, "end_sec", float64(duration)), 0, float64(duration)),
				Animation: stringParam(values, "animation", "fade_rise"),
				Position:  stringParam(values, "position", "middle_safe"),
			})
		}
		if len(out) > 0 {
			return out
		}
	}
	return layers
}

func visualLayers(params map[string]any, assets []Asset, duration int) []VisualLayer {
	if raw, ok := params["visual_layers"].([]any); ok && len(raw) > 0 {
		out := make([]VisualLayer, 0, len(raw))
		for _, item := range raw {
			values, ok := item.(map[string]any)
			if !ok {
				continue
			}
			inputRef := stringParam(values, "input_ref", "")
			if inputRef == "" || inputRef == "primary_image" {
				inputRef = strings.TrimSpace(assets[0].WorkspacePath)
			}
			out = append(out, VisualLayer{
				Role:     stringParam(values, "role", "product"),
				InputRef: inputRef,
				Fit:      stringParam(values, "fit", "contain"),
				Motion:   stringParam(values, "motion", "slow_push_in"),
				StartSec: clampFloat(floatParam(values, "start_sec", 0), 0, float64(duration)),
				EndSec:   clampFloat(floatParam(values, "end_sec", float64(duration)), 0, float64(duration)),
			})
		}
		if len(out) > 0 {
			return out
		}
	}
	return []VisualLayer{{
		Role:     "product",
		InputRef: strings.TrimSpace(assets[0].WorkspacePath),
		Fit:      "contain",
		Motion:   "slow_push_in",
		StartSec: 0,
		EndSec:   float64(duration),
	}}
}

func mapParam(params map[string]any, key string) map[string]any {
	if params == nil {
		return nil
	}
	value, ok := params[key].(map[string]any)
	if !ok {
		return nil
	}
	return value
}

func stringParam(params map[string]any, key string, fallback string) string {
	if params == nil {
		return fallback
	}
	value, ok := params[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func stringSliceParam(params map[string]any, key string) []string {
	if params == nil {
		return nil
	}
	switch values := params[key].(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, item := range values {
			if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
				out = append(out, strings.TrimSpace(value))
			}
		}
		return out
	default:
		return nil
	}
}

func floatParam(params map[string]any, key string, fallback float64) float64 {
	if params == nil {
		return fallback
	}
	switch value := params[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	default:
		return fallback
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func clampFloat(value float64, min float64, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
