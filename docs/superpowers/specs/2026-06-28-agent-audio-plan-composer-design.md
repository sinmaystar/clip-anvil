# Agent Audio Plan 与 Composer 混音方案设计

**状态**：待评审
**日期**：2026-06-28
**适用范围**：ClipAnvil Agent 模式，营销短视频旁白 + BGM 第一版音频链路，Producer / Craftsman / Worker / Composer / shared production / 火山豆包语音集成

## 结论

第一版音频能力只做 **营销短视频旁白 + BGM**。不做真人对口型，不做多角色对白连续性，不把分镜视频模型自带音频作为最终成片主音轨。

音频应成为全片级时间线事实源，而不是每个分镜视频的副产品。推荐主路径：

```text
Producer 生成全片旁白脚本和音频策略
  -> 用户确认或修改脚本 / 音色 / BGM 方向
  -> AudioPlan 持久化为全片级事实源
  -> Producer dispatch_craftsman(target_phase=voiceover_audio)
  -> Craftsman 生成 voiceover_audio RenderPlan
  -> Producer dispatch_craftsman(target_phase=bgm_audio)
  -> Craftsman 生成 bgm_audio RenderPlan
  -> Producer decide_render_plan accept
  -> Worker 调用 seed-audio-1.0 生成旁白和 BGM 音频
  -> Composer stage 分镜视频、旁白、BGM
  -> Composer 用 TimelinePlan 混音、ducking、fade、concat
  -> Composer 提交 final video artifact
  -> Reviewer / 用户确认最终音画节奏
```

核心原则：

- 分镜视频默认生成无声，`generate_audio=false`。
- 旁白按全片生成，保证语气、节奏和音色连续。
- BGM 第一版也由 `seed-audio-1.0` 生成。不要把用户上传音频或素材库作为第一版主路径。
- 旁白和 BGM 必须拆成两个独立 RenderPlan，分别生成 audio artifact，再进入 timeline。
- `shot.audio_plan` 只存全片 AudioPlan 切分到该 shot 的 cue，不作为各分镜独立生成音频的事实源。
- Composer 负责最终音轨混合，不让视频生成模型分别生成无法统一剪辑的音频。

## 背景与当前代码事实

当前 ClipAnvil 已具备一些音频相关结构，但还没有完整产品链路：

- `shot` 已有 `dialogue`、`narration`、`audio_plan` 字段。
- `media_node` / `asset_type` 已支持 `audio`。
- Reviewer rubric 已包含 `audio_sync` 维度。
- RenderPlan 已支持 `reference_audio`、`prompt_parts.audio`、`params.generate_audio`。
- Volcengine video runtime 已能把 `generate_audio` 和 audio refs 传给 Seedance。
- Composer 目前已落地 `timeline_plan`、`simple_concat` / `concat_with_fades`、sandbox ffmpeg、final video artifact 提交。
- 当前 Agent 文档仍明确标记：音频素材导入、旁白 / BGM / TTS、音频参与合成未接入 Agent。

因此下一步不是从零加一个音频功能，而是把已有字段收束成稳定产品路径：全片 AudioPlan -> audio RenderPlan -> audio artifact -> TimelinePlan 混音。

## 火山豆包语音能力边界

调研文档：

- 音频生成 HTTP：https://www.volcengine.com/docs/6561/2550782?lang=zh
- 模型列表：https://www.volcengine.com/docs/6561/2499930?lang=zh
- 豆包语音合成大模型产品简介：https://www.volcengine.com/docs/6561/1257543?lang=zh

本方案第一版主要使用 `seed-audio-1.0` 音频生成 HTTP：

| 能力 | 设计含义 |
|---|---|
| `text_prompt` 生成音频 | 可生成全片旁白，也可描述音效、环境声、音乐氛围。 |
| `speaker` 音色 ID | 第一版旁白推荐用预设音色或已训练声音复刻音色。 |
| 最长输出 120 秒 | 足够覆盖多数短视频；超过 120 秒必须拆段并由 Composer 拼接。 |
| Prompt 最大 2048 字符 | Producer 需要生成紧凑的旁白脚本和音频指令，不能把完整项目上下文塞进去。 |
| 参考音频最多 3 条，每条最长 30 秒、10 MB | 第一版不默认走参考音频克隆；后续再做品牌音色 / 上传参考音频。 |
| 参考图片最多 1 张，且不能和 audio refs / speaker 混用 | 第一版不把参考图片作为旁白主路径。 |
| 输出格式 `wav` / `mp3` / `pcm` / `ogg_opus` | ClipAnvil 默认用 `mp3` 或 `wav` 入库，Composer stage 后统一转码。 |
| 采样率支持 8k 到 48k | 成片混音建议默认 48k，和视频后期更匹配。 |
| 语速、音调、音量可调 | 映射到 AudioPlan 的 `speech_rate`、`pitch_rate`、`loudness_rate`。 |
| 返回 base64 音频和 2 小时有效 URL | 服务端应立即把结果上传到 MinIO，不能依赖临时 URL。 |

