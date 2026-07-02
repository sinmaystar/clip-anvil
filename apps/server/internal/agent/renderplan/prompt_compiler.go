package renderplan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type PromptCompiler struct{}

func NewPromptCompiler() PromptCompiler {
	return PromptCompiler{}
}

func (PromptCompiler) Compile(_ context.Context, input UpsertInput) (CompileResult, error) {
	profile, ok := ProfileByID(input.ModelPromptProfile)
	if !ok {
		return CompileResult{}, fmt.Errorf("unknown model_prompt_profile %q", input.ModelPromptProfile)
	}
	if !profile.AllowedOperations[input.Operation] {
		return CompileResult{}, fmt.Errorf("operation %q is not allowed for %s", input.Operation, profile.ID)
	}
	if profile.ID == ProfileSeedance2Video && strings.TrimSpace(input.PromptParts.Action) == "" && len(input.PromptParts.Sequence) == 0 {
		return CompileResult{}, fmt.Errorf("seedance_2_video requires action 或 sequence")
	}
	prompt := compilePromptParts(input.PromptParts)
	if len([]rune(prompt)) > profile.MaxPromptChars {
		return CompileResult{}, fmt.Errorf("compiled prompt exceeds profile budget")
	}
	request := map[string]any{
		"provider":           profile.DefaultProvider,
		"model":              profile.DefaultModelID,
		"profile":            input.ModelPromptProfile,
		"operation":          input.Operation,
		"params":             input.Params,
		"reference_bindings": input.ReferenceBindings,
	}
	audit := map[string]any{
		"profile":      profile.ID,
		"operation":    input.Operation,
		"prompt_chars": len([]rune(prompt)),
	}
	requestJSON, _ := json.Marshal(request)
	auditJSON, _ := json.Marshal(audit)
	costJSON, _ := json.Marshal(map[string]any{"estimate": "not_charged_until_provider_submit"})
	return CompileResult{
		CompiledPrompt:  prompt,
		CompiledRequest: requestJSON,
		PromptAudit:     auditJSON,
		CostEstimate:    costJSON,
	}, nil
}

func compilePromptParts(parts PromptParts) string {
	lines := []string{}
	add := func(label, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			lines = append(lines, label+"："+value)
		}
	}
	add("目标", parts.Objective)
	add("主体", parts.Subject)
	add("场景", parts.Setting)
	add("动作", parts.Action)
	add("镜头", parts.Camera)
	add("构图", parts.Composition)
	add("风格", parts.Style)
	add("光影", parts.Lighting)
	if len(parts.Sequence) > 0 {
		lines = append(lines, "事件顺序："+strings.Join(parts.Sequence, "；"))
	}
	add("台词", parts.Dialogue)
	add("旁白", parts.Narration)
	add("音频", parts.Audio)
	add("文字", parts.TextRendering)
	if len(parts.QualityPack) > 0 {
		lines = append(lines, "质量要求："+strings.Join(parts.QualityPack, "；"))
	}
	if len(parts.ConstraintPack) > 0 {
		lines = append(lines, "约束："+strings.Join(parts.ConstraintPack, "；"))
	}
	if len(parts.NegativeHints) > 0 {
		lines = append(lines, "避免："+strings.Join(parts.NegativeHints, "；"))
	}
	return strings.Join(lines, "\n")
}
