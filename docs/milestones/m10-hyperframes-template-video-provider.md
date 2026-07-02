# M10 HyperFrames Template Video Provider 与成本路由 — 里程碑

**状态**：M10 已完成
**日期**：2026-07-01
**目标**：为 ClipAnvil 引入低成本、确定性的模板视频生产路径。第一版接入 HyperFrames 作为 `internal_template_video` provider，让 Agent 能在营销视频中混用 Seedance hero shot 与 HyperFrames template shot，降低 Seedance 调用次数，并为后续批量变体和成本治理打基础。

参考文档：

- [HyperFrames Template Video Provider 与成本路由设计](../superpowers/specs/2026-07-01-hyperframes-template-video-provider-design.md)
- [M10.0 HyperFrames Sandbox Spike Report](../superpowers/reports/2026-07-01-m10-0-hyperframes-sandbox-spike.md)
- [M10.1 Template Video RenderPlan Routing Report](../superpowers/reports/2026-07-01-m10-1-template-video-renderplan-routing.md)
- [M10.2 Template Video Provider Vertical Slice Report](../superpowers/reports/2026-07-01-m10-2-template-video-provider-vertical-slice.md)
- [M10.3 Agent Cost Routing Report](../superpowers/reports/2026-07-01-m10-3-agent-cost-routing.md)
- [M10.4 Reviewer Fallback Strategy Report](../superpowers/reports/2026-07-01-m10-4-reviewer-fallback-strategy.md)
- [M10.5 Template Variant Input Hash Report](../superpowers/reports/2026-07-01-m10-5-template-variant-input-hash.md)
- [M7 Agent AudioPlan 与 Composer 音频成片](./m7-agent-audio-plan-composer.md)
- [M4 Shared Production Foundation](./m4-shared-production-foundation.md)
- [Agent MultiAgent 架构现状](../engineering/agent-multiagent-architecture.md)

## Codex Goal 建议

按阶段完成 M10 HyperFrames Template Video Provider 与成本路由。先验证 HyperFrames 在当前 OpenSandbox / MinIO / FFmpeg 生产边界内能稳定渲染 MP4，再接入 `internal_template_video` provider、`template_video` RenderPlan profile、Producer/Craftsman 路由规则和 Reviewer fallback 质量门。每个阶段都必须有明确实施计划、最小可验收 smoke 或单测、验证命令和结果记录；上一阶段未通过时，不进入下一阶段。

第一版只交付 **HyperFrames-backed internal template video provider + 成本路由基础**。不要一次性做完整模板编辑器、Remotion adapter、云渲染、Variant Factory UI、用户自由 HTML/JS 编辑或新的外部任务队列表。

## 已确认口径

- HyperFrames 是第一版 template engine；业务层命名使用 `internal_template_video`，避免把 provider family 绑定死到单一实现。
- Template video 是生产 provider 产物，不是 Composer 内部小技巧。产物必须走 `generation_job`、`artifact_version`、winner、stale、sandbox trace 的现有生产链路。
- Template video 可以进入 `shot_video` winner 槽位，Composer 继续按视频 winners + audio tracks 生成 final video。
- Seedance 不被替换。真实运动、复杂人物/场景和高价值 hero shot 仍优先走 Seedance。
- Agent 不直接写任意 HTML/JS。第一版只允许受控模板库 + schema-validated variables。
- 应用进程不直接执行 Node、Chrome、FFmpeg 或 npm。所有不可预测媒体执行仍通过 Sandbox Job Service。
- `generation_job.cost_cents` 和 `model_capability.pricing` 已存在，第一版先记录 template 内部成本为 0 或内部估算，Seedance 价格以后续配置或账单回填为准。

## 阶段里程碑

| 阶段 | 里程碑 | 可验收标准 |
|---|---|---|
| M10.0 HyperFrames Sandbox Spike | 在当前 sandbox runtime 中验证 HyperFrames 最小渲染链路。 | 已通过。OpenSandbox 可用 HyperFrames 0.7.22 渲染 3 秒 1080x1920 H.264 MP4；`ffprobe` 已读取 video stream；Node.js、FFmpeg、Chromium headless shell、中文字体和 CLI 参数要求已记录。验证见 [M10.0 report](../superpowers/reports/2026-07-01-m10-0-hyperframes-sandbox-spike.md)。 |
| M10.1 Capability 与 RenderPlan 路由基础 | 新增 `internal_template_video/hyperframes-html` capability 和 `template_video` profile。 | 已通过。`model_provider` / `model_capability` 包含 template provider；`shot_video` 可使用 `template_video`；非 `shot_video` 不可使用；Producer/Craftsman prompt 已写清成本路由口径。验证见 [M10.1 report](../superpowers/reports/2026-07-01-m10-1-template-video-renderplan-routing.md)。 |
| M10.2 Template Video Provider 竖切 | 新增 `TemplateVideoProvider` 和 sandbox `RenderTemplateVideo`，跑通一个最小模板。 | 已通过。`static_fallback_ken_burns_v1` 可通过真实 production API 生成 1080x1920 H.264 MP4；结果通过 MinIO 入库为 `artifact_version.winner=true`；`generation_job.provider=internal_template_video`；provider_request / provider_response 记录 template key、sandbox job、输出信息。验证见 [M10.2 report](../superpowers/reports/2026-07-01-m10-2-template-video-provider-vertical-slice.md)。 |
| M10.3 Agent 成本路由接入 | Producer / Craftsman 能在 Seedance 和 template route 间做受控选择。 | 已通过。批量 `shot_video` 派发默认最多推荐 1 个 Seedance hero shot，后续分镜推荐 `template_video/image_to_template_video`；Craftsman upsert 会继承推荐 profile、operation、template params 和 input refs；Worker 继续复用 M10.2 provider 执行 template RenderPlan。Workbench detail 已通过现有 model params / metadata / generation jobs JSON 展示 template engine / key。验证见 [M10.3 report](../superpowers/reports/2026-07-01-m10-3-agent-cost-routing.md)。 |
| M10.4 Reviewer 与 fallback 策略 | Reviewer 能识别 template shot，Producer 能在 Seedance 失败后合理降级或 HITL。 | 已通过。Worker 在 Seedance/Volcengine `shot_video` 失败后向 Producer signal 写入 provider/model/operation、`fallback_strategy=template_fallback_or_hitl`、`should_stop_same_route_retry=true` 和 `cost_risk=true`；Producer reminder 和 system prompt 明确不要继续同路线自动重试，应转 template fallback 或请求用户确认；Reviewer 按 readability、platform_selling_power、motion_rhythm、audio_sync、truthfulness 等维度评审 template fallback。验证见 [M10.4 report](../superpowers/reports/2026-07-01-m10-4-reviewer-fallback-strategy.md)。 |
| M10.5 Variant Factory 准备 | 为后续批量变体稳定落库 template params 和 input hash。 | 已通过。`InputHashFacts` 覆盖 template key、variables、upstream winner 和 input refs；同一 image winner + 不同 variables、不同 input role、不同 input 顺序或不同 upstream winner 都会产生不同 hash。`intentForNode` 可从 media node 恢复 template provider/model/operation/params 以支持 stale 重算。验证见 [M10.5 report](../superpowers/reports/2026-07-01-m10-5-template-variant-input-hash.md)。 |

