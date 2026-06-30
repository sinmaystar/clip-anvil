# Agent Subagent Thread Observer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Agent 对话区增加 B+C 方案的子 Agent 观察器：右侧列出 Craftsman / Reviewer 等持久化 Agent，点击后打开只读线程抽屉实时查看该 Agent 的消息和工具调用，同时保持 Producer 主对话只展示 Producer thread。

**Architecture:** 后端复用 `agent_thread`、`agent_task`、`agent_message` 作为事实源，新增线程列表和指定线程消息 API，不新增迁移。前端将 `/ws/agent` 事件按 `thread_id` 分流：Producer 进入主对话，子 Agent 进入独立 thread cache；dispatch 工具卡片只展示摘要和子 Agent 入口。

**Tech Stack:** Go 1.26、Hertz、pgx/sqlc、React 19、TypeScript 6、TanStack Query、Vite、现有 `/ws/agent` WebSocket。

---

## Scope Check

本计划覆盖 `docs/superpowers/specs/2026-06-26-agent-subagent-thread-observer-design.md` 的 M1、M2 和 M3 polish。实现时建议先完成 Task 1-6，形成可用的 M1；Task 7 完成 dispatch 工具卡片入口化；Task 8-9 做体验 polish 和 E2E。

不做数据库迁移，不新增用户向子 Agent 发送消息的能力，不改变 Producer/Craftsman/Reviewer 的执行职责。

## File Structure

后端：

- Modify: `apps/server/sqlc/queries/agent_thread.sql`
  - 增加按 workspace 列出线程详情的查询，包含 latest task 和 latest message 信息。
- Modify: `apps/server/sqlc/queries/agent_message.sql`
  - 增加按 thread + after_seq 拉消息的查询，沿用现有 `ListAgentMessagesByThread` 也可以，但计划中保留 route 所需参数。
- Modify: `apps/server/internal/store/db/*.go`
  - 由 `make sqlc-generate` 自动更新。
- Modify: `apps/server/internal/agent/runtime/service.go`
  - 暴露 `ListThreadsByWorkspace`、`GetThreadForWorkspace`、`ListThreadMessages`。
- Modify: `apps/server/internal/api/agent_response.go`
  - 增加 `agentThreadObserverResponse`、`agentThreadLatestTaskResponse`。
- Modify: `apps/server/internal/api/agent_handler.go`
  - 增加 `ListThreads` 和 `ListThreadMessages` handlers。
- Modify: `apps/server/cmd/server/main.go`
  - 注册 `/api/agent/workspaces/:workspaceID/threads` 和 `/api/agent/workspaces/:workspaceID/threads/:threadID/messages`。
- Test: `apps/server/internal/api/agent_handler_test.go`
  - route contract、response mapping、跨 workspace 保护的 handler-level 单测。
- Test: `apps/server/internal/agent/runtime/service_test.go`
  - runtime 参数校验和 message listing 行为。

前端：

- Modify: `apps/web/src/lib/agentApi.ts`
  - 增加 `AgentObservedThread` 类型、`fetchAgentThreads`、`fetchAgentThreadMessages`。
- Create: `apps/web/src/lib/agentThreads.ts`
  - 子线程列表 merge、消息 cache merge、preview 提取、按 thread 分流 helpers。
- Test: `apps/web/src/lib/agentThreads.test.mjs`
  - 覆盖 merge、unread、task/message 分流。
- Create: `apps/web/src/components/agent/AgentThreadObserverPanel.tsx`
  - 右侧 Agents 观察栏。
- Create: `apps/web/src/components/agent/AgentThreadDrawer.tsx`
  - 只读子线程抽屉。
- Create: `apps/web/src/components/agent/AgentThreadLinkChip.tsx`
  - dispatch 工具卡片中的线程入口 chip。
- Modify: `apps/web/src/components/agent/AgentToolStatusBlock.tsx`
  - 对 dispatch 工具渲染 thread chips。
- Modify: `apps/web/src/components/agent/AgentMessageRenderer.tsx`
  - 透传可选 thread selection action。
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
  - 接入 threads query、子线程 cache、抽屉状态、WebSocket 分流。
- Modify: `apps/web/src/lib/agentMessages.ts`
  - 移除主对话基于 `parent_tool_call_id` 的嵌套排序行为，或仅在同一 thread 内排序。
- Test: `apps/web/src/lib/agentMessages.test.mjs`
  - 更新“nested message”预期，确保主对话不嵌入子 Agent 消息。
- Modify: `apps/web/src/styles.css` or existing Agent CSS file used by `AgentWorkspacePage`
  - 增加观察栏和抽屉样式。

## Task 1: Backend Query And Runtime Support

**Files:**
- Modify: `apps/server/sqlc/queries/agent_thread.sql`
- Modify: `apps/server/sqlc/queries/agent_message.sql`
- Modify: `apps/server/internal/agent/runtime/service.go`
- Test: `apps/server/internal/agent/runtime/service_test.go`

- [ ] **Step 1: Add failing runtime tests**

Add tests that assert invalid workspace/thread inputs are rejected and that the runtime exposes workspace-scoped thread reads.

```go
func TestRuntimeListAgentThreadsByWorkspaceRejectsInvalidWorkspace(t *testing.T) {
	service := &Service{}
	_, err := service.ListAgentThreadsByWorkspace(context.Background(), pgtype.UUID{})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestRuntimeListThreadMessagesRejectsInvalidThread(t *testing.T) {
	service := &Service{}
	_, err := service.ListThreadMessages(context.Background(), pgtype.UUID{}, 0, 100)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}
```

- [ ] **Step 2: Run runtime tests and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/runtime -run 'TestRuntimeListAgentThreadsByWorkspaceRejectsInvalidWorkspace|TestRuntimeListThreadMessagesRejectsInvalidThread' -count=1
```

Expected: FAIL because `ListAgentThreadsByWorkspace` and `ListThreadMessages` do not exist yet.

- [ ] **Step 3: Add sqlc queries**

Append to `apps/server/sqlc/queries/agent_thread.sql`:

```sql
-- name: ListObservableAgentThreadsByWorkspace :many
SELECT *
FROM agent_thread
WHERE workspace_id = $1
  AND ($2::boolean OR role <> 'producer')
ORDER BY updated_at DESC, created_at DESC;

-- name: GetAgentThreadForWorkspace :one
SELECT *
FROM agent_thread
WHERE id = $1
  AND workspace_id = $2
LIMIT 1;

