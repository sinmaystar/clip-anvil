package contextcompact

import (
	"context"
	"strings"
)

type StaticFullSummarizer struct {
	Summary string
	ModelID string
}

func (s StaticFullSummarizer) Summarize(_ context.Context, input FullSummaryInput) (FullSummaryOutput, error) {
	summary := strings.TrimSpace(s.Summary)
	if summary == "" {
		summary = BuildFallbackFullSummary(input)
	}
	if err := ValidateFullSummaryMarkdown(summary); err != nil {
		return FullSummaryOutput{}, err
	}
	return FullSummaryOutput{Summary: summary, ModelID: strings.TrimSpace(s.ModelID)}, nil
}

func BuildFallbackFullSummary(input FullSummaryInput) string {
	facts := fullSummaryListOrUnknown(factSummaryLines(input.Facts))
	media := fullSummaryListOrUnknown(mediaSummaryLines(input.MediaCards))
	recent := fullSummaryListOrUnknown(input.RecentUserInstructions)
	recovery := fullSummaryListOrUnknown(input.RecoveryRefs)
	return strings.TrimSpace(`# Compacted Agent Handoff Summary

## User Goal
未确认

## Confirmed Decisions
未确认

## Current Project State
` + facts + `

## Media Assets
` + media + `

## Shot / RenderPlan Status
未确认

## Review Findings
未确认

## Audio / Timeline State
未确认

## Pending Signals And Tasks
未确认

## Known Failures And Avoidances
未确认

## Recent User Instructions To Preserve Verbatim
` + recent + `

## Next Recommended Actions
Continue the current ` + strings.TrimSpace(input.Role) + ` turn using DB facts and recovery tools.

## Recovery References
` + recovery)
}

func factSummaryLines(facts []FullSummaryFact) []string {
	out := make([]string, 0, len(facts))
	for _, fact := range facts {
		line := strings.TrimSpace(fact.Ref)
		if fact.Kind != "" {
			line += " [" + strings.TrimSpace(fact.Kind) + "]"
		}
		if fact.Source != "" {
			line += " source=" + strings.TrimSpace(fact.Source)
		}
		if fact.Summary != "" {
			line += ": " + strings.TrimSpace(fact.Summary)
		}
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func mediaSummaryLines(cards []MediaCard) []string {
	out := make([]string, 0, len(cards))
	for _, card := range cards {
		line := strings.TrimSpace(card.Ref)
		if card.Kind != "" {
			line += " [" + strings.TrimSpace(card.Kind) + "]"
		}
		if card.Status != "" {
			line += " status=" + strings.TrimSpace(card.Status)
		}
		if card.SourceRef != "" {
			line += " source=" + strings.TrimSpace(card.SourceRef)
		}
		if card.Summary != "" {
			line += ": " + strings.TrimSpace(card.Summary)
		}
		if card.SandboxPath != "" {
			line += " path=" + strings.TrimSpace(card.SandboxPath)
		}
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func fullSummaryListOrUnknown(lines []string) string {
	if len(lines) == 0 {
		return "- 未确认"
	}
	var b strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
