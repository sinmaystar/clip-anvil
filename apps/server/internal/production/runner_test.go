package production

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestQueuedJobFailureEventIncludesCanvasRoutingFields(t *testing.T) {
	runErr := errors.New("persist output asset failed")
	job := db.GenerationJob{
		ID:           pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true},
		WorkspaceID:  pgtype.UUID{Bytes: [16]byte{0x02}, Valid: true},
		TargetNodeID: pgtype.UUID{Bytes: [16]byte{0x03}, Valid: true},
	}

	event := queuedJobFailureEvent(job, runErr)

	if event.Type != ProductionEventJobFailed {
		t.Fatalf("event type = %q, want %q", event.Type, ProductionEventJobFailed)
	}
	if event.JobID != job.ID || event.WorkspaceID != job.WorkspaceID || event.TargetNodeID != job.TargetNodeID {
		t.Fatalf("event routing fields = %#v, want job/workspace/node ids", event)
	}
	if event.Progress != 100 {
		t.Fatalf("progress = %d, want 100", event.Progress)
	}
	if !errors.Is(event.Err, runErr) {
		t.Fatalf("event error = %v, want %v", event.Err, runErr)
	}
	if event.Payload["error"] != runErr.Error() {
		t.Fatalf("payload error = %#v, want %q", event.Payload["error"], runErr.Error())
	}
}

func TestProductionRunnerEnqueuePreservesTraceContext(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	ctx, span := tracerProvider.Tracer("clipanvil-test").Start(context.Background(), "worker_generation")
	defer span.End()
	runner := NewProductionRunner(nil, nil, 1, nil)
	job := db.GenerationJob{ID: pgtype.UUID{Bytes: [16]byte{0x01}, Valid: true}}

	runner.Enqueue(ctx, job)
	request := <-runner.jobs

	got := trace.SpanContextFromContext(request.ctx)
	if !got.IsValid() || got.TraceID() != span.SpanContext().TraceID() {
		t.Fatalf("queued trace context = %v, want trace id %v", got, span.SpanContext().TraceID())
	}
}
