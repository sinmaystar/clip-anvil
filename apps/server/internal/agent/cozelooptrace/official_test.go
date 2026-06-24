package cozelooptrace

import (
	"context"
	"strings"
	"testing"

	loopcallback "github.com/cloudwego/eino-ext/callbacks/cozeloop"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/compose"
	"github.com/coze-dev/cozeloop-go/spec/tracespec"
	"go.opentelemetry.io/otel/attribute"
)

func TestTagsFromAttributesConvertsOpenTelemetryValues(t *testing.T) {
	tags := tagsFromAttributes([]attribute.KeyValue{
		attribute.String("clipanvil.agent.role", "producer"),
		attribute.Int64("clipanvil.agent.attempt", 2),
		attribute.Bool("clipanvil.agent.recovered", true),
	})

	if tags["clipanvil.agent.role"] != "producer" {
		t.Fatalf("role tag = %#v, want producer", tags["clipanvil.agent.role"])
	}
	if tags["clipanvil.agent.attempt"] != int64(2) {
		t.Fatalf("attempt tag = %#v, want int64(2)", tags["clipanvil.agent.attempt"])
	}
	if tags["clipanvil.agent.recovered"] != true {
		t.Fatalf("recovered tag = %#v, want true", tags["clipanvil.agent.recovered"])
	}
}

func TestSanitizingDataParserOmitsFunctionFieldsFromInput(t *testing.T) {
	type graphInput struct {
		WorkspaceID string
		EmitDelta   func()
		MaxToolCall int
	}

	parser := newSanitizingDataParser(loopcallback.NewDefaultDataParser(true))
	tags := parser.ParseInput(context.Background(), &callbacks.RunInfo{
		Component: compose.ComponentOfLambda,
	}, graphInput{
		WorkspaceID: "workspace-123",
		EmitDelta:   func() {},
		MaxToolCall: 3,
	})

	input, ok := tags[tracespec.Input].(string)
	if !ok {
		t.Fatalf("input tag = %#v, want string", tags[tracespec.Input])
	}
	if strings.Contains(input, "unsupported type") {
		t.Fatalf("input tag contains marshal error: %s", input)
	}
	if strings.Contains(input, "EmitDelta") {
		t.Fatalf("input tag contains runtime callback field: %s", input)
	}
	if !strings.Contains(input, "workspace-123") {
		t.Fatalf("input tag = %s, want workspace id", input)
	}
}
