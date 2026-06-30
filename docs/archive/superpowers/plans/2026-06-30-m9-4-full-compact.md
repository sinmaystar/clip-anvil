# M9.4 Full Compact Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add recoverable full-context compaction that creates a structured handoff summary before model calls, preserves recent/current context, records sidecar metadata, and supports one context-overflow retry without changing Agent chat history.

**Architecture:** Extend the existing `contextcompact.Middleware` rather than adding role-specific compactors. The middleware will micro-compact first, then run full compact if the projected prompt still exceeds `FullTriggerTokens`; full compact inserts a structured handoff summary after the system prompt and preserves recent user/current/same-turn/pending messages. Role responders provide structured facts and media cards from already loaded context; Producer gets a lightweight facts provider so full summary is not based only on `agent_message`.

**Tech Stack:** Go 1.26, Eino `schema.Message`, existing `contextcompact` package, Volcengine chat model interfaces, sqlc sidecar compaction store, sandbox detail files, `GOCACHE=/private/tmp/clipanvil-go-build make server-test`.

---

## Source Documents

- `docs/superpowers/specs/2026-06-30-agent-context-compaction-design.md`
- `docs/milestones/m9-agent-context-compaction.md`
- `docs/superpowers/plans/2026-06-30-m9-1-contextcompact-file-tools.md`
- `docs/superpowers/plans/2026-06-30-m9-2-micro-compact.md`
- `docs/superpowers/plans/2026-06-30-m9-3-four-agent-contextcompact.md`
- `apps/server/internal/agent/contextcompact/middleware.go`
- `apps/server/internal/agent/contextcompact/store.go`
- `apps/server/internal/agent/producer/model_responder.go`
- `apps/server/internal/agent/craftsman/model_responder.go`
- `apps/server/internal/agent/reviewer/model_responder.go`
- `apps/server/internal/agent/composer/model_responder.go`

## Hard Boundaries

In scope:

- Add full compact to the shared middleware.
- Add a summary runner abstraction and Volcengine-backed implementation.
- Add a deterministic fake summarizer for tests.
- Persist full compact records in the existing `agent_context_compaction` table with `mode='full'`.
- Write full handoff summary detail files under `/workspace/.clipanvil/context/`.
- Preserve recent user messages, recent total messages, same-turn tool loop, pending reminders, and current role-specific task context.
- Provide structured facts and media cards to summary input.
- Add one-time context-overflow retry support in real model responders.
- Record full compact diagnostics in output metadata.

Out of scope:

- No vector search.
- No UI changes.
- No `agent_message` mutation, deletion, truncation, or message-list substitution.
- No change to Producer pending signal claim / drain timing.
- No media-card expansion beyond structured references needed for full summary; M9.5 owns richer media cards and true E2E.
- No infinite retry loop after context overflow.

Forbidden implementation paths:

- Do not call `UpdateAgentMessage`, `UpdateMessage`, or `ListAgentMessagesByThread` from compaction code.
- Do not run full compact after appending new persisted messages for the same failed model call.
- Do not use the full summary as a business facts source; DB facts remain authoritative.
- Do not summarize Reviewer current media in place of the actual current `UserInputMultiContent`.
- Do not omit `Recovery References` from the handoff summary.

## File Map

- Modify `apps/server/internal/agent/contextcompact/middleware.go`: add full-compact trigger, preservation indexes, summary insertion, diagnostics.
- Add `apps/server/internal/agent/contextcompact/full_summary.go`: summary input/output types, summary prompt builder, markdown section validator.
- Add `apps/server/internal/agent/contextcompact/full_summary_test.go`: prompt and section validation tests.
- Add `apps/server/internal/agent/contextcompact/full_projection_test.go`: middleware full projection tests.
- Modify `apps/server/internal/agent/contextcompact/detail_file.go`: allow detail type `full_summary` in generated file names or metadata.
- Modify `apps/server/internal/agent/contextcompact/store.go`: no schema change expected, but tests must prove `mode='full'` payload and source range are saved.
- Add `apps/server/internal/agent/contextcompact/overflow.go`: provider-neutral context overflow detection.
- Add `apps/server/internal/agent/contextcompact/overflow_test.go`: error string classification tests.
- Modify `apps/server/internal/agent/producer/model_responder.go`: pass facts/media cards and wrap one retry around stream start for context overflow.
- Modify `apps/server/internal/agent/craftsman/model_responder.go`: pass facts/media cards and wrap one retry around generate.
- Modify `apps/server/internal/agent/reviewer/model_responder.go`: pass facts/media cards and wrap one retry around stream start.
- Modify `apps/server/internal/agent/composer/model_responder.go`: pass facts/media cards and wrap one retry around generate.
- Add role fact tests near each responder package.
- Modify `apps/server/internal/agent/producer/context_loader.go`: add lightweight Producer facts provider based on `pss.Builder` or a small store interface.
- Modify `apps/server/cmd/server/main.go`: construct summary runner and Producer facts loader.
- Modify `docs/milestones/m9-agent-context-compaction.md`: mark M9.4 complete after validation passes.

