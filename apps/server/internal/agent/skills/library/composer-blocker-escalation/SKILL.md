---
name: composer-blocker-escalation
description: Use when Composer cannot safely render because required media is missing, staged files fail, probe results are invalid, audio duration does not match, or TimelinePlan constraints conflict.
role_scope: [composer]
task_types: [composer_turn]
domain: [final_video, blocker]
tools: [get_composition_context, stage_media_inputs, probe_media, create_timeline_plan, update_timeline_plan_status]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Composer Blocker Escalation

## Use When

Load this skill when final composition cannot proceed safely and Composer must explain a blocked status instead of producing a bad final artifact.

## Do

- Identify the exact missing or invalid input.
- State whether Producer, Craftsman, Worker, or user action is needed.
- Preserve existing TimelinePlan facts when possible.
- Use blocked status rather than silently dropping shots, narration, or BGM.

## Do Not

- Do not render a knowingly incomplete final video as success.
- Do not modify AudioPlan or Storyboard to fit missing media.
- Do not call submit_composition_artifact when no valid output exists.

## Tool Protocol

1. Get composition context and inspect missing readiness.
2. Stage or probe only when needed to confirm the blocker.
3. Use update_timeline_plan_status or create_timeline_plan to record blocked rationale when appropriate.
4. Return a concise final explanation for Producer.

## Quality Bar

- Producer can decide the next dispatch or user decision from the blocked message.
- No approved asset is silently omitted.
- The blocker is specific enough to reproduce and fix.
