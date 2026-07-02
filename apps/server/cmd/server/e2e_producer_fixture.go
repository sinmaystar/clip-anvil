package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	agentproducer "github.com/sinmaystar/clip-anvil/internal/agent/producer"
)

type e2eM1CreativeStateResponder struct{}
type e2eM2RenderPlanProducerResponder struct{}
type e2eM3ReviewerGateProducerResponder struct{}
type e2eMotionShotVideoProducerResponder struct{}

func (e2eMotionShotVideoProducerResponder) Respond(_ context.Context, producerContext agentproducer.ProducerContext) (agentproducer.ProducerTurnOutput, error) {
	toolResultCount := e2eToolResultCount(producerContext.SameTurnMessages)
	if e2eProducerContextHasSignal(producerContext, "worker_generation_completed") {
		return agentproducer.ProducerTurnOutput{
			AssistantText: "已确认 motion_only/no-Seedance 媒体生成完成信号；本 E2E fixture 不会因工程 signal 自动重复派发任务。",
			Metadata:      map[string]any{"e2e_fixture": "motion_shot_video", "signal_handled": "worker_generation_completed"},
		}, nil
	}
	if toolResultCount == 0 && e2eProducerContextWantsVoiceover(producerContext) {
		return e2eToolCallOutput("e2e-motion-dispatch-voiceover", "dispatch_craftsman", `{
			"brief":"基于已批准 AudioPlan 生成悦行行李箱广告旁白音频。",
			"scope":{"type":"audio_plan","id":"audio_plan.active"},
			"target_phase":"voiceover_audio",
			"execution_policy":"execute_immediately",
			"force":true,
			"max_attempts":1
		}`), nil
	}
	if toolResultCount == 0 && e2eProducerContextWantsBGM(producerContext) {
		return e2eToolCallOutput("e2e-motion-dispatch-bgm", "dispatch_craftsman", `{
			"brief":"基于已批准 AudioPlan 生成悦行行李箱广告 BGM 音频。",
			"scope":{"type":"audio_plan","id":"audio_plan.active"},
			"target_phase":"bgm_audio",
			"execution_policy":"execute_immediately",
			"force":true,
			"max_attempts":1
		}`), nil
	}
	if toolResultCount == 0 && e2eProducerContextWantsFinalComposition(producerContext) {
		return e2eToolCallOutput("e2e-motion-dispatch-composer", "dispatch_composer", `{
			"source_storyboard_ref":{"type":"media_node","key":"shot_01_hook.shot_video.r1.node"},
			"instructions":"把已成功的 5 段 Remotion motion shot 悦行行李箱 shot_video 与 AudioPlan 中已成功的 voiceover_audio、bgm_audio 合成为 34 秒 9:16 最终 MP4；保留多分镜节奏、转场、字幕安全区和 CTA，混入旁白和 BGM，禁止调用 Seedance。",
			"template_key":"simple_concat"
		}`), nil
	}
	if e2eProducerContextHasToolResult(producerContext, "dispatch_composer") {
		return agentproducer.ProducerTurnOutput{
			AssistantText: "已派发最终合成任务；等待 Composer 异步完成，不再在同一轮追加素材生成。",
			Metadata:      map[string]any{"e2e_fixture": "motion_shot_video", "composer_dispatched": true},
		}, nil
	}
	if toolResultCount == 0 && e2eProducerContextWantsMotionShotVideo(producerContext) {
		return e2eToolCallOutput("e2e-motion-dispatch-craftsman", "dispatch_craftsman", `{
			"brief":"Seedream 主视觉图片已完成，继续生成悦行行李箱 Remotion motion shot；禁止调用 Seedance。",
			"scope":{"type":"shot"},
			"target_phase":"shot_video",
			"execution_policy":"execute_immediately",
			"video_route_policy":"motion_only",
			"shot_refs":["shot_01_hook","shot_02_product","shot_03_wheels","shot_04_travel","shot_05_cta"],
			"input_node_refs":["shot_01_hook preview image","shot_02_product preview image","shot_03_wheels preview image","shot_04_travel preview image","shot_05_cta preview image"],
			"force":true,
			"max_attempts":1
		}`), nil
	}
	switch toolResultCount {
	case 0:
		return e2eToolCallOutput("e2e-motion-upsert-brief", "upsert_project_brief", `{
			"brief":"基于用户上传的 box.png，为悦行行李箱创建 34 秒 9:16 口播广告，硬性要求不调用 Seedance 视频；图片可以使用 Seedream，音频使用火山 TTS/BGM，视频只使用 Remotion motion shot。",
			"mode":"create",
			"title":"悦行行李箱 Remotion Motion Shot 口播广告",
			"video_type":"marketing_ad",
			"target_audience":"短途出行和商务通勤用户",
			"tone":"轻快、可信、现代",
			"visual_style":"Seedream 先为每个分镜生成商业广告主视觉/背景图，Remotion 再做 5 个动态 motion shot：痛点钩子、产品展示、万向轮卖点、短途出行情境、CTA；每段都有不同布局、转场和文字安全区。",
			"duration_sec":34,
			"aspect_ratio":"9:16",
			"language":"zh-CN",
			"objective":"用低成本 motion shot 突出悦行行李箱轻便、顺滑万向轮、短途出行和安心托运，并合成旁白音频。",
			"concept":"以用户上传的行李箱图片为商品参考，先用 Seedream 生成商业主视觉图片，再通过 Remotion 图片动效、文字卖点、TTS 旁白和轻快 BGM 完成短广告。",
			"constraints":[
				{"text":"禁止调用 Seedance 或任何 volcengine/seedance 视频模型。","severity":"blocking"},
				{"text":"视频必须使用 Remotion/internal_motion_video/motion_shot_video。","severity":"blocking"},
				{"text":"必须使用用户上传的 box.png 作为产品参考。","severity":"blocking"},
				{"text":"允许并鼓励使用 Seedream 生成图片资产，允许使用火山音频模型生成旁白和 BGM。","severity":"high"}
			],
			"reason":"E2E fixture 验证 Agent no-Seedance motion shot policy。"
		}`), nil
	case 1:
		return e2eToolCallOutput("e2e-motion-update-memory", "update_project_memory", `{
			"brief":"记录悦行行李箱 motion-only 广告约束。",
			"mode":"create",
			"core_intent":"用低成本 Remotion motion shot 快速生成悦行行李箱口播广告。",
			"soul":"轻装出发、顺滑好推、安心托运。",
			"brand_facts":[
				{"key":"product_name","value":"悦行行李箱"},
				{"key":"route_policy","value":"motion_shot_video_no_seedance_video"},
				{"key":"image_policy","value":"seedream_allowed"},
				{"key":"audio_policy","value":"volcengine_tts_required"}
			],
			"non_negotiables":[
				{"rule":"禁止调用 Seedance 或 volcengine/seedance 视频模型。","severity":"blocking"},
				{"rule":"shot_video 只能使用 motion_shot_video / internal_motion_video / remotion-motion-shot-v1。","severity":"blocking"},
				{"rule":"使用 box.png 作为商品参考，并先生成 Seedream 主视觉图片。","severity":"blocking"},
				{"rule":"旁白和 BGM 必须使用火山音频模型，不能把 mock 音频当作真实通过。","severity":"blocking"}
			],
			"visual_anchors":[
				{"key":"product_image","value":"用户上传的 box.png 行李箱产品图。"},
				{"key":"seedream_hero_image","value":"用 box.png 生成商业主视觉：清爽旅行广告背景，商品居中，留出字幕空间。"},
				{"key":"motion_style","value":"9:16 竖版，Seedream 主视觉轻微推进，清爽字幕卖点和 CTA。"}
			],
			"allowed":[
				{"rule":"可以使用 Seedream 生成图片资产和火山音频模型生成口播/BGM。","severity":"low"}
			],
			"forbidden":[
				{"rule":"不要生成 Seedance 视频，不要用视频模型做运动画面。","severity":"blocking"}
			],
			"prompt_injection_hints":[
				"Seedream images allowed",
				"motion_shot_video only for video",
				"Remotion internal_motion_video",
				"不要调用 Seedance"
			],
			"source_refs":[
				{"type":"user_attachment","note":"box.png"},
				{"type":"user_message","note":"用户要求 no-Seedance Remotion 口播广告"}
			],
			"requires_user_approval":false,
			"reason":"E2E fixture 记录 motion_only policy。"
		}`), nil
	case 2:
		return e2eToolCallOutput("e2e-motion-storyboard", "upsert_storyboard", `{
			"brief":"创建 34 秒悦行行李箱 Remotion motion shot 动态多分镜 storyboard。",
			"mode":"create",
			"scope":{"type":"workspace"},
			"scenes":[
				{
					"client_key":"scene_motion_ad_intro",
					"sort_order":1,
					"title":"痛点与产品建立",
					"description":"以短途出行痛点开场，再建立悦行行李箱商品主体。",
					"location":"Remotion 图文动效画面",
					"mood":"清爽、轻快、可信"
				},
				{
					"client_key":"scene_motion_ad_benefit",
					"sort_order":2,
					"title":"卖点与出行情境",
					"description":"解释万向轮、轻便、省力和短途出行收益。",
					"location":"Remotion 图文动效画面",
					"mood":"明快、可信、行动感"
				},
				{
					"client_key":"scene_motion_ad_outro",
					"sort_order":3,
					"title":"CTA 收束",
					"description":"品牌口号和行动按钮收束。",
					"location":"Remotion 图文动效画面",
					"mood":"明亮、明确、利落"
				}
			],
			"shots":[
				{
					"client_key":"shot_01_hook",
					"scene_client_key":"scene_motion_ad_intro",
					"sort_order":1,
					"title":"短途出行痛点钩子",
					"shot_kind":"hook_card",
					"creative_text":"用短途出行拖箱费力的痛点开场，商品图轻推近。",
					"narrative_purpose":"前三秒抓住短途出行用户注意。",
					"duration_sec":6,
					"visual_intent":"深色干净背景，行李箱位于中上，顶部短 hook。",
					"action_text":"产品图慢推近，痛点文字弹出。",
					"camera_intent":"Remotion slow push in，无真实复杂运动。",
					"narration":"短途出行，别让行李箱拖后腿。"
				},
				{
					"client_key":"shot_02_product",
					"scene_client_key":"scene_motion_ad_intro",
					"sort_order":2,
					"title":"悦行行李箱产品展示",
					"shot_kind":"product_hero",
					"creative_text":"建立悦行行李箱主体，突出银灰硬壳、竖向纹理和轻便外观。",
					"narrative_purpose":"让用户记住商品主体和品牌名。",
					"duration_sec":8,
					"visual_intent":"商品大图居中，品牌标题和轻便卖点分层。",
					"action_text":"商品轻微浮起，品牌名淡入。",
					"camera_intent":"轻微视差和中心聚焦。",
					"narration":"悦行行李箱，轻便好推，通勤和短途都省心。"
				},
				{
					"client_key":"shot_03_wheels",
					"scene_client_key":"scene_motion_ad_benefit",
					"sort_order":3,
					"title":"顺滑万向轮卖点",
					"shot_kind":"benefit_card",
					"creative_text":"用信息卡解释顺滑万向轮、转向稳定、推行省力。",
					"narrative_purpose":"证明核心功能卖点。",
					"duration_sec":8,
					"visual_intent":"底部轮子细节和三点卖点文字清晰分组。",
					"action_text":"三条卖点逐条入场。",
					"camera_intent":"稳定信息卡，轻微横向漂移。",
					"narration":"顺滑万向轮，转向更稳，推行更省力。"
				},
				{
					"client_key":"shot_04_travel",
					"scene_client_key":"scene_motion_ad_benefit",
					"sort_order":4,
					"title":"短途旅行场景",
					"shot_kind":"scenario_card",
					"creative_text":"把商品放进短途出行语境，强调安心托运和周末出发。",
					"narrative_purpose":"把功能转成用户生活收益。",
					"duration_sec":6,
					"visual_intent":"旅行氛围背景，商品和目的地标签形成层次。",
					"action_text":"背景慢移，目的地标签滑入。",
					"camera_intent":"柔和拉远，保留字幕安全区。",
					"narration":"短途旅行、商务通勤，轻装出发更从容。"
				},
				{
					"client_key":"shot_05_cta",
					"scene_client_key":"scene_motion_ad_outro",
					"sort_order":5,
					"title":"CTA 现在出发",
					"shot_kind":"cta_card",
					"creative_text":"品牌口号和 CTA 收束，强调现在出发。",
					"narrative_purpose":"促进行动。",
					"duration_sec":6,
					"visual_intent":"商品居中，按钮式 CTA 位于下方安全区。",
					"action_text":"CTA 按钮弹出，背景渐亮。",
					"camera_intent":"轻微拉远后定格。",
					"narration":"悦行行李箱，现在出发。"
				}
			],
			"reason":"E2E fixture 创建可派发 shot_video 的 5 个动态 motion shot 分镜。"
		}`), nil
	case 3:
		return e2eToolCallOutput("e2e-motion-audio-plan", "upsert_audio_plan", `{
			"brief":"创建悦行行李箱 motion shot 广告的旁白和 BGM AudioPlan。",
			"mode":"replace_draft",
			"title":"悦行行李箱 34 秒口播音频",
			"language":"zh-CN",
			"target_duration_sec":34,
			"voiceover_script":"短途出行，别让行李箱拖后腿。悦行行李箱，轻便好推，通勤和短途都省心。顺滑万向轮，转向更稳，推行更省力。短途旅行、商务通勤，轻装出发更从容。悦行行李箱，现在出发。",
			"voice_profile":{"source":"generated","speaker":"warm_female","style":"清爽、可信、轻快"},
			"bgm_plan":{"source":"generated","provider":"volcengine","model":"seed-audio-1.0","style":"轻快电子流行，无人声，弱鼓点，适合旅行广告"},
			"cue_plan":[
				{"shot_ref":"shot_01_hook","start_sec":0,"end_sec":6,"text":"短途出行，别让行李箱拖后腿。"},
				{"shot_ref":"shot_02_product","start_sec":6,"end_sec":14,"text":"悦行行李箱，轻便好推，通勤和短途都省心。"},
				{"shot_ref":"shot_03_wheels","start_sec":14,"end_sec":22,"text":"顺滑万向轮，转向更稳，推行更省力。"},
				{"shot_ref":"shot_04_travel","start_sec":22,"end_sec":28,"text":"短途旅行、商务通勤，轻装出发更从容。"},
				{"shot_ref":"shot_05_cta","start_sec":28,"end_sec":34,"text":"悦行行李箱，现在出发。"}
			],
			"generation_params":{"format":"mp3","sample_rate":48000,"speech_rate":1.04}
		}`), nil
	case 4:
		return e2eToolCallOutput("e2e-motion-audio-approve", "upsert_audio_plan", `{
			"brief":"E2E 自动批准 AudioPlan，继续生成旁白和 BGM。",
			"mode":"approve"
		}`), nil
	case 5:
		return e2eToolCallOutput("e2e-motion-dispatch-preview-image", "dispatch_craftsman", `{
			"brief":"先用 Seedream 基于 box.png 生成悦行行李箱商业广告主视觉图片，供 Remotion motion shot 使用。",
			"scope":{"type":"shot"},
			"target_phase":"preview_image",
			"execution_policy":"execute_immediately",
			"shot_refs":["shot_01_hook","shot_02_product","shot_03_wheels","shot_04_travel","shot_05_cta"],
			"input_node_refs":["box.png"],
			"force":true,
			"max_attempts":1
		}`), nil
	case 6:
		return e2eToolCallOutput("e2e-motion-dispatch-craftsman", "dispatch_craftsman", `{
			"brief":"生成悦行行李箱口播广告，不要调用 Seedance，只使用 Remotion motion shot。",
			"scope":{"type":"shot"},
			"target_phase":"shot_video",
			"execution_policy":"execute_immediately",
			"video_route_policy":"motion_only",
			"shot_refs":["shot_01_hook","shot_02_product","shot_03_wheels","shot_04_travel","shot_05_cta"],
			"input_node_refs":["shot_01_hook preview image","shot_02_product preview image","shot_03_wheels preview image","shot_04_travel preview image","shot_05_cta preview image"],
			"force":true,
			"max_attempts":1
		}`), nil
	case 7:
		return agentproducer.ProducerTurnOutput{
			AssistantText: "已按 motion_only/no-Seedance 策略派发悦行行李箱 5 个动态 motion shot。由于异步派发会结束当前 Producer turn，请后续发送“继续生成旁白音频”和“继续生成 BGM 音频”来派发音频素材。",
			Metadata:      map[string]any{"e2e_fixture": "motion_shot_video"},
		}, nil
	default:
		return agentproducer.ProducerTurnOutput{
			AssistantText: "motion_only/no-Seedance E2E fixture 已完成当前轮可同步执行的步骤。",
			Metadata:      map[string]any{"e2e_fixture": "motion_shot_video"},
		}, nil
	}
}

