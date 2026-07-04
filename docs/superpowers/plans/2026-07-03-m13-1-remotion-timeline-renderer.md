# M13.1 Remotion Timeline Renderer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `remotion_timeline_v1` as a Composer template that can render a fixture final video through a sandbox Remotion timeline renderer while preserving existing ffmpeg templates.

**Architecture:** Keep Composer's existing tool flow: `create_timeline_plan -> render_timeline_template -> submit_composition_artifact`. Add a second renderer path behind `render_timeline_template`: ffmpeg templates continue using `RunFFmpegCommand`, while `remotion_timeline_v1` validates `RemotionTimelinePlan` and calls a new sandbox `RenderRemotionTimeline` method. Add a separate `sandbox-image/remotion-timeline` project so final timeline rendering does not mix with the existing single-shot `remotion-motion-shot` provider.

**Tech Stack:** Go 1.26, Hertz/sqlc store types, existing sandbox job service, Remotion 4.0.484, React 19, TypeScript, ffprobe-based verification.

---

## Files

- Create: `apps/server/internal/remotiontimeline/plan.go`
  - Owns the Go-side `RemotionTimelinePlan` schema and validation.
- Create: `apps/server/internal/remotiontimeline/plan_test.go`
  - Unit tests for valid and invalid timeline plans.
- Create: `apps/server/internal/sandbox/remotion_timeline.go`
  - Adds `RenderRemotionTimeline` to `JobService`.
- Modify: `apps/server/internal/sandbox/composition.go`
  - Adds input/result types for Remotion timeline rendering only if shared composition types are the best local home.
- Modify: `apps/server/internal/sandbox/job_service_test.go`
  - Adds sandbox command/upload test for `RenderRemotionTimeline`.
- Modify: `apps/server/internal/agent/tools/composer_native.go`
  - Adds `remotion_timeline_v1` template key, validator branch, and renderer dispatch.
- Modify: `apps/server/internal/agent/tools/composer_tools_test.go`
  - Adds tool tests for template enum, invalid Remotion plan rejection, and renderer branch selection.
- Create: `sandbox-image/remotion-timeline/package.json`
  - Remotion timeline project dependencies.
- Create: `sandbox-image/remotion-timeline/src/schema.ts`
  - Runtime validation helpers for renderer-side JSON.
- Create: `sandbox-image/remotion-timeline/src/index.tsx`
  - Registers `MarketingTimeline`.
- Create: `sandbox-image/remotion-timeline/src/render.mjs`
  - Node renderer wrapper using `@remotion/renderer`.
- Create: `scripts/smoke-m13-1-remotion-timeline.sh`
  - Fixture smoke command for local/sandbox-adjacent validation.

## Task 1: Go Schema And Validation

**Files:**
- Create: `apps/server/internal/remotiontimeline/plan.go`
- Create: `apps/server/internal/remotiontimeline/plan_test.go`

- [ ] **Step 1: Write failing validation tests**

Create `apps/server/internal/remotiontimeline/plan_test.go` with tests covering:

```go
package remotiontimeline

import "testing"

func TestValidateAcceptsFixturePlan(t *testing.T) {
	plan := Plan{
		Schema:      SchemaV1,
		Composition: CompositionMarketingTimeline,
		Output: Output{Width: 1080, Height: 1920, FPS: 30, DurationSec: 10, Codec: "h264", AudioCodec: "aac"},
		Segments: []Segment{{
			ID: "seg-1", ShotRef: "shot_01", StartSec: 0, EndSec: 10, Layout: "hero_packshot",
			Assets: []Asset{{Role: "primary", Type: "image", WorkspacePath: "/workspace/input/product.png"}},
			Caption: Caption{Source: "audio_cue", Text: "轻松出发", StartSec: 0, EndSec: 10, Position: "subtitle_bottom"},
		}},
		AudioTracks: []AudioTrack{{ID: "voiceover", Role: "voiceover", WorkspacePath: "/workspace/input/voiceover.mp3", Volume: 1}},
	}
	if err := Validate(plan); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsBadWorkspacePath(t *testing.T) {
	plan := minimalValidPlan()
	plan.Segments[0].Assets[0].WorkspacePath = "/tmp/product.png"
	if err := Validate(plan); err == nil {
		t.Fatalf("expected bad workspace path to be rejected")
	}
}

func TestValidateRejectsInvalidTiming(t *testing.T) {
	plan := minimalValidPlan()
	plan.Segments[0].EndSec = plan.Segments[0].StartSec
	if err := Validate(plan); err == nil {
		t.Fatalf("expected invalid timing to be rejected")
	}
}

func TestValidateRejectsMissingSegments(t *testing.T) {
	plan := minimalValidPlan()
	plan.Segments = nil
	if err := Validate(plan); err == nil {
		t.Fatalf("expected missing segments to be rejected")
	}
}
```

