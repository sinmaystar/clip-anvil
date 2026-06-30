---
name: seedream-renderplan-craftsman
description: Use when Craftsman creates or repairs an image RenderPlan for Seedream-style reference image, preview image, product view, or visual anchor generation.
role_scope: [craftsman]
task_types: [craftsman_turn]
domain: [commerce_ad, image_generation]
tools: [read_project_memory, upsert_render_plan]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Seedream RenderPlan Craftsman

## Use When

Load this skill before creating reference_image, preview_image, or image variant RenderPlans.

## Do

- Preserve product identity, material, color, logo constraints, and key element state.
- Describe subject, setting, composition, lighting, style, quality, and negative hints in generation_text.
- Use reference bindings only for real input assets or upstream winners.
- Make images useful as video anchors when they are meant to feed shot_video.
- Make operation, output_type, model_prompt_profile, subject_bindings, reference strategy, and risk notes clear when they are needed for execution or review.

## Do Not

- Do not invent product facts missing from ProjectMemory.
- Do not mix multiple unrelated visual goals into one image plan.
- Do not create video strategy inside an image RenderPlan.

## Tool Protocol

1. Read ProjectMemory if product or brand constraints are not fully present.
2. Write one image-focused generation_text with clear subject and composition.
3. Call upsert_render_plan with preview_image or reference_image intent as scoped by the task.

## Quality Bar

- The image plan is concrete enough to generate a stable visual anchor.
- Product visibility and brand style are clear.
- Downstream Seedance video planning can reuse the image without reinterpreting the scene.
- Reviewer can audit subject consistency, reference strategy, and model_prompt_profile without guessing.
