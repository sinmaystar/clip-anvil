# M6.4 Agent Tool HITL Production Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the M6.4 Agent execution foundation: configurable Agent model selection, Tool Registry, first Producer tools, Eino CheckPointStore-backed HITL interrupt/resume, decision cards, and strict end-to-end verification.

**Architecture:** Keep Eino Graph as the top-level Producer orchestration layer. Persist every model choice, tool call, tool result, HITL card, event, task, and checkpoint through existing Agent runtime tables; use `workspace.settings` for Agent model selection and reuse existing M4/M5 canvas/asset/production primitives through internal tool services.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc/goose, Eino Graph, Eino CheckPointStore, PostgreSQL JSONB, React 19, Vite 8, TanStack Query, WebSocket, existing ClipAnvil Agent runtime and production services.

---

## Source Specs

- `docs/superpowers/specs/2026-06-21-m6-4-agent-tool-hitl-production-bridge-design.md`
- `docs/superpowers/specs/2026-06-21-m6-multiagent-agent-mode-design.md`
- `docs/milestones/m3-m6-studio-agent-roadmap.md`

## File Structure

Backend configuration and settings:

- Modify `apps/server/internal/config/config.go`: add `AgentConfig` with `ProducerMaxToolCalls` and `ToolTimeoutSeconds`.
- Modify `apps/server/internal/config/config_test.go`: env binding and defaults tests.
- Modify `apps/server/config.yaml`: add local defaults for Agent execution limits.
- Modify `apps/server/sqlc/queries/workspace.sql`: add `UpdateWorkspaceSettings`.
- Regenerate `apps/server/internal/store/db/workspace.sql.go`.

Backend model selection:

- Create `apps/server/internal/agent/modelselection/service.go`: read/write `workspace.settings.agent.model_selection`, filter Agent text/chat model options, resolve defaults.
- Create `apps/server/internal/agent/modelselection/service_test.go`: unit coverage for defaults, JSON merge, validation, disabled model rejection.
- Modify `apps/server/internal/api/agent_handler.go`: add `GetModelSelection` and `PutModelSelection`.
- Modify `apps/server/internal/api/agent_response.go`: add model selection response DTOs.
- Modify `apps/server/internal/api/agent_handler_test.go`: API coverage for auth/mode/capability validation.
- Modify `apps/server/cmd/server/main.go`: wire model selection service and routes.

Backend Producer runtime:

- Modify `apps/server/internal/agent/producer/types.go`: add model selection fields and tool loop config.
- Modify `apps/server/internal/agent/producer/context_loader.go`: load complete current conversation window and model selection.
- Modify `apps/server/internal/agent/producer/model_responder.go`: instantiate Volcengine model per turn using selected model.
- Modify `apps/server/internal/agent/producer/executor.go`: persist actual model metadata in assistant `raw_message` and task output.
- Modify `apps/server/internal/agent/producer/*_test.go`: model selection, disabled model failure, metadata assertions.

Backend tools and HITL:

- Create `apps/server/internal/agent/tools/registry.go`: tool definition registry and schema validation.
- Create `apps/server/internal/agent/tools/context.go`: `read_workspace_context`.
- Create `apps/server/internal/agent/tools/text_node.go`: `create_agent_text_node`.
- Create `apps/server/internal/agent/tools/canvas_nodes.go`: conditional `list_canvas_nodes` implementation, only execute Task 5 if Task 4 acceptance cannot be met through `read_workspace_context`.
- Create `apps/server/internal/agent/tools/decision.go`: `request_user_decision`.
- Create `apps/server/internal/agent/tools/executor.go`: tool call task/message/event persistence wrapper.
- Create `apps/server/internal/agent/hitl/checkpoint_store.go`: Eino CheckPointStore adapter backed by `eino_checkpoint`.
- Create `apps/server/internal/agent/hitl/service.go`: decision request/respond/resume helpers.
- Modify `apps/server/internal/agent/producer/graph.go`: add tool routing, bounded tool loop, HITL interrupt branch, resume entry.
- Modify `apps/server/internal/agent/runtime/service.go`: helper methods for checkpoint, tool task, decision event, event lookup.
- Modify `apps/server/sqlc/queries/agent_event.sql`: query decision event by id and workspace.
- Modify `apps/server/sqlc/queries/agent_task.sql`: add active Agent task query for send/selection blocking.
- Modify `apps/server/sqlc/queries/eino_checkpoint.sql`: add checkpoint queries used by the Eino CheckPointStore adapter.
- Modify `apps/server/internal/api/agent_handler.go`: add decision respond endpoint.
- Modify `apps/server/cmd/server/main.go`: wire tool registry, checkpoint store, HITL service, tool execution config.

Frontend:

