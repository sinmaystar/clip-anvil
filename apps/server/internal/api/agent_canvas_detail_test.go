package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestBuildAgentCanvasDetailRejectsUnsupportedObjectType(t *testing.T) {
	_, err := buildAgentCanvasDetail(
		context.Background(),
		db.New(fakeAgentDBTX{}),
		nil,
		uuidWithByteForWorkbenchTest(1),
		"legacy_tool",
		uuidToString(uuidWithByteForWorkbenchTest(2)),
	)
	if err == nil {
		t.Fatal("expected unsupported object_type error")
	}
	var detailErr agentCanvasDetailError
	if !errors.As(err, &detailErr) {
		t.Fatalf("error type = %T, want agentCanvasDetailError", err)
	}
	if detailErr.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", detailErr.Status)
	}
}

func TestAgentCanvasDetailDefinesFinalOutputDetailShape(t *testing.T) {
	if agentCanvasObjectFinalOutput != "final_output" {
		t.Fatalf("final output object type = %q", agentCanvasObjectFinalOutput)
	}
	var detail agentCanvasDetailResponse
	if detail.FinalOutput != nil {
		t.Fatalf("zero detail final output = %#v", detail.FinalOutput)
	}
}

func TestAgentCanvasDetailDefinesAudioFields(t *testing.T) {
	overview := agentCanvasOverviewDetailResponse{
		AudioPlan: &agentWorkbenchAudioPlanResponse{
			Status:          "composing",
			VoiceoverStatus: "succeeded",
			BGMStatus:       "running",
		},
	}
	if overview.AudioPlan == nil || overview.AudioPlan.BGMStatus != "running" {
		t.Fatalf("overview audio plan = %#v", overview.AudioPlan)
	}

	finalOutput := agentCanvasFinalOutputDetailResponse{
		AudioSummary: &agentWorkbenchAudioSummaryResponse{
			HasVoiceover: true,
			HasBGM:       true,
			AudioCodec:   "aac",
			TrackCount:   2,
			Ducking:      true,
		},
		AudioTracks: []agentWorkbenchAudioTrackResponse{{Role: "voiceover"}, {Role: "bgm", Ducking: true}},
		FinalReviews: []agentWorkbenchReviewSummaryResponse{{
			ReviewTask:  "final_video_review",
			TargetPhase: "final_video",
			Status:      "accepted_with_warnings",
		}},
		Issues: []agentWorkbenchIssueSummaryResponse{{Dimension: "audio_sync", Severity: "medium"}},
	}
	if finalOutput.AudioSummary == nil || finalOutput.AudioSummary.AudioCodec != "aac" || len(finalOutput.AudioTracks) != 2 || len(finalOutput.FinalReviews) != 1 || len(finalOutput.Issues) != 1 {
		t.Fatalf("final output audio fields = %#v", finalOutput)
	}
}

func TestAgentCanvasDetailDefinesReferenceVideoAnalysisShape(t *testing.T) {
	if agentCanvasObjectReferenceVideoAnalysis != "reference_video_analysis" {
		t.Fatalf("reference video analysis object type = %q", agentCanvasObjectReferenceVideoAnalysis)
	}
	var detail agentCanvasDetailResponse
	if detail.ReferenceVideoAnalysis != nil {
		t.Fatalf("zero detail reference video analysis = %#v", detail.ReferenceVideoAnalysis)
	}
}

func TestBuildAgentCanvasDetailRejectsInvalidObjectID(t *testing.T) {
	_, err := buildAgentCanvasDetail(
		context.Background(),
		db.New(fakeAgentDBTX{}),
		nil,
		uuidWithByteForWorkbenchTest(1),
		agentCanvasObjectShot,
		"not-a-uuid",
	)
	if err == nil {
		t.Fatal("expected invalid object_id error")
	}
	var detailErr agentCanvasDetailError
	if !errors.As(err, &detailErr) {
		t.Fatalf("error type = %T, want agentCanvasDetailError", err)
	}
	if detailErr.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", detailErr.Status)
	}
}

func TestAgentCanvasDetailMediaNodeResponseUsesPresentationFields(t *testing.T) {
	node := db.MediaNode{
		ID:               uuidWithByteForWorkbenchTest(11),
		WorkspaceID:      uuidWithByteForWorkbenchTest(12),
		NodeType:         db.NodeTypeImage,
		Title:            "shot_01 preview",
		Status:           db.NodeStatusFailed,
		Prompt:           "cinematic suitcase preview",
		Source:           "agent",
		OperationType:    "image_generation",
		ShotID:           uuidWithByteForWorkbenchTest(13),
		AssetID:          uuidWithByteForWorkbenchTest(14),
		ModelProvider:    pgtype.Text{String: "volcengine", Valid: true},
		ModelID:          pgtype.Text{String: "doubao-seedream-4-0", Valid: true},
		ModelParams:      []byte(`{"aspect_ratio":"9:16"}`),
		CurrentVersionID: uuidWithByteForWorkbenchTest(15),
		Metadata:         []byte(`{"agent_artifact_kind":"preview_image"}`),
	}

	got := mediaNodeDetail(node)
	if got.ID != uuidToString(node.ID) || got.NodeType != "image" || got.Status != "failed" {
		t.Fatalf("media node detail = %#v", got)
	}
	if got.ModelProvider != "volcengine" || got.ModelID != "doubao-seedream-4-0" {
		t.Fatalf("model fields = %q/%q", got.ModelProvider, got.ModelID)
	}
	if got.ModelParams == nil || got.Metadata == nil {
		t.Fatalf("json fields should be decoded: %#v", got)
	}
}

func TestAgentCanvasDetailKeyElementStateNeedsReferenceReason(t *testing.T) {
	state := db.KeyElementState{
		ID:              uuidWithByteForWorkbenchTest(21),
		KeyElementID:    uuidWithByteForWorkbenchTest(22),
		ClientKey:       "state_soft_light_room",
		Label:           "柔光房间",
		ReferenceStatus: "needs_reference",
		Status:          "active",
	}

	summary := keyElementStateSummary(state)
	if summary.ReferenceStatus != "needs_reference" {
		t.Fatalf("reference status = %q", summary.ReferenceStatus)
	}
	if summary.Label != "柔光房间" {
		t.Fatalf("label = %q", summary.Label)
	}
}
