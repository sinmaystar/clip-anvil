# M6.2 Agent API / WebSocket / Right Floating Chat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Agent conversation shell: persisted user messages, `/ws/agent` realtime sync, and a right-floating Producer chat panel over the read-only Agent canvas.

**Architecture:** Reuse the M6.1 `internal/agent/runtime.Service` as the only persistence boundary. Add thin HTTP and WebSocket handlers in `internal/api`, an `AgentHub` parallel to `CanvasHub`, and frontend `agentApi`/`agentWs` helpers consumed by `AgentWorkspacePage`.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, M6.1 Agent runtime service, hertz-contrib websocket, React 19, TanStack Query, Vite 8, TypeScript 6, CSS in `apps/web/src/main.css`.

---

## Source Specs

- `docs/superpowers/specs/2026-06-21-m6-2-agent-api-ws-chat-design.md`
- `docs/superpowers/specs/2026-06-21-m6-1-agent-runtime-persistence-design.md`
- `docs/superpowers/specs/2026-06-21-m6-multiagent-agent-mode-design.md`

## Scope Lock

M6.2 must not call Eino, create `producer_turn` tasks, create assistant replies, call production service, add HITL UI, or add storyboard/PSS schema. It only persists user messages and broadcasts Agent events.

## File Structure

- Create `apps/server/internal/api/agent_response.go`
  - Converts `db.AgentThread`, `db.AgentMessage`, and `db.AgentEvent` to stable JSON.
- Create `apps/server/internal/api/agent_hub.go`
  - Workspace-scoped WebSocket broadcast hub for Agent events.
- Create `apps/server/internal/api/agent_handler.go`
  - REST endpoints for thread/messages.
- Create `apps/server/internal/api/agent_ws_handler.go`
  - `/ws/agent` auth and connection lifecycle.
- Modify `apps/server/cmd/server/main.go`
  - Construct runtime service, AgentHub, handlers, and routes.
- Create `apps/server/internal/api/agent_handler_test.go`
  - Handler validation and DTO tests.
- Create `apps/server/internal/api/agent_hub_test.go`
  - Hub register/unregister/broadcast smoke tests.
- Create `apps/web/src/lib/agentApi.ts`
  - Agent DTO types and REST helpers.
- Create `apps/web/src/lib/agentWs.ts`
  - Agent WebSocket connector with reconnect behavior.
- Create `apps/web/src/lib/agentMessages.ts`
  - Dedupe/sort/merge helpers for message state.
- Create `apps/web/src/lib/agentMessages.test.mjs`
  - Node tests for message merging.
- Modify `apps/web/src/pages/AgentWorkspacePage.tsx`
  - Render right-floating chat and wire API/WS.
- Modify `apps/web/src/main.css`
  - New responsive floating panel styles.
- Modify `apps/web/package.json`
  - Add `src/lib/agentMessages.test.mjs` to `test:connections`.

## Task 1: Backend DTOs And Request Validation

**Files:**
- Create: `apps/server/internal/api/agent_response.go`
- Create: `apps/server/internal/api/agent_handler_test.go`

- [ ] **Step 1: Write failing DTO tests**

Add tests:

```go
func TestAgentMessageResponseMapsTextContent(t *testing.T) {
	msg := db.AgentMessage{
		ID:          testUUID(0x01),
		WorkspaceID: testUUID(0x02),
		ThreadID:    testUUID(0x03),
		Seq:         7,
		Role:        "user",
		MessageType: "text",
		Content:     []byte(`{"text":"hello"}`),
		RawMessage:  []byte(`{}`),
	}

	got := toAgentMessageResponse(msg)

	if got.ID != uuidToString(testUUID(0x01)) {
		t.Fatalf("id = %q", got.ID)
	}
	if got.Seq != 7 || got.Role != "user" || got.MessageType != "text" {
		t.Fatalf("message response = %#v", got)
	}
	if got.Content["text"] != "hello" {
		t.Fatalf("content = %#v", got.Content)
	}
}

func TestPostAgentMessageRequestRejectsBlankText(t *testing.T) {
	req := postAgentMessageRequest{Text: "   "}

	if req.valid() {
		t.Fatal("blank text must be invalid")
	}
}
```

- [ ] **Step 2: Run backend API tests and verify red**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: FAIL because `toAgentMessageResponse` and `postAgentMessageRequest` do not exist.

- [ ] **Step 3: Implement DTO helpers and request validation**

Create response structs:

```go
type agentThreadResponse struct {
	ID                   string  `json:"id"`
	WorkspaceID          string  `json:"workspace_id"`
	Role                 string  `json:"role"`
	ScopeType            string  `json:"scope_type"`
	ScopeID              *string `json:"scope_id"`
	RuntimeProvider      string  `json:"runtime_provider"`
	RuntimeAgentName     string  `json:"runtime_agent_name"`
	CurrentCheckpointKey *string `json:"current_checkpoint_key"`
	Status               string  `json:"status"`
	Summary              string  `json:"summary"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

type agentMessageResponse struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspace_id"`
	ThreadID    string         `json:"thread_id"`
	Seq         int64          `json:"seq"`
	Role        string         `json:"role"`
	MessageType string         `json:"message_type"`
	Content     map[string]any `json:"content"`
	RawMessage  map[string]any `json:"raw_message"`
	TaskID      *string        `json:"task_id"`
	EventID     *string        `json:"event_id"`
	CreatedAt   string         `json:"created_at"`
}

type agentEventResponse struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspace_id"`
	ThreadID    *string        `json:"thread_id"`
	TaskID      *string        `json:"task_id"`
	EventType   string         `json:"event_type"`
	SourceRole  string         `json:"source_role"`
	TargetRole  *string        `json:"target_role"`
	Scope       map[string]any `json:"scope"`
	Payload     map[string]any `json:"payload"`
	Status      string         `json:"status"`
	CreatedAt   string         `json:"created_at"`
	HandledAt   *string        `json:"handled_at"`
}
```

Create request validation:

```go
type postAgentMessageRequest struct {
	Text            string `json:"text"`
	ClientMessageID string `json:"client_message_id"`
}

func (r postAgentMessageRequest) valid() bool {
	text := strings.TrimSpace(r.Text)
	return text != "" && len([]rune(text)) <= 8000 && len([]rune(r.ClientMessageID)) <= 128
}
```

- [ ] **Step 4: Verify green**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: PASS.

## Task 2: AgentHub

**Files:**
- Create: `apps/server/internal/api/agent_hub.go`
- Create: `apps/server/internal/api/agent_hub_test.go`

- [ ] **Step 1: Write failing hub tests**

Add tests mirroring `CanvasHub`:

```go
func TestAgentHubRegisterAndUnregister(t *testing.T) {
	workspaceID := testUUID(0x21)
	hub := NewAgentHub()

	hub.Register(workspaceID, nil)

	if got := len(hub.conns[workspaceID]); got != 1 {
		t.Fatalf("connection count = %d, want 1", got)
	}

	hub.Unregister(workspaceID, nil)

	if _, ok := hub.conns[workspaceID]; ok {
		t.Fatal("workspace connections should be removed after unregister")
	}
}

func TestAgentHubBroadcastWithoutConnections(t *testing.T) {
	hub := NewAgentHub()

	hub.Broadcast(testUUID(0x22), AgentSocketEvent{
		Type: "agent.event.created",
		Payload: map[string]any{"event_id": "event"},
	})
}
```

- [ ] **Step 2: Run API tests and verify red**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: FAIL because `NewAgentHub` and `AgentSocketEvent` do not exist.

- [ ] **Step 3: Implement AgentHub**

Implement the same locking and broadcast pattern as `CanvasHub`:

```go
type AgentSocketEvent struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type AgentHub struct {
	mu    sync.RWMutex
	conns map[pgtype.UUID]map[*websocket.Conn]struct{}
}
```

- [ ] **Step 4: Verify green**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: PASS.

## Task 3: Agent REST Handler

**Files:**
- Create: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/server/internal/api/agent_handler_test.go`

- [ ] **Step 1: Write failing handler unit tests**

Add focused tests for pure helpers:

```go
func TestAgentMessageContentIncludesClientMessageID(t *testing.T) {
	body := agentMessageContent("hello", "client-1")

	if string(body) != `{"client_message_id":"client-1","text":"hello"}` {
		t.Fatalf("content = %s", body)
	}
}

func TestAgentMessageContentOmitsBlankClientMessageID(t *testing.T) {
	body := agentMessageContent("hello", "")

	if string(body) != `{"text":"hello"}` {
		t.Fatalf("content = %s", body)
	}
}
```

- [ ] **Step 2: Run API tests and verify red**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: FAIL because `agentMessageContent` does not exist.

- [ ] **Step 3: Implement AgentHandler**

Create:

```go
type AgentHandler struct {
	queries *db.Queries
	runtime *agentruntime.Service
	hub     *AgentHub
}

func NewAgentHandler(queries *db.Queries, runtime *agentruntime.Service, hub *AgentHub) *AgentHandler
func (h *AgentHandler) GetThread(ctx context.Context, c *app.RequestContext)
func (h *AgentHandler) ListMessages(ctx context.Context, c *app.RequestContext)
func (h *AgentHandler) PostMessage(ctx context.Context, c *app.RequestContext)
```

