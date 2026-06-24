package cozelooptrace

import (
	"context"

	"github.com/cloudwego/eino/callbacks"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type spanContextKey struct{}

func NewCallbackHandler(tracer trace.Tracer) callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
			spanName := runInfoName(info)
			ctx, span := tracer.Start(ctx, spanName,
				trace.WithAttributes(runInfoAttributes(info)...),
			)
			return context.WithValue(ctx, spanContextKey{}, span)
		}).
		OnEndFn(func(ctx context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackOutput) context.Context {
			if span := spanFromContext(ctx); span != nil {
				span.SetStatus(codes.Ok, "")
				span.End()
			}
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, _ *callbacks.RunInfo, err error) context.Context {
			if span := spanFromContext(ctx); span != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				span.End()
			}
			return ctx
		}).
		Build()
}

func runInfoName(info *callbacks.RunInfo) string {
	if info == nil {
		return "eino.unknown"
	}
	if info.Name != "" {
		return info.Name
	}
	if info.Component != "" {
		return string(info.Component)
	}
	if info.Type != "" {
		return info.Type
	}
	return "eino.unknown"
}

func runInfoAttributes(info *callbacks.RunInfo) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("cozeloop.span_type", "function"),
	}
	if info == nil {
		return attrs
	}
	if info.Name != "" {
		attrs = append(attrs, attribute.String("eino.name", info.Name))
	}
	if info.Type != "" {
		attrs = append(attrs, attribute.String("eino.type", info.Type))
	}
	if info.Component != "" {
		attrs = append(attrs, attribute.String("eino.component", string(info.Component)))
		if info.Component == "Graph" {
			attrs[0] = attribute.String("cozeloop.span_type", "graph")
		}
	}
	return attrs
}

func spanFromContext(ctx context.Context) trace.Span {
	span, _ := ctx.Value(spanContextKey{}).(trace.Span)
	return span
}
