# M13 Remotion Timeline Composer Codex Goals

**日期**：2026-07-03
**设计来源**：`docs/superpowers/specs/2026-07-03-agent-remotion-timeline-composer-design.md`
**里程碑说明**：`docs/milestones/m13-remotion-timeline-composer.md`
**用途**：本文件把 M13 拆成可逐阶段执行的 Codex goal。每次只复制一个阶段的 goal 作为当前目标，完成验收后再进入下一阶段。

## 执行原则

- 一次只执行一个 active goal。
- 每个阶段必须有新鲜验证输出，不能只靠聊天消息或代码推测。
- 涉及 Agent E2E 时必须记录 workspace id、关键 DB 记录、工具调用结果和 final artifact。
- 涉及最终视频时必须用 `ffprobe` 或等价工具验证 duration、video stream、audio stream 和 resolution。
- 不提交 runtime、browser、smoke 生成物，除非用户明确要求。
- 如果阶段验收失败，先修复当前阶段，不提前进入后续阶段。

## M13.1 Codex Goal

```text
Codex Goal: M13.1 Remotion Timeline Renderer 纵切

Objective:
在 ClipAnvil 中新增 `remotion_timeline_v1` Composer template，打通 sandbox Remotion final timeline renderer。使用 fixture RemotionTimelinePlan、fixture 图片、voiceover 和 BGM 渲染出一个可持久化的 final_video artifact，同时保证现有 ffmpeg `simple_concat` 和 `concat_with_fades` 不回归。

Tasks:
- 新增 `remotion_timeline_v1` template key，并让 `dispatch_composer`、`create_timeline_plan`、`render_timeline_template` 接受该 template。
- 新增 Go 侧 `RemotionTimelinePlan` decode 和 validation，校验 schema、composition、output、segments、asset workspace_path、audio workspace_path 和时间范围。
- 新增独立 sandbox Remotion timeline renderer，不复用单 shot `remotion-motion-shot` project。
- 扩展 `render_timeline_template`：旧 ffmpeg templates 保持原路径，`remotion_timeline_v1` 走 Remotion renderer。
- 新增 fixture smoke，使用 1-2 张图片、voiceover、BGM 渲染 8-12 秒竖屏 MP4。
- 记录 `timeline_plan` 的 sandbox job、output artifact 和 result metadata。

Deliverables:
- `apps/server/internal/remotiontimeline/` 包含 plan types、decode 和 validation。
- `apps/server/internal/sandbox/remotion_timeline.go` 或等价 sandbox job service 存在。
- `sandbox-image/remotion-timeline/` 包含 Remotion project、schema、entrypoint 和 render script。
- Composer tool tests 覆盖 `remotion_timeline_v1` route。
- smoke script 可在本地渲染 fixture final video。

Acceptance:
- fixture `remotion_timeline_v1` 可以渲染 final MP4。
- final MP4 有 video stream 和 audio stream。
- final MP4 resolution 为 `1080x1920`，duration 接近 timeline duration。
- `timeline_plan` 能回填 sandbox_job_id、artifact_version_id 和 completed/succeeded status。
- 旧 `simple_concat` 和 `concat_with_fades` 测试仍通过。

Verification:
- `cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/remotiontimeline ./internal/agent/tools ./internal/sandbox`
- `cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build make server-build`
- `bash -n scripts/smoke-m13-1-remotion-timeline.sh`
- `node --check sandbox-image/remotion-timeline/src/render.mjs`
- `./scripts/smoke-m13-1-remotion-timeline.sh`
- `git diff --check`

Stop Conditions:
- Remotion fixture render 无法生成带音频 MP4。
- 旧 ffmpeg template route 出现回归。
- timeline plan validation 无法阻止非法 workspace path 或非法时间范围。
```

## M13.2 Codex Goal