- Modify `apps/web/src/lib/agentApi.ts`: add model selection, decision response, tool message/card types.
- Create `apps/web/src/lib/agentDecision.ts`: parse pending/resolved decision cards and merge state.
- Create `apps/web/src/lib/agentDecision.test.mjs`: decision parsing and merge tests.
- Create `apps/web/src/lib/agentModelSelection.ts`: filter/format model options and build save payloads.
- Create `apps/web/src/lib/agentModelSelection.test.mjs`: option filtering and payload tests.
- Modify `apps/web/src/lib/agentTasks.ts`: detect Agent busy states including `waiting_for_user`.
- Modify `apps/web/src/lib/agentTasks.test.mjs`: busy state tests.
- Modify `apps/web/src/pages/AgentWorkspacePage.tsx`: render model selector, tool/status messages, decision cards, disabled composer states.
- Modify `apps/web/src/main.css`: model selector, decision card, tool/status message styles.
- Modify `apps/web/package.json` and `apps/web/tsconfig.test.json`: include new tests.

Verification:

- Use existing `./scripts/dev-start.sh` and script-provided Vite URL for browser E2E.
- Run `git diff --check` after each task and full verification at the end.

---

## Task 1: Agent Execution Config And Workspace Settings Foundation

**Files:**

- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/internal/config/config_test.go`
- Modify: `apps/server/config.yaml`
- Modify: `apps/server/sqlc/queries/workspace.sql`
- Generated: `apps/server/internal/store/db/workspace.sql.go`

- [ ] Add failing config tests in `apps/server/internal/config/config_test.go`:

```go
func TestLoadBindsAgentExecutionConfig(t *testing.T) {
	t.Setenv("CLIPANVIL_AGENT_PRODUCER_MAX_TOOL_CALLS", "77")
	t.Setenv("CLIPANVIL_AGENT_TOOL_TIMEOUT_SECONDS", "321")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Agent.ProducerMaxToolCalls != 77 {
		t.Fatalf("ProducerMaxToolCalls = %d, want 77", cfg.Agent.ProducerMaxToolCalls)
	}
	if cfg.Agent.ToolTimeoutSeconds != 321 {
		t.Fatalf("ToolTimeoutSeconds = %d, want 321", cfg.Agent.ToolTimeoutSeconds)
	}
}

func TestLoadDefaultsAgentExecutionConfig(t *testing.T) {
	t.Setenv("CLIPANVIL_AGENT_PRODUCER_MAX_TOOL_CALLS", "")
	t.Setenv("CLIPANVIL_AGENT_TOOL_TIMEOUT_SECONDS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Agent.ProducerMaxToolCalls != 50 {
		t.Fatalf("ProducerMaxToolCalls = %d, want 50", cfg.Agent.ProducerMaxToolCalls)
	}
	if cfg.Agent.ToolTimeoutSeconds != 300 {
		t.Fatalf("ToolTimeoutSeconds = %d, want 300", cfg.Agent.ToolTimeoutSeconds)
	}
}
```

- [ ] Run failing config tests:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/config -run 'TestLoad(BindsAgentExecutionConfig|DefaultsAgentExecutionConfig)' -count=1
```

Expected: FAIL because `Config.Agent` does not exist.

- [ ] Implement `AgentConfig` in `apps/server/internal/config/config.go`:

```go
type Config struct {
	Server     ServerConfig
	Postgres   PostgresConfig
	Redis      RedisConfig
	MinIO      MinIOConfig
	JWT        JWTConfig
	Sandbox    SandboxConfig
	Production ProductionConfig
	Agent      AgentConfig
}

type AgentConfig struct {
	ProducerMaxToolCalls int `mapstructure:"producer_max_tool_calls"`
	ToolTimeoutSeconds   int `mapstructure:"tool_timeout_seconds"`
}
```

Update `bindEnv`:

```go
"agent.producer_max_tool_calls",
"agent.tool_timeout_seconds",
```

After `v.Unmarshal(&cfg)`, normalize:

```go
if cfg.Agent.ProducerMaxToolCalls <= 0 {
	cfg.Agent.ProducerMaxToolCalls = 50
}
if cfg.Agent.ToolTimeoutSeconds <= 0 {
	cfg.Agent.ToolTimeoutSeconds = 300
}
```

- [ ] Add local defaults to `apps/server/config.yaml`:

```yaml
agent:
  producer_max_tool_calls: 50
  tool_timeout_seconds: 300
```

- [ ] Add workspace settings update query to `apps/server/sqlc/queries/workspace.sql`:

```sql
-- name: UpdateWorkspaceSettings :one
UPDATE workspace
SET settings = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;
```

- [ ] Regenerate sqlc:

```bash
make sqlc-generate
```

Expected: PASS and `apps/server/internal/store/db/workspace.sql.go` contains `UpdateWorkspaceSettings`.

