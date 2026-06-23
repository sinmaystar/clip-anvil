package storyboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrAgentWorkspaceRequired = errors.New("agent workspace required")
	ErrInvalidStoryboardInput = errors.New("invalid storyboard input")
	ErrShotReferenceNotFound  = errors.New("shot reference not found")
)

type txBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Store interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	ListActiveShotsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Shot, error)
	CreateShot(ctx context.Context, params db.CreateShotParams) (db.Shot, error)
	UpdateShot(ctx context.Context, params db.UpdateShotParams) (db.Shot, error)
	ArchiveShot(ctx context.Context, params db.ArchiveShotParams) (db.Shot, error)
	DeleteShotDependenciesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) error
	DeleteShotDependenciesForShot(ctx context.Context, params db.DeleteShotDependenciesForShotParams) error
	CreateShotDependency(ctx context.Context, params db.CreateShotDependencyParams) (db.ShotDependency, error)
	UpdateMediaNodeShot(ctx context.Context, params db.UpdateMediaNodeShotParams) (db.MediaNode, error)
}

type Service struct {
	pool   txBeginner
	store  Store
	withTx func(pgx.Tx) Store
}

func NewService(pool txBeginner, store Store) *Service {
	service := &Service{pool: pool, store: store}
	if queries, ok := store.(*db.Queries); ok {
		service.withTx = func(tx pgx.Tx) Store { return queries.WithTx(tx) }
	}
	return service
}

type UpdateInput struct {
	WorkspaceID  pgtype.UUID
	Intent       string
	Shots        []ShotInput
	Dependencies []DependencyInput
	Summary      string
}

type ShotInput struct {
	ID               string
	ClientKey        string
	SortOrder        int32
	Title            string
	Brief            map[string]any
	DurationSec      *float64
	NarrativePurpose string
	Status           string
	LinkedNodeIDs    []string
}

type DependencyInput struct {
	From             string
	To               string
	DependencyType   string
	RequiredArtifact string
	InjectionRole    string
	BlockingPhase    string
	StalePolicy      string
	Reason           string
}

type UpdateOutput struct {
	ShotsCreated        int
	ShotsUpdated        int
	ShotsArchived       int
	DependenciesCreated int
	Shots               []db.Shot
	Dependencies        []db.ShotDependency
}

