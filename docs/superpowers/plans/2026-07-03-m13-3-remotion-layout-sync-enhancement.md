# M13.3 Remotion Layout And Cue Sync Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade `remotion_timeline_v1` from a valid renderer into a controllable marketing video layout and cue-sync engine that prevents subtitle overlap, visual repetition, and obvious cue/asset mismatches.

**Architecture:** Keep Producer and Craftsman responsible for dynamic storyboard, Seedream still generation, and Volcengine audio generation. Composer owns the final `RemotionTimelinePlan`: cue timing, asset selection, layout/motion/transition choices, caption lane, and validation before Remotion render. Remotion remains a deterministic renderer that receives JSON, never raw agent-generated React code.

**Tech Stack:** Go 1.26, pgx/sqlc, ClipAnvil Agent native tools, Remotion/React/TypeScript in `sandbox-image/remotion-timeline`, Docker sandbox, ffprobe smoke checks.

---

## Current Code Facts

- `apps/server/internal/remotiontimeline/plan.go` already decodes and validates `schema`, `composition`, output timing, segment timing, asset workspace paths, audio workspace paths, and `motion.intensity` as string or number.
- `sandbox-image/remotion-timeline/src/index.tsx` currently renders every segment through one `SegmentView`, supports only simple image scale/pan, one bottom caption, and does not visually distinguish `hero_packshot`, `detail_focus`, `benefit_card`, `open_storage`, or `cta_endcard`.
- `apps/server/internal/agent/composer/tool_context_provider.go` exposes only five layout keys, six motion keys, and four transition keys in `remotion_timeline_schema`.
- `apps/server/internal/agent/composer/model_responder.go` has a deterministic Remotion fallback for cue-based plans, but it does not enforce layout diversity, asset repetition limits, or cue/asset mismatch blockers.
- `apps/server/internal/agent/reviewer/system_prompt.go` mentions final audio review and Remotion Motion Shot review, but not Remotion final timeline layout/caption/cue quality gates.

## File Map

- Modify `apps/server/internal/remotiontimeline/plan.go`: add controlled enum sets, stricter caption validation, repeated layout/image checks, and cue/asset compatibility checks.
- Modify `apps/server/internal/remotiontimeline/plan_test.go`: add failing tests for unknown enums, internal captions, repeated visuals, duplicate layouts, and cue/asset mismatch.
- Modify `sandbox-image/remotion-timeline/src/schema.ts`: align TypeScript unions with Go enum sets and keep `motion.intensity` compatible with string or number.
- Modify `sandbox-image/remotion-timeline/src/index.tsx`: introduce layout-specific rendering functions, transition styling, responsive Chinese caption wrapping, and non-overlapping text/caption lanes.
- Modify `apps/server/internal/agent/composer/tool_context_provider.go`: expose the complete supported layout, motion, and transition vocabulary to Composer.
- Modify `apps/server/internal/agent/composer/model_responder.go`: make deterministic plan generation choose cue-aware layouts/motions/transitions and reject obvious missing/mismatched assets.
- Modify `apps/server/internal/agent/composer/model_responder_test.go`: assert plan validity, layout diversity, caption source safety, and cue/asset matching.
- Modify `apps/server/internal/agent/composer/system_prompt.go`: teach Composer the stricter Remotion timeline contract.
- Modify `apps/server/internal/agent/skills/library/remotion-timeline-composer/SKILL.md`: add operational rules for layout diversity, cue/asset matching, and caption lane.
- Modify `apps/server/internal/agent/skills/library/composer-timeline-director/SKILL.md`: keep final Composer rules aligned with the new Remotion route.
- Modify `apps/server/internal/agent/reviewer/system_prompt.go`: add Remotion final timeline review rules.
- Modify or create `apps/server/internal/agent/skills/library/final-video-remotion-reviewer/SKILL.md`: dedicated Reviewer skill for Remotion final video layout, subtitle, no-Seedance, and cue sync checks.
- Create `scripts/smoke-m13-3-remotion-layouts.sh`: fixture render smoke covering all new layout keys and ffprobe verification.
- Modify `docs/milestones/m13-remotion-timeline-composer.md`: update M13.3 verification record only after implementation and verification pass.

## Controlled Vocabulary

