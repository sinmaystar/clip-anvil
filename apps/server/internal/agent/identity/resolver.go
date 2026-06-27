package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrInvalidRef = errors.New("invalid semantic ref")
	ErrNotFound   = errors.New("semantic ref not found")
)

type Store interface {
	GetAgentObjectBySemanticKey(ctx context.Context, arg db.GetAgentObjectBySemanticKeyParams) (db.AgentObjectIndex, error)
	GetArtifactVersionBySemanticKey(ctx context.Context, arg db.GetArtifactVersionBySemanticKeyParams) (db.ArtifactVersion, error)
	GetCurrentArtifactVersionByShotKeyAndKind(ctx context.Context, arg db.GetCurrentArtifactVersionByShotKeyAndKindParams) (db.ArtifactVersion, error)
	GetLatestArtifactVersionByShotKeyAndKind(ctx context.Context, arg db.GetLatestArtifactVersionByShotKeyAndKindParams) (db.ArtifactVersion, error)
	GetWinnerArtifactVersionByShotKeyAndKind(ctx context.Context, arg db.GetWinnerArtifactVersionByShotKeyAndKindParams) (db.ArtifactVersion, error)
}

type Resolver struct {
	store Store
}

func NewResolver(store Store) *Resolver {
	return &Resolver{store: store}
}

func (r *Resolver) ResolveObject(ctx context.Context, workspaceID pgtype.UUID, ref ObjectRef) (ResolvedObject, error) {
	objectType := strings.TrimSpace(ref.Type)
	key := strings.TrimSpace(ref.Key)
	if r == nil || r.store == nil || !workspaceID.Valid || objectType == "" || key == "" {
		return ResolvedObject{}, ErrInvalidRef
	}
	row, err := r.store.GetAgentObjectBySemanticKey(ctx, db.GetAgentObjectBySemanticKeyParams{
		WorkspaceID: workspaceID,
		ObjectType:  objectType,
		SemanticKey: key,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedObject{}, fmt.Errorf("%w: %s %s", ErrNotFound, objectType, key)
		}
		return ResolvedObject{}, err
	}
	return ResolvedObject{
		WorkspaceID:       row.WorkspaceID,
		ObjectType:        row.ObjectType,
		ObjectID:          row.ObjectID,
		SemanticKey:       row.SemanticKey,
		DisplayName:       row.DisplayName,
		ParentObjectType:  row.ParentObjectType,
		ParentObjectID:    row.ParentObjectID,
		ParentSemanticKey: row.ParentSemanticKey,
	}, nil
}

func (r *Resolver) ResolveArtifact(ctx context.Context, workspaceID pgtype.UUID, ref ArtifactSelectorRef) (db.ArtifactVersion, error) {
	if r == nil || r.store == nil || !workspaceID.Valid {
		return db.ArtifactVersion{}, ErrInvalidRef
	}
	if key := strings.TrimSpace(ref.Key); key != "" {
		version, err := r.store.GetArtifactVersionBySemanticKey(ctx, db.GetArtifactVersionBySemanticKeyParams{
			WorkspaceID: workspaceID,
			SemanticKey: key,
		})
		if err != nil {
			return db.ArtifactVersion{}, normalizeNotFound(err, "artifact_version", key)
		}
		return version, nil
	}
	if strings.TrimSpace(ref.Scope.Type) != ObjectShot || strings.TrimSpace(ref.Scope.Key) == "" || strings.TrimSpace(ref.ArtifactKind) == "" {
		return db.ArtifactVersion{}, ErrInvalidRef
	}
	selector := strings.TrimSpace(ref.Selector)
	if selector == "" {
		selector = "current"
	}
	params := db.GetCurrentArtifactVersionByShotKeyAndKindParams{
		WorkspaceID:  workspaceID,
		SemanticKey:  strings.TrimSpace(ref.Scope.Key),
		ArtifactKind: strings.TrimSpace(ref.ArtifactKind),
	}
	switch selector {
	case "current":
		version, err := r.store.GetCurrentArtifactVersionByShotKeyAndKind(ctx, params)
		if err != nil {
			return db.ArtifactVersion{}, normalizeNotFound(err, "artifact_selector", selector)
		}
		return version, nil
	case "latest":
		version, err := r.store.GetLatestArtifactVersionByShotKeyAndKind(ctx, db.GetLatestArtifactVersionByShotKeyAndKindParams(params))
		if err != nil {
			return db.ArtifactVersion{}, normalizeNotFound(err, "artifact_selector", selector)
		}
		return version, nil
	case "winner":
		version, err := r.store.GetWinnerArtifactVersionByShotKeyAndKind(ctx, db.GetWinnerArtifactVersionByShotKeyAndKindParams(params))
		if err != nil {
			return db.ArtifactVersion{}, normalizeNotFound(err, "artifact_selector", selector)
		}
		return version, nil
	default:
		return db.ArtifactVersion{}, ErrInvalidRef
	}
}

func normalizeNotFound(err error, objectType string, key string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s %s", ErrNotFound, objectType, key)
	}
	return err
}
