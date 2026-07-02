# Agent Template-Only Video Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make an explicit Agent instruction such as "不要调用 Seedance，只用 HyperFrames/template video" a hard production policy.

**Architecture:** Add a `video_route_policy` field to Producer dispatch input and Craftsman runtime context. `dispatch_craftsman` uses `template_only` to recommend template routes for every `shot_video`; `upsert_render_plan` rejects Seedance profiles under that policy so Craftsman cannot accidentally burn Seedance cost.

**Tech Stack:** Go 1.26, existing Agent native tools, existing RenderPlan/Worker/production chain.

---

## Task 1: Dispatch Policy

- [x] Add a failing test proving `dispatch_craftsman(video_route_policy=template_only)` recommends `template_video/image_to_template_video` for every shot video, including the first hero shot.
- [x] Run the test and verify it fails because the first shot is still `seedance_2_video`.
- [x] Add `VideoRoutePolicy` to `DispatchCraftsmanToolInput`, parsed dispatch args, task input JSON, and route recommendation.
- [x] Run the dispatch test and verify it passes.

## Task 2: Craftsman Runtime Guard

- [x] Add a failing test proving `upsert_render_plan` rejects `seedance_2_video` when runtime `VideoRoutePolicy=template_only`.
- [x] Run the test and verify it fails because no guard exists.
- [x] Add `VideoRoutePolicy` to `NativeRuntimeContext`, Craftsman parsed task input, GraphInput/context text, and native tool loop context propagation.
- [x] Add the runtime guard in `validateUpsertRenderPlanRuntime`.
- [x] Run the upsert test and verify it passes.

## Task 3: Prompts And E2E

- [x] Update Producer/Craftsman wording so explicit no-Seedance instructions map to `video_route_policy=template_only`.
- [x] Run focused Go tests for tools and craftsman.
- [x] Restart the dev profile.
- [x] Use `/Users/wanwan/Desktop/box.png` in a real Agent workspace.
- [x] Ask Agent to create a 悦行行李箱口播广告 with "不要调用 Seedance，只用 HyperFrames/template video".
- [x] Verify generated jobs contain zero `volcengine` or `seedance` jobs and at least one `internal_template_video/hyperframes-html` succeeded MP4.
