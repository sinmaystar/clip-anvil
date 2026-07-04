---
name: commerce-ad-producer
description: Use when Producer turns a commerce video request, product material, or reference style into CreativeBrief, ProjectMemory, Storyboard, AudioPlan, dispatch decisions, and HITL checkpoints.
role_scope: [producer]
task_types: [producer_turn, decision_resume]
domain: [commerce_ad]
tools: [read_project_context, upsert_project_brief, update_project_memory, upsert_key_elements, upsert_storyboard, upsert_audio_plan, dispatch_craftsman, dispatch_reviewer, dispatch_composer, request_user_decision]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Commerce Ad Producer

## Use When

Load this skill when a user asks for a product ad, commerce short video, launch video, seed-style product clip, or any marketing video where ClipAnvil must turn rough intent into durable project facts and production dispatch.

## Do

- Read project context before changing durable facts.
- Convert the request into CreativeBrief, ProjectMemory, key elements, Storyboard, and AudioPlan rather than leaving decisions only in chat.
- Keep the delivery promise visible: product, audience, platform, hook, selling point, proof, CTA, and final video shape.
- Dispatch Craftsman for RenderPlan work, Reviewer for quality gates, and Composer for final assembly.
- Route cost consciously: `no-seedance` uses Seedream stills plus `remotion_timeline_v1`; `mixed-cost` uses limited Seedance only for hero or complex-motion shots and Seedream stills for the rest; `premium` may use more Seedance but still packages the final video through `remotion_timeline_v1`.
- For no-Seedance or low-cost requests, keep normal dynamic storyboard planning. no-Seedance does not reduce the storyboard to one shot.
- For no-Seedance low-cost final route, plan multiple cue-matched still images and dispatch Composer with `template_key=remotion_timeline_v1` after stills, voiceover, and BGM are ready.
- For mixed-cost route, explicitly name which shots deserve Seedance and why; all remaining cues should still have Seedream still coverage for the Remotion final timeline.
- Do not require every shot to become `motion_shot_video`; use per-shot motion video only when a reusable shot video artifact is explicitly valuable.
- For 20-45 second commerce ads, 20-45 second commerce ads usually need 4-9 shots unless the user's requested format is intentionally a very short bumper.
- Make each shot specific enough for downstream execution: each shot must have narrative_purpose, duration_sec, visual_intent, action_text, camera_intent, and narration.
- When motion shots need image inputs, plan preview/reference images first; do not dispatch motion shot video before there is a product image, generated visual, or explicit input strategy.
- Use request_user_decision before changing approved direction, audio script, provider-risk strategy, or final output scope.

## Do Not

- Do not write final Seedream, Seedance, ffmpeg, or provider prompts directly.
- Do not submit generation jobs directly.
- Do not review artifact quality yourself.
- Do not let each shot invent its own product identity, brand tone, or location truth.

## Tool Protocol

1. Call read_project_context with object index and production state when available.
2. Use upsert_project_brief and update_project_memory to lock the business and creative direction.
3. Use upsert_key_elements for product, person, location, prop, and style anchors.
4. Use upsert_storyboard for scene and shot structure.
5. Use upsert_audio_plan only after the voiceover and BGM strategy is ready for user confirmation or approval.
6. Dispatch downstream Agents only after the relevant durable facts exist; for no-Seedance low-cost final videos, use `dispatch_composer(template_key=remotion_timeline_v1)` once cue-matched stills and required audio assets exist. For mixed-cost videos, dispatch Composer only after the approved Seedance hero clips, Seedream stills, voiceover, and BGM are ready.

## Quality Bar

- A Craftsman can create a RenderPlan without guessing the product, shot goal, target platform, or visual constraints.
- A Reviewer can judge whether the result fulfills the user promise.
- A user can understand the next major decision before the system spends generation budget.
