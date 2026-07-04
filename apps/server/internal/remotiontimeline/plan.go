package remotiontimeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

const (
	SchemaV1                     = "clipanvil.remotion_timeline.v1"
	CompositionMarketingTimeline = "MarketingTimeline"
	TemplateKeyV1                = "remotion_timeline_v1"
)

var allowedLayouts = map[string]bool{
	"hero_packshot": true,
	"detail_focus":  true,
	"benefit_card":  true,
	"split_compare": true,
	"scenario_card": true,
	"open_storage":  true,
	"cta_endcard":   true,
}

var allowedMotionPresets = map[string]bool{
	"":                 true,
	"push_in":          true,
	"pull_out":         true,
	"pan_left":         true,
	"pan_right":        true,
	"float_parallax":   true,
	"spotlight_reveal": true,
	"kinetic_text":     true,
	"cta_pop":          true,
}

var allowedTransitions = map[string]bool{
	"":          true,
	"cut":       true,
	"crossfade": true,
	"slide":     true,
	"wipe":      true,
	"zoom_blur": true,
}

var allowedCaptionSources = map[string]bool{
	"":                    true,
	"audio_cue":           true,
	"voiceover_alignment": true,
	"tts_alignment":       true,
	"manual_caption":      true,
}

var allowedAssetTypes = map[string]bool{
	"image": true,
	"video": true,
}

var forbiddenCaptionPhrases = []string{
	"短途出行痛点钩子",
	"前三秒抓住",
	"视觉意图",
	"导演",
	"narrative_purpose",
	"visual_intent",
	"action_text",
	"camera_intent",
	"director_note",
	"internal_note",
	"storyboard_note",
}

type Plan struct {
	Schema      string       `json:"schema"`
	Composition string       `json:"composition"`
	Output      Output       `json:"output"`
	Theme       Theme        `json:"theme,omitempty"`
	Segments    []Segment    `json:"segments"`
	AudioTracks []AudioTrack `json:"audio_tracks,omitempty"`
	Captions    Captions     `json:"captions,omitempty"`
}

type Output struct {
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FPS         int     `json:"fps"`
	DurationSec float64 `json:"duration_sec"`
	Codec       string  `json:"codec,omitempty"`
	AudioCodec  string  `json:"audio_codec,omitempty"`
}

type Theme struct {
	BrandColors []string `json:"brand_colors,omitempty"`
	FontFamily  string   `json:"font_family,omitempty"`
	Style       string   `json:"style,omitempty"`
}

type Segment struct {
	ID            string      `json:"id"`
	ShotRef       string      `json:"shot_ref,omitempty"`
	StartSec      float64     `json:"start_sec"`
	EndSec        float64     `json:"end_sec"`
	Layout        string      `json:"layout"`
	VisualFocus   string      `json:"visual_focus,omitempty"`
	Assets        []Asset     `json:"assets"`
	Motion        Motion      `json:"motion,omitempty"`
	TextLayers    []TextLayer `json:"text_layers,omitempty"`
	Caption       Caption     `json:"caption,omitempty"`
	TransitionIn  Transition  `json:"transition_in,omitempty"`
	TransitionOut Transition  `json:"transition_out,omitempty"`
}

