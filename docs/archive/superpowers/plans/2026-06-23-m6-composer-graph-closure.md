# M6 Composer Graph Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the M6.8 architecture gap by making Composer a first-class Eino Graph scheduled by Producer through `compose_final`.

**Architecture:** Keep `compose_final` as the Producer-facing tool and keep `composer.Executor` as the durable task runner. Add `composer.Graph` as the execution body for `composer_turn`, with explicit Eino nodes for context loading, final node creation, composition submission, and checkpoint/event persistence. Reuse the existing production service, `internal_ffmpeg`, sandbox job, node/version/job, and Agent canvas websocket paths.

**Tech Stack:** Go 1.26, CloudWeGo Eino Graph, existing Agent runtime tables, existing M4/M5 production service, existing browser E2E smoke path.

---

## Tasks

- [ ] Add failing ComposerGraph tests proving `composer_turn` goes through a graph and writes a checkpoint.
- [ ] Implement `composer.Graph` and move final composition submission from executor internals into graph nodes.
- [ ] Update `composer.Executor` to accept a `Runner`, mirroring Craftsman and Reviewer executors.
- [ ] Wire `agentcomposer.NewGraph` in `apps/server/cmd/server/main.go`.
- [ ] Run targeted Go tests, server build/test/lint, web build/lint, and `git diff --check`.
- [ ] Start the dev environment, run a real browser Agent conversation, inspect the UX/canvas, and save screenshots.

## Acceptance

- `apps/server/internal/agent/composer` contains `graph.go` with a compiled Eino graph named `composer_final`.
- `main.go` creates `agentcomposer.NewGraph(...)` and passes it into `agentcomposer.NewExecutor(...)`.
- A `composer_turn` task produces `composition_submitted` event and an `eino_checkpoint` with kind `composer_result`.
- Browser E2E screenshots show the Agent chat, production status/timeline, and canvas final-production state.
