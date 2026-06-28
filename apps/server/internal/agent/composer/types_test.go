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
