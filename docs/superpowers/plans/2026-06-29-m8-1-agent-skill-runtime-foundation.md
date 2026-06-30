# M8.1 Agent Skill Runtime Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task in this session, or use `superpowers:subagent-driven-development` with a fresh worker per task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the M8.1 foundation: role-scoped built-in Agent skills, metadata-only prompt injection, and a `load_agent_skill` native tool that returns full `SKILL.md` content as a tool result.

**Architecture:** Built-in skills live under `apps/server/internal/agent/skills/library/**/SKILL.md` and are embedded into the server binary. The `agent/skills` package parses YAML frontmatter, validates role and task filters, builds prompt blocks, and exposes a service for loading content. The native tool adapter lives in `agent/tools` so it can be registered in the existing Eino `NativeRegistry` for Producer, Craftsman, Reviewer, and Composer.

**Tech Stack:** Go 1.26, Go embed, `gopkg.in/yaml.v3`, CloudWeGo Eino native tools, existing ClipAnvil Agent system prompts and native tool registry.

---

## Current Code Facts

- Four active Agent graph paths are `producer_turn`, `craftsman_render_plan`, `reviewer_gate`, and `composer_timeline`.
- Runtime `agent_task.task_type` values for role tasks are `producer_turn`, `decision_resume`, `craftsman_turn`, `reviewer_turn`, and `composer_turn`.
- Native tools implement `agenttools.NativeTool`, with `Info(context.Context)` and `InvokableRun(context.Context, string, ...tool.Option)`.
- Native runtime context is provided through `agenttools.NativeRuntimeFromContext(ctx)`.
- Four role prompts live in:
  - `apps/server/internal/agent/producer/system_prompt.go`
  - `apps/server/internal/agent/craftsman/system_prompt.go`
  - `apps/server/internal/agent/reviewer/system_prompt.go`
  - `apps/server/internal/agent/composer/system_prompt.go`
- Four role registries are wired in `apps/server/cmd/server/main.go`.

## File Structure

- Create `apps/server/internal/agent/skills/types.go`: public skill metadata, role constants, load result types.
- Create `apps/server/internal/agent/skills/registry.go`: parse embedded `SKILL.md`, validate frontmatter, filter by role / task type, load full content, compute hash.
- Create `apps/server/internal/agent/skills/prompt_block.go`: render compact `Skills Library` system prompt block.
- Create `apps/server/internal/agent/skills/embed.go`: embed built-in `library/**/SKILL.md` and expose `DefaultRegistry()`.
- Create `apps/server/internal/agent/skills/registry_test.go`: parser, validation, role filtering, prompt metadata, load body tests.
- Create starter built-in skill files under `apps/server/internal/agent/skills/library/`.
- Create `apps/server/internal/agent/tools/load_agent_skill.go`: native tool adapter.
- Create `apps/server/internal/agent/tools/load_agent_skill_test.go`: tool info, success, missing skill, wrong role / task tests.
- Modify four role `system_prompt.go` files to append `skills.PromptBlock(role)`.
- Modify `apps/server/cmd/server/main.go` to register `load_agent_skill` in all four native registries.
- Update existing role prompt tests to assert `Skills Library` and `load_agent_skill`.

## Task 1: Skill Registry And Prompt Block

**Files:**
- Create: `apps/server/internal/agent/skills/types.go`
- Create: `apps/server/internal/agent/skills/registry.go`
- Create: `apps/server/internal/agent/skills/prompt_block.go`
- Create: `apps/server/internal/agent/skills/embed.go`
- Create: `apps/server/internal/agent/skills/registry_test.go`
- Create: `apps/server/internal/agent/skills/library/commerce-ad-producer/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/seedance-renderplan-craftsman/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/reviewer-quality-gate/SKILL.md`
- Create: `apps/server/internal/agent/skills/library/composer-timeline-director/SKILL.md`

- [ ] **Step 1: Write failing registry and prompt tests**

Create `apps/server/internal/agent/skills/registry_test.go`:

```go
package skills

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestRegistryParsesFrontmatterAndFiltersByRole(t *testing.T) {
	registry, err := NewRegistry(fstest.MapFS{
		"library/producer/SKILL.md": &fstest.MapFile{Data: []byte(`---
name: commerce-ad-producer
description: Use when Producer plans a commerce ad.
role_scope: [producer]
task_types: [producer_turn, decision_resume]
version: 0.1.0
---
# Producer Body
`)},
		"library/craftsman/SKILL.md": &fstest.MapFile{Data: []byte(`---
name: seedance-renderplan-craftsman
description: Use when Craftsman writes a Seedance RenderPlan.
role_scope: [craftsman]
task_types: [craftsman_turn]
version: 0.1.0
---
# Craftsman Body
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	producerSkills := registry.SkillsForRole(RoleProducer)
	if len(producerSkills) != 1 || producerSkills[0].Name != "commerce-ad-producer" {
		t.Fatalf("producer skills = %#v", producerSkills)
	}
	craftsmanSkills := registry.SkillsForRole(RoleCraftsman)
	if len(craftsmanSkills) != 1 || craftsmanSkills[0].Name != "seedance-renderplan-craftsman" {
		t.Fatalf("craftsman skills = %#v", craftsmanSkills)
	}
}

