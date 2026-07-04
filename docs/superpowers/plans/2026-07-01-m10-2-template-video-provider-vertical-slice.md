# M10.2 Template Video Provider Vertical Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first production vertical slice for `internal_template_video/hyperframes-html`: RenderPlan/Worker can submit a template video intent, the provider calls sandbox `RenderTemplateVideo`, sandbox renders a controlled HyperFrames template to MP4, and production persists the output as the shot video winner.

**Architecture:** Reuse the existing production provider bridge and sandbox job service. `TemplateVideoProvider` lives beside `InternalFFmpegProvider`, calls a new sandbox `RenderTemplateVideo` method, and returns `ProviderResult` so `production.Service` continues to create `generation_job`, `media_asset`, `artifact_version.winner`, and RenderPlan terminal state. Template HTML comes from a controlled Go template library; Agent only supplies `template_key`, `duration_sec`, `ratio`, `resolution`, `fps`, `variables`, and input refs.

**Tech Stack:** Go 1.26, pgx/sqlc, existing `production.ProviderBridge`, existing `sandbox.JobService`, HyperFrames 0.7.22 in the sandbox image, FFmpeg/ffprobe, MinIO storage.

---

## Files To Change

- Create: `apps/server/internal/templatevideo/template.go`
- Create: `apps/server/internal/templatevideo/template_test.go`
- Create: `apps/server/internal/sandbox/template_video.go`
- Create: `apps/server/internal/sandbox/template_video_test.go`
- Create: `apps/server/internal/production/template_video_provider.go`
- Create: `apps/server/internal/production/template_video_provider_test.go`
- Modify: `apps/server/internal/production/provider.go`
- Modify: `apps/server/internal/agent/tools/render_plan_submitter.go`
- Modify: `apps/server/internal/agent/tools/decide_render_plan_test.go`
- Modify: `apps/server/internal/agent/tools/render_plan_tools_test.go`
- Modify: `apps/server/cmd/server/main.go`
- Create: `scripts/smoke-m10-2-template-video-provider.sh`
- Modify: `docs/milestones/m10-hyperframes-template-video-provider.md`
- Create: `docs/superpowers/reports/2026-07-01-m10-2-template-video-provider-vertical-slice.md`

## Task 1: Controlled Template Library

**Files:**
- Create: `apps/server/internal/templatevideo/template.go`
- Create: `apps/server/internal/templatevideo/template_test.go`

- [ ] **Step 1: Write failing tests**

Add tests proving:

```go
func TestRenderStaticFallbackKenBurnsTemplateEscapesVariables(t *testing.T) {
	html, meta, err := Render(RenderInput{
		TemplateKey: "static_fallback_ken_burns_v1",
		DurationSec: 5,
		Ratio: "9:16",
		Resolution: "1080p",
		FPS: 24,
		Variables: map[string]any{
			"headline": `<script>alert("x")</script>`,
			"caption": "轻装出发",
			"cta": "现在了解",
		},
		Assets: []Asset{{ClientKey: "product", WorkspacePath: "/workspace/input/template-video/job-1/product.png", Mime: "image/png"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Width != 1080 || meta.Height != 1920 || meta.DurationFrames != 120 {
		t.Fatalf("meta = %#v", meta)
	}
	if strings.Contains(html, "<script>") || !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("html was not escaped: %s", html)
	}
	if !strings.Contains(html, "data-composition-id=\"static_fallback_ken_burns_v1\"") ||
		!strings.Contains(html, "window.__timelines") ||
		!strings.Contains(html, "/workspace/input/template-video/job-1/product.png") {
		t.Fatalf("html missing HyperFrames composition markers: %s", html)
	}
}
```

Also test invalid input:

```go
func TestRenderRejectsUnknownTemplateAndInvalidColor(t *testing.T) {
	_, _, err := Render(RenderInput{TemplateKey: "unknown", DurationSec: 5, Ratio: "9:16"})
	if err == nil || !strings.Contains(err.Error(), "unknown template_key") {
		t.Fatalf("error = %v", err)
	}
	_, _, err = Render(RenderInput{
		TemplateKey: "static_fallback_ken_burns_v1",
		DurationSec: 5,
		Ratio: "9:16",
		Variables: map[string]any{"brand_colors": []any{"red;display:none"}},
	})
	if err == nil || !strings.Contains(err.Error(), "brand_colors") {
		t.Fatalf("error = %v", err)
	}
}
```

- [ ] **Step 2: Run tests and verify RED**

Run from `apps/server`:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/templatevideo
```

Expected: package or `Render` undefined.

- [ ] **Step 3: Implement minimal library**

Implement:

- `RenderInput`
- `Asset`
- `Meta`
- `Render(input RenderInput) (html string, meta Meta, err error)`
- whitelist: only `static_fallback_ken_burns_v1`
- ratios: `9:16`, `16:9`, `1:1`
- durations: `3`, `4`, `5`, `6`, `8`, `10`
- resolutions: `720p`, `1080p`
- default fps: `24`
- escaped strings via `html/template`
- strict hex color validation for `brand_colors`

The generated HTML must include:

- root element with `data-composition-id`
- `data-width`, `data-height`, `data-duration`
- `window.__timelines = window.__timelines || {}`
- no external scripts

- [ ] **Step 4: Run tests and verify GREEN**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/templatevideo
```

Expected: PASS.

## Task 2: Sandbox RenderTemplateVideo

**Files:**
- Create: `apps/server/internal/sandbox/template_video.go`
- Create or modify: `apps/server/internal/sandbox/template_video_test.go`

- [ ] **Step 1: Write failing tests**

Add `TestJobServiceRenderTemplateVideoWritesProjectRunsHyperFramesAndUploadsMP4`:

```go
result, err := service.RenderTemplateVideo(context.Background(), RenderTemplateVideoInput{
	WorkspaceID:  testWorkspaceID(),
	TargetNodeID: testNodeID(),
	TemplateKey:  "static_fallback_ken_burns_v1",
	HTML:         "<html data-composition-id=\"static_fallback_ken_burns_v1\"></html>",
	Meta:         TemplateVideoMeta{DurationSec: 5, Width: 1080, Height: 1920, FPS: 24},
	Assets: []RenderTemplateAssetInput{{
		AssetID:    "product-image",
		StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/product.png",
		Mime:       "image/png",
		FileName:   "product.png",
	}},
})
```

Assert:

- created sandbox job has `OperationType == "template_to_video"`
- client uploaded `index.html`, `meta.json`, `variables.json`
- command contains `hyperframes render . --output /workspace/output/template-`
- command cwd is `/workspace/template-video/<job-id>`
- result MIME is `video/mp4`
- result asset storage URL is non-empty

- [ ] **Step 2: Write path failure test**

Add `TestJobServiceRenderTemplateVideoRejectsInvalidAssetStorageURL` asserting an invalid storage URL fails before render command.

- [ ] **Step 3: Run tests and verify RED**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'TestJobServiceRenderTemplateVideo'
```

Expected: `RenderTemplateVideo` undefined.

- [ ] **Step 4: Implement sandbox method**

Implement:

- `RenderTemplateVideoInput`
- `RenderTemplateAssetInput`
- `TemplateVideoMeta`
- `RenderTemplateVideo(ctx, input) (SandboxJobResult, error)`

Use existing helpers:

- `EnsureWorkspaceLayout`
- `storage.KeyFromStorageURL`
- `PresignedSandboxGetURL`
- `DownloadFromMinIO`
- `client.Upload`
- `RunExec`
- `InspectArtifact`
- `ValidateArtifactSize`
- `PresignedSandboxPutURL`
- `UploadToMinIO`
- `markFailed`

Command should be built from fixed args:

```bash
hyperframes render . --output /workspace/output/template-<job-id>.mp4 --fps <fps> --quality draft
```

Do not concatenate Agent-provided shell fragments. Only interpolate sanitized paths, integer fps, and known output path.

- [ ] **Step 5: Run tests and verify GREEN**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'TestJobServiceRenderTemplateVideo'
```

