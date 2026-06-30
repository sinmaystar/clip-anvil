# Reference Video Analysis 设计

## 背景

用户可能上传一条“同类优秀广告”或其他类型参考视频，希望 ClipAnvil 借鉴它的脚本结构、运镜、节奏、字幕、音频和商品呈现方式。这个能力不是“复制视频”，而是把参考视频拆成可复用的创作语言，再映射到当前商品、素材和项目约束。

当前 ClipAnvil 已经具备这些基础：

- Agent 附件 API 可把用户上传视频保存为 Agent-owned video source node。
- Producer 已有 `reference-video-analysis-producer` skill，但它只定义方法论，没有真实视频理解工具。
- RenderPlan / Craftsman 已支持 `multi_modal_reference_video`、`content_type=video_url`、`model_role=reference_video` 的 schema 和校验。
- Volcengine video runtime 已能把 `video_url` input ref 转成 Seedance 请求 content item。

缺口是：系统没有一个稳定的 `reference_video_analysis` 工具，把视频理解结果沉淀为可读、可审计、可被 Producer 写入 Storyboard 的结构化事实。

## 目标

第一版目标是打通“参考视频 -> 结构化分析 -> Producer 生成改编 Storyboard -> Craftsman 可选绑定 reference_video”的闭环。

用户体验目标：

1. 用户上传参考视频并说明“想借鉴什么”。
2. Producer 调用视频分析工具，得到脚本、镜头、节奏、运镜、音频、字幕和改编建议。
3. Producer 将分析结果转成 ProjectMemory、KeyElement、Scene、Shot 和 ShotDependency。
4. 系统明确告诉用户“会借鉴什么、不会复制什么”，必要时请求确认。
5. 生成分镜视频时，Craftsman 可以把参考视频作为 Seedance `reference_video` 输入，但只能用于风格、运动和节奏参考。

## 非目标

第一版不做：

- 逐帧复刻参考视频。
- 自动复制参考视频中的人物、品牌、Logo、字幕文案或独特创意表达。
- 直接把整条参考视频交给 Seedance 生成成片。
- 通用视频编辑器功能，例如任意剪切、分轨、手工时间线编辑。
- 对所有视频类型建立复杂分类体系。第一版优先服务电商/营销短视频。
- 训练或微调模型。

## 核心设计

新增一个 Agent 工具：`analyze_reference_video`。

这个工具由 Producer 调用，但 prompt 不完全由 Producer 生成。它采用三段式请求：

1. **工具固定分析协议**
   工具自带稳定 system / contract prompt，定义输出 schema、分析维度、安全边界和版权边界。

2. **Producer 生成 Analysis Brief**
   Producer 根据用户诉求、当前商品、项目目标和素材情况生成本次分析重点，例如“借鉴行李箱广告的 hook、产品揭示节奏和运镜，不复制原视频人物与品牌”。

3. **工具拼接媒体证据**
   工具负责附上视频 URL/File ID、关键帧、ffprobe 信息、可选音频转写和素材元数据。

这样可以让分析既稳定可测，又能跟随用户目标。

## 工具输入

`analyze_reference_video` 输入建议：

```json
{
  "brief": "分析这条参考视频的广告结构、运镜和节奏，用于改编成行李箱营销短视频。",
  "video_ref": {
    "type": "media_node",
    "key": "source.ref_video_01.node"
  },
  "focus": [
    "hook",
    "selling_script",
    "camera_language",
    "pacing",
    "subtitle_style",
    "audio_role"
  ],
  "adaptation_target": {
    "product": "悦行银灰色行李箱",
    "platform": "电商短视频",
    "aspect_ratio": "9:16",
    "duration_sec": 20
  }
}
```

`video_ref` 必须引用当前 workspace 内的 Agent source material，不接受模型编造 URL。

## 工具输出

工具输出必须结构化，第一版建议字段：

