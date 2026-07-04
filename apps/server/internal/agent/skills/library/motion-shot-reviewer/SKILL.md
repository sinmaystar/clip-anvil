---
name: motion-shot-reviewer
description: Use when Reviewer evaluates Remotion motion_shot_video RenderPlans or artifacts for timing, text safety, product visibility, route policy, and readiness for final composition.
role_scope: [reviewer]
task_types: [reviewer_turn]
domain: [commerce_ad, motion_shot_video, quality_gate]
tools: [read_project_context, read_project_memory, submit_review_result]
source:
  kind: clipanvil-local
  inspired_by: remotion composition review, commerce video quality gates
version: 0.1.0
---
# Motion Shot Reviewer

## Use When

Load this skill when reviewing a `motion_shot_video` RenderPlan, Remotion-rendered shot video, motion fallback, or no-Seedance final segment.

## Do

- Read project context when artifact status, provider request, or current version is not in the task payload.
- Read ProjectMemory when judging forbidden providers, product identity, brand tone, route policy, or delivery promise.
- Judge the motion artifact on axes that matter for image-driven Remotion video:
  - `scene_timing`: text and visual layers appear in exclusive windows, no final-frame stacking, no long dead hold.
  - `text_safety`: headline, caption, CTA, and labels fit safe areas and remain readable over product imagery.
  - `product_visibility`: product or generated hero image is visible, nonbroken, and not hidden behind copy.
  - `motion_rhythm`: light motion supports the selling rhythm without looking like a static slide or distracting from the product.
  - `audio_readiness`: voiceover/BGM plan exists or silence is explicitly accepted before final composition.
  - `seedance_policy`: no Seedance/provider drift when the route is motion-only.
- When calling `submit_review_result`, map those judgments onto the tool-supported rubric axes.
  - For `pre_render_plan_review`, always include `faithfulness`, `subject_consistency`, and `continuity`.
  - Use `faithfulness` for route policy, provider/model compliance, and prompt/parameter truthfulness.
  - Use `subject_consistency` for product identity, reference bindings, and asset readiness.
  - Use `continuity` for timing, motion flow, output format, audio/video handoff, and final-frame stacking risk.
  - Do not invent rubric axis names such as `scene_timing`, `motion_rhythm`, `audio_readiness`, or `seedance_policy` in the tool call.
- Recommend repair fields that Producer or Craftsman can act on: text_layers, visual_layers, motion_style, safe_area, input image, duration, transitions, or route policy.

## Do Not

- Do not penalize a motion shot for not having true generated motion, actor performance, lip sync, or physical scene continuity.
- Do not approve unreadable text just because the provider job succeeded.
- Do not accept a missing/broken product image for a product ad motion shot.
- Do not dispatch retries, change RenderPlans, or request users directly.

## Tool Protocol

1. `read_project_context` if the review target lacks provider, model, artifact, or version facts.
2. `read_project_memory` if policy or brand facts are needed.
3. `submit_review_result` exactly once with concrete scores, localized issues, and retry recommendation.

## Quality Bar

- A passing motion shot is usable in final composition without surprising the user about cost route or motion limits.
- A failing review points to a specific repair action, not generic "make it better" feedback.
- `scene_timing`, `text_safety`, `product_visibility`, `motion_rhythm`, `audio_readiness`, and `seedance_policy` are all considered before acceptance.
