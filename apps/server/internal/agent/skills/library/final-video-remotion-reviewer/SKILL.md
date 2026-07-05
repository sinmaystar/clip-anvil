---
name: final-video-remotion-reviewer
description: Use when Reviewer evaluates a final video rendered by remotion_timeline_v1 or agent_remotion_code_v1 for layout quality, caption lane safety, cue/asset sync, renderer safety, no-Seedance compliance, mixed-cost Seedance usage, audio presence, and marketing rhythm.
role_scope: [reviewer]
task_types: [reviewer_turn]
domain: [final_video, remotion_timeline, commerce_ad]
tools: [read_project_context, read_project_memory, submit_review_result]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Final Video Remotion Reviewer

## Use When

Load this skill for final_video_review when the final timeline uses `remotion_timeline_v1`, when the final video comes from an Agent-authored renderer using `agent_remotion_code_v1`, when Producer requested a low-cost no-Seedance route, when Producer requested mixed-cost, or when the output uses Seedream stills, existing clips, plus Remotion layout/motion.

## Do

- Verify there is one Composer-owned caption lane. Reject double subtitles, overlapping subtitles, or captions placed over core product text.
- Verify captions come from AudioPlan cue text, voiceover alignment, TTS alignment, or explicit manual captions.
- Reject internal planning text such as `narrative_purpose`, `visual_intent`, `action_text`, `camera_intent`, "短途出行痛点钩子", or "前三秒抓住".
- Verify cue/asset sync. Wheel cues need wheel/detail assets. Storage cues need open/interior/storage assets. CTA cues need packshot, brand, or CTA imagery.
- Verify layout diversity. Same layout should not repeat more than twice in a row. Same still should not cover most segments unless the context says assets are insufficient.
- Verify no-Seedance compliance when the user forbids Seedance. Seedream stills, Volcengine audio, and Remotion rendering are allowed; Seedance video generation is not.
- For mixed-cost or premium routes, summarize Seedance video segment count, Remotion still segment count, which shots used high-cost video, and whether this matches Producer's approved route.
- Flag cost_risk when Seedance clips are used for ordinary still-suitable selling points without Producer approval.
- Verify final video has voiceover when AudioPlan requires voiceover and BGM when AudioPlan requires BGM.
- For Agent-authored renderer outputs, read renderer artifact and renderer attempt facts: source_hash, props_hash, validation_result, compile_result, render_result, sandbox_job_id, and accepted attempt number.
- Check that dynamic renderer output matches route policy, especially no-Seedance and mixed-cost limits.
- Flag unsafe_renderer_code when renderer facts show forbidden imports, network access, eval, dynamic import, external URLs, or validation bypass.
- Use validation_failed, compile_failed, blank_frame, and fallback_required issue categories when the dynamic renderer failed validation, failed render, produced empty frames, or should fall back to `remotion_timeline_v1`.
- State whether the reviewed final video came from the fixed timeline renderer or an Agent-authored renderer.

## Do Not

- Do not accept a silent final video when approved audio exists.
- Do not accept captions sourced from internal director notes.
- Do not accept wheel narration over storage-only imagery or storage narration over wheel-only imagery.
- Do not treat Remotion still animation as Seedance video generation.
- Do not ignore Seedance overuse just because the final video looks acceptable.

## Tool Protocol

1. Read final video context, timeline plan summary, AudioPlan, render jobs, generation jobs, and available artifact facts.
2. For `agent_remotion_code_v1`, read renderer artifact, renderer attempt, validation_result, compile_result, render_result, source_hash, and props_hash before judging acceptance.
3. Read ProjectMemory if brand, product, route policy, or platform constraints are unclear.
4. Submit one review result. Use `audio_sync`, `continuity`, `faithfulness`, `visual_quality`, and `platform_selling_power` for required axes.
5. For blocking issues, write concrete fix hints that Producer/Composer can act on.

## Quality Bar

- The review result explicitly mentions caption lane safety, cue/asset sync, audio presence, layout repetition, no-Seedance compliance, and mixed-cost Seedance usage when applicable.
- For Agent-authored renderer outputs, the review result explicitly mentions renderer artifact, renderer attempt, validation and render facts, and whether fallback is required.
- Blocking issues point to the segment, cue, or asset class that needs repair.
