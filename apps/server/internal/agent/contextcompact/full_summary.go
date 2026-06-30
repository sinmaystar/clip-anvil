package contextcompact

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

var ErrInvalidFullSummary = errors.New("invalid full compact summary")

type FullSummaryFact struct {
	Ref     string
	Kind    string
	Summary string
	Source  string
}

type FullSummaryInput struct {
	Role                   string
	ModelID                string
	Messages               []*schema.Message
	Facts                  []FullSummaryFact
	MediaCards             []MediaCard
	RecentUserInstructions []string
	RecoveryRefs           []string
}

type FullSummaryOutput struct {
	Summary string
	ModelID string
}

type FullSummarizer interface {
	Summarize(ctx context.Context, input FullSummaryInput) (FullSummaryOutput, error)
}

func BuildFullSummaryPrompt(input FullSummaryInput) string {
	var b strings.Builder
	b.WriteString("Create a compacted ClipAnvil Agent handoff summary.\n")
	b.WriteString("Role: " + strings.TrimSpace(input.Role) + "\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Preserve recent user instructions verbatim.\n")
	b.WriteString("- Use DB facts and semantic refs as authoritative facts.\n")
	b.WriteString("- Do not invent visual or audio details for media.\n")
	b.WriteString("- Mark uncertain facts as 未确认.\n")
	b.WriteString("- Include all required markdown sections exactly once.\n\n")
	b.WriteString("Structured facts:\n")
	for _, fact := range input.Facts {
		fmt.Fprintf(&b, "- %s [%s] source=%s: %s\n", fact.Ref, fact.Kind, fact.Source, fact.Summary)
	}
	b.WriteString("\nMedia cards:\n")
	for _, card := range input.MediaCards {
		fmt.Fprintf(&b, "- %s [%s] status=%s source=%s summary=%s path=%s\n", card.Ref, card.Kind, card.Status, card.SourceRef, card.Summary, card.SandboxPath)
	}
	b.WriteString("\nRecent user instructions to preserve verbatim:\n")
	for _, text := range input.RecentUserInstructions {
		b.WriteString("- " + strings.TrimSpace(text) + "\n")
	}
	b.WriteString("\nRecovery refs:\n")
	for _, ref := range input.RecoveryRefs {
		b.WriteString("- " + strings.TrimSpace(ref) + "\n")
	}
	return strings.TrimSpace(b.String())
}

func ValidateFullSummaryMarkdown(summary string) error {
	if strings.TrimSpace(summary) == "" {
		return fmt.Errorf("%w: empty summary", ErrInvalidFullSummary)
	}
	for _, section := range fullSummaryRequiredSections() {
		if !strings.Contains(summary, section) {
			return fmt.Errorf("%w: missing %s", ErrInvalidFullSummary, section)
		}
	}
	return nil
}

func fullSummaryRequiredSections() []string {
	return []string{
		"# Compacted Agent Handoff Summary",
		"## User Goal",
		"## Confirmed Decisions",
		"## Current Project State",
		"## Media Assets",
		"## Shot / RenderPlan Status",
		"## Review Findings",
		"## Audio / Timeline State",
		"## Pending Signals And Tasks",
		"## Known Failures And Avoidances",
		"## Recent User Instructions To Preserve Verbatim",
		"## Next Recommended Actions",
		"## Recovery References",
	}
}
