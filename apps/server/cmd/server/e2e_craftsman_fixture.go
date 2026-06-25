package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	agentcraftsman "github.com/sinmaystar/clip-anvil/internal/agent/craftsman"
)

type e2eM2RenderPlanCraftsmanResponder struct{}

func (e2eM2RenderPlanCraftsmanResponder) Respond(_ context.Context, craftsmanContext agentcraftsman.Context) (agentcraftsman.CraftsmanTurnOutput, error) {
	switch e2eCraftsmanToolResultCount(craftsmanContext.SameTurnMessages) {
	case 0:
		return e2eCraftsmanToolCallOutput("e2e-read-project-memory-"+craftsmanContext.Shot.ClientKey, "read_project_memory", `{"include_prompt_hints":true}`), nil
	case 1:
		return e2eCraftsmanToolCallOutput("e2e-upsert-render-plan-"+craftsmanContext.Shot.ClientKey, "upsert_render_plan", e2eRenderPlanArgs(craftsmanContext)), nil
	default:
		return agentcraftsman.CraftsmanTurnOutput{
			AssistantText: "已为分镜创建 Seedream 预览图 RenderPlan，并完成 PromptCompiler 校验。",
			Metadata:      map[string]any{"e2e_fixture": "m2_render_plan"},
		}, nil
	}
}

func e2eCraftsmanToolResultCount(messages []agentcraftsman.CraftsmanSameTurnMessage) int {
	count := 0
	for _, message := range messages {
		if message.Role == "tool" {
			count++
		}
	}
	return count
}

func e2eCraftsmanToolCallOutput(id string, name string, arguments string) agentcraftsman.CraftsmanTurnOutput {
	return agentcraftsman.CraftsmanTurnOutput{
		ModelMessage: &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:   id,
				Type: "function",
				Function: schema.FunctionCall{
					Name:      name,
					Arguments: arguments,
				},
			}},
		},
		Metadata: map[string]any{"e2e_fixture": "m2_render_plan"},
	}
}

func e2eRenderPlanArgs(c agentcraftsman.Context) string {
	shotID := uuidString(c.Shot.ID)
	clientKey := strings.TrimSpace(c.Shot.ClientKey)
	title := strings.TrimSpace(c.Shot.Title)
	if title == "" {
		title = "机场广告分镜"
	}
	objective := fmt.Sprintf("为%s创建竖屏商业广告预览图。", title)
	setting := "现代机场出发大厅，玻璃幕墙，清晨柔和自然光，干净地面和商务出行氛围。"
	subject := "银灰色悦行行李箱作为核心商品，外观干净高级，四轮顺滑，商务旅客自然拉行。"
	action := strings.TrimSpace(c.Shot.ActionText)
	if action == "" {
		action = "商务旅客拉着悦行行李箱穿过机场大厅。"
	}
	camera := strings.TrimSpace(c.Shot.CameraIntent)
	if camera == "" {
		camera = "中景跟拍，轻微低机位，主体清晰。"
	}
	return fmt.Sprintf(`{
		"brief":"为分镜 %s 创建 Seedream 预览图 RenderPlan。",
		"mode":"create",
		"scope":{"type":"shot","id":%q},
		"target_phase":"preview_image",
		"task_type":"generate",
		"model_prompt_profile":"seedream_5_image",
		"operation":"text_to_image",
		"reference_bindings":[],
		"subject_bindings":[
			{
				"subject_key":"subject_yuexing_luggage",
				"label":"悦行银灰色行李箱",
				"prompt_handle":"主体1",
				"stable_traits":["银灰色硬壳","现代商务质感","四个万向轮","干净高级外观"],
				"must_preserve":true,
				"ambiguity_notes":"不要变成黑色软包、背包或竞品行李箱。"
			}
		],
		"prompt_parts":{
			"objective":%q,
			"subject":%q,
			"setting":%q,
			"action":%q,
			"camera":%q,
			"composition":"竖屏 9:16 构图，行李箱位于视觉中心，机场空间作为可信背景。",
			"style":"现代商业广告质感，真实摄影风格，干净高级。",
			"lighting":"清晨自然光，柔和反射，商品边缘清晰。",
			"quality_pack":["高清","商业广告质感","主体清晰","画面稳定"],
			"constraint_pack":["保持悦行行李箱外观一致","机场背景干净现代","不要出现竞品 Logo"],
			"negative_hints":["不要低质感背景","不要改变行李箱颜色","不要畸形轮子","不要多余文字"]
		},
		"params":{
			"ratio":"9:16",
			"resolution":"1080p",
			"watermark":false,
			"max_images":1
		},
		"audit_hints":{
			"auto_filled":["根据 ProjectMemory 补全机场晨光和商业广告质感。"],
			"consistency_risks":["E2E fixture 未绑定真实商品素材，仅验证 RenderPlan 链路。"],
			"prompt_compiler_notes":["把悦行行李箱作为主体1处理。"]
		},
		"rationale":"%s 分镜需要先生成可审阅的预览图；该 RenderPlan 把 Producer 的创意级事实转为 Seedream 可编译 prompt，并保留商品一致性约束。"
	}`, clientKey, shotID, objective, subject, setting, action, camera, clientKey)
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
