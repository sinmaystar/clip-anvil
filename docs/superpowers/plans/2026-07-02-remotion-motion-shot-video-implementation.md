# Remotion Motion Shot Video Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the HyperFrames/template video route with a Remotion-backed motion shot route that can generate low-cost image-driven silent shot videos for final Composer assembly.

**Architecture:** The implementation keeps the existing Agent -> RenderPlan -> Worker -> production service -> provider -> sandbox -> artifact winner architecture. HyperFrames-specific routes are removed, `motion_shot_video` becomes the low-cost `shot_video` profile, `internal_motion_video/remotion-motion-shot-v1` renders silent shot videos, and Composer remains responsible for voiceover, subtitles, BGM, and final audio/video synchronization.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, OpenSandbox, MinIO, FFmpeg/ffprobe, Node.js, Remotion 4.x, React, TypeScript, pnpm, existing Agent native tools and skill runtime.

---

## Phase Gates

Each phase must pass its acceptance commands before the next phase starts. Do not carry failing tests forward as "known issues".

| Phase | Deliverable | Acceptance |
|---|---|---|
| M11.0 | Milestone and implementation plan | `git diff --check`; no placeholders in plan/spec |
| M11.1 | HyperFrames cleanup and motion route foundation | focused Go tests for renderplan/tools/production pass; `rg` shows no active HyperFrames route in production/provider/renderplan layers |
| M11.2 | Remotion provider and sandbox vertical slice | provider unit tests plus real sandbox smoke MP4 |
| M11.3 | Agent skills and routing | Agent fixture tests show motion skills and no Seedance/template video route |
| M11.4 | Composer audio/caption sync | Composer tests and `ffprobe` final timing checks pass |
| M11.5 | Browser E2E and audit | browser-generated real ad, DB/log audit, media artifact verification |

## File Map

### Create

- `docs/milestones/m11-remotion-motion-shot-video.md`: human-readable phase milestone and acceptance gates.
- `docs/superpowers/plans/2026-07-02-remotion-motion-shot-video-implementation.md`: this implementation plan.
- `apps/server/migrations/039_m11_remotion_motion_video_capability.sql`: register motion capability for fresh checkouts and delete stale template capability rows when present.
- `apps/server/internal/production/motion_shot_provider.go`: provider adapter for `internal_motion_video`.
- `apps/server/internal/production/motion_shot_provider_test.go`: provider validation and sandbox handoff tests.
- `apps/server/internal/motionshot/plan.go`: typed motion plan normalization and validation.
- `apps/server/internal/motionshot/plan_test.go`: motion plan validation tests.
- `apps/server/internal/sandbox/motion_shot.go`: sandbox Remotion render orchestration.
- `apps/server/internal/sandbox/motion_shot_test.go`: command/project generation tests.
- `sandbox-image/remotion-motion-shot/`: controlled Remotion renderer package copied into the sandbox image.
- `scripts/smoke-m11-2-remotion-motion-shot-provider.sh`: API-level provider smoke.
- `apps/server/internal/agent/skills/library/motion-shot-producer/SKILL.md`: Producer motion shot route skill.
- `apps/server/internal/agent/skills/library/motion-shot-craftsman/SKILL.md`: Craftsman motion shot RenderPlan skill.
- `apps/server/internal/agent/skills/library/motion-shot-reviewer/SKILL.md`: Reviewer motion shot quality skill.
- `scripts/smoke-m11-5-agent-remotion-motion-e2e.sh`: browser/API-assisted final E2E audit helper.

### Modify

