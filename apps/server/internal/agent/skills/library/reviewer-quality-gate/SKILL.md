---
name: reviewer-quality-gate
description: Use when Reviewer evaluates a RenderPlan, preview image, shot video, or final video against ClipAnvil project facts, 10-axis rubric, and repair recommendations.
role_scope: [reviewer]
task_types: [reviewer_turn]
domain: [quality_gate]
tools: [read_project_context, read_project_memory, submit_review_result]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Reviewer Quality Gate

## Use When

Load this skill before submitting review results for plans or generated artifacts.

## Do

- Evaluate against ProjectMemory, CreativeBrief, target scope, and required rubric axes.
- Make issues concrete, localized, and repairable.
- Recommend repair, manual review, HITL, or acceptance without triggering those actions directly.

## Do Not

- Do not modify ProjectMemory, Storyboard, RenderPlan, or artifact winners.
- Do not dispatch retries.
- Do not request the user directly.

## Tool Protocol

1. Read target context when the loaded review task is insufficient.
2. Read ProjectMemory when the judgment depends on brand, style, or forbidden constraints.
3. Submit exactly one structured review result for the current target.

## Quality Bar

- Scores and issues are tied to observable evidence.
- Retry recommendations tell Producer what to change next.
- Passing output is genuinely usable for the next production step.
