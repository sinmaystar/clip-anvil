package sandbox

import (
	"context"
	"errors"
	"strings"
)

const (
	DefaultExecTimeoutSeconds = 120
	MaxExecTimeoutSeconds     = 1800
	DefaultOutputLimitBytes   = 64 << 10
)

type ExecInput struct {
	Command        string
	Cwd            string
	TimeoutSeconds int
}

func BuildExecRequest(input ExecInput) (ExecRequest, error) {
	command := strings.TrimSpace(input.Command)
	if command == "" {
		return ExecRequest{}, errors.New("command is required")
	}
	cwd := strings.TrimSpace(input.Cwd)
	if cwd == "" {
		cwd = DefaultWorkdir
	}
	if cwd != DefaultWorkdir && !strings.HasPrefix(cwd, DefaultWorkdir+"/") {
		return ExecRequest{}, errors.New("cwd must be inside /workspace")
	}
	timeout := input.TimeoutSeconds
	if timeout == 0 {
		timeout = DefaultExecTimeoutSeconds
	}
	if timeout < 1 || timeout > MaxExecTimeoutSeconds {
		return ExecRequest{}, errors.New("timeout_seconds out of range")
	}
	return ExecRequest{
		Command:        "bash -lc " + shellQuote(command),
		Cwd:            cwd,
		TimeoutSeconds: timeout,
	}, nil
}

func RunExec(ctx context.Context, client Client, sandboxID string, input ExecInput) (ExecResult, error) {
	req, err := BuildExecRequest(input)
	if err != nil {
		return ExecResult{}, err
	}
	result, err := client.Exec(ctx, sandboxID, req)
	if err != nil {
		return ExecResult{}, err
	}
	stdout, stdoutTruncated := TruncateOutput(result.Stdout, DefaultOutputLimitBytes)
	stderr, stderrTruncated := TruncateOutput(result.Stderr, DefaultOutputLimitBytes)
	result.Stdout = stdout
	result.Stderr = stderr
	result.Truncated = result.Truncated || stdoutTruncated || stderrTruncated
	return result, nil
}

func TruncateOutput(s string, limit int) (string, bool) {
	if limit < 0 || len(s) <= limit {
		return s, false
	}
	return s[:limit], true
}

func execTimeoutMillis(seconds int) int64 {
	return int64(seconds) * 1000
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
