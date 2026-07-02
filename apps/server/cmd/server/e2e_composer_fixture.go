package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	agentcomposer "github.com/sinmaystar/clip-anvil/internal/agent/composer"
)

type e2eMotionShotVideoComposerResponder struct{}

func (e2eMotionShotVideoComposerResponder) Respond(_ context.Context, composerContext agentcomposer.Context) (agentcomposer.ComposerTurnOutput, error) {
	switch e2eComposerToolResultCount(composerContext.SameTurnMessages) {
	case 0:
		return e2eComposerToolCallOutput("e2e-motion-composition-context", "get_composition_context", `{}`), nil
	case 1:
		args, err := e2eStageMediaInputsArgs(composerContext)
		if err != nil {
			return e2eComposerBlockedOutput(err.Error()), nil
		}
		return e2eComposerToolCallOutput("e2e-motion-stage-media", "stage_media_inputs", args), nil
	case 2:
		args, err := e2eCreateTimelinePlanArgs(composerContext)
		if err != nil {
			return e2eComposerBlockedOutput(err.Error()), nil
		}
		return e2eComposerToolCallOutput("e2e-motion-create-timeline", "create_timeline_plan", args), nil
	case 3:
		args, err := e2eRenderTimelineArgs(composerContext)
		if err != nil {
			return e2eComposerBlockedOutput(err.Error()), nil
		}
		return e2eComposerToolCallOutput("e2e-motion-render-timeline", "render_timeline_template", args), nil
	case 4:
		args, err := e2eSubmitCompositionArgs(composerContext)
		if err != nil {
			return e2eComposerBlockedOutput(err.Error()), nil
		}
		return e2eComposerToolCallOutput("e2e-motion-submit-composition", "submit_composition_artifact", args), nil
	default:
		return agentcomposer.ComposerTurnOutput{
			AssistantText: "已通过 Composer native tools 合成 motion_only/no-Seedance 最终视频。",
			Result: agentcomposer.CompositionOutput{
				Status:        "completed",
				OperationType: "compose_final_video",
			},
			Metadata: map[string]any{"e2e_fixture": "motion_shot_video"},
			ModelMessage: &schema.Message{
				Role:    schema.Assistant,
				Content: "已通过 Composer native tools 合成 motion_only/no-Seedance 最终视频。",
			},
		}, nil
	}
}

func e2eComposerToolResultCount(messages []agentcomposer.ComposerSameTurnMessage) int {
	count := 0
	for _, message := range messages {
		if message.Role == "tool" {
			count++
		}
	}
	return count
}

func e2eComposerToolCallOutput(id string, name string, arguments string) agentcomposer.ComposerTurnOutput {
	return agentcomposer.ComposerTurnOutput{
		ModelMessage: &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:   id,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      name,
					Arguments: arguments,
				},
			}},
		},
		Metadata: map[string]any{"e2e_fixture": "motion_shot_video"},
	}
}

func e2eComposerBlockedOutput(message string) agentcomposer.ComposerTurnOutput {
	text := "motion_only Composer fixture blocked: " + strings.TrimSpace(message)
	return agentcomposer.ComposerTurnOutput{
		AssistantText: text,
		Result: agentcomposer.CompositionOutput{
			Status:        "blocked",
			OperationType: "compose_final_video",
		},
		Metadata: map[string]any{"e2e_fixture": "motion_shot_video"},
		ModelMessage: &schema.Message{
			Role:    schema.Assistant,
			Content: text,
		},
	}
}

type e2eCompositionContextResult struct {
	AvailableCompositionAssets []e2eCompositionAsset `json:"available_composition_assets"`
	SourceStoryboardNodeID     string                `json:"source_storyboard_node_id"`
}

type e2eCompositionAsset struct {
	Role      string `json:"role"`
	AssetID   string `json:"asset_id"`
	SourceURL string `json:"source_url"`
	FileName  string `json:"file_name"`
	MimeType  string `json:"mime_type"`
}

type e2eStageMediaResult struct {
	Files []e2eStageMediaFile `json:"files"`
}

type e2eStageMediaFile struct {
	AssetID       string `json:"asset_id"`
	WorkspacePath string `json:"workspace_path"`
	FileName      string `json:"file_name"`
	MimeType      string `json:"mime_type"`
}

