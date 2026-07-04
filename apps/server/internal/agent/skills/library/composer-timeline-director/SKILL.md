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
- Use `remotion_timeline_v1` when Producer asks for a low-cost final timeline from still images, voiceover, BGM, and captions, or a mixed-cost final timeline that combines existing Seedance clips with Seedream stills.
- When AudioPlan includes `cue_plan`, order visual segments by cue `shot_ref`, scale cue windows to the generated voiceover duration when available, and use cue captions as the only final subtitle source unless a richer alignment track exists.
- For `remotion_timeline_v1`, still image assets and existing clip assets are normal visual inputs. Do not block only because shot videos are missing when cue-matched stills exist.
- In mixed-cost timelines, use video segments only for available `clip` assets returned by `get_composition_context`; Composer must not call Seedance or invent missing clip assets.
- For `remotion_timeline_v1`, choose layout, motion, transition, text layer, caption, and visual asset per cue; use multiple layouts for multi-cue marketing videos.
- Keep one Composer-owned subtitle lane and keep other text layers out of the bottom subtitle safe area.
- Match wheel cues to wheel/detail assets and storage cues to open/interior/storage assets.
- Stage media before probing or rendering, and probe only the workspace_path values returned by the staging manifest.
- Keep narration clear above BGM and use AAC audio for final MP4 outputs.
- Report blocked status when required media is missing or unusable.

## Do Not

- Do not rewrite Storyboard or AudioPlan.
- Do not duplicate subtitles already baked into motion shots; prefer one Composer-owned caption layer aligned to AudioPlan or voiceover alignment.
- Do not invent output storage URLs or node ids.
- Do not silently drop approved audio or shot winners.
- Do not use Storyboard director notes such as `narrative_purpose`, `visual_intent`, `action_text`, or `camera_intent` as final subtitles.
- Do not render a semantically mismatched Remotion timeline, such as a wheel narration cue backed only by storage/interior imagery.

## Tool Protocol

1. Get composition context and verify required inputs.
2. Stage media inputs before probing or rendering.
3. Probe inputs whose duration, dimensions, stream, or codec affects the timeline, using staged manifest paths only.
4. Create or update TimelinePlan.
5. Prefer render_timeline_template; use run_ffmpeg_command only when the template cannot express the timeline.
6. Submit the final artifact only after a valid rendered output path exists.

## Quality Bar

- TimelinePlan reflects the approved shot order, audio intent, and cost route.
- Missing or unusable inputs produce blocked status with a clear next action.
- Final artifact submission includes durable timeline and sandbox references.
