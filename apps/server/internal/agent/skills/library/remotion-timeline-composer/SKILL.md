---
name: remotion-timeline-composer
description: Use when Composer builds a final marketing video with remotion_timeline_v1 from Seedream stills, optional existing Seedance clips, shot metadata, AudioPlan cue_plan, voiceover, BGM, captions, and final layout/motion choices.
role_scope: [composer]
task_types: [composer_turn]
domain: [final_video, remotion_timeline, commerce_ad]
tools: [get_composition_context, stage_media_inputs, probe_media, create_timeline_plan, render_timeline_template, submit_composition_artifact]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Remotion Timeline Composer

## Use When

Load this skill when `template_key=remotion_timeline_v1`, when Producer requests a low-cost no-Seedance final video, or when the final video should be assembled from still images, optional existing clips, voiceover, BGM, captions, and Remotion layout/motion.

## Do

- Use `remotion_timeline_v1`.
- Treat still images as first-class visual assets in this route; still images are first-class visual assets, not emergency fallback.
- Treat existing `clip` assets as video segments only when Producer selected mixed-cost or premium, or when those clips already exist in the approved context.
- Treat AudioPlan cue_plan is the primary timing contract; AudioPlan cue_plan is the primary timing contract for segment order, duration, captions, and visual matching.
- Match every cue `shot_ref` to a staged still or clip with the same `shot_ref`.
- Use `type=image` for still assets and `type=video` for clip assets in the RemotionTimelinePlan.
- Use cue `caption`, voiceover alignment, TTS alignment, or explicit manual captions for final subtitles.
- Choose a layout, motion, transition, text layer, caption, and visual asset for each cue.
- match wheel cues to wheel/detail assets and storage cues to open-interior assets.
- Keep one Composer-owned `subtitle_bottom` caption lane inside the final Remotion timeline.
- Keep text layers outside the bottom subtitle safe area.
- Use at least four layout types for a 30 second marketing video when enough cues/assets exist.
- Avoid using the same still for most segments and avoid repeating the same layout more than twice in a row.
- Use generated voiceover as the primary audio track and BGM as supporting audio.

## Do Not

- Do not require every shot to have `motion_shot_video`.
- Do not call Seedance, request new video generation, or invent missing clip assets; Composer only packages assets returned by `get_composition_context`.
- Do not use video segments when the user explicitly forbids Seedance and only still assets are approved.
- Do not use narrative_purpose, visual_intent, action_text, or camera_intent as captions.
- Do not reuse a generic full-product still for every benefit when shot-specific stills are available.
- Do not render when a wheel cue only has storage/interior imagery, or a storage cue only has wheel/detail imagery; block and ask Producer/Craftsman for the missing still.
- Do not create raw Remotion code.
- Do not submit a silent final video when approved audio assets exist.

## Tool Protocol

1. Call `get_composition_context`.
2. Verify `audio_plan.cue_plan`, voiceover, BGM, and cue-matched still or clip assets.
3. Call `stage_media_inputs` for all selected visual and audio assets.
4. Create `RemotionTimelinePlan` with `schema=clipanvil.remotion_timeline.v1` and `composition=MarketingTimeline`.
5. Call `create_timeline_plan` with `template_key=remotion_timeline_v1`.
6. Call `render_timeline_template` with the same plan.
7. Call `submit_composition_artifact` after receiving a valid `/workspace/output/*.mp4`.

## Quality Bar

- Segment order follows cue_plan order.
- Segment timing follows cue windows scaled to the actual voiceover duration.
- Caption text is user-facing marketing copy, not internal director notes.
- Caption lane is single and readable, without overlap with headline or CTA text.
- Wheel cue uses wheel/detail imagery and storage cue uses open/interior/storage imagery.
- The final plan uses real staged `/workspace` paths for every image, video, and audio asset.
- The rendered final video has video, voiceover, BGM, and readable captions.
