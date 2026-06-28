package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	agentidentity "github.com/sinmaystar/clip-anvil/internal/agent/identity"
	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/agent/uimessage"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type DispatchReviewerStore interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	GetMediaNodeByID(ctx context.Context, id pgtype.UUID) (db.MediaNode, error)
	GetArtifactVersionByID(ctx context.Context, id pgtype.UUID) (db.ArtifactVersion, error)
	GetRenderPlanByID(ctx context.Context, params db.GetRenderPlanByIDParams) (db.RenderPlan, error)
	ListReviewRecordsByArtifactVersion(ctx context.Context, artifactVersionID pgtype.UUID) ([]db.ReviewRecord, error)
	GetAgentObjectBySemanticKey(ctx context.Context, params db.GetAgentObjectBySemanticKeyParams) (db.AgentObjectIndex, error)
}

type DispatchReviewerRuntime interface {
	GetOrCreateReviewerThreadForScope(ctx context.Context, workspaceID pgtype.UUID, scopeType string, scopeID pgtype.UUID) (db.AgentThread, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
	AppendMessage(ctx context.Context, params agentruntime.AppendMessageParams) (db.AgentMessage, error)
}

type DispatchReviewerTaskEnqueuer interface {
	EnqueueReviewerTask(ctx context.Context, task db.AgentTask)
}

type DispatchReviewerNativeTool struct {
	store    DispatchReviewerStore
	runtime  DispatchReviewerRuntime
	enqueuer DispatchReviewerTaskEnqueuer
}

type DispatchReviewerInput struct {
	Brief        string            `json:"brief" jsonschema:"required" jsonschema_description:"本次派发评审的业务目的，例如评审 shot_01 的分镜视频是否可进入剪辑。不要超过 160 个中文字符。"`
	ReviewTask   string            `json:"review_task" jsonschema:"required,enum=pre_render_plan_review,enum=preview_image_review,enum=shot_video_review,enum=final_video_review" jsonschema_description:"评审任务类型。pre_render_plan_review 评审生成计划；preview_image_review 评审分镜图；shot_video_review 评审分镜视频；final_video_review 评审成片。"`
	Target       ReviewTargetInput `json:"target" jsonschema:"required" jsonschema_description:"被评审对象。必须与 review_task 匹配。"`
	Policy       ReviewPolicyInput `json:"policy" jsonschema_description:"评审策略。通常留空使用默认 10 轴策略；需要更严格广告验收时可提高阈值。"`
	AutoDecision AutoDecisionInput `json:"auto_decision" jsonschema_description:"Producer 对评审后自动推进的授权范围。默认不自动重跑，只记录结果。"`
	Reason       string            `json:"reason" jsonschema:"required" jsonschema_description:"为什么现在需要 Reviewer 评审。必须说明生产阶段、风险或用户目标。"`
}

type ReviewPolicyInput struct {
	OverallThreshold float64  `json:"overall_threshold" jsonschema_description:"整体通过阈值，范围 0 到 1。为空使用默认值。"`
	AxisThreshold    float64  `json:"axis_threshold" jsonschema_description:"必选轴通过阈值，范围 0 到 1。为空使用默认值。"`
	RequiredAxes     []string `json:"required_axes" jsonschema_description:"覆盖默认必评轴。通常不要填写，除非 Producer 明确要做更严格或更轻量的评审。"`
	MaxAttempts      int32    `json:"max_attempts" jsonschema_description:"同一 review 链路最大尝试次数。默认 3。达到后 Producer 应请求用户决策或标记 manual。"`
}

type AutoDecisionInput struct {
	AllowAutoAccept     bool `json:"allow_auto_accept" jsonschema_description:"Reviewer accepted 后是否允许工程自动标记通过。用户要求确认时必须为 false。"`
	AllowAutoRepair     bool `json:"allow_auto_repair" jsonschema_description:"是否允许 Producer 后续自动派 Craftsman 修复。默认 false，除非用户已授权自动修复。"`
	RequireUserOnReject bool `json:"require_user_on_reject" jsonschema_description:"rejected 后是否必须先问用户。连续失败、高成本视频或审美争议建议 true。"`
}

func NewDispatchReviewerNativeTool(store DispatchReviewerStore, runtime DispatchReviewerRuntime, enqueuer DispatchReviewerTaskEnqueuer) *DispatchReviewerNativeTool {
	return &DispatchReviewerNativeTool{store: store, runtime: runtime, enqueuer: enqueuer}
}

func (t *DispatchReviewerNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[DispatchReviewerInput](toolDispatchReviewer, "派发 Reviewer 对 RenderPlan、preview image、shot video 或 final video 进行质量评审。这个工具只创建 reviewer_turn task，不直接修改 RenderPlan、选择版本或触发重跑。")
}

func (t *DispatchReviewerNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs(toolDispatchReviewer, argumentsInJSON, validateDispatchReviewerInput)
	if !ok {
		return msg, nil
	}
	if t.store == nil || t.runtime == nil {
		return NaturalToolError(toolDispatchReviewer, "dispatch reviewer service 未配置。", "请检查服务端 wiring 后重试。"), nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolDispatchReviewer)
	if !ok {
		return msg, nil
	}
	resolvedTarget, targetSummary, err := t.validateTargetState(ctx, runtime.WorkspaceID, input)
	if err != nil {
		return NaturalToolError(toolDispatchReviewer, err.Error(), "请读取项目上下文，确认 review_task、target 和 artifact 状态后重试。"), nil
	}
	input.Target = resolvedTarget
	scopeType, scopeID, err := dispatchReviewerScope(input)
	if err != nil {
		return NaturalToolError(toolDispatchReviewer, err.Error(), "请修正 target 后重试。"), nil
	}
	thread, err := t.runtime.GetOrCreateReviewerThreadForScope(ctx, runtime.WorkspaceID, scopeType, scopeID)
	if err != nil {
		return NaturalToolError(toolDispatchReviewer, err.Error(), "请检查 reviewer thread scope 配置后重试。"), nil
	}
	rawInput, _ := json.Marshal(dispatchReviewerTaskInput{
		DispatchReviewerInput: input,
		ProducerThreadID:      uuidString(runtime.ThreadID),
		ProducerTaskID:        uuidString(runtime.TaskID),
	})
	maxAttempts := input.Policy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	task, err := t.runtime.CreateTask(ctx, agentruntime.CreateTaskParams{
		WorkspaceID: runtime.WorkspaceID,
		ThreadID:    thread.ID,
		Role:        "reviewer",
		ScopeType:   scopeType,
		ScopeID:     scopeID,
		TaskType:    "reviewer_turn",
		MaxAttempts: maxAttempts,
		Input:       rawInput,
	})
	if err != nil {
		return NaturalToolError(toolDispatchReviewer, err.Error(), "请检查 reviewer task 参数后重试。"), nil
	}
	if err := t.appendDelegationMessage(ctx, runtime.WorkspaceID, thread.ID, task.ID, scopeType, scopeID, targetSummary, input); err != nil {
		return NaturalToolError(toolDispatchReviewer, err.Error(), "Reviewer 任务已创建，但委派消息写入失败；请检查 agent_message 写入链路。"), nil
	}
	_, _ = t.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: runtime.WorkspaceID,
		ThreadID:    thread.ID,
		TaskID:      task.ID,
		EventType:   "review_queued",
		SourceRole:  "producer",
		TargetRole:  "reviewer",
		Scope:       mustJSON(map[string]any{"review_task": input.ReviewTask, "target": input.Target}),
		Payload:     mustJSON(map[string]any{"brief": input.Brief, "reason": input.Reason}),
	})
	if t.enqueuer != nil {
		t.enqueuer.EnqueueReviewerTask(ctx, task)
	}
	return NaturalResult{
		Title: "已派发 Reviewer 评审任务",
		Items: []NaturalResultItem{
			{Label: "review_task", Value: input.ReviewTask},
			{Label: "scope_ref", Value: targetSummary.scopeLabel(scopeType)},
			{Label: "artifact_ref", Value: targetSummary.artifactLabel()},
		},
		Next: "派发成功只表示评审任务已入队。Producer 应读取 review_record 和 artifact_issue 后决定是否接受、修复或请求用户确认。",
	}.String(), nil
}

