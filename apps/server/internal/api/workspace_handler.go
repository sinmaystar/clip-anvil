package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinmaystar/clip-anvil/internal/auth"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type WorkspaceHandler struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewWorkspaceHandler(pool *pgxpool.Pool, queries *db.Queries) *WorkspaceHandler {
	return &WorkspaceHandler{pool: pool, queries: queries}
}

type createWorkspaceRequest struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

type workspaceResponse struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Mode      string          `json:"mode"`
	OwnerID   string          `json:"owner_id"`
	Settings  json.RawMessage `json:"settings,omitempty"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

func (h *WorkspaceHandler) Create(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	var req createWorkspaceRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	if !validWorkspaceName(req.Name) {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	mode, ok := req.workspaceMode()
	if !ok {
		writeError(c, consts.StatusBadRequest, "invalid workspace mode")
		return
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create workspace")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := h.queries.WithTx(tx)
	workspace, err := qtx.CreateWorkspace(ctx, db.CreateWorkspaceParams{
		Name:    strings.TrimSpace(req.Name),
		OwnerID: accountID,
		Mode:    mode,
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create workspace")
		return
	}
	if _, err := qtx.CreateCanvasDocument(ctx, workspace.ID); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create canvas")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to create workspace")
		return
	}

	c.JSON(consts.StatusOK, toWorkspaceResponse(workspace))
}

func (h *WorkspaceHandler) List(ctx context.Context, c *app.RequestContext) {
	accountID, ok := accountIDFromContext(c)
	if !ok {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
		return
	}

	workspaces, err := h.queries.ListWorkspacesByOwner(ctx, accountID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list workspaces")
		return
	}

	response := make([]workspaceResponse, 0, len(workspaces))
	for _, workspace := range workspaces {
		response = append(response, toWorkspaceResponse(workspace))
	}
	c.JSON(consts.StatusOK, response)
}

func (h *WorkspaceHandler) Get(ctx context.Context, c *app.RequestContext) {
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

	c.JSON(consts.StatusOK, toWorkspaceResponse(workspace))
}

func validWorkspaceName(name string) bool {
	return strings.TrimSpace(name) != ""
}

func (r createWorkspaceRequest) workspaceMode() (db.WorkspaceMode, bool) {
	switch strings.TrimSpace(r.Mode) {
	case "", string(db.WorkspaceModeStudio):
		return db.WorkspaceModeStudio, true
	case string(db.WorkspaceModeAgent):
		return db.WorkspaceModeAgent, true
	default:
		return "", false
	}
}

func accountIDFromContext(c *app.RequestContext) (pgtype.UUID, bool) {
	value, ok := c.Get(auth.AccountIDKey)
	if !ok {
		return pgtype.UUID{}, false
	}
	accountID, ok := value.(pgtype.UUID)
	if !ok || !accountID.Valid {
		return pgtype.UUID{}, false
	}
	return accountID, true
}

func uuidFromString(value string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, false
	}
	return id, id.Valid
}

func toWorkspaceResponse(workspace db.Workspace) workspaceResponse {
	return workspaceResponse{
		ID:        uuidToString(workspace.ID),
		Name:      workspace.Name,
		Mode:      string(workspace.Mode),
		OwnerID:   uuidToString(workspace.OwnerID),
		Settings:  json.RawMessage(workspace.Settings),
		CreatedAt: workspace.CreatedAt.Time.Format(timeFormatRFC3339),
		UpdatedAt: workspace.UpdatedAt.Time.Format(timeFormatRFC3339),
	}
}
