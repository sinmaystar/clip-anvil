package renderplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrInvalidInput = errors.New("invalid render plan input")
	ErrInvalidState = errors.New("invalid render plan state")
)

type Store interface {
	CreateRenderPlan(ctx context.Context, arg db.CreateRenderPlanParams) (db.RenderPlan, error)
	GetRenderPlanByID(ctx context.Context, arg db.GetRenderPlanByIDParams) (db.RenderPlan, error)
	UpdateRenderPlanDraft(ctx context.Context, arg db.UpdateRenderPlanDraftParams) (db.RenderPlan, error)
	MarkRenderPlanCompiled(ctx context.Context, arg db.MarkRenderPlanCompiledParams) (db.RenderPlan, error)
	MarkRenderPlanBlocked(ctx context.Context, arg db.MarkRenderPlanBlockedParams) (db.RenderPlan, error)
	MarkRenderPlanWaitingForApproval(ctx context.Context, arg db.MarkRenderPlanWaitingForApprovalParams) (db.RenderPlan, error)
	NextRenderPlanRevision(ctx context.Context, arg db.NextRenderPlanRevisionParams) (int32, error)
}

type Compiler interface {
	Compile(ctx context.Context, input UpsertInput) (CompileResult, error)
}

type Service struct {
	store    Store
	compiler Compiler
}

func NewService(store Store, compiler Compiler) *Service {
	return &Service{store: store, compiler: compiler}
}

func (s *Service) Upsert(ctx context.Context, input UpsertInput) (db.RenderPlan, error) {
	if s == nil || s.store == nil {
		return db.RenderPlan{}, fmt.Errorf("%w: render plan service is not configured", ErrInvalidInput)
	}
	if err := validateInput(input); err != nil {
		return db.RenderPlan{}, err
	}
	if input.Mode == "mark_blocked" {
		return s.markBlocked(ctx, input)
	}
	if input.Mode == "update_draft" {
		plan, err := s.store.GetRenderPlanByID(ctx, db.GetRenderPlanByIDParams{ID: input.RenderPlanID, WorkspaceID: input.WorkspaceID})
		if err != nil {
			return db.RenderPlan{}, err
		}
		if plan.Status != StatusDraft && plan.Status != StatusBlocked {
			return db.RenderPlan{}, fmt.Errorf("%w: 已执行 RenderPlan 不能 update_draft，只能 fork_from", ErrInvalidState)
		}
		updated, err := s.store.UpdateRenderPlanDraft(ctx, updateDraftParams(input, plan.ID, StatusDraft))
		if err != nil {
			return db.RenderPlan{}, err
		}
		return s.compileIfReady(ctx, input, updated)
	}
	revision, err := s.store.NextRenderPlanRevision(ctx, db.NextRenderPlanRevisionParams{
		WorkspaceID: input.WorkspaceID,
		ScopeType:   input.Scope.Type,
		ScopeID:     input.Scope.ID,
		TargetPhase: input.TargetPhase,
	})
	if err != nil {
		return db.RenderPlan{}, err
	}
	created, err := s.store.CreateRenderPlan(ctx, createParams(input, revision))
	if err != nil {
		return db.RenderPlan{}, err
	}
	return s.compileIfReady(ctx, input, created)
}

