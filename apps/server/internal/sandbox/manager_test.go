package sandbox

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/config"
)

func TestVolumeName(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{0xaa, 0xbb, 0xcc, 0xdd}, Valid: true}
	if got := VolumeName(id); got != "sandbox-ws-aabbccdd-0000-0000-0000-000000000000" {
		t.Fatalf("VolumeName() = %q", got)
	}
}

func TestEnsureSandboxCreatesWhenBindingIsCreating(t *testing.T) {
	store := newFakeBindingStore(Binding{
		Status:     StatusCreating,
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	})
	client := &fakeClient{createdID: "sandbox-1"}
	manager := NewManager(client, testSandboxConfig(), store)

	got, err := manager.EnsureSandbox(context.Background(), testWorkspaceID())
	if err != nil {
		t.Fatalf("EnsureSandbox error = %v", err)
	}
	if got.SandboxID != "sandbox-1" {
		t.Fatalf("sandbox id = %q, want sandbox-1", got.SandboxID)
	}
	if store.binding.Status != StatusRunning {
		t.Fatalf("binding status = %q, want running", store.binding.Status)
	}
	if client.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", client.createCalls)
	}
}

func TestEnsureSandboxReusesHealthyRunningSandbox(t *testing.T) {
	store := newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	})
	client := &fakeClient{}
	manager := NewManager(client, testSandboxConfig(), store)

	got, err := manager.EnsureSandbox(context.Background(), testWorkspaceID())
	if err != nil {
		t.Fatalf("EnsureSandbox error = %v", err)
	}
	if got.SandboxID != "sandbox-1" {
		t.Fatalf("sandbox id = %q, want sandbox-1", got.SandboxID)
	}
	if client.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", client.createCalls)
	}
	if !store.touched {
		t.Fatal("running sandbox must update last_seen_at")
	}
}

func TestEnsureSandboxReplacesUnhealthySandboxWithSameVolume(t *testing.T) {
	store := newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-old",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	})
	client := &fakeClient{pingErr: errors.New("not found"), createdID: "sandbox-new"}
	manager := NewManager(client, testSandboxConfig(), store)

	got, err := manager.EnsureSandbox(context.Background(), testWorkspaceID())
	if err != nil {
		t.Fatalf("EnsureSandbox error = %v", err)
	}
	if got.SandboxID != "sandbox-new" {
		t.Fatalf("sandbox id = %q, want sandbox-new", got.SandboxID)
	}
	if client.lastCreate.VolumeName != "sandbox-ws-aabbccdd-0000-0000-0000-000000000000" {
		t.Fatalf("volume = %q", client.lastCreate.VolumeName)
	}
}

func TestEnsureSandboxMarksUnhealthyWhenCreateFails(t *testing.T) {
	store := newFakeBindingStore(Binding{
		Status:     StatusCreating,
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	})
	client := &fakeClient{createErr: errors.New("create failed")}
	manager := NewManager(client, testSandboxConfig(), store)

	_, err := manager.EnsureSandbox(context.Background(), testWorkspaceID())
	if err == nil {
		t.Fatal("EnsureSandbox error = nil, want error")
	}
	if store.binding.Status != StatusUnhealthy {
		t.Fatalf("binding status = %q, want unhealthy", store.binding.Status)
	}
	if store.binding.ErrorMessage != "create failed" {
		t.Fatalf("error message = %q", store.binding.ErrorMessage)
	}
}

func TestEnsureSandboxSerializesConcurrentCreateWithBindingLock(t *testing.T) {
	store := newFakeBindingStore(Binding{
		Status:     StatusCreating,
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	})
	client := &fakeClient{createdID: "sandbox-1"}
	manager := NewManager(client, testSandboxConfig(), store)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := manager.EnsureSandbox(context.Background(), testWorkspaceID())
			if err != nil {
				errs <- err
				return
			}
			if got.SandboxID != "sandbox-1" {
				errs <- errors.New("unexpected sandbox id")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if client.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", client.createCalls)
	}
}

