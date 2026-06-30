# M9.3 Four Agent ContextCompact Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect the shared M9.2 `ContextCompactMiddleware` to Craftsman and Reviewer, then normalize Producer, Craftsman, Reviewer, and Composer protection boundaries so all four roles use one compaction implementation without corrupting active tool loops or user-visible Agent history.

**Architecture:** M9.3 keeps the M9.2 model-input-only contract. Each role builds raw `schema.Message` history, records prompt index to source `agent_message.id` when available, applies `ContextCompactMiddleware.Project` before provider calls, and passes only the projected messages to the model. Role-specific prompt builders return boundary metadata for same-turn messages, pending reminders, source message refs, and current critical media/context ranges.

**Tech Stack:** Go 1.26, Eino `schema.Message`, existing `contextcompact.Middleware`, existing native tool loops, existing `agentprompt.HistoryMessages`, `GOCACHE=/private/tmp/clipanvil-go-build make server-test`.

---

## Source Documents

- `docs/superpowers/specs/2026-06-30-agent-context-compaction-design.md`
- `docs/milestones/m9-agent-context-compaction.md`
- `docs/superpowers/plans/2026-06-30-m9-2-micro-compact.md`
- `apps/server/internal/agent/contextcompact/middleware.go`
- `apps/server/internal/agent/producer/model_responder.go`
- `apps/server/internal/agent/composer/model_responder.go`
- `apps/server/internal/agent/craftsman/model_responder.go`
- `apps/server/internal/agent/reviewer/model_responder.go`

## Hard Boundaries

In scope:

- Add `ContextCompactor contextcompact.Middleware` to Craftsman and Reviewer Volcengine responder configs.
- Apply compaction to Craftsman model input.
- Apply compaction to Reviewer model input.
- Keep Producer and Composer behavior from M9.2, but extract or align duplicated boundary helpers where it reduces drift.
- Preserve source `agent_message.id` links for Craftsman and Reviewer history messages when `agentprompt.HistoryMessages` emits a model message.
- Register no new tools in this stage; M9.2 already registered `search_agent_history`.
- Add tests for all four roles proving protected messages stay visible.

Out of scope:

- No full compact.
- No vector search.
- No UI changes.
- No media-card expansion beyond current `AssetURL` and prompt boundary protection.
- No change to Producer signal claim / drain timing.
- No rewrite, deletion, or truncation of `agent_message`.

Forbidden implementation paths:

- Do not call `UpdateAgentMessage`, `UpdateMessage`, or any chat-list mutation path from compaction code.
- Do not compact Reviewer current `reviewUserMessage` when it contains `UserInputMultiContent` image input.
- Do not compact Craftsman current `craftsmanContext.Text`.
- Do not compact current pending reminder text.
- Do not compact the latest same-turn assistant/tool pair.

## File Map

- Modify `apps/server/internal/agent/craftsman/model_responder.go`: compactor config, prompt boundary, model-input projection, metadata.
- Modify `apps/server/internal/agent/reviewer/model_responder.go`: compactor config, prompt boundary, model-input projection, metadata.
- Modify `apps/server/cmd/server/main.go`: pass `contextCompactor` into Craftsman and Reviewer responder factories.
- Modify `apps/server/cmd/server/main_test.go`: keep responder factory tests compiling after signature changes.
- Add `apps/server/internal/agent/craftsman/contextcompact_test.go`: fake store/writer and config for Craftsman tests.
- Add `apps/server/internal/agent/craftsman/model_contextcompact_test.go`: Craftsman projection tests.
- Add `apps/server/internal/agent/reviewer/contextcompact_test.go`: fake store/writer and config for Reviewer tests.
- Add `apps/server/internal/agent/reviewer/model_contextcompact_test.go`: Reviewer projection tests.
- Modify `docs/milestones/m9-agent-context-compaction.md`: add M9.3 plan link and completion record after implementation passes.

## Task 1: Add Craftsman Failing Tests

**Files:**
- Add: `apps/server/internal/agent/craftsman/contextcompact_test.go`
- Add: `apps/server/internal/agent/craftsman/model_contextcompact_test.go`

- [ ] **Step 1: Create Craftsman compaction test helpers**

