package reviewer

import (
	"strings"
	"testing"
)

func TestReviewerSystemPromptContainsGateRules(t *testing.T) {
	prompt := SystemPrompt()
	for _, required := range []string{
		"Reviewer / Quality Gate",
		"ProjectMemory 是项目创作宪法",
		"10 轴 Rubric",
		"Seedance",
		"submit_review_result",
		"不直接触发重跑",
		"不修改 RenderPlan",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("reviewer prompt missing %q", required)
		}
	}
	for _, forbidden := range []string{"TODO", "TBD", "M1 阶段", "M2 阶段"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("reviewer prompt contains placeholder wording %q", forbidden)
		}
	}
}