type dispatchReviewerTaskInput struct {
	DispatchReviewerInput
	ProducerThreadID string `json:"producer_thread_id,omitempty"`
	ProducerTaskID   string `json:"producer_task_id,omitempty"`
}

func (t *DispatchReviewerNativeTool) appendDelegationMessage(ctx context.Context, workspaceID pgtype.UUID, threadID pgtype.UUID, taskID pgtype.UUID, scopeType string, scopeID pgtype.UUID, targetSummary reviewerTargetSummary, input DispatchReviewerInput) error {
	text := reviewerDelegationText(scopeType, targetSummary, input)
	content, err := uimessage.BuildUserMessageContent(uimessage.UserMessageInput{Text: text})
	if err != nil {
		return err
	}
	_, err = t.runtime.AppendMessage(ctx, agentruntime.AppendMessageParams{
		WorkspaceID: workspaceID,
		ThreadID:    threadID,
		Role:        "user",
		MessageType: "text",
		Content:     content,
		RawMessage: mustJSON(map[string]any{
			"schema":      "clipanvil.agent.delegation.v1",
			"target_role": "reviewer",
			"scope_type":  scopeType,
			"scope_id":    uuidString(scopeID),
			"scope_ref":   targetSummary.scopeLabel(scopeType),
			"review_task": input.ReviewTask,
			"target":      input.Target,
			"brief":       input.Brief,
			"reason":      input.Reason,
		}),
		TaskID: taskID,
	})
	return err
}