Use exactly these values in Go validation, TypeScript schema, Composer context, prompts, and fixture smoke.

Layouts:

```text
hero_packshot
detail_focus
benefit_card
split_compare
scenario_card
open_storage
cta_endcard
```

Motions:

```text
push_in
pull_out
pan_left
pan_right
float_parallax
spotlight_reveal
kinetic_text
cta_pop
```

Transitions:

```text
cut
crossfade
slide
wipe
zoom_blur
```

Allowed caption sources:

```text
audio_cue
voiceover_alignment
tts_alignment
manual_caption
```

Forbidden final caption sources:

```text
narrative_purpose
visual_intent
action_text
camera_intent
director_note
internal_note
storyboard_note
```

## Task 1: Add RemotionTimelinePlan Quality Validation

**Files:**
- Modify: `apps/server/internal/remotiontimeline/plan.go`
- Modify: `apps/server/internal/remotiontimeline/plan_test.go`

- [ ] **Step 1: Add failing enum tests**

Add tests with these names in `apps/server/internal/remotiontimeline/plan_test.go`:

```go
func TestValidateRejectsUnknownLayoutMotionAndTransition(t *testing.T) {
	plan := validMarketingPlanForTest()
	plan.Segments[0].Layout = "freeform_layout"
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "layout") {
		t.Fatalf("Validate unknown layout error = %v, want layout error", err)
	}

	plan = validMarketingPlanForTest()
	plan.Segments[0].Motion.Preset = "spin_forever"
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "motion.preset") {
		t.Fatalf("Validate unknown motion error = %v, want motion.preset error", err)
	}

	plan = validMarketingPlanForTest()
	plan.Segments[0].TransitionIn.Type = "flashbang"
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "transition_in.type") {
		t.Fatalf("Validate unknown transition error = %v, want transition_in.type error", err)
	}
}
```

- [ ] **Step 2: Add failing caption safety test**

Add this test:

```go
func TestValidateRejectsInternalCaptionSourcesAndText(t *testing.T) {
	plan := validMarketingPlanForTest()
	plan.Captions.SingleLane = true
	plan.Segments[0].Caption = Caption{
		Source:   "visual_intent",
		Text:     "前三秒抓住短途出行用户注意",
		StartSec: 0,
		EndSec:   4,
		Position: "subtitle_bottom",
	}
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "caption.source") {
		t.Fatalf("Validate internal caption source error = %v, want caption.source error", err)
	}

	plan = validMarketingPlanForTest()
	plan.Segments[0].Caption.Text = "短途出行痛点钩子"
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "caption.text") {
		t.Fatalf("Validate internal caption text error = %v, want caption.text error", err)
	}
}
```

- [ ] **Step 3: Add failing repetition and mismatch tests**

Add this test:

```go
func TestValidateRejectsRepeatedVisualsLayoutsAndCueMismatch(t *testing.T) {
	plan := validMarketingPlanForTest()
	plan.Segments = []Segment{
		testSegment("shot_01", 0, 4, "hero_packshot", "短途出行", "/workspace/input/hero.png"),
		testSegment("shot_02", 4, 8, "hero_packshot", "顺滑万向轮", "/workspace/input/hero.png"),
		testSegment("shot_03", 8, 12, "hero_packshot", "大周出行收纳", "/workspace/input/hero.png"),
	}
	plan.Output.DurationSec = 12
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "repeated visual") {
		t.Fatalf("Validate repeated visual error = %v, want repeated visual error", err)
	}

	plan = validMarketingPlanForTest()
	plan.Segments = []Segment{
		testSegment("shot_01", 0, 4, "benefit_card", "轻便好推", "/workspace/input/hero.png"),
		testSegment("shot_02", 4, 8, "benefit_card", "顺滑万向轮", "/workspace/input/wheel.png"),
		testSegment("shot_03", 8, 12, "benefit_card", "收纳分区", "/workspace/input/storage.png"),
	}
	plan.Output.DurationSec = 12
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "repeated layout") {
		t.Fatalf("Validate repeated layout error = %v, want repeated layout error", err)
	}

	plan = validMarketingPlanForTest()
	plan.Segments[0] = testSegment("shot_02", 0, 4, "detail_focus", "顺滑万向轮", "/workspace/input/open-storage.png")
	if err := Validate(plan); err == nil || !strings.Contains(err.Error(), "cue/asset mismatch") {
		t.Fatalf("Validate mismatch error = %v, want cue/asset mismatch error", err)
	}
}
```