## 阶段验收建议

### M10.0

目标是先消除 runtime 最大不确定性。只做技术 spike，不写生产 provider。

必须确认：

- sandbox 内 Node.js 版本满足 HyperFrames 要求。
- sandbox 内 FFmpeg 可执行。
- HyperFrames 当前 CLI render 参数被实际验证。
- Chrome/headless shell 依赖明确。
- 中文字体可用，或至少明确第一版需要预装字体包。
- 输出 MP4 可被 `ffprobe` 读取。

建议记录：

- 使用的 HyperFrames 版本。
- 实际 render 命令。
- 输出文件路径、duration、resolution、fps。
- 失败 stderr 摘要和修复方式。

### M10.1

目标是把能力目录和 RenderPlan 语义打通，不接真实 HyperFrames 渲染。

必须交付：

- migration 插入 `internal_template_video` provider 和 `hyperframes-html` capability。
- `render_plan.model_prompt_profile` 支持 `template_video`。
- RenderPlan validation 允许 `target_phase=shot_video` + `model_prompt_profile=template_video`。
- RenderPlan validation 禁止 `template_video` 用于图片、音频或 final video。
- Producer / Craftsman prompt 明确不要默认所有 shot 都用 Seedance。

### M10.2

目标是第一条生产竖切：RenderPlan -> GenerationIntent -> provider -> sandbox -> MinIO -> artifact version。

必须交付：

- `apps/server/internal/production/template_video_provider.go`。
- `apps/server/internal/sandbox/template_video.go`。
- 一个最小内置模板。
- mock sandbox provider 单测。
- 本地 smoke 脚本。

通过后，即使 Agent 路由还没改，Studio/测试路径也应能证明 template video provider 可用。

### M10.3

目标是让 Agent 主链路使用 template route。

默认策略：

- 自动生成营销视频时，优先把 hero shot 分配给 Seedance。
- 卖点卡、CTA、品牌尾帧、静态商品展示优先分配给 template。
- 只有用户明确要求全动态或 Reviewer/Producer 判断必要时，才增加 Seedance shot 数量。

不做：

- 用户预算 UI。
- 模板选择器 UI。
- 自由编辑模板变量 UI。

### M10.4

目标是修复“生成失败后继续烧钱”的策略缺口。

必须覆盖：

- Seedance safety/provider rejection。
- 同一 shot 多次失败。
- template fallback 被 Reviewer 正确识别。
- brief 明确要求真实动态时，template fallback 不能直接当作高质量通过。

### M10.5

目标是为后续 Variant Factory 铺底，不做完整产品入口。

必须覆盖：

- template params 稳定进入 input hash。
- variables 变化产生新版本。
- 同一素材可低成本生成多个版本。
- Workbench / production state 能区分不同 template versions。

## 完成定义

- `internal_template_video/hyperframes-html` 已作为 enabled capability 存在。
- `template_video` RenderPlan profile 已可用于 `shot_video`。
- 至少一个 HyperFrames template 能通过 sandbox 生成 MP4，并通过 production service 入库为 video artifact winner。
- Agent 能生成混合路线：Seedance hero shot + template benefit / CTA shot。
- Composer 能使用 template shot winners 生成 final video。
- Reviewer 能识别 template shot 和 Seedance shot 的来源差异。
- Seedance 连续失败时，Producer 能选择 template fallback 或 HITL，而不是继续自动重试。
- 所有阶段都有新鲜验证输出，不把 smoke 产物或本地 runtime 产物提交。

## 暂不做

- Remotion adapter。
- 云渲染 / Lambda / Cloud Run。
- 完整模板编辑器。
- 用户自由 HTML、JS 或 CSS 编辑。
- 素材市场 / 模板市场。
- GPU 本地视频模型。
- 外部任务队列重构。
- Variant Factory UI。
