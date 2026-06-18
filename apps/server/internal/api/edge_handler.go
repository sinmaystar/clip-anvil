package api

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type EdgeHandler struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	hub     *CanvasHub
}

func NewEdgeHandler(pool *pgxpool.Pool, queries *db.Queries, hub ...*CanvasHub) *EdgeHandler {
	handler := &EdgeHandler{pool: pool, queries: queries}
	if len(hub) > 0 {
		handler.hub = hub[0]
	}
	return handler
}

type createEdgeRequest struct {
	WorkspaceID string `json:"workspace_id"`
	FromNodeID  string `json:"from_node_id"`
	ToNodeID    string `json:"to_node_id"`
}

type edgeCreationResult struct {
	edge    db.MediaEdge
	status  int
	message string
}

type dependencyEdgeLister interface {
	ListOutgoingDependencyEdges(context.Context, pgtype.UUID) ([]db.MediaEdge, error)
}

func (h *EdgeHandler) Create(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	var req createEdgeRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	workspaceID, ok := uuidFromString(req.WorkspaceID)
	if !ok {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	fromNodeID, ok := uuidFromString(req.FromNodeID)
	if !ok {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	toNodeID, ok := uuidFromString(req.ToNodeID)
	if !ok {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if fromNodeID == toNodeID {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	result := h.createDependencyEdge(ctx, accountID, workspaceID, fromNodeID, toNodeID)
	if result.status != consts.StatusOK {
		writeError(c, result.status, result.message)
		return
	}
	edgeResponse := toMediaEdgeResponse(result.edge)
	h.broadcast(result.edge.WorkspaceID, "EdgeCreated", map[string]any{"edge": edgeResponse})
	c.JSON(consts.StatusOK, edgeResponse)
}

func (h *EdgeHandler) Delete(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	edgeID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "edge not found")
		return
	}

	edge, err := h.queries.GetMediaEdgeByID(ctx, edgeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(c, consts.StatusNotFound, "edge not found")
			return
		}
		writeError(c, consts.StatusInternalServerError, "failed to load edge")
		return
	}
	if _, ok := requireStudioWorkspace(ctx, h.queries, edge.WorkspaceID, accountID, c); !ok {
		return
	}
	if err := h.queries.DeleteMediaEdge(ctx, edge.ID); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to delete edge")
		return
	}
	h.broadcast(edge.WorkspaceID, "EdgeDeleted", map[string]any{"edge_id": edge.ID})

	c.Status(consts.StatusNoContent)
}

func (h *EdgeHandler) broadcast(workspaceID pgtype.UUID, eventType string, payload any) {
	if h.hub == nil {
		return
	}
	h.hub.Broadcast(workspaceID, CanvasEvent{Type: eventType, Payload: payload})
}

func (h *EdgeHandler) createDependencyEdge(
	ctx context.Context,
	accountID pgtype.UUID,
	workspaceID pgtype.UUID,
	fromNodeID pgtype.UUID,
	toNodeID pgtype.UUID,
) edgeCreationResult {
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return edgeCreationResult{status: consts.StatusInternalServerError, message: "failed to create edge"}
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := h.queries.WithTx(tx)
	status, message := h.validateEdgeEndpoints(ctx, qtx, accountID, workspaceID, fromNodeID, toNodeID)
	if status != consts.StatusOK {
		return edgeCreationResult{status: status, message: message}
	}

	if _, err := qtx.GetDependencyEdgeByEndpoints(ctx, db.GetDependencyEdgeByEndpointsParams{
		FromNodeID: fromNodeID,
		ToNodeID:   toNodeID,
	}); err == nil {
		return edgeCreationResult{status: consts.StatusConflict, message: "edge already exists"}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return edgeCreationResult{status: consts.StatusInternalServerError, message: "failed to create edge"}
	}

	hasCycle, err := wouldCreateCycle(ctx, qtx, fromNodeID, toNodeID)
	if err != nil {
		return edgeCreationResult{status: consts.StatusInternalServerError, message: "failed to validate edge"}
	}
	if hasCycle {
		return edgeCreationResult{status: consts.StatusUnprocessableEntity, message: "edge creates cycle"}
	}

	edge, err := qtx.CreateMediaEdge(ctx, db.CreateMediaEdgeParams{
		WorkspaceID: workspaceID,
		FromNodeID:  fromNodeID,
		ToNodeID:    toNodeID,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return edgeCreationResult{status: consts.StatusConflict, message: "edge already exists"}
		}
		return edgeCreationResult{status: consts.StatusInternalServerError, message: "failed to create edge"}
	}
	if err := tx.Commit(ctx); err != nil {
		return edgeCreationResult{status: consts.StatusInternalServerError, message: "failed to create edge"}
	}

	return edgeCreationResult{edge: edge, status: consts.StatusOK}
}

func (h *EdgeHandler) validateEdgeEndpoints(
	ctx context.Context,
	q *db.Queries,
	accountID pgtype.UUID,
	workspaceID pgtype.UUID,
	fromNodeID pgtype.UUID,
	toNodeID pgtype.UUID,
) (int, string) {
	workspace, err := q.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return consts.StatusNotFound, "workspace not found"
		}
		return consts.StatusInternalServerError, "failed to load workspace"
	}
	if workspace.OwnerID != accountID {
		return consts.StatusForbidden, "forbidden"
	}
	if !isStudioWorkspaceMode(workspace.Mode) {
		return consts.StatusForbidden, "workspace is read-only in agent mode"
	}

	fromNode, err := q.GetMediaNodeByID(ctx, fromNodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return consts.StatusBadRequest, "invalid request"
		}
		return consts.StatusInternalServerError, "failed to load node"
	}
	toNode, err := q.GetMediaNodeByID(ctx, toNodeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return consts.StatusBadRequest, "invalid request"
		}
		return consts.StatusInternalServerError, "failed to load node"
	}
	if fromNode.WorkspaceID != workspaceID || toNode.WorkspaceID != workspaceID {
		return consts.StatusBadRequest, "invalid request"
	}

	return consts.StatusOK, ""
}

func wouldCreateCycle(ctx context.Context, q dependencyEdgeLister, fromNodeID pgtype.UUID, toNodeID pgtype.UUID) (bool, error) {
	visited := map[pgtype.UUID]bool{}
	queue := []pgtype.UUID{toNodeID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == fromNodeID {
			return true, nil
		}
		if visited[current] {
			continue
		}
		visited[current] = true

		edges, err := q.ListOutgoingDependencyEdges(ctx, current)
		if err != nil {
			return false, err
		}
		for _, edge := range edges {
			queue = append(queue, edge.ToNodeID)
		}
	}

	return false, nil
}