- [ ] **Step 4: Run tests and confirm failure**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/remotiontimeline -run 'TestValidateRejectsUnknownLayoutMotionAndTransition|TestValidateRejectsInternalCaptionSourcesAndText|TestValidateRejectsRepeatedVisualsLayoutsAndCueMismatch' -count=1
```

Expected: tests fail because validation does not yet check enum, caption source, repeated visuals, repeated layouts, or cue/asset mismatch.

- [ ] **Step 5: Implement validation helpers**

In `apps/server/internal/remotiontimeline/plan.go`, add package-level allow lists and helper functions:

```go
var allowedLayouts = map[string]bool{
	"hero_packshot": true, "detail_focus": true, "benefit_card": true,
	"split_compare": true, "scenario_card": true, "open_storage": true, "cta_endcard": true,
}

var allowedMotionPresets = map[string]bool{
	"push_in": true, "pull_out": true, "pan_left": true, "pan_right": true,
	"float_parallax": true, "spotlight_reveal": true, "kinetic_text": true, "cta_pop": true,
}

var allowedTransitions = map[string]bool{
	"cut": true, "crossfade": true, "slide": true, "wipe": true, "zoom_blur": true,
}

var allowedCaptionSources = map[string]bool{
	"": true, "audio_cue": true, "voiceover_alignment": true, "tts_alignment": true, "manual_caption": true,
}

var forbiddenCaptionPhrases = []string{
	"短途出行痛点钩子", "前三秒抓住", "视觉意图", "导演", "narrative_purpose",
	"visual_intent", "action_text", "camera_intent", "director_note", "internal_note", "storyboard_note",
}
```

Add helpers:

```go
func validateSegmentVocabulary(index int, segment Segment) error
func validateCaption(index int, segment Segment) error
func validateRepeatedVisualsAndLayouts(segments []Segment) error
func validateCueAssetMatch(index int, segment Segment) error
```

The cue/asset heuristic should be conservative:

```go
cue := strings.ToLower(segment.VisualFocus + " " + segment.Caption.Text + " " + segment.ShotRef + " " + segment.ID)
assetText := strings.ToLower(strings.Join(asset workspace_path, node_ref, role, " "))

wheel cue keywords: "万向轮", "轮", "wheel", "caster"
wheel asset keywords: "wheel", "caster", "detail", "close"
storage cue keywords: "收纳", "分区", "打开", "open", "storage", "interior"
storage asset keywords: "storage", "interior", "open", "inside", "packed"
```

Reject only when the cue clearly belongs to one bucket and the selected asset path/ref clearly belongs to the opposite bucket.

- [ ] **Step 6: Wire validation helpers into `Validate`**

Inside the segment loop in `Validate(plan Plan)`, after timing and asset checks, call:

```go
if err := validateSegmentVocabulary(i, segment); err != nil {
	return err
}
if err := validateCaption(i, segment); err != nil {
	return err
}
if err := validateCueAssetMatch(i, segment); err != nil {
	return err
}
```

After the segment loop, call:

```go
if err := validateRepeatedVisualsAndLayouts(plan.Segments); err != nil {
	return err
}
```

- [ ] **Step 7: Run tests and confirm pass**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/remotiontimeline -count=1
```

Expected: PASS.

## Task 2: Expand Remotion Renderer Layouts, Motions, Transitions, And Caption Lane

**Files:**
- Modify: `sandbox-image/remotion-timeline/src/schema.ts`
- Modify: `sandbox-image/remotion-timeline/src/index.tsx`
- Create: `scripts/smoke-m13-3-remotion-layouts.sh`

- [ ] **Step 1: Update TypeScript schema types**

In `sandbox-image/remotion-timeline/src/schema.ts`, define explicit exported union arrays:

```ts
export const layoutKeys = ["hero_packshot", "detail_focus", "benefit_card", "split_compare", "scenario_card", "open_storage", "cta_endcard"] as const;
export const motionKeys = ["push_in", "pull_out", "pan_left", "pan_right", "float_parallax", "spotlight_reveal", "kinetic_text", "cta_pop"] as const;
export const transitionKeys = ["cut", "crossfade", "slide", "wipe", "zoom_blur"] as const;
export type LayoutKey = (typeof layoutKeys)[number];
export type MotionKey = (typeof motionKeys)[number];
export type TransitionKey = (typeof transitionKeys)[number];
```

