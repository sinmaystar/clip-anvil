# Agent Remotion 与 Seedance Workspace 运行追踪

本文基于当前开发库 `postgres://clipanvil:clipanvil_dev@localhost:5432/clipanvil` 的真实 DB 记录，追踪两个最新代表性 Agent workspace：

- Remotion 路线：`cec790fb-2691-4b15-b0ee-0ac86beb052e`，`悦行行李箱 motion sync E2E 2`
- Seedance 路线：`616c8488-026c-471f-b74c-3e294f1d59ed`，`行李箱`

重点表：

- `agent_message`：用户消息、系统提醒、工具调用 trace、assistant 输出。
- `agent_task`：Producer / Craftsman / Worker / Composer / Reviewer 的任务队列和状态。
- `agent_event`：角色之间的唤醒事件。
- `render_plan`：Craftsman 写出的生产计划。
- `generation_job`：Worker/Production 提交的真实模型或内部 provider 调用。
- `media_node`、`artifact_version`、`media_asset`：生成出的图片、音频、视频资产。
- `audio_plan`：全片旁白、BGM、cue_plan。
- `timeline_plan`：Composer 成片时间线。
- `review_record`：Reviewer 的结构化评审结果。

需要先说明一个边界：当前 DB 没有独立的 `agent_model_call` 或 LLM request 表。也就是说，Producer/Craftsman/Reviewer/Composer 的底层 LLM 每一次 request/response 并没有作为单独模型调用记录落库；能从 DB 直接看到的是 Agent 模型输出后的工具调用 trace、assistant 文本、任务状态和 provider `generation_job`。所以本文中的“模型调用”分两类：Agent LLM 调用只能从任务和工具 trace 间接还原，图片/视频/音频模型调用则由 `generation_job` 精确记录。

## 代码事实入口

当前 Agent 主路径不是旧线性工具执行器，而是 Eino-native 多 Agent 图：

- Producer 图：`producer_turn`
- Craftsman 图：`craftsman_render_plan`
- Reviewer 图：`reviewer_gate`
- Composer 图：`composer_timeline`
- Worker：后台 executor，没有 Eino graph，负责执行 `GenerationIntent`

核心代码位置：

- `apps/server/cmd/server/main.go`：装配 Producer、Craftsman、Reviewer、Composer、Worker 和 E2E fixture。
- `apps/server/internal/agent/producer/system_prompt.go`：Producer 系统提示，包含低成本 Remotion 路由、AudioPlan、Composer 调度规则。
- `apps/server/internal/agent/craftsman/system_prompt.go`：Craftsman 只能写 RenderPlan，并遵守 `video_route_policy=motion_only`。
- `apps/server/internal/agent/tools/upsert_render_plan.go`：RenderPlan 工具 schema、motion-only 校验、profile/operation 推导。
- `apps/server/internal/agent/tools/render_plan_submitter.go`：把 RenderPlan 映射为 `GenerationIntent`。
- `apps/server/internal/agent/renderplan/profiles.go`：`seedream_5_image`、`seedance_2_video`、`seed_audio_1`、`motion_shot_video` profile。
- `apps/server/internal/production/motion_shot_provider.go`：`internal_motion_video/remotion-motion-shot-v1` provider。
- `apps/server/internal/production/volcengine_video.go`：Seedance provider。
- `apps/server/internal/production/volcengine_image.go`：Seedream provider。
- `apps/server/internal/production/volcengine_audio.go`：`seed-audio-1.0` provider。
- `apps/server/internal/agent/composer/*` 与 `apps/server/internal/agent/tools/composer_native.go`：Composer 时间线和 ffmpeg/sandbox 成片工具。

## 两条路线的共同协作模型

```mermaid
flowchart TD
  User["用户消息 + 附件"] --> Producer["Producer"]
  Producer --> State["CreativeBrief / ProjectMemory / Storyboard / AudioPlan"]
  Producer --> Craftsman["dispatch_craftsman"]
  Craftsman --> RenderPlan["upsert_render_plan"]
  RenderPlan --> Worker["Worker generation"]
  Worker --> Production["production.Service / Provider"]
  Production --> Assets["image / video / audio artifacts"]
  Assets --> ProducerWake["producer_pending_signal"]
  ProducerWake --> Producer
  Producer --> Composer["dispatch_composer"]
  Composer --> Timeline["timeline_plan"]
  Timeline --> FinalVideo["final_video artifact"]
  Producer --> Reviewer["dispatch_reviewer"]
  Reviewer --> Review["review_record / artifact_issue"]
```

角色边界：

- Producer 是总控。它读用户需求和项目状态，沉淀 brief/memory/storyboard/audio plan，派发 Craftsman、Composer、Reviewer。
- Craftsman 不直接调用模型。它只把某个 scope 的创意需求翻译成 `render_plan`。
- Worker 不思考创意。它把 `render_plan` 变成 `GenerationIntent` 并调用 production/provider。
- Composer 负责最终时间线：拼接 shot video，混旁白/BGM，输出 final video。
- Reviewer 负责质量 gate。当前可写 `review_record`，但最终音频试听能力还不完整。

## Plan 家族：谁生成、根据什么生成、作用是什么

ClipAnvil 现在有多种 `plan`，它们不是同一层级的东西。最容易混淆的是：Producer 产出的是“创意和结构计划”，Craftsman 产出的是“单个素材的生产计划”，Worker/Production 产出的是“真实 provider 执行记录”，Composer 产出的是“最终剪辑时间线”。

| Plan / Record | 主要生成方 | 主要输入 | 核心作用 | 后续消费方 |
| --- | --- | --- | --- | --- |
| `CreativeBrief` | Producer | 用户消息、附件、workspace 上下文、系统 prompt、可用 skill / provider 能力 | 把用户需求固化成项目级商业目标和创意约束 | Producer 自己、ProjectMemory、Storyboard、AudioPlan、Craftsman |
| `ProjectMemory` | Producer | CreativeBrief、商品事实、素材事实、用户硬约束、已经生成的中间结果 | 作为跨 agent 的“创意宪法”，防止后续任务丢失品牌、商品和禁用模型等约束 | Producer、Craftsman、Reviewer、Composer |
| `Storyboard` | Producer | CreativeBrief、ProjectMemory、商品卖点、目标时长、平台比例、用户最新反馈 | 决定分镜数量、顺序、每个 shot 的意图、画面、时长和依赖 | Craftsman、AudioPlan、Composer、Reviewer |
| `AudioPlan` | Producer | Storyboard、CreativeBrief、ProjectMemory、口播风格、目标时长 | 规划整条视频的旁白、BGM、cue 分段和音画同步锚点 | Craftsman 生成 voiceover/BGM render plan，Composer 对齐时间线 |
| `render_plan` | Craftsman | Producer 派发的 scope、Storyboard/AudioPlan、ProjectMemory、已有素材和引用关系 | 把一个 shot、key element 或 audio scope 翻译成可执行的素材生成计划 | Worker / RenderPlanSubmitter |
| `GenerationIntent` | Worker / RenderPlanSubmitter | `render_plan`、目标 media node、输入素材、provider profile/model capability | 把 DB 里的 render plan 变成 production service 能执行的 provider 请求契约 | production.Service / provider |
| `generation_job` | production service | GenerationIntent、provider/model 返回结果 | 记录真实模型或内部 provider 调用，包含 provider、model、operation、状态、耗时、错误和产物 | Worker、Producer 唤醒、排障和成本统计 |
| `timeline_plan` | Composer | 已完成的 shot video、voiceover、BGM、AudioPlan cue、Storyboard、输出规格 | 规划最终剪辑：片段顺序、字幕、音轨、转场、输出尺寸和渲染参数 | sandbox/ffmpeg final render |
| `review_record` | Reviewer | final artifact、ProjectMemory、brief/storyboard/audio plan、review rubric | 记录质量 gate 判断和问题 | Producer 决定返工或交付 |

### 责任层级

- Producer 负责“为什么做、做给谁、讲什么、分几段讲”：`CreativeBrief`、`ProjectMemory`、`Storyboard`、`AudioPlan`。
- Craftsman 负责“某个素材怎么生产”：`render_plan`。
- Worker / Production 负责“真实怎么调用模型或内部渲染器”：`GenerationIntent`、`generation_job`、`artifact_version`。
- Composer 负责“素材怎么排到最终视频里”：`timeline_plan`。
- Reviewer 负责“成片是否达标”：`review_record`。

### CreativeBrief

`CreativeBrief` 是项目级商业合同，不直接对应任何一次模型调用。它的作用是把用户的一句话变成所有 agent 都能复用的项目目标。

在 Remotion workspace 里，Producer 根据用户明确要求生成 brief：`悦行行李箱 Remotion Motion Shot 口播广告`，类型是 `marketing_ad`，目标用户是短途出行和商务通勤用户，比例 `9:16`，目标时长 `34s`。最重要的约束是：不能调用 Seedance；允许 Seedream 生成图片；旁白和 BGM 用火山；视频用 Remotion 图片动效。