- [ ] Run focused verification:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/config -run 'TestLoad(BindsAgentExecutionConfig|DefaultsAgentExecutionConfig)' -count=1
git diff --check
```

Expected: PASS.

---

## Task 2: Agent Model Selection Service And API

**Files:**

- Create: `apps/server/internal/agent/modelselection/service.go`
- Create: `apps/server/internal/agent/modelselection/service_test.go`
- Modify: `apps/server/internal/api/agent_response.go`
- Modify: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/server/internal/api/agent_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] Create failing service tests in `apps/server/internal/agent/modelselection/service_test.go`:

```go
func TestSelectionDefaultsToConfiguredProducerModel(t *testing.T) {
	service := NewService(fakeQueriesWithCapabilities(t, []db.ModelCapability{
		textCapability("volcengine", "doubao-mini", "Doubao Mini", true),
	}), Defaults{ProducerProviderID: "volcengine", ProducerModelID: "doubao-mini"})

	result, err := service.Resolve(context.Background(), db.Workspace{Settings: []byte(`{}`)})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Selection.Producer.ModelID != "doubao-mini" {
		t.Fatalf("model = %q, want doubao-mini", result.Selection.Producer.ModelID)
	}
}

func TestSelectionRejectsImageOnlyModel(t *testing.T) {
	service := NewService(fakeQueriesWithCapabilities(t, []db.ModelCapability{
		imageCapability("volcengine", "seedream", "Seedream", true),
	}), Defaults{ProducerProviderID: "volcengine", ProducerModelID: "seedream"})

	_, err := service.ValidateProducerModel(context.Background(), ModelRef{
		ProviderID: "volcengine",
		ModelID:    "seedream",
	})
	if !errors.Is(err, ErrUnsupportedProducerModel) {
		t.Fatalf("error = %v, want ErrUnsupportedProducerModel", err)
	}
}
```

Expected helper behavior:

- `textCapability` returns `output_types=["text"]` and `supported_operations=["text_generation"]`.
- `imageCapability` returns `output_types=["image"]` and `supported_operations=["text_to_image"]`.

- [ ] Run failing service tests:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/modelselection -count=1
```

Expected: FAIL because package does not exist.

- [ ] Implement `apps/server/internal/agent/modelselection/service.go` with these exported types:

```go
var (
	ErrUnsupportedProducerModel = errors.New("unsupported producer model")
	ErrInvalidSelection         = errors.New("invalid agent model selection")
)

type ModelRef struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

type Selection struct {
	Producer ModelRef `json:"producer"`
}

type Option struct {
	ProviderID  string         `json:"provider_id"`
	ModelID     string         `json:"model_id"`
	DisplayName string         `json:"display_name"`
	Limits      map[string]any `json:"limits"`
	Pricing     map[string]any `json:"pricing"`
}

type Defaults struct {
	ProducerProviderID string
	ProducerModelID    string
}

type Resolved struct {
	Selection Selection `json:"selection"`
	Defaults  Selection `json:"defaults"`
	Options   []Option  `json:"options"`
}
```

Implementation rules:

- `Resolve(ctx, workspace)` reads `workspace.Settings`.
- Empty settings use `Defaults`.
- `ValidateProducerModel` accepts only enabled capabilities with `output_types` containing `text` and `supported_operations` containing `text_generation`.
- `ApplyToWorkspaceSettings(rawSettings, selection)` deep-merges only `agent.model_selection.producer`.

- [ ] Add API DTOs to `apps/server/internal/api/agent_response.go`:

```go
type agentModelRefResponse struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

type agentModelSelectionResponse struct {
	Producer agentModelRefResponse `json:"producer"`
}

type agentModelOptionResponse struct {
	ProviderID  string         `json:"provider_id"`
	ModelID     string         `json:"model_id"`
	DisplayName string         `json:"display_name"`
	Limits      map[string]any `json:"limits"`
	Pricing     map[string]any `json:"pricing"`
}

type getAgentModelSelectionResponse struct {
	Selection agentModelSelectionResponse `json:"selection"`
	Defaults  agentModelSelectionResponse `json:"defaults"`
	Options   []agentModelOptionResponse  `json:"options"`
}

type putAgentModelSelectionRequest struct {
	Producer agentModelRefResponse `json:"producer"`
}
```

- [ ] Add handler methods to `apps/server/internal/api/agent_handler.go`:

```go
func (h *AgentHandler) GetModelSelection(ctx context.Context, c *app.RequestContext)
func (h *AgentHandler) PutModelSelection(ctx context.Context, c *app.RequestContext)
```

Behavior:

- Both use `agentWorkspaceForRequest`.
- `GET` returns resolved selection/default/options.
- `PUT` validates producer model, merges `workspace.settings`, persists with `UpdateWorkspaceSettings`, and returns resolved response.

- [ ] Wire routes in `apps/server/cmd/server/main.go`:

```go
h.GET("/api/agent/workspaces/:workspaceID/model-selection", authMiddleware, agentHandler.GetModelSelection)
h.PUT("/api/agent/workspaces/:workspaceID/model-selection", authMiddleware, agentHandler.PutModelSelection)
```

- [ ] Add API tests to `apps/server/internal/api/agent_handler_test.go`:

Test names:

```go
func TestAgentModelSelectionRejectsStudioWorkspace(t *testing.T)
func TestAgentModelSelectionRejectsUnsupportedModel(t *testing.T)
func TestAgentModelSelectionPersistsProducerModel(t *testing.T)
```

Expected assertions:

