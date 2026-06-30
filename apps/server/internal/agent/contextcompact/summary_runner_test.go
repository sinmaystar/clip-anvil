package contextcompact

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestStaticFullSummarizerValidatesReturnedSummary(t *testing.T) {
	summarizer := StaticFullSummarizer{Summary: validFullSummaryForTest("agent_context_compaction/full"), ModelID: "static-test"}
	out, err := summarizer.Summarize(context.Background(), FullSummaryInput{
		Role:     "producer",
		Messages: []*schema.Message{schema.UserMessage("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelID != "static-test" || !strings.Contains(out.Summary, "# Compacted Agent Handoff Summary") {
		t.Fatalf("output = %#v", out)
	}
}

func TestStaticFullSummarizerBuildsFallbackSummary(t *testing.T) {
	summarizer := StaticFullSummarizer{}
	out, err := summarizer.Summarize(context.Background(), FullSummaryInput{
		Role:                   "reviewer",
		Facts:                  []FullSummaryFact{{Ref: "review_record/current", Kind: "review", Source: "db", Summary: "产品偏小"}},
		MediaCards:             []MediaCard{{Ref: "artifact_version/shot_01.preview.r1", Kind: "image", Status: "succeeded", Summary: "未生成视觉摘要", SourceRef: "db"}},
		RecentUserInstructions: []string{"保持箱体颜色一致"},
		RecoveryRefs:           []string{"agent_context_compaction/micro"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Compacted Agent Handoff Summary",
		"review_record/current",
		"artifact_version/shot_01.preview.r1",
		"保持箱体颜色一致",
		"agent_context_compaction/micro",
	} {
		if !strings.Contains(out.Summary, want) {
			t.Fatalf("fallback summary missing %q:\n%s", want, out.Summary)
		}
	}
}
