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

type GroupHandler struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	hub     *CanvasHub
}

func NewGroupHandler(pool *pgxpool.Pool, queries *db.Queries, hub ...*CanvasHub) *GroupHandler {
	handler := &GroupHandler{pool: pool, queries: queries}
	if len(hub) > 0 {
		handler.hub = hub[0]
	}
	return handler
}

type createGroupRequest struct {
	WorkspaceID string   `json:"workspace_id"`
	Name        string   `json:"name"`
	NodeIDs     []string `json:"node_ids"`
}

type updateGroupRequest struct {
	Name      *string `json:"name"`
	SortOrder *int32  `json:"sort_order"`
}

type replaceGroupNodesRequest struct {
	NodeIDs []string `json:"node_ids"`
}

type groupResponse struct {
	Group   db.MediaGroup `json:"group"`
	NodeIDs []pgtype.UUID `json:"node_ids"`
}

func (h *GroupHandler) Create(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	var req createGroupRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	workspaceID, ok := uuidFromString(req.WorkspaceID)
	if !ok || strings.TrimSpace(req.Name) == "" {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if !h.workspaceBelongsToAccount(ctx, workspaceID, accountID, c) {
		return
	}

	group, nodeIDs, status, message := h.createGroupWithNodes(ctx, workspaceID, strings.TrimSpace(req.Name), req.NodeIDs)
	if status != consts.StatusOK {
		writeError(c, status, message)
		return
	}
	h.broadcast(workspaceID, "GroupCreated", map[string]any{"group": group})
	c.JSON(consts.StatusOK, groupResponse{Group: group, NodeIDs: nodeIDs})
}

func (h *GroupHandler) Update(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	group, ok := h.groupForAccount(ctx, c.Param("id"), accountID, c)
	if !ok {
		return
	}

	var req updateGroupRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if req.Name == nil && req.SortOrder == nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	var err error
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(c, consts.StatusBadRequest, "invalid request")
			return
		}
		group, err = h.queries.UpdateMediaGroupName(ctx, db.UpdateMediaGroupNameParams{ID: group.ID, Name: name})
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to update group")
			return
		}
	}
	if req.SortOrder != nil {
		group, err = h.queries.UpdateMediaGroupSortOrder(ctx, db.UpdateMediaGroupSortOrderParams{
			ID:        group.ID,
			SortOrder: *req.SortOrder,
		})
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to update group")
			return
		}
	}
	h.broadcast(group.WorkspaceID, "GroupUpdated", map[string]any{"group": group})

	c.JSON(consts.StatusOK, group)
}

func (h *GroupHandler) Delete(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	group, ok := h.groupForAccount(ctx, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	if err := h.queries.DeleteMediaGroup(ctx, group.ID); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to delete group")
		return
	}
	h.broadcast(group.WorkspaceID, "GroupDeleted", map[string]any{"group_id": group.ID})

	c.Status(consts.StatusNoContent)
}

func (h *GroupHandler) ReplaceNodes(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	group, ok := h.groupForAccount(ctx, c.Param("id"), accountID, c)
	if !ok {
		return
	}

	var req replaceGroupNodesRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	nodeIDs, status, message := h.replaceGroupNodes(ctx, group, req.NodeIDs)
	if status != consts.StatusOK {
		writeError(c, status, message)
		return
	}
	h.broadcast(group.WorkspaceID, "GroupUpdated", map[string]any{"group": group})

	c.JSON(consts.StatusOK, groupResponse{Group: group, NodeIDs: nodeIDs})
}

func (h *GroupHandler) broadcast(workspaceID pgtype.UUID, eventType string, payload any) {
	if h.hub == nil {
		return
	}
	h.hub.Broadcast(workspaceID, CanvasEvent{Type: eventType, Payload: payload})
}

