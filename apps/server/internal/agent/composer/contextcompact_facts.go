package composer

import (
	"fmt"
	"strings"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
)

func composerContextCompactionFacts(composerContext Context) ([]contextcompact.FullSummaryFact, []contextcompact.MediaCard) {
	facts := make([]contextcompact.FullSummaryFact, 0)
	if strings.TrimSpace(composerContext.WorkspaceMode) != "" {
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "workspace/mode",
			Kind:    "workspace",
			Source:  "db",
			Summary: strings.TrimSpace(composerContext.WorkspaceMode),
		})
	}
	if composerContext.SourceStoryboardNodeID.Valid {
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "media_node/" + uuidString(composerContext.SourceStoryboardNodeID),
			Kind:    "source_storyboard_node",
			Source:  "db",
			Summary: strings.TrimSpace(composerContext.SourceNodeTitle),
		})
	} else if strings.TrimSpace(composerContext.SourceNodeTitle) != "" {
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "media_node/current_source",
			Kind:    "source_storyboard_node",
			Source:  "runtime",
			Summary: strings.TrimSpace(composerContext.SourceNodeTitle),
		})
	}
	facts = append(facts, contextcompact.FullSummaryFact{
		Ref:     "timeline_plan/count",
		Kind:    "timeline_plan",
		Source:  "db",
		Summary: fmt.Sprintf("%d timeline plans loaded", composerContext.TimelinePlanCount),
	})
	if strings.TrimSpace(composerContext.Summary) != "" {
		facts = append(facts, contextcompact.FullSummaryFact{
			Ref:     "composer/context_summary",
			Kind:    "composer_context",
			Source:  "runtime",
			Summary: strings.TrimSpace(composerContext.Summary),
		})
	}
	return facts, nil
}
