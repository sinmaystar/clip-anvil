# M6.1 Agent Runtime Persistence 设计方案

**状态**：待评审
**日期**：2026-06-21
**所属里程碑**：M6 MultiAgent Agent Mode
**阶段目标**：建立通用 Agent runtime 持久化基础，供 Producer、Craftsman、Reviewer、Composer 等所有需要持久上下文的 Agent 复用。M6.1 只交付后端数据库、sqlc 查询和最小 runtime service，不实现前端对话、WebSocket、Eino Graph、HITL 或生产工具。

## 1. 背景

M6 要完成完整 MultiAgent 架构，但第一步必须先有可靠的状态事实源。Agent 对话、Graph 执行、HITL 暂停恢复、工具调用审计、Craftsman 分镜上下文、Review 记录和 Composer 合成状态都依赖同一套 runtime 持久化能力。

M6.1 的产物是后续阶段的地基：

```text
agent_thread
  -> agent_message
  -> agent_task
  -> agent_event
  -> eino_checkpoint
```

这些表不属于 Producer 专用能力，而是所有 Agent role 共用的 runtime 层。

## 2. 范围

### 2.1 包含

- 新增 Agent runtime migrations。
- 新增 sqlc query 文件。
- 生成 db types 和 query methods。
- 最小 runtime service，用于：
  - 获取或创建 workspace scoped Producer thread。
  - 创建 role scoped thread。
  - 追加 message，并生成稳定递增 `seq`。
  - 分页读取 messages。
  - 创建和更新 task。
  - 创建、标记和读取 event。
  - 写入、读取和删除 Eino checkpoint。
- 单元测试覆盖 runtime service 的事务和约束。

### 2.2 不包含

- `/api/agent` HTTP API。
- `/ws/agent` WebSocket。
- 前端 Agent 对话界面。
- Eino Graph 执行。
- HITL 决策卡片。
- Edge registry。
- Storyboard / shot / shot_dependency。
- PSS builder。
- Craftsman / Worker / Reviewer / Composer。
- 调用 M4/M5 production service。

这些能力从 M6.2 开始逐步接入。

## 3. 数据模型

### 3.1 agent_thread

`agent_thread` 表示一个持久 Agent 会话线程。它是通用 runtime 能力。

```sql
CREATE TABLE agent_thread (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    scope_type TEXT NOT NULL DEFAULT 'workspace',
    scope_id UUID,
    runtime_provider TEXT NOT NULL DEFAULT 'eino',
    runtime_agent_name TEXT NOT NULL DEFAULT '',
    current_checkpoint_key TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    summary TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_thread_role_check CHECK (role IN ('producer', 'craftsman', 'reviewer', 'composer')),
    CONSTRAINT agent_thread_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'final_output')),
    CONSTRAINT agent_thread_status_check CHECK (status IN ('active', 'paused', 'archived', 'failed'))
);
```

Indexes:

```sql
CREATE INDEX idx_agent_thread_workspace ON agent_thread(workspace_id, role, status);
CREATE INDEX idx_agent_thread_scope ON agent_thread(workspace_id, scope_type, scope_id);
CREATE UNIQUE INDEX idx_agent_thread_active_producer
    ON agent_thread(workspace_id)
    WHERE role = 'producer' AND scope_type = 'workspace' AND status = 'active';
```

规则：

- Producer 首期每个 Agent Workspace 只有一个 active workspace-scoped thread。
- Craftsman 后续按 `scope_type='shot'` + `scope_id=shot.id` 创建 thread。
- Reviewer 和 Composer 可以按 workspace 或 final output scope 创建 thread。

### 3.2 agent_message

`agent_message` 是对话 UI、Graph resume 和审计的事实源。

```sql
CREATE TABLE agent_message (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID NOT NULL REFERENCES agent_thread(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    role TEXT NOT NULL,
    message_type TEXT NOT NULL DEFAULT 'text',
    content JSONB NOT NULL DEFAULT '{}',
    raw_message JSONB NOT NULL DEFAULT '{}',
    task_id UUID,
    event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_message_role_check CHECK (role IN ('user', 'assistant', 'tool', 'system')),
    CONSTRAINT agent_message_type_check CHECK (message_type IN ('text', 'tool_call', 'tool_result', 'ui_card', 'error', 'status'))
);
```

Indexes:

```sql
CREATE UNIQUE INDEX idx_agent_message_thread_seq ON agent_message(thread_id, seq);
CREATE INDEX idx_agent_message_workspace_created ON agent_message(workspace_id, created_at DESC);
CREATE INDEX idx_agent_message_task ON agent_message(task_id) WHERE task_id IS NOT NULL;
CREATE INDEX idx_agent_message_event ON agent_message(event_id) WHERE event_id IS NOT NULL;
```

规则：

- `seq` 在同一 thread 内严格递增。
- message append 必须在事务中计算下一 `seq`，避免并发重复。
- `content` 是前端和业务可读 payload。
- `raw_message` 保存 Eino 原始 message 或 provider 原始消息，便于后续 resume/debug。

### 3.3 agent_task

`agent_task` 是工程层异步任务，不是新的智能角色。

```sql
CREATE TABLE agent_task (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    role TEXT NOT NULL,
    scope_type TEXT NOT NULL DEFAULT 'workspace',
    scope_id UUID,
    task_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    attempt INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 1,
    input JSONB NOT NULL DEFAULT '{}',
    output JSONB NOT NULL DEFAULT '{}',
    error_code TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    CONSTRAINT agent_task_role_check CHECK (role IN ('producer', 'craftsman', 'reviewer', 'composer', 'worker', 'system')),
    CONSTRAINT agent_task_scope_type_check CHECK (scope_type IN ('workspace', 'shot', 'node', 'job', 'final_output')),
    CONSTRAINT agent_task_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled', 'waiting_for_user')),
    CONSTRAINT agent_task_attempt_check CHECK (attempt >= 0 AND max_attempts >= 1)
);
```

Indexes:

```sql
CREATE INDEX idx_agent_task_workspace_status ON agent_task(workspace_id, status, created_at DESC);
CREATE INDEX idx_agent_task_thread ON agent_task(thread_id, created_at DESC);
CREATE INDEX idx_agent_task_scope ON agent_task(workspace_id, scope_type, scope_id, status);
```

首期允许的 `task_type`：

- `producer_turn`
- `tool_call`
- `decision_resume`

后续阶段扩展：

- `dispatch_craftsman`
- `generate_preview`
- `generate_video`
- `review_version`
- `compose_final`

### 3.4 agent_event

`agent_event` 是前端同步、Graph 唤醒和审计的事件源。

```sql
CREATE TABLE agent_event (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    source_role TEXT NOT NULL DEFAULT 'system',
    target_role TEXT,
    scope JSONB NOT NULL DEFAULT '{}',
    payload JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    handled_at TIMESTAMPTZ,
    CONSTRAINT agent_event_source_role_check CHECK (source_role IN ('user', 'producer', 'craftsman', 'reviewer', 'composer', 'worker', 'system')),
    CONSTRAINT agent_event_status_check CHECK (status IN ('pending', 'handled', 'failed', 'cancelled'))
);
```

Indexes:

```sql
CREATE INDEX idx_agent_event_workspace_status ON agent_event(workspace_id, status, created_at DESC);
CREATE INDEX idx_agent_event_thread ON agent_event(thread_id, created_at DESC);
CREATE INDEX idx_agent_event_task ON agent_event(task_id, created_at DESC);
CREATE INDEX idx_agent_event_type ON agent_event(workspace_id, event_type, created_at DESC);
```

首期事件类型：

- `message_created`
- `producer_turn_started`
- `producer_turn_completed`
- `producer_turn_failed`
- `tool_started`
- `tool_succeeded`
- `tool_failed`
- `decision_requested`
- `decision_resolved`

M6.1 只要求 event 可写、可读、可标记 handled；不要求 WebSocket 推送。

### 3.5 eino_checkpoint

`eino_checkpoint` 保存 Eino Graph interrupt/resume 状态。M6.1 只建持久化能力，M6.4 接入真实 HITL resume。

