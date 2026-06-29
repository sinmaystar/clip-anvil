package api

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestAgentWorkbenchSelectsBestArtifactNode(t *testing.T) {
	shotID := uuidWithByteForWorkbenchTest(1)
	oldSucceeded := db.MediaNode{
		ID:               uuidWithByteForWorkbenchTest(11),
		ShotID:           shotID,
		Source:           "agent",
		NodeType:         db.NodeTypeImage,
		Title:            "old preview",
		Status:           db.NodeStatusSucceeded,
		CurrentVersionID: uuidWithByteForWorkbenchTest(101),
		Metadata:         []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt:        pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true},
	}
	running := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(12),
		ShotID:    shotID,
		Source:    "agent",
		NodeType:  db.NodeTypeImage,
		Title:     "running preview",
		Status:    db.NodeStatusRunning,
		Metadata:  []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(200, 0), Valid: true},
	}
	got := bestAgentArtifactNode([]db.MediaNode{running, oldSucceeded}, "preview_image")
	if got == nil || got.ID != oldSucceeded.ID {
		t.Fatalf("best node = %#v, want succeeded current version node", got)
	}
}

func TestAgentWorkbenchSelectsNewestNodeWithinSameRank(t *testing.T) {
	older := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(21),
		Source:    "agent",
		NodeType:  db.NodeTypeImage,
		Status:    db.NodeStatusFailed,
		Metadata:  []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true},
	}
	newer := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(22),
		Source:    "agent",
		NodeType:  db.NodeTypeImage,
		Status:    db.NodeStatusFailed,
		Metadata:  []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(200, 0), Valid: true},
	}
	got := bestAgentArtifactNode([]db.MediaNode{older, newer}, "preview_image")
	if got == nil || got.ID != newer.ID {
		t.Fatalf("best node = %#v, want newest same-rank node", got)
	}
}

