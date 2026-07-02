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
type e2eTemplateOnlyVideoCraftsmanResponder struct{}

func (e2eTemplateOnlyVideoCraftsmanResponder) Respond(_ context.Context, craftsmanContext agentcraftsman.Context) (agentcraftsman.CraftsmanTurnOutput, error) {
	switch e2eCraftsmanToolResultCount(craftsmanContext.SameTurnMessages) {
	case 0:
		return e2eCraftsmanToolCallOutput("e2e-template-read-memory-"+e2eCraftsmanScopeKey(craftsmanContext), "read_project_memory", `{"brief":"读取 template_only/no-Seedance 视频和音频任务约束。","include_prompt_hints":true}`), nil
	case 1:
		switch craftsmanContext.Input.Mode {
		case "voiceover_audio":
			return e2eCraftsmanToolCallOutput("e2e-template-upsert-voiceover-"+e2eCraftsmanScopeKey(craftsmanContext), "upsert_render_plan", e2eVoiceoverRenderPlanArgs(craftsmanContext)), nil
		case "bgm_audio":
			return e2eCraftsmanToolCallOutput("e2e-template-upsert-bgm-"+e2eCraftsmanScopeKey(craftsmanContext), "upsert_render_plan", e2eBGMRenderPlanArgs(craftsmanContext)), nil
		case "preview_image", "reference_image":
			return e2eCraftsmanToolCallOutput("e2e-template-upsert-seedream-image-"+e2eCraftsmanScopeKey(craftsmanContext), "upsert_render_plan", e2eSeedreamTemplateAssetRenderPlanArgs(craftsmanContext)), nil
		default:
			return e2eCraftsmanToolCallOutput("e2e-template-upsert-render-plan-"+e2eCraftsmanScopeKey(craftsmanContext), "upsert_render_plan", e2eTemplateVideoRenderPlanArgs(craftsmanContext)), nil
		}
	default:
		return agentcraftsman.CraftsmanTurnOutput{
			AssistantText: "已创建 template_video 或 seed_audio_1 RenderPlan，并按 execute_immediately 提交 Worker。",
			Metadata:      map[string]any{"e2e_fixture": "template_only_video"},
		}, nil
	}
}

