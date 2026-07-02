---
name: motion-shot-producer
description: Use when Producer routes a commerce video toward low-cost Remotion motion shots, Seedream still assets, Volcengine audio, or no-Seedance video fallback.
role_scope: [producer]
task_types: [producer_turn, decision_resume]
domain: [commerce_ad, motion_shot_video, cost_routing]
tools: [read_project_context, upsert_project_brief, update_project_memory, upsert_key_elements, upsert_storyboard, upsert_audio_plan, dispatch_craftsman, dispatch_reviewer, dispatch_composer, request_user_decision]
source:
  kind: clipanvil-local
  inspired_by: remotion compositions, commerce storyboard planning
version: 0.1.0
---
# Motion Shot Producer

## Use When

Load this skill when the user wants a low-cost marketing video, explicitly forbids Seedance, asks for Remotion/image-driven motion, or accepts model-generated images and audio but not model-generated video.

## Do

- Read project context before changing durable facts or spending generation budget.
- Lock the route in ProjectMemory: `internal_motion_video` for video, `motion_shot_video` RenderPlan profile, `remotion-motion-shot-v1` model, Seedream allowed for still images, Volcengine audio allowed for voiceover or BGM.
- Separate the asset route from the video route: preview/reference images may use Seedream, audio may use Volcengine TTS/BGM, shot video must use motion shots when the user forbids Seedance.
- Keep motion shots scoped to one communication job: product hero, benefit, comparison, and CTA should usually be separate shots for better pacing.
- Create or update AudioPlan before final composition when voiceover/BGM is part of the deliverable. Composer owns captions, audio mixing, and final sync.
- Dispatch Craftsman with `video_route_policy: motion_only` when Seedance is forbidden.
- Dispatch Reviewer for motion-shot RenderPlans or artifacts when text readability, product visibility, motion rhythm, or route compliance matters.

## Do Not

- Do not dispatch Seedance or any generative video model when the user asks for no-Seedance or motion-only video.
- Do not ask Craftsman to write arbitrary Remotion code. Craftsman writes structured RenderPlan fields; the provider owns renderable Remotion components.
- Do not treat a queued Composer task as a completed final video.
- Do not hide image generation, motion shot, audio, and final composition behind one opaque step when the user needs cost transparency.

## Tool Protocol

1. `read_project_context` with object index, production state, and available media when route or current assets are uncertain.
2. `upsert_project_brief` and `update_project_memory` to record product promise, route policy, forbidden providers, allowed model asset types, motion-shot style, and target format.
3. `upsert_storyboard` with shots whose `shot_kind`, `creative_text`, and `visual_intent` identify the product image, short on-screen copy, and motion intent.
4. `upsert_audio_plan` when voiceover/BGM is part of the deliverable.
5. `dispatch_craftsman` first for Seedream preview/reference images when needed, then for `shot_video` with `video_route_policy: motion_only` when no Seedance is allowed.
6. `dispatch_reviewer` before accepting motion-shot video if it is a user-visible shot or fallback.
7. `dispatch_composer` only after successful shot videos and required audio assets exist.

## Quality Bar

- The route is auditable in durable facts: no one has to infer whether Seedance is allowed.
- Each motion shot has an explicit input asset strategy, short copy role, and motion role.
- Craftsman can write a valid `motion_shot_video` RenderPlan without inventing product facts, provider choice, or local file paths.
- Reviewer can evaluate readability, product visibility, motion rhythm, and Seedance policy compliance from project facts.
