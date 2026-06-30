package contextcompact

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestCurrentToolLoopFromIndexProtectsLatestAssistantToolPair(t *testing.T) {
	if got := CurrentToolLoopFromIndex(5, 0); got != 0 {
		t.Fatalf("no same-turn index = %d, want 0", got)
	}
	if got := CurrentToolLoopFromIndex(5, 1); got != 5 {
		t.Fatalf("single same-turn index = %d, want 5", got)
	}
	if got := CurrentToolLoopFromIndex(5, 4); got != 7 {
		t.Fatalf("four same-turn index = %d, want 7", got)
	}
}

func TestPendingReminderTargetIndexUsesLastUserOrTool(t *testing.T) {
	messages := []*schema.Message{
		{Role: schema.System, Content: "system"},
		{Role: schema.User, Content: "user"},
		{Role: schema.Assistant, Content: "assistant"},
		{Role: schema.Tool, Content: "tool"},
		{Role: schema.Assistant, Content: "after tool"},
	}
	if got := PendingReminderTargetIndex(messages, []string{"remind"}); got != 3 {
		t.Fatalf("pending reminder target = %d, want 3", got)
	}
	if got := PendingReminderTargetIndex(messages, nil); got != 0 {
		t.Fatalf("pending reminder target without reminders = %d, want 0", got)
	}
}
