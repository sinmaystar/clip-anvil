package main

import (
	"context"
	"io"
	"net/http"
	"testing"

	agentproducer "github.com/sinmaystar/clip-anvil/internal/agent/producer"
	"github.com/sinmaystar/clip-anvil/internal/config"
)

func TestSandboxHealthURLUsesServerHealthEndpoint(t *testing.T) {
	got, err := sandboxHealthURL("http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("sandboxHealthURL error = %v", err)
	}
	if got != "http://localhost:8080/health" {
		t.Fatalf("sandboxHealthURL = %q, want http://localhost:8080/health", got)
	}
}

func TestCheckSandboxServerHealthRequiresOK(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "http://localhost:8080/health" {
			t.Fatalf("request URL = %q", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(nil),
		}, nil
	})}

	if err := checkSandboxServerHealth(context.Background(), client, "http://localhost:8080/v1"); err == nil {
		t.Fatal("expected non-OK sandbox health response to fail")
	}
}

func TestProducerResponderForConfigUsesDeterministicOutsideRealMode(t *testing.T) {
	responder := producerResponderForConfig(&config.Config{
		Production: config.ProductionConfig{
			ProviderMode: "mock",
		},
	})
	if _, ok := responder.(agentproducer.DeterministicResponder); !ok {
		t.Fatalf("expected deterministic responder, got %T", responder)
	}
}

func TestProducerResponderForConfigUsesDeterministicWhenRealModeHasNoKey(t *testing.T) {
	responder := producerResponderForConfig(&config.Config{
		Production: config.ProductionConfig{
			ProviderMode: "real",
			Volcengine: config.VolcengineConfig{
				APIKey: "   ",
			},
		},
	})
	if _, ok := responder.(agentproducer.DeterministicResponder); !ok {
		t.Fatalf("expected deterministic responder, got %T", responder)
	}
}

func TestProducerResponderForConfigUsesM1E2EFixtureWhenEnabled(t *testing.T) {
	t.Setenv("CLIPANVIL_E2E_PRODUCER_FIXTURE", "m1_creative_state")
	responder := producerResponderForConfig(&config.Config{
		Production: config.ProductionConfig{
			ProviderMode: "mock",
		},
	})

	out, err := responder.Respond(context.Background(), agentproducer.ProducerContext{})
	if err != nil {
		t.Fatal(err)
	}
	if out.ModelMessage == nil || len(out.ModelMessage.ToolCalls) != 1 {
		t.Fatalf("fixture first response tool calls = %#v", out.ModelMessage)
	}
	if out.ModelMessage.ToolCalls[0].Function.Name != "upsert_project_brief" {
		t.Fatalf("first fixture tool = %q", out.ModelMessage.ToolCalls[0].Function.Name)
	}
}

func TestProducerResponderForConfigUsesVolcengineWhenRealModeHasKey(t *testing.T) {
	responder := producerResponderForConfig(&config.Config{
		Production: config.ProductionConfig{
			ProviderMode: "real",
			Volcengine: config.VolcengineConfig{
				APIKey:    "test-key",
				BaseURL:   "https://example.com",
				Region:    "cn-beijing",
				TextModel: "test-model",
			},
		},
	})
	if _, ok := responder.(agentproducer.VolcengineModelResponder); !ok {
		t.Fatalf("expected Volcengine responder, got %T", responder)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
