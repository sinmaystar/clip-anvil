package sandbox

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

func DownloadFromMinIO(ctx context.Context, client Client, sandboxID string, presignedURL string, destPath string) (ExecResult, error) {
	return DownloadURLToSandbox(ctx, client, sandboxID, presignedURL, destPath)
}

func DownloadURLToSandbox(ctx context.Context, client Client, sandboxID string, sourceURL string, destPath string) (ExecResult, error) {
	if err := validateTransferPath(destPath); err != nil {
		return ExecResult{}, err
	}
	if err := validatePresignedURL(sourceURL); err != nil {
		return ExecResult{}, err
	}
	command := "mkdir -p " + shellQuote(parentDir(destPath)) + " && curl -sS -f -L -o " + shellQuote(destPath) + " " + shellQuote(sourceURL)
	return RunExec(ctx, client, sandboxID, ExecInput{Command: command, TimeoutSeconds: DefaultExecTimeoutSeconds})
}

func UploadToMinIO(ctx context.Context, client Client, sandboxID string, srcPath string, presignedURL string) (ExecResult, error) {
	if err := validateTransferPath(srcPath); err != nil {
		return ExecResult{}, err
	}
	if err := validatePresignedURL(presignedURL); err != nil {
		return ExecResult{}, err
	}
	command := "curl -sS -f -L -X PUT -T " + shellQuote(srcPath) + " " + shellQuote(presignedURL)
	return RunExec(ctx, client, sandboxID, ExecInput{Command: command, TimeoutSeconds: DefaultExecTimeoutSeconds})
}

func validateTransferPath(path string) error {
	path = strings.TrimSpace(path)
	if path != DefaultWorkdir && !strings.HasPrefix(path, DefaultWorkdir+"/") {
		return errors.New("path must be inside /workspace")
	}
	return nil
}

func validatePresignedURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return errors.New("invalid presigned URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("invalid presigned URL")
	}
	return nil
}

func parentDir(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx <= 0 {
		return DefaultWorkdir
	}
	return path[:idx]
}
