---
name: agent-remotion-code-composer
description: Use when Composer builds a non-template final video with agent_remotion_code_v1 by writing, validating, repairing, rendering, and submitting an Agent-authored Remotion renderer attempt.
role_scope: [composer]
task_types: [composer_turn]
domain: [final_video, remotion_code, commerce_ad]
tools: [get_composition_context, stage_media_inputs, probe_media, create_timeline_plan, update_timeline_plan_status, create_remotion_renderer_attempt, validate_remotion_renderer_attempt, render_agent_remotion_renderer, submit_composition_artifact]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Agent Remotion Code Composer

## Use When

Load this skill when the Composer task uses `template_key=agent_remotion_code_v1`, when Producer asks for a non-template final video, or when the final assembly needs Agent-authored Remotion layout and motion beyond `remotion_timeline_v1`.

## Do

- Use `agent_remotion_code_v1` only for final video packaging from approved assets, voiceover, BGM, captions, and route policy.
- Call `get_composition_context` first, then stage all selected visual and audio assets before writing renderer code.
- Use staged `/workspace` paths and props data returned by tools; do not invent storage URLs, node ids, or missing assets.
- Create the TimelinePlan with `template_key=agent_remotion_code_v1` and record route rationale, fallback policy, renderer artifact id, attempt id, sandbox job id, and final output path in timeline result.
- Create renderer code through `create_remotion_renderer_attempt`; code lives in the sandbox attempt workspace and durable source snapshots live in DB.
- Use `read_file` and `edit_file` when available in the Composer tool list to inspect and patch sandbox files between attempts.
- In Remotion code, reference staged media through Remotion public assets: convert `/workspace/input/foo.jpg` to `staticFile('input/foo.jpg')`, or `/workspace/input/foo.mp4` to `staticFile('input/foo.mp4')`. Do not pass raw `/workspace/input/...` as `<Img>`, `<Video>`, or `<Audio>` `src`.
- When repairing an existing renderer, pass the original `renderer_artifact_id`, increment `attempt_no`, and set `repair_from_attempt_id`; do not create a second renderer artifact for the same timeline.
- Run `validate_remotion_renderer_attempt` after each attempt and require validate passed before render.
- Use `render_agent_remotion_renderer` only after validation passes, then call `submit_composition_artifact` with the returned output path and sandbox job id.
- Prefer small repair attempts: patch the sandbox file that failed validation or render instead of rewriting the whole renderer unless the structure is wrong.
- fallback to `remotion_timeline_v1` when validation, render, platform, audio, or QA problems exceed the attempt budget; record the fallback reason.

## Do Not

- Do not use this skill for `remotion_timeline_v1`; load `remotion-timeline-composer` for fixed renderer JSON plans.
- Do not install dependencies, run npm install, call package managers, access network APIs, or fetch external URLs.
- Do not import Node builtins, use eval, dynamic import, require, fetch, XMLHttpRequest, WebSocket, or absolute host paths.
- Do not bypass validation, render an unvalidated attempt, or submit an output with no video stream.
- Do not modify repository source files; renderer code belongs in the sandbox attempt workspace.
- Do not call Seedance, Seedream, TTS, or BGM providers. Composer packages assets that already exist.
- Do not bake internal Storyboard fields such as narrative_purpose, visual_intent, action_text, or camera_intent into final captions.

## Tool Protocol

1. Call `get_composition_context` and choose real assets, AudioPlan cue text, voiceover, BGM, and platform settings.
2. Call `stage_media_inputs` for every selected input and `probe_media` when dimensions, duration, stream presence, or codec affects the renderer.
3. Call `create_timeline_plan` with `template_key=agent_remotion_code_v1`; include route rationale and fallback policy in plan or render_settings.
4. Call `create_remotion_renderer_attempt` with `GeneratedComposition.tsx`, any local helper files, and props containing output width, height, fps, duration_sec, staged asset paths, captions, and audio tracks.
5. In `GeneratedComposition.tsx`, import `staticFile` from `remotion` and use public-relative asset paths such as `staticFile('input/shot_01.jpg')`, `staticFile('input/shot_02.mp4')`, `staticFile('input/voiceover.mp3')`, and `staticFile('input/bgm.mp3')`.
6. Call `validate_remotion_renderer_attempt`. If it fails, inspect the reported file and line, patch the sandbox files, create the next attempt number with the same `renderer_artifact_id`, and validate again.
7. After validation passes, call `render_agent_remotion_renderer`.
8. Call `update_timeline_plan_status` with `result_for_timeline_plan` when the render result should be merged into timeline result.
9. Call `submit_composition_artifact` only after a valid `/workspace/output/*.mp4` exists.
10. If repeated repair fails, call `update_timeline_plan_status` as blocked or fallback to `remotion_timeline_v1` with a clear reason.

## Quality Bar

- The route rationale explains why `agent_remotion_code_v1` is better than the fixed renderer for this request.
- The renderer attempt is auditable: source_hash, props_hash, validation_result, compile_result, render_result, and sandbox_job_id are present.
- Final video has visible product/story content, voiceover when required, BGM when required, readable captions, nonblank frames, and platform-safe layout.
- Mixed-cost outputs identify which existing Seedance clips were used and which cues are Seedream stills animated by Remotion.
- Fallback is explicit and user-understandable, not a silent switch to the fixed template route.