Expected: PASS.

## Task 3: TemplateVideoProvider

**Files:**
- Create: `apps/server/internal/production/template_video_provider.go`
- Create: `apps/server/internal/production/template_video_provider_test.go`
- Modify: `apps/server/internal/production/provider.go`

- [ ] **Step 1: Write failing provider tests**

Add tests:

```go
func TestTemplateVideoProviderUsesSandboxRenderer(t *testing.T) {
	renderer := &fakeTemplateRenderer{}
	provider := NewTemplateVideoProvider(renderer)
	result, err := provider.Run(context.Background(), GenerationIntent{
		WorkspaceID:   pgtype.UUID{Bytes: [16]byte{0xaa}, Valid: true},
		TargetNodeID:  pgtype.UUID{Bytes: [16]byte{0xbb}, Valid: true},
		OutputType:    "video",
		OperationType: "image_to_template_video",
		Model:         ModelSpec{Provider: "internal_template_video", ModelID: "hyperframes-html"},
		Params: map[string]any{
			"template_key": "static_fallback_ken_burns_v1",
			"duration_sec": float64(5),
			"ratio": "9:16",
			"resolution": "1080p",
			"variables": map[string]any{"headline": "轻松出发", "cta": "了解更多"},
		},
		InputRefs: []InputRef{{
			NodeType: "image",
			AssetID: "product-image",
			Mime: "image/png",
			StorageURL: "workspace-aabbccdd-0000-0000-0000-000000000000/production/product.png",
			ModelRole: "reference_image",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if renderer.input.TemplateKey != "static_fallback_ken_burns_v1" || len(renderer.input.Assets) != 1 {
		t.Fatalf("renderer input = %#v", renderer.input)
	}
	if result.AssetMIME != "video/mp4" || result.AssetStorageURL == "" {
		t.Fatalf("result = %#v", result)
	}
	if result.ProviderRequest["template_key"] != "static_fallback_ken_burns_v1" ||
		result.ProviderResponse["sandbox_job_id"] == "" ||
		result.AssetMetadata["rendering_family"] != "template_video" {
		t.Fatalf("metadata = %#v %#v %#v", result.ProviderRequest, result.ProviderResponse, result.AssetMetadata)
	}
}
```

Add failure tests for unknown template key and missing renderer.

- [ ] **Step 2: Run tests and verify RED**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestTemplateVideoProvider'
```

Expected: provider undefined.

- [ ] **Step 3: Implement provider**

Implement:

- `TemplateVideoProvider`
- `SandboxTemplateVideoRenderer` interface
- `NewTemplateVideoProvider(renderer any) TemplateVideoProvider`
- operations allowed: `template_to_video`, `image_to_template_video`
- image refs converted to sandbox assets
- provider request includes `provider`, `model_id`, `operation_type`, `template_key`, `asset_count`
- provider response includes `sandbox_job_id`, `mime`, `size_bytes`, `template_key`, `template_engine`
- asset metadata includes `provider`, `operation_type`, `rendering_family`, `template_engine`, `template_key`, `duration_sec`, `width`, `height`, `fps`

- [ ] **Step 4: Register default provider**

Modify `NewProviderRegistry` to include:

```go
"internal_template_video": NewTemplateVideoProvider(nil),
```

This makes missing sandbox renderer fail as provider config rather than provider unavailable.

- [ ] **Step 5: Run tests and verify GREEN**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestTemplateVideoProvider|TestProviderRegistry'
```

Expected: PASS.

## Task 4: Wire Provider and RenderPlan Submitter

