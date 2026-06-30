---
name: ffmpeg-audio-mix-composer
description: Use when Composer needs to mix voiceover, BGM, fades, ducking, AAC output, and duration alignment with staged video inputs through sandbox ffmpeg tools.
role_scope: [composer]
task_types: [composer_turn]
domain: [final_video, audio_mix]
tools: [get_composition_context, stage_media_inputs, probe_media, create_timeline_plan, update_timeline_plan_status, render_timeline_template, run_ffmpeg_command, submit_composition_artifact]
source:
  kind: clipanvil-local
version: 0.1.0
---
# FFmpeg Audio Mix Composer

## Use When

Load this skill when final video assembly includes voiceover, BGM, audio fades, ducking, or AAC export requirements.

## Do

- Stage and probe audio before mixing, using only workspace_path values returned by stage_media_inputs.
- Keep voiceover as the primary information track.
- Lower BGM under narration and fade in/out gently.
- Use render_timeline_template before raw ffmpeg unless fallback is needed.

## Do Not

- Do not modify voiceover script or BGM plan.
- Do not output final MP4 audio without AAC when audio tracks exist.
- Do not hide missing audio by rendering silent final output.

## Tool Protocol

1. Get context and verify AudioPlan requirements.
2. Stage video, voiceover, and BGM files.
3. Probe duration and streams.
4. Create TimelinePlan audio_tracks.
5. Prefer render_timeline_template; use controlled ffmpeg fallback only when template rendering fails for a clear reason.
6. Submit artifact only after render result is valid.

## Quality Bar

- Narration is intelligible.
- BGM supports rhythm without covering product information.
- Final video has expected video and audio streams.
