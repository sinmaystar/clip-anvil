package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

const (
	reviewTaskPreRenderPlan = "pre_render_plan_review"
	reviewTaskPreviewImage  = "preview_image_review"
	reviewTaskShotVideo     = "shot_video_review"
	reviewTaskFinalVideo    = "final_video_review"

	reviewVerdictAccepted             = "accepted"
	reviewVerdictAcceptedWithWarnings = "accepted_with_warnings"
	reviewVerdictRejected             = "rejected"
	reviewVerdictBlocked              = "blocked"
)

var reviewTaskAxes = map[string][]string{
	reviewTaskPreRenderPlan: {"faithfulness", "subject_consistency", "continuity"},
	reviewTaskPreviewImage:  {"faithfulness", "subject_consistency", "product_visibility", "brand_style_consistency", "composition_proportion", "visual_quality"},
	reviewTaskShotVideo:     {"faithfulness", "subject_consistency", "product_visibility", "brand_style_consistency", "composition_proportion", "visual_quality", "motion_physics", "continuity", "audio_sync"},
	reviewTaskFinalVideo:    {"faithfulness", "brand_style_consistency", "visual_quality", "continuity", "audio_sync", "platform_selling_power"},
}

var reviewAxes = map[string]bool{
	"faithfulness":            true,
	"subject_consistency":     true,
	"product_visibility":      true,
	"brand_style_consistency": true,
	"composition_proportion":  true,
	"motion_physics":          true,
	"visual_quality":          true,
	"continuity":              true,
	"audio_sync":              true,
	"platform_selling_power":  true,
}

var reviewIssueDimensions = map[string]bool{
	"faithfulness":            true,
	"subject_consistency":     true,
	"product_visibility":      true,
	"brand_style_consistency": true,
	"composition_proportion":  true,
	"motion_physics":          true,
	"visual_quality":          true,
	"continuity":              true,
	"audio_sync":              true,
	"platform_selling_power":  true,
	"model_capability":        true,
	"prompt_validity":         true,
	"reference_role_validity": true,
	"cost_risk":               true,
	"dependency_not_ready":    true,
	"project_memory_conflict": true,
}

type SubmitReviewResultStore interface {
	CreateReviewRecord(ctx context.Context, params db.CreateReviewRecordParams) (db.ReviewRecord, error)
	CompleteReviewRecord(ctx context.Context, params db.CompleteReviewRecordParams) (db.ReviewRecord, error)
	CreateArtifactIssue(ctx context.Context, params db.CreateArtifactIssueParams) (db.ArtifactIssue, error)
}

type SubmitReviewResultNativeTool struct {
	store SubmitReviewResultStore
}

type ReviewTargetInput struct {
	WorkspaceScope       string        `json:"workspace_scope" jsonschema:"enum=shot,enum=render_plan,enum=final_video" jsonschema_description:"目标归属范围。必须沿用当前 Reviewer 任务 target，不要推断或替换。pre_render_plan_review 通常是 render_plan；preview_image_review / shot_video_review 通常是 shot；final_video_review 是 final_video。"`
	ShotRef              ToolObjectRef `json:"shot_ref" jsonschema_description:"分镜语义引用。preview_image_review 和 shot_video_review 使用 read_project_context 返回的 type=shot,key=...。"`
	RenderPlanRef        ToolObjectRef `json:"render_plan_ref" jsonschema_description:"RenderPlan 语义引用。pre_render_plan_review 必填；artifact review 中可选。使用 type=render_plan,key=...。"`
	NodeRef              ToolObjectRef `json:"node_ref" jsonschema_description:"媒体节点语义引用。artifact review 使用 read_project_context 返回的 type=media_node,key=...。"`
	ArtifactVersionRef   ToolObjectRef `json:"artifact_version_ref" jsonschema_description:"被评审 artifact_version 语义引用。artifact review 使用 type=artifact_version,key=...。"`
	ParentReviewRef      ToolObjectRef `json:"parent_review_ref" jsonschema_description:"上一条 review_record 的语义引用；没有则留空。"`
	ShotID               string        `json:"shot_id" jsonschema_description:"兼容旧字段：内部 ID 或历史语义键。模型不要填写；请优先使用 shot_ref。"`
	RenderPlanID         string        `json:"render_plan_id" jsonschema_description:"兼容旧字段：内部 ID 或历史语义键。模型不要填写；请优先使用 render_plan_ref。"`
	NodeID               string        `json:"node_id" jsonschema_description:"兼容旧字段：内部 ID 或历史语义键。模型不要填写；请优先使用 node_ref。"`
	ArtifactVersionID    string        `json:"artifact_version_id" jsonschema_description:"兼容旧字段：内部 ID 或历史语义键。模型不要填写；请优先使用 artifact_version_ref。"`
	GenerationJobID      string        `json:"generation_job_id" jsonschema_description:"兼容旧字段：内部 ID。通常由当前 Reviewer 任务 target 自动注入，模型不要填写。"`
	ParentReviewRecordID string        `json:"parent_review_record_id" jsonschema_description:"兼容旧字段：内部 ID。通常由当前 Reviewer 任务 target 自动注入，模型不要填写。"`
}

