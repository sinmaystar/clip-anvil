package sandbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/config"
)

type Manager struct {
	client  Client
	cfg     config.SandboxConfig
	binding BindingStore
}

type WorkspaceSandbox struct {
	WorkspaceID pgtype.UUID
	SandboxID   string
	VolumeName  string
}

func NewManager(client Client, cfg config.SandboxConfig, binding ...BindingStore) *Manager {
	manager := &Manager{client: client, cfg: cfg}
	if len(binding) > 0 {
		manager.binding = binding[0]
	}
	return manager
}

func (m *Manager) EnsureSandbox(ctx context.Context, workspaceID pgtype.UUID) (WorkspaceSandbox, error) {
	if m.binding == nil {
		return WorkspaceSandbox{}, errors.New("sandbox binding store is required")
	}
	volumeName := VolumeName(workspaceID)
	return m.binding.WithWorkspaceBinding(ctx, workspaceID, volumeName, func(ctx context.Context, binding Binding, updater BindingUpdater) (WorkspaceSandbox, error) {
		return m.ensureWithBinding(ctx, workspaceID, binding, updater)
	})
}

func (m *Manager) DeleteSandbox(ctx context.Context, workspaceID pgtype.UUID) (WorkspaceSandbox, error) {
	if m.binding == nil {
		return WorkspaceSandbox{}, errors.New("sandbox binding store is required")
	}
	volumeName := VolumeName(workspaceID)
	return m.binding.WithWorkspaceBinding(ctx, workspaceID, volumeName, func(ctx context.Context, binding Binding, updater BindingUpdater) (WorkspaceSandbox, error) {
		if binding.VolumeName == "" {
			binding.VolumeName = volumeName
		}
		if binding.SandboxID != "" {
			if err := m.client.Delete(ctx, binding.SandboxID); err != nil {
				return WorkspaceSandbox{}, err
			}
		}
		terminated, err := updater.MarkTerminated(ctx, workspaceID)
		if err != nil {
			return WorkspaceSandbox{}, err
		}
		return WorkspaceSandbox{WorkspaceID: workspaceID, SandboxID: terminated.SandboxID, VolumeName: terminated.VolumeName}, nil
	})
}

func (m *Manager) ensureWithBinding(ctx context.Context, workspaceID pgtype.UUID, binding Binding, updater BindingUpdater) (WorkspaceSandbox, error) {
	if binding.VolumeName == "" {
		binding.VolumeName = VolumeName(workspaceID)
	}
	if binding.SandboxID != "" && binding.Status == StatusRunning {
		if err := m.client.Ping(ctx, binding.SandboxID); err == nil {
			if _, err := updater.TouchSeen(ctx, workspaceID); err != nil {
				return WorkspaceSandbox{}, err
			}
			return WorkspaceSandbox{WorkspaceID: workspaceID, SandboxID: binding.SandboxID, VolumeName: binding.VolumeName}, nil
		}
	}
	if _, err := updater.MarkCreating(ctx, workspaceID); err != nil {
		return WorkspaceSandbox{}, err
	}
	info, err := m.client.Create(ctx, CreateRequest{
		Image:          m.cfg.Image,
		VolumeName:     binding.VolumeName,
		MountPath:      m.workdir(),
		TimeoutSeconds: m.cfg.TimeoutSeconds,
		ResourceCPU:    m.cfg.ResourceLimits.CPU,
		ResourceMemory: m.cfg.ResourceLimits.Memory,
	})
	if err != nil {
		if _, markErr := updater.MarkUnhealthy(ctx, workspaceID, err.Error()); markErr != nil {
			return WorkspaceSandbox{}, fmt.Errorf("%w; mark unhealthy: %v", err, markErr)
		}
		return WorkspaceSandbox{}, err
	}
	if info.ID == "" {
		err := errors.New("opensandbox returned empty sandbox id")
		if _, markErr := updater.MarkUnhealthy(ctx, workspaceID, err.Error()); markErr != nil {
			return WorkspaceSandbox{}, fmt.Errorf("%w; mark unhealthy: %v", err, markErr)
		}
		return WorkspaceSandbox{}, err
	}
	if _, err := updater.MarkRunning(ctx, workspaceID, info.ID); err != nil {
		return WorkspaceSandbox{}, err
	}
	return WorkspaceSandbox{WorkspaceID: workspaceID, SandboxID: info.ID, VolumeName: binding.VolumeName}, nil
}

func (m *Manager) workdir() string {
	if m.cfg.Workdir != "" {
		return m.cfg.Workdir
	}
	return DefaultWorkdir
}