- [ ] **Step 2: Run tests and confirm failure**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/remotiontimeline
```

Expected: FAIL because the package/types do not exist yet.

- [ ] **Step 3: Implement schema and validation**

Create `apps/server/internal/remotiontimeline/plan.go` defining:

```go
package remotiontimeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

const (
	SchemaV1                     = "clipanvil.remotion_timeline.v1"
	CompositionMarketingTimeline = "MarketingTimeline"
	TemplateKeyV1                = "remotion_timeline_v1"
)

type Plan struct {
	Schema      string       `json:"schema"`
	Composition string       `json:"composition"`
	Output      Output       `json:"output"`
	Theme       Theme        `json:"theme,omitempty"`
	Segments    []Segment    `json:"segments"`
	AudioTracks []AudioTrack `json:"audio_tracks,omitempty"`
	Captions    Captions     `json:"captions,omitempty"`
}

type Output struct {
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FPS         int     `json:"fps"`
	DurationSec float64 `json:"duration_sec"`
	Codec       string  `json:"codec,omitempty"`
	AudioCodec  string  `json:"audio_codec,omitempty"`
}

type Theme struct {
	BrandColors []string `json:"brand_colors,omitempty"`
	FontFamily  string   `json:"font_family,omitempty"`
	Style       string   `json:"style,omitempty"`
}

type Segment struct {
	ID            string       `json:"id"`
	ShotRef       string       `json:"shot_ref,omitempty"`
	StartSec      float64      `json:"start_sec"`
	EndSec        float64      `json:"end_sec"`
	Layout        string       `json:"layout"`
	VisualFocus   string       `json:"visual_focus,omitempty"`
	Assets        []Asset      `json:"assets"`
	Motion        Motion       `json:"motion,omitempty"`
	TextLayers    []TextLayer  `json:"text_layers,omitempty"`
	Caption       Caption     `json:"caption,omitempty"`
	TransitionIn  Transition  `json:"transition_in,omitempty"`
	TransitionOut Transition  `json:"transition_out,omitempty"`
}

type Asset struct {
	Role          string `json:"role"`
	NodeRef       string `json:"node_ref,omitempty"`
	WorkspacePath string `json:"workspace_path"`
	Type          string `json:"type"`
}

type Motion struct {
	Preset    string `json:"preset,omitempty"`
	Intensity string `json:"intensity,omitempty"`
	Direction string `json:"direction,omitempty"`
}

type TextLayer struct {
	Role      string  `json:"role,omitempty"`
	Text      string  `json:"text"`
	StartSec  float64 `json:"start_sec"`
	EndSec    float64 `json:"end_sec"`
	Position  string  `json:"position,omitempty"`
	Animation string  `json:"animation,omitempty"`
}

type Caption struct {
	Source   string  `json:"source,omitempty"`
	Text     string  `json:"text,omitempty"`
	StartSec float64 `json:"start_sec,omitempty"`
	EndSec   float64 `json:"end_sec,omitempty"`
	Position string  `json:"position,omitempty"`
}

type Transition struct {
	Type        string  `json:"type,omitempty"`
	DurationSec float64 `json:"duration_sec,omitempty"`
}