type SubmitReviewResultInput struct {
	Brief               string                   `json:"brief" jsonschema:"required" jsonschema_description:"提交评审结果的业务目的，例如提交 shot_01 视频评审并指出商品漂移问题。"`
	ReviewTask          string                   `json:"review_task" jsonschema:"required,enum=pre_render_plan_review,enum=preview_image_review,enum=shot_video_review,enum=final_video_review" jsonschema_description:"评审任务类型。必须与当前 reviewer task 一致。"`
	Target              ReviewTargetInput        `json:"target" jsonschema:"required" jsonschema_description:"被评审对象。通常留空由当前 reviewer task 自动注入；如需填写，请使用 *_ref 语义引用，不要填写内部 ID。"`
	Verdict             string                   `json:"verdict" jsonschema:"required,enum=accepted,enum=accepted_with_warnings,enum=rejected,enum=blocked" jsonschema_description:"最终评审结论。accepted 可继续；accepted_with_warnings 可继续但需提示；rejected 阻塞推进；blocked 表示无法可靠评审。"`
	OverallScore        float64                  `json:"overall_score" jsonschema:"required" jsonschema_description:"整体评分，范围 0 到 1。blocked 时可填 0。"`
	Rubric              []ReviewRubricAxisInput  `json:"rubric" jsonschema:"required" jsonschema_description:"10 轴 rubric 的评分结果。必须包含当前 review_task 的 required axes。"`
	Critique            string                   `json:"critique" jsonschema:"required" jsonschema_description:"面向 Producer 和用户可读的评审摘要。必须指出通过理由或阻塞问题。"`
	Issues              []ReviewIssueInput       `json:"issues" jsonschema_description:"结构化问题列表。rejected 或 accepted_with_warnings 通常至少一条。"`
	RetryRecommendation RetryRecommendationInput `json:"retry_recommendation" jsonschema_description:"给 Producer 的下一步建议。Reviewer 只建议，不直接执行。"`
	EvidenceSummary     string                   `json:"evidence_summary" jsonschema_description:"证据摘要，例如参考图、分镜目标、画面帧、音频片段或 prompt 问题。不要写长篇逐帧日志。"`
	Reason              string                   `json:"reason" jsonschema:"required" jsonschema_description:"为什么给出这个 verdict。必须能和 rubric、issues 对上。"`
}

type ReviewRubricAxisInput struct {
	Axis     string  `json:"axis" jsonschema:"required" jsonschema_description:"评分轴。必须是 10 轴之一，pre-render 专用检查不要放这里，应放 issues.dimension。"`
	Score    float64 `json:"score" jsonschema:"required" jsonschema_description:"该轴评分，范围 0 到 1。"`
	Pass     bool    `json:"pass" jsonschema:"required" jsonschema_description:"该轴是否通过。低于阈值或有阻塞问题时为 false。"`
	Severity string  `json:"severity" jsonschema:"enum=info,enum=warning,enum=blocking" jsonschema_description:"问题严重程度。通过轴通常为 info；可继续但需注意为 warning；阻塞推进为 blocking。"`
	Reason   string  `json:"reason" jsonschema:"required" jsonschema_description:"评分理由。必须引用具体上下文或产物表现，不要只写“效果不好”。"`
	FixHint  string  `json:"fix_hint" jsonschema_description:"如果未通过或有 warning，给出具体修复建议。通过轴可为空。"`
}

