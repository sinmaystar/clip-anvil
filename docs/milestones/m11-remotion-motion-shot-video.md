# M11 Remotion Motion Shot Video 与 HyperFrames 清理 — 里程碑

**状态**：进行中
**日期**：2026-07-02
**目标**：按 [Remotion Motion Shot Video 与 HyperFrames 清理设计](../superpowers/specs/2026-07-02-remotion-motion-shot-video-design.md) 完成低成本图片驱动营销视频主线：清理 HyperFrames/template video，接入 Remotion motion shot provider，让 Agent 能用 Seedream 图片、火山音频、Remotion silent shot 和 Composer 生成最终口播广告。

## Codex Goal 建议

按照 Remotion design 文档逐阶段完成 plan 编写、代码开发和验收。每个阶段都必须先有可交付标准和可验收标准；当前阶段通过测试、smoke 或人工可复核证据后，才能进入下一阶段。全部完成后必须用浏览器 E2E 创建真实营销视频，验证不调用 Seedance，使用 Seedream 图片、Remotion motion shot、火山音频和 Composer，并读取 DB 与日志评估 Agent 会话、skill 和工具调用是否符合预期。

## 已确认口径

- 不保留 HyperFrames 作为 fallback 或 legacy 主线。
- 新低成本视频路线是 `motion_shot_video`，provider 为 `internal_motion_video/remotion-motion-shot-v1`。
- Motion shot 只生成 silent shot video；完整口播、字幕、BGM 和音画同步由 Composer final timeline 统一处理。
- Agent 不直接写任意 Remotion/React/CSS/JS，只写受控 motion plan JSON。
- 用户明确不调用 Seedance 时，`shot_video` 必须走 `motion_shot_video`；无法满足时 blocked。

## 阶段里程碑

| 阶段 | 里程碑 | 可交付标准 | 可验收标准 |
|---|---|---|---|
| M11.0 Plan 与验收门 | 写清阶段计划、交付标准和验收标准。 | `docs/milestones/m11-remotion-motion-shot-video.md` 与 implementation plan 存在；计划覆盖清理、profile、provider、sandbox、skills、Composer、E2E。 | `git diff --check` 通过；计划中没有未决占位；每个阶段都有独立验收命令或证据。 |
| M11.1 HyperFrames 清理与 Motion 路由基础 | 移除生产/provider/RenderPlan 层 HyperFrames/template route，新增 motion profile/capability。 | 迁移注册 `internal_motion_video/remotion-motion-shot-v1`；RenderPlan 支持 `motion_shot_video/image_to_motion_video`；生产 provider、sandbox render、RenderPlan profile 和 worker model mapping 不再引用 `internal_template_video/hyperframes-html/template_video`。 | 相关 Go 单测通过；`rg` 在 production、renderplan、templatevideo 和 sandbox template render 层无活跃 HyperFrames/template route；Agent prompt/fixture 残留在 M11.3 清理。 |
| M11.2 Remotion Provider 与 Sandbox 竖切 | 生产链路能把图片渲染为 silent motion shot MP4。 | `MotionShotProvider`、`sandbox.RenderMotionShot`、受控 Remotion renderer、sandbox image 依赖和 smoke 脚本完成。 | 真实 sandbox smoke 生成 3-5 秒 MP4；DB 中有 `generation_job.provider=internal_motion_video`、provider request/response、sandbox job、artifact winner。 |
| M11.3 Agent Skill 与 no-Seedance 路由 | Agent 使用 motion shot skill 和 route policy。 | 新增 `motion-shot-producer/craftsman/reviewer`；删除 template-video skills；Producer/Craftsman/Reviewer prompts 与 dispatch defaults 改为 motion route。 | Agent fixture 单测证明 no-Seedance 不推荐 Seedance 或 template video；skill trace 加载 motion skills；工具调用参数使用 `motion_shot_video/image_to_motion_video`。 |
| M11.4 Composer 音画同步 | Final composition 统一处理 voiceover、字幕、BGM 和 shot 拼接。 | Composer 接收 motion shot winners、voiceover/BGM、caption segments；字幕 overlay 与 audio ducking 规则明确。 | Composer 单测覆盖 duration/audio/caption alignment；final MP4 duration 与 voiceover 误差小于 300ms 或有明确尾部留白；字幕不跨错 shot。 |
| M11.5 浏览器 E2E 与 DB/日志审计 | 用真实浏览器和真实素材生成营销视频并审计链路。 | 浏览器创建 Agent workspace，上传商品图，生成悦行行李箱口播广告；读取 DB 和日志。 | 无 Seedance generation job；有 Seedream 图片、Remotion motion shot、火山音频、Composer final video；Agent 会话、skill、工具调用符合设计；最终视频可下载并经 `ffprobe` 校验。 |

## 完成定义

- 当前代码中没有活跃 HyperFrames/template video route。
- no-Seedance 低成本营销视频主线使用 Seedream + Remotion motion shot + 火山音频 + Composer。
- 每个阶段都有新鲜验证输出。
- 浏览器 E2E、DB 查询、日志检查和最终媒体检查证明全链路符合预期。