在 Seedance workspace 里，Producer 生成的是 `波尔盾行李箱开学/旅行电商短视频`，目标时长 `20s`，路线是 Seedream 预览图加 Seedance shot video。这里没有禁用 Seedance，反而要求用真实动态视频覆盖轮子、铝框锁扣、挂钩、干湿分离、尺寸颜色、热销卖点。

### ProjectMemory

`ProjectMemory` 是跨 agent 的长期上下文。它比 brief 更“项目内化”：保存商品事实、视觉锚点、不能违反的硬约束、已经确认的创意方向。它不是最终脚本，也不是单个素材 prompt。

Remotion workspace 的 ProjectMemory 里，核心意图是“用低成本 Remotion motion shot 快速生成悦行行李箱口播广告”。`non_negotiables` 写明：不要 Seedance；shot video 只能走 `motion_shot_video` / `internal_motion_video` / `remotion-motion-shot-v1`；必须使用 `box.png`；音频必须用火山。这个 memory 是后续 Craftsman 选择 provider 的关键依据。

Seedance workspace 的 ProjectMemory 则保存更详细的商品事实：波尔盾、铝框 PC 箱、竖纹箱体、金属包角、拉杆、静音轮、TSA/密码锁、挂钩、干湿分离、X 绑带、颜色、尺寸和服务信任。它的 `non_negotiables` 更偏向商品一致性和 20 秒内覆盖全卖点。

### Storyboard

`Storyboard` 是分镜结构计划，由 Producer 生成。它决定有多少 scene / shot、每个 shot 的目标、时长、画面内容、核心卖点和依赖关系。它不是素材生成请求；每个 shot 后面通常还需要一个或多个 `render_plan`。

Remotion workspace 的 storyboard 是 3 个 scene、5 个 shot、总时长 34 秒：

| Shot | 时长 | 核心意图 |
| --- | ---: | --- |
| `shot_01_hook` | 6s | 短途出行痛点钩子 |
| `shot_02_product` | 8s | 产品主视觉和轻便好推 |
| `shot_03_wheels` | 8s | 万向轮顺滑卖点 |
| `shot_04_storage` | 6s | 收纳空间和大周末出行 |
| `shot_05_cta` | 6s | CTA 和品牌收束 |

Seedance workspace 的 storyboard 是 4 个 scene、8 个 shot、总时长 20 秒：

| Shot | 时长 | 核心意图 |
| --- | ---: | --- |
| `shot_01` | 3s | 开学/旅行收拾混乱痛点 |
| `shot_02` | 2s | 产品主视觉 close-up |
| `shot_03` | 3s | 万向轮推行顺滑 |
| `shot_04` | 2s | 铝框和锁扣安全感 |
| `shot_05` | 2s | 挂钩功能 |
| `shot_06` | 3s | 干湿分离和收纳 |
| `shot_07` | 2s | 尺寸颜色选择 |
| `shot_08` | 3s | CTA |

这个差异说明：Agent 模式本来就支持非固定分镜数量。问题不在“是否模板固定”，而在后续 provider 能否按 shot 的语义生成相匹配的视觉素材。

### AudioPlan

`AudioPlan` 也是 Producer 生成的，但它不直接调用音频模型。它定义整条视频的声音结构：旁白文本、BGM 风格、语速、情绪、cue plan、每段对应哪个 shot。真正调用火山语音合成的是后续 Craftsman 为 `voiceover` scope 创建的 audio `render_plan`。

Remotion workspace 的 AudioPlan 是 34 秒、5 段 cue，和 5 个 shot 一一对齐：

| Cue | 时间 | 对应 shot | 用途 |
| --- | --- | --- | --- |
| 1 | 0-6s | `shot_01_hook` | 痛点钩子 |
| 2 | 6-14s | `shot_02_product` | 产品主视觉和轻便感 |
| 3 | 14-22s | `shot_03_wheels` | 万向轮顺滑 |
| 4 | 22-28s | `shot_04_storage` | 收纳空间 |
| 5 | 28-34s | `shot_05_cta` | CTA |

Seedance workspace 的 AudioPlan 是 20 秒、8 段 cue，基本和 8 个 shot 一一对应。这个结构更接近“先有分镜，再有口播，再按口播时间拼画面”的理想链路。

这也是之前视频失败的根因之一：如果字幕文本来自 shot 的内部目标，例如“短途出行痛点钩子”，而不是来自 voiceover/cue 的用户可见文案，就会把导演笔记渲染成字幕。字幕应该消费 AudioPlan 的 cue 文案或 TTS 返回的字幕时间戳，而不是消费 Storyboard 的 `intent` / `purpose`。

### RenderPlan

`render_plan` 是 Craftsman 的产物，是最接近 provider 的“素材生产计划”。它通常包含：

- `scope_type` / `scope_id`：计划属于哪个 shot、audio、scene 或 key element。
- `target_phase`：例如 `preview_image`、`shot_video`、`voiceover`、`bgm`、`key_element_ref`。
- `profile` / `operation`：使用哪个能力，例如 `seedream_5_image/image_to_image`、`seedance_2_video/image_to_video_first_frame`、`motion_shot_video/image_to_motion_video`、`seed_audio_1/text_to_audio`。
- `reference_bindings` / `subject_bindings`：输入图片、上一阶段产物、商品图、参考图。
- `prompt_parts` / `compiled_prompt`：面向 provider 的提示词。
- `params`：比例、分辨率、时长、fps、是否返回 last frame、字幕安全区、转场、文本层等。
- `rationale`：为什么这样生成。
- 输出目标：关联的 media node / artifact version。

两个 workspace 的 render plan 分布如下：

| Workspace | `key_element_ref` | `preview_image` | `shot_video` | `voiceover` | `bgm` |
| --- | ---: | ---: | ---: | ---: | ---: |
| Remotion | 0 | 10 | 5 | 3 | 1 |
| Seedance | 2 | 8 | 8 | 1 | 1 |

Remotion 的 `preview_image=10` 是因为修复和重跑产生了 r1/r2 两轮 preview。`voiceover=3` 里有一次重复 r1 触发唯一键冲突，随后 r2 成功。

### Remotion 路线里的 RenderPlan

Remotion workspace 的素材链路是：

1. Craftsman 为每个 shot 生成 `preview_image` render plan。
2. Worker 调用 Seedream 生成静态图。
3. Craftsman 为每个 shot 生成 `shot_video` render plan，引用对应 preview 图。
4. Worker 把 `motion_shot_video/image_to_motion_video` 映射到内部 Remotion provider：`internal_motion_video/remotion-motion-shot-v1`。
5. Craftsman 根据 AudioPlan 生成 `voiceover` 和 `bgm` render plan，Worker 调用火山 audio provider。
6. Composer 用 5 条 silent motion shot 加旁白/BGM 生成 `timeline_plan`。

典型例子是 `shot_03_wheels.shot_video.r1`：它来自 storyboard 的“万向轮顺滑”卖点，引用 `shot_03_wheels.preview_image`，profile 是 `motion_shot_video`，params 里包含 `fps=30`、`resolution=1080p`、`safe_area=caption_reserved_bottom`、`text_layers`、`transitions`。它应该只负责视觉动效，不负责生成旁白和最终字幕。

### Seedance 路线里的 RenderPlan

Seedance workspace 的素材链路是：

1. Craftsman 先生成部分 `key_element_ref`，例如宿舍收拾、通勤/出行场景参考。
2. Craftsman 为 8 个 shot 生成 `preview_image`，使用 Seedream 作为 first frame 或视觉锚点。
3. Craftsman 为 8 个 shot 生成 `shot_video`，profile 是 `seedance_2_video/image_to_video_first_frame`。
4. Worker 调用 Seedance 生成真实动态视频，并设置 `duration_sec=5`、`return_last_frame=true` 等参数。
5. Craftsman 生成 voiceover/BGM audio render plan。
6. Composer 生成带 fades 的最终 timeline。

典型例子是 `shot_03.shot_video.r1`：它来自 storyboard 的“万向轮推行顺滑”，引用对应 preview first frame，prompt 会要求轮子滚动、箱体被推动、镜头跟拍或低机位运动。这里运动能力来自 Seedance 模型，而不是 Remotion 的图片动效模板。

### GenerationIntent 和 GenerationJob

`GenerationIntent` 不是 Producer 或 Craftsman 写进 DB 的 plan，而是 Worker/RenderPlanSubmitter 从 `render_plan` 翻译出来的 Go 层执行契约。它会把 profile、operation、params、输入素材和目标 media node 组合成 production service 能理解的请求。

常见映射关系：