func (s *Segment) UnmarshalJSON(data []byte) error {
	type segmentAlias Segment
	var decoded struct {
		segmentAlias
		Captions []Caption `json:"captions,omitempty"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = Segment(decoded.segmentAlias)
	if strings.TrimSpace(s.Caption.Text) == "" && len(decoded.Captions) > 0 {
		s.Caption = decoded.Captions[0]
	}
	return nil
}

type Asset struct {
	Role          string `json:"role"`
	NodeRef       string `json:"node_ref,omitempty"`
	WorkspacePath string `json:"workspace_path"`
	Type          string `json:"type"`
}

type Motion struct {
	Preset    string `json:"preset,omitempty"`
	Intensity any    `json:"intensity,omitempty"`
	Direction string `json:"direction,omitempty"`
}

type TextLayer struct {
	Role      string  `json:"role,omitempty"`
	Text      string  `json:"text"`
	StartSec  float64 `json:"start_sec"`
	EndSec    float64 `json:"end_sec"`
	Position  string  `json:"position,omitempty"`
	Animation string  `json:"animation,omitempty"`
}

type Caption struct {
	Source   string  `json:"source,omitempty"`
	Text     string  `json:"text,omitempty"`
	StartSec float64 `json:"start_sec,omitempty"`
	EndSec   float64 `json:"end_sec,omitempty"`
	Position string  `json:"position,omitempty"`
}

type Transition struct {
	Type        string  `json:"type,omitempty"`
	DurationSec float64 `json:"duration_sec,omitempty"`
}

type AudioTrack struct {
	ID            string  `json:"id,omitempty"`
	Role          string  `json:"role"`
	NodeRef       string  `json:"node_ref,omitempty"`
	WorkspacePath string  `json:"workspace_path"`
	StartSec      float64 `json:"start_sec,omitempty"`
	Volume        float64 `json:"volume,omitempty"`
	FadeInSec     float64 `json:"fade_in_sec,omitempty"`
	FadeOutSec    float64 `json:"fade_out_sec,omitempty"`
	Loop          bool    `json:"loop,omitempty"`
}

type Captions struct {
	Source          string `json:"source,omitempty"`
	SingleLane      bool   `json:"single_lane,omitempty"`
	MaxCharsPerLine int    `json:"max_chars_per_line,omitempty"`
	Style           string `json:"style,omitempty"`
}

func Decode(raw map[string]any) (Plan, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return Plan{}, err
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func Validate(plan Plan) error {
	if strings.TrimSpace(plan.Schema) != SchemaV1 {
		return fmt.Errorf("schema must be %s", SchemaV1)
	}
	if strings.TrimSpace(plan.Composition) != CompositionMarketingTimeline {
		return fmt.Errorf("composition must be %s", CompositionMarketingTimeline)
	}
	if plan.Output.Width <= 0 || plan.Output.Height <= 0 || plan.Output.FPS <= 0 || plan.Output.DurationSec <= 0 {
		return errors.New("output width, height, fps and duration_sec are required")
	}
	if len(plan.Segments) == 0 {
		return errors.New("segments must contain at least one item")
	}
	for i, segment := range plan.Segments {
		if strings.TrimSpace(segment.ID) == "" {
			return fmt.Errorf("segments[%d].id is required", i)
		}
		if segment.StartSec < 0 || segment.EndSec <= segment.StartSec || segment.EndSec > plan.Output.DurationSec+0.001 {
			return fmt.Errorf("segments[%d] has invalid timing", i)
		}
		if strings.TrimSpace(segment.Layout) == "" {
			return fmt.Errorf("segments[%d].layout is required", i)
		}
		if err := validateSegmentVocabulary(i, segment); err != nil {
			return err
		}
		if len(segment.Assets) == 0 {
			return fmt.Errorf("segments[%d].assets must contain at least one item", i)
		}
		if err := validateMotionIntensity(segment.Motion.Intensity); err != nil {
			return fmt.Errorf("segments[%d].motion.intensity: %w", i, err)
		}
		for j, asset := range segment.Assets {
			assetType := strings.TrimSpace(asset.Type)
			if !allowedAssetTypes[assetType] {
				return fmt.Errorf("segments[%d].assets[%d].type %q is not supported", i, j, assetType)
			}
			if err := validateWorkspacePath(asset.WorkspacePath); err != nil {
				return fmt.Errorf("segments[%d].assets[%d].workspace_path: %w", i, j, err)
			}
		}
		if err := validateCaption(i, segment); err != nil {
			return err
		}
		if err := validateCueAssetMatch(i, segment); err != nil {
			return err
		}
	}
	if err := validateRepeatedVisualsAndLayouts(plan.Segments); err != nil {
		return err
	}
	for i, track := range plan.AudioTracks {
		if track.Role != "voiceover" && track.Role != "bgm" {
			return fmt.Errorf("audio_tracks[%d].role must be voiceover or bgm", i)
		}
		if err := validateWorkspacePath(track.WorkspacePath); err != nil {
			return fmt.Errorf("audio_tracks[%d].workspace_path: %w", i, err)
		}
	}
	return nil
}

func validateSegmentVocabulary(index int, segment Segment) error {
	layout := strings.TrimSpace(segment.Layout)
	if !allowedLayouts[layout] {
		return fmt.Errorf("segments[%d].layout %q is not supported", index, layout)
	}
	motionPreset := strings.TrimSpace(segment.Motion.Preset)
	if !allowedMotionPresets[motionPreset] {
		return fmt.Errorf("segments[%d].motion.preset %q is not supported", index, motionPreset)
	}
	if err := validateTransitionType(index, "transition_in", segment.TransitionIn.Type); err != nil {
		return err
	}
	if err := validateTransitionType(index, "transition_out", segment.TransitionOut.Type); err != nil {
		return err
	}
	return nil
}

func validateTransitionType(index int, field string, value string) error {
	transitionType := strings.TrimSpace(value)
	if allowedTransitions[transitionType] {
		return nil
	}
	return fmt.Errorf("segments[%d].%s %q is not supported", index, field+".type", transitionType)
}

func validateCaption(index int, segment Segment) error {
	if strings.TrimSpace(segment.Caption.Text) == "" && strings.TrimSpace(segment.Caption.Source) == "" {
		return nil
	}
	source := strings.TrimSpace(segment.Caption.Source)
	if !allowedCaptionSources[source] {
		return fmt.Errorf("segments[%d].caption.source %q is not supported", index, source)
	}
	text := strings.ToLower(strings.TrimSpace(segment.Caption.Text))
	for _, phrase := range forbiddenCaptionPhrases {
		if strings.Contains(text, strings.ToLower(phrase)) {
			return fmt.Errorf("segments[%d].caption.text contains internal planning phrase %q", index, phrase)
		}
	}
	if segment.Caption.EndSec > 0 && segment.Caption.EndSec < segment.Caption.StartSec {
		return fmt.Errorf("segments[%d].caption has invalid timing", index)
	}
	return nil
}

func validateRepeatedVisualsAndLayouts(segments []Segment) error {
	if len(segments) < 3 {
		return nil
	}
	visualCounts := map[string]int{}
	for _, segment := range segments {
		if len(segment.Assets) == 0 {
			continue
		}
		path := strings.TrimSpace(segment.Assets[0].WorkspacePath)
		if path != "" {
			visualCounts[path]++
		}
	}
	for workspacePath, count := range visualCounts {
		if count > len(segments)/2 {
			return fmt.Errorf("repeated visual %q is used by %d of %d segments", workspacePath, count, len(segments))
		}
	}
	runLayout := ""
	runCount := 0
	for _, segment := range segments {
		layout := strings.TrimSpace(segment.Layout)
		if layout == runLayout {
			runCount++
		} else {
			runLayout = layout
			runCount = 1
		}
		if layout != "" && runCount > 2 {
			return fmt.Errorf("repeated layout %q appears in %d consecutive segments", layout, runCount)
		}
	}
	return nil
}

func validateCueAssetMatch(index int, segment Segment) error {
	cueText := strings.ToLower(strings.Join([]string{segment.VisualFocus, segment.Caption.Text, segment.ShotRef, segment.ID}, " "))
	assetTextParts := make([]string, 0, len(segment.Assets)*3)
	for _, asset := range segment.Assets {
		assetTextParts = append(assetTextParts, asset.WorkspacePath, asset.NodeRef, asset.Role)
	}
	assetText := strings.ToLower(strings.Join(assetTextParts, " "))
	cueFocus := semanticFocus(cueText)
	assetFocus := semanticFocus(assetText)
	if cueFocus == "" || assetFocus == "" || cueFocus == assetFocus {
		return nil
	}
	if (cueFocus == "wheel" && assetFocus == "storage") || (cueFocus == "storage" && assetFocus == "wheel") {
		return fmt.Errorf("segments[%d] cue/asset mismatch: %s cue uses %s asset", index, cueFocus, assetFocus)
	}
	return nil
}

func semanticFocus(text string) string {
	switch {
	case containsAny(text, "万向轮", "轮", "wheel", "caster"):
		return "wheel"
	case containsAny(text, "收纳", "分区", "打开", "open", "storage", "interior", "inside", "packed"):
		return "storage"
	default:
		return ""
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func validateMotionIntensity(value any) error {
	if value == nil {
		return nil
	}
	switch value.(type) {
	case string, float64, float32, int, int64, int32:
		return nil
	default:
		return fmt.Errorf("must be string or number")
	}
}

func validateWorkspacePath(value string) error {
	clean := path.Clean(strings.TrimSpace(value))
	if clean == "." || !strings.HasPrefix(clean, "/workspace/") {
		return fmt.Errorf("%q must be under /workspace", value)
	}
	return nil
}