Add `contextcompact_test.go` with the same fake-store shape used by Producer / Composer, but with Craftsman-specific names:

```go
package craftsman

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
)

func compactCraftsmanResponderTestConfig() contextcompact.Config {
	cfg := contextcompact.DefaultConfig()
	cfg.MicroTriggerTokens = 100
	cfg.MicroTargetTokens = 40
	cfg.MicroMinReductionTokens = 1
	cfg.PreserveRecentUserMessages = 1
	cfg.PreserveRecentTotalMessages = 1
	return cfg
}

func craftsmanContextcompactTestStore() *craftsmanCompactionStore {
	return &craftsmanCompactionStore{records: map[string]contextcompact.CompactionRecord{}}
}

func craftsmanContextcompactTestFileWriter() *craftsmanCompactionFileWriter {
	return &craftsmanCompactionFileWriter{}
}

type craftsmanCompactionStore struct {
	mu      sync.Mutex
	nextID  byte
	records map[string]contextcompact.CompactionRecord
	links   []contextcompact.LinkMessageInput
}

func (s *craftsmanCompactionStore) CreateCompaction(_ context.Context, input contextcompact.CreateCompactionInput) (contextcompact.CompactionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, ok := s.records[input.SemanticKey]; ok {
		return record, nil
	}
	s.nextID++
	record := contextcompact.CompactionRecord{
		ID:                     uuidWithByte(s.nextID),
		WorkspaceID:            input.WorkspaceID,
		ThreadID:               input.ThreadID,
		TaskID:                 input.TaskID,
		Role:                   input.Role,
		Mode:                   input.Mode,
		Trigger:                input.Trigger,
		SemanticKey:            input.SemanticKey,
		SourceSeqStart:         input.SourceSeqStart,
		SourceSeqEnd:           input.SourceSeqEnd,
		SourceMessageIDs:       append([]string(nil), input.SourceMessageIDs...),
		OriginalTokenEstimate:  input.OriginalTokenEstimate,
		CompactedTokenEstimate: input.CompactedTokenEstimate,
		OriginalBytes:          input.OriginalBytes,
		Summary:                input.Summary,
		DetailFiles:            append([]string(nil), input.DetailFiles...),
	}
	s.records[input.SemanticKey] = record
	return record, nil
}

func (s *craftsmanCompactionStore) LinkMessage(_ context.Context, input contextcompact.LinkMessageInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links = append(s.links, input)
	return nil
}

func (s *craftsmanCompactionStore) CompactedMessageIDs(context.Context, pgtype.UUID, pgtype.UUID) (map[pgtype.UUID]contextcompact.CompactionRecord, error) {
	return map[pgtype.UUID]contextcompact.CompactionRecord{}, nil
}

func (s *craftsmanCompactionStore) GetBySemanticKey(_ context.Context, _ pgtype.UUID, key string) (contextcompact.CompactionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[key]
	if !ok {
		return contextcompact.CompactionRecord{}, contextcompact.ErrCompactionNotFound
	}
	return record, nil
}

func (s *craftsmanCompactionStore) Search(context.Context, contextcompact.SearchInput) ([]contextcompact.CompactionRecord, error) {
	return nil, nil
}

type craftsmanCompactionFileWriter struct{}

func (craftsmanCompactionFileWriter) WriteDetailFile(_ context.Context, input contextcompact.DetailFileInput) (contextcompact.DetailFileResult, error) {
	return contextcompact.DetailFileResult{
		Path:  "/workspace/.clipanvil/context/" + input.Role + "-0-0-0123456789abcdef.md",
		Hash:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Bytes: int64(len([]byte(input.Original))),
	}, nil
}
```

- [ ] **Step 2: Create Craftsman responder projection test**

Add `model_contextcompact_test.go`:

```go
package craftsman

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestVolcengineModelResponderAppliesContextCompactionProjection(t *testing.T) {
	longResult := strings.Repeat("render plan probe result with source media details\n", 180)
	store := craftsmanContextcompactTestStore()
	model := &fakeCraftsmanArkModel{final: &schema.Message{Role: schema.Assistant, Content: "ok"}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:     compactCraftsmanResponderTestConfig(),
			Store:      store,
			FileWriter: craftsmanContextcompactTestFileWriter(),
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatModel, error) {
			return model, nil
		},
	})

	out, err := responder.Respond(context.Background(), Context{
		Input: GraphInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3)},
		Messages: []db.AgentMessage{
			{
				ID:          uuidWithByte(9),
				Role:        "tool",
				MessageType: "tool_result",
				Content:     mustCraftsmanToolResultContent(t, longResult),
				RawMessage:  mustCraftsmanToolResultRaw(t, "call-old", "read_project_memory", longResult),
			},
		},
		Text: "current render plan target must stay visible",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Metadata["context_compaction_applied"] != true {
		t.Fatalf("metadata missing compaction applied: %#v", out.Metadata)
	}
	if len(model.messages) < 3 {
		t.Fatalf("messages = %#v", model.messages)
	}
	if !strings.Contains(model.messages[1].Content, "compact_ref:") {
		t.Fatalf("old history was not compacted: %#v", model.messages[1])
	}
	if !strings.Contains(model.messages[len(model.messages)-1].Content, "current render plan target must stay visible") {
		t.Fatalf("current craftsman context was compacted: %#v", model.messages[len(model.messages)-1])
	}
	if len(store.links) != 1 || store.links[0].MessageID != uuidWithByte(9) {
		t.Fatalf("links = %#v, want source agent_message linked", store.links)
	}
}

type fakeCraftsmanArkModel struct {
	messages []*schema.Message
	final    *schema.Message
}

func (f *fakeCraftsmanArkModel) Generate(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.Message, error) {
	f.messages = messages
	return f.final, nil
}

func (f *fakeCraftsmanArkModel) Stream(context.Context, []*schema.Message, ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func mustCraftsmanToolResultContent(t *testing.T, text string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"text": text})
	if err != nil {
		t.Fatalf("marshal tool result content: %v", err)
	}
	return raw
}

func mustCraftsmanToolResultRaw(t *testing.T, toolCallID string, toolName string, text string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"tool_call_id": toolCallID,
		"tool_name":    toolName,
		"result_text":  text,
	})
	if err != nil {
		t.Fatalf("marshal tool result raw: %v", err)
	}
	return raw
}
```

- [ ] **Step 3: Run failing Craftsman test**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/craftsman -run 'ContextCompactionProjection' -count=1
```

Expected: FAIL because `VolcengineModelResponderConfig.ContextCompactor` does not exist in Craftsman.

## Task 2: Add Reviewer Failing Tests

**Files:**
- Add: `apps/server/internal/agent/reviewer/contextcompact_test.go`
- Add: `apps/server/internal/agent/reviewer/model_contextcompact_test.go`

- [ ] **Step 1: Create Reviewer compaction test helpers**

Add `contextcompact_test.go` mirroring the Craftsman helper but with Reviewer names:

```go
package reviewer

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
)

func compactReviewerResponderTestConfig() contextcompact.Config {
	cfg := contextcompact.DefaultConfig()
	cfg.MicroTriggerTokens = 100
	cfg.MicroTargetTokens = 40
	cfg.MicroMinReductionTokens = 1
	cfg.PreserveRecentUserMessages = 1
	cfg.PreserveRecentTotalMessages = 1
	return cfg
}

func reviewerContextcompactTestStore() *reviewerCompactionStore {
	return &reviewerCompactionStore{records: map[string]contextcompact.CompactionRecord{}}
}

func reviewerContextcompactTestFileWriter() *reviewerCompactionFileWriter {
	return &reviewerCompactionFileWriter{}
}

type reviewerCompactionStore struct {
	mu      sync.Mutex
	nextID  byte
	records map[string]contextcompact.CompactionRecord
	links   []contextcompact.LinkMessageInput
}

