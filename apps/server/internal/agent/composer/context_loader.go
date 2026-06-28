package composer

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Context struct {
	WorkspaceID            pgtype.UUID `json:"workspace_id"`
	SourceStoryboardNodeID pgtype.UUID `json:"source_storyboard_node_id,omitempty"`
	Summary                string      `json:"summary"`
	WorkspaceMode          string      `json:"workspace_mode,omitempty"`
	SourceNodeTitle        string      `json:"source_node_title,omitempty"`
	TimelinePlanCount      int         `json:"timeline_plan_count"`
}

type ContextLoader interface {
	LoadCompositionContext(ctx context.Context, req Request) (Context, error)
}

type ContextStore interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	GetMediaNodeByID(ctx context.Context, id pgtype.UUID) (db.MediaNode, error)
	ListTimelinePlansByWorkspace(ctx context.Context, arg db.ListTimelinePlansByWorkspaceParams) ([]db.TimelinePlan, error)
}

type StoreContextLoader struct {
	store ContextStore
}

func NewStoreContextLoader(store ContextStore) StoreContextLoader {
	return StoreContextLoader{store: store}
}

func (l StoreContextLoader) LoadCompositionContext(ctx context.Context, req Request) (Context, error) {
	if l.store == nil {
		return Context{}, ErrInvalidConfig
	}
	workspace, err := l.store.GetWorkspaceByID(ctx, req.WorkspaceID)
	if err != nil {
		return Context{}, err
	}
	out := Context{
		WorkspaceID:            req.WorkspaceID,
		SourceStoryboardNodeID: req.SourceStoryboardNodeID,
		WorkspaceMode:          string(workspace.Mode),
		Summary:                "Composer context loaded.",
	}
	if req.SourceStoryboardNodeID.Valid {
		node, err := l.store.GetMediaNodeByID(ctx, req.SourceStoryboardNodeID)
		if err != nil {
			return Context{}, err
		}
		out.SourceNodeTitle = node.Title
	}
	plans, err := l.store.ListTimelinePlansByWorkspace(ctx, db.ListTimelinePlansByWorkspaceParams{WorkspaceID: req.WorkspaceID, LimitCount: 20})
	if err != nil {
		return Context{}, err
	}
	out.TimelinePlanCount = len(plans)
	return out, nil
}
