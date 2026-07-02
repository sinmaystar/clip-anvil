# M10.1 Template Video RenderPlan Routing Report

**日期**：2026-07-01
**状态**：通过

## 运行范围

M10.1 只交付 capability 与 RenderPlan 路由基础，不接真实 HyperFrames provider，不改变 Worker 执行路径，也不改变 `shot_video` 默认推断仍走 Seedance 的行为。

本阶段完成：

- 新增 `internal_template_video/hyperframes-html` capability migration。
- 新增 `template_video` RenderPlan profile。
- `shot_video` 允许 `seedance_2_video` 或 `template_video`。
- 非 `shot_video` 阶段禁止 `template_video`。
- PromptCompiler compiled request 增加 `provider` 和 `model`，让后续 provider dispatch 不必重新猜 profile 元数据。
- `upsert_render_plan` schema / validator 允许 Craftsman 显式写 `template_video`、`template_to_video`、`image_to_template_video`。
- Producer / Craftsman prompt 与相关 skill 文案写清成本路由：Seedance 保留给真实动态 hero shot，template video 用于卖点卡、CTA、packshot、产品图轻动效和静态兜底。

## TDD 记录

先写失败测试，初始失败点符合预期：

```text
undefined: ProfileTemplateVideo
schema missing template_video
mode 的值是 "template_video"，但只支持 seedream_5_image、seedance_2_video、seed_audio_1
```

实现后定向测试通过：

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/renderplan ./internal/agent/tools -run 'TestProfileByIDReturnsSeedreamSeedanceSeedAudioAndTemplateProfiles|TestServiceAcceptsTemplateShotVideoRenderPlan|TestServiceRejectsTemplateVideoForNonShotPhase|TestCompileTemplateShotVideoPromptIncludesInternalProvider|TestUpsertRenderPlanToolValidatesExplicitTemplateVideoPlan|TestRenderPlanToolInfosUseTypedSchemasAndChineseDescriptions'
```

```text
ok  	github.com/sinmaystar/clip-anvil/internal/agent/renderplan	0.728s
ok  	github.com/sinmaystar/clip-anvil/internal/agent/tools	0.754s
```

## 验证命令

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

## 验证结果

```text
make sqlc-generate
cd apps/server && /Users/wanwan/go/bin/sqlc generate
```

```text
GOCACHE=/private/tmp/clipanvil-go-build make server-test
cd apps/server && go test ./...
...
ok  	github.com/sinmaystar/clip-anvil/internal/agent/renderplan
ok  	github.com/sinmaystar/clip-anvil/internal/agent/tools
ok  	github.com/sinmaystar/clip-anvil/internal/agent/skills
ok  	github.com/sinmaystar/clip-anvil/internal/production
ok  	github.com/sinmaystar/clip-anvil/internal/sandbox
```

`git diff --check` 通过。

## 结论

M10.1 gate 通过。现在代码和 DB schema 已能表达低成本模板视频路线，但还不能真正执行 HyperFrames provider。下一阶段 M10.2 应实现 `TemplateVideoProvider`、sandbox `RenderTemplateVideo`、最小内置模板、MinIO 入库和 generation trace。
