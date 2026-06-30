---
name: reference-video-analysis-producer
description: Use when Producer needs to extract reusable style, pacing, shot language, selling structure, or constraints from a user-provided reference video without copying it frame by frame.
role_scope: [producer]
task_types: [producer_turn, decision_resume]
domain: [commerce_ad, reference_video]
tools: [read_project_context, analyze_reference_video, update_project_memory, upsert_key_elements, upsert_storyboard, request_user_decision]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Reference Video Analysis Producer

## Use When

Load this skill when the user uploads or mentions a reference video and asks ClipAnvil to follow its feeling, rhythm, layout, selling structure, camera language, or platform style.

## Do

- Separate reusable production language from literal copying.
- Extract pacing, hook structure, product reveal timing, shot density, camera rhythm, color mood, subtitle style, and audio role.
- Convert stable reusable elements into ProjectMemory and Storyboard constraints.
- Ask for user confirmation if the reference conflicts with the user's product, budget, platform, or available assets.

## Do Not

- Do not promise exact frame-level replication.
- Do not copy protected characters, logos, creator identity, or distinctive creative expression as project facts.
- Do not ask Craftsman to recreate unavailable shots without product anchors or reference assets.

## Tool Protocol

1. Call read_project_context to inspect uploaded media and current facts.
2. Call analyze_reference_video with the user's adaptation goal before writing ProjectMemory or Storyboard from the reference video.
3. Use update_project_memory for concise reference-derived style rules and forbidden copying boundaries.
4. Use upsert_key_elements when the reference establishes reusable style anchors.
5. Use upsert_storyboard to map the reference rhythm into ClipAnvil scenes and shots.
6. Use request_user_decision when there are multiple viable adaptation strategies.

## Quality Bar

- The adaptation explains what to preserve, what to ignore, and what must be original.
- The storyboard can use the reference as style guidance without depending on inaccessible footage.
- The user can see the difference between "inspired by" and "copied from."