```json
{
  "summary": "参考视频是快节奏旅行场景广告，前三秒用问题钩子引出产品。",
  "reference_intent": {
    "preserve": ["前三秒强钩子", "产品卖点按使用场景出现", "推拉结合的展示运镜"],
    "ignore": ["原视频品牌", "原视频人物身份", "不可迁移的场景细节"],
    "must_be_original": ["商品外观", "旁白文案", "字幕文案", "最终镜头组合"]
  },
  "script_structure": {
    "hook": "用出行痛点开场",
    "beats": [
      "痛点出现",
      "产品进入画面",
      "功能证明",
      "使用场景",
      "购买理由"
    ],
    "cta": "强调轻松出行和限时购买"
  },
  "shot_breakdown": [
    {
      "index": 1,
      "purpose": "建立痛点",
      "visual": "人物在机场拖行普通行李箱显得吃力",
      "camera": "低机位跟拍",
      "motion": "向前移动",
      "estimated_duration_sec": 3,
      "adaptation_note": "改为展示悦行行李箱顺滑移动"
    }
  ],
  "camera_language": {
    "dominant_moves": ["低机位跟拍", "产品特写推近", "横向滑轨"],
    "rules": ["一镜一主要运镜", "产品特写镜头保持稳定"]
  },
  "pacing": {
    "tempo": "fast",
    "average_shot_sec": 2.5,
    "transition_style": "hard_cut_with_match_motion"
  },
  "audio": {
    "voiceover_role": "解释卖点",
    "bgm_role": "轻快旅行感",
    "sound_effects": ["轮子滑动声", "拉杆弹出声"]
  },
  "text_style": {
    "subtitle_density": "medium",
    "on_screen_text_role": "短卖点标签",
    "warnings": ["不要复制原字幕文案"]
  },
  "adaptation_plan": {
    "project_memory_patch": {
      "visual_anchors": ["机场晨光", "顺滑拖行", "硬壳箱体商业质感"],
      "forbidden": ["复制原品牌 Logo", "复刻原视频人物和字幕"]
    },
    "storyboard_suggestion": [
      {
        "client_key": "shot_01",
        "purpose": "痛点 hook",
        "camera_intent": "低机位跟拍",
        "creative_text": "出行赶时间时，普通箱子卡顿形成对比。"
      }
    ]
  },
  "confidence": 0.82,
  "warnings": ["参考视频有品牌元素，改编时必须替换为用户商品事实。"]
}
```

## Prompt 归属

Prompt 的责任边界如下：

| 部分 | 归属 | 原因 |
|---|---|---|
| 输出 schema、分析维度、安全边界 | `analyze_reference_video` 工具固定协议 | 保证稳定、可测、可入库 |
| 本次用户想借鉴什么 | Producer 生成 `brief/focus/adaptation_target` | Producer 了解用户目标和项目事实 |
| 视频 URL/File ID、关键帧、ffprobe、转写 | 工具自动拼接 | 避免 Producer 编造媒体证据 |
| 改编成 Storyboard 的最终取舍 | Producer | Producer 是创作事实 owner |
| Seedance 最终生成 prompt | Craftsman + PromptCompiler | Producer 不写 provider prompt |

因此，Producer 不能生成整段视频理解 prompt；工具也不能完全忽略 Producer 的业务目标。固定协议 + Producer brief 是第一版的正确边界。

## 模型调用策略

优先使用火山方舟支持视频理解的 Doubao Seed 多模态模型。

第一版调用策略：

1. 如果模型支持直接视频输入，工具使用视频 URL/File ID 作为主输入。
2. 工具同时生成关键帧和基础媒体信息，作为辅助证据和调试记录。
3. 如果视频理解调用失败或模型能力不可用，降级为关键帧 + ffprobe + 可选音频转写分析。
4. 工具记录 `provider_request` 和 `provider_response` 摘要，便于后续回放和调试。

不建议第一版只靠抽关键帧。原因是脚本节奏、运镜、转场、音画关系通常需要时序理解；关键帧只能兜底视觉内容。

## 数据模型

第一版建议新增 `reference_video_analysis` 表，而不是只塞进 `media_node.metadata`。

建议字段：

