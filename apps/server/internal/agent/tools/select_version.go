package tools

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	agentruntime "github.com/sinmaystar/clip-anvil/internal/agent/runtime"
	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type VersionSelectionService interface {
	SelectArtifactVersion(ctx context.Context, nodeID, versionID pgtype.UUID) (production.ArtifactSelectionResult, error)
}

type SelectVersionRuntime interface {
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type CanvasNodeBroadcaster interface {
	BroadcastAgentNodeCreated(workspaceID pgtype.UUID, node db.MediaNode)
}

type SelectVersionTool struct {
	service VersionSelectionService
	runtime SelectVersionRuntime
	canvas  CanvasNodeBroadcaster
}

func NewSelectVersionTool(service VersionSelectionService, runtime SelectVersionRuntime, canvas CanvasNodeBroadcaster) SelectVersionTool {
	return SelectVersionTool{service: service, runtime: runtime, canvas: canvas}
}

func (t SelectVersionTool) Definition() Definition {
	return Definition{
		Name:        "select_version",
		Description: "Select a succeeded artifact version as the current winner for an Agent-owned production node. This reuses production version selection and stale propagation.",
		Parameters: objectSchema(map[string]any{
			"node_id":      map[string]any{"type": "string"},
			"version_id":   map[string]any{"type": "string"},
			"reason":       map[string]any{"type": "string"},
			"target_phase": map[string]any{"type": "string", "enum": []string{"preview_image"}},
		}),
		Result:     map[string]any{"type": "object"},
		Safety:     SafetySpec{UsesProductionService: true, MaxCallsPerTurn: 10},
		Visibility: VisibilitySpec{ShowCallMessage: true, ShowResultMessage: true, UserLabel: "选择版本"},
	}
}

func (t SelectVersionTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	if t.service == nil {
		return ExecuteOutput{}, errors.New("select_version service is not configured")
	}
	nodeID, ok := pgUUIDFromString(stringValue(input.Arguments, "node_id"))
	if !ok {
		return ExecuteOutput{}, errors.New("select_version requires node_id")
	}
	versionID, ok := pgUUIDFromString(stringValue(input.Arguments, "version_id"))
	if !ok {
		return ExecuteOutput{}, errors.New("select_version requires version_id")
	}
	result, err := t.service.SelectArtifactVersion(ctx, nodeID, versionID)
	if err != nil {
		return ExecuteOutput{}, err
	}
	if t.runtime != nil {
		_, _ = t.runtime.CreateEvent(ctx, agentruntime.CreateEventParams{
			WorkspaceID: input.WorkspaceID,
			ThreadID:    input.ThreadID,
			TaskID:      input.TaskID,
			EventType:   "version_selected",
			SourceRole:  "producer",
			Scope:       mustJSON(map[string]any{"node_id": uuidString(nodeID), "version_id": uuidString(versionID)}),
			Payload:     mustJSON(map[string]any{"reason": stringValue(input.Arguments, "reason"), "target_phase": stringValue(input.Arguments, "target_phase")}),
		})
	}
	summary := "已选择该版本作为当前结果。"
	return ExecuteOutput{Summary: summary, Result: map[string]any{"status": "succeeded", "summary": summary, "node_id": uuidString(result.Node.ID), "version_id": uuidString(result.Version.ID)}}, nil
}
