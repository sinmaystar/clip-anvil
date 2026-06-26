package creative

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrAgentWorkspaceRequired    = errors.New("agent workspace required")
	ErrInvalidCreativeStateInput = errors.New("invalid creative state input")
	ErrCreativeStateNotFound     = errors.New("creative state object not found")
)

type Store interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)

	GetActiveCreativeBriefByWorkspace(ctx context.Context, workspaceID pgtype.UUID) (db.CreativeBrief, error)
	GetCreativeBriefByID(ctx context.Context, arg db.GetCreativeBriefByIDParams) (db.CreativeBrief, error)
	CreateCreativeBrief(ctx context.Context, arg db.CreateCreativeBriefParams) (db.CreativeBrief, error)
	UpdateCreativeBrief(ctx context.Context, arg db.UpdateCreativeBriefParams) (db.CreativeBrief, error)
	ArchiveCreativeBrief(ctx context.Context, arg db.ArchiveCreativeBriefParams) (db.CreativeBrief, error)

	GetActiveProjectMemoryByWorkspace(ctx context.Context, workspaceID pgtype.UUID) (db.ProjectMemory, error)
	ListProjectMemoriesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.ProjectMemory, error)
	ArchiveActiveProjectMemoryByWorkspace(ctx context.Context, workspaceID pgtype.UUID) error
	CreateProjectMemory(ctx context.Context, arg db.CreateProjectMemoryParams) (db.ProjectMemory, error)

	GetKeyElementByClientKey(ctx context.Context, arg db.GetKeyElementByClientKeyParams) (db.KeyElement, error)
	ListActiveKeyElementsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.KeyElement, error)
	CreateKeyElement(ctx context.Context, arg db.CreateKeyElementParams) (db.KeyElement, error)
	UpdateKeyElement(ctx context.Context, arg db.UpdateKeyElementParams) (db.KeyElement, error)
	GetKeyElementStateByClientKey(ctx context.Context, arg db.GetKeyElementStateByClientKeyParams) (db.KeyElementState, error)
	GetDefaultKeyElementState(ctx context.Context, keyElementID pgtype.UUID) (db.KeyElementState, error)
	ListActiveKeyElementStatesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.KeyElementState, error)
	ClearDefaultKeyElementState(ctx context.Context, keyElementID pgtype.UUID) error
	CreateKeyElementState(ctx context.Context, arg db.CreateKeyElementStateParams) (db.KeyElementState, error)
	UpdateKeyElementState(ctx context.Context, arg db.UpdateKeyElementStateParams) (db.KeyElementState, error)

	GetSceneByClientKey(ctx context.Context, arg db.GetSceneByClientKeyParams) (db.Scene, error)
	ListActiveScenesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Scene, error)
	CreateScene(ctx context.Context, arg db.CreateSceneParams) (db.Scene, error)
	UpdateScene(ctx context.Context, arg db.UpdateSceneParams) (db.Scene, error)

	GetShotByClientKey(ctx context.Context, arg db.GetShotByClientKeyParams) (db.Shot, error)
	ListActiveShotsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Shot, error)
	CreateShot(ctx context.Context, arg db.CreateShotParams) (db.Shot, error)
	UpdateShot(ctx context.Context, arg db.UpdateShotParams) (db.Shot, error)
	ArchiveShot(ctx context.Context, arg db.ArchiveShotParams) (db.Shot, error)

	DeleteShotKeyElementsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) error
	DeleteShotKeyElementsByShot(ctx context.Context, arg db.DeleteShotKeyElementsByShotParams) error
	CreateShotKeyElement(ctx context.Context, arg db.CreateShotKeyElementParams) (db.ShotKeyElement, error)
	ListShotKeyElementsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.ShotKeyElement, error)

	DeleteShotDependenciesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) error
	DeleteShotDependenciesForShot(ctx context.Context, arg db.DeleteShotDependenciesForShotParams) error
	CreateShotDependency(ctx context.Context, arg db.CreateShotDependencyParams) (db.ShotDependency, error)
	ListShotDependenciesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.ShotDependency, error)

	ListRenderPlansByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.RenderPlan, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) UpsertProjectBrief(ctx context.Context, input UpsertProjectBriefInput) (db.CreativeBrief, error) {
	if err := s.requireAgentWorkspace(ctx, input.WorkspaceID); err != nil {
		return db.CreativeBrief{}, err
	}
	mode := strings.TrimSpace(input.Mode)
	if input.Brief == "" || !oneOf(mode, "create", "patch", "archive") {
		return db.CreativeBrief{}, ErrInvalidCreativeStateInput
	}
	if input.DurationSec != nil && *input.DurationSec <= 0 {
		return db.CreativeBrief{}, ErrInvalidCreativeStateInput
	}
	if mode == "create" {
		return s.store.CreateCreativeBrief(ctx, db.CreateCreativeBriefParams{
			WorkspaceID:       input.WorkspaceID,
			Title:             strings.TrimSpace(input.Title),
			VideoType:         strings.TrimSpace(input.VideoType),
			TargetAudience:    strings.TrimSpace(input.TargetAudience),
			Tone:              strings.TrimSpace(input.Tone),
			VisualStyle:       strings.TrimSpace(input.VisualStyle),
			DurationSec:       float8(input.DurationSec),
			AspectRatio:       strings.TrimSpace(input.AspectRatio),
			Language:          strings.TrimSpace(input.Language),
			Objective:         strings.TrimSpace(input.Objective),
			Concept:           strings.TrimSpace(input.Concept),
			Constraints:       jsonBytes(input.Constraints, "[]"),
			Metadata:          jsonBytes(map[string]any{"brief": input.Brief, "reason": input.Reason}, "{}"),
			Status:            "active",
			CreatedByThreadID: input.ThreadID,
			CreatedByTaskID:   input.TaskID,
		})
	}

	current, err := s.resolveBrief(ctx, input.WorkspaceID, input.BriefID)
	if err != nil {
		return db.CreativeBrief{}, err
	}
	if mode == "archive" {
		return s.store.ArchiveCreativeBrief(ctx, db.ArchiveCreativeBriefParams{ID: current.ID, WorkspaceID: input.WorkspaceID})
	}
	return s.store.UpdateCreativeBrief(ctx, db.UpdateCreativeBriefParams{
		ID:             current.ID,
		WorkspaceID:    input.WorkspaceID,
		Title:          patchString(current.Title, input.Title),
		VideoType:      patchString(current.VideoType, input.VideoType),
		TargetAudience: patchString(current.TargetAudience, input.TargetAudience),
		Tone:           patchString(current.Tone, input.Tone),
		VisualStyle:    patchString(current.VisualStyle, input.VisualStyle),
		DurationSec:    patchFloat8(current.DurationSec, input.DurationSec),
		AspectRatio:    patchString(current.AspectRatio, input.AspectRatio),
		Language:       patchString(current.Language, input.Language),
		Objective:      patchString(current.Objective, input.Objective),
		Concept:        patchString(current.Concept, input.Concept),
		Constraints:    patchJSON(current.Constraints, input.Constraints),
		Metadata:       jsonBytes(map[string]any{"brief": input.Brief, "reason": input.Reason}, "{}"),
		Status:         patchString(current.Status, "active"),
	})
}