- `sandbox-image/Dockerfile`: remove HyperFrames install; add Remotion renderer dependencies.
- `apps/server/cmd/server/main.go`: register `internal_motion_video`; remove `internal_template_video`.
- `apps/server/cmd/server/e2e_producer_fixture.go`: replace HyperFrames fixture language with Remotion motion shot.
- `apps/server/cmd/server/main_test.go`: update E2E fixture tests.
- `apps/server/internal/agent/renderplan/types.go`: replace `ProfileTemplateVideo` with `ProfileMotionShotVideo`.
- `apps/server/internal/agent/renderplan/profiles.go`: map motion profile to internal motion provider.
- `apps/server/internal/agent/renderplan/service.go`: allow motion profile only for `shot_video`.
- `apps/server/internal/agent/renderplan/*_test.go`: update profile, service, and compiler expectations.
- `apps/server/internal/agent/tools/upsert_render_plan.go`: schema, defaults, validation, params, operation list.
- `apps/server/internal/agent/tools/dispatch_craftsman.go`: no-Seedance route defaults and keywords.
- `apps/server/internal/agent/tools/*_test.go`: update route expectations.
- `apps/server/internal/agent/producer/system_prompt.go`: replace template route guidance with motion shot guidance.
- `apps/server/internal/agent/craftsman/system_prompt.go`: replace template-only rules with motion-only rules.
- `apps/server/internal/agent/craftsman/context_loader.go`: update route policy context.
- `apps/server/internal/agent/reviewer/context_loader.go`: read motion provider metadata.
- `apps/server/internal/agent/reviewer/system_prompt.go`: update review criteria.
- `apps/server/internal/agent/skills/registry tests`: remove template-video skills and add motion-shot skills.
- `apps/server/internal/production/provider.go`: register motion provider in default registry.
- `apps/server/internal/production/service.go` and tests: restore motion provider/model/operation/params from media nodes.
- `apps/server/internal/production/input_hash.go` and tests: include motion plan fields in input hash.
- `apps/server/internal/agent/composer/*`: ensure final captions/audio are timeline-level and not baked into motion shot.
- `docs/README.md` and relevant milestones: point current low-cost video direction to M11; archive or mark M10 superseded.

### Delete

- `apps/server/internal/production/template_video_provider.go`
- `apps/server/internal/templatevideo/`
- `apps/server/internal/sandbox/template_video.go`
- `apps/server/internal/agent/skills/library/template-video-producer/SKILL.md`
- `apps/server/internal/agent/skills/library/template-video-craftsman/SKILL.md`
- `apps/server/internal/agent/skills/library/template-video-reviewer/SKILL.md`
- `scripts/smoke-m10-0-hyperframes-sandbox.sh`
- `scripts/smoke-m10-2-template-video-provider.sh`

## Task M11.0: Plan And Gate Documents

**Files:**
- Create: `docs/milestones/m11-remotion-motion-shot-video.md`
- Create: `docs/superpowers/plans/2026-07-02-remotion-motion-shot-video-implementation.md`
- Verify: `docs/superpowers/specs/2026-07-02-remotion-motion-shot-video-design.md`

- [ ] **Step 1: Write the milestone file**

Create `docs/milestones/m11-remotion-motion-shot-video.md` with:

```markdown
# M11 Remotion Motion Shot Video 与 HyperFrames 清理 — 里程碑

**状态**：进行中
**日期**：2026-07-02
**目标**：按 Remotion design 文档完成低成本图片驱动营销视频主线。
```

Include a phase table with columns `阶段`, `里程碑`, `可交付标准`, and `可验收标准`.

- [ ] **Step 2: Write this implementation plan**

Create `docs/superpowers/plans/2026-07-02-remotion-motion-shot-video-implementation.md` and include every phase gate, file map, task list, command, and expected outcome shown in this plan.

- [ ] **Step 3: Verify the plan has no placeholders**

Run:

```bash
rg -n "T[B]D|T[O]DO|待[定]|PLACEH[O]LDER|implement lat[e]r|fill in detai[l]s" docs/milestones/m11-remotion-motion-shot-video.md docs/superpowers/plans/2026-07-02-remotion-motion-shot-video-implementation.md
```

Expected: no matches.

- [ ] **Step 4: Verify whitespace**

Run:

```bash
git diff --check -- docs/milestones/m11-remotion-motion-shot-video.md docs/superpowers/plans/2026-07-02-remotion-motion-shot-video-implementation.md
```

Expected: no output and exit code 0.

- [ ] **Step 5: Commit M11.0**

Run:

```bash
git add docs/milestones/m11-remotion-motion-shot-video.md docs/superpowers/plans/2026-07-02-remotion-motion-shot-video-implementation.md
git commit -m "docs: plan remotion motion shot implementation"
```

Expected: commit succeeds. Do not stage local E2E screenshots.

## Task M11.1: HyperFrames Cleanup And Motion Route Foundation

