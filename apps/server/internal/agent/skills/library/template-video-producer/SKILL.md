---
name: template-video-producer
description: Use when Producer routes a commerce video toward low-cost template video, HyperFrames-backed shot planning, Seedream still assets, Volcengine audio, or no-Seedance fallback.
role_scope: [producer]
task_types: [producer_turn, decision_resume]
domain: [commerce_ad, template_video, cost_routing]
tools: [read_project_context, upsert_project_brief, update_project_memory, upsert_key_elements, upsert_storyboard, upsert_audio_plan, dispatch_craftsman, dispatch_reviewer, dispatch_composer, request_user_decision]
source:
  kind: clipanvil-local
  inspired_by: heygen-com/hyperframes skills product-launch-video hyperframes-animation
version: 0.1.0
---
# Template Video Producer

## Use When

Load this skill when the user wants a low-cost marketing video, explicitly forbids Seedance, asks for HyperFrames/template video, or accepts model-generated images and audio but not model-generated video.

## Do

- Read project context before changing durable facts or spending generation budget.
- Lock the route in ProjectMemory: `internal_template_video` for video, `template_video` RenderPlan profile, `hyperframes-html` model, Seedream allowed for still images, Volcengine audio allowed for voiceover or BGM.
- Separate the asset route from the video route: preview/reference images may use Seedream, audio may use Volcengine TTS/BGM, shot video must use template video when the user forbids Seedance.
- Pick one blueprint per template shot and store the choice in Storyboard shot intent or ProjectMemory notes:
  - `product_hero_v2`: product packshot or hero still with light camera move.
  - `benefit_grid_assemble`: selling-point cards, feature list, logo/proof wall.
  - `comparison_split`: before/after, old/new, two complementary capabilities.
  - `kinetic_type_beats`: hook, pain point, or fast copy-led selling line.
  - `cta_morph_press`: final CTA, brand lockup, action button, price or offer close.
- Keep each template shot scoped to one communication job. Split product hero, benefits, comparison, and CTA into separate shots when the final video needs richer pacing.
- Create or update AudioPlan before final composition. Composer should only be dispatched when the needed shot video and audio assets exist or the user explicitly accepts silent output.
- Dispatch Craftsman with `video_route_policy: template_only` when Seedance is forbidden.
- Dispatch Reviewer for template artifacts when text readability, product visibility, or scene timing matters.

## Do Not

- Do not dispatch Seedance or any video model when the user asks for no-Seedance, template-only, low-cost fallback, or HyperFrames-only video.
- Do not ask Craftsman to write arbitrary HyperFrames HTML. Producer chooses blueprint and constraints; TemplateVideoProvider owns renderable HTML.
- Do not treat a queued Composer task as a completed final video.
- Do not merge image generation, template video, audio, and final composition into one hidden step when the user needs cost transparency.

## Tool Protocol

1. `read_project_context` with object index, production state, and available media when route or current assets are uncertain.
2. `upsert_project_brief` and `update_project_memory` to record product promise, route policy, forbidden providers, allowed model asset types, blueprint choices, and target format.
3. `upsert_storyboard` with shots whose `shot_kind`, `creative_text`, and `visual_intent` identify the blueprint and asset needs.
4. `upsert_audio_plan` when voiceover/BGM is part of the deliverable.
5. `dispatch_craftsman` first for Seedream preview/reference images when needed, then for `shot_video` with `video_route_policy: template_only`.
6. `dispatch_reviewer` before accepting template video if it is a user-visible shot or final fallback.
7. `dispatch_composer` only after successful template video and required audio assets exist.

## Quality Bar

- The route is auditable in durable facts: no one has to infer whether Seedance is allowed.
- Each template shot has a named blueprint and an explicit input asset strategy.
- Craftsman can write a valid `template_video` RenderPlan without inventing product facts, provider choice, or template variables.
- Reviewer can evaluate `scene_timing`, `text_safety`, product visibility, and Seedance policy compliance from project facts.