注意：`seed-audio-1.0` 是音频生成模型，不等同于 `seed-tts-2.0-standard` / `seed-tts-2.0-expressive` 这类普通 TTS 模型。第一版可以先只接 `seed-audio-1.0`，模型能力表后续再区分 `audio_generation`、`tts`、`voice_clone`。

## 范围

范围内：

- Agent 模式全片级 AudioPlan 事实源。
- Producer 生成并请求用户确认旁白脚本、音色和 BGM 方向。
- Craftsman 支持 `voiceover_audio` 和 `bgm_audio` RenderPlan，把 AudioPlan 翻译成模型 prompt、speaker、format、sample_rate、speech_rate 等执行计划。
- Worker / production 接入 `seed-audio-1.0`，按 audio RenderPlan 生成旁白和 BGM audio artifact。
- Worker / production 同时支持 `voiceover_audio` 和 `bgm_audio` 两类 audio artifact。
- Composer TimelinePlan 支持旁白轨、BGM 轨、ducking、fade、音量、音频与分镜时长对齐。
- Reviewer final video review 使用 `audio_sync` 评估旁白节奏、BGM 音量、音画匹配。
- 最小 smoke：两到三个分镜视频 + generated voiceover + generated BGM 混成最终视频。

范围外：

- 真人 / 角色对口型。
- 多角色对白连续性和逐词口型时间戳。
- 自动声音复刻下单 / 训练流程。
- 多段长视频超过 120 秒的复杂旁白拆分策略。
- 视频模型自带音频作为最终主音轨。
- 用户上传音频素材、音频素材库和商用授权素材管理。
- 专业 DAW 级时间线编辑 UI。

## 产品体验

用户要求生成一条营销短视频时，Producer 应补充音频策划：

```text
我会先为这条视频生成全片旁白和 BGM 方向。
旁白音色：年轻女声，清爽、有信任感。
BGM：轻快电子流行，前 2 秒抓注意力，中段降低音量突出卖点。
```

当分镜和视频路径具备后，Producer 生成 AudioPlan 草案并通过 decision card 请求确认：

```json
{
  "voiceover_script": "出发前，先把行李这件事变简单。悦行银灰色行李箱，轻推顺滑，登机路上更从容。现在入手，让每一次出发都更轻松。",
  "voice_profile": {
    "source": "preset",
    "speaker": "marketing_female_clear",
    "style": "清爽、有信任感、轻微兴奋"
  },
  "bgm_plan": {
    "source": "generated",
    "provider": "volcengine",
    "model": "seed-audio-1.0",
    "style": "轻快电子流行",
    "ducking": "旁白期间自动降低 8 到 12 dB"
  },
  "cues": [
    {"shot_ref": "shot_01", "start_sec": 0.0, "end_sec": 3.8, "text": "出发前，先把行李这件事变简单。"},
    {"shot_ref": "shot_02", "start_sec": 3.8, "end_sec": 8.2, "text": "悦行银灰色行李箱，轻推顺滑，登机路上更从容。"},
    {"shot_ref": "shot_03", "start_sec": 8.2, "end_sec": 12.0, "text": "现在入手，让每一次出发都更轻松。"}
  ]
}
```

用户可以：

- 直接确认。
- 修改旁白文本。
- 换音色方向。
- 选择不要 BGM。
- 修改 BGM 风格方向。

确认后，Producer 分别派发 Craftsman 创建旁白音频 RenderPlan 和 BGM 音频 RenderPlan；RenderPlan 被 Producer 接受后，Worker 生成两条音频 artifact，再进入 Composer 混音。

## Agent 职责边界

### Producer