func reviewerDelegationText(scopeType string, targetSummary reviewerTargetSummary, input DispatchReviewerInput) string {
	lines := []string{
		"Producer 派发 Reviewer 评审任务。",
		"- scope_ref: " + targetSummary.scopeLabel(scopeType),
		"- review_task: " + input.ReviewTask,
		"- artifact_ref: " + targetSummary.artifactLabel(),
		"- brief: " + input.Brief,
		"- reason: " + input.Reason,
	}
	return strings.Join(lines, "\n")
}

func validateDispatchReviewerInput(input DispatchReviewerInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if err := requireMode(input.ReviewTask, reviewTaskPreRenderPlan, reviewTaskPreviewImage, reviewTaskShotVideo, reviewTaskFinalVideo); err != nil {
		return err
	}
	if err := validateReviewTarget(input.ReviewTask, input.Target); err != nil {
		return err
	}
	if err := requireText(input.Reason, "reason"); err != nil {
		return err
	}
	if input.Policy.OverallThreshold < 0 || input.Policy.OverallThreshold > 1 {
		return fmt.Errorf("policy.overall_threshold 必须在 0 到 1 之间")
	}
	if input.Policy.AxisThreshold < 0 || input.Policy.AxisThreshold > 1 {
		return fmt.Errorf("policy.axis_threshold 必须在 0 到 1 之间")
	}
	if input.Policy.MaxAttempts < 0 || input.Policy.MaxAttempts > 3 {
		return fmt.Errorf("policy.max_attempts 必须在 1 到 3 之间，或留空使用默认值")
	}
	return nil
}

type reviewerTargetSummary struct {
	shotKey       string
	renderPlanKey string
	nodeKey       string
	versionKey    string
}

func (s reviewerTargetSummary) scopeLabel(scopeType string) string {
	switch strings.TrimSpace(scopeType) {
	case "render_plan":
		return objectLabel(agentidentity.ObjectRenderPlan, s.renderPlanKey)
	case "final_output":
		return objectLabel(agentidentity.ObjectMediaNode, s.nodeKey)
	default:
		return objectLabel(agentidentity.ObjectShot, s.shotKey)
	}
}

func (s reviewerTargetSummary) artifactLabel() string {
	if strings.TrimSpace(s.versionKey) != "" {
		return objectLabel(agentidentity.ObjectArtifactVersion, s.versionKey)
	}
	if strings.TrimSpace(s.nodeKey) != "" {
		return objectLabel(agentidentity.ObjectMediaNode, s.nodeKey)
	}
	return "artifact_version.semantic_key_missing"
}

func objectLabel(objectType string, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return objectType + ".semantic_key_missing"
	}
	return objectType + "/" + key
}