Make `Segment.layout`, `Motion.preset`, and `Transition.type` use these types while still allowing runtime `assertTimelinePlan` to throw a readable error for invalid plans.

- [ ] **Step 2: Run TypeScript syntax check**

Run:

```bash
node --check sandbox-image/remotion-timeline/src/render.mjs
```

Expected: PASS.

- [ ] **Step 3: Split renderer into layout helpers**

In `sandbox-image/remotion-timeline/src/index.tsx`, keep one Remotion composition but replace the single generic image block with functions:

```tsx
const renderLayout = (segment: Segment, accent: string, progress: number) => {
  switch (segment.layout) {
    case "hero_packshot":
      return <HeroPackshot segment={segment} accent={accent} progress={progress} />;
    case "detail_focus":
      return <DetailFocus segment={segment} accent={accent} progress={progress} />;
    case "benefit_card":
      return <BenefitCard segment={segment} accent={accent} progress={progress} />;
    case "split_compare":
      return <SplitCompare segment={segment} accent={accent} progress={progress} />;
    case "scenario_card":
      return <ScenarioCard segment={segment} accent={accent} progress={progress} />;
    case "open_storage":
      return <OpenStorage segment={segment} accent={accent} progress={progress} />;
    case "cta_endcard":
      return <CtaEndcard segment={segment} accent={accent} progress={progress} />;
    default:
      return <HeroPackshot segment={segment} accent={accent} progress={progress} />;
  }
};
```

Each helper must use the same `assetSrc()` function and preserve the current no-raw-code JSON contract.

- [ ] **Step 4: Implement layout-specific positioning**

Implement these stable positions:

- `hero_packshot`: large centered product, headline in upper third, caption bottom.
- `detail_focus`: product/detail image fills 78% height, a small focus label above caption.
- `benefit_card`: product right or centered, benefit text block upper left, caption bottom.
- `split_compare`: two visual panels when two assets exist; when one asset exists, duplicate it with different crop/scale and add text layers as comparison labels.
- `scenario_card`: product image with travel-context text card in upper third, no text in caption lane.
- `open_storage`: interior/open luggage image lower middle with headline above image, caption bottom.
- `cta_endcard`: centered product, strong CTA text above caption, brand/accent bar near bottom but above caption lane.

Do not place any text layer lower than `bottom: 18%`; reserve `bottom: 6%` to `bottom: 16%` for subtitles.

- [ ] **Step 5: Add transition styling**

Replace the fixed fade opacity with a helper:

```tsx
const transitionStyle = (segment: Segment, frame: number, durationInFrames: number): React.CSSProperties => {
  const transition = segment.transition_in?.type ?? "crossfade";
  // cut: no entrance fade
  // crossfade: opacity 0 -> 1 over 10 frames
  // slide: translateY(48px) -> 0 and opacity 0 -> 1
  // wipe: clipPath inset changes from left/right to 0
  // zoom_blur: scale 1.08 -> 1 and blur 8px -> 0
};
```

The exit fade may stay gentle for all presets, but must not blank the final frame before the next sequence starts.

- [ ] **Step 6: Add safe Chinese caption wrapping**

Update `CaptionView` so it receives `text`, `maxCharsPerLine`, and `position`. Implement a helper:

```tsx
const wrapChineseCaption = (text: string, maxChars = 18): string[] => {
  const clean = text.trim();
  if (clean.length <= maxChars) return [clean];
  const chunks: string[] = [];
  for (let i = 0; i < clean.length; i += maxChars) {
    chunks.push(clean.slice(i, i + maxChars));
  }
  return chunks.slice(0, 2);
};
```

Render each line in its own block inside one subtitle container. Keep font size <= `44`, line height `1.18`, and bottom offset `6%`.

- [ ] **Step 7: Add layout smoke fixture**

Create `scripts/smoke-m13-3-remotion-layouts.sh` by copying the M13.1 smoke structure, but create a `timeline-plan.json` with seven segments, one per layout key, and two fixture images copied into the sandbox workspace. Use generated silent/short audio only if an existing M13.1 smoke utility already does so; otherwise use the existing fixture audio from M13.1.

