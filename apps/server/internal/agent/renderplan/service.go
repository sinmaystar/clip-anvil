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
	if err := requireValue(input.Scope.Type, "scope.type", ScopeKeyElementState, ScopeShot); err != nil {
		return err
	}
	if err := requireValue(input.TargetPhase, "target_phase", PhaseReferenceImage, PhasePreviewImage, PhaseShotVideo); err != nil {
		return err
	}
	if err := requireValue(input.TaskType, "task_type", TaskGenerate, TaskEdit, TaskExtend, TaskBridge); err != nil {
		return err
	}
	if err := requireValue(input.ModelPromptProfile, "model_prompt_profile", ProfileSeedream5Image, ProfileSeedance2Video); err != nil {
		return err
	}
	if strings.TrimSpace(input.Operation) == "" {
		return fmt.Errorf("%w: operation 必填", ErrInvalidInput)
	}
	if input.Scope.Type == ScopeKeyElementState && input.TargetPhase != PhaseReferenceImage {
		return fmt.Errorf("%w: key_element_state 只能用于 reference_image", ErrInvalidInput)
	}
	if input.TargetPhase == PhaseShotVideo && input.ModelPromptProfile != ProfileSeedance2Video {
		return fmt.Errorf("%w: shot_video 必须使用 seedance_2_video", ErrInvalidInput)
	}
	if input.TargetPhase != PhaseShotVideo && input.ModelPromptProfile == ProfileSeedance2Video {
		return fmt.Errorf("%w: 图片阶段不能使用 seedance_2_video", ErrInvalidInput)
	}
	if input.ModelPromptProfile == ProfileSeedance2Video {
		duration := int(input.Params.DurationSec)
		if duration > 0 && duration != 5 && duration != 10 {
			return fmt.Errorf("%w: duration_sec 只能是 5 或 10，当前是 %d", ErrInvalidInput, duration)
		}
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
		RenderPlanKey:          renderPlanKey(input, revision),
		ReferenceBindings:      mustJSON(input.ReferenceBindings),
		SubjectBindings:        mustJSON(input.SubjectBindings),
		PromptParts:            mustJSON(input.PromptParts),
		Params:                 mustJSON(input.Params),
		AuditHints:             mustJSON(input.AuditHints),
		Blocker:                mustJSON(input.Blocker),
		Rationale:              input.Rationale,
		CreatedByThreadID:      input.ThreadID,
		CreatedByTaskID:        input.TaskID,
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
	return fmt.Sprintf("%s:%s:%s:%d", input.Scope.Type, uuidString(input.Scope.ID), input.TargetPhase, revision)
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}
