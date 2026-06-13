package api

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type NodeHandler struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewNodeHandler(pool *pgxpool.Pool, queries *db.Queries) *NodeHandler {
	return &NodeHandler{pool: pool, queries: queries}
}

type createNodeRequest struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	NodeType    string  `json:"node_type"`
	Title       string  `json:"title"`
	Prompt      string  `json:"prompt"`
	Status      string  `json:"status"`
	CanvasX     float32 `json:"canvas_x"`
	CanvasY     float32 `json:"canvas_y"`
}

func (r createNodeRequest) nodeID() (pgtype.UUID, bool) {
	if strings.TrimSpace(r.ID) == "" {
		return pgtype.UUID{}, false
	}
	return uuidFromString(r.ID)
}

func (r createNodeRequest) nodeStatus() (db.NodeStatus, bool) {
	if strings.TrimSpace(r.Status) == "" {
		return db.NodeStatusDraft, true
	}
	status := db.NodeStatus(r.Status)
	return status, isKnownNodeStatus(status)
}

type updateNodeRequest struct {
	Title  *string `json:"title"`
	Prompt *string `json:"prompt"`
	Status *string `json:"status"`
}

func (r updateNodeRequest) hasChanges() bool {
	return r.Title != nil || r.Prompt != nil || r.Status != nil
}

type positionRequest struct {
	ID      string  `json:"id"`
	CanvasX float32 `json:"canvas_x"`
	CanvasY float32 `json:"canvas_y"`
}

type batchPositionRequest struct {
	Positions []positionRequest `json:"positions"`
}

func (h *NodeHandler) Create(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	var req createNodeRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	workspaceID, ok := uuidFromString(req.WorkspaceID)
	if !ok {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	nodeType := db.MediaType(req.NodeType)
	if !isAllowedM1NodeType(nodeType) {
		writeError(c, consts.StatusBadRequest, "invalid node type")
		return
	}
	if !h.workspaceBelongsToAccount(ctx, workspaceID, accountID, c) {
		return
	}

	status, ok := req.nodeStatus()
	if !ok {
		writeError(c, consts.StatusBadRequest, "invalid status")
		return
	}

	w, nodeH := defaultNodeSize(nodeType)
	title := strings.TrimSpace(req.Title)
	nodeID, hasNodeID := req.nodeID()
	if strings.TrimSpace(req.ID) != "" && !hasNodeID {
		writeError(c, consts.StatusBadRequest, "invalid node id")
		return
	}
	var node db.MediaNode
	var err error
	if hasNodeID {
		node, err = h.queries.CreateMediaNodeWithID(ctx, db.CreateMediaNodeWithIDParams{
			ID:          nodeID,
			WorkspaceID: workspaceID,
			NodeType:    nodeType,
			Title:       title,
			Prompt:      req.Prompt,
			Status:      status,
			CanvasX:     req.CanvasX,
			CanvasY:     req.CanvasY,
			CanvasW:     w,
			CanvasH:     nodeH,
		})
	} else {
		node, err = h.queries.CreateMediaNode(ctx, db.CreateMediaNodeParams{
			WorkspaceID: workspaceID,
			NodeType:    nodeType,
			Title:       title,
			Prompt:      req.Prompt,
			CanvasX:     req.CanvasX,
			CanvasY:     req.CanvasY,
			CanvasW:     w,
			CanvasH:     nodeH,
		})
	}
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create node")
		return
	}

	c.JSON(consts.StatusOK, node)
}

func (h *NodeHandler) Get(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	node, ok := h.nodeForAccount(ctx, c.Param("id"), accountID, c)
	if !ok {
		return
	}

	c.JSON(consts.StatusOK, node)
}