| RenderPlan profile / operation | 实际 provider / model |
| --- | --- |
| `seedream_5_image` / `image_to_image` | `volcengine/doubao-seedream-5-0-260128` |
| `seedance_2_video` / `image_to_video_first_frame` | `volcengine/doubao-seedance-2-0-260128` |
| `motion_shot_video` / `image_to_motion_video` | `internal_motion_video/remotion-motion-shot-v1` |
| `seed_audio_1` / `text_to_audio` | `volcengine/seed-audio-1.0` |

`generation_job` 是真实执行记录，不是计划。它能回答“到底有没有调用 Seedance”“实际用的是哪个 provider/model”“失败原因是什么”“产物是什么”。所以排查成本路由时，`generation_job` 比 storyboard 或 render plan 更接近事实。

### TimelinePlan

`timeline_plan` 是 Composer 的产物。它消费已经完成的 shot video、voiceover、BGM 和 AudioPlan cue，生成最终剪辑结构。它通常包含：

- `template_key`：当前使用的剪辑策略，例如 `simple_concat` 或 `concat_with_fades`。
- 输出规格：宽高、fps、总时长。
- segments：每段视频素材的开始结束时间、转场、裁剪/缩放方式。
- audio tracks：voiceover、BGM、音量、淡入淡出。
- captions / text overlays：字幕文本、时间段、安全区。
- render settings：ffmpeg/sandbox 所需参数。

Remotion workspace 里有 2 条 completed `simple_concat` timeline plan，输出 `1080x1920`、`24fps`，使用 5 个 Remotion motion shots 加 voiceover/BGM。它的问题是：视觉素材和 cue 没有真正一一语义匹配，且字幕来源曾混入导演笔记。

Seedance workspace 里有 1 条 completed `concat_with_fades` timeline plan，输出 `496x864`、`24fps`，总时长约 `20.3s`，使用 8 个 Seedance shot videos 加 voiceover/BGM。它的视频语义更依赖 Seedance 自身生成能力，成本更高，但单 shot 的真实运动表达更强。

### Plan 依赖图

```mermaid
flowchart TD
  User["用户消息 + 附件"] --> Brief["CreativeBrief"]
  Brief --> Memory["ProjectMemory"]
  Brief --> Storyboard["Storyboard"]
  Memory --> Storyboard
  Storyboard --> AudioPlan["AudioPlan"]
  Memory --> AudioPlan

  Storyboard --> ImageRP["preview/key image render_plan"]
  Memory --> ImageRP
  ImageRP --> ImageJob["Seedream generation_job"]
  ImageJob --> ImageAsset["image artifact"]

  Storyboard --> VideoRP["shot_video render_plan"]
  ImageAsset --> VideoRP
  Memory --> VideoRP
  VideoRP --> Intent["GenerationIntent"]
  Intent --> VideoJob["Seedance or Remotion generation_job"]
  VideoJob --> ShotVideo["shot video artifact"]

  AudioPlan --> AudioRP["voiceover/BGM render_plan"]
  AudioRP --> AudioJob["Seed audio generation_job"]
  AudioJob --> AudioAsset["audio artifact"]

  ShotVideo --> Timeline["timeline_plan"]
  AudioAsset --> Timeline
  AudioPlan --> Timeline
  Timeline --> Final["final video artifact"]
```

### 三个容易混的点

1. `AudioPlan` 不是音频模型调用。它是声音结构和 cue contract；音频模型调用发生在 voiceover/BGM `render_plan` 被 Worker 执行之后。
2. `Storyboard shot` 不是素材生成 prompt。它是导演层的结构和意图；真正给 Seedream/Seedance/Remotion/audio provider 的是 Craftsman 写出的 `render_plan.compiled_prompt` 和 `params`。
3. `generation_job` 不是 plan。它是执行事实；要判断有没有调用 Seedance、用没用火山 TTS、Remotion 是否真的参与，最终看 `generation_job` 和产物记录。

## Remotion Workspace：悦行行李箱 motion sync E2E 2

Workspace：`cec790fb-2691-4b15-b0ee-0ac86beb052e`

用户原始消息：

> 用这张图生成一个 30 秒以上的悦行行李箱口播广告。不要调用 Seedance；图片可以用 Seedream；旁白和 BGM 用火山；视频用 Remotion 图片动效；需要多分镜、转场、字幕和最终成片。

附件：`box.png`

### 总量

- Producer task：34 个 succeeded。
- Craftsman task：22 个 succeeded。
- Worker task：18 个 succeeded，1 个 failed。
- Composer task：2 个 succeeded。
- Reviewer task：0 个。
- `generation_job`：
  - Seedream 图片：10 个，均 succeeded。
  - Remotion motion shot：5 个，均 succeeded。
  - 火山 `seed-audio-1.0`：3 个，均 succeeded。
  - `internal_ffmpeg/ffmpeg` 成片：2 个，均 succeeded。
  - Seedance：0 个。

注意：这个 workspace 后段在修复期间出现了一次 14:51 的 preview image r2 重新生成，所以 Seedream 图片总数是 10。主流程第一次成片在 14:20，修复后再次成片在 14:56。两次成片都没有 Seedance。

### 时间线

1. 13:19:37，用户上传 `box.png` 并提出 no-Seedance Remotion 广告需求。
2. 13:19:37，Producer 写入 `CreativeBrief`：
   - 标题：悦行行李箱 Remotion Motion Shot 口播广告。
   - 约束：禁止调用 Seedance；视频必须使用 `internal_motion_video/motion_shot_video`；图片可用 Seedream；音频用火山。
3. 13:19:37，Producer 写入 `ProjectMemory`：
   - `route_policy=motion_shot_video_no_seedance_video`
   - `image_policy=seedream_allowed`
   - `audio_policy=volcengine_tts_required`
   - `non_negotiables` 里明确禁止 Seedance 视频模型。
4. 13:19:37，Producer 创建 5 个 shot 的 storyboard，总目标 34 秒：
   - `shot_01_hook`，6 秒，短途出行痛点钩子。
   - `shot_02_product`，8 秒，产品展示。
   - `shot_03_wheels`，8 秒，顺滑万向轮卖点。
   - `shot_04_storage`，6 秒，短途收纳。
   - `shot_05_cta`，6 秒，CTA。
5. 13:19:37，Producer 创建并批准 AudioPlan 初版：
   - cue_plan 5 段，对应 5 个 shot。
   - voiceover script 与 shot 顺序一致。
   - BGM 指向 `seed-audio-1.0`。
6. 13:19:37，Producer 派发 5 个 `preview_image` Craftsman：
   - 每个任务都要求基于 `box.png` 先用 Seedream 生成商业主视觉。
   - `execution_policy=execute_immediately`，所以 Craftsman 写完 RenderPlan 后直接进入 Worker。
7. 13:19:37，Craftsman 写 5 个 `seedream_5_image` RenderPlan：
   - profile：`seedream_5_image`
   - operation：`image_to_image`
   - target_phase：`preview_image`
8. 13:19:37 至 13:19:38，Worker/Production 调用 Seedream：
   - provider：`volcengine`
   - model：`doubao-seedream-5-0-260128`
   - 输出 5 张 preview image。
9. 13:21:31，Producer 派发 5 个 `shot_video` Craftsman，并带上 no-Seedance 路由策略。
10. 13:21:31，Craftsman 写 5 个 Remotion RenderPlan：
    - profile：`motion_shot_video`
    - operation：`image_to_motion_video`
    - target_phase：`shot_video`
    - rationale 中写明 `video_route_policy=motion_only 禁止 Seedance`。
11. 13:21:31，Worker/Production 调用 Remotion motion provider：
    - provider：`internal_motion_video`
    - model：`remotion-motion-shot-v1`
    - 输出 5 段 silent `shot_video` mp4。
12. 13:33:47，Producer 派发 voiceover audio：
    - profile：`seed_audio_1`
    - operation：`text_to_audio`
    - provider/model：`volcengine/seed-audio-1.0`
    - 输出旁白 r1，但后续发现时长/同步不足，音频方案被改写。
13. 14:11:27，重新提交 voiceover r1 时 Worker 失败：
    - 错误：`duplicate key value violates unique constraint "idx_media_node_workspace_semantic"`。
    - 这说明语义 key 复用导致目标 media node 冲突。
14. 14:17:43，Producer/Craftsman 改用 `voiceover_audio.r2`，Worker 成功生成新的火山旁白。
15. 14:18:40，Producer/Craftsman 生成 BGM：
    - profile：`seed_audio_1`
    - operation：`text_to_audio`
    - prompt：34 秒轻快电子流行 BGM，无人声，弱鼓点，不抢旁白。
16. 14:19:21，Producer 派发 Composer：
    - 指令：把 5 段 Remotion motion shot、voiceover、BGM 合成为 34 秒 final video。
    - 明确禁止 Seedance。
17. 14:19:31，Composer 写 `timeline_plan`：
    - template：`simple_concat`
    - 输出：1080x1920，24fps，mp4，AAC。
    - segments：5 个 shot，使用 cue text 作为 caption。