**Files:**
- Create: `apps/server/migrations/039_m11_remotion_motion_video_capability.sql`
- Modify: `apps/server/internal/agent/renderplan/types.go`
- Modify: `apps/server/internal/agent/renderplan/profiles.go`
- Modify: `apps/server/internal/agent/renderplan/service.go`
- Modify: `apps/server/internal/agent/renderplan/profiles_test.go`
- Modify: `apps/server/internal/agent/renderplan/service_test.go`
- Modify: `apps/server/internal/agent/renderplan/prompt_compiler_test.go`
- Modify: `apps/server/internal/agent/tools/upsert_render_plan.go`
- Modify: `apps/server/internal/agent/tools/render_plan_tools_test.go`
- Delete: `apps/server/internal/production/template_video_provider.go`
- Delete: `apps/server/internal/templatevideo/`
- Delete: `apps/server/internal/sandbox/template_video.go`

- [ ] **Step 1: Write failing renderplan profile tests**

Update `apps/server/internal/agent/renderplan/profiles_test.go` to assert:

```go
motionProfile, ok := ProfileByID(ProfileMotionShotVideo)
if !ok {
	t.Fatalf("motion shot profile missing")
}
if motionProfile.DefaultProvider != "internal_motion_video" ||
	motionProfile.DefaultModelID != "remotion-motion-shot-v1" ||
	motionProfile.OutputType != "video" ||
	!motionProfile.AllowedOperations["image_to_motion_video"] {
	t.Fatalf("unexpected motion profile: %+v", motionProfile)
}
if _, ok := ProfileByID("template_video"); ok {
	t.Fatalf("template_video profile should not remain active")
}
```

- [ ] **Step 2: Run profile tests and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/renderplan -run 'TestProfileByID' -count=1
```

Expected: FAIL because `ProfileMotionShotVideo` is undefined or profile lookup does not return it.

- [ ] **Step 3: Implement motion profile constants and profile lookup**

In `apps/server/internal/agent/renderplan/types.go`, replace:

```go
ProfileTemplateVideo = "template_video"
```

with:

```go
ProfileMotionShotVideo = "motion_shot_video"
```

In `apps/server/internal/agent/renderplan/profiles.go`, add:

```go
case ProfileMotionShotVideo:
	return ModelPromptProfile{
		ID:              ProfileMotionShotVideo,
		DefaultProvider: "internal_motion_video",
		DefaultModelID:  "remotion-motion-shot-v1",
		OutputType:      "video",
		AllowedOperations: map[string]bool{
			"image_to_motion_video": true,
		},
		DefaultParams:  Params{Ratio: "9:16", DurationSec: 5, Resolution: "1080p", FPS: 30},
		MaxPromptChars: 3000,
	}, true
```

- [ ] **Step 4: Write failing service validation tests**

Update `apps/server/internal/agent/renderplan/service_test.go`:

```go
func TestServiceAcceptsMotionShotVideoRenderPlan(t *testing.T) {
	input := validUpsertInput()
	input.TargetPhase = PhaseShotVideo
	input.ModelPromptProfile = ProfileMotionShotVideo
	input.Operation = "image_to_motion_video"
	plan, err := NewService(fakeStore{}).Upsert(context.Background(), input)
	if err != nil {
		t.Fatalf("upsert motion shot video: %v", err)
	}
	if plan.TargetPhase != PhaseShotVideo || plan.ModelPromptProfile != ProfileMotionShotVideo || plan.Operation != "image_to_motion_video" {
		t.Fatalf("unexpected plan route: %+v", plan)
	}
}

func TestServiceRejectsMotionShotVideoForNonShotPhase(t *testing.T) {
	input := validUpsertInput()
	input.TargetPhase = PhasePreviewImage
	input.ModelPromptProfile = ProfileMotionShotVideo
	input.Operation = "image_to_motion_video"
	_, err := NewService(fakeStore{}).Upsert(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "motion_shot_video 只能用于 shot_video") {
		t.Fatalf("expected motion non-shot rejection, got %v", err)
	}
}
```

- [ ] **Step 5: Run service tests and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/renderplan -run 'TestServiceAcceptsMotionShotVideoRenderPlan|TestServiceRejectsMotionShotVideoForNonShotPhase' -count=1
```

