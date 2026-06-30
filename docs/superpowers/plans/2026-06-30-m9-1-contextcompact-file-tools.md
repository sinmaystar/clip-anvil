# M9.1 ContextCompact Foundation And Sandbox File Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the M9.1 foundation: shared context compaction config/token/media/planner primitives plus reusable sandbox `read_file` and `edit_file` native tools registered for Producer, Craftsman, Reviewer, and Composer.

**Architecture:** M9.1 is deliberately read-only for model prompts: it records token estimates and compaction candidates but does not rewrite messages. Sandbox file access is implemented as a small service over the existing workspace sandbox manager/client, exposed through native Agent tools. Four Agent registries receive the same file tools, while message-list UI remains untouched because no `agent_message` content is modified.

**Tech Stack:** Go 1.26, Eino `schema.Message` / native tool registry, OpenSandbox client `Upload` / `Download`, pgx `pgtype.UUID`, `github.com/pkoukk/tiktoken-go`, existing `make server-test` validation.

---

## Source Documents

- `docs/superpowers/specs/2026-06-30-agent-context-compaction-design.md`
- `docs/milestones/m9-agent-context-compaction.md`
- `apps/server/internal/agent/tools/native.go`
- `apps/server/internal/sandbox/client.go`
- `apps/server/internal/sandbox/workspace.go`
- `apps/server/cmd/server/main.go`

## M9.1 Boundaries

In scope:

- Add `apps/server/internal/agent/contextcompact`.
- Add config, token counter, media card skeleton, planner diagnostics.
- Add sandbox text file service.
- Add `read_file` and `edit_file` native tools.
- Register both tools for Producer, Craftsman, Reviewer, and Composer.
- Add focused unit tests.

Out of scope:

- No `agent_context_compaction` DB tables.
- No prompt rewrite.
- No `search_agent_history`.
- No full compact.
- No UI changes.
- No changes to Agent message list projection.

## File Map

- Create `apps/server/internal/agent/contextcompact/config.go`: config structs, defaults, validation.
- Create `apps/server/internal/agent/contextcompact/token_counter.go`: `TokenCounter` interface and tiktoken-backed implementation with fallback.
- Create `apps/server/internal/agent/contextcompact/media_cards.go`: media-ref and media-card data structures used by the planner.
- Create `apps/server/internal/agent/contextcompact/planner.go`: read-only candidate planner and diagnostics.
- Create `apps/server/internal/agent/contextcompact/*_test.go`: M9.1 unit tests.
- Create `apps/server/internal/sandbox/files.go`: path validation and text read/write/edit service.
- Create `apps/server/internal/sandbox/files_test.go`: sandbox file path and edit behavior tests.
- Create `apps/server/internal/agent/tools/sandbox_file_tools.go`: `read_file` and `edit_file` native tools.
- Create `apps/server/internal/agent/tools/sandbox_file_tools_test.go`: native tool schema/runtime tests.
- Modify `apps/server/internal/config/config.go`: add `Agent.ContextCompaction` config and env bindings.
- Modify `apps/server/cmd/server/main.go`: construct file tools and register them in all four native registries.
- Modify `apps/server/internal/agent/tools/composer_tools_test.go`: include `read_file` and `edit_file` in expected Composer tools.
- Add or modify a registry-focused test for Producer/Craftsman/Reviewer tool registration if the current suite lacks one.
- Modify `docs/milestones/m9-agent-context-compaction.md`: add the M9.1 plan link and leave status as planning until implementation passes.

## Task 1: Add Context Compaction Config

**Files:**
- Modify: `apps/server/internal/config/config.go`
- Test: `apps/server/internal/config/config_test.go`
- Create: `apps/server/internal/agent/contextcompact/config.go`
- Test: `apps/server/internal/agent/contextcompact/config_test.go`

- [ ] **Step 1: Write config tests**

Add tests that prove defaults and env binding work:

