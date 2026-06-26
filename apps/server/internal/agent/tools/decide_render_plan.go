package tools

import (
	"context"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type RenderPlanDecisionStore interface {
	RenderPlanSubmitStore
	MarkRenderPlanRejected(ctx context.Context, params db.MarkRenderPlanRejectedParams) (db.RenderPlan, error)
}

type DecideRenderPlanNativeTool struct {
	store     RenderPlanDecisionStore
	submitter *RenderPlanSubmitter
}

type DecideRenderPlanInput struct {
	Brief                string `json:"brief" jsonschema:"required" jsonschema_description:"一句话描述调用该工具的意图，例如接受 shot_01 预览图 RenderPlan 并提交 Worker。不要超过 160 个中文字符。"`
	RenderPlanID         string `json:"render_plan_id" jsonschema:"required" jsonschema_description:"Producer 要决策的 RenderPlan UUID。通常来自 read_project_context 返回的 waiting_for_approval RenderPlan。"`
	Decision             string `json:"decision" jsonschema:"required,enum=accept,enum=reject" jsonschema_description:"Producer 决策。accept 表示接受该 RenderPlan；reject 表示拒绝该 RenderPlan。"`
	Reason               string `json:"reason" jsonschema:"required" jsonschema_description:"接受或拒绝的原因，面向用户和审计可读。"`
	NextAction           string `json:"next_action" jsonschema:"required,enum=submit_worker,enum=revise_with_craftsman,enum=no_action" jsonschema_description:"accept 只能配 submit_worker；reject 可配 revise_with_craftsman 或 no_action。请求用户确认请另行调用 request_user_decision。"`
	RevisionInstructions string `json:"revision_instructions" jsonschema_description:"reject 且 next_action=revise_with_craftsman 时，给后续 Craftsman 修订 RenderPlan 的具体要求。"`
}

func NewDecideRenderPlanNativeTool(store RenderPlanDecisionStore, runtime RenderPlanSubmitRuntime, enqueuer WorkerTaskEnqueuer) *DecideRenderPlanNativeTool {
	return &DecideRenderPlanNativeTool{store: store, submitter: NewRenderPlanSubmitter(store, runtime, enqueuer)}
}

func (t *DecideRenderPlanNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[DecideRenderPlanInput](toolDecideRenderPlan, "Producer 对等待决策的 RenderPlan 做 accept/reject。accept 会提交 worker_generation；reject 只标记计划被拒绝，不会直接重写计划或生成图片视频。")
}

func (t *DecideRenderPlanNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolDecideRenderPlan, argumentsInJSON, validateDecideRenderPlanInput)
	if !ok {
		return msg, nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolDecideRenderPlan)
	if !ok {
		return msg, nil
	}
	renderPlanID, _ := pgUUIDFromString(input.RenderPlanID)
	switch input.Decision {
	case "accept":
		task, _, err := t.submitter.SubmitRenderPlan(ctx, runtime.WorkspaceID, renderPlanID, runtime.ThreadID, input.Reason)
		if err != nil {
			return NaturalToolError(toolDecideRenderPlan, err.Error(), "请读取 RenderPlan 状态，确认它处于 compiled 或 waiting_for_approval 后重试。"), nil
		}
		return NaturalResult{
			Title: "已接受 RenderPlan 并提交 worker_generation",
			Items: []NaturalResultItem{
				{Label: "RenderPlan", Value: input.RenderPlanID},
				{Label: "WorkerTask", Value: uuidString(task.ID)},
				{Label: "原因", Value: input.Reason},
			},
			Next: "Worker 已入队。Producer 后续应读取 generation_job、artifact_version 和 RenderPlan 状态，不要把提交等同于产物完成。",
		}.String(), nil
	default:
		if t == nil || t.store == nil {
			return NaturalToolError(toolDecideRenderPlan, "render plan decision store 未配置。", "请检查服务端 wiring 后重试。"), nil
		}
		blocker := mustJSON(map[string]any{
			"blocker_type":          "producer_rejected",
			"message":               input.Reason,
			"revision_instructions": input.RevisionInstructions,
			"next_action":           input.NextAction,
		})
		audit := mustJSON(map[string]any{"producer_decision": "reject"})
		_, err := t.store.MarkRenderPlanRejected(ctx, db.MarkRenderPlanRejectedParams{
			ID:          renderPlanID,
			WorkspaceID: runtime.WorkspaceID,
			Blocker:     blocker,
			AuditHints:  audit,
		})
		if err != nil {
			return NaturalToolError(toolDecideRenderPlan, err.Error(), "请确认 RenderPlan 存在且仍在等待 Producer 决策。"), nil
		}
		return NaturalResult{
			Title: "已拒绝 RenderPlan",
			Items: []NaturalResultItem{
				{Label: "RenderPlan", Value: input.RenderPlanID},
				{Label: "下一步", Value: input.NextAction},
				{Label: "原因", Value: input.Reason},
			},
			Next: "如需修订，请 Producer 再调用 dispatch_craftsman，并在 critique / fix_hints 中带上 revision_instructions。",
		}.String(), nil
	}
}

func validateDecideRenderPlanInput(input DecideRenderPlanInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if _, ok := pgUUIDFromString(input.RenderPlanID); !ok {
		return fmt.Errorf("render_plan_id 必须是 UUID")
	}
	if err := requireMode(input.Decision, "accept", "reject"); err != nil {
		return err
	}
	if err := requireText(input.Reason, "reason"); err != nil {
		return err
	}
	if err := requireMode(input.NextAction, "submit_worker", "revise_with_craftsman", "no_action"); err != nil {
		return err
	}
	if input.Decision == "accept" && input.NextAction != "submit_worker" {
		return fmt.Errorf("accept 只能搭配 next_action=submit_worker")
	}
	if input.Decision == "reject" && input.NextAction == "submit_worker" {
		return fmt.Errorf("reject 不能搭配 next_action=submit_worker")
	}
	if input.Decision == "reject" && input.NextAction == "revise_with_craftsman" && input.RevisionInstructions == "" {
		return fmt.Errorf("revise_with_craftsman 需要 revision_instructions")
	}
	return nil
}
