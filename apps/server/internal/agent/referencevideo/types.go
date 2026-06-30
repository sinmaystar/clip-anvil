package referencevideo

import (
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"

	ErrorCodeAnalyzerFailed = "reference_video_analyzer_failed"
)

var ErrInvalidSourceVideo = errors.New("reference video analysis requires an agent-owned video source node")

type AnalyzeInput struct {
	WorkspaceID      pgtype.UUID
	ThreadID         pgtype.UUID
	TaskID           pgtype.UUID
	SourceNodeID     pgtype.UUID
	Brief            string
	Focus            []string
	AdaptationTarget map[string]any
}

type AnalyzeOutput struct {
	ID       string
	Status   string
	Summary  string
	Warnings []string
}

type AnalyzerRequest struct {
	FixedProtocol    string
	Brief            string
	Focus            []string
	AdaptationTarget map[string]any
	Media            MediaEvidence
}

type MediaEvidence struct {
	SourceNodeID string
	Title        string
	Mime         string
	StorageURL   string
}

type AnalyzerResponse struct {
	ModelProvider  string
	ModelID        string
	RequestSummary map[string]any
	Result         AnalysisResult
}

type AnalysisResult struct {
	Summary         string           `json:"summary"`
	ReferenceIntent ReferenceIntent  `json:"reference_intent"`
	ScriptStructure map[string]any   `json:"script_structure,omitempty"`
	ShotBreakdown   []map[string]any `json:"shot_breakdown,omitempty"`
	CameraLanguage  map[string]any   `json:"camera_language,omitempty"`
	Pacing          map[string]any   `json:"pacing,omitempty"`
	Audio           map[string]any   `json:"audio,omitempty"`
	TextStyle       map[string]any   `json:"text_style,omitempty"`
	AdaptationPlan  map[string]any   `json:"adaptation_plan,omitempty"`
	Confidence      float64          `json:"confidence,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
}

type ReferenceIntent struct {
	Preserve       []string `json:"preserve,omitempty"`
	Ignore         []string `json:"ignore,omitempty"`
	MustBeOriginal []string `json:"must_be_original,omitempty"`
}

const FixedAnalysisProtocol = `Analyze the reference video for reusable production language, not copying.
Return strict JSON with summary, reference_intent.preserve, reference_intent.ignore,
reference_intent.must_be_original, script_structure, shot_breakdown, camera_language,
pacing, audio, text_style, adaptation_plan, confidence, and warnings.
Do not output frame-by-frame replication instructions. Do not copy brand, logo,
creator identity, people, original subtitle copy, or distinctive expression.`
