package main

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"

	agentreviewer "github.com/sinmaystar/clip-anvil/internal/agent/reviewer"
)

type e2eM3ReviewerGateResponder struct{}

func (e2eM3ReviewerGateResponder) Respond(_ context.Context, reviewContext agentreviewer.Context) (agentreviewer.ReviewerTurnOutput, error) {
	switch e2eReviewerToolResultCount(reviewContext.SameTurnMessages) {
	case 0:
		return e2eReviewerToolCallOutput("e2e-submit-review-result", "submit_review_result", e2eSubmitReviewResultArgs(reviewContext)), nil
	default:
		return agentreviewer.ReviewerTurnOutput{
			AssistantText: "已完成 M3 Reviewer Gate fixture 评审：预览图可继续，但记录了一个商品边缘一致性 warning。",
			Metadata:      map[string]any{"e2e_fixture": "m3_reviewer_gate"},
		}, nil
	}
}

func e2eReviewerToolResultCount(messages []agentreviewer.ReviewerSameTurnMessage) int {
	count := 0
	for _, message := range messages {
		if message.Role == "tool" {
			count++
		}
	}
	return count
}

func e2eReviewerToolCallOutput(id string, name string, arguments string) agentreviewer.ReviewerTurnOutput {
	return agentreviewer.ReviewerTurnOutput{
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
		Metadata: map[string]any{"e2e_fixture": "m3_reviewer_gate"},
	}
}

func e2eSubmitReviewResultArgs(c agentreviewer.Context) string {
	shotID := uuidString(c.Shot.ID)
	nodeID := uuidString(c.Node.ID)
	versionID := uuidString(c.Version.ID)
	return fmt.Sprintf(`{
		"brief":"提交悦行行李箱预览图 Reviewer Gate 评审结果。",
		"review_task":"preview_image_review",
		"target":{
			"workspace_scope":"shot",
			"shot_id":%q,
			"node_id":%q,
			"artifact_version_id":%q
		},
		"verdict":"accepted_with_warnings",
		"overall_score":0.82,
		"rubric":[
			{"axis":"faithfulness","score":0.86,"pass":true,"severity":"info","reason":"画面符合机场广告和悦行行李箱核心诉求。","fix_hint":""},
			{"axis":"subject_consistency","score":0.74,"pass":true,"severity":"warning","reason":"行李箱整体一致，但边缘反光略强，后续视频生成需继续锁定银灰硬壳特征。","fix_hint":"Craftsman 后续 RenderPlan 应继续强调银灰硬壳、四轮、简洁拉杆。"},
			{"axis":"product_visibility","score":0.88,"pass":true,"severity":"info","reason":"商品位于画面中心且可见度足够。","fix_hint":""},
			{"axis":"brand_style_consistency","score":0.84,"pass":true,"severity":"info","reason":"现代机场晨光和商务质感与 ProjectMemory 一致。","fix_hint":""},
			{"axis":"composition_proportion","score":0.79,"pass":true,"severity":"info","reason":"9:16 构图中商品和机场环境比例合理。","fix_hint":""},
			{"axis":"visual_quality","score":0.81,"pass":true,"severity":"info","reason":"画面清晰稳定，满足预览图验收。","fix_hint":""}
		],
		"critique":"预览图可以进入后续视频生成，但需要保留一个商品边缘反光和外观一致性的 warning，避免 Seedance 阶段放大漂移。",
		"issues":[
			{
				"dimension":"subject_consistency",
				"severity":"warning",
				"title":"商品边缘反光可能导致视频阶段外观漂移",
				"description":"预览图主体可通过，但箱体边缘高光略强，视频生成时可能被模型解释为不同材质或颜色。",
				"evidence":"预览图中银灰行李箱边缘反光偏亮。",
				"target_object_type":"artifact_version",
				"target_object_id":%q,
				"suggested_fix":"revise_render_plan",
				"fix_hint":"后续视频 RenderPlan 继续强调银灰色硬壳、统一箱体比例、四轮和简洁拉杆；负向约束避免变成白色软箱或竞品箱。",
				"requires_user_confirmation":false
			}
		],
		"retry_recommendation":{
			"should_repair":false,
			"suggested_fix":"none",
			"target_object_type":"artifact_version",
			"target_object_id":%q,
			"fix_hints":["视频阶段继续锁定悦行行李箱银灰硬壳外观。"],
			"requires_user_confirmation":false,
			"escalation_reason":""
		},
		"evidence_summary":"基于当前 preview image artifact、分镜目标和 ProjectMemory 做 M3 fixture 评审。",
		"reason":"所有 preview_image_review 必选轴通过；subject_consistency 有 warning 但不阻塞后续。"
	}`, shotID, nodeID, versionID, versionID, versionID)
}