-- name: GetLatestAgentTaskByThread :one
SELECT *
FROM agent_task
WHERE thread_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: GetLatestAgentMessageByThread :one
SELECT *
FROM agent_message
WHERE thread_id = $1
ORDER BY seq DESC
LIMIT 1;
```

Keep `apps/server/sqlc/queries/agent_message.sql` unchanged unless implementation needs `after_created_at` for the new route; current `ListAgentMessagesByThread(thread_id, seq, limit)` already supports `after_seq`.

- [ ] **Step 4: Generate sqlc code**

Run:

```bash
make sqlc-generate
```

Expected: `apps/server/internal/store/db/agent_thread.sql.go` contains `ListObservableAgentThreadsByWorkspace`, `GetAgentThreadForWorkspace`, `GetLatestAgentTaskByThread`, and `GetLatestAgentMessageByThread`.

- [ ] **Step 5: Add runtime methods**

In `apps/server/internal/agent/runtime/service.go`, add:

```go
func (s *Service) ListAgentThreadsByWorkspace(ctx context.Context, workspaceID pgtype.UUID, includeProducer bool) ([]db.AgentThread, error) {
	if s == nil || s.queries == nil {
		return nil, ErrInvalidConfig
	}
	if !workspaceID.Valid {
		return nil, ErrInvalidRequest
	}
	return s.queries.ListObservableAgentThreadsByWorkspace(ctx, db.ListObservableAgentThreadsByWorkspaceParams{
		WorkspaceID: workspaceID,
		Column2:     includeProducer,
	})
}

func (s *Service) GetThreadForWorkspace(ctx context.Context, threadID pgtype.UUID, workspaceID pgtype.UUID) (db.AgentThread, error) {
	if s == nil || s.queries == nil {
		return db.AgentThread{}, ErrInvalidConfig
	}
	if !threadID.Valid || !workspaceID.Valid {
		return db.AgentThread{}, ErrInvalidRequest
	}
	return s.queries.GetAgentThreadForWorkspace(ctx, db.GetAgentThreadForWorkspaceParams{
		ID:          threadID,
		WorkspaceID: workspaceID,
	})
}

func (s *Service) ListThreadMessages(ctx context.Context, threadID pgtype.UUID, afterSeq int64, limit int32) ([]db.AgentMessage, error) {
	return s.ListMessages(ctx, threadID, afterSeq, limit)
}

func (s *Service) LatestTaskByThread(ctx context.Context, threadID pgtype.UUID) (db.AgentTask, error) {
	if s == nil || s.queries == nil {
		return db.AgentTask{}, ErrInvalidConfig
	}
	if !threadID.Valid {
		return db.AgentTask{}, ErrInvalidRequest
	}
	return s.queries.GetLatestAgentTaskByThread(ctx, threadID)
}

func (s *Service) LatestMessageByThread(ctx context.Context, threadID pgtype.UUID) (db.AgentMessage, error) {
	if s == nil || s.queries == nil {
		return db.AgentMessage{}, ErrInvalidConfig
	}
	if !threadID.Valid {
		return db.AgentMessage{}, ErrInvalidRequest
	}
	return s.queries.GetLatestAgentMessageByThread(ctx, threadID)
}
```

If sqlc names the boolean argument `IncludeProducer` instead of `Column2`, use the generated param field name from `agent_thread.sql.go`.

- [ ] **Step 6: Run runtime tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/runtime -count=1
```

Expected: PASS.

## Task 2: Backend API Responses, Handlers, And Routes

**Files:**
- Modify: `apps/server/internal/api/agent_response.go`
- Modify: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/server/cmd/server/main.go`
- Test: `apps/server/internal/api/agent_handler_test.go`

- [ ] **Step 1: Add failing route contract tests**

Append to `apps/server/internal/api/agent_handler_test.go`:

```go
func TestAgentThreadObserverRouteContract(t *testing.T) {
	handlerSource, err := os.ReadFile("agent_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(handlerSource), "func (h *AgentHandler) ListThreads") {
		t.Fatal("AgentHandler.ListThreads must be implemented")
	}
	if !strings.Contains(string(handlerSource), "func (h *AgentHandler) ListThreadMessages") {
		t.Fatal("AgentHandler.ListThreadMessages must be implemented")
	}

	serverSource, err := os.ReadFile("../../cmd/server/main.go")
	if err != nil {
		t.Fatal(err)
	}
	wantThreadsRoute := `GET("/api/agent/workspaces/:workspaceID/threads", authMiddleware, agentHandler.ListThreads)`
	if !strings.Contains(string(serverSource), wantThreadsRoute) {
		t.Fatalf("server route %q is not registered", wantThreadsRoute)
	}
	wantMessagesRoute := `GET("/api/agent/workspaces/:workspaceID/threads/:threadID/messages", authMiddleware, agentHandler.ListThreadMessages)`
	if !strings.Contains(string(serverSource), wantMessagesRoute) {
		t.Fatalf("server route %q is not registered", wantMessagesRoute)
	}
}
```

- [ ] **Step 2: Run route test and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run TestAgentThreadObserverRouteContract -count=1
```

Expected: FAIL because handlers/routes are absent.

- [ ] **Step 3: Add response types**

In `apps/server/internal/api/agent_response.go`, add:

```go
type agentObservedThreadResponse struct {
	agentThreadResponse
	DisplayName          string                   `json:"display_name"`
	ScopeLabel           string                   `json:"scope_label"`
	ScopeTitle           string                   `json:"scope_title,omitempty"`
	LatestTask           *agentTaskResponse       `json:"latest_task,omitempty"`
	LatestMessageAt      *string                  `json:"latest_message_at,omitempty"`
	LatestMessagePreview string                   `json:"latest_message_preview,omitempty"`
	Metadata             map[string]any           `json:"metadata,omitempty"`
}

type listAgentThreadsResponse struct {
	Threads []agentObservedThreadResponse `json:"threads"`
}
```

Add helper functions near existing response mappers:

```go
func toObservedAgentThreadResponse(thread db.AgentThread, latestTask *db.AgentTask, latestMessage *db.AgentMessage) agentObservedThreadResponse {
	base := toAgentThreadResponse(thread)
	out := agentObservedThreadResponse{
		agentThreadResponse: base,
		DisplayName:         observedThreadDisplayName(thread),
		ScopeLabel:          observedThreadScopeLabel(thread),
	}
	if latestTask != nil {
		task := toAgentTaskResponse(*latestTask)
		out.LatestTask = &task
	}
	if latestMessage != nil {
		createdAt := timestamptzString(latestMessage.CreatedAt)
		if createdAt != nil {
			out.LatestMessageAt = createdAt
		}
		out.LatestMessagePreview = agentMessagePreview(*latestMessage)
	}
	return out
}

func observedThreadDisplayName(thread db.AgentThread) string {
	role := strings.Title(thread.Role)
	label := observedThreadScopeLabel(thread)
	if label == "" {
		return role
	}
	return role + " · " + label
}

func observedThreadScopeLabel(thread db.AgentThread) string {
	if thread.ScopeID.Valid {
		return thread.ScopeType + ":" + uuidToString(thread.ScopeID)
	}
	return thread.ScopeType
}

func agentMessagePreview(message db.AgentMessage) string {
	response := toAgentMessageResponse(message)
	return strings.TrimSpace(agentMessageContentText(response.Content))
}

func agentMessageContentText(content map[string]any) string {
	blocks, _ := content["blocks"].([]any)
	lines := make([]string, 0, len(blocks))
	for _, block := range blocks {
		value, _ := block.(map[string]any)
		text, _ := value["text"].(string)
		if strings.TrimSpace(text) != "" {
			lines = append(lines, strings.TrimSpace(text))
		}
	}
	return strings.Join(lines, "\n")
}
```

