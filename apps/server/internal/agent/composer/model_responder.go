package composer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	agentprompt "github.com/sinmaystar/clip-anvil/internal/agent/prompt"
	remotiontimeline "github.com/sinmaystar/clip-anvil/internal/remotiontimeline"
)

type arkChatModel interface {
	Generate(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.Message, error)
	Stream(ctx context.Context, in []*schema.Message, opts ...einoModel.Option) (*schema.StreamReader[*schema.Message], error)
}

type arkChatModelFactory func(ctx context.Context, config *ark.ChatModelConfig) (arkChatModel, error)

type VolcengineModelResponderConfig struct {
	APIKey           string
	BaseURL          string
	Region           string
	Model            string
	MaxTokens        int
	Temperature      float32
	Factory          arkChatModelFactory
	ContextCompactor contextcompact.Middleware
}

type VolcengineModelResponder struct {
	cfg     VolcengineModelResponderConfig
	factory arkChatModelFactory
}

func NewVolcengineModelResponder(cfg VolcengineModelResponderConfig) VolcengineModelResponder {
	factory := cfg.Factory
	if factory == nil {
		factory = func(ctx context.Context, config *ark.ChatModelConfig) (arkChatModel, error) {
			return ark.NewChatModel(ctx, config)
		}
	}
	return VolcengineModelResponder{cfg: cfg, factory: factory}
}

func (r VolcengineModelResponder) Respond(ctx context.Context, composerContext Context) (ComposerTurnOutput, error) {
	if out, ok, err := deterministicTemplateComposerTurn(composerContext); ok || err != nil {
		return out, err
	}
	apiKey := strings.TrimSpace(r.cfg.APIKey)
	if apiKey == "" {
		return ComposerTurnOutput{}, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY is required for Composer model")
	}
	modelID := strings.TrimSpace(r.cfg.Model)
	if modelID == "" {
		return ComposerTurnOutput{}, fmt.Errorf("CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL is required for Composer model")
	}
	config := &ark.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: strings.TrimSpace(r.cfg.BaseURL),
		Region:  strings.TrimSpace(r.cfg.Region),
		Model:   modelID,
		Timeout: durationPtr(10 * time.Minute),
	}
	if r.cfg.MaxTokens > 0 {
		config.MaxTokens = &r.cfg.MaxTokens
	}
	if r.cfg.Temperature > 0 {
		config.Temperature = &r.cfg.Temperature
	}
	model, err := r.factory(ctx, config)
	if err != nil {
		return ComposerTurnOutput{}, fmt.Errorf("create composer ark chat model: %w", err)
	}
	generator := model
	if len(composerContext.ToolInfos) > 0 {
		toolCallingModel, ok := model.(einoModel.ToolCallingChatModel)
		if !ok {
			return ComposerTurnOutput{}, fmt.Errorf("selected Composer model does not support tool calling")
		}
		boundModel, err := toolCallingModel.WithTools(composerContext.ToolInfos)
		if err != nil {
			return ComposerTurnOutput{}, fmt.Errorf("bind composer tools: %w", err)
		}
		generator = boundModel
	}
	prompt := composerPromptMessagesWithBoundaries(composerContext)
	messages := prompt.Messages
	facts, mediaCards := composerContextCompactionFacts(composerContext)
	var compacted contextcompact.ProjectionOutput
	if r.cfg.ContextCompactor != nil {
		compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
			WorkspaceID:       composerContext.Input.WorkspaceID,
			ThreadID:          composerContext.Input.ThreadID,
			TaskID:            composerContext.Input.TaskID,
			Role:              "composer",
			ModelID:           modelID,
			Messages:          messages,
			ToolInfos:         composerContext.ToolInfos,
			MediaCards:        mediaCards,
			Facts:             facts,
			Trigger:           "composer_before_model",
			SameTurnFromIndex: prompt.SameTurnFromIndex,
			PendingFromIndex:  prompt.PendingFromIndex,
		})
		if err != nil {
			return ComposerTurnOutput{}, fmt.Errorf("compact composer context: %w", err)
		}
		messages = compacted.Messages
	}
	retriedContextOverflow := false
	final, err := generator.Generate(ctx, messages)
	if err != nil && contextcompact.IsContextOverflowError(err) && r.cfg.ContextCompactor != nil {
		retriedContextOverflow = true
		compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
			WorkspaceID:       composerContext.Input.WorkspaceID,
			ThreadID:          composerContext.Input.ThreadID,
			TaskID:            composerContext.Input.TaskID,
			Role:              "composer",
			ModelID:           modelID,
			Messages:          prompt.Messages,
			ToolInfos:         composerContext.ToolInfos,
			MediaCards:        mediaCards,
			Facts:             facts,
			Trigger:           "model_error_context_overflow",
			SameTurnFromIndex: prompt.SameTurnFromIndex,
			PendingFromIndex:  prompt.PendingFromIndex,
			ForceFullCompact:  true,
		})
		if err != nil {
			return ComposerTurnOutput{}, fmt.Errorf("compact composer context after overflow: %w", err)
		}
		messages = compacted.Messages
		final, err = generator.Generate(ctx, messages)
	}
	if err != nil {
		return ComposerTurnOutput{}, fmt.Errorf("generate composer ark chat model: %w", err)
	}
	if final == nil {
		return ComposerTurnOutput{}, fmt.Errorf("generate composer ark chat model returned nil message")
	}
	metadata := map[string]any{
		"provider":               "volcengine",
		"model_id":               modelID,
		"native_tool_call_count": len(final.ToolCalls),
	}
	enrichComposerContextCompactionMetadata(metadata, compacted)
	if retriedContextOverflow {
		metadata["context_compaction_retry"] = true
	}
	return ComposerTurnOutput{
		AssistantText: strings.TrimSpace(final.Content),
		Metadata:      metadata,
		ModelMessage:  final,
	}, nil
}

