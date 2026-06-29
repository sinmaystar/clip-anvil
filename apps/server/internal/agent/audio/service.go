package audio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrAgentWorkspaceRequired = errors.New("agent workspace required")
	ErrInvalidAudioPlanInput  = errors.New("invalid audio plan input")
	ErrAudioPlanNotFound      = errors.New("audio plan not found")
)

type Store interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	GetActiveAudioPlanByWorkspace(ctx context.Context, workspaceID pgtype.UUID) (db.AudioPlan, error)
	ArchiveActiveAudioPlansByWorkspace(ctx context.Context, workspaceID pgtype.UUID) error
	CreateAudioPlan(ctx context.Context, arg db.CreateAudioPlanParams) (db.AudioPlan, error)
	UpdateAudioPlan(ctx context.Context, arg db.UpdateAudioPlanParams) (db.AudioPlan, error)
	UpdateAudioPlanStatus(ctx context.Context, arg db.UpdateAudioPlanStatusParams) (db.AudioPlan, error)
}

type Service struct {
	store Store
}

type UpsertInput struct {
	WorkspaceID       pgtype.UUID
	TaskID            pgtype.UUID
	Mode              string
	Title             string
	Language          string
	TargetDurationSec *float64
	VoiceoverScript   string
	VoiceProfile      map[string]any
	BGMPlan           map[string]any
	CuePlan           []CueInput
	GenerationParams  map[string]any
}

type CueInput struct {
	ShotRef  string  `json:"shot_ref"`
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
	Text     string  `json:"text"`
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Upsert(ctx context.Context, input UpsertInput) (db.AudioPlan, error) {
	if err := s.requireAgentWorkspace(ctx, input.WorkspaceID); err != nil {
		return db.AudioPlan{}, err
	}
	mode := strings.TrimSpace(input.Mode)
	switch mode {
	case "replace_draft":
		if err := validateWritablePlan(input); err != nil {
			return db.AudioPlan{}, err
		}
		if err := s.store.ArchiveActiveAudioPlansByWorkspace(ctx, input.WorkspaceID); err != nil {
			return db.AudioPlan{}, err
		}
		return s.store.CreateAudioPlan(ctx, db.CreateAudioPlanParams{
			WorkspaceID:       input.WorkspaceID,
			Status:            "waiting_for_user",
			Title:             defaultString(input.Title, "营销短视频音频方案"),
			PlanKind:          "marketing_voiceover_bgm",
			Language:          defaultString(input.Language, "zh"),
			TargetDurationSec: float8(input.TargetDurationSec),
			VoiceoverScript:   strings.TrimSpace(input.VoiceoverScript),
			VoiceProfile:      jsonBytes(input.VoiceProfile, "{}"),
			BgmPlan:           jsonBytes(input.BGMPlan, "{}"),
			CuePlan:           jsonBytes(input.CuePlan, "[]"),
			GenerationParams:  jsonBytes(input.GenerationParams, "{}"),
			CreatedByTaskID:   input.TaskID,
			SemanticKey:       "audio_plan.active",
			DisplayName:       defaultString(input.Title, "AudioPlan"),
		})
	case "patch":
		if err := validateWritablePlan(input); err != nil {
			return db.AudioPlan{}, err
		}
		current, err := s.activePlan(ctx, input.WorkspaceID)
		if err != nil {
			return db.AudioPlan{}, err
		}
		status := "waiting_for_user"
		if current.Status == "approved" {
			status = "approved"
		}
		return s.store.UpdateAudioPlan(ctx, db.UpdateAudioPlanParams{
			ID:                current.ID,
			WorkspaceID:       input.WorkspaceID,
			Status:            status,
			Title:             defaultString(input.Title, current.Title),
			Language:          defaultString(input.Language, current.Language),
			TargetDurationSec: float8(input.TargetDurationSec),
			VoiceoverScript:   strings.TrimSpace(input.VoiceoverScript),
			VoiceProfile:      jsonBytes(input.VoiceProfile, "{}"),
			BgmPlan:           jsonBytes(input.BGMPlan, "{}"),
			CuePlan:           jsonBytes(input.CuePlan, "[]"),
			GenerationParams:  jsonBytes(input.GenerationParams, "{}"),
			DisplayName:       defaultString(input.Title, current.DisplayName),
		})
	case "approve", "block":
		current, err := s.activePlan(ctx, input.WorkspaceID)
		if err != nil {
			return db.AudioPlan{}, err
		}
		status := "approved"
		if mode == "block" {
			status = "blocked"
		}
		return s.store.UpdateAudioPlanStatus(ctx, db.UpdateAudioPlanStatusParams{
			ID:          current.ID,
			WorkspaceID: input.WorkspaceID,
			Status:      status,
		})
	default:
		return db.AudioPlan{}, fmt.Errorf("%w: unsupported mode %q", ErrInvalidAudioPlanInput, mode)
	}
}

func (s *Service) requireAgentWorkspace(ctx context.Context, workspaceID pgtype.UUID) error {
	if s == nil || s.store == nil || !workspaceID.Valid {
		return ErrInvalidAudioPlanInput
	}
	workspace, err := s.store.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return err
	}
	if workspace.Mode != db.WorkspaceModeAgent {
		return ErrAgentWorkspaceRequired
	}
	return nil
}

func (s *Service) activePlan(ctx context.Context, workspaceID pgtype.UUID) (db.AudioPlan, error) {
	current, err := s.store.GetActiveAudioPlanByWorkspace(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.AudioPlan{}, ErrAudioPlanNotFound
	}
	return current, err
}

func validateWritablePlan(input UpsertInput) error {
	if strings.TrimSpace(input.VoiceoverScript) == "" {
		return fmt.Errorf("%w: voiceover_script required", ErrInvalidAudioPlanInput)
	}
	if input.TargetDurationSec != nil && *input.TargetDurationSec <= 0 {
		return fmt.Errorf("%w: target_duration_sec must be positive", ErrInvalidAudioPlanInput)
	}
	for index, cue := range input.CuePlan {
		if strings.TrimSpace(cue.ShotRef) == "" || strings.TrimSpace(cue.Text) == "" || cue.EndSec <= cue.StartSec || cue.StartSec < 0 {
			return fmt.Errorf("%w: invalid cue_plan[%d]", ErrInvalidAudioPlanInput, index)
		}
	}
	if len(input.BGMPlan) > 0 {
		if strings.TrimSpace(stringAny(input.BGMPlan["source"])) != "generated" {
			return fmt.Errorf("%w: bgm_plan.source must be generated", ErrInvalidAudioPlanInput)
		}
		if model := strings.TrimSpace(stringAny(input.BGMPlan["model"])); model != "" && model != "seed-audio-1.0" {
			return fmt.Errorf("%w: bgm_plan.model must be seed-audio-1.0", ErrInvalidAudioPlanInput)
		}
	}
	return nil
}

func jsonBytes(value any, fallback string) []byte {
	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "null" {
		return []byte(fallback)
	}
	return raw
}

func float8(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *value, Valid: true}
}

func defaultString(value string, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func stringAny(value any) string {
	text, _ := value.(string)
	return text
}
