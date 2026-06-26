package reviewer

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/schema"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrInvalidConfig = errors.New("invalid reviewer config")
	ErrInvalidInput  = errors.New("invalid reviewer input")
	ErrInvalidRubric = errors.New("invalid reviewer rubric")
)

const (
	TargetPhasePreviewImage  = "preview_image"
	TargetPhaseShotVideo     = "shot_video"
	TargetPhaseFinalVideo    = "final_video"
	TargetPhasePreRenderPlan = "pre_render_plan"

	ReviewTaskPreRenderPlan = "pre_render_plan_review"
	ReviewTaskPreviewImage  = "preview_image_review"
	ReviewTaskShotVideo     = "shot_video_review"
	ReviewTaskFinalVideo    = "final_video_review"

	ReviewStatusRunning              = "running"
	ReviewStatusAccepted             = "accepted"
	ReviewStatusAcceptedWithWarnings = "accepted_with_warnings"
	ReviewStatusRejected             = "rejected"
	ReviewStatusBlocked              = "blocked"
	ReviewStatusFailed               = "failed"

	ReviewVerdictAccepted             = ReviewStatusAccepted
	ReviewVerdictAcceptedWithWarnings = ReviewStatusAcceptedWithWarnings
	ReviewVerdictRejected             = ReviewStatusRejected
	ReviewVerdictBlocked              = ReviewStatusBlocked

	AxisFaithfulness          = "faithfulness"
	AxisSubjectConsistency    = "subject_consistency"
	AxisProductVisibility     = "product_visibility"
	AxisBrandStyleConsistency = "brand_style_consistency"
	AxisCompositionProportion = "composition_proportion"
	AxisMotionPhysics         = "motion_physics"
	AxisVisualQuality         = "visual_quality"
	AxisContinuity            = "continuity"
	AxisAudioSync             = "audio_sync"
	AxisPlatformSellingPower  = "platform_selling_power"

	IssueSeverityInfo     = "info"
	IssueSeverityWarning  = "warning"
	IssueSeverityBlocking = "blocking"
)

type RunTaskInput struct {
	Task db.AgentTask
}

type TaskInput struct {
	Brief                string       `json:"brief"`
	ReviewTask           string       `json:"review_task"`
	Target               ReviewTarget `json:"target"`
	TargetPhase          string       `json:"target_phase"`
	ShotID               string       `json:"shot_id"`
	NodeID               string       `json:"node_id"`
	ArtifactVersionID    string       `json:"artifact_version_id"`
	GenerationJobID      string       `json:"generation_job_id,omitempty"`
	ParentReviewRecordID string       `json:"parent_review_record_id,omitempty"`
	AttemptNo            int32        `json:"attempt_no"`
	MaxAttempts          int32        `json:"max_attempts"`
	AutoRetry            bool         `json:"auto_retry"`
}

type ReviewTarget struct {
	WorkspaceScope       string `json:"workspace_scope"`
	ShotID               string `json:"shot_id,omitempty"`
	RenderPlanID         string `json:"render_plan_id,omitempty"`
	NodeID               string `json:"node_id,omitempty"`
	ArtifactVersionID    string `json:"artifact_version_id,omitempty"`
	GenerationJobID      string `json:"generation_job_id,omitempty"`
	ParentReviewRecordID string `json:"parent_review_record_id,omitempty"`
}

type GraphInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	Task        TaskInput
}

type GraphOutput struct {
	Record           db.ReviewRecord
	Decision         ReviewDecision
	Result           ReviewResult
	SameTurnMessages []ReviewerSameTurnMessage
}

type RetryDispatchInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	ShotRef     string
	TargetPhase string
	ReviewID    string
	Critique    string
	FixHints    []string
	AttemptNo   int32
	MaxAttempts int32
}

type Context struct {
	Input            GraphInput
	Shot             db.Shot
	Node             db.MediaNode
	Version          db.ArtifactVersion
	GenerationJob    db.GenerationJob
	PriorReviews     []db.ReviewRecord
	ProductionText   string
	AssetURL         string
	AssetMime        string
	Text             string
	ToolInfos        []*schema.ToolInfo
	SameTurnMessages []ReviewerSameTurnMessage
}

type ReviewerSameTurnMessage struct {
	Role          string
	MessageType   string
	Content       string
	ToolCallID    string
	ToolName      string
	ToolArguments map[string]any
}

type RubricAxis struct {
	Score    float64 `json:"score"`
	Pass     bool    `json:"pass"`
	Severity string  `json:"severity,omitempty"`
	Reason   string  `json:"reason"`
	FixHint  string  `json:"fix_hint"`
}

type RetryRecommendation struct {
	ShouldRetry              bool     `json:"should_retry,omitempty"`
	ShouldRepair             bool     `json:"should_repair,omitempty"`
	SuggestedFix             string   `json:"suggested_fix,omitempty"`
	TargetObjectType         string   `json:"target_object_type,omitempty"`
	TargetObjectID           string   `json:"target_object_id,omitempty"`
	FixHints                 []string `json:"fix_hints,omitempty"`
	RequiresUserConfirmation bool     `json:"requires_user_confirmation,omitempty"`
	EscalationReason         string   `json:"escalation_reason,omitempty"`
}

type ReviewIssue struct {
	Dimension                string `json:"dimension"`
	Severity                 string `json:"severity"`
	Title                    string `json:"title"`
	Description              string `json:"description"`
	Evidence                 string `json:"evidence,omitempty"`
	TargetObjectType         string `json:"target_object_type"`
	TargetObjectID           string `json:"target_object_id"`
	SuggestedFix             string `json:"suggested_fix"`
	FixHint                  string `json:"fix_hint"`
	RequiresUserConfirmation bool   `json:"requires_user_confirmation"`
}

type ReviewResult struct {
	ReviewTask          string                `json:"review_task,omitempty"`
	Target              ReviewTarget          `json:"target,omitempty"`
	Verdict             string                `json:"verdict,omitempty"`
	OverallScore        float64               `json:"overall_score"`
	Rubric              map[string]RubricAxis `json:"rubric"`
	Critique            string                `json:"critique"`
	Issues              []ReviewIssue         `json:"issues,omitempty"`
	RetryRecommendation RetryRecommendation   `json:"retry_recommendation"`
	RequiredAxes        []string              `json:"required_axes,omitempty"`
	EvidenceSummary     string                `json:"evidence_summary,omitempty"`
	Reason              string                `json:"reason,omitempty"`
	Metadata            map[string]any        `json:"metadata,omitempty"`
	Raw                 map[string]any        `json:"raw,omitempty"`
}

type ReviewDecision struct {
	Status      string
	ShouldRetry bool
	FixHints    []string
	Reasons     []string
}

type ReviewPolicy struct {
	OverallThreshold float64
	AxisThreshold    float64
	RequiredAxes     []string
	MaxAttempts      int32
}

type Loader interface {
	Load(ctx context.Context, input GraphInput) (Context, error)
}

type ToolResponder interface {
	Respond(ctx context.Context, reviewContext Context) (ReviewerTurnOutput, error)
}

type ReviewerTurnOutput struct {
	AssistantText string
	Metadata      map[string]any
	ModelMessage  *schema.Message
}

type RetryDispatcher interface {
	DispatchRetry(ctx context.Context, input RetryDispatchInput) error
}

type DependencyNotifier interface {
	NotifyShotUpdated(ctx context.Context, workspaceID, upstreamShotID pgtype.UUID, phase string) error
}
