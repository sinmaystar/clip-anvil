package tools

import (
	"context"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type DispatchCraftsmanNativeTool struct {
	legacy DispatchCraftsmanTool
}

type DispatchCraftsmanToolInput struct {
	ShotRefs      []string `json:"shot_refs" jsonschema_description:"要派发的分镜 UUID 或稳定 client_key。为空表示派发所有可生成的 active planned shots。"`
	Mode          string   `json:"mode" jsonschema:"required,enum=preview_image,enum=shot_video" jsonschema_description:"生成阶段。preview_image 生成分镜预览图；shot_video 基于已确认预览图生成分镜视频。"`
	Force         bool     `json:"force" jsonschema_description:"为 true 时即使已有结果也创建新尝试；默认 false。"`
	MaxAttempts   int32    `json:"max_attempts" jsonschema_description:"Craftsman 最大尝试次数，范围 1 到 3；为空时默认 3。"`
	Critique      string   `json:"critique" jsonschema_description:"可选的评审意见或用户修改意见，Craftsman 必须在 RenderPlan 中回应。"`
	FixHints      []string `json:"fix_hints" jsonschema_description:"可选的具体修复建议，例如保持行李箱银灰色、改成低机位跟拍。"`
	InputNodeRefs []string `json:"input_node_refs" jsonschema_description:"可选输入节点引用，例如上一个分镜尾帧或已确认预览图。没有明确依赖时留空。"`
}

func NewDispatchCraftsmanNativeTool(store CraftsmanDispatcherStore, runtime CraftsmanRuntime, enqueuer CraftsmanTaskEnqueuer) DispatchCraftsmanNativeTool {
	return DispatchCraftsmanNativeTool{legacy: NewDispatchCraftsmanTool(store, runtime, enqueuer)}
}

func (t DispatchCraftsmanNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[DispatchCraftsmanToolInput](
		"dispatch_craftsman",
		"把 Producer 已确认的 ClipAnvil 分镜生成任务派发给 Craftsman。工具只创建持久化任务并入队，不直接生成图片或视频；Craftsman 会继续创建 RenderPlan、绑定参考资源并触发后续生产链路。",
	)
}

func (t DispatchCraftsmanNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs("dispatch_craftsman", argumentsInJSON, validateDispatchCraftsmanInput)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, "dispatch_craftsman")
	if !ok {
		return msg, nil
	}
	output, err := t.legacy.Execute(ctx, ExecuteInput{
		WorkspaceID: runtime.WorkspaceID,
		ThreadID:    runtime.ThreadID,
		TaskID:      runtime.TaskID,
		Arguments: map[string]any{
			"shot_refs":       input.ShotRefs,
			"mode":            input.Mode,
			"force":           input.Force,
			"max_attempts":    input.MaxAttempts,
			"critique":        input.Critique,
			"fix_hints":       input.FixHints,
			"input_node_refs": input.InputNodeRefs,
		},
	})
	if err != nil {
		return NaturalToolError("dispatch_craftsman", err.Error(), "请读取项目上下文，确认分镜状态、mode 和 shot_refs 后重试。"), nil
	}
	return NaturalResult{
		Title: "已派发 Craftsman 任务",
		Items: []NaturalResultItem{
			{Label: "阶段", Value: input.Mode},
			{Label: "摘要", Value: output.Summary},
		},
		Next: "任务已经入队。请稍后读取项目上下文或观察画布状态，不要把派发结果当成图片或视频已经完成。",
	}.String(), nil
}

func validateDispatchCraftsmanInput(input DispatchCraftsmanToolInput) error {
	if err := requireMode(input.Mode, "preview_image", "shot_video"); err != nil {
		return err
	}
	if input.MaxAttempts < 0 || input.MaxAttempts > 3 {
		return fmt.Errorf("max_attempts 必须在 1 到 3 之间，或留空使用默认值")
	}
	return nil
}