## Task 1: Full Summary Prompt and Validator

**Files:**
- Add: `apps/server/internal/agent/contextcompact/full_summary.go`
- Add: `apps/server/internal/agent/contextcompact/full_summary_test.go`

- [ ] **Step 1: Write failing section validation test**

Add `full_summary_test.go`:

```go
package contextcompact

import (
	"strings"
	"testing"
)

func TestValidateFullSummaryRequiresHandoffSections(t *testing.T) {
	summary := `# Compacted Agent Handoff Summary

## User Goal
Create a 15s suitcase ad.

## Confirmed Decisions
- Use airport departure mood.

## Current Project State
- storyboard: confirmed

## Media Assets
- artifact_version/shot_01.preview.r1: 未生成视觉摘要

## Shot / RenderPlan Status
- shot_01 preview is ready.

## Review Findings
- 未确认

## Audio / Timeline State
- audio plan pending.

## Pending Signals And Tasks
- producer_pending_signal/render_done pending.

## Known Failures And Avoidances
- Avoid unsupported provider parameters.

## Recent User Instructions To Preserve Verbatim
- "保持箱体颜色一致"

## Next Recommended Actions
- Producer should dispatch Reviewer.

## Recovery References
- agent_context_compaction/ctxcmp-producer-full
`
	if err := ValidateFullSummaryMarkdown(summary); err != nil {
		t.Fatalf("ValidateFullSummaryMarkdown error = %v", err)
	}
	if err := ValidateFullSummaryMarkdown(strings.Replace(summary, "## Recovery References\n- agent_context_compaction/ctxcmp-producer-full\n", "", 1)); err == nil {
		t.Fatal("expected missing Recovery References to fail")
	}
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact -run 'ValidateFullSummaryRequiresHandoffSections' -count=1
```

Expected: FAIL because `ValidateFullSummaryMarkdown` is undefined.

- [ ] **Step 3: Implement summary types, prompt builder, and validator**

Add `full_summary.go`:

```go
package contextcompact

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

var ErrInvalidFullSummary = errors.New("invalid full compact summary")

type FullSummaryFact struct {
	Ref     string
	Kind    string
	Summary string
	Source  string
}

type FullSummaryInput struct {
	Role                   string
	ModelID                string
	Messages               []*schema.Message
	Facts                  []FullSummaryFact
	MediaCards             []MediaCard
	RecentUserInstructions []string
	RecoveryRefs           []string
}

type FullSummaryOutput struct {
	Summary string
	ModelID string
}

type FullSummarizer interface {
	Summarize(ctx context.Context, input FullSummaryInput) (FullSummaryOutput, error)
}

func BuildFullSummaryPrompt(input FullSummaryInput) string {
	var b strings.Builder
	b.WriteString("Create a compacted ClipAnvil Agent handoff summary.\n")
	b.WriteString("Role: " + strings.TrimSpace(input.Role) + "\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Preserve recent user instructions verbatim.\n")
	b.WriteString("- Use DB facts and semantic refs as authoritative facts.\n")
	b.WriteString("- Do not invent visual or audio details for media.\n")
	b.WriteString("- Mark uncertain facts as 未确认.\n")
	b.WriteString("- Include all required markdown sections exactly once.\n\n")
	b.WriteString("Structured facts:\n")
	for _, fact := range input.Facts {
		b.WriteString(fmt.Sprintf("- %s [%s] source=%s: %s\n", fact.Ref, fact.Kind, fact.Source, fact.Summary))
	}
	b.WriteString("\nMedia cards:\n")
	for _, card := range input.MediaCards {
		b.WriteString(fmt.Sprintf("- %s [%s] status=%s source=%s summary=%s path=%s\n", card.Ref, card.Kind, card.Status, card.SourceRef, card.Summary, card.SandboxPath))
	}
	b.WriteString("\nRecent user instructions to preserve verbatim:\n")
	for _, text := range input.RecentUserInstructions {
		b.WriteString("- " + strings.TrimSpace(text) + "\n")
	}
	b.WriteString("\nRecovery refs:\n")
	for _, ref := range input.RecoveryRefs {
		b.WriteString("- " + strings.TrimSpace(ref) + "\n")
	}
	return strings.TrimSpace(b.String())
}

func ValidateFullSummaryMarkdown(summary string) error {
	if strings.TrimSpace(summary) == "" {
		return fmt.Errorf("%w: empty summary", ErrInvalidFullSummary)
	}
	for _, section := range fullSummaryRequiredSections() {
		if !strings.Contains(summary, section) {
			return fmt.Errorf("%w: missing %s", ErrInvalidFullSummary, section)
		}
	}
	return nil
}

func fullSummaryRequiredSections() []string {
	return []string{
		"# Compacted Agent Handoff Summary",
		"## User Goal",
		"## Confirmed Decisions",
		"## Current Project State",
		"## Media Assets",
		"## Shot / RenderPlan Status",
		"## Review Findings",
		"## Audio / Timeline State",
		"## Pending Signals And Tasks",
		"## Known Failures And Avoidances",
		"## Recent User Instructions To Preserve Verbatim",
		"## Next Recommended Actions",
		"## Recovery References",
	}
}
```

- [ ] **Step 4: Run test and verify GREEN**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact -run 'ValidateFullSummaryRequiresHandoffSections' -count=1
```

Expected: PASS.

## Task 2: Full Compact Projection

**Files:**
- Modify: `apps/server/internal/agent/contextcompact/middleware.go`
- Add: `apps/server/internal/agent/contextcompact/full_projection_test.go`

- [ ] **Step 1: Write failing projection test**

Add `full_projection_test.go`:

```go
package contextcompact

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestProjectRunsFullCompactAfterMicroWhenStillOverFullTrigger(t *testing.T) {
	oldTool := strings.Repeat("old ffmpeg stderr line\n", 1600)
	messages := []*schema.Message{
		schema.SystemMessage("system prompt"),
		schema.UserMessage("old user"),
		{Role: schema.Tool, ToolCallID: "call-old", ToolName: "run_ffmpeg_command", Content: oldTool},
		schema.UserMessage("latest user instruction must stay visible"),
	}
	store := newMemoryStore()
	summarizer := fakeFullSummarizer{summary: validFullSummaryForTest("agent_context_compaction/summary")}
	middleware := NewMiddleware(MiddlewareConfig{
		Config: compactTestConfig(CompactionThresholds{
			MicroTriggerTokens:          100,
			MicroTargetTokens:           90,
			MicroMinReductionTokens:     1,
			PreserveRecentUserMessages:  1,
			PreserveRecentTotalMessages: 1,
		}),
		Store:          store,
		FileWriter:     newMemoryDetailFileWriter(),
		FullSummarizer: summarizer,
	})
	middleware.config.FullTriggerTokens = 100
	middleware.config.FullTargetTokens = 80

	out, err := middleware.Project(context.Background(), ProjectionInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Role:        "producer",
		ModelID:     "doubao-test",
		Messages:    messages,
		MessageRefs: []SourceMessageRef{{MessageIndex: 2, MessageID: uuidWithByte(4)}},
		Facts:       []FullSummaryFact{{Ref: "shot/shot_01", Kind: "shot", Source: "db", Summary: "preview ready"}},
		Trigger:     "producer_before_model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.CompactionMode != "full" || len(out.Applied) == 0 {
		t.Fatalf("output = %#v", out)
	}
	if len(out.Messages) < 3 || out.Messages[1].Role != schema.User || !strings.Contains(out.Messages[1].Content, "# Compacted Agent Handoff Summary") {
		t.Fatalf("handoff summary message missing: %#v", out.Messages)
	}
	if !strings.Contains(out.Messages[len(out.Messages)-1].Content, "latest user instruction must stay visible") {
		t.Fatalf("latest user instruction not preserved: %#v", out.Messages)
	}
	if len(store.links) != 1 || store.links[0].MessageID != uuidWithByte(4) {
		t.Fatalf("source message links = %#v", store.links)
	}
	if messages[2].Content != oldTool {
		t.Fatal("original message content was mutated")
	}
}
```

Add test helpers to the same file:

```go
type fakeFullSummarizer struct {
	summary string
	input   FullSummaryInput
}

