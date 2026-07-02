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
		"BGM ducking",
		"relative volume",
		"voiceover",
		"audio_sync",
		"platform_selling_power",
		"Template Video",
		"readability",
		"motion_rhythm",
		"truthfulness",
		"cost_risk",
		"requires_user_confirmation",
		"submit_review_result",
		"不直接触发重跑",
		"不修改 RenderPlan",
		"## Skills Library",
		"load_agent_skill",
		"reviewer-quality-gate",
		"media_node",
		"shot_04.preview_image.r1.node",
		"artifact_version",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("reviewer prompt missing %q", required)
		}
	}
	for _, forbidden := range []string{"TODO", "TBD", "M1 阶段", "M2 阶段", "# Reviewer Quality Gate"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("reviewer prompt contains placeholder wording %q", forbidden)
		}
	}
}