func (t *DispatchReviewerNativeTool) validateTargetState(ctx context.Context, workspaceID pgtype.UUID, input DispatchReviewerInput) (ReviewTargetInput, reviewerTargetSummary, error) {
	workspace, err := t.store.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return ReviewTargetInput{}, reviewerTargetSummary{}, err
	}
	if workspace.Mode != db.WorkspaceModeAgent {
		return ReviewTargetInput{}, reviewerTargetSummary{}, fmt.Errorf("dispatch_reviewer requires an Agent workspace")
	}
	target := input.Target
	summary := reviewerTargetSummary{}
	if input.ReviewTask == reviewTaskPreRenderPlan {
		renderPlanID, renderPlanKey, err := t.resolveReviewerObjectID(ctx, workspaceID, agentidentity.ObjectRenderPlan, target.RenderPlanRef, target.RenderPlanID, "target.render_plan_ref")
		if err != nil {
			return ReviewTargetInput{}, reviewerTargetSummary{}, err
		}
		target.RenderPlanID = uuidString(renderPlanID)
		target.RenderPlanRef = ensureToolObjectRef(target.RenderPlanRef, agentidentity.ObjectRenderPlan, renderPlanKey)
		summary.renderPlanKey = renderPlanKey
		plan, err := t.store.GetRenderPlanByID(ctx, db.GetRenderPlanByIDParams{WorkspaceID: workspaceID, ID: renderPlanID})
		if err != nil {
			return ReviewTargetInput{}, reviewerTargetSummary{}, err
		}
		if plan.WorkspaceID != workspaceID {
			return ReviewTargetInput{}, reviewerTargetSummary{}, fmt.Errorf("render_plan 不属于当前 workspace")
		}
		if strings.TrimSpace(summary.renderPlanKey) == "" {
			summary.renderPlanKey = plan.SemanticKey
			target.RenderPlanRef = ensureToolObjectRef(target.RenderPlanRef, agentidentity.ObjectRenderPlan, plan.SemanticKey)
		}
		return target, summary, nil
	}
	nodeID, nodeKey, err := t.resolveReviewerObjectID(ctx, workspaceID, agentidentity.ObjectMediaNode, target.NodeRef, target.NodeID, "target.node_ref")
	if err != nil {
		return ReviewTargetInput{}, reviewerTargetSummary{}, err
	}
	versionID, versionKey, err := t.resolveReviewerObjectID(ctx, workspaceID, agentidentity.ObjectArtifactVersion, target.ArtifactVersionRef, target.ArtifactVersionID, "target.artifact_version_ref")
	if err != nil {
		return ReviewTargetInput{}, reviewerTargetSummary{}, err
	}
	target.NodeID = uuidString(nodeID)
	target.ArtifactVersionID = uuidString(versionID)
	target.NodeRef = ensureToolObjectRef(target.NodeRef, agentidentity.ObjectMediaNode, nodeKey)
	target.ArtifactVersionRef = ensureToolObjectRef(target.ArtifactVersionRef, agentidentity.ObjectArtifactVersion, versionKey)
	summary.nodeKey = nodeKey
	summary.versionKey = versionKey
	if input.ReviewTask == reviewTaskPreviewImage || input.ReviewTask == reviewTaskShotVideo {
		shotID, shotKey, err := t.resolveReviewerObjectID(ctx, workspaceID, agentidentity.ObjectShot, target.ShotRef, target.ShotID, "target.shot_ref")
		if err != nil {
			return ReviewTargetInput{}, reviewerTargetSummary{}, err
		}
		target.ShotID = uuidString(shotID)
		target.ShotRef = ensureToolObjectRef(target.ShotRef, agentidentity.ObjectShot, shotKey)
		summary.shotKey = shotKey
	}
	if strings.TrimSpace(target.RenderPlanID) != "" || strings.TrimSpace(target.RenderPlanRef.Key) != "" {
		if renderPlanID, renderPlanKey, err := t.resolveReviewerObjectID(ctx, workspaceID, agentidentity.ObjectRenderPlan, target.RenderPlanRef, target.RenderPlanID, "target.render_plan_ref"); err == nil {
			target.RenderPlanID = uuidString(renderPlanID)
			target.RenderPlanRef = ensureToolObjectRef(target.RenderPlanRef, agentidentity.ObjectRenderPlan, renderPlanKey)
			summary.renderPlanKey = renderPlanKey
		}
	}
	node, err := t.store.GetMediaNodeByID(ctx, nodeID)
	if err != nil {
		return ReviewTargetInput{}, reviewerTargetSummary{}, err
	}
	version, err := t.store.GetArtifactVersionByID(ctx, versionID)
	if err != nil {
		return ReviewTargetInput{}, reviewerTargetSummary{}, err
	}
	if node.WorkspaceID != workspaceID || version.WorkspaceID != workspaceID || version.NodeID != node.ID {
		return ReviewTargetInput{}, reviewerTargetSummary{}, fmt.Errorf("target node 或 artifact_version 不属于当前 workspace")
	}
	if version.Status != db.JobStatusSucceeded {
		return ReviewTargetInput{}, reviewerTargetSummary{}, fmt.Errorf("artifact_version.status=%s，不可评审，必须是 succeeded", version.Status)
	}
	if input.ReviewTask == reviewTaskFinalVideo {
		if err := t.ensureNoCompletedFinalVideoReview(ctx, version.ID); err != nil {
			return ReviewTargetInput{}, reviewerTargetSummary{}, err
		}
	}
	if input.ReviewTask == reviewTaskShotVideo && node.NodeType != db.NodeTypeVideo {
		return ReviewTargetInput{}, reviewerTargetSummary{}, fmt.Errorf("shot_video_review 的目标 node 必须是 video")
	}
	if input.ReviewTask == reviewTaskPreviewImage && node.NodeType != db.NodeTypeImage {
		return ReviewTargetInput{}, reviewerTargetSummary{}, fmt.Errorf("preview_image_review 的目标 node 必须是 image")
	}
	if strings.TrimSpace(summary.nodeKey) == "" {
		summary.nodeKey = node.SemanticKey
		target.NodeRef = ensureToolObjectRef(target.NodeRef, agentidentity.ObjectMediaNode, node.SemanticKey)
	}
	if strings.TrimSpace(summary.versionKey) == "" {
		summary.versionKey = version.SemanticKey
		target.ArtifactVersionRef = ensureToolObjectRef(target.ArtifactVersionRef, agentidentity.ObjectArtifactVersion, version.SemanticKey)
	}
	return target, summary, nil
}