Rules:

- Use `accountIDFromContext`.
- Parse `workspaceID` from route param.
- Use `workspaceForAccount`.
- Reject non-Agent workspace with `403`.
- `PostMessage` persists message first, then creates event, then broadcasts.
- Do not create task.
- Do not create assistant message.

- [ ] **Step 4: Verify green**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: PASS.

## Task 4: Agent WebSocket Handler And Server Wiring

**Files:**
- Create: `apps/server/internal/api/agent_ws_handler.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Write constructor compile test**

Add or extend API tests:

```go
func TestNewAgentWSHandlerConstructs(t *testing.T) {
	handler := NewAgentWSHandler(db.New(fakeDBTX{}), NewAgentHub(), "secret")

	if handler == nil {
		t.Fatal("handler should be constructed")
	}
}
```

Use the existing package test fake pattern if available; otherwise create a tiny `fakeDBTX` in the test file with `Exec`, `Query`, and `QueryRow` methods.

- [ ] **Step 2: Run API tests and verify red**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: FAIL because `NewAgentWSHandler` does not exist.

- [ ] **Step 3: Implement AgentWSHandler**

Mirror `CanvasWSHandler` with Agent mode guard:

```go
type AgentWSHandler struct {
	queries   *db.Queries
	hub       *AgentHub
	jwtSecret string
	upgrader  websocket.HertzUpgrader
}
```

The `Agent` method must reject invalid workspace id, invalid token, missing workspace, non-owner workspace, and non-Agent workspace before upgrading.

- [ ] **Step 4: Wire server routes**

In `apps/server/cmd/server/main.go`:

```go
agentHub := api.NewAgentHub()
agentRuntime, err := agentruntime.NewService(pgPool, queries)
if err != nil {
	slog.Error("failed to create agent runtime", "error", err)
	os.Exit(1)
}
agentHandler := api.NewAgentHandler(queries, agentRuntime, agentHub)
agentWSHandler := api.NewAgentWSHandler(queries, agentHub, cfg.JWT.Secret)
```

Add routes:

```go
h.GET("/api/agent/workspaces/:workspaceID/thread", authMiddleware, agentHandler.GetThread)
h.GET("/api/agent/workspaces/:workspaceID/messages", authMiddleware, agentHandler.ListMessages)
h.POST("/api/agent/workspaces/:workspaceID/messages", authMiddleware, agentHandler.PostMessage)
h.GET("/ws/agent", agentWSHandler.Agent)
```

- [ ] **Step 5: Verify server compiles**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: PASS.

## Task 5: Frontend Agent API And Message Merge Helpers

**Files:**
- Create: `apps/web/src/lib/agentApi.ts`
- Create: `apps/web/src/lib/agentMessages.ts`
- Create: `apps/web/src/lib/agentMessages.test.mjs`
- Modify: `apps/web/package.json`

- [ ] **Step 1: Write failing frontend message merge tests**

Create test:

```js
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { mergeAgentMessages } from "../../dist-test/lib/agentMessages.js";

describe("agent messages", () => {
  it("dedupes by id and sorts by seq", () => {
    const messages = mergeAgentMessages(
      [{ id: "b", seq: 2 }, { id: "a", seq: 1 }],
      [{ id: "b", seq: 2 }, { id: "c", seq: 3 }],
    );

    assert.deepEqual(messages.map((message) => message.id), ["a", "b", "c"]);
  });
});
```

Add it to `apps/web/package.json` `test:connections`.

- [ ] **Step 2: Run frontend tests and verify red**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL because `agentMessages.ts` does not exist.

- [ ] **Step 3: Implement `agentMessages.ts`**

```ts
export interface SequencedAgentMessage {
  id: string;
  seq: number;
}

export function mergeAgentMessages<T extends SequencedAgentMessage>(
  current: T[],
  incoming: T[],
) {
  const byID = new Map<string, T>();
  for (const message of current) {
    byID.set(message.id, message);
  }
  for (const message of incoming) {
    byID.set(message.id, message);
  }
  return Array.from(byID.values()).sort((a, b) => a.seq - b.seq);
}
```

- [ ] **Step 4: Implement `agentApi.ts`**

Define types and helpers:

```ts
export interface AgentMessage {
  id: string;
  workspace_id: string;
  thread_id: string;
  seq: number;
  role: "user" | "assistant" | "tool" | "system";
  message_type: "text" | "tool_call" | "tool_result" | "ui_card" | "error" | "status";
  content: Record<string, unknown>;
  raw_message: Record<string, unknown>;
  task_id?: string | null;
  event_id?: string | null;
  created_at: string;
}

