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
		Metadata: map[string]any{"e2e_fixture": "m1_creative_state"},
	}
}
