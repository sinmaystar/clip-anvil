# M5 UX Corrections Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the M5 Studio node editing experience so one floating production editor replaces the duplicated right inspector, Reference Packs cannot feed their own members, and Prompt `@` text stays aligned with node title changes.

**Architecture:** Reuse the existing `PropertyPanel` production controls as the single node popover instead of keeping a separate inline editor plus dock. Keep dependency graph rules enforced on both frontend helpers and backend APIs. Treat `prompt_refs.refs[].node_id` as the source of truth and rewrite known `@label` tokens when referenced node titles change.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, React 19, TypeScript 6, TanStack Query, React Flow, Vite 8, Node test runner.

---

## File Structure

- Modify `apps/web/src/lib/productionPanel.ts`
  - Add node-aware simplified operation option helpers.
- Modify `apps/web/src/lib/productionPanel.test.mjs`
  - Cover Image/Video simplified operation choices.
- Modify `apps/web/src/lib/referencePack.ts`
  - Add helper to detect invalid Pack -> member dependency.
- Modify `apps/web/src/lib/referencePack.test.mjs`
  - Cover Pack -> member rejection.
- Modify `apps/server/internal/api/edge_handler.go`
  - Reject dependency edges from a Reference Pack to one of its members.
- Modify `apps/server/internal/api/reference_pack_handler.go`
  - Reject adding a member that already depends on the pack.
- Modify `apps/server/internal/api/edge_handler_test.go`
  - Cover helper-level Pack -> member cycle rejection.
- Modify `apps/server/internal/api/reference_pack_handler_test.go`
  - Cover member validation for existing Pack dependency.
- Modify `apps/web/src/lib/promptRefs.ts`
  - Add a helper that rewrites `@oldLabel` tokens and refreshes labels for a renamed referenced node.
- Modify `apps/web/src/lib/promptRefs.test.mjs`
  - Cover title rename propagation.
- Modify `apps/web/src/components/PropertyPanel.tsx`
  - Hide graph-obvious sections, simplify Operation UI, keep core production controls first, move versions/job/stale into secondary details.
- Modify `apps/web/src/pages/WorkspaceDetailPage.tsx`
  - Render the production editor as a floating node popover and remove the right-side always-on dock.
  - Apply prompt ref rename patches when a node title is committed.
- Modify `apps/web/src/main.css`
  - Style the floating popover and remove right dock assumptions.

## Tasks

### Task 1: Simplify The Single Node React Flow

- [ ] Add failing frontend tests for simplified operation choices.
- [ ] Replace the always-on property dock with a node-positioned popover using existing production controls.
- [ ] Remove the old lightweight inline editor from selected-node rendering.
- [ ] Hide group/dependency/prompt-reference sections from the node production editor.
- [ ] Move Versions, Latest Job, and Stale Reasons into secondary `<details>` sections.
- [ ] Run `pnpm --filter @clip-anvil/web test:connections`.

### Task 2: Block Reference Pack Semantic Cycles

- [ ] Add failing frontend helper and backend helper tests for Pack -> member rejection.
- [ ] Reject Pack -> member dependency edge creation in the backend.
- [ ] Reject Reference Pack membership updates when a candidate member already depends on that pack.
- [ ] Use the frontend helper to prevent accidental UI attempts where possible.
- [ ] Run `GOCACHE=/private/tmp/clipanvil-go-build make server-test`.

### Task 3: Keep Prompt Ref Labels In Sync

- [ ] Add failing frontend tests for `@oldTitle` to `@newTitle` rewrite.
- [ ] Add a pure prompt-ref rename helper.
- [ ] When a title commit succeeds, patch downstream nodes that explicitly reference the renamed node.
- [ ] Run `pnpm --filter @clip-anvil/web test:connections`.

### Task 4: Final Verification

- [ ] Run `pnpm --filter @clip-anvil/web lint`.
- [ ] Run `pnpm --filter @clip-anvil/web... build`.
- [ ] Run `GOCACHE=/private/tmp/clipanvil-go-build make server-build`.
- [ ] Run `git diff --check`.
