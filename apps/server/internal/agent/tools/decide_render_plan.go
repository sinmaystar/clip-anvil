package tools

import (
	"context"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

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

type RenderPlanDecisionSignalRuntime interface {
	MarkProducerPendingSignalsProcessedByRenderPlan(ctx context.Context, workspaceID, renderPlanID, taskID pgtype.UUID) ([]db.ProducerPendingSignal, error)
}

type DecideRenderPlanInput struct {
	Brief                string                    `json:"brief" jsonschema:"required" jsonschema_description:"一句话描述调用该工具的意图，例如批量接受本轮 Craftsman 提交的预览图 RenderPlan 并提交 Worker。不要超过 160 个中文字符。"`
	RenderPlanID         string                    `json:"render_plan_id" jsonschema_description:"单条决策模式使用：Producer 要决策的 RenderPlan UUID。批量处理时请使用 decisions 数组。"`
	Decision             string                    `json:"decision" jsonschema:"enum=accept,enum=reject" jsonschema_description:"单条决策模式使用：accept 表示接受该 RenderPlan；reject 表示拒绝该 RenderPlan。批量处理时请使用 decisions 数组。"`
	Reason               string                    `json:"reason" jsonschema_description:"单条决策模式使用：接受或拒绝的原因，面向用户和审计可读。批量处理时每个 decisions 项都要填写 reason。"`
	NextAction           string                    `json:"next_action" jsonschema:"enum=submit_worker,enum=revise_with_craftsman,enum=no_action" jsonschema_description:"单条决策模式使用：accept 只能配 submit_worker；reject 可配 revise_with_craftsman 或 no_action。请求用户确认请另行调用 request_user_decision。"`
	RevisionInstructions string                    `json:"revision_instructions" jsonschema_description:"单条决策模式使用：reject 且 next_action=revise_with_craftsman 时，给后续 Craftsman 修订 RenderPlan 的具体要求。"`
	Decisions            []RenderPlanDecisionInput `json:"decisions" jsonschema_description:"批量决策列表。处理 system-reminder 中多条 craftsman_render_plan_ready signal 时优先使用该字段；每一项独立指定 render_plan_id、decision、reason、next_action 和可选 revision_instructions。"`
}

type RenderPlanDecisionInput struct {
	RenderPlanID         string `json:"render_plan_id" jsonschema:"required" jsonschema_description:"本项要决策的 RenderPlan UUID。"`
	Decision             string `json:"decision" jsonschema:"required,enum=accept,enum=reject" jsonschema_description:"本项决策。accept 表示接受该 RenderPlan；reject 表示拒绝该 RenderPlan。"`
	Reason               string `json:"reason" jsonschema:"required" jsonschema_description:"本项接受或拒绝的原因，面向用户和审计可读。"`
	NextAction           string `json:"next_action" jsonschema:"required,enum=submit_worker,enum=revise_with_craftsman,enum=no_action" jsonschema_description:"本项下一步动作。accept 只能配 submit_worker；reject 可配 revise_with_craftsman 或 no_action。"`
	RevisionInstructions string `json:"revision_instructions" jsonschema_description:"本项 reject 且 next_action=revise_with_craftsman 时，给后续 Craftsman 修订 RenderPlan 的具体要求。"`
}

func NewDecideRenderPlanNativeTool(store RenderPlanDecisionStore, runtime RenderPlanSubmitRuntime, enqueuer WorkerTaskEnqueuer) *DecideRenderPlanNativeTool {
	return &DecideRenderPlanNativeTool{store: store, submitter: NewRenderPlanSubmitter(store, runtime, enqueuer)}
}

func (t *DecideRenderPlanNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[DecideRenderPlanInput](toolDecideRenderPlan, "Producer 对一个或多个等待决策的 RenderPlan 做 accept/reject。处理多条 craftsman_render_plan_ready signal 时应使用 decisions 批量参数；accept 会提交 worker_generation；reject 只标记计划被拒绝，不会直接重写计划或生成图片视频。")
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
	decisions := normalizedRenderPlanDecisions(input)
	if len(decisions) == 1 {
		return t.runSingleDecision(ctx, runtime, decisions[0])
	}
	items := make([]NaturalResultItem, 0, len(decisions))
	for index, decision := range decisions {
		result, errText := t.applyDecision(ctx, runtime, decision)
		label := fmt.Sprintf("决策 %d", index+1)
		if errText != "" {
			items = append(items, NaturalResultItem{Label: label, Value: decision.RenderPlanID + " 失败：" + errText})
			continue
		}
		items = append(items, NaturalResultItem{Label: label, Value: result})
	}
	return NaturalResult{
		Title: "批量 RenderPlan 决策完成",
		Items: items,
		Next:  "Producer 应读取项目上下文确认 RenderPlan、WorkerTask 和 signal 状态；如有失败项，请只重试失败的 RenderPlan。",
	}.String(), nil
}

func (t *DecideRenderPlanNativeTool) runSingleDecision(ctx context.Context, runtime NativeRuntimeContext, input RenderPlanDecisionInput) (string, error) {
	renderPlanID, _ := pgUUIDFromString(input.RenderPlanID)
	switch input.Decision {
	case "accept":
		task, _, err := t.submitter.SubmitRenderPlan(ctx, runtime.WorkspaceID, renderPlanID, runtime.ThreadID, input.Reason)
		if err != nil {
			return NaturalToolError(toolDecideRenderPlan, err.Error(), "请读取 RenderPlan 状态，确认它处于 compiled 或 waiting_for_approval 后重试。"), nil
		}
		signalNote := t.markRenderPlanReadySignalProcessed(ctx, runtime, renderPlanID)
		next := "Worker 已入队。Producer 后续应读取 generation_job、artifact_version 和 RenderPlan 状态，不要把提交等同于产物完成。"
		if signalNote != "" {
			next += "\n" + signalNote
		}
		return NaturalResult{
			Title: "已接受 RenderPlan 并提交 worker_generation",
			Items: []NaturalResultItem{
				{Label: "RenderPlan", Value: input.RenderPlanID},
				{Label: "WorkerTask", Value: uuidString(task.ID)},
				{Label: "原因", Value: input.Reason},
			},
			Next: next,
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
		signalNote := t.markRenderPlanReadySignalProcessed(ctx, runtime, renderPlanID)
		next := "如需修订，请 Producer 再调用 dispatch_craftsman，并在 critique / fix_hints 中带上 revision_instructions。"
		if signalNote != "" {
			next += "\n" + signalNote
		}
		return NaturalResult{
			Title: "已拒绝 RenderPlan",
			Items: []NaturalResultItem{
				{Label: "RenderPlan", Value: input.RenderPlanID},
				{Label: "下一步", Value: input.NextAction},
				{Label: "原因", Value: input.Reason},
			},
			Next: next,
		}.String(), nil
	}
}

func (t *DecideRenderPlanNativeTool) applyDecision(ctx context.Context, runtime NativeRuntimeContext, input RenderPlanDecisionInput) (string, string) {
	renderPlanID, _ := pgUUIDFromString(input.RenderPlanID)
	switch input.Decision {
	case "accept":
		task, _, err := t.submitter.SubmitRenderPlan(ctx, runtime.WorkspaceID, renderPlanID, runtime.ThreadID, input.Reason)
		if err != nil {
			return "", err.Error()
		}
		signalNote := t.markRenderPlanReadySignalProcessed(ctx, runtime, renderPlanID)
		parts := []string{input.RenderPlanID, "accept", "worker=" + uuidString(task.ID)}
		if signalNote != "" {
			parts = append(parts, signalNote)
		}
		return strings.Join(parts, " / "), ""
	default:
		if t == nil || t.store == nil {
			return "", "render plan decision store 未配置"
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
			return "", err.Error()
		}
		signalNote := t.markRenderPlanReadySignalProcessed(ctx, runtime, renderPlanID)
		parts := []string{input.RenderPlanID, "reject", input.NextAction}
		if signalNote != "" {
			parts = append(parts, signalNote)
		}
		return strings.Join(parts, " / "), ""
	}
}

func (t *DecideRenderPlanNativeTool) markRenderPlanReadySignalProcessed(ctx context.Context, runtime NativeRuntimeContext, renderPlanID pgtype.UUID) string {
	if t == nil || t.submitter == nil || t.submitter.runtime == nil || !runtime.WorkspaceID.Valid || !runtime.TaskID.Valid || !renderPlanID.Valid {
		return ""
	}
	marker, ok := t.submitter.runtime.(RenderPlanDecisionSignalRuntime)
	if !ok {
		return ""
	}
	signals, err := marker.MarkProducerPendingSignalsProcessedByRenderPlan(ctx, runtime.WorkspaceID, renderPlanID, runtime.TaskID)
	if err != nil {
		return "Signal 回写警告：RenderPlan 已完成决策，但 pending signal 标记 processed 失败：" + strings.TrimSpace(err.Error())
	}
	if len(signals) == 0 {
		return "Signal 回写：未找到对应的待处理 craftsman_render_plan_ready signal，可能已被其他 Producer 任务处理。"
	}
	return fmt.Sprintf("Signal 回写：已将 %d 个 craftsman_render_plan_ready signal 标记为 processed。", len(signals))
}

func validateDecideRenderPlanInput(input DecideRenderPlanInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	decisions := normalizedRenderPlanDecisions(input)
	if len(decisions) == 0 {
		return fmt.Errorf("必须填写 render_plan_id/decision/reason/next_action，或提供 decisions 批量决策列表")
	}
	for index, decision := range decisions {
		if err := validateRenderPlanDecisionInput(decision, fmt.Sprintf("decisions[%d]", index)); err != nil {
			return err
		}
	}
	return nil
}

func normalizedRenderPlanDecisions(input DecideRenderPlanInput) []RenderPlanDecisionInput {
	if len(input.Decisions) > 0 {
		return input.Decisions
	}
	if strings.TrimSpace(input.RenderPlanID) == "" &&
		strings.TrimSpace(input.Decision) == "" &&
		strings.TrimSpace(input.Reason) == "" &&
		strings.TrimSpace(input.NextAction) == "" {
		return nil
	}
	return []RenderPlanDecisionInput{{
		RenderPlanID:         input.RenderPlanID,
		Decision:             input.Decision,
		Reason:               input.Reason,
		NextAction:           input.NextAction,
		RevisionInstructions: input.RevisionInstructions,
	}}
}

func validateRenderPlanDecisionInput(input RenderPlanDecisionInput, fieldPrefix string) error {
	if fieldPrefix == "" {
		fieldPrefix = "decision"
	}
	if _, ok := pgUUIDFromString(input.RenderPlanID); !ok {
		return fmt.Errorf("%s.render_plan_id 必须是 UUID", fieldPrefix)
	}
	if err := requireMode(input.Decision, "accept", "reject"); err != nil {
		return fmt.Errorf("%s.decision %w", fieldPrefix, err)
	}
	if err := requireText(input.Reason, fieldPrefix+".reason"); err != nil {
		return err
	}
	if err := requireMode(input.NextAction, "submit_worker", "revise_with_craftsman", "no_action"); err != nil {
		return fmt.Errorf("%s.next_action %w", fieldPrefix, err)
	}
	if input.Decision == "accept" && input.NextAction != "submit_worker" {
		return fmt.Errorf("%s accept 只能搭配 next_action=submit_worker", fieldPrefix)
	}
	if input.Decision == "reject" && input.NextAction == "submit_worker" {
		return fmt.Errorf("%s reject 不能搭配 next_action=submit_worker", fieldPrefix)
	}
	if input.Decision == "reject" && input.NextAction == "revise_with_craftsman" && strings.TrimSpace(input.RevisionInstructions) == "" {
		return fmt.Errorf("%s revise_with_craftsman 需要 revision_instructions", fieldPrefix)
	}
	return nil
}
