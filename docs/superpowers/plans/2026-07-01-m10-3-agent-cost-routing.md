# M10.3 Agent Cost Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development. Keep M10.3 behind the M10.2 gate; do not proceed if `internal_template_video` smoke is failing.

**Goal:** Let Agent use the low-cost template route by default for non-hero shot videos. Producer can still explicitly choose Seedance, but broad `dispatch_craftsman(target_phase=shot_video)` should recommend at most one Seedance hero route and route remaining benefit / CTA / packshot shots to `template_video`.

**Architecture:** Keep rendering in the existing RenderPlan -> Worker -> Production path. Add per-shot route recommendations to `dispatch_craftsman` task input. Pass those recommendations through `NativeRuntimeContext`. Let `upsert_render_plan` inherit recommended `model_prompt_profile`, `operation`, template params, and default input references when Craftsman omits them. This gives a deterministic cost policy without creating a separate scheduler or bypassing existing production services.

## Files To Change

- Modify: `apps/server/internal/agent/tools/native.go`
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman.go`
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman_native.go`
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman_test.go`
- Modify: `apps/server/internal/agent/tools/upsert_render_plan.go`
- Modify: `apps/server/internal/agent/tools/render_plan_tools_test.go`
- Modify: `apps/server/internal/agent/renderplan/types.go`
- Modify: `apps/server/internal/agent/renderplan/profiles.go`
- Modify: `apps/server/internal/agent/renderplan/profiles_test.go`
- Modify: `apps/server/internal/agent/craftsman/types.go`
- Modify: `apps/server/internal/agent/craftsman/executor.go`
- Modify: `apps/server/internal/agent/craftsman/native_tool_loop.go`
- Modify: `apps/server/internal/agent/craftsman/context_loader.go`
- Create: `docs/superpowers/reports/2026-07-01-m10-3-agent-cost-routing.md`
- Modify: `docs/milestones/m10-hyperframes-template-video-provider.md`

## Tasks

- [ ] Add route recommendation fields to Craftsman task input and native runtime context:
  - `recommended_model_prompt_profile`
  - `recommended_operation`
  - `recommended_params`
  - `recommended_route_reason`
  - `input_node_refs`
- [ ] Make `dispatch_craftsman` recommend `seedance_2_video/image_to_video_first_frame` for the first likely hero shot in broad `shot_video` dispatch and `template_video/image_to_template_video` for the rest.
- [ ] Keep explicit Producer choices possible: if Producer dispatches a single shot or Craftsman explicitly passes `model_prompt_profile`, do not override that explicit tool input.
- [ ] Let `upsert_render_plan` inherit recommended route fields when tool input leaves them blank.
- [ ] Let `upsert_render_plan` convert task `input_node_refs` into default `reference_bindings` when the model omits bindings.
- [ ] Extend RenderPlan params with template-specific fields: `template_key`, `fps`, and `variables`.
- [ ] Validate with unit tests plus targeted server tests.

## Acceptance

- Broad shot video dispatch creates at most one Seedance-recommended Craftsman task by default.
- Non-hero shot video tasks default to `template_video` and `image_to_template_video`.
- A template shot RenderPlan submitted from Craftsman carries the preview-image binding into Worker input.
- `modelForRenderPlan` resolves template profile to `internal_template_video/hyperframes-html`.
- Verification includes:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./internal/agent/renderplan ./internal/agent/craftsman ./internal/agent/worker
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```