func e2eProducerContextHasSignal(producerContext agentproducer.ProducerContext, signalType string) bool {
	needle := strings.TrimSpace(signalType)
	if needle == "" {
		return false
	}
	if strings.Contains(producerContext.RuntimeTriggerText, needle) {
		return true
	}
	for _, reminder := range producerContext.PendingReminders {
		if strings.Contains(reminder, needle) {
			return true
		}
	}
	return false
}

func e2eProducerContextHasToolResult(producerContext agentproducer.ProducerContext, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	for _, message := range producerContext.SameTurnMessages {
		if message.MessageType == "tool_result" && message.ToolName == toolName {
			return true
		}
	}
	return false
}

func e2eProducerContextWantsVoiceover(producerContext agentproducer.ProducerContext) bool {
	text := strings.ToLower(strings.TrimSpace(producerContext.LatestUserText))
	return strings.Contains(text, "voiceover_audio") ||
		strings.Contains(text, "continue voiceover") ||
		strings.Contains(text, "继续生成旁白")
}

func e2eProducerContextWantsBGM(producerContext agentproducer.ProducerContext) bool {
	text := strings.ToLower(strings.TrimSpace(producerContext.LatestUserText))
	return strings.Contains(text, "bgm_audio") ||
		strings.Contains(text, "continue bgm") ||
		strings.Contains(text, "继续生成 bgm") ||
		strings.Contains(text, "继续生成背景音乐")
}