func TestDeleteSandboxTerminatesRunningSandboxAndKeepsVolume(t *testing.T) {
	store := newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	})
	client := &fakeClient{}
	manager := NewManager(client, testSandboxConfig(), store)

	got, err := manager.DeleteSandbox(context.Background(), testWorkspaceID())
	if err != nil {
		t.Fatalf("DeleteSandbox error = %v", err)
	}
	if client.deleteCalls != 1 || client.deletedID != "sandbox-1" {
		t.Fatalf("delete call = (%d, %q), want (1, sandbox-1)", client.deleteCalls, client.deletedID)
	}
	if got.SandboxID != "" {
		t.Fatalf("sandbox id = %q, want empty after terminate", got.SandboxID)
	}
	if got.VolumeName != "sandbox-ws-aabbccdd-0000-0000-0000-000000000000" {
		t.Fatalf("volume = %q", got.VolumeName)
	}
	if store.binding.Status != StatusTerminated {
		t.Fatalf("binding status = %q, want terminated", store.binding.Status)
	}
	if store.binding.SandboxID != "" {
		t.Fatalf("stored sandbox id = %q, want empty", store.binding.SandboxID)
	}
}

type fakeBindingStore struct {
	mu      sync.Mutex
	binding Binding
	touched bool
}

func newFakeBindingStore(binding Binding) *fakeBindingStore {
	return &fakeBindingStore{binding: binding}
}

func (s *fakeBindingStore) WithWorkspaceBinding(ctx context.Context, workspaceID pgtype.UUID, volumeName string, fn BindingFunc) (WorkspaceSandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.binding.VolumeName == "" {
		s.binding.VolumeName = volumeName
		s.binding.Status = StatusCreating
	}
	updater := fakeBindingUpdater{store: s}
	return fn(ctx, s.binding, updater)
}

type fakeBindingUpdater struct {
	store *fakeBindingStore
}

func (u fakeBindingUpdater) MarkCreating(ctx context.Context, workspaceID pgtype.UUID) (Binding, error) {
	u.store.binding.Status = StatusCreating
	u.store.binding.ErrorMessage = ""
	return u.store.binding, nil
}

func (u fakeBindingUpdater) MarkRunning(ctx context.Context, workspaceID pgtype.UUID, sandboxID string) (Binding, error) {
	u.store.binding.Status = StatusRunning
	u.store.binding.SandboxID = sandboxID
	u.store.binding.ErrorMessage = ""
	return u.store.binding, nil
}

func (u fakeBindingUpdater) MarkUnhealthy(ctx context.Context, workspaceID pgtype.UUID, message string) (Binding, error) {
	u.store.binding.Status = StatusUnhealthy
	u.store.binding.ErrorMessage = message
	return u.store.binding, nil
}

func (u fakeBindingUpdater) MarkTerminated(ctx context.Context, workspaceID pgtype.UUID) (Binding, error) {
	u.store.binding.Status = StatusTerminated
	u.store.binding.SandboxID = ""
	return u.store.binding, nil
}

func (u fakeBindingUpdater) TouchSeen(ctx context.Context, workspaceID pgtype.UUID) (Binding, error) {
	u.store.touched = true
	return u.store.binding, nil
}

type fakeClient struct {
	pingErr     error
	createErr   error
	createdID   string
	createCalls int
	lastCreate  CreateRequest
	deleteCalls int
	deletedID   string
}

func (f *fakeClient) Create(ctx context.Context, req CreateRequest) (SandboxInfo, error) {
	f.createCalls++
	f.lastCreate = req
	if f.createErr != nil {
		return SandboxInfo{}, f.createErr
	}
	return SandboxInfo{ID: f.createdID, State: "Running"}, nil
}

func (f *fakeClient) Get(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	return SandboxInfo{ID: sandboxID, State: "Running"}, nil
}

func (f *fakeClient) Ping(ctx context.Context, sandboxID string) error {
	return f.pingErr
}

func (f *fakeClient) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecResult, error) {
	return ExecResult{}, nil
}

func (f *fakeClient) Upload(ctx context.Context, sandboxID string, path string, r io.Reader) error {
	return nil
}

func (f *fakeClient) Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, FileInfo, error) {
	return io.NopCloser(strings.NewReader("")), FileInfo{}, nil
}

func (f *fakeClient) Delete(ctx context.Context, sandboxID string) error {
	f.deleteCalls++
	f.deletedID = sandboxID
	return nil
}

func testSandboxConfig() config.SandboxConfig {
	return config.SandboxConfig{
		Image:          "clipanvil-sandbox:dev",
		TimeoutSeconds: 1800,
		Workdir:        "/workspace",
		ResourceLimits: config.SandboxResourceLimits{CPU: "2", Memory: "4Gi"},
	}
}

func testWorkspaceID() pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{0xaa, 0xbb, 0xcc, 0xdd}, Valid: true}
}
