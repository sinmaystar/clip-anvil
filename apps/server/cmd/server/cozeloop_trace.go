package main

import (
	"context"
	"log/slog"

	"github.com/cloudwego/eino/callbacks"
	"go.opentelemetry.io/otel/trace"

	"github.com/sinmaystar/clip-anvil/internal/agent/cozelooptrace"
)

type agentTracing struct {
	Callbacks []callbacks.Handler
	Tracer    trace.Tracer
	Shutdown  func(context.Context) error
}

func initAgentTracing(ctx context.Context, logger *slog.Logger) agentTracing {
	cozelooptrace.LoadDotEnvFiles(
		".env",
		"../../.env",
		"deploy/cozeloop/.env",
		"../../deploy/cozeloop/.env",
	)

	cfg := cozelooptrace.ConfigFromEnv()
	return initAgentTracingFromConfig(ctx, logger, cfg)
}

func initAgentTracingFromConfig(ctx context.Context, logger *slog.Logger, cfg cozelooptrace.Config) agentTracing {
	client, err := cozelooptrace.NewClient(cfg)
	if err != nil {
		loggerOrDefault(logger).Warn("cozeloop agent tracing disabled", "error", err)
		return agentTracing{Shutdown: func(context.Context) error { return nil }}
	}

	var tracer trace.Tracer
	shutdownTracer := func(context.Context) error { return nil }
	tracerProvider, err := cozelooptrace.NewTracerProvider(ctx, cfg)
	if err != nil {
		loggerOrDefault(logger).Warn("cozeloop opentelemetry spans disabled", "error", err)
	} else {
		tracer = tracerProvider.Tracer("clipanvil/agent")
		shutdownTracer = tracerProvider.Shutdown
	}

	loggerOrDefault(logger).Info("cozeloop agent tracing enabled", "endpoint", cfg.Endpoint, "workspace_id", cfg.WorkspaceID)
	return agentTracing{
		Callbacks: []callbacks.Handler{cozelooptrace.NewOfficialCallbackHandler(client, cfg.ServiceName)},
		Tracer:    tracer,
		Shutdown: func(ctx context.Context) error {
			err := shutdownTracer(ctx)
			client.Close(ctx)
			return err
		},
	}
}

func loggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}