```text
Codex Goal: M13.2 Composer Agent 接入 Remotion Timeline

Objective:
让 ClipAnvil 真实 Agent 模式能够在 no-Seedance 低成本营销视频请求中，由 Composer 自动生成 `remotion_timeline_v1` 的 RemotionTimelinePlan，并使用 Seedream still、火山 voiceover/BGM 和 AudioPlan cue_plan 渲染 30 秒以上 final video。

Tasks:
- 扩展 `get_composition_context`，让 still images、shot metadata、AudioPlan cue_plan、voiceover/BGM 和 remotion timeline schema 成为 Composer 可见的一等输入。
- 新增或更新 Composer skill，指导 Composer 生成 `RemotionTimelinePlan`，而不是只生成 ffmpeg concat plan。
- 更新 Composer system prompt：允许 `remotion_timeline_v1`，以 AudioPlan cue order 作为 primary timeline，禁止把 `narrative_purpose`、`visual_intent`、`action_text` 写成字幕。
- 更新 deterministic Composer fallback，使模型没有主动调用工具时仍能基于 cue plan 和 still assets 生成基本 `RemotionTimelinePlan`。
- 更新 Producer prompt 和 skill：用户要求低成本或 no-Seedance 时，优先生成 Seedream still + 火山音频 + `dispatch_composer(template_key=remotion_timeline_v1)`。
- 增加浏览器 Agent E2E：上传商品图，请求 30 秒以上中文口播广告，明确禁止 Seedance，允许 Seedream 图片和火山音频，最终用 Remotion timeline 成片。
- 增加 DB smoke 查询，审计 provider route、timeline_plan、media assets 和 final artifact。

Deliverables:
- Composer context 中能看到 shot_ref、sort_order、duration_sec、narrative_purpose、visual_intent、action_text、camera_intent、narration、cue_plan 和 still/audio assets。
- `timeline_plan.template_key=remotion_timeline_v1` 可由真实 Composer Agent 创建。
- `plan_json` 使用真实 shot_ref、真实 staged asset path、真实 audio path 和 cue-derived captions。
- no-Seedance 主线不再要求每个 shot 先生成 `motion_shot_video`。
- E2E 产出一个 30 秒以上 final video artifact。

Acceptance:
- 浏览器 Agent E2E 真实跑通。
- DB 中有 `timeline_plan.template_key=remotion_timeline_v1`。
- DB 中没有 Seedance video generation_job。
- DB 中有多张 Seedream image job。
- DB 中有 voiceover 和 BGM audio job。
- final video duration >= 30 秒，且有 audio stream。
- 抽查 `timeline_plan.plan_json`：segments 与 cue plan 对齐，captions 不包含内部导演笔记。
- 讲万向轮的 cue 使用 wheel/detail 相关素材；讲收纳的 cue 使用 open storage/interior 相关素材。

Verification:
- `cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer ./internal/agent/skills ./internal/agent/producer ./internal/agent/tools ./internal/remotiontimeline ./internal/sandbox`
- `cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build make server-build`
- After E2E, set `WORKSPACE_ID` to the recorded workspace UUID, then run `psql "$DATABASE_URL" -v workspace_id="$WORKSPACE_ID" -f scripts/smoke-m13-2-agent-remotion-route.sql`.
- `ffprobe` final video and record duration, stream types and resolution.
- `git diff --check`

Stop Conditions:
- Composer 仍创建 ffmpeg `simple_concat` 或 `concat_with_fades`，而不是 `remotion_timeline_v1`。
- E2E 出现 Seedance video generation_job。
- 字幕来自内部导演字段。
- 口播 cue 与画面素材明显错配但系统没有 blocked、修复或记录 issue。
```

## M13.3 Codex Goal

```text
Codex Goal: M13.3 Remotion 营销 Layout 与 Cue 同步增强

Objective:
在 `remotion_timeline_v1` 主线中增加可控的营销 layout、motion、transition、caption lane 和 cue/asset mismatch 检查，让低成本 Remotion final video 从可渲染提升到具备营销视频观感和基本音画语义同步可靠性。

Tasks:
- 扩展 Remotion layout enum 和 components：`hero_packshot`、`detail_focus`、`benefit_card`、`split_compare`、`scenario_card`、`open_storage`、`cta_endcard`。
- 扩展 motion preset：`push_in`、`pull_out`、`pan_left`、`pan_right`、`float_parallax`、`spotlight_reveal`、`kinetic_text`、`cta_pop`。
- 扩展 transition preset：`cut`、`crossfade`、`slide`、`wipe`、`zoom_blur`。
- 强化 caption lane：单一字幕轨、底部安全区、中文自动分行、避免与主标题 text layer 重叠。
- 增加 cue/asset mismatch blocker：cue visual_focus 或 visual_intent 与 selected asset metadata 冲突时 blocked。
- 增加重复视觉检查：同一图片不能无解释覆盖多数 segments，同一 layout 不能连续重复过多。
- 增加 Reviewer 规则：字幕来源、字幕重叠、BGM/voiceover 存在性、no-Seedance compliance、素材语义错配。
- 如当前火山 TTS 接口支持 subtitle/alignment，则保存并用于 caption timing；否则继续使用 AudioPlan cue scaling。

Deliverables:
- Remotion renderer 支持多 layout、多 motion 和多 transition。
- Composer plan validation 拒绝未知 layout、motion、transition。
- Composer 或 Reviewer 能发现 wheel cue 使用 storage still、storage cue 使用 wheel still 等明显错配。
- 字幕 lane 稳定、可读、不出现双字幕重叠。
- Reviewer 对故意错配 fixture 输出 blocked 或 issue。

Acceptance:
- E2E final video 至少使用 4 种 layout。
- 同一 still image 不超过总 segment 的 50%，除非 Producer 明确说明素材不足。
- captions 只来自 cue text、audio cue 或 TTS alignment，不来自 `narrative_purpose`、`visual_intent`、`action_text`。
- wheel cue 不使用 storage still；storage cue 不使用 wheel still。
- 字幕 lane 不与主标题明显重叠。
- 故意错配 fixture 会 blocked 或产生 Reviewer issue。

Verification:
- `cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/remotiontimeline ./internal/agent/composer ./internal/agent/reviewer ./internal/agent/tools ./internal/sandbox`
- `cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build make server-build`
- `pnpm --filter @clip-anvil/web... build` if frontend review UI changes are included.
- Run render smoke covering every new layout key.
- Run browser Agent E2E and record workspace id, final artifact id and DB evidence.
- `ffprobe` final video and record duration, stream types and resolution.
- `git diff --check`

Stop Conditions:
- Layout/motion/transition enum can accept arbitrary strings.
- Captions overlap or duplicate lanes in normal E2E output.
- Composer silently uses semantically wrong asset for a cue.
```

