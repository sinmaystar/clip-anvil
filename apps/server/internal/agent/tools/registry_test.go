package tools

import (
	"context"
	"errors"
	"testing"
)

func TestRegistryRejectsDuplicateToolNames(t *testing.T) {
	_, err := NewRegistry(fakeTool{name: "read_workspace_context"}, fakeTool{name: "read_workspace_context"})
	if !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("error = %v, want ErrDuplicateTool", err)
	}
}

func TestRegistryDefinitionIncludesSchemaAndDescription(t *testing.T) {
	registry, err := NewRegistry(fakeTool{name: "read_workspace_context"})
	if err != nil {
		t.Fatal(err)
	}

	def, ok := registry.Definition("read_workspace_context")
	if !ok {
		t.Fatal("definition not found")
	}
	if def.Description == "" {
		t.Fatal("definition must include description")
	}
	if def.Parameters["type"] != "object" {
		t.Fatalf("parameters = %#v", def.Parameters)
	}
}

func TestRegistryFindsRequiredM6Tools(t *testing.T) {
	registry, err := NewRegistry(
		NewReadWorkspaceContextTool(nil),
		NewCreateAgentTextNodeTool(nil, nil),
		NewRequestUserDecisionTool(nil),
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"read_workspace_context", "create_agent_text_node", "request_user_decision"} {
		if _, ok := registry.Definition(name); !ok {
			t.Fatalf("missing required tool %s", name)
		}
	}
}

type fakeTool struct {
	name string
}

func (f fakeTool) Definition() Definition {
	return Definition{
		Name:        f.name,
		Description: "fake description",
		Parameters:  map[string]any{"type": "object"},
		Result:      map[string]any{"type": "object"},
	}
}

func (f fakeTool) Execute(context.Context, ExecuteInput) (ExecuteOutput, error) {
	return ExecuteOutput{}, nil
}