type e2eTimelineResult struct {
	TimelinePlanID string `json:"timeline_plan_id"`
}

type e2eRenderResult struct {
	OutputPath   string `json:"output_path"`
	SandboxJobID string `json:"sandbox_job_id"`
}

func e2eStageMediaInputsArgs(context agentcomposer.Context) (string, error) {
	composition, err := e2eCompositionContext(context)
	if err != nil {
		return "", err
	}
	assets := make([]map[string]any, 0, len(composition.AvailableCompositionAssets))
	for _, asset := range composition.AvailableCompositionAssets {
		role := strings.TrimSpace(asset.Role)
		if role != "clip" && role != "voiceover" && role != "bgm" {
			continue
		}
		assets = append(assets, map[string]any{
			"asset_id":   asset.AssetID,
			"source_url": asset.SourceURL,
			"file_name":  asset.FileName,
			"mime_type":  asset.MimeType,
		})
	}
	if len(assets) < 3 {
		return "", fmt.Errorf("composition assets need clip, voiceover and bgm; got %d", len(assets))
	}
	return e2eJSON(map[string]any{
		"assets":     assets,
		"target_dir": "/workspace/input",
	})
}

func e2eCreateTimelinePlanArgs(context agentcomposer.Context) (string, error) {
	composition, err := e2eCompositionContext(context)
	if err != nil {
		return "", err
	}
	plan, err := e2eTimelinePlan(context)
	if err != nil {
		return "", err
	}
	return e2eJSON(map[string]any{
		"source_storyboard_node_id": composition.SourceStoryboardNodeID,
		"template_key":              "simple_concat",
		"plan":                      plan,
		"render_settings": map[string]any{
			"reason": "motion_only/no-Seedance E2E final composition",
		},
	})
}

func e2eRenderTimelineArgs(context agentcomposer.Context) (string, error) {
	timelineID, err := e2eTimelinePlanID(context)
	if err != nil {
		return "", err
	}
	plan, err := e2eTimelinePlan(context)
	if err != nil {
		return "", err
	}
	return e2eJSON(map[string]any{
		"timeline_plan_id": timelineID,
		"template_key":     "simple_concat",
		"plan":             plan,
	})
}

func e2eSubmitCompositionArgs(context agentcomposer.Context) (string, error) {
	timelineID, err := e2eTimelinePlanID(context)
	if err != nil {
		return "", err
	}
	render, err := e2eRenderResultForContext(context)
	if err != nil {
		return "", err
	}
	return e2eJSON(map[string]any{
		"timeline_plan_id": timelineID,
		"output_path":      render.OutputPath,
		"sandbox_job_id":   render.SandboxJobID,
		"mime_type":        "video/mp4",
		"result": map[string]any{
			"e2e_fixture": "motion_shot_video",
		},
	})
}

func e2eTimelinePlan(context agentcomposer.Context) (map[string]any, error) {
	composition, err := e2eCompositionContext(context)
	if err != nil {
		return nil, err
	}
	stage, err := e2eStageMedia(context)
	if err != nil {
		return nil, err
	}
	roleByAsset := map[string]string{}
	for _, asset := range composition.AvailableCompositionAssets {
		roleByAsset[strings.TrimSpace(asset.AssetID)] = strings.TrimSpace(asset.Role)
	}
	pathByRole := map[string]e2eStageMediaFile{}
	clipByShot := map[string]e2eStageMediaFile{}
	for _, file := range stage.Files {
		role := roleByAsset[strings.TrimSpace(file.AssetID)]
		if role == "clip" {
			if shotKey := e2eShotKeyForStageFile(file); shotKey != "" {
				clipByShot[shotKey] = file
			}
			continue
		}
		if role != "" {
			pathByRole[role] = file
		}
	}
	voiceover, ok := pathByRole["voiceover"]
	if !ok {
		return nil, fmt.Errorf("staged voiceover asset missing")
	}
	bgm, ok := pathByRole["bgm"]
	if !ok {
		return nil, fmt.Errorf("staged bgm asset missing")
	}
	segments, totalDuration, err := e2eMotionSegments(clipByShot)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"segments": segments,
		"audio_tracks": []map[string]any{
			{
				"id":             "voiceover",
				"role":           "voiceover",
				"asset_id":       voiceover.AssetID,
				"workspace_path": voiceover.WorkspacePath,
				"start_sec":      0,
				"duration_sec":   totalDuration,
				"volume":         1,
				"fade_in_sec":    0.05,
				"fade_out_sec":   0.1,
			},
			{
				"id":             "bgm",
				"role":           "bgm",
				"asset_id":       bgm.AssetID,
				"workspace_path": bgm.WorkspacePath,
				"start_sec":      0,
				"duration_sec":   totalDuration,
				"volume":         0.28,
				"fade_in_sec":    0.3,
				"fade_out_sec":   0.8,
				"ducking": map[string]any{
					"sidechain_role": "voiceover",
					"threshold":      0.08,
					"ratio":          8,
					"attack_ms":      20,
					"release_ms":     250,
				},
			},
		},
		"output": map[string]any{
			"workspace_path": "/workspace/output/yuexing-dynamic-remotion-final.mp4",
			"width":          1080,
			"height":         1920,
			"fps":            24,
			"format":         "mp4",
			"audio_codec":    "aac",
		},
	}, nil
}

