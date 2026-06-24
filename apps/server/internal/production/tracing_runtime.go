package production

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type tracingRuntime struct {
	inner  EinoProductionRuntime
	tracer trace.Tracer
}

func NewTracingRuntime(inner EinoProductionRuntime, tracer trace.Tracer) EinoProductionRuntime {
	if inner == nil || tracer == nil {
		return inner
	}
	return tracingRuntime{inner: inner, tracer: tracer}
}

func (r tracingRuntime) Start(ctx context.Context, job ProductionJob, intent GenerationIntent) (<-chan ProductionEvent, error) {
	ctx, span := r.tracer.Start(ctx, "production_provider_runtime", trace.WithAttributes(
		attribute.String("clipanvil.workspace_id", uuidToString(job.WorkspaceID)),
		attribute.String("clipanvil.production.job_id", uuidToString(job.ID)),
		attribute.String("clipanvil.production.target_node_id", uuidToString(job.TargetNodeID)),
		attribute.String("clipanvil.production.provider", intent.Model.Provider),
		attribute.String("clipanvil.production.model_id", intent.Model.ModelID),
		attribute.String("clipanvil.production.output_type", intent.OutputType),
		attribute.String("clipanvil.production.operation_type", intent.OperationType),
	))
	events, err := r.inner.Start(ctx, job, intent)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, err
	}

	out := make(chan ProductionEvent, 8)
	go func() {
		defer close(out)
		defer span.End()
		statusSet := false
		for event := range events {
			span.AddEvent("production."+event.Type, trace.WithAttributes(productionEventAttributes(event)...))
			switch event.Type {
			case ProductionEventJobSucceeded:
				span.SetStatus(codes.Ok, "")
				statusSet = true
			case ProductionEventJobFailed, ProductionEventJobCancelled:
				if event.Err != nil {
					span.RecordError(event.Err)
					span.SetStatus(codes.Error, event.Err.Error())
				} else {
					span.SetStatus(codes.Error, string(event.Type))
				}
				statusSet = true
			}
			out <- event
		}
		if !statusSet {
			span.SetStatus(codes.Ok, "")
		}
	}()
	return out, nil
}

func productionEventAttributes(event ProductionEvent) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("clipanvil.production.event_type", event.Type),
		attribute.Int64("clipanvil.production.progress", int64(event.Progress)),
	}
	if stage, ok := event.Payload["stage"].(string); ok && stage != "" {
		attrs = append(attrs, attribute.String("clipanvil.production.stage", stage))
	}
	if event.Err != nil {
		attrs = append(attrs, attribute.String("clipanvil.production.error", event.Err.Error()))
	}
	if event.Output.AssetMIME != "" {
		attrs = append(attrs, attribute.String("clipanvil.production.asset_mime", event.Output.AssetMIME))
	}
	if event.Output.AssetSourceURL != "" {
		attrs = append(attrs, attribute.String("clipanvil.production.asset_source", "url"))
	}
	if len(event.Output.AssetContent) > 0 {
		attrs = append(attrs, attribute.String("clipanvil.production.asset_source", "content"))
	}
	if event.Output.TextContent != "" {
		attrs = append(attrs, attribute.String("clipanvil.production.output_kind", "text"))
	}
	if event.Output.AssetStorageURL != "" {
		attrs = append(attrs, attribute.String("clipanvil.production.asset_storage_url", event.Output.AssetStorageURL))
	}
	if event.Output.AssetThumbnailURL != "" {
		attrs = append(attrs, attribute.String("clipanvil.production.asset_thumbnail_url", event.Output.AssetThumbnailURL))
	}
	if event.Payload["error"] != nil {
		attrs = append(attrs, attribute.String("clipanvil.production.payload_error", fmt.Sprint(event.Payload["error"])))
	}
	return attrs
}
