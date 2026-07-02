---
name: template-video-craftsman
description: Use when Craftsman creates or repairs a HyperFrames-backed template_video RenderPlan for product cards, benefit grids, comparisons, kinetic copy, CTA shots, or no-Seedance video fallback.
role_scope: [craftsman]
task_types: [craftsman_turn]
domain: [commerce_ad, template_video, render_plan]
tools: [read_project_memory, upsert_render_plan]
source:
  kind: clipanvil-local
  inspired_by: heygen-com/hyperframes skills hyperframes-core hyperframes-animation product-launch-video
version: 0.1.0
---
# Template Video Craftsman

## Use When

Load this skill before creating or repairing a RenderPlan whose video must be rendered by `internal_template_video/hyperframes-html` instead of Seedance.

## Do

- Read ProjectMemory when route policy, blueprint, product identity, text copy, or forbidden providers are not already in the task context.
- Use `model_prompt_profile: template_video`.
- Use `operation: image_to_template_video` when a product, Seedream hero, or preview image is an input; use `operation: template_to_video` only when no image asset is required.
- Set `target_phase: shot_video` for template video output.
- Put template choice and variables in `params`:
  - `template_key`: one of the approved ClipAnvil template keys.
  - `duration_sec`, `ratio`, `resolution`, `fps`.
  - `variables.headline`, `variables.caption`, `variables.cta`, `variables.scenes`, `variables.brand_colors`.
- Bind input media through `reference_bindings` or inherited `input_node_refs`; do not invent file paths.
- Write `generation_text` as a compact shot contract: blueprint, product role, copy hierarchy, asset placement, scene order, motion level, and forbidden video models.
- Include audit hints that the video route is template-only and that Seedance is not allowed when the user requested no-Seedance.

## Blueprint Menu

- `product_hero_v2`: product image is the visual hero. Use for packshot, product reveal, premium still, or Seedream-generated commercial hero. Motion should be light camera move, product drift, and copy reveal.
- `benefit_grid_assemble`: multiple benefits assemble as cards, pills, or list lines. Use for features, proof, logos, or "3 reasons" structures.
- `comparison_split`: two equal panels compare before/after, old/new, problem/solution, or two capabilities. Use only when there are exactly two primary sides.
- `kinetic_type_beats`: text carries the shot. Use for hook, pain, claim, or punchy copy where words change on beats.
- `cta_morph_press`: final action. Use for offer, price, brand lockup, "now buy/book/start", or CTA button close.

## Do Not

- Do not choose `seedance_2_video`, `text_to_video`, `image_to_video`, or any video model when the requested route is template-only.
- Do not write raw HyperFrames HTML in RenderPlan fields.
- Do not add complex real-world actions, people, lip sync, physics, or camera moves that require video generation.
- Do not make one scene hold all selling points if separate shots would improve pacing.
- Do not use template video for phases other than `shot_video`.

## Tool Protocol

1. `read_project_memory` if product facts, route policy, style, or blueprint choice are incomplete.
2. Create one RenderPlan with:
   - `model_prompt_profile: template_video`
   - `operation: image_to_template_video` or `template_to_video`
   - `target_phase: shot_video`
   - `params.template_key`
   - blueprint-specific `variables`
3. Call `upsert_render_plan` once. Prefer `mode: create` unless repairing an existing plan.

## Quality Bar

- The compiled request can route to `internal_template_video/hyperframes-html` without fallback guessing.
- `template_key` and variables are sufficient for TemplateVideoProvider to render without LLM-authored HTML.
- Product image, text safe area, scene order, and CTA are clear.
- The plan explicitly preserves `seedance_policy` when Seedance is forbidden.