type ReviewIssueInput struct {
	Dimension                string        `json:"dimension" jsonschema:"required" jsonschema_description:"问题维度。优先使用 10 轴之一；pre-render 可用 model_capability、prompt_validity、reference_role_validity、cost_risk、dependency_not_ready、project_memory_conflict。"`
	Severity                 string        `json:"severity" jsonschema:"required,enum=info,enum=warning,enum=blocking" jsonschema_description:"严重程度。blocking 会阻止继续推进。"`
	Title                    string        `json:"title" jsonschema:"required" jsonschema_description:"短标题，例如商品外观漂移、首尾帧不连续、运镜冲突。"`
	Description              string        `json:"description" jsonschema:"required" jsonschema_description:"问题描述。说明问题发生在哪里，以及为什么影响目标。"`
	Evidence                 string        `json:"evidence" jsonschema_description:"证据，例如画面区域、帧范围、prompt 片段、reference binding 或音频时间段。不要使用模型不可理解的裸 asset id。"`
	TargetObjectType         string        `json:"target_object_type" jsonschema:"required,enum=render_plan,enum=artifact_version,enum=shot,enum=final_video,enum=project_memory" jsonschema_description:"问题归属对象类型。不要把所有问题都挂在 artifact 上；prompt 问题归 RenderPlan，故事问题归 Shot。"`
	TargetObjectRef          ToolObjectRef `json:"target_object_ref" jsonschema_description:"问题归属对象语义引用。通常可留空；工具会按 target_object_type 从当前 target 自动派生。"`
	TargetObjectID           string        `json:"target_object_id" jsonschema_description:"兼容旧字段：内部 ID。模型不要填写；优先使用 target_object_ref，或只填写 target_object_type 让工具自动派生。"`
	SuggestedFix             string        `json:"suggested_fix" jsonschema:"required,enum=none,enum=regenerate,enum=edit,enum=extend,enum=bridge,enum=revise_render_plan,enum=revise_shot_plan,enum=manual" jsonschema_description:"建议修复动作。Reviewer 只建议，Producer 决定是否执行。"`
	FixHint                  string        `json:"fix_hint" jsonschema:"required" jsonschema_description:"具体修复建议。应该能直接帮助 Producer 派 Craftsman 或请求用户确认。"`
	RequiresUserConfirmation bool          `json:"requires_user_confirmation" jsonschema_description:"是否需要用户确认后才能修复。涉及审美偏好、成本较高或改变用户方向时为 true。"`
}

type RetryRecommendationInput struct {
	ShouldRepair             bool          `json:"should_repair" jsonschema_description:"是否建议修复。accepted 通常 false；rejected 通常 true，blocked 视情况。"`
	SuggestedFix             string        `json:"suggested_fix" jsonschema:"enum=none,enum=regenerate,enum=edit,enum=extend,enum=bridge,enum=revise_render_plan,enum=revise_shot_plan,enum=manual" jsonschema_description:"总体建议修复动作。"`
	TargetObjectType         string        `json:"target_object_type" jsonschema:"enum=render_plan,enum=shot,enum=artifact_version,enum=final_video" jsonschema_description:"建议 Producer 下一步处理哪个对象。"`
	TargetObjectRef          ToolObjectRef `json:"target_object_ref" jsonschema_description:"建议处理对象语义引用。通常可留空；Producer 会结合 target_object_type 和当前 target 判断。"`
	TargetObjectID           string        `json:"target_object_id" jsonschema_description:"兼容旧字段：内部 ID。模型不要填写；优先使用 target_object_ref。"`
	FixHints                 []string      `json:"fix_hints" jsonschema_description:"给 Producer / Craftsman 的具体修复建议列表。"`
	RequiresUserConfirmation bool          `json:"requires_user_confirmation" jsonschema_description:"是否必须先走 HITL。连续失败、高成本视频、manual 或审美争议通常为 true。"`
	EscalationReason         string        `json:"escalation_reason" jsonschema_description:"如果建议停止自动修复，说明原因，例如同一维度连续失败 3 次。"`
}

func NewSubmitReviewResultNativeTool(store SubmitReviewResultStore) *SubmitReviewResultNativeTool {
	return &SubmitReviewResultNativeTool{store: store}
}