func (s *Service) UpdateProjectMemory(ctx context.Context, input UpdateProjectMemoryInput) (db.ProjectMemory, error) {
	if err := s.requireAgentWorkspace(ctx, input.WorkspaceID); err != nil {
		return db.ProjectMemory{}, err
	}
	mode := strings.TrimSpace(input.Mode)
	if input.Brief == "" || input.Reason == "" || !oneOf(mode, "create", "patch", "replace") {
		return db.ProjectMemory{}, ErrInvalidCreativeStateInput
	}
	if input.RequiresUserApproval {
		return db.ProjectMemory{}, fmt.Errorf("%w: requires user approval", ErrInvalidCreativeStateInput)
	}
	for _, hint := range input.PromptInjectionHints {
		if len([]rune(strings.TrimSpace(hint))) > 80 {
			return db.ProjectMemory{}, ErrInvalidCreativeStateInput
		}
	}

	current, err := s.store.GetActiveProjectMemoryByWorkspace(ctx, input.WorkspaceID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return db.ProjectMemory{}, err
	}
	if mode == "create" && err == nil {
		mode = "patch"
	}
	if mode == "patch" && errors.Is(err, pgx.ErrNoRows) {
		mode = "create"
	}
	if mode == "replace" && strings.TrimSpace(input.CoreIntent) == "" && strings.TrimSpace(input.Soul) == "" {
		return db.ProjectMemory{}, ErrInvalidCreativeStateInput
	}

	version := int32(1)
	if err == nil {
		version = current.Version + 1
	}
	if err := s.store.ArchiveActiveProjectMemoryByWorkspace(ctx, input.WorkspaceID); err != nil {
		return db.ProjectMemory{}, err
	}
	return s.store.CreateProjectMemory(ctx, db.CreateProjectMemoryParams{
		WorkspaceID:          input.WorkspaceID,
		Version:              version,
		Status:               "active",
		CoreIntent:           mergeString(current.CoreIntent, input.CoreIntent, mode),
		Soul:                 mergeString(current.Soul, input.Soul, mode),
		BrandFacts:           mergeArray(current.BrandFacts, input.BrandFacts, mode),
		NonNegotiables:       mergeArray(current.NonNegotiables, input.NonNegotiables, mode),
		VisualAnchors:        mergeArray(current.VisualAnchors, input.VisualAnchors, mode),
		Allowed:              mergeArray(current.Allowed, input.Allowed, mode),
		Forbidden:            mergeArray(current.Forbidden, input.Forbidden, mode),
		PromptInjectionHints: mergeArray(current.PromptInjectionHints, input.PromptInjectionHints, mode),
		SourceRefs:           mergeArray(current.SourceRefs, input.SourceRefs, mode),
		CreatedByThreadID:    input.ThreadID,
		CreatedByTaskID:      input.TaskID,
	})
}

