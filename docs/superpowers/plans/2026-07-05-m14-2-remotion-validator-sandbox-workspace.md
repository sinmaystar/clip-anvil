# M14.2 Remotion Validator and Sandbox Workspace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the attempt sandbox workspace helper and static validator for `agent_remotion_code_v1`, so generated renderer files can be written, snapshotted, checked, hashed, and persisted back to `remotion_renderer_attempt`.

**Architecture:** M14.2 stays below Composer tools. The `sandbox` package owns attempt workspace paths, file limits, snapshot construction, validation, and DB persistence through a small repository interface. A minimal `sandbox-image/remotion-agent-runtime/src/validate.mjs` mirrors the static checks as a future runtime entrypoint, while full Remotion compile/render remains M14.3.

**Tech Stack:** Go 1.26, pgx v5 generated DB package from M14.1, OpenSandbox `Client` interface, JSON schema-style validation in Go, SHA-256 hashes, Node syntax check for the future runtime validator script.

---

## Stage Boundary

This plan covers only M14.2 from `docs/milestones/m14-agent-authored-remotion-renderer.md`.

Do not start M14.3 runtime render work until these checks pass in the current worktree:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/sandbox ./internal/agent/tools ./internal/agent/composer
node --check sandbox-image/remotion-agent-runtime/src/validate.mjs
git diff --check
```

M14.2 may create `sandbox-image/remotion-agent-runtime/src/validate.mjs`, but must not create `render.mjs`, `harness.tsx`, or a render job path. Those belong to M14.3.

## File Map

- Create: `apps/server/internal/sandbox/agent_remotion_workspace.go`
  - Defines `/workspace/agent-remotion/<renderer_artifact_id>/<attempt_no>/`.
  - Normalizes generated file names.
  - Writes initial renderer files and `props.json` to sandbox.
  - Reads attempt files back into a bounded snapshot.
  - Computes source and props hashes.
- Create: `apps/server/internal/sandbox/agent_remotion_workspace_test.go`
  - Covers path normalization, path escape rejection, file count limit, file size limit, write commands, read snapshot, and hash changes.
- Create: `apps/server/internal/sandbox/agent_remotion_validator.go`
  - Validates props JSON and generated TS/TSX files.
  - Enforces import whitelist and forbidden API checks.
  - Returns structured errors/warnings with file, line, code, and message.
- Create: `apps/server/internal/sandbox/agent_remotion_validator_test.go`
  - Covers valid fixture, illegal import, Node builtin, network API, external URL, eval/Function, dynamic import, invalid props JSON, file limits, and hash updates.
- Create: `apps/server/internal/sandbox/agent_remotion_persist.go`
  - Persists validated snapshot/hash/result to `remotion_renderer_attempt` through `UpdateRemotionRendererAttemptSnapshot`.
- Create: `apps/server/internal/sandbox/agent_remotion_persist_test.go`
  - Covers successful DB update payload and failure propagation.
- Create: `sandbox-image/remotion-agent-runtime/src/validate.mjs`
  - Static validator CLI/syntax entrypoint for later sandbox runtime.
  - This phase only requires `node --check`; execution wiring waits for M14.3.

## Public Contracts

Add these Go contracts in `apps/server/internal/sandbox/agent_remotion_workspace.go`:

```go
const (
	AgentRemotionDir              = "/workspace/agent-remotion"
	AgentRemotionPropsFile        = "props.json"
	DefaultAgentRemotionMaxFiles  = 16
	DefaultAgentRemotionMaxBytes  = 512 << 10
	DefaultAgentRemotionMaxFileBytes = 128 << 10
)

type AgentRemotionWorkspaceInput struct {
	RendererArtifactID string
	AttemptNo          int32
	Files              map[string]string
	PropsJSON          []byte
}

type AgentRemotionFile struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	SizeBytes int64  `json:"size_bytes"`
}

type AgentRemotionSnapshot struct {
	WorkspaceDir   string               `json:"workspace_dir"`
	Files          []AgentRemotionFile  `json:"files"`
	PropsJSON      []byte               `json:"props_json"`
	SourceHash     string               `json:"source_hash"`
	PropsHash      string               `json:"props_hash"`
	SourceSnapshot map[string]any       `json:"source_snapshot"`
}