func (s *reviewerCompactionStore) CreateCompaction(_ context.Context, input contextcompact.CreateCompactionInput) (contextcompact.CompactionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, ok := s.records[input.SemanticKey]; ok {
		return record, nil
	}
	s.nextID++
	record := contextcompact.CompactionRecord{
		ID:               uuidWithByte(s.nextID),
		WorkspaceID:      input.WorkspaceID,
		ThreadID:         input.ThreadID,
		TaskID:           input.TaskID,
		Role:             input.Role,
		Mode:             input.Mode,
		Trigger:          input.Trigger,
		SemanticKey:      input.SemanticKey,
		SourceSeqStart:   input.SourceSeqStart,
		SourceSeqEnd:     input.SourceSeqEnd,
		SourceMessageIDs: append([]string(nil), input.SourceMessageIDs...),
		Summary:          input.Summary,
		DetailFiles:      append([]string(nil), input.DetailFiles...),
	}
	s.records[input.SemanticKey] = record
	return record, nil
}

func (s *reviewerCompactionStore) LinkMessage(_ context.Context, input contextcompact.LinkMessageInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links = append(s.links, input)
	return nil
}

func (s *reviewerCompactionStore) CompactedMessageIDs(context.Context, pgtype.UUID, pgtype.UUID) (map[pgtype.UUID]contextcompact.CompactionRecord, error) {
	return map[pgtype.UUID]contextcompact.CompactionRecord{}, nil
}

func (s *reviewerCompactionStore) GetBySemanticKey(_ context.Context, _ pgtype.UUID, key string) (contextcompact.CompactionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[key]
	if !ok {
		return contextcompact.CompactionRecord{}, contextcompact.ErrCompactionNotFound
	}
	return record, nil
}

func (s *reviewerCompactionStore) Search(context.Context, contextcompact.SearchInput) ([]contextcompact.CompactionRecord, error) {
	return nil, nil
}

type reviewerCompactionFileWriter struct{}

func (reviewerCompactionFileWriter) WriteDetailFile(_ context.Context, input contextcompact.DetailFileInput) (contextcompact.DetailFileResult, error) {
	return contextcompact.DetailFileResult{
		Path:  "/workspace/.clipanvil/context/" + input.Role + "-0-0-0123456789abcdef.md",
		Hash:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Bytes: int64(len([]byte(input.Original))),
	}, nil
}
```

- [ ] **Step 2: Create Reviewer projection test**

Add `model_contextcompact_test.go`:

```go
package reviewer

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	einoModel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestVolcengineModelResponderAppliesContextCompactionAndProtectsCurrentReviewMedia(t *testing.T) {
	longResult := strings.Repeat("prior review diagnostic with old artifact details\n", 180)
	store := reviewerContextcompactTestStore()
	streamer := &fakeReviewerArkStreamer{chunks: []*schema.Message{{Role: schema.Assistant, Content: "ok"}}}
	responder := NewVolcengineModelResponder(VolcengineModelResponderConfig{
		APIKey: "test-key",
		Model:  "doubao-test",
		ContextCompactor: contextcompact.NewMiddleware(contextcompact.MiddlewareConfig{
			Config:     compactReviewerResponderTestConfig(),
			Store:      store,
			FileWriter: reviewerContextcompactTestFileWriter(),
		}),
		Factory: func(context.Context, *ark.ChatModelConfig) (arkChatStreamer, error) {
			return streamer, nil
		},
	})

	out, err := responder.Respond(context.Background(), Context{
		Input: GraphInput{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3)},
		Messages: []db.AgentMessage{
			{
				ID:          uuidWithByte(9),
				Role:        "tool",
				MessageType: "tool_result",
				Content:     mustReviewerToolResultContent(t, longResult),
				RawMessage:  mustReviewerToolResultRaw(t, "call-old", "read_project_context", longResult),
			},
		},
		Text:      "review current artifact",
		AssetURL:  "data:image/png;base64,AAAA",
		AssetMime: "image/png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Metadata["context_compaction_applied"] != true {
		t.Fatalf("metadata missing compaction applied: %#v", out.Metadata)
	}
	if len(streamer.messages) < 3 {
		t.Fatalf("messages = %#v", streamer.messages)
	}
	if !strings.Contains(streamer.messages[1].Content, "compact_ref:") {
		t.Fatalf("old history was not compacted: %#v", streamer.messages[1])
	}
	current := streamer.messages[len(streamer.messages)-1]
	if current.Role != schema.User || len(current.UserInputMultiContent) == 0 {
		t.Fatalf("current review media input was not preserved: %#v", current)
	}
	if len(store.links) != 1 || store.links[0].MessageID != uuidWithByte(9) {
		t.Fatalf("links = %#v, want source agent_message linked", store.links)
	}
}

