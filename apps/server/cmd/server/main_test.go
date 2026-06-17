package main

import (
	"context"
	"io"
	"net/http"
	"testing"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