func AgentRemotionAttemptDir(rendererArtifactID string, attemptNo int32) (string, error)
func NormalizeAgentRemotionRelativePath(value string) (string, error)
func BuildAgentRemotionSnapshot(workspaceDir string, files map[string]string, propsJSON []byte) (AgentRemotionSnapshot, error)
func WriteAgentRemotionAttemptWorkspace(ctx context.Context, client Client, sandboxID string, input AgentRemotionWorkspaceInput) (AgentRemotionSnapshot, error)
func ReadAgentRemotionAttemptWorkspace(ctx context.Context, client Client, sandboxID string, workspaceDir string) (AgentRemotionSnapshot, error)
```

Add these validation contracts in `apps/server/internal/sandbox/agent_remotion_validator.go`:

```go
type AgentRemotionValidationIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

type AgentRemotionValidationResult struct {
	Passed     bool                             `json:"passed"`
	Errors     []AgentRemotionValidationIssue   `json:"errors"`
	Warnings   []AgentRemotionValidationIssue   `json:"warnings"`
	SourceHash string                           `json:"source_hash"`
	PropsHash  string                           `json:"props_hash"`
}

func ValidateAgentRemotionSnapshot(snapshot AgentRemotionSnapshot) AgentRemotionValidationResult
```

Add this persistence contract in `apps/server/internal/sandbox/agent_remotion_persist.go`:

```go
type AgentRemotionAttemptRepository interface {
	UpdateRemotionRendererAttemptSnapshot(ctx context.Context, arg db.UpdateRemotionRendererAttemptSnapshotParams) (db.RemotionRendererAttempt, error)
}

func PersistAgentRemotionValidation(ctx context.Context, repo AgentRemotionAttemptRepository, attemptID pgtype.UUID, snapshot AgentRemotionSnapshot, result AgentRemotionValidationResult) (db.RemotionRendererAttempt, error)
```

## Tasks

### Task 1: RED Tests for Attempt Workspace Paths and Snapshots

**Files:**
- Create: `apps/server/internal/sandbox/agent_remotion_workspace_test.go`

- [ ] **Step 1: Add path normalization tests**

Create tests:

```go
func TestAgentRemotionAttemptDirBuildsStableWorkspacePath(t *testing.T) {
	got, err := AgentRemotionAttemptDir("3f2f72c8-7ac7-4e30-b2f1-e0f7d61ef111", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace/agent-remotion/3f2f72c8-7ac7-4e30-b2f1-e0f7d61ef111/2" {
		t.Fatalf("dir = %q", got)
	}
}

func TestAgentRemotionAttemptDirRejectsUnsafeSegments(t *testing.T) {
	for _, input := range []string{"", "../x", "x/y", ".hidden"} {
		if _, err := AgentRemotionAttemptDir(input, 1); err == nil {
			t.Fatalf("expected unsafe renderer id %q to fail", input)
		}
	}
	if _, err := AgentRemotionAttemptDir("artifact", 0); err == nil {
		t.Fatal("expected attempt_no <= 0 to fail")
	}
}

func TestNormalizeAgentRemotionRelativePathRejectsEscape(t *testing.T) {
	for _, input := range []string{"", "../x.tsx", "/workspace/x.tsx", "nested/../x.tsx", "node_modules/x.ts", "package.json"} {
		if _, err := NormalizeAgentRemotionRelativePath(input); err == nil {
			t.Fatalf("expected %q to fail", input)
		}
	}
}
```

- [ ] **Step 2: Add snapshot limit and hash tests**

Append:

```go
func TestBuildAgentRemotionSnapshotHashesSortedFilesAndProps(t *testing.T) {
	snapshot, err := BuildAgentRemotionSnapshot("/workspace/agent-remotion/artifact/1", map[string]string{
		"GeneratedComposition.tsx": `import React from "react"; export default function Video(){ return <div/>; }`,
		"styles.ts":               `export const color = "#fff";`,
	}, []byte(`{"output":{"fps":30,"duration_sec":5}}`))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SourceHash == "" || snapshot.PropsHash == "" {
		t.Fatalf("missing hashes: %#v", snapshot)
	}
	if len(snapshot.Files) != 2 || snapshot.Files[0].Path != "GeneratedComposition.tsx" || snapshot.Files[1].Path != "styles.ts" {
		t.Fatalf("files not sorted: %#v", snapshot.Files)
	}
}

