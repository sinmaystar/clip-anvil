# OpenSandbox Workspace Sandbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a DB-first, workspace-scoped OpenSandbox execution layer for Agent media workflows.

**Architecture:** ClipAnvil keeps sandbox edges, permissions, MinIO writes, assets, nodes, and WebSocket events in the Go backend. OpenSandbox owns container lifecycle and in-sandbox command/file operations. Each workspace has a stable `/workspace` volume and a replaceable current sandbox container recorded in `workspace_sandbox`.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc/goose, MinIO Go SDK, OpenSandbox Go SDK, Docker Compose, PostgreSQL 16, Redis 7, MinIO, NGINX.

---

## File Map

- Create `sandbox-image/Dockerfile`: media-capable sandbox image with ffmpeg, Python, ImageMagick, jq, file, and fonts.
- Create `sandbox-image/policy.xml`: ImageMagick policy that blocks URL/HTTP/HTTPS coders.
- Modify `deploy/docker-compose.yml`: add OpenSandbox Server and Docker socket mount.
- Create `deploy/config/sandbox.toml`: local OpenSandbox Docker runtime config.
- Modify `apps/server/config.yaml`: add sandbox runtime config.
- Modify `apps/server/internal/config/config.go`: add `SandboxConfig` and nested resource limits.
- Create `apps/server/migrations/005_add_workspace_sandbox.sql`: DB-first sandbox edge table.
- Create `apps/server/sqlc/queries/sandbox.sql`: CRUD and row-lock queries for `workspace_sandbox`.
- Regenerate `apps/server/internal/store/db/*`: sqlc generated code.
- Create `apps/server/internal/sandbox/config.go`: constants and config validation helpers.
- Create `apps/server/internal/sandbox/client.go`: ClipAnvil OpenSandbox adapter interface and SDK-backed implementation.
- Create `apps/server/internal/sandbox/store.go`: typed DB wrapper around generated sandbox queries.
- Create `apps/server/internal/sandbox/manager.go`: `EnsureSandbox`, health checks, and reset flow.
- Create `apps/server/internal/sandbox/workspace.go`: `/workspace` layout and asset manifest preparation.
- Create `apps/server/internal/sandbox/exec.go`: `sandbox_exec` behavior.
- Create `apps/server/internal/sandbox/paths.go`: output path and safe filename validation.
- Create `apps/server/internal/sandbox/artifact.go`: `submit_artifact` behavior.
- Modify `apps/server/sqlc/queries/node.sql`: add a query that can create agent-owned nodes.
- Modify `apps/server/internal/api/upload_handler.go`: extract reusable MinIO asset storage helper only if needed by `artifact.go`.
- Modify `apps/server/cmd/server/main.go`: wire sandbox config, manager, and debug status/reset handlers.
- Modify `docs/engineering/development.md`: document OpenSandbox local startup and health check.

## Task 1: Infrastructure and Config

**Files:**
- Create: `sandbox-image/Dockerfile`
- Create: `sandbox-image/policy.xml`
- Create: `deploy/config/sandbox.toml`
- Modify: `deploy/docker-compose.yml`
- Modify: `apps/server/config.yaml`
- Modify: `apps/server/internal/config/config.go`
- Modify: `docs/engineering/development.md`
- Test: `apps/server/internal/config/config_test.go`

- [ ] **Step 1: Add failing config test**

Add this test to `apps/server/internal/config/config_test.go`:

```go
func TestLoadSandboxConfig(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Sandbox.Endpoint == "" {
		t.Fatal("sandbox endpoint must be configured")
	}
	if cfg.Sandbox.APIKey == "" {
		t.Fatal("sandbox api key must be configured")
	}
	if cfg.Sandbox.Image == "" {
		t.Fatal("sandbox image must be configured")
	}
	if cfg.Sandbox.Workdir != "/workspace" {
		t.Fatalf("sandbox workdir = %q, want /workspace", cfg.Sandbox.Workdir)
	}
	if cfg.Sandbox.TimeoutSeconds != 1800 {
		t.Fatalf("sandbox timeout = %d, want 1800", cfg.Sandbox.TimeoutSeconds)
	}
	if cfg.Sandbox.ResourceLimits.CPU != "2" {
		t.Fatalf("sandbox cpu limit = %q, want 2", cfg.Sandbox.ResourceLimits.CPU)
	}
	if cfg.Sandbox.ResourceLimits.Memory != "4Gi" {
		t.Fatalf("sandbox memory limit = %q, want 4Gi", cfg.Sandbox.ResourceLimits.Memory)
	}
	if !cfg.Sandbox.UseServerProxy {
		t.Fatal("sandbox use_server_proxy must be true")
	}
}
```

- [ ] **Step 2: Run config test and verify it fails**

Run:

```bash
cd apps/server && go test ./internal/config -run TestLoadSandboxConfig -v
```

Expected: fail because `Config` has no `Sandbox` field.

- [ ] **Step 3: Extend backend config structs**

Modify `apps/server/internal/config/config.go`:

```go
type Config struct {
	Server   ServerConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	MinIO    MinIOConfig
	JWT      JWTConfig
	Sandbox  SandboxConfig
}

type SandboxConfig struct {
	Endpoint       string
	APIKey         string `mapstructure:"api_key"`
	Image          string
	TimeoutSeconds int `mapstructure:"timeout_seconds"`
	Workdir        string
	UseServerProxy bool `mapstructure:"use_server_proxy"`
	ResourceLimits SandboxResourceLimits `mapstructure:"resource_limits"`
}

type SandboxResourceLimits struct {
	CPU    string
	Memory string
}
```

- [ ] **Step 4: Add local sandbox config**

Append to `apps/server/config.yaml`:

```yaml
sandbox:
  endpoint: "http://localhost:8080/v1"
  api_key: "clipanvil-dev-sandbox-key"
  image: "clipanvil-sandbox:dev"
  timeout_seconds: 1800
  workdir: "/workspace"
  use_server_proxy: true
  resource_limits:
    cpu: "2"
    memory: "4Gi"
```

- [ ] **Step 5: Add sandbox image**

Create `sandbox-image/Dockerfile`:

```dockerfile
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash \
        ca-certificates \
        coreutils \
        curl \
        ffmpeg \
        file \
        fonts-noto-cjk \
        fonts-noto-color-emoji \
        imagemagick \
        jq \
        python3 \
        python3-pip \
    && rm -rf /var/lib/apt/lists/*

COPY policy.xml /etc/ImageMagick-6/policy.xml

RUN mkdir -p /workspace/assets /workspace/scripts /workspace/tmp /workspace/output

WORKDIR /workspace

CMD ["sleep", "infinity"]
```

Create `sandbox-image/policy.xml`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<policymap>
  <policy domain="coder" rights="none" pattern="URL" />
  <policy domain="coder" rights="none" pattern="HTTP" />
  <policy domain="coder" rights="none" pattern="HTTPS" />
</policymap>
```

- [ ] **Step 6: Add OpenSandbox Server config**

Create `deploy/config/sandbox.toml`:

```toml
[server]
host = "0.0.0.0"
port = 8080
api_key = "clipanvil-dev-sandbox-key"

[runtime]
type = "docker"
timeout_seconds = 1800

[runtime.docker]
network_mode = "bridge"
host_ip = "0.0.0.0"

[store]
type = "sqlite"
sqlite_path = "/data/opensandbox.db"
```

Before committing this task, verify the file against the OpenSandbox server version installed locally by running `opensandbox-server init-config /tmp/clipanvil-sandbox.toml --example docker --force` and comparing required key names. Preserve these design requirements when adjusting key names: API key on, Docker runtime, bridge networking, persistent server store.

- [ ] **Step 7: Add OpenSandbox service to compose**

Modify `deploy/docker-compose.yml` by adding the service:

```yaml
  opensandbox-server:
    image: ghcr.io/opensandbox-group/opensandbox-server:latest
    command: ["opensandbox-server", "--config", "/etc/opensandbox/sandbox.toml"]
    ports:
      - "8080:8080"
    environment:
      OPEN-SANDBOX-API-KEY: clipanvil-dev-sandbox-key
    volumes:
      - ./config/sandbox.toml:/etc/opensandbox/sandbox.toml:ro
      - opensandbox_data:/data
      - /var/run/docker.sock:/var/run/docker.sock
```

Add the named volume:

```yaml
  opensandbox_data:
```

- [ ] **Step 8: Document startup**

Add this section to `docs/engineering/development.md` under backend or local development commands:

```markdown
### OpenSandbox

The local development stack includes OpenSandbox Server on `http://localhost:8080`.
It is an internal infrastructure service used only by the Go backend.

Health check:

```bash
curl http://127.0.0.1:8080/health
```

Build the local sandbox image when changing `sandbox-image/`:

```bash
docker build -t clipanvil-sandbox:dev sandbox-image
```
```

- [ ] **Step 9: Run verification**

Run:

```bash
cd apps/server && go test ./internal/config -run TestLoadSandboxConfig -v
make server-build
docker build -t clipanvil-sandbox:dev sandbox-image
```

Expected:

- Config test passes.
- Server build exits 0.
- Docker image build exits 0.

If Docker or network access is blocked by the execution environment, record the exact error and rerun with the required permissions before debugging code.

- [ ] **Step 10: Commit**

```bash
git add sandbox-image deploy/config/sandbox.toml deploy/docker-compose.yml apps/server/config.yaml apps/server/internal/config/config.go apps/server/internal/config/config_test.go docs/engineering/development.md
git commit -m "feat: add opensandbox local infrastructure"
```

## Task 2: DB-First Sandbox Manager

**Files:**
- Create: `apps/server/migrations/005_add_workspace_sandbox.sql`
- Create: `apps/server/sqlc/queries/sandbox.sql`
- Create: `apps/server/internal/sandbox/config.go`
- Create: `apps/server/internal/sandbox/client.go`
- Create: `apps/server/internal/sandbox/store.go`
- Create: `apps/server/internal/sandbox/manager.go`
- Test: `apps/server/internal/sandbox/manager_test.go`
- Generated: `apps/server/internal/store/db/*`

- [ ] **Step 1: Add migration**

Create `apps/server/migrations/005_add_workspace_sandbox.sql`:

```sql
-- +goose Up
CREATE TABLE workspace_sandbox (
    workspace_id UUID PRIMARY KEY REFERENCES workspace(id) ON DELETE CASCADE,
    sandbox_id TEXT,
    volume_name TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('creating', 'running', 'unhealthy', 'terminated')),
    last_health_check_at TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_workspace_sandbox_status ON workspace_sandbox(status);

-- +goose Down
DROP INDEX IF EXISTS idx_workspace_sandbox_status;
DROP TABLE IF EXISTS workspace_sandbox;
```

- [ ] **Step 2: Add sqlc queries**

Create `apps/server/sqlc/queries/sandbox.sql`:

```sql
-- name: CreateWorkspaceSandboxBinding :one
INSERT INTO workspace_sandbox (
    workspace_id,
    volume_name,
    status
) VALUES (
    $1, $2, $3
)
ON CONFLICT (workspace_id) DO UPDATE
SET updated_at = workspace_sandbox.updated_at
RETURNING *;

-- name: GetWorkspaceSandboxBinding :one
SELECT *
FROM workspace_sandbox
WHERE workspace_id = $1;

-- name: LockWorkspaceSandboxBinding :one
SELECT *
FROM workspace_sandbox
WHERE workspace_id = $1
FOR UPDATE;

-- name: MarkWorkspaceSandboxCreating :one
UPDATE workspace_sandbox
SET status = 'creating',
    error_message = NULL,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;

-- name: MarkWorkspaceSandboxRunning :one
UPDATE workspace_sandbox
SET sandbox_id = $2,
    status = 'running',
    last_health_check_at = now(),
    last_seen_at = now(),
    error_message = NULL,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;

-- name: MarkWorkspaceSandboxUnhealthy :one
UPDATE workspace_sandbox
SET status = 'unhealthy',
    last_health_check_at = now(),
    error_message = $2,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;

-- name: MarkWorkspaceSandboxTerminated :one
UPDATE workspace_sandbox
SET status = 'terminated',
    sandbox_id = NULL,
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;

-- name: TouchWorkspaceSandboxSeen :one
UPDATE workspace_sandbox
SET last_health_check_at = now(),
    last_seen_at = now(),
    updated_at = now()
WHERE workspace_id = $1
RETURNING *;
```

- [ ] **Step 3: Generate sqlc code**

Run:

```bash
make sqlc-generate
```

Expected: generated Go code includes `WorkspaceSandbox` model and the sandbox query methods.

- [ ] **Step 4: Add sandbox package constants**

Create `apps/server/internal/sandbox/config.go`:

```go
package sandbox

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusCreating   = "creating"
	StatusRunning    = "running"
	StatusUnhealthy  = "unhealthy"
	StatusTerminated = "terminated"

	DefaultWorkdir = "/workspace"
)

func VolumeName(workspaceID pgtype.UUID) string {
	return "sandbox-ws-" + uuidString(workspaceID)
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}
```

- [ ] **Step 5: Add client interface and fake**

Create `apps/server/internal/sandbox/client.go`:

```go
package sandbox

import (
	"context"
	"io"
)

type CreateRequest struct {
	Image          string
	VolumeName     string
	MountPath      string
	TimeoutSeconds int
	ResourceCPU    string
	ResourceMemory string
}

type SandboxInfo struct {
	ID    string
	State string
}

type ExecRequest struct {
	Command        string
	Cwd            string
	TimeoutSeconds int
}

type ExecResult struct {
	ExitCode   int
	Stdout     string
	Stderr     string
	DurationMS int64
	Truncated  bool
}

type FileInfo struct {
	Path      string
	SizeBytes int64
	Mime      string
}

type Client interface {
	Create(ctx context.Context, req CreateRequest) (SandboxInfo, error)
	Get(ctx context.Context, sandboxID string) (SandboxInfo, error)
	Ping(ctx context.Context, sandboxID string) error
	Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecResult, error)
	Upload(ctx context.Context, sandboxID string, path string, r io.Reader) error
	Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, FileInfo, error)
	Delete(ctx context.Context, sandboxID string) error
}
```

Task 6 adds the SDK-backed implementation after the OpenSandbox module is installed. Manager tests in this task use a fake implementing this interface.

- [ ] **Step 6: Add DB store wrapper**

Create `apps/server/internal/sandbox/store.go`:

```go
package sandbox

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

type Queryer interface {
	CreateWorkspaceSandboxBinding(ctx context.Context, arg db.CreateWorkspaceSandboxBindingParams) (db.WorkspaceSandbox, error)
	LockWorkspaceSandboxBinding(ctx context.Context, workspaceID pgtype.UUID) (db.WorkspaceSandbox, error)
	MarkWorkspaceSandboxCreating(ctx context.Context, workspaceID pgtype.UUID) (db.WorkspaceSandbox, error)
	MarkWorkspaceSandboxRunning(ctx context.Context, arg db.MarkWorkspaceSandboxRunningParams) (db.WorkspaceSandbox, error)
	MarkWorkspaceSandboxUnhealthy(ctx context.Context, arg db.MarkWorkspaceSandboxUnhealthyParams) (db.WorkspaceSandbox, error)
	MarkWorkspaceSandboxTerminated(ctx context.Context, workspaceID pgtype.UUID) (db.WorkspaceSandbox, error)
	TouchWorkspaceSandboxSeen(ctx context.Context, workspaceID pgtype.UUID) (db.WorkspaceSandbox, error)
}

type Store struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewStore(pool *pgxpool.Pool, queries *db.Queries) *Store {
	return &Store{pool: pool, queries: queries}
}
```

Keep tests focused on `Manager` by injecting fake query behavior where possible; integration coverage can exercise the concrete `Store` with PostgreSQL.

- [ ] **Step 7: Add manager tests**

Create `apps/server/internal/sandbox/manager_test.go` with table-driven tests for:

```go
func TestVolumeName(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{0xaa, 0xbb, 0xcc, 0xdd}, Valid: true}
	if got := VolumeName(id); got != "sandbox-ws-aabbccdd-0000-0000-0000-000000000000" {
		t.Fatalf("VolumeName() = %q", got)
	}
}

func TestEnsureSandboxCreatesWhenMissing(t *testing.T) {
	client := &fakeClient{createdID: "sandbox-1"}
	manager := NewManager(client, testSandboxConfig())
	got, err := manager.ensureWithBinding(context.Background(), testWorkspaceID(), Binding{
		Status:     StatusCreating,
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatalf("EnsureSandbox error = %v", err)
	}
	if got.SandboxID != "sandbox-1" {
		t.Fatalf("sandbox id = %q, want sandbox-1", got.SandboxID)
	}
	if client.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", client.createCalls)
	}
}

func TestEnsureSandboxReusesHealthyRunningSandbox(t *testing.T) {
	client := &fakeClient{}
	manager := NewManager(client, testSandboxConfig())
	got, err := manager.ensureWithBinding(context.Background(), testWorkspaceID(), Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatalf("EnsureSandbox error = %v", err)
	}
	if got.SandboxID != "sandbox-1" {
		t.Fatalf("sandbox id = %q, want sandbox-1", got.SandboxID)
	}
	if client.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", client.createCalls)
	}
}

func TestEnsureSandboxReplacesUnhealthySandboxWithSameVolume(t *testing.T) {
	client := &fakeClient{pingErr: errors.New("not found"), createdID: "sandbox-new"}
	manager := NewManager(client, testSandboxConfig())
	got, err := manager.ensureWithBinding(context.Background(), testWorkspaceID(), Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-old",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	})
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
```

Add the fake client and helpers in the same test file:

```go
type fakeClient struct {
	pingErr     error
	createdID   string
	createCalls int
	lastCreate  CreateRequest
}

func (f *fakeClient) Create(ctx context.Context, req CreateRequest) (SandboxInfo, error) {
	f.createCalls++
	f.lastCreate = req
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
```

- [ ] **Step 8: Implement manager**

Create `apps/server/internal/sandbox/manager.go`:

```go
package sandbox

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/config"
)

type Manager struct {
	client Client
	cfg    config.SandboxConfig
}

type WorkspaceSandbox struct {
	WorkspaceID pgtype.UUID
	SandboxID   string
	VolumeName  string
}

type Binding struct {
	SandboxID  string
	VolumeName string
	Status     string
}

func NewManager(client Client, cfg config.SandboxConfig) *Manager {
	return &Manager{client: client, cfg: cfg}
}

func (m *Manager) ensureWithBinding(ctx context.Context, workspaceID pgtype.UUID, edge Binding) (WorkspaceSandbox, error) {
	if edge.SandboxID != "" && edge.Status == StatusRunning {
		if err := m.client.Ping(ctx, edge.SandboxID); err == nil {
			return WorkspaceSandbox{WorkspaceID: workspaceID, SandboxID: edge.SandboxID, VolumeName: edge.VolumeName}, nil
		}
	}
	info, err := m.client.Create(ctx, CreateRequest{
		Image:          m.cfg.Image,
		VolumeName:     edge.VolumeName,
		MountPath:      m.cfg.Workdir,
		TimeoutSeconds: m.cfg.TimeoutSeconds,
		ResourceCPU:    m.cfg.ResourceLimits.CPU,
		ResourceMemory: m.cfg.ResourceLimits.Memory,
	})
	if err != nil {
		return WorkspaceSandbox{}, err
	}
	if info.ID == "" {
		return WorkspaceSandbox{}, errors.New("opensandbox returned empty sandbox id")
	}
	return WorkspaceSandbox{WorkspaceID: workspaceID, SandboxID: info.ID, VolumeName: edge.VolumeName}, nil
}
```

Adapt the exact public method node to the final transaction wrapper. Preserve the tested behavior: DB edge first, ping running sandbox, recreate with the same volume on failure.

- [ ] **Step 9: Run migration and tests**

Run:

```bash
make migrate-up
make sqlc-generate
cd apps/server && go test ./internal/sandbox -v
make server-test
```

Expected:

- Migration applies.
- sqlc generation succeeds.
- Sandbox unit tests pass.
- Full server tests pass.

- [ ] **Step 10: Commit**

```bash
git add apps/server/migrations/005_add_workspace_sandbox.sql apps/server/sqlc/queries/sandbox.sql apps/server/internal/store/db apps/server/internal/sandbox
git commit -m "feat(server): add db-first sandbox manager"
```

## Task 3: Workspace File Preparation

**Files:**
- Create: `apps/server/internal/sandbox/workspace.go`
- Test: `apps/server/internal/sandbox/workspace_test.go`

- [ ] **Step 1: Add manifest tests**

Create `apps/server/internal/sandbox/workspace_test.go`:

```go
package sandbox

import (
	"encoding/json"
	"testing"
)

func TestSafeAssetName(t *testing.T) {
	if got := SafeAssetName("产品 图.png"); got != "------.png" {
		t.Fatalf("SafeAssetName() = %q", got)
	}
	if got := SafeAssetName(""); got != "asset.bin" {
		t.Fatalf("SafeAssetName(empty) = %q", got)
	}
}

func TestBuildManifest(t *testing.T) {
	manifest := WorkspaceManifest{
		WorkspaceID: "workspace-1",
		AssetsDir:   "/workspace/assets",
		OutputDir:   "/workspace/output",
		Assets: []ManifestAsset{{
			ID:    "asset-1",
			Type:  "image",
			Mime:  "image/png",
			Path:  "/workspace/assets/asset-1-product.png",
			Title: "产品主图",
		}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if string(data) == "" {
		t.Fatal("manifest json must not be empty")
	}
	if strings.Contains(string(data), "clipanvil_dev") {
		t.Fatal("manifest must not contain credentials")
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd apps/server && go test ./internal/sandbox -run 'TestSafeAssetName|TestBuildManifest' -v
```

Expected: fail because workspace manifest types and helpers do not exist.

- [ ] **Step 3: Implement workspace helpers**

Create `apps/server/internal/sandbox/workspace.go`:

```go
package sandbox

import (
	"context"
	"encoding/json"
	"strings"
)

const (
	AssetsDir  = "/workspace/assets"
	ScriptsDir = "/workspace/scripts"
	TmpDir     = "/workspace/tmp"
	OutputDir  = "/workspace/output"
)

type WorkspaceManifest struct {
	WorkspaceID string          `json:"workspace_id"`
	AssetsDir   string          `json:"assets_dir"`
	OutputDir   string          `json:"output_dir"`
	Assets      []ManifestAsset `json:"assets"`
}

type ManifestAsset struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Mime  string `json:"mime"`
	Path  string `json:"path"`
	Title string `json:"title"`
}

func SafeAssetName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "asset.bin"
	}
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	if b.Len() == 0 {
		return "asset.bin"
	}
	return b.String()
}

func EnsureWorkspaceLayout(ctx context.Context, client Client, sandboxID string) error {
	_, err := client.Exec(ctx, sandboxID, ExecRequest{
		Command:        "mkdir -p /workspace/assets /workspace/scripts /workspace/tmp /workspace/output",
		Cwd:            "/workspace",
		TimeoutSeconds: 30,
	})
	return err
}

func WriteManifest(ctx context.Context, client Client, sandboxID string, manifest WorkspaceManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return client.Upload(ctx, sandboxID, "/workspace/manifest.json", strings.NewReader(string(data)))
}
```

Add `strings` to the test imports.

- [ ] **Step 4: Add asset preloading structure**

Extend `workspace.go` with:

```go
type AssetForPreload struct {
	ID         string
	Type       string
	Mime       string
	Title      string
	Filename   string
	Reader     io.Reader
}

func PrepareWorkspaceFiles(ctx context.Context, client Client, sandboxID string, workspaceID string, assets []AssetForPreload) (WorkspaceManifest, error) {
	if err := EnsureWorkspaceLayout(ctx, client, sandboxID); err != nil {
		return WorkspaceManifest{}, err
	}
	manifest := WorkspaceManifest{
		WorkspaceID: workspaceID,
		AssetsDir:   AssetsDir,
		OutputDir:   OutputDir,
		Assets:      make([]ManifestAsset, 0, len(assets)),
	}
	for _, asset := range assets {
		path := AssetsDir + "/" + asset.ID + "-" + SafeAssetName(asset.Filename)
		if err := client.Upload(ctx, sandboxID, path, asset.Reader); err != nil {
			return WorkspaceManifest{}, err
		}
		manifest.Assets = append(manifest.Assets, ManifestAsset{
			ID:    asset.ID,
			Type:  asset.Type,
			Mime:  asset.Mime,
			Path:  path,
			Title: asset.Title,
		})
	}
	if err := WriteManifest(ctx, client, sandboxID, manifest); err != nil {
		return WorkspaceManifest{}, err
	}
	return manifest, nil
}
```

Add `io` to imports.

- [ ] **Step 5: Run tests**

Run:

```bash
cd apps/server && go test ./internal/sandbox -run 'TestSafeAssetName|TestBuildManifest' -v
make server-test
```

Expected: tests pass.

- [ ] **Step 6: Commit**

```bash
git add apps/server/internal/sandbox/workspace.go apps/server/internal/sandbox/workspace_test.go
git commit -m "feat(server): prepare sandbox workspace files"
```

## Task 4: sandbox_exec Edge Logic

**Files:**
- Create: `apps/server/internal/sandbox/exec.go`
- Test: `apps/server/internal/sandbox/exec_test.go`

- [ ] **Step 1: Add exec tests**

Create `apps/server/internal/sandbox/exec_test.go`:

```go
package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestBuildExecRequestDefaults(t *testing.T) {
	req, err := BuildExecRequest(ExecInput{Command: "echo ok"})
	if err != nil {
		t.Fatalf("BuildExecRequest error = %v", err)
	}
	if req.Cwd != "/workspace" {
		t.Fatalf("cwd = %q, want /workspace", req.Cwd)
	}
	if req.TimeoutSeconds != 120 {
		t.Fatalf("timeout = %d, want 120", req.TimeoutSeconds)
	}
}

func TestBuildExecRequestRejectsOutsideWorkspace(t *testing.T) {
	_, err := BuildExecRequest(ExecInput{Command: "pwd", Cwd: "/etc"})
	if err == nil {
		t.Fatal("expected error for cwd outside /workspace")
	}
}

func TestTruncateOutput(t *testing.T) {
	out, truncated := TruncateOutput(strings.Repeat("a", 10), 4)
	if !truncated {
		t.Fatal("expected truncated output")
	}
	if out != "aaaa" {
		t.Fatalf("output = %q, want aaaa", out)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
cd apps/server && go test ./internal/sandbox -run 'TestBuildExecRequest|TestTruncateOutput' -v
```

Expected: fail because exec helpers do not exist.

- [ ] **Step 3: Implement exec helpers**

Create `apps/server/internal/sandbox/exec.go`:

```go
package sandbox

import (
	"context"
	"errors"
	"strings"
)

const (
	DefaultExecTimeoutSeconds = 120
	MaxExecTimeoutSeconds     = 1800
	DefaultOutputLimitBytes   = 64 << 10
)

type ExecInput struct {
	Command        string
	Cwd            string
	TimeoutSeconds int
}

func BuildExecRequest(input ExecInput) (ExecRequest, error) {
	command := strings.TrimSpace(input.Command)
	if command == "" {
		return ExecRequest{}, errors.New("command is required")
	}
	cwd := strings.TrimSpace(input.Cwd)
	if cwd == "" {
		cwd = DefaultWorkdir
	}
	if cwd != DefaultWorkdir && !strings.HasPrefix(cwd, DefaultWorkdir+"/") {
		return ExecRequest{}, errors.New("cwd must be inside /workspace")
	}
	timeout := input.TimeoutSeconds
	if timeout == 0 {
		timeout = DefaultExecTimeoutSeconds
	}
	if timeout < 1 || timeout > MaxExecTimeoutSeconds {
		return ExecRequest{}, errors.New("timeout_seconds out of range")
	}
	return ExecRequest{
		Command:        "bash -lc " + shellQuote(command),
		Cwd:            cwd,
		TimeoutSeconds: timeout,
	}, nil
}

func RunExec(ctx context.Context, client Client, sandboxID string, input ExecInput) (ExecResult, error) {
	req, err := BuildExecRequest(input)
	if err != nil {
		return ExecResult{}, err
	}
	result, err := client.Exec(ctx, sandboxID, req)
	if err != nil {
		return ExecResult{}, err
	}
	stdout, stdoutTruncated := TruncateOutput(result.Stdout, DefaultOutputLimitBytes)
	stderr, stderrTruncated := TruncateOutput(result.Stderr, DefaultOutputLimitBytes)
	result.Stdout = stdout
	result.Stderr = stderr
	result.Truncated = result.Truncated || stdoutTruncated || stderrTruncated
	return result, nil
}

func TruncateOutput(s string, limit int) (string, bool) {
	if limit < 0 || len(s) <= limit {
		return s, false
	}
	return s[:limit], true
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
cd apps/server && go test ./internal/sandbox -run 'TestBuildExecRequest|TestTruncateOutput' -v
make server-test
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add apps/server/internal/sandbox/exec.go apps/server/internal/sandbox/exec_test.go
git commit -m "feat(server): add sandbox exec tool logic"
```

## Task 5: submit_artifact Edge Logic

**Files:**
- Create: `apps/server/internal/sandbox/paths.go`
- Create: `apps/server/internal/sandbox/artifact.go`
- Test: `apps/server/internal/sandbox/paths_test.go`
- Test: `apps/server/internal/sandbox/artifact_test.go`
- Modify: `apps/server/sqlc/queries/node.sql`
- Generated: `apps/server/internal/store/db/*`

- [ ] **Step 1: Add path tests**

Create `apps/server/internal/sandbox/paths_test.go`:

```go
package sandbox

import "testing"

func TestValidateOutputPathAcceptsOutputFile(t *testing.T) {
	path, err := ValidateOutputPath("/workspace/output/result.mp4")
	if err != nil {
		t.Fatalf("ValidateOutputPath error = %v", err)
	}
	if path != "/workspace/output/result.mp4" {
		t.Fatalf("path = %q", path)
	}
}

func TestValidateOutputPathRejectsEscape(t *testing.T) {
	for _, input := range []string{"/workspace/output/../secret", "/workspace/assets/a.png", "result.mp4", ""} {
		if _, err := ValidateOutputPath(input); err == nil {
			t.Fatalf("expected reject for %q", input)
		}
	}
}
```

- [ ] **Step 2: Implement path validation**

Create `apps/server/internal/sandbox/paths.go`:

```go
package sandbox

import (
	"errors"
	"path"
	"strings"
)

func ValidateOutputPath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("path is required")
	}
	if !strings.HasPrefix(input, OutputDir+"/") {
		return "", errors.New("artifact path must be inside /workspace/output")
	}
	clean := path.Clean(input)
	if clean == OutputDir || !strings.HasPrefix(clean, OutputDir+"/") {
		return "", errors.New("artifact path must be a file inside /workspace/output")
	}
	if strings.Contains(clean, "/../") {
		return "", errors.New("artifact path must not escape output")
	}
	return clean, nil
}
```

- [ ] **Step 3: Add agent node query**

Extend `apps/server/sqlc/queries/node.sql`:

```sql
-- name: CreateAgentMediaNode :one
INSERT INTO media_node (
    workspace_id,
    node_type,
    title,
    prompt,
    status,
    source,
    asset_id,
    canvas_x,
    canvas_y,
    canvas_w,
    canvas_h
)
VALUES ($1, $2, $3, $4, 'succeeded', 'agent', $5, $6, $7, $8, $9)
RETURNING *;
```

Run:

```bash
make sqlc-generate
```

- [ ] **Step 4: Add artifact service skeleton**

Create `apps/server/internal/sandbox/artifact.go`:

```go
package sandbox

import (
	"context"
	"errors"
	"io"
	"net/http"
)

const MaxArtifactBytes int64 = 500 << 20

type ArtifactInput struct {
	Path      string
	Title     string
	NodeID    string
	MediaType string
}

type ArtifactResult struct {
	AssetID   string `json:"asset_id"`
	NodeID    string `json:"node_id"`
	AccessURL string `json:"access_url"`
}

func DetectMIME(r io.Reader) (string, []byte, error) {
	head := make([]byte, 512)
	n, err := r.Read(head)
	if err != nil && err != io.EOF {
		return "", nil, err
	}
	return http.DetectContentType(head[:n]), head[:n], nil
}

func MediaTypeForArtifactMIME(mime string) (string, bool) {
	switch mime {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return "image", true
	case "video/mp4", "video/quicktime", "video/webm":
		return "video", true
	case "audio/mpeg", "audio/wav", "audio/aac", "audio/ogg":
		return "audio", true
	default:
		return "", false
	}
}

func ValidateArtifactSize(size int64) error {
	if size < 0 {
		return errors.New("artifact size is unknown")
	}
	if size > MaxArtifactBytes {
		return errors.New("artifact too large")
	}
	return nil
}

func SubmitArtifact(ctx context.Context, client Client, sandboxID string, input ArtifactInput) (ArtifactResult, error) {
	artifactPath, err := ValidateOutputPath(input.Path)
	if err != nil {
		return ArtifactResult{}, err
	}
	reader, info, err := client.Download(ctx, sandboxID, artifactPath)
	if err != nil {
		return ArtifactResult{}, err
	}
	defer func() { _ = reader.Close() }()
	if err := ValidateArtifactSize(info.SizeBytes); err != nil {
		return ArtifactResult{}, err
	}
	return ArtifactResult{}, errors.New("artifact persistence is wired in the API/service integration step")
}
```

This task delivers validation and download guardrails. Task 6 wires the validated artifact into MinIO, DB writes, and WebSocket events.

- [ ] **Step 5: Add tests for MIME and size**

Create `apps/server/internal/sandbox/artifact_test.go`:

```go
package sandbox

import "testing"

func TestMediaTypeForArtifactMIME(t *testing.T) {
	if got, ok := MediaTypeForArtifactMIME("image/png"); !ok || got != "image" {
		t.Fatalf("image/png = %q, %v", got, ok)
	}
	if _, ok := MediaTypeForArtifactMIME("text/plain"); ok {
		t.Fatal("text/plain must not be accepted")
	}
}

func TestValidateArtifactSize(t *testing.T) {
	if err := ValidateArtifactSize(MaxArtifactBytes + 1); err == nil {
		t.Fatal("expected oversized artifact to fail")
	}
	if err := ValidateArtifactSize(1024); err != nil {
		t.Fatalf("small artifact failed: %v", err)
	}
}
```

- [ ] **Step 6: Run tests**

Run:

```bash
cd apps/server && go test ./internal/sandbox -run 'TestValidateOutputPath|TestMediaTypeForArtifactMIME|TestValidateArtifactSize' -v
make server-test
```

Expected: tests pass.

- [ ] **Step 7: Commit**

```bash
git add apps/server/internal/sandbox/paths.go apps/server/internal/sandbox/paths_test.go apps/server/internal/sandbox/artifact.go apps/server/internal/sandbox/artifact_test.go apps/server/sqlc/queries/node.sql apps/server/internal/store/db
git commit -m "feat(server): add sandbox artifact validation"
```

## Task 6: End-to-End Wiring and Smoke

**Files:**
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/sandbox/client.go`
- Modify: `apps/server/internal/sandbox/artifact.go`
- Create: `apps/server/internal/api/sandbox_handler.go`
- Test: `apps/server/internal/api/sandbox_handler_test.go`

- [ ] **Step 1: Add OpenSandbox Go SDK**

Run:

```bash
cd apps/server && go get github.com/alibaba/OpenSandbox/sdks/sandbox/go
```

Expected: `apps/server/go.mod` and `apps/server/go.sum` update. If network is blocked, rerun with network permission instead of replacing the SDK with ad hoc HTTP code.

- [ ] **Step 2: Implement SDK-backed client**

Extend `apps/server/internal/sandbox/client.go` with an implementation named `OpenSandboxClient`. Keep all OpenSandbox SDK imports in this file only. Preserve the existing `Client` interface.

Implementation requirements:

- `Create` sends image, timeout, resource limits, and a `pvc` volume mounted at `/workspace`.
- `Ping` verifies the sandbox exists and `execd` is reachable.
- `Exec` calls command execution through server proxy when configured.
- `Upload` and `Download` use OpenSandbox file APIs.
- Convert SDK errors to ordinary Go errors with sandbox ID context.

- [ ] **Step 3: Wire manager in main**

Modify `apps/server/cmd/server/main.go` after MinIO initialization:

```go
sandboxClient := sandbox.NewOpenSandboxClient(cfg.Sandbox)
sandboxManager := sandbox.NewManager(sandboxClient, cfg.Sandbox)
_ = sandboxManager
```

Import:

```go
github.com/sinmaystar/clip-anvil/internal/sandbox
```

Keep `_ = sandboxManager` only until a handler or Agent runtime uses it. Remove the temporary assignment in the same task if a debug handler is added.

- [ ] **Step 4: Add debug status/reset API**

Create `apps/server/internal/api/sandbox_handler.go`:

```go
package api

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/jackc/pgx/v5/pgtype"
)

type SandboxManager interface {
	EnsureSandbox(ctx context.Context, workspaceID pgtype.UUID) (any, error)
	ResetSandbox(ctx context.Context, workspaceID pgtype.UUID) error
}

type SandboxHandler struct {
	manager SandboxManager
}

func NewSandboxHandler(manager SandboxManager) *SandboxHandler {
	return &SandboxHandler{manager: manager}
}

func (h *SandboxHandler) Status(ctx context.Context, c *app.RequestContext) {
	workspaceID, ok := uuidFromString(c.Param("id"))
	if !ok {
		writeError(c, consts.StatusNotFound, "workspace not found")
		return
	}
	info, err := h.manager.EnsureSandbox(ctx, workspaceID)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to ensure sandbox")
		return
	}
	c.JSON(consts.StatusOK, info)
}
```

Before shipping this handler, add workspace ownership checks using the same pattern as `workspaceBelongsToAccount`.

- [ ] **Step 5: Complete artifact persistence**

Extend `SubmitArtifact` or create an `ArtifactService` that receives:

- `queries *db.Queries`
- `minioClient *minio.Client`
- `hub *api.CanvasHub` or a small broadcaster interface

Required behavior:

- Download file from sandbox.
- Detect MIME.
- Map MIME to `db.MediaType`.
- Put object to `workspace-{workspaceID}/artifacts/{timestamp}/{filename}`.
- Create `media_asset`.
- Update provided node or create agent node.
- Broadcast `AssetCreated` and `NodeCreated` or `NodeUpdated`.
- Return `ArtifactResult`.

- [ ] **Step 6: Run full backend verification**

Run:

```bash
make server-build
make server-test
```

Expected: both commands exit 0.

- [ ] **Step 7: Run real smoke**

Start services:

```bash
docker compose -f deploy/docker-compose.yml up -d
make migrate-up
make server-dev
```

In another terminal, use a small script or API client to:

1. Register or log in.
2. Create a workspace.
3. Upload a small image through `POST /api/upload`.
4. Ensure sandbox through the debug API or internal smoke helper.
5. Prepare workspace files.
6. Run `sandbox_exec("python3 - <<'PY'\nfrom pathlib import Path\nPath('/workspace/output/result.txt').write_text('ok')\nPY")`.
7. Use an image or video output for the final artifact once MIME validation is enabled.
8. Submit the artifact.
9. Fetch `/api/workspaces/:id/canvas` and confirm the node exists.

Expected:

- `workspace_sandbox` has the workspace ID, sandbox ID, and `sandbox-ws-{workspaceID}` volume name.
- The generated asset exists in MinIO.
- The generated node appears in canvas API.
- Restarting Go backend does not require in-memory state to continue.

- [ ] **Step 8: Commit**

```bash
git add apps/server/go.mod apps/server/go.sum apps/server/cmd/server/main.go apps/server/internal/sandbox apps/server/internal/api
git commit -m "feat(server): wire opensandbox workspace execution"
```

## Self-Review Checklist

- Spec coverage:
  - Workspace-scoped lifecycle: Task 2 and Task 6.
  - DB as source of truth: Task 2.
  - OpenSandbox infrastructure: Task 1.
  - Workspace layout and asset preloading: Task 3.
  - `sandbox_exec`: Task 4.
  - `submit_artifact`: Task 5 and Task 6.
  - Security limits: Tasks 1, 4, 5, and 6.
  - End-to-end smoke: Task 6.
- Type consistency:
  - All sandbox business code depends on `sandbox.Client`.
  - SDK-specific types stay in the SDK-backed implementation.
  - `WorkspaceSandbox`, `ExecInput`, `ExecResult`, `ArtifactInput`, and `ArtifactResult` are introduced before use.
- Verification:
  - Each implementation task includes a failing test or a clear verification command.
  - Each task ends with a focused commit.
