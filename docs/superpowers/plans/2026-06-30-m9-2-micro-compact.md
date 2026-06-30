# M9.2 Micro Compact And Recoverable History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build recoverable micro compact for long historical Agent context: persist compaction sidecar records, write long originals into sandbox detail files, replace only model-input history with compact placeholders, and expose `search_agent_history` for recovery.

**Architecture:** M9.2 must keep two histories separate. `agent_message` remains the canonical conversation history used by the Agent chat list and audit trail. `agent_context_compaction` / `agent_message_compaction` are sidecar metadata. `ContextCompactMiddleware` receives a prompt-building request, creates or reuses compaction records, writes `/workspace/.clipanvil/context/` detail files when needed, and returns a temporary `[]*schema.Message` projection for the next model call. The projection can contain compact placeholders; persisted `agent_message.content` and `agent_message.raw_message` cannot.

**Tech Stack:** Go 1.26, PostgreSQL migration, sqlc, Eino `schema.Message`, existing workspace sandbox manager/client, native tool registry, M9.1 `contextcompact` config/token/planner/file primitives.

---

## Source Documents

- `docs/superpowers/specs/2026-06-30-agent-context-compaction-design.md`
- `docs/milestones/m9-agent-context-compaction.md`
- `docs/superpowers/plans/2026-06-30-m9-1-contextcompact-file-tools.md`
- `apps/server/internal/agent/producer/model_responder.go`
- `apps/server/internal/agent/composer/model_responder.go`
- `apps/server/sqlc/queries/agent_message.sql`
- `apps/server/internal/agent/runtime/service.go`

## Hard Boundaries

In scope:

- Add `agent_context_compaction` and `agent_message_compaction` tables.
- Add sqlc queries for create/link/list/search/read-by-ref.
- Add micro compact execution on old, large, recoverable history items.
- Write long originals to `/workspace/.clipanvil/context/`.
- Add `search_agent_history` native tool.
- Connect Producer and Composer model calls to the temporary compacted projection.
- Add tests proving the Agent chat message list still reads original `agent_message` content.

Out of scope:

- No full compact.
- No vector search.
- No UI changes.
- No user-editable compaction summary UI.
- No changes to Producer signal claim / drain timing.
- No default message-list API rewrite.
- No deletion, truncation, or overwrite of existing `agent_message` rows.

Forbidden implementation paths:

- Do not use `UpdateAgentMessage` for compaction.
- Do not write compact placeholders into `agent_message.content`.
- Do not write compact placeholders into `agent_message.raw_message`.
- Do not change `ListAgentMessagesByThread` default semantics.
- Do not compact same-turn tool messages that are still part of the active native tool loop.

## Message Separation Contract

M9.2 must preserve this dataflow:

```text
agent_message original rows
  -> Agent chat list / history API
  -> user-visible original content

agent_message original rows + compaction sidecars
  -> prompt builder
  -> ContextCompactMiddleware
  -> temporary compacted schema.Message projection
  -> model provider input only
```

Acceptance depends on both halves passing: model input becomes smaller, and the Agent chat list still displays the original conversation history.

## File Map

- Create `apps/server/migrations/036_agent_context_compaction.sql`.
- Create `apps/server/sqlc/queries/agent_context_compaction.sql`.
- Regenerate `apps/server/internal/store/db/*`.
- Create `apps/server/internal/agent/contextcompact/store.go`.
- Create `apps/server/internal/agent/contextcompact/micro_compact.go`.
- Create `apps/server/internal/agent/contextcompact/detail_file.go`.
- Create `apps/server/internal/agent/contextcompact/projection.go`.
- Create `apps/server/internal/agent/contextcompact/middleware.go`.
- Create `apps/server/internal/agent/contextcompact/search.go`.
- Create `apps/server/internal/agent/tools/search_agent_history.go`.
- Modify `apps/server/internal/agent/producer/model_responder.go`.
- Modify `apps/server/internal/agent/composer/model_responder.go`.
- Modify `apps/server/cmd/server/main.go`.
- Add focused tests in `apps/server/internal/agent/contextcompact`.
- Add focused tests in `apps/server/internal/agent/tools`.
- Add Producer / Composer responder tests for model-input projection.
- Add or extend runtime/store tests proving chat-list rows remain original.

