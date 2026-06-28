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
	if plan.SemanticKey != "element_airport.state_morning.reference_image.r1" {
		t.Fatalf("semantic key = %q", plan.SemanticKey)
	}
	if plan.RenderPlanKey != plan.SemanticKey {
		t.Fatalf("render plan key = %q, want semantic key", plan.RenderPlanKey)
	}
	if strings.Contains(plan.RenderPlanKey, "0000-") {
		t.Fatalf("render plan key should not expose uuid: %q", plan.RenderPlanKey)
	}
}

func TestServiceWaitForProducerMarksCompiledPlanWaiting(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, NewPromptCompiler())
	input := validReferenceInput()
	input.ExecutionPolicy = ExecutionPolicyWaitForProducer
	plan, err := service.Upsert(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != StatusWaitingForApproval {
		t.Fatalf("status = %q, want waiting_for_approval", plan.Status)
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

func TestServiceRejectsUnsupportedSeedanceDurationBeforeCreatingPlan(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, NewPromptCompiler())
	input := validReferenceInput()
	input.Scope.Type = ScopeShot
	input.TargetPhase = PhaseShotVideo
	input.ModelPromptProfile = ProfileSeedance2Video
	input.Operation = "image_to_video_first_frame"
	input.PromptParts.Action = "行李箱在极简背景中旋转展示。"
	input.Params.DurationSec = 4

	_, err := service.Upsert(context.Background(), input)

	if err == nil || !strings.Contains(err.Error(), "duration_sec 只能是 5 或 10") {
		t.Fatalf("error = %v", err)
	}
	if len(store.plans) != 0 {
		t.Fatalf("store writes = %d, want 0", len(store.plans))
	}
}

func TestServiceAcceptsVoiceoverAudioRenderPlan(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, NewPromptCompiler())
	input := validAudioInput()
	input.TargetPhase = PhaseVoiceoverAudio

	plan, err := service.Upsert(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ScopeType != ScopeAudioPlan || plan.TargetPhase != PhaseVoiceoverAudio || plan.ModelPromptProfile != ProfileSeedAudio1 {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.Operation != "text_to_audio" {
		t.Fatalf("operation = %q", plan.Operation)
	}
}

func TestServiceAcceptsBGMAudioRenderPlan(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, NewPromptCompiler())
	input := validAudioInput()
	input.TargetPhase = PhaseBGMAudio
	input.PromptParts.Objective = "生成 12 秒轻快电子流行 BGM，旁白期间可 ducking。"

	plan, err := service.Upsert(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TargetPhase != PhaseBGMAudio || plan.ModelPromptProfile != ProfileSeedAudio1 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestServiceRejectsAudioPlanScopeForImagePhase(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, NewPromptCompiler())
	input := validAudioInput()
	input.TargetPhase = PhasePreviewImage
	input.ModelPromptProfile = ProfileSeedream5Image
	input.Operation = "text_to_image"

	_, err := service.Upsert(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "audio_plan 只能用于 voiceover_audio 或 bgm_audio") {
		t.Fatalf("error = %v", err)
	}
}

func TestServiceRejectsAudioPhaseWithVideoProfile(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, NewPromptCompiler())
	input := validAudioInput()
	input.ModelPromptProfile = ProfileSeedance2Video

	_, err := service.Upsert(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "音频阶段必须使用 seed_audio_1") {
		t.Fatalf("error = %v", err)
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
		Scope:              Scope{Type: ScopeKeyElementState, ID: uuidWithByte(4), Key: "element_airport.state_morning"},
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

func validAudioInput() UpsertInput {
	return UpsertInput{
		WorkspaceID:        uuidWithByte(1),
		ThreadID:           uuidWithByte(2),
		TaskID:             uuidWithByte(3),
		Brief:              "为已确认 AudioPlan 创建旁白音频计划。",
		Mode:               "create",
		Scope:              Scope{Type: ScopeAudioPlan, ID: uuidWithByte(9), Key: "audio_plan.active"},
		TargetPhase:        PhaseVoiceoverAudio,
		TaskType:           TaskGenerate,
		ModelPromptProfile: ProfileSeedAudio1,
		Operation:          "text_to_audio",
		PromptParts: PromptParts{
			Objective: "使用清爽可信的年轻女声生成完整营销短视频旁白。",
			Narration: "现在出发，让旅程更轻松。",
			Audio:     "旁白清晰，语速自然。",
		},
		Params:    Params{},
		Rationale: "AudioPlan 已确认，先生成整片连续旁白。",
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
		SemanticKey:            arg.SemanticKey,
		DisplayName:            arg.DisplayName,
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

func (f *fakeStore) MarkRenderPlanWaitingForApproval(_ context.Context, arg db.MarkRenderPlanWaitingForApprovalParams) (db.RenderPlan, error) {
	for i, plan := range f.plans {
		if plan.ID == arg.ID && plan.WorkspaceID == arg.WorkspaceID {
			plan.Status = StatusWaitingForApproval
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
