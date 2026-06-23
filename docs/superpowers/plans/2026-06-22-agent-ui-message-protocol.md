# Agent UI Message Protocol Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the temporary Agent chat rendering shape with the versioned `clipanvil.agent.message.v1` block protocol, then rebuild message rendering and the composer around that protocol.

**Architecture:** Backend builders become the only supported way to create new Agent message content: `schema + blocks` is the UI source of truth, while `raw_message` remains diagnostics-only. WebSocket streaming emits block deltas, Producer prompt construction extracts model context from blocks, and the frontend renders blocks through a registry with a redesigned multi-line composer.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, PostgreSQL JSONB, Eino ProducerGraph, React 19, Vite 8, TypeScript 6, TanStack Query, WebSocket, `react-markdown`, `remark-gfm`, existing ClipAnvil Agent runtime.

---

## Source Specs

- `docs/superpowers/specs/2026-06-22-agent-ui-message-protocol-design.md`
- `docs/superpowers/specs/2026-06-21-m6-4-agent-tool-hitl-production-bridge-design.md`
- `docs/superpowers/specs/2026-06-21-m6-4-agent-chat-shell-attachments-design.md`

## Non-Negotiable Decisions

- Do not add a long-term `content.text` compatibility renderer.
- Do not add a data migration for old Agent messages.
- New Agent messages must persist `content.schema = "clipanvil.agent.message.v1"` and `content.blocks`.
- `message_type` may remain as a coarse database category, but UI rendering uses blocks.
- `raw_message` may store provider metadata and diagnostics, but ordinary UI rendering must not depend on it.
- If a model does not support visible thinking, reasoning deltas may be logged in diagnostics but must not become user-visible thinking blocks.
- Model and thinking controls belong in the composer toolbar, not the header.

## File Structure

Backend UI message protocol:

- Create `apps/server/internal/agent/uimessage/blocks.go`: Go block structs, schema constant, validation helpers, text extraction helpers.
- Create `apps/server/internal/agent/uimessage/builder.go`: builders for user, assistant, tool status, decision card, media, attachment, and error messages.
- Create `apps/server/internal/agent/uimessage/stream.go`: streaming block ids, delta payloads, and visible-thinking policy helpers.
- Create `apps/server/internal/agent/uimessage/blocks_test.go`: schema and extraction tests.
- Create `apps/server/internal/agent/uimessage/builder_test.go`: message content builder tests.
- Create `apps/server/internal/agent/uimessage/stream_test.go`: block delta and visible-thinking policy tests.

Backend runtime/API integration:

- Modify `apps/server/internal/api/agent_handler.go`: create user messages with blocks; attachment metadata becomes an attachment block.
- Modify `apps/server/internal/api/agent_response.go`: ensure DTO exposes block content without flattening.
- Modify `apps/server/internal/api/agent_broadcaster.go`: broadcast block delta payload shape.
- Modify `apps/server/internal/api/agent_handler_test.go`: assert new persisted content shape and response shape.
- Modify `apps/server/internal/agent/runtime/service.go`: keep append semantics but tests must assert content schema.
- Modify `apps/server/internal/agent/runtime/service_test.go`: update expected content.
- Modify `apps/server/internal/agent/producer/model_responder.go`: emit block deltas, write assistant blocks, suppress visible thinking for non-thinking models.
- Modify `apps/server/internal/agent/producer/context_loader.go`: load attachment references from attachment blocks.
- Modify `apps/server/internal/agent/producer/model_responder_test.go`: prompt extraction and responder metadata tests.
- Modify `apps/server/internal/agent/tools/executor.go`: persist tool status blocks for call/result messages.
- Modify `apps/server/internal/agent/tools/*_test.go`: assert tool status block content where tool messages are created.
- Modify `apps/server/internal/agent/hitl/service.go`: persist decision card blocks.
- Modify `apps/server/internal/agent/hitl/*_test.go`: assert decision card block content.

Frontend block protocol:

- Create `apps/web/src/lib/agentMessageBlocks.ts`: TypeScript block types, parsing, text extraction, attachment extraction, unsupported detection.
- Create `apps/web/src/lib/agentMessageBlocks.test.mjs`: parser and extraction tests.
- Modify `apps/web/src/lib/agentStreaming.ts`: stream state becomes block-based.
- Modify `apps/web/src/lib/agentStreaming.test.mjs`: block delta merge tests.
- Modify `apps/web/src/lib/agentWs.ts`: socket event type includes block delta fields.
- Modify `apps/web/src/lib/agentApi.ts`: `AgentMessage.content` gets typed block envelope.
- Remove or reduce `apps/web/src/lib/agentDecision.ts`: decision parsing should read decision blocks; keep helpers only if they operate on blocks.
- Modify `apps/web/src/lib/agentDecision.test.mjs`: decision block tests.

Frontend renderers:

- Create `apps/web/src/components/agent/AgentMessageRenderer.tsx`: registry and message shell.
- Create `apps/web/src/components/agent/AgentMarkdownBlock.tsx`: markdown renderer with GFM and `skipHtml`.
- Create `apps/web/src/components/agent/AgentThinkingBlock.tsx`: collapsible thinking renderer.
- Create `apps/web/src/components/agent/AgentDecisionCardBlock.tsx`: HITL card renderer.
- Create `apps/web/src/components/agent/AgentToolStatusBlock.tsx`: tool call/result renderer.
- Create `apps/web/src/components/agent/AgentAttachmentBlock.tsx`: attachment chips.
- Create `apps/web/src/components/agent/AgentMediaBlock.tsx`: media preview shells.
- Create `apps/web/src/components/agent/AgentErrorBlock.tsx`: error renderer.

Frontend composer:

- Create `apps/web/src/components/agent/AgentComposer.tsx`: multi-line composer, attachment chips, model selector, thinking selector, send button.
- Modify `apps/web/src/pages/AgentWorkspacePage.tsx`: remove inline message renderer and composer; wire new components.
- Modify `apps/web/src/main.css`: Agent block renderer, markdown, media, tool, decision, and composer styles.
- Modify `apps/web/package.json` and `apps/web/tsconfig.test.json`: include new tests if needed.

Verification:

- Use `./scripts/dev-start.sh` and the script-provided Vite URL for E2E.
- Use targeted tests after each task and full verification at the end.
- Current worktree is already dirty with M6 work. Execution must stage only files touched by this protocol work and must not commit unless the user explicitly asks.

---

## Task 1: Backend UI Message Block Types

**Files:**

- Create: `apps/server/internal/agent/uimessage/blocks.go`
- Create: `apps/server/internal/agent/uimessage/blocks_test.go`

- [ ] **Step 1: Write failing block type tests**

Create `apps/server/internal/agent/uimessage/blocks_test.go`:

```go
package uimessage

import (
	"encoding/json"
	"testing"
)

func TestEnvelopeSchemaAndBlocks(t *testing.T) {
	envelope := Envelope{
		Schema: SchemaV1,
		Blocks: []Block{
			MarkdownBlock{BaseBlock: BaseBlock{ID: "blk_text"}, Text: "hello"},
		},
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded["schema"] != "clipanvil.agent.message.v1" {
		t.Fatalf("schema = %#v", decoded["schema"])
	}
	blocks, ok := decoded["blocks"].([]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("blocks = %#v", decoded["blocks"])
	}
	first := blocks[0].(map[string]any)
	if first["type"] != "markdown" || first["text"] != "hello" {
		t.Fatalf("first block = %#v", first)
	}
}

func TestExtractMarkdownTextSkipsThinkingAndToolStatus(t *testing.T) {
	raw := []byte(`{
	  "schema":"clipanvil.agent.message.v1",
	  "blocks":[
	    {"id":"blk_thinking","type":"thinking","text":"hidden reasoning","status":"done","default_collapsed":true},
	    {"id":"blk_answer","type":"markdown","text":"visible answer"},
	    {"id":"blk_tool","type":"tool_status","tool_call_id":"call_1","tool_name":"read_workspace_context","label":"done","status":"succeeded"}
	  ]
	}`)
	texts := ExtractMarkdownTexts(raw)
	if len(texts) != 1 || texts[0] != "visible answer" {
		t.Fatalf("texts = %#v, want visible answer only", texts)
	}
}

func TestExtractAttachmentsFromBlocks(t *testing.T) {
	raw := []byte(`{
	  "schema":"clipanvil.agent.message.v1",
	  "blocks":[
	    {"id":"blk_text","type":"markdown","text":"see image"},
	    {"id":"blk_attachment","type":"attachment","attachments":[
	      {"asset_id":"asset-1","node_id":"node-1","kind":"image","name":"hero.png","mime":"image/png","size_bytes":123}
	    ]}
	  ]
	}`)
	attachments := ExtractAttachments(raw)
	if len(attachments) != 1 {
		t.Fatalf("attachments len = %d, want 1", len(attachments))
	}
	if attachments[0].AssetID != "asset-1" || attachments[0].Kind != "image" {
		t.Fatalf("attachment = %#v", attachments[0])
	}
}
```

- [ ] **Step 2: Run test to verify failure**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/uimessage -count=1
```

Expected: FAIL because package/types do not exist.

- [ ] **Step 3: Implement block types and extractors**

Create `apps/server/internal/agent/uimessage/blocks.go`:

```go
package uimessage

import (
	"encoding/json"
	"strings"
)

const SchemaV1 = "clipanvil.agent.message.v1"

