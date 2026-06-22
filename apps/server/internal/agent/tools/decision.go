package tools

import (
	"context"
	"errors"
)

type DecisionRequester interface {
	RequestUserDecision(ctx context.Context, input ExecuteInput) (ExecuteOutput, error)
}

type RequestUserDecisionTool struct {
	requester DecisionRequester
}

func NewRequestUserDecisionTool(requester DecisionRequester) RequestUserDecisionTool {
	return RequestUserDecisionTool{requester: requester}
}

func (t RequestUserDecisionTool) Definition() Definition {
	return Definition{
		Name:        "request_user_decision",
		Description: "Ask the user to make a decision before continuing. This creates a persisted decision card, checkpoint, and waiting_for_user task state.",
		Parameters: objectSchema(map[string]any{
			"title":           map[string]any{"type": "string", "minLength": 1, "maxLength": 120},
			"message":         map[string]any{"type": "string", "minLength": 1, "maxLength": 2000},
			"options":         map[string]any{"type": "array", "maxItems": 6},
			"allow_free_text": map[string]any{"type": "boolean"},
		}),
		Result: map[string]any{"type": "object"},
		Safety: SafetySpec{
			RequiresHITL:    true,
			MaxCallsPerTurn: 1,
		},
		Visibility: VisibilitySpec{
			ShowCallMessage: true,
			UserLabel:       "请求用户决策",
		},
	}
}

func (t RequestUserDecisionTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	if t.requester == nil {
		return ExecuteOutput{}, errors.New("request_user_decision service is not configured")
	}
	return t.requester.RequestUserDecision(ctx, input)
}
