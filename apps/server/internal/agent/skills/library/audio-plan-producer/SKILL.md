---
name: audio-plan-producer
description: Use when Producer creates, revises, confirms, or dispatches a whole-video AudioPlan for marketing voiceover, BGM, cue timing, and Composer handoff.
role_scope: [producer]
task_types: [producer_turn, decision_resume]
domain: [commerce_ad, audio_plan]
tools: [read_project_context, upsert_audio_plan, dispatch_craftsman, dispatch_composer, request_user_decision]
source:
  kind: clipanvil-local
version: 0.1.0
---
# AudioPlan Producer

## Use When

Load this skill when the project needs voiceover, BGM, audio cue planning, audio RenderPlan dispatch, or final video handoff involving AudioPlan.

## Do

- Treat AudioPlan as whole-video intent, not a per-shot side effect.
- Draft a concise voiceover script aligned to selling points and shot order.
- Define BGM mood, intensity, ducking expectation, and target duration.
- Ask the user to confirm script, voice direction, and BGM direction before approving the plan unless they explicitly requested autopilot.
- Dispatch voiceover_audio and bgm_audio as separate Craftsman tasks.

## Do Not

- Do not let each shot generate its own final narration.
- Do not approve AudioPlan silently after a major script or voice change.
- Do not ask Composer to rewrite the AudioPlan; Composer only mixes approved audio intent.

## Tool Protocol

1. Call read_project_context to inspect brief, storyboard, current AudioPlan, and artifact readiness.
2. Use upsert_audio_plan mode replace_draft or patch for drafts.
3. Use request_user_decision for confirmation.
4. Use upsert_audio_plan mode approve only after confirmation or explicit autopilot instruction.
5. Dispatch Craftsman for voiceover_audio and bgm_audio after approval.
6. Dispatch Composer only when required video and audio artifacts are ready.

## Quality Bar

- The script is short enough for the target duration and clear enough for commercial conversion.
- BGM supports the product message instead of overpowering it.
- Composer can identify voiceover and BGM requirements without asking Producer again.
