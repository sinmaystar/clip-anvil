package main

import (
	"context"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/agent/cozelooptrace"
)

func TestInitAgentTracingFromConfigDisablesTracingWhenConfigMissing(t *testing.T) {
	tracing := initAgentTracingFromConfig(context.Background(), nil, cozelooptrace.Config{})

	if len(tracing.Callbacks) != 0 {
		t.Fatalf("callbacks len = %d, want 0", len(tracing.Callbacks))
	}
	if tracing.Tracer != nil {
		t.Fatal("tracer is enabled, want nil")
	}
	if tracing.Shutdown == nil {
		t.Fatal("shutdown is nil")
	}
	if err := tracing.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error = %v", err)
	}
}