func validateInput(input UpsertInput) error {
	if strings.TrimSpace(input.Brief) == "" {
		return fmt.Errorf("%w: brief 必填", ErrInvalidInput)
	}
	if err := requireValue(input.Mode, "mode", "create", "update_draft", "fork_from", "mark_blocked"); err != nil {
		return err
	}
	if !input.WorkspaceID.Valid || !input.Scope.ID.Valid {
		return fmt.Errorf("%w: workspace_id 和 scope.id 必须有效", ErrInvalidInput)
	}
	if err := requireValue(input.Scope.Type, "scope.type", ScopeKeyElementState, ScopeShot, ScopeAudioPlan); err != nil {
		return err
	}
	if err := requireValue(input.TargetPhase, "target_phase", PhaseReferenceImage, PhasePreviewImage, PhaseShotVideo, PhaseVoiceoverAudio, PhaseBGMAudio); err != nil {
		return err
	}
	if err := requireValue(input.TaskType, "task_type", TaskGenerate, TaskEdit, TaskExtend, TaskBridge); err != nil {
		return err
	}
	if err := requireValue(input.ModelPromptProfile, "model_prompt_profile", ProfileSeedream5Image, ProfileSeedance2Video, ProfileSeedAudio1, ProfileMotionShotVideo); err != nil {
		return err
	}
	if strings.TrimSpace(input.Operation) == "" {
		return fmt.Errorf("%w: operation 必填", ErrInvalidInput)
	}
	if input.Scope.Type == ScopeKeyElementState && input.TargetPhase != PhaseReferenceImage {
		return fmt.Errorf("%w: key_element_state 只能用于 reference_image", ErrInvalidInput)
	}
	if input.Scope.Type == ScopeAudioPlan && input.TargetPhase != PhaseVoiceoverAudio && input.TargetPhase != PhaseBGMAudio {
		return fmt.Errorf("%w: audio_plan 只能用于 voiceover_audio 或 bgm_audio", ErrInvalidInput)
	}
	if (input.TargetPhase == PhaseVoiceoverAudio || input.TargetPhase == PhaseBGMAudio) && input.ModelPromptProfile != ProfileSeedAudio1 {
		return fmt.Errorf("%w: 音频阶段必须使用 seed_audio_1", ErrInvalidInput)
	}
	if input.ModelPromptProfile == ProfileSeedAudio1 && input.TargetPhase != PhaseVoiceoverAudio && input.TargetPhase != PhaseBGMAudio {
		return fmt.Errorf("%w: seed_audio_1 只能用于 voiceover_audio 或 bgm_audio", ErrInvalidInput)
	}
	if (input.TargetPhase == PhaseVoiceoverAudio || input.TargetPhase == PhaseBGMAudio) && input.Operation != "text_to_audio" {
		return fmt.Errorf("%w: 音频阶段 operation 必须是 text_to_audio", ErrInvalidInput)
	}
	if input.TargetPhase == PhaseShotVideo && input.ModelPromptProfile != ProfileSeedance2Video && input.ModelPromptProfile != ProfileMotionShotVideo {
		return fmt.Errorf("%w: shot_video 必须使用 seedance_2_video 或 motion_shot_video", ErrInvalidInput)
	}
	if input.TargetPhase != PhaseShotVideo && input.ModelPromptProfile == ProfileSeedance2Video {
		return fmt.Errorf("%w: 图片阶段不能使用 seedance_2_video", ErrInvalidInput)
	}
	if input.TargetPhase != PhaseShotVideo && input.ModelPromptProfile == ProfileMotionShotVideo {
		return fmt.Errorf("%w: motion_shot_video 只能用于 shot_video", ErrInvalidInput)
	}
	if input.ModelPromptProfile == ProfileSeedance2Video {
		duration := int(input.Params.DurationSec)
		if duration > 0 && duration != 5 && duration != 10 {
			return fmt.Errorf("%w: duration_sec 只能是 5 或 10，当前是 %d", ErrInvalidInput, duration)
		}
	}
	if err := validateReferenceBindingContent(input.ReferenceBindings, input.Operation); err != nil {
		return err
	}
	if input.Mode == "update_draft" && !input.RenderPlanID.Valid {
		return fmt.Errorf("%w: update_draft 需要 render_plan_id", ErrInvalidInput)
	}
	if input.Mode == "fork_from" && !input.ForkFromRenderPlanID.Valid {
		return fmt.Errorf("%w: fork_from 需要 fork_from_render_plan_id", ErrInvalidInput)
	}
	if input.Mode == "mark_blocked" {
		if strings.TrimSpace(input.Blocker.BlockerType) == "" || strings.TrimSpace(input.Blocker.Message) == "" {
			return fmt.Errorf("%w: mark_blocked 需要 blocker.blocker_type 和 blocker.message", ErrInvalidInput)
		}
		return nil
	}
	if strings.TrimSpace(input.PromptParts.Objective) == "" {
		return fmt.Errorf("%w: prompt_parts.objective 必填", ErrInvalidInput)
	}
	if strings.TrimSpace(input.Rationale) == "" {
		return fmt.Errorf("%w: rationale 必填", ErrInvalidInput)
	}
	return nil
}