func (f fakeFullSummarizer) Summarize(_ context.Context, input FullSummaryInput) (FullSummaryOutput, error) {
	f.input = input
	return FullSummaryOutput{Summary: f.summary, ModelID: "fake-summary-model"}, nil
}

func validFullSummaryForTest(ref string) string {
	return `# Compacted Agent Handoff Summary

## User Goal
Create a marketing video.

## Confirmed Decisions
- 未确认

## Current Project State
- shot/shot_01 exists.

## Media Assets
- 未生成视觉摘要

## Shot / RenderPlan Status
- shot_01 preview ready.

## Review Findings
- 未确认

## Audio / Timeline State
- 未确认

## Pending Signals And Tasks
- 未确认

## Known Failures And Avoidances
- 未确认

## Recent User Instructions To Preserve Verbatim
- latest user instruction must stay visible

## Next Recommended Actions
- Continue current role task.

## Recovery References
- ` + ref + `
`
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact -run 'ProjectRunsFullCompactAfterMicroWhenStillOverFullTrigger' -count=1
```

Expected: FAIL because `MiddlewareConfig.FullSummarizer` and `ProjectionInput.Facts` are undefined.

- [ ] **Step 3: Implement minimal full compact support**

Modify middleware structs:

```go
type MiddlewareConfig struct {
	Config         Config
	Store          Store
	FileWriter     DetailFileWriter
	Counter        TokenCounter
	FullSummarizer FullSummarizer
}

type ProjectionInput struct {
	WorkspaceID       pgtype.UUID
	ThreadID          pgtype.UUID
	TaskID            pgtype.UUID
	Role              string
	ModelID           string
	Messages          []*schema.Message
	MessageRefs       []SourceMessageRef
	ToolInfos         []*schema.ToolInfo
	MediaCards        []MediaCard
	Facts             []FullSummaryFact
	Trigger           string
	SameTurnFromIndex int
	PendingFromIndex  int
	ForceFullCompact  bool
}
```

Add full compact flow after micro compaction:

```go
if (input.ForceFullCompact || out.TokenAfter >= m.config.FullTriggerTokens) && m.fullSummarizer != nil {
	full, err := m.fullCompact(ctx, input, messages, out)
	if err != nil {
		return ProjectionOutput{}, err
	}
	return full, nil
}
```

Implement `fullCompact` to:

- build `FullSummaryInput` from early messages, `Facts`, `MediaCards`, recent user instructions, and existing `out.CompactionRefs`;
- validate the returned Markdown;
- write a detail file with `ToolName: "full_compact_summary"` and original summary content;
- create an `agent_context_compaction` record with `Mode: "full"`, `Trigger: defaultTrigger(input.Trigger)`, source seq range covering compacted early messages, `Summary` set to the first 500 chars of the handoff summary, and `DetailFiles` containing the summary file path;
- replace early history with `schema.UserMessage("Compacted handoff summary...\n\n" + summary + "\n\nRecovery: use search_agent_history or read_file.")`;
- preserve system prompt, recent user messages, recent total messages, same-turn messages, pending reminders, and current role-specific messages via existing boundary indexes.

- [ ] **Step 4: Run projection tests and verify GREEN**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact -run 'ProjectRunsFullCompactAfterMicroWhenStillOverFullTrigger|MicroCompaction|ValidateFullSummary' -count=1
```

Expected: PASS.

## Task 3: Summary Runner

**Files:**
- Add: `apps/server/internal/agent/contextcompact/summary_runner.go`
- Add: `apps/server/internal/agent/contextcompact/summary_runner_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Write failing runner test**

Add `summary_runner_test.go`:

```go
package contextcompact

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestStaticFullSummarizerValidatesReturnedSummary(t *testing.T) {
	summarizer := StaticFullSummarizer{Summary: validFullSummaryForTest("agent_context_compaction/full")}
	out, err := summarizer.Summarize(context.Background(), FullSummaryInput{
		Role:     "producer",
		Messages: []*schema.Message{schema.UserMessage("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Summary, "# Compacted Agent Handoff Summary") {
		t.Fatalf("summary = %s", out.Summary)
	}
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact -run 'StaticFullSummarizerValidatesReturnedSummary' -count=1
```

Expected: FAIL because `StaticFullSummarizer` is undefined.

- [ ] **Step 3: Implement static and model-backed summarizers**

Add a test-oriented static summarizer:

```go
type StaticFullSummarizer struct {
	Summary string
	ModelID string
}

func (s StaticFullSummarizer) Summarize(_ context.Context, input FullSummaryInput) (FullSummaryOutput, error) {
	summary := strings.TrimSpace(s.Summary)
	if summary == "" {
		summary = BuildFallbackFullSummary(input)
	}
	if err := ValidateFullSummaryMarkdown(summary); err != nil {
		return FullSummaryOutput{}, err
	}
	return FullSummaryOutput{Summary: summary, ModelID: strings.TrimSpace(s.ModelID)}, nil
}
```

Add `BuildFallbackFullSummary(input FullSummaryInput) string` that creates all required sections from facts, media cards, recent user instructions, and recovery refs. This fallback is allowed only for tests and fail-safe local operation; production Volcengine mode should use model-backed summarization when configured.

Implement `VolcengineFullSummarizer` using the existing Ark chat model pattern:

```go
type VolcengineFullSummarizerConfig struct {
	APIKey      string
	BaseURL     string
	Region      string
	Model       string
	MaxTokens   int
	Temperature float32
	Factory     func(context.Context, *ark.ChatModelConfig) (fullSummaryChatModel, error)
}
```

It sends:

- system message: concise summary instruction and required sections;
- user message: `BuildFullSummaryPrompt(input)`.

It must trim, validate, and return the model output. If validation fails, return an error and let compaction fail closed.

- [ ] **Step 4: Wire summarizer in `cmd/server/main.go`**

When `Production.ProviderMode == "real"` and Volcengine API key/model are present, construct `VolcengineFullSummarizer`. Otherwise use `StaticFullSummarizer{}` only for deterministic local tests.

- [ ] **Step 5: Run runner tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./cmd/server -run 'FullSummarizer|ResponderForConfig|AgentModelMaxTokenBudgets' -count=1
```

Expected: PASS.

## Task 4: Role Facts and Media Cards

**Files:**
- Add: `apps/server/internal/agent/producer/contextcompact_facts.go`
- Add: `apps/server/internal/agent/producer/contextcompact_facts_test.go`
- Add: `apps/server/internal/agent/craftsman/contextcompact_facts.go`
- Add: `apps/server/internal/agent/craftsman/contextcompact_facts_test.go`
- Add: `apps/server/internal/agent/reviewer/contextcompact_facts.go`
- Add: `apps/server/internal/agent/reviewer/contextcompact_facts_test.go`
- Add: `apps/server/internal/agent/composer/contextcompact_facts.go`
- Add: `apps/server/internal/agent/composer/contextcompact_facts_test.go`
- Modify: role responder files to pass `Facts` and `MediaCards`.

- [ ] **Step 1: Write failing facts tests**

Producer test must prove DB facts are not empty:

```go
func TestProducerFullCompactFactsIncludeProjectStateAndImages(t *testing.T) {
	ctx := ProducerContext{
		LatestUserText: "做一条悦行行李箱广告",
		ImageAttachments: map[string]ProducerImageAttachment{
			"asset-1": {AssetID: "asset-1", NodeID: "node-1", Name: "box.png", Mime: "image/png"},
		},
		ProjectFacts: []contextcompact.FullSummaryFact{
			{Ref: "project_memory/active", Kind: "project_memory", Source: "db", Summary: "核心意图：轻松出行"},
		},
	}
	facts, cards := producerContextCompactionFacts(ctx)
	if len(facts) == 0 || len(cards) != 1 {
		t.Fatalf("facts=%#v cards=%#v", facts, cards)
	}
}
```

This test intentionally requires a new `ProjectFacts []contextcompact.FullSummaryFact` field on `ProducerContext`.

Craftsman / Reviewer / Composer tests should assert:

- Craftsman includes `shot`, `render_plan`, `source_material`, and current `Context.Text`.
- Reviewer includes `review_target`, `generation_job`, `prior_review`, and current media card.
- Composer includes `source_storyboard_node`, `timeline_plan_count`, `workspace_mode`, and current source node title.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer -run 'FullCompactFacts' -count=1
```

Expected: FAIL because facts builders and `ProducerContext.ProjectFacts` do not exist.

- [ ] **Step 3: Implement role facts builders**

Each builder returns `([]contextcompact.FullSummaryFact, []contextcompact.MediaCard)` and uses stable refs:

```text
shot/{client_key or uuid}
render_plan/{semantic_key or uuid}
media_node/{semantic_key or uuid}
artifact_version/{semantic_key or uuid}
generation_job/{uuid}
timeline_plan/{uuid}
producer_pending_signal/{semantic_key or uuid}
```

For media cards, never invent visual/audio summary. Use:

- `Summary: "未生成视觉摘要"` when no trusted summary exists.
- `SourceRef: "db"`, `"review"`, `"probe"`, `"user"`, or `"model_output"` when known.
- `SandboxPath` only when a real sandbox path is already present in context.

- [ ] **Step 4: Pass facts into middleware**

In each responder `ProjectionInput`, add:

```go
Facts:      facts,
MediaCards: mediaCards,
```

Use role builders immediately before `ContextCompactor.Project`.

- [ ] **Step 5: Run role tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer -run 'FullCompactFacts|ContextCompaction' -count=1
```

Expected: PASS.

## Task 5: Producer Facts Provider

**Files:**
- Modify: `apps/server/internal/agent/producer/types.go`
- Modify: `apps/server/internal/agent/producer/context_loader.go`
- Add: `apps/server/internal/agent/producer/context_facts_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Write failing Producer facts loader test**

Add a test proving `LoadProducerContext` includes DB-derived facts when a facts provider is configured:

```go
func TestRuntimeContextLoaderLoadsProjectFactsForFullCompact(t *testing.T) {
	loader := RuntimeContextLoader{
		Runtime: fakeProducerContextRuntime{},
		Facts: fakeProducerFactsProvider{
			facts: []contextcompact.FullSummaryFact{{Ref: "project_memory/active", Kind: "project_memory", Source: "db", Summary: "轻松出行"}},
		},
	}
	out, err := loader.LoadProducerContext(context.Background(), ProducerTurnInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2)})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.ProjectFacts) != 1 || out.ProjectFacts[0].Ref != "project_memory/active" {
		t.Fatalf("ProjectFacts = %#v", out.ProjectFacts)
	}
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'LoadsProjectFactsForFullCompact' -count=1
```

Expected: FAIL because `RuntimeContextLoader.Facts` and `ProducerContext.ProjectFacts` are undefined.

- [ ] **Step 3: Implement facts provider**

Add:

```go
type ProducerFactsProvider interface {
	LoadProducerFacts(ctx context.Context, workspaceID pgtype.UUID) ([]contextcompact.FullSummaryFact, []contextcompact.MediaCard, error)
}
```

Use the existing `pss.Builder` where possible. If direct reuse would create an import cycle, create `producer.PSSFactsProvider` in the producer package that depends on an interface with the subset of store methods already used by `pss.Builder`.

`LoadProducerContext` must:

- call the provider only when `input.WorkspaceID.Valid`;
- fail closed by returning the provider error, because a full summary without DB facts is misleading;
- populate `ProducerContext.ProjectFacts` and `ProducerContext.ProjectMediaCards`.

- [ ] **Step 4: Wire provider in server**

In `cmd/server/main.go`, construct the provider next to the existing Producer context loader. Reuse `queries` as the store where the interface is satisfied.

- [ ] **Step 5: Run Producer tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer ./cmd/server -run 'LoadsProjectFactsForFullCompact|ContextLoader|ProducerResponderForConfig' -count=1
```

Expected: PASS.

## Task 6: One-Time Context Overflow Retry

**Files:**
- Add: `apps/server/internal/agent/contextcompact/overflow.go`
- Add: `apps/server/internal/agent/contextcompact/overflow_test.go`
- Modify: `apps/server/internal/agent/producer/model_responder.go`
- Modify: `apps/server/internal/agent/craftsman/model_responder.go`
- Modify: `apps/server/internal/agent/reviewer/model_responder.go`
- Modify: `apps/server/internal/agent/composer/model_responder.go`

- [ ] **Step 1: Write failing overflow classifier test**

```go
func TestIsContextOverflowErrorClassifiesProviderMessages(t *testing.T) {
	for _, msg := range []string{
		"context length exceeded",
		"maximum context length",
		"tokens exceed context window",
		"prompt is too long",
	} {
		if !IsContextOverflowError(errors.New(msg)) {
			t.Fatalf("expected overflow for %q", msg)
		}
	}
	if IsContextOverflowError(errors.New("network timeout")) {
		t.Fatal("network timeout must not be classified as context overflow")
	}
}
```

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact -run 'IsContextOverflowError' -count=1
```

Expected: FAIL because `IsContextOverflowError` is undefined.

- [ ] **Step 3: Implement classifier**

Implement lowercase substring matching over `err.Error()` with conservative phrases only. Do not classify rate limits, timeouts, auth errors, or provider unavailable errors as overflow.

- [ ] **Step 4: Add responder retry tests**

For each role, add one test where fake model first returns a context overflow error, the responder calls `ContextCompactor.Project` again with `ForceFullCompact: true` and `Trigger: "model_error_context_overflow"`, then the second model call succeeds.

The tests must assert:

- exactly two provider attempts;
- no third attempt;
- metadata includes `context_compaction_retry=true`;
- pending reminders and same-turn protected messages remain visible in the retry prompt.

- [ ] **Step 5: Implement retry wrapper**

Each real responder should wrap only the model call:

```go
final, err := callModel(messages)
if err != nil && contextcompact.IsContextOverflowError(err) && r.cfg.ContextCompactor != nil && !retried {
	retried = true
	compacted, compactErr := r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
		WorkspaceID:       input.WorkspaceID,
		ThreadID:          input.ThreadID,
		TaskID:            input.TaskID,
		Role:              roleName,
		ModelID:           modelID,
		Messages:          prompt.Messages,
		MessageRefs:       prompt.MessageRefs,
		ToolInfos:         toolInfos,
		MediaCards:        mediaCards,
		Facts:             facts,
		Trigger:           "model_error_context_overflow",
		SameTurnFromIndex: prompt.SameTurnFromIndex,
		PendingFromIndex:  prompt.PendingFromIndex,
		ForceFullCompact:  true,
	})
	if compactErr != nil {
		return output{}, compactErr
	}
	messages = compacted.Messages
	final, err = callModel(messages)
}
```

If the retry fails, return the second error. Do not loop.

- [ ] **Step 6: Run retry tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer -run 'ContextOverflow|ContextCompactionRetry' -count=1
```

Expected: PASS.

## Task 7: Diagnostics, Sidecar Search, and Chat-List Boundary

**Files:**
- Modify: `apps/server/internal/agent/tools/search_agent_history_test.go`
- Modify: `apps/server/internal/agent/contextcompact/chat_list_boundary_test.go`
- Modify: role responder tests for metadata.

- [ ] **Step 1: Add search test for full records**

Add a test proving `search_agent_history(compact_ref=...)` returns `mode=full`, `summary`, and `detail_files` for full compact records.

- [ ] **Step 2: Add metadata assertions**

For every role context compaction projection test, assert metadata includes:

```text
context_compaction_applied=true
context_compaction_mode=full or micro
context_compaction_count > 0
context_compaction_refs contains ctxcmp
context_compaction_detail_files contains /workspace/.clipanvil/context/
```

- [ ] **Step 3: Extend chat-list boundary search if needed**

Keep `TestCompactionCodeDoesNotUseAgentMessageUpdatePath` scanning the whole `contextcompact` package and role model responders. Do not whitelist production files.

- [ ] **Step 4: Run focused diagnostics tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/tools ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer -run 'FullCompact|SearchAgentHistory|AgentMessageUpdatePath|ContextCompaction' -count=1
```

Expected: PASS.

## Task 8: Documentation and Milestone Record

**Files:**
- Modify: `docs/milestones/m9-agent-context-compaction.md`
- Modify: `docs/superpowers/plans/2026-06-30-m9-4-full-compact.md`

- [ ] **Step 1: Update milestone completion record**

After all tests pass, update M9.4 status to completed and record:

- files changed;
- full compact trigger behavior;
- summary runner behavior;
- facts/media-card source;
- one-time overflow retry behavior;
- chat-list non-mutation evidence;
- verification commands.

- [ ] **Step 2: Add completion record to this plan**

Append a `## Completion Record` section with the exact commands and observed pass results.

- [ ] **Step 3: Run final verification**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/tools ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer ./cmd/server -run 'FullCompact|ContextOverflow|ContextCompaction|SearchAgentHistory|AgentMessageUpdatePath|ResponderForConfig' -count=1
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
rg -n "UpdateAgentMessage|UpdateMessage\\(|ListAgentMessagesByThread" apps/server/internal/agent/contextcompact apps/server/internal/agent/producer/model_responder.go apps/server/internal/agent/craftsman/model_responder.go apps/server/internal/agent/reviewer/model_responder.go apps/server/internal/agent/composer/model_responder.go
```

Expected:

- focused tests PASS;
- `make server-test` PASS;
- `git diff --check` has no output;
- `rg` only matches boundary tests.

## Acceptance Criteria

- Full compact triggers before model calls when token count remains at or above `FullTriggerTokens`.
- Full compact writes `mode='full'` sidecar records and summary detail files.
- Full summary contains all required handoff sections.
- Full summary input contains DB facts and media cards, not only `agent_message` history.
- Recent user messages, recent total messages, same-turn tool loop, pending reminders, and current role task context remain visible.
- Reviewer current image/video/audio input is never replaced by summary.
- Producer signal drain / claim timing is unchanged.
- Context overflow retry happens at most once and uses `ForceFullCompact`.
- `search_agent_history` can find full compact records and detail files.
- Agent chat message list still displays original `agent_message` content.
- Focused tests, `make server-test`, `git diff --check`, and forbidden-path `rg` pass.

## Self-Review

- Spec coverage: covers full compact trigger, structured summary, DB facts, media cards, preservation policy, sidecar persistence, recovery references, overflow retry, and chat-list boundary.
- Placeholder scan: no implementation step relies on undefined behavior without naming the file, function, and expected test.
- Type consistency: new `ProjectionInput` fields are reused by role responders and retry paths; `FullSummaryFact`, `FullSummarizer`, and `ForceFullCompact` names are consistent across tasks.

## Completion Record

✅ Completed on 2026-06-30.

Implemented:

- Added full summary types, prompt builder, required-section validator, static fallback summarizer, and Volcengine full summarizer.
- Extended `ContextCompactMiddleware` with `Facts`, `MediaCards`, `ForceFullCompact`, full compact projection, sidecar record creation, summary detail files, and preserved-message rebuild.
- Added conservative context overflow error classification.
- Added Producer PSS-based facts provider and wired it into `RuntimeContextLoader` from `cmd/server/main.go`.
- Added role facts/media-card builders for Producer, Craftsman, Reviewer, and Composer.
- Passed facts/media cards into all four real Volcengine responders.
- Added one-time context-overflow retry for Producer, Craftsman, Reviewer, and Composer.
- Added `mode` to `search_agent_history` output so full compact records are distinguishable.

Verified:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/tools ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer ./cmd/server -run 'FullCompact|ContextOverflow|ContextCompaction|SearchAgentHistory|AgentMessageUpdatePath|ContextFullSummarizer|FullSummarizer|FullCompactFacts' -count=1
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
rg -n "UpdateAgentMessage|UpdateMessage\\(|ListAgentMessagesByThread" apps/server/internal/agent/contextcompact apps/server/internal/agent/producer/model_responder.go apps/server/internal/agent/craftsman/model_responder.go apps/server/internal/agent/reviewer/model_responder.go apps/server/internal/agent/composer/model_responder.go
```

The forbidden-path `rg` inspection only matched `apps/server/internal/agent/contextcompact/chat_list_boundary_test.go`, so compaction remains model-input-only and does not mutate the Agent chat message list.
