package cozelooptrace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigValidateRequiresWorkspaceID(t *testing.T) {
	cfg := Config{Endpoint: "http://localhost:19098", Authorization: "Bearer pat"}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing workspace id error")
	}
}

func TestConfigValidateRequiresAuthorization(t *testing.T) {
	cfg := Config{Endpoint: "http://localhost:19098", WorkspaceID: "123"}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing authorization error")
	}
}

func TestConfigOTLPEndpointURLDefaultsAndTrims(t *testing.T) {
	cfg := Config{
		Endpoint:      "http://localhost:19098/",
		WorkspaceID:   "123",
		Authorization: "Bearer pat",
	}

	got, err := cfg.OTLPEndpointURL()
	if err != nil {
		t.Fatalf("OTLPEndpointURL() error = %v", err)
	}
	want := "http://localhost:19098/v1/loop/opentelemetry/v1/traces"
	if got != want {
		t.Fatalf("OTLPEndpointURL() = %q, want %q", got, want)
	}
}

func TestConfigAPIBaseURLUsesEndpointOrigin(t *testing.T) {
	cfg := Config{
		Endpoint:      "http://localhost:19098/v1/loop/opentelemetry/v1/traces?debug=1",
		WorkspaceID:   "123",
		Authorization: "Bearer pat",
	}

	got, err := cfg.APIBaseURL()
	if err != nil {
		t.Fatalf("APIBaseURL() error = %v", err)
	}
	if got != "http://localhost:19098" {
		t.Fatalf("APIBaseURL() = %q, want origin", got)
	}
}

func TestNormalizeAPITokenStripsBearerPrefix(t *testing.T) {
	if got := normalizeAPIToken("Bearer pat-1"); got != "pat-1" {
		t.Fatalf("normalizeAPIToken() = %q, want pat-1", got)
	}
}

func TestLoadDotEnvFilesDoesNotOverrideExistingEnvironment(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("CLIPANVIL_COZELOOP_WORKSPACE_ID=from-file\nCLIPANVIL_COZELOOP_PAT=file-pat\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLIPANVIL_COZELOOP_WORKSPACE_ID", "from-env")

	LoadDotEnvFiles(envPath)

	if got := os.Getenv("CLIPANVIL_COZELOOP_WORKSPACE_ID"); got != "from-env" {
		t.Fatalf("workspace id = %q, want from-env", got)
	}
	if got := os.Getenv("CLIPANVIL_COZELOOP_PAT"); got != "file-pat" {
		t.Fatalf("pat = %q, want file-pat", got)
	}
}

func TestConfigFromEnvUsesPATFallbackAndAgentServiceName(t *testing.T) {
	t.Setenv("CLIPANVIL_COZELOOP_ENDPOINT", "http://localhost:19098")
	t.Setenv("CLIPANVIL_COZELOOP_WORKSPACE_ID", "workspace-1")
	t.Setenv("CLIPANVIL_COZELOOP_PAT", "pat-1")

	cfg := ConfigFromEnv()

	if cfg.Endpoint != "http://localhost:19098" {
		t.Fatalf("endpoint = %q", cfg.Endpoint)
	}
	if cfg.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace id = %q", cfg.WorkspaceID)
	}
	if cfg.Authorization != "pat-1" {
		t.Fatalf("authorization = %q", cfg.Authorization)
	}
	if cfg.ServiceName != "clipanvil-agent" {
		t.Fatalf("service name = %q", cfg.ServiceName)
	}
}