The script must:

```bash
set -euo pipefail
```

Then render `remotion_timeline_v1`, run `ffprobe`, and print:

```text
M13.3 Remotion layout smoke passed
```

- [ ] **Step 8: Run renderer checks**

Run:

```bash
bash -n scripts/smoke-m13-3-remotion-layouts.sh
node --check sandbox-image/remotion-timeline/src/render.mjs
./scripts/smoke-m13-3-remotion-layouts.sh
```

Expected: smoke renders a `1080x1920` MP4 with all seven layouts, video stream, audio stream, and duration matching the fixture timeline.

## Task 3: Teach Composer The Stricter Remotion Timeline Contract

**Files:**
- Modify: `apps/server/internal/agent/composer/tool_context_provider.go`
- Modify: `apps/server/internal/agent/composer/model_responder.go`
- Modify: `apps/server/internal/agent/composer/model_responder_test.go`
- Modify: `apps/server/internal/agent/composer/system_prompt.go`
- Modify: `apps/server/internal/agent/skills/library/remotion-timeline-composer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/library/composer-timeline-director/SKILL.md`

- [ ] **Step 1: Update Composer schema context**

In `remotionTimelineSchemaContext()`, replace the layout, motion, and transition arrays with the controlled vocabulary from this plan. Change `caption_source` to:

```go
"caption_source": "audio_plan.cue_plan.caption, voiceover_alignment, tts_alignment, or manual_caption only; never narrative_purpose, visual_intent, action_text, camera_intent, or internal director notes",
```

Add:

```go
"caption_lane": "single Composer-owned subtitle_bottom lane; text layers must stay outside the bottom 18 percent safe area",
"asset_matching": "match cue shot_ref and visual_focus to same-shot still/clip; wheel cues require wheel/detail assets; storage cues require open/interior/storage assets",
"repetition_limits": "avoid using the same visual asset for more than half of segments and avoid the same layout more than two segments in a row",
```

- [ ] **Step 2: Add deterministic layout classifier tests**

In `apps/server/internal/agent/composer/model_responder_test.go`, add assertions to the existing deterministic Remotion plan test:

```go
layouts := map[string]bool{}
for _, segment := range plan.Segments {
	layouts[segment.Layout] = true
	if segment.Caption.Source != "audio_cue" {
		t.Fatalf("caption source = %q, want audio_cue", segment.Caption.Source)
	}
}
if len(layouts) < 4 {
	t.Fatalf("layout diversity = %d, want at least 4 layouts: %#v", len(layouts), layouts)
}
```

Also assert:

```go
if strings.Contains(strings.Join(captionsForTest(plan), " "), "痛点钩子") || strings.Contains(strings.Join(captionsForTest(plan), " "), "前三秒抓住") {
	t.Fatalf("captions contain internal planning text: %#v", captionsForTest(plan))
}
```

