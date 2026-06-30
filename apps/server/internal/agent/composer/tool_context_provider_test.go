package composer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agenttools "github.com/sinmaystar/clip-anvil/internal/agent/tools"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestCompositionContextIncludesApprovedAudioPlanAndAudioArtifacts(t *testing.T) {
	workspaceID := uuidWithByte(1)
	shotID := uuidWithByte(20)
	shotNodeID := uuidWithByte(21)
	shotVersionID := uuidWithByte(22)
	shotAssetID := uuidWithByte(23)
	voiceNodeID := uuidWithByte(30)
	voiceVersionID := uuidWithByte(31)
	voiceAssetID := uuidWithByte(32)
	bgmNodeID := uuidWithByte(40)
	bgmVersionID := uuidWithByte(41)
	bgmAssetID := uuidWithByte(42)
	audioPlanID := uuidWithByte(50)

	voiceProfile := mustComposerJSON(t, map[string]any{"tone": "warm", "speaker": "zh_female"})
	bgmPlan := mustComposerJSON(t, map[string]any{"mood": "bright", "instrumentation": "light pop"})
	cuePlan := mustComposerJSON(t, []map[string]any{{"shot_ref": "shot-01", "start_sec": 0, "end_sec": 4.2}})
	store := &fakeComposerStore{
		audioPlan: &db.AudioPlan{
			ID:                audioPlanID,
			WorkspaceID:       workspaceID,
			Status:            "approved",
			Title:             "Launch voiceover and BGM",
			TargetDurationSec: float8Value(12),
			VoiceoverScript:   "新品上线，轻松完成创意短片。",
			VoiceProfile:      voiceProfile,
			BgmPlan:           bgmPlan,
			CuePlan:           cuePlan,
			VoiceoverNodeID:   voiceNodeID,
			BgmNodeID:         bgmNodeID,
			SemanticKey:       "audio_plan.active",
			DisplayName:       "AudioPlan active",
		},
		shots: []db.Shot{{ID: shotID, WorkspaceID: workspaceID, SortOrder: 1, Title: "shot 01"}},
		nodesByShot: map[pgtype.UUID][]db.MediaNode{
			shotID: {
				{
					ID:               shotNodeID,
					WorkspaceID:      workspaceID,
					NodeType:         db.NodeTypeVideo,
					Title:            "Shot 01 video",
					CurrentVersionID: shotVersionID,
					Metadata:         mustComposerJSON(t, map[string]any{"agent_artifact_kind": "shot_video"}),
					SemanticKey:      "shot.01.video",
				},
			},
		},
		nodes: []db.MediaNode{
			{
				ID:               voiceNodeID,
				WorkspaceID:      workspaceID,
				NodeType:         db.NodeTypeAudio,
				Title:            "Voiceover",
				CurrentVersionID: voiceVersionID,
				Metadata:         mustComposerJSON(t, map[string]any{"agent_artifact_kind": "voiceover_audio"}),
				SemanticKey:      "audio_plan.active.voiceover_audio.node",
			},
			{
				ID:               bgmNodeID,
				WorkspaceID:      workspaceID,
				NodeType:         db.NodeTypeAudio,
				Title:            "BGM",
				CurrentVersionID: bgmVersionID,
				Metadata:         mustComposerJSON(t, map[string]any{"agent_artifact_kind": "bgm_audio"}),
				SemanticKey:      "audio_plan.active.bgm_audio.node",
			},
		},
		versions: map[pgtype.UUID]db.ArtifactVersion{
			shotVersionID:  {ID: shotVersionID, AssetID: shotAssetID, Status: db.JobStatusSucceeded},
			voiceVersionID: {ID: voiceVersionID, AssetID: voiceAssetID, Status: db.JobStatusSucceeded},
			bgmVersionID:   {ID: bgmVersionID, AssetID: bgmAssetID, Status: db.JobStatusSucceeded},
		},
		assets: map[pgtype.UUID]db.MediaAsset{
			shotAssetID:  {ID: shotAssetID, WorkspaceID: workspaceID, Mime: "video/mp4", StorageUrl: textValue("workspace/shot-01.mp4")},
			voiceAssetID: {ID: voiceAssetID, WorkspaceID: workspaceID, Mime: "audio/mpeg", StorageUrl: textValue("workspace/voiceover.mp3")},
			bgmAssetID:   {ID: bgmAssetID, WorkspaceID: workspaceID, Mime: "audio/mpeg", StorageUrl: textValue("workspace/bgm.mp3")},
		},
	}

	ctx, err := NewToolContextProvider(store).GetCompositionContext(context.Background(), agenttools.NativeRuntimeContext{WorkspaceID: workspaceID, ScopeID: uuidWithByte(60)}, uuidWithByte(60))
	if err != nil {
		t.Fatal(err)
	}

	audioPlan, ok := ctx["audio_plan"].(map[string]any)
	if !ok {
		t.Fatalf("audio_plan missing from context: %#v", ctx)
	}
	if audioPlan["status"] != "approved" || audioPlan["semantic_key"] != "audio_plan.active" || audioPlan["voiceover_script"] == "" {
		t.Fatalf("audio plan = %#v", audioPlan)
	}
	assets, _ := ctx["available_composition_assets"].([]map[string]any)
	if len(assets) != 3 {
		t.Fatalf("assets = %#v", assets)
	}
	if !composerContextHasRole(assets, "voiceover") || !composerContextHasRole(assets, "bgm") {
		t.Fatalf("audio assets missing: %#v", assets)
	}
	schema, _ := ctx["timeline_plan_schema"].(map[string]any)
	if _, ok := schema["audio_tracks"]; !ok {
		t.Fatalf("schema missing audio_tracks: %#v", schema)
	}
}