type Envelope struct {
	Schema   string         `json:"schema"`
	Blocks   []Block        `json:"blocks"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Block interface {
	UIBlockType() string
}

type BaseBlock struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	CreatedAt  string `json:"created_at,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

type MarkdownBlock struct {
	BaseBlock
	Text string `json:"text"`
}

func (MarkdownBlock) UIBlockType() string { return "markdown" }

type ThinkingBlock struct {
	BaseBlock
	Text             string `json:"text"`
	Status           string `json:"status"`
	DefaultCollapsed bool   `json:"default_collapsed"`
}

func (ThinkingBlock) UIBlockType() string { return "thinking" }

type DecisionOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type DecisionCardBlock struct {
	BaseBlock
	DecisionID       string           `json:"decision_id"`
	Title            string           `json:"title"`
	Message          string           `json:"message"`
	Options          []DecisionOption `json:"options"`
	AllowFreeText    bool             `json:"allow_free_text"`
	Status           string           `json:"status"`
	SelectedOptionID string           `json:"selected_option_id,omitempty"`
	FreeText         string           `json:"free_text,omitempty"`
}

func (DecisionCardBlock) UIBlockType() string { return "decision_card" }

type ToolStatusBlock struct {
	BaseBlock
	ToolCallID   string `json:"tool_call_id"`
	ToolName     string `json:"tool_name"`
	Label        string `json:"label"`
	Status       string `json:"status"`
	Summary      string `json:"summary,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

func (ToolStatusBlock) UIBlockType() string { return "tool_status" }

type Attachment struct {
	AssetID   string `json:"asset_id"`
	NodeID    string `json:"node_id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	SizeBytes int64  `json:"size_bytes"`
}

type AttachmentBlock struct {
	BaseBlock
	Attachments []Attachment `json:"attachments"`
}

func (AttachmentBlock) UIBlockType() string { return "attachment" }

type MediaBlock struct {
	BaseBlock
	AssetID      string `json:"asset_id"`
	NodeID       string `json:"node_id,omitempty"`
	Kind         string `json:"kind"`
	Title        string `json:"title,omitempty"`
	URL          string `json:"url,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Mime         string `json:"mime,omitempty"`
}

func (MediaBlock) UIBlockType() string { return "media" }

type ErrorBlock struct {
	BaseBlock
	Title     string `json:"title"`
	Message   string `json:"message"`
	Code      string `json:"code,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

func (ErrorBlock) UIBlockType() string { return "error" }

func NewBaseBlock(id string, blockType string) BaseBlock {
	return BaseBlock{ID: strings.TrimSpace(id), Type: blockType}
}

func ExtractMarkdownTexts(raw []byte) []string {
	var envelope struct {
		Schema string `json:"schema"`
		Blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Schema != SchemaV1 {
		return nil
	}
	out := make([]string, 0, len(envelope.Blocks))
	for _, block := range envelope.Blocks {
		if block.Type != "markdown" {
			continue
		}
		text := strings.TrimSpace(block.Text)
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func ExtractAttachments(raw []byte) []Attachment {
	var envelope struct {
		Schema string `json:"schema"`
		Blocks []struct {
			Type        string       `json:"type"`
			Attachments []Attachment `json:"attachments"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Schema != SchemaV1 {
		return nil
	}
	out := []Attachment{}
	for _, block := range envelope.Blocks {
		if block.Type == "attachment" {
			out = append(out, block.Attachments...)
		}
	}
	return out
}
```

- [ ] **Step 4: Run focused tests**

```bash
gofmt -w apps/server/internal/agent/uimessage/blocks.go apps/server/internal/agent/uimessage/blocks_test.go
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/uimessage -count=1
```

Expected: PASS.

---

## Task 2: Backend Message Builders

**Files:**

- Create: `apps/server/internal/agent/uimessage/builder.go`
- Create: `apps/server/internal/agent/uimessage/builder_test.go`

- [ ] **Step 1: Write failing builder tests**

Create `apps/server/internal/agent/uimessage/builder_test.go`:

```go
package uimessage

import (
	"encoding/json"
	"testing"
)

func TestBuildUserMessageContentIncludesMarkdownAndAttachments(t *testing.T) {
	raw, err := BuildUserMessageContent(UserMessageInput{
		Text:            "第一行\n第二行",
		ClientMessageID: "client-1",
		Attachments: []Attachment{{
			AssetID: "asset-1", NodeID: "node-1", Kind: "image", Name: "hero.png", Mime: "image/png", SizeBytes: 123,
		}},
	})
	if err != nil {
		t.Fatalf("BuildUserMessageContent() error = %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if envelope["schema"] != SchemaV1 {
		t.Fatalf("schema = %#v", envelope["schema"])
	}
	if got := ExtractMarkdownTexts(raw); len(got) != 1 || got[0] != "第一行\n第二行" {
		t.Fatalf("markdown texts = %#v", got)
	}
	if got := ExtractAttachments(raw); len(got) != 1 || got[0].Name != "hero.png" {
		t.Fatalf("attachments = %#v", got)
	}
}

func TestBuildAssistantMessageContentOmitsEmptyThinking(t *testing.T) {
	raw, err := BuildAssistantMessageContent(AssistantMessageInput{
		Text:              "最终回复",
		ReasoningContent:  "   ",
		IncludeThinking:   true,
		DefaultCollapsed:  true,
	})
	if err != nil {
		t.Fatalf("BuildAssistantMessageContent() error = %v", err)
	}
	if string(raw) == "" {
		t.Fatal("expected content")
	}
	if len(ExtractMarkdownTexts(raw)) != 1 {
		t.Fatalf("expected one markdown block: %s", raw)
	}
	var envelope struct {
		Blocks []struct {
			Type string `json:"type"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, block := range envelope.Blocks {
		if block.Type == "thinking" {
			t.Fatalf("unexpected thinking block: %s", raw)
		}
	}
}

func TestBuildAssistantMessageContentIncludesVisibleThinking(t *testing.T) {
	raw, err := BuildAssistantMessageContent(AssistantMessageInput{
		Text:              "最终回复",
		ReasoningContent:  "先分析",
		IncludeThinking:   true,
		DefaultCollapsed:  true,
	})
	if err != nil {
		t.Fatalf("BuildAssistantMessageContent() error = %v", err)
	}
	var envelope struct {
		Blocks []struct {
			Type             string `json:"type"`
			Text             string `json:"text"`
			Status           string `json:"status"`
			DefaultCollapsed bool   `json:"default_collapsed"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(envelope.Blocks) != 2 || envelope.Blocks[0].Type != "thinking" || envelope.Blocks[1].Type != "markdown" {
		t.Fatalf("blocks = %#v", envelope.Blocks)
	}
	if envelope.Blocks[0].Text != "先分析" || envelope.Blocks[0].Status != "done" || !envelope.Blocks[0].DefaultCollapsed {
		t.Fatalf("thinking block = %#v", envelope.Blocks[0])
	}
}

func TestBuildToolStatusAndDecisionCardContent(t *testing.T) {
	toolRaw, err := BuildToolStatusMessageContent(ToolStatusInput{
		ToolCallID: "call-1",
		ToolName:   "read_workspace_context",
		Label:      "工具执行完成",
		Status:     "succeeded",
	})
	if err != nil {
		t.Fatalf("BuildToolStatusMessageContent() error = %v", err)
	}
	if !containsBlockType(t, toolRaw, "tool_status") {
		t.Fatalf("tool content missing tool_status: %s", toolRaw)
	}

	cardRaw, err := BuildDecisionCardMessageContent(DecisionCardInput{
		DecisionID:    "decision-1",
		Title:         "确认方向",
		Message:       "请选择",
		AllowFreeText: true,
		Status:        "pending",
		Options:       []DecisionOption{{ID: "a", Label: "方案 A"}},
	})
	if err != nil {
		t.Fatalf("BuildDecisionCardMessageContent() error = %v", err)
	}
	if !containsBlockType(t, cardRaw, "decision_card") {
		t.Fatalf("decision content missing decision_card: %s", cardRaw)
	}
}

func containsBlockType(t *testing.T, raw []byte, blockType string) bool {
	t.Helper()
	var envelope struct {
		Blocks []struct {
			Type string `json:"type"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, block := range envelope.Blocks {
		if block.Type == blockType {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run failing tests**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/uimessage -run 'TestBuild' -count=1
```

Expected: FAIL because builders do not exist.

- [ ] **Step 3: Implement builders**

Create `apps/server/internal/agent/uimessage/builder.go`:

```go
package uimessage

import (
	"encoding/json"
	"strings"
)

type UserMessageInput struct {
	Text            string
	ClientMessageID string
	Attachments     []Attachment
}

type AssistantMessageInput struct {
	Text             string
	ReasoningContent string
	IncludeThinking  bool
	DefaultCollapsed bool
}

type ToolStatusInput struct {
	ToolCallID   string
	ToolName     string
	Label        string
	Status       string
	Summary      string
	ErrorMessage string
}

type DecisionCardInput struct {
	DecisionID    string
	Title         string
	Message       string
	Options       []DecisionOption
	AllowFreeText bool
	Status        string
}

func BuildUserMessageContent(input UserMessageInput) ([]byte, error) {
	blocks := []Block{
		MarkdownBlock{
			BaseBlock: NewBaseBlock("blk_text", "markdown"),
			Text:      strings.TrimSpace(input.Text),
		},
	}
	if len(input.Attachments) > 0 {
		blocks = append(blocks, AttachmentBlock{
			BaseBlock:   NewBaseBlock("blk_attachments", "attachment"),
			Attachments: input.Attachments,
		})
	}
	metadata := map[string]any{}
	if strings.TrimSpace(input.ClientMessageID) != "" {
		metadata["client_message_id"] = strings.TrimSpace(input.ClientMessageID)
	}
	return marshalEnvelope(blocks, metadata)
}

func BuildAssistantMessageContent(input AssistantMessageInput) ([]byte, error) {
	blocks := []Block{}
	reasoning := strings.TrimSpace(input.ReasoningContent)
	if input.IncludeThinking && reasoning != "" {
		blocks = append(blocks, ThinkingBlock{
			BaseBlock:        NewBaseBlock("blk_thinking", "thinking"),
			Text:             reasoning,
			Status:           "done",
			DefaultCollapsed: input.DefaultCollapsed,
		})
	}
	text := strings.TrimSpace(input.Text)
	if text != "" {
		blocks = append(blocks, MarkdownBlock{
			BaseBlock: NewBaseBlock("blk_answer", "markdown"),
			Text:      text,
		})
	}
	return marshalEnvelope(blocks, nil)
}

func BuildToolStatusMessageContent(input ToolStatusInput) ([]byte, error) {
	return marshalEnvelope([]Block{
		ToolStatusBlock{
			BaseBlock:    NewBaseBlock("blk_tool_status", "tool_status"),
			ToolCallID:  strings.TrimSpace(input.ToolCallID),
			ToolName:    strings.TrimSpace(input.ToolName),
			Label:       strings.TrimSpace(input.Label),
			Status:      strings.TrimSpace(input.Status),
			Summary:     strings.TrimSpace(input.Summary),
			ErrorMessage: strings.TrimSpace(input.ErrorMessage),
		},
	}, nil)
}

func BuildDecisionCardMessageContent(input DecisionCardInput) ([]byte, error) {
	return marshalEnvelope([]Block{
		DecisionCardBlock{
			BaseBlock:     NewBaseBlock("blk_decision", "decision_card"),
			DecisionID:    strings.TrimSpace(input.DecisionID),
			Title:         strings.TrimSpace(input.Title),
			Message:       strings.TrimSpace(input.Message),
			Options:       input.Options,
			AllowFreeText: input.AllowFreeText,
			Status:        strings.TrimSpace(input.Status),
		},
	}, nil)
}

func marshalEnvelope(blocks []Block, metadata map[string]any) ([]byte, error) {
	if metadata != nil && len(metadata) == 0 {
		metadata = nil
	}
	return json.Marshal(Envelope{
		Schema:   SchemaV1,
		Blocks:   blocks,
		Metadata: metadata,
	})
}
```

- [ ] **Step 4: Run builder tests**

```bash
gofmt -w apps/server/internal/agent/uimessage/builder.go apps/server/internal/agent/uimessage/builder_test.go
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/uimessage -count=1
```

Expected: PASS.

---

## Task 3: User Message Persistence Uses Blocks

**Files:**

- Modify: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/server/internal/api/agent_handler_test.go`
- Modify: `apps/server/internal/agent/runtime/service_test.go`

- [ ] **Step 1: Add API test for block content**

In `apps/server/internal/api/agent_handler_test.go`, add or update a POST message test:

```go
func TestPostAgentMessagePersistsUIMessageBlocks(t *testing.T) {
	h, workspaceID, token := newAgentHandlerTestHarness(t)
	body := strings.NewReader(`{
	  "text": "第一行\n第二行",
	  "client_message_id": "client-blocks-1"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/workspaces/"+workspaceID+"/messages", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp := performAgentRequest(h, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Message struct {
			Content struct {
				Schema string `json:"schema"`
				Blocks []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"blocks"`
				Metadata map[string]string `json:"metadata"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload.Message.Content.Schema != uimessage.SchemaV1 {
		t.Fatalf("schema = %q", payload.Message.Content.Schema)
	}
	if len(payload.Message.Content.Blocks) != 1 || payload.Message.Content.Blocks[0].Type != "markdown" {
		t.Fatalf("blocks = %#v", payload.Message.Content.Blocks)
	}
	if payload.Message.Content.Blocks[0].Text != "第一行\n第二行" {
		t.Fatalf("text = %q", payload.Message.Content.Blocks[0].Text)
	}
	if payload.Message.Content.Metadata["client_message_id"] != "client-blocks-1" {
		t.Fatalf("metadata = %#v", payload.Message.Content.Metadata)
	}
}
```

Use the existing handler test harness names if they differ; keep the assertion shape exactly.

- [ ] **Step 2: Run failing API test**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run TestPostAgentMessagePersistsUIMessageBlocks -count=1
```

Expected: FAIL because POST message still writes `content.text`.

- [ ] **Step 3: Replace user message content construction**

In `apps/server/internal/api/agent_handler.go`, replace the current user content map with:

```go
content, err := uimessage.BuildUserMessageContent(uimessage.UserMessageInput{
	Text:            req.trimmedText(),
	ClientMessageID: req.ClientMessageID,
	Attachments:     toUIMessageAttachments(req.Attachments),
})
if err != nil {
	writeError(c, consts.StatusInternalServerError, "failed to build agent message")
	return
}
```

Add helper in the same file:

```go
func toUIMessageAttachments(attachments []agentMessageAttachment) []uimessage.Attachment {
	out := make([]uimessage.Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, uimessage.Attachment{
			AssetID:   attachment.AssetID,
			NodeID:    attachment.NodeID,
			Kind:      attachment.Kind,
			Name:      attachment.Name,
			Mime:      attachment.Mime,
			SizeBytes: attachment.SizeBytes,
		})
	}
	return out
}
```

Pass `content` to `AppendMessage`.

- [ ] **Step 4: Update runtime service test expectations**

In `apps/server/internal/agent/runtime/service_test.go`, replace assertions that expect `content.text` with schema/block assertions:

```go
var content struct {
	Schema string `json:"schema"`
	Blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"blocks"`
}
if err := json.Unmarshal(msg.Content, &content); err != nil {
	t.Fatalf("Unmarshal() error = %v", err)
}
if content.Schema != uimessage.SchemaV1 {
	t.Fatalf("schema = %q", content.Schema)
}
if len(content.Blocks) != 1 || content.Blocks[0].Text != "hello" {
	t.Fatalf("blocks = %#v", content.Blocks)
}
```

- [ ] **Step 5: Run focused verification**

```bash
gofmt -w apps/server/internal/api/agent_handler.go apps/server/internal/api/agent_handler_test.go apps/server/internal/agent/runtime/service_test.go
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api ./internal/agent/runtime -count=1
git diff --check
```

Expected: PASS.

---

## Task 4: Assistant Messages, Visible Thinking Policy, And Block Deltas

**Files:**

- Create: `apps/server/internal/agent/uimessage/stream.go`
- Create: `apps/server/internal/agent/uimessage/stream_test.go`
- Modify: `apps/server/internal/agent/producer/model_responder.go`
- Modify: `apps/server/internal/agent/producer/model_responder_test.go`
- Modify: `apps/server/internal/api/agent_broadcaster.go`
- Modify: `apps/web/src/lib/agentWs.ts`

- [ ] **Step 1: Write visible-thinking policy and delta tests**

Create `apps/server/internal/agent/uimessage/stream_test.go`:

```go
package uimessage

import "testing"

func TestShouldShowThinkingRequiresSupportAndNonMinimalEffort(t *testing.T) {
	cases := []struct {
		name     string
		supports bool
		effort   string
		want     bool
	}{
		{name: "unsupported high hidden", supports: false, effort: "high", want: false},
		{name: "supported minimal hidden", supports: true, effort: "minimal", want: false},
		{name: "supported empty hidden", supports: true, effort: "", want: false},
		{name: "supported low visible", supports: true, effort: "low", want: true},
		{name: "supported medium visible", supports: true, effort: "medium", want: true},
		{name: "supported high visible", supports: true, effort: "high", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldShowThinking(tc.supports, tc.effort); got != tc.want {
				t.Fatalf("ShouldShowThinking() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStreamDeltaPayloadUsesBlockShape(t *testing.T) {
	payload := NewStreamDelta(StreamDeltaInput{
		WorkspaceID: "workspace-1",
		ThreadID:    "thread-1",
		TaskID:      "task-1",
		BlockID:     "blk_answer",
		BlockType:   "markdown",
		Delta:       "hello",
		Sequence:    3,
	})
	if payload.BlockID != "blk_answer" || payload.BlockType != "markdown" || payload.Sequence != 3 {
		t.Fatalf("payload = %#v", payload)
	}
}
```

- [ ] **Step 2: Run failing tests**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/uimessage -run 'TestShouldShowThinking|TestStreamDeltaPayload' -count=1
```

Expected: FAIL because stream helpers do not exist.

- [ ] **Step 3: Implement stream helpers**

Create `apps/server/internal/agent/uimessage/stream.go`:

```go
package uimessage

import "strings"

const (
	BlockIDAnswer   = "blk_answer"
	BlockIDThinking = "blk_thinking"
)

type StreamDeltaInput struct {
	WorkspaceID string
	ThreadID    string
	TaskID      string
	MessageID   string
	BlockID     string
	BlockType   string
	Delta       string
	Sequence    int64
}

type StreamDeltaPayload struct {
	WorkspaceID string `json:"workspace_id"`
	ThreadID    string `json:"thread_id"`
	TaskID      string `json:"task_id"`
	MessageID   string `json:"message_id,omitempty"`
	BlockID     string `json:"block_id"`
	BlockType   string `json:"block_type"`
	Delta       string `json:"delta"`
	Sequence    int64  `json:"sequence"`
}

func ShouldShowThinking(supportsThinking bool, effort string) bool {
	if !supportsThinking {
		return false
	}
	switch strings.TrimSpace(effort) {
	case "low", "medium", "high":
		return true
	default:
		return false
	}
}

func NewStreamDelta(input StreamDeltaInput) StreamDeltaPayload {
	return StreamDeltaPayload{
		WorkspaceID: input.WorkspaceID,
		ThreadID:    input.ThreadID,
		TaskID:      input.TaskID,
		MessageID:   input.MessageID,
		BlockID:     input.BlockID,
		BlockType:   input.BlockType,
		Delta:       input.Delta,
		Sequence:    input.Sequence,
	}
}
```

- [ ] **Step 4: Update Producer stream emission**

In `apps/server/internal/agent/producer/model_responder.go`:

- Replace content delta broadcasts with `BlockID: uimessage.BlockIDAnswer`, `BlockType: "markdown"`.
- Broadcast thinking deltas only when `uimessage.ShouldShowThinking(model.SupportsThinking, model.ReasoningEffort)` is true.
- Track a monotonically increasing sequence per task.
- Build final assistant content using:

```go
content, err := uimessage.BuildAssistantMessageContent(uimessage.AssistantMessageInput{
	Text:             finalText,
	ReasoningContent: reasoningContent,
	IncludeThinking:  uimessage.ShouldShowThinking(producerContext.Model.SupportsThinking, producerContext.Model.ReasoningEffort),
	DefaultCollapsed: true,
})
```

- Keep `reasoning_content` in `raw_message` diagnostics metadata for debugging even when no visible thinking block is emitted.

- [ ] **Step 5: Update backend broadcaster event shape**

In `apps/server/internal/api/agent_broadcaster.go`, update `BroadcastAgentMessageDelta` to emit:

```go
b.hub.Broadcast(workspaceID, AgentSocketEvent{
	Type: "agent.message.delta",
	Payload: map[string]any{
		"workspace_id": uuidToString(workspaceID),
		"thread_id":    delta.ThreadID,
		"task_id":      delta.TaskID,
		"message_id":   delta.MessageID,
		"block_id":     delta.BlockID,
		"block_type":   delta.BlockType,
		"delta":        delta.Delta,
		"sequence":     delta.Sequence,
	},
})
```

Use existing UUID helpers and current `ProducerStreamDelta` field names; if `ProducerStreamDelta` does not yet have these fields, extend it in `apps/server/internal/agent/producer/types.go`.

- [ ] **Step 6: Update producer tests**

In `apps/server/internal/agent/producer/model_responder_test.go`, update or add:

```go
func TestResponderWritesAssistantBlocksAndSuppressesUnsupportedThinking(t *testing.T) {
	out := runResponderWithModel(t, ProducerModelSelection{
		ProviderID:       "volcengine",
		ModelID:          "doubao-mini",
		SupportsThinking: false,
		ReasoningEffort:  "high",
	}, fakeStream{
		thinking: "provider returned hidden reasoning",
		content:  "## 标题\n\n- 要点",
	})
	var content struct {
		Schema string `json:"schema"`
		Blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(out.Content, &content); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if content.Schema != uimessage.SchemaV1 {
		t.Fatalf("schema = %q", content.Schema)
	}
	for _, block := range content.Blocks {
		if block.Type == "thinking" {
			t.Fatalf("unexpected visible thinking block: %#v", content.Blocks)
		}
	}
	if len(content.Blocks) != 1 || content.Blocks[0].Text != "## 标题\n\n- 要点" {
		t.Fatalf("blocks = %#v", content.Blocks)
	}
}
```

Use existing fake responder helpers where possible; keep the exact assertion that unsupported thinking is not visible.

- [ ] **Step 7: Run focused verification**

```bash
gofmt -w apps/server/internal/agent/uimessage/stream.go apps/server/internal/agent/uimessage/stream_test.go apps/server/internal/agent/producer apps/server/internal/api/agent_broadcaster.go
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/uimessage ./internal/agent/producer ./internal/api -count=1
git diff --check
```

Expected: PASS.

---

## Task 5: Prompt Context Reads Blocks

**Files:**

- Modify: `apps/server/internal/agent/producer/model_responder.go`
- Modify: `apps/server/internal/agent/producer/context_loader.go`
- Modify: `apps/server/internal/agent/producer/model_responder_test.go`
- Modify: `apps/server/internal/agent/producer/context_loader_test.go` if present; otherwise add tests to `model_responder_test.go`.

- [ ] **Step 1: Add prompt extraction tests**

Add tests in `apps/server/internal/agent/producer/model_responder_test.go`:

```go
func TestProducerPromptMessagesUseMarkdownBlocks(t *testing.T) {
	userContent := mustUIContent(t, uimessage.BuildUserMessageContent(uimessage.UserMessageInput{Text: "用户需求"}))
	assistantContent := mustUIContent(t, uimessage.BuildAssistantMessageContent(uimessage.AssistantMessageInput{Text: "助手回复"}))
	messages := producerPromptMessages(ProducerContext{
		Messages: []db.AgentMessage{
			{Role: "user", MessageType: "text", Content: userContent},
			{Role: "assistant", MessageType: "text", Content: assistantContent},
		},
	})
	if len(messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(messages))
	}
	if messages[1].Role != schema.User || messages[1].Content != "用户需求" {
		t.Fatalf("user prompt = %#v", messages[1])
	}
	if messages[2].Role != schema.Assistant || messages[2].Content != "助手回复" {
		t.Fatalf("assistant prompt = %#v", messages[2])
	}
}

func TestProducerPromptMessagesSkipThinkingBlocks(t *testing.T) {
	content := []byte(`{
	  "schema":"clipanvil.agent.message.v1",
	  "blocks":[
	    {"id":"blk_thinking","type":"thinking","text":"不要进 prompt","status":"done","default_collapsed":true},
	    {"id":"blk_answer","type":"markdown","text":"只保留正文"}
	  ]
	}`)
	messages := producerPromptMessages(ProducerContext{
		Messages: []db.AgentMessage{{Role: "assistant", MessageType: "text", Content: content}},
	})
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if strings.Contains(messages[1].Content, "不要进 prompt") {
		t.Fatalf("prompt leaked thinking: %#v", messages[1])
	}
}

func mustUIContent(t *testing.T, raw []byte, err error) []byte {
	t.Helper()
	if err != nil {
		t.Fatalf("build content error = %v", err)
	}
	return raw
}
```

- [ ] **Step 2: Run failing tests**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'TestProducerPromptMessagesUseMarkdownBlocks|TestProducerPromptMessagesSkipThinkingBlocks' -count=1
```

Expected: FAIL until `agentMessageText` reads blocks.

- [ ] **Step 3: Update prompt text extraction**

In `apps/server/internal/agent/producer/model_responder.go`, replace `agentMessageText(msg.Content)` usage with a block-aware helper:

```go
func agentMessageText(raw []byte) string {
	texts := uimessage.ExtractMarkdownTexts(raw)
	if len(texts) > 0 {
		return strings.Join(texts, "\n\n")
	}
	return ""
}
```

Remove old `content.text` fallback except in tests that create non-v1 content. The protocol direction is no long-term legacy fallback.

- [ ] **Step 4: Update attachment loading**

In `apps/server/internal/agent/producer/context_loader.go`, replace parsing of `content.attachments` with:

```go
for _, attachment := range uimessage.ExtractAttachments(msg.Content) {
	if strings.TrimSpace(attachment.Kind) != "image" {
		continue
	}
	assetID := strings.TrimSpace(attachment.AssetID)
	...
}
```

Map `uimessage.Attachment` fields into existing `ProducerImageAttachment`.

- [ ] **Step 5: Run focused verification**

```bash
gofmt -w apps/server/internal/agent/producer/model_responder.go apps/server/internal/agent/producer/context_loader.go apps/server/internal/agent/producer/model_responder_test.go
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -count=1
git diff --check
```

Expected: PASS.

---

## Task 6: Tool Status And Decision Card Blocks

**Files:**

- Modify: `apps/server/internal/agent/tools/executor.go`
- Modify: `apps/server/internal/agent/tools/*_test.go`
- Modify: `apps/server/internal/agent/hitl/service.go`
- Modify: `apps/server/internal/agent/hitl/*_test.go`
- Modify: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/server/internal/api/agent_handler_test.go`

- [ ] **Step 1: Add tool status persistence test**

In the existing tool executor test file, add:

```go
func TestToolExecutorPersistsToolStatusBlocks(t *testing.T) {
	out := runFakeToolExecution(t, fakeToolResult{
		ToolCallID: "call-1",
		ToolName:   "read_workspace_context",
		Label:      "工具执行完成",
		Status:     "succeeded",
	})
	var content struct {
		Schema string `json:"schema"`
		Blocks []struct {
			Type       string `json:"type"`
			ToolName   string `json:"tool_name"`
			ToolCallID string `json:"tool_call_id"`
			Status     string `json:"status"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(out.ToolResultMessage.Content, &content); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if content.Schema != uimessage.SchemaV1 {
		t.Fatalf("schema = %q", content.Schema)
	}
	if len(content.Blocks) != 1 || content.Blocks[0].Type != "tool_status" {
		t.Fatalf("blocks = %#v", content.Blocks)
	}
	if content.Blocks[0].ToolName != "read_workspace_context" || content.Blocks[0].Status != "succeeded" {
		t.Fatalf("tool block = %#v", content.Blocks[0])
	}
}
```

Use the executor test harness names already present in the package. If none exist, create the smallest fake repository needed to capture `CreateAgentMessageParams`.

- [ ] **Step 2: Add decision card block test**

In `apps/server/internal/agent/hitl/service_test.go`, add:

```go
func TestDecisionRequestPersistsDecisionCardBlock(t *testing.T) {
	out := requestFakeDecision(t, DecisionRequest{
		DecisionID:    "decision-1",
		Title:         "确认方向",
		Message:       "请选择一个方向",
		Options:       []DecisionOption{{ID: "a", Label: "方案 A"}},
		AllowFreeText: true,
	})
	var content struct {
		Schema string `json:"schema"`
		Blocks []struct {
			Type          string `json:"type"`
			DecisionID    string `json:"decision_id"`
			Title         string `json:"title"`
			AllowFreeText bool   `json:"allow_free_text"`
			Status        string `json:"status"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(out.Message.Content, &content); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if content.Schema != uimessage.SchemaV1 {
		t.Fatalf("schema = %q", content.Schema)
	}
	if len(content.Blocks) != 1 || content.Blocks[0].Type != "decision_card" {
		t.Fatalf("blocks = %#v", content.Blocks)
	}
	if content.Blocks[0].DecisionID != "decision-1" || content.Blocks[0].Status != "pending" {
		t.Fatalf("decision block = %#v", content.Blocks[0])
	}
}
```

- [ ] **Step 3: Run failing tests**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./internal/agent/hitl -count=1
```

Expected: FAIL where old `content.card_type` or old tool text is still written.

- [ ] **Step 4: Use UI message builders**

In tool execution code, replace old content payloads:

```go
content, err := uimessage.BuildToolStatusMessageContent(uimessage.ToolStatusInput{
	ToolCallID:   result.ToolCallID,
	ToolName:     result.ToolName,
	Label:        result.Label,
	Status:       result.Status,
	Summary:      result.Summary,
	ErrorMessage: result.ErrorMessage,
})
```

In HITL decision request code:

```go
content, err := uimessage.BuildDecisionCardMessageContent(uimessage.DecisionCardInput{
	DecisionID:    decisionID,
	Title:         title,
	Message:       message,
	Options:       toUIMessageDecisionOptions(options),
	AllowFreeText: allowFreeText,
	Status:        "pending",
})
```

Add option mapper:

```go
func toUIMessageDecisionOptions(options []DecisionOption) []uimessage.DecisionOption {
	out := make([]uimessage.DecisionOption, 0, len(options))
	for _, option := range options {
		out = append(out, uimessage.DecisionOption{
			ID:          option.ID,
			Label:       option.Label,
			Description: option.Description,
		})
	}
	return out
}
```

- [ ] **Step 5: Update decision response to write resolved block**

When a user responds to a decision, persist the user response as a markdown block and update or emit a handled decision card block:

```go
content, err := uimessage.BuildDecisionCardMessageContent(uimessage.DecisionCardInput{
	DecisionID:    decisionID,
	Title:         existingTitle,
	Message:       existingMessage,
	Options:       existingOptions,
	AllowFreeText: existingAllowFreeText,
	Status:        "handled",
})
```

If the current schema does not support in-place message updates, emit a new `ui_card` message with `status="handled"` and rely on the frontend to prefer the latest block for the same `decision_id`.

- [ ] **Step 6: Run focused verification**

```bash
gofmt -w apps/server/internal/agent/tools apps/server/internal/agent/hitl apps/server/internal/api/agent_handler.go apps/server/internal/api/agent_handler_test.go
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./internal/agent/hitl ./internal/api -count=1
git diff --check
```

Expected: PASS.

---

## Task 7: Frontend Block Parser And Streaming State

**Files:**

- Create: `apps/web/src/lib/agentMessageBlocks.ts`
- Create: `apps/web/src/lib/agentMessageBlocks.test.mjs`
- Modify: `apps/web/src/lib/agentStreaming.ts`
- Modify: `apps/web/src/lib/agentStreaming.test.mjs`
- Modify: `apps/web/src/lib/agentWs.ts`
- Modify: `apps/web/src/lib/agentApi.ts`
- Modify: `apps/web/package.json`
- Modify: `apps/web/tsconfig.test.json`

- [ ] **Step 1: Write block parser tests**

Create `apps/web/src/lib/agentMessageBlocks.test.mjs`:

```js
import assert from "node:assert/strict";
import { test } from "node:test";
import {
  agentMessageBlocks,
  agentMessageMarkdownText,
  agentMessageAttachments,
  isDecisionCardBlock,
  isUnsupportedAgentMessage,
} from "./agentMessageBlocks.js";

test("agentMessageBlocks returns v1 blocks", () => {
  const message = {
    content: {
      schema: "clipanvil.agent.message.v1",
      blocks: [{ id: "blk_answer", type: "markdown", text: "hello" }],
    },
  };
  assert.deepEqual(agentMessageBlocks(message), [
    { id: "blk_answer", type: "markdown", text: "hello" },
  ]);
});

test("agentMessageMarkdownText joins markdown blocks only", () => {
  const message = {
    content: {
      schema: "clipanvil.agent.message.v1",
      blocks: [
        { id: "blk_thinking", type: "thinking", text: "hidden" },
        { id: "blk_answer", type: "markdown", text: "visible" },
      ],
    },
  };
  assert.equal(agentMessageMarkdownText(message), "visible");
});

test("agentMessageAttachments reads attachment block", () => {
  const message = {
    content: {
      schema: "clipanvil.agent.message.v1",
      blocks: [
        {
          id: "blk_attachment",
          type: "attachment",
          attachments: [
            {
              asset_id: "asset-1",
              node_id: "node-1",
              kind: "image",
              name: "hero.png",
              mime: "image/png",
              size_bytes: 123,
            },
          ],
        },
      ],
    },
  };
  assert.equal(agentMessageAttachments(message).length, 1);
  assert.equal(agentMessageAttachments(message)[0].name, "hero.png");
});

test("decision card block guard", () => {
  assert.equal(
    isDecisionCardBlock({
      id: "blk_decision",
      type: "decision_card",
      decision_id: "decision-1",
      title: "确认",
      message: "请选择",
      options: [{ id: "a", label: "方案 A" }],
      allow_free_text: true,
      status: "pending",
    }),
    true,
  );
});

test("missing schema is unsupported", () => {
  assert.equal(isUnsupportedAgentMessage({ content: { text: "old" } }), true);
});
```

- [ ] **Step 2: Run failing parser tests**

```bash
pnpm --filter @clip-anvil/web test:connections -- --test-name-pattern agentMessageBlocks
```

Expected: FAIL because file is not included or module does not exist.

- [ ] **Step 3: Implement parser**

Create `apps/web/src/lib/agentMessageBlocks.ts`:

```ts
import type { AgentAttachment, AgentMessage } from "./agentApi";

export const agentMessageSchemaV1 = "clipanvil.agent.message.v1";

export type AgentMessageBlock =
  | AgentMarkdownBlock
  | AgentThinkingBlock
  | AgentDecisionCardBlock
  | AgentToolStatusBlock
  | AgentAttachmentBlock
  | AgentMediaBlock
  | AgentErrorBlock
  | AgentUnknownBlock;

export interface AgentBaseBlock {
  id: string;
  type: string;
  visibility?: "user" | "debug" | "hidden";
}

export interface AgentMarkdownBlock extends AgentBaseBlock {
  type: "markdown";
  text: string;
}

export interface AgentThinkingBlock extends AgentBaseBlock {
  type: "thinking";
  text: string;
  status: "streaming" | "done";
  default_collapsed: boolean;
}

export interface AgentDecisionCardBlock extends AgentBaseBlock {
  type: "decision_card";
  decision_id: string;
  title: string;
  message: string;
  options: Array<{ id: string; label: string; description?: string }>;
  allow_free_text: boolean;
  status: "pending" | "handled" | "failed" | "cancelled";
  selected_option_id?: string;
  free_text?: string;
}

export interface AgentToolStatusBlock extends AgentBaseBlock {
  type: "tool_status";
  tool_call_id: string;
  tool_name: string;
  label: string;
  status: "running" | "succeeded" | "failed";
  summary?: string;
  error_message?: string;
}

export interface AgentAttachmentBlock extends AgentBaseBlock {
  type: "attachment";
  attachments: AgentAttachment[];
}

export interface AgentMediaBlock extends AgentBaseBlock {
  type: "media";
  asset_id: string;
  node_id?: string;
  kind: "image" | "video" | "text" | "final_video";
  title?: string;
  url?: string;
  thumbnail_url?: string;
  mime?: string;
}

export interface AgentErrorBlock extends AgentBaseBlock {
  type: "error";
  title: string;
  message: string;
  code?: string;
  retryable?: boolean;
}

export interface AgentUnknownBlock extends AgentBaseBlock {
  type: string;
  [key: string]: unknown;
}

export function agentMessageBlocks(
  message: Pick<AgentMessage, "content"> | { content: unknown },
): AgentMessageBlock[] {
  const content = message.content;
  if (!content || typeof content !== "object") {
    return [];
  }
  const envelope = content as { schema?: unknown; blocks?: unknown };
  if (envelope.schema !== agentMessageSchemaV1 || !Array.isArray(envelope.blocks)) {
    return [];
  }
  return envelope.blocks.filter(isAgentBlock);
}

export function isUnsupportedAgentMessage(message: { content: unknown }) {
  const content = message.content;
  return !content || typeof content !== "object" || (content as { schema?: unknown }).schema !== agentMessageSchemaV1;
}

export function agentMessageMarkdownText(message: { content: unknown }) {
  return agentMessageBlocks(message)
    .filter((block): block is AgentMarkdownBlock => block.type === "markdown" && typeof block.text === "string")
    .map((block) => block.text.trim())
    .filter(Boolean)
    .join("\n\n");
}

export function agentMessageAttachments(message: { content: unknown }): AgentAttachment[] {
  return agentMessageBlocks(message)
    .filter((block): block is AgentAttachmentBlock => block.type === "attachment" && Array.isArray((block as AgentAttachmentBlock).attachments))
    .flatMap((block) => block.attachments.filter(isAgentAttachment));
}

export function isDecisionCardBlock(block: unknown): block is AgentDecisionCardBlock {
  if (!block || typeof block !== "object") {
    return false;
  }
  const value = block as Partial<AgentDecisionCardBlock>;
  return (
    value.type === "decision_card" &&
    typeof value.id === "string" &&
    typeof value.decision_id === "string" &&
    typeof value.title === "string" &&
    typeof value.message === "string" &&
    Array.isArray(value.options) &&
    typeof value.allow_free_text === "boolean" &&
    typeof value.status === "string"
  );
}

function isAgentBlock(value: unknown): value is AgentMessageBlock {
  if (!value || typeof value !== "object") {
    return false;
  }
  const block = value as Partial<AgentBaseBlock>;
  return typeof block.id === "string" && typeof block.type === "string";
}

function isAgentAttachment(value: unknown): value is AgentAttachment {
  if (!value || typeof value !== "object") {
    return false;
  }
  const attachment = value as Partial<AgentAttachment>;
  return (
    typeof attachment.asset_id === "string" &&
    typeof attachment.node_id === "string" &&
    (attachment.kind === "image" || attachment.kind === "video" || attachment.kind === "text") &&
    typeof attachment.name === "string" &&
    typeof attachment.mime === "string" &&
    typeof attachment.size_bytes === "number"
  );
}
```

- [ ] **Step 4: Include tests**

Update `apps/web/package.json` `test:connections` script to include:

```json
"src/lib/agentMessageBlocks.test.mjs"
```

Update `apps/web/tsconfig.test.json` include list if it enumerates test source files.

- [ ] **Step 5: Update streaming state**

Modify `apps/web/src/lib/agentStreaming.ts` to use block deltas:

```ts
export interface AgentStreamDelta {
  task_id: string;
  block_id: string;
  block_type: "markdown" | "thinking" | string;
  delta: string;
  sequence?: number;
}

export interface AgentStreamBlock {
  id: string;
  type: "markdown" | "thinking" | string;
  text: string;
  sequence: number;
}

export interface AgentStreamState {
  task_id: string;
  blocks: AgentStreamBlock[];
}
```

Update `mergeAgentStreamDelta` so it appends to the matching `task_id + block_id` block and sorts blocks by first appearance.

- [ ] **Step 6: Update streaming tests**

In `apps/web/src/lib/agentStreaming.test.mjs`, assert:

```js
test("mergeAgentStreamDelta appends by block id", () => {
  const state = mergeAgentStreamDelta([], {
    task_id: "task-1",
    block_id: "blk_answer",
    block_type: "markdown",
    delta: "hello",
    sequence: 1,
  });
  const next = mergeAgentStreamDelta(state, {
    task_id: "task-1",
    block_id: "blk_answer",
    block_type: "markdown",
    delta: " world",
    sequence: 2,
  });
  assert.equal(next[0].blocks[0].text, "hello world");
});

test("mergeAgentStreamDelta keeps thinking and markdown separate", () => {
  const state = [
    {
      task_id: "task-1",
      blocks: [{ id: "blk_thinking", type: "thinking", text: "think", sequence: 1 }],
    },
  ];
  const next = mergeAgentStreamDelta(state, {
    task_id: "task-1",
    block_id: "blk_answer",
    block_type: "markdown",
    delta: "answer",
    sequence: 2,
  });
  assert.equal(next[0].blocks.length, 2);
  assert.equal(next[0].blocks[1].type, "markdown");
});
```

- [ ] **Step 7: Run frontend unit tests**

```bash
pnpm --filter @clip-anvil/web test:connections
git diff --check
```

Expected: PASS.

---

## Task 8: Frontend Block Renderers

**Files:**

- Create: `apps/web/src/components/agent/AgentMessageRenderer.tsx`
- Create: `apps/web/src/components/agent/AgentMarkdownBlock.tsx`
- Create: `apps/web/src/components/agent/AgentThinkingBlock.tsx`
- Create: `apps/web/src/components/agent/AgentDecisionCardBlock.tsx`
- Create: `apps/web/src/components/agent/AgentToolStatusBlock.tsx`
- Create: `apps/web/src/components/agent/AgentAttachmentBlock.tsx`
- Create: `apps/web/src/components/agent/AgentMediaBlock.tsx`
- Create: `apps/web/src/components/agent/AgentErrorBlock.tsx`
- Modify: `apps/web/src/lib/agentDecision.ts`
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Create Markdown renderer**

Create `apps/web/src/components/agent/AgentMarkdownBlock.tsx`:

```tsx
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { AgentMarkdownBlock } from "../../lib/agentMessageBlocks";

export function AgentMarkdownBlockView({ block }: { block: AgentMarkdownBlock }) {
  return (
    <div className="agent-markdown-block">
      <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>
        {block.text}
      </ReactMarkdown>
    </div>
  );
}
```

- [ ] **Step 2: Create Thinking renderer**

Create `apps/web/src/components/agent/AgentThinkingBlock.tsx`:

```tsx
import { useState } from "react";
import type { AgentThinkingBlock } from "../../lib/agentMessageBlocks";

export function AgentThinkingBlockView({ block }: { block: AgentThinkingBlock }) {
  const [expanded, setExpanded] = useState(!block.default_collapsed || block.status === "streaming");
  const label = block.status === "streaming" ? "ClipAnvil 正在思考" : "ClipAnvil 的思考";
  return (
    <section className={`agent-thinking-block${block.status === "streaming" ? " agent-thinking-block-streaming" : ""}`}>
      <button className="agent-thinking-toggle" onClick={() => setExpanded((current) => !current)} type="button">
        <span className="agent-thinking-shimmer">{label}</span>
        <span aria-hidden="true">{expanded ? "⌃" : "⌄"}</span>
      </button>
      {expanded ? <p className="agent-thinking-content">{block.text}</p> : null}
    </section>
  );
}
```

- [ ] **Step 3: Create decision card renderer**

Create `apps/web/src/components/agent/AgentDecisionCardBlock.tsx`:

```tsx
import { useState } from "react";
import type { AgentDecisionCardBlock } from "../../lib/agentMessageBlocks";

export interface AgentDecisionCardActions {
  respondDecision: (input: { decisionId: string; selectedOptionId?: string; freeText?: string }) => void;
  decisionPending: boolean;
  resolvedDecisionIds: Set<string>;
}

export function AgentDecisionCardBlockView({
  block,
  actions,
}: {
  block: AgentDecisionCardBlock;
  actions: AgentDecisionCardActions;
}) {
  const [freeText, setFreeText] = useState("");
  const resolved = block.status === "handled" || actions.resolvedDecisionIds.has(block.decision_id);
  const disabled = resolved || actions.decisionPending;
  return (
    <div className="agent-decision-card">
      <div className="agent-decision-card-header">
        <strong>{block.title}</strong>
        <span>{resolved ? "已完成" : "待选择"}</span>
      </div>
      {block.message ? <p>{block.message}</p> : null}
      {block.options.length > 0 ? (
        <div className="agent-decision-options">
          {block.options.map((option) => (
            <button
              disabled={disabled}
              key={option.id}
              onClick={() => actions.respondDecision({ decisionId: block.decision_id, selectedOptionId: option.id })}
              type="button"
            >
              {option.label}
            </button>
          ))}
        </div>
      ) : null}
      {block.allow_free_text && !resolved ? (
        <div className="agent-decision-free-text">
          <input
            aria-label="补充选择"
            disabled={disabled}
            onChange={(event) => setFreeText(event.target.value)}
            placeholder="补充说明"
            value={freeText}
          />
          <button
            disabled={disabled || freeText.trim() === ""}
            onClick={() => actions.respondDecision({ decisionId: block.decision_id, freeText: freeText.trim() })}
            type="button"
          >
            提交
          </button>
        </div>
      ) : null}
    </div>
  );
}
```

- [ ] **Step 4: Create tool, attachment, media, and error renderers**

Create `AgentToolStatusBlock.tsx`:

```tsx
import type { AgentToolStatusBlock } from "../../lib/agentMessageBlocks";

export function AgentToolStatusBlockView({ block }: { block: AgentToolStatusBlock }) {
  return (
    <div className={`agent-tool-status agent-tool-status-${block.status}`}>
      <span className="agent-tool-status-dot" />
      <span>{block.label || block.tool_name}</span>
      {block.summary ? <small>{block.summary}</small> : null}
      {block.error_message ? <small>{block.error_message}</small> : null}
    </div>
  );
}
```

Create `AgentAttachmentBlock.tsx`:

```tsx
import { formatAgentAttachmentLabel } from "../../lib/agentAttachments";
import type { AgentAttachmentBlock } from "../../lib/agentMessageBlocks";

export function AgentAttachmentBlockView({ block }: { block: AgentAttachmentBlock }) {
  if (block.attachments.length === 0) {
    return null;
  }
  return (
    <div className="agent-attachment-row">
      {block.attachments.map((attachment) => (
        <span className="agent-attachment-chip" key={`${attachment.asset_id}:${attachment.node_id}`}>
          {formatAgentAttachmentLabel(attachment)}
        </span>
      ))}
    </div>
  );
}
```

Create `AgentMediaBlock.tsx`:

```tsx
import type { AgentMediaBlock } from "../../lib/agentMessageBlocks";

export function AgentMediaBlockView({ block }: { block: AgentMediaBlock }) {
  return (
    <figure className={`agent-media-block agent-media-block-${block.kind}`}>
      {block.kind === "image" && block.url ? <img alt={block.title || "生成图片"} src={block.url} /> : null}
      {block.kind === "video" && block.url ? <video controls src={block.url} /> : null}
      <figcaption>{block.title || block.kind}</figcaption>
    </figure>
  );
}
```

Create `AgentErrorBlock.tsx`:

```tsx
import type { AgentErrorBlock } from "../../lib/agentMessageBlocks";

export function AgentErrorBlockView({ block }: { block: AgentErrorBlock }) {
  return (
    <div className="agent-error-block">
      <strong>{block.title}</strong>
      <p>{block.message}</p>
      {block.code ? <small>{block.code}</small> : null}
    </div>
  );
}
```

- [ ] **Step 5: Create registry**

Create `apps/web/src/components/agent/AgentMessageRenderer.tsx`:

```tsx
import type { AgentMessage } from "../../lib/agentApi";
import {
  agentMessageBlocks,
  isUnsupportedAgentMessage,
  type AgentMessageBlock,
} from "../../lib/agentMessageBlocks";
import { AgentAttachmentBlockView } from "./AgentAttachmentBlock";
import { AgentDecisionCardBlockView, type AgentDecisionCardActions } from "./AgentDecisionCardBlock";
import { AgentErrorBlockView } from "./AgentErrorBlock";
import { AgentMarkdownBlockView } from "./AgentMarkdownBlock";
import { AgentMediaBlockView } from "./AgentMediaBlock";
import { AgentThinkingBlockView } from "./AgentThinkingBlock";
import { AgentToolStatusBlockView } from "./AgentToolStatusBlock";

export interface AgentMessageActions extends AgentDecisionCardActions {}

export function AgentMessageRenderer({
  message,
  actions,
}: {
  message: AgentMessage;
  actions: AgentMessageActions;
}) {
  if (isUnsupportedAgentMessage(message)) {
    return <div className="agent-unsupported-message">Unsupported message format</div>;
  }
  return (
    <>
      {agentMessageBlocks(message).map((block) => (
        <AgentBlockRenderer block={block} actions={actions} key={block.id} />
      ))}
    </>
  );
}

function AgentBlockRenderer({ block, actions }: { block: AgentMessageBlock; actions: AgentMessageActions }) {
  if (block.visibility === "hidden" || block.visibility === "debug") {
    return null;
  }
  switch (block.type) {
    case "markdown":
      return <AgentMarkdownBlockView block={block} />;
    case "thinking":
      return <AgentThinkingBlockView block={block} />;
    case "decision_card":
      return <AgentDecisionCardBlockView block={block} actions={actions} />;
    case "tool_status":
      return <AgentToolStatusBlockView block={block} />;
    case "attachment":
      return <AgentAttachmentBlockView block={block} />;
    case "media":
      return <AgentMediaBlockView block={block} />;
    case "error":
      return <AgentErrorBlockView block={block} />;
    default:
      return <div className="agent-unsupported-block">Unsupported block: {block.type}</div>;
  }
}
```

- [ ] **Step 6: Wire renderer into AgentWorkspacePage**

In `apps/web/src/pages/AgentWorkspacePage.tsx`:

- Import `AgentMessageRenderer`.
- Replace inline `decisionCard ? <DecisionCard ... /> : <AgentTextMessage ... />` with:

```tsx
<AgentMessageRenderer
  actions={{
    decisionPending: respondDecisionMutation.isPending,
    resolvedDecisionIds,
    respondDecision: ({ decisionId, selectedOptionId, freeText }) =>
      respondDecisionMutation.mutate({
        eventId: decisionId,
        selectedOptionId,
        freeText,
      }),
  }}
  message={message}
/>
```

- Replace streaming render with block render over synthetic messages:

```tsx
{streams.map((stream) => (
  <article className="agent-message agent-message-assistant agent-message-streaming" key={stream.task_id}>
    <AgentMessageRenderer
      actions={messageActions}
      message={streamToAgentMessage(stream)}
    />
  </article>
))}
```

Add local helper:

```ts
function streamToAgentMessage(stream: AgentStreamState): AgentMessage {
  return {
    id: stream.task_id,
    workspace_id: "",
    thread_id: "",
    seq: 0,
    role: "assistant",
    message_type: "text",
    content: {
      schema: "clipanvil.agent.message.v1",
      blocks: stream.blocks.map((block) => ({
        id: block.id,
        type: block.type,
        text: block.text,
        status: block.type === "thinking" ? "streaming" : undefined,
        default_collapsed: false,
      })),
    },
    raw_message: {},
    task_id: stream.task_id,
    event_id: null,
    created_at: new Date(0).toISOString(),
  };
}
```

- Remove old `AgentTextMessage`, `ThinkingBlock`, and `DecisionCard` from this file after replacement.

- [ ] **Step 7: Add styles**

In `apps/web/src/main.css`, add:

```css
.agent-markdown-block {
  display: grid;
  gap: 8px;
  line-height: 1.58;
}

.agent-markdown-block :where(p, ul, ol, pre, blockquote, table) {
  margin: 0;
}

.agent-markdown-block :where(h1, h2, h3) {
  margin: 2px 0 4px;
  font-size: 14px;
  line-height: 1.35;
}

.agent-markdown-block :where(ul, ol) {
  padding-left: 18px;
}

.agent-markdown-block :where(pre) {
  overflow-x: auto;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--fg-primary) 6%, transparent);
  padding: 10px;
}

.agent-markdown-block :where(code) {
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 12px;
}

.agent-markdown-block :where(table) {
  display: block;
  overflow-x: auto;
  border-collapse: collapse;
}

.agent-tool-status,
.agent-error-block,
.agent-unsupported-message,
.agent-unsupported-block {
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--fg-primary) 5%, transparent);
  padding: 10px 12px;
  color: var(--fg-secondary);
  font-size: 12px;
}

.agent-tool-status {
  display: flex;
  align-items: center;
  gap: 8px;
}

.agent-tool-status-dot {
  width: 7px;
  height: 7px;
  border-radius: var(--radius-pill);
  background: var(--status-running);
}

.agent-tool-status-succeeded .agent-tool-status-dot {
  background: var(--status-succeeded);
}

.agent-tool-status-failed .agent-tool-status-dot {
  background: var(--status-failed);
}

.agent-media-block {
  display: grid;
  gap: 8px;
  margin: 0;
}

.agent-media-block img,
.agent-media-block video {
  max-width: 100%;
  border-radius: var(--radius-md);
}
```

- [ ] **Step 8: Run frontend verification**

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected: PASS.

---

## Task 9: Composer Redesign With Model And Thinking Controls

**Files:**

- Create: `apps/web/src/components/agent/AgentComposer.tsx`
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/main.css`
- Modify: `apps/web/src/lib/agentThinking.test.mjs`
- Modify: `apps/web/src/lib/agentModelSelection.test.mjs`

- [ ] **Step 1: Create composer component**

Create `apps/web/src/components/agent/AgentComposer.tsx`:

```tsx
import type { ChangeEvent, KeyboardEvent, RefObject } from "react";
import type { AgentAttachment, AgentModelOption } from "../../lib/agentApi";
import { formatAgentAttachmentLabel } from "../../lib/agentAttachments";
import {
  agentModelSelectionValue,
  formatAgentModelOption,
} from "../../lib/agentModelSelection";
import {
  agentModelSupportsThinking,
  agentThinkingEffortLabel,
  agentThinkingEffortOptions,
} from "../../lib/agentThinking";

export interface AgentComposerProps {
  draft: string;
  setDraft: (value: string) => void;
  attachments: AgentAttachment[];
  removeAttachment: (nodeID: string) => void;
  chooseAttachment: () => void;
  fileInputRef: RefObject<HTMLInputElement | null>;
  attachmentAccept: string;
  uploadSelectedAttachment: (file: File | undefined) => void;
  agentBusy: boolean;
  uploadPending: boolean;
  canSend: boolean;
  submitMessage: () => void;
  modelOptions: AgentModelOption[];
  selectedModelValue: string;
  selectedReasoningEffort: string;
  modelSelectionPending: boolean;
  onSelectModel: (value: string) => void;
  onSelectReasoningEffort: (value: string) => void;
}

export function AgentComposer(props: AgentComposerProps) {
  const selectedOption = props.modelOptions.find(
    (option) => agentModelSelectionValue(option) === props.selectedModelValue,
  );
  const thinkingOptions = agentThinkingEffortOptions(selectedOption);
  const showThinking = agentModelSupportsThinking(selectedOption) && thinkingOptions.length > 0;

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      props.submitMessage();
    }
  }

  function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
    props.uploadSelectedAttachment(event.target.files?.[0]);
    event.currentTarget.value = "";
  }

  return (
    <form
      className="agent-chat-composer agent-chat-composer-modern"
      onSubmit={(event) => {
        event.preventDefault();
        props.submitMessage();
      }}
    >
      <input
        accept={props.attachmentAccept}
        className="agent-file-input"
        onChange={handleFileChange}
        ref={props.fileInputRef}
        type="file"
      />
      {props.attachments.length > 0 ? (
        <div className="agent-composer-attachments">
          {props.attachments.map((attachment) => (
            <span className="agent-attachment-chip" key={attachment.node_id}>
              {formatAgentAttachmentLabel(attachment)}
              <button
                aria-label={`移除 ${attachment.name}`}
                onClick={() => props.removeAttachment(attachment.node_id)}
                type="button"
              >
                ×
              </button>
            </span>
          ))}
        </div>
      ) : null}
      <div className="agent-composer-surface">
        <textarea
          aria-label="发送给 ClipAnvil"
          disabled={props.agentBusy}
          onChange={(event) => props.setDraft(event.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={props.agentBusy ? "等待当前任务完成" : "输入需求或反馈"}
          rows={2}
          value={props.draft}
        />
        <div className="agent-composer-toolbar">
          <div className="agent-composer-toolbar-left">
            <button
              aria-label="添加附件"
              className="agent-composer-icon-button"
              disabled={props.agentBusy || props.uploadPending}
              onClick={props.chooseAttachment}
              type="button"
            >
              +
            </button>
            <select
              aria-label="对话模型"
              className="agent-composer-select agent-model-select"
              disabled={props.agentBusy || props.modelSelectionPending}
              onChange={(event) => props.onSelectModel(event.target.value)}
              value={props.selectedModelValue}
            >
              {props.modelOptions.map((option) => (
                <option key={`${option.provider_id}:${option.model_id}`} value={agentModelSelectionValue(option)}>
                  {formatAgentModelOption(option)}
                </option>
              ))}
            </select>
            {showThinking ? (
              <select
                aria-label="思考深度"
                className="agent-composer-select agent-thinking-select"
                disabled={props.agentBusy || props.modelSelectionPending}
                onChange={(event) => props.onSelectReasoningEffort(event.target.value)}
                value={props.selectedReasoningEffort}
              >
                {thinkingOptions.map((effort) => (
                  <option key={effort} value={effort}>
                    思考 {agentThinkingEffortLabel(effort)}
                  </option>
                ))}
              </select>
            ) : null}
          </div>
          <button
            aria-label="发送"
            className="agent-composer-send-button"
            disabled={!props.canSend}
            type="submit"
          >
            ↑
          </button>
        </div>
      </div>
    </form>
  );
}
```

- [ ] **Step 2: Wire model selection handlers**

In `apps/web/src/pages/AgentWorkspacePage.tsx`, add:

```ts
function selectAgentModel(nextValue: string) {
  const nextOption = agentModelSelectionQuery.data?.options.find(
    (option) => agentModelSelectionValue(option) === nextValue,
  );
  const nextEfforts = agentThinkingEffortOptions(nextOption);
  modelSelectionMutation.mutate({
    value: nextValue,
    reasoningEffort: nextOption?.default_reasoning_effort || nextEfforts[0],
  });
}

function selectAgentReasoningEffort(reasoningEffort: string) {
  modelSelectionMutation.mutate({
    value: selectedModelValue,
    reasoningEffort,
  });
}
```

Remove the model and thinking `<select>` elements from `.agent-chat-header`.

- [ ] **Step 3: Replace composer JSX**

Replace the old `<form className="agent-chat-composer">...</form>` with:

```tsx
<AgentComposer
  agentBusy={agentBusy}
  attachmentAccept={attachmentAccept}
  attachments={attachments}
  canSend={canSend}
  chooseAttachment={chooseAttachment}
  draft={draft}
  fileInputRef={fileInputRef}
  modelOptions={agentModelSelectionQuery.data?.options ?? []}
  modelSelectionPending={modelSelectionMutation.isPending}
  onSelectModel={selectAgentModel}
  onSelectReasoningEffort={selectAgentReasoningEffort}
  removeAttachment={(nodeID) =>
    setAttachments((current) =>
      current.filter((item) => item.node_id !== nodeID),
    )
  }
  selectedModelValue={selectedModelValue}
  selectedReasoningEffort={selectedReasoningEffort}
  setDraft={setDraft}
  submitMessage={submitMessage}
  uploadPending={uploadAttachmentMutation.isPending}
  uploadSelectedAttachment={uploadSelectedAttachment}
/>
```

- [ ] **Step 4: Add composer styles**

In `apps/web/src/main.css`, replace old single-line composer assumptions with:

```css
.agent-chat-composer-modern {
  display: grid;
  gap: 8px;
}

.agent-composer-surface {
  display: grid;
  gap: 10px;
  border: 1px solid var(--border-default);
  border-radius: 22px;
  background: color-mix(in srgb, var(--fg-primary) 4%, transparent);
  padding: 10px;
  transition:
    border-color 180ms var(--ease-out),
    background 180ms var(--ease-out);
}

.agent-composer-surface:focus-within {
  border-color: color-mix(in srgb, var(--accent) 48%, var(--border-default));
  background: color-mix(in srgb, var(--fg-primary) 6%, transparent);
}

.agent-composer-surface textarea {
  width: 100%;
  min-height: 48px;
  max-height: 180px;
  resize: none;
  overflow-y: auto;
  border: 0;
  background: transparent;
  color: var(--fg-primary);
  line-height: 1.5;
  outline: none;
  padding: 4px 6px;
}

.agent-composer-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.agent-composer-toolbar-left {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.agent-composer-select {
  min-height: 32px;
  max-width: 190px;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-pill);
  background: color-mix(in srgb, var(--fg-primary) 5%, transparent);
  color: var(--fg-secondary);
  font-size: 12px;
  padding: 0 28px 0 12px;
}
```

Keep existing `.agent-composer-send-button` and icon button styles unless they conflict.

- [ ] **Step 5: Run frontend verification**

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected: PASS.

---

## Task 10: End-To-End Browser Verification And Full Test Run

**Files:**

- No source files unless E2E exposes a defect.

- [ ] **Step 1: Run full backend verification**

```bash
make sqlc-generate
make server-test
make server-build
make server-lint
```

Expected: PASS.

- [ ] **Step 2: Run full frontend verification**

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected: PASS.

- [ ] **Step 3: Start dev environment**

```bash
./scripts/dev-start.sh
```

Expected:

- Script prints backend and Vite URLs.
- `/api/health` passes.
- Use the printed Vite URL for browser E2E.

- [ ] **Step 4: Browser E2E: composer controls**

Using the script-provided Vite URL:

1. Open an Agent workspace.
2. Assert header contains `ClipAnvil`.
3. Assert header does not contain `对话模型` or `思考深度`.
4. Assert composer toolbar contains `对话模型`.
5. Select `Doubao Seed 2.0 Mini`.
6. Assert `思考深度` control is not visible.
7. Select `Doubao Seed 2.0 Pro`.
8. Assert `思考深度` control is visible.

Expected: PASS.

- [ ] **Step 5: Browser E2E: multiline and Markdown**

1. Type a multi-line prompt:

```text
请用 Markdown 回复：

## 标题
- 第一条
- 第二条

并包含一个代码块。
```

2. Press `Shift+Enter` while editing and confirm it inserts a newline.
3. Press `Enter` without Shift and confirm it sends.
4. Wait for assistant completion.
5. Assert rendered assistant contains an actual heading/list/code block, not escaped plain text.

Expected: PASS.

- [ ] **Step 6: Browser E2E: tool and HITL blocks**

1. Send a prompt that triggers `read_workspace_context` tool call.
2. Assert a `tool_status` block appears for running or completed tool state.
3. Send a prompt that triggers `request_user_decision`.
4. Assert a decision card renders.
5. Click an option.
6. Refresh the page.
7. Assert the decision card state remains completed.

Expected: PASS.

- [ ] **Step 7: Browser E2E: attachments and mini to pro context**

1. Upload a small text or image attachment.
2. Send a message with the attachment.
3. Assert user message renders an attachment block.
4. Select Mini and send `请记住代号 ALPHA-MINI-BLOCK。`
5. Select Pro and send `请引用上一轮代号。`
6. Assert Pro response references `ALPHA-MINI-BLOCK`.

Expected: PASS.

- [ ] **Step 8: Inspect database and logs**

Run:

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U clipanvil -d clipanvil -P pager=off -c "
SELECT seq,
       role,
       message_type,
       content->>'schema' AS schema,
       jsonb_path_query_array(content, '$.blocks[*].type') AS block_types
FROM agent_message
WHERE created_at > now() - interval '30 minutes'
ORDER BY created_at DESC
LIMIT 20;
"
```

Expected:

- New messages show `schema = clipanvil.agent.message.v1`.
- User messages include `markdown` and optional `attachment`.
- Assistant messages include `markdown` and optional `thinking`.
- Tool messages include `tool_status`.
- HITL messages include `decision_card`.

Check server logs:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

Use the printed `CLIPANVIL_SERVER_LOG` value:

```bash
tail -n 200 "$CLIPANVIL_SERVER_LOG" | rg 'producer model response completed|reasoning_passback|model_id|finish_reason'
```

Expected:

- No empty-response regression.
- Model id and reasoning diagnostics are present.
- No unsupported thinking model produces visible thinking blocks.

- [ ] **Step 9: Stop dev environment when E2E is done**

```bash
./scripts/dev-stop.sh
```

Expected: frontend/backend for this worktree stop cleanly; shared middleware remains running.

---

## Final Verification Matrix

Run before reporting completion:

```bash
make sqlc-generate
make server-test
make server-build
make server-lint
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected: all commands pass.

Browser E2E must verify:

- Header no longer contains model/thinking controls.
- Composer contains model selection.
- Mini hides thinking control.
- Pro shows thinking control.
- Multi-line prompt sends correctly.
- Markdown renders as Markdown.
- Tool status block renders.
- Decision card block renders and survives refresh.
- Attachment block renders.
- Mini-to-Pro history context works through markdown blocks.

## Self-Review Checklist

- Spec coverage: Tasks 1-6 cover backend schema, builders, REST, WS, prompt context, visible-thinking policy, tool status, and decision cards. Tasks 7-9 cover frontend parser, renderer registry, Markdown, thinking, media, error, composer controls, and conditional thinking display. Task 10 covers full verification and E2E.
- Placeholder scan: this plan intentionally contains no placeholder markers and no vague "handle edge cases" steps.
- Type consistency: backend protocol fields use `schema`, `blocks`, `id`, `type`, `text`, `default_collapsed`; frontend types use the same names. Streaming uses `block_id`, `block_type`, `delta`, `sequence` on both sides.
- Scope check: this plan does not implement new image/video generation, full Craftsman/Worker/Composer protocols, Studio/Agent import-export, or old message migration.