func (s *Service) UpsertKeyElements(ctx context.Context, input UpsertKeyElementsInput) (UpsertKeyElementsOutput, error) {
	if err := s.requireAgentWorkspace(ctx, input.WorkspaceID); err != nil {
		return UpsertKeyElementsOutput{}, err
	}
	mode := strings.TrimSpace(input.Mode)
	if input.Brief == "" || len(input.Elements) == 0 || !oneOf(mode, "create", "patch", "replace") {
		return UpsertKeyElementsOutput{}, ErrInvalidCreativeStateInput
	}
	out := UpsertKeyElementsOutput{}
	for _, elementInput := range input.Elements {
		if err := validateKeyElement(elementInput); err != nil {
			return UpsertKeyElementsOutput{}, err
		}
		element, err := s.store.GetKeyElementByClientKey(ctx, db.GetKeyElementByClientKeyParams{WorkspaceID: input.WorkspaceID, ClientKey: elementInput.ClientKey})
		created := false
		if errors.Is(err, pgx.ErrNoRows) {
			element, err = s.store.CreateKeyElement(ctx, db.CreateKeyElementParams{
				WorkspaceID:       input.WorkspaceID,
				ClientKey:         elementInput.ClientKey,
				ElementType:       elementInput.ElementType,
				Name:              elementInput.Name,
				Description:       elementInput.Description,
				SourceType:        elementInput.SourceType,
				SourceRefs:        jsonBytes(elementInput.SourceRefs, "[]"),
				Status:            "active",
				CreatedByThreadID: input.ThreadID,
				CreatedByTaskID:   input.TaskID,
			})
			created = true
		} else if err == nil {
			element, err = s.store.UpdateKeyElement(ctx, db.UpdateKeyElementParams{
				ID:          element.ID,
				WorkspaceID: input.WorkspaceID,
				ElementType: patchString(element.ElementType, elementInput.ElementType),
				Name:        patchString(element.Name, elementInput.Name),
				Description: patchString(element.Description, elementInput.Description),
				SourceType:  patchString(element.SourceType, elementInput.SourceType),
				SourceRefs:  patchJSON(element.SourceRefs, elementInput.SourceRefs),
				Status:      "active",
			})
		}
		if err != nil {
			return UpsertKeyElementsOutput{}, err
		}
		if created {
			out.ElementsCreated++
		} else {
			out.ElementsUpdated++
		}
		out.Elements = append(out.Elements, element)

		for _, stateInput := range elementInput.States {
			state, stateCreated, err := s.upsertElementState(ctx, input, element, stateInput)
			if err != nil {
				return UpsertKeyElementsOutput{}, err
			}
			if stateCreated {
				out.StatesCreated++
			} else {
				out.StatesUpdated++
			}
			out.States = append(out.States, state)
		}
	}
	return out, nil
}