If `strings.Title` is unavailable or lint rejects it, replace with a small switch:

```go
switch thread.Role {
case "craftsman":
	return "Craftsman"
case "reviewer":
	return "Reviewer"
case "composer":
	return "Composer"
default:
	return thread.Role
}
```

- [ ] **Step 4: Add handlers**

In `apps/server/internal/api/agent_handler.go`, add:

```go
func (h *AgentHandler) ListThreads(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}
	includeProducer := strings.TrimSpace(c.Query("include_producer")) == "true"
	threads, err := h.runtime.ListAgentThreadsByWorkspace(ctx, workspace.ID, includeProducer)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list agent threads")
		return
	}
	out := make([]agentObservedThreadResponse, 0, len(threads))
	for _, thread := range threads {
		var latestTask *db.AgentTask
		if task, err := h.runtime.LatestTaskByThread(ctx, thread.ID); err == nil {
			latestTask = &task
		}
		var latestMessage *db.AgentMessage
		if message, err := h.runtime.LatestMessageByThread(ctx, thread.ID); err == nil {
			latestMessage = &message
		}
		out = append(out, toObservedAgentThreadResponse(thread, latestTask, latestMessage))
	}
	c.JSON(consts.StatusOK, listAgentThreadsResponse{Threads: out})
}

func (h *AgentHandler) ListThreadMessages(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}
	threadID, ok := uuidParam(c, "threadID")
	if !ok {
		writeError(c, consts.StatusBadRequest, "invalid agent thread")
		return
	}
	thread, err := h.runtime.GetThreadForWorkspace(ctx, threadID, workspace.ID)
	if err != nil {
		writeError(c, consts.StatusNotFound, "agent thread not found")
		return
	}
	messages, err := h.runtime.ListThreadMessages(ctx, thread.ID, queryInt64(c, "after_seq", 0), queryInt32(c, "limit", 1000))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "failed to list agent thread messages")
		return
	}
	out := make([]agentMessageResponse, 0, len(messages))
	for _, msg := range messages {
		out = append(out, h.toAgentMessageResponse(ctx, msg))
	}
	c.JSON(consts.StatusOK, listAgentMessagesResponse{
		Thread:   toAgentThreadResponse(thread),
		Messages: out,
	})
}
```

If `uuidParam` or `queryInt64` does not exist, add local helpers beside existing `queryInt32`:

```go
func queryInt64(c *app.RequestContext, key string, fallback int64) int64 {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
```

- [ ] **Step 5: Register routes**

In `apps/server/cmd/server/main.go`, add routes near the existing Agent API routes:

```go
h.GET("/api/agent/workspaces/:workspaceID/threads", authMiddleware, agentHandler.ListThreads)
h.GET("/api/agent/workspaces/:workspaceID/threads/:threadID/messages", authMiddleware, agentHandler.ListThreadMessages)
```

- [ ] **Step 6: Run API tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestAgentThreadObserverRouteContract|TestAgentMessageResponse|TestAgentTaskResponse' -count=1
```

Expected: PASS.

## Task 3: Frontend API Types And Thread Helpers

**Files:**
- Modify: `apps/web/src/lib/agentApi.ts`
- Create: `apps/web/src/lib/agentThreads.ts`
- Test: `apps/web/src/lib/agentThreads.test.mjs`
- Modify: `apps/web/package.json` only if test build script requires explicit inclusion; otherwise leave unchanged.

- [ ] **Step 1: Add failing frontend tests**

Create `apps/web/src/lib/agentThreads.test.mjs`:

```js
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  mergeAgentThreadMessages,
  mergeObservedAgentThreads,
  threadPreviewFromMessage,
  updateObservedThreadFromMessage,
  updateObservedThreadFromTask,
} from "../../dist-test/lib/agentThreads.js";

describe("agent thread observer helpers", () => {
  it("dedupes and sorts observed threads by active work and latest message", () => {
    const threads = mergeObservedAgentThreads(
      [
        { id: "a", role: "craftsman", status: "active", latest_task: { id: "ta", status: "succeeded", task_type: "craftsman_turn" }, latest_message_at: "2026-06-26T09:00:00Z" },
      ],
      [
        { id: "b", role: "reviewer", status: "active", latest_task: { id: "tb", status: "running", task_type: "reviewer_turn" }, latest_message_at: "2026-06-26T08:00:00Z" },
      ],
    );
    assert.deepEqual(threads.map((thread) => thread.id), ["b", "a"]);
  });

  it("merges messages into a per-thread cache", () => {
    const cache = mergeAgentThreadMessages({}, "thread-1", [
      { id: "m2", thread_id: "thread-1", seq: 2 },
      { id: "m1", thread_id: "thread-1", seq: 1 },
    ]);
    assert.deepEqual(cache["thread-1"].messages.map((message) => message.id), ["m1", "m2"]);
    assert.equal(cache["thread-1"].hasLoadedInitial, true);
  });

  it("extracts markdown preview from an agent message", () => {
    assert.equal(
      threadPreviewFromMessage({
        content: {
          schema: "clipanvil.agent.message.v1",
          blocks: [{ id: "blk", type: "markdown", text: "已创建 RenderPlan" }],
        },
      }),
      "已创建 RenderPlan",
    );
  });

  it("updates thread item from task and message events", () => {
    const threads = [{ id: "thread-1", latest_message_preview: "" }];
    const withTask = updateObservedThreadFromTask(threads, { id: "task-1", thread_id: "thread-1", status: "running", task_type: "craftsman_turn" });
    assert.equal(withTask[0].latest_task.status, "running");
    const withMessage = updateObservedThreadFromMessage(withTask, {
      id: "m1",
      thread_id: "thread-1",
      seq: 1,
      created_at: "2026-06-26T09:05:00Z",
      content: { schema: "clipanvil.agent.message.v1", blocks: [{ id: "blk", type: "markdown", text: "完成" }] },
    });
    assert.equal(withMessage[0].latest_message_preview, "完成");
    assert.equal(withMessage[0].latest_message_at, "2026-06-26T09:05:00Z");
  });
});
```

- [ ] **Step 2: Run frontend helper test and verify failure**

Run:

```bash
pnpm --filter @clip-anvil/web... build
node --test apps/web/src/lib/agentThreads.test.mjs
```

Expected: build or test FAIL because `agentThreads.ts` does not exist.

- [ ] **Step 3: Add API types and fetchers**

In `apps/web/src/lib/agentApi.ts`, add:

```ts
export interface AgentObservedThread extends AgentThread {
  display_name: string;
  scope_label: string;
  scope_title?: string;
  latest_task?: AgentTask;
  latest_message_at?: string;
  latest_message_preview?: string;
  metadata?: Record<string, unknown>;
}

export interface AgentThreadsResponse {
  threads: AgentObservedThread[];
}
```

Add fetchers near existing Agent fetch functions:

```ts
export function fetchAgentThreads(workspaceId: string) {
  return apiFetch<AgentThreadsResponse>(
    `/agent/workspaces/${workspaceId}/threads`,
  );
}