18. 14:20:02，Composer 通过 `internal_ffmpeg/ffmpeg` 生成第一次 final video。
19. 14:51:08，修复期间又触发 5 张 preview image r2 的 Seedream 生成。
20. 14:55:25，Producer 再次派发 Composer。
21. 14:56:02，Composer 生成第二个 final video。

### Remotion Producer 创建的 CreativeBrief

Producer 写入的 `creative_brief.main` 是这个 workspace 的商业目标合同。字段内容如下：

| 字段 | 值 |
|---|---|
| `title` | 悦行行李箱 Remotion Motion Shot 口播广告 |
| `video_type` | `marketing_ad` |
| `target_audience` | 短途出行和商务通勤用户 |
| `tone` | 轻快、可信、现代 |
| `aspect_ratio` | `9:16` |
| `duration_sec` | `34` |
| `language` | `zh-CN` |
| `objective` | 用低成本 motion shot 突出悦行行李箱轻便、顺滑万向轮、短途出行和安心托运，并合成旁白音频 |
| `concept` | 以用户上传的行李箱图片为商品参考，先用 Seedream 生成商业主视觉图片，再通过 Remotion 图片动效、文字卖点、TTS 旁白和轻快 BGM 完成短广告 |
| `visual_style` | Seedream 先为每个分镜生成商业广告主视觉/背景图，Remotion 再做 5 个动态 motion shot：痛点钩子、产品展示、万向轮卖点、短途出行情境、CTA；每段都有不同布局、转场和文字安全区 |

`constraints` 是这条路线最关键的部分：

| severity | text |
|---|---|
| blocking | 禁止调用 Seedance 或任何 volcengine/seedance 视频模型 |
| blocking | 视频必须使用 Remotion/internal_motion_video/motion_shot_video |
| blocking | 必须使用用户上传的 box.png 作为产品参考 |
| high | 允许并鼓励使用 Seedream 生成图片资产，允许使用火山音频模型生成旁白和 BGM |

`metadata.brief` 进一步把需求写死为：基于 `box.png` 创建 34 秒 9:16 口播广告，图片可用 Seedream，音频可用火山 TTS/BGM，视频只使用 Remotion motion shot。这个字段是后续 Producer/Craftsman 不应改走 Seedance 的依据之一。

### Remotion Producer 创建的 ProjectMemory

`project_memory` 有两个版本。v1 后来归档，v2 为 active，但两者内容一致，说明这次不是重新创意，而是版本状态调整。

核心字段：

| 字段 | 值 |
|---|---|
| `core_intent` | 用低成本 Remotion motion shot 快速生成悦行行李箱口播广告 |
| `soul` | 轻装出发、顺滑好推、安心托运 |
| `brand_facts.product_name` | 悦行行李箱 |
| `brand_facts.route_policy` | `motion_shot_video_no_seedance_video` |
| `brand_facts.image_policy` | `seedream_allowed` |
| `brand_facts.audio_policy` | `volcengine_tts_required` |
| `visual_anchors.product_image` | 用户上传的 `box.png` 行李箱产品图 |
| `visual_anchors.seedream_hero_image` | 用 `box.png` 生成商业主视觉：清爽旅行广告背景，商品居中，留出字幕空间 |
| `visual_anchors.motion_style` | 9:16 竖版，Seedream 主视觉轻微推进，清爽字幕卖点和 CTA |

`non_negotiables`：

- 禁止调用 Seedance 或 volcengine/seedance 视频模型。
- `shot_video` 只能使用 `motion_shot_video / internal_motion_video / remotion-motion-shot-v1`。
- 使用 `box.png` 作为商品参考，并先生成 Seedream 主视觉图片。
- 旁白和 BGM 必须使用火山音频模型，不能把 mock 音频当作真实通过。

`prompt_injection_hints`：

- `Seedream images allowed`
- `motion_shot_video only for video`
- `Remotion internal_motion_video`
- `不要调用 Seedance`

这解释了为什么 Craftsman 后续读取 ProjectMemory 后，所有 `shot_video` 都应该写成 `motion_shot_video`。

### Remotion Storyboard 具体内容

Producer 创建了 3 个 scene：

| sort | scene | title | location | mood | description |
|---:|---|---|---|---|---|
| 1 | `scene_motion_ad_intro` | 痛点与产品建立 | Remotion 图文动效画面 | 清爽、轻快、可信 | 以短途出行痛点开场，再建立悦行行李箱商品主体 |
| 2 | `scene_motion_ad_benefit` | 卖点与出行情境 | Remotion 图文动效画面 | 明快、可信、行动感 | 解释万向轮、轻便、省力和短途出行收益 |
| 3 | `scene_motion_ad_outro` | CTA 收束 | Remotion 图文动效画面 | 明亮、明确、利落 | 品牌口号和行动按钮收束 |

5 个 shot：

| sort | shot | scene | duration | kind | narrative purpose | visual intent | action text | camera intent | narration |
|---:|---|---|---:|---|---|---|---|---|---|
| 1 | `shot_01_hook` | intro | 6 | `hook_card` | 前三秒抓住短途出行用户注意 | 深色干净背景，行李箱位于中上，顶部短 hook | 产品图慢推近，痛点文字弹出 | Remotion slow push in，无真实复杂运动 | 短途出行，最怕箱子沉、转弯卡，一路拖得很狼狈 |
| 2 | `shot_02_product` | intro | 8 | `product_hero` | 让用户记住商品主体和品牌名 | 商品大图居中，品牌标题和轻便卖点分层 | 商品轻微浮起，品牌名淡入 | 轻微视差和中心聚焦 | 悦行行李箱采用轻量硬壳和顺滑手感，从地铁口到酒店前台都推得更稳 |
| 3 | `shot_03_wheels` | benefit | 8 | `benefit_card` | 证明核心功能卖点 | 底部万向轮超近景特写，轮组占画面主要区域，箱体只露出下沿，不要完整商品大图 | 三条卖点逐条入场 | 稳定信息卡，轻微横向漂移 | 底部万向轮顺滑转向，转弯不抢手，赶车换乘也更省力 |
| 4 | `shot_04_storage` | benefit | 6 | `scenario_card` | 把功能转成用户生活收益 | 打开的行李箱内景，衣物、电脑、洗漱包分区摆放整齐 | 收纳分区标签轻微滑入 | 柔和拉远，保留字幕安全区 | 两三天换洗衣物、电脑和洗漱包分区放好，打开就能快速拿取 |
| 5 | `shot_05_cta` | outro | 6 | `cta_card` | 促进行动 | 商品居中，按钮式 CTA 位于下方安全区 | CTA 按钮弹出，背景渐亮 | 轻微拉远后定格 | 周末旅行、商务通勤、短途回家，一个箱子装下刚刚好的从容。悦行行李箱，现在出发 |

### Remotion AudioPlan 具体内容

Producer 先创建 34 秒 AudioPlan，再在修复过程中扩写了 voiceover。最终 active plan：

| 字段 | 值 |
|---|---|
| `title` | 悦行行李箱 34 秒口播音频 |
| `target_duration_sec` | 34 |
| `status` | `composing` |
| `voice_profile` | 清爽、可信、轻快，`warm_female` |
| `bgm_plan` | 轻快电子流行，无人声，弱鼓点，明亮但不抢旁白 |
| `generation_params` | `format=mp3`，`sample_rate=48000`，`speech_rate=0.98` |

cue_plan：

| time | shot_ref | text |
|---|---|---|
| 0-6 | `shot_01_hook` | 短途出行，最怕箱子沉、转弯卡，一路拖得很狼狈；地铁换乘和酒店大厅，每一步都怕被行李拖慢 |
| 6-14 | `shot_02_product` | 悦行行李箱采用轻量硬壳和顺滑手感，从地铁口到酒店前台都推得更稳 |
| 14-22 | `shot_03_wheels` | 底部万向轮顺滑转向，转弯不抢手，狭窄通道也能轻松掉头，赶车换乘更省力 |
| 22-28 | `shot_04_storage` | 两三天换洗衣物、电脑和洗漱包分区放好，拉链网袋一眼看清，打开就能快速拿取 |
| 28-34 | `shot_05_cta` | 周末旅行、商务通勤、短途回家，一个箱子装下刚刚好的从容。悦行行李箱，现在出发 |

这里的 cue_plan 是 Remotion 路线的音画同步合同：每个时间窗绑定一个真实 shot，Composer 使用这些 caption 和 duration 拼时间线。

### Remotion Craftsman 写出的 RenderPlan

Craftsman 总共写了 19 条 RenderPlan：5 条 preview image r1、5 条 motion shot、1 条 voiceover r1、1 条失败 voiceover r1 重试、1 条 voiceover r2、1 条 BGM、5 条 preview image r2。主流程里最关键的是前 12 条。

#### Seedream preview image plans

共同结构：

