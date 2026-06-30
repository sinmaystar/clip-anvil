---
name: renderplan-repair-craftsman
description: Use when Craftsman repairs or forks a RenderPlan after Reviewer issues, failed generation, repeated artifacts problems, or Producer revision instructions.
role_scope: [craftsman]
task_types: [craftsman_turn]
domain: [repair]
tools: [read_project_memory, upsert_render_plan]
source:
  kind: clipanvil-local
version: 0.1.0
---
# RenderPlan Repair Craftsman

## Use When

Load this skill when the task is repair, revise, regenerate planning, or fork_from after Reviewer feedback or failed output.

## Do

- Identify the smallest change that addresses the issue.
- Preserve constraints and successful parts of the prior plan.
- Prefer fork_from when repairing an already submitted or executed plan.
- Explain the repair rationale for Producer and Reviewer.
- Preserve or correct operation, output_type, model_prompt_profile, subject_bindings, reference strategy, and risk notes instead of hiding them in generic prompt text.

## Do Not

- Do not rewrite the whole plan just because one issue exists.
- Do not ignore repeated failure; mark the need for Producer decision when simple prompt repair is exhausted.
- Do not directly trigger generation.

## Tool Protocol

1. Read ProjectMemory if the issue conflicts with global constraints.
2. Map each Reviewer issue to prompt, reference, subject, params, or blocker changes.
3. Call upsert_render_plan with a repaired plan or a blocked rationale.

## Quality Bar

- The repair targets the actual failure instead of adding generic quality words.
- The new plan remains compatible with the original shot or audio intent.
- Repeated failures escalate rather than loop.
- The repaired RenderPlan is more auditable than the failed one.
