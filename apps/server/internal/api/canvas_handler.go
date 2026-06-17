package api

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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
	Camera cameraResponse        `json:"camera"`
	Nodes  []canvasNodeResponse  `json:"nodes"`
	Edges  []db.MediaEdge        `json:"edges"`
	Groups []canvasGroupResponse `json:"groups"`
}

type canvasNodeResponse struct {
	db.MediaNode
	ThumbnailURL *string `json:"thumbnail_url,omitempty"`
}

type canvasGroupResponse struct {
	ID          pgtype.UUID   `json:"id"`
	WorkspaceID pgtype.UUID   `json:"workspace_id"`
	Name        string        `json:"name"`
	SortOrder   int32         `json:"sort_order"`
	NodeIDs     []pgtype.UUID `json:"node_ids"`
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
	edges, err := h.queries.ListMediaEdgesByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load edges")
		return
	}
	groups, err := h.queries.ListMediaGroupsByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load groups")
		return
	}
	assets, err := h.queries.ListMediaAssetsByWorkspace(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to load assets")
		return
	}
	assetsByID := make(map[pgtype.UUID]db.MediaAsset, len(assets))
	for _, asset := range assets {
		assetsByID[asset.ID] = asset
	}

	c.JSON(consts.StatusOK, canvasResponse{
		Camera: toCameraResponse(canvas),
		Nodes:  toCanvasNodeResponses(nodes, assetsByID),
		Edges:  edges,
		Groups: toCanvasGroupResponses(groups, nodes),
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

func toCanvasNodeResponses(nodes []db.MediaNode, assets map[pgtype.UUID]db.MediaAsset) []canvasNodeResponse {
	responses := make([]canvasNodeResponse, 0, len(nodes))
	for _, node := range nodes {
		response := canvasNodeResponse{MediaNode: node}
		if node.AssetID.Valid {
			if asset, ok := assets[node.AssetID]; ok && asset.ThumbnailUrl.Valid {
				response.ThumbnailURL = &asset.ThumbnailUrl.String
			}
		}
		responses = append(responses, response)
	}
	return responses
}

func toCanvasGroupResponses(groups []db.MediaGroup, nodes []db.MediaNode) []canvasGroupResponse {
	nodeIDsByGroup := make(map[pgtype.UUID][]pgtype.UUID, len(groups))
	for _, node := range nodes {
		if node.GroupID.Valid {
			nodeIDsByGroup[node.GroupID] = append(nodeIDsByGroup[node.GroupID], node.ID)
		}
	}

	responses := make([]canvasGroupResponse, 0, len(groups))
	for _, group := range groups {
		responses = append(responses, canvasGroupResponse{
			ID:          group.ID,
			WorkspaceID: group.WorkspaceID,
			Name:        group.Name,
			SortOrder:   group.SortOrder,
			NodeIDs:     nodeIDsByGroup[group.ID],
		})
	}
	return responses
}