- `target_phase=preview_image`
- `model_prompt_profile=seedream_5_image`
- `operation=image_to_image`
- `params={"ratio":"9:16","resolution":"2K","max_images":1}`
- `reference_bindings` 都绑定用户上传 `box.png`，`model_role=reference_image`。
- rationale 都写明：先生成广告质感静态主视觉，再交给 Remotion 做低成本视频合成，不调用 Seedance。

| render_plan | shot | prompt 目标 |
|---|---|---|
| `shot_01_hook.preview_image.r1` | `shot_01_hook` | 深色干净背景，行李箱位于中上，顶部短 hook；保持银灰硬壳、竖向纹理、四个万向轮 |
| `shot_02_product.preview_image.r1` | `shot_02_product` | 商品大图居中，品牌标题和轻便卖点分层；商品位于中上部，下方保留文字安全区 |
| `shot_03_wheels.preview_image.r1` | `shot_03_wheels` | 底部万向轮超近景特写，轮组占画面 55% 以上，箱体只露出下沿；服务顺滑转向卖点 |
| `shot_04_storage.preview_image.r1` | `shot_04_storage` | 打开的银灰色行李箱内景，衣物、电脑、洗漱包分区摆放整齐 |
| `shot_05_cta.preview_image.r1` | `shot_05_cta` | 商品居中，按钮式 CTA 位于下方安全区，品牌收束画面 |

这一步的 Craftsman 产物是图片生成计划，不是视频计划；Worker 才把这些计划提交给 Seedream。

#### Remotion motion shot plans

共同结构：

- `target_phase=shot_video`
- `model_prompt_profile=motion_shot_video`
- `operation=image_to_motion_video`
- `provider/model` 由提交器映射成 `internal_motion_video/remotion-motion-shot-v1`
- `reference_bindings` 绑定对应 shot 的 preview image。
- `params.ratio=9:16`
- `params.resolution=1080p`
- `params.fps=30`
- `params.safe_area=caption_reserved_bottom`
- `params.text_layers` 只放短 hook/benefit，不放完整口播字幕。
- `params.transitions` 定义入场/出场动效。
- rationale 明确：该 shot 是卖点卡/CTA 型产品图轻动效，符合 `motion_shot_video` 能力边界；`video_route_policy=motion_only` 禁止 Seedance。

| render_plan | duration | input image | text_layers | transitions |
|---|---:|---|---|---|
| `shot_01_hook.shot_video.r1` | 6 | `shot_01_hook.preview_image.r1.node` | `短途出行` 0.2-2.2s，`别让行李箱拖后腿` 2.2-5.7s | `in=soft_zoom`，`out=swipe_up` |
| `shot_02_product.shot_video.r1` | 8 | `shot_02_product.preview_image.r1.node` | `悦行行李箱` 0.2-2.2s，`轻便好推，短途省心` 2.2-7.7s | `in=fade`，`out=fade` |
| `shot_03_wheels.shot_video.r1` | 8 | `shot_03_wheels.preview_image.r1.node` | `顺滑万向轮` 0.2-2.2s，`转向更稳，推行更省力` 2.2-7.7s | `in=slide_left`，`out=slide_right` |
| `shot_04_storage.shot_video.r1` | 6 | `shot_04_storage.preview_image.r1.node` | `轻松收纳` 0.2-2.2s，`分区放好，快速拿取` 2.2-5.7s | `in=soft_zoom`，`out=fade` |
| `shot_05_cta.shot_video.r1` | 6 | `shot_05_cta.preview_image.r1.node` | `现在出发` 0.2-2.2s，`悦行行李箱` 2.2-5.7s | `in=fade`，`out=hold` |

每条 motion shot 的 `compiled_prompt` 都有一段共同约束：

```text
只输出无声 shot_video，完整旁白、BGM、底部字幕同步交给 Composer。
不要把 shot title、narrative_purpose、visual_intent、action_text 这类内部导演说明写进画面文字。
禁止调用 Seedance；video_route_policy=motion_only。
```

这正是后来修复“把导演说明写成字幕”的关键规则。

#### Remotion audio RenderPlans

| render_plan | target_phase | profile | operation | status | params | 说明 |
|---|---|---|---|---|---|---|
| `audio_plan.active.voiceover_audio.r1` | `voiceover_audio` | `seed_audio_1` | `text_to_audio` | succeeded | `format=mp3`，`speaker=warm_female`，`sample_rate=48000`，`speech_rate=0.98` | 初版旁白，脚本较短 |
| `audio_plan.active.voiceover_audio.r1` | `voiceover_audio` | `seed_audio_1` | `text_to_audio` | failed | 同上 | 修订脚本后仍使用 r1 semantic key，Worker 创建 media node 时撞唯一键失败 |
| `audio_plan.active.voiceover_audio.r2` | `voiceover_audio` | `seed_audio_1` | `text_to_audio` | succeeded | 同上 | 修订后的旁白，使用 r2 避免 semantic key 冲突 |
| `audio_plan.active.bgm_audio.r1` | `bgm_audio` | `seed_audio_1` | `text_to_audio` | succeeded | `format=mp3`，`sample_rate=48000` | 34 秒轻快电子流行 BGM，无人声，弱鼓点，给旁白留空间 |

### Remotion 路线里的工具调用

`agent_message` 中工具 trace 计数：

| 工具 | 次数 | 作用 |
|---|---:|---|
| `read_project_memory` | 22 | Craftsman 读取 ProjectMemory 和 no-Seedance 约束 |
| `upsert_render_plan` | 22 | Craftsman 写图片、视频、音频 RenderPlan |
| `dispatch_craftsman` | 10 | Producer 派发 5 个 preview image、5 个 shot video |
| `upsert_audio_plan` | 6 | Producer 创建、批准、修订音频方案 |
| `dispatch_composer` | 2 | Producer 派发两次 Composer |
| `get_composition_context` | 2 | Composer 读取可合成素材 |
| `stage_media_inputs` | 2 | Composer staging 输入媒体 |
| `create_timeline_plan` | 2 | Composer 写 timeline_plan |
| `render_timeline_template` | 2 | Composer 渲染 timeline |
| `submit_composition_artifact` | 2 | Composer 提交 final video artifact |
| `upsert_project_brief` | 2 | Producer 写 brief |
| `update_project_memory` | 2 | Producer 写 memory |
| `upsert_storyboard` | 2 | Producer 写 storyboard |

### Remotion 路线的媒体资产

核心输出：

- 1 个用户上传商品图：`box.png`
- 5 张 Seedream preview image r1
- 5 段 Remotion silent shot video r1
- 1 条 voiceover audio r1
- 1 条 voiceover audio r2
- 1 条 BGM audio r1
- 2 个 final video
- 5 张 Seedream preview image r2，属于后段修复期间重跑

最关键的事实是：所有 `shot_video` 的 `generation_job.model_id` 都是 `remotion-motion-shot-v1`，没有任何 `seedance`。

## Seedance Workspace：行李箱

Workspace：`616c8488-026c-471f-b74c-3e294f1d59ed`

用户原始消息：

> 请基于我上传的波尔盾行李箱商品图，制作一条 20 秒竖版营销短视频广告，比例 9:16，风格年轻、干净、快节奏、电商转化导向，适合抖音/小红书信息流。

附件：8 张波尔盾行李箱商品图，`baorerden-suitcase-02.png` 到 `baorerden-suitcase-09.png`。

### 总量

- Producer task：24 个 succeeded，1 个 decision resume，1 个 waiting_for_user。
- Craftsman task：20 个 succeeded。
- Worker task：20 个 succeeded。
- Composer task：1 个 succeeded。
- Reviewer task：1 个 succeeded。
- `generation_job`：
  - Seedream 图片：10 个，均 succeeded。
  - Seedance 视频：8 个，均 succeeded。
  - 火山 `seed-audio-1.0`：2 个，均 succeeded。
  - `internal_ffmpeg/ffmpeg` 成片：1 个，succeeded。

### 时间线

1. 09:00 到 09:04，用户上传 8 张商品图。
2. 09:05:18，用户提出 20 秒 9:16 波尔盾行李箱电商广告需求。
3. 09:05:24，Producer 调用 `load_agent_skill(commerce-ad-producer)`：
   - skill 要求把用户需求沉淀为 CreativeBrief、ProjectMemory、KeyElement、Storyboard、AudioPlan。
   - 明确 Producer 不直接写最终 Seedream/Seedance/ffmpeg prompt，不直接提交 generation job。
4. 09:05 到 09:08，Producer 读取上下文，创建 `CreativeBrief` 和 `ProjectMemory`：
   - 创意主题：《一个箱子，解决开学/旅行前的混乱》。
   - 核心卖点：静音万向轮、铝框锁具、前置挂钩、干湿分离、多尺寸多颜色、热销信任背书。