- Studio workspace returns `403`.
- Image/video-only model returns `400`.
- Valid text model persists under `settings.agent.model_selection.producer`.

- [ ] Run focused verification:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/modelselection ./internal/api -run 'TestAgentModelSelection|TestSelection' -count=1
git diff --check
```

Expected: PASS.

---

## Task 3: ProducerGraph Uses Workspace Model Selection

**Files:**

- Modify: `apps/server/internal/agent/producer/types.go`
- Modify: `apps/server/internal/agent/producer/context_loader.go`
- Modify: `apps/server/internal/agent/producer/model_responder.go`
- Modify: `apps/server/internal/agent/producer/executor.go`
- Modify: `apps/server/internal/agent/producer/model_responder_test.go`
- Modify: `apps/server/internal/agent/producer/executor_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] Add failing tests:

```go
func TestVolcengineModelResponderUsesSelectedModel(t *testing.T)
func TestExecutorPersistsSelectedModelMetadata(t *testing.T)
func TestProducerTurnFailsWhenSelectedModelUnavailable(t *testing.T)
```

Expected:

- Responder factory receives selected `Model`.
- Assistant raw metadata contains `provider`, `model_id`, and `model_display_name`.
- Disabled/unavailable model returns error code `agent_model_unavailable`.

- [ ] Extend producer types:

```go
type ProducerModelSelection struct {
	ProviderID   string
	ModelID      string
	DisplayName  string
}

type ProducerTurnInput struct {
	WorkspaceID      pgtype.UUID
	ThreadID         pgtype.UUID
	TaskID           pgtype.UUID
	TriggerMessageID pgtype.UUID
	EmitDelta        ProducerDeltaHandler
	MaxToolCalls     int
	ToolTimeout      time.Duration
}

type ProducerContext struct {
	Input          ProducerTurnInput
	Messages       []db.AgentMessage
	LatestUserText string
	Model          ProducerModelSelection
	EmitDelta      ProducerDeltaHandler
}
```

- [ ] Update `RuntimeContextLoader` to depend on model selection resolver:

```go
type RuntimeContextLoader struct {
	Runtime        *agentruntime.Service
	ModelResolver  ModelResolver
}

type ModelResolver interface {
	ResolveProducerModel(ctx context.Context, workspaceID pgtype.UUID) (ProducerModelSelection, error)
}
```

Load all current messages needed by the turn. M6.4 keeps the existing bounded window if full-history loading is too expensive, but the loader owns that decision; `read_workspace_context` must not read message history.

- [ ] Update `VolcengineModelResponder.Respond`:

Model choice priority:

1. `producerContext.Model.ModelID`.
2. config fallback `cfg.Model`.

If selected provider is not `volcengine`, return `agent_model_unavailable` until other chat providers exist.

- [ ] Update `Executor.RunTask` to pass config:

```go
ProducerTurnInput{
	MaxToolCalls: config.ProducerMaxToolCalls,
	ToolTimeout:  time.Duration(config.ToolTimeoutSeconds) * time.Second,
}
```

Persist output metadata:

```go
metadata["provider"] = output.Metadata["provider"]
metadata["model_id"] = output.Metadata["model_id"]
metadata["model_display_name"] = output.Metadata["model_display_name"]
```

- [ ] Wire in `main.go`:

- Create model selection service with defaults from `cfg.Production.Volcengine.TextModel`.
- Pass resolver into `RuntimeContextLoader`.
- Pass Agent execution config into `Executor`.

- [ ] Run focused verification:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'TestVolcengineModelResponderUsesSelectedModel|TestExecutorPersistsSelectedModelMetadata|TestProducerTurnFailsWhenSelectedModelUnavailable' -count=1
git diff --check
```

Expected: PASS.

---

## Task 4: Tool Registry And Required Producer Tools

**Files:**

- Create: `apps/server/internal/agent/tools/registry.go`
- Create: `apps/server/internal/agent/tools/context.go`
- Create: `apps/server/internal/agent/tools/text_node.go`
- Create: `apps/server/internal/agent/tools/executor.go`
- Create: `apps/server/internal/agent/tools/registry_test.go`
- Create: `apps/server/internal/agent/tools/context_test.go`
- Create: `apps/server/internal/agent/tools/text_node_test.go`
- Modify: `apps/server/internal/agent/runtime/service.go`
- Modify: `apps/server/internal/api/agent_broadcaster.go` if tool events require helper payloads.

- [ ] Write failing registry tests:

```go
func TestRegistryRejectsDuplicateToolNames(t *testing.T)
func TestRegistryDefinitionIncludesSchemaAndDescription(t *testing.T)
func TestRegistryFindsRequiredM6Tools(t *testing.T)
```

Expected required names:

```go
[]string{"read_workspace_context", "create_agent_text_node", "request_user_decision"}
```

- [ ] Implement `registry.go`:

```go
type Definition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Result      map[string]any
	Safety      SafetySpec
	Timeout     time.Duration
	Visibility  VisibilitySpec
}

