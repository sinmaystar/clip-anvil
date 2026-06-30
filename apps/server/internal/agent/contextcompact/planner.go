package contextcompact

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/schema"
)

type PlanInput struct {
	Role       string
	ModelID    string
	Messages   []*schema.Message
	ToolInfos  []*schema.ToolInfo
	MediaCards []MediaCard
}

type PlanOutput struct {
	Messages    []*schema.Message
	TokenBefore int
	Candidates  []Candidate
}

type Candidate struct {
	MessageIndex          int
	Role                  string
	OriginalTokenEstimate int
	EstimatedSavings      int
	Reason                string
}

type Planner struct {
	config  Config
	counter TokenCounter
}

func NewPlanner(config Config) Planner {
	return Planner{config: config.WithDefaults(), counter: NewTokenCounter()}
}

func (p Planner) Plan(ctx context.Context, input PlanInput) (PlanOutput, error) {
	count, err := p.counter.CountMessages(ctx, CountMessagesInput{
		ModelID:    input.ModelID,
		Messages:   input.Messages,
		ToolInfos:  input.ToolInfos,
		MediaCards: input.MediaCards,
	})
	if err != nil {
		return PlanOutput{}, err
	}
	out := PlanOutput{Messages: input.Messages, TokenBefore: count.TotalTokens}
	for i, msg := range input.Messages {
		if msg == nil || !isCandidateRole(msg.Role) {
			continue
		}
		text := messageText(msg)
		if len([]rune(strings.TrimSpace(text))) < 1000 {
			continue
		}
		tokens := heuristicTokens(text)
		savings := tokens - 64
		if savings < 1 {
			savings = 1
		}
		out.Candidates = append(out.Candidates, Candidate{
			MessageIndex:          i,
			Role:                  string(msg.Role),
			OriginalTokenEstimate: tokens,
			EstimatedSavings:      savings,
			Reason:                "old long assistant/tool-like message can be compacted in a later M9 phase",
		})
	}
	return out, nil
}

func isCandidateRole(role schema.RoleType) bool {
	return role == schema.Assistant || role == schema.Tool
}
