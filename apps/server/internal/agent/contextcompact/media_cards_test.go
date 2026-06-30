package contextcompact

import (
	"strings"
	"testing"
)

func TestMediaCardUsesTrustedSummarySources(t *testing.T) {
	card := MediaCard{
		Ref:         "artifact_version/shot_01.video.r1",
		Kind:        "video",
		Status:      "succeeded",
		Summary:     "未生成视觉摘要",
		DurationSec: 5.2,
		SandboxPath: "/workspace/renders/shot_01.mp4",
		SourceRef:   "probe",
	}
	text := MediaCardPromptText(card)
	for _, want := range []string{"artifact_version/shot_01.video.r1", "video", "5.2", "/workspace/renders/shot_01.mp4", "source=probe"} {
		if !strings.Contains(text, want) {
			t.Fatalf("card text missing %q: %s", want, text)
		}
	}
}

func TestMediaCardPromptTextFallsBackWithoutInventingMediaSummary(t *testing.T) {
	text := MediaCardPromptText(MediaCard{Ref: "artifact_version/audio.r1", Kind: "audio"})
	if !strings.Contains(text, "未生成音频摘要") {
		t.Fatalf("audio fallback summary missing: %s", text)
	}
	text = MediaCardPromptText(MediaCard{Ref: "artifact_version/image.r1", Kind: "image"})
	if !strings.Contains(text, "未生成视觉摘要") {
		t.Fatalf("image fallback summary missing: %s", text)
	}
}
