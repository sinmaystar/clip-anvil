package composer

import "testing"

func TestComposerFullCompactFactsIncludeTimelineContext(t *testing.T) {
	facts, cards := composerContextCompactionFacts(Context{
		WorkspaceMode:     "agent",
		SourceNodeTitle:   "final storyboard",
		TimelinePlanCount: 2,
		Summary:           "Composer context loaded.",
	})

	if len(facts) < 3 {
		t.Fatalf("facts = %#v", facts)
	}
	if len(cards) != 0 {
		t.Fatalf("composer should not invent media cards from summary-only context: %#v", cards)
	}
}
