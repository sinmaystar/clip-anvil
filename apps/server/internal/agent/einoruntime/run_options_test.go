package einoruntime

import (
	"context"
	"testing"
)

func TestApplyRunOptionsAddsCheckpointOption(t *testing.T) {
	_, options := ApplyRunOptions(context.Background(), RunOptions{
		CheckPointID: "cp-1",
		ForceNewRun:  true,
	})

	if len(options) != 2 {
		t.Fatalf("options len = %d", len(options))
	}
}

func TestApplyRunOptionsAppliesBatchResumeData(t *testing.T) {
	resumeData := map[string]any{"decision": "accepted"}
	ctx, _ := ApplyRunOptions(context.Background(), RunOptions{ResumeData: resumeData})

	if ctx == nil {
		t.Fatal("ApplyRunOptions returned nil context")
	}
}

func TestResumeDecisionDataUsesStableKeys(t *testing.T) {
	eventID := uuidWithByte(9)
	got := ResumeDecisionData(eventID, "option-a", "free text")

	if got["decision_event_id"] != "09000000-0000-0000-0000-000000000000" {
		t.Fatalf("decision_event_id = %#v", got["decision_event_id"])
	}
	if got["selected_option_id"] != "option-a" {
		t.Fatalf("selected_option_id = %#v", got["selected_option_id"])
	}
	if got["free_text"] != "free text" {
		t.Fatalf("free_text = %#v", got["free_text"])
	}
}