func (s *Service) UpdateStoryboard(ctx context.Context, input UpdateInput) (UpdateOutput, error) {
	if s == nil || s.store == nil || !input.WorkspaceID.Valid {
		return UpdateOutput{}, ErrInvalidStoryboardInput
	}
	intent := strings.TrimSpace(input.Intent)
	if intent == "" {
		intent = "upsert"
	}
	if !validIntent(intent) {
		return UpdateOutput{}, ErrInvalidStoryboardInput
	}
	if intent == "replace" && len(input.Shots) == 0 && strings.TrimSpace(input.Summary) == "" {
		return UpdateOutput{}, ErrInvalidStoryboardInput
	}

	if s.pool == nil || s.withTx == nil {
		return s.update(ctx, s.store, input, intent)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return UpdateOutput{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	out, err := s.update(ctx, s.withTx(tx), input, intent)
	if err != nil {
		return UpdateOutput{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return UpdateOutput{}, err
	}
	committed = true
	return out, nil
}

func (s *Service) update(ctx context.Context, store Store, input UpdateInput, intent string) (UpdateOutput, error) {
	workspace, err := store.GetWorkspaceByID(ctx, input.WorkspaceID)
	if err != nil {
		return UpdateOutput{}, err
	}
	if workspace.Mode != db.WorkspaceModeAgent {
		return UpdateOutput{}, ErrAgentWorkspaceRequired
	}
	existing, err := store.ListActiveShotsByWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return UpdateOutput{}, err
	}

	byID := map[pgtype.UUID]db.Shot{}
	byKey := map[string]db.Shot{}
	for _, shot := range existing {
		byID[shot.ID] = shot
		if strings.TrimSpace(shot.ClientKey) != "" {
			byKey[shot.ClientKey] = shot
		}
	}

	out := UpdateOutput{}
	kept := map[pgtype.UUID]bool{}
	for i, shotInput := range input.Shots {
		if shotInput.SortOrder == 0 {
			shotInput.SortOrder = int32(i + 1)
		}
		shot, created, err := upsertShot(ctx, store, input.WorkspaceID, byID, byKey, shotInput)
		if err != nil {
			return UpdateOutput{}, err
		}
		if created {
			out.ShotsCreated++
		} else {
			out.ShotsUpdated++
		}
		out.Shots = append(out.Shots, shot)
		kept[shot.ID] = true
		byID[shot.ID] = shot
		if shot.ClientKey != "" {
			byKey[shot.ClientKey] = shot
		}
	}

	if intent == "replace" {
		for _, shot := range existing {
			if kept[shot.ID] {
				continue
			}
			if _, err := store.ArchiveShot(ctx, db.ArchiveShotParams{ID: shot.ID, WorkspaceID: input.WorkspaceID}); err != nil {
				return UpdateOutput{}, err
			}
			out.ShotsArchived++
		}
		if err := store.DeleteShotDependenciesByWorkspace(ctx, input.WorkspaceID); err != nil {
			return UpdateOutput{}, err
		}
	}

	if intent == "archive" {
		for _, shot := range out.Shots {
			if err := store.DeleteShotDependenciesForShot(ctx, db.DeleteShotDependenciesForShotParams{
				WorkspaceID: input.WorkspaceID,
				FromShotID:  shot.ID,
			}); err != nil {
				return UpdateOutput{}, err
			}
			if _, err := store.ArchiveShot(ctx, db.ArchiveShotParams{ID: shot.ID, WorkspaceID: input.WorkspaceID}); err != nil {
				return UpdateOutput{}, err
			}
			out.ShotsArchived++
		}
		out.Shots = nil
	}

	deps, err := resolveDependencyInputs(input.Dependencies, byID, byKey)
	if err != nil {
		return UpdateOutput{}, err
	}
	for _, dep := range deps {
		created, err := store.CreateShotDependency(ctx, db.CreateShotDependencyParams{
			WorkspaceID:      input.WorkspaceID,
			FromShotID:       dep.FromShotID,
			ToShotID:         dep.ToShotID,
			DependencyType:   dep.DependencyType,
			RequiredArtifact: dep.RequiredArtifact,
			InjectionRole:    dep.InjectionRole,
			BlockingPhase:    dep.BlockingPhase,
			StalePolicy:      dep.StalePolicy,
			Reason:           dep.Reason,
		})
		if err != nil {
			return UpdateOutput{}, err
		}
		out.Dependencies = append(out.Dependencies, created)
		out.DependenciesCreated++
	}

	sort.Slice(out.Shots, func(i, j int) bool { return out.Shots[i].SortOrder < out.Shots[j].SortOrder })
	return out, nil
}

func upsertShot(ctx context.Context, store Store, workspaceID pgtype.UUID, byID map[pgtype.UUID]db.Shot, byKey map[string]db.Shot, input ShotInput) (db.Shot, bool, error) {
	title := strings.TrimSpace(input.Title)
	clientKey := strings.TrimSpace(input.ClientKey)
	if title == "" || !validClientKey(clientKey) {
		return db.Shot{}, false, ErrInvalidStoryboardInput
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "planned"
	}
	if !validStatus(status) {
		return db.Shot{}, false, ErrInvalidStoryboardInput
	}
	brief, err := json.Marshal(input.Brief)
	if err != nil {
		return db.Shot{}, false, ErrInvalidStoryboardInput
	}
	if len(brief) == 0 || string(brief) == "null" {
		brief = []byte("{}")
	}
	duration := pgtype.Float8{}
	if input.DurationSec != nil {
		duration = pgtype.Float8{Float64: *input.DurationSec, Valid: true}
	}

	var existing db.Shot
	var found bool
	if id, ok := pgUUIDFromString(input.ID); ok {
		existing, found = byID[id]
	}
	if !found && clientKey != "" {
		existing, found = byKey[clientKey]
	}
	if found {
		shot, err := store.UpdateShot(ctx, db.UpdateShotParams{
			ID:               existing.ID,
			ClientKey:        clientKey,
			SortOrder:        input.SortOrder,
			Title:            title,
			Brief:            brief,
			DurationSec:      duration,
			NarrativePurpose: strings.TrimSpace(input.NarrativePurpose),
			Status:           status,
			WorkspaceID:      workspaceID,
		})
		if err != nil {
			return db.Shot{}, false, err
		}
		if err := linkNodes(ctx, store, workspaceID, shot.ID, input.LinkedNodeIDs); err != nil {
			return db.Shot{}, false, err
		}
		return shot, false, nil
	}
	shot, err := store.CreateShot(ctx, db.CreateShotParams{
		WorkspaceID:      workspaceID,
		ClientKey:        clientKey,
		SortOrder:        input.SortOrder,
		Title:            title,
		Brief:            brief,
		DurationSec:      duration,
		NarrativePurpose: strings.TrimSpace(input.NarrativePurpose),
		Status:           status,
	})
	if err != nil {
		return db.Shot{}, false, err
	}
	if err := linkNodes(ctx, store, workspaceID, shot.ID, input.LinkedNodeIDs); err != nil {
		return db.Shot{}, false, err
	}
	return shot, true, nil
}

type resolvedDependency struct {
	FromShotID       pgtype.UUID
	ToShotID         pgtype.UUID
	DependencyType   string
	RequiredArtifact string
	InjectionRole    string
	BlockingPhase    string
	StalePolicy      string
	Reason           string
}

func resolveDependencyInputs(inputs []DependencyInput, byID map[pgtype.UUID]db.Shot, byKey map[string]db.Shot) ([]resolvedDependency, error) {
	out := make([]resolvedDependency, 0, len(inputs))
	for _, input := range inputs {
		from, ok := resolveShotRef(input.From, byID, byKey)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrShotReferenceNotFound, input.From)
		}
		to, ok := resolveShotRef(input.To, byID, byKey)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrShotReferenceNotFound, input.To)
		}
		dependencyType := strings.TrimSpace(input.DependencyType)
		blockingPhase := strings.TrimSpace(input.BlockingPhase)
		if from.ID == to.ID || !validDependencyType(dependencyType) || !validBlockingPhase(blockingPhase) {
			return nil, ErrInvalidStoryboardInput
		}
		stalePolicy := strings.TrimSpace(input.StalePolicy)
		if stalePolicy == "" {
			stalePolicy = "mark_downstream_stale"
		}
		out = append(out, resolvedDependency{
			FromShotID:       from.ID,
			ToShotID:         to.ID,
			DependencyType:   dependencyType,
			RequiredArtifact: strings.TrimSpace(input.RequiredArtifact),
			InjectionRole:    strings.TrimSpace(input.InjectionRole),
			BlockingPhase:    blockingPhase,
			StalePolicy:      stalePolicy,
			Reason:           strings.TrimSpace(input.Reason),
		})
	}
	return out, nil
}