```go
func TestAgentContextCompactionDefaults(t *testing.T) {
	cfg := contextcompact.DefaultConfig()
	if !cfg.Enabled {
		t.Fatal("Enabled = false, want true")
	}
	if cfg.ModelContextWindowTokens != 256000 {
		t.Fatalf("ModelContextWindowTokens = %d", cfg.ModelContextWindowTokens)
	}
	if cfg.MicroTriggerTokens != 180000 || cfg.MicroTargetTokens != 150000 {
		t.Fatalf("micro thresholds = %d -> %d", cfg.MicroTriggerTokens, cfg.MicroTargetTokens)
	}
	if cfg.FullTriggerTokens != 200000 || cfg.FullTargetTokens != 140000 {
		t.Fatalf("full thresholds = %d -> %d", cfg.FullTriggerTokens, cfg.FullTargetTokens)
	}
}
```

Add a config loader test in `apps/server/internal/config/config_test.go` using the existing style in that file:

```go
if !cfg.Agent.ContextCompaction.Enabled {
	t.Fatal("Agent.ContextCompaction.Enabled = false, want true")
}
if cfg.Agent.ContextCompaction.MicroTriggerTokens != 180000 {
	t.Fatalf("MicroTriggerTokens = %d", cfg.Agent.ContextCompaction.MicroTriggerTokens)
}
```

- [ ] **Step 2: Run failing config tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/config ./internal/agent/contextcompact -run 'ContextCompaction|AgentContext' -count=1
```

Expected: FAIL because `contextcompact` and `Agent.ContextCompaction` do not exist yet.

- [ ] **Step 3: Implement config**

Create `apps/server/internal/agent/contextcompact/config.go`:

```go
package contextcompact

type Config struct {
	Enabled                    bool
	ModelContextWindowTokens   int
	MicroTriggerTokens         int
	MicroTargetTokens          int
	MicroMinReductionTokens    int
	FullTriggerTokens          int
	FullTargetTokens           int
	PreserveRecentUserMessages int
	PreserveRecentTotalMessages int
	SearchMaxResults           int
	MediaImageInputTokenWeight int
	MediaCardMaxChars          int
}

func DefaultConfig() Config {
	return Config{
		Enabled:                    true,
		ModelContextWindowTokens:   256000,
		MicroTriggerTokens:         180000,
		MicroTargetTokens:          150000,
		MicroMinReductionTokens:    8000,
		FullTriggerTokens:          200000,
		FullTargetTokens:           140000,
		PreserveRecentUserMessages: 6,
		PreserveRecentTotalMessages: 40,
		SearchMaxResults:           50,
		MediaImageInputTokenWeight: 1500,
		MediaCardMaxChars:          1200,
	}
}

func (c Config) WithDefaults() Config {
	defaults := DefaultConfig()
	if c.ModelContextWindowTokens <= 0 {
		c.ModelContextWindowTokens = defaults.ModelContextWindowTokens
	}
	if c.MicroTriggerTokens <= 0 {
		c.MicroTriggerTokens = defaults.MicroTriggerTokens
	}
	if c.MicroTargetTokens <= 0 {
		c.MicroTargetTokens = defaults.MicroTargetTokens
	}
	if c.MicroMinReductionTokens <= 0 {
		c.MicroMinReductionTokens = defaults.MicroMinReductionTokens
	}
	if c.FullTriggerTokens <= 0 {
		c.FullTriggerTokens = defaults.FullTriggerTokens
	}
	if c.FullTargetTokens <= 0 {
		c.FullTargetTokens = defaults.FullTargetTokens
	}
	if c.PreserveRecentUserMessages <= 0 {
		c.PreserveRecentUserMessages = defaults.PreserveRecentUserMessages
	}
	if c.PreserveRecentTotalMessages <= 0 {
		c.PreserveRecentTotalMessages = defaults.PreserveRecentTotalMessages
	}
	if c.SearchMaxResults <= 0 {
		c.SearchMaxResults = defaults.SearchMaxResults
	}
	if c.MediaImageInputTokenWeight <= 0 {
		c.MediaImageInputTokenWeight = defaults.MediaImageInputTokenWeight
	}
	if c.MediaCardMaxChars <= 0 {
		c.MediaCardMaxChars = defaults.MediaCardMaxChars
	}
	return c
}
```

Modify `apps/server/internal/config/config.go`:

```go
type AgentConfig struct {
	ProducerMaxToolCalls int `mapstructure:"producer_max_tool_calls"`
	ToolTimeoutSeconds   int `mapstructure:"tool_timeout_seconds"`
	ContextCompaction    contextcompact.Config `mapstructure:"context_compaction"`
}
```

Import:

```go
import contextcompact "github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
```

After existing Agent defaults in `Load()`:

```go
cfg.Agent.ContextCompaction = cfg.Agent.ContextCompaction.WithDefaults()
```

Add env keys:

```go
"agent.context_compaction.enabled",
"agent.context_compaction.model_context_window_tokens",
"agent.context_compaction.micro_trigger_tokens",
"agent.context_compaction.micro_target_tokens",
"agent.context_compaction.micro_min_reduction_tokens",
"agent.context_compaction.full_trigger_tokens",
"agent.context_compaction.full_target_tokens",
"agent.context_compaction.preserve_recent_user_messages",
"agent.context_compaction.preserve_recent_total_messages",
"agent.context_compaction.search_max_results",
"agent.context_compaction.media_image_input_token_weight",
"agent.context_compaction.media_card_max_chars",
```

- [ ] **Step 4: Run config tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/config ./internal/agent/contextcompact -run 'ContextCompaction|AgentContext' -count=1
```