export function fetchAgentThreadMessages(
  workspaceId: string,
  threadId: string,
  afterSeq = 0,
  limit = 1000,
) {
  const params = new URLSearchParams({ limit: String(limit) });
  if (afterSeq > 0) {
    params.set("after_seq", String(afterSeq));
  }
  return apiFetch<AgentMessagesResponse>(
    `/agent/workspaces/${workspaceId}/threads/${threadId}/messages?${params.toString()}`,
  );
}
```

Update `AgentThread.scope_type` union if needed:

```ts
scope_type:
  | "workspace"
  | "shot"
  | "final_output"
  | "render_plan"
  | "key_element_state";
```

- [ ] **Step 4: Implement helper module**

Create `apps/web/src/lib/agentThreads.ts`:

```ts
import type { AgentMessage, AgentObservedThread, AgentTask } from "./agentApi";
import { mergeAgentMessages } from "./agentMessages";

export interface AgentThreadMessageCacheEntry {
  messages: AgentMessage[];
  hasLoadedInitial: boolean;
}

export type AgentThreadMessageCache = Record<string, AgentThreadMessageCacheEntry>;

export function mergeObservedAgentThreads(
  current: Partial<AgentObservedThread>[],
  incoming: Partial<AgentObservedThread>[],
) {
  const byId = new Map<string, Partial<AgentObservedThread>>();
  for (const thread of current) {
    if (thread.id) {
      byId.set(thread.id, thread);
    }
  }
  for (const thread of incoming) {
    if (thread.id) {
      byId.set(thread.id, { ...byId.get(thread.id), ...thread });
    }
  }
  return Array.from(byId.values()).sort(compareObservedThreads);
}

export function mergeAgentThreadMessages(
  current: AgentThreadMessageCache,
  threadId: string,
  incoming: AgentMessage[],
): AgentThreadMessageCache {
  const entry = current[threadId] ?? { messages: [], hasLoadedInitial: false };
  return {
    ...current,
    [threadId]: {
      messages: mergeAgentMessages(entry.messages, incoming),
      hasLoadedInitial: true,
    },
  };
}

export function updateObservedThreadFromTask<T extends Partial<AgentObservedThread>>(
  threads: T[],
  task: Partial<AgentTask>,
): T[] {
  if (!task.thread_id) {
    return threads;
  }
  return threads.map((thread) =>
    thread.id === task.thread_id
      ? ({ ...thread, latest_task: task } as T)
      : thread,
  );
}

export function updateObservedThreadFromMessage<T extends Partial<AgentObservedThread>>(
  threads: T[],
  message: Partial<AgentMessage>,
): T[] {
  if (!message.thread_id) {
    return threads;
  }
  return threads.map((thread) =>
    thread.id === message.thread_id
      ? ({
          ...thread,
          latest_message_at: message.created_at,
          latest_message_preview: threadPreviewFromMessage(message),
        } as T)
      : thread,
  );
}

export function threadPreviewFromMessage(message: Pick<Partial<AgentMessage>, "content">) {
  const content = message.content;
  if (!content || typeof content !== "object") {
    return "";
  }
  const blocks = (content as { blocks?: unknown }).blocks;
  if (!Array.isArray(blocks)) {
    return "";
  }
  return blocks
    .map((block) => {
      if (!block || typeof block !== "object") {
        return "";
      }
      const text = (block as { text?: unknown }).text;
      return typeof text === "string" ? text.trim() : "";
    })
    .filter(Boolean)
    .join("\n")
    .slice(0, 160);
}

function compareObservedThreads(
  left: Partial<AgentObservedThread>,
  right: Partial<AgentObservedThread>,
) {
  const leftActive = activeRank(left.latest_task?.status);
  const rightActive = activeRank(right.latest_task?.status);
  if (leftActive !== rightActive) {
    return leftActive - rightActive;
  }
  const leftTime = Date.parse(left.latest_message_at ?? left.updated_at ?? left.created_at ?? "");
  const rightTime = Date.parse(right.latest_message_at ?? right.updated_at ?? right.created_at ?? "");
  if (!Number.isNaN(leftTime) && !Number.isNaN(rightTime) && leftTime !== rightTime) {
    return rightTime - leftTime;
  }
  return String(left.display_name ?? left.id ?? "").localeCompare(String(right.display_name ?? right.id ?? ""));
}

function activeRank(status: unknown) {
  if (status === "running") {
    return 0;
  }
  if (status === "queued") {
    return 1;
  }
  if (status === "failed") {
    return 2;
  }
  return 3;
}
```

- [ ] **Step 5: Run helper tests**

Run:

```bash
pnpm --filter @clip-anvil/web... build
node --test apps/web/src/lib/agentThreads.test.mjs
```

Expected: PASS.

## Task 4: Read-Only Observer UI Components

**Files:**
- Create: `apps/web/src/components/agent/AgentThreadObserverPanel.tsx`
- Create: `apps/web/src/components/agent/AgentThreadDrawer.tsx`
- Create: `apps/web/src/components/agent/AgentThreadLinkChip.tsx`
- Modify: `apps/web/src/styles.css`
- Test: `apps/web/src/lib/agentCanvas.test.mjs` or create `apps/web/src/lib/agentThreadComponents.test.mjs`

- [ ] **Step 1: Add component source tests**

Create `apps/web/src/lib/agentThreadComponents.test.mjs`:

```js
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { readFileSync } from "node:fs";

describe("agent thread observer components", () => {
  it("defines observer panel, read-only drawer, and link chip", () => {
    const panel = readFileSync(new URL("../components/agent/AgentThreadObserverPanel.tsx", import.meta.url), "utf8");
    const drawer = readFileSync(new URL("../components/agent/AgentThreadDrawer.tsx", import.meta.url), "utf8");
    const chip = readFileSync(new URL("../components/agent/AgentThreadLinkChip.tsx", import.meta.url), "utf8");
    assert.match(panel, /agent-thread-observer-panel/);
    assert.match(drawer, /agent-thread-drawer/);
    assert.match(drawer, /只读/);
    assert.match(chip, /agent-thread-link-chip/);
  });
});
```

- [ ] **Step 2: Run component source test and verify failure**

Run:

```bash
node --test apps/web/src/lib/agentThreadComponents.test.mjs
```

Expected: FAIL because component files do not exist.

- [ ] **Step 3: Implement `AgentThreadLinkChip`**

Create `apps/web/src/components/agent/AgentThreadLinkChip.tsx`:

```tsx
import type { AgentObservedThread } from "../../lib/agentApi";

export function AgentThreadLinkChip({
  thread,
  onSelect,
}: {
  thread: Pick<AgentObservedThread, "id" | "display_name" | "latest_task">;
  onSelect: (threadId: string) => void;
}) {
  const status = thread.latest_task?.status ?? "idle";
  return (
    <button
      className={`agent-thread-link-chip agent-thread-link-chip-${status}`}
      onClick={() => onSelect(thread.id)}
      type="button"
    >
      <span aria-hidden="true" className="agent-thread-status-dot" />
      <span>{thread.display_name}</span>
    </button>
  );
}
```

- [ ] **Step 4: Implement `AgentThreadObserverPanel`**

Create `apps/web/src/components/agent/AgentThreadObserverPanel.tsx`:

```tsx
import type { AgentObservedThread } from "../../lib/agentApi";
import { formatAgentMessageTime } from "../../lib/agentMessages";