func TestPromptBlockIncludesOnlyMetadata(t *testing.T) {
	registry, err := NewRegistry(fstest.MapFS{
		"library/producer/SKILL.md": &fstest.MapFile{Data: []byte(`---
name: commerce-ad-producer
description: Use when Producer plans a commerce ad.
role_scope: [producer]
task_types: [producer_turn]
version: 0.1.0
---
# Producer Body

Detailed body must not be in system prompt.
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	block := PromptBlock(registry, RoleProducer)
	for _, want := range []string{"## Skills Library", "load_agent_skill", "commerce-ad-producer", "Use when Producer plans a commerce ad."} {
		if !strings.Contains(block, want) {
			t.Fatalf("prompt block missing %q:\n%s", want, block)
		}
	}
	for _, forbidden := range []string{"# Producer Body", "Detailed body must not be in system prompt."} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("prompt block leaked body %q:\n%s", forbidden, block)
		}
	}
}

func TestLoadSkillValidatesRoleAndTaskType(t *testing.T) {
	registry, err := NewRegistry(fstest.MapFS{
		"library/producer/SKILL.md": &fstest.MapFile{Data: []byte(`---
name: commerce-ad-producer
description: Use when Producer plans a commerce ad.
role_scope: [producer]
task_types: [producer_turn]
version: 0.1.0
---
# Producer Body
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := registry.Load("commerce-ad-producer", RoleProducer, "producer_turn")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != "0.1.0" || loaded.SourceHash == "" || !strings.Contains(loaded.Content, "# Producer Body") {
		t.Fatalf("loaded = %#v", loaded)
	}
	if _, err := registry.Load("commerce-ad-producer", RoleReviewer, "reviewer_turn"); err == nil {
		t.Fatal("expected wrong role to be rejected")
	}
	if _, err := registry.Load("commerce-ad-producer", RoleProducer, "composer_turn"); err == nil {
		t.Fatal("expected wrong task type to be rejected")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills -run 'TestRegistry|TestPrompt|TestLoad' -count=1)
```

Expected: FAIL because `apps/server/internal/agent/skills` does not exist.

- [ ] **Step 3: Implement the skills package and starter skills**

Implement the package with:

- `type Role string` and constants `RoleProducer`, `RoleCraftsman`, `RoleReviewer`, `RoleComposer`.
- `type SkillMetadata` containing `Name`, `Description`, `RoleScope`, `TaskTypes`, `Domain`, `Tools`, `Source`, `Version`.
- `type LoadedSkill` containing `SkillMetadata`, `Content`, and `SourceHash`.
- `NewRegistry(fsys fs.FS) (*Registry, error)` scanning `library/**/SKILL.md`.
- `SkillsForRole(role Role) []SkillMetadata`.
- `Load(name string, role Role, taskType string) (LoadedSkill, error)`.
- `PromptBlock(registry *Registry, role Role) string`.
- `DefaultRegistry() *Registry` backed by embedded skills.

Starter skill bodies should be short, role-scoped, and reference only existing tools:

- Producer: `commerce-ad-producer`
- Craftsman: `seedance-renderplan-craftsman`
- Reviewer: `reviewer-quality-gate`
- Composer: `composer-timeline-director`

- [ ] **Step 4: Run registry tests to verify they pass**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills -count=1)
```

Expected: PASS.

## Task 2: `load_agent_skill` Native Tool

**Files:**
- Create: `apps/server/internal/agent/tools/load_agent_skill.go`
- Create: `apps/server/internal/agent/tools/load_agent_skill_test.go`

- [ ] **Step 1: Write failing native tool tests**

Create `apps/server/internal/agent/tools/load_agent_skill_test.go` with tests that:

- `Info()` exposes tool name `load_agent_skill`.
- Producer can load `commerce-ad-producer` with task type `producer_turn`.
- Reviewer cannot load `commerce-ad-producer`.
- Missing skill returns a natural tool error without a Go error.

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run LoadAgentSkill -count=1)
```

Expected: FAIL because `NewLoadAgentSkillNativeTool` does not exist.

- [ ] **Step 3: Implement the tool**

Add:

```go
const toolLoadAgentSkill = "load_agent_skill"

type SkillLoader interface {
	Load(name string, role agentskills.Role, taskType string) (agentskills.LoadedSkill, error)
	SkillsForRole(role agentskills.Role) []agentskills.SkillMetadata
}

type LoadAgentSkillNativeTool struct {
	registry SkillLoader
	role     agentskills.Role
}
```

`InvokableRun` must:

- Decode `name`, `reason`, optional `sections`.
- Require non-empty `name` and `reason`.
- Resolve task type from `NativeRuntimeContext.ExecutionPolicy` only if needed later; for M8.1 pass an explicit role to the constructor and map task type from runtime scope when available.
- Load through registry with the configured role and current task type.
- Return JSON containing `name`, `version`, `source_hash`, `role_scope`, `content`, and `warnings`.
- Return `NaturalToolError` for bad args, missing runtime when task filtering needs it, missing skill, wrong role, or wrong task.

- [ ] **Step 4: Run native tool tests to verify they pass**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run LoadAgentSkill -count=1)
```

Expected: PASS.

## Task 3: System Prompt Integration

**Files:**
- Modify: `apps/server/internal/agent/producer/system_prompt.go`
- Modify: `apps/server/internal/agent/craftsman/system_prompt.go`
- Modify: `apps/server/internal/agent/reviewer/system_prompt.go`
- Modify: `apps/server/internal/agent/composer/system_prompt.go`
- Modify tests:
  - `apps/server/internal/agent/producer/model_responder_test.go`
  - `apps/server/internal/agent/craftsman/model_responder_test.go`
  - `apps/server/internal/agent/reviewer/system_prompt_test.go`
  - add or modify a Composer prompt test if needed.

- [ ] **Step 1: Write failing prompt tests**

Add assertions that each role prompt contains:

- `## Skills Library`
- `load_agent_skill`
- that role's starter skill name

Add one negative assertion where practical, such as Producer prompt not containing `# Commerce Ad Producer` or a Craftsman-only skill body heading.

- [ ] **Step 2: Run prompt tests to verify they fail**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer -run 'SystemPrompt|PromptMessages|Composer' -count=1)
```

Expected: FAIL because prompts do not include `Skills Library`.

- [ ] **Step 3: Append role-scoped prompt blocks**

Use `agentskills.PromptBlock(agentskills.DefaultRegistry(), agentskills.RoleProducer)` etc. in each system prompt. Keep the existing role prompt text intact.

- [ ] **Step 4: Run prompt tests to verify they pass**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer -run 'SystemPrompt|PromptMessages|Composer' -count=1)
```

Expected: PASS.

## Task 4: Runtime Wiring

**Files:**
- Modify: `apps/server/cmd/server/main.go`
- Modify or create: `apps/server/cmd/server/main_test.go`
- Modify existing tool info tests if they assert exact counts.

- [ ] **Step 1: Write failing wiring test**

Add a test or extend existing registry tests to prove each role registry includes `load_agent_skill`. If direct `main.go` testing is too heavy, add role-specific native tool info tests near existing tests:

- Producer registry test includes `load_agent_skill`.
- Craftsman registry test includes `load_agent_skill`.
- Reviewer registry test includes `load_agent_skill`.
- Composer registry test includes `load_agent_skill`.

- [ ] **Step 2: Run wiring tests to verify they fail**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./cmd/server -run 'LoadAgentSkill|NativeToolInfos|Main' -count=1)
```

Expected: FAIL until registries include `NewLoadAgentSkillNativeTool(...)`.

- [ ] **Step 3: Register the tool for all four roles**

In `apps/server/cmd/server/main.go`, create:

```go
skillRegistry := agentskills.DefaultRegistry()
```

Then register:

- `agenttools.NewLoadAgentSkillNativeTool(skillRegistry, agentskills.RoleComposer)` in Composer registry.
- `agenttools.NewLoadAgentSkillNativeTool(skillRegistry, agentskills.RoleCraftsman)` in Craftsman registry.
- `agenttools.NewLoadAgentSkillNativeTool(skillRegistry, agentskills.RoleReviewer)` in Reviewer registry.
- `agenttools.NewLoadAgentSkillNativeTool(skillRegistry, agentskills.RoleProducer)` in Producer registry.

- [ ] **Step 4: Run wiring tests to verify they pass**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./cmd/server -run 'LoadAgentSkill|NativeToolInfos|Main' -count=1)
```

Expected: PASS.

## Task 5: Full M8.1 Verification

**Files:**
- Update if needed: `docs/milestones/m8-agent-skill-runtime.md`

- [ ] **Step 1: Run focused server tests**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills ./internal/agent/tools ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer -count=1)
```

Expected: PASS.

- [ ] **Step 2: Run M8.1 milestone verification**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

Expected: PASS.

- [ ] **Step 3: Update milestone status if the phase is fully done**

If and only if M8.1 passes, update `docs/milestones/m8-agent-skill-runtime.md` to mark M8.1 completed and record verification commands. Leave M8.2-M8.4 as pending.

## Self-Review

Spec coverage:

- Metadata-only prompt injection is covered by Task 1 and Task 3.
- Role-scoped loading is covered by Task 1 and Task 2.
- Native tool result loading is covered by Task 2.
- Four Agent registration is covered by Task 4.
- No DB table in M8.1 is preserved by the file structure and task list.
- M8.2 skill pack is not implemented here except for four starter skills needed to prove M8.1; full skill pack remains a later plan.

No placeholders remain. All tasks have exact paths, commands, and expected outcomes.
