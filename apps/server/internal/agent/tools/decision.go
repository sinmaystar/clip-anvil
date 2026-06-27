package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type DecisionRequester interface {
	RequestUserDecision(ctx context.Context, input ExecuteInput) (ExecuteOutput, error)
}

type RequestUserDecisionTool struct {
	requester DecisionRequester
}

func NewRequestUserDecisionTool(requester DecisionRequester) RequestUserDecisionTool {
	return RequestUserDecisionTool{requester: requester}
}

func (t RequestUserDecisionTool) Definition() Definition {
	return Definition{
		Name:        "request_user_decision",
		Description: "Ask the user to make a decision before continuing. This creates a persisted decision card, checkpoint, and waiting_for_user task state.",
		Parameters: objectSchema(map[string]any{
			"brief":   map[string]any{"type": "string", "minLength": 1, "maxLength": 160, "description": "一句话描述调用该工具的意图，例如请用户确认是否立即生成机场预览图。"},
			"title":   map[string]any{"type": "string", "minLength": 1, "maxLength": 120},
			"message": map[string]any{"type": "string", "minLength": 1, "maxLength": 2000},
			"options": map[string]any{
				"type":     "array",
				"maxItems": 6,
				"items": objectSchema(map[string]any{
					"id":          map[string]any{"type": "string", "minLength": 1, "maxLength": 80},
					"label":       map[string]any{"type": "string", "minLength": 1, "maxLength": 120},
					"description": map[string]any{"type": "string", "maxLength": 300},
				}),
			},
			"allow_free_text": map[string]any{"type": "boolean"},
		}),
		Result: map[string]any{"type": "object"},
		Safety: SafetySpec{
			RequiresHITL:    true,
			MaxCallsPerTurn: 1,
		},
		Visibility: VisibilitySpec{
			ShowCallMessage: true,
			UserLabel:       "请求用户决策",
		},
	}
}

func (t RequestUserDecisionTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	if t.requester == nil {
		return ExecuteOutput{}, errors.New("request_user_decision service is not configured")
	}
	return t.requester.RequestUserDecision(ctx, input)
}

type RequestUserDecisionNativeTool struct {
	requester DecisionRequester
}

type RequestUserDecisionInput struct {
	Brief         string                 `json:"brief" jsonschema:"required" jsonschema_description:"一句话描述调用该工具的意图，例如请用户确认是否立即生成机场预览图。不要超过 160 个中文字符。"`
	Title         string                 `json:"title" jsonschema:"required" jsonschema_description:"决策卡标题，必须直接说明用户要确认什么。"`
	Message       string                 `json:"message" jsonschema:"required" jsonschema_description:"给用户看的决策说明，包含背景、影响和需要用户选择的原因。"`
	Options       []DecisionOptionInput  `json:"options" jsonschema_description:"可选项列表。需要用户在固定选项中选择时填写；每个选项必须有稳定 id 和清晰 label。"`
	AllowFreeText bool                   `json:"allow_free_text" jsonschema_description:"是否允许用户用自由文本补充或替代选项。"`
	Metadata      map[string]interface{} `json:"metadata" jsonschema_description:"可选内部上下文，例如关联的 shot_id、render_plan_id 或风险说明。"`
}

type DecisionOptionInput struct {
	ID          string `json:"id" jsonschema:"required" jsonschema_description:"稳定选项 ID，例如 approve、revise、cancel。"`
	Label       string `json:"label" jsonschema:"required" jsonschema_description:"展示给用户的短标签。"`
	Description string `json:"description" jsonschema_description:"选项的影响说明。"`
}

func NewRequestUserDecisionNativeTool(requester DecisionRequester) *RequestUserDecisionNativeTool {
	return &RequestUserDecisionNativeTool{requester: requester}
}

func (t *RequestUserDecisionNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[RequestUserDecisionInput](
		"request_user_decision",
		"当 Producer 遇到关键歧义、高成本生成、用户必须确认的参考资源或方向变化时，请求用户决策。工具会创建可交互决策卡并通过 Eino 原生 interrupt 暂停当前 Producer 图，直到用户 resume。",
	)
}

func (t *RequestUserDecisionNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	if wasInterrupted, _, state := einotool.GetInterruptState[map[string]any](ctx); wasInterrupted {
		if isResumeTarget, hasData, data := einotool.GetResumeContext[map[string]any](ctx); isResumeTarget {
			items := []NaturalResultItem{{Label: "状态", Value: "已收到用户决策，Producer 可以继续执行。"}}
			if hasData {
				items = append(items, NaturalResultItem{Label: "用户输入", Value: compactJSON(data)})
			}
			return NaturalResult{Title: "用户决策已返回", Items: items}.String(), nil
		}
		return "", einotool.StatefulInterrupt(ctx, map[string]any{"tool_name": "request_user_decision"}, state)
	}
	input, msg, ok := decodeToolArgs("request_user_decision", argumentsInJSON, validateRequestUserDecisionInput)
	if !ok {
		return msg, nil
	}
	if t.requester == nil {
		return NaturalToolError("request_user_decision", "HITL decision service 未配置。", "请检查服务端 wiring 后重试。"), nil
	}
	runtime, msg, ok := runtimeOrError(ctx, "request_user_decision")
	if !ok {
		return msg, nil
	}
	args := map[string]any{}
	if strings.TrimSpace(argumentsInJSON) != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			return NaturalToolError("request_user_decision", "参数 JSON 无法解析。", "请按工具 schema 重新生成参数。"), nil
		}
	}
	output, err := t.requester.RequestUserDecision(ctx, ExecuteInput{
		WorkspaceID: runtime.WorkspaceID,
		ThreadID:    runtime.ThreadID,
		TaskID:      runtime.TaskID,
		Arguments:   args,
	})
	if err != nil {
		return NaturalToolError("request_user_decision", err.Error(), "请检查决策卡参数、线程和任务状态后重试。"), nil
	}
	if !output.Interrupted {
		if strings.TrimSpace(output.Summary) != "" {
			return strings.TrimSpace(output.Summary), nil
		}
		return NaturalResult{Title: "用户决策请求已完成"}.String(), nil
	}
	interruptInfo := map[string]any{
		"tool_name":    "request_user_decision",
		"tool_call_id": runtime.ToolCallID,
		"summary":      strings.TrimSpace(output.Summary),
	}
	interruptState := map[string]any{
		"tool_name":    "request_user_decision",
		"tool_call_id": runtime.ToolCallID,
		"arguments":    args,
		"result":       output.Result,
		"title":        input.Title,
	}
	return "", einotool.StatefulInterrupt(ctx, interruptInfo, interruptState)
}

func validateRequestUserDecisionInput(input RequestUserDecisionInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if err := requireText(input.Title, "title"); err != nil {
		return err
	}
	if err := requireText(input.Message, "message"); err != nil {
		return err
	}
	for index, option := range input.Options {
		if strings.TrimSpace(option.ID) == "" {
			return fmt.Errorf("options[%d].id 不能为空", index)
		}
		if strings.TrimSpace(option.Label) == "" {
			return fmt.Errorf("options[%d].label 不能为空", index)
		}
	}
	return nil
}

func compactJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}