func validateReferenceBindingContent(bindings []ReferenceBinding, operation string) error {
	for index, binding := range bindings {
		if strings.TrimSpace(binding.ContentType) == "" {
			return fmt.Errorf("%w: reference_bindings[%d].content_type 必填", ErrInvalidInput, index)
		}
		if strings.TrimSpace(binding.ModelRole) == "" {
			return fmt.Errorf("%w: reference_bindings[%d].model_role 必填", ErrInvalidInput, index)
		}
		if err := validateContentModelRole(binding.ContentType, binding.ModelRole); err != nil {
			return fmt.Errorf("%w: reference_bindings[%d]: %v", ErrInvalidInput, index, err)
		}
	}
	switch strings.TrimSpace(operation) {
	case "image_to_video_first_frame":
		if len(bindings) != 1 || bindings[0].ContentType != "image_url" || bindings[0].ModelRole != "first_frame" {
			return fmt.Errorf("%w: image_to_video_first_frame 需要且只需要 1 个 reference_binding: content_type=image_url, model_role=first_frame", ErrInvalidInput)
		}
	case "image_to_video_first_last_frame":
		firstFrames, lastFrames := 0, 0
		for _, binding := range bindings {
			if binding.ContentType != "image_url" {
				return fmt.Errorf("%w: image_to_video_first_last_frame 只能使用 image_url", ErrInvalidInput)
			}
			switch binding.ModelRole {
			case "first_frame":
				firstFrames++
			case "last_frame":
				lastFrames++
			default:
				return fmt.Errorf("%w: image_to_video_first_last_frame 只能使用 model_role=first_frame 或 last_frame", ErrInvalidInput)
			}
		}
		if len(bindings) != 2 || firstFrames != 1 || lastFrames != 1 {
			return fmt.Errorf("%w: image_to_video_first_last_frame 必须提供 2 个 image_url，分别是 first_frame 和 last_frame", ErrInvalidInput)
		}
	case "multi_modal_reference_video":
		if len(bindings) == 0 {
			return fmt.Errorf("%w: multi_modal_reference_video 至少需要 1 个 reference_binding", ErrInvalidInput)
		}
		imageRefs := 0
		for _, binding := range bindings {
			switch binding.ModelRole {
			case "reference_image":
				imageRefs++
			case "reference_video", "reference_audio":
			default:
				return fmt.Errorf("%w: multi_modal_reference_video 只能使用 reference_image、reference_video 或 reference_audio", ErrInvalidInput)
			}
		}
		if imageRefs > 9 {
			return fmt.Errorf("%w: multi_modal_reference_video 最多支持 9 张 reference_image", ErrInvalidInput)
		}
	}
	return nil
}

func validateContentModelRole(contentType string, modelRole string) error {
	switch strings.TrimSpace(contentType) {
	case "image_url":
		if modelRole == "first_frame" || modelRole == "last_frame" || modelRole == "reference_image" {
			return nil
		}
		return fmt.Errorf("image_url 只能使用 model_role=first_frame、last_frame 或 reference_image")
	case "video_url":
		if modelRole == "reference_video" {
			return nil
		}
		return fmt.Errorf("video_url 只能使用 model_role=reference_video")
	case "audio_url":
		if modelRole == "reference_audio" {
			return nil
		}
		return fmt.Errorf("audio_url 只能使用 model_role=reference_audio")
	default:
		return fmt.Errorf("content_type 只能是 image_url、video_url 或 audio_url")
	}
}

func requireValue(value string, field string, allowed ...string) error {
	value = strings.TrimSpace(value)
	for _, item := range allowed {
		if value == item {
			return nil
		}
	}
	return fmt.Errorf("%w: %s 只能是 %s", ErrInvalidInput, field, strings.Join(allowed, "、"))
}

func (s *Service) markBlocked(ctx context.Context, input UpsertInput) (db.RenderPlan, error) {
	blocker := mustJSON(input.Blocker)
	audit := mustJSON(input.AuditHints)
	if input.RenderPlanID.Valid {
		return s.store.MarkRenderPlanBlocked(ctx, db.MarkRenderPlanBlockedParams{
			ID:          input.RenderPlanID,
			WorkspaceID: input.WorkspaceID,
			Blocker:     blocker,
			AuditHints:  audit,
		})
	}
	revision, err := s.store.NextRenderPlanRevision(ctx, db.NextRenderPlanRevisionParams{
		WorkspaceID: input.WorkspaceID,
		ScopeType:   input.Scope.Type,
		ScopeID:     input.Scope.ID,
		TargetPhase: input.TargetPhase,
	})
	if err != nil {
		return db.RenderPlan{}, err
	}
	return s.store.CreateRenderPlan(ctx, createParamsWithStatus(input, revision, StatusBlocked))
}

