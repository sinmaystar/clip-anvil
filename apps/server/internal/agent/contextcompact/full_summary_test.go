package contextcompact

import (
	"strings"
	"testing"
)

func TestValidateFullSummaryRequiresHandoffSections(t *testing.T) {
	summary := `# Compacted Agent Handoff Summary

## User Goal
Create a 15s suitcase ad.

## Confirmed Decisions
- Use airport departure mood.

## Current Project State
- storyboard: confirmed

## Media Assets
- artifact_version/shot_01.preview.r1: 未生成视觉摘要

## Shot / RenderPlan Status
- shot_01 preview is ready.

## Review Findings
- 未确认

## Audio / Timeline State
- audio plan pending.

## Pending Signals And Tasks
- producer_pending_signal/render_done pending.

## Known Failures And Avoidances
- Avoid unsupported provider parameters.

## Recent User Instructions To Preserve Verbatim
- "保持箱体颜色一致"

## Next Recommended Actions
- Producer should dispatch Reviewer.

## Recovery References
- agent_context_compaction/ctxcmp-producer-full
`
	if err := ValidateFullSummaryMarkdown(summary); err != nil {
		t.Fatalf("ValidateFullSummaryMarkdown error = %v", err)
	}
	if err := ValidateFullSummaryMarkdown(strings.Replace(summary, "## Recovery References\n- agent_context_compaction/ctxcmp-producer-full\n", "", 1)); err == nil {
		t.Fatal("expected missing Recovery References to fail")
	}
}