Producer 是全局音频策略 owner：

- 从 CreativeBrief、ProjectMemory、Storyboard 和平台目标生成全片旁白脚本。
- 选择默认音色方向和 BGM 方向。
- 把全片脚本切分成 shot cues。
- 请求用户确认关键音频决策。
- 分别派发 Craftsman 创建 `voiceover_audio` 和 `bgm_audio` RenderPlan。
- 接受或拒绝音频 RenderPlan。
- 在旁白和 BGM audio artifacts ready 后派发 Composer。

Producer 不做：

- 不直接写 ffmpeg 命令。
- 不直接写 provider prompt 或 provider 私有参数。
- 不让每个 shot 独立生成旁白音频。
- 不把用户上传音频或素材库作为第一版主路径。
- 不把视频模型自带音频当作多分镜最终音轨。
- 不在未确认音色 / 脚本时静默生成最终音频，除非用户明确要求自动推进。

### Craftsman

Craftsman 负责把 AudioPlan 翻译成可执行的 audio RenderPlan：

- 读取 approved AudioPlan、ProjectMemory、Storyboard、目标平台和可用音色配置。
- 生成 `target_phase=voiceover_audio` 的 RenderPlan。
- 编译适合 `seed-audio-1.0` 的 `generation_text` / `prompt_parts.audio`。
- 设置 `operation=audio_generation`、`output_type=audio`、`model_prompt_profile=seed_audio_1`。
- 设置 `params.speaker`、`format`、`sample_rate`、`speech_rate`、`pitch_rate`、`loudness_rate`、`watermark`。
- 为 BGM 生成独立 `target_phase=bgm_audio` RenderPlan。

Craftsman 不做：

- 不修改 AudioPlan 原始脚本。
- 不直接提交 generation job。
- 不把 BGM 和旁白混在同一个 RenderPlan 里生成。
- 不绕过 Producer 的 `decide_render_plan`。

### Worker / Production

Worker 执行音频生成：

- 使用 shared production `GenerationIntent`。
- 从 accepted audio RenderPlan 构造 `GenerationIntent`。
- 新增 `operation_type=audio_generation`，`output_type=audio`。
- 调用 Volcengine `seed-audio-1.0`。
- 把 base64 / 临时 URL 结果上传到 MinIO。
- 创建 audio `media_node`、`generation_job`、`artifact_version`。
- 失败必须落库，和现有 production failure 语义一致。

### Composer

Composer 是后期混音 owner：

- 读取 confirmed AudioPlan、shot video winners、旁白 artifact 和 BGM artifact。
- stage 视频、旁白和 BGM 到 sandbox。
- probe duration、streams、sample rate。
- 创建包含 audio tracks 的 TimelinePlan。
- 使用 ffmpeg 渲染最终视频。
- 回填 timeline result 并提交 final video artifact。

Composer 不修改 AudioPlan 文案；如果旁白长度和视频时长严重不匹配，应 blocked 给 Producer。

### Reviewer

Reviewer 在 final video review 中评估：

- 旁白是否覆盖用户核心卖点。
- 旁白节奏是否贴合分镜内容。
- BGM 是否压过旁白。
- 片头片尾 fade 是否自然。
- 最终视频是否存在无声、爆音、音画明显错位。

`audio_sync` 第一版不要求口型同步，只评估旁白 / BGM / 分镜节奏匹配。

## 数据模型

### 新增 `audio_plan`

推荐新增全片级 `audio_plan` 表，避免把全片脚本只塞进 `shot.audio_plan` 或 `timeline_plan.plan_json`。

```sql
CREATE TABLE audio_plan (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'draft',
    title TEXT NOT NULL DEFAULT '',
    plan_kind TEXT NOT NULL DEFAULT 'marketing_voiceover_bgm',
    language TEXT NOT NULL DEFAULT 'zh',
    target_duration_sec DOUBLE PRECISION,
    voiceover_script TEXT NOT NULL DEFAULT '',
    voice_profile JSONB NOT NULL DEFAULT '{}',
    bgm_plan JSONB NOT NULL DEFAULT '{}',
    cue_plan JSONB NOT NULL DEFAULT '[]',
    generation_params JSONB NOT NULL DEFAULT '{}',
    voiceover_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    bgm_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    voiceover_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    bgm_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    timeline_plan_id UUID REFERENCES timeline_plan(id) ON DELETE SET NULL,
    created_by_role TEXT NOT NULL DEFAULT 'producer',
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    semantic_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT audio_plan_status_check CHECK (status IN ('draft', 'waiting_for_user', 'approved', 'generating', 'voiceover_ready', 'composing', 'completed', 'blocked', 'failed')),
    CONSTRAINT audio_plan_kind_check CHECK (plan_kind IN ('marketing_voiceover_bgm'))
);
```

