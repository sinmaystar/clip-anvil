package tools

import (
	"context"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/creative"
)

type ReadProjectMemoryNativeTool struct {
	service *creative.Service
}

type ReadProjectMemoryToolInput struct {
	Brief                  string `json:"brief" jsonschema:"required" jsonschema_description:"读取 ProjectMemory 的目的，例如为分镜视频计划注入商品外观和机场商务氛围约束。不要超过 160 个中文字符。"`
	IncludePromptHints     bool   `json:"include_prompt_hints" jsonschema_description:"是否包含 prompt_injection_hints。Craftsman 写 RenderPlan 时通常需要 true。"`
	IncludeSourceRefs      bool   `json:"include_source_refs" jsonschema_description:"是否包含 memory 来源引用。只有需要解释约束来源时填写 true。"`
	IncludePreviousVersion bool   `json:"include_previous_version" jsonschema_description:"是否包含上一版本摘要。默认 false；只有处理 memory 变化导致的重做时使用。"`
}

func NewReadProjectMemoryNativeTool(service *creative.Service) *ReadProjectMemoryNativeTool {
	return &ReadProjectMemoryNativeTool{service: service}
}

func (t *ReadProjectMemoryNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[ReadProjectMemoryToolInput](toolReadProjectMemory, "读取当前 ClipAnvil ProjectMemory，也就是本项目所有分镜、RenderPlan、PromptCompiler 和 Reviewer 都必须遵守的创作宪法。这个工具只读，不会修改 memory。")
}

func (t *ReadProjectMemoryNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolReadProjectMemory, argumentsInJSON, validateReadProjectMemoryInput)
	if !ok {
		return msg, nil
	}
	if msg, ok := serviceOrError(t.service, toolReadProjectMemory); !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolReadProjectMemory)
	if !ok {
		return msg, nil
	}
	packet, err := t.service.ReadProjectContext(ctx, creative.ReadContextInput{
		WorkspaceID: runtime.WorkspaceID,
		ScopeType:   "workspace",
		DetailLevel: "summary",
		Include:     []string{"memory"},
	})
	if err != nil {
		return naturalErrorFromErr(toolReadProjectMemory, err), nil
	}
	if packet.Memory == nil {
		return NaturalResult{
			Title: "未找到 active ProjectMemory",
			Next:  "请让 Producer 先通过 update_project_memory 创建项目创作宪法。",
		}.String(), nil
	}
	items := []NaturalResultItem{
		{Label: "版本", Value: fmt.Sprintf("v%d / %s", packet.Memory.Version, packet.Memory.Status)},
		{Label: "核心意图", Value: packet.Memory.CoreIntent},
		{Label: "创作灵魂", Value: packet.Memory.Soul},
		{Label: "品牌事实", Value: fmt.Sprintf("%d 字节", len(packet.Memory.BrandFacts))},
		{Label: "不可妥协约束", Value: fmt.Sprintf("%d 字节", len(packet.Memory.NonNegotiables))},
		{Label: "视觉锚点", Value: fmt.Sprintf("%d 字节", len(packet.Memory.VisualAnchors))},
	}
	if input.IncludePromptHints {
		items = append(items, NaturalResultItem{Label: "Prompt 注入提示", Value: string(packet.Memory.PromptInjectionHints)})
	}
	if input.IncludeSourceRefs {
		items = append(items, NaturalResultItem{Label: "来源引用", Value: string(packet.Memory.SourceRefs)})
	}
	return NaturalResult{
		Title: "已读取 ProjectMemory",
		Items: items,
		Next:  "写 RenderPlan 时必须继承这些约束；如果冲突，标记 blocked 并交给 Producer。",
	}.String(), nil
}

func validateReadProjectMemoryInput(input ReadProjectMemoryToolInput) error {
	return requireText(input.Brief, "brief")
}
