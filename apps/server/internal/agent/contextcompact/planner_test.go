package contextcompact

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestPlannerRecordsCandidatesWithoutRewritingMessages(t *testing.T) {
	planner := NewPlanner(DefaultConfig())
	msgs := []*schema.Message{
		schema.UserMessage("latest instruction"),
		schema.AssistantMessage(strings.Repeat("old tool output ", 1000), nil),
	}
	out, err := planner.Plan(context.Background(), PlanInput{
		Role:     "producer",
		ModelID:  "gpt-4o",
		Messages: msgs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != len(msgs) {
		t.Fatalf("len(Messages) = %d, want %d", len(out.Messages), len(msgs))
	}
	if out.Messages[1].Content != msgs[1].Content {
		t.Fatal("M9.1 planner must not rewrite messages")
	}
	if out.TokenBefore <= 0 {
		t.Fatalf("TokenBefore = %d, want > 0", out.TokenBefore)
	}
	if len(out.Candidates) == 0 {
		t.Fatal("expected at least one compaction candidate for long old assistant output")
	}
	if out.Candidates[0].Reason == "" {
		t.Fatal("candidate reason must be populated")
	}
}