type AudioTrack struct {
	ID            string  `json:"id,omitempty"`
	Role          string  `json:"role"`
	NodeRef       string  `json:"node_ref,omitempty"`
	WorkspacePath string  `json:"workspace_path"`
	StartSec      float64 `json:"start_sec,omitempty"`
	Volume        float64 `json:"volume,omitempty"`
	FadeInSec     float64 `json:"fade_in_sec,omitempty"`
	FadeOutSec    float64 `json:"fade_out_sec,omitempty"`
	Loop          bool    `json:"loop,omitempty"`
}

type Captions struct {
	Source          string `json:"source,omitempty"`
	SingleLane      bool   `json:"single_lane,omitempty"`
	MaxCharsPerLine int    `json:"max_chars_per_line,omitempty"`
	Style           string `json:"style,omitempty"`
}

func Decode(raw map[string]any) (Plan, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return Plan{}, err
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func Validate(plan Plan) error {
	if strings.TrimSpace(plan.Schema) != SchemaV1 {
		return fmt.Errorf("schema must be %s", SchemaV1)
	}
	if strings.TrimSpace(plan.Composition) != CompositionMarketingTimeline {
		return fmt.Errorf("composition must be %s", CompositionMarketingTimeline)
	}
	if plan.Output.Width <= 0 || plan.Output.Height <= 0 || plan.Output.FPS <= 0 || plan.Output.DurationSec <= 0 {
		return errors.New("output width, height, fps and duration_sec are required")
	}
	if len(plan.Segments) == 0 {
		return errors.New("segments must contain at least one item")
	}
	for i, segment := range plan.Segments {
		if strings.TrimSpace(segment.ID) == "" {
			return fmt.Errorf("segments[%d].id is required", i)
		}
		if segment.StartSec < 0 || segment.EndSec <= segment.StartSec || segment.EndSec > plan.Output.DurationSec+0.001 {
			return fmt.Errorf("segments[%d] has invalid timing", i)
		}
		if strings.TrimSpace(segment.Layout) == "" {
			return fmt.Errorf("segments[%d].layout is required", i)
		}
		if len(segment.Assets) == 0 {
			return fmt.Errorf("segments[%d].assets must contain at least one item", i)
		}
		for j, asset := range segment.Assets {
			if err := validateWorkspacePath(asset.WorkspacePath); err != nil {
				return fmt.Errorf("segments[%d].assets[%d].workspace_path: %w", i, j, err)
			}
		}
	}
	for i, track := range plan.AudioTracks {
		if track.Role != "voiceover" && track.Role != "bgm" {
			return fmt.Errorf("audio_tracks[%d].role must be voiceover or bgm", i)
		}
		if err := validateWorkspacePath(track.WorkspacePath); err != nil {
			return fmt.Errorf("audio_tracks[%d].workspace_path: %w", i, err)
		}
	}
	return nil
}

func validateWorkspacePath(value string) error {
	clean := path.Clean(strings.TrimSpace(value))
	if clean == "." || !strings.HasPrefix(clean, "/workspace/") {
		return fmt.Errorf("%q must be under /workspace", value)
	}
	return nil
}
```

- [ ] **Step 4: Add test helper and pass tests**

Add `minimalValidPlan()` helper to `plan_test.go`, run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/remotiontimeline
```

Expected: PASS.

## Task 2: Composer Template Routing

**Files:**
- Modify: `apps/server/internal/agent/tools/composer_native.go`
- Modify: `apps/server/internal/agent/tools/composer_tools_test.go`

- [ ] **Step 1: Write failing Composer tool tests**

Add tests to `composer_tools_test.go`:

```go
func TestRenderTimelineTemplateRoutesRemotionTimeline(t *testing.T) {
	remotion := &fakeRemotionTimelineRenderer{}
	renderer := NewSandboxTimelineTemplateRenderer(&fakeCompositionSandbox{}).WithRemotionRenderer(remotion)
	tool := NewRenderTimelineTemplateNativeTool(renderer)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1), TaskID: uuidWithByte(2), ScopeType: "final_output", ScopeID: uuidWithByte(3),
	})
	got, err := tool.InvokableRun(ctx, `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"template_key":"remotion_timeline_v1",
		"plan":{
			"schema":"clipanvil.remotion_timeline.v1",
			"composition":"MarketingTimeline",
			"output":{"width":1080,"height":1920,"fps":30,"duration_sec":10,"codec":"h264","audio_codec":"aac"},
			"segments":[{"id":"seg-1","shot_ref":"shot_01","start_sec":0,"end_sec":10,"layout":"hero_packshot","assets":[{"role":"primary","type":"image","workspace_path":"/workspace/input/product.png"}],"caption":{"source":"audio_cue","text":"轻松出发","start_sec":0,"end_sec":10}}],
			"audio_tracks":[{"id":"voiceover","role":"voiceover","workspace_path":"/workspace/input/voiceover.mp3","volume":1}]
		}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "工具调用失败") {
		t.Fatalf("render_timeline_template failed: %s", got)
	}
	if remotion.input.TemplateKey != "remotion_timeline_v1" {
		t.Fatalf("remotion input = %#v", remotion.input)
	}
	if strings.Contains(strings.Join(renderer.ffmpegSandbox.ffmpegInput.Args, " "), "ffmpeg") {
		t.Fatalf("remotion route should not call ffmpeg path")
	}
}

