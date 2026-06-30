# M8.4 Skill Resource And Governance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task in this session, or use `superpowers:subagent-driven-development` with a fresh worker per task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add minimal governance around built-in skills: controlled skill resource loading, role/task-aware enablement checks, usage statistics primitives, and tests that skill tool references stay aligned with role registries.

**Architecture:** M8.4 remains repo-local and read-only. Additional resources live inside each skill directory and are loaded only through a white-listed `load_agent_skill_resource` native tool. Skill enablement and usage stats stay in-memory/service-level for M8.4, avoiding database schema before product requirements are clearer. Tool consistency is enforced by tests.

**Tech Stack:** Go tests, embedded `fs.FS`, existing Agent native tools, built-in skill library, no database migration, no UI.

---

## Current Code Facts

- Built-in skills are embedded with `//go:embed library/*/SKILL.md`.
- `load_agent_skill` already returns full `SKILL.md` content by name with role / task filtering.
- M8.4 milestone calls for optional `load_agent_skill_resource`, workspace / tenant switch, usage statistics, and tool-name consistency.
- There is no current product requirement for persistent DB skill settings, so M8.4 should not add migrations.

## File Structure

- Modify: `apps/server/internal/agent/skills/embed.go`
- Modify: `apps/server/internal/agent/skills/registry.go`
- Modify: `apps/server/internal/agent/skills/types.go`
- Modify: `apps/server/internal/agent/skills/registry_test.go`
- Create: `apps/server/internal/agent/tools/load_agent_skill_resource.go`
- Create: `apps/server/internal/agent/tools/load_agent_skill_resource_test.go`
- Modify: `apps/server/cmd/server/main.go`
- Modify if needed: role prompt/tool tests.
- Modify after verification: `docs/milestones/m8-agent-skill-runtime.md`

## Task 1: Registry Resource Loading And Usage Stats

**Files:**
- Modify: `apps/server/internal/agent/skills/types.go`
- Modify: `apps/server/internal/agent/skills/registry.go`
- Modify: `apps/server/internal/agent/skills/embed.go`
- Modify: `apps/server/internal/agent/skills/registry_test.go`
- Add one markdown resource under a skill directory, for example `apps/server/internal/agent/skills/library/commerce-ad-producer/references/checklist.md`

- [ ] **Step 1: Write failing registry tests**

Add tests proving:

- `LoadResource(skillName, role, taskType, resourcePath)` loads only files under the selected skill directory.
- path traversal such as `../other/SKILL.md` is rejected.
- non-markdown or missing resources are rejected.
- usage stats increment for `Load` and `LoadResource`.

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills -run 'Resource|Usage|ToolReferences' -count=1)
```

Expected: FAIL until registry supports resource loading and usage stats.

- [ ] **Step 3: Implement registry support**

Implement:

- `type LoadedSkillResource`.
- `LoadResource(name string, role Role, taskType string, resourcePath string) (LoadedSkillResource, error)`.
- `UsageStats() []UsageStat` or equivalent sorted readout.
- Resource path validation:
  - trim whitespace,
  - reject empty,
  - reject absolute paths,
  - reject `..`,
  - allow `.md` only.
- Expand embed pattern to include markdown resources, not scripts.

- [ ] **Step 4: Run registry tests to verify they pass**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills -count=1)
```

Expected: PASS.

## Task 2: `load_agent_skill_resource` Native Tool

**Files:**
- Create: `apps/server/internal/agent/tools/load_agent_skill_resource.go`
- Create: `apps/server/internal/agent/tools/load_agent_skill_resource_test.go`

- [ ] **Step 1: Write failing native tool tests**

Tests should prove:

- Tool name is `load_agent_skill_resource`.
- Correct role/task can load `commerce-ad-producer` resource `references/checklist.md`.
- Wrong role is rejected without leaking content.
- Path traversal is rejected.

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run LoadAgentSkillResource -count=1)
```

Expected: FAIL until tool exists.

- [ ] **Step 3: Implement tool**

The tool input:

```json
{
  "name": "commerce-ad-producer",
  "resource_path": "references/checklist.md",
  "reason": "Need the detailed checklist."
}
```

The output includes:

- `name`
- `resource_path`
- `version`
- `source_hash`
- `content`

- [ ] **Step 4: Run tool tests to verify they pass**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run LoadAgentSkillResource -count=1)
```

Expected: PASS.

## Task 3: Register Resource Tool And Tool-Reference Consistency Tests

**Files:**
- Modify: `apps/server/cmd/server/main.go`
- Modify or create tests under `apps/server/internal/agent/skills/registry_test.go` or `apps/server/internal/agent/tools`.

- [ ] **Step 1: Register `load_agent_skill_resource` for all four roles**

Add it next to `load_agent_skill` in Producer, Craftsman, Reviewer, and Composer registries.

- [ ] **Step 2: Add consistency tests**

Test every default skill frontmatter `tools` entry is present in that role's expected native tool name set. Include both `load_agent_skill` and `load_agent_skill_resource` only if a skill explicitly references them.

- [ ] **Step 3: Run focused tests**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/skills ./internal/agent/tools ./cmd/server -run 'Resource|ToolReferences|NativeToolInfos' -count=1)
```

Expected: PASS.

## Task 4: Full M8.4 Verification And Milestone Update

**Files:**
- Modify: `docs/milestones/m8-agent-skill-runtime.md`

- [ ] **Step 1: Run server tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

Expected: PASS.

- [ ] **Step 2: Update milestone**

Mark M8.4 completed only if:

- resource loading works and is path-safe,
- usage stats are test-covered,
- tool reference consistency is test-covered,
- server tests and diff check pass.

## Self-Review

Spec coverage:

- `load_agent_skill_resource` is covered by Tasks 1-3.
- Usage statistics are covered by Task 1.
- Tool-name consistency is covered by Task 3.
- Workspace / tenant persistent switches are intentionally deferred because there is no current DB/UI requirement; M8.4 delivers the in-process governance substrate without schema churn.

No placeholders remain. This plan keeps M8.4 inside repo-local, read-only skill governance and does not add remote install, scripts, marketplace, or UI.
