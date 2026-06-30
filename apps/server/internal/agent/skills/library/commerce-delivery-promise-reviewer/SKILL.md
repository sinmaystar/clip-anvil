---
name: commerce-delivery-promise-reviewer
description: Use when Reviewer checks whether a preview, shot video, or final video fulfills the commerce promise, product selling point, audience, platform, and CTA.
role_scope: [reviewer]
task_types: [reviewer_turn]
domain: [commerce_ad]
tools: [read_project_context, read_project_memory, submit_review_result]
source:
  kind: clipanvil-local
version: 0.1.0
---
# Commerce Delivery Promise Reviewer

## Use When

Load this skill when the review target must be judged for advertising usefulness, product clarity, selling power, or platform fit.

## Do

- Check that the product and key selling point are visible and understandable.
- Judge hook strength, information efficiency, audience fit, and CTA support.
- Distinguish visual beauty from commercial usefulness.

## Do Not

- Do not fail a result only because it differs from personal taste.
- Do not trigger reruns or choose winners.
- Do not rewrite the brief.

## Tool Protocol

1. Read project context for CreativeBrief, shot, artifact, or final output facts.
2. Read ProjectMemory when selling promise or brand constraints are unclear.
3. Submit review_result with concrete commerce issues and recommendation.

## Quality Bar

- A passing artifact can plausibly sell the product or support the intended conversion goal.
- A failing issue names the missing promise and the likely repair path.
- Producer can decide accept, repair, HITL, or stop from the result.