func TestRenderTimelineTemplateRejectsInvalidRemotionTimeline(t *testing.T) {
	tool := NewRenderTimelineTemplateNativeTool(NewSandboxTimelineTemplateRenderer(&fakeCompositionSandbox{}))
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1), TaskID: uuidWithByte(2), ScopeType: "final_output", ScopeID: uuidWithByte(3),
	})
	got, err := tool.InvokableRun(ctx, `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"template_key":"remotion_timeline_v1",
		"plan":{"segments":[]}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "工具调用失败") {
		t.Fatalf("expected invalid plan failure, got %s", got)
	}
}
```

- [ ] **Step 2: Run tests and confirm failure**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'RenderTimelineTemplate|ComposerNativeTools' -count=1
```

Expected: FAIL because the template enum, renderer branch, and fake renderer do not exist.

- [ ] **Step 3: Implement template key and renderer branch**

Modify `composer_native.go`:

- Add constant `composerTemplateRemotionTimeline = "remotion_timeline_v1"`.
- Add `RemotionTimelineSandbox` interface:

```go
type RemotionTimelineSandbox interface {
	RenderRemotionTimeline(ctx context.Context, input sandbox.RenderRemotionTimelineInput) (sandbox.SandboxJobResult, error)
}
```

- Extend `SandboxTimelineTemplateRenderer`:

```go
type SandboxTimelineTemplateRenderer struct {
	sandbox  CompositionSandbox
	remotion RemotionTimelineSandbox
}

func (r SandboxTimelineTemplateRenderer) WithRemotionRenderer(renderer RemotionTimelineSandbox) SandboxTimelineTemplateRenderer {
	r.remotion = renderer
	return r
}
```

- In `RenderTimelineTemplate`, branch:

```go
if input.TemplateKey == composerTemplateRemotionTimeline {
	return r.renderRemotionTimeline(ctx, runtime, input)
}
```

- Implement `renderRemotionTimeline`:
  - Decode plan using `remotiontimeline.Decode(input.Plan)`.
  - Validate with `remotiontimeline.Validate(plan)`.
  - Call `r.remotion.RenderRemotionTimeline`.
  - Return output path `/workspace/output/final-<timeline>.mp4`, sandbox job id, and summary.

- Update jsonschema enum descriptions for `DispatchComposerInput`, `CreateTimelinePlanInput`, and `RenderTimelineTemplateInput`.
- Update `validateDispatchComposerInput`, `validateCreateTimelinePlan`, and `validateRenderTimelineTemplate` to allow `remotion_timeline_v1`.

- [ ] **Step 4: Add fake renderer and pass Composer tests**

Update `composer_tools_test.go` fake types with:

```go
type fakeRemotionTimelineRenderer struct {
	input sandbox.RenderRemotionTimelineInput
}

func (f *fakeRemotionTimelineRenderer) RenderRemotionTimeline(_ context.Context, input sandbox.RenderRemotionTimelineInput) (sandbox.SandboxJobResult, error) {
	f.input = input
	return sandbox.SandboxJobResult{Job: db.SandboxJob{ID: uuidWithByte(6)}, MIME: "video/mp4", Size: 123}, nil
}
```

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'RenderTimelineTemplate|ComposerNativeTools' -count=1
```

Expected: PASS.

## Task 3: Sandbox Remotion Timeline Job

**Files:**
- Create: `apps/server/internal/sandbox/remotion_timeline.go`
- Modify: `apps/server/internal/sandbox/job_service_test.go`

- [ ] **Step 1: Write failing sandbox service test**

Add a test near `TestJobServiceRenderMotionShotRunsRemotionAndUploadsMP4`:

```go
func TestJobServiceRenderRemotionTimelineRunsRendererAndKeepsOutput(t *testing.T) {
	repo := newFakeSandboxJobRepository()
	client := &jobServiceFakeClient{
		result: ExecResult{ExitCode: 0, Stdout: "timeline rendered", DurationMS: 120},
		inspect: FileInfo{Path: "/workspace/output/final-timeline.mp4", SizeBytes: 789, Mime: "video/mp4"},
	}
	manager := NewManager(client, testSandboxConfig(), newFakeBindingStore(Binding{
		Status: StatusRunning, SandboxID: "sandbox-1", VolumeName: "sandbox-ws-aabbccdd-0000-0000-0000-000000000000",
	}))
	storage := &fakeSandboxJobStorage{}
	service := NewJobService(manager, client, repo, storage)

	result, err := service.RenderRemotionTimeline(context.Background(), RenderRemotionTimelineInput{
		WorkspaceID:  testWorkspaceID(),
		TargetNodeID: testNodeID(),
		TimelinePlanID: pgtype.UUID{Bytes: [16]byte{0x40}, Valid: true},
		Plan: remotiontimeline.Plan{
			Schema: remotiontimeline.SchemaV1, Composition: remotiontimeline.CompositionMarketingTimeline,
			Output: remotiontimeline.Output{Width: 1080, Height: 1920, FPS: 30, DurationSec: 10, Codec: "h264", AudioCodec: "aac"},
			Segments: []remotiontimeline.Segment{{ID: "seg-1", StartSec: 0, EndSec: 10, Layout: "hero_packshot", Assets: []remotiontimeline.Asset{{Role: "primary", Type: "image", WorkspacePath: "/workspace/input/product.png"}}}},
		},
		OutputPath: "/workspace/output/final-timeline.mp4",
	})
	if err != nil {
		t.Fatalf("RenderRemotionTimeline error = %v", err)
	}
	if result.Job.Status != db.JobStatusSucceeded || result.Job.OperationType != "render_remotion_timeline" || result.MIME != "video/mp4" {
		t.Fatalf("result = %#v", result)
	}
	joined := strings.Join(client.commands, "\n")
	for _, want := range []string{"timeline-plan.json", "remotion-timeline", "render.mjs", "/workspace/output/final-timeline.mp4"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected command containing %q, got %q", want, joined)
		}
	}
}
```

- [ ] **Step 2: Run test and confirm failure**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run RenderRemotionTimeline -count=1
```

Expected: FAIL because `RenderRemotionTimeline` does not exist.

- [ ] **Step 3: Implement `RenderRemotionTimeline`**

Create `apps/server/internal/sandbox/remotion_timeline.go`:

- Define `RenderRemotionTimelineInput`.
- Validate service dependencies.
- Validate output path with existing output path rules.
- Create sandbox job with:
  - `JobType: "internal_media"`
  - `OperationType: "render_remotion_timeline"`
  - input JSON containing timeline id, duration, segment count, audio count.
- Ensure sandbox and workspace layout.
- Upload `timeline-plan.json` into `/workspace/remotion-timeline/<job_id>/timeline-plan.json`.
- Execute command:

```text
node /opt/clipanvil/remotion-timeline/src/render.mjs --props /workspace/remotion-timeline/<job_id>/timeline-plan.json --out /workspace/output/final-<timeline>.mp4 --browser-executable /usr/bin/chromium-headless-shell
```

- Inspect output, validate size, require video MIME.
- Mark sandbox job succeeded with `output_path`, MIME, size, and `renderer_engine`.
- Leave object storage upload to the existing `submit_composition_artifact` step after `render_timeline_template`.

- [ ] **Step 4: Pass sandbox service test**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'RenderRemotionTimeline|RenderMotionShot|RunFFmpegCommand' -count=1
```

Expected: PASS.

## Task 4: Remotion Timeline Project

**Files:**
- Create: `sandbox-image/remotion-timeline/package.json`
- Create: `sandbox-image/remotion-timeline/src/schema.ts`
- Create: `sandbox-image/remotion-timeline/src/index.tsx`
- Create: `sandbox-image/remotion-timeline/src/render.mjs`

- [ ] **Step 1: Create package manifest**

Create `sandbox-image/remotion-timeline/package.json` mirroring Remotion versions already used by motion shot:

```json
{
  "name": "@clip-anvil/remotion-timeline",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "dependencies": {
    "@remotion/cli": "4.0.484",
    "@remotion/renderer": "4.0.484",
    "remotion": "4.0.484",
    "typescript": "6.0.0-dev.20251201",
    "react": "19.2.3",
    "react-dom": "19.2.3"
  },
  "devDependencies": {}
}
```

- [ ] **Step 2: Implement renderer-side schema guard**

Create `src/schema.ts` exporting `assertTimelinePlan(value: unknown)` with checks for schema, composition, output, segments and workspace paths. Keep it dependency-free.

- [ ] **Step 3: Implement `MarketingTimeline`**

Create `src/index.tsx`:

- Register composition id `MarketingTimeline`.
- Use `calculateMetadata` to derive duration, fps, width and height from props.
- Render each segment with `<Sequence>`.
- For P0, implement one generic layout:
  - gradient background from theme colors.
  - primary image via `Img`.
  - optional caption at bottom.
  - optional text layers.
  - voiceover/BGM via `<Html5Audio>`.

- [ ] **Step 4: Implement `render.mjs`**

Create `src/render.mjs`:

- Parse `--props`, `--out`, optional `--browser-executable`.
- Read JSON.
- Validate with `assertTimelinePlan`.
- Use `bundle`, `selectComposition`, and `renderMedia` from Remotion packages.
- Render h264 MP4 with AAC audio.

- [ ] **Step 5: Local syntax check**

Run:

```bash
node --check sandbox-image/remotion-timeline/src/render.mjs
```

Expected: PASS.

## Task 5: Fixture Smoke

**Files:**
- Create: `scripts/smoke-m13-1-remotion-timeline.sh`

- [ ] **Step 1: Add smoke script**

Create a script that:

- Finds Node and ffmpeg/ffprobe.
- Creates a temp workdir.
- Generates fixture PNG with ffmpeg color source.
- Generates short voiceover-like sine audio and BGM-like sine audio.
- Writes a fixture `timeline-plan.json`.
- Runs the Remotion timeline renderer locally if dependencies are installed.
- Uses `ffprobe` to verify:
  - video stream exists.
  - audio stream exists.
  - width/height are `1080x1920`.
  - duration is near 10 seconds.

- [ ] **Step 2: Make script executable**

Run:

```bash
chmod +x scripts/smoke-m13-1-remotion-timeline.sh
```

- [ ] **Step 3: Run smoke**

Run:

```bash
./scripts/smoke-m13-1-remotion-timeline.sh
```

Expected: PASS. If local Remotion deps are not installed, the script must print a clear skip reason and exit non-zero only when a dependency that should be present in CI/dev image is missing.

## Task 6: Verification

**Files:**
- All files changed above.

- [ ] **Step 1: Run targeted Go tests**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/remotiontimeline ./internal/agent/tools ./internal/sandbox
```

Expected: PASS.

- [ ] **Step 2: Run server build**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: PASS.

- [ ] **Step 3: Run smoke script**

Run:

```bash
./scripts/smoke-m13-1-remotion-timeline.sh
```

Expected: PASS or documented skip with reason if Remotion dependencies cannot be installed in the current environment.

- [ ] **Step 4: Run diff check**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 5: Update milestone status**

If all checks pass, update `docs/milestones/m13-remotion-timeline-composer.md` M13.1 status line to note implementation completion and verification commands. Do not mark M13.2 started until M13.1 evidence is recorded.
