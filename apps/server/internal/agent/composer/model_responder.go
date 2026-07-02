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
	if templateKey != "simple_concat" && templateKey != "concat_with_fades" {
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
	segments := []any{}
	audioTracks := []any{}
	targetDuration := composerAudioTargetDuration(compositionContext)
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
