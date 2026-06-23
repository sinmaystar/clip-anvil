package preview

import "testing"

func TestShotStatusForPhaseTransitions(t *testing.T) {
	tests := []struct {
		name  string
		event string
		want  string
	}{
		{name: "dispatch", event: EventDispatched, want: ShotStatusPreviewRunning},
		{name: "submitted", event: EventSubmitted, want: ShotStatusPreviewRunning},
		{name: "succeeded", event: EventSucceeded, want: ShotStatusPreviewReady},
		{name: "failed", event: EventFailed, want: ShotStatusFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ShotStatusForEvent(tt.event)
			if !ok || got != tt.want {
				t.Fatalf("ShotStatusForEvent(%q) = %q, %v; want %q, true", tt.event, got, ok, tt.want)
			}
		})
	}
}

func TestShotStatusForPhaseTransitionsRejectsUnknownEvent(t *testing.T) {
	if got, ok := ShotStatusForEvent("unknown"); ok || got != "" {
		t.Fatalf("ShotStatusForEvent unknown = %q, %v", got, ok)
	}
}
