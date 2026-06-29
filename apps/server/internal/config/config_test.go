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
    base_url: "https://ark.cn-beijing.volces.com/api/v3"
    region: "cn-beijing"
    text_model: "doubao-seed-2-0-mini-260428"
    image_model: "doubao-seedream-5-0-260128"
    video_model: "doubao-seedance-1-0-pro-fast-251015"
    video_resolution_override: "480p"
    audio_model: ""
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
	if cfg.Production.Volcengine.AudioAPIKey != "" {
		t.Fatalf("Volcengine.AudioAPIKey must not be set by committed yaml")
	}
	if cfg.Production.Volcengine.Region != "cn-beijing" {
		t.Fatalf("Volcengine.Region = %q, want cn-beijing", cfg.Production.Volcengine.Region)
	}
	if cfg.Production.Volcengine.TextModel != "doubao-seed-2-0-mini-260428" {
		t.Fatalf("Volcengine.TextModel = %q", cfg.Production.Volcengine.TextModel)
	}
	if cfg.Production.Volcengine.ImageModel != "doubao-seedream-5-0-260128" {
		t.Fatalf("Volcengine.ImageModel = %q", cfg.Production.Volcengine.ImageModel)
	}
	if cfg.Production.Volcengine.VideoModel != "doubao-seedance-1-0-pro-fast-251015" {
		t.Fatalf("Volcengine.VideoModel = %q", cfg.Production.Volcengine.VideoModel)
	}
	if cfg.Production.Volcengine.VideoResolutionOverride != "480p" {
		t.Fatalf("Volcengine.VideoResolutionOverride = %q", cfg.Production.Volcengine.VideoResolutionOverride)
	}
	if cfg.Production.Volcengine.AudioModel != "" {
		t.Fatalf("Volcengine.AudioModel = %q, want empty", cfg.Production.Volcengine.AudioModel)
	}
	if cfg.Production.Volcengine.AudioBaseURL != "" {
		t.Fatalf("Volcengine.AudioBaseURL = %q, want empty", cfg.Production.Volcengine.AudioBaseURL)
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
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_API_KEY", "speech-key")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL", "doubao-cheap")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_VIDEO_RESOLUTION_OVERRIDE", "480p")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_REGION", "cn-beijing")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_MODEL", "seed-audio-1.0")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_BASE_URL", "https://openspeech.bytedance.com/api/v3")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_ACCESS_KEY_ID", "tos-ak")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_SECRET_ACCESS_KEY", "tos-sk")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_BUCKET", "clip-anvil-temp-bucket")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_ENDPOINT", "tos-cn-beijing.volces.com")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_REGION", "cn-beijing")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_PUBLIC_BASE_URL", "https://clip-anvil-temp-bucket.tos-cn-beijing.volces.com")
	t.Setenv("CLIPANVIL_PRODUCTION_VOLCENGINE_TOS_SIGNED_URL_TTL_SECONDS", "3600")
	t.Setenv("CLIPANVIL_PRODUCTION_WORKER_CONCURRENCY", "2")
	t.Setenv("CLIPANVIL_PRODUCTION_PROVIDER_POLL_INTERVAL_SECONDS", "5")
	t.Setenv("CLIPANVIL_PRODUCTION_PROVIDER_MAX_POLL_SECONDS", "1800")

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
	if cfg.Production.Volcengine.AudioAPIKey != "speech-key" {
		t.Fatalf("Volcengine.AudioAPIKey was not loaded from env")
	}
	if cfg.Production.Volcengine.TextModel != "doubao-cheap" {
		t.Fatalf("Volcengine.TextModel = %q, want doubao-cheap", cfg.Production.Volcengine.TextModel)
	}
	if cfg.Production.Volcengine.VideoResolutionOverride != "480p" {
		t.Fatalf("Volcengine.VideoResolutionOverride = %q, want 480p", cfg.Production.Volcengine.VideoResolutionOverride)
	}
	if cfg.Production.Volcengine.Region != "cn-beijing" {
		t.Fatalf("Volcengine.Region = %q, want cn-beijing", cfg.Production.Volcengine.Region)
	}
	if cfg.Production.Volcengine.AudioModel != "seed-audio-1.0" {
		t.Fatalf("Volcengine.AudioModel = %q, want seed-audio-1.0", cfg.Production.Volcengine.AudioModel)
	}
	if cfg.Production.Volcengine.AudioBaseURL != "https://openspeech.bytedance.com/api/v3" {
		t.Fatalf("Volcengine.AudioBaseURL = %q", cfg.Production.Volcengine.AudioBaseURL)
	}
	if cfg.Production.Volcengine.TOS.AccessKeyID != "tos-ak" {
		t.Fatalf("Volcengine.TOS.AccessKeyID was not loaded from env")
	}
	if cfg.Production.Volcengine.TOS.SecretAccessKey != "tos-sk" {
		t.Fatalf("Volcengine.TOS.SecretAccessKey was not loaded from env")
	}
	if cfg.Production.Volcengine.TOS.Bucket != "clip-anvil-temp-bucket" {
		t.Fatalf("Volcengine.TOS.Bucket = %q", cfg.Production.Volcengine.TOS.Bucket)
	}
	if cfg.Production.Volcengine.TOS.Endpoint != "tos-cn-beijing.volces.com" {
		t.Fatalf("Volcengine.TOS.Endpoint = %q", cfg.Production.Volcengine.TOS.Endpoint)
	}
	if cfg.Production.Volcengine.TOS.Region != "cn-beijing" {
		t.Fatalf("Volcengine.TOS.Region = %q", cfg.Production.Volcengine.TOS.Region)
	}
	if cfg.Production.Volcengine.TOS.PublicBaseURL != "https://clip-anvil-temp-bucket.tos-cn-beijing.volces.com" {
		t.Fatalf("Volcengine.TOS.PublicBaseURL = %q", cfg.Production.Volcengine.TOS.PublicBaseURL)
	}
	if cfg.Production.Volcengine.TOS.SignedURLTTLSeconds != 3600 {
		t.Fatalf("Volcengine.TOS.SignedURLTTLSeconds = %d", cfg.Production.Volcengine.TOS.SignedURLTTLSeconds)
	}
	if cfg.Production.WorkerConcurrency != 2 {
		t.Fatalf("WorkerConcurrency = %d, want 2", cfg.Production.WorkerConcurrency)
	}
	if cfg.Production.ProviderPollIntervalSeconds != 5 {
		t.Fatalf("ProviderPollIntervalSeconds = %d, want 5", cfg.Production.ProviderPollIntervalSeconds)
	}
	if cfg.Production.ProviderMaxPollSeconds != 1800 {
		t.Fatalf("ProviderMaxPollSeconds = %d, want 1800", cfg.Production.ProviderMaxPollSeconds)
	}
}

