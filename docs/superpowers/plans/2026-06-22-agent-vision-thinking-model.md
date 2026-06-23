# Agent Vision Thinking Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `doubao-seed-2-0-pro-260215` as a selectable Agent Producer model with image input, text output, four thinking-depth levels, streamed thinking display, and real image attachment input.

**Architecture:** Keep `model_capability` as the model catalog and `workspace.settings.agent.model_selection.producer` as the workspace runtime configuration. Extend Producer model selection to carry `reasoning_effort`, use Eino Ark `ReasoningEffort` / `MaxCompletionTokens`, emit separate thinking stream deltas, and build multimodal user messages from Agent image attachments. Frontend stores the thinking selector through the existing model-selection API and renders a collapsible shimmer thinking block tied to task streaming.

**Tech Stack:** Go 1.26, sqlc/goose, Eino Ark ChatModel, Hertz API, React 19, TanStack Query, WebSocket.

---

## Files

- Create migration: `apps/server/migrations/016_agent_vision_thinking_model.sql`
- Modify backend model selection: `apps/server/internal/agent/modelselection/service.go`, `service_test.go`
- Modify API DTOs: `apps/server/internal/api/agent_response.go`, `agent_handler_test.go`
- Modify Producer runtime: `apps/server/internal/agent/producer/types.go`, `context_loader.go`, `model_responder.go`, `model_responder_test.go`, `executor.go`, `executor_test.go`
- Modify runtime interfaces if needed: `apps/server/internal/agent/runtime/service.go`
- Modify frontend API/types: `apps/web/src/lib/agentApi.ts`
- Add frontend helpers/tests: `apps/web/src/lib/agentThinking.ts`, `apps/web/src/lib/agentThinking.test.mjs`, update `apps/web/tsconfig.test.json`, `apps/web/package.json`
- Modify frontend UI: `apps/web/src/pages/AgentWorkspacePage.tsx`, `apps/web/src/main.css`

## Task 1: Seed Vision Thinking Model Capability

- [ ] Add `apps/server/migrations/016_agent_vision_thinking_model.sql` with an upsert for `volcengine/doubao-seed-2-0-pro-260215`.
- [ ] The row must set `output_types=["text"]`, `supported_operations=["text_generation"]`, `supported_input_node_types=["text","image"]`, and `enabled=true`.
- [ ] Store thinking metadata in JSON:
  - `defaults.reasoning_effort="minimal"`
  - `defaults.max_completion_tokens=4096`
  - `limits.reasoning_efforts=["minimal","low","medium","high"]`
  - `limits.max_input_images=8`
- [ ] Down migration deletes only this new row.
- [ ] Verify: `make migrate-up`, `make sqlc-generate`, `git diff --check`.

## Task 2: Persist Thinking Effort In Agent Model Selection

- [ ] Add failing tests in `apps/server/internal/agent/modelselection/service_test.go`:
  - `ApplyToWorkspaceSettings` preserves existing settings and writes `producer.reasoning_effort`.
  - unsupported effort is rejected when model `limits.reasoning_efforts` omits it.
  - empty effort defaults to capability `defaults.reasoning_effort`.
- [ ] Extend `modelselection.ModelRef` with `ReasoningEffort string`.
- [ ] Extend `modelselection.Option` with `SupportsThinking bool`, `ReasoningEfforts []string`, and `DefaultReasoningEffort string`.
- [ ] `Resolve` should default missing `reasoning_effort` from selected option defaults.
- [ ] Validate only `minimal|low|medium|high` values exposed by the selected capability.
- [ ] Verify focused tests: `GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/modelselection -count=1`.

## Task 3: Expose Thinking Effort Through API

- [ ] Add failing API DTO tests in `apps/server/internal/api/agent_handler_test.go`.
- [ ] Extend model-selection response:
  - `selection.producer.reasoning_effort`
  - each option has `supports_thinking`, `reasoning_efforts`, `default_reasoning_effort`.
- [ ] Extend PUT request so frontend can submit `producer.reasoning_effort`.
- [ ] Keep backward compatibility: missing `reasoning_effort` is accepted and defaulted.
- [ ] Verify: `GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestAgentModelSelection|TestPutAgentModelSelection' -count=1`.

## Task 4: Producer Reasoning And Multimodal Input

- [ ] Add failing Producer tests:
  - selected `ReasoningEffort=high` sets Ark `ReasoningEffortHigh`.
  - `ReasoningEffort=minimal` disables thinking and uses `MaxCompletionTokens` instead of `MaxTokens` for thinking-capable models.
  - stream chunks with `ReasoningContent` emit `ProducerStreamDelta{Kind:"thinking"}`.
  - image attachments produce a user message with `UserInputMultiContent` containing text and image URL parts.
- [ ] Extend `ProducerStreamDelta` with `Kind string`; default content deltas use `Kind:"content"`.
- [ ] Extend `ProducerModelSelection` with `ReasoningEffort`, `SupportsThinking`, and `MaxCompletionTokens`.
- [ ] Load these fields from `modelselection.Option`.
- [ ] Resolve image attachment URLs from `agent_message.content.attachments[].asset_id/node_id/kind` by loading matching media assets. Use `storage_url` for remote image URL; if no URL exists, fall back to text-only attachment summary.
- [ ] In `VolcengineModelResponder`, set `config.ReasoningEffort` for thinking-capable models and set `MaxCompletionTokens`; do not set `MaxTokens` at the same time.
- [ ] Emit thinking deltas for `chunk.ReasoningContent`, append final `reasoning_content` to output metadata, and keep final assistant text from `final.Content`.
- [ ] Verify: `GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -count=1`.

## Task 5: Frontend Thinking Selector And Stream Rendering

- [ ] Add failing frontend helper tests for:
  - mapping `minimal/low/medium/high` to labels `关闭/低/中/高`
  - determining when a model supports thinking
  - merging content and thinking stream deltas separately.
- [ ] Extend `AgentSocketEvent` handling for `agent.message.delta` with `kind`.
- [ ] Add selector next to model selector:
  - disabled when selected model does not support thinking or Agent is busy
  - options: 关闭、低、中、高
  - submits via existing `putAgentModelSelection`.
- [ ] Render streamed thinking in a collapsible block:
  - while streaming, expanded by default with shimmer text styling matching `ClipAnvil 正在思考`
  - after assistant content arrives, collapsed by default
  - can expand/collapse manually.
- [ ] Persist final thinking content from assistant `raw_message.reasoning_content` and render collapsed below the assistant message.
- [ ] Verify: `pnpm --filter @clip-anvil/web test:connections`, `pnpm --filter @clip-anvil/web... build`.

## Task 6: Full Verification And E2E

- [ ] Run:
  - `make server-build`
  - `make server-test`
  - `make server-lint`
  - `pnpm --filter @clip-anvil/web lint`
  - `pnpm --filter @clip-anvil/web test:connections`
  - `pnpm --filter @clip-anvil/web... build`
  - `git diff --check`
- [ ] Start app with `./scripts/dev-start.sh`.
- [ ] Use the script-provided Vite URL.
- [ ] Browser E2E:
  - login/create or seed an Agent workspace
  - verify new model appears in Agent model selector
  - select `Doubao Seed 2.0 Pro`
  - switch thinking among 关闭/低/中/高
  - upload an image attachment and send a message
  - verify thinking stream appears, shimmers, then collapses after final answer
  - verify console has no errors
- [ ] Stop app with `CLIPANVIL_DEV_NAME=<profile> ./scripts/dev-stop.sh`.
