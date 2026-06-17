package sandbox

import (
	"context"
	"io"
	"strings"
	"testing"

	osb "github.com/alibaba/OpenSandbox/sdks/sandbox/go"
)

func TestBuildExecRequestDefaults(t *testing.T) {
	req, err := BuildExecRequest(ExecInput{Command: "echo ok"})
	if err != nil {
		t.Fatalf("BuildExecRequest error = %v", err)
	}
	if req.Cwd != "/workspace" {
		t.Fatalf("cwd = %q, want /workspace", req.Cwd)
	}
	if req.TimeoutSeconds != 120 {
		t.Fatalf("timeout = %d, want 120", req.TimeoutSeconds)
	}
	if req.Command != "bash -lc 'echo ok'" {
		t.Fatalf("command = %q", req.Command)
	}
}

func TestBuildExecRequestRejectsOutsideWorkspace(t *testing.T) {
	_, err := BuildExecRequest(ExecInput{Command: "pwd", Cwd: "/etc"})
	if err == nil {
		t.Fatal("expected error for cwd outside /workspace")
	}
}

func TestBuildExecRequestRejectsInvalidTimeout(t *testing.T) {
	for _, timeout := range []int{-1, 1801} {
		if _, err := BuildExecRequest(ExecInput{Command: "pwd", TimeoutSeconds: timeout}); err == nil {
			t.Fatalf("expected error for timeout %d", timeout)
		}
	}
}

func TestTruncateOutput(t *testing.T) {
	out, truncated := TruncateOutput(strings.Repeat("a", 10), 4)
	if !truncated {
		t.Fatal("expected truncated output")
	}
	if out != "aaaa" {
		t.Fatalf("output = %q, want aaaa", out)
	}
}

func TestExecTimeoutSecondsAreConvertedToMilliseconds(t *testing.T) {
	if got := execTimeoutMillis(30); got != 30000 {
		t.Fatalf("execTimeoutMillis(30) = %d, want 30000", got)
	}
}

func TestOutputTextPreservesLineBoundaries(t *testing.T) {
	got := outputText([]osb.OutputMessage{{Text: "first"}, {Text: "second"}})
	if got != "first\nsecond" {
		t.Fatalf("outputText() = %q, want line-separated output", got)
	}
}

func TestRunExecPreservesNonZeroExitAndTruncatesOutput(t *testing.T) {
	client := &execFakeClient{result: ExecResult{
		ExitCode: 2,
		Stdout:   strings.Repeat("o", DefaultOutputLimitBytes+1),
		Stderr:   "failed",
	}}
	result, err := RunExec(context.Background(), client, "sandbox-1", ExecInput{Command: "false"})
	if err != nil {
		t.Fatalf("RunExec error = %v", err)
	}
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if len(result.Stdout) != DefaultOutputLimitBytes {
		t.Fatalf("stdout len = %d, want %d", len(result.Stdout), DefaultOutputLimitBytes)
	}
	if !result.Truncated {
		t.Fatal("expected result to be marked truncated")
	}
}

type execFakeClient struct {
	result ExecResult
}

func (f *execFakeClient) Create(ctx context.Context, req CreateRequest) (SandboxInfo, error) {
	return SandboxInfo{}, nil
}

func (f *execFakeClient) Get(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	return SandboxInfo{}, nil
}

func (f *execFakeClient) Ping(ctx context.Context, sandboxID string) error {
	return nil
}

func (f *execFakeClient) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecResult, error) {
	return f.result, nil
}

func (f *execFakeClient) Upload(ctx context.Context, sandboxID string, path string, r io.Reader) error {
	return nil
}

func (f *execFakeClient) Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, FileInfo, error) {
	return io.NopCloser(strings.NewReader("")), FileInfo{}, nil
}

func (f *execFakeClient) Delete(ctx context.Context, sandboxID string) error {
	return nil
}