func e2eMotionSegments(clipByShot map[string]e2eStageMediaFile) ([]map[string]any, int, error) {
	shotOrder := []string{"shot_01_hook", "shot_02_product", "shot_03_wheels", "shot_04_travel", "shot_05_cta"}
	shotDurations := map[string]int{
		"shot_01_hook":    6,
		"shot_02_product": 8,
		"shot_03_wheels":  8,
		"shot_04_travel":  6,
		"shot_05_cta":     6,
	}
	segments := make([]map[string]any, 0, len(shotOrder))
	startSec := 0
	for _, shotKey := range shotOrder {
		clip, ok := clipByShot[shotKey]
		if !ok {
			return nil, 0, fmt.Errorf("staged clip for %s missing", shotKey)
		}
		duration := shotDurations[shotKey]
		segments = append(segments, map[string]any{
			"id":             shotKey,
			"asset_id":       clip.AssetID,
			"workspace_path": clip.WorkspacePath,
			"start_sec":      startSec,
			"duration_sec":   duration,
		})
		startSec += duration
	}
	return segments, startSec, nil
}

func e2eShotKeyForStageFile(file e2eStageMediaFile) string {
	text := strings.Join([]string{file.AssetID, file.WorkspacePath, file.FileName}, " ")
	for _, shotKey := range []string{"shot_01_hook", "shot_02_product", "shot_03_wheels", "shot_04_travel", "shot_05_cta"} {
		if strings.Contains(text, shotKey) {
			return shotKey
		}
	}
	return ""
}

func e2eCompositionContext(context agentcomposer.Context) (e2eCompositionContextResult, error) {
	var out e2eCompositionContextResult
	if err := e2eLastComposerToolJSON(context, "get_composition_context", &out); err != nil {
		return out, err
	}
	return out, nil
}

func e2eStageMedia(context agentcomposer.Context) (e2eStageMediaResult, error) {
	var out e2eStageMediaResult
	if err := e2eLastComposerToolJSON(context, "stage_media_inputs", &out); err != nil {
		return out, err
	}
	return out, nil
}

func e2eTimelinePlanID(context agentcomposer.Context) (string, error) {
	var out e2eTimelineResult
	if err := e2eLastComposerToolJSON(context, "create_timeline_plan", &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.TimelinePlanID) == "" {
		return "", fmt.Errorf("create_timeline_plan returned empty timeline_plan_id")
	}
	return out.TimelinePlanID, nil
}

func e2eRenderResultForContext(context agentcomposer.Context) (e2eRenderResult, error) {
	var out e2eRenderResult
	if err := e2eLastComposerToolJSON(context, "render_timeline_template", &out); err != nil {
		return out, err
	}
	if strings.TrimSpace(out.OutputPath) == "" || strings.TrimSpace(out.SandboxJobID) == "" {
		return out, fmt.Errorf("render_timeline_template returned incomplete output")
	}
	return out, nil
}

func e2eLastComposerToolJSON(context agentcomposer.Context, toolName string, out any) error {
	for i := len(context.SameTurnMessages) - 1; i >= 0; i-- {
		message := context.SameTurnMessages[i]
		if message.Role == "tool" && message.ToolName == toolName {
			return json.Unmarshal([]byte(message.Content), out)
		}
	}
	return fmt.Errorf("%s result missing", toolName)
}

func e2eJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
