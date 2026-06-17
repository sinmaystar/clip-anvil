package sandbox

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestDownloadFromMinIOBuildsCurlCommand(t *testing.T) {
	client := &transferFakeClient{}
	result, err := DownloadFromMinIO(context.Background(), client, "sandbox-1", "http://host.docker.internal:9000/bucket/key?X-Amz-Signature=abc", "/workspace/assets/input.txt")
	if err != nil {
		t.Fatalf("DownloadFromMinIO error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
	if !strings.Contains(client.command, "curl -sS -f -L -o") ||
		!strings.Contains(client.command, "/workspace/assets/input.txt") ||
		!strings.Contains(client.command, "http://host.docker.internal:9000/bucket/key?X-Amz-Signature=abc") {
		t.Fatalf("command = %q", client.command)
	}
}

func TestUploadToMinIOBuildsCurlCommand(t *testing.T) {
	client := &transferFakeClient{}
	_, err := UploadToMinIO(context.Background(), client, "sandbox-1", "/workspace/output/result.mp4", "http://host.docker.internal:9000/bucket/result.mp4?X-Amz-Signature=abc")
	if err != nil {
		t.Fatalf("UploadToMinIO error = %v", err)
	}
	if !strings.Contains(client.command, "curl -sS -f -L -X PUT -T") ||
		!strings.Contains(client.command, "/workspace/output/result.mp4") ||
		!strings.Contains(client.command, "http://host.docker.internal:9000/bucket/result.mp4?X-Amz-Signature=abc") {
		t.Fatalf("command = %q", client.command)
	}
}

func TestMinIOTransferRejectsUnsafePathAndURL(t *testing.T) {
	client := &transferFakeClient{}
	if _, err := DownloadFromMinIO(context.Background(), client, "sandbox-1", "http://host/key", "/tmp/input.txt"); err == nil {
		t.Fatal("expected unsafe destination path to be rejected")
	}
	if _, err := UploadToMinIO(context.Background(), client, "sandbox-1", "/workspace/output/result.mp4", "file:///etc/passwd"); err == nil {
		t.Fatal("expected unsafe URL to be rejected")
	}
}

type transferFakeClient struct {
	command string
}

func (f *transferFakeClient) Create(ctx context.Context, req CreateRequest) (SandboxInfo, error) {
	return SandboxInfo{}, nil
}

func (f *transferFakeClient) Get(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	return SandboxInfo{}, nil
}

func (f *transferFakeClient) Ping(ctx context.Context, sandboxID string) error {
	return nil
}

func (f *transferFakeClient) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecResult, error) {
	f.command = req.Command
	return ExecResult{ExitCode: 0}, nil
}

func (f *transferFakeClient) Upload(ctx context.Context, sandboxID string, path string, r io.Reader) error {
	return nil
}

func (f *transferFakeClient) Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, FileInfo, error) {
	return io.NopCloser(strings.NewReader("")), FileInfo{}, nil
}

func (f *transferFakeClient) Delete(ctx context.Context, sandboxID string) error {
	return nil
}
