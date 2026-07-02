---
name: motion-shot-craftsman
description: Use when Craftsman creates or repairs a Remotion-backed motion_shot_video RenderPlan for product cards, benefit grids, comparisons, CTA shots, or no-Seedance video fallback.
role_scope: [craftsman]
task_types: [craftsman_turn]
domain: [commerce_ad, motion_shot_video, render_plan]
tools: [read_project_memory, upsert_render_plan]
source:
  kind: clipanvil-local
  inspired_by: remotion composition patterns, commerce product motion
version: 0.1.0
---
# Motion Shot Craftsman

## Use When

Load this skill before creating or repairing a RenderPlan whose video must be rendered by `internal_motion_video/remotion-motion-shot-v1` instead of Seedance.

## Do

- Read ProjectMemory when route policy, product identity, text copy, visual style, or forbidden providers are not already in the task context.
- Use `model_prompt_profile: motion_shot_video`.
- Use `operation: image_to_motion_video`.
- Set `target_phase: shot_video`.
- Bind input media through `reference_bindings` or inherited `input_node_refs`; do not invent local file paths.
- Put controlled motion instructions in `params`: `duration_sec`, `ratio`, `resolution`, `fps`, `motion_style`, `safe_area`, `visual_layers`, `text_layers`, `transitions`, and `brand_colors`.
- Keep `text_layers` short. Use them for hook, benefit, label, or CTA text; full voiceover subtitles belong to Composer/final captioning.
- Write `generation_text` as a compact shot contract: product role, image source, copy hierarchy, visual layer order, motion level, and forbidden video models.
- Include audit hints that the video route is motion-only and Seedance is not allowed when the user requested no-Seedance.

## Do Not

- Do not choose `seedance_2_video`, `text_to_video`, or any video generation model when the requested route is motion-only.
- Do not write raw Remotion/React code in RenderPlan fields.
- Do not add complex real-world actions, people, lip sync, physics, or camera moves that require video generation.
- Do not bake voiceover audio or full subtitle tracks into motion-shot params.
- Do not use motion shot for phases other than `shot_video`.

## Tool Protocol

1. `read_project_memory` if product facts, route policy, style, or asset choice are incomplete.
2. Create one RenderPlan with:
   - `model_prompt_profile: motion_shot_video`
   - `operation: image_to_motion_video`
   - `target_phase: shot_video`
   - product image binding through `reference_bindings` or inherited task inputs
   - concise `text_layers` and `visual_layers`
3. Call `upsert_render_plan` once. Prefer `mode: create` unless repairing an existing plan.

## Quality Bar

- The compiled request can route to `internal_motion_video/remotion-motion-shot-v1` without fallback guessing.
- Product image, text safe area, shot duration, motion style, and CTA are clear.
- The plan explicitly preserves `seedance_policy` when Seedance is forbidden.
- Composer can later align voiceover, subtitles, BGM, and shot timing without fighting baked-in audio assumptions.