Expected: FAIL because service still allows/rejects using template profile rules.

- [ ] **Step 6: Implement service validation**

In `apps/server/internal/agent/renderplan/service.go`, make `shot_video` allow `ProfileSeedance2Video` or `ProfileMotionShotVideo`. Reject `ProfileMotionShotVideo` outside `shot_video` with the exact message:

```text
motion_shot_video 只能用于 shot_video
```

- [ ] **Step 7: Write failing PromptCompiler test**

Update `apps/server/internal/agent/renderplan/prompt_compiler_test.go` so the motion case expects:

```json
{
  "provider": "internal_motion_video",
  "model": "remotion-motion-shot-v1",
  "profile": "motion_shot_video",
  "operation": "image_to_motion_video"
}
```

- [ ] **Step 8: Implement tool schema updates**

In `apps/server/internal/agent/tools/upsert_render_plan.go`, replace template-specific enums and descriptions:

- `template_video` -> `motion_shot_video`
- `template_to_video` and `image_to_template_video` -> `image_to_motion_video`
- `TemplateKey` field becomes unused by active motion route; leave it only if Composer still needs it elsewhere, otherwise remove from RenderPlan tool input.
- Add motion fields to `Params`: keep `FPS` and `Variables` if reused, and add `MotionStyle`, `SafeArea`, `VisualLayers`, `TextLayers`, `Transitions`, and `BrandColors` if typed params are required by existing JSON flow.

The runtime policy error for no-Seedance must say:

```text
当前任务设置 video_route_policy=motion_only，禁止 Seedance
```

- [ ] **Step 9: Add migration**

Create `apps/server/migrations/039_m11_remotion_motion_video_capability.sql` by replacing the previous unmerged HyperFrames 039 migration. Include Up actions:

```sql
DELETE FROM model_capability
WHERE provider_id = 'internal_template_video'
  AND model_id = 'hyperframes-html';

DELETE FROM model_provider
WHERE id = 'internal_template_video';

INSERT INTO model_provider (id, display_name, provider_type, config, enabled)
VALUES ('internal_motion_video', 'Internal Motion Video', 'internal_media', '{"engine":"remotion"}', true)
ON CONFLICT (id) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    provider_type = EXCLUDED.provider_type,
    config = EXCLUDED.config,
    enabled = EXCLUDED.enabled,
    updated_at = now();

INSERT INTO model_capability (
    provider_id, model_id, display_name, output_types, supported_operations,
    supported_input_node_types, limits, pricing, defaults, enabled
) VALUES (
    'internal_motion_video',
    'remotion-motion-shot-v1',
    'Remotion Motion Shot Video',
    '["video"]',
    '["image_to_motion_video"]',
    '["image", "text"]',
    '{"max_prompt_chars":3000,"max_attempts":1,"async_required":true,"durations_sec":[3,4,5,6,8],"resolutions":["720p","1080p"],"ratios":["9:16","16:9","1:1"],"max_input_images":4}',
    '{"tier":"internal","cost_class":"low","external_api_cost":false}',
    '{"ratio":"9:16","duration_sec":5,"resolution":"1080p","fps":30,"watermark":false}',
    true
)
ON CONFLICT (provider_id, model_id) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    output_types = EXCLUDED.output_types,
    supported_operations = EXCLUDED.supported_operations,
    supported_input_node_types = EXCLUDED.supported_input_node_types,
    limits = EXCLUDED.limits,
    pricing = EXCLUDED.pricing,
    defaults = EXCLUDED.defaults,
    enabled = EXCLUDED.enabled,
    updated_at = now();

ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_profile_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_profile_check
    CHECK (model_prompt_profile IN ('seedream_5_image', 'seedance_2_video', 'seed_audio_1', 'motion_shot_video'));
```

Down action should restore the pre-M11 state with `seedream_5_image`, `seedance_2_video`, and `seed_audio_1`; do not re-register HyperFrames.

- [ ] **Step 10: Delete active HyperFrames implementation files**

Remove:

```bash
rm apps/server/internal/production/template_video_provider.go
rm -rf apps/server/internal/templatevideo
rm apps/server/internal/sandbox/template_video.go
```

Use `apply_patch` for deletions when working manually.