`cue_plan` 示例：

```json
[
  {
    "cue_key": "voiceover.shot_01.c1",
    "shot_ref": "scene_main.shot_01",
    "start_sec": 0.0,
    "end_sec": 3.8,
    "text": "出发前，先把行李这件事变简单。",
    "delivery": "开场抓注意力，语速略快",
    "required": true
  }
]
```

`voice_profile` 示例：

```json
{
  "provider": "volcengine",
  "model": "seed-audio-1.0",
  "source": "speaker",
  "speaker": "marketing_female_clear",
  "style_prompt": "年轻女声，清爽、有信任感，适合电商种草短视频",
  "speech_rate": 0,
  "pitch_rate": 0,
  "loudness_rate": 0
}
```

`bgm_plan` 示例：

```json
{
  "source": "generated",
  "provider": "volcengine",
  "model": "seed-audio-1.0",
  "style": "轻快电子流行，干净、有速度感",
  "generation_prompt": "生成一段 12 秒轻快电子流行 BGM，干净、有速度感，适合机场行李箱种草广告；不要人声，不要明显鼓点压过旁白。",
  "target_lufs": -18,
  "ducking": {
    "enabled": true,
    "voiceover_reduction_db": 10,
    "attack_ms": 250,
    "release_ms": 600
  },
  "fade_in_sec": 0.3,
  "fade_out_sec": 1.0
}
```

### 更新 `shot.audio_plan`

`shot.audio_plan` 不再承载全片音频策略，只保存从 AudioPlan 派生的 shot 局部 cue 摘要：

```json
{
  "audio_plan_id": "...",
  "cue_keys": ["voiceover.shot_01.c1"],
  "voiceover_text": "出发前，先把行李这件事变简单。",
  "bgm_energy": "intro_hook",
  "sfx": []
}
```

### 扩展 `timeline_plan.plan_json`

Composer 的 TimelinePlan 消费 AudioPlan，并写完整音轨结构：

```json
{
  "template_key": "voiceover_bgm_concat",
  "audio_plan_id": "...",
  "video_tracks": [
    {"track": "v1", "shot_ref": "scene_main.shot_01", "asset_ref": "shot_01.shot_video.current", "start_sec": 0.0, "duration_sec": 3.8}
  ],
  "audio_tracks": [
    {"track": "a_voiceover", "asset_ref": "audio_plan.voiceover.current", "start_sec": 0.0, "gain_db": 0},
    {"track": "a_bgm", "asset_ref": "bgm.current", "start_sec": 0.0, "gain_db": -16, "duck_under": "a_voiceover"}
  ],
  "mix": {
    "sample_rate": 48000,
    "normalize": true,
    "target_lufs": -16,
    "fade_in_sec": 0.2,
    "fade_out_sec": 0.8
  }
}
```

## Tool 契约

### Producer `upsert_audio_plan`

职责：创建或更新全片 AudioPlan 草稿。

```json
{
  "brief": "为悦行行李箱三分镜种草视频创建全片旁白和 BGM 计划。",
  "mode": "create",
  "plan_kind": "marketing_voiceover_bgm",
  "target_duration_sec": 12,
  "voiceover_script": "...",
  "voice_profile": {},
  "bgm_plan": {},
  "cue_plan": []
}
```

规则：

- `voiceover_script` 必须是全片脚本，不是分镜散句集合。
- `cue_plan[*].shot_ref` 必须来自当前 storyboard。
- `cue_plan` 总时长应接近目标视频时长。
- `text_prompt` 最终进入 `seed-audio-1.0` 前必须控制在 2048 字符内。
- 如果用户要求换脚本，更新 AudioPlan 而不是直接改 TimelinePlan。

### Producer `dispatch_craftsman`

职责：把 approved AudioPlan 派发给 Craftsman 创建音频 RenderPlan。复用现有 `dispatch_craftsman` 工具，不新增绕过 RenderPlan 的 `dispatch_audio_generation`。第一版需要分别派发旁白和 BGM。