Expected: PASS.

## Task 2: Add Token Counter, Media Cards, And Read-Only Planner

**Files:**
- Create: `apps/server/internal/agent/contextcompact/token_counter.go`
- Create: `apps/server/internal/agent/contextcompact/media_cards.go`
- Create: `apps/server/internal/agent/contextcompact/planner.go`
- Test: `apps/server/internal/agent/contextcompact/token_counter_test.go`
- Test: `apps/server/internal/agent/contextcompact/planner_test.go`
- Modify: `apps/server/go.mod`
- Modify: `apps/server/go.sum`

- [ ] **Step 1: Add dependency**

Run:

```bash
cd apps/server && go get github.com/pkoukk/tiktoken-go@latest
```

Expected: `apps/server/go.mod` includes `github.com/pkoukk/tiktoken-go`.

- [ ] **Step 2: Write token counter tests**

Add tests:

```go
func TestTokenCounterCountsMessagesAndTools(t *testing.T) {
	counter := contextcompact.NewTokenCounter()
	result, err := counter.CountMessages(context.Background(), contextcompact.CountMessagesInput{
		ModelID: "gpt-4o",
		Messages: []*schema.Message{
			schema.SystemMessage("You are Producer."),
			schema.UserMessage("Create a 15s suitcase ad."),
		},
		ToolInfos: []*schema.ToolInfo{{Name: "read_project_context", Desc: "读取项目上下文。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalTokens <= 0 {
		t.Fatalf("TotalTokens = %d, want > 0", result.TotalTokens)
	}
	if result.MessageTokens <= 0 || result.ToolTokens <= 0 {
		t.Fatalf("message/tool tokens = %d/%d", result.MessageTokens, result.ToolTokens)
	}
}

func TestTokenCounterFallsBackForUnknownModel(t *testing.T) {
	counter := contextcompact.NewTokenCounter()
	result, err := counter.CountMessages(context.Background(), contextcompact.CountMessagesInput{
		ModelID: "doubao-seed-2-0-mini-260428",
		Messages: []*schema.Message{schema.UserMessage(strings.Repeat("长", 200))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalTokens <= 0 {
		t.Fatal("fallback counter returned zero tokens")
	}
	if result.EstimatedBy != "heuristic" && result.EstimatedBy != "tiktoken" {
		t.Fatalf("EstimatedBy = %q", result.EstimatedBy)
	}
}
```

- [ ] **Step 3: Write planner tests**

Add tests:

```go
func TestPlannerRecordsCandidatesWithoutRewritingMessages(t *testing.T) {
	planner := contextcompact.NewPlanner(contextcompact.DefaultConfig())
	msgs := []*schema.Message{
		schema.UserMessage("latest instruction"),
		schema.AssistantMessage(strings.Repeat("old tool output ", 1000), nil),
	}
	out, err := planner.Plan(context.Background(), contextcompact.PlanInput{
		Role:     "producer",
		ModelID:  "gpt-4o",
		Messages: msgs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != len(msgs) || out.Messages[1].Content != msgs[1].Content {
		t.Fatal("M9.1 planner must not rewrite messages")
	}
	if out.TokenBefore <= 0 {
		t.Fatalf("TokenBefore = %d", out.TokenBefore)
	}
}
```

- [ ] **Step 4: Run failing contextcompact tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact -run 'TokenCounter|Planner' -count=1
```

Expected: FAIL because token counter and planner files are missing.

- [ ] **Step 5: Implement token counter and planner**

Create small focused types:

```go
type CountMessagesInput struct {
	ModelID    string
	Messages   []*schema.Message
	ToolInfos  []*schema.ToolInfo
	MediaCards []MediaCard
}

type CountMessagesResult struct {
	TotalTokens   int
	MessageTokens int
	ToolTokens    int
	MediaTokens   int
	EstimatedBy   string
}

type PlanInput struct {
	Role      string
	ModelID   string
	Messages  []*schema.Message
	ToolInfos []*schema.ToolInfo
}

type PlanOutput struct {
	Messages   []*schema.Message
	TokenBefore int
	Candidates []Candidate
}
```

Use tiktoken when it recognizes the model. For unknown models, use a conservative fallback:

```go
func heuristicTokens(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	return max(1, len([]rune(s))/3)
}
```

M9.1 `Planner.Plan` must return the original `[]*schema.Message` slice content unchanged. Candidate selection can be minimal: identify long assistant/tool-like messages and report estimated savings without applying them.

- [ ] **Step 6: Run contextcompact tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact -count=1
```

Expected: PASS.

## Task 3: Add Sandbox Text File Service

**Files:**
- Create: `apps/server/internal/sandbox/files.go`
- Test: `apps/server/internal/sandbox/files_test.go`

- [ ] **Step 1: Write sandbox file service tests**

Cover these tests:

```go
func TestValidateWorkspaceTextPathRejectsEscape(t *testing.T) {
	for _, input := range []string{"", "../secret", "/tmp/x", "/workspace/../etc/passwd"} {
		if _, err := sandbox.ValidateWorkspaceTextPath(input); err == nil {
			t.Fatalf("ValidateWorkspaceTextPath(%q) returned nil error", input)
		}
	}
}

func TestValidateWorkspaceTextPathAcceptsWorkspacePath(t *testing.T) {
	got, err := sandbox.ValidateWorkspaceTextPath("/workspace/.clipanvil/notes/plan.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace/.clipanvil/notes/plan.md" {
		t.Fatalf("path = %q", got)
	}
}

func TestApplyTextEditReplaceRequiresUniqueMatch(t *testing.T) {
	_, err := sandbox.ApplyTextEdit("one one", sandbox.TextEditInput{Mode: "replace", OldText: "one", NewText: "two"})
	if err == nil {
		t.Fatal("expected non-unique old_text to fail")
	}
}
```

- [ ] **Step 2: Run failing sandbox tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'WorkspaceTextPath|ApplyTextEdit' -count=1
```

Expected: FAIL because file helpers do not exist.

- [ ] **Step 3: Implement sandbox file helpers**

Create `files.go` with:

```go
type TextEditInput struct {
	Mode    string
	Content string
	OldText string
	NewText string
}

func ValidateWorkspaceTextPath(input string) (string, error)
func ApplyTextEdit(existing string, input TextEditInput) (string, error)
```

Rules:

- Clean paths with `path.Clean`.
- Require cleaned path to equal `/workspace/<file>` and not `/workspace`.
- Reject paths not under `/workspace`.
- Reject `..` segments after cleaning.
- `replace` fails when `old_text` is empty, missing, or appears more than once.
- `create`, `create_or_overwrite`, `append`, `replace` are the only modes.

- [ ] **Step 4: Run sandbox tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'WorkspaceTextPath|ApplyTextEdit' -count=1
```

Expected: PASS.

## Task 4: Add read_file And edit_file Native Tools

