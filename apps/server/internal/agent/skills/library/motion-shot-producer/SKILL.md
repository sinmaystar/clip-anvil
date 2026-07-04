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
- Lock the route in ProjectMemory: Seedream allowed for still images, Volcengine audio allowed for voiceover or BGM, and `remotion_timeline_v1` as the preferred final Composer route when the user forbids Seedance.
- Separate the asset route from the final video route: preview/reference images may use Seedream, audio may use Volcengine TTS/BGM, and the final video should usually be assembled by Remotion timeline rather than per-shot generated video.
- This skill must be paired with commerce-ad-producer. Commerce ad structure still comes from CreativeBrief, ProjectMemory, KeyElement, Storyboard, and AudioPlan.
- Route policy only: do not create a fixed storyboard, do not replace dynamic shot planning, and do not choose a canned 30 second template.
- Keep motion shots scoped to one communication job: product hero, benefit, comparison, and CTA should usually be separate shots for better pacing.
- Create or update AudioPlan before final composition when voiceover/BGM is part of the deliverable. Composer owns captions, audio mixing, and final sync.
- For voiceover-led motion videos, make AudioPlan cue timing the sync contract: each cue must name the matching `shot_ref`, selling point, short caption, and intended cue window before Composer assembles the final timeline.
- Size the voiceover script to the target duration. For a 30-35s Chinese commerce ad, write a real 140-180 Chinese-character narration unless the user asks for a sparse music-led video; do not stretch a 10-15s script over a 30s timeline.
- Align every claim in the cue to a visual asset strategy. If the cue says wheels, the shot should request a wheel close-up; if it says storage, the shot should request an open-interior/storage still; do not reuse the same full-product hero for every benefit.
- Put the expected still-image intent into `visual_intent` and `creative_text` so Craftsman can generate different Seedream assets before Remotion motion.
- Dispatch Craftsman with `video_route_policy: motion_only` only when a per-shot `motion_shot_video` artifact is explicitly needed.
- For no-Seedance requests, first generate cue-matched still images for each selling point, then dispatch Composer with `template_key=remotion_timeline_v1`.
- Preserve real shot_refs from the dynamic storyboard; do not dispatch only one synthetic shot unless the dynamic storyboard truly has one shot.
- Dispatch Reviewer for still images, motion-shot RenderPlans, final timeline plans, or final artifacts when text readability, product visibility, motion rhythm, audio sync, or route compliance matters.

## Do Not

- Do not dispatch Seedance or any generative video model when the user asks for no-Seedance or motion-only video.
- Do not ask Craftsman to write arbitrary Remotion code. Craftsman writes structured RenderPlan fields; the provider owns renderable Remotion components.
- Do not treat a queued Composer task as a completed final video.
- Do not hide image generation, motion shot, audio, and final composition behind one opaque step when the user needs cost transparency.
- Do not turn a multi-shot request into a single internal motion card.
- Do not require every shot to become `motion_shot_video`; Remotion final timeline can animate stills, captions, transitions, voiceover, and BGM.

## Tool Protocol

1. `read_project_context` with object index, production state, and available media when route or current assets are uncertain.
2. `upsert_project_brief` and `update_project_memory` to record product promise, route policy, forbidden providers, allowed model asset types, motion-shot style, and target format.
3. `upsert_storyboard` with shots whose `shot_kind`, `creative_text`, and `visual_intent` identify the product image, short on-screen copy, and motion intent.
4. `upsert_audio_plan` when voiceover/BGM is part of the deliverable; align `cue_plan` to real storyboard `shot_ref` values before dispatching final shot videos.
5. `dispatch_craftsman` first for Seedream preview/reference images when needed, then dispatch `voiceover_audio`/`bgm_audio` for the approved AudioPlan when required, then dispatch Composer with `template_key=remotion_timeline_v1` for the final video. Dispatch `shot_video` with `video_route_policy: motion_only` only when a separate per-shot motion artifact is useful.
6. `dispatch_reviewer` before accepting motion-shot video if it is a user-visible shot or fallback.
7. `dispatch_composer` only after successful shot videos and required audio assets exist.

## Quality Bar

- The route is auditable in durable facts: no one has to infer whether Seedance is allowed.
- Each still or optional motion shot has an explicit input asset strategy, short copy role, and motion role.
- Cue text, shot_ref, and visual_intent agree on the same feature or scene, so Composer does not show one benefit while the voiceover says another.
- Craftsman can write a valid `motion_shot_video` RenderPlan without inventing product facts, provider choice, or local file paths.
- Reviewer can evaluate readability, product visibility, motion rhythm, and Seedance policy compliance from project facts.