func e2eProducerContextWantsMotionShotVideo(producerContext agentproducer.ProducerContext) bool {
	text := strings.ToLower(strings.TrimSpace(producerContext.LatestUserText))
	return strings.Contains(text, "shot_video") ||
		strings.Contains(text, "继续生成动效视频") ||
		strings.Contains(text, "继续生成视频")
}

func e2eProducerContextWantsFinalComposition(producerContext agentproducer.ProducerContext) bool {
	text := strings.ToLower(strings.TrimSpace(producerContext.LatestUserText))
	return strings.Contains(text, "compose_final_video") ||
		strings.Contains(text, "final video") ||
		strings.Contains(text, "继续合成最终视频") ||
		strings.Contains(text, "合成最终成片")
}

func (e2eM1CreativeStateResponder) Respond(_ context.Context, producerContext agentproducer.ProducerContext) (agentproducer.ProducerTurnOutput, error) {
	switch e2eToolResultCount(producerContext.SameTurnMessages) {
	case 0:
		return e2eToolCallOutput("e2e-upsert-brief", "upsert_project_brief", `{
			"brief":"为悦行行李箱机场抖音广告创建 active brief。",
			"mode":"create",
			"title":"悦行行李箱机场广告",
			"video_type":"marketing_ad",
			"target_audience":"短途商务出行用户",
			"tone":"轻快、可靠、高级",
			"visual_style":"现代机场出发大厅，清晨自然光，商业广告质感",
			"duration_sec":15,
			"aspect_ratio":"9:16",
			"language":"zh-CN",
			"objective":"突出悦行行李箱轻便、稳定、适合短途商务出行。",
			"concept":"一位商务旅客在机场轻松拉着悦行行李箱完成出发，强调轻盈顺滑和可靠陪伴。",
			"constraints":[
				{"text":"行李箱必须作为全片视觉核心。","severity":"blocking"},
				{"text":"整体风格必须保持现代机场商务广告质感。","severity":"high"}
			],
			"reason":"E2E fixture 验证 Producer native tool 写入 CreativeBrief。"
		}`), nil
	case 1:
		return e2eToolCallOutput("e2e-update-memory", "update_project_memory", `{
			"brief":"记录悦行行李箱广告的全局一致性约束。",
			"mode":"create",
			"core_intent":"让用户相信悦行行李箱适合高频短途商务出行。",
			"soul":"轻松出发、可靠陪伴、现代商务感。",
			"brand_facts":[
				{"key":"product_name","value":"悦行行李箱"},
				{"key":"product_positioning","value":"短途商务出行行李箱"}
			],
			"non_negotiables":[
				{"rule":"每个分镜都必须保持悦行行李箱外观一致。","severity":"blocking"},
				{"rule":"机场场景不能出现杂乱低质感背景。","severity":"high"}
			],
			"visual_anchors":[
				{"key":"product_anchor","value":"银灰色硬壳行李箱，线条干净，轮子顺滑。"},
				{"key":"scene_anchor","value":"现代机场出发大厅，晨光，大面积玻璃和清洁地面。"}
			],
			"allowed":[
				{"rule":"可以出现商务旅客、登机牌、机场导视。","severity":"low"}
			],
			"forbidden":[
				{"rule":"不要出现竞品 Logo。","severity":"blocking"}
			],
			"prompt_injection_hints":[
				"保持银灰色悦行行李箱外观一致",
				"现代机场晨光商业广告质感"
			],
			"source_refs":[
				{"type":"user_message","note":"用户要求制作悦行行李箱机场广告"}
			],
			"requires_user_approval":false,
			"reason":"E2E fixture 验证 Producer 写入 ProjectMemory。"
		}`), nil
	case 2:
		return e2eToolCallOutput("e2e-upsert-elements", "upsert_key_elements", `{
			"brief":"沉淀悦行行李箱和机场大厅两个一致性关键元素。",
			"mode":"create",
			"elements":[
				{
					"client_key":"product_yuexing_luggage",
					"element_type":"product",
					"name":"悦行行李箱",
					"description":"广告主商品，全片核心视觉锚点。",
					"source_type":"prompt_derived",
					"source_refs":[{"type":"user_message","note":"用户提出悦行行李箱广告"}],
					"states":[
						{
							"client_key":"state_silver_business",
							"label":"银灰商务默认状态",
							"visual_description":"银灰色硬壳行李箱，边角圆润，拉杆简洁，四轮顺滑，外观干净高级。",
							"reference_status":"needs_reference",
							"is_default":true,
							"state_facts":[
								{"key":"color","value":"silver gray"},
								{"key":"style","value":"modern business luggage"}
							],
							"source_refs":[{"type":"user_message","note":"从用户广告需求中派生"}]
						}
					]
				},
				{
					"client_key":"scene_airport_departure_hall",
					"element_type":"scene",
					"name":"现代机场出发大厅",
					"description":"全片主场景，用于保持空间、光线和商业质感一致。",
					"source_type":"prompt_derived",
					"source_refs":[{"type":"user_message","note":"用户要求机场拍摄"}],
					"states":[
						{
							"client_key":"state_morning_softlight",
							"label":"晨光柔和大厅",
							"visual_description":"现代机场出发大厅，玻璃幕墙，清晨柔和自然光，干净地面和清晰导视。",
							"reference_status":"needs_reference",
							"is_default":true,
							"state_facts":[
								{"key":"lighting","value":"morning soft natural light"},
								{"key":"location","value":"airport departure hall"}
							],
							"source_refs":[{"type":"user_message","note":"从机场广告需求中派生"}]
						}
					]
				}
			],
			"reason":"E2E fixture 验证 Producer 写入 KeyElement 和 KeyElementState。"
		}`), nil
	case 3:
		return e2eToolCallOutput("e2e-upsert-storyboard", "upsert_storyboard", `{
			"brief":"创建悦行行李箱机场广告的两镜头 storyboard。",
			"mode":"create",
			"scope":{"type":"workspace"},
			"scenes":[
				{
					"client_key":"scene_airport_departure_hall",
					"sort_order":1,
					"title":"机场轻松出发",
					"description":"商务旅客在现代机场出发大厅拉着悦行行李箱轻松前行。",
					"location":"机场出发大厅",
					"mood":"明亮、轻快、可靠"
				}
			],
			"shots":[
				{
					"client_key":"shot_01_airport_walk",
					"scene_client_key":"scene_airport_departure_hall",
					"sort_order":1,
					"title":"机场拉箱开场",
					"shot_kind":"lifestyle",
					"creative_text":"商务旅客单手拉着银灰色悦行行李箱穿过晨光中的机场大厅。",
					"narrative_purpose":"建立出行场景和轻松感。",
					"duration_sec":7,
					"visual_intent":"突出机场空间和行李箱顺滑跟随。",
					"action_text":"旅客步伐轻快，行李箱轮子稳定滚动。",
					"camera_intent":"中景跟拍，轻微低机位。",
					"narration":"出发，从容一点。"
				},
				{
					"client_key":"shot_02_product_closeup",
					"scene_client_key":"scene_airport_departure_hall",
					"sort_order":2,
					"title":"产品质感特写",
					"shot_kind":"product_closeup",
					"creative_text":"镜头切到悦行行李箱轮子和箱体细节，银灰色外壳在机场晨光里有干净反光。",
					"narrative_purpose":"证明产品质感和顺滑卖点。",
					"duration_sec":8,
					"visual_intent":"突出箱体材质、轮子顺滑和高级质感。",
					"action_text":"行李箱从画面前景滑过，轮子平稳。",
					"camera_intent":"低机位产品特写，慢速推近。",
					"narration":"悦行，陪你轻松抵达。"
				}
			],
			"shot_key_elements":[
				{"shot_client_key":"shot_01_airport_walk","element_client_key":"product_yuexing_luggage","state_client_key":"state_silver_business","role":"hero_product","required":true,"sort_order":1},
				{"shot_client_key":"shot_01_airport_walk","element_client_key":"scene_airport_departure_hall","state_client_key":"state_morning_softlight","role":"location","required":true,"sort_order":2},
				{"shot_client_key":"shot_02_product_closeup","element_client_key":"product_yuexing_luggage","state_client_key":"state_silver_business","role":"hero_product","required":true,"sort_order":1},
				{"shot_client_key":"shot_02_product_closeup","element_client_key":"scene_airport_departure_hall","state_client_key":"state_morning_softlight","role":"location","required":true,"sort_order":2}
			],
			"dependencies":[
				{
					"from_shot_client_key":"shot_01_airport_walk",
					"to_shot_client_key":"shot_02_product_closeup",
					"dependency_type":"same_product_consistency",
					"required_artifact":"preview_image",
					"injection_role":"product_reference",
					"blocking_phase":"preview_generation",
					"reason":"第二镜头必须延续第一镜头中的同一只悦行行李箱外观。"
				}
			],
			"reason":"E2E fixture 验证 Producer 写入 Scene、Shot、引用和依赖。"
		}`), nil
	default:
		return agentproducer.ProducerTurnOutput{
			AssistantText: "已完成悦行行李箱机场广告的 M1 创作状态建模：CreativeBrief、ProjectMemory、关键元素和两镜头 storyboard 已写入，并已投影到画布。",
			Metadata:      map[string]any{"e2e_fixture": "m1_creative_state"},
		}, nil
	}
}

