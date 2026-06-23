package scheduler

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type DependencyStore interface {
	ListShotDependenciesByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.ShotDependency, error)
	ListMediaNodesByShot(ctx context.Context, params db.ListMediaNodesByShotParams) ([]db.MediaNode, error)
	ListReviewRecordsByShotPhase(ctx context.Context, params db.ListReviewRecordsByShotPhaseParams) ([]db.ReviewRecord, error)
}

type DependencyScheduler struct {
	store DependencyStore
}

type ReadinessResult struct {
	Ready          bool
	Phase          string
	BlockedReasons []BlockedReason
}

type BlockedReason struct {
	FromShotID string
	Phase      string
	Reason     string
	Code       string
}

func NewDependencyScheduler(store DependencyStore) *DependencyScheduler {
	return &DependencyScheduler{store: store}
}

func (s *DependencyScheduler) Readiness(ctx context.Context, workspaceID, shotID pgtype.UUID, phase string) (ReadinessResult, error) {
	if s == nil || s.store == nil || !workspaceID.Valid || !shotID.Valid {
		return ReadinessResult{}, fmt.Errorf("invalid dependency scheduler")
	}
	phase = normalizePhase(phase)
	result := ReadinessResult{Ready: true, Phase: phase}
	deps, err := s.store.ListShotDependenciesByWorkspace(ctx, workspaceID)
	if err != nil {
		return ReadinessResult{}, err
	}
	for _, dep := range deps {
		if dep.ToShotID != shotID || normalizePhase(dep.BlockingPhase) != phase {
			continue
		}
		ready, code, err := s.upstreamReady(ctx, workspaceID, dep.FromShotID, phase)
		if err != nil {
			return ReadinessResult{}, err
		}
		if ready {
			continue
		}
		result.Ready = false
		reason := strings.TrimSpace(dep.Reason)
		if reason == "" {
			reason = code
		}
		result.BlockedReasons = append(result.BlockedReasons, BlockedReason{
			FromShotID: uuidString(dep.FromShotID),
			Phase:      phase,
			Reason:     reason,
			Code:       code,
		})
	}
	return result, nil
}

func (s *DependencyScheduler) upstreamReady(ctx context.Context, workspaceID, upstreamShotID pgtype.UUID, phase string) (bool, string, error) {
	switch phase {
	case "preview":
		nodes, err := s.store.ListMediaNodesByShot(ctx, db.ListMediaNodesByShotParams{WorkspaceID: workspaceID, ShotID: upstreamShotID})
		if err != nil {
			return false, "", err
		}
		for _, node := range nodes {
			if node.NodeType == db.NodeTypeImage && node.CurrentVersionID.Valid {
				return true, "", nil
			}
		}
		return false, "upstream_preview_winner_missing", nil
	case "review":
		reviews, err := s.store.ListReviewRecordsByShotPhase(ctx, db.ListReviewRecordsByShotPhaseParams{
			WorkspaceID: workspaceID,
			ShotID:      upstreamShotID,
			TargetPhase: "preview_image",
		})
		if err != nil {
			return false, "", err
		}
		for _, review := range reviews {
			if review.Status == "accepted" {
				return true, "", nil
			}
		}
		return false, "upstream_review_accepted_missing", nil
	case "video", "composer":
		return false, "unsupported_phase", nil
	default:
		return false, "unknown_phase", nil
	}
}

func normalizePhase(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "preview_generation", "preview_image":
		return "preview"
	default:
		return value
	}
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}
