package cozelooptrace

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

type attributesContextKey struct{}

func ContextWithAttributes(ctx context.Context, attrs ...attribute.KeyValue) context.Context {
	if len(attrs) == 0 {
		return ctx
	}
	existing, _ := ctx.Value(attributesContextKey{}).([]attribute.KeyValue)
	merged := make([]attribute.KeyValue, 0, len(existing)+len(attrs))
	merged = append(merged, existing...)
	merged = append(merged, attrs...)
	return context.WithValue(ctx, attributesContextKey{}, merged)
}

func AttributesFromContext(ctx context.Context) []attribute.KeyValue {
	attrs, _ := ctx.Value(attributesContextKey{}).([]attribute.KeyValue)
	return attrs
}