func (t *SubmitReviewResultNativeTool) Info(context.Context) (*schema.ToolInfo, error) {
	return toolInfoFor[SubmitReviewResultInput](toolSubmitReviewResult, "Reviewer 提交当前评审任务的结构化结果。target 通常由 runtime 自动注入；如需显式填写，请使用当前 Reviewer 任务 target 中的语义 ref，不要填写或编造内部 ID。工具会写入 review_record，并根据 issues 创建 artifact_issue；不会修改 RenderPlan、ShotPlan、ProjectMemory，也不会直接触发重跑。")
}

func (t *SubmitReviewResultNativeTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	input, msg, ok := decodeToolArgs[SubmitReviewResultInput](toolSubmitReviewResult, argumentsInJSON, nil)
	if !ok {
		return msg, nil
	}
	if t.store == nil {
		return NaturalToolError(toolSubmitReviewResult, "Reviewer review store 未配置。", "请检查服务端 wiring 后重试。"), nil
	}
	runtime, msg, ok := runtimeOrError(ctx, toolSubmitReviewResult)
	if !ok {
		return msg, nil
	}
	input = applySubmitReviewResultRuntimeTarget(input, runtime)
	if err := validateSubmitReviewResultInput(input); err != nil {
		return NaturalToolError(toolSubmitReviewResult, err.Error(), "请修正参数后重试，不要重复提交相同错误参数。"), nil
	}
	record, err := t.createAndCompleteReview(ctx, runtime, input)
	if err != nil {
		return NaturalToolError(toolSubmitReviewResult, err.Error(), "请检查目标对象、workspace 归属和 review schema 后重试。"), nil
	}
	blocking := 0
	for _, issue := range input.Issues {
		if issue.Severity == "blocking" {
			blocking++
		}
	}
	return NaturalResult{
		Title: fmt.Sprintf("已提交 Reviewer 评审结果：%s", input.Verdict),
		Items: []NaturalResultItem{
			{Label: "review_record_ref", Value: reviewRecordRef(record)},
			{Label: "review_task", Value: input.ReviewTask},
			{Label: "verdict", Value: input.Verdict},
			{Label: "overall_score", Value: fmt.Sprintf("%.2f", input.OverallScore)},
			{Label: "blocking issues", Value: fmt.Sprintf("%d", blocking)},
			{Label: "建议修复", Value: input.RetryRecommendation.SuggestedFix},
		},
		Next: "Producer 应读取 review_record 和 artifact_issue，决定是否接受、派 Craftsman fork RenderPlan 或请求用户确认。",
	}.String(), nil
}