```json
{
  "brief": "为已确认 AudioPlan 创建全片旁白音频 RenderPlan。",
  "scope": {"type": "audio_plan", "key": "audio_plan.marketing_voiceover.v1"},
  "target_phase": "voiceover_audio",
  "execution_policy": "execute_immediately"
}
```

规则：

- AudioPlan 必须是 `approved`。
- 第一版允许 `target_phase=voiceover_audio` 和 `target_phase=bgm_audio`。
- `dispatch_craftsman` 需要扩展 scope enum，支持 `audio_plan`。
- 旁白和 BGM 必须是两个独立 RenderPlan，不要共用同一个模型调用。

### Craftsman `upsert_render_plan`

职责：为 AudioPlan 创建或修订 audio RenderPlan。

```json
{
  "brief": "为已确认 AudioPlan 生成 seed-audio-1.0 旁白计划。",
  "mode": "create",
  "scope": {"type": "audio_plan", "key": "audio_plan.marketing_voiceover.v1"},
  "target_phase": "voiceover_audio",
  "operation": "audio_generation",
  "model_prompt_profile": "seed_audio_1",
  "generation_text": "用年轻女声、清爽有信任感的语气朗读：出发前，先把行李这件事变简单...",
  "prompt_parts": {
    "audio": "营销短视频旁白，清爽、有信任感、轻微兴奋；不要加入背景音乐或音效。",
    "narration": "出发前，先把行李这件事变简单。悦行银灰色行李箱..."
  },
  "params": {
    "speaker": "marketing_female_clear",
    "format": "mp3",
    "sample_rate": 48000,
    "speech_rate": 0,
    "pitch_rate": 0,
    "loudness_rate": 0,
    "watermark": {"enable": false}
  }
}
```

规则：

- `voiceover_audio` RenderPlan 必须使用 `scope.type=audio_plan`。
- `generation_text` 必须来自 AudioPlan 的 confirmed script 和 voice profile，不得重写卖点或改脚本含义。
- `generation_text` / 最终 `text_prompt` 必须满足 `seed-audio-1.0` 2048 字符限制。
- 旁白 RenderPlan 不包含 BGM、环境声或音效，除非 AudioPlan 明确要求合成到同一条旁白中；第一版默认禁止。
- 如果缺少 `speaker` 或音色配置不合法，Craftsman 应 `mark_blocked`，交给 Producer 请求用户选择音色。

`bgm_audio` RenderPlan 示例：

```json
{
  "brief": "为已确认 AudioPlan 生成 seed-audio-1.0 BGM 计划。",
  "mode": "create",
  "scope": {"type": "audio_plan", "key": "audio_plan.marketing_voiceover.v1"},
  "target_phase": "bgm_audio",
  "operation": "audio_generation",
  "model_prompt_profile": "seed_audio_1",
  "generation_text": "生成一段 12 秒轻快电子流行 BGM，干净、有速度感，适合机场行李箱种草广告；不要人声，不要明显鼓点压过旁白。",
  "prompt_parts": {
    "audio": "轻快电子流行 BGM，干净、有速度感；无歌词、无人声；为旁白留出中频空间。"
  },
  "params": {
    "format": "mp3",
    "sample_rate": 48000,
    "loudness_rate": -10,
    "watermark": {"enable": false}
  }
}
```

规则：

- `target_phase=bgm_audio` 时，`generation_text` 必须只描述 BGM，不生成旁白、人声、对白或音效堆叠。
- BGM 时长应覆盖目标视频时长；不足时 Composer 可以 loop，过长时 Composer trim。
- BGM RenderPlan 不需要 `speaker`。

### Producer `decide_render_plan`

职责：Producer 审核并接受 audio RenderPlan。接受后 Worker 才能执行模型调用。

规则：

- audio RenderPlan 和 preview / shot video RenderPlan 一样，必须经过 `decide_render_plan`。
- 如果 voiceover RenderPlan 改写了已确认旁白脚本，Producer 应 reject 并要求 Craftsman 修订。
- 如果 bgm RenderPlan 加入了人声、歌词或与旁白冲突的强音效，Producer 应 reject 并要求 Craftsman 修订。
- 如果只是补全 provider 参数、采样率和音量策略，Producer 可以 accept。