func (s *Service) UpsertStoryboard(ctx context.Context, input UpsertStoryboardInput) (UpsertStoryboardOutput, error) {
	if err := s.requireAgentWorkspace(ctx, input.WorkspaceID); err != nil {
		return UpsertStoryboardOutput{}, err
	}
	mode := strings.TrimSpace(input.Mode)
	if input.Brief == "" || !oneOf(mode, "create", "patch", "replace", "archive") || input.Scope.Type == "" {
		return UpsertStoryboardOutput{}, ErrInvalidCreativeStateInput
	}
	if mode == "replace" {
		if err := s.store.DeleteShotKeyElementsByWorkspace(ctx, input.WorkspaceID); err != nil {
			return UpsertStoryboardOutput{}, err
		}
		if err := s.store.DeleteShotDependenciesByWorkspace(ctx, input.WorkspaceID); err != nil {
			return UpsertStoryboardOutput{}, err
		}
	}
	out := UpsertStoryboardOutput{}
	scenesByKey := map[string]db.Scene{}
	for _, scene := range input.Scenes {
		createdScene, created, err := s.upsertScene(ctx, input, scene)
		if err != nil {
			return UpsertStoryboardOutput{}, err
		}
		if created {
			out.ScenesCreated++
		} else {
			out.ScenesUpdated++
		}
		scenesByKey[createdScene.ClientKey] = createdScene
		out.Scenes = append(out.Scenes, createdScene)
	}
	existingScenes, err := s.store.ListActiveScenesByWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return UpsertStoryboardOutput{}, err
	}
	for _, scene := range existingScenes {
		scenesByKey[scene.ClientKey] = scene
	}

	shotsByKey := map[string]db.Shot{}
	for _, shot := range input.Shots {
		createdShot, created, err := s.upsertShot(ctx, input, scenesByKey, shot)
		if err != nil {
			return UpsertStoryboardOutput{}, err
		}
		if created {
			out.ShotsCreated++
		} else {
			out.ShotsUpdated++
		}
		shotsByKey[createdShot.ClientKey] = createdShot
		out.Shots = append(out.Shots, createdShot)
	}
	existingShots, err := s.store.ListActiveShotsByWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return UpsertStoryboardOutput{}, err
	}
	for _, shot := range existingShots {
		shotsByKey[shot.ClientKey] = shot
	}

	for _, link := range input.ShotKeyElements {
		if err := s.createShotKeyElement(ctx, input.WorkspaceID, shotsByKey, link); err != nil {
			return UpsertStoryboardOutput{}, err
		}
		out.ShotKeyElements++
	}
	for _, dep := range input.Dependencies {
		created, err := s.createShotDependency(ctx, input.WorkspaceID, shotsByKey, dep)
		if err != nil {
			return UpsertStoryboardOutput{}, err
		}
		out.DependenciesCreated++
		out.Dependencies = append(out.Dependencies, created)
	}
	return out, nil
}