- [ ] **Step 3: Run Composer test and confirm failure**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer -run TestDeterministicComposerRemotionTimelinePlanUsesCuePlanAndStills -count=1
```

Expected: FAIL if deterministic fallback still chooses too few layouts or misses caption source guarantees.

- [ ] **Step 4: Implement cue-aware deterministic layout selection**

In `apps/server/internal/agent/composer/model_responder.go`, update the Remotion fallback classifier:

```go
func remotionLayoutForCue(index int, cue composerCue, asset composerStagedAsset, total int) string {
	text := strings.ToLower(strings.Join([]string{cue.ShotRef, cue.Caption, cue.VisualFocus, cue.VisualIntent, asset.FileName, asset.NodeRef}, " "))
	switch {
	case index == total-1 || strings.Contains(text, "cta") || strings.Contains(text, "现在") || strings.Contains(text, "出发"):
		return "cta_endcard"
	case strings.Contains(text, "万向轮") || strings.Contains(text, "wheel") || strings.Contains(text, "caster"):
		return "detail_focus"
	case strings.Contains(text, "收纳") || strings.Contains(text, "分区") || strings.Contains(text, "open") || strings.Contains(text, "storage") || strings.Contains(text, "interior"):
		return "open_storage"
	case strings.Contains(text, "场景") || strings.Contains(text, "出差") || strings.Contains(text, "周末"):
		return "scenario_card"
	case strings.Contains(text, "对比") || strings.Contains(text, "compare"):
		return "split_compare"
	case index == 0:
		return "hero_packshot"
	default:
		return "benefit_card"
	}
}
```

Add similar helper functions for motion and transition:

```go
func remotionMotionForLayout(layout string, index int) remotiontimeline.Motion
func remotionTransitionForIndex(index int) remotiontimeline.Transition
```

Use `caption.Source = "audio_cue"` and set `Caption.Position = "subtitle_bottom"` for every cue-derived caption.

- [ ] **Step 5: Prevent obvious asset mismatch in deterministic fallback**

Before choosing a staged asset for a cue, prefer same `shot_ref`. If there are multiple stills for a shot, prefer:

- wheel/detail cue: filename, node ref, title, or visual intent containing `wheel`, `caster`, `detail`, `轮`, `万向轮`
- storage cue: containing `storage`, `interior`, `open`, `inside`, `收纳`, `分区`, `打开`
- CTA cue: containing `hero`, `packshot`, `cta`, `product`

If no same-shot asset exists but a cross-shot asset clearly matches the visual focus, use it and include the original `node_ref` in the plan. If every available asset clearly belongs to an opposite focus bucket, return a blocked Composer response instead of creating a bad timeline.

- [ ] **Step 6: Update Composer prompts and skills**

Add to `apps/server/internal/agent/composer/system_prompt.go`:

```text
For remotion_timeline_v1, Composer is the layout editor. Choose a layout, motion, transition, text layer, caption, and visual asset for each cue. Use cue captions or alignment as subtitles; never use internal fields such as narrative_purpose, visual_intent, action_text, or camera_intent as final captions. Block instead of rendering when a wheel cue only has storage/interior imagery, or a storage cue only has wheel/detail imagery.
```

Add the same operational rules to:

- `apps/server/internal/agent/skills/library/remotion-timeline-composer/SKILL.md`
- `apps/server/internal/agent/skills/library/composer-timeline-director/SKILL.md`

- [ ] **Step 7: Run Composer tests**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer -count=1
```

Expected: PASS.

## Task 4: Add Reviewer Quality Gate For Remotion Final Timeline

**Files:**
- Modify: `apps/server/internal/agent/reviewer/system_prompt.go`
- Create: `apps/server/internal/agent/skills/library/final-video-remotion-reviewer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/registry.go` only if the registry requires explicit skill listing.
- Modify: `apps/server/internal/agent/reviewer/system_prompt_test.go`

- [ ] **Step 1: Add Reviewer prompt test**

In `apps/server/internal/agent/reviewer/system_prompt_test.go`, add:

```go
func TestSystemPromptIncludesRemotionTimelineFinalReviewRules(t *testing.T) {
	prompt := SystemPrompt()
	for _, want := range []string{
		"Remotion Timeline final video",
		"single Composer-owned caption lane",
		"wheel cue",
		"storage cue",
		"no-Seedance",
		"layout repetition",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("SystemPrompt() missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run test and confirm failure**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run TestSystemPromptIncludesRemotionTimelineFinalReviewRules -count=1
```

Expected: FAIL because the prompt lacks these rules.

- [ ] **Step 3: Add Reviewer prompt section**

In `apps/server/internal/agent/reviewer/system_prompt.go`, after `final video 音频评审重点`, add:

```text
Remotion Timeline final video 评审重点：
- 检查 final timeline 是否使用 `remotion_timeline_v1` 时保留 single Composer-owned caption lane；不能出现双字幕、字幕与标题重叠、字幕超出底部安全区。
- 检查 cue/asset 语义同步：wheel cue 必须使用万向轮、轮子、细节或可解释的商品近景素材；storage cue 必须使用打开、内里、分区或收纳素材。
- 检查 layout repetition：同一 layout 不应连续重复超过 2 次，同一 still image 不应覆盖多数 segments，除非 Producer 明确说明素材不足。
- 检查 no-Seedance compliance：用户禁止 Seedance 时，final timeline 可以使用 Seedream still、Volcengine audio 和 Remotion renderer，但不能出现 Seedance video generation job 或伪装成真实视频生成。
- 检查 captions 来源：字幕只能来自 AudioPlan cue、voiceover alignment、TTS alignment 或人工字幕；不能使用 narrative_purpose、visual_intent、action_text、camera_intent 或内部导演笔记。
- 发现字幕重叠、口播画面错配、音频缺失、Seedance 违规或重复视觉过高时，提交 blocking issue 或 rejected verdict。
```