## Task 1: Add Sidecar Persistence

**Files:**
- Create: `apps/server/migrations/036_agent_context_compaction.sql`
- Create: `apps/server/sqlc/queries/agent_context_compaction.sql`
- Generate: `apps/server/internal/store/db/*`

- [ ] **Step 1: Write migration**

Create `agent_context_compaction` with:

- `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
- `workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE`
- `thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL`
- `task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL`
- `role TEXT NOT NULL`
- `mode TEXT NOT NULL`
- `trigger TEXT NOT NULL`
- `semantic_key TEXT NOT NULL`
- `source_seq_start BIGINT NOT NULL`
- `source_seq_end BIGINT NOT NULL`
- `source_message_ids JSONB NOT NULL DEFAULT '[]'::jsonb`
- `source_media_refs JSONB NOT NULL DEFAULT '[]'::jsonb`
- `original_token_estimate BIGINT NOT NULL DEFAULT 0`
- `compacted_token_estimate BIGINT NOT NULL DEFAULT 0`
- `original_bytes BIGINT NOT NULL DEFAULT 0`
- `summary TEXT NOT NULL`
- `detail_files JSONB NOT NULL DEFAULT '[]'::jsonb`
- `payload JSONB NOT NULL DEFAULT '{}'::jsonb`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`

Create `agent_message_compaction` with:

- `message_id UUID PRIMARY KEY REFERENCES agent_message(id) ON DELETE CASCADE`
- `compaction_id UUID NOT NULL REFERENCES agent_context_compaction(id) ON DELETE CASCADE`
- `compacted_role TEXT NOT NULL`
- `compacted_at TIMESTAMPTZ NOT NULL DEFAULT now()`

Indexes:

- `(workspace_id, semantic_key)`
- `(thread_id, created_at DESC)`
- `(workspace_id, created_at DESC)`
- `agent_message_compaction(compaction_id)`

- [ ] **Step 2: Add sqlc queries**

Create queries:

- `CreateAgentContextCompaction`
- `LinkAgentMessageCompaction`
- `GetAgentContextCompactionBySemanticKey`
- `ListAgentContextCompactionsByThread`
- `ListAgentContextCompactionsByWorkspace`
- `ListCompactedMessageIDsByThread`
- `SearchAgentContextCompactions`

`SearchAgentContextCompactions` can use PostgreSQL text matching in M9.2:

```sql
summary ILIKE '%' || sqlc.arg(query)::text || '%'
OR payload::text ILIKE '%' || sqlc.arg(query)::text || '%'
OR semantic_key = sqlc.arg(query)::text
```

- [ ] **Step 3: Generate db code**

Run:

```bash
make sqlc-generate
```

Expected: generated db code includes the compaction query methods and all existing query methods remain intact.

## Task 2: Add Store, Detail Files, And Projection Primitives

**Files:**
- Create: `apps/server/internal/agent/contextcompact/store.go`
- Create: `apps/server/internal/agent/contextcompact/detail_file.go`
- Create: `apps/server/internal/agent/contextcompact/projection.go`
- Test: `apps/server/internal/agent/contextcompact/*_test.go`

- [ ] **Step 1: Write failing tests first**

Add tests for:

- Creating a compaction record stores source message ids and token estimates.
- Linking messages marks only sidecar rows, not message content.
- Detail file writer stores long text under `/workspace/.clipanvil/context/`.
- Detail file names are deterministic from `workspace_id`, `thread_id`, role, seq range, and content hash.
- Projection replaces only eligible historical model messages with compact placeholders.
- Projection preserves `ToolCallID`, `ToolName`, role, and order for tool results.
- Projection skips recent user messages, recent total messages, same-turn messages, and pending reminder messages.

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact -run 'Store|DetailFile|Projection|MicroCompact' -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 2: Implement store abstraction**

Define a small interface in `contextcompact` so middleware tests can use fakes:

```go
type Store interface {
	CreateCompaction(ctx context.Context, input CreateCompactionInput) (CompactionRecord, error)
	LinkMessage(ctx context.Context, input LinkMessageInput) error
	CompactedMessageIDs(ctx context.Context, threadID pgtype.UUID) (map[pgtype.UUID]CompactionRecord, error)
	GetBySemanticKey(ctx context.Context, workspaceID pgtype.UUID, semanticKey string) (CompactionRecord, error)
	Search(ctx context.Context, input SearchInput) ([]CompactionRecord, error)
}
```

Implement a sqlc-backed store that uses the generated query methods.

- [ ] **Step 3: Implement detail file writer**

Write detail files through the existing workspace sandbox manager/client:

- Directory: `/workspace/.clipanvil/context/`
- Extension: `.md`
- Content includes source role, message ids, seq range, tool name, tool call id, original bytes, hash, and original text.
- Parent directory is created with sandbox exec before upload.
- Existing files with the same deterministic name can be reused after hash match.

- [ ] **Step 4: Implement projection primitives**

Add types:

```go
type ProjectionInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	Role        string
	Messages    []*schema.Message
	Candidates  []Candidate
	Trigger     string
}

type ProjectionOutput struct {
	Messages          []*schema.Message
	Applied           []CompactionRecord
	TokenBefore        int
	TokenAfter         int
	OriginalUnchanged  bool
}
```

The `Messages` field is a deep-ish copy for model input. Original `[]*schema.Message` must not be mutated.

## Task 3: Implement Micro Compact Execution

**Files:**
- Create: `apps/server/internal/agent/contextcompact/micro_compact.go`
- Create: `apps/server/internal/agent/contextcompact/middleware.go`
- Test: `apps/server/internal/agent/contextcompact/*_test.go`

- [ ] **Step 1: Candidate rules**

Use M9.1 planner output, then filter:

- Eligible: old tool result content, old large tool call args if provider-safe, old skill result, old ffprobe / ffmpeg logs.
- Protected: system prompt, latest user messages, recent total messages, current same-turn tool loop, pending reminders, current review target, current timeline target, current media input.
- M9.2 provider-safe default: compact tool result content first; keep historical assistant tool call arguments unchanged unless tests prove the provider accepts projected arguments.

- [ ] **Step 2: Create or reuse records**

For each selected candidate:

- Build semantic key such as `ctxcmp:<role>:<thread_id>:<seq_start>-<seq_end>:<hash>`.
- If a record already exists for `(workspace_id, semantic_key)`, reuse it.
- If long original text exceeds inline limit, write detail file and store path in `detail_files`.
- Insert compaction row.
- Link all source messages in `agent_message_compaction`.

- [ ] **Step 3: Replace only model-input messages**

Use placeholder content like:

```text
历史工具结果已压缩。
compact_ref: ctxcmp:producer:...
summary: ...
detail_file: /workspace/.clipanvil/context/...
恢复方式: 使用 read_file 读取 detail_file；不知道路径时用 search_agent_history(compact_ref=...)。
```

Rules:

- For `schema.Tool` messages, keep `Role`, `ToolCallID`, and `ToolName`.
- For assistant tool-call messages, keep `ToolCalls` unchanged in M9.2.
- Keep message order stable.
- Do not append placeholders as persisted `agent_message` rows.

- [ ] **Step 4: Add diagnostics**

Return metadata:

- `context_compaction_applied`
- `context_compaction_mode`
- `context_compaction_count`
- `context_compaction_token_before`
- `context_compaction_token_after`
- `context_compaction_refs`
- `context_compaction_detail_files`

Diagnostics may be stored in task/event metadata, but must not replace chat message body content.

## Task 4: Add `search_agent_history`

