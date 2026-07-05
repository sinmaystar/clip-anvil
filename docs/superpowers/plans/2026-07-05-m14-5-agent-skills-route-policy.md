# M14.5 Agent Skills And Route Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Teach Producer, Composer, and Reviewer when and how to use `agent_remotion_code_v1`, while keeping `remotion_timeline_v1` as baseline/fallback and keeping Craftsman out of raw Remotion TSX.

**Architecture:** Add one Composer-only skill for Agent-authored Remotion code attempts. Update existing route-policy skills and registry tests so dynamic route selection, attempt workflow, validation gate, fallback, and review facts are explicit and loadable through the existing embedded skill registry.

**Tech Stack:** Go 1.26 skill registry tests, embedded Markdown `SKILL.md` files, Composer/Producer/Reviewer native tool prompt blocks.

---

## File Structure

- Create `apps/server/internal/agent/skills/library/agent-remotion-code-composer/SKILL.md`: dedicated dynamic Remotion attempt workflow.
- Modify `apps/server/internal/agent/skills/library/commerce-ad-producer/SKILL.md`: route policy for dynamic/non-template requests and rationale/fallback recording.
- Modify `apps/server/internal/agent/skills/library/composer-timeline-director/SKILL.md`: route split between fixed timeline and dynamic code skill.
- Modify `apps/server/internal/agent/skills/library/final-video-remotion-reviewer/SKILL.md`: review dynamic renderer facts and issue categories.
- Modify `apps/server/internal/agent/skills/library/remotion-timeline-composer/SKILL.md`: keep raw Remotion code ban and clarify it is only for `remotion_timeline_v1`.
- Modify `apps/server/internal/agent/skills/registry_test.go`: expected skill list, allowed Composer tools, and skill content assertions.

## Task 1: Registry Surface And New Skill Contract

**Files:**
- Modify: `apps/server/internal/agent/skills/registry_test.go`
- Create: `apps/server/internal/agent/skills/library/agent-remotion-code-composer/SKILL.md`

- [ ] **Step 1: Write failing registry/content tests**

Add tests requiring:

```go
func TestM145DefaultRegistryContainsAgentRemotionCodeComposer(t *testing.T) {
	loaded, err := DefaultRegistry().Load("agent-remotion-code-composer", RoleComposer, "composer_turn")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"agent_remotion_code_v1",
		"create_remotion_renderer_attempt",
		"validate_remotion_renderer_attempt",
		"render_agent_remotion_renderer",
		"Do not install dependencies",
		"validate passed before render",
		"fallback to `remotion_timeline_v1`",
	} {
		if !strings.Contains(loaded.Content, needle) {
			t.Fatalf("agent-remotion-code-composer missing %q\n%s", needle, loaded.Content)
		}
	}
}
```

Also update `TestM82DefaultRegistryContainsCommerceSkillPack` expected Composer skill list to include `agent-remotion-code-composer`, and update allowed Composer tools to include the three new M14.4 tools.

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/skills -run 'TestM82DefaultRegistryContainsCommerceSkillPack|TestM82DefaultSkillsHaveRequiredSectionsAndAllowedTools|TestM145DefaultRegistryContainsAgentRemotionCodeComposer'
```

Expected: FAIL because the new skill does not exist and allowed tool list does not include new tools.

- [ ] **Step 3: Create new skill**

Create `agent-remotion-code-composer/SKILL.md` with frontmatter:

```yaml
---
name: agent-remotion-code-composer
description: Use when Composer builds a non-template final video with agent_remotion_code_v1 by writing, validating, repairing, rendering, and submitting an Agent-authored Remotion renderer attempt.
role_scope: [composer]
task_types: [composer_turn]
domain: [final_video, remotion_code, commerce_ad]
tools: [get_composition_context, stage_media_inputs, probe_media, create_timeline_plan, update_timeline_plan_status, create_remotion_renderer_attempt, validate_remotion_renderer_attempt, render_agent_remotion_renderer, submit_composition_artifact]
source:
  kind: clipanvil-local
version: 0.1.0
---
```

The body must include `## Use When`, `## Do`, `## Do Not`, `## Tool Protocol`, and `## Quality Bar`.

- [ ] **Step 4: Run tests and confirm GREEN**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/skills -run 'TestM82DefaultRegistryContainsCommerceSkillPack|TestM82DefaultSkillsHaveRequiredSectionsAndAllowedTools|TestM145DefaultRegistryContainsAgentRemotionCodeComposer'
```

Expected: PASS.

## Task 2: Producer Route Policy

**Files:**
- Modify: `apps/server/internal/agent/skills/library/commerce-ad-producer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/registry_test.go`

- [ ] **Step 1: Write failing producer skill test**

Add:

```go
func TestM145CommerceProducerCanChooseAgentRemotionRoute(t *testing.T) {
	loaded, err := DefaultRegistry().Load("commerce-ad-producer", RoleProducer, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"agent_remotion_code_v1",
		"non-template",
		"route rationale",
		"fallback policy",
		"Storyboard complexity",
		"mixed-cost can combine Seedance hero clips, Seedream stills, and dynamic Remotion packaging",
		"dispatch_composer(template_key=agent_remotion_code_v1)",
	} {
		if !strings.Contains(loaded.Content, needle) {
			t.Fatalf("commerce-ad-producer missing %q\n%s", needle, loaded.Content)
		}
	}
}
```

- [ ] **Step 2: Run and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/skills -run 'TestM145CommerceProducerCanChooseAgentRemotionRoute'
```

Expected: FAIL because Producer still treats `remotion_timeline_v1` as the ordinary final route.