## M13.4 Codex Goal

```text
Codex Goal: M13.4 Remotion Final Composer 混合 Seedance 路线

Objective:
让 `remotion_timeline_v1` 成为通用 final composer，既能在 no-Seedance 路线中使用 Seedream still，也能在 mixed-cost 路线中混入少量 Seedance hero video，并由 Reviewer 输出成本/质量摘要。

Tasks:
- 扩展 Producer route policy：`no-seedance` 使用 Seedream still + audio + Remotion final timeline；`mixed-cost` 允许 hero/complex motion shot 使用 Seedance；`premium` 可使用更多 Seedance，但 final packaging 仍由 Remotion timeline 完成。
- 扩展 `RemotionTimelinePlan` 和 renderer，让 segment 可引用 image asset、video asset 或 image/video overlay。
- Composer 支持 staged video asset，包含 trim、fit、text overlay、caption overlay 和 transition。
- Reviewer 输出成本/质量摘要：Seedance 使用了几个 shot，Remotion still 使用了几个 shot，哪些 shot 可降级，外部 API 成本风险是什么。
- 增加本地 mixed-media smoke：至少 1 个 staged video segment、多个 Seedream-style still segments、火山/fixture audio、Remotion final timeline。
- 增加 no-Seedance regression E2E，确保用户禁止 Seedance 时仍没有 Seedance video job。
- 真实 mixed-cost Seedance E2E 属于成本型验收项，只有用户显式授权后才运行。

Deliverables:
- Remotion final composer 能在同一 timeline 中混合 Seedance video 和 Seedream still。
- Producer 能根据用户成本偏好选择 no-Seedance、mixed-cost 或 premium route。
- DB 可审计每个 shot 使用 Seedance video 还是 still image。
- Reviewer 或 final report 有成本/质量摘要。

Acceptance:
- 本地 mixed-media smoke 成功生成 final video。
- 如用户显式授权真实 mixed-cost E2E，DB 中 Seedance video job 数量符合 Producer plan。
- `timeline_plan.plan_json` 中同时存在 image segment 和 video segment。
- final video 有音频、字幕、转场和 CTA。
- Reviewer 或 final report 明确列出 Seedance 使用数量、Remotion still 使用数量和成本风险。
- no-Seedance E2E 不回归，DB 中仍没有 Seedance video generation_job。

Verification:
- `cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/remotiontimeline ./internal/agent/producer ./internal/agent/composer ./internal/agent/reviewer ./internal/agent/tools ./internal/sandbox`
- `cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build make server-build`
- Run local mixed-media smoke and record ffprobe evidence.
- Run real mixed-cost browser Agent E2E only after explicit user approval, then record workspace id, final artifact id and DB evidence.
- Run no-Seedance regression browser Agent E2E and record workspace id, final artifact id and DB evidence.
- `ffprobe` final videos and record duration, stream types and resolution.
- `git diff --check`

Stop Conditions:
- no-Seedance route accidentally calls Seedance.
- mixed-cost final timeline cannot combine video and image segments.
- Reviewer cannot explain Seedance usage scope or cost risk.
```

## 建议复制顺序

1. 复制 `M13.1 Codex Goal`，完成并验收。
2. 复制 `M13.2 Codex Goal`，完成并验收。
3. 复制 `M13.3 Codex Goal`，完成并验收。
4. 复制 `M13.4 Codex Goal`，完成并验收。

不要把四个 goal 合并成一个长目标执行。M13.1 是 renderer 能力，M13.2 是真实 Agent 接入，M13.3 是观感和同步质量，M13.4 是混合 Seedance 成本路线；它们的失败模式不同，应该分阶段验收。
