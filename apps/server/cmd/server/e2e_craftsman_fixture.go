package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	agentcraftsman "github.com/sinmaystar/clip-anvil/internal/agent/craftsman"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type e2eM2RenderPlanCraftsmanResponder struct{}
type e2eMotionShotVideoCraftsmanResponder struct{}

func (e2eMotionShotVideoCraftsmanResponder) Respond(_ context.Context, craftsmanContext agentcraftsman.Context) (agentcraftsman.CraftsmanTurnOutput, error) {
	switch e2eCraftsmanToolResultCount(craftsmanContext.SameTurnMessages) {
	case 0:
		return e2eCraftsmanToolCallOutput("e2e-motion-read-memory-"+e2eCraftsmanScopeKey(craftsmanContext), "read_project_memory", `{"brief":"读取 motion_only/no-Seedance 视频和音频任务约束。","include_prompt_hints":true}`), nil
	case 1:
		switch craftsmanContext.Input.Mode {
		case "voiceover_audio":
			return e2eCraftsmanToolCallOutput("e2e-motion-upsert-voiceover-"+e2eCraftsmanScopeKey(craftsmanContext), "upsert_render_plan", e2eVoiceoverRenderPlanArgs(craftsmanContext)), nil
		case "bgm_audio":
			return e2eCraftsmanToolCallOutput("e2e-motion-upsert-bgm-"+e2eCraftsmanScopeKey(craftsmanContext), "upsert_render_plan", e2eBGMRenderPlanArgs(craftsmanContext)), nil
		case "preview_image", "reference_image":
			return e2eCraftsmanToolCallOutput("e2e-motion-upsert-seedream-image-"+e2eCraftsmanScopeKey(craftsmanContext), "upsert_render_plan", e2eSeedreamMotionAssetRenderPlanArgs(craftsmanContext)), nil
		default:
			return e2eCraftsmanToolCallOutput("e2e-motion-upsert-render-plan-"+e2eCraftsmanScopeKey(craftsmanContext), "upsert_render_plan", e2eMotionShotVideoRenderPlanArgs(craftsmanContext)), nil
		}
	default:
		return agentcraftsman.CraftsmanTurnOutput{
			AssistantText: "已创建 motion_shot_video 或 seed_audio_1 RenderPlan，并按 execute_immediately 提交 Worker。",
			Metadata:      map[string]any{"e2e_fixture": "motion_shot_video"},
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

func e2eMotionShotVideoRenderPlanArgs(c agentcraftsman.Context) string {
	shotID := uuidString(c.Shot.ID)
	clientKey := strings.TrimSpace(c.Shot.ClientKey)
	if clientKey == "" {
		clientKey = "shot_01_hook"
	}
	duration := e2eShotDuration(c.Shot)
	motionStyle, productMotion, textPosition, transitionIn, transitionOut := e2eMotionShotVariant(clientKey)
	headline, benefitText := e2eMotionShotUserText(clientKey)
	generationText := fmt.Sprintf("生成 %d 秒 9:16 悦行行李箱 Remotion motion shot：画面必须服务该 shot 的旁白“%s”；只输出无声 shot_video，完整旁白、BGM、底部字幕同步交给 Composer。不要把 shot title、narrative_purpose、visual_intent、action_text 这类内部导演说明写进画面文字。禁止调用 Seedance；video_route_policy=motion_only。", duration, firstNonEmptyString(strings.TrimSpace(c.Shot.Narration), benefitText))
	return fmt.Sprintf(`{
		"brief":"为分镜 %s 创建 no-Seedance Remotion motion shot RenderPlan。",
		"mode":"create",
		"scope":{"type":"shot","id":%q},
		"target_phase":"shot_video",
		"task_type":"generate",
		"model_prompt_profile":"motion_shot_video",
		"operation":"image_to_motion_video",
		"generation_text":%q,
		"params":{
			"ratio":"9:16",
			"duration_sec":%d,
			"resolution":"1080p",
			"watermark":false,
			"fps":30,
			"motion_style":%q,
			"safe_area":"caption_reserved_bottom",
			"brand_colors":["#111827","#F5C542"],
			"visual_layers":[
				{"id":"product_hero","input_ref":"primary_image","role":"product","start_sec":0,"end_sec":%d,"motion":%q,"fit":"contain"},
				{"id":"warm_glow","role":"background","start_sec":0,"end_sec":%d,"motion":"ambient_gradient","fit":"cover"}
			],
			"text_layers":[
				{"role":"hook","text":%q,"start_sec":0.2,"end_sec":2.2,"animation":"pop_slide_up","position":%q},
				{"role":"benefit","text":%q,"start_sec":2.2,"end_sec":%0.1f,"animation":"fade_rise","position":"side_label"}
			],
			"transitions":{"in":%q,"out":%q}
		},
		"audit_hints":{
			"route_policy":"video_route_policy=motion_only",
			"auto_filled":["根据 motion_only 策略使用 internal_motion_video/remotion-motion-shot-v1。"],
			"prompt_compiler_notes":["reference_bindings 由 dispatch_craftsman 的 input_node_refs 自动继承 Seedream 主视觉图片；Composer 负责旁白、BGM、字幕同步。"]
		},
		"rationale":"%s 分镜是卖点卡/CTA 型产品图轻动效广告，符合 motion_shot_video 能力边界；video_route_policy=motion_only 禁止 Seedance。"
	}`, clientKey, shotID, generationText, duration, motionStyle, duration, productMotion, duration, headline, textPosition, benefitText, float64(duration)-0.3, transitionIn, transitionOut, clientKey)
}

func e2eSeedreamMotionAssetRenderPlanArgs(c agentcraftsman.Context) string {
	shotID := uuidString(c.Shot.ID)
	clientKey := strings.TrimSpace(c.Shot.ClientKey)
	if clientKey == "" {
		clientKey = "shot_01_hook"
	}
	title := firstNonEmptyString(strings.TrimSpace(c.Shot.Title), "悦行行李箱")
	visualIntent := firstNonEmptyString(strings.TrimSpace(c.Shot.VisualIntent), "清爽现代的短途旅行广告背景，留出字幕空间。")
	visualSpec := e2eSeedreamShotVisualSpec(clientKey, title, visualIntent)
	referenceBindings := `"reference_bindings":[]`
	if len(c.Input.InputNodeRefs) > 0 && strings.TrimSpace(c.Input.InputNodeRefs[0]) != "" {
		referenceBindings = fmt.Sprintf(`"reference_bindings":[{
			"client_key":"ref_product_box",
			"source_type":"media_node",
			"source_id":%q,
			"content_type":"image_url",
			"model_role":"reference_image",
			"prompt_alias":"商品参考图",
			"semantic_target":%q,
			"priority":1,
			"required":true,
			"notes":%q
		}]`, strings.TrimSpace(c.Input.InputNodeRefs[0]), visualSpec.ReferenceTarget, visualSpec.ReferenceNotes)
	}
	return fmt.Sprintf(`{
		"brief":"为分镜 %s 创建 Seedream 商业主视觉图片 RenderPlan。",
		"mode":"create",
		"scope":{"type":"shot","id":%q},
		"target_phase":"preview_image",
		"task_type":"generate",
		"model_prompt_profile":"seedream_5_image",
		"operation":"image_to_image",
		"generation_text":%q,
		%s,
		"prompt_parts":{
			"objective":"生成供 Remotion motion shot 使用的商业主视觉图片。",
			"subject":%q,
			"setting":%q,
			"composition":%q,
			"style":"真实商业广告摄影，高级、干净、明亮。",
			"lighting":"柔和棚拍光和轻微环境反射，商品边缘清晰。",
			"quality_pack":["高清","商业广告质感","商品清晰","字幕安全区干净"],
			"constraint_pack":%s,
			"negative_hints":%s
		},
		"params":{
			"ratio":"9:16",
			"resolution":"2K",
			"watermark":false,
			"max_images":1
		},
		"audit_hints":{
			"auto_filled":["根据 motion_only 策略，图片允许使用 Seedream，视频阶段仍禁止 Seedance。"],
			"prompt_compiler_notes":["该图片作为后续 Remotion motion shot 的产品主视觉输入。"]
		},
		"rationale":"%s 分镜需要先生成更有广告质感的静态主视觉图片，再交给 Remotion 做低成本视频合成；此步骤使用 Seedream 图片模型，不调用 Seedance 视频。"
	}`, clientKey, shotID, visualSpec.GenerationText, referenceBindings, visualSpec.Subject, visualSpec.Setting, visualSpec.Composition, e2eJSONStringArray(visualSpec.Constraints), e2eJSONStringArray(visualSpec.NegativeHints), clientKey)
}

type e2eSeedreamVisualSpec struct {
	GenerationText  string
	Subject         string
	Setting         string
	Composition     string
	ReferenceTarget string
	ReferenceNotes  string
	Constraints     []string
	NegativeHints   []string
}

func e2eSeedreamShotVisualSpec(clientKey string, title string, visualIntent string) e2eSeedreamVisualSpec {
	switch clientKey {
	case "shot_03_wheels":
		return e2eSeedreamVisualSpec{
			GenerationText:  fmt.Sprintf("基于用户上传的 box.png 行李箱商品图，为“%s”生成一张 9:16 竖版商业广告主视觉：底部万向轮超近景特写，轮组占画面 55%% 以上，箱体只露出下沿；画面必须服务旁白中的顺滑万向轮、转向稳定、推行省力，不要生成完整行李箱大图，不要出现人物、竞品 Logo 或难以阅读的文字。", title),
			Subject:         "银灰色悦行行李箱底部万向轮特写，参考 box.png 的硬壳材质、竖向纹理和轮子造型。",
			Setting:         "干净棚拍背景，底部轮组清晰可见，画面突出轮子和箱体下沿。",
			Composition:     "9:16 竖版，万向轮和箱体底部位于画面中下到中央区域，轮组占主要视觉面积，下方保留字幕安全区。",
			ReferenceTarget: "悦行行李箱底部万向轮外观",
			ReferenceNotes:  "保持 box.png 的银灰硬壳、竖向纹理和轮子造型，但本 shot 需要轮组特写，不要求完整箱体入镜。",
			Constraints:     []string{"保持 box.png 商品材质和轮子外观", "生成静态图片，不要生成视频", "万向轮必须清晰且占画面主体", "不要出现竞品 Logo"},
			NegativeHints:   []string{"不要完整行李箱大图", "不要多余文字", "不要人物遮挡商品", "不要改变行李箱颜色", "不要畸形轮子"},
		}
	case "shot_04_storage":
		return e2eSeedreamVisualSpec{
			GenerationText:  fmt.Sprintf("基于用户上传的 box.png 行李箱商品图，为“%s”生成一张 9:16 竖版商业广告主视觉：打开的银灰色行李箱内景，衣物、电脑、洗漱包分区摆放整齐，突出两三天短途收纳能力；不要出现人物、竞品 Logo 或难以阅读的文字。", title),
			Subject:         "打开的银灰色悦行行李箱内部分区收纳图，外壳材质参考 box.png，内部放有衣物、电脑和洗漱包。",
			Setting:         "明亮干净棚拍背景，行李箱打开平放，内部空间和分区结构清晰。",
			Composition:     "9:16 竖版，打开的箱体占画面中部，内部收纳物品清楚，下方保留字幕安全区。",
			ReferenceTarget: "悦行行李箱外观和内部分区收纳",
			ReferenceNotes:  "保持 box.png 的银灰硬壳和品牌外观，本 shot 可以生成打开箱体内景，用于表现收纳容量。",
			Constraints:     []string{"保持 box.png 商品外观", "生成静态图片，不要生成视频", "内部收纳分区必须清晰", "不要出现竞品 Logo"},
			NegativeHints:   []string{"不要万向轮特写", "不要多余文字", "不要人物遮挡商品", "不要杂乱收纳", "不要改变行李箱颜色"},
		}
	default:
		return e2eSeedreamVisualSpec{
			GenerationText:  fmt.Sprintf("基于用户上传的 box.png 行李箱商品图，为“%s”生成一张 9:16 竖版商业广告主视觉：%s；保持行李箱硬壳竖向纹理、四个万向轮和银灰色质感，不要出现人物、竞品 Logo 或难以阅读的文字。", title, visualIntent),
			Subject:         "银灰色悦行行李箱，硬壳竖向纹理，四个万向轮，外观参考 box.png。",
			Setting:         visualIntent,
			Composition:     "9:16 竖版，商品位于中上部，下方保留文字安全区。",
			ReferenceTarget: "悦行行李箱外观和商品主体",
			ReferenceNotes:  "保持 box.png 中银灰色硬壳、竖向纹理和万向轮外观，生成更适合 Remotion motion shot 的视频主视觉图片。",
			Constraints:     []string{"保持 box.png 商品外观", "不要生成视频", "不要出现竞品 Logo"},
			NegativeHints:   []string{"不要多余文字", "不要人物遮挡商品", "不要改变行李箱颜色", "不要畸形轮子"},
		}
	}
}

func e2eJSONStringArray(values []string) string {
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func e2eVoiceoverRenderPlanArgs(c agentcraftsman.Context) string {
	audioPlanID := uuidString(c.AudioPlan.ID)
	if audioPlanID == "" {
		audioPlanID = uuidString(c.Input.ScopeID)
	}
	mode, forkFrom := e2eRenderPlanWriteMode(c, "voiceover_audio")
	voiceoverScript := "短途出行，最怕箱子沉、转弯卡，一路拖得很狼狈；地铁换乘和酒店大厅，每一步都怕被行李拖慢。悦行行李箱采用轻量硬壳和顺滑手感，从地铁口到酒店前台都推得更稳。底部万向轮顺滑转向，转弯不抢手，狭窄通道也能轻松掉头，赶车换乘更省力。两三天换洗衣物、电脑和洗漱包分区放好，拉链网袋一眼看清，打开就能快速拿取。周末旅行、商务通勤、短途回家，一个箱子装下刚刚好的从容。悦行行李箱，现在出发。"
	return fmt.Sprintf(`{
		"brief":"为悦行行李箱 34 秒动态多分镜广告生成中文旁白音频 RenderPlan。",
		"mode":%q,
		%s
		"scope":{"type":"audio_plan","id":%q},
		"target_phase":"voiceover_audio",
		"task_type":"generate",
		"model_prompt_profile":"seed_audio_1",
		"operation":"text_to_audio",
		"generation_text":%q,
		"prompt_parts":{
			"objective":"生成 34 秒中文广告旁白。",
			"narration":%q,
			"audio":"清爽可信的女声口播，语速略快但清晰，适合短视频广告。"
		},
		"params":{"speaker":"warm_female","format":"mp3","sample_rate":48000,"speech_rate":0.98,"watermark":false},
		"rationale":"AudioPlan 已批准，先生成 voiceover_audio 供 Composer 合成最终视频。"
	}`, mode, forkFrom, audioPlanID, voiceoverScript, voiceoverScript)
}

func e2eBGMRenderPlanArgs(c agentcraftsman.Context) string {
	audioPlanID := uuidString(c.AudioPlan.ID)
	if audioPlanID == "" {
		audioPlanID = uuidString(c.Input.ScopeID)
	}
	mode, forkFrom := e2eRenderPlanWriteMode(c, "bgm_audio")
	return fmt.Sprintf(`{
		"brief":"为悦行行李箱 34 秒动态多分镜广告生成轻快 BGM RenderPlan。",
		"mode":%q,
		%s
		"scope":{"type":"audio_plan","id":%q},
		"target_phase":"bgm_audio",
		"task_type":"generate",
		"model_prompt_profile":"seed_audio_1",
		"operation":"text_to_audio",
		"generation_text":"生成 34 秒轻快电子流行 BGM，无人声，弱鼓点，明亮但不抢旁白，适合旅行行李箱短视频广告。",
		"prompt_parts":{
			"objective":"生成 34 秒广告背景音乐。",
			"audio":"轻快电子流行，无人声，弱鼓点，明亮清爽，给旁白留出空间。"
		},
		"params":{"format":"mp3","sample_rate":48000,"watermark":false},
		"rationale":"AudioPlan 已批准，生成 bgm_audio 并在最终合成时 duck 到旁白下方。"
	}`, mode, forkFrom, audioPlanID)
}

func e2eRenderPlanWriteMode(c agentcraftsman.Context, targetPhase string) (string, string) {
	for _, plan := range c.RenderPlans {
		if strings.TrimSpace(plan.TargetPhase) != targetPhase {
			continue
		}
		id := uuidString(plan.ID)
		if id == "" {
			continue
		}
		return "fork_from", fmt.Sprintf(`"fork_from_render_plan_id":%q,`, id)
	}
	return "create", ""
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

func e2eShotDuration(shot db.Shot) int {
	if shot.DurationSec.Valid && shot.DurationSec.Float64 > 0 {
		return motionFixtureDuration(shot.DurationSec.Float64)
	}
	return 6
}

func motionFixtureDuration(value float64) int {
	rounded := int(value + 0.5)
	switch rounded {
	case 3, 4, 5, 6, 8:
		return rounded
	case 7:
		return 8
	default:
		if rounded > 8 {
			return 8
		}
		return 6
	}
}

func e2eMotionShotVariant(clientKey string) (motionStyle string, productMotion string, textPosition string, transitionIn string, transitionOut string) {
	switch clientKey {
	case "shot_01_hook":
		return "bold_hook_card", "slow_push_in", "upper_third", "soft_zoom", "swipe_up"
	case "shot_02_product":
		return "premium_product_ad", "float_up", "middle_safe", "fade", "fade"
	case "shot_03_wheels":
		return "benefit_grid", "slow_pan_left", "upper_third", "slide_left", "slide_right"
	case "shot_04_storage":
		return "scenario_postcard", "slow_push_in", "middle_safe", "soft_zoom", "fade"
	case "shot_05_cta":
		return "cta_packshot", "settle_center", "middle_safe", "fade", "hold"
	default:
		return "premium_product_ad", "slow_push_in", "upper_third", "fade", "fade"
	}
}

func e2eMotionShotUserText(clientKey string) (string, string) {
	switch clientKey {
	case "shot_01_hook":
		return "短途出行", "别让行李箱拖后腿"
	case "shot_02_product":
		return "悦行行李箱", "轻便好推，短途省心"
	case "shot_03_wheels":
		return "顺滑万向轮", "转向更稳，推行更省力"
	case "shot_04_storage":
		return "轻松收纳", "分区放好，快速拿取"
	case "shot_05_cta":
		return "现在出发", "悦行行李箱"
	default:
		return "悦行行李箱", "轻便好推，短途出行更省心"
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