type SafetySpec struct {
	ReadOnly              bool
	RequiresHITL          bool
	WritesCanvas          bool
	UsesProductionService bool
	MaxCallsPerTurn       int
}

type VisibilitySpec struct {
	ShowCallMessage   bool
	ShowResultMessage bool
	UserLabel         string
}

type Executor interface {
	Definition() Definition
	Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error)
}
```

- [ ] Write failing `read_workspace_context` tests:

```go
func TestReadWorkspaceContextDoesNotReadMessageHistory(t *testing.T)
func TestReadWorkspaceContextReturnsSourceMaterialRefs(t *testing.T)
```

Expected output contains:

```json
{
  "workspace": {"mode": "agent"},
  "source_material_refs": [{"node_id": "...", "asset_id": "...", "type": "image", "title": "商品主图"}],
  "canvas_summary": "当前画布包含"
}
```

- [ ] Implement `context.go`:

- Query workspace facts.
- Query source material nodes and assets.
- Return summary plus `source_material_refs`.
- Do not read `agent_message`.

- [ ] Write failing `create_agent_text_node` tests:

```go
func TestCreateAgentTextNodeRejectsStudioWorkspace(t *testing.T)
func TestCreateAgentTextNodeCreatesAgentSourceMaterial(t *testing.T)
func TestCreateAgentTextNodeBroadcastsCanvasNode(t *testing.T)
```

Expected:

- Studio mode returns forbidden-style tool error.
- Agent mode creates text asset and `source='agent'` text node through existing `CreateAgentMediaNode`.
- Tool result returns `node_id` and `asset_id`.

- [ ] Implement `text_node.go` using existing query/service boundaries:

```go
type CreateAgentTextNodeArgs struct {
	Title     string     `json:"title"`
	Text      string     `json:"text"`
	Placement *Placement `json:"placement,omitempty"`
}
```

Validation:

- title: 1..120 runes.
- text: 1..12000 runes.
- workspace mode must be `agent`.

- [ ] Implement `executor.go` wrapper:

The wrapper must:

1. Append `tool_call` message.
2. Create `agent_task(task_type='tool_call')`.
3. Create `tool_call_started` event.
4. Execute the tool with timeout.
5. Append `tool_result` message.
6. Create completed/failed event.
7. Broadcast message/task/event updates.

- [ ] Run focused verification:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./internal/agent/runtime -count=1
git diff --check
```

Expected: PASS.

---

## Task 5: Conditional `list_canvas_nodes` Detailed Drilldown

**Execute this task only if Task 4 cannot satisfy tool-chain or E2E acceptance with `read_workspace_context.source_material_refs`.**

**Files:**

- Create: `apps/server/internal/agent/tools/canvas_nodes.go`
- Create: `apps/server/internal/agent/tools/canvas_nodes_test.go`

- [ ] Before starting, record the concrete blocker in the implementation notes:

```text
list_canvas_nodes required because: read_workspace_context source_material_refs did not include current version state needed by TestProducerGraphExecutesCreateAgentTextNodeTool.
```

The accepted reasons are:

- `read_workspace_context` cannot keep payload small while returning required node details.
- HITL/tool E2E cannot resolve a user-referenced node from summary refs.
- The first production bridge needs exact current version/job state for a node.

- [ ] Write failing tests:

```go
func TestListCanvasNodesFiltersSourceOnly(t *testing.T)
func TestListCanvasNodesPaginatesResults(t *testing.T)
func TestListCanvasNodesReturnsTextSummaryNotFullContent(t *testing.T)
```

- [ ] Implement `list_canvas_nodes` as a read-only tool:

Parameters:

```json
{
  "node_types": ["text", "image"],
  "source_only": true,
  "limit": 50
}
```

Result:

```json
{
  "nodes": [
    {
      "node_id": "...",
      "asset_id": "...",
      "type": "text",
      "title": "商品 brief",
      "source": "agent",
      "text_summary": "前 200 字摘要"
    }
  ]
}
```

- [ ] Add registry assertion that `list_canvas_nodes` is present only when the package is wired into the registry for this phase.

