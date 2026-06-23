package einoruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/compose"
)

func TestGraphInfoMermaidIsDeterministic(t *testing.T) {
	registry := NewGraphInfoRegistry()
	graph := compose.NewGraph[string, string]()
	if err := graph.AddLambdaNode("load_context", compose.InvokableLambda(func(context.Context, string) (string, error) {
		return "loaded", nil
	})); err != nil {
		t.Fatal(err)
	}
	if err := graph.AddLambdaNode("draft_response", compose.InvokableLambda(func(context.Context, string) (string, error) {
		return "draft", nil
	})); err != nil {
		t.Fatal(err)
	}
	if err := graph.AddEdge(compose.START, "load_context"); err != nil {
		t.Fatal(err)
	}
	if err := graph.AddEdge("load_context", "draft_response"); err != nil {
		t.Fatal(err)
	}
	if err := graph.AddEdge("draft_response", compose.END); err != nil {
		t.Fatal(err)
	}
	if _, err := graph.Compile(context.Background(), compose.WithGraphName("producer_turn"), compose.WithGraphCompileCallbacks(registry.CompileCallback())); err != nil {
		t.Fatal(err)
	}

	got, err := registry.Mermaid("producer_turn")
	if err != nil {
		t.Fatal(err)
	}
	wantParts := []string{
		"flowchart TD",
		`  START["START"]`,
		`  load_context["load_context"]`,
		`  draft_response["draft_response"]`,
		`  END["END"]`,
		"  START --> load_context",
		"  load_context --> draft_response",
		"  draft_response --> END",
	}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Fatalf("mermaid missing %q:\n%s", part, got)
		}
	}
}

func TestGraphInfoJSONIncludesGraphNames(t *testing.T) {
	registry := NewGraphInfoRegistry()
	registry.OnFinish(context.Background(), &compose.GraphInfo{Name: "producer_turn"})
	registry.OnFinish(context.Background(), &compose.GraphInfo{Name: "composer_final"})

	raw, err := registry.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["producer_turn"] == nil || payload["composer_final"] == nil {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestGraphInfoGetReturnsCapturedGraph(t *testing.T) {
	registry := NewGraphInfoRegistry()
	registry.OnFinish(context.Background(), &compose.GraphInfo{Name: "reviewer_preview"})

	info, ok := registry.Get("reviewer_preview")
	if !ok {
		t.Fatal("graph info not found")
	}
	if info.Name != "reviewer_preview" {
		t.Fatalf("graph name = %q", info.Name)
	}
}