export function AgentThreadObserverPanel({
  threads,
  selectedThreadId,
  onSelectThread,
}: {
  threads: AgentObservedThread[];
  selectedThreadId?: string;
  onSelectThread: (threadId: string) => void;
}) {
  return (
    <section className="agent-thread-observer-panel" aria-label="子 Agent">
      <header>
        <span>Agents</span>
        <small>{threads.length}</small>
      </header>
      {threads.length === 0 ? (
        <p className="agent-thread-observer-empty">暂无子 Agent</p>
      ) : (
        <div className="agent-thread-observer-list">
          {threads.map((thread) => (
            <button
              className={`agent-thread-observer-item${selectedThreadId === thread.id ? " selected" : ""}`}
              key={thread.id}
              onClick={() => onSelectThread(thread.id)}
              type="button"
            >
              <span className={`agent-thread-role agent-thread-role-${thread.role}`}>
                {thread.role}
              </span>
              <strong>{thread.scope_label || thread.display_name}</strong>
              {thread.latest_task ? (
                <small>{thread.latest_task.status}</small>
              ) : null}
              {thread.latest_message_preview ? (
                <span>{thread.latest_message_preview}</span>
              ) : null}
              {thread.latest_message_at ? (
                <time dateTime={thread.latest_message_at}>
                  {formatAgentMessageTime(thread.latest_message_at)}
                </time>
              ) : null}
            </button>
          ))}
        </div>
      )}
    </section>
  );
}
```

- [ ] **Step 5: Implement `AgentThreadDrawer`**

Create `apps/web/src/components/agent/AgentThreadDrawer.tsx`:

```tsx
import type { AgentMessage, AgentObservedThread } from "../../lib/agentApi";
import { formatAgentMessageTime, visibleAgentMessages } from "../../lib/agentMessages";
import { AgentMessageRenderer } from "./AgentMessageRenderer";

export function AgentThreadDrawer({
  thread,
  messages,
  isLoading,
  onClose,
}: {
  thread?: AgentObservedThread;
  messages: AgentMessage[];
  isLoading: boolean;
  onClose: () => void;
}) {
  if (!thread) {
    return null;
  }
  return (
    <aside className="agent-thread-drawer" aria-label={`${thread.display_name} 只读线程`}>
      <header className="agent-thread-drawer-header">
        <div>
          <span className="agent-thread-drawer-kicker">只读 Agent 线程</span>
          <h3>{thread.display_name}</h3>
          <small>{thread.id}</small>
        </div>
        <button aria-label="关闭子 Agent 线程" onClick={onClose} type="button">
          ×
        </button>
      </header>
      <div className="agent-thread-drawer-body">
        {isLoading ? (
          <p className="agent-empty-text">正在加载子 Agent 对话</p>
        ) : visibleAgentMessages(messages).length === 0 ? (
          <p className="agent-empty-text">这个 Agent 还没有消息。</p>
        ) : (
          visibleAgentMessages(messages).map((message) => (
            <article className={`agent-thread-message agent-message-${message.role}`} key={message.id}>
              <AgentMessageRenderer message={message} />
              {message.created_at ? (
                <time dateTime={message.created_at}>
                  {formatAgentMessageTime(message.created_at)}
                </time>
              ) : null}
            </article>
          ))
        )}
      </div>
    </aside>
  );
}
```

- [ ] **Step 6: Add CSS**

Append to the existing Agent CSS section in `apps/web/src/styles.css`:

```css
.agent-chat-with-observer {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 132px;
  min-height: 0;
  flex: 1;
}

.agent-thread-observer-panel {
  border-left: 1px solid var(--border);
  background: color-mix(in srgb, var(--surface) 92%, var(--muted) 8%);
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.agent-thread-observer-panel header {
  display: flex;
  justify-content: space-between;
  padding: 10px;
  font-size: 12px;
  font-weight: 700;
}

.agent-thread-observer-list {
  display: grid;
  gap: 6px;
  padding: 0 8px 10px;
  overflow: auto;
}

.agent-thread-observer-item,
.agent-thread-link-chip {
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text);
  cursor: pointer;
}

.agent-thread-observer-item {
  border-radius: 8px;
  padding: 8px;
  display: grid;
  gap: 3px;
  text-align: left;
  font-size: 11px;
}

.agent-thread-observer-item.selected {
  border-color: var(--accent);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent) 18%, transparent);
}

.agent-thread-link-chip {
  border-radius: 999px;
  padding: 5px 8px;
  display: inline-flex;
  gap: 6px;
  align-items: center;
  font-size: 11px;
}

.agent-thread-drawer {
  position: absolute;
  inset: 0 0 0 auto;
  width: min(520px, 46vw);
  border-left: 1px solid var(--border);
  background: var(--surface);
  box-shadow: -18px 0 36px rgba(15, 23, 42, 0.16);
  display: flex;
  flex-direction: column;
  z-index: 5;
}

.agent-thread-drawer-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 14px;
  border-bottom: 1px solid var(--border);
}

.agent-thread-drawer-body {
  flex: 1;
  overflow: auto;
  padding: 14px;
  display: grid;
  align-content: start;
  gap: 10px;
}

.agent-thread-message {
  border-radius: 10px;
  padding: 9px 10px;
  background: var(--muted);
}
```

Use existing CSS variables if names differ; keep selectors stable for tests.

- [ ] **Step 7: Run component source test and web build**

Run:

```bash
node --test apps/web/src/lib/agentThreadComponents.test.mjs
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

## Task 5: Page Integration And WebSocket Thread Splitting

