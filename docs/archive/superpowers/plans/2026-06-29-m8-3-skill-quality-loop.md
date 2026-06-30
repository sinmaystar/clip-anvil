# M8.3 Skill Quality Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task in this session, or use `superpowers:subagent-driven-development` with a fresh worker per task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove the skill system is observable and useful: skill loads must be visible in tool trace with reason/version/hash, and a repeatable commerce-video smoke must compare behavior with and without skill loading.

**Architecture:** M8.3 should not change the skill loading contract unless verification exposes a gap. It adds trace-focused tests around existing native tool trace persistence and a deterministic smoke script/fixture that exercises `load_agent_skill` before role work. Model-quality comparisons are recorded as structured artifacts or markdown notes; runtime correctness remains covered by Go tests.

**Tech Stack:** Go tests, existing `agent_message` tool trace persistence, deterministic Agent fixtures where practical, shell smoke script if needed, existing M8 skill registry.

---

## Current Code Facts

- `load_agent_skill` returns JSON with `name`, `version`, `source_hash`, `role_scope`, `content`, and `warnings`.
- Native tool call and result traces are already persisted through existing `NativeToolTraceSink` paths.
- M8.1 did not add `agent_skill_load` DB tables; M8.3 should keep using existing trace unless a test proves it is insufficient.
- M8.2 added 16 role-scoped skills and tests for completeness, sections, role tools, and prompt body exclusion.

## File Structure

- Modify: `apps/server/internal/agent/tools/load_agent_skill_test.go`
- Modify if needed: `apps/server/internal/agent/producer/graph_test.go`
- Create if useful: `scripts/smoke-m8-3-skill-quality-loop.sh`
- Create if useful: `docs/superpowers/reports/2026-06-29-m8-3-skill-quality-loop.md`
- Modify after verification: `docs/milestones/m8-agent-skill-runtime.md`

## Task 1: Strengthen Tool Result Observability Tests

**Files:**
- Modify: `apps/server/internal/agent/tools/load_agent_skill_test.go`

- [ ] **Step 1: Add assertions for reason, version, hash, and warnings behavior**

Extend tests so `load_agent_skill` verifies:

- `reason` is required.
- `source_hash` is non-empty and prefixed with `sha256:`.
- `version` is returned.
- `sections` returns a warning rather than silently pretending to crop.
- wrong-role and wrong-task errors do not include body content.

- [ ] **Step 2: Run tests to verify they fail if behavior is missing**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run LoadAgentSkill -count=1)
```

Expected: PASS if M8.1 already satisfies all observability requirements; otherwise FAIL and fix the smallest gap.

## Task 2: Verify Skill Load Appears In Native Tool Trace

**Files:**
- Modify: `apps/server/internal/agent/producer/graph_test.go` or create a focused test near existing native tool trace tests.

- [ ] **Step 1: Write a deterministic graph or middleware test**

The test should run a Producer graph turn with a responder that calls `load_agent_skill` before final answer. Assert persisted or captured native trace contains:

- tool name `load_agent_skill`
- arguments containing `name` and `reason`
- result containing `version` and `source_hash`

If full graph persistence is too heavy, test the native middleware trace sink path with a fake tool node input and fake sink.

- [ ] **Step 2: Run the focused test**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run SkillTrace -count=1)
```

Expected: FAIL until trace capture is proven; then PASS.

## Task 3: Add Repeatable Commerce Brief Comparison Smoke

**Files:**
- Create if useful: `scripts/smoke-m8-3-skill-quality-loop.sh`
- Create if useful: `docs/superpowers/reports/2026-06-29-m8-3-skill-quality-loop.md`

- [ ] **Step 1: Define the fixed brief**

Use one stable commerce brief:

```text
做一条 15 秒 9:16 电商短视频，推广银灰色登机箱。风格参考为快节奏机场出发场景，卖点是顺滑万向轮、轻量、商务质感。需要旁白和轻快 BGM，最终用于抖音。
```

- [ ] **Step 2: Define comparison dimensions**

Record these fields for skill-enabled and skill-disabled runs:

- Producer: durable facts created or planned.
- Craftsman: RenderPlan completeness, including subject bindings, reference strategy, operation, output type, model prompt profile, risk notes.
- Reviewer: issue specificity and repairability.
- Composer: missing-input blocked quality or finalization readiness.

- [ ] **Step 3: Implement the smoke only if it can be deterministic**

Prefer deterministic fixtures or mock responders. Do not rely on paid provider calls or nondeterministic model output for M8.3 acceptance.

- [ ] **Step 4: Run smoke**

If a script is added:

```bash
bash -n scripts/smoke-m8-3-skill-quality-loop.sh
./scripts/smoke-m8-3-skill-quality-loop.sh
```

Expected: script exits 0 and writes comparison output or assertions.

If a script is not added, write the comparison protocol and evidence template to the report document and keep M8.3 marked pending until an automated or manual comparison is actually executed.

## Task 4: Full M8.3 Verification And Milestone Update

**Files:**
- Modify if M8.3 passes: `docs/milestones/m8-agent-skill-runtime.md`

- [ ] **Step 1: Run focused tests**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./internal/agent/producer -run 'LoadAgentSkill|SkillTrace' -count=1)
```

Expected: PASS.

- [ ] **Step 2: Run milestone verification**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

Expected: PASS.

- [ ] **Step 3: Update M8 milestone only if evidence is real**

Mark M8.3 completed only if:

- tool trace evidence is covered by tests,
- skill-enabled comparison evidence exists,
- verification commands pass.

## Self-Review

Spec coverage:

- Trace observability is covered by Tasks 1-2.
- Skill usefulness comparison is covered by Task 3.
- M8.3 avoids adding unplanned DB tables or remote skill functionality.
- M8.4 governance remains intentionally deferred.

No placeholders remain. The plan explicitly allows keeping M8.3 pending if only the protocol exists but no comparison evidence has been executed.
