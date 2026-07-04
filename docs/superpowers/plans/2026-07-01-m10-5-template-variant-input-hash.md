# M10.5 Template Variant Input Hash Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Prepare template video for Variant Factory by making template params, variables, and input refs produce stable, distinct artifact versions and stale checks.

**Architecture:** Keep versioning in the existing production input hash. Extend `InputHashFacts` with a compact, stable `input_refs` summary derived from `GenerationIntent.InputRefs`. This preserves the existing dependency winner hash behavior while adding template-critical binding facts such as ref order, content type, model role, required flag, and current version.

**Tech Stack:** Go 1.26, existing production service/input hash, existing Worker input binding resolution, existing RenderPlan params.

---

## Files To Change

- Modify: `apps/server/internal/production/input_hash.go`
- Modify: `apps/server/internal/production/input_hash_test.go`
- Modify: `apps/server/internal/production/service_test.go`
- Create: `docs/superpowers/reports/2026-07-01-m10-5-template-variant-input-hash.md`
- Modify: `docs/milestones/m10-hyperframes-template-video-provider.md`

## Task 1: Template Params And Variables Are Hash Inputs

- [x] **Step 1: Write hash tests**

Add tests in `apps/server/internal/production/input_hash_test.go` proving:

- changing `params.template_key` changes the hash
- changing `params.variables.headline` changes the hash
- changing nested variable ordering does not change the hash

- [x] **Step 2: Run tests**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestComputeInputHashChangesWhenTemplate'
```

Expected: PASS if existing params hashing already covers this. If it fails, fix `ComputeInputHash`.

## Task 2: Input Refs Are Hash Inputs

- [x] **Step 1: Write failing input ref tests**

Add tests proving template hash changes when:

- the same image winner is bound with a different `model_role`
- input ref order changes
- `current_version_id` / `input_hash` changes

Expected initial failure: current `InputHashFacts` does not include `GenerationIntent.InputRefs`.

- [x] **Step 2: Implement compact input ref facts**

Add:

```go
type InputHashInputRef struct {
    OrderIndex       int    `json:"order_index"`
    NodeID           string `json:"node_id"`
    Kind             string `json:"kind"`
    Required         bool   `json:"required"`
    NodeType         string `json:"node_type,omitempty"`
    CurrentVersionID string `json:"current_version_id,omitempty"`
    AssetID          string `json:"asset_id,omitempty"`
    AssetType        string `json:"asset_type,omitempty"`
    Mime             string `json:"mime,omitempty"`
    ContentType      string `json:"content_type,omitempty"`
    ModelRole        string `json:"model_role,omitempty"`
    InputHash        string `json:"input_hash,omitempty"`
}
```

Do not include `StorageURL` or `TextContent`; dependency/source material hash already covers content identity, and URLs may be environment-specific.

- [x] **Step 3: Wire into `InputHashFactsForNode`**

Populate `InputRefs` from `intent.InputRefs`.

## Task 3: Production Stale Recompute Uses Persisted Template Config

- [x] **Step 1: Add service-level test**

Add a focused test proving `intentForNode` restores:

- `model_provider=internal_template_video`
- `model_id=hyperframes-html`
- `operation_type=image_to_template_video`
- `model_params.template_key`
- `model_params.variables`

and `InputHashFactsForNode` uses those params when computing a current hash.

Expected: PASS if Worker node persistence from M10.2/M10.3 is already correct. If it fails, persist missing fields on production node creation.

## Task 4: Verification And Docs

- [x] **Step 1: Run focused tests**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'TestComputeInputHash|TestIntentForNodeRestoresTemplateVideoConfig'
```

- [x] **Step 2: Run milestone verification**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production ./internal/agent/worker
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web... lint
git diff --check
```

- [x] **Step 3: Update report and milestone**

Record command output in the M10.5 report and mark M10.5 passed only after fresh verification.
