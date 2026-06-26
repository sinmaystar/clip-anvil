package tools

import (
	"context"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/creative"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ReadProjectContextNativeTool struct {
	service *creative.Service
}

type ReadProjectContextToolInput struct {
	Brief       string              `json:"brief" jsonschema:"required" jsonschema_description:"本次读取上下文的目的，例如判断是否需要创建 brief、memory、关键元素或 storyboard。不要超过 160 个中文字符。"`
	Scope       ProjectContextScope `json:"scope" jsonschema:"required" jsonschema_description:"读取范围。Producer 做全局决策时使用 workspace；修改局部分镜时使用 shot；检查场景时使用 scene；查看关键元素时使用 key_element。"`
	Include     []string            `json:"include" jsonschema_description:"要返回的对象类型。可选值包括 brief、memory、elements、scenes、shots、dependencies、render_plans、canvas_projection。为空时返回 Producer 默认上下文。"`
	DetailLevel string              `json:"detail_level" jsonschema:"enum=summary,enum=full" jsonschema_description:"summary 返回摘要，适合普通规划；full 返回完整当前事实，适合写入前决策。默认 summary。"`
}

type ProjectContextScope struct {
	Type string `json:"type" jsonschema:"required,enum=workspace,enum=scene,enum=shot,enum=key_element" jsonschema_description:"上下文范围类型。workspace 表示整个项目；scene 表示单个场景；shot 表示单个分镜；key_element 表示单个关键元素。"`
	ID   string `json:"id" jsonschema_description:"scope 对象 ID。type=workspace 时可以为空，由运行时 workspace 注入；其他类型必须填写 UUID。"`
}

func NewReadProjectContextNativeTool(service *creative.Service) *ReadProjectContextNativeTool {
	return &ReadProjectContextNativeTool{service: service}
}

func (t *ReadProjectContextNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[ReadProjectContextToolInput](toolReadProjectContext, "读取当前 ClipAnvil Agent workspace 的创作事实源，用于 Producer 在行动前理解 CreativeBrief、ProjectMemory、KeyElement、KeyElementState、Scene、Shot、shot_key_element、shot_dependency 和只读画布投影。")
}

func (t *ReadProjectContextNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolReadProjectContext, argumentsInJSON, validateReadProjectContextInput)
	if !ok {
		return msg, nil
	}
	if msg, ok := serviceOrError(t.service, toolReadProjectContext); !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolReadProjectContext)
	if !ok {
		return msg, nil
	}
	packet, err := t.service.ReadProjectContext(ctx, creative.ReadContextInput{
		WorkspaceID: runtime.WorkspaceID,
		ScopeType:   input.Scope.Type,
		ScopeID:     input.Scope.ID,
		Include:     input.Include,
		DetailLevel: input.DetailLevel,
	})
	if err != nil {
		return naturalErrorFromErr(toolReadProjectContext, err), nil
	}
	missingReferences := 0
	for _, state := range packet.ElementStates {
		if state.ReferenceStatus == "needs_reference" {
			missingReferences++
		}
	}
	briefStatus := "没有 active CreativeBrief"
	if packet.Brief != nil {
		briefStatus = packet.Brief.Title + " / " + packet.Brief.Status
	}
	memoryStatus := "没有 active ProjectMemory"
	if packet.Memory != nil {
		memoryStatus = fmt.Sprintf("v%d / %s", packet.Memory.Version, packet.Memory.Status)
	}
	renderPlanStatus := summarizeRenderPlans(packet.RenderPlans)
	return NaturalResult{
		Title: "已读取项目创作上下文",
		Items: []NaturalResultItem{
			{Label: "CreativeBrief", Value: briefStatus},
			{Label: "ProjectMemory", Value: memoryStatus},
			{Label: "关键元素", Value: fmt.Sprintf("%d 个，缺少参考 %d 个", len(packet.Elements), missingReferences)},
			{Label: "Storyboard", Value: fmt.Sprintf("%d 个场景，%d 个分镜，%d 个依赖", len(packet.Scenes), len(packet.Shots), len(packet.Dependencies))},
			{Label: "RenderPlan", Value: renderPlanStatus},
		},
		Next: "如果存在 waiting_for_approval RenderPlan，Producer 应调用 decide_render_plan accept/reject，或先派 Reviewer；否则根据缺失对象选择创作状态工具。",
	}.String(), nil
}

func summarizeRenderPlans(plans []db.RenderPlan) string {
	if len(plans) == 0 {
		return "0 个"
	}
	byStatus := map[string]int{}
	pending := make([]string, 0, 5)
	for _, plan := range plans {
		byStatus[plan.Status]++
		if plan.Status == "waiting_for_approval" && len(pending) < 5 {
			pending = append(pending, fmt.Sprintf("%s %s=%s phase=%s operation=%s", uuidString(plan.ID), plan.ScopeType, uuidString(plan.ScopeID), plan.TargetPhase, plan.Operation))
		}
	}
	parts := []string{fmt.Sprintf("%d 个", len(plans))}
	for _, status := range []string{"waiting_for_approval", "compiled", "submitted", "running", "succeeded", "failed", "rejected", "blocked", "draft"} {
		if count := byStatus[status]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", status, count))
		}
	}
	if len(pending) > 0 {
		parts = append(parts, "待决策："+strings.Join(pending, "；"))
	}
	return strings.Join(parts, "，")
}

func validateReadProjectContextInput(input ReadProjectContextToolInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if input.Scope.Type == "" {
		return requireText("", "scope.type")
	}
	if err := requireMode(input.Scope.Type, "workspace", "scene", "shot", "key_element"); err != nil {
		return err
	}
	if input.Scope.Type != "workspace" && input.Scope.ID == "" {
		return requireText("", "scope.id")
	}
	if input.DetailLevel != "" {
		return requireMode(input.DetailLevel, "summary", "full")
	}
	return nil
}
