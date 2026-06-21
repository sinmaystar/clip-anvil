package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinmaystar/clip-anvil/internal/production"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type ReferencePackHandler struct {
	pool       *pgxpool.Pool
	queries    *db.Queries
	production *production.Service
}

type replaceReferencePackItemsRequest struct {
	MemberNodeIDs []string `json:"member_node_ids"`
}

type referencePackItemResponse struct {
	ID           string `json:"id"`
	PackNodeID   string `json:"pack_node_id"`
	MemberNodeID string `json:"member_node_id"`
	Position     int32  `json:"position"`
}

func NewReferencePackHandler(pool *pgxpool.Pool, queries *db.Queries, productionService *production.Service) *ReferencePackHandler {
	return &ReferencePackHandler{pool: pool, queries: queries, production: productionService}
}

func (r replaceReferencePackItemsRequest) memberUUIDs() ([]pgtype.UUID, error) {
	seen := map[pgtype.UUID]bool{}
	out := make([]pgtype.UUID, 0, len(r.MemberNodeIDs))
	for _, raw := range r.MemberNodeIDs {
		id, ok := uuidFromString(raw)
		if !ok {
			return nil, fmt.Errorf("invalid member")
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate member")
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

func validateReferencePackMember(pack db.MediaNode, member db.MediaNode) error {
	if pack.NodeType != db.NodeTypeReferencePack {
		return fmt.Errorf("node is not a reference pack")
	}
	if pack.WorkspaceID != member.WorkspaceID {
		return fmt.Errorf("member is outside workspace")
	}
	if pack.ID == member.ID {
		return fmt.Errorf("pack cannot contain itself")
	}
	if member.NodeType == db.NodeTypeReferencePack {
		return fmt.Errorf("reference pack nesting is not supported")
	}
	return nil
}

func (h *ReferencePackHandler) ListItems(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	pack, ok := nodeForAccountByQueries(ctx, h.queries, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	if pack.NodeType != db.NodeTypeReferencePack {
		writeError(c, consts.StatusBadRequest, "node is not a reference pack")
		return
	}
	items, err := h.queries.ListReferencePackItems(ctx, pack.ID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list reference pack items")
		return
	}
	resp := make([]referencePackItemResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReferencePackItemResponse(item))
	}
	c.JSON(consts.StatusOK, resp)
}

func (h *ReferencePackHandler) ReplaceItems(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}
	pack, ok := nodeForAccountByQueries(ctx, h.queries, c.Param("id"), accountID, c)
	if !ok {
		return
	}
	if _, ok := requireStudioWorkspace(ctx, h.queries, pack.WorkspaceID, accountID, c); !ok {
		return
	}
	var req replaceReferencePackItemsRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	memberIDs, err := req.memberUUIDs()
	if err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}

	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to update reference pack")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := h.queries.WithTx(tx)
	if pack.NodeType != db.NodeTypeReferencePack {
		writeError(c, consts.StatusBadRequest, "node is not a reference pack")
		return
	}
	if err := qtx.DeleteReferencePackItems(ctx, pack.ID); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to update reference pack")
		return
	}
	items := make([]db.ReferencePackItem, 0, len(memberIDs))
	for i, memberID := range memberIDs {
		member, err := qtx.GetMediaNodeByID(ctx, memberID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(c, consts.StatusBadRequest, "invalid request")
				return
			}
			writeError(c, consts.StatusInternalServerError, "failed to update reference pack")
			return
		}
		if err := validateReferencePackMember(pack, member); err != nil {
			writeError(c, consts.StatusBadRequest, "invalid request")
			return
		}
		existingPackInput, err := qtx.GetDependencyEdgeByEndpoints(ctx, db.GetDependencyEdgeByEndpointsParams{
			FromNodeID: pack.ID,
			ToNodeID:   member.ID,
		})
		if err == nil && referencePackMemberHasPackInput([]db.MediaEdge{existingPackInput}, pack.ID, member.ID) {
			writeError(c, consts.StatusUnprocessableEntity, "reference pack cannot contain a node that depends on it")
			return
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(c, consts.StatusInternalServerError, "failed to update reference pack")
			return
		}
		item, err := qtx.CreateReferencePackItem(ctx, db.CreateReferencePackItemParams{
			WorkspaceID:  pack.WorkspaceID,
			PackNodeID:   pack.ID,
			MemberNodeID: member.ID,
			Position:     int32(i),
			Metadata:     []byte("{}"),
		})
		if err != nil {
			writeError(c, consts.StatusInternalServerError, "failed to update reference pack")
			return
		}
		items = append(items, item)
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to update reference pack")
		return
	}
	if h.production != nil {
		if err := h.production.MarkDownstreamStale(ctx, pack.ID, "reference_pack_membership_changed", "Reference pack membership changed."); err != nil {
			slog.Warn("failed to mark reference pack dependents stale", "pack_id", uuidToString(pack.ID), "error", err)
		}
	}
	resp := make([]referencePackItemResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toReferencePackItemResponse(item))
	}
	c.JSON(consts.StatusOK, resp)
}

func referencePackMemberHasPackInput(edges []db.MediaEdge, packID pgtype.UUID, memberID pgtype.UUID) bool {
	for _, edge := range edges {
		if edge.FromNodeID == packID && edge.ToNodeID == memberID {
			return true
		}
	}
	return false
}

func toReferencePackItemResponse(item db.ReferencePackItem) referencePackItemResponse {
	return referencePackItemResponse{
		ID:           uuidToString(item.ID),
		PackNodeID:   uuidToString(item.PackNodeID),
		MemberNodeID: uuidToString(item.MemberNodeID),
		Position:     item.Position,
	}
}