func (s *Service) ReadProjectContext(ctx context.Context, input ReadContextInput) (ContextPacket, error) {
	if err := s.requireAgentWorkspace(ctx, input.WorkspaceID); err != nil {
		return ContextPacket{}, err
	}
	workspace, err := s.store.GetWorkspaceByID(ctx, input.WorkspaceID)
	if err != nil {
		return ContextPacket{}, err
	}
	packet := ContextPacket{Workspace: workspace}
	if brief, err := s.store.GetActiveCreativeBriefByWorkspace(ctx, input.WorkspaceID); err == nil {
		packet.Brief = &brief
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ContextPacket{}, err
	}
	if memory, err := s.store.GetActiveProjectMemoryByWorkspace(ctx, input.WorkspaceID); err == nil {
		packet.Memory = &memory
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ContextPacket{}, err
	}
	if packet.Elements, err = s.store.ListActiveKeyElementsByWorkspace(ctx, input.WorkspaceID); err != nil {
		return ContextPacket{}, err
	}
	if packet.ElementStates, err = s.store.ListActiveKeyElementStatesByWorkspace(ctx, input.WorkspaceID); err != nil {
		return ContextPacket{}, err
	}
	if packet.Scenes, err = s.store.ListActiveScenesByWorkspace(ctx, input.WorkspaceID); err != nil {
		return ContextPacket{}, err
	}
	if packet.Shots, err = s.store.ListActiveShotsByWorkspace(ctx, input.WorkspaceID); err != nil {
		return ContextPacket{}, err
	}
	if packet.ShotKeyElements, err = s.store.ListShotKeyElementsByWorkspace(ctx, input.WorkspaceID); err != nil {
		return ContextPacket{}, err
	}
	if packet.Dependencies, err = s.store.ListShotDependenciesByWorkspace(ctx, input.WorkspaceID); err != nil {
		return ContextPacket{}, err
	}
	if packet.RenderPlans, err = s.store.ListRenderPlansByWorkspace(ctx, input.WorkspaceID); err != nil {
		return ContextPacket{}, err
	}
	return packet, nil
}