func deterministicTemplateComposerTurn(composerContext Context) (ComposerTurnOutput, bool, error) {
	templateKey := strings.TrimSpace(composerContext.Input.Input.TemplateKey)
	if templateKey != "simple_concat" && templateKey != "concat_with_fades" && templateKey != remotiontimeline.TemplateKeyV1 {
		return ComposerTurnOutput{}, false, nil
	}
	if !hasComposerToolResult(composerContext, "get_composition_context") {
		return deterministicComposerToolCall("get_composition_context", map[string]any{
			"source_storyboard_node_id": strings.TrimSpace(composerContext.Input.Input.SourceStoryboardNodeID),
		}), true, nil
	}
	if !hasComposerToolResult(composerContext, "stage_media_inputs") {
		compositionContext, err := composerToolResultMap(composerContext, "get_composition_context")
		if err != nil {
			return ComposerTurnOutput{}, true, err
		}
		assets := composerAssetsForStaging(compositionContext)
		if len(assets) == 0 {
			return deterministicComposerBlocked("Composer 没有可合成的媒体资产。"), true, nil
		}
		return deterministicComposerToolCall("stage_media_inputs", map[string]any{
			"assets":     assets,
			"target_dir": "/workspace/input",
		}), true, nil
	}
	if !hasComposerToolResult(composerContext, "create_timeline_plan") {
		plan, err := deterministicComposerTimelinePlan(composerContext, templateKey)
		if err != nil {
			return ComposerTurnOutput{}, true, err
		}
		return deterministicComposerToolCall("create_timeline_plan", map[string]any{
			"source_storyboard_node_id": strings.TrimSpace(composerContext.Input.Input.SourceStoryboardNodeID),
			"template_key":              templateKey,
			"plan":                      plan,
			"render_settings":           map[string]any{"mode": "deterministic_template"},
		}), true, nil
	}
	if !hasComposerToolResult(composerContext, "render_timeline_template") {
		plan, err := deterministicComposerTimelinePlan(composerContext, templateKey)
		if err != nil {
			return ComposerTurnOutput{}, true, err
		}
		created, err := composerToolResultMap(composerContext, "create_timeline_plan")
		if err != nil {
			return ComposerTurnOutput{}, true, err
		}
		timelinePlanID := strings.TrimSpace(composerString(created["timeline_plan_id"]))
		if timelinePlanID == "" {
			return deterministicComposerBlocked("create_timeline_plan 未返回 timeline_plan_id。"), true, nil
		}
		return deterministicComposerToolCall("render_timeline_template", map[string]any{
			"timeline_plan_id": timelinePlanID,
			"template_key":     templateKey,
			"plan":             plan,
		}), true, nil
	}
	if !hasComposerToolResult(composerContext, "submit_composition_artifact") {
		created, err := composerToolResultMap(composerContext, "create_timeline_plan")
		if err != nil {
			return ComposerTurnOutput{}, true, err
		}
		rendered, err := composerToolResultMap(composerContext, "render_timeline_template")
		if err != nil {
			return ComposerTurnOutput{}, true, err
		}
		timelinePlanID := strings.TrimSpace(composerString(created["timeline_plan_id"]))
		outputPath := strings.TrimSpace(composerString(rendered["output_path"]))
		if timelinePlanID == "" || outputPath == "" {
			return deterministicComposerBlocked("render_timeline_template 未返回可提交的 timeline_plan_id/output_path。"), true, nil
		}
		return deterministicComposerToolCall("submit_composition_artifact", map[string]any{
			"timeline_plan_id": timelinePlanID,
			"output_path":      outputPath,
			"sandbox_job_id":   strings.TrimSpace(composerString(rendered["sandbox_job_id"])),
			"mime_type":        "video/mp4",
			"result": map[string]any{
				"mode":         "deterministic_template",
				"template_key": templateKey,
				"summary":      "Rendered by deterministic Composer template path.",
			},
		}), true, nil
	}
	submitted, err := composerToolResultMap(composerContext, "submit_composition_artifact")
	if err != nil {
		return ComposerTurnOutput{}, true, err
	}
	return ComposerTurnOutput{
		AssistantText: "Composer 已完成确定性模板合成并提交最终成片。",
		Result: CompositionOutput{
			Status:            "completed",
			OperationType:     "compose_final_video",
			TimelinePlanID:    strings.TrimSpace(composerString(submitted["timeline_plan_id"])),
			NodeID:            strings.TrimSpace(composerString(submitted["output_node_id"])),
			GenerationJobID:   strings.TrimSpace(composerString(submitted["generation_job_id"])),
			ArtifactVersionID: strings.TrimSpace(composerString(submitted["artifact_version_id"])),
			SandboxJobID:      strings.TrimSpace(composerString(submitted["sandbox_job_id"])),
		},
		Metadata: map[string]any{"provider": "deterministic_template", "template_key": templateKey},
		ModelMessage: &schema.Message{
			Role:    schema.Assistant,
			Content: "Composer 已完成确定性模板合成并提交最终成片。",
		},
	}, true, nil
}