func applySubmitReviewResultRuntimeTarget(input SubmitReviewResultInput, runtime NativeRuntimeContext) SubmitReviewResultInput {
	if strings.TrimSpace(runtime.ReviewTask) != "" {
		input.ReviewTask = strings.TrimSpace(runtime.ReviewTask)
	}
	if strings.TrimSpace(runtime.ReviewShotID) != "" {
		input.Target.ShotID = strings.TrimSpace(runtime.ReviewShotID)
	}
	if strings.TrimSpace(runtime.ReviewShotKey) != "" {
		input.Target.ShotRef = ensureToolObjectRef(input.Target.ShotRef, "shot", runtime.ReviewShotKey)
	}
	if strings.TrimSpace(runtime.ReviewNodeID) != "" {
		input.Target.NodeID = strings.TrimSpace(runtime.ReviewNodeID)
	}
	if strings.TrimSpace(runtime.ReviewNodeKey) != "" {
		input.Target.NodeRef = ensureToolObjectRef(input.Target.NodeRef, "media_node", runtime.ReviewNodeKey)
	}
	if strings.TrimSpace(runtime.ReviewVersionID) != "" {
		input.Target.ArtifactVersionID = strings.TrimSpace(runtime.ReviewVersionID)
	}
	if strings.TrimSpace(runtime.ReviewVersionKey) != "" {
		input.Target.ArtifactVersionRef = ensureToolObjectRef(input.Target.ArtifactVersionRef, "artifact_version", runtime.ReviewVersionKey)
	}
	if strings.TrimSpace(runtime.ReviewJobID) != "" {
		input.Target.GenerationJobID = strings.TrimSpace(runtime.ReviewJobID)
	}
	if strings.TrimSpace(runtime.ReviewRenderPlanID) != "" {
		input.Target.RenderPlanID = strings.TrimSpace(runtime.ReviewRenderPlanID)
	}
	if strings.TrimSpace(runtime.ReviewRenderPlanKey) != "" {
		input.Target.RenderPlanRef = ensureToolObjectRef(input.Target.RenderPlanRef, "render_plan", runtime.ReviewRenderPlanKey)
	}
	if strings.TrimSpace(runtime.ReviewParentReviewRecordID) != "" {
		input.Target.ParentReviewRecordID = strings.TrimSpace(runtime.ReviewParentReviewRecordID)
	}
	if strings.TrimSpace(runtime.ReviewParentReviewKey) != "" {
		input.Target.ParentReviewRef = ensureToolObjectRef(input.Target.ParentReviewRef, "review_record", runtime.ReviewParentReviewKey)
	}
	if isZeroUUIDText(input.Target.ParentReviewRecordID) {
		input.Target.ParentReviewRecordID = ""
	}
	if strings.TrimSpace(input.Target.WorkspaceScope) == "" {
		input.Target.WorkspaceScope = workspaceScopeForReviewTask(input.ReviewTask)
	}
	for index := range input.Issues {
		switch input.Issues[index].TargetObjectType {
		case "artifact_version":
			input.Issues[index].TargetObjectID = input.Target.ArtifactVersionID
			input.Issues[index].TargetObjectRef = ensureToolObjectRef(input.Issues[index].TargetObjectRef, "artifact_version", input.Target.ArtifactVersionRef.Key)
		case "render_plan":
			if strings.TrimSpace(input.Target.RenderPlanID) != "" {
				input.Issues[index].TargetObjectID = input.Target.RenderPlanID
			}
			input.Issues[index].TargetObjectRef = ensureToolObjectRef(input.Issues[index].TargetObjectRef, "render_plan", input.Target.RenderPlanRef.Key)
		case "shot":
			if strings.TrimSpace(input.Target.ShotID) != "" {
				input.Issues[index].TargetObjectID = input.Target.ShotID
			}
			input.Issues[index].TargetObjectRef = ensureToolObjectRef(input.Issues[index].TargetObjectRef, "shot", input.Target.ShotRef.Key)
		}
	}
	switch input.RetryRecommendation.TargetObjectType {
	case "artifact_version":
		input.RetryRecommendation.TargetObjectID = input.Target.ArtifactVersionID
	case "render_plan":
		if strings.TrimSpace(input.Target.RenderPlanID) != "" {
			input.RetryRecommendation.TargetObjectID = input.Target.RenderPlanID
		}
	case "shot":
		if strings.TrimSpace(input.Target.ShotID) != "" {
			input.RetryRecommendation.TargetObjectID = input.Target.ShotID
		}
	}
	return input
}

func workspaceScopeForReviewTask(reviewTask string) string {
	switch reviewTask {
	case reviewTaskPreRenderPlan:
		return "render_plan"
	case reviewTaskFinalVideo:
		return "final_video"
	default:
		return "shot"
	}
}

func isZeroUUIDText(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "00000000-0000-0000-0000-000000000000")
}

