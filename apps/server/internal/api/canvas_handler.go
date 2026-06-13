package api

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type CanvasHandler struct {
	queries *db.Queries
}

func NewCanvasHandler(queries *db.Queries) *CanvasHandler {
	return &CanvasHandler{queries: queries}
}

type cameraResponse struct {
	X    float32 `json:"x"`
	Y    float32 `json:"y"`
	Zoom float32 `json:"zoom"`
}

type canvasResponse struct {
	Camera cameraResponse `json:"camera"`
	Nodes  []db.MediaNode `json:"nodes"`
}

type updateCameraRequest struct {
	X    float32 `json:"x"`
	Y    float32 `json:"y"`
	Zoom float32 `json:"zoom"`
}

func (h *CanvasHandler) GetCanvas(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	workspaceID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "workspace not found")
		return
	}

	workspace, err := h.queries.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(c, consts.StatusNotFound, "workspace not found")
			return
		}
		writeError(c, consts.StatusInternalServerError, "failed to load workspace")
		return
	}
	if workspace.OwnerID != accountID {
		writeError(c, consts.StatusForbidden, "forbidden")
		return
	}

	canvas, err := h.queries.GetCanvasDocumentByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load canvas")
		return
	}
	nodes, err := h.queries.ListMediaNodesByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load nodes")
		return
	}

	c.JSON(consts.StatusOK, canvasResponse{
		Camera: toCameraResponse(canvas),
		Nodes:  nodes,
	})
}

func (h *CanvasHandler) UpdateCamera(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	workspaceID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "workspace not found")
		return
	}

	workspace, err := h.queries.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(c, consts.StatusNotFound, "workspace not found")
			return
		}
		writeError(c, consts.StatusInternalServerError, "failed to load workspace")
		return
	}
	if workspace.OwnerID != accountID {
		writeError(c, consts.StatusForbidden, "forbidden")
		return
	}

	var req updateCameraRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if req.Zoom <= 0 {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	if _, err := h.queries.UpdateCamera(ctx, db.UpdateCameraParams{
		WorkspaceID: workspaceID,
		CameraX:     req.X,
		CameraY:     req.Y,
		CameraZoom:  req.Zoom,
	}); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to update camera")
		return
	}

	c.Status(consts.StatusNoContent)
}

func toCameraResponse(canvas db.CanvasDocument) cameraResponse {
	return cameraResponse{
		X:    canvas.CameraX,
		Y:    canvas.CameraY,
		Zoom: canvas.CameraZoom,
	}
}