func (s *Service) compileIfReady(ctx context.Context, input UpsertInput, plan db.RenderPlan) (db.RenderPlan, error) {
	if s.compiler == nil || plan.Status != StatusDraft {
		return plan, nil
	}
	compiled, err := s.compiler.Compile(ctx, input)
	if err != nil {
		return db.RenderPlan{}, err
	}
	compiledPlan, err := s.store.MarkRenderPlanCompiled(ctx, db.MarkRenderPlanCompiledParams{
		ID:              plan.ID,
		WorkspaceID:     input.WorkspaceID,
		CompiledPrompt:  compiled.CompiledPrompt,
		CompiledRequest: []byte(compiled.CompiledRequest),
		PromptAudit:     []byte(compiled.PromptAudit),
		CostEstimate:    []byte(compiled.CostEstimate),
	})
	if err != nil {
		return db.RenderPlan{}, err
	}
	if input.ExecutionPolicy == ExecutionPolicyWaitForProducer {
		return s.store.MarkRenderPlanWaitingForApproval(ctx, db.MarkRenderPlanWaitingForApprovalParams{
			ID:          compiledPlan.ID,
			WorkspaceID: input.WorkspaceID,
		})
	}
	return compiledPlan, nil
}

func createParams(input UpsertInput, revision int32) db.CreateRenderPlanParams {
	return createParamsWithStatus(input, revision, StatusDraft)
}

func createParamsWithStatus(input UpsertInput, revision int32, status string) db.CreateRenderPlanParams {
	semanticKey := renderPlanKey(input, revision)
	return db.CreateRenderPlanParams{
		WorkspaceID:            input.WorkspaceID,
		ScopeType:              input.Scope.Type,
		ScopeID:                input.Scope.ID,
		TargetPhase:            input.TargetPhase,
		TaskType:               input.TaskType,
		ModelPromptProfile:     input.ModelPromptProfile,
		Operation:              input.Operation,
		Status:                 status,
		Revision:               revision,
		ForkedFromRenderPlanID: input.ForkFromRenderPlanID,
		RenderPlanKey:          semanticKey,
		ReferenceBindings:      mustJSON(input.ReferenceBindings),
		SubjectBindings:        mustJSON(input.SubjectBindings),
		PromptParts:            mustJSON(input.PromptParts),
		Params:                 mustJSON(input.Params),
		AuditHints:             mustJSON(input.AuditHints),
		Blocker:                mustJSON(input.Blocker),
		Rationale:              input.Rationale,
		CreatedByThreadID:      input.ThreadID,
		CreatedByTaskID:        input.TaskID,
		SemanticKey:            semanticKey,
		DisplayName:            renderPlanDisplayName(input, revision),
	}
}

func updateDraftParams(input UpsertInput, id pgtype.UUID, status string) db.UpdateRenderPlanDraftParams {
	return db.UpdateRenderPlanDraftParams{
		ID:                 id,
		WorkspaceID:        input.WorkspaceID,
		TaskType:           input.TaskType,
		ModelPromptProfile: input.ModelPromptProfile,
		Operation:          input.Operation,
		ReferenceBindings:  mustJSON(input.ReferenceBindings),
		SubjectBindings:    mustJSON(input.SubjectBindings),
		PromptParts:        mustJSON(input.PromptParts),
		Params:             mustJSON(input.Params),
		AuditHints:         mustJSON(input.AuditHints),
		Blocker:            mustJSON(input.Blocker),
		Rationale:          input.Rationale,
		Status:             status,
	}
}

func renderPlanKey(input UpsertInput, revision int32) string {
	scopeKey := strings.TrimSpace(input.Scope.Key)
	if scopeKey == "" {
		scopeKey = strings.TrimSpace(input.Scope.Type) + ".semantic_key_missing"
	}
	return fmt.Sprintf("%s.%s.r%d", scopeKey, input.TargetPhase, revision)
}

func renderPlanDisplayName(input UpsertInput, revision int32) string {
	scopeKey := strings.TrimSpace(input.Scope.Key)
	if scopeKey == "" {
		scopeKey = strings.TrimSpace(input.Scope.Type)
	}
	return fmt.Sprintf("%s %s RenderPlan r%d", scopeKey, input.TargetPhase, revision)
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
