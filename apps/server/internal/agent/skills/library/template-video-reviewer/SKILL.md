---
name: template-video-reviewer
description: Use when Reviewer evaluates HyperFrames/template video RenderPlans or artifacts for scene timing, text safety, product visibility, template route policy, and readiness for final composition.
role_scope: [reviewer]
task_types: [reviewer_turn]
domain: [commerce_ad, template_video, quality_gate]
tools: [read_project_context, read_project_memory, submit_review_result]
source:
  kind: clipanvil-local
  inspired_by: heygen-com/hyperframes skills hyperframes-core hyperframes-animation
version: 0.1.0
---
# Template Video Reviewer

## Use When

Load this skill when reviewing a `template_video` RenderPlan, HyperFrames-rendered shot video, template fallback, or no-Seedance final segment.

## Do

- Read project context when artifact status, provider request, or current version is not in the task payload.
- Read ProjectMemory when judging forbidden providers, product identity, brand tone, route policy, or delivery promise.
- Judge the template artifact on axes that matter for deterministic template video:
  - `scene_timing`: scenes appear in exclusive windows, no final-frame stacking, no long dead hold.
  - `text_safety`: headline, caption, CTA, and badge fit safe areas and remain readable over product imagery.
  - `product_visibility`: product or generated hero image is visible, nonbroken, and not hidden behind copy.
  - `blueprint_fit`: chosen blueprint matches the shot job: product hero, benefit grid, comparison, kinetic type, or CTA.
  - `audio_readiness`: voiceover/BGM plan exists or silence is explicitly accepted before final composition.
  - `seedance_policy`: no Seedance/provider drift when the route is template-only.
- When calling `submit_review_result`, map those template-specific judgments onto the tool-supported rubric axes.
  - For `pre_render_plan_review`, always include `faithfulness`, `subject_consistency`, and `continuity`.
  - Use `faithfulness` for route policy, provider/model compliance, and prompt/parameter truthfulness.
  - Use `subject_consistency` for product identity, reference bindings, and asset readiness.
  - Use `continuity` for scene timing, blueprint flow, output format, audio/video handoff, and final-frame stacking risk.
  - Do not invent rubric axis names such as `scene_timing`, `blueprint_fit`, `audio_readiness`, or `seedance_policy` in the tool call.
- Recommend repair fields that Producer or Craftsman can act on: template_key, variables, text length, scene count, safe panel, input image, or route policy.

## Do Not

- Do not penalize a template video for not having true generated motion, actor performance, lip sync, or physical scene continuity.
- Do not approve unreadable text just because the provider job succeeded.
- Do not accept a missing/broken product image for a product ad template.
- Do not dispatch retries, change RenderPlans, or request users directly.

## Tool Protocol

1. `read_project_context` if the review target lacks provider, model, artifact, or version facts.
2. `read_project_memory` if policy or brand facts are needed.
3. `submit_review_result` exactly once with concrete scores, localized issues, and retry recommendation.

## Quality Bar

- A passing template video is usable in final composition without surprising the user about cost route or motion limits.
- A failing review points to specific repair action, not generic "make it better" feedback.
- `scene_timing`, `text_safety`, `product_visibility`, `audio_readiness`, and `seedance_policy` are all considered before acceptance.