- [ ] Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'TestListCanvasNodes|TestRegistry' -count=1
git diff --check
```

Expected: PASS.

---

## Task 6: Eino CheckPointStore And HITL Decision Service

**Files:**

- Create: `apps/server/internal/agent/hitl/checkpoint_store.go`
- Create: `apps/server/internal/agent/hitl/checkpoint_store_test.go`
- Create: `apps/server/internal/agent/hitl/service.go`
- Create: `apps/server/internal/agent/hitl/service_test.go`
- Modify: `apps/server/sqlc/queries/agent_event.sql`
- Modify: `apps/server/sqlc/queries/eino_checkpoint.sql`
- Modify: `apps/server/internal/agent/runtime/service.go`

- [ ] Add failing CheckPointStore tests:

```go
func TestCheckpointStorePutGetDelete(t *testing.T)
func TestCheckpointStorePersistsMetadata(t *testing.T)
func TestCheckpointStoreScopesCheckpointToWorkspaceThreadTask(t *testing.T)
```

Expected:

- Put writes `eino_checkpoint.value`.
- Get returns the same bytes.
- Delete removes the row.
- Metadata includes `workspace_id`, `thread_id`, `task_id`, `interrupt_type`.

- [ ] Implement `checkpoint_store.go` as the Eino CheckPointStore adapter backed by `eino_checkpoint`.

Adapter requirements:

- Use `agentruntime.Service.UpsertCheckpoint`.
- Use `GetCheckpoint`.
- Use `DeleteCheckpoint`.
- Preserve metadata JSON.
- Never store only ephemeral in-memory state.

- [ ] Add failing HITL service tests:

```go
func TestRequestUserDecisionCreatesCardEventCheckpointAndWaitingTask(t *testing.T)
func TestRespondDecisionRejectsInvalidOption(t *testing.T)
func TestRespondDecisionCreatesResumeTaskAndResolvedEvent(t *testing.T)
func TestRespondDecisionIsIdempotentForClientResponseID(t *testing.T)
```

- [ ] Implement `service.go`:

Decision request behavior:

1. Create `decision_requested` event.
2. Append assistant `ui_card` message.
3. Store Eino checkpoint through CheckPointStore adapter.
4. Set `agent_thread.current_checkpoint_key`.
5. Mark current `producer_turn` as `waiting_for_user`.

Decision response behavior:

1. Validate pending event and option.
2. Append user decision message.
3. Mark event handled.
4. Create `decision_resolved` event.
5. Create `decision_resume` task.
6. Broadcast events through caller-provided broadcaster.

- [ ] Add required sqlc queries:

`apps/server/sqlc/queries/agent_event.sql`:

```sql
-- name: GetAgentEventForWorkspace :one
SELECT *
FROM agent_event
WHERE id = $1
  AND workspace_id = $2;
```

`apps/server/sqlc/queries/eino_checkpoint.sql` only if current methods do not satisfy adapter metadata tests.

- [ ] Regenerate sqlc:

```bash
make sqlc-generate
```

Expected: PASS.

- [ ] Run focused verification:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/hitl ./internal/agent/runtime -count=1
git diff --check
```

Expected: PASS.

---

## Task 7: ProducerGraph Tool Loop, HITL Interrupt, And Resume

**Files:**

- Modify: `apps/server/internal/agent/producer/graph.go`
- Modify: `apps/server/internal/agent/producer/types.go`
- Modify: `apps/server/internal/agent/producer/executor.go`
- Modify: `apps/server/internal/agent/producer/responder.go`
- Create: `apps/server/internal/agent/producer/tool_parser.go`
- Create: `apps/server/internal/agent/producer/tool_parser_test.go`
- Modify: `apps/server/internal/agent/producer/graph_test.go`
- Modify: `apps/server/internal/agent/producer/executor_test.go`

- [ ] Add failing tool parser tests:

```go
func TestParseToolCallFromJSONFence(t *testing.T)
func TestParseToolCallReturnsTextWhenNoToolCall(t *testing.T)
func TestParseToolCallRejectsUnknownShape(t *testing.T)
```

Supported deterministic test shape:

```json
{
  "tool_call": {
    "name": "create_agent_text_node",
    "arguments": {
      "title": "商品 brief",
      "text": "低糖燕麦拿铁，主打轻负担早餐场景。"
    }
  }
}
```

- [ ] Implement `tool_parser.go`:

- Extract strict JSON from plain response or fenced block.
- Accept exactly one `tool_call`.
- Return normal text if no tool call exists.
- Reject missing `name` or non-object `arguments`.

- [ ] Add failing graph tests:

```go
func TestProducerGraphExecutesCreateAgentTextNodeTool(t *testing.T)
func TestProducerGraphStopsAtRequestUserDecision(t *testing.T)
func TestProducerGraphStopsAtMaxToolCalls(t *testing.T)
func TestProducerGraphResumeContinuesAfterDecision(t *testing.T)
```

- [ ] Update `Responder` to support deterministic tool fixture tests:

```go
type Responder interface {
	Respond(ctx context.Context, producerContext ProducerContext) (ProducerTurnOutput, error)
}
```

Use test responders that return JSON tool calls. Do not rely on live LLM for graph unit tests.

- [ ] Update `Graph`:

New logical nodes:

```text
load_context
-> call_model
-> route_model_output
-> execute_tool
-> continue_or_interrupt
-> finalize_response
```

Behavior:

- If output is text, finalize assistant message.
- If output is `create_agent_text_node`, execute tool and continue model loop.
- If output is `request_user_decision`, call HITL service and return interrupted output.
- Stop with `agent_tool_loop_exhausted` after `MaxToolCalls`.

- [ ] Add `ResumeDecision` to executor:

```go
func (e *Executor) ResumeDecision(ctx context.Context, input ResumeDecisionInput) error
```

Behavior:

- Mark `decision_resume` running.
- Resume graph from CheckPointStore.
- Persist follow-up assistant text.
- Mark `decision_resume` succeeded.
- Broadcast task/message/event.