- [ ] **Step 11: Run M11.1 acceptance**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/renderplan ./apps/server/internal/agent/tools ./apps/server/internal/production -run 'Motion|motion|Template|template|RenderPlanTool|ProfileByID|Compile' -count=1
rg -n "internal_template_video|hyperframes-html|template_video|TemplateVideo|template_to_video|image_to_template_video" apps/server/internal/production apps/server/internal/agent/renderplan apps/server/internal/sandbox sandbox-image
git diff --check
```

Expected:

- Go tests pass.
- `rg` has no production/provider/renderplan/sandbox route hits. Migration cleanup SQL may mention old IDs only to delete stale DB rows; Agent prompt, skill, and fixture cleanup happens in M11.3.
- `git diff --check` passes.

- [ ] **Step 12: Commit M11.1**

Run:

```bash
git add apps/server/migrations apps/server/internal apps/server/cmd/server sandbox-image scripts
git commit -m "feat: replace template route with motion shot profile"
```

## Task M11.2: Remotion Provider And Sandbox Vertical Slice

**Files:**
- Create: `apps/server/internal/motionshot/plan.go`
- Create: `apps/server/internal/motionshot/plan_test.go`
- Create: `apps/server/internal/production/motion_shot_provider.go`
- Create: `apps/server/internal/production/motion_shot_provider_test.go`
- Create: `apps/server/internal/sandbox/motion_shot.go`
- Create: `apps/server/internal/sandbox/motion_shot_test.go`
- Create: `sandbox-image/remotion-motion-shot/package.json`
- Create: `sandbox-image/remotion-motion-shot/src/Root.tsx`
- Create: `sandbox-image/remotion-motion-shot/src/render.ts`
- Modify: `sandbox-image/Dockerfile`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/production/provider.go`
- Create: `scripts/smoke-m11-2-remotion-motion-shot-provider.sh`

- [ ] **Step 1: Add motion plan failing tests**

Create `apps/server/internal/motionshot/plan_test.go` with tests:

```go
func TestNormalizeRequiresImageForMotionShot(t *testing.T) {
	_, err := Normalize(RenderInput{DurationSec: 5, Ratio: "9:16", Resolution: "1080p", FPS: 30})
	if err == nil || !strings.Contains(err.Error(), "requires at least one image") {
		t.Fatalf("expected missing image error, got %v", err)
	}
}

func TestNormalizeClampsSupportedMotionMeta(t *testing.T) {
	plan, err := Normalize(RenderInput{
		DurationSec: 5,
		Ratio: "9:16",
		Resolution: "1080p",
		FPS: 30,
		Assets: []Asset{{WorkspacePath: "assets/product.png"}},
		Params: map[string]any{"motion_style": "premium_product_ad"},
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if plan.Width != 1080 || plan.Height != 1920 || plan.DurationFrames != 150 {
		t.Fatalf("unexpected meta: %+v", plan)
	}
}
```

- [ ] **Step 2: Run motion plan tests and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/motionshot -count=1
```

Expected: FAIL because package does not exist.

- [ ] **Step 3: Implement `motionshot.Normalize`**

Create `apps/server/internal/motionshot/plan.go` with typed input structs, supported ratio/resolution/fps/duration validation, safe default visual/text layers, and JSON serializable output. Use only structured params; do not accept raw code.

- [ ] **Step 4: Add provider failing tests**

Create `apps/server/internal/production/motion_shot_provider_test.go` with tests proving:

- non-video output fails.
- unsupported operation fails.
- missing image for `image_to_motion_video` fails.
- valid image intent calls the sandbox renderer and returns metadata with `renderer_engine=remotion`.

- [ ] **Step 5: Run provider tests and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestMotionShotProvider' -count=1
```

Expected: FAIL because `NewMotionShotProvider` does not exist.

- [ ] **Step 6: Implement `MotionShotProvider`**

Create `apps/server/internal/production/motion_shot_provider.go`. The provider must:

- require `OutputType=="video"`;
- require `OperationType=="image_to_motion_video"`;
- require at least one image input ref;
- call `motionshot.Normalize`;
- call `sandbox.RenderMotionShot`;
- set request metadata provider/model/operation/asset_count;
- set response metadata `renderer_engine=remotion`.