### Composer `create_timeline_plan`

现有 `create_timeline_plan` 需要扩展 template enum：

- `simple_concat`
- `concat_with_fades`
- `voiceover_bgm_concat`

`voiceover_bgm_concat` 必须包含：

- video tracks
- voiceover track
- optional BGM track
- mix settings
- audio_plan_id

### Composer `render_timeline_template`

`voiceover_bgm_concat` 编译成 ffmpeg 时，至少支持：

- 拼接 shot videos。
- 提取或忽略原视频音轨。第一版默认忽略原音轨。
- 旁白音轨对齐到 0 秒或指定 `start_sec`。
- BGM loop / trim 到最终时长。
- BGM fade in/out。
- BGM ducking under voiceover。
- 输出 `mp4`，音频编码 AAC，采样率默认 48k。

如果 BGM artifact 缺失：

- 用户明确要求无 BGM：继续。
- 用户要求 BGM 但 `bgm_audio` artifact 未生成成功：Composer blocked 给 Producer，Producer 应重试或修改 BGM RenderPlan。

## Volcengine Provider 设计

新增 audio runtime：`volcengine_audio.go`。

`GenerationIntent` 映射：

```json
{
  "operation_type": "audio_generation",
  "output_type": "audio",
  "model": {"provider": "volcengine", "model_id": "seed-audio-1.0"},
  "prompt": "全片旁白脚本和音频指令",
  "params": {
    "speaker": "marketing_female_clear",
    "format": "mp3",
    "sample_rate": 48000,
    "speech_rate": 0,
    "pitch_rate": 0,
    "loudness_rate": 0,
    "watermark": {"enable": false}
  }
}
```

HTTP 请求：

- URL：`https://openspeech.bytedance.com/api/v3/tts/create`
- Header：`Content-Type: application/json`
- Header：`X-Api-Key`
- Header：`X-Api-Request-Id`
- Body：`model`、`text_prompt`、`references`、`audio_config`、`watermark`

响应处理：

- 如果返回 `audio` base64，立即解码并上传 MinIO。
- 如果返回 `url`，只作为兜底下载源；下载后仍上传 MinIO。
- 保存 `duration`、`original_duration`、`X-Tt-Logid` 到 `provider_response`。
- 不把 2 小时临时 URL 作为长期 `storage_url`。

能力表建议：

```json
{
  "provider": "volcengine",
  "model_id": "seed-audio-1.0",
  "operation_type": "audio_generation",
  "output_type": "audio",
  "status": "enabled",
  "limits": {
    "max_duration_sec": 120,
    "max_prompt_chars": 2048,
    "max_reference_audios": 3,
    "max_reference_audio_duration_sec": 30,
    "max_reference_audio_size_mb": 10,
    "max_reference_images": 1,
    "max_reference_image_size_mb": 10,
    "sample_rates": [8000, 16000, 24000, 32000, 44100, 48000],
    "formats": ["wav", "mp3", "pcm", "ogg_opus"]
  },
  "defaults": {
    "format": "mp3",
    "sample_rate": 48000,
    "speech_rate": 0,
    "pitch_rate": 0,
    "loudness_rate": 0
  }
}
```

## ffmpeg 混音策略

第一版不用追求专业级自动混音，但必须稳定可复现。

默认输出：

- container：mp4
- video codec：继承或转 h264
- audio codec：AAC
- sample rate：48000
- channel layout：stereo

推荐模板行为：

1. 所有 shot video 统一转码到目标规格。
2. concat 视频段。
3. voiceover 音频 trim / pad 到 timeline。
4. BGM trim 或 loop 到最终时长。
5. BGM 做 fade in/out。
6. BGM 在旁白期间降低音量。
7. 混音后做基础 loudness normalize。

模板输出必须写入 sandbox job：

- ffmpeg args
- input manifest
- stdout / stderr summary
- exit code
- output probe summary

## UI / Workbench 投影

第一版 Agent Workbench 只需要展示音频状态，不做复杂时间线编辑：

- Project overview 显示 AudioPlan 状态：待确认、生成中、旁白 ready、混音中、完成、失败。
- Shot node 显示该 shot 的旁白 cue 文本。
- Final video card 显示最终音频轨摘要：旁白、BGM、总时长、音色。
- Detail panel 可查看完整 AudioPlan JSON 摘要和生成 job / timeline job。