func (e2eM2RenderPlanProducerResponder) Respond(ctx context.Context, producerContext agentproducer.ProducerContext) (agentproducer.ProducerTurnOutput, error) {
	if e2eToolResultCount(producerContext.SameTurnMessages) < 4 {
		return e2eM1CreativeStateResponder{}.Respond(ctx, producerContext)
	}
	switch e2eToolResultCount(producerContext.SameTurnMessages) {
	case 4:
		return e2eToolCallOutput("e2e-dispatch-craftsman", "dispatch_craftsman", `{
			"brief":"直接生成两条分镜的 Seedream 预览图。",
			"mode":"preview_image",
			"execution_policy":"execute_immediately",
			"shot_refs":[],
			"force":true,
			"max_attempts":1
		}`), nil
	default:
		return agentproducer.ProducerTurnOutput{
			AssistantText: "已完成悦行行李箱机场广告的 M2 生成计划：创意状态已建模，两个分镜的 Craftsman 任务已派发，RenderPlan 会写入数据库并投影到制作过程。",
			Metadata:      map[string]any{"e2e_fixture": "m2_render_plan"},
		}, nil
	}
}

func (e2eM3ReviewerGateProducerResponder) Respond(ctx context.Context, producerContext agentproducer.ProducerContext) (agentproducer.ProducerTurnOutput, error) {
	if !strings.Contains(producerContext.LatestUserText, "E2E_M3_REVIEWER_GATE") {
		return e2eM2RenderPlanProducerResponder{}.Respond(ctx, producerContext)
	}
	switch e2eToolResultCount(producerContext.SameTurnMessages) {
	case 0:
		return e2eToolCallOutput("e2e-read-production-state", "read_project_context", `{
			"brief":"读取生产状态，寻找可评审的 succeeded 预览图 artifact。",
			"scope":{"type":"workspace","id":""},
			"include":["production_state"],
			"detail_level":"summary"
		}`), nil
	case 1:
		args, ok := e2eDispatchReviewerArgsFromText(e2eLatestToolResultText(producerContext.SameTurnMessages))
		if !ok {
			return agentproducer.ProducerTurnOutput{
				AssistantText: "M3 Reviewer Gate fixture 未找到可评审的 preview image，请先完成 M2 RenderPlan 并插入 succeeded artifact。",
				Metadata:      map[string]any{"e2e_fixture": "m3_reviewer_gate", "missing_preview": true},
			}, nil
		}
		return e2eToolCallOutput("e2e-dispatch-reviewer", "dispatch_reviewer", args), nil
	default:
		return agentproducer.ProducerTurnOutput{
			AssistantText: "已完成悦行行李箱机场广告的 M3 Reviewer Gate：Producer 已派发 Reviewer 评审预览图，Reviewer 会写入 review_record 和 artifact_issue，并投影到制作过程画布。",
			Metadata:      map[string]any{"e2e_fixture": "m3_reviewer_gate"},
		}, nil
	}
}

