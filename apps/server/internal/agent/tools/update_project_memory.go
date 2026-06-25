package tools

import (
	"context"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/creative"
)

type UpdateProjectMemoryNativeTool struct {
	service *creative.Service
}

type UpdateProjectMemoryToolInput struct {
	Brief                string              `json:"brief" jsonschema:"required" jsonschema_description:"本次写入 ProjectMemory 的业务目的，例如记录商品外观一致性和机场商务氛围。不要超过 160 个中文字符。"`
	Mode                 string              `json:"mode" jsonschema:"required,enum=create,enum=patch,enum=replace" jsonschema_description:"create 创建第一个版本；patch 基于当前 active memory 增量创建新版本；replace 整体替换当前创作宪法。"`
	CoreIntent           string              `json:"core_intent" jsonschema_description:"项目核心意图，描述这条视频最重要的目标。patch 时为空表示不修改。"`
	Soul                 string              `json:"soul" jsonschema_description:"项目气质和创作灵魂，用来约束多个分镜保持同一种感觉。patch 时为空表示不修改。"`
	BrandFacts           []MemoryFactInput   `json:"brand_facts" jsonschema_description:"品牌和商品事实，例如颜色、材质、Logo、卖点。patch 时追加或更新同 key 的事实。"`
	NonNegotiables       []MemoryRuleInput   `json:"non_negotiables" jsonschema_description:"不可妥协约束，例如商品外观必须一致。通常会影响 Reviewer blocking 判断。"`
	VisualAnchors        []MemoryFactInput   `json:"visual_anchors" jsonschema_description:"全片复用的视觉锚点，例如银灰色箱体、现代机场晨光。"`
	Allowed              []MemoryRuleInput   `json:"allowed" jsonschema_description:"明确允许出现的内容。"`
	Forbidden            []MemoryRuleInput   `json:"forbidden" jsonschema_description:"明确禁止出现的内容，例如竞品 Logo、低质感杂乱背景。"`
	PromptInjectionHints []string            `json:"prompt_injection_hints" jsonschema_description:"短约束列表，后续可注入每个 shot prompt。每条应短小明确，不要写完整 prompt。"`
	SourceRefs           []MemorySourceInput `json:"source_refs" jsonschema_description:"这次 memory 修改来自哪些用户消息、素材或对象。"`
	RequiresUserApproval bool                `json:"requires_user_approval" jsonschema_description:"如果这次修改会改变用户已确认的核心方向，填写 true；工具会提示应先请求用户决策。"`
	Reason               string              `json:"reason" jsonschema:"required" jsonschema_description:"为什么写入这个 memory 版本，用于审计和后续解释。"`
}

type MemoryFactInput struct {
	Key   string `json:"key" jsonschema:"required" jsonschema_description:"稳定键，例如 product_color、airport_mood。"`
	Value string `json:"value" jsonschema:"required" jsonschema_description:"事实内容。"`
}

type MemoryRuleInput struct {
	Rule     string `json:"rule" jsonschema:"required" jsonschema_description:"规则内容，必须具体可判断。"`
	Severity string `json:"severity" jsonschema:"enum=low,enum=medium,enum=high,enum=blocking" jsonschema_description:"严重程度。blocking 表示违反后不能自动接受。"`
}

type MemorySourceInput struct {
	Type string `json:"type" jsonschema:"required,enum=user_message,enum=asset,enum=creative_brief,enum=key_element,enum=shot" jsonschema_description:"来源类型。"`
	ID   string `json:"id" jsonschema_description:"来源对象 ID，可为空。"`
	Note string `json:"note" jsonschema_description:"来源说明。"`
}

func NewUpdateProjectMemoryNativeTool(service *creative.Service) *UpdateProjectMemoryNativeTool {
	return &UpdateProjectMemoryNativeTool{service: service}
}

func (t *UpdateProjectMemoryNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[UpdateProjectMemoryToolInput](toolUpdateMemory, "写入新的 ClipAnvil 项目创作宪法 ProjectMemory 版本，记录全片必须遵守的核心意图、创作灵魂、品牌事实、不可妥协约束、视觉锚点、允许项、禁止项和短提示词注入约束。")
}

func (t *UpdateProjectMemoryNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolUpdateMemory, argumentsInJSON, validateUpdateProjectMemoryInput)
	if !ok {
		return msg, nil
	}
	if msg, ok := serviceOrError(t.service, toolUpdateMemory); !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolUpdateMemory)
	if !ok {
		return msg, nil
	}
	memory, err := t.service.UpdateProjectMemory(ctx, creative.UpdateProjectMemoryInput{
		WorkspaceID:          runtime.WorkspaceID,
		ThreadID:             runtime.ThreadID,
		TaskID:               runtime.TaskID,
		Brief:                input.Brief,
		Mode:                 input.Mode,
		CoreIntent:           input.CoreIntent,
		Soul:                 input.Soul,
		BrandFacts:           toMemoryFacts(input.BrandFacts),
		NonNegotiables:       toMemoryRules(input.NonNegotiables),
		VisualAnchors:        toMemoryFacts(input.VisualAnchors),
		Allowed:              toMemoryRules(input.Allowed),
		Forbidden:            toMemoryRules(input.Forbidden),
		PromptInjectionHints: input.PromptInjectionHints,
		SourceRefs:           toMemorySources(input.SourceRefs),
		RequiresUserApproval: input.RequiresUserApproval,
		Reason:               input.Reason,
	})
	if err != nil {
		return naturalErrorFromErr(toolUpdateMemory, err), nil
	}
	return NaturalResult{
		Title: "已写入 ProjectMemory",
		Items: []NaturalResultItem{
			{Label: "版本", Value: fmt.Sprintf("v%d", memory.Version)},
			{Label: "核心意图", Value: memory.CoreIntent},
			{Label: "创作灵魂", Value: memory.Soul},
		},
		Next: "继续检查关键元素和 storyboard 是否已经覆盖这些约束。",
	}.String(), nil
}

func validateUpdateProjectMemoryInput(input UpdateProjectMemoryToolInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if err := requireText(input.Reason, "reason"); err != nil {
		return err
	}
	if err := requireMode(input.Mode, "create", "patch", "replace"); err != nil {
		return err
	}
	if input.Mode == "replace" && input.CoreIntent == "" && input.Soul == "" {
		return fmt.Errorf("replace 必须提供 core_intent 或 soul")
	}
	for _, hint := range input.PromptInjectionHints {
		if len([]rune(hint)) > 80 {
			return fmt.Errorf("prompt_injection_hints 每条不能超过 80 个中文字符")
		}
	}
	return nil
}

func toMemoryFacts(input []MemoryFactInput) []creative.MemoryFact {
	out := make([]creative.MemoryFact, 0, len(input))
	for _, item := range input {
		out = append(out, creative.MemoryFact{Key: item.Key, Value: item.Value})
	}
	return out
}

func toMemoryRules(input []MemoryRuleInput) []creative.MemoryRule {
	out := make([]creative.MemoryRule, 0, len(input))
	for _, item := range input {
		out = append(out, creative.MemoryRule{Rule: item.Rule, Severity: item.Severity})
	}
	return out
}

func toMemorySources(input []MemorySourceInput) []creative.MemorySource {
	out := make([]creative.MemorySource, 0, len(input))
	for _, item := range input {
		out = append(out, creative.MemorySource{Type: item.Type, ID: item.ID, Note: item.Note})
	}
	return out
}