**Files:**
- Create: `apps/server/internal/agent/tools/sandbox_file_tools.go`
- Test: `apps/server/internal/agent/tools/sandbox_file_tools_test.go`

- [ ] **Step 1: Write native tool tests**

Use a fake sandbox manager and fake sandbox client. Cover:

```go
func TestReadFileRequiresRuntimeWorkspace(t *testing.T)
func TestReadFileReturnsChunkAndNextOffset(t *testing.T)
func TestEditFileCreateOrOverwriteUploadsContent(t *testing.T)
func TestEditFileAppendDownloadsThenUploads(t *testing.T)
func TestEditFileReplaceRejectsAmbiguousOldText(t *testing.T)
func TestSandboxFileToolInfosUseTypedSchemas(t *testing.T)
```

Expected output for a truncated read should include JSON fields:

```json
{
  "path": "/workspace/.clipanvil/context/large.md",
  "content": "first chunk",
  "offset": 0,
  "next_offset": 100,
  "truncated": true
}
```

- [ ] **Step 2: Run failing native tool tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'ReadFile|EditFile|SandboxFile' -count=1
```

Expected: FAIL because tools do not exist.

- [ ] **Step 3: Implement tool interfaces and constructors**

Add constructor functions:

```go
func NewReadFileNativeTool(manager SandboxEnsurer, client sandbox.Client) *ReadFileNativeTool
func NewEditFileNativeTool(manager SandboxEnsurer, client sandbox.Client) *EditFileNativeTool
```

Use a local interface compatible with `sandbox.Manager`:

```go
type SandboxEnsurer interface {
	EnsureSandbox(ctx context.Context, workspaceID pgtype.UUID) (sandbox.WorkspaceSandbox, error)
}
```

Input structs:

```go
type ReadFileInput struct {
	Path   string `json:"path" jsonschema:"required,description=Sandbox path under /workspace."`
	Offset int64  `json:"offset,omitempty" jsonschema:"description=Byte offset to start reading from."`
	Limit  int64  `json:"limit,omitempty" jsonschema:"description=Maximum bytes to return. Default 20000, max 200000."`
}

type EditFileInput struct {
	Path    string `json:"path" jsonschema:"required"`
	Mode    string `json:"mode" jsonschema:"required,enum=create,enum=create_or_overwrite,enum=append,enum=replace"`
	Content string `json:"content,omitempty"`
	OldText string `json:"old_text,omitempty"`
	NewText string `json:"new_text,omitempty"`
	Reason  string `json:"reason,omitempty"`
}
```

Tool names and descriptions:

```go
const (
	toolReadFile = "read_file"
	toolEditFile = "edit_file"
)
```

Descriptions must say:

- Reads or edits UTF-8 text files inside the current workspace sandbox.
- Does not list directories; use exec shell for `ls`, `find`, `grep`, `rg`.
- Business facts must still use structured tools.

- [ ] **Step 4: Implement read_file behavior**

Implementation rules:

- Require `NativeRuntimeContext` with valid `WorkspaceID`.
- Require non-nil sandbox manager and client.
- Validate path with `sandbox.ValidateWorkspaceTextPath`.
- Ensure workspace sandbox.
- Download file using `client.Download`.
- Read content as bytes.
- Apply offset and limit.
- Default limit: `20000`.
- Max limit: `200000`.
- Return JSON with `path`, `content`, `offset`, `limit`, `bytes_total`, `next_offset`, `truncated`.

- [ ] **Step 5: Implement edit_file behavior**

Implementation rules:

- Require runtime workspace.
- Validate path.
- Ensure workspace sandbox.
- `create`: fail if `client.Download` succeeds.
- `create_or_overwrite`: upload content directly.
- `append`: download existing content if present, append content, upload.
- `replace`: download existing content, call `sandbox.ApplyTextEdit`, upload.
- Return JSON with `path`, `mode`, `bytes_written`, `reason`.

- [ ] **Step 6: Run native tool tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'ReadFile|EditFile|SandboxFile' -count=1
```

Expected: PASS.

## Task 5: Register File Tools For Four Agents

