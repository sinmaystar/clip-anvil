package sandbox

import (
	"context"
	"strings"
	"testing"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestJobServiceRenderTemplateVideoWritesProjectRunsHyperFramesAndUploadsMP4(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{
		result: ExecResult{ExitCode: 0, Stdout: "rendered", DurationMS: 900},
		inspect: FileInfo{
			Path:      "/workspace/output/template-node-1.mp4",
			SizeBytes: 789,
			Mime:      "video/mp4",
		},
	}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	storage := &fakeSandboxJobStorage{}
	service := NewJobService(manager, client, repo, storage)

	result, err := service.RenderTemplateVideo(context.Background(), RenderTemplateVideoInput{
		WorkspaceID:  testWorkspaceID(),
		TargetNodeID: testNodeID(),
		TemplateKey:  "static_fallback_ken_burns_v1",
		HTML:         `<html data-composition-id="static_fallback_ken_burns_v1"></html>`,
		Meta:         TemplateVideoMeta{DurationSec: 5, Width: 1080, Height: 1920, FPS: 24},
		Variables:    map[string]any{"headline": "轻装出发"},
		Assets: []RenderTemplateAssetInput{{
			AssetID:    "product-image",
			StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/product.png",
			Mime:       "image/png",
			FileName:   "product.png",
		}},
	})
	if err != nil {
		t.Fatalf("RenderTemplateVideo error = %v", err)
	}
	if result.Job.Status != db.JobStatusSucceeded || result.Job.OperationType != "template_to_video" {
		t.Fatalf("job = %#v", result.Job)
	}
	if result.MIME != "video/mp4" || result.Size != 123 || result.Asset.StorageURL == "" {
		t.Fatalf("result = %#v", result)
	}
	joined := strings.Join(client.commands, "\n")
	if !strings.Contains(joined, "curl -sS -f -L -o") || !strings.Contains(joined, "product.png") {
		t.Fatalf("expected asset download command, got %q", joined)
	}
	if !strings.Contains(joined, "/workspace/template-video/") || !strings.Contains(joined, "/assets/product.png") {
		t.Fatalf("expected template asset to be downloaded into the project assets directory, got %q", joined)
	}
	if !strings.Contains(joined, "hyperframes render .") ||
		!strings.Contains(joined, "test -s") ||
		!strings.Contains(joined, "/workspace/output/template-") ||
		!strings.Contains(joined, "--fps 24") ||
		!strings.Contains(result.Job.Cwd, "/workspace/template-video/") {
		t.Fatalf("expected HyperFrames command in template project cwd, command=%q cwd=%q", joined, result.Job.Cwd)
	}
	uploads := strings.Join(client.uploads, "\n")
	for _, want := range []string{"index.html", "meta.json", "variables.json"} {
		if !strings.Contains(uploads, want) {
			t.Fatalf("expected upload %q, got %q", want, uploads)
		}
	}
	if len(storage.putKeys) != 1 || !strings.HasSuffix(storage.putKeys[0], ".mp4") {
		t.Fatalf("put keys = %#v", storage.putKeys)
	}
}

func TestJobServiceRenderTemplateVideoRejectsInvalidAssetStorageURL(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{result: ExecResult{ExitCode: 0}}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status:     StatusRunning,
		SandboxID:  "sandbox-1",
		VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	service := NewJobService(manager, client, repo, &fakeSandboxJobStorage{})

	_, err := service.RenderTemplateVideo(context.Background(), RenderTemplateVideoInput{
		WorkspaceID:  testWorkspaceID(),
		TargetNodeID: testNodeID(),
		TemplateKey:  "static_fallback_ken_burns_v1",
		HTML:         `<html></html>`,
		Meta:         TemplateVideoMeta{DurationSec: 5, Width: 1080, Height: 1920, FPS: 24},
		Assets: []RenderTemplateAssetInput{{
			AssetID:    "bad",
			StorageURL: "https://example.com/not-workspace.png",
			Mime:       "image/png",
			FileName:   "bad.png",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "storage URL") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(strings.Join(client.commands, "\n"), "hyperframes render") {
		t.Fatalf("render command should not run after invalid storage URL: %#v", client.commands)
	}
}