**Files:**
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/lib/agentWs.ts` only if event types need optional role/task fields; prefer no change.
- Test: `apps/web/src/lib/agentThreads.test.mjs`
- Test: `apps/web/src/lib/agentTasks.test.mjs`

- [ ] **Step 1: Add tests for Producer input not blocked by sub Agent tasks**

In `apps/web/src/lib/agentTasks.test.mjs`, add:

```js
it("does not treat running craftsman tasks as Producer input blocking work", () => {
  assert.equal(
    hasProcessingAgentTask([
      { id: "craftsman", task_type: "craftsman_turn", status: "running" },
    ]),
    false,
  );
  assert.equal(
    hasProcessingAgentTask([
      { id: "producer", task_type: "producer_turn", status: "running" },
    ]),
    true,
  );
});
```

This may already pass; keep it as regression coverage.

- [ ] **Step 2: Add page source contract test**

In `apps/web/src/lib/agentThreadComponents.test.mjs`, add:

```js
it("wires the observer panel and drawer into AgentWorkspacePage", () => {
  const page = readFileSync(new URL("../pages/AgentWorkspacePage.tsx", import.meta.url), "utf8");
  assert.match(page, /fetchAgentThreads/);
  assert.match(page, /fetchAgentThreadMessages/);
  assert.match(page, /AgentThreadObserverPanel/);
  assert.match(page, /AgentThreadDrawer/);
});
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
node --test apps/web/src/lib/agentThreadComponents.test.mjs
node --test apps/web/src/lib/agentTasks.test.mjs
```

Expected: page contract test FAIL until imports and components are wired.

- [ ] **Step 4: Add imports and state to page**

In `apps/web/src/pages/AgentWorkspacePage.tsx`, update imports:

```ts
import {
  type AgentAttachment,
  type AgentMessage,
  type AgentObservedThread,
  type AgentTask,
  fetchAgentCanvasDetail,
  fetchAgentModelSelection,
  fetchAgentMessages,
  fetchAgentCanvasWorkbench,
  fetchAgentTasks,
  fetchAgentThread,
  fetchAgentThreadMessages,
  fetchAgentThreads,
  postAgentDecision,
  postAgentMessage,
  putAgentModelSelection,
  uploadAgentAttachment,
} from "../lib/agentApi";
import { AgentThreadDrawer } from "../components/agent/AgentThreadDrawer";
import { AgentThreadObserverPanel } from "../components/agent/AgentThreadObserverPanel";
import {
  type AgentThreadMessageCache,
  mergeAgentThreadMessages,
  mergeObservedAgentThreads,
  updateObservedThreadFromMessage,
  updateObservedThreadFromTask,
} from "../lib/agentThreads";
```

Add state near existing Agent state:

```ts
const [observedThreads, setObservedThreads] = useState<AgentObservedThread[]>([]);
const [selectedAgentThreadId, setSelectedAgentThreadId] = useState("");
const [agentThreadMessageCache, setAgentThreadMessageCache] =
  useState<AgentThreadMessageCache>({});
const [subThreadStreams, setSubThreadStreams] = useState<Record<string, AgentStreamState[]>>({});
```

- [ ] **Step 5: Add queries**

Add query:

```ts
const agentThreadsQuery = useQuery({
  queryKey: ["agent", id, "threads"],
  queryFn: () => fetchAgentThreads(id ?? ""),
  enabled: agentEnabled,
});

const selectedAgentThread = observedThreads.find(
  (thread) => thread.id === selectedAgentThreadId,
);

const selectedAgentThreadMessagesQuery = useQuery({
  queryKey: ["agent", id, "threads", selectedAgentThreadId, "messages"],
  queryFn: () => fetchAgentThreadMessages(id ?? "", selectedAgentThreadId),
  enabled: agentEnabled && Boolean(selectedAgentThreadId),
});
```

Add effects:

```ts
useEffect(() => {
  if (agentThreadsQuery.data) {
    setObservedThreads((current) =>
      mergeObservedAgentThreads(current, agentThreadsQuery.data.threads) as AgentObservedThread[],
    );
  }
}, [agentThreadsQuery.data]);

useEffect(() => {
  if (selectedAgentThreadId && selectedAgentThreadMessagesQuery.data) {
    setAgentThreadMessageCache((current) =>
      mergeAgentThreadMessages(
        current,
        selectedAgentThreadId,
        selectedAgentThreadMessagesQuery.data.messages,
      ),
    );
  }
}, [selectedAgentThreadId, selectedAgentThreadMessagesQuery.data]);
```

- [ ] **Step 6: Split WebSocket messages by thread**

In the existing `connectAgentSocket` `onEvent`, change message branch:

```ts
const eventThreadID = event.payload.thread_id || event.payload.message?.thread_id;
if (
  (event.type === "agent.message.created" ||
    event.type === "agent.message.updated") &&
  event.payload.workspace_id === id
) {
  const message = event.payload.message;
  if (isProducerThreadMessage(message, producerThreadID)) {
    setMessages((current) => mergeAgentMessages(current, [message]));
    // keep existing stream finalization and canvas refetch behavior
  } else if (message.thread_id) {
    setAgentThreadMessageCache((current) =>
      mergeAgentThreadMessages(current, message.thread_id, [message]),
    );
    setObservedThreads((current) =>
      updateObservedThreadFromMessage(current, message) as AgentObservedThread[],
    );
    if (!observedThreads.some((thread) => thread.id === message.thread_id)) {
      void queryClient.invalidateQueries({ queryKey: ["agent", id, "threads"] });
    }
  }
}
```

Use functional state only for `observedThreads` membership if React stale closure causes lint issues:

```ts
setObservedThreads((current) => {
  const next = updateObservedThreadFromMessage(current, message) as AgentObservedThread[];
  if (!current.some((thread) => thread.id === message.thread_id)) {
    void queryClient.invalidateQueries({ queryKey: ["agent", id, "threads"] });
  }
  return next;
});
```

For `agent.message.delta`, keep Producer path unchanged and add:

```ts
if (event.type === "agent.message.delta" && event.payload.workspace_id === id) {
  if (event.payload.thread_id === producerThreadID) {
    setStreams((current) => mergeAgentStreamDelta(current, deltaPayload, finalizedStreamKeysRef.current));
  } else if (event.payload.thread_id === selectedAgentThreadId) {
    setSubThreadStreams((current) => ({
      ...current,
      [event.payload.thread_id]: mergeAgentStreamDelta(
        current[event.payload.thread_id] ?? [],
        deltaPayload,
      ),
    }));
  }
}
```

For `agent.task.updated`, add:

```ts
setObservedThreads((current) =>
  updateObservedThreadFromTask(current, event.payload.task) as AgentObservedThread[],
);
```

- [ ] **Step 7: Render observer and drawer**

Wrap message list and observer panel:

```tsx
<div className="agent-chat-with-observer">
  <div className="agent-chat-main-column">
    <div className="agent-message-list" ...>
      ...
    </div>
    ...
  </div>
  <AgentThreadObserverPanel
    threads={observedThreads}
    selectedThreadId={selectedAgentThreadId}
    onSelectThread={setSelectedAgentThreadId}
  />
</div>
<AgentThreadDrawer
  thread={selectedAgentThread}
  messages={[
    ...(selectedAgentThreadId
      ? agentThreadMessageCache[selectedAgentThreadId]?.messages ?? []
      : []),
  ]}
  isLoading={selectedAgentThreadMessagesQuery.isLoading}
  onClose={() => setSelectedAgentThreadId("")}
/>
```

Move send error/composer below `agent-chat-with-observer` so Producer input remains anchored at the bottom.

- [ ] **Step 8: Run frontend tests and build**

Run:

```bash
node --test apps/web/src/lib/agentThreadComponents.test.mjs
node --test apps/web/src/lib/agentThreads.test.mjs
node --test apps/web/src/lib/agentTasks.test.mjs
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Expected: PASS.

## Task 6: Backend And Frontend Route Integration Validation

**Files:**
- Modify if needed: `apps/server/internal/api/agent_handler_test.go`
- Modify if needed: `apps/web/src/lib/agentApi.ts`

- [ ] **Step 1: Run server tests for touched packages**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api ./internal/agent/runtime -count=1
```

Expected: PASS.

- [ ] **Step 2: Run full server checks**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
make server-lint
```

Expected: PASS.