- [ ] Run focused verification:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'TestParseToolCall|TestProducerGraph|TestExecutor' -count=1
git diff --check
```

Expected: PASS.

---

## Task 8: Agent Decision Respond API And Busy-State Guards

**Files:**

- Modify: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/server/internal/api/agent_response.go`
- Modify: `apps/server/internal/api/agent_handler_test.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] Add failing API tests:

```go
func TestRespondAgentDecisionRejectsNonOwner(t *testing.T)
func TestRespondAgentDecisionRejectsResolvedEvent(t *testing.T)
func TestRespondAgentDecisionCreatesResumeTask(t *testing.T)
func TestPostAgentMessageRejectsWhileAgentBusy(t *testing.T)
func TestPutAgentModelSelectionRejectsWhileAgentBusy(t *testing.T)
```

Expected:

- Pending decision response returns `201`.
- Duplicate non-idempotent response returns `409`.
- `POST /messages` while queued/running/waiting returns `409`.
- `PUT /model-selection` while queued/running/waiting returns `409`.

- [ ] Add response DTO:

```go
type postAgentDecisionResponse struct {
	DecisionEvent agentEventResponse   `json:"decision_event"`
	ResolvedEvent agentEventResponse   `json:"resolved_event"`
	Message       agentMessageResponse `json:"message"`
	Task          agentTaskResponse    `json:"task"`
}
```

- [ ] Add request DTO:

```go
type postAgentDecisionRequest struct {
	SelectedOptionID string `json:"selected_option_id"`
	FreeText         string `json:"free_text"`
	ClientResponseID string `json:"client_response_id"`
}
```

- [ ] Implement handler:

```go
func (h *AgentHandler) RespondDecision(ctx context.Context, c *app.RequestContext)
```

Route:

```go
h.POST("/api/agent/workspaces/:workspaceID/decisions/:eventID/respond", authMiddleware, agentHandler.RespondDecision)
```

- [ ] Implement busy guard helper:

```go
func (h *AgentHandler) rejectIfAgentBusy(ctx context.Context, workspaceID pgtype.UUID, c *app.RequestContext) bool
```

Busy statuses:

```go
queued, running, waiting_for_user
```

Apply to:

- `PostMessage`
- `PutModelSelection`

Do not apply to:

- `RespondDecision`
- `GetModelSelection`
- `ListMessages`

- [ ] Run focused verification:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestRespondAgentDecision|TestPostAgentMessageRejectsWhileAgentBusy|TestPutAgentModelSelectionRejectsWhileAgentBusy' -count=1
git diff --check
```

Expected: PASS.

---

## Task 9: Frontend Model Selector, Decision Cards, Tool Messages

**Files:**

- Modify: `apps/web/src/lib/agentApi.ts`
- Create: `apps/web/src/lib/agentDecision.ts`
- Create: `apps/web/src/lib/agentDecision.test.mjs`
- Create: `apps/web/src/lib/agentModelSelection.ts`
- Create: `apps/web/src/lib/agentModelSelection.test.mjs`
- Modify: `apps/web/src/lib/agentTasks.ts`
- Modify: `apps/web/src/lib/agentTasks.test.mjs`
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/main.css`
- Modify: `apps/web/package.json`
- Modify: `apps/web/tsconfig.test.json`

- [ ] Add frontend API types:

```ts
export interface AgentModelRef {
  provider_id: string;
  model_id: string;
}

export interface AgentModelOption extends AgentModelRef {
  display_name: string;
  limits: Record<string, unknown>;
  pricing: Record<string, unknown>;
}

export interface AgentModelSelectionResponse {
  selection: { producer: AgentModelRef };
  defaults: { producer: AgentModelRef };
  options: AgentModelOption[];
}
```

Functions:

```ts
export function fetchAgentModelSelection(workspaceId: string)
export function putAgentModelSelection(workspaceId: string, producer: AgentModelRef)
export function respondAgentDecision(workspaceId: string, eventId: string, input: AgentDecisionResponseInput)
```

- [ ] Add failing `agentModelSelection.test.mjs`:

```js
it("formats model labels with display name and provider", () => {
  assert.equal(
    formatAgentModelOptionLabel({
      provider_id: "volcengine",
      model_id: "doubao-mini",
      display_name: "Doubao Mini",
      limits: {},
      pricing: {},
    }),
    "Doubao Mini · volcengine",
  );
});
```

- [ ] Implement `agentModelSelection.ts` helpers:

```ts
export function formatAgentModelOptionLabel(option: AgentModelOption) {
  return `${option.display_name || option.model_id} · ${option.provider_id}`;
}

