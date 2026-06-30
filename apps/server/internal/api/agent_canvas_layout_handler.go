package api

import (
	"context"
	"errors"
	"math"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type putAgentCanvasLayoutRequest struct {
	Positions []agentCanvasLayoutPositionRequest `json:"positions"`
}

type agentCanvasLayoutPositionRequest struct {
	ObjectType string  `json:"object_type"`
	ObjectID   string  `json:"object_id"`
	X          float32 `json:"x"`
	Y          float32 `json:"y"`
}

type agentCanvasLayoutPositionUpdate struct {
	ObjectType string
	ObjectID   pgtype.UUID
	X          float32
	Y          float32
}

func (h *AgentHandler) PutCanvasLayout(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}

	var req putAgentCanvasLayoutRequest
	if err := c.BindJSON(&req); err != nil || len(req.Positions) == 0 || len(req.Positions) > 200 {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	updates := make([]agentCanvasLayoutPositionUpdate, 0, len(req.Positions))
	for _, position := range req.Positions {
		update, ok := h.agentCanvasLayoutPositionUpdate(ctx, workspace.ID, position)
		if !ok {
			writeError(c, consts.StatusBadRequest, "invalid layout position")
			return
		}
		updates = append(updates, update)
	}

	for _, update := range updates {
		if _, err := h.queries.UpsertAgentCanvasLayout(ctx, db.UpsertAgentCanvasLayoutParams{
			WorkspaceID: workspace.ID,
			ObjectType:  update.ObjectType,
			ObjectID:    update.ObjectID,
			CanvasX:     update.X,
			CanvasY:     update.Y,
		}); err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to update canvas layout")
			return
		}
	}

	c.Status(consts.StatusNoContent)
}

func (h *AgentHandler) agentCanvasLayoutPositionUpdate(ctx context.Context, workspaceID pgtype.UUID, position agentCanvasLayoutPositionRequest) (agentCanvasLayoutPositionUpdate, bool) {
	if position.ObjectType == "" || position.ObjectID == "" || !finiteFloat32(position.X) || !finiteFloat32(position.Y) {
		return agentCanvasLayoutPositionUpdate{}, false
	}
	objectID, ok := uuidFromString(position.ObjectID)
	if !ok {
		return agentCanvasLayoutPositionUpdate{}, false
	}
	objectType := position.ObjectType
	if !h.agentCanvasLayoutObjectBelongsToWorkspace(ctx, workspaceID, objectType, objectID) {
		return agentCanvasLayoutPositionUpdate{}, false
	}
	return agentCanvasLayoutPositionUpdate{
		ObjectType: objectType,
		ObjectID:   objectID,
		X:          position.X,
		Y:          position.Y,
	}, true
}

func (h *AgentHandler) agentCanvasLayoutObjectBelongsToWorkspace(ctx context.Context, workspaceID pgtype.UUID, objectType string, objectID pgtype.UUID) bool {
	switch objectType {
	case agentCanvasObjectOverview:
		return objectID == workspaceID
	case agentCanvasObjectScene:
		scene, err := h.queries.GetSceneByID(ctx, objectID)
		return err == nil && scene.WorkspaceID == workspaceID
	case agentCanvasObjectShot:
		shot, err := h.queries.GetShotByID(ctx, objectID)
		return err == nil && shot.WorkspaceID == workspaceID
	case agentCanvasObjectArtifact:
		node, err := h.queries.GetMediaNodeByID(ctx, objectID)
		return err == nil && node.WorkspaceID == workspaceID
	case agentCanvasObjectFinalOutput:
		plan, err := h.queries.GetTimelinePlan(ctx, objectID)
		if err == nil {
			return plan.WorkspaceID == workspaceID
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return false
		}
		node, err := h.queries.GetMediaNodeByID(ctx, objectID)
		return err == nil && node.WorkspaceID == workspaceID
	default:
		return false
	}
}

func finiteFloat32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
