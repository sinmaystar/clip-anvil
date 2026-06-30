---
name: reference-consistency-reviewer
description: Use when Reviewer checks whether product, person, location, prop, brand style, or reference video language remains consistent with KeyElementState and ProjectMemory.
role_scope: [reviewer]
task_types: [reviewer_turn]
domain: [reference_consistency]
tools: [read_project_context, read_project_memory, submit_review_result]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Reference Consistency Reviewer

## Use When

Load this skill when the target has reference images, reference video style, key elements, first-frame chains, last-frame chains, or repeated product/person/location identity.

## Do

- Compare visible subject details against key element state and prompt constraints.
- Check product color, material, logo, proportions, and distinctive parts.
- Check continuity between chained frames or adjacent shots.
- For outputs using reference_video, judge whether motion/style was adapted without copying brand, people, subtitle copy, or distinctive expression.
- Separate acceptable stylization from identity drift.

## Do Not

- Do not demand pixel-level cloning.
- Do not ignore drift just because the output looks high quality.
- Do not repair the RenderPlan yourself.

## Tool Protocol

1. Read target context for references, key elements, and artifact metadata.
2. Read ProjectMemory when style anchors or forbidden rules are needed.
3. Submit review_result with consistency issues mapped to subject_consistency, continuity, or faithfulness.

## Quality Bar

- Important product and character identity survives across generation steps.
- Reference video influence is judged as reusable language, not literal copying.
- Issues are specific enough for Craftsman to repair bindings or prompt parts.
