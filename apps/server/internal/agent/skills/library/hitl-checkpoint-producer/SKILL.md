---
name: hitl-checkpoint-producer
description: Use when Producer must decide whether to continue automatically, ask the user, or block before a major creative, cost, provider, audio, storyboard, or final-output choice.
role_scope: [producer]
task_types: [producer_turn, decision_resume]
domain: [hitl]
tools: [read_project_context, request_user_decision, update_project_memory, upsert_audio_plan, dispatch_craftsman, dispatch_composer]
source:
  kind: clipanvil-local
version: 0.1.0
---
# HITL Checkpoint Producer

## Use When

Load this skill before decisions that materially change creative direction, user-approved facts, production cost, model path, audio script, final composition, or manual-vs-automatic next steps.

## Do

- Ask the user when multiple reasonable options would produce meaningfully different videos.
- Summarize the decision in plain Chinese and provide clear options.
- Record confirmed decisions into durable facts after the user responds.
- Continue automatically only for low-risk execution details within an approved direction.

## Do Not

- Do not ask for trivial micro-decisions that the Agent can safely decide.
- Do not silently change approved direction, platform shape, script, or final output promise.
- Do not use request_user_decision as a substitute for reading project context.

## Tool Protocol

1. Call read_project_context if the current state is unclear.
2. Use request_user_decision with concise options and a recommended default when a decision is needed.
3. After resume, write the user's decision into ProjectMemory, Storyboard, AudioPlan, or dispatch choice as appropriate.
4. Dispatch downstream work only after the decision is resolved.

## Quality Bar

- The user can answer without understanding internal Agent architecture.
- The decision card explains consequences, not just labels.
- The system does not block on choices that are not actually consequential.