5. 09:08:29，Producer 创建 8 个 shot：
   - `shot_01`，3 秒，混乱痛点开场。
   - `shot_02`，2 秒，产品登场。
   - `shot_03`，3 秒，静音轮顺滑出发。
   - `shot_04`，2 秒，铝框锁具安全感。
   - `shot_05`，2 秒，前置挂钩便利。
   - `shot_06`，3 秒，干湿分离超能装。
   - `shot_07`，2 秒，多尺寸多颜色。
   - `shot_08`，3 秒，热销 CTA 收尾。
6. 09:08:52，Producer 创建 20 秒 AudioPlan：
   - cue_plan 8 段，逐段绑定 `shot_ref`。
   - 因需要用户确认，状态先到 `waiting_for_user`。
7. 09:09:57，用户确认方案，Producer 通过 `decision_resume` 继续。
8. 09:10:42，Producer 派发 2 个 key element reference image Craftsman：
   - 宿舍整理场景锚点。
   - 干净通行空间锚点。
9. 09:11:17 到 09:11:48，Worker/Production 调用 Seedream 生成 2 张 reference image：
   - 一个 `image_to_image`
   - 一个 `text_to_image`
10. 09:12:46，Producer 派发 8 个 shot preview image Craftsman。
11. 09:13:40 到 09:15:39，Worker/Production 调用 Seedream 生成 8 张 preview image：
    - 4 个 `image_to_image`
    - 3 个 `multi_image_to_image`
    - 1 个 `text_to_image`
12. 09:16:41，Producer 派发 audio_plan voiceover/BGM Craftsman。
13. 09:17:16，Worker/Production 调用 `seed-audio-1.0` 生成 voiceover。
14. 09:17:17 左右，Producer 派发 8 个 `shot_video` Craftsman。
15. 09:17:47 到 09:18:31，Craftsman 写 8 个 Seedance RenderPlan：
    - profile：`seedance_2_video`
    - operation：`image_to_video_first_frame`
    - 每个分镜都引用对应 preview image 作为首帧。
16. 09:18:16 到 09:18:57，Worker/Production 调用 Seedance：
    - provider：`volcengine`
    - model：`doubao-seedance-2-0-260128`
    - 输出 8 段 shot video。
17. 09:18:17，Worker/Production 调用 `seed-audio-1.0` 生成 BGM。
18. 09:41:57，Producer 派发 Composer：
    - source refs：8 个 shot video、voiceover、BGM。
    - template：`concat_with_fades`。
19. 09:43:56，Composer 写 `timeline_plan`：
    - 496x864，24fps，20.3 秒。
    - 8 shots 按 story order 拼接，hard cuts + subtle fades。
    - voiceover primary，BGM bed ducked low。
20. 09:46:17，Composer 通过 `internal_ffmpeg/ffmpeg` 生成 final video。
21. 09:51:09，Producer 派发 Reviewer 做 final video review。
22. 09:53:56，Reviewer 提交 `review_record`：
    - review_task：`final_video_review`
    - status：`blocked`
    - overall_score：0.55
    - 原因：生产链路完整，但 Reviewer 当前无法直接播放或解析 mp4 音轨，无法真实判断 audio sync、BGM ducking、相对音量和节奏连续性。
23. 09:54:02，Producer 进入 `waiting_for_user`，等待用户处理最终音频专项确认。

### Seedance Producer 创建的 CreativeBrief

Producer 写入的 `creative_brief.main`：

| 字段 | 值 |
|---|---|
| `title` | 波尔盾行李箱开学/旅行电商短视频 |
| `video_type` | `marketing_ad` |
| `target_audience` | 学生、年轻上班族、短途旅行与开学出行人群，偏好清爽高颜值、高性价比出行装备 |
| `tone` | 年轻、干净、快节奏、轻松有秩序感、电商转化导向 |
| `aspect_ratio` | `9:16` |
| `duration_sec` | `20` |
| `language` | `zh-CN` |
| `objective` | 通过“一个箱子解决开学/旅行前的混乱”的创意，快速展示波尔盾行李箱静音顺滑、铝框安全、前置挂钩、干湿分离、多尺寸多颜色等卖点，激发学生和年轻上班族购买兴趣并引导转化 |
| `concept` | 《一个箱子，解决开学/旅行前的混乱》：前 3 秒用出行前杂乱、手忙脚乱的痛点抓眼，随后用顺滑移动、结实铝框、方便挂钩、整齐收纳和多色选择快速建立“一箱搞定”的爽感，最后以热销背书和行动号召收尾 |
| `visual_style` | 抖音/小红书信息流竖版视觉；明亮干净自然光与浅色系场景；产品质感高级但不高冷；快切镜头配合清晰卖点字幕 |

`constraints`：

| severity | text |
|---|---|
| blocking | 视频比例必须为 9:16 竖版，总时长约 20 秒 |
| high | 风格必须年轻、干净、快节奏，避免老气、厚重、廉价电商感 |
| high | 核心卖点需覆盖静音万向轮、铝框箱体/密码锁、前置挂钩、干湿分离、多尺寸、多颜色、热销高性价比 |
| medium | 价格不能成为唯一卖点，热销 8 万+、高性价比只作为信任背书 |
| blocking | 商品外观需基于用户上传的波尔盾素材，保持竖条纹铝框 PC 箱体、金属包角、TSA/密码锁、万向轮等特征一致 |

这个 brief 没有限制 Seedance，且强调“快节奏短视频、顺滑移动、动作和场景”，所以后续 shot_video 默认进入 Seedance。

### Seedance Producer 创建的 ProjectMemory

核心字段：

| 字段 | 值 |
|---|---|
| `core_intent` | 为波尔盾行李箱制作一条 20 秒 9:16 竖版电商转化短视频，用“一个箱子解决开学/旅行前的混乱”的创意，快速打动学生和年轻上班族，突出高颜值、顺滑、安全、能装、好整理和高性价比 |
| `soul` | 年轻清爽、秩序感强、节奏利落；从“出行前混乱”快速切换到“一箱搞定”的轻松爽感 |
| `brand_facts.brand_name` | 波尔盾 / Baoerden |
| `brand_facts.product_category` | 铝框 PC 旅行箱/行李箱 |
| `brand_facts.product_design` | 竖条纹箱体、金属包角、加粗合金拉杆、静音万向轮、TSA/密码锁、前置挂钩、干湿分离收纳、X 型束衣带 |
| `brand_facts.color_range` | 白色、浅绿、浅蓝、黑色、灰、奶黄、紫等年轻清爽配色 |
| `brand_facts.size_range` | 覆盖 3-4 天短途到 21 天以上长途出行 |
| `brand_facts.service_trust` | 热销 8 万+、高性价比、顺丰包邮、终身免费保修、赠送定制箱套等 |

`visual_anchors`：

- `hero_product_white`：主视觉优先白色款波尔盾铝框行李箱，奶白竖条纹箱体、银色铝框和包角、白色/黑色静音轮、顶部提手和密码锁清晰可见。
- `product_color_family`：辅助展示浅绿、浅蓝、黑色等年轻清爽配色。
- `scene_light`：明亮柔和自然光，浅白/浅灰/浅木色场景。
- `motion_feel`：轮子顺滑滚动、箱体轻松拉走、收纳整齐利落、快切节奏带爽感。
- `platform_look`：抖音/小红书信息流竖版构图，卖点字幕简洁醒目。

`non_negotiables`：

- 商品必须保持波尔盾铝框行李箱外观一致。
- 全片必须年轻、干净、快节奏、电商转化导向。
- 静音万向轮、铝框箱体/密码锁、前置挂钩、干湿分离、多尺寸、多颜色、热销信任背书必须在 20 秒内有清晰表达。
- 视频比例 9:16，分镜视频时长需符合 5 秒或 10 秒生成能力，最终通过剪辑组合成约 20 秒。
- 产品是绝对主角，人物只作为使用场景和动作辅助。

### Seedance Storyboard 具体内容

Producer 创建了 4 个 scene：

| sort | scene | title | location | mood | description |
|---:|---|---|---|---|---|
| 1 | `scene_packaging_problem` | 出行前混乱 | 明亮宿舍/卧室整理台 | 略忙乱但不脏乱，快速制造痛点 | 开学/旅行出发前，宿舍或卧室里衣物、洗漱包、书包散落，制造“出行前混乱”的痛点 |
| 2 | `scene_product_solution` | 产品功能解决 | 干净通行空间与产品特写背景 | 轻快、顺滑、有安全感 | 波尔盾作为解决方案登场，展示顺滑移动、铝框锁具、前置挂钩等外部卖点 |
| 3 | `scene_storage_choice` | 收纳与选择 | 明亮整理台与简洁展示背景 | 整齐、清爽、选择丰富 | 打开箱体展示干湿分离和收纳，再切换多尺寸多颜色 |
| 4 | `scene_cta` | 信任背书与转化 | 简洁浅背景产品展示区 | 干净、可信、促点击 | 回到主产品 hero 画面，配合热销 8 万+、高性价比和行动号召 |

8 个 shot：