```sql
CREATE TABLE eino_checkpoint (
    key TEXT PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    value BYTEA NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Indexes:

```sql
CREATE INDEX idx_eino_checkpoint_workspace ON eino_checkpoint(workspace_id, updated_at DESC);
CREATE INDEX idx_eino_checkpoint_thread ON eino_checkpoint(thread_id, updated_at DESC);
CREATE INDEX idx_eino_checkpoint_task ON eino_checkpoint(task_id, updated_at DESC);
```

规则：

- `key` 由 Eino checkpoint store 或 ClipAnvil runtime 生成。
- 同 key 写入应 upsert 并更新 `updated_at`。
- 删除 checkpoint 不删除 thread/message/task/event。

## 4. Query And Service Contract

### 4.1 sqlc query files

M6.1 新增：

- `apps/server/sqlc/queries/agent_thread.sql`
- `apps/server/sqlc/queries/agent_message.sql`
- `apps/server/sqlc/queries/agent_task.sql`
- `apps/server/sqlc/queries/agent_event.sql`
- `apps/server/sqlc/queries/eino_checkpoint.sql`

### 4.2 Required query methods

Thread:

- `CreateAgentThread`
- `GetAgentThreadByID`
- `GetActiveProducerThreadByWorkspace`
- `ListAgentThreadsByWorkspace`
- `UpdateAgentThreadCheckpoint`
- `UpdateAgentThreadStatus`

Message:

- `NextAgentMessageSeq`
- `CreateAgentMessage`
- `ListAgentMessagesByThread`
- `ListAgentMessagesByThreadAfterSeq`

Task:

- `CreateAgentTask`
- `GetAgentTaskByID`
- `MarkAgentTaskRunning`
- `MarkAgentTaskSucceeded`
- `MarkAgentTaskFailed`
- `MarkAgentTaskWaitingForUser`
- `ListAgentTasksByWorkspace`

Event:

- `CreateAgentEvent`
- `GetAgentEventByID`
- `MarkAgentEventHandled`
- `MarkAgentEventFailed`
- `ListAgentEventsByWorkspace`
- `ListPendingAgentEventsByWorkspace`

Checkpoint:

- `UpsertEinoCheckpoint`
- `GetEinoCheckpoint`
- `DeleteEinoCheckpoint`
- `ListEinoCheckpointsByThread`

### 4.3 Runtime service

M6.1 should introduce a focused service package:

```text
apps/server/internal/agent/runtime/
```

Responsibilities:

- Normalize runtime writes.
- Enforce workspace mode where the operation requires Agent Workspace.
- Append messages transactionally.
- Keep `seq` generation out of API handlers and Graph nodes.
- Offer simple methods that later Graph/API code can call.

Suggested public methods:

```go
type Service struct {
    pool *pgxpool.Pool
    queries *db.Queries
}