type fakeReviewerArkStreamer struct {
	messages []*schema.Message
	chunks   []*schema.Message
}

func (f *fakeReviewerArkStreamer) Stream(_ context.Context, messages []*schema.Message, _ ...einoModel.Option) (*schema.StreamReader[*schema.Message], error) {
	f.messages = messages
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		for _, chunk := range f.chunks {
			sw.Send(chunk, nil)
		}
		sw.Send(nil, io.EOF)
	}()
	return sr, nil
}

func mustReviewerToolResultContent(t *testing.T, text string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"text": text})
	if err != nil {
		t.Fatalf("marshal tool result content: %v", err)
	}
	return raw
}

func mustReviewerToolResultRaw(t *testing.T, toolCallID string, toolName string, text string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"tool_call_id": toolCallID,
		"tool_name":    toolName,
		"result_text":  text,
	})
	if err != nil {
		t.Fatalf("marshal tool result raw: %v", err)
	}
	return raw
}
```

- [ ] **Step 3: Run failing Reviewer test**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run 'ContextCompactionAndProtectsCurrentReviewMedia' -count=1
```

Expected: FAIL because `VolcengineModelResponderConfig.ContextCompactor` does not exist in Reviewer.

## Task 3: Implement Shared Prompt Boundary Helpers

**Files:**
- Modify: `apps/server/internal/agent/contextcompact/middleware.go`
- Add: `apps/server/internal/agent/contextcompact/boundary.go`
- Test: `apps/server/internal/agent/contextcompact/*_test.go`

- [ ] **Step 1: Add boundary helper tests**

Add tests in `boundary_test.go`:

```go
func TestCurrentToolLoopFromIndexProtectsLatestPair(t *testing.T) {
	if got := CurrentToolLoopFromIndex(2, 0); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
	if got := CurrentToolLoopFromIndex(2, 1); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
	if got := CurrentToolLoopFromIndex(2, 3); got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
}

func TestPendingReminderTargetIndexProtectsLatestUserOrTool(t *testing.T) {
	messages := []*schema.Message{
		schema.SystemMessage("system"),
		schema.AssistantMessage("assistant", nil),
		schema.UserMessage("user"),
	}
	if got := PendingReminderTargetIndex(messages, []string{"<system-reminder>x</system-reminder>"}); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
	if got := PendingReminderTargetIndex(messages, nil); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run failing boundary tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact -run 'CurrentToolLoopFromIndex|PendingReminderTargetIndex' -count=1
```

Expected: FAIL because helper functions do not exist.

- [ ] **Step 3: Implement helpers**

Create `boundary.go`:

```go
package contextcompact

import "github.com/cloudwego/eino/schema"

func CurrentToolLoopFromIndex(baseLen int, sameTurnCount int) int {
	if sameTurnCount == 0 {
		return 0
	}
	protectCount := 2
	if sameTurnCount < protectCount {
		protectCount = sameTurnCount
	}
	return baseLen + sameTurnCount - protectCount
}

func PendingReminderTargetIndex(messages []*schema.Message, reminders []string) int {
	if len(reminders) == 0 {
		return 0
	}
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil {
			continue
		}
		if message.Role == schema.User || message.Role == schema.Tool {
			return index
		}
	}
	return len(messages)
}
```

- [ ] **Step 4: Refactor Producer / Composer to use shared helpers**

Modify:

- `apps/server/internal/agent/producer/model_responder.go`
- `apps/server/internal/agent/composer/model_responder.go`

Replace package-local pending/current-loop helpers with:

```go
pendingFromIndex := contextcompact.PendingReminderTargetIndex(messages, producerContext.PendingReminders)
```

and:

```go
sameTurnFromIndex := contextcompact.CurrentToolLoopFromIndex(len(messages), len(composerContext.SameTurnMessages))
```

