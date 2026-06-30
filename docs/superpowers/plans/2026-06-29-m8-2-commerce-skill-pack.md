# M8.2 Commerce Short-Video Skill Pack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task in this session, or use `superpowers:subagent-driven-development` with a fresh worker per task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expand the M8.1 skill foundation into a role-scoped commerce short-video skill pack for Producer, Craftsman, Reviewer, and Composer.

**Architecture:** M8.2 adds high-signal `SKILL.md` files under the existing embedded `apps/server/internal/agent/skills/library/` directory. Runtime code should stay unchanged unless tests expose a real validation gap. Every skill must use the M8.1 frontmatter schema, reference only tools already available to that role, and keep complete instructions out of the default system prompt.

**Tech Stack:** Markdown `SKILL.md`, YAML frontmatter, existing `agent/skills` registry tests, existing role prompt tests, Go server tests.

---

## Current Code Facts

- M8.1 provides embedded skill loading, role / task filtering, `PromptBlock`, and `load_agent_skill`.
- Starter skills already exist:
  - `commerce-ad-producer`
  - `seedance-renderplan-craftsman`
  - `reviewer-quality-gate`
  - `composer-timeline-director`
- M8.2 should enrich or add skill documents only. It should not add DB tables, remote skill loading, script execution, UI, or new tools.
- Prompt block tests must continue proving only `name` / `description` are injected.

## File Structure

- Modify: `apps/server/internal/agent/skills/library/commerce-ad-producer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/reference-video-analysis-producer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/audio-plan-producer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/hitl-checkpoint-producer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/library/seedance-renderplan-craftsman/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/seedream-renderplan-craftsman/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/audio-renderplan-craftsman/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/renderplan-repair-craftsman/SKILL.md`
- Modify: `apps/server/internal/agent/skills/library/reviewer-quality-gate/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/commerce-delivery-promise-reviewer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/reference-consistency-reviewer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/final-video-audio-reviewer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/library/composer-timeline-director/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/ffmpeg-audio-mix-composer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/platform-export-composer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/composer-blocker-escalation/SKILL.md`
- Modify: `apps/server/internal/agent/skills/registry_test.go`

## Skill Quality Contract

Every M8.2 skill must contain these sections:

```markdown
## Use When

## Do

## Do Not

## Tool Protocol

## Quality Bar
```

Every M8.2 skill frontmatter must include:

```yaml
---
name: stable-skill-name
description: Use when Role performs a precise production task.
role_scope: [producer]
task_types: [producer_turn]
domain: [commerce_ad]
tools: [existing_tool_name]
source:
  kind: clipanvil-local
version: 0.1.0
---
```

## Task 1: Add Pack Contract Tests

**Files:**
- Modify: `apps/server/internal/agent/skills/registry_test.go`

- [ ] **Step 1: Write failing tests for skill pack completeness**

Add tests that assert:

- Producer has exactly the expected M8.2 names.
- Craftsman has exactly the expected M8.2 names.
- Reviewer has exactly the expected M8.2 names.
- Composer has exactly the expected M8.2 names.
- Every default skill has required sections.
- PromptBlock still excludes body text.

Expected skill names:

```go
var expectedM82Skills = map[Role][]string{
	RoleProducer: {
		"audio-plan-producer",
		"commerce-ad-producer",
		"hitl-checkpoint-producer",
		"reference-video-analysis-producer",
	},
	RoleCraftsman: {
		"audio-renderplan-craftsman",
		"renderplan-repair-craftsman",
		"seedance-renderplan-craftsman",
		"seedream-renderplan-craftsman",
	},
	RoleReviewer: {
		"commerce-delivery-promise-reviewer",
		"final-video-audio-reviewer",
		"reference-consistency-reviewer",
		"reviewer-quality-gate",
	},
	RoleComposer: {
		"composer-blocker-escalation",
		"composer-timeline-director",
		"ffmpeg-audio-mix-composer",
		"platform-export-composer",
	},
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills -run 'M82|DefaultRegistry|PromptBlock' -count=1)
```