**Files:**
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/agent/tools/composer_tools_test.go`
- Test: `apps/server/cmd/server/main_test.go`

- [ ] **Step 1: Add registration tests**

Update `TestComposerNativeToolsRegisterExpectedNames` to include:

```go
"read_file",
"edit_file",
```

Add a focused server wiring test if there is no existing four-role registry coverage:

```go
func TestSandboxFileToolsCanRegisterWithNativeRegistry(t *testing.T) {
	registry, err := agenttools.NewNativeRegistry(
		agenttools.NewReadFileNativeTool(nil, nil),
		agenttools.NewEditFileNativeTool(nil, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := registry.ToolInfos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, info := range infos {
		got[info.Name] = info.ParamsOneOf != nil
	}
	for _, name := range []string{"read_file", "edit_file"} {
		if !got[name] {
			t.Fatalf("missing %s in %#v", name, got)
		}
	}
}
```

- [ ] **Step 2: Run failing registration tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./cmd/server ./internal/agent/tools -run 'SandboxFileTools|ComposerNativeToolsRegisterExpectedNames' -count=1
```

Expected: FAIL until tools are registered in Composer test and constructors compile.

- [ ] **Step 3: Register tools in `main.go`**

Create reusable constructors near `skillRegistry := agentskills.DefaultRegistry()`:

```go
newReadFileTool := func() agenttools.NativeTool {
	return agenttools.NewReadFileNativeTool(sandboxManager, sandboxClient)
}
newEditFileTool := func() agenttools.NativeTool {
	return agenttools.NewEditFileNativeTool(sandboxManager, sandboxClient)
}
```

Add `newReadFileTool()` and `newEditFileTool()` to:

- `composerNativeToolRegistry`
- Craftsman `mustNativeRegistry`
- `reviewerNativeToolRegistry`
- `producerNativeToolRegistry`

Keep `load_agent_skill` / `load_agent_skill_resource` first in each registry so skill behavior remains easy to scan.

- [ ] **Step 4: Run registration tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./cmd/server ./internal/agent/tools -run 'SandboxFileTools|ComposerNativeToolsRegisterExpectedNames' -count=1
```

Expected: PASS.

## Task 6: M9.1 Regression And Milestone Record

**Files:**
- Modify: `docs/milestones/m9-agent-context-compaction.md`

- [ ] **Step 1: Run full server tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Expected: PASS.

- [ ] **Step 2: Run diff check**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 3: Update M9 milestone**

In `docs/milestones/m9-agent-context-compaction.md`, add this implementation plan under references:

```markdown
- [M9.1 实施计划](../superpowers/plans/2026-06-30-m9-1-contextcompact-file-tools.md)
```

After implementation passes, update M9.1 status text to say M9.1 is complete and list the exact validation commands that passed.

- [ ] **Step 4: Inspect intended diff**

Run:

```bash
git status --short
git diff --stat
```

Expected: only M9.1 files, M9 docs, Go module files, and intended tests are changed.

- [ ] **Step 5: Commit M9.1 if requested**

Only commit after the user asks to publish or commit:

```bash
git add apps/server/go.mod apps/server/go.sum apps/server/internal/agent/contextcompact apps/server/internal/agent/tools apps/server/internal/config apps/server/internal/sandbox apps/server/cmd/server docs/milestones/m9-agent-context-compaction.md docs/superpowers/plans/2026-06-30-m9-1-contextcompact-file-tools.md docs/superpowers/specs/2026-06-30-agent-context-compaction-design.md
git commit -m "feat: add m9 context compaction foundation"
```

## Self-Review

Spec coverage:

- M9.1 config is covered by Task 1.
- M9.1 token counter, media card skeleton, and planner are covered by Task 2.
- Sandbox `read_file` / `edit_file` service is covered by Task 3 and Task 4.
- Four Agent registration is covered by Task 5.
- No prompt rewrite is enforced by Task 2 planner tests.
- No Agent message list mutation is preserved because M9.1 does not touch `agent_message` persistence or API projection.

Commands to verify the plan file itself:

```bash
git diff --check -- docs/superpowers/plans/2026-06-30-m9-1-contextcompact-file-tools.md
```