func TestBuildAgentRemotionSnapshotRejectsOversizedInputs(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < DefaultAgentRemotionMaxFiles+1; i++ {
		files[fmt.Sprintf("file%d.ts", i)] = "export const x = 1;"
	}
	if _, err := BuildAgentRemotionSnapshot("/workspace/agent-remotion/artifact/1", files, []byte(`{}`)); err == nil {
		t.Fatal("expected too many files to fail")
	}
	large := strings.Repeat("x", DefaultAgentRemotionMaxFileBytes+1)
	if _, err := BuildAgentRemotionSnapshot("/workspace/agent-remotion/artifact/1", map[string]string{"GeneratedComposition.tsx": large}, []byte(`{}`)); err == nil {
		t.Fatal("expected oversized file to fail")
	}
}
```

- [ ] **Step 3: Run RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'TestAgentRemotion|TestBuildAgentRemotion' -count=1
```

Expected: FAIL because the new functions do not exist.

### Task 2: Implement Workspace Snapshot Helpers

**Files:**
- Create: `apps/server/internal/sandbox/agent_remotion_workspace.go`

- [ ] **Step 1: Implement path and snapshot functions**

Implement the public contracts from this plan. Use:

```go
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
```

Rules:

- Relative files must end in `.ts`, `.tsx`, or `.json`.
- `props.json` is reserved and comes from `PropsJSON`.
- Relative files cannot contain `..`, absolute paths, `node_modules`, `.git`, or empty path segments.
- Snapshot files must be sorted by path before hashing.
- `SourceSnapshot` should be JSON-like data:

```go
map[string]any{
	"files": filesByPath,
	"file_count": len(filesByPath),
}
```

- [ ] **Step 2: Implement sandbox write/read helpers**

Use `client.Exec` to create the attempt directory:

```go
mkdir -p '<workspaceDir>'
```

Upload each normalized file to `workspaceDir + "/" + relativePath`. Upload props to `workspaceDir + "/props.json"`.

For readback, use:

```go
find '<workspaceDir>' -maxdepth 3 -type f | sort
```

Then `Download` every listed file, reject paths outside the attempt dir, treat `props.json` separately, and call `BuildAgentRemotionSnapshot`.

- [ ] **Step 3: Run workspace tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'TestAgentRemotion|TestBuildAgentRemotion' -count=1
```

Expected: PASS.

### Task 3: RED Tests for Static Validator

**Files:**
- Create: `apps/server/internal/sandbox/agent_remotion_validator_test.go`

- [ ] **Step 1: Add valid fixture test**

Create:

```go
func TestValidateAgentRemotionSnapshotAcceptsSafeRenderer(t *testing.T) {
	snapshot := validAgentRemotionSnapshot(t, map[string]string{
		"GeneratedComposition.tsx": `import React from "react";
import {AbsoluteFill, Img, staticFile} from "remotion";

export default function Video(props) {
  return <AbsoluteFill><Img src={staticFile(props.asset_manifest[0].path)} /></AbsoluteFill>;
}`,
	}, []byte(`{"output":{"width":1080,"height":1920,"fps":30,"duration_sec":6},"asset_manifest":[{"path":"input/product.png"}]}`))
	result := ValidateAgentRemotionSnapshot(snapshot)
	if !result.Passed || len(result.Errors) != 0 {
		t.Fatalf("validation failed: %#v", result)
	}
	if result.SourceHash != snapshot.SourceHash || result.PropsHash != snapshot.PropsHash {
		t.Fatalf("hashes not propagated: %#v", result)
	}
}
```

- [ ] **Step 2: Add unsafe fixture tests**

Append table tests where each fixture must fail with a specific code:

```go
cases := []struct {
	name string
	code string
	body string
}{
	{"fs import", "forbidden_import", `import fs from "fs"; export default function Video(){ return null; }`},
	{"node builtin", "forbidden_import", `import cp from "node:child_process"; export default function Video(){ return null; }`},
	{"dynamic import", "dynamic_import", `export default async function Video(){ await import("fs"); return null; }`},
	{"require", "require_call", `const fs = require("fs"); export default function Video(){ return null; }`},
	{"fetch", "network_api", `export default function Video(){ fetch("https://example.com"); return null; }`},
	{"external url", "external_url", `export default function Video(){ return "https://example.com/a.png"; }`},
	{"eval", "eval_call", `export default function Video(){ eval("1+1"); return null; }`},
	{"function constructor", "function_constructor", `export default function Video(){ return Function("return 1")(); }`},
}
```

Also add:

```go
func TestValidateAgentRemotionSnapshotRejectsInvalidPropsJSON(t *testing.T)
func TestValidateAgentRemotionSnapshotRequiresGeneratedComposition(t *testing.T)
```

- [ ] **Step 3: Run RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'TestValidateAgentRemotion' -count=1
```