func deterministicComposerToolCall(name string, args map[string]any) ComposerTurnOutput {
	raw := string(mustJSON(args))
	return ComposerTurnOutput{
		Metadata: map[string]any{"provider": "deterministic_template", "native_tool_call_count": 1},
		ModelMessage: &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:   "call_deterministic_" + strings.ReplaceAll(name, "_", "-"),
				Type: "function",
				Function: schema.FunctionCall{
					Name:      name,
					Arguments: raw,
				},
			}},
		},
	}
}

func deterministicComposerBlocked(message string) ComposerTurnOutput {
	text := strings.TrimSpace(message)
	if text == "" {
		text = "Composer deterministic template path blocked."
	}
	return ComposerTurnOutput{
		AssistantText: text,
		Result: CompositionOutput{
			Status:        "blocked",
			OperationType: "compose_final_video",
		},
		Metadata: map[string]any{"provider": "deterministic_template"},
		ModelMessage: &schema.Message{
			Role:    schema.Assistant,
			Content: text,
		},
	}
}

func hasComposerToolResult(composerContext Context, name string) bool {
	_, err := composerToolResultMap(composerContext, name)
	return err == nil
}

func composerToolResultMap(composerContext Context, name string) (map[string]any, error) {
	for i := len(composerContext.SameTurnMessages) - 1; i >= 0; i-- {
		message := composerContext.SameTurnMessages[i]
		if message.Role != "tool" || strings.TrimSpace(message.ToolName) != name {
			continue
		}
		if strings.Contains(message.Content, "工具调用失败") {
			return nil, fmt.Errorf("%s failed: %s", name, message.Content)
		}
		out := map[string]any{}
		if err := json.Unmarshal([]byte(message.Content), &out); err != nil {
			return nil, fmt.Errorf("parse %s result: %w", name, err)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s result not found", name)
}

func composerAssetsForStaging(compositionContext map[string]any) []map[string]any {
	rawAssets, _ := compositionContext["available_composition_assets"].([]any)
	assets := make([]map[string]any, 0, len(rawAssets))
	for _, raw := range rawAssets {
		asset, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		assetID := strings.TrimSpace(composerString(asset["asset_id"]))
		sourceURL := strings.TrimSpace(composerString(asset["source_url"]))
		fileName := strings.TrimSpace(composerString(asset["file_name"]))
		if assetID == "" || sourceURL == "" || fileName == "" {
			continue
		}
		assets = append(assets, map[string]any{
			"asset_id":   assetID,
			"source_url": sourceURL,
			"file_name":  fileName,
			"mime_type":  strings.TrimSpace(composerString(asset["mime_type"])),
		})
	}
	return assets
}

func deterministicComposerTimelinePlan(composerContext Context, templateKey string) (map[string]any, error) {
	if strings.TrimSpace(templateKey) == remotiontimeline.TemplateKeyV1 {
		return deterministicComposerRemotionTimelinePlan(composerContext)
	}
	compositionContext, err := composerToolResultMap(composerContext, "get_composition_context")
	if err != nil {
		return nil, err
	}
	staged, err := composerToolResultMap(composerContext, "stage_media_inputs")
	if err != nil {
		return nil, err
	}
	pathsByAsset := composerStagedPaths(staged)
	rawAssets, _ := compositionContext["available_composition_assets"].([]any)
	segments := deterministicComposerCueSegments(compositionContext, rawAssets, pathsByAsset)
	audioTracks := []any{}
	targetDuration := composerBestAudioTargetDuration(compositionContext, rawAssets)
	for _, raw := range rawAssets {
		asset, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(composerString(asset["role"]))
		assetID := strings.TrimSpace(composerString(asset["asset_id"]))
		workspacePath := pathsByAsset[assetID]
		if workspacePath == "" {
			continue
		}
		switch role {
		case "clip", "still":
			if len(segments) > 0 {
				continue
			}
			segment := map[string]any{
				"id":             firstNonEmpty(strings.TrimSpace(composerString(asset["shot_ref"])), assetID),
				"asset_id":       assetID,
				"workspace_path": workspacePath,
				"role":           role,
				"mime_type":      strings.TrimSpace(composerString(asset["mime_type"])),
			}
			if role == "still" {
				segment["duration_sec"] = 5
			}
			segments = append(segments, segment)
		case "voiceover":
			audioTracks = append(audioTracks, map[string]any{
				"id":             "voiceover-main",
				"role":           "voiceover",
				"asset_id":       assetID,
				"workspace_path": workspacePath,
				"start_sec":      0,
				"duration_sec":   targetDuration,
				"volume":         1,
				"fade_in_sec":    0.05,
				"fade_out_sec":   0.1,
			})
		case "bgm":
			audioTracks = append(audioTracks, map[string]any{
				"id":             "bgm-main",
				"role":           "bgm",
				"asset_id":       assetID,
				"workspace_path": workspacePath,
				"start_sec":      0,
				"duration_sec":   targetDuration,
				"volume":         0.28,
				"fade_in_sec":    0.5,
				"fade_out_sec":   1.2,
				"ducking": map[string]any{
					"sidechain_role": "voiceover",
					"threshold":      0.08,
					"ratio":          8,
					"attack_ms":      20,
					"release_ms":     250,
				},
			})
		}
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("deterministic composer has no staged video/still segments")
	}
	plan := map[string]any{
		"template_key": templateKey,
		"segments":     segments,
		"output": map[string]any{
			"workspace_path": "/workspace/output/final-deterministic-template.mp4",
			"format":         "mp4",
			"audio_codec":    "aac",
		},
	}
	if len(audioTracks) > 0 {
		plan["audio_tracks"] = audioTracks
	}
	return plan, nil
}

func deterministicComposerRemotionTimelinePlan(composerContext Context) (map[string]any, error) {
	compositionContext, err := composerToolResultMap(composerContext, "get_composition_context")
	if err != nil {
		return nil, err
	}
	staged, err := composerToolResultMap(composerContext, "stage_media_inputs")
	if err != nil {
		return nil, err
	}
	pathsByAsset := composerStagedPaths(staged)
	rawAssets, _ := compositionContext["available_composition_assets"].([]any)
	cues := composerAudioCues(compositionContext)
	if len(cues) == 0 {
		return nil, fmt.Errorf("remotion timeline requires audio_plan.cue_plan")
	}
	targetDuration := composerBestAudioTargetDuration(compositionContext, rawAssets)
	if targetDuration <= 0 {
		targetDuration = composerAudioTargetDuration(compositionContext)
	}
	scale := composerCueDurationScale(cues, targetDuration)
	segments := []any{}
	for index, cue := range cues {
		asset := composerVisualAssetForCue(rawAssets, pathsByAsset, cue)
		if asset == nil {
			continue
		}
		assetID := strings.TrimSpace(composerString(asset["asset_id"]))
		start := cue.StartSec * scale
		end := cue.EndSec * scale
		if end <= start {
			continue
		}
		layout := composerRemotionLayoutForCue(index, cue, asset, len(cues))
		segments = append(segments, map[string]any{
			"id":           cue.ShotRef,
			"shot_ref":     cue.ShotRef,
			"start_sec":    start,
			"end_sec":      end,
			"layout":       layout,
			"visual_focus": composerVisualFocusForCue(cue, layout),
			"assets": []any{
				map[string]any{
					"id":             assetID,
					"role":           "primary",
					"type":           composerRemotionAssetType(asset),
					"workspace_path": pathsByAsset[assetID],
					"node_ref":       strings.TrimSpace(composerString(asset["node_ref"])),
				},
			},
			"motion":        map[string]any{"preset": composerRemotionMotionForLayout(layout, index), "intensity": 0.55},
			"transition_in": composerRemotionTransitionForIndex(index),
			"caption": map[string]any{
				"source":    "audio_cue",
				"text":      firstNonEmpty(cue.Caption, cue.Text),
				"start_sec": start,
				"end_sec":   end,
				"position":  "subtitle_bottom",
			},
		})
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("remotion timeline has no cue-matched staged still or clip assets")
	}
	plan := map[string]any{
		"schema":       remotiontimeline.SchemaV1,
		"composition":  remotiontimeline.CompositionMarketingTimeline,
		"template_key": remotiontimeline.TemplateKeyV1,
		"output": map[string]any{
			"width":        1080,
			"height":       1920,
			"fps":          30,
			"duration_sec": targetDuration,
			"codec":        "h264",
			"audio_codec":  "aac",
		},
		"segments":     segments,
		"audio_tracks": deterministicComposerRemotionAudioTracks(rawAssets, pathsByAsset, targetDuration),
		"captions":     map[string]any{"source": "audio_cue", "single_lane": true, "max_chars_per_line": 18, "style": "commerce_subtitle"},
		"theme":        map[string]any{"background": "#0f172a", "accent": "#facc15"},
	}
	decoded, err := remotiontimeline.Decode(plan)
	if err != nil {
		return nil, err
	}
	if err := remotiontimeline.Validate(decoded); err != nil {
		return nil, err
	}
	return plan, nil
}

func composerVisualAssetForCue(rawAssets []any, pathsByAsset map[string]string, cue composerAudioCue) map[string]any {
	bestScore := -1
	var best map[string]any
	cueFocus := composerSemanticFocus(strings.Join([]string{cue.ShotRef, cue.Caption, cue.Text, cue.VisualFocus}, " "))
	for _, raw := range rawAssets {
		asset, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(composerString(asset["role"]))
		if role != "still" && role != "clip" {
			continue
		}
		assetID := strings.TrimSpace(composerString(asset["asset_id"]))
		if pathsByAsset[assetID] == "" {
			continue
		}
		assetFocus := composerSemanticFocus(composerAssetSearchText(asset, pathsByAsset[assetID]))
		if composerOppositeFocus(cueFocus, assetFocus) {
			continue
		}
		score := 0
		if strings.TrimSpace(composerString(asset["shot_ref"])) == cue.ShotRef {
			score += 100
		}
		if cueFocus != "" && cueFocus == assetFocus {
			score += 45
		}
		if role == "clip" {
			score += 12
		}
		if role == "still" {
			score += 5
		}
		if score > bestScore {
			bestScore = score
			best = asset
		}
	}
	if bestScore < 45 {
		return nil
	}
	return best
}

func composerRemotionAssetType(asset map[string]any) string {
	if strings.TrimSpace(composerString(asset["role"])) == "clip" {
		return "video"
	}
	return "image"
}

func composerRemotionLayoutForCue(index int, cue composerAudioCue, asset map[string]any, total int) string {
	text := strings.ToLower(strings.Join([]string{
		composerString(asset["shot_ref"]),
		composerString(asset["shot_title"]),
		composerString(asset["title"]),
		composerAssetSearchText(asset, ""),
		cue.Text,
		cue.Caption,
		cue.VisualFocus,
	}, " "))
	switch {
	case index == total-1 || strings.Contains(text, "cta") || strings.Contains(text, "现在") || strings.Contains(text, "出发"):
		return "cta_endcard"
	case strings.Contains(text, "wheel") || strings.Contains(text, "轮"):
		return "detail_focus"
	case strings.Contains(text, "storage") || strings.Contains(text, "收纳") || strings.Contains(text, "分区") || strings.Contains(text, "打开") || strings.Contains(text, "interior") || strings.Contains(text, "内景"):
		return "open_storage"
	case strings.Contains(text, "对比") || strings.Contains(text, "compare"):
		return "split_compare"
	case strings.Contains(text, "场景") || strings.Contains(text, "出差") || strings.Contains(text, "周末"):
		return "scenario_card"
	case index == 0:
		return "hero_packshot"
	default:
		return "benefit_card"
	}
}

func composerRemotionMotionForLayout(layout string, index int) string {
	switch layout {
	case "detail_focus":
		return "spotlight_reveal"
	case "open_storage":
		return "pull_out"
	case "benefit_card":
		return "float_parallax"
	case "split_compare":
		return "pan_left"
	case "scenario_card":
		return "pan_right"
	case "cta_endcard":
		return "cta_pop"
	default:
		if index%2 == 0 {
			return "push_in"
		}
		return "pan_right"
	}
}

func composerVisualFocusForCue(cue composerAudioCue, layout string) string {
	if strings.TrimSpace(cue.VisualFocus) != "" && len(strings.TrimSpace(cue.VisualFocus)) <= 12 {
		return strings.TrimSpace(cue.VisualFocus)
	}
	caption := strings.TrimSpace(cue.Caption)
	if len(caption) <= 12 {
		return caption
	}
	switch layout {
	case "detail_focus":
		return "顺滑万向轮"
	case "open_storage":
		return "分区收纳"
	case "scenario_card":
		return "出行场景"
	case "cta_endcard":
		return "现在出发"
	case "benefit_card":
		return "核心卖点"
	case "split_compare":
		return "对比清晰"
	default:
		return "悦行行李箱"
	}
}

func composerRemotionTransitionForIndex(index int) map[string]any {
	transitions := []string{"crossfade", "slide", "wipe", "zoom_blur", "crossfade"}
	if index == 0 {
		return map[string]any{"type": "cut", "duration_sec": 0}
	}
	return map[string]any{"type": transitions[index%len(transitions)], "duration_sec": 0.28}
}

func composerAssetSearchText(asset map[string]any, workspacePath string) string {
	return strings.Join([]string{
		workspacePath,
		composerString(asset["shot_ref"]),
		composerString(asset["shot_title"]),
		composerString(asset["title"]),
		composerString(asset["visual_intent"]),
		composerString(asset["action_text"]),
		composerString(asset["camera_intent"]),
		composerString(asset["node_ref"]),
		composerString(asset["file_name"]),
	}, " ")
}

func composerSemanticFocus(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "万向轮") || strings.Contains(lower, "wheel") || strings.Contains(lower, "caster") || strings.Contains(lower, "轮组"):
		return "wheel"
	case strings.Contains(lower, "收纳") || strings.Contains(lower, "分区") || strings.Contains(lower, "打开") || strings.Contains(lower, "storage") || strings.Contains(lower, "interior") || strings.Contains(lower, "inside") || strings.Contains(lower, "open"):
		return "storage"
	default:
		return ""
	}
}

