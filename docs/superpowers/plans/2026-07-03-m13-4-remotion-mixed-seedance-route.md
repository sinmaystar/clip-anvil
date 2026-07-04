# M13.4 Remotion Final Composer Mixed Seedance Route Implementation Plan

## Goal

Make `remotion_timeline_v1` the final Composer route for both low-cost still-image videos and mixed-cost videos. A mixed-cost timeline can combine a small number of Seedance `shot_video` clips with Seedream still-image segments, then apply Remotion layout, text, captions, transitions, voiceover, and BGM as the final packaging layer.

## Current Code Facts

- `get_composition_context` already prefers a shot `shot_video` node when present and falls back to `preview_image`; it marks these assets as `role=clip` or `role=still`.
- `deterministicComposerRemotionTimelinePlan` already maps `role=clip` to timeline asset `type=video`, and `role=still` to `type=image`.
- `RemotionTimelinePlan` schema accepts arbitrary asset `type` strings today, but validation does not enforce `image` / `video`.
- The Remotion renderer currently renders visual assets through `<Img>`, so `type=video` cannot actually render correctly yet.
- M13.3 validation already blocks unknown layout/motion/transition, repeated visuals, internal-caption leakage, and obvious wheel/storage mismatch.
- Existing Producer / Composer / Reviewer prompts describe no-Seedance Remotion route, but do not yet clearly define `no-seedance` / `mixed-cost` / `premium` cost route policies.

## Non-Goals

- Do not make mixed-cost the default when the user explicitly says no Seedance.
- Do not run an expensive real Seedance API E2E unless explicitly requested.
- Do not replace Seedance provider internals.
- Do not build a full interactive timeline editor.
- Do not let Agent generate arbitrary TSX/CSS.

## Phase 1: Plan Schema And Renderer Video Support

### Tasks

1. Add controlled asset type validation in Go:
   - allowed segment asset types: `image`, `video`
   - reject unknown non-empty asset types
   - ensure at least one visual asset exists per segment
2. Mirror the same asset type validation in Remotion TS schema.
3. Add a `MediaLayer` abstraction in `sandbox-image/remotion-timeline/src/index.tsx`:
   - image asset uses `<Img>`
   - video asset uses Remotion `<Video>`
   - video assets are muted by default to avoid clashing with Composer voiceover/BGM
   - video assets use `objectFit` consistent with existing image layouts
4. Update layout components to render `MediaLayer` instead of direct `<Img>`.
5. Add fixture/smoke coverage for at least one video segment and at least one image segment in the same timeline.

### Deliverables

- Go validation rejects invalid asset type.
- TS schema rejects invalid asset type.
- Remotion renderer can render mixed image/video segments.
- Smoke script proves mixed timeline renders an MP4 with audio.

### Acceptance

- `remotiontimeline.Validate` accepts `image` and `video` assets.
- `remotiontimeline.Validate` rejects unknown asset types.
- Mixed fixture output has video stream and audio stream.
- Mixed fixture output duration matches plan duration.

## Phase 2: Composer Mixed-Cost Semantics

### Tasks

1. Update Composer context schema text:
   - `remotion_timeline_v1` accepts image and video visual assets.
   - `clip` assets should be used for Seedance hero/complex-motion shots when present.
   - `still` assets remain first-class for all other segments.
2. Update Composer system prompt and skills:
   - Composer must preserve no-Seedance compliance.
   - Composer may mix `type=video` and `type=image` only when Producer route permits or such assets already exist.
   - Composer should not invent Seedance usage; it only packages available assets.
3. Update deterministic Composer fallback:
   - prefer same-shot `clip` for hero/CTA/complex-motion cues when available.
   - otherwise use same-shot `still`.
   - include asset type in plan and keep cue timing.
4. Add unit tests:
   - deterministic Composer can output a plan containing both `video` and `image` segments.
   - no-Seedance path with only still assets remains unchanged.

### Deliverables

- Composer route can create mixed image/video Remotion plans.
- Existing no-Seedance Remotion route does not regress.
- Prompt and skill docs tell Composer that Remotion is the packaging/editing engine, not a model video generator.

### Acceptance

- Composer test confirms mixed timeline contains at least one video segment and one image segment.
- Composer test confirms no-Seedance still-only fixture has zero video segments.
- Plan validation passes for both.

## Phase 3: Producer And Reviewer Route Policy

### Tasks

1. Update Producer prompt / skill route policy:
   - `no-seedance`: Seedream stills + Volcengine audio + Remotion final timeline.
   - `mixed-cost`: limited Seedance only for hero/complex-motion shots; Seedream stills for the rest; Remotion final timeline.
   - `premium`: more Seedance can be used, but Remotion still handles final packaging.
2. Update Reviewer prompt / skill:
   - report Seedance video segment count.
   - report Remotion still segment count.
   - flag no-Seedance violation if any Seedance provider/model job exists.
   - flag mixed-cost overuse if Seedance is used outside Producer-approved shot scope.
3. Add tests for prompt/skill text where practical.

### Deliverables

- Producer can intentionally request mixed-cost without breaking no-Seedance.
- Reviewer has cost/quality summary expectations.

### Acceptance

- Prompt tests confirm route policy language exists.
- Skill registry includes mixed route guidance.

## Phase 4: Verification

### Required Local Verification

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/remotiontimeline ./internal/agent/composer ./internal/agent/producer ./internal/agent/reviewer ./internal/agent/skills ./internal/agent/tools ./internal/sandbox -count=1
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

```bash
node --check sandbox-image/remotion-timeline/src/render.mjs
bash -n scripts/smoke-m13-4-remotion-mixed-media.sh
./scripts/smoke-m13-4-remotion-mixed-media.sh
git diff --check
```

### Browser E2E

Run a real browser Agent E2E as a no-Seedance regression:

- upload `box.png`
- ask for a Chinese product marketing video
- explicitly forbid Seedance
- require Seedream images, Volcengine voiceover/BGM, and `remotion_timeline_v1`
- verify final artifact, audio/video streams, duration, and DB Seedance provider/model job count = 0

Mixed-cost real Seedance E2E is cost-bearing and should only run when explicitly requested. Until then, mixed media behavior is accepted through deterministic fixture/smoke and unit tests.

## Risks

- Remotion `<Video>` may increase render time compared with still-only timelines.
- Real Seedance clips may include their own embedded audio; renderer must mute video layer by default.
- If Producer generates too few stills, repetition validation may reject otherwise valid mixed plans.
- Prompt-only route policy is not a hard runtime cost router yet; DB audit remains required for no-Seedance compliance.

## Completion Criteria

- Mixed image/video Remotion timeline renders locally.
- Composer can produce and validate mixed plans.
- no-Seedance route remains protected.
- Producer/Reviewer/Composer prompts describe the three route policies consistently.
- M13.4 milestone records verification evidence and clearly distinguishes cost-free fixture mixed validation from cost-bearing real Seedance E2E.
