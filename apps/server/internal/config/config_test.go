package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsJWTConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configData := []byte(`
server:
  port: 9999
postgres:
  dsn: "postgres://test:test@localhost:5432/test?sslmode=disable"
redis:
  addr: "localhost:6379"
minio:
  endpoint: "localhost:9000"
  sandbox_endpoint: "host.docker.internal:9000"
  access_key: "clipanvil"
  secret_key: "clipanvil_dev"
  use_ssl: false
jwt:
  secret: "test-secret"
  expire_hours: 12
`)

	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.JWT.Secret != "test-secret" {
		t.Fatalf("JWT.Secret = %q, want %q", cfg.JWT.Secret, "test-secret")
	}
	if cfg.JWT.ExpireHours != 12 {
		t.Fatalf("JWT.ExpireHours = %d, want %d", cfg.JWT.ExpireHours, 12)
	}
	if cfg.MinIO.SandboxEndpoint != "host.docker.internal:9000" {
		t.Fatalf("MinIO.SandboxEndpoint = %q", cfg.MinIO.SandboxEndpoint)
	}
}

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

func TestLoadAllowsEnvOverrideForServerPort(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configData := []byte(`
server:
  port: 9999
postgres:
  dsn: "postgres://test:test@localhost:5432/test?sslmode=disable"
redis:
  addr: "localhost:6379"
minio:
  endpoint: "localhost:9000"
  sandbox_endpoint: "host.docker.internal:9000"
  access_key: "clipanvil"
  secret_key: "clipanvil_dev"
  use_ssl: false
jwt:
  secret: "test-secret"
  expire_hours: 12
`)

	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(dir)
	t.Setenv("CLIPANVIL_SERVER_PORT", "8891")
	t.Setenv("CLIPANVIL_MINIO_SANDBOX_ENDPOINT", "sandbox-minio:9000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Port != 8891 {
		t.Fatalf("Server.Port = %d, want 8891", cfg.Server.Port)
	}
	if cfg.MinIO.SandboxEndpoint != "sandbox-minio:9000" {
		t.Fatalf("MinIO.SandboxEndpoint = %q", cfg.MinIO.SandboxEndpoint)
	}
}