func e2eDispatchReviewerArgsFromText(text string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "PreviewArtifact:") || !strings.Contains(line, "version_status=succeeded") {
			continue
		}
		fields := e2eKeyValueFields(line)
		shotID := fields["shot_id"]
		clientKey := fields["shot"]
		nodeID := fields["node_id"]
		versionID := fields["version_id"]
		if shotID == "" || nodeID == "" || versionID == "" {
			continue
		}
		return fmt.Sprintf(`{
			"brief":"评审 %s 的 Seedream 预览图是否可进入后续视频生成。",
			"review_task":"preview_image_review",
			"target":{
				"workspace_scope":"shot",
				"shot_id":%q,
				"node_id":%q,
				"artifact_version_id":%q
			},
			"policy":{
				"overall_threshold":0.72,
				"axis_threshold":0.70,
				"max_attempts":1
			},
			"auto_decision":{
				"allow_auto_accept":true,
				"allow_auto_repair":false,
				"require_user_on_reject":true
			},
			"reason":"M3 E2E 验证 Producer 派发 Reviewer Gate，并要求 Reviewer 写入结构化评审和开放问题。"
		}`, clientKey, shotID, nodeID, versionID), true
	}
	return "", false
}

func e2eKeyValueFields(line string) map[string]string {
	out := map[string]string{}
	for _, field := range strings.Fields(line) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "；,，")
	}
	return out
}

func e2eLatestToolResultText(messages []agentproducer.ProducerSameTurnMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" {
			return messages[i].Content
		}
	}
	return ""
}

func e2eToolResultCount(messages []agentproducer.ProducerSameTurnMessage) int {
	count := 0
	for _, message := range messages {
		if message.Role == "tool" {
			count++
		}
	}
	return count
}

func e2eToolCallOutput(id string, name string, arguments string) agentproducer.ProducerTurnOutput {
	fixture := "m1_creative_state"
	if strings.HasPrefix(id, "e2e-motion-") {
		fixture = "motion_shot_video"
	}
	return agentproducer.ProducerTurnOutput{
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
		Metadata: map[string]any{"e2e_fixture": fixture},
	}
}