func (h *NodeHandler) Update(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	node, ok := h.nodeForAccount(ctx, c.Param("id"), accountID, c)
	if !ok {
		return
	}

	var req updateNodeRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if !req.hasChanges() {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	var err error
	if req.Title != nil {
		node, err = h.queries.UpdateMediaNodeTitle(ctx, db.UpdateMediaNodeTitleParams{
			ID:    node.ID,
			Title: strings.TrimSpace(*req.Title),
		})
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to update node")
			return
		}
	}
	if req.Prompt != nil {
		node, err = h.queries.UpdateMediaNodePrompt(ctx, db.UpdateMediaNodePromptParams{
			ID:     node.ID,
			Prompt: *req.Prompt,
		})
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to update node")
			return
		}
	}
	if req.Status != nil {
		status := db.NodeStatus(*req.Status)
		if !isKnownNodeStatus(status) {
			writeError(c, consts.StatusBadRequest, "invalid status")
			return
		}
		node, err = h.queries.UpdateMediaNodeStatus(ctx, db.UpdateMediaNodeStatusParams{
			ID:     node.ID,
			Status: status,
		})
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to update node")
			return
		}
	}

	c.JSON(consts.StatusOK, node)
}

func (h *NodeHandler) Delete(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	node, ok := h.nodeForAccount(ctx, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	if err := h.queries.DeleteMediaNode(ctx, node.ID); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to delete node")
		return
	}

	c.Status(consts.StatusNoContent)
}

func (h *NodeHandler) BatchUpdatePosition(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	var req batchPositionRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if len(req.Positions) == 0 {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	positions := make([]struct {
		id pgtype.UUID
		x  float32
		y  float32
	}, 0, len(req.Positions))

	for _, position := range req.Positions {
		node, ok := h.nodeForAccount(ctx, position.ID, accountID, c)
		if !ok {
			return
		}
		positions = append(positions, struct {
			id pgtype.UUID
			x  float32
			y  float32
		}{id: node.ID, x: position.CanvasX, y: position.CanvasY})
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to update positions")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := h.queries.WithTx(tx)
	for _, position := range positions {
		if _, err := qtx.UpdateMediaNodePosition(ctx, db.UpdateMediaNodePositionParams{
			ID:      position.id,
			CanvasX: position.x,
			CanvasY: position.y,
		}); err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to update positions")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to update positions")
		return
	}

	c.Status(consts.StatusNoContent)
}

func (h *NodeHandler) nodeForAccount(ctx context.Context, id string, accountID pgtype.UUID, c *app.RequestContext) (db.MediaNode, bool) {
	nodeID, ok := uuidFromString(id)
	if !ok {
		writeError(c, consts.StatusNotFound, "node not found")
		return db.MediaNode{}, false
	}

	node, err := h.queries.GetMediaNodeByID(ctx, nodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(c, consts.StatusNotFound, "node not found")
			return db.MediaNode{}, false
		}
		writeError(c, consts.StatusInternalServerError, "failed to load node")
		return db.MediaNode{}, false
	}

	if !h.workspaceBelongsToAccount(ctx, node.WorkspaceID, accountID, c) {
		return db.MediaNode{}, false
	}
	return node, true
}

func (h *NodeHandler) workspaceBelongsToAccount(ctx context.Context, workspaceID pgtype.UUID, accountID pgtype.UUID, c *app.RequestContext) bool {
	workspace, err := h.queries.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(c, consts.StatusNotFound, "workspace not found")
			return false
		}
		writeError(c, consts.StatusInternalServerError, "failed to load workspace")
		return false
	}
	if workspace.OwnerID != accountID {
		writeError(c, consts.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func defaultNodeSize(nodeType db.MediaType) (float32, float32) {
	switch nodeType {
	case db.MediaTypeText:
		return 200, 120
	default:
		return 200, 120
	}
}

func isAllowedM1NodeType(nodeType db.MediaType) bool {
	return nodeType == db.MediaTypeText
}

func isKnownNodeStatus(status db.NodeStatus) bool {
	switch status {
	case db.NodeStatusDraft,
		db.NodeStatusReady,
		db.NodeStatusQueued,
		db.NodeStatusRunning,
		db.NodeStatusSucceeded,
		db.NodeStatusFailed,
		db.NodeStatusStale,
		db.NodeStatusUserEditing:
		return true
	default:
		return false
	}
}
