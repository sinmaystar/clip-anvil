package cozelooptrace

import "testing"

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