- [ ] **Step 3: Update Producer skill**

Patch `commerce-ad-producer` so it says:

- Explicit non-template, brand-custom, strong visual differentiation, or “Agent writes Remotion” requests should prefer `agent_remotion_code_v1`.
- Even without explicit ask, Producer may choose based on Storyboard complexity, brand expression, asset richness, delivery risk, cost, and repair budget.
- Route rationale and fallback policy must be recorded in ProjectMemory or Composer dispatch instructions.
- `remotion_timeline_v1` is baseline/fallback rather than the only low-cost endpoint.
- mixed-cost can combine Seedance hero clips, Seedream stills, and dynamic Remotion packaging.

- [ ] **Step 4: Run and confirm GREEN**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/skills -run 'TestM145CommerceProducerCanChooseAgentRemotionRoute|TestCommerceAdProducerPreservesDynamicStoryboardForNoSeedance|TestMotionShotProducerIsRoutePolicyOnly'
```

Expected: PASS.

## Task 3: Composer Route Split

**Files:**
- Modify: `apps/server/internal/agent/skills/library/composer-timeline-director/SKILL.md`
- Modify: `apps/server/internal/agent/skills/library/remotion-timeline-composer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/registry_test.go`

- [ ] **Step 1: Write failing Composer tests**

Add:

```go
func TestM145ComposerTimelineDirectorRoutesDynamicRenderer(t *testing.T) {
	loaded, err := DefaultRegistry().Load("composer-timeline-director", RoleComposer, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"agent_remotion_code_v1",
		"load `agent-remotion-code-composer`",
		"must not follow the remotion_timeline_v1 JSON-only protocol",
		"record fallback reason",
	} {
		if !strings.Contains(loaded.Content, needle) {
			t.Fatalf("composer-timeline-director missing %q\n%s", needle, loaded.Content)
		}
	}
}

func TestM145RemotionTimelineComposerStaysFixedRendererOnly(t *testing.T) {
	loaded, err := DefaultRegistry().Load("remotion-timeline-composer", RoleComposer, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"only for `remotion_timeline_v1`",
		"Do not create raw Remotion code",
		"Do not use agent_remotion_code_v1 attempt tools",
	} {
		if !strings.Contains(loaded.Content, needle) {
			t.Fatalf("remotion-timeline-composer missing %q\n%s", needle, loaded.Content)
		}
	}
}
```

- [ ] **Step 2: Run and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/skills -run 'TestM145ComposerTimelineDirectorRoutesDynamicRenderer|TestM145RemotionTimelineComposerStaysFixedRendererOnly'
```

Expected: FAIL.

- [ ] **Step 3: Update Composer skills**

Patch `composer-timeline-director` to require loading `agent-remotion-code-composer` when `template_key=agent_remotion_code_v1`. Patch `remotion-timeline-composer` to explicitly stay fixed-renderer-only and avoid dynamic attempt tools.

- [ ] **Step 4: Run and confirm GREEN**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/skills -run 'TestM145ComposerTimelineDirectorRoutesDynamicRenderer|TestM145RemotionTimelineComposerStaysFixedRendererOnly|TestRemotionTimelineComposerSkillGuidesCueAlignedStillTimeline'
```

Expected: PASS.

## Task 4: Reviewer Dynamic Renderer Facts

**Files:**
- Modify: `apps/server/internal/agent/skills/library/final-video-remotion-reviewer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/registry_test.go`

- [ ] **Step 1: Write failing Reviewer test**

Add:

```go
func TestM145FinalVideoRemotionReviewerCoversAgentAuthoredRenderer(t *testing.T) {
	loaded, err := DefaultRegistry().Load("final-video-remotion-reviewer", RoleReviewer, "reviewer_turn")
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"Agent-authored renderer",
		"renderer artifact",
		"renderer attempt",
		"source_hash",
		"props_hash",
		"validation_result",
		"compile_result",
		"render_result",
		"unsafe_renderer_code",
		"fallback_required",
	} {
		if !strings.Contains(loaded.Content, needle) {
			t.Fatalf("final-video-remotion-reviewer missing %q:\n%s", needle, loaded.Content)
		}
	}
}
```

- [ ] **Step 2: Run and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/skills -run 'TestM145FinalVideoRemotionReviewerCoversAgentAuthoredRenderer'
```

Expected: FAIL.

- [ ] **Step 3: Update Reviewer skill**

Patch reviewer skill so it covers fixed `remotion_timeline_v1` and `agent_remotion_code_v1`, reads renderer attempt facts, names dynamic issue categories, and still keeps existing caption/audio/Seedance gates.

- [ ] **Step 4: Run and confirm GREEN**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/skills -run 'TestM145FinalVideoRemotionReviewerCoversAgentAuthoredRenderer|TestFinalVideoRemotionReviewerSkillNamesTimelineQualityGates'
```

Expected: PASS.

## Task 5: M14.5 Verification

- [ ] **Step 1: Run skill package tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/skills
```

Expected: PASS.

- [ ] **Step 2: Run related role/tool tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/producer ./internal/agent/composer ./internal/agent/reviewer ./internal/agent/tools
```

Expected: PASS.

- [ ] **Step 3: Run diff check**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 4: Confirm acceptance**

Confirm:

- Producer can choose `agent_remotion_code_v1` and must record route rationale/fallback.
- Composer has a dedicated dynamic Remotion skill and does not mix it with fixed JSON-only renderer protocol.
- Reviewer distinguishes fixed timeline renderer from Agent-authored renderer and reads attempt facts.
- `remotion-timeline-composer` still says “Do not create raw Remotion code.”