- [ ] **Step 7: Add sandbox command tests**

Create `apps/server/internal/sandbox/motion_shot_test.go` with tests asserting the generated render command:

- runs under `/workspace/motion-shot/<jobID>`;
- reads `motion-plan.json`;
- writes `/workspace/output/motion-<jobID>.mp4`;
- uses Remotion renderer package, not HyperFrames.

- [ ] **Step 8: Implement sandbox render orchestration**

Create `apps/server/internal/sandbox/motion_shot.go` following `ComposeVideos` and previous internal media patterns:

- create sandbox job;
- ensure workspace layout;
- download image assets via presigned URLs;
- upload `motion-plan.json` and controlled renderer files if needed;
- run Remotion command;
- inspect output, validate size/MIME;
- upload MP4 to MinIO;
- mark job succeeded or failed with specific error codes.

- [ ] **Step 9: Add controlled Remotion renderer**

Create `sandbox-image/remotion-motion-shot` with a pinned package. Use Remotion official guidance that all `remotion` and `@remotion/*` packages should use the same exact version. The renderer must accept a JSON props file and render one composition with:

- image layer motion;
- short text layers;
- safe area;
- no voiceover or BGM.

- [ ] **Step 10: Update sandbox image**

Modify `sandbox-image/Dockerfile`:

- remove `npm install -g hyperframes@...`;
- install/copy the controlled Remotion renderer;
- keep Node.js, FFmpeg, Chromium, and CJK fonts available.

- [ ] **Step 11: Register provider**

Update:

- `apps/server/cmd/server/main.go` to register `internal_motion_video` with the real sandbox job service.
- `apps/server/internal/production/provider.go` to include `NewMotionShotProvider(nil)` in the default registry.

- [ ] **Step 12: Add real smoke**

Create `scripts/smoke-m11-2-remotion-motion-shot-provider.sh`. It must:

- create or use an authenticated workspace;
- upload a local image;
- create an image node and shot video node;
- call generation with `model_provider=internal_motion_video`, `model_id=remotion-motion-shot-v1`, `operation_type=image_to_motion_video`;
- poll production state;
- download the artifact;
- run `ffprobe` and assert video stream exists.

- [ ] **Step 13: Run M11.2 acceptance**

Run:

```bash
bash -n scripts/smoke-m11-2-remotion-motion-shot-provider.sh
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/motionshot ./apps/server/internal/production ./apps/server/internal/sandbox -run 'Motion|motion' -count=1
./scripts/smoke-m11-2-remotion-motion-shot-provider.sh
git diff --check
```

Expected: tests pass; smoke prints provider `internal_motion_video`, output path, duration/resolution from `ffprobe`, and artifact winner id.

- [ ] **Step 14: Commit M11.2**

Run:

```bash
git add apps/server/internal/motionshot apps/server/internal/production apps/server/internal/sandbox apps/server/cmd/server sandbox-image scripts/smoke-m11-2-remotion-motion-shot-provider.sh
git commit -m "feat: add remotion motion shot provider"
```

## Task M11.3: Agent Skills And Routing

**Files:**
- Create: `apps/server/internal/agent/skills/library/motion-shot-producer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/motion-shot-craftsman/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/motion-shot-reviewer/SKILL.md`
- Delete: `apps/server/internal/agent/skills/library/template-video-producer/SKILL.md`
- Delete: `apps/server/internal/agent/skills/library/template-video-craftsman/SKILL.md`
- Delete: `apps/server/internal/agent/skills/library/template-video-reviewer/SKILL.md`
- Modify: `apps/server/internal/agent/producer/system_prompt.go`
- Modify: `apps/server/internal/agent/craftsman/system_prompt.go`
- Modify: `apps/server/internal/agent/reviewer/system_prompt.go`
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman.go`
- Modify: `apps/server/cmd/server/e2e_producer_fixture.go`
- Modify: Agent skill registry and fixture tests.

- [ ] **Step 1: Write failing route tests**

Update `apps/server/internal/agent/tools/dispatch_craftsman_test.go` so no-Seedance and non-hero shot dispatch expect:

```text
recommended_model_prompt_profile=motion_shot_video
recommended_operation=image_to_motion_video
video_route_policy=motion_only
```

and assert no returned context includes `template_video`, `hyperframes`, or `internal_template_video`.

- [ ] **Step 2: Run route tests and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'DispatchCraftsman|Template|Motion' -count=1
```

