package tools

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	agentpss "github.com/sinmaystar/clip-anvil/internal/agent/pss"
)

type PSSBuilder interface {
	BuildProducerPSS(ctx context.Context, workspaceID pgtype.UUID) (agentpss.ProducerPSS, error)
}

type GetProductionStateTool struct {
	builder PSSBuilder
}

func NewGetProductionStateTool(builder PSSBuilder) GetProductionStateTool {
	return GetProductionStateTool{builder: builder}
}

func (t GetProductionStateTool) Definition() Definition {
	return Definition{
		Name:        "get_production_state",
		Description: "Read the current Agent production state, including storyboard shots, shot dependencies, source materials, canvas nodes, versions, stale reasons, pending decisions, and running tasks. Returns deterministic PSS text plus structured state.",
		Parameters: objectSchema(map[string]any{
			"include_structured":      map[string]any{"type": "boolean"},
			"include_recent_activity": map[string]any{"type": "boolean"},
		}),
		Result: map[string]any{"type": "object"},
		Safety: SafetySpec{
			ReadOnly:        true,
			MaxCallsPerTurn: 10,
		},
		Visibility: VisibilitySpec{
			ShowCallMessage:   true,
			ShowResultMessage: true,
			UserLabel:         "读取生产状态",
		},
	}
}

func (t GetProductionStateTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error) {
	pss, err := t.builder.BuildProducerPSS(ctx, input.WorkspaceID)
	if err != nil {
		return ExecuteOutput{}, err
	}
	return ExecuteOutput{Result: map[string]any{
		"pss":        pss.Text,
		"structured": pss.Structured,
	}}, nil
}
