---
name: audio-renderplan-craftsman
description: Use when Craftsman creates voiceover_audio or bgm_audio RenderPlans from an approved AudioPlan for generated narration or generated BGM.
role_scope: [craftsman]
task_types: [craftsman_turn]
domain: [audio_plan, audio_generation]
tools: [read_project_memory, upsert_render_plan]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Audio RenderPlan Craftsman

## Use When

Load this skill before creating a voiceover_audio or bgm_audio RenderPlan.

## Do

- Treat AudioPlan as approved input and do not rewrite its script.
- Create separate RenderPlans for voiceover and BGM.
- Keep generation_text short, including target duration, language, voice or BGM style, cue intent, and avoidances.
- Leave provider speaker IDs empty unless the task context gives a real ID.
- Make operation=text_to_audio, output_type=audio, model_prompt_profile=seed_audio_1, and risk notes explicit when the current task context does not already provide them.

## Do Not

- Do not combine voiceover and BGM in one RenderPlan.
- Do not use uploaded music, stock music, or video model audio as the first-version BGM path.
- Do not approve or modify AudioPlan.

## Tool Protocol

1. Read ProjectMemory only if brand tone or forbidden audio style is unclear.
2. For voiceover_audio, base generation_text on voiceover_script, voice_profile, and cue plan.
3. For bgm_audio, base generation_text on bgm_plan and whole-video duration.
4. Call upsert_render_plan once for the scoped audio phase.

## Quality Bar

- The plan can generate an audio artifact without Composer guessing the intent.
- Voiceover remains intelligible and aligned to the marketing promise.
- BGM supports pacing and does not compete with narration.
- Producer and Reviewer can see the audio operation, output_type, model_prompt_profile, and generation risks.