- [ ] **Step 3: Check generated code and formatting**

Run:

```bash
gofmt -w apps/server/internal/api/agent_handler.go apps/server/internal/api/agent_response.go apps/server/internal/agent/runtime/service.go apps/server/internal/api/agent_handler_test.go apps/server/internal/agent/runtime/service_test.go
git diff --check
```

Expected: no output from `git diff --check`.

## Task 7: Dispatch Tool Card Thread Chips And Remove Nested Sub-Agent Rendering

**Files:**
- Modify: `apps/web/src/components/agent/AgentToolStatusBlock.tsx`
- Modify: `apps/web/src/components/agent/AgentMessageRenderer.tsx`
- Modify: `apps/web/src/lib/agentMessages.ts`
- Modify: `apps/web/src/lib/agentMessageBlocks.ts` only if block type needs stronger typing.
- Test: `apps/web/src/lib/agentMessages.test.mjs`
- Test: `apps/web/src/lib/agentMessageBlocks.test.mjs` if block parsing changes.

- [ ] **Step 1: Change nested message test expectation**

In `apps/web/src/lib/agentMessages.test.mjs`, replace the test that expects nested child messages after a parent tool call with:

```js
it("does not reorder child thread messages into the Producer main chat", () => {
  const messages = mergeAgentMessages(
    [
      {
        id: "child-tool",
        thread_id: "craftsman-thread",
        seq: 1,
        created_at: "2026-06-25T10:00:02Z",
        raw_message: { parent_tool_call_id: "dispatch-1" },
      },
      {
        id: "producer-tool",
        thread_id: "producer-thread",
        seq: 10,
        created_at: "2026-06-25T10:00:10Z",
        raw_message: { tool_call_id: "dispatch-1" },
      },
    ],
    [
      {
        id: "producer-answer",
        thread_id: "producer-thread",
        seq: 11,
        created_at: "2026-06-25T10:00:11Z",
      },
    ],
  );

  assert.deepEqual(
    messages.map((message) => message.id),
    ["child-tool", "producer-tool", "producer-answer"],
  );
});
```

Add a Producer-only visibility check:

```js
it("keeps Producer visible messages free from sub-agent child rows", () => {
  const producerThreadID = "producer-thread";
  const all = [
    { id: "producer-tool", thread_id: producerThreadID, seq: 1, message_type: "tool_call" },
    { id: "child-tool", thread_id: "craftsman-thread", seq: 1, message_type: "tool_call", raw_message: { parent_tool_call_id: "dispatch-1" } },
  ];
  const producerOnly = all.filter((message) =>
    isProducerThreadMessage(message, producerThreadID),
  );
  assert.deepEqual(visibleAgentMessages(producerOnly).map((message) => message.id), ["producer-tool"]);
});
```

- [ ] **Step 2: Run message tests and verify failure if current nested order conflicts**

Run:

```bash
pnpm --filter @clip-anvil/web... build
node --test apps/web/src/lib/agentMessages.test.mjs
```

Expected: FAIL until `orderNestedAgentMessages` is removed or constrained to same-thread messages.

- [ ] **Step 3: Restrict nested ordering to same thread or remove it**

In `apps/web/src/lib/agentMessages.ts`, change child grouping so parent/child must share `thread_id`:

```ts
const parentKey = `${stringValue(message.thread_id)}:${stringValue(message.raw_message?.parent_tool_call_id)}`;
```

And parent lookup:

```ts
const toolCallID = stringValue(message.raw_message?.tool_call_id);
const key = `${stringValue(message.thread_id)}:${toolCallID}`;
```

If nested ordering is no longer useful in any same-thread UI, simplify `mergeAgentMessages` to:

```ts
return Array.from(byID.values()).sort(compareAgentMessages);
```

Prefer the same-thread constraint first because it preserves local tool grouping inside child drawers.

- [ ] **Step 4: Add thread selection action to tool block**

Update `AgentMessageRenderer` action type:

```ts
export interface AgentThreadSelectionAction {
  onSelectAgentThread?: (threadId: string) => void;
}

export type AgentMessageActions = AgentDecisionActions & AgentThreadSelectionAction;
```

Pass action into `AgentToolStatusBlock`:

```tsx
if (isToolStatusBlock(block)) {
  return <AgentToolStatusBlock actions={actions} block={block} />;
}
```

Update `AgentToolStatusBlock` props and render chips when result contains dispatch rows:

```tsx
export function AgentToolStatusBlock({
  block,
  actions,
}: {
  block: AgentToolStatusBlockData;
  actions?: { onSelectAgentThread?: (threadId: string) => void };
}) {
  const threads = dispatchThreads(block.result);
  ...
  {threads.length > 0 && actions?.onSelectAgentThread ? (
    <div className="agent-tool-thread-links">
      {threads.map((thread) => (
        <AgentThreadLinkChip
          key={thread.thread_id}
          thread={{ id: thread.thread_id, display_name: thread.label }}
          onSelect={actions.onSelectAgentThread}
        />
      ))}
    </div>
  ) : null}
}

function dispatchThreads(result: Record<string, unknown> | undefined) {
  const dispatched = Array.isArray(result?.dispatched) ? result.dispatched : [];
  return dispatched
    .map((item) => {
      if (!item || typeof item !== "object") {
        return null;
      }
      const value = item as Record<string, unknown>;
      const threadID =
        typeof value.craftsman_thread_id === "string"
          ? value.craftsman_thread_id
          : typeof value.reviewer_thread_id === "string"
            ? value.reviewer_thread_id
            : "";
      if (!threadID) {
        return null;
      }
      const label =
        typeof value.client_key === "string" && value.client_key
          ? String(value.client_key)
          : threadID.slice(0, 8);
      return { thread_id: threadID, label };
    })
    .filter((item): item is { thread_id: string; label: string } => Boolean(item));
}
```

- [ ] **Step 5: Pass selection action from page**

In `AgentWorkspacePage.tsx`, include:

```tsx
<AgentMessageRenderer
  actions={{
    ...messageActions,
    onSelectAgentThread: setSelectedAgentThreadId,
    disabled: isDecisionResolved || respondDecisionMutation.isPending,
  }}
  message={message}
/>
```

- [ ] **Step 6: Run frontend checks**

Run:

```bash
node --test apps/web/src/lib/agentMessages.test.mjs
node --test apps/web/src/lib/agentThreadComponents.test.mjs
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Expected: PASS.

## Task 8: Polish, Empty States, And URL Restoration

**Files:**
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/components/agent/AgentThreadObserverPanel.tsx`
- Modify: `apps/web/src/components/agent/AgentThreadDrawer.tsx`
- Modify: `apps/web/src/styles.css`
- Test: `apps/web/src/lib/agentThreadComponents.test.mjs`

- [ ] **Step 1: Add source contract for URL param restoration**

Add to `agentThreadComponents.test.mjs`:

```js
it("persists selected sub-agent thread through the agentThread query parameter", () => {
  const page = readFileSync(new URL("../pages/AgentWorkspacePage.tsx", import.meta.url), "utf8");
  assert.match(page, /agentThread/);
  assert.match(page, /URLSearchParams/);
});
```