func resolveShotRef(ref string, byID map[pgtype.UUID]db.Shot, byKey map[string]db.Shot) (db.Shot, bool) {
	ref = strings.TrimSpace(ref)
	if id, ok := pgUUIDFromString(ref); ok {
		shot, found := byID[id]
		return shot, found
	}
	shot, found := byKey[ref]
	return shot, found
}

func linkNodes(ctx context.Context, store Store, workspaceID pgtype.UUID, shotID pgtype.UUID, nodeIDs []string) error {
	for _, rawID := range nodeIDs {
		nodeID, ok := pgUUIDFromString(rawID)
		if !ok {
			return ErrInvalidStoryboardInput
		}
		if _, err := store.UpdateMediaNodeShot(ctx, db.UpdateMediaNodeShotParams{
			ID:          nodeID,
			ShotID:      shotID,
			WorkspaceID: workspaceID,
		}); err != nil {
			return err
		}
	}
	return nil
}

var clientKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func validClientKey(value string) bool {
	return clientKeyPattern.MatchString(strings.TrimSpace(value))
}

func validIntent(value string) bool {
	switch value {
	case "replace", "upsert", "patch", "archive":
		return true
	default:
		return false
	}
}

func validStatus(value string) bool {
	switch value {
	case "planned", "draft", "waiting_for_user", "approved", "preview_running", "preview_ready", "video_running", "video_ready", "review_running", "approved_output", "failed", "archived":
		return true
	default:
		return false
	}
}

func validDependencyType(value string) bool {
	switch value {
	case "story_order", "last_frame_continuity", "same_subject_consistency", "visual_reference", "asset_reuse", "narrative_dependency":
		return true
	default:
		return false
	}
}

func validBlockingPhase(value string) bool {
	switch value {
	case "", "preview_generation", "video_generation", "review", "composition":
		return true
	default:
		return false
	}
}

func pgUUIDFromString(value string) (pgtype.UUID, bool) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}