```sql
CREATE TABLE reference_video_analysis (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    source_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    brief TEXT NOT NULL DEFAULT '',
    focus JSONB NOT NULL DEFAULT '[]',
    model_provider TEXT NOT NULL DEFAULT '',
    model_id TEXT NOT NULL DEFAULT '',
    request_summary JSONB NOT NULL DEFAULT '{}',
    result JSONB NOT NULL DEFAULT '{}',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

理由：

- 分析结果是可复用的项目事实，不只是素材节点内部备注。
- Producer、Reviewer、Workbench、后续重分析都需要独立索引。
- 同一视频可以针对不同目标多次分析。

`media_node.metadata` 可以保留轻量摘要，例如 latest analysis id。

## API 与 Agent 工具

后端内部服务：

- `ReferenceVideoAnalysisService.Analyze(ctx, input)`
- 负责加载 asset、准备 provider-reachable URL、抽帧、调用 Doubao、校验 JSON、写 DB。

Agent native tool：

- `analyze_reference_video`
- Producer-only。
- 输入使用 semantic ref，不暴露内部 UUID。
- 输出返回 analysis ref、摘要、主要可借鉴点、主要风险。

可选 REST API：

- `POST /api/agent/workspaces/:workspaceID/reference-video-analyses`
- `GET /api/agent/workspaces/:workspaceID/reference-video-analyses`
- `GET /api/agent/workspaces/:workspaceID/reference-video-analyses/:id`

REST 第一版不是必须；如果只服务 Agent 主链路，可以先做 tool + projection。

## Producer 工作流

用户上传视频后：

1. Producer 读取上下文，识别可用 reference video source node。
2. Producer 加载 `reference-video-analysis-producer` skill。
3. Producer 调用 `analyze_reference_video`，传入用户诉求和分析重点。
4. Producer 根据 analysis result 更新 ProjectMemory：
   - `visual_anchors`: 可借鉴的风格和节奏。
   - `forbidden`: 不可复制元素。
   - `prompt_injection_hints`: 简短可复用约束。
   - `source_refs`: analysis ref。
5. Producer 调用 `upsert_storyboard` 创建或修改 Scene / Shot / dependency。
6. 如果存在高风险改编分歧，Producer 调用 `request_user_decision`。

Producer 不直接把分析 JSON 全部写入 ProjectMemory。ProjectMemory 只保存会影响全片一致性的精炼约束；完整分析保存在 `reference_video_analysis.result`。

## Craftsman 与 Reference Video 生成

Craftsman 在 shot video RenderPlan 中可以绑定参考视频：

```json
{
  "operation": "multi_modal_reference_video",
  "reference_bindings": [
    {
      "client_key": "ref_competitor_luggage_ad_motion",
      "source_type": "media_node",
      "source_id": "source.ref_video_01.node",
      "content_type": "video_url",
      "model_role": "reference_video",
      "semantic_target": "借鉴运镜节奏和产品展示密度，不复制人物、品牌和字幕",
      "priority": 2,
      "required": false,
      "notes": "仅作为 motion/style reference；商品外观以用户商品图为准。"
    }
  ]
}
```

这个能力对应 Seedance 2.0 API 的 `reference_video` / `video_url` 多模态参考。当前 ClipAnvil 代码已经支持这一形态的 schema 和 provider request builder，但还需要补 E2E 测试和 Workbench 可视化。

## Workbench 展示

Agent Workbench 第一版展示：

- Reference video source material 缩略图。
- Analysis card：
  - 分析状态：pending / running / succeeded / failed。
  - 参考视频摘要。
  - 可借鉴点。
  - 不可复制点。
  - 建议 storyboard beats。
  - 关联 ProjectMemory / Shots。

点击 analysis card 打开详情 panel，展示完整结构化结果和 provider request 摘要。

## 错误处理

必须覆盖：

- 视频文件不存在或非当前 workspace。
- 视频过大、过长或 provider 不可访问。
- Doubao 视频理解返回非 JSON。
- 模型拒绝分析。
- 抽帧失败。
- 降级路径：视频理解失败时使用关键帧分析。

工具返回自然语言错误时，应告诉 Producer 下一步：

- 让用户换一个更短的视频。
- 先用关键帧分析。
- 请求用户明确想借鉴哪一部分。
- 跳过 reference_video 绑定，只保留文字化风格约束。

## 安全与版权边界

工具固定协议必须要求：

- 不输出逐帧复刻计划。
- 不把参考视频中的品牌、Logo、人物身份、原字幕文案当成可复制事实。
- 输出 `must_be_original` 和 `do_not_copy`。
- Producer 在写 ProjectMemory 时必须保留禁止复制边界。
- Craftsman 绑定 `reference_video` 时，`notes` 必须说明参考用途和不可复制元素。

## 测试计划

后端单测：

- `analyze_reference_video` tool 只接受当前 workspace video source node。
- 工具将 Producer brief 与固定协议拼接进 provider request summary。
- provider 返回合法 JSON 时写入 `reference_video_analysis`。
- provider 返回非法 JSON 时保存 failed 状态和错误。
- 视频理解失败时可走关键帧降级。

RenderPlan / generation 测试：

- `content_type=video_url` + `model_role=reference_video` 通过校验。
- `multi_modal_reference_video` 至少需要一个 reference binding。
- provider request 中 video ref 进入 Seedance content item。

前端/Workbench 测试：

- source material reference video 可见。
- analysis card 显示摘要、状态和 warnings。
- analysis detail panel 可读。

Smoke：

- 上传一条短参考视频。
- Producer 分析并生成 Storyboard。
- 用户确认改编方向。
- Craftsman 为一个 shot 创建含 `reference_video` 的 RenderPlan。

## 分阶段实施

### Phase 1: 分析工具与持久化

- 新增 `reference_video_analysis` migration / sqlc。
- 新增 `ReferenceVideoAnalysisService`。
- 新增 Producer native tool `analyze_reference_video`。
- 使用 mock provider 完成单测。

### Phase 2: Volcengine Doubao 视频理解接入

- 扩展 Volcengine text/multimodal runtime，支持视频输入。
- 支持 provider-reachable URL/File ID。
- 支持关键帧降级。
- 记录 request/response summary。

### Phase 3: Producer 工作流

- 更新 `reference-video-analysis-producer` skill，要求先分析、再写 ProjectMemory/Storyboard。
- `read_project_context` 返回 analysis refs。
- Producer 可基于 analysis ref 创建 storyboard 和 HITL 决策。

### Phase 4: Workbench 可视化

- source material 区展示参考视频。
- Agent Workbench 增加 analysis card。
- Detail panel 展示完整分析。

### Phase 5: Reference Video 生成闭环

- 为 `multi_modal_reference_video` 增加 E2E 覆盖。
- Craftsman skill 增加 reference_video 使用规则。
- Reviewer 检查“借鉴而非复制”的一致性。

## 验收标准

第一版完成时应满足：

- 用户上传参考视频后，Producer 可以调用工具生成结构化分析。
- 分析结果可持久化、可被上下文读取、可在 Workbench 查看。
- Producer 能把分析转成 ProjectMemory 和 Storyboard，而不是只在聊天里总结。
- Craftsman 可以在 RenderPlan 中绑定该视频作为 `reference_video`。
- 系统清楚区分“可借鉴的镜头语言/节奏/脚本结构”和“不能复制的品牌/人物/字幕/独创表达”。

## 开放问题

- 是否第一版就做音频转写，还是先只用视频理解模型输出音频角色摘要？
- 视频时长上限设为 30 秒、60 秒还是按 provider 限制动态读取？
- Analysis card 是投影成单独 Workbench 节点，还是作为 Project overview 的子卡片？
- 是否允许用户手动选择“只借鉴脚本 / 只借鉴运镜 / 只借鉴字幕节奏”？

建议第一版默认：

- 视频上限 60 秒。
- 不单独做 ASR，先依赖视频理解模型；必要时再加转写。
- Analysis card 放在 Project overview 附近，点击进详情。
- Focus 由 Producer 从用户话语中推断，用户不明确时默认分析脚本、镜头、节奏和字幕。
