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

func isStudioWorkspaceMode(mode db.WorkspaceMode) bool {
	return mode == db.WorkspaceModeStudio
}

func isCanvasLayoutWorkspaceMode(mode db.WorkspaceMode) bool {
	return mode == db.WorkspaceModeStudio || mode == db.WorkspaceModeAgent
}

func workspaceForAccount(
	ctx context.Context,
	queries *db.Queries,
	workspaceID pgtype.UUID,
	accountID pgtype.UUID,
	c *app.RequestContext,
) (db.Workspace, bool) {
	workspace, err := queries.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(c, consts.StatusNotFound, "workspace not found")
			return db.Workspace{}, false
		}
		writeError(c, consts.StatusInternalServerError, "failed to load workspace")
		return db.Workspace{}, false
	}
	if workspace.OwnerID != accountID {
		writeError(c, consts.StatusForbidden, "forbidden")
		return db.Workspace{}, false
	}
	return workspace, true
}

func workspaceBelongsToAccount(
	ctx context.Context,
	queries *db.Queries,
	workspaceID pgtype.UUID,
	accountID pgtype.UUID,
	c *app.RequestContext,
) bool {
	_, ok := workspaceForAccount(ctx, queries, workspaceID, accountID, c)
	return ok
}

func requireStudioWorkspace(
	ctx context.Context,
	queries *db.Queries,
	workspaceID pgtype.UUID,
	accountID pgtype.UUID,
	c *app.RequestContext,
) (db.Workspace, bool) {
	workspace, ok := workspaceForAccount(ctx, queries, workspaceID, accountID, c)
	if !ok {
		return db.Workspace{}, false
	}
	if !isStudioWorkspaceMode(workspace.Mode) {
		writeError(c, consts.StatusForbidden, "workspace is read-only in agent mode")
		return db.Workspace{}, false
	}
	return workspace, true
}

func requireCanvasLayoutWorkspace(
	ctx context.Context,
	queries *db.Queries,
	workspaceID pgtype.UUID,
	accountID pgtype.UUID,
	c *app.RequestContext,
) (db.Workspace, bool) {
	workspace, ok := workspaceForAccount(ctx, queries, workspaceID, accountID, c)
	if !ok {
		return db.Workspace{}, false
	}
	if !isCanvasLayoutWorkspaceMode(workspace.Mode) {
		writeError(c, consts.StatusForbidden, "workspace layout is not writable")
		return db.Workspace{}, false
	}
	return workspace, true
}