func TestAgentWorkbenchKeepsMultipleArtifactNodes(t *testing.T) {
	previewA := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(31),
		Source:    "agent",
		NodeType:  db.NodeTypeImage,
		Title:     "preview A",
		Status:    db.NodeStatusSucceeded,
		Metadata:  []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true},
	}
	previewB := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(32),
		Source:    "agent",
		NodeType:  db.NodeTypeImage,
		Title:     "preview B",
		Status:    db.NodeStatusRunning,
		Metadata:  []byte(`{"agent_artifact_kind":"preview_image"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(200, 0), Valid: true},
	}
	video := db.MediaNode{
		ID:        uuidWithByteForWorkbenchTest(33),
		Source:    "agent",
		NodeType:  db.NodeTypeVideo,
		Title:     "video",
		Status:    db.NodeStatusQueued,
		Metadata:  []byte(`{"agent_artifact_kind":"shot_video"}`),
		UpdatedAt: pgtype.Timestamptz{Time: time.Unix(300, 0), Valid: true},
	}

	got := agentArtifactNodes([]db.MediaNode{video, previewB, previewA})
	if len(got) != 3 {
		t.Fatalf("artifact node count = %d, want 3", len(got))
	}
	if got[0].ID != previewB.ID || got[1].ID != previewA.ID || got[2].ID != video.ID {
		t.Fatalf("artifact order = %#v", got)
	}
}

func TestAgentWorkbenchMissingArtifactSlot(t *testing.T) {
	slot := agentWorkbenchArtifactSlotResponse{Kind: "shot_video", Status: "missing"}
	if slot.Kind != "shot_video" || slot.Status != "missing" || slot.NodeID != "" {
		t.Fatalf("slot = %#v", slot)
	}
}

func TestAgentWorkbenchArtifactSlotIncludesAssetDimensions(t *testing.T) {
	assetID := uuidWithByteForWorkbenchTest(41)
	versionID := uuidWithByteForWorkbenchTest(42)
	node := db.MediaNode{
		ID:               uuidWithByteForWorkbenchTest(43),
		Source:           "agent",
		NodeType:         db.NodeTypeImage,
		Title:            "vertical preview",
		Status:           db.NodeStatusSucceeded,
		CurrentVersionID: versionID,
		Metadata:         []byte(`{"agent_artifact_kind":"preview_image"}`),
	}
	version := db.ArtifactVersion{
		ID:      versionID,
		AssetID: assetID,
	}
	asset := db.MediaAsset{
		ID:          assetID,
		Type:        db.AssetTypeImage,
		Mime:        "image/png",
		Metadata:    []byte(`{"width":900,"height":1600}`),
		StorageUrl:  pgtype.Text{String: "s3://bucket/vertical.png", Valid: true},
		WorkspaceID: uuidWithByteForWorkbenchTest(44),
	}

	slot, err := agentWorkbenchArtifactSlotFromNode(
		context.Background(),
		nil,
		node,
		map[pgtype.UUID]db.MediaAsset{assetID: asset},
		map[pgtype.UUID]db.ArtifactVersion{versionID: version},
	)
	if err != nil {
		t.Fatalf("agentWorkbenchArtifactSlotFromNode error = %v", err)
	}
	if slot.Width != 900 || slot.Height != 1600 {
		t.Fatalf("slot dimensions = %dx%d, want 900x1600", slot.Width, slot.Height)
	}
}

func TestAgentWorkbenchFinalOutputFromTimelinePlan(t *testing.T) {
	workspaceID := uuidWithByteForWorkbenchTest(50)
	outputNodeID := uuidWithByteForWorkbenchTest(51)
	versionID := uuidWithByteForWorkbenchTest(52)
	assetID := uuidWithByteForWorkbenchTest(53)
	plan := db.TimelinePlan{
		ID:                uuidWithByteForWorkbenchTest(54),
		WorkspaceID:       workspaceID,
		OutputNodeID:      outputNodeID,
		ArtifactVersionID: versionID,
		SandboxJobID:      uuidWithByteForWorkbenchTest(55),
		Status:            "completed",
		TemplateKey:       "concat_with_fades",
		PlanJson:          []byte(`{"template_key":"concat_with_fades","audio_tracks":[{"role":"voiceover","asset_id":"voice-asset","workspace_path":"/workspace/input/voiceover.mp3","duration_sec":12,"volume":1},{"role":"bgm","asset_id":"bgm-asset","workspace_path":"/workspace/input/bgm.mp3","duration_sec":12,"volume":0.28,"ducking":{"sidechain_role":"voiceover"}}],"output":{"audio_codec":"aac"}}`),
		Result:            []byte(`{"summary":"rendered with fades"}`),
		UpdatedAt:         pgtype.Timestamptz{Time: time.Unix(300, 0), Valid: true},
	}
	node := db.MediaNode{
		ID:               outputNodeID,
		WorkspaceID:      workspaceID,
		NodeType:         db.NodeTypeVideo,
		Title:            "Agent final video",
		Status:           db.NodeStatusSucceeded,
		CurrentVersionID: versionID,
	}
	version := db.ArtifactVersion{ID: versionID, NodeID: outputNodeID, AssetID: assetID}
	asset := db.MediaAsset{
		ID:          assetID,
		WorkspaceID: workspaceID,
		Type:        db.AssetTypeVideo,
		Mime:        "video/mp4",
		StorageUrl:  pgtype.Text{String: "workspace/final.mp4", Valid: true},
	}

	review := db.ReviewRecord{
		ID:                uuidWithByteForWorkbenchTest(56),
		WorkspaceID:       workspaceID,
		NodeID:            outputNodeID,
		ArtifactVersionID: versionID,
		ReviewTask:        "final_video_review",
		TargetPhase:       "final_video",
		Status:            "accepted_with_warnings",
		OverallScore:      pgtype.Float4{Float32: 0.86, Valid: true},
		CreatedAt:         pgtype.Timestamptz{Time: time.Unix(400, 0), Valid: true},
	}

	out := agentWorkbenchFinalOutputFromTimelinePlan(context.Background(), nil, plan, map[pgtype.UUID]db.MediaNode{outputNodeID: node}, map[pgtype.UUID]db.ArtifactVersion{versionID: version}, map[pgtype.UUID]db.MediaAsset{assetID: asset}, []db.ReviewRecord{review})

	if out == nil || out.TimelinePlanID != uuidToString(plan.ID) || out.TemplateKey != "concat_with_fades" || out.Summary != "rendered with fades" {
		t.Fatalf("final output = %#v", out)
	}
	if out.OutputNodeID != uuidToString(outputNodeID) || out.ArtifactVersionID != uuidToString(versionID) || out.AssetID != uuidToString(assetID) {
		t.Fatalf("final output ids = %#v", out)
	}
	if out.Plan["template_key"] != "concat_with_fades" || out.Result["summary"] != "rendered with fades" {
		t.Fatalf("final output plan/result = %#v %#v", out.Plan, out.Result)
	}
	if out.AudioSummary == nil || !out.AudioSummary.HasVoiceover || !out.AudioSummary.HasBGM || out.AudioSummary.TrackCount != 2 || out.AudioSummary.AudioCodec != "aac" || !out.AudioSummary.Ducking {
		t.Fatalf("audio summary = %#v", out.AudioSummary)
	}
	if len(out.AudioTracks) != 2 || out.AudioTracks[0].Role != "voiceover" || out.AudioTracks[1].Role != "bgm" {
		t.Fatalf("audio tracks = %#v", out.AudioTracks)
	}
	if out.FinalReview == nil || out.FinalReview.Status != "accepted_with_warnings" || out.FinalReview.Score != 0.86 {
		t.Fatalf("final review = %#v", out.FinalReview)
	}
}

func TestAgentWorkbenchAudioPlanSummaryTracksAudioNodeStatus(t *testing.T) {
	voiceNodeID := uuidWithByteForWorkbenchTest(61)
	bgmNodeID := uuidWithByteForWorkbenchTest(62)
	timelinePlanID := uuidWithByteForWorkbenchTest(63)
	audioPlan := db.AudioPlan{
		ID:                uuidWithByteForWorkbenchTest(60),
		Status:            "composing",
		Title:             "Audio plan",
		Language:          "zh",
		TargetDurationSec: pgtype.Float8{Float64: 12, Valid: true},
		VoiceoverScript:   "新品上线，轻松完成创意短片。",
		VoiceProfile:      []byte(`{"speaker":"zh_female","tone":"warm"}`),
		BgmPlan:           []byte(`{"mood":"bright"}`),
		CuePlan:           []byte(`[{"shot_ref":"shot-01","start_sec":0}]`),
		VoiceoverNodeID:   voiceNodeID,
		BgmNodeID:         bgmNodeID,
		TimelinePlanID:    timelinePlanID,
		SemanticKey:       "audio_plan.active",
		DisplayName:       "AudioPlan active",
	}
	nodes := map[pgtype.UUID]db.MediaNode{
		voiceNodeID: {ID: voiceNodeID, Status: db.NodeStatusSucceeded},
		bgmNodeID:   {ID: bgmNodeID, Status: db.NodeStatusRunning},
	}

	got := agentWorkbenchAudioPlanSummary(audioPlan, nodes)
	if got == nil || got.Status != "composing" || got.VoiceoverStatus != "succeeded" || got.BGMStatus != "running" {
		t.Fatalf("audio plan summary = %#v", got)
	}
	if got.TimelinePlanID != uuidToString(timelinePlanID) || got.TargetDurationSec == nil || *got.TargetDurationSec != 12 {
		t.Fatalf("audio plan timeline/duration = %#v", got)
	}
	if got.VoiceProfile["speaker"] != "zh_female" || got.BGMPlan["mood"] != "bright" {
		t.Fatalf("audio plan json summaries = %#v / %#v", got.VoiceProfile, got.BGMPlan)
	}
}

func uuidWithByteForWorkbenchTest(value byte) pgtype.UUID {
	return pgtype.UUID{
		Bytes: [16]byte{value, value, value, value, value, value, value, value, value, value, value, value, value, value, value, value},
		Valid: true,
	}
}