**Files:**
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/agent/tools/render_plan_submitter.go`
- Modify: `apps/server/internal/agent/tools/decide_render_plan_test.go`
- Modify: `apps/server/internal/agent/tools/render_plan_tools_test.go`

- [ ] **Step 1: Write failing submitter test**

Add or extend a test proving `modelForRenderPlan` maps:

```go
plan.ModelPromptProfile = "template_video"
plan.TargetPhase = "shot_video"
```

to:

```go
Provider: "internal_template_video"
ModelID: "hyperframes-html"
```

- [ ] **Step 2: Run test and verify RED**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'Test.*TemplateVideo'
```

Expected: provider/model empty or test missing behavior.

- [ ] **Step 3: Implement submitter mapping**

Modify `modelForRenderPlan`:

```go
if plan.ModelPromptProfile == "template_video" {
	if provider == "" {
		provider = "internal_template_video"
	}
	if modelID == "" {
		modelID = "hyperframes-html"
	}
}
```

- [ ] **Step 4: Wire server registry**

In `apps/server/cmd/server/main.go`, after sandbox job service creation:

```go
providerRegistry.Register("internal_template_video", production.NewTemplateVideoProvider(sandboxJobService))
```

- [ ] **Step 5: Run tests and verify GREEN**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./cmd/server
```

Expected: PASS.

## Task 5: Smoke Script

**Files:**
- Create: `scripts/smoke-m10-2-template-video-provider.sh`

- [ ] **Step 1: Create smoke**

The smoke should:

- require `CLIPANVIL_SERVER_PORT`
- create or reuse a workspace through API if existing smoke helpers support it
- upload or create a simple image asset if existing API supports it; otherwise use a minimal pre-existing generated preview asset path from test setup
- create a `template_video` RenderPlan or direct worker generation input through existing internal smoke route
- wait until `generation_job.provider=internal_template_video` succeeds
- download signed artifact URL
- run `ffprobe` and assert video stream exists

- [ ] **Step 2: Syntax check**

```bash
bash -n scripts/smoke-m10-2-template-video-provider.sh
```

Expected: no output.

- [ ] **Step 3: Local smoke**

After server and middleware are running:

```bash
CLIPANVIL_SERVER_PORT=<current-port> ./scripts/smoke-m10-2-template-video-provider.sh
```

Expected:

- prints workspace id, generation job id, artifact version id
- prints `provider=internal_template_video`
- prints `winner=true`
- `ffprobe` reports `codec_type=video`

## Task 6: Verification and Report

**Files:**
- Modify: `docs/milestones/m10-hyperframes-template-video-provider.md`
- Create: `docs/superpowers/reports/2026-07-01-m10-2-template-video-provider-vertical-slice.md`

- [ ] **Step 1: Run required checks**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/templatevideo ./internal/sandbox ./internal/production ./internal/agent/tools ./cmd/server
GOCACHE=/private/tmp/clipanvil-go-build make server-test
bash -n scripts/smoke-m10-2-template-video-provider.sh
git diff --check
```

- [ ] **Step 2: Run smoke**

```bash
CLIPANVIL_SERVER_PORT=<current-port> ./scripts/smoke-m10-2-template-video-provider.sh
```

- [ ] **Step 3: Record report**

Report must include:

- exact commands and pass/fail results
- sandbox job id
- generation job id
- artifact version id
- `generation_job.provider=internal_template_video`
- `artifact_version.winner=true`
- ffprobe codec/width/height/fps
- known limitations and M10.3 handoff notes

- [ ] **Step 4: Update milestone**

Set M10 status to `M10.2 已通过，M10.3 待实施` only after all checks and smoke pass.

## Acceptance Checklist

- [ ] `TemplateVideoProvider` exists and is registered.
- [ ] `RenderTemplateVideo` exists and runs fixed HyperFrames command in sandbox.
- [ ] `static_fallback_ken_burns_v1` is rendered from controlled template code.
- [ ] Worker-submitted `template_video` RenderPlan maps to `internal_template_video/hyperframes-html`.
- [ ] production persists template video as normal video asset and artifact winner.
- [ ] provider request/response record template key and sandbox job id.
- [ ] local smoke proves MP4 output with ffprobe.
- [ ] server tests and `git diff --check` pass.
