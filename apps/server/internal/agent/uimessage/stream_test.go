package uimessage

import "testing"

func TestShouldShowThinkingRequiresSupportAndNonMinimalEffort(t *testing.T) {
	cases := []struct {
		name     string
		supports bool
		effort   string
		want     bool
	}{
		{name: "unsupported high hidden", supports: false, effort: "high", want: false},
		{name: "supported minimal hidden", supports: true, effort: "minimal", want: false},
		{name: "supported empty hidden", supports: true, effort: "", want: false},
		{name: "supported low visible", supports: true, effort: "low", want: true},
		{name: "supported medium visible", supports: true, effort: "medium", want: true},
		{name: "supported high visible", supports: true, effort: "high", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldShowThinking(tc.supports, tc.effort); got != tc.want {
				t.Fatalf("ShouldShowThinking() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStreamDeltaPayloadUsesBlockShape(t *testing.T) {
	payload := NewStreamDelta(StreamDeltaInput{
		WorkspaceID: "workspace-1",
		ThreadID:    "thread-1",
		TaskID:      "task-1",
		BlockID:     "blk_answer",
		BlockType:   "markdown",
		Delta:       "hello",
		Sequence:    3,
	})
	if payload.BlockID != "blk_answer" || payload.BlockType != "markdown" || payload.Sequence != 3 {
		t.Fatalf("payload = %#v", payload)
	}
}