func (t *DispatchReviewerNativeTool) ensureNoCompletedFinalVideoReview(ctx context.Context, artifactVersionID pgtype.UUID) error {
	records, err := t.store.ListReviewRecordsByArtifactVersion(ctx, artifactVersionID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.ReviewTask != reviewTaskFinalVideo && record.TargetPhase != "final_video" {
			continue
		}
		switch record.Status {
		case reviewVerdictAccepted, reviewVerdictAcceptedWithWarnings, reviewVerdictRejected, reviewVerdictBlocked:
			recordRef := strings.TrimSpace(record.SemanticKey)
			if recordRef == "" {
				recordRef = uuidString(record.ID)
			}
			return fmt.Errorf("final_video artifact_version 已有终态评审 %s：review_record/%s；请读取项目上下文并处理该评审结果，不要重复派发 Reviewer", record.Status, recordRef)
		}
	}
	return nil
}

func (t *DispatchReviewerNativeTool) resolveReviewerObjectID(ctx context.Context, workspaceID pgtype.UUID, objectType string, ref ToolObjectRef, legacyValue string, field string) (pgtype.UUID, string, error) {
	key := strings.TrimSpace(ref.Key)
	legacyValue = strings.TrimSpace(legacyValue)
	if key != "" {
		if strings.TrimSpace(ref.Type) == "" {
			ref.Type = objectType
		}
		if strings.TrimSpace(ref.Type) != objectType {
			return pgtype.UUID{}, "", fmt.Errorf("%s.type 必须是 %s", field, objectType)
		}
		return t.resolveReviewerSemanticKey(ctx, workspaceID, objectType, key, field)
	}
	if id, ok := pgUUIDFromString(legacyValue); ok {
		return id, "", nil
	}
	if legacyValue != "" {
		return t.resolveReviewerSemanticKey(ctx, workspaceID, objectType, legacyValue, field)
	}
	return pgtype.UUID{}, "", fmt.Errorf("%s 必填，请使用 read_project_context 返回的 %s semantic_key，不要填写或编造内部 ID", field, objectType)
}

func (t *DispatchReviewerNativeTool) resolveReviewerSemanticKey(ctx context.Context, workspaceID pgtype.UUID, objectType string, key string, field string) (pgtype.UUID, string, error) {
	object, err := t.store.GetAgentObjectBySemanticKey(ctx, db.GetAgentObjectBySemanticKeyParams{
		WorkspaceID: workspaceID,
		ObjectType:  objectType,
		SemanticKey: strings.TrimSpace(key),
	})
	if err != nil {
		return pgtype.UUID{}, "", fmt.Errorf("%s 找不到 %s/%s，请先读取项目上下文确认真实 semantic_key", field, objectType, key)
	}
	return object.ObjectID, object.SemanticKey, nil
}

func ensureToolObjectRef(ref ToolObjectRef, objectType string, key string) ToolObjectRef {
	if strings.TrimSpace(ref.Type) == "" {
		ref.Type = objectType
	}
	if strings.TrimSpace(ref.Key) == "" {
		ref.Key = strings.TrimSpace(key)
	}
	return ref
}

func dispatchReviewerScope(input DispatchReviewerInput) (string, pgtype.UUID, error) {
	switch input.ReviewTask {
	case reviewTaskPreRenderPlan:
		id, _ := pgUUIDFromString(input.Target.RenderPlanID)
		return "render_plan", id, nil
	case reviewTaskFinalVideo:
		id, _ := pgUUIDFromString(input.Target.NodeID)
		return "final_output", id, nil
	default:
		id, _ := pgUUIDFromString(input.Target.ShotID)
		return "shot", id, nil
	}
}