Expected: FAIL because most M8.2 skill files do not exist and starter skills do not yet all include `Quality Bar`.

## Task 2: Producer Skill Pack

**Files:**
- Modify: `apps/server/internal/agent/skills/library/commerce-ad-producer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/reference-video-analysis-producer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/audio-plan-producer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/hitl-checkpoint-producer/SKILL.md`

- [ ] **Step 1: Write Producer skills**

Each Producer skill must only reference Producer tools:

```text
read_project_context
upsert_project_brief
update_project_memory
upsert_key_elements
upsert_storyboard
upsert_audio_plan
dispatch_craftsman
dispatch_composer
decide_render_plan
dispatch_reviewer
request_user_decision
```

- [ ] **Step 2: Run Producer skill tests**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills -run M82 -count=1)
```

Expected: Producer portion passes; other roles may still fail until later tasks.

## Task 3: Craftsman Skill Pack

**Files:**
- Modify: `apps/server/internal/agent/skills/library/seedance-renderplan-craftsman/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/seedream-renderplan-craftsman/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/audio-renderplan-craftsman/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/renderplan-repair-craftsman/SKILL.md`

- [ ] **Step 1: Write Craftsman skills**

Each Craftsman skill must only reference:

```text
read_project_memory
upsert_render_plan
```

The skill body should explain how to produce better RenderPlan structure, not claim direct generation capability.

- [ ] **Step 2: Run Craftsman skill tests**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills -run M82 -count=1)
```

Expected: Producer and Craftsman portions pass; Reviewer / Composer may still fail.

## Task 4: Reviewer Skill Pack

**Files:**
- Modify: `apps/server/internal/agent/skills/library/reviewer-quality-gate/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/commerce-delivery-promise-reviewer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/reference-consistency-reviewer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/final-video-audio-reviewer/SKILL.md`

- [ ] **Step 1: Write Reviewer skills**

Each Reviewer skill must only reference:

```text
read_project_context
read_project_memory
submit_review_result
```

Every Reviewer skill must explicitly say it does not modify facts, pick winners, dispatch retries, or request users directly.

- [ ] **Step 2: Run Reviewer skill tests**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills -run M82 -count=1)
```

Expected: Producer, Craftsman, and Reviewer portions pass; Composer may still fail.

## Task 5: Composer Skill Pack

**Files:**
- Modify: `apps/server/internal/agent/skills/library/composer-timeline-director/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/ffmpeg-audio-mix-composer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/platform-export-composer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/composer-blocker-escalation/SKILL.md`

- [ ] **Step 1: Write Composer skills**

Composer skills may reference only:

```text
get_composition_context
stage_media_inputs
probe_media
create_timeline_plan
update_timeline_plan_status
render_timeline_template
run_ffmpeg_command
submit_composition_artifact
```

Every Composer skill must stay within existing sandbox ffmpeg / TimelinePlan capability and avoid Remotion / HyperFrames assumptions.

- [ ] **Step 2: Run Composer skill tests**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills -run M82 -count=1)
```

Expected: PASS.

## Task 6: Full M8.2 Verification And Milestone Update

**Files:**
- Modify if M8.2 passes: `docs/milestones/m8-agent-skill-runtime.md`

- [ ] **Step 1: Run focused skill and prompt tests**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer -count=1)
```

Expected: PASS.

- [ ] **Step 2: Run milestone verification**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

Expected: PASS.

- [ ] **Step 3: Update M8 milestone**

If M8.2 passes, update `docs/milestones/m8-agent-skill-runtime.md` to mark M8.2 completed and record verification commands. Leave M8.3-M8.4 pending.

## Self-Review

Spec coverage:

- M8.2 role skill list is covered by Tasks 2-5.
- Role boundary and real native tool references are covered by the skill quality contract and tests.
- Default prompt non-bloat is covered by Task 1.
- No runtime mechanism changes are planned unless tests reveal a validation gap.

No placeholders remain. M8.3 quality smoke and M8.4 governance are intentionally deferred to later milestone plans.
