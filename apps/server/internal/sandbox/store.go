package sandbox

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Binding struct {
	SandboxID    string
	VolumeName   string
	Status       string
	ErrorMessage string
}

type BindingFunc func(ctx context.Context, binding Binding, updater BindingUpdater) (WorkspaceSandbox, error)

type BindingStore interface {
	WithWorkspaceBinding(ctx context.Context, workspaceID pgtype.UUID, volumeName string, fn BindingFunc) (WorkspaceSandbox, error)
}

type BindingUpdater interface {
	MarkCreating(ctx context.Context, workspaceID pgtype.UUID) (Binding, error)
	MarkRunning(ctx context.Context, workspaceID pgtype.UUID, sandboxID string) (Binding, error)
	MarkUnhealthy(ctx context.Context, workspaceID pgtype.UUID, message string) (Binding, error)
	MarkTerminated(ctx context.Context, workspaceID pgtype.UUID) (Binding, error)
	TouchSeen(ctx context.Context, workspaceID pgtype.UUID) (Binding, error)
}

type Store struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewStore(pool *pgxpool.Pool, queries *db.Queries) *Store {
	return &Store{pool: pool, queries: queries}
}

func (s *Store) WithWorkspaceBinding(ctx context.Context, workspaceID pgtype.UUID, volumeName string, fn BindingFunc) (WorkspaceSandbox, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WorkspaceSandbox{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := s.queries.WithTx(tx)
	if _, err := qtx.CreateWorkspaceSandboxBinding(ctx, db.CreateWorkspaceSandboxBindingParams{
		WorkspaceID: workspaceID,
		VolumeName:  volumeName,
		Status:      StatusCreating,
	}); err != nil {
		return WorkspaceSandbox{}, err
	}
	row, err := qtx.LockWorkspaceSandboxBinding(ctx, workspaceID)
	if err != nil {
		return WorkspaceSandbox{}, err
	}
	got, fnErr := fn(ctx, bindingFromRow(row), dbBindingUpdater{queries: qtx})
	if err := tx.Commit(ctx); err != nil {
		return WorkspaceSandbox{}, err
	}
	return got, fnErr
}

type dbBindingUpdater struct {
	queries *db.Queries
}

func (u dbBindingUpdater) MarkCreating(ctx context.Context, workspaceID pgtype.UUID) (Binding, error) {
	row, err := u.queries.MarkWorkspaceSandboxCreating(ctx, workspaceID)
	return bindingFromRow(row), err
}

func (u dbBindingUpdater) MarkRunning(ctx context.Context, workspaceID pgtype.UUID, sandboxID string) (Binding, error) {
	row, err := u.queries.MarkWorkspaceSandboxRunning(ctx, db.MarkWorkspaceSandboxRunningParams{
		WorkspaceID: workspaceID,
		SandboxID:   pgtype.Text{String: sandboxID, Valid: true},
	})
	return bindingFromRow(row), err
}

func (u dbBindingUpdater) MarkUnhealthy(ctx context.Context, workspaceID pgtype.UUID, message string) (Binding, error) {
	row, err := u.queries.MarkWorkspaceSandboxUnhealthy(ctx, db.MarkWorkspaceSandboxUnhealthyParams{
		WorkspaceID:  workspaceID,
		ErrorMessage: pgtype.Text{String: message, Valid: message != ""},
	})
	return bindingFromRow(row), err
}

func (u dbBindingUpdater) MarkTerminated(ctx context.Context, workspaceID pgtype.UUID) (Binding, error) {
	row, err := u.queries.MarkWorkspaceSandboxTerminated(ctx, workspaceID)
	return bindingFromRow(row), err
}

func (u dbBindingUpdater) TouchSeen(ctx context.Context, workspaceID pgtype.UUID) (Binding, error) {
	row, err := u.queries.TouchWorkspaceSandboxSeen(ctx, workspaceID)
	return bindingFromRow(row), err
}

func bindingFromRow(row db.WorkspaceSandbox) Binding {
	binding := Binding{
		VolumeName: row.VolumeName,
		Status:     row.Status,
	}
	if row.SandboxID.Valid {
		binding.SandboxID = row.SandboxID.String
	}
	if row.ErrorMessage.Valid {
		binding.ErrorMessage = row.ErrorMessage.String
	}
	return binding
}
