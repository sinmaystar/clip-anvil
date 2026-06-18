package api

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestNewRunHandlerCreatesHandler(t *testing.T) {
	handler := NewRunHandler(nil, nil)
	if handler == nil {
		t.Fatal("expected handler")
	}
}

func TestRunJobResponseExposesIntentAndSummaries(t *testing.T) {
	job := db.GenerationJob{
		OperationType:    "text_generation",
		Provider:         "mock",
		ModelID:          "mock-text",
		Intent:           []byte(`{"model":{"provider":"mock","model_id":"mock-text"}}`),
		RenderedPrompt:   "write a short ad",
		ProviderRequest:  []byte(`{"provider":"mock"}`),
		ProviderResponse: []byte(`{"text":"ok"}`),
		Status:           db.JobStatusSucceeded,
		Attempt:          2,
		MaxAttempts:      3,
		RequestedByType:  "user",
		RequestedByID:    pgtype.Text{String: "account-123", Valid: true},
	}

	resp := toGenerationJobResponse(job)
	if resp.Provider != "mock" || resp.ModelID != "mock-text" {
		t.Fatalf("provider/model = %s/%s", resp.Provider, resp.ModelID)
	}
	if resp.Intent["model"].(map[string]any)["provider"] != "mock" {
		t.Fatalf("intent = %#v", resp.Intent)
	}
	if resp.RenderedPrompt != "write a short ad" {
		t.Fatalf("rendered prompt = %q", resp.RenderedPrompt)
	}
	if resp.RequestedByID != "account-123" {
		t.Fatalf("requested by id = %q", resp.RequestedByID)
	}
	if resp.Attempt != 2 || resp.MaxAttempts != 3 {
		t.Fatalf("attempts = %d/%d, want 2/3", resp.Attempt, resp.MaxAttempts)
	}
}

func TestRunNodeRequestDefaultsMaxAttempts(t *testing.T) {
	req := runNodeRequest{}
	if got := req.runOptions().MaxAttempts; got != 1 {
		t.Fatalf("max attempts = %d, want 1", got)
	}
}

func TestRunNodeRequestCapsMaxAttempts(t *testing.T) {
	req := runNodeRequest{MaxAttempts: 99}
	if got := req.runOptions().MaxAttempts; got != 3 {
		t.Fatalf("max attempts = %d, want 3", got)
	}
}

func TestStaleReasonResponseExposesDetails(t *testing.T) {
	reason := db.NodeStaleReason{
		ID:                pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true},
		NodeID:            pgtype.UUID{Bytes: [16]byte{0x02}, Valid: true},
		UpstreamNodeID:    pgtype.UUID{Bytes: [16]byte{0x03}, Valid: true},
		UpstreamVersionID: pgtype.UUID{Bytes: [16]byte{0x04}, Valid: true},
		ReasonCode:        "upstream_current_version_changed",
		ReasonMessage:     "Upstream dependency current version changed.",
		Details:           []byte(`{"upstream_node_id":"node-a","current_input_hash":"sha256:new"}`),
	}

	resp := toStaleReasonResponse(reason)
	if resp.ReasonCode != "upstream_current_version_changed" {
		t.Fatalf("reason code = %q", resp.ReasonCode)
	}
	if resp.UpstreamNodeID == "" || resp.UpstreamVersionID == "" {
		t.Fatalf("upstream ids should be exposed: %#v", resp)
	}
	if resp.Details["upstream_node_id"] != "node-a" {
		t.Fatalf("details = %#v", resp.Details)
	}
}

func TestToArtifactVersionResponseIncludesTextAsset(t *testing.T) {
	versionID := pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true}
	assetID := pgtype.UUID{Bytes: [16]byte{0x02}, Valid: true}
	version := db.ArtifactVersion{
		ID:        versionID,
		AssetID:   assetID,
		VersionNo: 2,
		Winner:    true,
		InputHash: "sha256:test",
		Output:    []byte(`{"text_preview":"hello"}`),
	}
	asset := db.MediaAsset{
		ID:          assetID,
		Type:        db.AssetTypeText,
		Mime:        "text/plain; charset=utf-8",
		TextContent: pgtype.Text{String: "hello world", Valid: true},
	}
	got := toArtifactVersionResponse(version, &asset, "")
	if got.ID == "" || got.Asset == nil || got.Asset.TextContent != "hello world" {
		t.Fatalf("response = %#v", got)
	}
	if !got.Winner || got.VersionNo != 2 {
		t.Fatalf("winner/version = %v/%d", got.Winner, got.VersionNo)
	}
}

func TestToSandboxJobResponseIncludesExecutionDetails(t *testing.T) {
	job := db.SandboxJob{
		ID:            pgtype.UUID{Bytes: [16]byte{0x03}, Valid: true},
		WorkspaceID:   pgtype.UUID{Bytes: [16]byte{0x04}, Valid: true},
		JobType:       "internal_media",
		OperationType: "extract_first_frame",
		Status:        db.JobStatusSucceeded,
		SandboxID:     pgtype.Text{String: "sandbox-1", Valid: true},
		Command:       "ffmpeg -y ...",
		Cwd:           "/workspace",
		Input:         []byte(`{"mode":"first"}`),
		Output:        []byte(`{"mime":"image/png"}`),
		ExitCode:      pgtype.Int4{Int32: 0, Valid: true},
		Stdout:        "ok",
		DurationMs:    12,
	}
	got := toSandboxJobResponse(job)
	if got.ID == "" || got.SandboxID != "sandbox-1" || got.ExitCode == nil || *got.ExitCode != 0 {
		t.Fatalf("response = %#v", got)
	}
	if got.Input["mode"] != "first" || got.Output["mime"] != "image/png" {
		t.Fatalf("input/output = %#v/%#v", got.Input, got.Output)
	}
}
