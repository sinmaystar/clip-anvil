package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentstoryboard "github.com/sinmaystar/clip-anvil/internal/agent/storyboard"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type StoryboardUpdater interface {
	UpdateStoryboard(ctx context.Context, input agentstoryboard.UpdateInput) (agentstoryboard.UpdateOutput, error)
}

type UpdateStoryboardTool struct {
	updater StoryboardUpdater
}

func NewUpdateStoryboardTool(updater StoryboardUpdater) UpdateStoryboardTool {
	return UpdateStoryboardTool{updater: updater}
}

func (t UpdateStoryboardTool) Definition() Definition {
	return Definition{
		Name:        "update_storyboard",
		Description: "Create or modify the Agent storyboard. This writes shot and shot dependency facts only. It does not generate preview images, videos, reviews, or final composition.",
		Parameters: objectSchema(map[string]any{
			"intent":           map[string]any{"type": "string", "enum": []string{"replace", "upsert", "patch", "archive"}},
			"shots":            map[string]any{"type": "array"},
			"storyboard_shots": map[string]any{"type": "array", "description": "Compatibility alias for shots. Each item may use shot_number, duration, content, voice_over, and ui_overlay."},
			"dependencies":     map[string]any{"type": "array"},
			"summary":          map[string]any{"type": "string"},
		}),
		Result: map[string]any{"type": "object"},
		Safety: SafetySpec{
			MaxCallsPerTurn: 5,
		},
		Visibility: VisibilitySpec{
			ShowCallMessage:   true,
			ShowResultMessage: true,
			UserLabel:         "更新分镜",
		},
	}
}

func (t UpdateStoryboardTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	if t.updater == nil {
		return ExecuteOutput{}, errors.New("update_storyboard service is not configured")
	}
	args, err := storyboardArgs(input.Arguments)
	if err != nil {
		return ExecuteOutput{}, err
	}
	update := agentstoryboard.UpdateInput{
		WorkspaceID:  input.WorkspaceID,
		Intent:       args.Intent,
		Shots:        args.Shots,
		Dependencies: args.Dependencies,
		Summary:      args.Summary,
	}
	out, err := t.updater.UpdateStoryboard(ctx, update)
	if err != nil {
		return ExecuteOutput{}, err
	}
	return ExecuteOutput{Result: map[string]any{
		"status":               "succeeded",
		"shots_created":        out.ShotsCreated,
		"shots_updated":        out.ShotsUpdated,
		"shots_archived":       out.ShotsArchived,
		"dependencies_created": out.DependenciesCreated,
		"shots":                shotResults(out.Shots),
	}}, nil
}

type parsedStoryboardArgs struct {
	Intent       string
	Summary      string
	Shots        []agentstoryboard.ShotInput
	Dependencies []agentstoryboard.DependencyInput
}

func storyboardArgs(raw map[string]any) (parsedStoryboardArgs, error) {
	intent, _ := raw["intent"].(string)
	intent = strings.TrimSpace(intent)
	if intent == "" {
		intent = "upsert"
	}
	shotsRaw, ok := raw["shots"].([]any)
	if !ok {
		shotsRaw, ok = raw["storyboard_shots"].([]any)
		if !ok {
			return parsedStoryboardArgs{}, fmt.Errorf("invalid update_storyboard shots")
		}
	}
	shots := make([]agentstoryboard.ShotInput, 0, len(shotsRaw))
	for i, item := range shotsRaw {
		object, ok := item.(map[string]any)
		if !ok {
			return parsedStoryboardArgs{}, fmt.Errorf("invalid update_storyboard shot")
		}
		sortOrder := int32Value(object, "sort_order", 0)
		if sortOrder <= 0 {
			sortOrder = int32Value(object, "shot_number", int32(i+1))
		}
		clientKey := stringValue(object, "client_key")
		if clientKey == "" {
			clientKey = stringValue(object, "shot_id")
		}
		if clientKey == "" && sortOrder > 0 {
			clientKey = fmt.Sprintf("shot-%02d", sortOrder)
		}
		title := stringValue(object, "title")
		if title == "" && sortOrder > 0 {
			title = fmt.Sprintf("分镜 %d", sortOrder)
		}
		shot := agentstoryboard.ShotInput{
			ID:               stringValue(object, "id"),
			ClientKey:        clientKey,
			SortOrder:        sortOrder,
			Title:            title,
			Brief:            storyboardBriefValue(object),
			NarrativePurpose: firstStringValue(object, "narrative_purpose", "purpose"),
			Status:           stringValue(object, "status"),
			LinkedNodeIDs:    stringSliceValue(object, "linked_node_ids"),
		}
		if duration, ok := floatValue(object, "duration_sec"); ok {
			shot.DurationSec = &duration
		} else if duration, ok := floatValue(object, "duration"); ok {
			shot.DurationSec = &duration
		}
		shots = append(shots, shot)
	}
	deps := []agentstoryboard.DependencyInput{}
	if depsRaw, ok := raw["dependencies"].([]any); ok {
		for _, item := range depsRaw {
			object, ok := item.(map[string]any)
			if !ok {
				return parsedStoryboardArgs{}, fmt.Errorf("invalid update_storyboard dependency")
			}
			deps = append(deps, agentstoryboard.DependencyInput{
				From:             stringValue(object, "from"),
				To:               stringValue(object, "to"),
				DependencyType:   stringValue(object, "dependency_type"),
				RequiredArtifact: stringValue(object, "required_artifact"),
				InjectionRole:    stringValue(object, "injection_role"),
				BlockingPhase:    stringValue(object, "blocking_phase"),
				StalePolicy:      stringValue(object, "stale_policy"),
				Reason:           stringValue(object, "reason"),
			})
		}
	}
	return parsedStoryboardArgs{Intent: intent, Summary: stringValue(raw, "summary"), Shots: shots, Dependencies: deps}, nil
}

func shotResults(shots []db.Shot) []map[string]any {
	out := make([]map[string]any, 0, len(shots))
	for _, shot := range shots {
		out = append(out, map[string]any{
			"id":         uuidString(shot.ID),
			"client_key": shot.ClientKey,
			"title":      shot.Title,
			"status":     shot.Status,
		})
	}
	return out
}

func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func firstStringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(values, key); value != "" {
			return value
		}
	}
	return ""
}

func int32Value(values map[string]any, key string, fallback int32) int32 {
	switch value := values[key].(type) {
	case float64:
		return int32(value)
	case int:
		return int32(value)
	default:
		return fallback
	}
}

func floatValue(values map[string]any, key string) (float64, bool) {
	value, ok := values[key].(float64)
	return value, ok
}

func mapValue(values map[string]any, key string) map[string]any {
	value, ok := values[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return value
}

func storyboardBriefValue(values map[string]any) map[string]any {
	brief := mapValue(values, "brief")
	if len(brief) == 0 {
		brief = map[string]any{}
	}
	for _, key := range []string{"content", "voice_over", "ui_overlay"} {
		if value := stringValue(values, key); value != "" {
			brief[key] = value
		}
	}
	if _, ok := brief["summary"]; !ok {
		if content := stringValue(values, "content"); content != "" {
			brief["summary"] = content
		}
	}
	return brief
}

func stringSliceValue(values map[string]any, key string) []string {
	switch raw := values[key].(type) {
	case []string:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if value := strings.TrimSpace(item); value != "" {
				out = append(out, value)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
				out = append(out, strings.TrimSpace(value))
			}
		}
		return out
	default:
		return nil
	}
}
