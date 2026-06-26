package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/creative"
	"github.com/sinmaystar/clip-anvil/internal/agent/renderplan"
)

const (
	toolReadProjectContext = "read_project_context"
	toolUpsertBrief        = "upsert_project_brief"
	toolUpdateMemory       = "update_project_memory"
	toolUpsertElements     = "upsert_key_elements"
	toolUpsertStoryboard   = "upsert_storyboard"
	toolReadProjectMemory  = "read_project_memory"
	toolUpsertRenderPlan   = "upsert_render_plan"
	toolDecideRenderPlan   = "decide_render_plan"
	toolDispatchReviewer   = "dispatch_reviewer"
	toolSubmitReviewResult = "submit_review_result"
)

func toolInfoFor[T any](name string, desc string) (*schema.ToolInfo, error) {
	params, err := toolutils.GoStruct2ParamsOneOf[T]()
	if err != nil {
		return nil, err
	}
	return &schema.ToolInfo{Name: name, Desc: desc, ParamsOneOf: params}, nil
}

func decodeToolArgs[T any](toolName string, raw string, validate func(T) error) (T, string, bool) {
	var input T
	if strings.TrimSpace(raw) == "" {
		return input, NaturalToolError(toolName, "工具参数不能为空。", "请按工具 schema 重新填写参数。"), false
	}
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return input, NaturalToolError(toolName, "参数不是合法 JSON："+err.Error(), "请重新生成合法 JSON 参数。"), false
	}
	if validate != nil {
		if err := validate(input); err != nil {
			return input, NaturalToolError(toolName, err.Error(), "请修正参数后重试，不要重复提交相同错误参数。"), false
		}
	}
	return input, "", true
}

func runtimeOrError(ctx context.Context, toolName string) (NativeRuntimeContext, string, bool) {
	runtime, ok := NativeRuntimeFromContext(ctx)
	if !ok || !runtime.WorkspaceID.Valid {
		return NativeRuntimeContext{}, NaturalToolError(toolName, "缺少 Producer runtime context，无法确定 workspace。", "请由 Producer graph 注入 workspace_id 后重试。"), false
	}
	return runtime, "", true
}

func serviceOrError(service *creative.Service, toolName string) (string, bool) {
	if service == nil {
		return NaturalToolError(toolName, "creative state service 未配置。", "请检查服务端 wiring 后重试。"), false
	}
	return "", true
}

func renderPlanServiceOrError(service *renderplan.Service, toolName string) (string, bool) {
	if service == nil {
		return NaturalToolError(toolName, "render plan service 未配置。", "请检查服务端 wiring 后重试。"), false
	}
	return "", true
}

func naturalErrorFromErr(toolName string, err error) string {
	if err == nil {
		return ""
	}
	retry := "请读取当前项目上下文，修正参数后重试。"
	if errors.Is(err, creative.ErrAgentWorkspaceRequired) {
		retry = "请确认当前 workspace 是 Agent 模式。"
	}
	if errors.Is(err, creative.ErrCreativeStateNotFound) {
		retry = "请先创建被引用的 brief、key element、scene 或 shot。"
	}
	if errors.Is(err, renderplan.ErrInvalidInput) || errors.Is(err, renderplan.ErrInvalidState) {
		retry = "请按工具 schema 修正 RenderPlan 参数，必要时先读取当前项目上下文。"
	}
	return NaturalToolError(toolName, err.Error(), retry)
}

func requireText(value string, field string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s 必填", field)
	}
	return nil
}

func requireMode(value string, allowed ...string) error {
	for _, item := range allowed {
		if value == item {
			return nil
		}
	}
	return fmt.Errorf("mode 的值是 %q，但只支持 %s", value, strings.Join(allowed, "、"))
}

func ptrFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	next := *value
	return &next
}
