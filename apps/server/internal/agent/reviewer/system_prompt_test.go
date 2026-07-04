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
		"Remotion Motion Shot",
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

func TestSystemPromptIncludesRemotionTimelineFinalReviewRules(t *testing.T) {
	prompt := SystemPrompt()
	for _, want := range []string{
		"Remotion Timeline final video",
		"single Composer-owned caption lane",
		"wheel cue",
		"storage cue",
		"no-Seedance",
		"mixed-cost",
		"Seedance video segment count",
		"Remotion still segment count",
		"cost_risk",
		"layout repetition",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("SystemPrompt() missing %q", want)
		}
	}
}
