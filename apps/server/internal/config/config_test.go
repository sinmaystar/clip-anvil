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

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Port != 8891 {
		t.Fatalf("Server.Port = %d, want 8891", cfg.Server.Port)
	}
}