func composerOppositeFocus(left, right string) bool {
	return (left == "wheel" && right == "storage") || (left == "storage" && right == "wheel")
}

func deterministicComposerRemotionAudioTracks(rawAssets []any, pathsByAsset map[string]string, duration float64) []any {
	tracks := []any{}
	for _, raw := range rawAssets {
		asset, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(composerString(asset["role"]))
		if role != "voiceover" && role != "bgm" {
			continue
		}
		assetID := strings.TrimSpace(composerString(asset["asset_id"]))
		path := pathsByAsset[assetID]
		if path == "" {
			continue
		}
		track := map[string]any{
			"id":             role + "-main",
			"role":           role,
			"asset_id":       assetID,
			"workspace_path": path,
			"start_sec":      0,
			"duration_sec":   duration,
			"volume":         1,
			"fade_in_sec":    0.05,
			"fade_out_sec":   0.1,
		}
		if role == "bgm" {
			track["volume"] = 0.28
			track["fade_in_sec"] = 0.5
			track["fade_out_sec"] = 1.2
		}
		tracks = append(tracks, track)
	}
	return tracks
}

func deterministicComposerCueSegments(compositionContext map[string]any, rawAssets []any, pathsByAsset map[string]string) []any {
	cues := composerAudioCues(compositionContext)
	if len(cues) == 0 {
		return nil
	}
	assetsByShot := map[string]map[string]any{}
	for _, raw := range rawAssets {
		asset, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(composerString(asset["role"]))
		if role != "clip" && role != "still" {
			continue
		}
		shotRef := strings.TrimSpace(composerString(asset["shot_ref"]))
		if shotRef == "" {
			continue
		}
		assetID := strings.TrimSpace(composerString(asset["asset_id"]))
		if pathsByAsset[assetID] == "" {
			continue
		}
		assetsByShot[shotRef] = asset
	}
	if len(assetsByShot) == 0 {
		return nil
	}
	scale := composerCueDurationScale(cues, composerBestAudioTargetDuration(compositionContext, rawAssets))
	segments := []any{}
	for _, cue := range cues {
		asset := assetsByShot[cue.ShotRef]
		if asset == nil {
			continue
		}
		assetID := strings.TrimSpace(composerString(asset["asset_id"]))
		duration := (cue.EndSec - cue.StartSec) * scale
		if duration <= 0 {
			continue
		}
		segment := map[string]any{
			"id":             cue.ShotRef,
			"asset_id":       assetID,
			"workspace_path": pathsByAsset[assetID],
			"role":           strings.TrimSpace(composerString(asset["role"])),
			"mime_type":      strings.TrimSpace(composerString(asset["mime_type"])),
			"duration_sec":   duration,
			"cue_start_sec":  cue.StartSec * scale,
			"cue_end_sec":    cue.EndSec * scale,
		}
		if cue.Caption != "" {
			segment["caption"] = cue.Caption
		}
		if cue.Text != "" {
			segment["cue_text"] = cue.Text
		}
		segments = append(segments, segment)
	}
	return segments
}

