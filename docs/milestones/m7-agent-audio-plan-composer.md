# M7 Agent AudioPlan 与 Composer 音频成片 — 里程碑

**状态**：M7.1、M7.2 已完成，M7.3 待启动（2026-06-28）
**目标**：为 Agent 第一版短视频生产链路补齐音频能力：Producer 生成全片级 AudioPlan，Craftsman 产出旁白 / BGM 音频 RenderPlan，Worker 调用 `seed-audio-1.0` 生成音频 artifact，Composer 将分镜视频、旁白和 BGM 混成最终视频，并由 Producer 决定是否进入最终 review。

参考文档：

- [方案规格](../superpowers/specs/2026-06-28-agent-audio-plan-composer-design.md)
- [M7.1 实施计划](../superpowers/plans/2026-06-28-m7-1-audioplan-producer-confirmation.md)
- [M7.2 实施计划](../superpowers/plans/2026-06-28-m7-2-audio-renderplan-seed-audio.md)
- [M7.3 实施计划](../superpowers/plans/2026-06-28-m7-3-composer-audio-mixing.md)

## Codex Goal 建议

按阶段完成 M7 Agent 音频链路。每个阶段都必须先写清该阶段实施计划，再开发、验证、记录验收结果；上一阶段未通过验收时，不进入下一阶段。

第一版只交付 **模型生成的营销短视频旁白 + BGM**。不接用户上传音频素材，不接素材库 BGM，不做真人对口型，不做多角色对白连续性，不把视频生成模型自带音频作为多分镜最终主音轨。

## 已确认口径

- 音频事实源是全片级 `AudioPlan`，不是单个 shot 的独立音频副产物。
- 每个 workspace 同一时间只保留一个 active AudioPlan。
- 旁白和 BGM 第一版都由音频模型生成，BGM 必须接入 `seed-audio-1.0`。
- `.env` 需要写入 Volcengine 音频生成所需配置；`model_capability` 数据库也要插入 `seed-audio-1.0` 的 audio generation 能力。
- Producer 负责 AudioPlan、用户确认、音频策略和 Composer 后是否进入 review 的决策。
- Craftsman 只负责把 approved AudioPlan 翻译成 `voiceover_audio` / `bgm_audio` RenderPlan。
- Worker / shared production 负责调用 provider、上传音频结果、落库 generation job 和 artifact。
- Composer 只负责混音、ducking、fade、concat 和 final video artifact，不修改 AudioPlan 文案。

## 阶段里程碑

| 阶段 | 里程碑 | 可验收标准 |
|---|---|---|
| M7.1 AudioPlan 事实源与 Producer 确认 | 新增全片级 AudioPlan 数据模型、服务、工具和 Producer 决策流。 | ✅ 已完成；数据库存在 `audio_plan`，有 workspace active unique index；Producer 已获得 `upsert_audio_plan`，可写入待确认方案并在用户确认后 approve；`read_project_context` / Producer PSS / prompt 已能读取和约束 AudioPlan。验证：`make sqlc-generate`、`GOCACHE=/private/tmp/clipanvil-go-build make server-build`、`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`。 |
| M7.2 音频 RenderPlan 与 `seed-audio-1.0` 生成 | Craftsman 支持 `voiceover_audio` / `bgm_audio`，shared production 接入 Volcengine audio generation。 | ✅ 已完成；`model_capability` 新增 enabled `seed-audio-1.0`；`.env.example` 和本地 ignored `.env` 已写入音频模型配置；Craftsman 可为 approved AudioPlan 派发并生成 `voiceover_audio` / `bgm_audio` RenderPlan；Worker 可提交 audio generation intent 并回填 AudioPlan render plan / node；Volcengine runtime 支持 base64 audio 与临时 URL 返回，MockProvider 可生成 audio artifact。验证：`make sqlc-generate`、`GOCACHE=/private/tmp/clipanvil-go-build make server-build`、`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`、`git check-ignore -v .env`。真实 Volcengine 付费 smoke 未执行。 |
| M7.3 Composer 混音成片 | Composer 能读取视频 winners、voiceover artifact、BGM artifact 和 AudioPlan，产出带音频的 final video。 | TimelinePlan 支持 voiceover / BGM tracks、音量、fade、ducking；ffmpeg sandbox job 可完成 2-3 个分镜视频 + generated voiceover + generated BGM 的端到端合成；最终 artifact 含 AAC 音轨并可播放。 |
| M7.4 Workbench 投影与最终 Review | Workbench / overview 显示 AudioPlan、音频 cue、生成状态和最终音轨摘要；Producer 决定是否派发 final review。 | 用户能看到当前音频方案、旁白文本、BGM 方向、生成状态和 Composer 结果；Reviewer final video review 使用 `audio_sync` 评估旁白节奏、BGM 音量和音画匹配；Producer 可选择 review、请求用户确认或回修。 |

## 阶段验收建议

### M7.1

- ✅ 已完成（2026-06-28）。实施计划、迁移、sqlc query、service、Agent tool、Producer prompt / context / PSS 均已落地。
- 验证命令：
  - `make sqlc-generate`
  - `GOCACHE=/private/tmp/clipanvil-go-build make server-build`
  - `make server-test`
  - `git diff --check`

### M7.2

- ✅ 已完成（2026-06-28）。实施计划、RenderPlan audio scope、Craftsman audio_plan context、Worker audio generation mapping、Volcengine Seed Audio runtime、mock audio artifact、model capability 和 env 示例均已落地。
- 验证命令：
  - `make sqlc-generate`
  - `GOCACHE=/private/tmp/clipanvil-go-build make server-build`
  - `GOCACHE=/private/tmp/clipanvil-go-build make server-test`
  - `git diff --check`
  - `git check-ignore -v .env`
- 真实 `seed-audio-1.0` 付费外部 smoke 未执行；当前验收以 mock path、runtime HTTP mock 和完整 server test 为准。

### M7.3

- 先写阶段实施计划，明确 TimelinePlan 音轨结构、Composer staging、ffmpeg filter graph、sandbox job 记录和 final artifact 回填。
- 验证命令：
  - `make server-build`
  - `make server-test`
  - `git diff --check`
- 必须补一个最小端到端 smoke：2-3 个分镜视频 + generated voiceover + generated BGM -> final video。

### M7.4

- 先写阶段实施计划，明确前端 overview / workbench 投影、Producer post-composer decision、Reviewer final loader 的变更点。
- 验证命令：
  - `pnpm --filter @clip-anvil/web... build`
  - `pnpm --filter @clip-anvil/web lint`
  - `make server-test`
  - `git diff --check`

## 完成定义

- M7.1-M7.4 全部通过各自验收，且验收结果写入对应阶段总结或 PR 描述。
- Agent 能从已确认的短视频需求生成 AudioPlan，并由用户确认或修改。
- Worker 能通过 `seed-audio-1.0` 生成旁白和 BGM 两类 audio artifact。
- Composer 能把分镜视频、旁白和 BGM 混成一个可播放的 final video artifact。
- Workbench 能展示音频方案和生成状态，最终 review 能覆盖 `audio_sync`。
- 第一版范围外能力没有被半接入主路径，避免后续产品语义不清。

## 暂不做

- 用户上传音频素材作为第一版主路径。
- BGM 素材库、授权素材管理和人工选曲。
- 真人或角色对口型。
- 多角色对白连续性。
- 自动声音复刻训练流程。
- 专业 DAW 级时间线编辑 UI。
- 超过 `seed-audio-1.0` 单次输出限制的复杂长视频拆段策略。
