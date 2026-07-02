package renderplan

import (
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ScopeKeyElementState = "key_element_state"
	ScopeShot            = "shot"
	ScopeAudioPlan       = "audio_plan"

	PhaseReferenceImage = "reference_image"
	PhasePreviewImage   = "preview_image"
	PhaseShotVideo      = "shot_video"
	PhaseVoiceoverAudio = "voiceover_audio"
	PhaseBGMAudio       = "bgm_audio"

	TaskGenerate = "generate"
	TaskEdit     = "edit"
	TaskExtend   = "extend"
	TaskBridge   = "bridge"

	ProfileSeedream5Image = "seedream_5_image"
	ProfileSeedance2Video = "seedance_2_video"
	ProfileSeedAudio1     = "seed_audio_1"
	ProfileTemplateVideo  = "template_video"

	StatusDraft              = "draft"
	StatusBlocked            = "blocked"
	StatusCompiled           = "compiled"
	StatusWaitingForApproval = "waiting_for_approval"
	StatusSubmitted          = "submitted"
	StatusRejected           = "rejected"

	ExecutionPolicyExecuteImmediately = "execute_immediately"
	ExecutionPolicyWaitForProducer    = "wait_for_producer"
)

type Scope struct {
	Type string
	ID   pgtype.UUID
	Key  string
}

type UpsertInput struct {
	WorkspaceID          pgtype.UUID
	ThreadID             pgtype.UUID
	TaskID               pgtype.UUID
	Brief                string
	Mode                 string
	RenderPlanID         pgtype.UUID
	ForkFromRenderPlanID pgtype.UUID
	Scope                Scope
	TargetPhase          string
	TaskType             string
	ModelPromptProfile   string
	Operation            string
	ReferenceBindings    []ReferenceBinding
	SubjectBindings      []SubjectBinding
	PromptParts          PromptParts
	Params               Params
	AuditHints           AuditHints
	Blocker              Blocker
	Rationale            string
	AutoCompileAndSubmit bool
	ExecutionPolicy      string
}

type ReferenceBinding struct {
	ClientKey      string `json:"client_key"`
	SourceType     string `json:"source_type"`
	SourceID       string `json:"source_id"`
	ContentType    string `json:"content_type"`
	ModelRole      string `json:"model_role"`
	PromptAlias    string `json:"prompt_alias,omitempty"`
	SemanticTarget string `json:"semantic_target,omitempty"`
	Priority       int    `json:"priority,omitempty"`
	Required       bool   `json:"required,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

type SubjectBinding struct {
	SubjectKey     string   `json:"subject_key"`
	Label          string   `json:"label"`
	ElementStateID string   `json:"element_state_id,omitempty"`
	PromptHandle   string   `json:"prompt_handle,omitempty"`
	StableTraits   []string `json:"stable_traits,omitempty"`
	MustPreserve   bool     `json:"must_preserve,omitempty"`
	AmbiguityNotes string   `json:"ambiguity_notes,omitempty"`
}

type PromptParts struct {
	Objective      string   `json:"objective"`
	Subject        string   `json:"subject,omitempty"`
	Setting        string   `json:"setting,omitempty"`
	Action         string   `json:"action,omitempty"`
	Camera         string   `json:"camera,omitempty"`
	Composition    string   `json:"composition,omitempty"`
	Style          string   `json:"style,omitempty"`
	Lighting       string   `json:"lighting,omitempty"`
	Sequence       []string `json:"sequence,omitempty"`
	Dialogue       string   `json:"dialogue,omitempty"`
	Narration      string   `json:"narration,omitempty"`
	Audio          string   `json:"audio,omitempty"`
	TextRendering  string   `json:"text_rendering,omitempty"`
	QualityPack    []string `json:"quality_pack,omitempty"`
	ConstraintPack []string `json:"constraint_pack,omitempty"`
	NegativeHints  []string `json:"negative_hints,omitempty"`
}

type Params struct {
	Ratio                     string         `json:"ratio,omitempty"`
	DurationSec               float64        `json:"duration_sec,omitempty"`
	Resolution                string         `json:"resolution,omitempty"`
	Watermark                 bool           `json:"watermark,omitempty"`
	Speaker                   string         `json:"speaker,omitempty"`
	Format                    string         `json:"format,omitempty"`
	SampleRate                int            `json:"sample_rate,omitempty"`
	SpeechRate                float64        `json:"speech_rate,omitempty"`
	PitchRate                 float64        `json:"pitch_rate,omitempty"`
	LoudnessRate              float64        `json:"loudness_rate,omitempty"`
	GenerateAudio             bool           `json:"generate_audio,omitempty"`
	ReturnLastFrame           bool           `json:"return_last_frame,omitempty"`
	CameraFixed               bool           `json:"camera_fixed,omitempty"`
	TemplateKey               string         `json:"template_key,omitempty"`
	FPS                       int            `json:"fps,omitempty"`
	Variables                 map[string]any `json:"variables,omitempty"`
	SequentialImageGeneration string         `json:"sequential_image_generation,omitempty"`
	MaxImages                 int            `json:"max_images,omitempty"`
	Seed                      int64          `json:"seed,omitempty"`
}

type AuditHints struct {
	AutoFilled          []string `json:"auto_filled,omitempty"`
	NeedsUserDecision   []string `json:"needs_user_decision,omitempty"`
	CapabilityRisks     []string `json:"capability_risks,omitempty"`
	ConsistencyRisks    []string `json:"consistency_risks,omitempty"`
	PromptCompilerNotes []string `json:"prompt_compiler_notes,omitempty"`
}

type Blocker struct {
	BlockerType string   `json:"blocker_type,omitempty"`
	Message     string   `json:"message,omitempty"`
	NeededBy    string   `json:"needed_by,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

type CompileResult struct {
	CompiledPrompt  string
	CompiledRequest json.RawMessage
	PromptAudit     json.RawMessage
	CostEstimate    json.RawMessage
}