- [ ] **Step 2: Implement URL param state**

In `AgentWorkspacePage.tsx`, import `useSearchParams` from `react-router`:

```ts
import { Navigate, useNavigate, useParams, useSearchParams } from "react-router";
```

Add:

```ts
const [searchParams, setSearchParams] = useSearchParams();
```

Initialize selected thread from URL:

```ts
const [selectedAgentThreadId, setSelectedAgentThreadIdState] = useState(
  () => searchParams.get("agentThread") ?? "",
);

const setSelectedAgentThreadId = (threadId: string) => {
  setSelectedAgentThreadIdState(threadId);
  const next = new URLSearchParams(searchParams);
  if (threadId) {
    next.set("agentThread", threadId);
  } else {
    next.delete("agentThread");
  }
  setSearchParams(next, { replace: true });
};
```

If this function conflicts with existing state setter references, rename to `selectAgentThread`.

- [ ] **Step 3: Add failed/running status polish**

In `AgentThreadObserverPanel`, map statuses:

```tsx
const statusLabel =
  thread.latest_task?.status === "running"
    ? "执行中"
    : thread.latest_task?.status === "queued"
      ? "排队中"
      : thread.latest_task?.status === "failed"
        ? "失败"
        : thread.latest_task?.status === "succeeded"
          ? "完成"
          : thread.status;
```

Render status label with stable class:

```tsx
<small className={`agent-thread-task-status agent-thread-task-status-${thread.latest_task?.status ?? thread.status}`}>
  {statusLabel}
</small>
```

- [ ] **Step 4: Run frontend checks**

Run:

```bash
node --test apps/web/src/lib/agentThreadComponents.test.mjs
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Expected: PASS.

## Task 9: End-To-End Verification

**Files:**
- Modify only if E2E reveals bugs.
- Test evidence can be manual command output and DB queries.

- [ ] **Step 1: Start local app through repo script**

Run:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
./scripts/dev-start.sh
```

Expected: script prints the current worktree frontend URL and starts backend/frontend. Use the printed Vite URL; do not assume `localhost:5175`.

- [ ] **Step 2: Use browser to create or open an Agent workspace**

Use the in-app browser or Chrome. Open the printed Vite URL, log in with an existing local account, and open an Agent workspace.

Expected: Agent page loads, Producer chat is visible, Agents observer panel shows zero or existing sub Agents.

- [ ] **Step 3: Trigger Producer dispatch**

Send a message that causes multiple Craftsman tasks, for example:

```text
做一个 15 秒行李箱抖音广告，先拆成 4 个分镜，并生成每个分镜的预览图。
```

Expected:

- Producer main chat shows user message, Producer assistant/tool messages.
- Agents observer panel gains 4 Craftsman items as dispatch happens.
- Producer input is blocked only while Producer task is queued/running; Craftsman running does not keep input disabled.

- [ ] **Step 4: Open a Craftsman drawer**

Click a Craftsman item or dispatch tool chip.

Expected:

- Drawer opens with read-only title.
- Delegation message “Producer 派发 Craftsman 任务。” appears in drawer, not in Producer main chat.
- Craftsman tool call/result messages append in real time if task is still running.

- [ ] **Step 5: Verify DB thread separation**

Use the local database connection from the dev script environment. Run equivalent SQL:

```sql
SELECT role, scope_type, scope_id, COUNT(*) AS messages
FROM agent_thread
JOIN agent_message ON agent_message.thread_id = agent_thread.id
WHERE agent_thread.workspace_id = '<workspace-id>'
GROUP BY role, scope_type, scope_id
ORDER BY role, scope_type;
```

Expected:

- Producer messages are under `role='producer'`.
- Craftsman messages are under `role='craftsman'`.
- Delegation messages are not in Producer thread.

- [ ] **Step 6: Refresh recovery**

Refresh the Agent page.

Expected:

- Agents observer panel restores from DB.
- If URL has `?agentThread=<threadID>`, drawer reopens.
- Drawer history loads from `GET /threads/:threadID/messages`.

- [ ] **Step 7: Stop dev server**

Run:

```bash
./scripts/dev-stop.sh
```

Expected: current worktree dev services stop cleanly.

## Task 10: Final Verification And Commit

**Files:**
- Stage only files changed for this feature.

- [ ] **Step 1: Run full verification**

Run:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
make server-lint
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected: all commands PASS.

- [ ] **Step 2: Inspect status and exclude runtime artifacts**

Run:

```bash
git status --short --branch
git diff --stat
git ls-files --others --exclude-standard
```

Expected: only source/test/doc files for this feature are modified or untracked. `.superpowers/brainstorm/`, `.vite`, `dist`, local screenshots, and runtime files are not staged.

- [ ] **Step 3: Stage explicit files**

Run with exact paths touched during implementation:

```bash
git add \
  apps/server/sqlc/queries/agent_thread.sql \
  apps/server/internal/store/db/agent_thread.sql.go \
  apps/server/internal/agent/runtime/service.go \
  apps/server/internal/agent/runtime/service_test.go \
  apps/server/internal/api/agent_response.go \
  apps/server/internal/api/agent_handler.go \
  apps/server/internal/api/agent_handler_test.go \
  apps/server/cmd/server/main.go \
  apps/web/src/lib/agentApi.ts \
  apps/web/src/lib/agentThreads.ts \
  apps/web/src/lib/agentThreads.test.mjs \
  apps/web/src/lib/agentMessages.ts \
  apps/web/src/lib/agentMessages.test.mjs \
  apps/web/src/lib/agentTasks.test.mjs \
  apps/web/src/lib/agentThreadComponents.test.mjs \
  apps/web/src/components/agent/AgentThreadObserverPanel.tsx \
  apps/web/src/components/agent/AgentThreadDrawer.tsx \
  apps/web/src/components/agent/AgentThreadLinkChip.tsx \
  apps/web/src/components/agent/AgentToolStatusBlock.tsx \
  apps/web/src/components/agent/AgentMessageRenderer.tsx \
  apps/web/src/pages/AgentWorkspacePage.tsx \
  apps/web/src/styles.css
```

If a listed file was not changed, remove it from the `git add` command.

- [ ] **Step 4: Commit**

Run:

```bash
git diff --cached --stat
git commit -m "feat: add agent subthread observer"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage:
  - M1 threads list and read-only drawer: Tasks 1-6.
  - WebSocket real-time rendering: Task 5.
  - Producer-only main chat: Tasks 5 and 7.
  - Dispatch card entrance and no nested child rendering: Task 7.
  - Refresh recovery: Task 8 and Task 9.
  - E2E and DB verification: Task 9.
- Placeholder scan:
  - No placeholder markers or unspecified implementation slots are left in the task steps.
- Type consistency:
  - `AgentObservedThread`, `AgentThreadMessageCache`, `fetchAgentThreads`, `fetchAgentThreadMessages`, `AgentThreadObserverPanel`, `AgentThreadDrawer`, and `AgentThreadLinkChip` are introduced before use.
  - Thread selection action is consistently named `onSelectAgentThread`.
