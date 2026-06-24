package cozelooptrace

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/compose"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestCallbackHandlerRecordsEinoGraphSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	tracer := tp.Tracer("clipanvil-test")

	graph := compose.NewGraph[string, string]()
	if err := graph.AddLambdaNode("load_context", compose.InvokableLambda(func(_ context.Context, input string) (string, error) {
		return input + " loaded", nil
	})); err != nil {
		t.Fatalf("AddLambdaNode(load_context): %v", err)
	}
	if err := graph.AddLambdaNode("mock_model", compose.InvokableLambda(func(_ context.Context, input string) (string, error) {
		return input + " answered", nil
	})); err != nil {
		t.Fatalf("AddLambdaNode(mock_model): %v", err)
	}
	if err := graph.AddEdge(compose.START, "load_context"); err != nil {
		t.Fatalf("AddEdge start: %v", err)
	}
	if err := graph.AddEdge("load_context", "mock_model"); err != nil {
		t.Fatalf("AddEdge middle: %v", err)
	}
	if err := graph.AddEdge("mock_model", compose.END); err != nil {
		t.Fatalf("AddEdge end: %v", err)
	}
	runnable, err := graph.Compile(context.Background(), compose.WithGraphName("trace_smoke"))
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	output, err := runnable.Invoke(context.Background(), "input", compose.WithCallbacks(NewCallbackHandler(tracer)))
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if output != "input loaded answered" {
		t.Fatalf("output = %q", output)
	}

	names := map[string]bool{}
	lambdaCount := 0
	for _, span := range recorder.Ended() {
		names[span.Name()] = true
		if span.Name() == "Lambda" {
			lambdaCount++
		}
	}
	if !names["trace_smoke"] {
		t.Fatalf("missing graph span trace_smoke; got names=%v", names)
	}
	if lambdaCount < 2 {
		t.Fatalf("lambda spans = %d, want at least 2; got names=%v", lambdaCount, names)
	}
}
