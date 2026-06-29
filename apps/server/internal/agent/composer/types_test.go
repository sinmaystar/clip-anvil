package composer

import (
	"encoding/json"
	"testing"
)

func TestComposerDomainTypesDescribeTimelinePlanLifecycle(t *testing.T) {
	result := Result{
		Status:  StatusCompleted,
		Summary: "final video rendered",
	}
	if result.Status != "completed" || StatusBlocked != "blocked" || StatusFailed != "failed" {
		t.Fatalf("statuses are not stable: %#v", result)
	}

	plan := TimelinePlan{
		TemplateKey: "concat_with_fades",
		Segments: []Segment{
			{ID: "seg-1", AssetID: "asset-1", WorkspacePath: "/workspace/input/shot-1.mp4", StartSec: 0, DurationSec: 2.5},
		},
		Transitions: []Transition{
			{FromSegmentID: "seg-1", ToSegmentID: "seg-2", Type: "fade", DurationSec: 0.5},
		},
		Output: OutputSettings{WorkspacePath: "/workspace/output/final.mp4", Width: 1280, Height: 720, FPS: 30, Format: "mp4"},
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["template_key"] != "concat_with_fades" {
		t.Fatalf("json = %s", raw)
	}
}

func TestTimelinePlanSupportsAudioTracks(t *testing.T) {
	plan := TimelinePlan{
		TemplateKey: "concat_with_fades",
		Segments: []Segment{
			{ID: "seg-1", AssetID: "asset-video", WorkspacePath: "/workspace/input/shot-1.mp4", DurationSec: 4.2},
		},
		AudioTracks: []AudioTrack{
			{
				ID:            "voiceover-main",
				Role:          "voiceover",
				AssetID:       "asset-voiceover",
				WorkspacePath: "/workspace/input/voiceover.mp3",
				StartSec:      0,
				DurationSec:   12,
				Volume:        1,
				FadeInSec:     0.05,
				FadeOutSec:    0.1,
			},
			{
				ID:            "bgm-main",
				Role:          "bgm",
				AssetID:       "asset-bgm",
				WorkspacePath: "/workspace/input/bgm.mp3",
				StartSec:      0,
				DurationSec:   12,
				Volume:        0.28,
				FadeInSec:     0.5,
				FadeOutSec:    1.2,
				Ducking: &AudioDucking{
					SidechainRole: "voiceover",
					Threshold:     0.08,
					Ratio:         8,
					AttackMS:      20,
					ReleaseMS:     250,
				},
			},
		},
		Output: OutputSettings{WorkspacePath: "/workspace/output/final.mp4", Format: "mp4", AudioCodec: "aac"},
	}

	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TimelinePlan
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.AudioTracks) != 2 {
		t.Fatalf("audio tracks = %#v", decoded.AudioTracks)
	}
	if got := decoded.AudioTracks[1].Ducking; got == nil || got.SidechainRole != "voiceover" || got.Ratio != 8 {
		t.Fatalf("ducking = %#v", got)
	}
	if decoded.Output.AudioCodec != "aac" {
		t.Fatalf("audio codec = %q, json = %s", decoded.Output.AudioCodec, raw)
	}

	var decodedMap map[string]any
	if err := json.Unmarshal(raw, &decodedMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := decodedMap["audio_tracks"]; !ok {
		t.Fatalf("json missing audio_tracks: %s", raw)
	}
}