type composerAudioCue struct {
	ShotRef     string
	StartSec    float64
	EndSec      float64
	Text        string
	Caption     string
	VisualFocus string
}

func composerAudioCues(compositionContext map[string]any) []composerAudioCue {
	audioPlan, _ := compositionContext["audio_plan"].(map[string]any)
	rawCues, _ := audioPlan["cue_plan"].([]any)
	cues := make([]composerAudioCue, 0, len(rawCues))
	for _, raw := range rawCues {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		shotRef := strings.TrimSpace(composerString(item["shot_ref"]))
		text := strings.TrimSpace(composerString(item["text"]))
		start := composerFloat(item["start_sec"], 0)
		end := composerFloat(item["end_sec"], 0)
		if shotRef == "" || end <= start {
			continue
		}
		caption := strings.TrimSpace(composerString(item["caption"]))
		if caption == "" {
			caption = text
		}
		visualFocus := strings.TrimSpace(firstNonEmpty(composerString(item["visual_focus"]), composerString(item["visual_intent"])))
		cues = append(cues, composerAudioCue{ShotRef: shotRef, StartSec: start, EndSec: end, Text: text, Caption: caption, VisualFocus: visualFocus})
	}
	return cues
}

func composerCueDurationScale(cues []composerAudioCue, targetDuration float64) float64 {
	if targetDuration <= 0 {
		return 1
	}
	maxEnd := 0.0
	for _, cue := range cues {
		if cue.EndSec > maxEnd {
			maxEnd = cue.EndSec
		}
	}
	if maxEnd <= 0 {
		return 1
	}
	return targetDuration / maxEnd
}

