---
name: composer-timeline-director
description: Use when Composer creates or repairs a final video TimelinePlan from approved shot videos, AudioPlan, voiceover, BGM, subtitles, and platform export requirements.
role_scope: [composer]
task_types: [composer_turn]
domain: [final_video]
tools: [get_composition_context, stage_media_inputs, probe_media, create_timeline_plan, update_timeline_plan_status, render_timeline_template, run_ffmpeg_command, submit_composition_artifact]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Composer Timeline Director

## Use When

Load this skill before building or repairing a final video TimelinePlan.

## Do

- Treat approved AudioPlan as read-only production intent.
- When AudioPlan includes `cue_plan`, order visual segments by cue `shot_ref`, scale cue windows to the generated voiceover duration when available, and use cue captions as the only final subtitle source unless a richer alignment track exists.
- Stage media before probing or rendering, and probe only the workspace_path values returned by the staging manifest.
- Keep narration clear above BGM and use AAC audio for final MP4 outputs.
- Report blocked status when required media is missing or unusable.

## Do Not

- Do not rewrite Storyboard or AudioPlan.
- Do not duplicate subtitles already baked into motion shots; prefer one Composer-owned caption layer aligned to AudioPlan or voiceover alignment.
- Do not invent output storage URLs or node ids.
- Do not silently drop approved audio or shot winners.

## Tool Protocol

1. Get composition context and verify required inputs.
2. Stage media inputs before probing or rendering.
3. Probe inputs whose duration, dimensions, stream, or codec affects the timeline, using staged manifest paths only.
4. Create or update TimelinePlan.
5. Prefer render_timeline_template; use run_ffmpeg_command only when the template cannot express the timeline.
6. Submit the final artifact only after a valid rendered output path exists.

## Quality Bar

- TimelinePlan reflects the approved shot order and audio intent.
- Missing or unusable inputs produce blocked status with a clear next action.
- Final artifact submission includes durable timeline and sandbox references.