Expected: FAIL because validator does not exist.

### Task 4: Implement Static Validator

**Files:**
- Create: `apps/server/internal/sandbox/agent_remotion_validator.go`

- [ ] **Step 1: Implement structured validation result**

Implement `ValidateAgentRemotionSnapshot` with:

- Props JSON parse with `encoding/json`.
- Required object root.
- Required `output` object.
- Required `GeneratedComposition.tsx` file.
- Allowed imports:
  - `react`
  - `remotion`
  - `./...`
  - `../runtime/safe`
- Forbidden imports:
  - `fs`, `path`, `os`, `child_process`, `net`, `tls`, `http`, `https`, `crypto`, `process`
  - any `node:` builtin.
- Forbidden code tokens:
  - `import(`
  - `require(`
  - `fetch(`
  - `XMLHttpRequest`
  - `WebSocket`
  - `eval(`
  - `Function(`
  - `new Function`
  - `http://` or `https://`

- [ ] **Step 2: Add line-aware issue detection**

Scan files line by line. For each error, emit:

```go
AgentRemotionValidationIssue{
	Severity: "error",
	Code:     "forbidden_import",
	File:     file.Path,
	Line:     lineNo,
	Message:  "import fs is not allowed in agent Remotion renderer",
}
```

Only set `Passed=true` when there are no errors.

- [ ] **Step 3: Run validator tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'TestValidateAgentRemotion' -count=1
```

Expected: PASS.

### Task 5: Persist Validation Snapshot to DB Attempt

**Files:**
- Create: `apps/server/internal/sandbox/agent_remotion_persist_test.go`
- Create: `apps/server/internal/sandbox/agent_remotion_persist.go`

- [ ] **Step 1: Add RED persistence test**

Create:

```go
func TestPersistAgentRemotionValidationUpdatesAttemptSnapshot(t *testing.T) {
	repo := &fakeAgentRemotionAttemptRepo{}
	attemptID := pgtype.UUID{Bytes: [16]byte{0x42}, Valid: true}
	snapshot := mustAgentRemotionSnapshot(t)
	result := ValidateAgentRemotionSnapshot(snapshot)

	_, err := PersistAgentRemotionValidation(context.Background(), repo, attemptID, snapshot, result)
	if err != nil {
		t.Fatal(err)
	}
	arg := repo.arg
	if arg.ID != attemptID || arg.SourceHash != snapshot.SourceHash || arg.PropsHash != snapshot.PropsHash {
		t.Fatalf("unexpected update arg: %#v", arg)
	}
	if arg.Status != "validated" {
		t.Fatalf("status = %q, want validated", arg.Status)
	}
	if len(arg.SourceSnapshot) == 0 || len(arg.PropsJson) == 0 || len(arg.ValidationResult) == 0 || len(arg.CompileResult) == 0 {
		t.Fatalf("missing persisted payloads: %#v", arg)
	}
}
```

Also test invalid validation result sets status `validation_failed`.

- [ ] **Step 2: Implement persistence helper**

Implement:

```go
func PersistAgentRemotionValidation(ctx context.Context, repo AgentRemotionAttemptRepository, attemptID pgtype.UUID, snapshot AgentRemotionSnapshot, result AgentRemotionValidationResult) (db.RemotionRendererAttempt, error)
```

Status mapping:

- `validated` when `result.Passed`.
- `validation_failed` otherwise.

`CompileResult` for M14.2 should be:

```json
{"status":"not_run","phase":"m14_2_static_validation"}
```

M14.3 replaces this with actual compile results.

- [ ] **Step 3: Run persistence tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'TestPersistAgentRemotion' -count=1
```

