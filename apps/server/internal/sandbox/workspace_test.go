package sandbox

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestSafeAssetName(t *testing.T) {
	if got := SafeAssetName("产品 图.png"); got != "----.png" {
		t.Fatalf("SafeAssetName() = %q", got)
	}
	if got := SafeAssetName(""); got != "asset.bin" {
		t.Fatalf("SafeAssetName(empty) = %q", got)
	}
}

func TestBuildManifestDoesNotExposeCredentials(t *testing.T) {
	manifest := WorkspaceManifest{
		WorkspaceID: "workspace-1",
		AssetsDir:   AssetsDir,
		OutputDir:   OutputDir,
		Assets: []ManifestAsset{{
			ID:    "asset-1",
			Type:  "image",
			Mime:  "image/png",
			Path:  "/workspace/assets/asset-1-product.png",
			Title: "产品主图",
		}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	text := string(data)
	if text == "" {
		t.Fatal("manifest json must not be empty")
	}
	if strings.Contains(text, "clipanvil_dev") || strings.Contains(text, "MINIO") || strings.Contains(text, "secret") {
		t.Fatalf("manifest must not contain credentials: %s", text)
	}
}

func TestPrepareWorkspaceFilesIsIdempotent(t *testing.T) {
	client := &workspaceFakeClient{uploads: map[string]string{}}
	assets := []AssetForPreload{{
		ID:       "asset-1",
		Type:     "image",
		Mime:     "image/png",
		Title:    "产品主图",
		Filename: "产品 图.png",
		Reader:   strings.NewReader("image-data"),
	}}

	first, err := PrepareWorkspaceFiles(context.Background(), client, "sandbox-1", "workspace-1", assets)
	if err != nil {
		t.Fatalf("PrepareWorkspaceFiles first error = %v", err)
	}
	assets[0].Reader = strings.NewReader("image-data")
	second, err := PrepareWorkspaceFiles(context.Background(), client, "sandbox-1", "workspace-1", assets)
	if err != nil {
		t.Fatalf("PrepareWorkspaceFiles second error = %v", err)
	}

	if first.Assets[0].Path != "/workspace/assets/asset-1-----.png" {
		t.Fatalf("asset path = %q", first.Assets[0].Path)
	}
	if first.Assets[0].Path != second.Assets[0].Path {
		t.Fatalf("asset path changed: %q -> %q", first.Assets[0].Path, second.Assets[0].Path)
	}
	if client.execCalls != 2 {
		t.Fatalf("exec calls = %d, want 2", client.execCalls)
	}
	if got := client.uploads["/workspace/assets/asset-1-----.png"]; got != "image-data" {
		t.Fatalf("uploaded asset = %q", got)
	}
	manifestJSON := client.uploads["/workspace/manifest.json"]
	if manifestJSON == "" {
		t.Fatal("manifest was not uploaded")
	}
	if strings.Contains(manifestJSON, "clipanvil_dev") || strings.Contains(manifestJSON, "secret") {
		t.Fatalf("manifest contains credentials: %s", manifestJSON)
	}
}

type workspaceFakeClient struct {
	execCalls int
	uploads   map[string]string
}

func (f *workspaceFakeClient) Create(ctx context.Context, req CreateRequest) (SandboxInfo, error) {
	return SandboxInfo{}, nil
}

func (f *workspaceFakeClient) Get(ctx context.Context, sandboxID string) (SandboxInfo, error) {
	return SandboxInfo{}, nil
}

func (f *workspaceFakeClient) Ping(ctx context.Context, sandboxID string) error {
	return nil
}

func (f *workspaceFakeClient) Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecResult, error) {
	f.execCalls++
	return ExecResult{}, nil
}

func (f *workspaceFakeClient) Upload(ctx context.Context, sandboxID string, path string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.uploads[path] = string(data)
	return nil
}

func (f *workspaceFakeClient) Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, FileInfo, error) {
	return io.NopCloser(strings.NewReader("")), FileInfo{}, nil
}

func (f *workspaceFakeClient) Delete(ctx context.Context, sandboxID string) error {
	return nil
}
