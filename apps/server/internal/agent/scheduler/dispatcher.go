package scheduler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type EventRuntime interface {
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type Dispatcher struct {
	scheduler *DependencyScheduler
	runtime   EventRuntime
}

func NewDispatcher(scheduler *DependencyScheduler, runtime EventRuntime) *Dispatcher {
	return &Dispatcher{scheduler: scheduler, runtime: runtime}
}

func (d *Dispatcher) NotifyShotUpdated(ctx context.Context, workspaceID, upstreamShotID pgtype.UUID, phase string) error {
	if d == nil || d.scheduler == nil || d.scheduler.store == nil || d.runtime == nil || !workspaceID.Valid || !upstreamShotID.Valid {
		return fmt.Errorf("invalid dependency dispatcher")
	}
	phase = normalizePhase(phase)
	deps, err := d.scheduler.store.ListShotDependenciesByWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, dep := range deps {
		if dep.FromShotID != upstreamShotID || normalizePhase(dep.BlockingPhase) != phase {
			continue
		}
		readiness, err := d.scheduler.Readiness(ctx, workspaceID, dep.ToShotID, dep.BlockingPhase)
		if err != nil {
			return err
		}
		if readiness.Ready {
			if err := d.emit(ctx, workspaceID, "shot_unblocked", upstreamShotID, dep.ToShotID, dep.BlockingPhase, readiness); err != nil {
				return err
			}
			if err := d.emit(ctx, workspaceID, "dependency_ready", upstreamShotID, dep.ToShotID, dep.BlockingPhase, readiness); err != nil {
				return err
			}
			continue
		}
		if err := d.emit(ctx, workspaceID, "shot_blocked", upstreamShotID, dep.ToShotID, dep.BlockingPhase, readiness); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) emit(ctx context.Context, workspaceID pgtype.UUID, eventType string, fromShotID, toShotID pgtype.UUID, phase string, readiness ReadinessResult) error {
	payload := map[string]any{
		"phase":           normalizePhase(phase),
		"ready":           readiness.Ready,
		"blocked_reasons": readiness.BlockedReasons,
	}
	_, err := d.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
		WorkspaceID: workspaceID,
		EventType:   eventType,
		SourceRole:  "system",
		TargetRole:  "producer",
		Scope: mustJSON(map[string]any{
			"from_shot_id": uuidString(fromShotID),
			"to_shot_id":   uuidString(toShotID),
			"phase":        normalizePhase(phase),
		}),
		Payload: mustJSON(payload),
	})
	return err
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