func (t *SubmitReviewResultNativeTool) createAndCompleteReview(ctx context.Context, runtime NativeRuntimeContext, input SubmitReviewResultInput) (db.ReviewRecord, error) {
	targetObjectType, targetObjectID, err := reviewTargetObject(input)
	if err != nil {
		return db.ReviewRecord{}, err
	}
	shotID, _ := pgUUIDFromString(input.Target.ShotID)
	nodeID, _ := pgUUIDFromString(input.Target.NodeID)
	versionID, _ := pgUUIDFromString(input.Target.ArtifactVersionID)
	jobID, _ := pgUUIDFromString(input.Target.GenerationJobID)
	parentID, _ := pgUUIDFromString(input.Target.ParentReviewRecordID)
	renderPlanID, _ := pgUUIDFromString(input.Target.RenderPlanID)
	requiredAxes := reviewTaskAxes[input.ReviewTask]
	requiredAxesJSON, _ := json.Marshal(requiredAxes)
	attemptNo := runtime.ReviewAttemptNo
	if attemptNo <= 0 {
		attemptNo = 1
	}
	maxAttempts := runtime.ReviewMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	reviewKey := reviewRecordSemanticKey(input, attemptNo)
	record, err := t.store.CreateReviewRecord(ctx, db.CreateReviewRecordParams{
		WorkspaceID:          runtime.WorkspaceID,
		ShotID:               shotID,
		NodeID:               nodeID,
		ArtifactVersionID:    versionID,
		GenerationJobID:      jobID,
		ReviewerThreadID:     runtime.ThreadID,
		ReviewerTaskID:       runtime.TaskID,
		ParentReviewRecordID: parentID,
		TargetPhase:          targetPhaseForReviewTask(input.ReviewTask),
		ReviewTask:           input.ReviewTask,
		TargetObjectType:     targetObjectType,
		TargetObjectID:       targetObjectID,
		RenderPlanID:         renderPlanID,
		AttemptNo:            attemptNo,
		MaxAttempts:          maxAttempts,
		RequiredAxes:         requiredAxesJSON,
		SemanticKey:          reviewKey,
		DisplayName:          reviewRecordDisplayName(input, reviewKey),
	})
	if err != nil {
		return db.ReviewRecord{}, err
	}
	rubricJSON, _ := json.Marshal(input.Rubric)
	retryJSON, _ := json.Marshal(input.RetryRecommendation)
	escalationJSON, _ := json.Marshal(map[string]any{"reason": input.RetryRecommendation.EscalationReason})
	record, err = t.store.CompleteReviewRecord(ctx, db.CompleteReviewRecordParams{
		ID:                  record.ID,
		Status:              input.Verdict,
		OverallScore:        pgtype.Float4{Float32: float32(input.OverallScore), Valid: true},
		Rubric:              rubricJSON,
		Critique:            strings.TrimSpace(input.Critique),
		RetryRecommendation: retryJSON,
		Escalation:          escalationJSON,
	})
	if err != nil {
		return db.ReviewRecord{}, err
	}
	for index, issue := range input.Issues {
		issueTargetID, _ := pgUUIDFromString(issue.TargetObjectID)
		if _, err := t.store.CreateArtifactIssue(ctx, db.CreateArtifactIssueParams{
			WorkspaceID:              runtime.WorkspaceID,
			ReviewRecordID:           record.ID,
			Dimension:                issue.Dimension,
			Severity:                 issue.Severity,
			TargetObjectType:         issue.TargetObjectType,
			TargetObjectID:           issueTargetID,
			Title:                    strings.TrimSpace(issue.Title),
			Description:              strings.TrimSpace(issue.Description),
			Evidence:                 strings.TrimSpace(issue.Evidence),
			SuggestedFix:             issue.SuggestedFix,
			FixHint:                  strings.TrimSpace(issue.FixHint),
			RequiresUserConfirmation: issue.RequiresUserConfirmation,
			SemanticKey:              artifactIssueSemanticKey(reviewKey, index),
			DisplayName:              artifactIssueDisplayName(issue, index),
		}); err != nil {
			return db.ReviewRecord{}, err
		}
	}
	return record, nil
}

func validateSubmitReviewResultInput(input SubmitReviewResultInput) error {
	if err := requireText(input.Brief, "brief"); err != nil {
		return err
	}
	if err := requireMode(input.ReviewTask, reviewTaskPreRenderPlan, reviewTaskPreviewImage, reviewTaskShotVideo, reviewTaskFinalVideo); err != nil {
		return err
	}
	if err := validateReviewTarget(input.ReviewTask, input.Target); err != nil {
		return err
	}
	if err := requireMode(input.Verdict, reviewVerdictAccepted, reviewVerdictAcceptedWithWarnings, reviewVerdictRejected, reviewVerdictBlocked); err != nil {
		return err
	}
	if input.OverallScore < 0 || input.OverallScore > 1 {
		return fmt.Errorf("overall_score 必须在 0 到 1 之间")
	}
	if len(input.Rubric) == 0 {
		return fmt.Errorf("rubric 必填")
	}
	if err := validateReviewRubricAxes(input.ReviewTask, input.Rubric); err != nil {
		return err
	}
	if err := requireText(input.Critique, "critique"); err != nil {
		return err
	}
	if err := requireText(input.Reason, "reason"); err != nil {
		return err
	}
	hasBlocking := false
	for index, issue := range input.Issues {
		if err := validateReviewIssue(index, issue); err != nil {
			return err
		}
		if issue.Severity == "blocking" {
			hasBlocking = true
		}
	}
	if input.Verdict == reviewVerdictRejected && !hasBlocking {
		return fmt.Errorf("verdict=rejected 时至少需要一个 severity=blocking 的 issue")
	}
	if input.Verdict == reviewVerdictAccepted && hasBlocking {
		return fmt.Errorf("verdict=accepted 时不能包含 blocking issue")
	}
	return nil
}

