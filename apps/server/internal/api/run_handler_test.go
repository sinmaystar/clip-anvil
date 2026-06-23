package api

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/production"
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

func TestSourceMaterialNodeCannotRun(t *testing.T) {
	if !isSourceMaterialNode(db.MediaNode{
		NodeType:      db.NodeTypeImage,
		OperationType: "upload",
		AssetID:       pgtype.UUID{Valid: true},
	}) {
		t.Fatal("upload asset node should be source material")
	}
	if !isSourceMaterialNode(db.MediaNode{
		NodeType:      db.NodeTypeText,
		OperationType: "manual",
	}) {
		t.Fatal("manual text node should be source material")
	}
	if isSourceMaterialNode(db.MediaNode{
		NodeType:      db.NodeTypeImage,
		OperationType: "text_to_image",
	}) {
		t.Fatal("generated image node should remain runnable")
	}
}

func TestSourceMaterialProductionStateIsEmpty(t *testing.T) {
	node := db.MediaNode{
		ID:            pgtype.UUID{Bytes: [16]byte{0x51}, Valid: true},
		NodeType:      db.NodeTypeVideo,
		Title:         "用户视频",
		OperationType: "upload",
		AssetID:       pgtype.UUID{Bytes: [16]byte{0x52}, Valid: true},
		Status:        db.NodeStatusSucceeded,
	}

	resp := sourceMaterialProductionState(node)
	if len(resp.Versions) != 0 || resp.LatestJob != nil || resp.CurrentVersion != nil {
		t.Fatalf("source material production state should be empty: %#v", resp)
	}
	if len(resp.ActiveStaleReasons) != 0 || len(resp.SandboxJobs) != 0 {
		t.Fatalf("source material state should not include run side data: %#v", resp)
	}
}

func TestRunNodeResponseCanRepresentAsyncQueuedJob(t *testing.T) {
	resp := runNodeResponse{
		Job: generationJobResponse{
			ID:              "job-1",
			Status:          "queued",
			Provider:        "volcengine",
			ModelID:         "doubao-seed-2-0-mini-260428",
			Attempt:         1,
			MaxAttempts:     1,
			RequestedByType: "user",
		},
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["node"]; ok {
		t.Fatalf("async queued response must omit node: %s", raw)
	}
	if _, ok := got["version"]; ok {
		t.Fatalf("async queued response must omit version: %s", raw)
	}
	job := got["job"].(map[string]any)
	if job["status"] != "queued" {
		t.Fatalf("job status = %v", job["status"])
	}
}

func TestToReviewRecordResponseExposesRubricAndCritique(t *testing.T) {
	record := db.ReviewRecord{
		ID:                  pgtype.UUID{Bytes: [16]byte{0x61}, Valid: true},
		ShotID:              pgtype.UUID{Bytes: [16]byte{0x62}, Valid: true},
		NodeID:              pgtype.UUID{Bytes: [16]byte{0x63}, Valid: true},
		ArtifactVersionID:   pgtype.UUID{Bytes: [16]byte{0x64}, Valid: true},
		TargetPhase:         "preview_image",
		Status:              "rejected",
		AttemptNo:           1,
		MaxAttempts:         3,
		OverallScore:        pgtype.Float4{Float32: 0.52, Valid: true},
		Rubric:              []byte(`{"visual_quality":{"score":0.4,"pass":false}}`),
		Critique:            "商品不够清晰",
		RetryRecommendation: []byte(`{"should_retry":true}`),
	}

	resp := toReviewRecordResponse(record)
	if resp.Status != "rejected" || resp.OverallScore == nil || *resp.OverallScore != 0.52 {
		t.Fatalf("response = %#v", resp)
	}
	if resp.Rubric["visual_quality"].(map[string]any)["pass"] != false {
		t.Fatalf("rubric = %#v", resp.Rubric)
	}
	if resp.Critique != "商品不够清晰" || resp.RetryRecommendation["should_retry"] != true {
		t.Fatalf("response = %#v", resp)
	}
}

func TestStatusForProductionEvent(t *testing.T) {
	cases := map[string]string{
		production.ProductionEventJobStarted:   "running",
		production.ProductionEventJobSucceeded: "succeeded",
		production.ProductionEventJobFailed:    "failed",
		production.ProductionEventJobCancelled: "cancelled",
	}
	for eventType, want := range cases {
		if got := statusForProductionEvent(eventType); got != want {
			t.Fatalf("statusForProductionEvent(%q) = %q, want %q", eventType, got, want)
		}
	}
}

func TestShouldBroadcastProductionNodeSnapshot(t *testing.T) {
	for _, eventType := range []string{
		production.ProductionEventJobSucceeded,
		production.ProductionEventJobFailed,
		production.ProductionEventJobCancelled,
	} {
		if !shouldBroadcastProductionNodeSnapshot(eventType) {
			t.Fatalf("expected node snapshot for %s", eventType)
		}
	}
	for _, eventType := range []string{
		production.ProductionEventJobStarted,
		production.ProductionEventProviderProgress,
		production.ProductionEventModelStreamDelta,
	} {
		if shouldBroadcastProductionNodeSnapshot(eventType) {
			t.Fatalf("did not expect node snapshot for %s", eventType)
		}
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