- [ ] **Step 4: Create dedicated Reviewer skill**

Create `apps/server/internal/agent/skills/library/final-video-remotion-reviewer/SKILL.md`:

```markdown
---
name: final-video-remotion-reviewer
description: Use when Reviewer evaluates a final video rendered by remotion_timeline_v1 for layout quality, caption lane safety, cue/asset sync, no-Seedance compliance, audio presence, and marketing rhythm.
role_scope: [reviewer]
task_types: [reviewer_turn]
domain: [final_video, remotion_timeline, commerce_ad]
tools: [read_project_context, read_project_memory, submit_review_result]
---

# Final Video Remotion Reviewer

## When To Load

Load this skill for final_video_review when the final timeline uses `remotion_timeline_v1`, when Producer requested a low-cost no-Seedance route, or when the output uses Seedream stills plus Remotion layout/motion.

## Rules

- Verify there is one Composer-owned caption lane. Reject double subtitles, overlapping subtitles, or captions placed over core product text.
- Verify captions come from AudioPlan cue text, voiceover alignment, TTS alignment, or explicit manual captions. Reject internal planning text such as `narrative_purpose`, `visual_intent`, `action_text`, `camera_intent`, "短途出行痛点钩子", or "前三秒抓住".
- Verify cue/asset sync. Wheel cues need wheel/detail assets. Storage cues need open/interior/storage assets. CTA cues need packshot, brand, or CTA imagery.
- Verify layout diversity. Same layout should not repeat more than twice in a row. Same still should not cover most segments unless the context says assets are insufficient.
- Verify no-Seedance compliance when the user forbids Seedance. Seedream stills, Volcengine audio, and Remotion rendering are allowed; Seedance video generation is not.
- Verify final video has voiceover when AudioPlan requires voiceover and BGM when AudioPlan requires BGM.

## Workflow

1. Read final video context, timeline plan summary, AudioPlan, render jobs, generation jobs, and available artifact facts.
2. Read ProjectMemory if brand, product, or platform constraints are unclear.
3. Submit one review result. Use `audio_sync`, `continuity`, `faithfulness`, `visual_quality`, and `platform_selling_power` for required axes.
4. For blocking issues, write concrete fix hints that Producer/Composer can act on.

## Done

- The review result explicitly mentions caption lane safety, cue/asset sync, audio presence, layout repetition, and no-Seedance compliance when applicable.
- Blocking issues point to the segment, cue, or asset class that needs repair.
```

- [ ] **Step 5: Register skill if needed**

Run:

```bash
rg -n "DefaultRegistry|library" apps/server/internal/agent/skills
```

If skills are loaded from the filesystem dynamically, no code registration is needed. If `registry.go` has explicit entries, add `final-video-remotion-reviewer` in the same format as `final-video-audio-reviewer`.

- [ ] **Step 6: Run Reviewer tests**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer ./internal/agent/skills -count=1
```

Expected: PASS.

## Task 5: End-To-End Verification And Milestone Update

**Files:**
- Modify: `docs/milestones/m13-remotion-timeline-composer.md`

- [ ] **Step 1: Run focused Go tests**

Run:

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/remotiontimeline ./internal/agent/composer ./internal/agent/reviewer ./internal/agent/skills ./internal/agent/tools ./internal/sandbox -count=1
```

Expected: PASS.

- [ ] **Step 2: Run server build**

Run from repo root:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: PASS.

- [ ] **Step 3: Run renderer smoke**

Run:

```bash
./scripts/smoke-m13-3-remotion-layouts.sh
```

Expected: prints `M13.3 Remotion layout smoke passed`, and ffprobe shows:

```text
1080x1920
video stream present
audio stream present
duration matches fixture timeline
```

- [ ] **Step 4: Run browser Agent E2E when external account state permits**

Use a clean workspace or the current M13 E2E workspace if the user wants continuity. The prompt must include:

```text
请基于上传的悦行行李箱图片，生成一个 30 秒以上中文口播营销视频。不要调用 Seedance 或任何视频生成模型。可以使用 Seedream 生成多张商品 still，包括万向轮特写、打开收纳空间、材质/轻便卖点、出行场景和 CTA packshot。使用火山引擎生成 voiceover 和 BGM。最终必须用 remotion_timeline_v1 合成，要求字幕来自口播 cue，画面和口播卖点同步，至少使用 4 种不同 layout。
```

If Volcengine returns `AccountOverdueError` again, record the exact failing task and do not mark M13.3 complete. Continue to validate local deterministic renderer, Composer, and Reviewer behavior.

- [ ] **Step 5: Run DB route smoke after E2E**

Run:

```bash
psql "$DATABASE_URL" -v workspace_id="$WORKSPACE_ID" -f scripts/smoke-m13-2-agent-remotion-route.sql
```

Expected:

- `timeline_plan >= 1`
- `seedance_generation_jobs = 0`
- `seedream_render_plans >= 4`
- `audio_render_plans >= 2`
- latest timeline status is `completed`

- [ ] **Step 6: Inspect `plan_json`**

Query latest plan:

```bash
psql "$DATABASE_URL" -v workspace_id="$WORKSPACE_ID" -c "
select id, template_key, status, jsonb_array_length(plan_json->'segments') as segments,
       (select count(distinct s->>'layout') from jsonb_array_elements(plan_json->'segments') s) as distinct_layouts,
       plan_json
from timeline_plan
where workspace_id = :'workspace_id'::uuid
order by created_at desc
limit 1;"
```

Expected:

- `template_key=remotion_timeline_v1`
- `distinct_layouts >= 4`
- captions use allowed sources only
- no caption text includes internal planning fields
- wheel cue uses wheel/detail asset
- storage cue uses open/interior/storage asset

- [ ] **Step 7: ffprobe final video**

Run:

```bash
ffprobe -v error \
  -show_entries format=duration \
  -show_entries stream=index,codec_type,codec_name,width,height,duration \
  -of json "$FINAL_VIDEO_PATH"
```

Expected:

- format duration >= 30 seconds for real E2E
- video stream present
- audio stream present
- width `1080`, height `1920`

- [ ] **Step 8: Capture frame checks**

Extract frames:

```bash
mkdir -p /tmp/clipanvil-m13-3-frames
ffmpeg -y -ss 8 -i "$FINAL_VIDEO_PATH" -frames:v 1 /tmp/clipanvil-m13-3-frames/wheel.png
ffmpeg -y -ss 16 -i "$FINAL_VIDEO_PATH" -frames:v 1 /tmp/clipanvil-m13-3-frames/storage.png
ffmpeg -y -ss 28 -i "$FINAL_VIDEO_PATH" -frames:v 1 /tmp/clipanvil-m13-3-frames/cta.png
```

Expected:

- wheel frame visually matches wheel/detail cue.
- storage frame visually matches open/storage cue.
- CTA frame has CTA/endcard layout.
- captions are readable and do not overlap title text.

- [ ] **Step 9: Run whitespace check**

Run:

```bash
git diff --check
```

Expected: PASS.

- [ ] **Step 10: Update milestone doc**

Only after all required checks pass, update `docs/milestones/m13-remotion-timeline-composer.md`:

```text
**状态**：M13.1、M13.2、M13.3 已实现并验证；M13.4 未开始
```

Add a concise M13.3 verification record with:

- workspace id
- final artifact id
- timeline plan id
- ffprobe duration, streams, and resolution
- distinct layout count
- seedance job count
- frame spot-check summary
- any caveat such as external account state

## Self-Review Checklist

- [ ] Every M13.3 acceptance criterion has a task and a verification step.
- [ ] No arbitrary layout/motion/transition strings can pass validation.
- [ ] Captions are single-lane and cannot come from internal planning fields.
- [ ] Composer context, Composer deterministic fallback, prompt, and skill all share the same controlled vocabulary.
- [ ] Reviewer has explicit rules for Remotion final timeline outputs.
- [ ] Renderer smoke covers every new layout key.
- [ ] Browser E2E remains the required final proof; external provider failure is recorded as a blocker, not treated as success.