export function agentModelOptionValue(option: AgentModelOption) {
  return `${option.provider_id}:${option.model_id}`;
}
```

- [ ] Add failing `agentDecision.test.mjs`:

```js
it("parses pending decision cards", () => {
  const card = parseAgentDecisionCard({
    card_type: "decision_request",
    decision_id: "event-1",
    title: "确认方向",
    message: "选择风格",
    options: [{ id: "a", label: "方案 A" }],
    allow_free_text: true,
    status: "pending",
  });
  assert.equal(card.decision_id, "event-1");
  assert.equal(card.status, "pending");
});
```

- [ ] Implement `agentDecision.ts`:

- Parse only `card_type === "decision_request"`.
- Require `decision_id`, `title`, and `message`.
- Return typed options.
- Merge resolved status by `decision_id`.

- [ ] Update `agentTasks.ts`:

```ts
export function hasBusyAgentTask(tasks: AgentTaskState[]) {
  return tasks.some(
    (task) =>
      (task.task_type === "producer_turn" ||
        task.task_type === "decision_resume") &&
      (task.status === "queued" ||
        task.status === "running" ||
        task.status === "waiting_for_user"),
  );
}
```

- [ ] Update `AgentWorkspacePage.tsx`:

UI behavior:

- Fetch model selection when `agentEnabled`.
- Render a compact model select in the ClipAnvil panel header or composer toolbar.
- Disable select when `hasBusyAgentTask(tasks)` is true.
- Render `ui_card` decision request messages with option buttons.
- On option click, call `respondAgentDecision`.
- Disable composer while Agent is queued/running/waiting.
- Render `tool_call` and `tool_result` as compact status rows, not large chat bubbles.

- [ ] Add CSS in `apps/web/src/main.css`:

Classes:

```css
.agent-model-select
.agent-decision-card
.agent-decision-options
.agent-tool-status
.agent-composer-disabled-hint
```

- [ ] Update test config:

Add new test files to `apps/web/package.json` `test:connections`.
Add files to `apps/web/tsconfig.test.json`.

- [ ] Run frontend verification:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected: PASS.

---

## Task 10: Full Verification And Browser E2E

**Files:**

- No planned source edits. Use this task to verify the complete M6.4 slice.

- [ ] Run backend verification:

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
make server-lint
```

Expected: PASS.

- [ ] Run frontend verification:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected: PASS.

- [ ] Start the app:

```bash
./scripts/dev-start.sh
```

Expected:

- Script prints the current worktree Vite URL.
- Backend `/api/health` passes.
- Use the printed Vite URL for browser E2E.

- [ ] Browser E2E flow:

1. Register or log in.
2. Create an Agent Workspace.
3. Open the ClipAnvil panel.
4. Change the conversation model through the model selector.
5. Refresh and confirm the model selection persists.
6. Send a normal message and confirm streamed assistant reply.
7. Confirm database assistant metadata records selected provider/model.
8. Send the deterministic fixture phrase that triggers `create_agent_text_node`.
9. Confirm tool status appears in chat.
10. Confirm read-only canvas shows the Agent-created text source node.
11. Confirm ordinary Agent workspace node mutation API still returns `403`.
12. Send the deterministic fixture phrase that triggers `request_user_decision`.
13. Confirm decision card appears and composer is disabled.
14. Open a second same-workspace tab and confirm the card is visible.
15. Select an option in the first tab.
16. Confirm card resolved state syncs to the second tab.
17. Confirm Producer resumes and posts a follow-up assistant message.
18. Refresh and confirm model selection, tool call/result, decision card, user selection, and assistant follow-up all persist.

- [ ] Database spot checks:

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select settings->'agent'->'model_selection' as agent_model_selection from workspace order by updated_at desc limit 5;"
```

Expected: latest Agent workspace has Producer model selection.

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select message_type, role, content from agent_message order by created_at desc limit 10;"
```

Expected: includes `tool_call`, `tool_result`, `ui_card`, user decision response, and assistant follow-up.

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select task_type,status,input,output from agent_task order by created_at desc limit 10;"
```

Expected: includes `producer_turn`, `tool_call`, and `decision_resume`; the HITL producer turn reached `waiting_for_user`.

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select event_type,status,payload from agent_event order by created_at desc limit 10;"
```

Expected: includes `tool_call_started`, `tool_call_completed`, `decision_requested`, and `decision_resolved`.

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres \
  psql -U clipanvil -d clipanvil \
  -c "select key, metadata from eino_checkpoint order by updated_at desc limit 5;"
```

Expected: latest decision checkpoint metadata includes workspace/thread/task references.

- [ ] Stop runtime:

```bash
./scripts/dev-stop.sh
```

Expected: current profile frontend/backend processes are stopped; shared PostgreSQL/Redis/MinIO/NGINX containers remain running.

---

## Execution Notes

- Do not implement Studio / Agent import-export in this plan.
- Do not expose the internal `Producer` name in user-facing UI.
- Keep ordinary user canvas mutation APIs blocked in Agent Workspace.
- Use deterministic responders for automated tool/HITL tests; live Volcengine streaming is only required for local/browser smoke.
- If `list_canvas_nodes` is skipped, record: `M6.4 skipped list_canvas_nodes because read_workspace_context.source_material_refs satisfied all required tests.`
- If `list_canvas_nodes` is implemented, record the exact failing acceptance reason before coding Task 5.