func validateReviewTarget(reviewTask string, target ReviewTargetInput) error {
	switch reviewTask {
	case reviewTaskPreRenderPlan:
		if !hasReviewTargetRefOrValue(target.RenderPlanRef, target.RenderPlanID) {
			return fmt.Errorf("pre_render_plan_review 需要 target.render_plan_ref，请使用 read_project_context 返回的 render_plan semantic_key")
		}
	case reviewTaskPreviewImage, reviewTaskShotVideo:
		if !hasReviewTargetRefOrValue(target.ShotRef, target.ShotID) {
			return fmt.Errorf("%s 需要 target.shot_ref，请使用 read_project_context 返回的 shot semantic_key", reviewTask)
		}
		if !hasReviewTargetRefOrValue(target.NodeRef, target.NodeID) {
			return fmt.Errorf("%s 需要 target.node_ref，请使用 read_project_context 返回的 media_node semantic_key", reviewTask)
		}
		if !hasReviewTargetRefOrValue(target.ArtifactVersionRef, target.ArtifactVersionID) {
			return fmt.Errorf("%s 需要 target.artifact_version_ref，请使用 read_project_context 返回的 artifact_version semantic_key", reviewTask)
		}
	case reviewTaskFinalVideo:
		if !hasReviewTargetRefOrValue(target.NodeRef, target.NodeID) {
			return fmt.Errorf("final_video_review 需要 target.node_ref，请使用 read_project_context 返回的 media_node semantic_key")
		}
		if !hasReviewTargetRefOrValue(target.ArtifactVersionRef, target.ArtifactVersionID) {
			return fmt.Errorf("final_video_review 需要 target.artifact_version_ref，请使用 read_project_context 返回的 artifact_version semantic_key")
		}
	}
	return nil
}

func hasReviewTargetRefOrValue(ref ToolObjectRef, legacyValue string) bool {
	return strings.TrimSpace(ref.Key) != "" || strings.TrimSpace(legacyValue) != ""
}

func validateReviewRubricAxes(reviewTask string, rubric []ReviewRubricAxisInput) error {
	seen := map[string]bool{}
	for _, axis := range rubric {
		if !reviewAxes[axis.Axis] {
			return fmt.Errorf("rubric.axis %q 不是支持的 10 轴", axis.Axis)
		}
		if axis.Score < 0 || axis.Score > 1 {
			return fmt.Errorf("rubric.%s.score 必须在 0 到 1 之间", axis.Axis)
		}
		if axis.Severity != "" {
			if err := requireMode(axis.Severity, "info", "warning", "blocking"); err != nil {
				return fmt.Errorf("rubric.%s.severity 无效：%w", axis.Axis, err)
			}
		}
		if err := requireText(axis.Reason, "rubric."+axis.Axis+".reason"); err != nil {
			return err
		}
		seen[axis.Axis] = true
	}
	for _, axis := range reviewTaskAxes[reviewTask] {
		if !seen[axis] {
			return fmt.Errorf("%s 缺少 required axis %s", reviewTask, axis)
		}
	}
	return nil
}

func validateReviewIssue(index int, issue ReviewIssueInput) error {
	prefix := fmt.Sprintf("issues[%d]", index)
	if !reviewIssueDimensions[issue.Dimension] {
		return fmt.Errorf("%s.dimension %q 不支持", prefix, issue.Dimension)
	}
	if err := requireMode(issue.Severity, "info", "warning", "blocking"); err != nil {
		return fmt.Errorf("%s.severity 无效：%w", prefix, err)
	}
	if err := requireText(issue.Title, prefix+".title"); err != nil {
		return err
	}
	if err := requireText(issue.Description, prefix+".description"); err != nil {
		return err
	}
	if err := requireMode(issue.TargetObjectType, "render_plan", "artifact_version", "shot", "final_video", "project_memory"); err != nil {
		return fmt.Errorf("%s.target_object_type 无效：%w", prefix, err)
	}
	if strings.TrimSpace(issue.TargetObjectRef.Key) != "" {
		if err := validateObjectRef(issue.TargetObjectRef, prefix+".target_object_ref"); err != nil {
			return err
		}
	}
	if strings.TrimSpace(issue.TargetObjectID) != "" {
		if _, ok := pgUUIDFromString(issue.TargetObjectID); !ok {
			return fmt.Errorf("%s.target_object_id 是兼容旧字段，请不要手写；改用 target_object_ref 或让工具按 target_object_type 自动派生", prefix)
		}
	}
	if err := requireMode(issue.SuggestedFix, "none", "regenerate", "edit", "extend", "bridge", "revise_render_plan", "revise_shot_plan", "manual"); err != nil {
		return fmt.Errorf("%s.suggested_fix 无效：%w", prefix, err)
	}
	if err := requireText(issue.FixHint, prefix+".fix_hint"); err != nil {
		return err
	}
	return nil
}

