package renderplan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestServiceCreatesReferenceImageRenderPlanAndCompiles(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, NewPromptCompiler())
	plan, err := service.Upsert(context.Background(), validReferenceInput())
	if err != nil {
		t.Fatal(err)
	}
	if plan.TargetPhase != PhaseReferenceImage || plan.ModelPromptProfile != ProfileSeedream5Image {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.Status != StatusCompiled {
		t.Fatalf("status = %q, want compiled", plan.Status)
	}
	if strings.TrimSpace(plan.CompiledPrompt) == "" {
		t.Fatalf("compiled prompt is empty")
	}
}

func TestServiceRejectsShotVideoWithSeedreamProfile(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, NewPromptCompiler())
	input := validReferenceInput()
	input.Scope.Type = ScopeShot
	input.TargetPhase = PhaseShotVideo
	input.ModelPromptProfile = ProfileSeedream5Image
	input.Operation = "image_to_video_first_frame"
	input.PromptParts.Action = "旅客拉着行李箱穿过机场大厅。"
	_, err := service.Upsert(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "shot_video 必须使用 seedance_2_video") {
		t.Fatalf("error = %v", err)
	}
	if len(store.plans) != 0 {
		t.Fatalf("store writes = %d, want 0", len(store.plans))
	}
}

func TestServiceForksExecutedPlanInsteadOfUpdating(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, NewPromptCompiler())
	input := validReferenceInput()
	plan, err := service.Upsert(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	update := input
	update.Mode = "update_draft"
	update.RenderPlanID = plan.ID
	_, err = service.Upsert(context.Background(), update)
	if err == nil || !strings.Contains(err.Error(), "只能 fork_from") {
		t.Fatalf("error = %v", err)
	}
}

func TestServiceMarksBlockedWhenReferenceMissing(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, NewPromptCompiler())
	input := validReferenceInput()
	input.Mode = "mark_blocked"
	input.Blocker = Blocker{BlockerType: "missing_reference", Message: "机场场景参考图缺失。"}
	plan, err := service.Upsert(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != StatusBlocked {
		t.Fatalf("status = %q", plan.Status)
	}
	if !strings.Contains(string(plan.Blocker), "missing_reference") {
		t.Fatalf("blocker = %s", string(plan.Blocker))
	}
}

func validReferenceInput() UpsertInput {
	return UpsertInput{
		WorkspaceID:        uuidWithByte(1),
		ThreadID:           uuidWithByte(2),
		TaskID:             uuidWithByte(3),
		Brief:              "创建机场参考图计划。",
		Mode:               "create",
		Scope:              Scope{Type: ScopeKeyElementState, ID: uuidWithByte(4)},
		TargetPhase:        PhaseReferenceImage,
		TaskType:           TaskGenerate,
		ModelPromptProfile: ProfileSeedream5Image,
		Operation:          "text_to_image",
		PromptParts: PromptParts{
			Objective:      "生成现代机场出发大厅参考图。",
			Setting:        "清晨自然光，玻璃幕墙。",
			Style:          "真实商业广告质感。",
			ConstraintPack: []string{"不要出现竞品 Logo"},
		},
		Params:    Params{Ratio: "9:16", MaxImages: 1},
		Rationale: "先生成场景锚点，避免每个分镜各自发明机场。",
	}
}

type fakeStore struct {
	plans []db.RenderPlan
}

func newFakeStore() *fakeStore {
	return &fakeStore{plans: []db.RenderPlan{}}
}

func (f *fakeStore) CreateRenderPlan(_ context.Context, arg db.CreateRenderPlanParams) (db.RenderPlan, error) {
	plan := db.RenderPlan{
		ID:                     uuidWithByte(byte(len(f.plans) + 10)),
		WorkspaceID:            arg.WorkspaceID,
		ScopeType:              arg.ScopeType,
		ScopeID:                arg.ScopeID,
		TargetPhase:            arg.TargetPhase,
		TaskType:               arg.TaskType,
		ModelPromptProfile:     arg.ModelPromptProfile,
		Operation:              arg.Operation,
		Status:                 arg.Status,
		Revision:               arg.Revision,
		ForkedFromRenderPlanID: arg.ForkedFromRenderPlanID,
		RenderPlanKey:          arg.RenderPlanKey,
		ReferenceBindings:      arg.ReferenceBindings,
		SubjectBindings:        arg.SubjectBindings,
		PromptParts:            arg.PromptParts,
		Params:                 arg.Params,
		AuditHints:             arg.AuditHints,
		Blocker:                arg.Blocker,
		Rationale:              arg.Rationale,
		CreatedByThreadID:      arg.CreatedByThreadID,
		CreatedByTaskID:        arg.CreatedByTaskID,
	}
	f.plans = append(f.plans, plan)
	return plan, nil
}

func (f *fakeStore) GetRenderPlanByID(_ context.Context, arg db.GetRenderPlanByIDParams) (db.RenderPlan, error) {
	for _, plan := range f.plans {
		if plan.ID == arg.ID && plan.WorkspaceID == arg.WorkspaceID {
			return plan, nil
		}
	}
	return db.RenderPlan{}, errors.New("not found")
}

func (f *fakeStore) UpdateRenderPlanDraft(_ context.Context, arg db.UpdateRenderPlanDraftParams) (db.RenderPlan, error) {
	for i, plan := range f.plans {
		if plan.ID == arg.ID && plan.WorkspaceID == arg.WorkspaceID {
			plan.TaskType = arg.TaskType
			plan.ModelPromptProfile = arg.ModelPromptProfile
			plan.Operation = arg.Operation
			plan.ReferenceBindings = arg.ReferenceBindings
			plan.SubjectBindings = arg.SubjectBindings
			plan.PromptParts = arg.PromptParts
			plan.Params = arg.Params
			plan.AuditHints = arg.AuditHints
			plan.Blocker = arg.Blocker
			plan.Rationale = arg.Rationale
			plan.Status = arg.Status
			f.plans[i] = plan
			return plan, nil
		}
	}
	return db.RenderPlan{}, errors.New("not found")
}

func (f *fakeStore) MarkRenderPlanCompiled(_ context.Context, arg db.MarkRenderPlanCompiledParams) (db.RenderPlan, error) {
	for i, plan := range f.plans {
		if plan.ID == arg.ID && plan.WorkspaceID == arg.WorkspaceID {
			plan.Status = StatusCompiled
			plan.CompiledPrompt = arg.CompiledPrompt
			plan.CompiledRequest = arg.CompiledRequest
			plan.PromptAudit = arg.PromptAudit
			plan.CostEstimate = arg.CostEstimate
			f.plans[i] = plan
			return plan, nil
		}
	}
	return db.RenderPlan{}, errors.New("not found")
}

func (f *fakeStore) MarkRenderPlanBlocked(_ context.Context, arg db.MarkRenderPlanBlockedParams) (db.RenderPlan, error) {
	for i, plan := range f.plans {
		if plan.ID == arg.ID && plan.WorkspaceID == arg.WorkspaceID {
			plan.Status = StatusBlocked
			plan.Blocker = arg.Blocker
			plan.AuditHints = arg.AuditHints
			f.plans[i] = plan
			return plan, nil
		}
	}
	return db.RenderPlan{}, errors.New("not found")
}

func (f *fakeStore) NextRenderPlanRevision(context.Context, db.NextRenderPlanRevisionParams) (int32, error) {
	return int32(len(f.plans) + 1), nil
}

func uuidWithByte(value byte) pgtype.UUID {
	var bytes [16]byte
	bytes[15] = value
	return pgtype.UUID{Bytes: bytes, Valid: true}
}
