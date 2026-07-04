# M10.4 Reviewer Fallback Strategy Report

**日期**：2026-07-01
**状态**：通过

## 运行范围

M10.4 修复 Seedance 失败后的策略缺口：Worker 不再只告诉 Producer “生成失败”，而是把 provider/model/operation 和 fallback guidance 一起写入 Producer pending signal；Producer 被唤醒时能看到 cost/fallback 约束；Reviewer 也能识别 template video route 并按模板视频标准评审。

本阶段完成：

- Reviewer context 增加 `Route Facts`，展示 provider、model、operation、rendering family、template engine、template key 和 template review focus。
- Reviewer system prompt 增加 Template Video 评审标准：`readability`、`platform_selling_power`、`brand_consistency`、`motion_rhythm`、`audio_sync`、`truthfulness`。
- Worker 在 Seedance/Volcengine `shot_video` 失败后，向 Producer pending signal 写入：
  - `model_provider`
  - `model_id`
  - `operation_type`
  - `fallback_strategy=template_fallback_or_hitl`
  - `recommended_next_action=route_to_template_fallback_or_request_user_confirmation`
  - `should_stop_same_route_retry=true`
  - `cost_risk=true`
- Producer runtime reminder 消费这些字段，在失败唤醒文案中明确：
  - 不要继续同一路线自动重试。
  - 优先考虑 template fallback。
  - 如果 brief 要求真实复杂运动或用户质量门槛不确定，先请求用户确认。
- Producer system prompt 增加长期规则：遇到 `fallback_strategy=template_fallback_or_hitl` 或 `cost_risk` 时，不盲目重试 Seedance。

## 策略边界

Worker 只发出失败事实和 fallback guidance，不自动创建 template RenderPlan。是否转 template fallback、是否继续 Seedance、是否 HITL，仍由 Producer 基于 brief、review、成本风险和用户授权决策。

Template fallback 不是万能通过项。若用户明确要求真实复杂运动、人物表演、镜头穿越或强物理运动，Reviewer 应按 `faithfulness`、`motion_physics` 或 `cost_risk` 给出 warning/rejection，并在需要时要求用户确认。

## TDD 记录

Reviewer context 先红在缺少 route facts：

```text
context text missing "Route Facts"
```

Worker signal 先红在缺少 provider/model/fallback 字段：

```text
payload[model_provider] = <nil>, want "volcengine"
```

Producer reminder 先红在 payload struct 缺少 fallback 字段：

```text
unknown field FallbackStrategy in struct literal of type producerTaskTriggerPayload
```

Producer system prompt 先红在缺少长期路由规则：

```text
prompt missing current capability wording "fallback_strategy"
```

实现后定向测试通过：

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run TestContextLoaderIncludesTemplateVideoRoutingFacts
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run TestReviewerSystemPromptContainsGateRules
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/worker -run TestWorkerSignalsSeedanceFailureFallbackGuidance
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run TestProducerRuntimeTriggerTextIncludesFallbackGuidance
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run TestProducerSystemPromptEnablesCurrentGenerationAndReviewerGate
```

## 验证命令

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer ./internal/agent/worker ./internal/agent/producer
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

## 验证结果

```text
ok  	github.com/sinmaystar/clip-anvil/internal/agent/reviewer	2.092s
ok  	github.com/sinmaystar/clip-anvil/internal/agent/worker	0.529s
ok  	github.com/sinmaystar/clip-anvil/internal/agent/producer	1.201s
```

```text
GOCACHE=/private/tmp/clipanvil-go-build make server-test
cd apps/server && go test ./...
ok  	github.com/sinmaystar/clip-anvil/cmd/server	1.253s
ok  	github.com/sinmaystar/clip-anvil/internal/agent/producer	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/agent/reviewer	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/agent/worker	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/production	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/sandbox	(cached)
ok  	github.com/sinmaystar/clip-anvil/internal/templatevideo	(cached)
```

```text
pnpm --filter @clip-anvil/web... build
Unsupported engine: wanted: {"node":">=26 <27"} (current: {"node":"v24.14.0","pnpm":"11.7.0"})
✓ built in 700ms
```

```text
pnpm --filter @clip-anvil/web... lint
Unsupported engine: wanted: {"node":">=26 <27"} (current: {"node":"v24.14.0","pnpm":"11.7.0"})
$ eslint .
```

前端 build/lint 均 exit 0。Node 24 engine warning 是当前 shell 环境已知现象，repo 目标仍是 Node 26。

## 结论

M10.4 gate 通过。ClipAnvil 现在具备 Seedance 失败后的成本保护信号：Worker 暴露 provider/model/operation 和 fallback guidance，Producer 被唤醒时不会只看到普通失败，Reviewer 也能按 template fallback 的实际质量边界评审产物。下一阶段 M10.5 应稳定 template params / variables / input refs 的 input hash，为 Variant Factory 做准备。
