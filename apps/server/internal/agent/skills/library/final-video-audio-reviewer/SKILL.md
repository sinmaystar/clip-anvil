---
name: final-video-audio-reviewer
description: Use when Reviewer evaluates final video audio, voiceover timing, BGM ducking, mix balance, subtitle safety, rhythm, and AudioPlan faithfulness.
role_scope: [reviewer]
task_types: [reviewer_turn]
domain: [final_video, audio_plan]
tools: [read_project_context, read_project_memory, submit_review_result]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Final Video Audio Reviewer

## Use When

Load this skill for final_video_review when voiceover, BGM, timeline mix, or audio_sync is part of the acceptance decision.

## Do

- Check that voiceover and BGM exist when AudioPlan requires them.
- Judge narration intelligibility, BGM volume, ducking, fades, and timing against picture.
- Check that audio supports platform_selling_power rather than distracting from it.
- Submit the complete final_video_review required axes: faithfulness, brand_style_consistency, visual_quality, continuity, audio_sync, and platform_selling_power.
- Note missing, clipped, delayed, or overpowering audio as concrete issues.

## Do Not

- Do not modify AudioPlan or TimelinePlan.
- Do not run ffmpeg.
- Do not dispatch Composer or Craftsman.

## Tool Protocol

1. Read final video context including AudioPlan, timeline summary, and artifact facts.
2. Read ProjectMemory if brand voice or audio style constraints matter.
3. Submit review_result with the complete final_video_review required axes. Audio-specific evidence belongs in audio_sync and platform_selling_power, but the final result must also score faithfulness, brand_style_consistency, visual_quality, and continuity.

## Quality Bar

- Passing final video is watchable with clear narration and supportive music.
- Failure explains whether repair belongs to Producer, Craftsman, or Composer.
- Audio issues are actionable, not just "sounds bad."
