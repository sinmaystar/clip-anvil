package contextcompact

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestProjectRunsFullCompactAfterMicroWhenStillOverFullTrigger(t *testing.T) {
	oldTool := strings.Repeat("old ffmpeg stderr line\n", 1600)
	messages := []*schema.Message{
		schema.SystemMessage("system prompt"),
		schema.UserMessage("old user"),
		{Role: schema.Tool, ToolCallID: "call-old", ToolName: "run_ffmpeg_command", Content: oldTool},
		schema.UserMessage("latest user instruction must stay visible"),
	}
	store := newMemoryStore()
	summarizer := &fakeFullSummarizer{summary: validFullSummaryForTest("agent_context_compaction/summary")}
	middleware := NewMiddleware(MiddlewareConfig{
		Config: compactTestConfig(CompactionThresholds{
			MicroTriggerTokens:          100,
			MicroTargetTokens:           90,
			MicroMinReductionTokens:     1,
			PreserveRecentUserMessages:  1,
			PreserveRecentTotalMessages: 1,
		}),
		Store:          store,
		FileWriter:     newMemoryDetailFileWriter(),
		FullSummarizer: summarizer,
	})
	middleware.config.FullTriggerTokens = 100
	middleware.config.FullTargetTokens = 80

	out, err := middleware.Project(context.Background(), ProjectionInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Role:        "producer",
		ModelID:     "doubao-test",
		Messages:    messages,
		MessageRefs: []SourceMessageRef{{MessageIndex: 2, MessageID: uuidWithByte(4)}},
		Facts:       []FullSummaryFact{{Ref: "shot/shot_01", Kind: "shot", Source: "db", Summary: "preview ready"}},
		Trigger:     "producer_before_model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.CompactionMode != "full" || len(out.Applied) == 0 {
		t.Fatalf("output = %#v", out)
	}
	if len(out.Messages) < 3 || out.Messages[1].Role != schema.User || !strings.Contains(out.Messages[1].Content, "# Compacted Agent Handoff Summary") {
		t.Fatalf("handoff summary message missing: %#v", out.Messages)
	}
	if !strings.Contains(out.Messages[len(out.Messages)-1].Content, "latest user instruction must stay visible") {
		t.Fatalf("latest user instruction not preserved: %#v", out.Messages)
	}
	if len(store.links) == 0 || store.links[len(store.links)-1].MessageID != uuidWithByte(4) {
		t.Fatalf("source message links = %#v", store.links)
	}
	if messages[2].Content != oldTool {
		t.Fatal("original message content was mutated")
	}
	if len(summarizer.input.Facts) != 1 || summarizer.input.Facts[0].Ref != "shot/shot_01" {
		t.Fatalf("summary facts = %#v", summarizer.input.Facts)
	}
}

func TestProjectFallsBackWhenFullCompactSummaryIsInvalid(t *testing.T) {
	oldTool := strings.Repeat("old producer context line\n", 1600)
	messages := []*schema.Message{
		schema.SystemMessage("system prompt"),
		{Role: schema.Tool, ToolCallID: "call-old", ToolName: "read_project_context", Content: oldTool},
		schema.UserMessage("修复 final review 后继续推进"),
	}
	store := newMemoryStore()
	summarizer := &fakeFullSummarizer{
		summary: "普通摘要，没有固定 handoff 标题",
		err:     fmt.Errorf("%w: missing # Compacted Agent Handoff Summary", ErrInvalidFullSummary),
	}
	middleware := NewMiddleware(MiddlewareConfig{
		Config: compactTestConfig(CompactionThresholds{
			MicroTriggerTokens:          100,
			MicroTargetTokens:           90,
			MicroMinReductionTokens:     1,
			PreserveRecentUserMessages:  1,
			PreserveRecentTotalMessages: 1,
		}),
		Store:          store,
		FileWriter:     newMemoryDetailFileWriter(),
		FullSummarizer: summarizer,
	})
	middleware.config.FullTriggerTokens = 100

	out, err := middleware.Project(context.Background(), ProjectionInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Role:        "producer",
		ModelID:     "doubao-test",
		Messages:    messages,
		MessageRefs: []SourceMessageRef{{MessageIndex: 1, MessageID: uuidWithByte(4)}},
		Facts: []FullSummaryFact{
			{Ref: "review_record/final", Kind: "review", Source: "db", Summary: "final_video_review rejected"},
		},
		Trigger: "producer_before_model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.CompactionMode != "full" {
		t.Fatalf("compaction mode = %q, want full", out.CompactionMode)
	}
	if len(out.Messages) < 2 || !strings.Contains(out.Messages[1].Content, "# Compacted Agent Handoff Summary") {
		t.Fatalf("fallback handoff summary missing: %#v", out.Messages)
	}
	if !strings.Contains(out.Messages[1].Content, "review_record/final") {
		t.Fatalf("fallback summary lost db facts: %s", out.Messages[1].Content)
	}
	if len(out.Applied) == 0 || out.Applied[len(out.Applied)-1].Mode != "full" {
		t.Fatalf("full compaction record missing: %#v", out.Applied)
	}
}

type fakeFullSummarizer struct {
	summary string
	input   FullSummaryInput
	err     error
}

func (f *fakeFullSummarizer) Summarize(_ context.Context, input FullSummaryInput) (FullSummaryOutput, error) {
	f.input = input
	return FullSummaryOutput{Summary: f.summary, ModelID: "fake-summary-model"}, f.err
}

func validFullSummaryForTest(ref string) string {
	return `# Compacted Agent Handoff Summary

## User Goal
Create a marketing video.

## Confirmed Decisions
- 未确认

## Current Project State
- shot/shot_01 exists.

## Media Assets
- 未生成视觉摘要

## Shot / RenderPlan Status
- shot_01 preview ready.

## Review Findings
- 未确认

## Audio / Timeline State
- 未确认

## Pending Signals And Tasks
- 未确认

## Known Failures And Avoidances
- 未确认

## Recent User Instructions To Preserve Verbatim
- latest user instruction must stay visible

## Next Recommended Actions
- Continue current role task.

## Recovery References
- ` + ref + `
`
}