func composerBestAudioTargetDuration(compositionContext map[string]any, rawAssets []any) float64 {
	if duration := composerVoiceoverDuration(rawAssets); duration > 0 {
		return duration
	}
	return composerAudioTargetDuration(compositionContext)
}

func composerVoiceoverDuration(rawAssets []any) float64 {
	for _, raw := range rawAssets {
		asset, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(composerString(asset["role"])) != "voiceover" {
			continue
		}
		metadata, _ := asset["metadata"].(map[string]any)
		if duration := composerFloat(metadata["duration_sec"], 0); duration > 0 {
			return duration
		}
		alignment, _ := metadata["alignment"].(map[string]any)
		if duration := composerAlignmentDuration(alignment); duration > 0 {
			return duration
		}
	}
	return 0
}

func composerAlignmentDuration(alignment map[string]any) float64 {
	rawSegments, _ := alignment["segments"].([]any)
	maxEnd := 0.0
	for _, raw := range rawSegments {
		segment, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if end := composerFloat(segment["end_sec"], 0); end > maxEnd {
			maxEnd = end
		}
	}
	return maxEnd
}

func composerStagedPaths(staged map[string]any) map[string]string {
	out := map[string]string{}
	rawFiles, _ := staged["files"].([]any)
	for _, raw := range rawFiles {
		file, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		assetID := strings.TrimSpace(composerString(file["asset_id"]))
		path := strings.TrimSpace(composerString(file["workspace_path"]))
		if assetID != "" && path != "" {
			out[assetID] = path
		}
	}
	return out
}

