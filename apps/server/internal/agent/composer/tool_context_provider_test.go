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

func composerContextHasRole(assets []map[string]any, role string) bool {
	for _, asset := range assets {
		if asset["role"] == role {
			return true
		}
	}
	return false
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
