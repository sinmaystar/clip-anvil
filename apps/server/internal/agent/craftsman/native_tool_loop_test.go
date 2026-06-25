package craftsman

import "testing"

func TestCraftsmanDefaultMaxToolCallsIsLargeDuringArchitectureIteration(t *testing.T) {
	if got := maxCraftsmanToolCalls(); got != 1000 {
		t.Fatalf("maxCraftsmanToolCalls() = %d, want 1000", got)
	}
}
