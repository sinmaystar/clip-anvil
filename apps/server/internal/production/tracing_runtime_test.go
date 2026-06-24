package production

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingRuntimeRecordsProviderEvents(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	runtime := NewTracingRuntime(
		fakeTracingRuntime{events: []ProductionEvent{
			{Type: ProductionEventProviderProgress, Progress: 20, Payload: map[string]any{"stage": "video_generation_started"}},
			{Type: ProductionEventJobSucceeded, Progress: 100, Output: ProductionOutput{AssetMIME: "video/mp4"}},
		}},
		tracerProvider.Tracer("clipanvil-test"),
	)

	events, err := runtime.Start(context.Background(), ProductionJob{
		ID:           uuidWithByte(30),
		WorkspaceID:  uuidWithByte(1),
		TargetNodeID: uuidWithByte(20),
	}, GenerationIntent{
		OutputType:    "video",
		OperationType: "image_to_video",
		Model:         ModelSpec{Provider: "volcengine", ModelID: "video-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}

	for _, span := range recorder.Ended() {
		if span.Name() != "production_provider_runtime" {
			continue
		}
		if got := spanAttribute(span, "clipanvil.production.operation_type"); got != "image_to_video" {
			t.Fatalf("operation attr = %q, want image_to_video", got)
		}
		if len(span.Events()) < 2 {
			t.Fatalf("span events = %#v, want provider progress and success", span.Events())
		}
		return
	}
	t.Fatalf("production_provider_runtime span not found; spans=%v", recorder.Ended())
}

type fakeTracingRuntime struct {
	events []ProductionEvent
	err    error
}

func (f fakeTracingRuntime) Start(context.Context, ProductionJob, GenerationIntent) (<-chan ProductionEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(chan ProductionEvent, len(f.events))
	for _, event := range f.events {
		out <- event
	}
	close(out)
	return out, nil
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}

func spanAttribute(span sdktrace.ReadOnlySpan, key string) string {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
}