| sort | shot | scene | duration | kind | purpose | visual intent | action | camera | narration |
|---:|---|---|---:|---|---|---|---|---|---|
| 1 | `shot_01` | packaging_problem | 3 | `lifestyle` | 3 秒痛点 hook | 年轻出行场景和“乱”的视觉压力，但保持干净明亮 | 衣物、洗漱包、书包、外套散落，年轻人匆忙收拾 | 竖版中近景快速切换，略带手持动感 | 开学、旅行前，总觉得东西多到乱？ |
| 2 | `shot_02` | packaging_problem | 2 | `product_closeup` | 产品正式登场 | 白色箱体、银色铝框、金属包角、高级干净质感 | 白色行李箱稳稳落在整理区中央，散乱物品被迅速收束 | 产品 hero shot，低机位或正面中景，快速推近 | 一个波尔盾，就够了 |
| 3 | `shot_03` | product_solution | 3 | `lifestyle` | 展示静音万向轮和顺滑移动 | 轮子滚动顺滑、地面反光、人物拉箱轻松 | 出行者单手拉箱，在走廊/车站/机场通道顺滑前行 | 低机位跟拍轮子，再接中景侧跟 | 静音万向轮，宿舍、车站、机场，一路顺滑 |
| 4 | `shot_04` | product_solution | 2 | `product_closeup` | 展示铝框箱体和密码锁 | 金属锁具、铝框中框和包角细节 | 手指轻按锁扣，锁具开合利落 | 极近特写，快速推近锁具和铝框 | 铝框箱体加密码锁，结实又安心 |
| 5 | `shot_05` | product_solution | 2 | `lifestyle` | 展示前置挂钩便利 | 挂钩位置、挂物动作、人物轻松状态 | 随手把背包或外套挂在前置挂钩上 | 中近景切到手部动作和挂钩细节 | 前置挂钩，小包外套随手挂 |
| 6 | `shot_06` | storage_choice | 3 | `product_closeup` | 展示干湿分离和大容量收纳 | 内部分区、整齐摆放、年轻出行物品 | 浅绿款打开，人物快速把洗漱包、湿毛巾、衣物、鞋子收纳 | 俯拍和侧拍结合，快切展示内部 | 干湿分离，洗漱衣物整整齐齐 |
| 7 | `shot_07` | storage_choice | 2 | `product_closeup` | 展示多尺寸多颜色 | 清爽高级的颜色排列 | 不同尺寸和颜色快速切换展示 | 快切产品陈列 | 多尺寸、多颜色，短途开学到长途旅行都能选 |
| 8 | `shot_08` | cta | 3 | `cta` | 热销信任背书和 CTA | 回到白色主款高级感，字幕简洁 | 白色主款 hero，热销 8 万+、高性价比轻量字幕 | 正面 hero 定格或轻微推近 | 热销 8 万+，高颜值高性价比，开学旅行，带上波尔盾出发 |

### Seedance AudioPlan 具体内容

| 字段 | 值 |
|---|---|
| `title` | 波尔盾行李箱 20 秒电商广告音频方案 |
| `target_duration_sec` | 20 |
| `status` | `composing` |
| `voice_profile` | 年轻女声，清爽利落，带轻快感和信任感，语速略快但吐字清晰 |
| `bgm_plan` | 118-128 BPM，年轻清爽电子流行，前 3 秒轻微忙乱，3 秒后轻快鼓点和明亮合成器 |
| `generation_params` | `format=mp4_audio`，`sample_rate=44100`，`speech_rate=1.12`，`voice_gender=female` |

cue_plan：

| time | shot_ref | text |
|---|---|---|
| 0-3 | `shot_01` | 开学、旅行前，总觉得东西多到乱？ |
| 3-5 | `shot_02` | 一个波尔盾，就够了。 |
| 5-8 | `shot_03` | 静音万向轮，宿舍、车站、机场，一路顺滑。 |
| 8-10 | `shot_04` | 铝框箱体加密码锁，结实又安心。 |
| 10-12 | `shot_05` | 前置挂钩，小包外套随手挂。 |
| 12-15 | `shot_06` | 干湿分离，洗漱衣物整整齐齐。 |
| 15-17 | `shot_07` | 多尺寸、多颜色，短途开学到长途旅行都能选。 |
| 17-20 | `shot_08` | 热销 8 万+，高颜值高性价比，开学旅行，带上波尔盾出发。 |

### Seedance Craftsman 写出的 RenderPlan

Craftsman 总共写了 20 条 RenderPlan：2 条 key element reference image、8 条 preview image、2 条 audio、8 条 shot video。

#### KeyElementState reference image plans

| render_plan | target_phase | profile | operation | 目的 | 绑定 |
|---|---|---|---|---|---|
| `element_scene_dorm_packing.state_bright_dorm_mess_to_neat.reference_image.r1` | `reference_image` | `seedream_5_image` | `image_to_image` | 生成明亮宿舍/卧室整理场景锚点，服务 `shot_01/02/06` | 绑定商品图，要求行李箱结构正确 |
| `element_scene_transit_corridor.state_clean_transit_morning.reference_image.r1` | `reference_image` | `seedream_5_image` | `text_to_image` | 生成现代干净通行空间锚点，服务 `shot_03` 等移动场景 | 纯文生图，文本中约束白色波尔盾箱体 |

#### Seedream preview image plans

共同结构：

- `target_phase=preview_image`
- `model_prompt_profile=seedream_5_image`
- `params.ratio=9:16`
- `params.resolution=1080p`
- `params.max_images=1`
- 大部分 `reference_bindings` 引用用户商品图或 key element reference image。
- 部分计划有 `subject_bindings`，用于锁定“白色主款铝框行李箱”的稳定外观。

| render_plan | operation | shot | prompt 目标 |
|---|---|---|---|
| `shot_01.preview_image.r1` | `image_to_image` | 痛点开场 | 明亮宿舍，白色波尔盾箱居中，周围衣物/洗漱包适度散落但不脏乱 |
| `shot_02.preview_image.r1` | `text_to_image` | 产品登场 | 白色波尔盾 hero 登场，明亮整洁宿舍环境，产品占约 60% 画面 |
| `shot_03.preview_image.r1` | `image_to_image` | 静音轮顺滑 | 低角度聚焦万向轮，干净明亮通道，轮子滚动顺滑感 |
| `shot_04.preview_image.r1` | `multi_image_to_image` | 铝框锁具 | 近景特写，铝框锁具和手部按压动作，白色主款外观一致 |
| `shot_05.preview_image.r1` | `image_to_image` | 前置挂钩 | 通行通道中白色箱体，前置挂钩挂小包/雨伞，人物只露手部 |
| `shot_06.preview_image.r1` | `multi_image_to_image` | 干湿分离 | 浅绿色打开箱体，衣物、洗漱包、隔层清晰 |
| `shot_07.preview_image.r1` | `multi_image_to_image` | 多尺寸多颜色 | 3-4 只不同尺寸/配色行李箱阶梯式或扇形阵列 |
| `shot_08.preview_image.r1` | `image_to_image` | CTA | 白色主款 hero，浅米白/浅灰渐变背景，热销 CTA 收尾画面 |

#### Seedance shot video plans

共同结构：

- `target_phase=shot_video`
- `model_prompt_profile=seedance_2_video`
- `operation=image_to_video_first_frame`
- `params.ratio=9:16`
- `params.resolution=1080p`
- `params.duration_sec=5`
- `params.return_last_frame=true`
- `reference_bindings` 使用对应 `preview_image.current` 作为 `first_frame`。
- prompt 明确“视频动作 + 镜头运动 + 产品一致性 + 禁止项”。

| render_plan | shot | first frame | 主要动作 | 镜头语言 | 备注 |
|---|---|---|---|---|---|
| `shot_01.shot_video.r1` | `shot_01` | `shot_01.preview_image.current` | 手部翻找物品，表现出行前焦虑和混乱 | 缓慢推镜，把焦点收拢到白色行李箱 | 痛点开场 |
| `shot_02.shot_video.r1` | `shot_02` | `shot_02.preview_image.current` | 手伸入握住拉杆并微微提起 | 产品中心缓慢推近，背景虚化 | hero 登场 |
| `shot_03.shot_video.r1` | `shot_03` | `shot_03.preview_image.current` | 行李箱被轻拉向前，轮子顺滑转动 | 低角度侧前方跟拍，聚焦轮子和箱体下半部分 | 功能卖点 |
| `shot_04.shot_video.r1` | `shot_04` | `shot_04.preview_image.current` | 手指按压铝框锁扣，扣合动作清晰 | 极缓推近至锁具区域 | 安全感 |
| `shot_05.shot_video.r1` | `shot_05` | `shot_05.preview_image.current` | 手把小包挂到前置挂钩，随后轻推箱子 | 中近景推近挂钩区域，背景虚化 | 便利卖点 |
| `shot_06.shot_video.r1` | `shot_06` | `shot_06.preview_image.current` | 手放入衣物并拉上干湿分离袋拉链 | 俯拍微推进，靠近箱体内部 | 收纳卖点 |
| `shot_07.shot_video.r1` | `shot_07` | `shot_07.preview_image.current` | 多色多尺寸箱体依次高光扫过、轻微放大 | 阵列全景缓慢前推 | 产品选择 |
| `shot_08.shot_video.r1` | `shot_08` | `shot_08.preview_image.current` | 手轻搭拉杆，轻推/邀请点击 | 极缓推近 + 微环绕，最后 hero 定格 | CTA |