Producer may continue to protect all same-turn messages by using the first same-turn index. Composer, Craftsman, and Reviewer should protect only the latest pair to keep long loops bounded.

- [ ] **Step 5: Run boundary and existing projection tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/producer ./internal/agent/composer -run 'CurrentToolLoopFromIndex|PendingReminderTargetIndex|ContextCompactionProjection' -count=1
```

Expected: PASS.

## Task 4: Implement Craftsman Integration

**Files:**
- Modify: `apps/server/internal/agent/craftsman/model_responder.go`

- [ ] **Step 1: Add config field and import**

Add imports:

```go
import "github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
```

Add field:

```go
ContextCompactor contextcompact.Middleware
```

- [ ] **Step 2: Add prompt boundary type**

Add:

```go
type craftsmanPromptBoundary struct {
	Messages          []*schema.Message
	MessageRefs       []contextcompact.SourceMessageRef
	SameTurnFromIndex int
	PendingFromIndex  int
}
```

- [ ] **Step 3: Replace `craftsmanToolPromptMessages` body**

Keep existing function signature, but delegate to `craftsmanToolPromptMessagesWithBoundaries`:

```go
func craftsmanToolPromptMessages(craftsmanContext Context) []*schema.Message {
	return craftsmanToolPromptMessagesWithBoundaries(craftsmanContext).Messages
}
```

The new helper must:

- append system prompt;
- append `agentprompt.HistoryMessages(craftsmanContext.Messages)` while mapping emitted indexes to `db.AgentMessage.ID`;
- append current `craftsmanContext.Text`;
- append same-turn messages;
- protect latest same-turn pair with `contextcompact.CurrentToolLoopFromIndex`;
- protect pending reminder target with `contextcompact.PendingReminderTargetIndex`;
- return `MessageRefs`.

- [ ] **Step 4: Apply compactor before Generate**

Before `generator.Generate`:

```go
prompt := craftsmanToolPromptMessagesWithBoundaries(craftsmanContext)
messages := prompt.Messages
var compacted contextcompact.ProjectionOutput
if r.cfg.ContextCompactor != nil {
	compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
		WorkspaceID:       craftsmanContext.Input.WorkspaceID,
		ThreadID:          craftsmanContext.Input.ThreadID,
		TaskID:            craftsmanContext.Input.TaskID,
		Role:              "craftsman",
		ModelID:           modelID,
		Messages:          messages,
		MessageRefs:       prompt.MessageRefs,
		ToolInfos:         craftsmanContext.ToolInfos,
		Trigger:           "craftsman_before_model",
		SameTurnFromIndex: prompt.SameTurnFromIndex,
		PendingFromIndex:  prompt.PendingFromIndex,
	})
	if err != nil {
		return CraftsmanTurnOutput{}, fmt.Errorf("compact craftsman context: %w", err)
	}
	messages = compacted.Messages
}
final, err := generator.Generate(ctx, messages)
```

- [ ] **Step 5: Add metadata helper**

Add:

```go
func enrichCraftsmanContextCompactionMetadata(metadata map[string]any, output contextcompact.ProjectionOutput) {
	if len(output.Applied) == 0 {
		return
	}
	metadata["context_compaction_applied"] = true
	metadata["context_compaction_mode"] = output.CompactionMode
	metadata["context_compaction_count"] = len(output.Applied)
	metadata["context_compaction_refs"] = output.CompactionRefs
	metadata["context_compaction_detail_files"] = output.DetailFiles
}
```

Call it before returning `CraftsmanTurnOutput`.

- [ ] **Step 6: Run Craftsman test**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/craftsman -run 'ContextCompactionProjection|PendingReminders|PromptMessages' -count=1
```

Expected: PASS.

## Task 5: Implement Reviewer Integration

**Files:**
- Modify: `apps/server/internal/agent/reviewer/model_responder.go`

- [ ] **Step 1: Add config field and import**

Add:

```go
import "github.com/sinmaystar/clip-anvil/internal/agent/contextcompact"
```

Add field:

```go
ContextCompactor contextcompact.Middleware
```

- [ ] **Step 2: Add prompt boundary type**

Add:

