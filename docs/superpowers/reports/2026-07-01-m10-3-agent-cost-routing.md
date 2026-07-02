# M10.3 Agent Cost Routing Report

**日期**：2026-07-01
**状态**：通过

## 运行范围

M10.3 将 M10.2 的 template video provider 接入 Agent 路由默认值。目标不是新增独立调度器，而是在现有 `dispatch_craftsman -> upsert_render_plan -> worker_generation -> production` 链路内，让 broad shot video dispatch 默认控制 Seedance 数量。

本阶段完成：

- `dispatch_craftsman(target_phase=shot_video)` 在批量派发分镜视频时写入 per-shot route recommendation。
- 默认策略：第一条 broad shot video dispatch 作为 Seedance hero route；后续 shot 推荐 `template_video/image_to_template_video`。
- 单条 shot dispatch 默认仍推荐 Seedance；若 brief/scope 明显是 CTA、packshot、卖点卡、静态兜底或模板内容，则推荐 template route。
- Craftsman task input 和 `NativeRuntimeContext` 增加：
  - `recommended_model_prompt_profile`
  - `recommended_operation`
  - `recommended_params`
  - `recommended_route_reason`
  - `input_node_refs`
- `upsert_render_plan` 在模型未显式填写 profile/operation/reference bindings/params 时继承 runtime recommendation。
- `upsert_render_plan` 可将 task `input_node_refs` 自动转为 `media_node` reference bindings，避免 Craftsman 忘记把已确认预览图带到 Worker。
- RenderPlan params 增加 template 字段：`template_key`、`fps`、`variables`。
- Craftsman Current Task context 会展示推荐 route 和 input refs，便于模型按工程策略创建 RenderPlan。

## 路由策略

批量分镜视频派发时：

```text
shot_01 -> seedance_2_video / image_to_video_first_frame
shot_02 -> template_video / image_to_template_video
shot_03 -> template_video / image_to_template_video
...
```

该策略只作为默认推荐，不锁死 Craftsman。Craftsman 显式填写 `model_prompt_profile` 或 `operation` 时，工具尊重显式输入并继续执行已有 validation。

## TDD 记录

新增失败测试先红在缺少 runtime 字段和 params 字段：

```text
unknown field RecommendedModelPromptProfile in struct literal of type NativeRuntimeContext
got.Params.TemplateKey undefined
```

实现后定向测试通过：

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'TestDispatchCraftsmanRecommendsTemplateRouteForNonHeroShotVideos|TestUpsertRenderPlanRuntimeDefaultsUseRecommendedTemplateRoute'
```

```text
ok  	github.com/sinmaystar/clip-anvil/internal/agent/tools	0.930s
```

## 验证命令

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools ./internal/agent/renderplan ./internal/agent/craftsman ./internal/agent/worker
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

## 验证结果

```text
ok  	github.com/sinmaystar/clip-anvil/internal/agent/tools	0.596s
ok  	github.com/sinmaystar/clip-anvil/internal/agent/renderplan	1.140s
ok  	github.com/sinmaystar/clip-anvil/internal/agent/craftsman	0.897s
ok  	github.com/sinmaystar/clip-anvil/internal/agent/worker
```

```text
GOCACHE=/private/tmp/clipanvil-go-build make server-test
cd apps/server && go test ./...
...
ok  	github.com/sinmaystar/clip-anvil/internal/agent/producer
ok  	github.com/sinmaystar/clip-anvil/internal/agent/renderplan
ok  	github.com/sinmaystar/clip-anvil/internal/agent/tools
ok  	github.com/sinmaystar/clip-anvil/internal/agent/worker
ok  	github.com/sinmaystar/clip-anvil/internal/production
ok  	github.com/sinmaystar/clip-anvil/internal/sandbox
ok  	github.com/sinmaystar/clip-anvil/internal/templatevideo
```

`git diff --check` 通过。

前端验证通过，但当前 shell 使用 Node v24.14.0，pnpm 输出了 repo engine warning：

```text
Unsupported engine: wanted: {"node":">=26 <27"} (current: {"node":"v24.14.0","pnpm":"11.7.0"})
```

`pnpm --filter @clip-anvil/web... build` 和 `pnpm --filter @clip-anvil/web lint` 均 exit 0。

## 结论

M10.3 gate 通过。Agent 现在具备工程侧成本路由默认值：批量分镜视频默认最多推荐一个 Seedance hero shot，后续分镜默认推荐 HyperFrames template video。下一阶段 M10.4 应处理 Reviewer 与 fallback：Seedance provider rejection、连续失败、template fallback 的质量评审和 HITL 边界。