Expected: PASS.

### Task 6: Add Runtime Validator Script Syntax Gate

**Files:**
- Create: `sandbox-image/remotion-agent-runtime/src/validate.mjs`

- [ ] **Step 1: Add minimal validator CLI module**

Create `validate.mjs`:

```js
#!/usr/bin/env node

import {readFile} from "node:fs/promises";

const forbiddenPatterns = [
  ["dynamic_import", /import\s*\(/],
  ["require_call", /require\s*\(/],
  ["network_api", /\b(fetch|XMLHttpRequest|WebSocket)\b/],
  ["eval_call", /\beval\s*\(/],
  ["function_constructor", /\bnew\s+Function\b|\bFunction\s*\(/],
  ["external_url", /https?:\/\//],
];

export function validateSourceText(file, text) {
  const errors = [];
  const lines = String(text).split(/\r?\n/);
  for (let i = 0; i < lines.length; i += 1) {
    for (const [code, pattern] of forbiddenPatterns) {
      if (pattern.test(lines[i])) {
        errors.push({severity: "error", code, file, line: i + 1});
      }
    }
  }
  return {passed: errors.length === 0, errors};
}

export async function validateFile(file) {
  const text = await readFile(file, "utf8");
  return validateSourceText(file, text);
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const file = process.argv[2];
  if (!file) {
    console.error("usage: validate.mjs <file>");
    process.exit(2);
  }
  const result = await validateFile(file);
  console.log(JSON.stringify(result));
  process.exit(result.passed ? 0 : 1);
}
```

- [ ] **Step 2: Run syntax gate**

Run:

```bash
node --check sandbox-image/remotion-agent-runtime/src/validate.mjs
```

Expected: exits 0.

### Task 7: Run M14.2 Stage Gate

**Files:**
- Inspect all M14.2 code and verify no M14.3 render path exists.

- [ ] **Step 1: Run required Go tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/sandbox ./internal/agent/tools ./internal/agent/composer
```

Expected: exits 0.

- [ ] **Step 2: Run Node syntax gate**

Run:

```bash
node --check sandbox-image/remotion-agent-runtime/src/validate.mjs
```

Expected: exits 0.

- [ ] **Step 3: Run whitespace check**

Run:

```bash
git diff --check
```

Expected: exits 0.

- [ ] **Step 4: Stage audit**

Inspect:

```bash
git status --short
rg -n "render_agent_remotion_renderer|RenderAgentRemotion|render.mjs|harness.tsx" apps/server/internal sandbox-image/remotion-agent-runtime
```

Acceptance requires:

- Attempt workspace path is fixed at `/workspace/agent-remotion/<renderer_artifact_id>/<attempt_no>/`.
- Path escapes and unsafe relative paths are rejected.
- File count and size limits are enforced.
- Static validator returns structured errors with code/file/line/message.
- Illegal imports, Node builtins, network APIs, dynamic import, external URL, eval, and Function constructor are rejected.
- Valid fixture passes.
- Snapshot, props, source hash, props hash, validation result, and placeholder compile result can be persisted to DB attempt.
- No M14.3 renderer, harness, or render job is added yet.

## Self-Review

Spec coverage:

- Sandbox attempt workspace helper: Tasks 1 and 2.
- File tree read/write and limits: Tasks 1 and 2.
- Static validator: Tasks 3 and 4.
- Structured errors/warnings: Task 4.
- Snapshot/hash/result persistence: Task 5.
- Runtime validator syntax gate: Task 6.
- M14.2 verification and stop condition: Task 7.

Placeholder scan:

- This plan intentionally contains no deferred implementation placeholders.

Type consistency:

- `AgentRemotionSnapshot` is the object passed from workspace helper to validator and persistence helper.
- `PersistAgentRemotionValidation` maps to M14.1 generated `UpdateRemotionRendererAttemptSnapshotParams`.
- M14.3 compile/render fields remain explicit `not_run` placeholders in persisted JSON, not hidden behavior.
