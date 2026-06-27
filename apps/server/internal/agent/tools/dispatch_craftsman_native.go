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
	Brief           string                 `json:"brief" jsonschema:"required" jsonschema_description:"一句话描述调用该工具的意图，例如生成机场晨光统一参考图或直接生成所有分镜预览图。不要超过 160 个中文字符。"`
	Scope           DispatchCraftsmanScope `json:"scope" jsonschema:"required" jsonschema_description:"生产归属范围。shot 表示分镜图或分镜视频；key_element_state 表示共享参考图。scope.type=shot 且 key 为空时，可配合 shot_refs 批量派发；scope.type=key_element_state 时 key 填 read_project_context 返回的 semantic_key。"`
	ShotRefs        []string               `json:"shot_refs" jsonschema_description:"scope.type=shot 时可填写分镜 semantic_key 或稳定 client_key。为空表示派发所有可生成的 active shots。scope.type=key_element_state 时必须留空。"`
	TargetPhase     string                 `json:"target_phase" jsonschema:"required,enum=reference_image,enum=preview_image,enum=shot_video" jsonschema_description:"生成阶段。reference_image 生成 KeyElementState 统一参考图；preview_image 生成分镜预览图；shot_video 基于已确认预览图生成分镜视频。"`
	Mode            string                 `json:"mode" jsonschema:"enum=preview_image,enum=shot_video" jsonschema_description:"兼容旧参数；新调用请使用 target_phase。"`
	ExecutionPolicy string                 `json:"execution_policy" jsonschema:"required,enum=execute_immediately,enum=wait_for_producer" jsonschema_description:"执行策略。execute_immediately 表示 Craftsman 编译 RenderPlan 后工程自动提交 Worker；wait_for_producer 表示只编译并等待 Producer 后续 accept/reject。"`
	Force           bool                   `json:"force" jsonschema_description:"为 true 时即使已有完成结果也创建新尝试；默认 false。不能用于绕过正在排队或运行中的同 scope/target_phase Craftsman 任务。"`
	MaxAttempts     int32                  `json:"max_attempts" jsonschema_description:"Craftsman 最大尝试次数，范围 1 到 3；为空时默认 3。"`
	Critique        string                 `json:"critique" jsonschema_description:"可选的评审意见或用户修改意见，Craftsman 必须在 RenderPlan 中回应。"`
	FixHints        []string               `json:"fix_hints" jsonschema_description:"可选的具体修复建议，例如保持行李箱银灰色、改成低机位跟拍。"`
	InputNodeRefs   []string               `json:"input_node_refs" jsonschema_description:"可选输入节点引用，例如上一个分镜尾帧或已确认预览图。没有明确依赖时留空。"`
}

type DispatchCraftsmanScope struct {
	Type string `json:"type" jsonschema:"required,enum=shot,enum=key_element_state" jsonschema_description:"scope 类型。shot 用于分镜预览图或分镜视频；key_element_state 用于共享参考图。"`
	ID   string `json:"id" jsonschema_description:"兼容旧字段。模型不要填写 UUID；请填写 read_project_context 返回的 semantic_key，或留空并用 shot_refs 批量派发 shot。"`
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
			"brief":               input.Brief,
			"scope":               map[string]any{"type": input.Scope.Type, "id": input.Scope.ID},
			"shot_refs":           input.ShotRefs,
			"target_phase":        targetPhaseFromNativeInput(input),
			"mode":                input.Mode,
			"execution_policy":    input.ExecutionPolicy,
			"parent_tool_call_id": runtime.ToolCallID,
			"force":               input.Force,
			"max_attempts":        input.MaxAttempts,
			"critique":            input.Critique,
			"fix_hints":           input.FixHints,
			"input_node_refs":     input.InputNodeRefs,
		},
	})
	if err != nil {
		return NaturalToolError("dispatch_craftsman", err.Error(), "请读取项目上下文，确认分镜状态、mode 和 shot_refs 后重试。"), nil
	}
	return NaturalResult{
		Title: "Craftsman 派发结果",
		Items: []NaturalResultItem{
			{Label: "阶段", Value: targetPhaseFromNativeInput(input)},
			{Label: "摘要", Value: output.Summary},
		},
		Next: "任务已经入队。请稍后读取项目上下文或观察画布状态，不要把派发结果当成图片或视频已经完成。",
	}.String(), nil
}

func validateDispatchCraftsmanInput(input DispatchCraftsmanToolInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if err := requireMode(input.Scope.Type, "shot", "key_element_state"); err != nil {
		return err
	}
	targetPhase := targetPhaseFromNativeInput(input)
	if err := requireMode(targetPhase, "reference_image", "preview_image", "shot_video"); err != nil {
		return err
	}
	if input.Scope.Type == "key_element_state" && input.Scope.ID == "" {
		return fmt.Errorf("scope.id 必填；key_element_state 参考图任务请填写 read_project_context 返回的 semantic_key")
	}
	if input.Scope.Type == "key_element_state" && targetPhase != "reference_image" {
		return fmt.Errorf("key_element_state 只能派发 reference_image")
	}
	if input.Scope.Type == "shot" && targetPhase == "reference_image" {
		return fmt.Errorf("shot 不能派发 reference_image")
	}
	if err := requireMode(input.ExecutionPolicy, "execute_immediately", "wait_for_producer"); err != nil {
		return err
	}
	if input.MaxAttempts < 0 || input.MaxAttempts > 3 {
		return fmt.Errorf("max_attempts 必须在 1 到 3 之间，或留空使用默认值")
	}
	return nil
}

func targetPhaseFromNativeInput(input DispatchCraftsmanToolInput) string {
	if input.TargetPhase != "" {
		return input.TargetPhase
	}
	return input.Mode
}