func (s *Service) GetOrCreateProducerThread(ctx context.Context, workspaceID pgtype.UUID) (db.AgentThread, error)
func (s *Service) CreateThread(ctx context.Context, input CreateThreadInput) (db.AgentThread, error)
func (s *Service) AppendMessage(ctx context.Context, input AppendMessageInput) (db.AgentMessage, error)
func (s *Service) ListMessages(ctx context.Context, threadID pgtype.UUID, afterSeq int64, limit int32) ([]db.AgentMessage, error)
func (s *Service) CreateTask(ctx context.Context, input CreateTaskInput) (db.AgentTask, error)
func (s *Service) MarkTaskRunning(ctx context.Context, taskID pgtype.UUID) (db.AgentTask, error)
func (s *Service) MarkTaskSucceeded(ctx context.Context, taskID pgtype.UUID, output []byte) (db.AgentTask, error)
func (s *Service) MarkTaskFailed(ctx context.Context, taskID pgtype.UUID, code string, message string) (db.AgentTask, error)
func (s *Service) CreateEvent(ctx context.Context, input CreateEventInput) (db.AgentEvent, error)
func (s *Service) MarkEventHandled(ctx context.Context, eventID pgtype.UUID) (db.AgentEvent, error)
func (s *Service) UpsertCheckpoint(ctx context.Context, input CheckpointInput) (db.EinoCheckpoint, error)
func (s *Service) GetCheckpoint(ctx context.Context, key string) (db.EinoCheckpoint, error)
func (s *Service) DeleteCheckpoint(ctx context.Context, key string) error
```

M6.1 does not need HTTP handlers. Handlers start in M6.2.

## 5. 可交付标准

M6.1 完成时必须交付：

1. **Migration**
   - 新增 `015_m6_agent_runtime.sql`。
   - `goose up` 可成功执行。
   - `goose down` 可完整回滚新增表和索引。

2. **sqlc**
   - 新增 5 个 query 文件。
   - `make sqlc-generate` 后生成 db models/query methods。
   - 生成代码不需要手工修改。

3. **Runtime service**
   - 新增 `apps/server/internal/agent/runtime`。
   - 提供 thread/message/task/event/checkpoint 最小方法。
   - message append 使用事务保证同一 thread 内 `seq` 递增且唯一。

4. **Tests**
   - 覆盖 Producer thread get-or-create。
   - 覆盖 generic role scoped thread 创建。
   - 覆盖 message append seq。
   - 覆盖 task status transitions。
   - 覆盖 event create/handled/failed。
   - 覆盖 checkpoint upsert/get/delete。

5. **No behavior regression**
   - M3 Agent Workspace 普通画布写接口继续 403。
   - M5 Studio production tests 不应因新增 schema 失败。

## 6. 验收测试标准

### 6.1 Migration acceptance

场景：

1. 启动数据库中间件。
2. 执行 `make migrate-up`。
3. 查询 schema 中存在：
   - `agent_thread`
   - `agent_message`
   - `agent_task`
   - `agent_event`
   - `eino_checkpoint`

通过标准：

- migration 无错误。
- 所有表和索引存在。
- 外键指向 `workspace`、`agent_thread`、`agent_task`。

### 6.2 Producer thread acceptance

场景：

1. 创建 Agent Workspace。
2. 调用 runtime service `GetOrCreateProducerThread`。
3. 再次调用同一方法。

通过标准：

- 两次返回同一个 active Producer thread。
- `role='producer'`。
- `scope_type='workspace'`。
- `workspace_id` 正确。

### 6.3 Generic thread acceptance

场景：

1. 创建 Agent Workspace。
2. 调用 runtime service 创建 `role='craftsman'`、`scope_type='shot'`、`scope_id=<uuid>` thread。

通过标准：

- thread 创建成功。
- role/scope 字段按输入保存。
- 不影响 Producer thread 唯一约束。

### 6.4 Message seq acceptance

场景：

1. 创建 thread。
2. 连续追加 3 条 message。
3. 查询 thread messages。

通过标准：

- `seq` 为 `1, 2, 3`。
- 查询按 `seq ASC` 返回。
- `content` 和 `raw_message` JSON 按输入保存。
- `(thread_id, seq)` 唯一。

### 6.5 Task status acceptance

场景：

1. 创建 `producer_turn` task。
2. 标记 running。
3. 标记 succeeded。

通过标准：

- 初始 status 为 `queued`。
- running 后 `started_at` 非空。
- succeeded 后 `completed_at` 非空，`output` 保存成功 payload。

失败场景：

1. 创建 task。
2. 标记 failed。

通过标准：

- status 为 `failed`。
- `error_code` 和 `error_message` 非空。
- `completed_at` 非空。

### 6.6 Event acceptance

场景：

1. 创建 `message_created` event。
2. 标记 handled。

通过标准：

- 初始 status 为 `pending`。
- handled 后 status 为 `handled`。
- `handled_at` 非空。

失败场景：

1. 创建 `tool_failed` event。
2. 标记 failed。

通过标准：

- status 为 `failed`。
- payload 保留失败上下文。

### 6.7 Checkpoint acceptance

场景：

1. Upsert checkpoint key `test-key`。
2. Get checkpoint。
3. 用同 key upsert 新 value。
4. Get checkpoint。
5. Delete checkpoint。
6. Get checkpoint。

通过标准：

- 第一次 get 返回原 value。
- 第二次 get 返回新 value。
- `updated_at` 更新。
- delete 后 get 返回 not found。

### 6.8 Regression acceptance

场景：

- 运行现有 server tests。

通过标准：

- 现有 workspace mode guard 测试仍通过。
- 现有 production service tests 仍通过。
- 新增表不影响 M4/M5 query generation。

## 7. 严格测试验收命令

M6.1 实现完成后必须按顺序运行：

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
git diff --check
```

如果修改了前端文件或共享 TypeScript 类型，额外运行：

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

M6.1 正常不应修改前端；如果出现前端修改，需要在 PR/最终说明中解释原因。

## 8. 实施备注

- `agent_thread`、`agent_message`、`agent_task`、`agent_event`、`eino_checkpoint` 的表名不要加 Producer 前缀。
- `thread` 和 `message` 是通用 runtime 概念，不要在命名上绑定 Producer。
- M6.1 service 不应调用 Eino、production service、CanvasHub 或 WebSocket。
- M6.1 不开放 HTTP API，避免在 runtime schema 未稳定前绑定外部接口。
- Agent Workspace 的普通写接口仍由现有 mode guard 保护；M6.1 不改这条规则。