func (e2eM2RenderPlanCraftsmanResponder) Respond(_ context.Context, craftsmanContext agentcraftsman.Context) (agentcraftsman.CraftsmanTurnOutput, error) {
	switch e2eCraftsmanToolResultCount(craftsmanContext.SameTurnMessages) {
	case 0:
		return e2eCraftsmanToolCallOutput("e2e-read-project-memory-"+craftsmanContext.Shot.ClientKey, "read_project_memory", `{"brief":"读取分镜预览图 RenderPlan 需要遵守的 ProjectMemory 约束。","include_prompt_hints":true}`), nil
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

func e2eTemplateVideoRenderPlanArgs(c agentcraftsman.Context) string {
	shotID := uuidString(c.Shot.ID)
	clientKey := strings.TrimSpace(c.Shot.ClientKey)
	if clientKey == "" {
		clientKey = "shot_01_template_ad"
	}
	return fmt.Sprintf(`{
		"brief":"为分镜 %s 创建 no-Seedance HyperFrames template video RenderPlan。",
		"mode":"create",
		"scope":{"type":"shot","id":%q},
		"target_phase":"shot_video",
		"task_type":"generate",
		"model_prompt_profile":"template_video",
		"operation":"image_to_template_video",
		"generation_text":"生成 8 秒 9:16 悦行行李箱 4 段口播广告模板视频：使用用户上传的 box.png 产品图作为主视觉，画面轻微推进；第 1 段开场痛点：短途出行，行李箱别拖后腿；第 2 段产品展示：悦行行李箱轻便硬壳；第 3 段卖点卡：顺滑万向轮、安心托运；第 4 段 CTA：现在出发。禁止调用 Seedance，不要真实复杂运动或人物表演。",
		"params":{
			"ratio":"9:16",
			"duration_sec":8,
			"resolution":"1080p",
			"watermark":false,
			"template_key":"marketing_ad_4_scene_v1",
			"fps":24,
			"variables":{
				"headline":"悦行行李箱",
				"caption":"轻便好推｜顺滑万向轮｜短途出行更省心",
				"cta":"现在出发",
				"brand_colors":["#111827","#F5C542"],
				"scenes":[
					{"badge":"01","headline":"短途出行别费力","caption":"登机、出站、通勤，行李箱不该拖慢你。"},
					{"badge":"02","headline":"悦行行李箱","caption":"轻便硬壳，干净利落，短途刚刚好。"},
					{"badge":"03","headline":"顺滑万向轮","caption":"转弯更稳，推行更省力，安心托运。"},
					{"badge":"GO","headline":"现在出发","caption":"悦行行李箱，让短途出行更省心。"}
				]
			}
		},
		"audit_hints":{
			"auto_filled":["根据 template_only 策略使用 HyperFrames marketing_ad_4_scene_v1。"],
			"prompt_compiler_notes":["reference_bindings 由 dispatch_craftsman 的 input_node_refs 自动继承 box.png。"]
		},
		"rationale":"%s 分镜是卖点卡/CTA 型产品图轻动效广告，符合 template_video 能力边界；video_route_policy=template_only 禁止 Seedance。"
	}`, clientKey, shotID, clientKey)
}

func e2eSeedreamTemplateAssetRenderPlanArgs(c agentcraftsman.Context) string {
	shotID := uuidString(c.Shot.ID)
	clientKey := strings.TrimSpace(c.Shot.ClientKey)
	if clientKey == "" {
		clientKey = "shot_01_template_ad"
	}
	referenceBindings := `"reference_bindings":[]`
	if len(c.Input.InputNodeRefs) > 0 && strings.TrimSpace(c.Input.InputNodeRefs[0]) != "" {
		referenceBindings = fmt.Sprintf(`"reference_bindings":[{
			"client_key":"ref_product_box",
			"source_type":"media_node",
			"source_id":%q,
			"content_type":"image_url",
			"model_role":"reference_image",
			"prompt_alias":"商品参考图",
			"semantic_target":"悦行行李箱外观和商品主体",
			"priority":1,
			"required":true,
			"notes":"保持 box.png 中银灰色硬壳、竖向纹理和万向轮外观，生成更适合广告模板的视频主视觉图片。"
		}]`, strings.TrimSpace(c.Input.InputNodeRefs[0]))
	}
	return fmt.Sprintf(`{
		"brief":"为分镜 %s 创建 Seedream 商业主视觉图片 RenderPlan。",
		"mode":"create",
		"scope":{"type":"shot","id":%q},
		"target_phase":"preview_image",
		"task_type":"generate",
		"model_prompt_profile":"seedream_5_image",
		"operation":"image_to_image",
		"generation_text":"基于用户上传的 box.png 行李箱商品图，生成一张 9:16 竖版商业广告主视觉：银灰色悦行行李箱位于画面中上部，背景是干净现代的短途出行场景或抽象旅行空间，光线明亮高级，留出下方字幕和 CTA 区域；保持行李箱硬壳竖向纹理、四个万向轮和银灰色质感，不要出现人物、竞品 Logo 或难以阅读的文字。",
		%s,
		"prompt_parts":{
			"objective":"生成供 HyperFrames 模板视频使用的商业主视觉图片。",
			"subject":"银灰色悦行行李箱，硬壳竖向纹理，四个万向轮，外观参考 box.png。",
			"setting":"清爽现代的短途旅行广告背景，留出字幕空间。",
			"composition":"9:16 竖版，商品位于中上部，下方保留文字安全区。",
			"style":"真实商业广告摄影，高级、干净、明亮。",
			"lighting":"柔和棚拍光和轻微环境反射，商品边缘清晰。",
			"quality_pack":["高清","商业广告质感","商品清晰","字幕安全区干净"],
			"constraint_pack":["保持 box.png 商品外观","不要生成视频","不要出现竞品 Logo"],
			"negative_hints":["不要多余文字","不要人物遮挡商品","不要改变行李箱颜色","不要畸形轮子"]
		},
		"params":{
			"ratio":"9:16",
			"resolution":"2K",
			"watermark":false,
			"max_images":1
		},
		"audit_hints":{
			"auto_filled":["根据 template_only 策略，图片允许使用 Seedream，视频阶段仍禁止 Seedance。"],
			"prompt_compiler_notes":["该图片作为后续 HyperFrames template video 的产品主视觉输入。"]
		},
		"rationale":"%s 分镜需要先生成更有广告质感的静态主视觉图片，再交给 HyperFrames 做低成本视频合成；此步骤使用 Seedream 图片模型，不调用 Seedance 视频。"
	}`, clientKey, shotID, referenceBindings, clientKey)
}

func e2eVoiceoverRenderPlanArgs(c agentcraftsman.Context) string {
	audioPlanID := uuidString(c.AudioPlan.ID)
	if audioPlanID == "" {
		audioPlanID = uuidString(c.Input.ScopeID)
	}
	return fmt.Sprintf(`{
		"brief":"为悦行行李箱 8 秒模板广告生成中文旁白音频 RenderPlan。",
		"mode":"create",
		"scope":{"type":"audio_plan","id":%q},
		"target_phase":"voiceover_audio",
		"task_type":"generate",
		"model_prompt_profile":"seed_audio_1",
		"operation":"text_to_audio",
		"generation_text":"短途出行，行李箱别再拖后腿。悦行行李箱，轻便好推，顺滑万向轮转向更稳。安心托运，现在出发。",
		"prompt_parts":{
			"objective":"生成 8 秒中文广告旁白。",
			"narration":"短途出行，行李箱别再拖后腿。悦行行李箱，轻便好推，顺滑万向轮转向更稳。安心托运，现在出发。",
			"audio":"清爽可信的女声口播，语速略快但清晰，适合短视频广告。"
		},
		"params":{"speaker":"warm_female","format":"mp3","sample_rate":48000,"speech_rate":1.04,"watermark":false},
		"rationale":"AudioPlan 已批准，先生成 voiceover_audio 供 Composer 合成最终视频。"
	}`, audioPlanID)
}

func e2eBGMRenderPlanArgs(c agentcraftsman.Context) string {
	audioPlanID := uuidString(c.AudioPlan.ID)
	if audioPlanID == "" {
		audioPlanID = uuidString(c.Input.ScopeID)
	}
	return fmt.Sprintf(`{
		"brief":"为悦行行李箱 8 秒模板广告生成轻快 BGM RenderPlan。",
		"mode":"create",
		"scope":{"type":"audio_plan","id":%q},
		"target_phase":"bgm_audio",
		"task_type":"generate",
		"model_prompt_profile":"seed_audio_1",
		"operation":"text_to_audio",
		"generation_text":"生成 8 秒轻快电子流行 BGM，无人声，弱鼓点，明亮但不抢旁白，适合旅行行李箱短视频广告。",
		"prompt_parts":{
			"objective":"生成 8 秒广告背景音乐。",
			"audio":"轻快电子流行，无人声，弱鼓点，明亮清爽，给旁白留出空间。"
		},
		"params":{"format":"mp3","sample_rate":48000,"watermark":false},
		"rationale":"AudioPlan 已批准，生成 bgm_audio 并在最终合成时 duck 到旁白下方。"
	}`, audioPlanID)
}

func e2eCraftsmanScopeKey(c agentcraftsman.Context) string {
	if strings.TrimSpace(c.Shot.ClientKey) != "" {
		return c.Shot.ClientKey
	}
	if strings.TrimSpace(c.Input.Mode) != "" {
		return c.Input.Mode
	}
	return "task"
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
