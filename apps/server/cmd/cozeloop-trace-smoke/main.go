package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/compose"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/sinmaystar/clip-anvil/internal/agent/cozelooptrace"
)

type smokeInput struct {
	Prompt string
}

type smokeContext struct {
	Prompt string
	Loaded string
}

type smokeDraft struct {
	Prompt string
	Answer string
}

type smokeOutput struct {
	Answer string
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "cozeloop trace smoke failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	loadDotEnvFiles()

	flags := flag.NewFlagSet("cozeloop-trace-smoke", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	endpoint := flags.String("endpoint", envOrDefault("CLIPANVIL_COZELOOP_ENDPOINT", cozelooptrace.DefaultEndpoint), "Coze Loop app endpoint")
	workspaceID := flags.String("workspace-id", os.Getenv("CLIPANVIL_COZELOOP_WORKSPACE_ID"), "Coze Loop workspace id")
	authorization := flags.String("authorization", envOrDefault("CLIPANVIL_COZELOOP_AUTHORIZATION", os.Getenv("CLIPANVIL_COZELOOP_PAT")), "Coze Loop PAT token or Authorization header")
	serviceName := flags.String("service-name", envOrDefault("CLIPANVIL_COZELOOP_SERVICE_NAME", "clipanvil-cozeloop-trace-smoke"), "OpenTelemetry service.name")
	prompt := flags.String("prompt", "hello from ClipAnvil Eino smoke", "demo prompt text")
	if err := flags.Parse(args); err != nil {
		return err
	}

	config := cozelooptrace.Config{
		Endpoint:      *endpoint,
		WorkspaceID:   *workspaceID,
		Authorization: *authorization,
		ServiceName:   *serviceName,
		Timeout:       10 * time.Second,
	}
	tracerProvider, err := cozelooptrace.NewTracerProvider(ctx, config)
	if err != nil {
		if errors.Is(err, cozelooptrace.ErrMissingWorkspaceID) {
			return fmt.Errorf("%w; set CLIPANVIL_COZELOOP_WORKSPACE_ID or pass -workspace-id", err)
		}
		if errors.Is(err, cozelooptrace.ErrMissingAuthorization) {
			return fmt.Errorf("%w; create a Coze Loop PAT and set CLIPANVIL_COZELOOP_PAT, CLIPANVIL_COZELOOP_AUTHORIZATION, or pass -authorization", err)
		}
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = tracerProvider.Shutdown(shutdownCtx)
	}()

	tracer := tracerProvider.Tracer("clipanvil/eino/cozeloop-smoke")
	rootCtx, rootSpan := tracer.Start(ctx, "clipanvil.coze_loop.trace_smoke",
		trace.WithAttributes(traceAttributes(config.WorkspaceID, *prompt)...),
	)

	graph, err := newSmokeGraph()
	if err != nil {
		rootSpan.RecordError(err)
		rootSpan.SetStatus(codes.Error, err.Error())
		rootSpan.End()
		return err
	}
	output, err := graph.Invoke(rootCtx, smokeInput{Prompt: *prompt}, compose.WithCallbacks(cozelooptrace.NewCallbackHandler(tracer)))
	if err != nil {
		rootSpan.RecordError(err)
		rootSpan.SetStatus(codes.Error, err.Error())
		rootSpan.End()
		return err
	}
	traceID := rootSpan.SpanContext().TraceID().String()
	rootSpan.SetAttributes(attribute.String("smoke.answer", output.Answer))
	rootSpan.SetStatus(codes.Ok, "")
	rootSpan.End()

	flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tracerProvider.ForceFlush(flushCtx); err != nil {
		return err
	}

	endpointURL, _ := config.OTLPEndpointURL()
	fmt.Printf("sent Coze Loop trace\n")
	fmt.Printf("workspace_id: %s\n", config.WorkspaceID)
	fmt.Printf("trace_id: %s\n", traceID)
	fmt.Printf("endpoint: %s\n", endpointURL)
	fmt.Printf("answer: %s\n", output.Answer)
	return nil
}

func newSmokeGraph() (compose.Runnable[smokeInput, smokeOutput], error) {
	graph := compose.NewGraph[smokeInput, smokeOutput]()
	if err := graph.AddLambdaNode("load_context", compose.InvokableLambda(func(_ context.Context, input smokeInput) (smokeContext, error) {
		return smokeContext{
			Prompt: input.Prompt,
			Loaded: "workspace context loaded",
		}, nil
	})); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode("mock_model", compose.InvokableLambda(func(_ context.Context, input smokeContext) (smokeDraft, error) {
		return smokeDraft{
			Prompt: input.Prompt,
			Answer: fmt.Sprintf("mock answer for %q with %s", input.Prompt, input.Loaded),
		}, nil
	})); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode("finalize", compose.InvokableLambda(func(_ context.Context, input smokeDraft) (smokeOutput, error) {
		return smokeOutput{Answer: input.Answer}, nil
	})); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(compose.START, "load_context"); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("load_context", "mock_model"); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("mock_model", "finalize"); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("finalize", compose.END); err != nil {
		return nil, err
	}
	return graph.Compile(context.Background(), compose.WithGraphName("clipanvil_cozeloop_trace_smoke"))
}

func traceAttributes(workspaceID string, prompt string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("cozeloop.span_type", "graph"),
		attribute.String("cozeloop.workspace_id", workspaceID),
		attribute.String("smoke.prompt", prompt),
	}
}

func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func loadDotEnvFiles() {
	candidates := []string{
		".env",
		filepath.Join("..", "..", ".env"),
		filepath.Join("deploy", "cozeloop", ".env"),
		filepath.Join("..", "..", "deploy", "cozeloop", ".env"),
	}
	for _, path := range candidates {
		loadDotEnv(path)
	}
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
