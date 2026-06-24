package cozelooptrace

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

func TestContextWithAttributesMergesAttributes(t *testing.T) {
	ctx := ContextWithAttributes(context.Background(), attribute.String("clipanvil.agent.role", "producer"))
	ctx = ContextWithAttributes(ctx, attribute.String("clipanvil.agent.task_type", "producer_run"))

	attrs := AttributesFromContext(ctx)
	if len(attrs) != 2 {
		t.Fatalf("attrs len = %d, want 2", len(attrs))
	}
	if attrs[0].Key != "clipanvil.agent.role" || attrs[0].Value.AsString() != "producer" {
		t.Fatalf("first attr = %#v", attrs[0])
	}
	if attrs[1].Key != "clipanvil.agent.task_type" || attrs[1].Value.AsString() != "producer_run" {
		t.Fatalf("second attr = %#v", attrs[1])
	}
}
