package overview

import "time"

type Phase string

const (
	PhasePlanning            Phase = "planning"
	PhasePreview             Phase = "preview"
	PhaseReview              Phase = "review"
	PhaseVideo               Phase = "video"
	PhaseFinal               Phase = "final"
	PhaseWaitingConfirmation Phase = "waiting_confirmation"
	PhaseComplete            Phase = "complete"
	PhaseNeedsAttention      Phase = "needs_attention"
	PhaseError               Phase = "error"
)

type Status string

const (
	StatusNone    Status = "none"
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	StatusReady   Status = "ready"
	StatusFailed  Status = "failed"
)

type ProductionOverview struct {
	WorkspaceID  string         `json:"workspace_id"`
	Phase        Phase          `json:"phase"`
	Counts       Counts         `json:"counts"`
	Shots        []ShotSummary  `json:"shots"`
	Timeline     []TimelineItem `json:"timeline"`
	FinalOutputs []FinalOutput  `json:"final_outputs"`
	Diagnostics  map[string]any `json:"diagnostics,omitempty"`
	UpdatedAt    string         `json:"updated_at"`
}

type Counts struct {
	ShotsTotal       int `json:"shots_total"`
	PreviewsReady    int `json:"previews_ready"`
	ReviewsAccepted  int `json:"reviews_accepted"`
	VideosReady      int `json:"videos_ready"`
	FinalOutputs     int `json:"final_outputs"`
	RunningTasks     int `json:"running_tasks"`
	FailedTasks      int `json:"failed_tasks"`
	WaitingDecisions int `json:"waiting_decisions"`
}

type ShotSummary struct {
	ID            string   `json:"id"`
	ClientKey     string   `json:"client_key"`
	SortOrder     int32    `json:"sort_order"`
	Title         string   `json:"title"`
	DurationSec   float64  `json:"duration_sec,omitempty"`
	Status        string   `json:"status"`
	PreviewStatus Status   `json:"preview_status"`
	ReviewStatus  Status   `json:"review_status"`
	VideoStatus   Status   `json:"video_status"`
	PreviewNodeID string   `json:"preview_node_id,omitempty"`
	VideoNodeID   string   `json:"video_node_id,omitempty"`
	ReviewScore   *float32 `json:"review_score,omitempty"`
}

type TimelineItem struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Label       string         `json:"label"`
	Status      string         `json:"status"`
	Role        string         `json:"role,omitempty"`
	Scope       map[string]any `json:"scope,omitempty"`
	Diagnostics map[string]any `json:"diagnostics,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
	CompletedAt string         `json:"completed_at,omitempty"`
}

type FinalOutput struct {
	NodeID      string `json:"node_id"`
	VersionID   string `json:"version_id,omitempty"`
	AssetID     string `json:"asset_id,omitempty"`
	Title       string `json:"title"`
	Status      Status `json:"status"`
	Operation   string `json:"operation"`
	CompletedAt string `json:"completed_at,omitempty"`
}

func timestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