这组 RenderPlan 与 Remotion 最大的不同是：Seedance prompt 要求真实动态动作，比如手部翻找、轻拉箱体、轮子滚动、锁扣扣合、挂包、拉链、产品阵列高光等；Remotion prompt 则只要求图片轻动效和文字层。

#### Seedance audio RenderPlans

| render_plan | target_phase | profile | operation | params | prompt 目标 |
|---|---|---|---|---|---|
| `audio_plan.active.voiceover_audio.r1` | `voiceover_audio` | `seed_audio_1` | `text_to_audio` | `duration_sec=20`，`speech_rate=1.1`，`loudness_rate=0.9`，`sample_rate=48000` | 中文普通话年轻女声旁白，目标约 20 秒，清爽利落，不叫卖 |
| `audio_plan.active.bgm_audio.r1` | `bgm_audio` | `seed_audio_1` | `text_to_audio` | `duration_sec=20`，`loudness_rate=0.7`，`sample_rate=48000` | 20 秒纯音乐 BGM，118-128BPM，年轻清爽电子流行，给人声留空间 |

### Seedance 路线里的工具调用

`agent_message` 中工具 trace 计数：

| 工具 | 次数 | 作用 |
|---|---:|---|
| `upsert_render_plan` | 34 | Craftsman 写 reference/preview/shot/audio RenderPlan，包含后续修订和上下文轮次 |
| `read_project_context` | 29 | Producer/Reviewer/Composer 读取项目状态 |
| `load_agent_skill` | 26 | Producer/Craftsman/Reviewer 加载 commerce/audio/seedance 等 skill |
| `probe_media` | 12 | Composer 检查媒体时长、尺寸、编码 |
| `search_agent_history` | 11 | Agent 检索历史消息 |
| `read_project_memory` | 10 | Craftsman 读取全局创意约束 |
| `decide_render_plan` | 7 | Producer 接受 RenderPlan 并提交 Worker |
| `dispatch_craftsman` | 7 | Producer 派发 reference、preview、audio、shot video |
| `dispatch_composer` | 2 | Producer 派发 Composer |
| `request_user_decision` | 2 | Producer 请求用户确认音频/方案 |
| `dispatch_reviewer` | 1 | Producer 派发最终视频 Reviewer |
| `submit_review_result` | 1 | Reviewer 提交 final video review |
| `run_ffmpeg_command` | 1 | Composer ffmpeg/ffprobe fallback |
| `submit_composition_artifact` | 1 | Composer 提交成片 artifact |

### Seedance 路线的媒体资产

核心输出：

- 8 个用户上传商品图。
- 2 张 Seedream reference image。
- 8 张 Seedream preview image。
- 8 段 Seedance shot video。
- 1 条 voiceover audio。
- 1 条 BGM audio。
- 1 个 final video。
- 1 条 final video review record。

## 两条路线的核心差异

| 维度 | Seedance 路线 | Remotion 路线 |
|---|---|---|
| 用户目标 | 20 秒波尔盾电商短视频 | 30 秒以上悦行 no-Seedance 口播广告 |
| 分镜数量 | 8 个，时长 2-3 秒 | 5 个，时长 6-8 秒 |
| Producer 规划方式 | 模型驱动，加载 `commerce-ad-producer`、`audio-plan-producer` 等 skill | E2E fixture/自动化 trace 为主，硬写 no-Seedance 约束 |
| 图片生成 | Seedream reference + preview | Seedream preview |
| shot video | `seedance_2_video` | `motion_shot_video` |
| video provider | `volcengine/doubao-seedance-2-0-260128` | `internal_motion_video/remotion-motion-shot-v1` |
| video operation | `image_to_video_first_frame` | `image_to_motion_video` |
| 音频 | `seed-audio-1.0` voiceover + BGM | `seed-audio-1.0` voiceover + BGM |
| 成片 | Composer `concat_with_fades` | Composer `simple_concat` |
| Reviewer | final video review 运行，但 blocked | 未运行 Reviewer |
| 成本 | 高，8 次 Seedance 视频调用 | 低，无 Seedance，5 次内部 Remotion |

## skill 在两条链路中的作用

当前 skill 不是外部插件执行器，而是 Agent prompt/工具协议的一部分。Producer 或其他 Agent 通过 `load_agent_skill` 把 skill 内容加载进本轮上下文，约束后续工具调用。

Seedance workspace 明确加载了：

- `commerce-ad-producer`：指导 Producer 把商品广告需求转为 brief/memory/key elements/storyboard/audio plan。
- `audio-plan-producer`：指导 Producer 创建全片 AudioPlan、请求确认、分离 voiceover/BGM。
- Seedance/Craftsman 相关 skill 从工具 trace 和 RenderPlan 可看出参与了 profile 选择，具体表现为 `seedance_2_video + image_to_video_first_frame`。

Remotion workspace 当前 DB 主要是 E2E fixture trace，不像 Seedance workspace 那样大量记录 `load_agent_skill`。但代码中已有对应 skill：

- `motion-shot-producer`：约束 no-Seedance/cost route，要求 Producer 在 `dispatch_craftsman` 中填写 `video_route_policy=motion_only`。
- `motion-shot-craftsman`：要求 Craftsman 写 `motion_shot_video + image_to_motion_video`，不要写 Seedance，不要写原始 Remotion 代码。
- `motion-shot-reviewer`：用于评审 Remotion motion shot 的文本安全区、产品可见性、节奏和最终可合成性。

## 如何理解 Remotion 在项目中的角色

Remotion 不是 Producer、Craftsman、Reviewer、Composer、Worker 之外的第六个 Agent。它在 ClipAnvil 中扮演的是低成本 `shot_video` provider：

```text
Producer 决定：这个 shot 走低成本图片动效
Craftsman 写：motion_shot_video RenderPlan
Worker 提交：GenerationIntent
Production 选择：internal_motion_video/remotion-motion-shot-v1
Remotion 生成：silent motion shot mp4
Composer 负责：拼接、口播、BGM、字幕、最终音画同步
```

所以，Remotion 替代的是 Seedance 在“每个 shot video 生成”这一层的部分能力，不替代 Producer 的分镜规划，也不替代 Composer 的最终时间线。

## 当前暴露的问题

1. Remotion workspace 仍是 E2E fixture 特征较重。它证明了 no-Seedance 路由，但还不是完全自然的模型驱动 Producer 工作流。
2. Remotion workspace 后段出现 preview image r2 重跑，说明修复/重合成指令容易误触发上游图片再生成，需要更清晰的 Producer 状态机和“只重合成”工具路径。
3. Reviewer 没有参与 Remotion final video review，质量 gate 缺失。
4. Seedance final review 被 blocked，因为 Reviewer 不能真实试听/解析音轨。音画同步目前主要依赖 AudioPlan cue 和 Composer timeline，而不是自动审核。
5. Remotion 成片使用 `simple_concat`，Seedance 成片使用 `concat_with_fades`。低成本路线要更吸引眼球，需要 Composer 使用更丰富但受控的 timeline 模板，而不是只证明能拼起来。
6. 当前 DB 中没有 Agent LLM call 独立审计表。要深入分析模型每轮推理成本、token、工具选择原因，需要新增模型调用审计或 trace 表。

## 对项目理解的关键结论

ClipAnvil 的核心不是“某个视频模型”，而是多 Agent 生产编排：

- Producer 负责意图、结构、预算和推进。
- Craftsman 负责把结构翻译成可执行 RenderPlan。
- Worker 负责真正调用模型/provider。
- Composer 负责把素材变成成片。
- Reviewer 负责质量判断，但当前最终音频/音画同步审核还不够强。

Seedance 路线和 Remotion 路线的上半身应该相同：用户需求、商品理解、storyboard、AudioPlan、cue_plan、dispatch、Composer。真正不同的是 shot video 的 RenderPlan profile 和 provider：

- Seedance：`seedance_2_video -> volcengine/doubao-seedance-*`
- Remotion：`motion_shot_video -> internal_motion_video/remotion-motion-shot-v1`

因此未来要做好低成本视频，不应该让 Remotion 变成固定模板系统，也不应该让它决定分镜。正确方向是：Producer 继续动态生成分镜和 cue_plan，Seedream 生成每个 shot 需要的关键图，Remotion 生成 silent motion shot，Composer 用 cue_plan 做统一字幕、口播、BGM、转场和最终音画同步。