**Files:**
- Create: `apps/server/internal/agent/tools/search_agent_history.go`
- Test: `apps/server/internal/agent/tools/search_agent_history_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Tool contract**

Tool name: `search_agent_history`.

Arguments:

- `compact_ref` optional string for exact compaction lookup.
- `query` optional string for text search.
- `media_ref` optional string for media-related lookup.
- `thread_id` optional string; default to current runtime thread when available.
- `limit` optional integer; default from context compaction config.

Result fields:

- `matches`
- `compact_ref`
- `summary`
- `detail_files`
- `source_message_ids`
- `source_seq_start`
- `source_seq_end`
- `source_media_refs`
- `created_at`
- `excerpt`

- [ ] **Step 2: Runtime behavior**

The tool requires `NativeRuntimeContext` with workspace id. It uses exact `compact_ref` first. If no exact ref is supplied, it performs bounded text search. It returns short excerpts and detail file paths; full original text is recovered through `read_file`.

- [ ] **Step 3: Register tool**

Register `search_agent_history` for Producer, Craftsman, Reviewer, and Composer. M9.2 only wires micro compact into Producer / Composer model input, but the recovery tool can be shared across roles immediately.

## Task 5: Connect Producer And Composer Model Input

**Files:**
- Modify: `apps/server/internal/agent/producer/model_responder.go`
- Modify: `apps/server/internal/agent/composer/model_responder.go`
- Modify related constructor/config wiring in `apps/server/cmd/server/main.go`
- Test: Producer / Composer responder tests

- [ ] **Step 1: Add optional middleware dependency**

Extend responder config with an optional context compaction middleware:

```go
ContextCompactor contextcompact.Middleware
```

Keep deterministic responders unchanged unless their tests need explicit nil behavior.

- [ ] **Step 2: Apply after raw prompt construction**

Producer:

- Build raw messages through existing `producerPromptMessages`.
- Apply compactor after `agentprompt.AppendPendingReminders`.
- Pass compacted projection to provider.
- Keep original `producerContext.Messages` unchanged.

Composer:

- Build raw messages through existing `composerPromptMessages`.
- Apply compactor before provider `Generate`.
- Keep `composerContext.SameTurnMessages` unchanged.

- [ ] **Step 3: Preserve tool loop semantics**

Tests must prove:

- Same-turn assistant `ToolCalls` and tool results are not compacted.
- Historical tool result placeholders retain `ToolCallID` and `ToolName`.
- Historical assistant tool call arguments remain unchanged in M9.2 provider input.
- Pending reminders stay at the end and are not selected as compaction candidates.

## Task 6: Prove Chat List Is Unaffected

**Files:**
- Add or extend runtime/store tests around `ListAgentMessagesByThread`
- Add Producer / Composer projection tests

- [ ] **Step 1: Write regression test**

Create a test with a long historical `agent_message` tool result. Trigger micro compact with small thresholds. Then assert:

- `ListAgentMessagesByThread` returns the original long content.
- `raw_message` still contains original JSON.
- No returned chat-list message contains `compact_ref`.
- No returned chat-list message contains `历史工具结果已压缩`.
- `agent_message_compaction` links the source message.
- Provider input contains the compact placeholder.

- [ ] **Step 2: Guard against accidental mutation**

Add a unit test that passes a `[]*schema.Message` slice into the middleware, then checks the original slice content after projection. Original content must remain byte-for-byte equal.

## Verification

Run these after implementation:

```bash
make sqlc-generate
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/tools ./internal/agent/producer ./internal/agent/composer -run 'Store|DetailFile|Projection|MicroCompact|SearchAgentHistory|ContextCompaction|MessageProjection|ChatList' -count=1
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

Manual inspection after tests:

- Search for `UpdateAgentMessage` usage and confirm compaction code does not call it.
- Search for compact placeholder strings and confirm they only appear in contextcompact tests/docs, not chat-list rendering code.
- Inspect generated sqlc diff and confirm `ListAgentMessagesByThread` query semantics are unchanged.

## Acceptance Criteria

- Micro compact triggers under small thresholds for Producer and Composer historical tool results.
- Compaction records are persisted in sidecar tables.
- Long originals are recoverable through `/workspace/.clipanvil/context/` detail files.
- `read_file(path=detail_file)` can recover long original text in chunks.
- `search_agent_history(compact_ref=...)` returns the matching record and detail file path.
- Provider model input contains compact placeholders for eligible historical messages.
- Agent chat message list still returns original `agent_message` content and raw JSON.
- Tool result `ToolCallID` / `ToolName` and message order remain valid.
- Same-turn tool loop and pending reminders are not compacted.
- `GOCACHE=/private/tmp/clipanvil-go-build make server-test` passes.
