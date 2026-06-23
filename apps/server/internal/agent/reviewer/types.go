package reviewer

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrInvalidConfig = errors.New("invalid reviewer config")
	ErrInvalidInput  = errors.New("invalid reviewer input")
	ErrInvalidRubric = errors.New("invalid reviewer rubric")
)

const (
	TargetPhasePreviewImage = "preview_image"
	TargetPhaseShotVideo    = "shot_video"

	ReviewStatusRunning  = "running"
	ReviewStatusAccepted = "accepted"
	ReviewStatusRejected = "rejected"
	ReviewStatusFailed   = "failed"
)

type RunTaskInput struct {
	Task db.AgentTask
}

type TaskInput struct {
	TargetPhase          string `json:"target_phase"`
	ShotID               string `json:"shot_id"`
	NodeID               string `json:"node_id"`
	ArtifactVersionID    string `json:"artifact_version_id"`
	GenerationJobID      string `json:"generation_job_id,omitempty"`
	ParentReviewRecordID string `json:"parent_review_record_id,omitempty"`
	AttemptNo            int32  `json:"attempt_no"`
	MaxAttempts          int32  `json:"max_attempts"`
	AutoRetry            bool   `json:"auto_retry"`
}

type GraphInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	Task        TaskInput
}

type GraphOutput struct {
	Record   db.ReviewRecord
	Decision ReviewDecision
	Result   ReviewResult
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
	Input          GraphInput
	Shot           db.Shot
	Node           db.MediaNode
	Version        db.ArtifactVersion
	GenerationJob  db.GenerationJob
	PriorReviews   []db.ReviewRecord
	ProductionText string
	AssetURL       string
	AssetMime      string
	Text           string
}

type RubricAxis struct {
	Score   float64 `json:"score"`
	Pass    bool    `json:"pass"`
	Reason  string  `json:"reason"`
	FixHint string  `json:"fix_hint"`
}

type RetryRecommendation struct {
	ShouldRetry bool     `json:"should_retry"`
	FixHints    []string `json:"fix_hints,omitempty"`
}

type ReviewResult struct {
	OverallScore        float64               `json:"overall_score"`
	Rubric              map[string]RubricAxis `json:"rubric"`
	Critique            string                `json:"critique"`
	RetryRecommendation RetryRecommendation   `json:"retry_recommendation"`
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

type ModelResponder interface {
	Review(ctx context.Context, reviewContext Context) (ReviewResult, map[string]any, error)
}

type RetryDispatcher interface {
	DispatchRetry(ctx context.Context, input RetryDispatchInput) error
}

type DependencyNotifier interface {
	NotifyShotUpdated(ctx context.Context, workspaceID, upstreamShotID pgtype.UUID, phase string) error
}