export function fetchAgentThread(workspaceId: string)
export function fetchAgentMessages(workspaceId: string, afterSeq?: number, limit?: number)
export function postAgentMessage(workspaceId: string, input: { text: string; client_message_id?: string })
```

- [ ] **Step 5: Verify frontend tests**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

## Task 6: Frontend Agent WebSocket Helper

**Files:**
- Create: `apps/web/src/lib/agentWs.ts`

- [ ] **Step 1: Implement connector**

Create a connector parallel to `connectCanvasSocket`:

```ts
export type AgentConnectionStatus = "connecting" | "connected" | "reconnecting" | "offline";

export type AgentSocketEvent =
  | { type: "agent.message.created"; payload: { workspace_id: string; thread_id: string; message: AgentMessage; event: AgentEvent } }
  | { type: "agent.event.created"; payload: { workspace_id: string; thread_id?: string; event: AgentEvent } };

export function connectAgentSocket(input: {
  workspaceId: string;
  token: string;
  onEvent: (event: AgentSocketEvent) => void;
  onReconnect: () => void;
  onStatusChange: (status: AgentConnectionStatus) => void;
}) {
  // Use /ws/agent?workspaceId=...&token=...
}
```

- [ ] **Step 2: Verify TypeScript build catches contract issues**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

## Task 7: Right Floating Producer Chat UI

**Files:**
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/main.css`

- [ ] **Step 1: Refactor page state**

Use:

```ts
const [messages, setMessages] = useState<AgentMessage[]>([]);
const [draft, setDraft] = useState("");
const [collapsed, setCollapsed] = useState(false);
const [connectionStatus, setConnectionStatus] = useState<AgentConnectionStatus>("offline");
const lastSeq = messages.at(-1)?.seq ?? 0;
```

Queries:

```ts
useQuery({ queryKey: ["agent", id, "thread"], queryFn: () => fetchAgentThread(id ?? ""), enabled: Boolean(id) });
useQuery({ queryKey: ["agent", id, "messages"], queryFn: () => fetchAgentMessages(id ?? ""), enabled: Boolean(id) });
```

- [ ] **Step 2: Wire send mutation**

Use `postAgentMessage`; on success merge returned message into state and clear draft. On failure keep draft and show an error.

- [ ] **Step 3: Wire WebSocket**

Use `connectAgentSocket`; on `agent.message.created`, merge event payload message. On reconnect, fetch messages after `lastSeq`.

- [ ] **Step 4: Update layout**

Replace current left-column chat with:

```tsx
<section className="agent-canvas-stage" aria-label="只读画布">
  <div className="agent-readonly-canvas">...</div>
  <aside className={collapsed ? "agent-chat-float is-collapsed" : "agent-chat-float"}>...</aside>
</section>
```

CSS requirements:

- `.agent-canvas-stage { position: relative; min-height: 0; overflow: hidden; }`
- `.agent-chat-float { position: absolute; top: 16px; right: 16px; bottom: 16px; width: min(420px, calc(100vw - 32px)); z-index: 5; }`
- mobile uses bottom/full-width layout.
- textarea and send button have stable dimensions and no text overflow.

- [ ] **Step 5: Verify frontend build and lint**

Run:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Expected: PASS.

## Task 8: Full Verification

**Files:**
- All M6.2 files.

- [ ] **Step 1: Run strict commands**

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
make server-lint
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web test:connections
git diff --check
```

Expected: all commands succeed.

- [ ] **Step 2: Optional local smoke**

Start the dev runtime with the worktree-aware script:

```bash
./scripts/dev-start.sh
```

Use the printed Vite URL. Create or open an Agent Workspace, send a Producer message, refresh, and verify the message remains visible. Stop the app with:

```bash
./scripts/dev-stop.sh
```

Expected: no manual canvas edit controls appear in Agent Workspace, and the right-floating chat remains usable.

## Self-Review Checklist

- [ ] No Eino Graph execution was introduced.
- [ ] No assistant message or mock Producer reply is created in M6.2.
- [ ] No `producer_turn` task is created in M6.2.
- [ ] `message_created` event stays pending for M6.3 ProducerGraph consumption.
- [ ] Agent API rejects Studio Workspace.
- [ ] `/ws/agent` rejects invalid token, non-owner workspace, and Studio Workspace.
- [ ] Right-floating chat overlays canvas on desktop and degrades cleanly on mobile.
- [ ] Agent canvas ordinary write APIs still return `403`.
- [ ] Verification commands all pass.
