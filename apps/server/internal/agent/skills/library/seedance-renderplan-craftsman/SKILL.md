---
name: seedance-renderplan-craftsman
description: Use when Craftsman creates or repairs a shot video RenderPlan for Seedance-style generation from storyboard facts, key elements, reference images, or Reviewer feedback.
role_scope: [craftsman]
task_types: [craftsman_turn]
domain: [commerce_ad, shot_video]
tools: [read_project_memory, upsert_render_plan]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Seedance RenderPlan Craftsman

## Use When

Load this skill before creating or repairing a shot video RenderPlan.

## Do

- Translate shot goals into subject, action, scene, spatial composition, camera movement, and temporal order.
- Preserve ProjectMemory constraints and explicit reference bindings.
- Use seedance_2_video only when the shot needs real dynamic generation; for selling-point cards, CTA, packshot, product-image light motion, or static fallback, write an explicit motion_shot_video RenderPlan instead.
- Prefer first-frame or first/last-frame strategy when the shot depends on generated preview images.
- When using a reference video, bind it as content_type=video_url and model_role=reference_video.
- Use reference videos only for motion, pacing, camera language, or style. Product appearance must come from user product images or approved KeyElementState references.
- In notes, explicitly state what to borrow and what must not be copied.
- Keep each shot focused on one main action and one main camera movement.
- Make the RenderPlan explicit about operation, output_type, model_prompt_profile, subject_bindings, reference strategy, and risk notes when those fields affect execution or review.

## Do Not

- Do not submit generation jobs directly.
- Do not change Storyboard or AudioPlan.
- Do not invent unavailable provider parameters.
- Do not write absolute second-by-second timing unless the task explicitly requires editorial timing.
- When task context contains video_route_policy=motion_only, must not create seedance_2_video; mark the task blocked or ask Producer to change the route.

## Tool Protocol

1. Call read_project_memory when global constraints, prompt hints, or previous issues are not in context.
2. Create a concise generation_text covering subject, action, scene, composition, camera, continuity, and avoidances.
3. Use upsert_render_plan once with the minimum structured fields needed for a valid plan.

## Quality Bar

- The RenderPlan can compile without hidden assumptions.
- Reference roles are explicit enough for PromptCompiler and Worker.
- Reviewer can tell whether the shot should pass before spending another generation attempt.
- The plan includes enough structured intent for operation, output_type, model_prompt_profile, subject consistency, and review risk to be audited.
