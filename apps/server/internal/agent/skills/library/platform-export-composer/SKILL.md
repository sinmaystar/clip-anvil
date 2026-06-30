---
name: platform-export-composer
description: Use when Composer prepares final video output for target platform shape, aspect ratio, duration, codec, bitrate, subtitle safety, and delivery constraints.
role_scope: [composer]
task_types: [composer_turn]
domain: [final_video, platform_export]
tools: [get_composition_context, stage_media_inputs, probe_media, create_timeline_plan, update_timeline_plan_status, render_timeline_template, run_ffmpeg_command, submit_composition_artifact]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Platform Export Composer

## Use When

Load this skill when final output must fit TikTok, Douyin, Xiaohongshu, YouTube Shorts, internal preview, or another platform profile.

## Do

- Preserve target aspect ratio and avoid unsafe text or product cropping.
- Keep duration and pacing aligned with the brief and available clips.
- Prefer stable codec and container choices compatible with preview and download.
- Report blocked if source clips cannot satisfy required export shape without bad cropping.

## Do Not

- Do not invent platform requirements missing from context.
- Do not stretch product footage unnaturally to fit an aspect ratio.
- Do not use unsupported render runtimes.

## Tool Protocol

1. Read composition context for platform, aspect ratio, and final-output instructions.
2. Stage inputs and probe only concrete staged workspace_path values, never directories or guessed filenames.
3. Encode platform constraints in TimelinePlan.
4. Prefer render_timeline_template; use raw ffmpeg only for a documented fallback.
5. Submit final artifact with the resulting metadata.

## Quality Bar

- Final video is shaped for the target platform without hiding the product.
- Text, subtitles, and CTA remain inside safe visual areas.
- Export choices are compatible with current ClipAnvil playback and persistence.