```go
type reviewerPromptBoundary struct {
	Messages          []*schema.Message
	MessageRefs       []contextcompact.SourceMessageRef
	SameTurnFromIndex int
	PendingFromIndex  int
}
```

- [ ] **Step 3: Replace `reviewToolPromptMessages` body**

Keep existing function signature and delegate:

```go
func reviewToolPromptMessages(reviewContext Context) []*schema.Message {
	return reviewToolPromptMessagesWithBoundaries(reviewContext).Messages
}
```

The new helper must:

- append system prompt;
- append `agentprompt.HistoryMessages(reviewContext.Messages)` with source message refs;
- append `reviewUserMessage(reviewContext)` as a protected current review target;
- append same-turn messages;
- protect latest same-turn pair;
- protect pending reminder target.

- [ ] **Step 4: Apply compactor before Stream**

Before `streamer.Stream`:

```go
prompt := reviewToolPromptMessagesWithBoundaries(reviewContext)
messages := prompt.Messages
var compacted contextcompact.ProjectionOutput
if r.cfg.ContextCompactor != nil {
	compacted, err = r.cfg.ContextCompactor.Project(ctx, contextcompact.ProjectionInput{
		WorkspaceID:       reviewContext.Input.WorkspaceID,
		ThreadID:          reviewContext.Input.ThreadID,
		TaskID:            reviewContext.Input.TaskID,
		Role:              "reviewer",
		ModelID:           modelID,
		Messages:          messages,
		MessageRefs:       prompt.MessageRefs,
		ToolInfos:         reviewContext.ToolInfos,
		Trigger:           "reviewer_before_model",
		SameTurnFromIndex: prompt.SameTurnFromIndex,
		PendingFromIndex:  prompt.PendingFromIndex,
	})
	if err != nil {
		return ReviewerTurnOutput{}, fmt.Errorf("compact reviewer context: %w", err)
	}
	messages = compacted.Messages
}
stream, err := streamer.Stream(ctx, messages)
```

- [ ] **Step 5: Add metadata helper**

Add:

```go
func enrichReviewerContextCompactionMetadata(metadata map[string]any, output contextcompact.ProjectionOutput) {
	if len(output.Applied) == 0 {
		return
	}
	metadata["context_compaction_applied"] = true
	metadata["context_compaction_mode"] = output.CompactionMode
	metadata["context_compaction_count"] = len(output.Applied)
	metadata["context_compaction_refs"] = output.CompactionRefs
	metadata["context_compaction_detail_files"] = output.DetailFiles
}
```

Call it before returning `ReviewerTurnOutput`.

- [ ] **Step 6: Run Reviewer test**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run 'ContextCompactionAndProtectsCurrentReviewMedia|PendingReminders|ToolPromptMessages' -count=1
```

Expected: PASS.

## Task 6: Wire Craftsman And Reviewer In Server

**Files:**
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/cmd/server/main_test.go`

- [ ] **Step 1: Change responder factory signatures**

Change:

```go
func craftsmanResponderForConfig(cfg *config.Config) agentcraftsman.ToolCallingResponder
func reviewerResponderForConfig(cfg *config.Config) agentreviewer.ToolResponder
```

to:

```go
func craftsmanResponderForConfig(cfg *config.Config, contextCompactor agentcontextcompact.Middleware) agentcraftsman.ToolCallingResponder
func reviewerResponderForConfig(cfg *config.Config, contextCompactor agentcontextcompact.Middleware) agentreviewer.ToolResponder
```

- [ ] **Step 2: Pass compactor into Volcengine configs**

For Craftsman:

```go
ContextCompactor: contextCompactor,
```

For Reviewer:

```go
ContextCompactor: contextCompactor,
```

Do not attach compactor to deterministic fixture responders.

- [ ] **Step 3: Update call sites**

In `main()`:

```go
ToolResponder: craftsmanResponderForConfig(cfg, contextCompactor),
ToolResponder: reviewerResponderForConfig(cfg, contextCompactor),
```

In `cmd/server/main_test.go`, pass `nil` for tests that only assert fixture/deterministic behavior.