func reviewTargetObject(input SubmitReviewResultInput) (string, pgtype.UUID, error) {
	switch input.ReviewTask {
	case reviewTaskPreRenderPlan:
		id, ok := pgUUIDFromString(input.Target.RenderPlanID)
		if !ok {
			return "", pgtype.UUID{}, fmt.Errorf("target.render_plan_ref 未解析到内部对象；请使用当前 Reviewer 任务 target，或填写 read_project_context 返回的 render_plan semantic_key")
		}
		return "render_plan", id, nil
	case reviewTaskPreviewImage, reviewTaskShotVideo, reviewTaskFinalVideo:
		id, ok := pgUUIDFromString(input.Target.ArtifactVersionID)
		if !ok {
			return "", pgtype.UUID{}, fmt.Errorf("target.artifact_version_ref 未解析到内部对象；请使用当前 Reviewer 任务 target，或填写 read_project_context 返回的 artifact_version semantic_key")
		}
		return "artifact_version", id, nil
	default:
		return "", pgtype.UUID{}, fmt.Errorf("不支持的 review_task %s", input.ReviewTask)
	}
}

func targetPhaseForReviewTask(reviewTask string) string {
	switch reviewTask {
	case reviewTaskPreRenderPlan:
		return "pre_render_plan"
	case reviewTaskShotVideo:
		return "shot_video"
	case reviewTaskFinalVideo:
		return "final_video"
	default:
		return "preview_image"
	}
}

func reviewRecordRef(record db.ReviewRecord) string {
	if key := strings.TrimSpace(record.SemanticKey); key != "" {
		return "review_record/" + key
	}
	if name := strings.TrimSpace(record.DisplayName); name != "" {
		return name
	}
	return "review_record.semantic_key_missing"
}

func reviewRecordSemanticKey(input SubmitReviewResultInput, attemptNo int32) string {
	base := reviewSemanticBaseKey(input)
	if base == "" {
		return ""
	}
	if attemptNo <= 0 {
		attemptNo = 1
	}
	return fmt.Sprintf("%s.review.v%d", base, attemptNo)
}

func reviewSemanticBaseKey(input SubmitReviewResultInput) string {
	switch input.ReviewTask {
	case reviewTaskPreRenderPlan:
		if key := strings.TrimSpace(input.Target.RenderPlanRef.Key); key != "" {
			return key
		}
	case reviewTaskPreviewImage, reviewTaskShotVideo, reviewTaskFinalVideo:
		if key := strings.TrimSpace(input.Target.ArtifactVersionRef.Key); key != "" {
			return strings.TrimSuffix(key, ".artifact.v1")
		}
	}
	for _, key := range []string{
		input.Target.RenderPlanRef.Key,
		input.Target.NodeRef.Key,
		input.Target.ShotRef.Key,
	} {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func reviewRecordDisplayName(input SubmitReviewResultInput, key string) string {
	if key != "" {
		return "Review " + key
	}
	return strings.TrimSpace(input.Brief)
}

func artifactIssueSemanticKey(reviewKey string, index int) string {
	if strings.TrimSpace(reviewKey) == "" {
		return ""
	}
	return fmt.Sprintf("%s.issue.%02d", reviewKey, index+1)
}

func artifactIssueDisplayName(issue ReviewIssueInput, index int) string {
	if title := strings.TrimSpace(issue.Title); title != "" {
		return title
	}
	return fmt.Sprintf("Issue %02d", index+1)
}