func composerAudioTargetDuration(compositionContext map[string]any) float64 {
	audioPlan, _ := compositionContext["audio_plan"].(map[string]any)
	switch value := audioPlan["target_duration_sec"].(type) {
	case float64:
		if value > 0 {
			return value
		}
	case int:
		if value > 0 {
			return float64(value)
		}
	}
	return 30
}

func composerString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func composerFloat(value any, defaultValue float64) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return defaultValue
	}
}

type DeterministicResponder struct{}

func NewDeterministicResponder() DeterministicResponder {
	return DeterministicResponder{}
}

func (DeterministicResponder) Respond(_ context.Context, composerContext Context) (ComposerTurnOutput, error) {
	text := "Composer Agent 已接入 native tool loop；当前非 real 模式不会沿用旧线性合成逻辑自动伪造成片。"
	return ComposerTurnOutput{
		AssistantText: text,
		Result: CompositionOutput{
			Status:        "blocked",
			OperationType: "compose_final_video",
		},
		Metadata: map[string]any{"provider": "deterministic"},
		ModelMessage: &schema.Message{
			Role:    schema.Assistant,
			Content: strings.TrimSpace(text + "\n\nContext: " + composerContext.Summary),
		},
	}, nil
}

type composerPromptBoundary struct {
	Messages          []*schema.Message
	SameTurnFromIndex int
	PendingFromIndex  int
}

