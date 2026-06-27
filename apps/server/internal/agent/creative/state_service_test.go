package creative

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestServiceBuildsYuexingAirportCreativeState(t *testing.T) {
	workspaceID := testUUID(1)
	store := newFakeStore(workspaceID, db.WorkspaceModeAgent)
	service := NewService(store)

	brief, err := service.UpsertProjectBrief(context.Background(), UpsertProjectBriefInput{
		WorkspaceID:    workspaceID,
		Brief:          "为悦行行李箱抖音广告创建创意简报",
		Mode:           "create",
		Title:          "悦行行李箱机场广告",
		VideoType:      "marketing_ad",
		TargetAudience: "短途商务出行用户",
		Tone:           "轻快、可靠、有掌控感",
		VisualStyle:    "现代机场清晨自然光，商业质感",
		AspectRatio:    "9:16",
		Language:       "zh-CN",
		Objective:      "突出悦行行李箱适合短途商务出行",
		Concept:        "在机场拉箱的轻松出行广告",
	})
	if err != nil {
		t.Fatalf("upsert brief: %v", err)
	}
	if brief.Status != "active" || brief.Title != "悦行行李箱机场广告" {
		t.Fatalf("unexpected brief: %#v", brief)
	}

	memory, err := service.UpdateProjectMemory(context.Background(), UpdateProjectMemoryInput{
		WorkspaceID: workspaceID,
		Brief:       "记录商品一致性和机场商务氛围",
		Mode:        "create",
		CoreIntent:  "突出悦行行李箱让短途商务出行更轻松",
		Soul:        "轻松出门，行程有掌控感",
		BrandFacts: []MemoryFact{{
			Key: "product", Value: "悦行行李箱，外观必须和用户上传素材一致",
		}},
		NonNegotiables: []MemoryRule{{Rule: "行李箱外观必须保持一致", Severity: "blocking"}},
		Reason:         "用户只上传了行李箱素材，需要建立全局一致性约束",
	})
	if err != nil {
		t.Fatalf("update memory: %v", err)
	}
	if memory.Version != 1 || memory.Status != "active" {
		t.Fatalf("unexpected memory: %#v", memory)
	}

	elements, err := service.UpsertKeyElements(context.Background(), UpsertKeyElementsInput{
		WorkspaceID: workspaceID,
		Brief:       "把上传行李箱和机场场景沉淀为可复用锚点",
		Mode:        "create",
		Elements: []KeyElementInput{
			{
				ClientKey:   "product_yuexing_luggage",
				ElementType: "product",
				Name:        "悦行行李箱",
				Description: "广告主角商品",
				SourceType:  "user_asset",
				States: []KeyElementStateInput{{
					ClientKey:         "state_uploaded_front",
					Label:             "用户上传素材状态",
					VisualDescription: "以用户上传图片为准的悦行行李箱外观",
					ReferenceStatus:   "ready",
					IsDefault:         true,
				}},
			},
			{
				ClientKey:   "scene_airport_departure_hall",
				ElementType: "scene",
				Name:        "机场出发大厅",
				SourceType:  "prompt_derived",
				States: []KeyElementStateInput{{
					ClientKey:         "state_modern_morning",
					Label:             "现代机场清晨状态",
					VisualDescription: "现代机场出发大厅，清晨自然光，开阔明亮，商务但不冷淡",
					ReferenceStatus:   "needs_reference",
					IsDefault:         true,
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("upsert elements: %v", err)
	}
	if elements.ElementsCreated != 2 || elements.StatesCreated != 2 {
		t.Fatalf("unexpected element output: %#v", elements)
	}

	storyboard, err := service.UpsertStoryboard(context.Background(), UpsertStoryboardInput{
		WorkspaceID: workspaceID,
		Brief:       "创建机场广告第一版场景和两个分镜",
		Mode:        "create",
		Scope:       StoryboardScope{Type: "workspace"},
		Scenes: []SceneInput{{
			ClientKey:   "scene_airport_departure_hall",
			SortOrder:   1,
			Title:       "机场出发大厅",
			Description: "展示轻松出行的主要场景",
			Location:    "机场出发大厅",
			Mood:        "明亮、轻快、商务",
		}},
		Shots: []ShotInput{
			{
				ClientKey:        "shot_01",
				SceneClientKey:   "scene_airport_departure_hall",
				SortOrder:        1,
				Title:            "机场拉箱开场",
				ShotKind:         "lifestyle",
				CreativeText:     "短途商务旅客单手拉着悦行行李箱穿过机场出发大厅",
				NarrativePurpose: "建立轻松出行氛围",
				DurationSec:      3,
				VisualIntent:     "突出空间开阔和行李箱质感",
				ActionText:       "人物步伐轻快，行李箱顺滑跟随",
				CameraIntent:     "中景跟拍",
			},
			{
				ClientKey:        "shot_02",
				SceneClientKey:   "scene_airport_departure_hall",
				SortOrder:        2,
				Title:            "商品质感特写",
				ShotKind:         "product_closeup",
				CreativeText:     "镜头靠近悦行行李箱轮子和箱体细节",
				NarrativePurpose: "展示顺滑和高级质感",
				DurationSec:      3,
				VisualIntent:     "商品外观与上传素材一致",
				ActionText:       "轮子平稳滑过机场地面",
				CameraIntent:     "低角度产品特写",
			},
		},
		ShotKeyElements: []ShotKeyElementInput{
			{ShotClientKey: "shot_01", ElementClientKey: "product_yuexing_luggage", StateClientKey: "state_uploaded_front", Role: "hero_product", Required: true},
			{ShotClientKey: "shot_01", ElementClientKey: "scene_airport_departure_hall", StateClientKey: "state_modern_morning", Role: "location", Required: true},
			{ShotClientKey: "shot_02", ElementClientKey: "product_yuexing_luggage", StateClientKey: "state_uploaded_front", Role: "hero_product", Required: true},
		},
		Dependencies: []ShotDependencyInput{{
			FromShotClientKey: "shot_01",
			ToShotClientKey:   "shot_02",
			DependencyType:    "same_product_consistency",
			BlockingPhase:     "video_generation",
			Reason:            "两个分镜必须保持同一个悦行行李箱外观",
		}},
	})
	if err != nil {
		t.Fatalf("upsert storyboard: %v", err)
	}
	if storyboard.ScenesCreated != 1 || storyboard.ShotsCreated != 2 || storyboard.ShotKeyElements != 3 || storyboard.DependenciesCreated != 1 {
		t.Fatalf("unexpected storyboard output: %#v", storyboard)
	}

	packet, err := service.ReadProjectContext(context.Background(), ReadContextInput{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("read context: %v", err)
	}
	if packet.Brief == nil || packet.Memory == nil || len(packet.Elements) != 2 || len(packet.Scenes) != 1 || len(packet.Shots) != 2 {
		t.Fatalf("unexpected context packet: %#v", packet)
	}
	if packet.Brief.SemanticKey != "creative_brief.main" || packet.Brief.DisplayName == "" {
		t.Fatalf("brief semantic identity missing: %#v", packet.Brief)
	}
	if packet.Memory.SemanticKey != "project_memory.v1" || packet.Memory.DisplayName == "" {
		t.Fatalf("memory semantic identity missing: %#v", packet.Memory)
	}
	if packet.Elements[0].SemanticKey == "" || packet.ElementStates[0].SemanticKey == "" || packet.Scenes[0].SemanticKey == "" || packet.Shots[0].SemanticKey == "" {
		t.Fatalf("semantic identity missing in context packet: %#v", packet)
	}
	if packet.Dependencies[0].SemanticKey == "" {
		t.Fatalf("dependency semantic identity missing: %#v", packet.Dependencies[0])
	}
}

func TestServiceReadProjectContextIncludesRenderPlansForProducerDecisions(t *testing.T) {
	workspaceID := testUUID(1)
	store := newFakeStore(workspaceID, db.WorkspaceModeAgent)
	shotID := testUUID(2)
	planID := testUUID(3)
	store.renderPlans = []db.RenderPlan{{
		ID:                 planID,
		WorkspaceID:        workspaceID,
		ScopeType:          "shot",
		ScopeID:            shotID,
		TargetPhase:        "preview_image",
		TaskType:           "generate",
		ModelPromptProfile: "seedream_5_image",
		Operation:          "text_to_image",
		Status:             "waiting_for_approval",
		Revision:           1,
		CompiledPrompt:     "目标：生成行李箱预览图",
	}}
	service := NewService(store)

	packet, err := service.ReadProjectContext(context.Background(), ReadContextInput{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("read context: %v", err)
	}
	if len(packet.RenderPlans) != 1 {
		t.Fatalf("render plans len = %d, want 1", len(packet.RenderPlans))
	}
	if packet.RenderPlans[0].ID != planID || packet.RenderPlans[0].Status != "waiting_for_approval" {
		t.Fatalf("unexpected render plan: %#v", packet.RenderPlans[0])
	}
}

func TestServiceReadProjectContextIncludesObjectIndex(t *testing.T) {
	workspaceID := testUUID(1)
	store := newFakeStore(workspaceID, db.WorkspaceModeAgent)
	store.objectIndex = []db.AgentObjectIndex{{
		WorkspaceID: workspaceID,
		ObjectType:  "shot",
		ObjectID:    testUUID(2),
		SemanticKey: "shot_01",
		DisplayName: "产品开场",
		Status:      "preview_ready",
		Kind:        "lifestyle",
		SortOrder:   1,
	}}
	service := NewService(store)

	packet, err := service.ReadProjectContext(context.Background(), ReadContextInput{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("read context: %v", err)
	}
	if len(packet.ObjectIndex) != 1 || packet.ObjectIndex[0].SemanticKey != "shot_01" {
		t.Fatalf("object index = %#v", packet.ObjectIndex)
	}
}

func TestServiceReadProjectContextFiltersRenderPlansForShotScope(t *testing.T) {
	workspaceID := testUUID(1)
	store := newFakeStore(workspaceID, db.WorkspaceModeAgent)
	targetShotID := testUUID(2)
	otherShotID := testUUID(3)
	targetPlanID := testUUID(4)
	store.renderPlans = []db.RenderPlan{
		{
			ID:                 targetPlanID,
			WorkspaceID:        workspaceID,
			ScopeType:          "shot",
			ScopeID:            targetShotID,
			TargetPhase:        "preview_image",
			TaskType:           "generate",
			ModelPromptProfile: "seedream_5_image",
			Operation:          "image_to_image",
			Status:             "waiting_for_approval",
			Revision:           1,
		},
		{
			ID:                 testUUID(5),
			WorkspaceID:        workspaceID,
			ScopeType:          "shot",
			ScopeID:            otherShotID,
			TargetPhase:        "preview_image",
			TaskType:           "generate",
			ModelPromptProfile: "seedream_5_image",
			Operation:          "image_to_image",
			Status:             "waiting_for_approval",
			Revision:           1,
		},
	}
	service := NewService(store)

	packet, err := service.ReadProjectContext(context.Background(), ReadContextInput{
		WorkspaceID: workspaceID,
		ScopeType:   "shot",
		ScopeID:     uuidString(targetShotID),
	})
	if err != nil {
		t.Fatalf("read context: %v", err)
	}
	if len(packet.RenderPlans) != 1 {
		t.Fatalf("render plans len = %d, want 1: %#v", len(packet.RenderPlans), packet.RenderPlans)
	}
	if packet.RenderPlans[0].ID != targetPlanID {
		t.Fatalf("render plan id = %s, want %s", uuidString(packet.RenderPlans[0].ID), uuidString(targetPlanID))
	}
}

func TestServiceRejectsStudioWorkspace(t *testing.T) {
	workspaceID := testUUID(1)
	service := NewService(newFakeStore(workspaceID, db.WorkspaceModeStudio))
	_, err := service.UpsertProjectBrief(context.Background(), UpsertProjectBriefInput{
		WorkspaceID: workspaceID,
		Brief:       "should fail",
		Mode:        "create",
		Title:       "Studio workspace",
	})
	if !errors.Is(err, ErrAgentWorkspaceRequired) {
		t.Fatalf("expected agent workspace error, got %v", err)
	}
}

func TestServiceAllowsPromptDerivedElementWithoutReferenceWhenStateDoesNotNeedOne(t *testing.T) {
	workspaceID := testUUID(1)
	service := NewService(newFakeStore(workspaceID, db.WorkspaceModeAgent))
	out, err := service.UpsertKeyElements(context.Background(), UpsertKeyElementsInput{
		WorkspaceID: workspaceID,
		Brief:       "创建无需统一参考图的人物抽象状态",
		Mode:        "create",
		Elements: []KeyElementInput{{
			ClientKey:   "character_young_traveler",
			ElementType: "character",
			Name:        "年轻出行者",
			SourceType:  "prompt_derived",
			States: []KeyElementStateInput{{
				ClientKey:         "state_general",
				VisualDescription: "年轻、利落、时尚的都市出行者，不需要固定长相参考。",
				ReferenceStatus:   "none",
				IsDefault:         true,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("upsert key elements: %v", err)
	}
	if out.ElementsCreated != 1 || out.StatesCreated != 1 {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestServiceUpsertStoryboardSkipsDuplicateShotKeyElementsAndDependencies(t *testing.T) {
	workspaceID := testUUID(1)
	store := newFakeStore(workspaceID, db.WorkspaceModeAgent)
	service := NewService(store)

	_, err := service.UpsertKeyElements(context.Background(), UpsertKeyElementsInput{
		WorkspaceID: workspaceID,
		Brief:       "创建商品关键元素",
		Mode:        "create",
		Elements: []KeyElementInput{{
			ClientKey:   "product_luggage",
			ElementType: "product",
			Name:        "行李箱",
			SourceType:  "user_asset",
			States: []KeyElementStateInput{{
				ClientKey:         "state_uploaded",
				VisualDescription: "用户上传的银色行李箱",
				ReferenceStatus:   "ready",
				IsDefault:         true,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("upsert elements: %v", err)
	}
	input := UpsertStoryboardInput{
		WorkspaceID: workspaceID,
		Brief:       "创建两个分镜并绑定商品",
		Mode:        "create",
		Scope:       StoryboardScope{Type: "workspace"},
		Shots: []ShotInput{
			{ClientKey: "shot_01", SortOrder: 1, Title: "开场", CreativeText: "行李箱亮相"},
			{ClientKey: "shot_02", SortOrder: 2, Title: "细节", CreativeText: "行李箱细节"},
		},
		ShotKeyElements: []ShotKeyElementInput{
			{ShotClientKey: "shot_01", ElementClientKey: "product_luggage", StateClientKey: "state_uploaded", Role: "hero_product", Required: true},
		},
		Dependencies: []ShotDependencyInput{
			{FromShotClientKey: "shot_01", ToShotClientKey: "shot_02", DependencyType: "same_product_consistency", Reason: "保持同一个商品"},
		},
	}
	if _, err := service.UpsertStoryboard(context.Background(), input); err != nil {
		t.Fatalf("first upsert storyboard: %v", err)
	}
	if _, err := service.UpsertStoryboard(context.Background(), input); err != nil {
		t.Fatalf("second upsert storyboard should be idempotent: %v", err)
	}
	if len(store.links) != 1 {
		t.Fatalf("links len = %d, want 1: %#v", len(store.links), store.links)
	}
	if len(store.deps) != 1 {
		t.Fatalf("deps len = %d, want 1: %#v", len(store.deps), store.deps)
	}
}

type fakeStore struct {
	workspace   db.Workspace
	briefs      map[string]db.CreativeBrief
	memories    []db.ProjectMemory
	elements    map[string]db.KeyElement
	states      map[string]db.KeyElementState
	scenes      map[string]db.Scene
	shots       map[string]db.Shot
	links       []db.ShotKeyElement
	deps        []db.ShotDependency
	renderPlans []db.RenderPlan
	objectIndex []db.AgentObjectIndex
}

func newFakeStore(workspaceID pgtype.UUID, mode db.WorkspaceMode) *fakeStore {
	return &fakeStore{
		workspace: db.Workspace{ID: workspaceID, Name: "测试项目", Mode: mode},
		briefs:    map[string]db.CreativeBrief{},
		elements:  map[string]db.KeyElement{},
		states:    map[string]db.KeyElementState{},
		scenes:    map[string]db.Scene{},
		shots:     map[string]db.Shot{},
	}
}

func (s *fakeStore) GetWorkspaceByID(_ context.Context, id pgtype.UUID) (db.Workspace, error) {
	if id == s.workspace.ID {
		return s.workspace, nil
	}
	return db.Workspace{}, pgx.ErrNoRows
}

func (s *fakeStore) GetActiveCreativeBriefByWorkspace(context.Context, pgtype.UUID) (db.CreativeBrief, error) {
	for _, brief := range s.briefs {
		if brief.ArchivedAt.Valid || brief.Status == "archived" {
			continue
		}
		return brief, nil
	}
	return db.CreativeBrief{}, pgx.ErrNoRows
}

func (s *fakeStore) GetCreativeBriefByID(_ context.Context, arg db.GetCreativeBriefByIDParams) (db.CreativeBrief, error) {
	for _, brief := range s.briefs {
		if brief.ID == arg.ID {
			return brief, nil
		}
	}
	return db.CreativeBrief{}, pgx.ErrNoRows
}

func (s *fakeStore) CreateCreativeBrief(_ context.Context, arg db.CreateCreativeBriefParams) (db.CreativeBrief, error) {
	brief := db.CreativeBrief{
		ID:                testUUID(byte(len(s.briefs) + 20)),
		WorkspaceID:       arg.WorkspaceID,
		Title:             arg.Title,
		VideoType:         arg.VideoType,
		TargetAudience:    arg.TargetAudience,
		Tone:              arg.Tone,
		VisualStyle:       arg.VisualStyle,
		DurationSec:       arg.DurationSec,
		AspectRatio:       arg.AspectRatio,
		Language:          arg.Language,
		Objective:         arg.Objective,
		Concept:           arg.Concept,
		Constraints:       arg.Constraints,
		Metadata:          arg.Metadata,
		Status:            arg.Status,
		SemanticKey:       arg.SemanticKey,
		DisplayName:       arg.DisplayName,
		CreatedByThreadID: arg.CreatedByThreadID,
		CreatedByTaskID:   arg.CreatedByTaskID,
	}
	s.briefs[uuidString(brief.ID)] = brief
	return brief, nil
}

func (s *fakeStore) UpdateCreativeBrief(_ context.Context, arg db.UpdateCreativeBriefParams) (db.CreativeBrief, error) {
	brief, ok := s.briefs[uuidString(arg.ID)]
	if !ok {
		return db.CreativeBrief{}, pgx.ErrNoRows
	}
	brief.Title = arg.Title
	brief.VideoType = arg.VideoType
	brief.TargetAudience = arg.TargetAudience
	brief.Tone = arg.Tone
	brief.VisualStyle = arg.VisualStyle
	brief.DurationSec = arg.DurationSec
	brief.AspectRatio = arg.AspectRatio
	brief.Language = arg.Language
	brief.Objective = arg.Objective
	brief.Concept = arg.Concept
	brief.Constraints = arg.Constraints
	brief.Metadata = arg.Metadata
	brief.Status = arg.Status
	s.briefs[uuidString(brief.ID)] = brief
	return brief, nil
}

func (s *fakeStore) ArchiveCreativeBrief(_ context.Context, arg db.ArchiveCreativeBriefParams) (db.CreativeBrief, error) {
	brief, ok := s.briefs[uuidString(arg.ID)]
	if !ok {
		return db.CreativeBrief{}, pgx.ErrNoRows
	}
	brief.Status = "archived"
	brief.ArchivedAt.Valid = true
	s.briefs[uuidString(brief.ID)] = brief
	return brief, nil
}

func (s *fakeStore) GetActiveProjectMemoryByWorkspace(context.Context, pgtype.UUID) (db.ProjectMemory, error) {
	for i := len(s.memories) - 1; i >= 0; i-- {
		if s.memories[i].Status == "active" {
			return s.memories[i], nil
		}
	}
	return db.ProjectMemory{}, pgx.ErrNoRows
}

func (s *fakeStore) ArchiveActiveProjectMemoryByWorkspace(context.Context, pgtype.UUID) error {
	for i := range s.memories {
		if s.memories[i].Status == "active" {
			s.memories[i].Status = "archived"
		}
	}
	return nil
}

func (s *fakeStore) CreateProjectMemory(_ context.Context, arg db.CreateProjectMemoryParams) (db.ProjectMemory, error) {
	memory := db.ProjectMemory{
		ID:                   testUUID(byte(len(s.memories) + 40)),
		WorkspaceID:          arg.WorkspaceID,
		Version:              arg.Version,
		Status:               arg.Status,
		CoreIntent:           arg.CoreIntent,
		Soul:                 arg.Soul,
		BrandFacts:           arg.BrandFacts,
		NonNegotiables:       arg.NonNegotiables,
		VisualAnchors:        arg.VisualAnchors,
		Allowed:              arg.Allowed,
		Forbidden:            arg.Forbidden,
		PromptInjectionHints: arg.PromptInjectionHints,
		SourceRefs:           arg.SourceRefs,
		SemanticKey:          arg.SemanticKey,
		DisplayName:          arg.DisplayName,
		CreatedByThreadID:    arg.CreatedByThreadID,
		CreatedByTaskID:      arg.CreatedByTaskID,
	}
	s.memories = append(s.memories, memory)
	return memory, nil
}

func (s *fakeStore) ListProjectMemoriesByWorkspace(context.Context, pgtype.UUID) ([]db.ProjectMemory, error) {
	return append([]db.ProjectMemory(nil), s.memories...), nil
}

func (s *fakeStore) GetKeyElementByClientKey(_ context.Context, arg db.GetKeyElementByClientKeyParams) (db.KeyElement, error) {
	el, ok := s.elements[arg.ClientKey]
	if !ok {
		return db.KeyElement{}, pgx.ErrNoRows
	}
	return el, nil
}

func (s *fakeStore) ListActiveKeyElementsByWorkspace(context.Context, pgtype.UUID) ([]db.KeyElement, error) {
	out := make([]db.KeyElement, 0, len(s.elements))
	for _, el := range s.elements {
		out = append(out, el)
	}
	return out, nil
}

func (s *fakeStore) CreateKeyElement(_ context.Context, arg db.CreateKeyElementParams) (db.KeyElement, error) {
	el := db.KeyElement{
		ID:                testUUID(byte(len(s.elements) + 60)),
		WorkspaceID:       arg.WorkspaceID,
		ClientKey:         arg.ClientKey,
		ElementType:       arg.ElementType,
		Name:              arg.Name,
		Description:       arg.Description,
		SourceType:        arg.SourceType,
		SourceRefs:        arg.SourceRefs,
		Status:            arg.Status,
		SemanticKey:       arg.SemanticKey,
		DisplayName:       arg.DisplayName,
		CreatedByThreadID: arg.CreatedByThreadID,
		CreatedByTaskID:   arg.CreatedByTaskID,
	}
	s.elements[el.ClientKey] = el
	return el, nil
}

func (s *fakeStore) UpdateKeyElement(_ context.Context, arg db.UpdateKeyElementParams) (db.KeyElement, error) {
	for key, el := range s.elements {
		if el.ID == arg.ID {
			el.ElementType = arg.ElementType
			el.Name = arg.Name
			el.Description = arg.Description
			el.SourceType = arg.SourceType
			el.SourceRefs = arg.SourceRefs
			el.Status = arg.Status
			s.elements[key] = el
			return el, nil
		}
	}
	return db.KeyElement{}, pgx.ErrNoRows
}

func (s *fakeStore) GetKeyElementStateByClientKey(_ context.Context, arg db.GetKeyElementStateByClientKeyParams) (db.KeyElementState, error) {
	for _, state := range s.states {
		if state.KeyElementID == arg.KeyElementID && state.ClientKey == arg.ClientKey {
			return state, nil
		}
	}
	return db.KeyElementState{}, pgx.ErrNoRows
}

func (s *fakeStore) GetDefaultKeyElementState(_ context.Context, keyElementID pgtype.UUID) (db.KeyElementState, error) {
	for _, state := range s.states {
		if state.KeyElementID == keyElementID && state.IsDefault {
			return state, nil
		}
	}
	return db.KeyElementState{}, pgx.ErrNoRows
}

func (s *fakeStore) ListActiveKeyElementStatesByWorkspace(context.Context, pgtype.UUID) ([]db.KeyElementState, error) {
	out := make([]db.KeyElementState, 0, len(s.states))
	for _, state := range s.states {
		out = append(out, state)
	}
	return out, nil
}

func (s *fakeStore) ClearDefaultKeyElementState(_ context.Context, keyElementID pgtype.UUID) error {
	for key, state := range s.states {
		if state.KeyElementID == keyElementID {
			state.IsDefault = false
			s.states[key] = state
		}
	}
	return nil
}

func (s *fakeStore) CreateKeyElementState(_ context.Context, arg db.CreateKeyElementStateParams) (db.KeyElementState, error) {
	state := db.KeyElementState{
		ID:                 testUUID(byte(len(s.states) + 80)),
		WorkspaceID:        arg.WorkspaceID,
		KeyElementID:       arg.KeyElementID,
		ClientKey:          arg.ClientKey,
		Label:              arg.Label,
		VisualDescription:  arg.VisualDescription,
		ReferenceStatus:    arg.ReferenceStatus,
		ReferenceNodeID:    arg.ReferenceNodeID,
		ReferenceVersionID: arg.ReferenceVersionID,
		IsDefault:          arg.IsDefault,
		StateFacts:         arg.StateFacts,
		SourceRefs:         arg.SourceRefs,
		Status:             arg.Status,
		SemanticKey:        arg.SemanticKey,
		DisplayName:        arg.DisplayName,
		CreatedByThreadID:  arg.CreatedByThreadID,
		CreatedByTaskID:    arg.CreatedByTaskID,
	}
	s.states[state.ClientKey] = state
	return state, nil
}

func (s *fakeStore) UpdateKeyElementState(_ context.Context, arg db.UpdateKeyElementStateParams) (db.KeyElementState, error) {
	for key, state := range s.states {
		if state.ID == arg.ID {
			state.Label = arg.Label
			state.VisualDescription = arg.VisualDescription
			state.ReferenceStatus = arg.ReferenceStatus
			state.ReferenceNodeID = arg.ReferenceNodeID
			state.ReferenceVersionID = arg.ReferenceVersionID
			state.IsDefault = arg.IsDefault
			state.StateFacts = arg.StateFacts
			state.SourceRefs = arg.SourceRefs
			state.Status = arg.Status
			s.states[key] = state
			return state, nil
		}
	}
	return db.KeyElementState{}, pgx.ErrNoRows
}

func (s *fakeStore) GetSceneByClientKey(_ context.Context, arg db.GetSceneByClientKeyParams) (db.Scene, error) {
	scene, ok := s.scenes[arg.ClientKey]
	if !ok {
		return db.Scene{}, pgx.ErrNoRows
	}
	return scene, nil
}

func (s *fakeStore) ListActiveScenesByWorkspace(context.Context, pgtype.UUID) ([]db.Scene, error) {
	out := make([]db.Scene, 0, len(s.scenes))
	for _, scene := range s.scenes {
		out = append(out, scene)
	}
	return out, nil
}

func (s *fakeStore) CreateScene(_ context.Context, arg db.CreateSceneParams) (db.Scene, error) {
	scene := db.Scene{ID: testUUID(byte(len(s.scenes) + 100)), WorkspaceID: arg.WorkspaceID, ClientKey: arg.ClientKey, SortOrder: arg.SortOrder, Title: arg.Title, Description: arg.Description, Location: arg.Location, Mood: arg.Mood, Status: arg.Status, SemanticKey: arg.SemanticKey, DisplayName: arg.DisplayName, CreatedByThreadID: arg.CreatedByThreadID, CreatedByTaskID: arg.CreatedByTaskID}
	s.scenes[scene.ClientKey] = scene
	return scene, nil
}

func (s *fakeStore) UpdateScene(_ context.Context, arg db.UpdateSceneParams) (db.Scene, error) {
	for key, scene := range s.scenes {
		if scene.ID == arg.ID {
			scene.ClientKey = arg.ClientKey
			scene.SortOrder = arg.SortOrder
			scene.Title = arg.Title
			scene.Description = arg.Description
			scene.Location = arg.Location
			scene.Mood = arg.Mood
			scene.Status = arg.Status
			delete(s.scenes, key)
			s.scenes[scene.ClientKey] = scene
			return scene, nil
		}
	}
	return db.Scene{}, pgx.ErrNoRows
}

func (s *fakeStore) GetShotByClientKey(_ context.Context, arg db.GetShotByClientKeyParams) (db.Shot, error) {
	shot, ok := s.shots[arg.ClientKey]
	if !ok {
		return db.Shot{}, pgx.ErrNoRows
	}
	return shot, nil
}

func (s *fakeStore) ListActiveShotsByWorkspace(context.Context, pgtype.UUID) ([]db.Shot, error) {
	out := make([]db.Shot, 0, len(s.shots))
	for _, shot := range s.shots {
		out = append(out, shot)
	}
	return out, nil
}

func (s *fakeStore) CreateShot(_ context.Context, arg db.CreateShotParams) (db.Shot, error) {
	shot := db.Shot{ID: testUUID(byte(len(s.shots) + 120)), WorkspaceID: arg.WorkspaceID, ClientKey: arg.ClientKey, SortOrder: arg.SortOrder, Title: arg.Title, Brief: arg.Brief, DurationSec: arg.DurationSec, NarrativePurpose: arg.NarrativePurpose, Status: arg.Status, SceneID: arg.SceneID, ShotKind: arg.ShotKind, CreativeText: arg.CreativeText, VisualIntent: arg.VisualIntent, ActionText: arg.ActionText, CameraIntent: arg.CameraIntent, Dialogue: arg.Dialogue, Narration: arg.Narration, AudioPlan: arg.AudioPlan, SemanticKey: arg.SemanticKey, DisplayName: arg.DisplayName}
	s.shots[shot.ClientKey] = shot
	return shot, nil
}

func (s *fakeStore) UpdateShot(_ context.Context, arg db.UpdateShotParams) (db.Shot, error) {
	for key, shot := range s.shots {
		if shot.ID == arg.ID {
			shot.ClientKey = arg.ClientKey
			shot.SortOrder = arg.SortOrder
			shot.Title = arg.Title
			shot.Brief = arg.Brief
			shot.DurationSec = arg.DurationSec
			shot.NarrativePurpose = arg.NarrativePurpose
			shot.Status = arg.Status
			shot.SceneID = arg.SceneID
			shot.ShotKind = arg.ShotKind
			shot.CreativeText = arg.CreativeText
			shot.VisualIntent = arg.VisualIntent
			shot.ActionText = arg.ActionText
			shot.CameraIntent = arg.CameraIntent
			shot.Dialogue = arg.Dialogue
			shot.Narration = arg.Narration
			shot.AudioPlan = arg.AudioPlan
			delete(s.shots, key)
			s.shots[shot.ClientKey] = shot
			return shot, nil
		}
	}
	return db.Shot{}, pgx.ErrNoRows
}

func (s *fakeStore) ArchiveShot(_ context.Context, arg db.ArchiveShotParams) (db.Shot, error) {
	for key, shot := range s.shots {
		if shot.ID == arg.ID {
			shot.Status = "archived"
			delete(s.shots, key)
			return shot, nil
		}
	}
	return db.Shot{}, pgx.ErrNoRows
}

func (s *fakeStore) DeleteShotKeyElementsByWorkspace(context.Context, pgtype.UUID) error {
	s.links = nil
	return nil
}

func (s *fakeStore) DeleteShotKeyElementsByShot(_ context.Context, arg db.DeleteShotKeyElementsByShotParams) error {
	out := s.links[:0]
	for _, link := range s.links {
		if link.ShotID != arg.ShotID {
			out = append(out, link)
		}
	}
	s.links = out
	return nil
}

func (s *fakeStore) CreateShotKeyElement(_ context.Context, arg db.CreateShotKeyElementParams) (db.ShotKeyElement, error) {
	link := db.ShotKeyElement{ID: testUUID(byte(len(s.links) + 140)), WorkspaceID: arg.WorkspaceID, ShotID: arg.ShotID, KeyElementID: arg.KeyElementID, KeyElementStateID: arg.KeyElementStateID, Role: arg.Role, Required: arg.Required, SortOrder: arg.SortOrder}
	s.links = append(s.links, link)
	return link, nil
}

func (s *fakeStore) ListShotKeyElementsByWorkspace(context.Context, pgtype.UUID) ([]db.ShotKeyElement, error) {
	return append([]db.ShotKeyElement(nil), s.links...), nil
}

func (s *fakeStore) DeleteShotDependenciesByWorkspace(context.Context, pgtype.UUID) error {
	s.deps = nil
	return nil
}

func (s *fakeStore) DeleteShotDependenciesForShot(_ context.Context, arg db.DeleteShotDependenciesForShotParams) error {
	out := s.deps[:0]
	for _, dep := range s.deps {
		if dep.FromShotID != arg.FromShotID && dep.ToShotID != arg.FromShotID {
			out = append(out, dep)
		}
	}
	s.deps = out
	return nil
}

func (s *fakeStore) CreateShotDependency(_ context.Context, arg db.CreateShotDependencyParams) (db.ShotDependency, error) {
	dep := db.ShotDependency{ID: testUUID(byte(len(s.deps) + 160)), WorkspaceID: arg.WorkspaceID, FromShotID: arg.FromShotID, ToShotID: arg.ToShotID, DependencyType: arg.DependencyType, RequiredArtifact: arg.RequiredArtifact, InjectionRole: arg.InjectionRole, BlockingPhase: arg.BlockingPhase, StalePolicy: arg.StalePolicy, Reason: arg.Reason, SemanticKey: arg.SemanticKey, DisplayName: arg.DisplayName}
	s.deps = append(s.deps, dep)
	return dep, nil
}

func (s *fakeStore) ListShotDependenciesByWorkspace(context.Context, pgtype.UUID) ([]db.ShotDependency, error) {
	return append([]db.ShotDependency(nil), s.deps...), nil
}

func (s *fakeStore) ListRenderPlansByWorkspace(context.Context, pgtype.UUID) ([]db.RenderPlan, error) {
	return append([]db.RenderPlan(nil), s.renderPlans...), nil
}

func (s *fakeStore) ListRenderPlansByScope(_ context.Context, arg db.ListRenderPlansByScopeParams) ([]db.RenderPlan, error) {
	out := []db.RenderPlan{}
	for _, plan := range s.renderPlans {
		if plan.WorkspaceID == arg.WorkspaceID && plan.ScopeType == arg.ScopeType && plan.ScopeID == arg.ScopeID {
			out = append(out, plan)
		}
	}
	return out, nil
}

func (s *fakeStore) ListAgentObjectsByWorkspace(context.Context, pgtype.UUID) ([]db.AgentObjectIndex, error) {
	return append([]db.AgentObjectIndex(nil), s.objectIndex...), nil
}

func testUUID(seed byte) pgtype.UUID {
	var bytes [16]byte
	bytes[15] = seed
	return pgtype.UUID{Bytes: bytes, Valid: true}
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
