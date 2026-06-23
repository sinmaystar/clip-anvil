package tools

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	agentpss "github.com/sinmaystar/clip-anvil/internal/agent/pss"
)

func TestGetProductionStateDefinitionIsReadOnly(t *testing.T) {
	tool := NewGetProductionStateTool(fakePSSBuilder{})
	def := tool.Definition()
	if def.Name != "get_production_state" {
		t.Fatalf("Name = %q", def.Name)
	}
	if !def.Safety.ReadOnly || def.Safety.WritesCanvas || def.Safety.UsesProductionService {
		t.Fatalf("Safety = %#v", def.Safety)
	}
}

func TestGetProductionStateReturnsPSS(t *testing.T) {
	tool := NewGetProductionStateTool(fakePSSBuilder{pss: agentpss.ProducerPSS{
		Text:       "当前项目\n- Workspace: demo",
		Structured: map[string]any{"shots": []any{}},
	}})
	out, err := tool.Execute(context.Background(), ExecuteInput{WorkspaceID: uuidWithByte(1)})
	if err != nil {
		t.Fatal(err)
	}
	if out.Result["pss"] == "" {
		t.Fatalf("result = %#v", out.Result)
	}
	if out.Result["structured"] == nil {
		t.Fatalf("structured missing: %#v", out.Result)
	}
}

type fakePSSBuilder struct {
	pss agentpss.ProducerPSS
}

func (f fakePSSBuilder) BuildProducerPSS(context.Context, pgtype.UUID) (agentpss.ProducerPSS, error) {
	return f.pss, nil
}