func (s *Service) requireAgentWorkspace(ctx context.Context, workspaceID pgtype.UUID) error {
	if s == nil || s.store == nil || !workspaceID.Valid {
		return ErrInvalidCreativeStateInput
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

func (s *Service) resolveBrief(ctx context.Context, workspaceID pgtype.UUID, briefID string) (db.CreativeBrief, error) {
	if strings.TrimSpace(briefID) == "" {
		brief, err := s.store.GetActiveCreativeBriefByWorkspace(ctx, workspaceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return db.CreativeBrief{}, ErrCreativeStateNotFound
		}
		return brief, err
	}
	id, err := parseUUID(briefID)
	if err != nil {
		return db.CreativeBrief{}, ErrInvalidCreativeStateInput
	}
	return s.store.GetCreativeBriefByID(ctx, db.GetCreativeBriefByIDParams{ID: id, WorkspaceID: workspaceID})
}

func (s *Service) upsertElementState(ctx context.Context, input UpsertKeyElementsInput, element db.KeyElement, stateInput KeyElementStateInput) (db.KeyElementState, bool, error) {
	if strings.TrimSpace(stateInput.ClientKey) == "" || !oneOf(stateInput.ReferenceStatus, "none", "needs_reference", "ready", "approved", "rejected") {
		return db.KeyElementState{}, false, ErrInvalidCreativeStateInput
	}
	if stateInput.IsDefault {
		if err := s.store.ClearDefaultKeyElementState(ctx, element.ID); err != nil {
			return db.KeyElementState{}, false, err
		}
	}
	state, err := s.store.GetKeyElementStateByClientKey(ctx, db.GetKeyElementStateByClientKeyParams{KeyElementID: element.ID, ClientKey: stateInput.ClientKey})
	if errors.Is(err, pgx.ErrNoRows) {
		created, err := s.store.CreateKeyElementState(ctx, db.CreateKeyElementStateParams{
			WorkspaceID:        input.WorkspaceID,
			KeyElementID:       element.ID,
			ClientKey:          stateInput.ClientKey,
			Label:              defaultString(stateInput.Label, stateInput.ClientKey),
			VisualDescription:  stateInput.VisualDescription,
			ReferenceStatus:    stateInput.ReferenceStatus,
			ReferenceNodeID:    uuidOrZero(stateInput.ReferenceNodeID),
			ReferenceVersionID: uuidOrZero(stateInput.ReferenceVersionID),
			IsDefault:          stateInput.IsDefault,
			StateFacts:         jsonBytes(stateInput.StateFacts, "[]"),
			SourceRefs:         jsonBytes(stateInput.SourceRefs, "[]"),
			Status:             "active",
			CreatedByThreadID:  input.ThreadID,
			CreatedByTaskID:    input.TaskID,
		})
		return created, true, err
	}
	if err != nil {
		return db.KeyElementState{}, false, err
	}
	updated, err := s.store.UpdateKeyElementState(ctx, db.UpdateKeyElementStateParams{
		ID:                 state.ID,
		WorkspaceID:        input.WorkspaceID,
		Label:              patchString(state.Label, stateInput.Label),
		VisualDescription:  patchString(state.VisualDescription, stateInput.VisualDescription),
		ReferenceStatus:    patchString(state.ReferenceStatus, stateInput.ReferenceStatus),
		ReferenceNodeID:    patchUUID(state.ReferenceNodeID, stateInput.ReferenceNodeID),
		ReferenceVersionID: patchUUID(state.ReferenceVersionID, stateInput.ReferenceVersionID),
		IsDefault:          stateInput.IsDefault,
		StateFacts:         patchJSON(state.StateFacts, stateInput.StateFacts),
		SourceRefs:         patchJSON(state.SourceRefs, stateInput.SourceRefs),
		Status:             "active",
	})
	return updated, false, err
}

func (s *Service) upsertScene(ctx context.Context, input UpsertStoryboardInput, sceneInput SceneInput) (db.Scene, bool, error) {
	if sceneInput.ClientKey == "" || sceneInput.Title == "" {
		return db.Scene{}, false, ErrInvalidCreativeStateInput
	}
	if sceneInput.SortOrder <= 0 {
		sceneInput.SortOrder = 1
	}
	scene, err := s.store.GetSceneByClientKey(ctx, db.GetSceneByClientKeyParams{WorkspaceID: input.WorkspaceID, ClientKey: sceneInput.ClientKey})
	if errors.Is(err, pgx.ErrNoRows) {
		created, err := s.store.CreateScene(ctx, db.CreateSceneParams{
			WorkspaceID:       input.WorkspaceID,
			ClientKey:         sceneInput.ClientKey,
			SortOrder:         sceneInput.SortOrder,
			Title:             sceneInput.Title,
			Description:       sceneInput.Description,
			Location:          sceneInput.Location,
			Mood:              sceneInput.Mood,
			Status:            "planned",
			CreatedByThreadID: input.ThreadID,
			CreatedByTaskID:   input.TaskID,
		})
		return created, true, err
	}
	if err != nil {
		return db.Scene{}, false, err
	}
	updated, err := s.store.UpdateScene(ctx, db.UpdateSceneParams{
		ID:          scene.ID,
		WorkspaceID: input.WorkspaceID,
		ClientKey:   sceneInput.ClientKey,
		SortOrder:   sceneInput.SortOrder,
		Title:       patchString(scene.Title, sceneInput.Title),
		Description: patchString(scene.Description, sceneInput.Description),
		Location:    patchString(scene.Location, sceneInput.Location),
		Mood:        patchString(scene.Mood, sceneInput.Mood),
		Status:      patchString(scene.Status, "planned"),
	})
	return updated, false, err
}

func (s *Service) upsertShot(ctx context.Context, input UpsertStoryboardInput, scenesByKey map[string]db.Scene, shotInput ShotInput) (db.Shot, bool, error) {
	if shotInput.ClientKey == "" || shotInput.Title == "" || shotInput.CreativeText == "" {
		return db.Shot{}, false, ErrInvalidCreativeStateInput
	}
	if shotInput.SortOrder <= 0 {
		shotInput.SortOrder = 1
	}
	sceneID := pgtype.UUID{}
	if strings.TrimSpace(shotInput.SceneClientKey) != "" {
		scene, ok := scenesByKey[shotInput.SceneClientKey]
		if !ok {
			return db.Shot{}, false, ErrCreativeStateNotFound
		}
		sceneID = scene.ID
	}
	duration := pgtype.Float8{}
	if shotInput.DurationSec > 0 {
		duration = pgtype.Float8{Float64: shotInput.DurationSec, Valid: true}
	}
	brief := jsonBytes(map[string]any{"brief": input.Brief, "reason": input.Reason}, "{}")
	audioPlan := jsonBytes(shotInput.AudioPlan, "{}")
	shot, err := s.store.GetShotByClientKey(ctx, db.GetShotByClientKeyParams{WorkspaceID: input.WorkspaceID, ClientKey: shotInput.ClientKey})
	if errors.Is(err, pgx.ErrNoRows) {
		created, err := s.store.CreateShot(ctx, db.CreateShotParams{
			WorkspaceID:      input.WorkspaceID,
			ClientKey:        shotInput.ClientKey,
			SortOrder:        shotInput.SortOrder,
			Title:            shotInput.Title,
			Brief:            brief,
			DurationSec:      duration,
			NarrativePurpose: shotInput.NarrativePurpose,
			Status:           "planned",
			SceneID:          sceneID,
			ShotKind:         shotInput.ShotKind,
			CreativeText:     shotInput.CreativeText,
			VisualIntent:     shotInput.VisualIntent,
			ActionText:       shotInput.ActionText,
			CameraIntent:     shotInput.CameraIntent,
			Dialogue:         shotInput.Dialogue,
			Narration:        shotInput.Narration,
			AudioPlan:        audioPlan,
		})
		return created, true, err
	}
	if err != nil {
		return db.Shot{}, false, err
	}
	updated, err := s.store.UpdateShot(ctx, db.UpdateShotParams{
		ID:               shot.ID,
		ClientKey:        shotInput.ClientKey,
		SortOrder:        shotInput.SortOrder,
		Title:            patchString(shot.Title, shotInput.Title),
		Brief:            brief,
		DurationSec:      duration,
		NarrativePurpose: patchString(shot.NarrativePurpose, shotInput.NarrativePurpose),
		Status:           patchString(shot.Status, "planned"),
		SceneID:          sceneID,
		ShotKind:         patchString(shot.ShotKind, shotInput.ShotKind),
		CreativeText:     patchString(shot.CreativeText, shotInput.CreativeText),
		VisualIntent:     patchString(shot.VisualIntent, shotInput.VisualIntent),
		ActionText:       patchString(shot.ActionText, shotInput.ActionText),
		CameraIntent:     patchString(shot.CameraIntent, shotInput.CameraIntent),
		Dialogue:         patchString(shot.Dialogue, shotInput.Dialogue),
		Narration:        patchString(shot.Narration, shotInput.Narration),
		AudioPlan:        audioPlan,
		WorkspaceID:      input.WorkspaceID,
	})
	return updated, false, err
}

func (s *Service) createShotKeyElement(ctx context.Context, workspaceID pgtype.UUID, shotsByKey map[string]db.Shot, input ShotKeyElementInput) error {
	shot, ok := shotsByKey[input.ShotClientKey]
	if !ok || input.ElementClientKey == "" || input.Role == "" {
		return ErrInvalidCreativeStateInput
	}
	element, err := s.store.GetKeyElementByClientKey(ctx, db.GetKeyElementByClientKeyParams{WorkspaceID: workspaceID, ClientKey: input.ElementClientKey})
	if err != nil {
		return err
	}
	stateID := pgtype.UUID{}
	if strings.TrimSpace(input.StateClientKey) != "" {
		state, err := s.store.GetKeyElementStateByClientKey(ctx, db.GetKeyElementStateByClientKeyParams{KeyElementID: element.ID, ClientKey: input.StateClientKey})
		if err != nil {
			return err
		}
		stateID = state.ID
	} else if state, err := s.store.GetDefaultKeyElementState(ctx, element.ID); err == nil {
		stateID = state.ID
	}
	_, err = s.store.CreateShotKeyElement(ctx, db.CreateShotKeyElementParams{
		WorkspaceID:       workspaceID,
		ShotID:            shot.ID,
		KeyElementID:      element.ID,
		KeyElementStateID: stateID,
		Role:              input.Role,
		Required:          input.Required,
		SortOrder:         input.SortOrder,
	})
	return err
}

func (s *Service) createShotDependency(ctx context.Context, workspaceID pgtype.UUID, shotsByKey map[string]db.Shot, input ShotDependencyInput) (db.ShotDependency, error) {
	fromShot, ok := shotsByKey[input.FromShotClientKey]
	if !ok {
		return db.ShotDependency{}, ErrCreativeStateNotFound
	}
	toShot, ok := shotsByKey[input.ToShotClientKey]
	if !ok || fromShot.ID == toShot.ID || input.DependencyType == "" || input.Reason == "" {
		return db.ShotDependency{}, ErrInvalidCreativeStateInput
	}
	return s.store.CreateShotDependency(ctx, db.CreateShotDependencyParams{
		WorkspaceID:      workspaceID,
		FromShotID:       fromShot.ID,
		ToShotID:         toShot.ID,
		DependencyType:   input.DependencyType,
		RequiredArtifact: input.RequiredArtifact,
		InjectionRole:    input.InjectionRole,
		BlockingPhase:    input.BlockingPhase,
		StalePolicy:      "mark_downstream_stale",
		Reason:           input.Reason,
	})
}

func validateKeyElement(input KeyElementInput) error {
	if input.ClientKey == "" || input.ElementType == "" || input.Name == "" {
		return ErrInvalidCreativeStateInput
	}
	if !oneOf(input.ElementType, "product", "character", "scene", "prop", "style") {
		return ErrInvalidCreativeStateInput
	}
	if strings.TrimSpace(input.SourceType) != "" && !oneOf(input.SourceType, "user_asset", "material_analysis", "prompt_derived", "agent_created") {
		return ErrInvalidCreativeStateInput
	}
	if input.SourceType == "prompt_derived" {
		for _, state := range input.States {
			if state.ReferenceStatus != "needs_reference" && state.ReferenceNodeID == "" && state.ReferenceVersionID == "" {
				return ErrInvalidCreativeStateInput
			}
		}
	}
	defaults := 0
	for _, state := range input.States {
		if state.IsDefault {
			defaults++
		}
	}
	if defaults > 1 {
		return ErrInvalidCreativeStateInput
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func patchString(current string, next string) string {
	if strings.TrimSpace(next) == "" {
		return current
	}
	return strings.TrimSpace(next)
}

func mergeString(current string, next string, mode string) string {
	if mode == "replace" || strings.TrimSpace(current) == "" {
		return strings.TrimSpace(next)
	}
	return patchString(current, next)
}

func patchFloat8(current pgtype.Float8, next *float64) pgtype.Float8 {
	if next == nil {
		return current
	}
	return float8(next)
}

func float8(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *value, Valid: true}
}

func patchUUID(current pgtype.UUID, next string) pgtype.UUID {
	if strings.TrimSpace(next) == "" {
		return current
	}
	return uuidOrZero(next)
}

func uuidOrZero(value string) pgtype.UUID {
	parsed, err := parseUUID(value)
	if err != nil {
		return pgtype.UUID{}
	}
	return parsed
}

func parseUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func jsonBytes(value any, empty string) []byte {
	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "null" {
		return []byte(empty)
	}
	return raw
}

func patchJSON(current []byte, next any) []byte {
	raw := jsonBytes(next, "")
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" || string(raw) == "{}" {
		return current
	}
	return raw
}

func mergeArray[T any](current []byte, next []T, mode string) []byte {
	if mode == "patch" && len(next) == 0 {
		if len(current) > 0 {
			return current
		}
		return []byte("[]")
	}
	return jsonBytes(next, "[]")
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
