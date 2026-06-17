package api

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/websocket"
	"github.com/jackc/pgx/v5"

	"github.com/sinmaystar/clip-anvil/internal/auth"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type CanvasWSHandler struct {
	queries   *db.Queries
	hub       *CanvasHub
	jwtSecret string
	upgrader  websocket.HertzUpgrader
}

func NewCanvasWSHandler(queries *db.Queries, hub *CanvasHub, jwtSecret string) *CanvasWSHandler {
	return &CanvasWSHandler{
		queries:   queries,
		hub:       hub,
		jwtSecret: jwtSecret,
		upgrader: websocket.HertzUpgrader{
			CheckOrigin: func(*app.RequestContext) bool {
				return true
			},
		},
	}
}

func (h *CanvasWSHandler) Canvas(ctx context.Context, c *app.RequestContext) {
	workspaceID, ok := uuidFromString(string(c.Query("workspaceId")))
	if !ok {
		writeError(c, consts.StatusBadRequest, "invalid request")
		return
	}
	token := string(c.Query("token"))
	accountID, err := auth.VerifyToken(token, h.jwtSecret)
	if err != nil {
		writeError(c, consts.StatusUnauthorized, "unauthorized")
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

	if err := h.upgrader.Upgrade(c, func(conn *websocket.Conn) {
		h.hub.Register(workspaceID, conn)
		defer h.hub.Unregister(workspaceID, conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}); err != nil {
		return
	}
}