func TestCompositionContextIncludesPreviewImageFallbackWhenShotVideoMissing(t *testing.T) {
	workspaceID := uuidWithByte(1)
	shot1ID := uuidWithByte(21)
	shot2ID := uuidWithByte(22)
	shot3ID := uuidWithByte(23)
	shot4ID := uuidWithByte(24)
	video1VersionID := uuidWithByte(31)
	video2VersionID := uuidWithByte(32)
	video3VersionID := uuidWithByte(33)
	preview4VersionID := uuidWithByte(34)
	video1AssetID := uuidWithByte(41)
	video2AssetID := uuidWithByte(42)
	video3AssetID := uuidWithByte(43)
	preview4AssetID := uuidWithByte(44)

	store := &fakeComposerStore{
		shots: []db.Shot{
			{ID: shot1ID, WorkspaceID: workspaceID, ClientKey: "shot_01", SemanticKey: "shot_01", SortOrder: 1, Title: "顺滑拉箱"},
			{ID: shot2ID, WorkspaceID: workspaceID, ClientKey: "shot_02", SemanticKey: "shot_02", SortOrder: 2, Title: "轻量提拿"},
			{ID: shot3ID, WorkspaceID: workspaceID, ClientKey: "shot_03", SemanticKey: "shot_03", SortOrder: 3, Title: "商务特写"},
			{ID: shot4ID, WorkspaceID: workspaceID, ClientKey: "shot_04", SemanticKey: "shot_04", SortOrder: 4, Title: "分区收纳收尾"},
		},
		nodesByShot: map[pgtype.UUID][]db.MediaNode{
			shot1ID: {{
				ID:               uuidWithByte(51),
				WorkspaceID:      workspaceID,
				ShotID:           shot1ID,
				NodeType:         db.NodeTypeVideo,
				Title:            "shot_01 shot video",
				CurrentVersionID: video1VersionID,
				Metadata:         mustComposerJSON(t, map[string]any{"agent_artifact_kind": "shot_video"}),
				SemanticKey:      "shot_01.shot_video.r1.node",
				ArtifactKind:     "shot_video",
			}},
			shot2ID: {{
				ID:               uuidWithByte(52),
				WorkspaceID:      workspaceID,
				ShotID:           shot2ID,
				NodeType:         db.NodeTypeVideo,
				Title:            "shot_02 shot video",
				CurrentVersionID: video2VersionID,
				Metadata:         mustComposerJSON(t, map[string]any{"agent_artifact_kind": "shot_video"}),
				SemanticKey:      "shot_02.shot_video.r1.node",
				ArtifactKind:     "shot_video",
			}},
			shot3ID: {{
				ID:               uuidWithByte(53),
				WorkspaceID:      workspaceID,
				ShotID:           shot3ID,
				NodeType:         db.NodeTypeVideo,
				Title:            "shot_03 shot video",
				CurrentVersionID: video3VersionID,
				Metadata:         mustComposerJSON(t, map[string]any{"agent_artifact_kind": "shot_video"}),
				SemanticKey:      "shot_03.shot_video.r1.node",
				ArtifactKind:     "shot_video",
			}},
			shot4ID: {{
				ID:               uuidWithByte(54),
				WorkspaceID:      workspaceID,
				ShotID:           shot4ID,
				NodeType:         db.NodeTypeImage,
				Title:            "shot_04 preview image",
				CurrentVersionID: preview4VersionID,
				Metadata:         mustComposerJSON(t, map[string]any{"agent_artifact_kind": "preview_image"}),
				SemanticKey:      "shot_04.preview_image.r1.node",
				ArtifactKind:     "preview_image",
			}},
		},
		versions: map[pgtype.UUID]db.ArtifactVersion{
			video1VersionID:   {ID: video1VersionID, AssetID: video1AssetID, Status: db.JobStatusSucceeded},
			video2VersionID:   {ID: video2VersionID, AssetID: video2AssetID, Status: db.JobStatusSucceeded},
			video3VersionID:   {ID: video3VersionID, AssetID: video3AssetID, Status: db.JobStatusSucceeded},
			preview4VersionID: {ID: preview4VersionID, AssetID: preview4AssetID, Status: db.JobStatusSucceeded},
		},
		assets: map[pgtype.UUID]db.MediaAsset{
			video1AssetID:   {ID: video1AssetID, WorkspaceID: workspaceID, Mime: "video/mp4", StorageUrl: textValue("workspace/shot-01.mp4")},
			video2AssetID:   {ID: video2AssetID, WorkspaceID: workspaceID, Mime: "video/mp4", StorageUrl: textValue("workspace/shot-02.mp4")},
			video3AssetID:   {ID: video3AssetID, WorkspaceID: workspaceID, Mime: "video/mp4", StorageUrl: textValue("workspace/shot-03.mp4")},
			preview4AssetID: {ID: preview4AssetID, WorkspaceID: workspaceID, Mime: "image/jpeg", StorageUrl: textValue("workspace/shot-04.jpg")},
		},
	}

	ctx, err := NewToolContextProvider(store).GetCompositionContext(context.Background(), agenttools.NativeRuntimeContext{WorkspaceID: workspaceID}, pgtype.UUID{})
	if err != nil {
		t.Fatal(err)
	}

	assets, _ := ctx["available_composition_assets"].([]map[string]any)
	if len(assets) != 4 {
		t.Fatalf("assets = %#v", assets)
	}
	preview, ok := composerContextAssetByNodeRef(assets, "shot_04.preview_image.r1.node")
	if !ok {
		t.Fatalf("shot_04 preview fallback missing from assets: %#v", assets)
	}
	if preview["role"] != "still" || preview["artifact_kind"] != "preview_image" || preview["mime_type"] != "image/jpeg" {
		t.Fatalf("preview fallback asset = %#v", preview)
	}
	if preview["node_ref"] != "shot_04.preview_image.r1.node" {
		t.Fatalf("preview fallback should expose full media_node semantic key: %#v", preview)
	}
}

func composerContextHasRole(assets []map[string]any, role string) bool {
	for _, asset := range assets {
		if asset["role"] == role {
			return true
		}
	}
	return false
}

func composerContextAssetByNodeRef(assets []map[string]any, nodeRef string) (map[string]any, bool) {
	for _, asset := range assets {
		if asset["node_ref"] == nodeRef {
			return asset, true
		}
	}
	return nil, false
}

func mustComposerJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func textValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func float8Value(value float64) pgtype.Float8 {
	return pgtype.Float8{Float64: value, Valid: true}
}