用户可通过 Agent 对话修改脚本或音色，不需要第一版做可视化拖拽音轨。

## 错误处理

### AudioPlan 脚本过长

Producer 应压缩脚本或改为多段生成。第一版若超过 2048 字符，直接 blocked，提示用户缩短脚本或拆成长视频后续能力。

### 旁白时长与视频时长不匹配

允许轻微偏差：

- 旁白短于视频：BGM 补尾，允许。
- 旁白长于视频 10% 以内：Composer 可轻微加速或延长视频尾部静帧，需写入 result。
- 旁白长于视频超过 10%：blocked 给 Producer，要求改脚本或延长视频。

### BGM 生成失败或缺失

- 用户没要求 BGM：可无 BGM。
- 用户要求 BGM：Producer 应派发或重试 `bgm_audio` RenderPlan。
- Composer 不应自行编造不存在的 BGM。

### Volcengine 失败

失败写 `generation_job`，保存 HTTP status、错误码、message、`X-Tt-Logid`。Producer 可 retry 一次；重复失败后请求用户换音色或改脚本。

## 分阶段实施建议

### Phase 1：AudioPlan 事实源和脚本确认

- 新增 `audio_plan` migration / sqlc。
- Producer native tool `upsert_audio_plan`。
- `read_project_context` 返回 AudioPlan 摘要。
- `shot.audio_plan` 写入 cue 摘要。
- decision card 支持确认旁白脚本、音色、BGM 方向。

验证：

- 单测 AudioPlan validation。
- Producer 工具单测。
- `git diff --check`。

### Phase 2：Volcengine audio generation

- 扩展 `dispatch_craftsman` scope，支持 `audio_plan`。
- 扩展 `render_plan` / RenderPlan submitter，支持 `target_phase=voiceover_audio|bgm_audio`、`operation=audio_generation`、`output_type=audio`。
- 扩展 Craftsman prompt / context loader，读取 AudioPlan 并生成 audio RenderPlan。
- 新增 `volcengine_audio.go` runtime。
- 扩展 `model_capability` seed-audio-1.0。
- 支持 `audio_generation` `GenerationIntent`。
- 保存 audio artifact / media node。

验证：

- Craftsman audio RenderPlan 单测。
- `decide_render_plan` accept audio RenderPlan 单测。
- provider request builder 单测。
- mock audio generation 单测。
- server build / test。

### Phase 3：Composer voiceover + BGM template

- TimelinePlan 支持 `voiceover_bgm_concat`。
- `stage_media_inputs` 支持 audio。
- `render_timeline_template` 编译 audio tracks。
- final video result 写 audio probe summary。

验证：

- ffmpeg args 单测。
- sandbox mock 单测。
- smoke：2 shots + generated voiceover + generated BGM -> final video。

### Phase 4：Review 和 Workbench 展示

- Reviewer final video loader 支持 final artifact + AudioPlan + TimelinePlan。
- `audio_sync` 第一版 rubric 明确为旁白 / BGM / 分镜节奏。
- Workbench 展示 AudioPlan 状态和 cue 摘要。

验证：

- Reviewer prompt / submit result 单测。
- overview builder 单测。
- Web build / lint。

## 关键决策记录

1. 第一版不使用视频模型自带多分镜音频作为最终主音轨。
2. 第一版不做对口型。
3. 旁白脚本由 Producer 生成全片版本，并请求用户确认。
4. 第一版旁白和 BGM 都由音频模型生成，不依赖用户上传音频素材。
5. 音色第一版优先使用预设 `speaker`，后续再接上传参考音频 / 声音复刻。
6. AudioPlan 是全片级事实源，TimelinePlan 是后期执行计划。
7. 音频生成计划由 Craftsman 负责，Producer 只派发和审核，Worker 只执行。
8. Composer 负责混音，不负责重写旁白，也不负责生成音频。
9. `seed-audio-1.0` 生成结果必须上传到 ClipAnvil 自己的对象存储，不能长期依赖临时 URL。

## 待确认问题

- 第一批预设音色 ID 从火山控制台如何配置：写入 `.env` 还是 `model_capability.defaults`。
- `audio_plan` 是否允许多个 active 版本，还是每个 workspace 只保留一个 active plan。
- final video review 是否在 Composer 完成后自动派发，还是先让 Producer 决定。