Expected: FAIL because current route still recommends template video.

- [ ] **Step 3: Implement dispatch route changes**

In `dispatch_craftsman.go`:

- replace template route recommendation with motion route recommendation;
- map no-Seedance and image-driven ad keywords to `motion_only`;
- preserve Seedance only when Producer explicitly requests real complex motion.

- [ ] **Step 4: Add motion skills**

Create the three skill files with frontmatter:

```yaml
role_scope: [producer]
domain: [commerce_ad, motion_shot, cost_routing]
```

Producer skill must require:

- Seedream allowed for images;
- Volcengine allowed for audio;
- `motion_shot_video` for no-Seedance shot video;
- Composer only after shot and audio assets exist.

Craftsman skill must require:

- `target_phase: shot_video`;
- `model_prompt_profile: motion_shot_video`;
- `operation: image_to_motion_video`;
- no raw React/Remotion code.

Reviewer skill must require checks for product visibility, text safe area, motion rhythm, no-Seedance compliance, and final audio/caption readiness.

- [ ] **Step 5: Remove template skills and update registry tests**

Delete the three template-video skill files. Update registry tests so:

- motion skills are registered for the correct roles;
- template-video skills are absent.

- [ ] **Step 6: Update prompts and E2E fixture**

Replace prompt text in Producer/Craftsman/Reviewer and `e2e_producer_fixture.go`:

- `HyperFrames` -> `Remotion motion shot`;
- `template_video` -> `motion_shot_video`;
- `template_only` -> `motion_only`;
- `image_to_template_video` -> `image_to_motion_video`.

- [ ] **Step 7: Run M11.3 acceptance**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./apps/server/internal/agent/producer ./apps/server/internal/agent/craftsman ./apps/server/internal/agent/reviewer ./apps/server/cmd/server -run 'Skill|Dispatch|Fixture|Motion|Template' -count=1
rg -n "hyperframes|HyperFrames|internal_template_video|hyperframes-html|template_video|image_to_template_video|template_only" apps/server/internal/agent apps/server/cmd/server
git diff --check
```

Expected:

- Tests pass.
- `rg` has no active Agent/cmd hits.

- [ ] **Step 8: Commit M11.3**

Run:

```bash
git add apps/server/internal/agent apps/server/cmd/server
git commit -m "feat: route agent low cost video to motion shots"
```

## Task M11.4: Composer Audio And Caption Synchronization

**Files:**
- Modify: `apps/server/internal/agent/composer/types.go`
- Modify: `apps/server/internal/agent/composer/executor.go`
- Modify: `apps/server/internal/production/internal_ffmpeg_provider.go`
- Modify: `apps/server/internal/sandbox/video_composition.go`
- Modify: composer and production tests.

- [ ] **Step 1: Write failing Composer sync tests**

Add tests proving final composition:

- accepts ordered motion shot video refs;
- accepts voiceover and BGM refs;
- accepts caption segments derived from AudioPlan;
- emits timeline plan where caption segments are final-level overlays;
- does not expect subtitles inside each shot video.

- [ ] **Step 2: Run Composer tests and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer ./apps/server/internal/production ./apps/server/internal/sandbox -run 'Caption|Audio|Compose|Timeline' -count=1
```

Expected: FAIL for missing caption/timing handling if not already implemented.

- [ ] **Step 3: Implement final timeline caption/audio fields**

Keep motion shot outputs silent. Add or reuse Composer plan fields:

```json
{
  "caption_tracks": [
    {"text":"轻松出发", "start_sec":0.0, "end_sec":1.8, "position":"bottom_safe"}
  ],
  "audio_tracks": [
    {"role":"voiceover", "asset_id":"...", "volume":1.0},
    {"role":"bgm", "asset_id":"...", "volume":0.28, "ducking":{"sidechain_role":"voiceover"}}
  ]
}
```

- [ ] **Step 4: Implement timing checks**

Final composition should validate:

- ordered shot durations sum to final target duration, or plan includes explicit trim/pad;
- voiceover duration mismatch above 300ms is recorded in review context;
- captions do not cross unrelated shot boundaries unless explicitly allowed.

