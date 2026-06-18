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

func TestLoadProductionConfigDefaultsFromYaml(t *testing.T) {
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
production:
  provider_mode: "mock"
  default_provider: "mock"
  default_text_model: "mock-text"
  volcengine:
    base_url: "https://ark.cn-beijing.volces.com"
    text_model: "doubao-seed-1-6-lite"
    image_model: "seedream-lite"
    video_model: "seedance-lite"
`)

	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Production.ProviderMode != "mock" {
		t.Fatalf("ProviderMode = %q, want mock", cfg.Production.ProviderMode)
	}
	if cfg.Production.DefaultTextModel != "mock-text" {
		t.Fatalf("DefaultTextModel = %q, want mock-text", cfg.Production.DefaultTextModel)
	}
	if cfg.Production.Volcengine.APIKey != "" {
		t.Fatalf("Volcengine.APIKey must not be set by committed yaml")
	}
}

func TestLoadProductionConfigAllowsEnvOverride(t *testing.T) {
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
production:
  provider_mode: "mock"
  default_provider: "mock"
  default_text_model: "mock-text"
  volcengine:
    base_url: "https://example.invalid"
    text_model: "cheap-default"
`)

	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(dir)
	t.Setenv("CLIPANVIL_PRODUCTION_PROVIDER_MODE", "real")
	t.Setenv("CLIPANVIL_PRODUCTION_DEFAULT_PROVIDER", "volcengine")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY", "local-key")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL", "doubao-cheap")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Production.ProviderMode != "real" {
		t.Fatalf("ProviderMode = %q, want real", cfg.Production.ProviderMode)
	}
	if cfg.Production.DefaultProvider != "volcengine" {
		t.Fatalf("DefaultProvider = %q, want volcengine", cfg.Production.DefaultProvider)
	}
	if cfg.Production.Volcengine.APIKey != "local-key" {
		t.Fatalf("Volcengine.APIKey was not loaded from env")
	}
	if cfg.Production.Volcengine.TextModel != "doubao-cheap" {
		t.Fatalf("Volcengine.TextModel = %q, want doubao-cheap", cfg.Production.Volcengine.TextModel)
	}
}

func TestLoadProductionConfigReadsDotEnv(t *testing.T) {
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
production:
  provider_mode: "mock"
  default_provider: "mock"
  default_text_model: "mock-text"
`)

	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	envData := []byte(`
CLIPANVIL_PRODUCTION_PROVIDER_MODE=real
CLIPANVIL_PRODUCTION_DEFAULT_PROVIDER=volcengine
CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY=dotenv-key
`)
	if err := os.WriteFile(filepath.Join(dir, ".env"), envData, 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("CLIPANVIL_PRODUCTION_PROVIDER_MODE")
		_ = os.Unsetenv("CLIPANVIL_PRODUCTION_DEFAULT_PROVIDER")
		_ = os.Unsetenv("CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY")
	}()

	t.Chdir(dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Production.ProviderMode != "real" {
		t.Fatalf("ProviderMode = %q, want real", cfg.Production.ProviderMode)
	}
	if cfg.Production.DefaultProvider != "volcengine" {
		t.Fatalf("DefaultProvider = %q, want volcengine", cfg.Production.DefaultProvider)
	}
	if cfg.Production.Volcengine.APIKey != "dotenv-key" {
		t.Fatalf("Volcengine.APIKey was not loaded from .env")
	}
}
