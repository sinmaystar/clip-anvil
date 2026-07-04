# M10.1 Template Video RenderPlan Routing Plan

## Goal

在不接入真实模板渲染 provider 的前提下，先让 ClipAnvil 的能力表、RenderPlan profile、校验层、Craftsman 工具 schema 和 Agent prompt 都能表达低成本模板视频路线：

- `internal_template_video/hyperframes-html` 作为 enabled capability 存在。
- `template_video` 作为 RenderPlan profile 存在。
- `shot_video` 可以选择 `seedance_2_video` 或 `template_video`。
- 非 `shot_video` 阶段不能使用 `template_video`。
- 默认行为仍保持 `shot_video -> seedance_2_video`，避免在 M10.1 提前改变自动成片执行路径。

## Files To Change

- `apps/server/migrations/039_m10_template_video_capability.sql`
- `apps/server/internal/agent/renderplan/types.go`
- `apps/server/internal/agent/renderplan/profiles.go`
- `apps/server/internal/agent/renderplan/profiles_test.go`
- `apps/server/internal/agent/renderplan/service.go`
- `apps/server/internal/agent/renderplan/service_test.go`
- `apps/server/internal/agent/renderplan/prompt_compiler_test.go`
- `apps/server/internal/agent/tools/upsert_render_plan.go`
- `apps/server/internal/agent/tools/render_plan_tools_test.go`
- `apps/server/internal/agent/producer/system_prompt.go`
- `apps/server/internal/agent/craftsman/system_prompt.go`
- `apps/server/internal/agent/skills/library/commerce-ad-producer/SKILL.md`
- `apps/server/internal/agent/skills/library/seedance-renderplan-craftsman/SKILL.md`
- `docs/milestones/m10-hyperframes-template-video-provider.md`
- `docs/superpowers/reports/2026-07-01-m10-1-template-video-renderplan-routing.md`

## Implementation Steps

- [ ] Add failing tests for `template_video` profile lookup, default provider/model, allowed operations, and default params.
- [ ] Add failing RenderPlan service tests proving `shot_video + template_video` compiles and non-`shot_video + template_video` is rejected before DB writes.
- [ ] Add failing PromptCompiler test proving `template_video` compiles a request with `provider=internal_template_video`, `model=hyperframes-html`, `profile=template_video`, and `operation=template_to_video`.
- [ ] Add failing Craftsman tool tests proving schema/validation accepts explicit `template_video` and `template_to_video`, while runtime defaults still infer Seedance for generic `shot_video`.
- [ ] Add `ProfileTemplateVideo` and profile metadata in `renderplan`, with operations `template_to_video` and `image_to_template_video`.
- [ ] Update RenderPlan service validation to allow `shot_video` with either `seedance_2_video` or `template_video`, forbid `template_video` elsewhere, and keep Seedance duration validation scoped to Seedance only.
- [ ] Update `upsert_render_plan` schema descriptions and validator enum lists so Craftsman can explicitly write template video plans.
- [ ] Add migration `039_m10_template_video_capability.sql` to upsert `internal_template_video` provider, `hyperframes-html` capability, and update `render_plan_profile_check`.
- [ ] Update Producer/Craftsman prompts and relevant embedded skill docs to explain the cost route: reserve Seedance for true dynamic hero shots; use template video for product cards, CTA, packshot, text-led or fallback shots.
- [ ] Run targeted tests, then `make sqlc-generate`, `GOCACHE=/private/tmp/clipanvil-go-build make server-test`, and `git diff --check`.
- [ ] Record verification results in the M10.1 report and update the M10 milestone status.

## Acceptance

- Tests fail before implementation and pass after implementation.
- Migration is idempotent on provider/capability upsert and reversible enough for local rollback.
- No worker/provider execution path is changed in this stage.
- Existing default `shot_video` inference remains Seedance until M10.3 introduces full cost-routing automation.