func (h *GroupHandler) createGroupWithNodes(
	ctx context.Context,
	workspaceID pgtype.UUID,
	name string,
	nodeIDStrings []string,
) (db.MediaGroup, []pgtype.UUID, int, string) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return db.MediaGroup{}, nil, consts.StatusInternalServerError, "failed to create group"
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := h.queries.WithTx(tx)

	group, err := qtx.CreateMediaGroup(ctx, db.CreateMediaGroupParams{
		WorkspaceID: workspaceID,
		Name:        name,
		SortOrder:   0,
	})
	if err != nil {
		return db.MediaGroup{}, nil, consts.StatusInternalServerError, "failed to create group"
	}
	nodeIDs, status, message := assignNodesToGroup(ctx, qtx, workspaceID, group.ID, nodeIDStrings)
	if status != consts.StatusOK {
		return db.MediaGroup{}, nil, status, message
	}
	if err := tx.Commit(ctx); err != nil {
		return db.MediaGroup{}, nil, consts.StatusInternalServerError, "failed to create group"
	}
	return group, nodeIDs, consts.StatusOK, ""
}

func (h *GroupHandler) replaceGroupNodes(
	ctx context.Context,
	group db.MediaGroup,
	nodeIDStrings []string,
) ([]pgtype.UUID, int, string) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return nil, consts.StatusInternalServerError, "failed to update group"
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := h.queries.WithTx(tx)

	members, err := qtx.ListMediaNodesByGroup(ctx, group.ID)
	if err != nil {
		return nil, consts.StatusInternalServerError, "failed to update group"
	}
	for _, member := range members {
		if _, err := qtx.ClearMediaNodeGroup(ctx, member.ID); err != nil {
			return nil, consts.StatusInternalServerError, "failed to update group"
		}
	}

	nodeIDs, status, message := assignNodesToGroup(ctx, qtx, group.WorkspaceID, group.ID, nodeIDStrings)
	if status != consts.StatusOK {
		return nil, status, message
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, consts.StatusInternalServerError, "failed to update group"
	}
	return nodeIDs, consts.StatusOK, ""
}

func assignNodesToGroup(
	ctx context.Context,
	q *db.Queries,
	workspaceID pgtype.UUID,
	groupID pgtype.UUID,
	nodeIDStrings []string,
) ([]pgtype.UUID, int, string) {
	nodeIDs := make([]pgtype.UUID, 0, len(nodeIDStrings))
	seen := map[pgtype.UUID]bool{}
	for _, id := range nodeIDStrings {
		nodeID, ok := uuidFromString(id)
		if !ok {
			return nil, consts.StatusBadRequest, "invalid request"
		}
		if seen[nodeID] {
			continue
		}
		node, err := q.GetMediaNodeByID(ctx, nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, consts.StatusBadRequest, "invalid request"
			}
			return nil, consts.StatusInternalServerError, "failed to load node"
		}
		if node.WorkspaceID != workspaceID {
			return nil, consts.StatusBadRequest, "invalid request"
		}
		if _, err := q.UpdateMediaNodeGroup(ctx, db.UpdateMediaNodeGroupParams{ID: node.ID, GroupID: groupID}); err != nil {
			return nil, consts.StatusInternalServerError, "failed to update node"
		}
		seen[nodeID] = true
		nodeIDs = append(nodeIDs, nodeID)
	}
	return nodeIDs, consts.StatusOK, ""
}

func (h *GroupHandler) groupForAccount(ctx context.Context, id string, accountID pgtype.UUID, c *app.RequestContext) (db.MediaGroup, bool) {
	groupID, ok := uuidFromString(id)
	if !ok {
		writeError(c, consts.StatusNotFound, "group not found")
		return db.MediaGroup{}, false
	}
	group, err := h.queries.GetMediaGroupByID(ctx, groupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(c, consts.StatusNotFound, "group not found")
			return db.MediaGroup{}, false
		}
		writeError(c, consts.StatusInternalServerError, "failed to load group")
		return db.MediaGroup{}, false
	}
	if !h.workspaceBelongsToAccount(ctx, group.WorkspaceID, accountID, c) {
		return db.MediaGroup{}, false
	}
	return group, true
}

func (h *GroupHandler) workspaceBelongsToAccount(ctx context.Context, workspaceID pgtype.UUID, accountID pgtype.UUID, c *app.RequestContext) bool {
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