- [ ] **Step 4: Run server package tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./cmd/server -count=1
```

Expected: PASS.

## Task 7: Cross-Role Consistency Tests

**Files:**
- Modify: `apps/server/internal/agent/contextcompact/chat_list_boundary_test.go`
- Add: `apps/server/internal/agent/contextcompact/role_policy_test.go`

- [ ] **Step 1: Add role policy test**

Add `role_policy_test.go`:

```go
package contextcompact

import "testing"

func TestM93RoleNamesAreStable(t *testing.T) {
	for _, role := range []string{"producer", "craftsman", "reviewer", "composer"} {
		if safePathPart(role) != role {
			t.Fatalf("role %q is not stable for compaction paths", role)
		}
	}
}
```

- [ ] **Step 2: Extend chat-list boundary test**

Keep `TestCompactionCodeDoesNotUseAgentMessageUpdatePath` and ensure it still scans all production files in `apps/server/internal/agent/contextcompact`.

- [ ] **Step 3: Run cross-role focused tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer -run 'RoleNames|AgentMessageUpdatePath|ContextCompaction|Projection|PendingReminders' -count=1
```

Expected: PASS.

## Verification

Run the full M9.3 validation set:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer ./internal/agent/tools -run 'ContextCompaction|Projection|SearchAgentHistory|PendingReminders|ToolPromptMessages|PromptMessages|ComposerNativeToolsRegisterExpectedNames' -count=1
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

Manual inspection:

```bash
rg -n "UpdateAgentMessage|UpdateMessage\\(|ListAgentMessagesByThread" apps/server/internal/agent/contextcompact apps/server/internal/agent/producer/model_responder.go apps/server/internal/agent/craftsman/model_responder.go apps/server/internal/agent/reviewer/model_responder.go apps/server/internal/agent/composer/model_responder.go
```

Expected: only boundary tests mention those forbidden symbols.

## Acceptance Criteria

- Producer, Craftsman, Reviewer, and Composer all use `contextcompact.Middleware` for model-input projection in real Volcengine responders.
- Craftsman current `Context.Text` is not compacted.
- Reviewer current `reviewUserMessage`, including image multi-content, is not compacted.
- Producer pending signal reminders remain protected and signal claim / drain timing remains unchanged.
- Composer latest same-turn assistant/tool pair remains protected.
- Historical source `agent_message.id` links are written to `agent_message_compaction` when the role has persisted history.
- `search_agent_history` remains available to all four roles.
- No compaction code mutates `agent_message` or message-list API behavior.
- Focused tests, `make server-test`, and `git diff --check` pass.

## Completion Record

✅ Completed on 2026-06-30.

Implemented:

- Added shared `contextcompact.CurrentToolLoopFromIndex` and `contextcompact.PendingReminderTargetIndex` helpers.
- Kept Producer / Composer on the existing M9.2 projection path while aligning duplicated boundary logic.
- Added `ContextCompactor` to Craftsman / Reviewer Volcengine responder configs.
- Applied model-input-only projection before Craftsman `Generate` and Reviewer `Stream`.
- Preserved prompt index to source `agent_message.id` refs for Craftsman / Reviewer history messages emitted by `agentprompt.HistoryMessages`.
- Wired the shared `contextCompactor` from `cmd/server/main.go` into Craftsman / Reviewer factories.
- Ensured reused compaction records also write source message links.
- Added role policy, source-link, Craftsman projection, and Reviewer projection tests.

Verified:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer ./internal/agent/tools -run 'RoleNames|AgentMessageUpdatePath|ContextCompaction|Projection|SearchAgentHistory|PendingReminders|ToolPromptMessages|PromptMessages|ComposerNativeToolsRegisterExpectedNames' -count=1
GOCACHE=/private/tmp/clipanvil-go-build make server-test
rg -n "UpdateAgentMessage|UpdateMessage\\(|ListAgentMessagesByThread" apps/server/internal/agent/contextcompact apps/server/internal/agent/producer/model_responder.go apps/server/internal/agent/craftsman/model_responder.go apps/server/internal/agent/reviewer/model_responder.go apps/server/internal/agent/composer/model_responder.go
git diff --check
```

The `rg` inspection only matched `chat_list_boundary_test.go`, so compaction remains model-input-only and does not affect the Agent chat message list.