- [ ] **Step 5: Run M11.4 acceptance**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer ./apps/server/internal/production ./apps/server/internal/sandbox -run 'Caption|Audio|Compose|Timeline' -count=1
git diff --check
```

Expected: tests pass and composition metadata shows final-level captions/audio tracks.

- [ ] **Step 6: Commit M11.4**

Run:

```bash
git add apps/server/internal/agent/composer apps/server/internal/production apps/server/internal/sandbox
git commit -m "feat: align composer captions and audio for motion shots"
```

## Task M11.5: Browser E2E And DB/Log Audit

**Files:**
- Create: `scripts/smoke-m11-5-agent-remotion-motion-e2e.sh`
- Use browser: current app Browser or Playwright against the Vite URL.
- Use DB/log inspection commands from current dev environment.

- [ ] **Step 1: Start current checkout runtime**

Run:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
./scripts/dev-start.sh
```

Expected: script prints backend/frontend ports and starts current worktree services.

- [ ] **Step 2: Open browser E2E workspace**

Use the in-app browser to open the Vite Agent URL. Create a new Agent workspace. Upload the desktop `box.png` or the user-provided product image.

- [ ] **Step 3: Submit real prompt**

Use this prompt:

```text
用这张商品图生成悦行行李箱口播广告。不要调用 Seedance。可以用 Seedream 生成多张商业广告图片，使用 Remotion motion shot 做分镜动效，使用火山语音生成旁白和 BGM，最后由 Composer 合成带字幕、口播、BGM 的 9:16 营销视频。
```

- [ ] **Step 4: Wait for final video**

Expected evidence:

- at least two Seedream image generation jobs;
- at least two `internal_motion_video/remotion-motion-shot-v1` jobs;
- at least one Volcengine audio job;
- one `compose_final_video` job;
- no Seedance job.

- [ ] **Step 5: Query DB**

Run SQL queries against the dev DB to verify:

```sql
SELECT provider, model_id, operation_type, status, count(*)
FROM generation_job
WHERE workspace_id = '<workspace-id>'
GROUP BY provider, model_id, operation_type, status
ORDER BY provider, model_id, operation_type, status;
```

Expected:

- no provider/model for Seedance;
- `internal_motion_video/remotion-motion-shot-v1` exists and succeeded;
- Volcengine image/audio jobs exist;
- final compose job exists and succeeded.

- [ ] **Step 6: Inspect Agent traces**

Query tool calls or task logs for the workspace. Expected:

- Producer loaded `motion-shot-producer`;
- Craftsman loaded `motion-shot-craftsman`;
- Reviewer loaded `motion-shot-reviewer`;
- `dispatch_craftsman` used `video_route_policy=motion_only`;
- `upsert_render_plan` used `motion_shot_video/image_to_motion_video`;
- Composer dispatched only after required shot/audio assets existed.

- [ ] **Step 7: Inspect logs**

Read server logs. Expected:

- no `internal_template_video`, `hyperframes`, or `template_video` route;
- Remotion sandbox job command present;
- no repeated reviewer or craftsman loop.

- [ ] **Step 8: Download and inspect final media**

Download final MP4 via signed URL and run:

```bash
ffprobe -v error -show_streams -show_format -of json <final-video.mp4>
```

Expected:

- video stream exists;
- audio stream exists;
- duration matches planned final duration or voiceover with acceptable tail;
- resolution is 9:16.

- [ ] **Step 9: Commit M11.5**

Run:

```bash
git add scripts/smoke-m11-5-agent-remotion-motion-e2e.sh docs/milestones/m11-remotion-motion-shot-video.md
git commit -m "test: add remotion motion shot e2e audit"
```

## Final Completion Audit

Before marking the goal complete, verify:

```bash
rg -n "internal_template_video|hyperframes-html|template_video|TemplateVideo|image_to_template_video|template_to_video|template_only" apps/server/internal apps/server/cmd/server sandbox-image scripts
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected:

- no active HyperFrames/template route hits;
- all tests/builds pass;
- browser E2E, DB audit, log audit, and media audit are recorded in the milestone or a report;
- final video exists and meets the no-Seedance Remotion motion shot requirements.