func TestLoadBindsAgentExecutionConfig(t *testing.T) {
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
agent:
  producer_max_tool_calls: 1000
  tool_timeout_seconds: 300
`)
	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Chdir(dir)
	t.Setenv("CLIPANVIL_AGENT_PRODUCER_MAX_TOOL_CALLS", "77")
	t.Setenv("CLIPANVIL_AGENT_TOOL_TIMEOUT_SECONDS", "321")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Agent.ProducerMaxToolCalls != 77 {
		t.Fatalf("ProducerMaxToolCalls = %d, want 77", cfg.Agent.ProducerMaxToolCalls)
	}
	if cfg.Agent.ToolTimeoutSeconds != 321 {
		t.Fatalf("ToolTimeoutSeconds = %d, want 321", cfg.Agent.ToolTimeoutSeconds)
	}
}

func TestLoadDefaultsAgentExecutionConfig(t *testing.T) {
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
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Agent.ProducerMaxToolCalls != 1000 {
		t.Fatalf("ProducerMaxToolCalls = %d, want 1000", cfg.Agent.ProducerMaxToolCalls)
	}
	if cfg.Agent.ToolTimeoutSeconds != 300 {
		t.Fatalf("ToolTimeoutSeconds = %d, want 300", cfg.Agent.ToolTimeoutSeconds)
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