func composerPromptMessagesWithBoundaries(composerContext Context) composerPromptBoundary {
	messages := []*schema.Message{
		{Role: schema.System, Content: SystemPrompt},
		{Role: schema.User, Content: composerUserMessage(composerContext)},
	}
	sameTurnFromIndex := contextcompact.CurrentToolLoopFromIndex(len(messages), len(composerContext.SameTurnMessages))
	for _, message := range composerContext.SameTurnMessages {
		switch message.Role {
		case "assistant":
			messages = append(messages, &schema.Message{
				Role:    schema.Assistant,
				Content: message.Content,
				ToolCalls: []schema.ToolCall{{
					ID:   message.ToolCallID,
					Type: "function",
					Function: schema.FunctionCall{
						Name:      message.ToolName,
						Arguments: string(mustJSON(message.ToolArguments)),
					},
				}},
			})
		case "tool":
			messages = append(messages, &schema.Message{
				Role:       schema.Tool,
				Content:    message.Content,
				ToolCallID: message.ToolCallID,
				ToolName:   message.ToolName,
			})
		}
	}
	pendingFromIndex := contextcompact.PendingReminderTargetIndex(messages, composerContext.PendingReminders)
	messages = agentprompt.AppendPendingReminders(messages, composerContext.PendingReminders)
	return composerPromptBoundary{Messages: messages, SameTurnFromIndex: sameTurnFromIndex, PendingFromIndex: pendingFromIndex}
}

func enrichComposerContextCompactionMetadata(metadata map[string]any, output contextcompact.ProjectionOutput) {
	if len(output.Applied) == 0 {
		return
	}
	metadata["context_compaction_applied"] = true
	metadata["context_compaction_mode"] = output.CompactionMode
	metadata["context_compaction_count"] = len(output.Applied)
	metadata["context_compaction_refs"] = output.CompactionRefs
	metadata["context_compaction_detail_files"] = output.DetailFiles
}

func composerUserMessage(composerContext Context) string {
	lines := []string{
		"Run a Composer final-output turn.",
		"Workspace summary: " + strings.TrimSpace(composerContext.Summary),
		"Instructions: " + strings.TrimSpace(composerContext.Input.Input.Instructions),
		"Template: " + strings.TrimSpace(composerContext.Input.Input.TemplateKey),
	}
	if composerContext.SourceStoryboardNodeID.Valid {
		lines = append(lines, "Source storyboard node id: "+uuidString(composerContext.SourceStoryboardNodeID))
	}
	if strings.TrimSpace(composerContext.SourceNodeTitle) != "" {
		lines = append(lines, "Source node title: "+strings.TrimSpace(composerContext.SourceNodeTitle))
	}
	return strings.Join(lines, "\n")
}

func durationPtr(value time.Duration) *time.Duration {
	return &value
}
