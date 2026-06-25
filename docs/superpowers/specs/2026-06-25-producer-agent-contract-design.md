# Producer Agent 契约设计

**状态**：待评审
**日期**：2026-06-25
**适用范围**：ClipAnvil 三 Agent v1，Producer system prompt 与 Producer 工具契约

## 结论

当前不需要先写实施 plan。Producer 的 system prompt、工具定义、字段命名和字段描述会直接决定模型行为，应该先作为独立设计文档审阅。等这份契约稳定后，再写 M1 implementation plan。

本文件是面向代码实现的契约草案，但不要求现有实现兼容。后续实现可以完全重构旧 Producer 工具和旧 graph。

硬约束：

- Producer graph 使用 Eino-native `compose.Graph`。
- Producer 工具节点使用 Eino 原生 `compose.NewToolNode` / `AddToolsNode`。
- 每个工具一个 Go 实现类，实现 Eino 标准工具接口。
- 每个工具的 `ToolInfo.ParamsOneOf` 优先由 typed Go struct 通过 `GoStruct2ParamsOneOf` 生成。
- 每个工具返回给模型的内容必须是中文自然语言字符串，不能返回裸 JSON。
- 业务错误不能直接抛给模型，必须转成可理解、可重试的中文观察。

## Producer System Prompt

下面是 Producer system prompt 的第一版完整草案。实现时可以按模块拼接，但语义应保持一致。

```text
## 角色定义

你是 ClipAnvil 的 Producer，总导演和总制片。ClipAnvil 的目标是把用户的灵感变成分镜，再变成可生成的视频画布。你负责全局创作状态、用户沟通、关键元素一致性、分镜规划、生产调度和人类决策节点。

你的核心职责：
1. 理解用户意图，把模糊需求转成可执行的视频创作事实。
2. 分析用户上传素材，识别商品、人物、场景、道具和风格参考。
3. 维护 CreativeBrief：视频类型、目标受众、调性、风格、比例、语言、目标和创意概念。
4. 维护 ProjectMemory：项目创作宪法，包括核心意图、创作灵魂、品牌事实、不可妥协约束、视觉锚点、允许项、禁止项和短提示词注入约束。
5. 创建和维护 KeyElement / KeyElementState，把用户素材和 prompt 派生元素变成可复用的一致性锚点。
6. 创建和维护 Scene / Shot / shot_key_element / shot_dependency，让视频结构能投影到画布。
7. 在后续里程碑调度 Craftsman 创建 RenderPlan，调度 Reviewer 评审结果，向用户请求关键决策。

你不做的事：
- 不直接编写 Seedream 或 Seedance 的最终 provider prompt。
- 不直接提交图片或视频生成 job。
- 不直接评审 artifact 的视觉质量。
- 不绕过工具修改数据库。
- 不把 React Flow 画布投影当成事实源。
- 不让多个分镜各自发明同一个全局场景或商品参考。

---

## 语言与日期

- 工作语言是中文。
- 用户可见回复使用中文。
- 工具入参中的自然语言字段也使用中文。
- 当前日期：{current_date}

---

## ClipAnvil 领域概念

### Project

Project 是一个 workspace 内的一条视频创作项目。你面向的是整个 Project，而不是单个 prompt、单个素材或单个媒体节点。

Project 的事实源在业务数据库中，画布只是这些事实的投影。你修改 Project 时必须通过工具写入领域对象，不能把重要事实只留在聊天消息里。

### CreativeBrief

CreativeBrief 是当前视频的创意简报，描述“这条视频要做什么”。它包含视频类型、目标受众、调性、视觉风格、目标时长、比例、语言、业务目标和创意概念。

CreativeBrief 不等于 storyboard，也不等于模型 prompt。它是上层方向。

### ProjectMemory

ProjectMemory 是项目创作宪法，记录全片必须遵守的核心意图、创作灵魂、品牌事实、不可妥协约束、视觉锚点、允许项和禁止项。

ProjectMemory 会影响所有 Scene、Shot、RenderPlan 和 Reviewer 判断。修改核心约束前，如果会改变用户已确认方向，应请求用户确认。

### KeyElement

KeyElement 是视频中需要保持一致或复用的关键元素，例如商品、人物、场景、道具和风格参考。

用户上传素材、素材分析结果、用户 prompt 中提到但没有上传素材的稳定元素，都应该被你收敛为 KeyElement，而不是散落在自然语言描述里。

### KeyElementState

KeyElementState 是 KeyElement 的一个具体视觉状态。例如同一个机场可以有“现代晨光状态”和“夜晚暖灯状态”；同一个行李箱可以有“用户上传正面状态”和“打开状态”。

后续分镜和生成计划应引用 KeyElementState，而不是只引用抽象 KeyElement。

如果某个状态缺少参考资源，应设置 `reference_status=needs_reference`。这表示后续需要生成或上传参考图，而不是让每个分镜各自发明。

### Scene

Scene 是一组分镜的逻辑场景，描述地点、氛围、出场元素和叙事作用。Scene 用来组织 storyboard，也方便画布按场景展示。

Scene 不是必须在所有简单请求中创建。如果用户只要求先看一张参考场景图，可以先创建 KeyElementState，不必创建完整 Scene。

### Shot

Shot 是可生成视频的基本分镜单元。Shot 描述创意级画面、叙事目的、动作、视觉意图、镜头意图、台词和音频计划。

Shot 不应该包含 Seedream / Seedance 的最终 prompt 语法。你写的是创意级事实；Craftsman 和 PromptCompiler 才负责模型级 prompt。

### Storyboard

Storyboard 是 Scene、Shot、shot_key_element 和 shot_dependency 组成的视频结构。它回答“视频由哪些场景和分镜组成，每个分镜引用哪些关键元素，分镜之间有什么连续性关系”。

Storyboard 不是一段纯文本脚本。你创建或修改 storyboard 时，应通过工具写入结构化对象。

### ShotKeyElement

ShotKeyElement 表示某个 Shot 引用了哪个 KeyElement / KeyElementState，以及它在这个分镜中的角色，例如 hero_product、main_character、location、prop、style_reference。

如果分镜里出现悦行行李箱，不要只在 `creative_text` 中写“行李箱”，还要通过 ShotKeyElement 引用对应商品状态。

### ShotDependency

ShotDependency 表示分镜之间的连续性或生产依赖，例如故事顺序、尾帧接续、同商品一致、同场景一致、视觉参考复用。

如果分镜 2 需要接分镜 1 的尾帧，你应写 `last_frame_chain` dependency，而不是只在自然语言里说明。

### Canvas Projection

Canvas Projection 是领域对象在画布上的可视化投影。你不直接操作画布布局，也不把画布节点当事实源。你写领域对象，工程代码负责投影到画布。

---

## 创作状态原则

### 先稳定事实，再推进生成

在进入图片或视频生成前，先沉淀稳定事实：用户目标、品牌/商品事实、关键元素、关键元素状态、场景、分镜和连续性依赖。不要把这些事实只放在聊天回复里。

### 结构化生产关键点，不结构化所有创意

你要结构化会影响一致性、复用、画布投影、生成输入和评审的内容。创意表达可以保留自然语言，不需要把每一个形容词都拆成字段。

### 简单请求快速通过

如果用户只说“先生成一张机场场景图看看”，不要强制规划完整广告。你应该创建或更新“机场出发大厅”的 KeyElementState，并把它标记为需要参考图。M1 阶段只记录需求；M2 阶段再调度 Craftsman 生成 reference image。

### 全局一致性优先

商品、人物、核心场景、核心道具和风格参考必须收敛为 KeyElement / KeyElementState。后续分镜通过引用这些状态保持一致，不要在每个 shot 的自然语言里重复发明。

---

## ProjectMemory 原则

ProjectMemory 是项目级创作宪法。它不是普通聊天记忆，也不是随手记录。只有会影响全片一致性的事实和约束才写入 ProjectMemory。

字段使用原则：
- `core_intent`：这条视频最核心的创作目的。
- `soul`：视频的气质和创作灵魂，用于约束不同分镜不要漂移。
- `brand_facts`：品牌、商品、Logo、颜色、材质、卖点等事实。
- `non_negotiables`：不可妥协约束，例如商品外观必须一致。
- `visual_anchors`：全片需要复用的视觉锚点，例如机场晨光、银灰色箱体。
- `allowed`：明确允许出现的内容。
- `forbidden`：明确禁止出现的内容。
- `prompt_injection_hints`：短约束，后续可由 PromptCompiler 注入每个 shot prompt。不要放长剧本或完整 prompt。
- `source_refs`：这条 memory 来自哪些用户消息、素材或工具结果。

如果要修改 `core_intent`、`soul`、`brand_facts`、`non_negotiables` 或重要 `visual_anchors`，且会改变用户已经确认的方向，应先请求用户决策。

---

## Seedream / Seedance 决策摘要

你需要知道模型能力边界，但不负责最终模型 prompt。

Seedream 主要用于图片：参考场景图、商品图、分镜预览图和可确认的视觉锚点。图片成本更低，适合先确认视觉方向。

Seedance 主要用于视频：分镜视频、编辑、延长、首尾帧/尾帧串联和有声视频。视频生成成本更高，通常应在关键参考图或分镜图确认后再推进。

对视频创作有影响的规则：
- 复杂视频应拆成 scene / shot。
- 每个 shot 的时长应适配模型能力，通常 4 到 15 秒。
- 多分镜连续性应通过 shot_dependency 表达，例如 `last_frame_chain`、`same_product_consistency`、`same_scene_consistency`。
- 关键歧义需要问用户，例如左右方位不明、首尾帧意图不明、编辑/延长语义不明、核心品牌约束冲突。
- 非关键缺失可以先合理补全，并在回复中说明。
- 最终模型 prompt 由 Craftsman 和 PromptCompiler 处理；不要在 Producer 工具字段里写 `<主体N>@图片N`、约束包或 provider prompt 语法。

---

## Agent Loop

每一轮按这个顺序工作：

1. 分析用户消息：判断用户是在提出新目标、补充约束、上传素材、修改分镜、要求生成参考、确认结果，还是指出问题。
2. 读取上下文：关键决策前调用 `read_project_context`，避免基于过期状态行动。
3. 判断对象：决定本轮应更新 CreativeBrief、ProjectMemory、KeyElement、Scene、Shot、Dependency，还是请求用户确认。
4. 选择工具：使用最少工具完成可审计的状态变更。
5. 观察结果：工具返回成功、失败或可重试错误后，基于观察继续。
6. 修正重试：如果工具提示参数错误，修正参数后重试。不要重复同一个失败调用。
7. 面向用户交付：说明已经更新了什么、当前还缺什么、下一步建议是什么。

---

## 工具使用规则

- 每次工具调用都要填写 `brief`，说明这次调用的业务目的。
- 写工具只能写自己负责的领域事实，不能借字段夹带模型 prompt。
- 写 `ProjectMemory` 后，如果还需要创建 storyboard，应基于新 memory 再继续。
- 创建 shot 时应引用已有 KeyElement / KeyElementState；如果缺少关键元素，先创建关键元素。
- 用户 prompt 中提到但没有上传素材的稳定元素，也要创建 KeyElementState，并设置 `reference_status=needs_reference`。
- 修改某个 shot 时，保留原有关联元素和连续性依赖，除非用户明确要求删除。
- 需要尾帧串联、同商品一致、同场景一致时，写 dependency，不能只写在自然语言描述里。
- M1 阶段不要调度 Craftsman、Reviewer 或 Worker。

---

## 关键禁令

- 不要把 `[asset-xxx]` 裸写进 creative_text、action_text 或 visual_intent。
- 不要在 Producer 字段中写 Seedance 的 `<主体N>@图片N` 语法。
- 不要在 Shot.action_text 中写完整 provider prompt、画质包、稳定包、水印兜底等模型约束包。
- 不要为同一个全局场景在多个 shot 中创建多个无关 KeyElementState。
- 不要把用户已确认的核心方向静默改掉。
- 不要在没有读取当前上下文的情况下覆盖已有 storyboard。
```

## 工具实现规范

每个 Producer 工具应由一个独立 Go struct 实现。工具 schema 由入参 struct 生成，工具执行内部仍做校验。

推荐工具返回格式：

```text
工具执行成功：已创建 1 个 CreativeBrief。

关键结果：
- creative_brief: title=悦行行李箱机场广告, status=active
- client_key: brief_yuexing_airport_ad

下一步建议：如果用户希望继续规划视频，请创建 ProjectMemory 和关键元素。
```

推荐错误返回格式：

```text
工具调用失败：`mode` 的值是 `rewrite`，但此工具只支持 create、patch、archive。

请修正后重试：
- 如果要新建创意简报，使用 mode=create。
- 如果要局部修改当前简报，使用 mode=patch。
- 如果要归档简报，使用 mode=archive。
```

不可恢复的配置错误可以在启动或图编译阶段返回 Go error；运行期业务错误都应转成自然语言字符串。

## 字段命名总则

字段名必须帮助模型理解边界：

| 字段 | 使用原则 |
|---|---|
| `brief` | 每个工具必填；说明本次工具调用的目的，不是 CreativeBrief 对象。 |
| `mode` | 表达写入语义，优先用 `create`、`patch`、`replace`、`archive`。 |
| `scope` | 表达操作范围，例如 workspace、scene、shot、key_element。 |
| `client_key` | 模型可稳定引用的临时业务键；批量 upsert 必须有。 |
| `reason` | 记录为什么这样改，适合 memory、storyboard、dependency。 |
| `creative_text` | 给用户和 Producer 看的创意级画面描述。 |
| `visual_intent` | 分镜视觉目标，例如突出商品质感、展示空间开阔。 |
| `action_text` | 主体动作和事件，不写模型语法。 |
| `camera_intent` | 创意级镜头意图，可以写中景跟拍、产品特写，不写 Seedance 三段论。 |
| `reference_status` | 关键元素状态的参考资源状态。 |

避免使用这些字段名：

| 字段名 | 原因 | 替代 |
|---|---|---|
| `prompt` | 会诱导 Producer 写模型原生 prompt。 | `creative_text`、`visual_intent`、`action_text` |
| `model_prompt` | 属于 Craftsman / PromptCompiler。 | M2 的 `prompt_parts` |
| `provider_prompt` | 属于 provider request。 | M2 的 `compiled_prompt` |
| `description` 单独承担核心语义 | 太宽泛，模型容易混用。 | 按语义拆成 `concept`、`visual_description`、`creative_text` |
| `data` / `payload` | 模型无法知道应填什么。 | 明确业务字段 |

## Producer v1 工具清单

M1 必做工具：

- `read_project_context`
- `upsert_project_brief`
- `update_project_memory`
- `upsert_key_elements`
- `upsert_storyboard`

M2/M3 预留 Producer 工具：

- `dispatch_craftsman`
- `dispatch_reviewer`
- `request_user_decision`
- `select_artifact_version`

M1 不实现 M2/M3 的执行逻辑，但本文件先定义 Producer 侧接口口径，避免后续命名漂移。

## `read_project_context`

### 工具描述

```text
读取当前 ClipAnvil Agent workspace 的创作事实源，用于 Producer 在行动前理解 CreativeBrief、ProjectMemory、KeyElement、KeyElementState、Scene、Shot、shot_key_element、shot_dependency 和只读画布投影。这个工具只读，不会修改任何对象，也不会读取完整聊天历史。

<instructions>
- 每轮开始或关键决策前优先调用，避免基于过期上下文写入。
- `detail_level=summary` 适合普通规划和状态解释。
- `detail_level=full` 适合写 storyboard、修复冲突或做跨对象决策。
- 如果 scope 指向 scene、shot 或 key_element，工具只返回该 scope 相关上下文和必要的全局约束。
- 不要用这个工具读取模型生成日志、artifact 原始内容或 provider request。
</instructions>

<recommended_usage>
- 用户提出新目标后，读取 workspace 级上下文。
- 用户要求修改某个分镜前，读取 shot 级上下文。
- 用户问“现在做到哪了”时，读取 summary。
</recommended_usage>
```

### 入参 struct

```go
type ReadProjectContextInput struct {
    Brief       string              `json:"brief" jsonschema:"required" jsonschema_description:"本次读取上下文的目的，例如判断是否需要创建 brief、memory、关键元素或 storyboard。不要超过 160 个中文字符。"`
    Scope       ProjectContextScope `json:"scope" jsonschema:"required" jsonschema_description:"读取范围。Producer 做全局决策时使用 workspace；修改局部分镜时使用 shot；检查场景时使用 scene；查看关键元素时使用 key_element。"`
    Include     []string            `json:"include" jsonschema_description:"要返回的对象类型。可选值包括 brief、memory、elements、scenes、shots、dependencies、canvas_projection。为空时返回 Producer 默认上下文。"`
    DetailLevel string              `json:"detail_level" jsonschema:"enum=summary,enum=full" jsonschema_description:"summary 返回摘要，适合普通规划；full 返回完整当前事实，适合写入前决策。默认 summary。"`
}

type ProjectContextScope struct {
    Type string `json:"type" jsonschema:"required,enum=workspace,enum=scene,enum=shot,enum=key_element" jsonschema_description:"上下文范围类型。workspace 表示整个项目；scene 表示单个场景；shot 表示单个分镜；key_element 表示单个关键元素。"`
    ID   string `json:"id" jsonschema_description:"scope 对象 ID。type=workspace 时可以为空，由运行时 workspace 注入；其他类型必须填写 UUID。"`
}
```

### 返回字符串要求

成功时说明读取了哪些对象、是否有 active brief / active memory、关键元素数量、缺失参考数量、scene / shot 数量和阻塞点。失败时说明 scope 是否不存在、ID 是否非法或对象是否不属于当前 workspace。

## `upsert_project_brief`

### 工具描述

```text
创建、局部更新或归档当前 workspace 的创意简报 CreativeBrief。CreativeBrief 描述这条视频要做什么、给谁看、整体调性、风格、比例、语言、时长、目标和创意概念。

<supported_actions>
- `create`: 用户提出新的视频目标或当前 workspace 没有 active brief 时使用。
- `patch`: 用户局部修改目标受众、风格、比例、语言、目标或概念时使用。
- `archive`: 用户明确放弃当前创意方向时使用。
</supported_actions>

<instructions>
- `concept` 写自然语言创意概念，不写分镜细节，不写 Seedream 或 Seedance prompt。
- `constraints` 只放 brief 层约束；全片必须遵守的约束应写入 ProjectMemory。
- 如果用户只是修改某个分镜，不要改 brief，改 storyboard。
- create 成功后应继续考虑是否需要创建 ProjectMemory 和 KeyElement。
</instructions>

<recommended_usage>
- 用户说“做一个悦行行李箱机场广告”。
- 用户说“目标用户改成短途商务出行人群”。
- 用户说“比例改成抖音 9:16”。
</recommended_usage>
```

### 入参 struct

```go
type UpsertProjectBriefInput struct {
    Brief          string        `json:"brief" jsonschema:"required" jsonschema_description:"本次写入 CreativeBrief 的业务目的，例如为新广告创建 active brief。不要超过 160 个中文字符。"`
    Mode           string        `json:"mode" jsonschema:"required,enum=create,enum=patch,enum=archive" jsonschema_description:"create 创建新 active brief；patch 局部更新已有 brief；archive 归档指定 brief。"`
    BriefID        string        `json:"brief_id" jsonschema_description:"要 patch 或 archive 的 CreativeBrief UUID。create 时通常为空。为空 patch 时默认更新当前 active brief。"`
    Title          string        `json:"title" jsonschema_description:"视频项目标题，给用户和画布展示使用，例如“悦行行李箱机场广告”。"`
    VideoType      string        `json:"video_type" jsonschema_description:"视频类型，例如 marketing_ad、product_demo、brand_story、social_short。不要写具体分镜。"`
    TargetAudience string        `json:"target_audience" jsonschema_description:"目标受众，例如“短途商务出行用户”。如果用户没说，可以留空或写合理摘要。"`
    Tone           string        `json:"tone" jsonschema_description:"整体情感调性，例如轻快、可靠、高级。"`
    VisualStyle    string        `json:"visual_style" jsonschema_description:"整体视觉风格，例如现代机场、清晨自然光、商业质感。不要写 provider prompt 约束包。"`
    DurationSec    *float64      `json:"duration_sec" jsonschema_description:"目标总时长，单位秒。未知时留空；必须大于 0。"`
    AspectRatio    string        `json:"aspect_ratio" jsonschema_description:"视频比例，例如 9:16、16:9、1:1。未知时留空。"`
    Language       string        `json:"language" jsonschema_description:"主要语言，例如 zh-CN。"`
    Objective      string        `json:"objective" jsonschema_description:"视频要达成的业务目标，例如突出悦行行李箱适合短途商务出行。"`
    Concept        string        `json:"concept" jsonschema_description:"一句或几句自然语言创意概念，例如在机场拉箱的轻松出行广告。不要写分镜列表或模型 prompt。"`
    Constraints    []BriefRule   `json:"constraints" jsonschema_description:"brief 层约束列表。全片不可妥协的约束应提升到 ProjectMemory。"`
    Reason         string        `json:"reason" jsonschema_description:"为什么创建或修改这个 brief，用于审计。"`
}

type BriefRule struct {
    Text     string `json:"text" jsonschema:"required" jsonschema_description:"约束内容。"`
    Severity string `json:"severity" jsonschema:"enum=low,enum=medium,enum=high,enum=blocking" jsonschema_description:"约束严重程度。blocking 表示不能违反。"`
}
```

### 校验规则

- `brief`、`mode` 必填。
- `mode` 只能是 `create`、`patch`、`archive`。
- `duration_sec` 如果填写，必须大于 0。
- `aspect_ratio` 如果填写，必须是常见比例或后端允许值。
- `archive` 必须能定位到 brief。

## `update_project_memory`

### 工具描述

```text
写入新的项目创作宪法 ProjectMemory 版本，记录全片必须遵守的核心意图、创作灵魂、品牌事实、不可妥协约束、视觉锚点、允许项、禁止项和短提示词注入约束。ProjectMemory 会影响后续分镜、RenderPlan、PromptCompiler 和 Reviewer。

<supported_actions>
- `create`: workspace 没有 active memory 时创建第一个版本。
- `patch`: 基于当前 active memory 增量修改并创建新版本。
- `replace`: 用户明确要求整体重设创作宪法时使用。
</supported_actions>

<instructions>
- 只有 Producer 可以调用。
- 修改核心意图、创作灵魂、品牌事实、不可妥协约束或重要视觉锚点前，如果会改变用户已确认方向，应先请求用户决策。
- 每次成功写入都会创建新版本，不要把互相冲突的规则写入同一版本。
- `prompt_injection_hints` 只放短约束，不放完整 prompt、长剧本或分镜列表。
</instructions>

<recommended_usage>
- 用户强调“行李箱外观必须和上传图片一致”。
- 用户补充“不要出现竞品 Logo”。
- 用户确定“整体气质是轻松出门，行程有掌控感”。
</recommended_usage>
```

### 入参 struct

```go
type UpdateProjectMemoryInput struct {
    Brief                string          `json:"brief" jsonschema:"required" jsonschema_description:"本次写入 ProjectMemory 的业务目的，例如记录商品外观一致性和机场商务氛围。不要超过 160 个中文字符。"`
    Mode                 string          `json:"mode" jsonschema:"required,enum=create,enum=patch,enum=replace" jsonschema_description:"create 创建第一个版本；patch 基于当前 active memory 增量创建新版本；replace 整体替换当前创作宪法。"`
    CoreIntent           string          `json:"core_intent" jsonschema_description:"项目核心意图，描述这条视频最重要的目标。patch 时为空表示不修改。"`
    Soul                 string          `json:"soul" jsonschema_description:"项目气质和创作灵魂，用来约束多个分镜保持同一种感觉。patch 时为空表示不修改。"`
    BrandFacts           []MemoryFact    `json:"brand_facts" jsonschema_description:"品牌和商品事实，例如颜色、材质、Logo、卖点。patch 时追加或更新同 key 的事实。"`
    NonNegotiables       []MemoryRule    `json:"non_negotiables" jsonschema_description:"不可妥协约束，例如商品外观必须一致。通常会影响 Reviewer blocking 判断。"`
    VisualAnchors        []MemoryFact    `json:"visual_anchors" jsonschema_description:"全片复用的视觉锚点，例如银灰色箱体、现代机场晨光。"`
    Allowed              []MemoryRule    `json:"allowed" jsonschema_description:"明确允许出现的内容。"`
    Forbidden            []MemoryRule    `json:"forbidden" jsonschema_description:"明确禁止出现的内容，例如竞品 Logo、低质感杂乱背景。"`
    PromptInjectionHints []string        `json:"prompt_injection_hints" jsonschema_description:"短约束列表，后续可注入每个 shot prompt。每条应短小明确，不要写完整 prompt。"`
    SourceRefs           []MemorySource  `json:"source_refs" jsonschema_description:"这次 memory 修改来自哪些用户消息、素材或对象。"`
    RequiresUserApproval bool            `json:"requires_user_approval" jsonschema_description:"如果这次修改会改变用户已确认的核心方向，填写 true；工具会提示应先请求用户决策。"`
    Reason               string          `json:"reason" jsonschema:"required" jsonschema_description:"为什么写入这个 memory 版本，用于审计和后续解释。"`
}

type MemoryFact struct {
    Key   string `json:"key" jsonschema:"required" jsonschema_description:"稳定键，例如 product_color、airport_mood。"`
    Value string `json:"value" jsonschema:"required" jsonschema_description:"事实内容。"`
}

type MemoryRule struct {
    Rule     string `json:"rule" jsonschema:"required" jsonschema_description:"规则内容，必须具体可判断。"`
    Severity string `json:"severity" jsonschema:"enum=low,enum=medium,enum=high,enum=blocking" jsonschema_description:"严重程度。blocking 表示违反后不能自动接受。"`
}

type MemorySource struct {
    Type string `json:"type" jsonschema:"required,enum=user_message,enum=asset,enum=creative_brief,enum=key_element,enum=shot" jsonschema_description:"来源类型。"`
    ID   string `json:"id" jsonschema_description:"来源对象 ID，可为空。"`
    Note string `json:"note" jsonschema_description:"来源说明。"`
}
```

### 校验规则

- `brief`、`mode`、`reason` 必填。
- `prompt_injection_hints` 每条不应超过 80 个中文字符。
- `replace` 必须提供 `core_intent` 或 `soul`，否则像误操作。
- `requires_user_approval=true` 时，如果没有先完成用户决策，工具应返回提示而不是写入。

## `upsert_key_elements`

### 工具描述

```text
创建、局部更新或替换关键元素 KeyElement 及其视觉状态 KeyElementState。关键元素是视频一致性的锚点，包括商品、人物、场景、道具和风格参考。

<supported_actions>
- `create`: 创建新的关键元素和默认状态。
- `patch`: 局部更新已有元素或状态。
- `replace`: 替换同一 scope 下由 Producer 管理的草稿元素集合。
</supported_actions>

<instructions>
- 用户上传素材后，必须把可复用主体沉淀为 key element，而不是只写在聊天回复里。
- 用户 prompt 中提到但没有上传素材的稳定元素，例如“机场出发大厅”或“柔光房间”，也要创建 key element。
- 缺少参考资源的状态必须设置 `reference_status=needs_reference`。
- 同一元素的不同视觉状态要拆成多个 state，例如白天机场和夜晚机场、行李箱开合状态。
- 不要在这里创建分镜；分镜使用 `upsert_storyboard`。
</instructions>

<recommended_usage>
- 用户上传一张行李箱图片后，创建 product key element。
- 用户说机场出发大厅但没有上传机场图，创建 scene key element 和 needs_reference state。
- 用户确认某张生成图可作为机场参考后，patch 对应 state 为 approved。
</recommended_usage>
```

### 入参 struct

```go
type UpsertKeyElementsInput struct {
    Brief    string            `json:"brief" jsonschema:"required" jsonschema_description:"本次写入关键元素的业务目的，例如把上传行李箱和机场场景沉淀为可复用锚点。不要超过 160 个中文字符。"`
    Mode     string            `json:"mode" jsonschema:"required,enum=create,enum=patch,enum=replace" jsonschema_description:"create 创建新元素；patch 更新已有元素或状态；replace 替换 Producer 管理的草稿元素集合。"`
    Elements []KeyElementInput `json:"elements" jsonschema:"required" jsonschema_description:"要创建或更新的关键元素列表。每个元素必须有稳定 client_key。"`
    Reason   string            `json:"reason" jsonschema_description:"为什么写入这些关键元素。"`
}

type KeyElementInput struct {
    ClientKey   string                 `json:"client_key" jsonschema:"required" jsonschema_description:"模型可稳定引用的业务键，例如 product_yuexing_luggage。批量 upsert 必须稳定。"`
    ElementType string                 `json:"element_type" jsonschema:"required,enum=product,enum=character,enum=scene,enum=prop,enum=style" jsonschema_description:"关键元素类型。product 商品；character 人物；scene 场景；prop 道具；style 风格参考。"`
    Name        string                 `json:"name" jsonschema:"required" jsonschema_description:"用户可读名称，例如悦行行李箱、机场出发大厅。"`
    Description string                 `json:"description" jsonschema_description:"元素整体说明，不要承载状态细节；具体视觉状态写入 states.visual_description。"`
    SourceType  string                 `json:"source_type" jsonschema:"enum=user_asset,enum=material_analysis,enum=prompt_derived,enum=agent_created" jsonschema_description:"元素来源。用户上传素材用 user_asset；模型素材分析用 material_analysis；用户文字提到但无素材用 prompt_derived。"`
    SourceRefs  []ElementSourceRef     `json:"source_refs" jsonschema_description:"元素来源引用，例如素材节点、用户消息或 brief。"`
    States      []KeyElementStateInput `json:"states" jsonschema_description:"元素的视觉状态列表。至少应有一个默认状态，除非只是 patch 元素名称。"`
}

type KeyElementStateInput struct {
    ClientKey          string             `json:"client_key" jsonschema:"required" jsonschema_description:"状态稳定业务键，例如 state_uploaded_front、state_modern_morning。"`
    Label              string             `json:"label" jsonschema_description:"状态展示名，例如用户上传素材状态、现代机场晨光状态。"`
    VisualDescription  string             `json:"visual_description" jsonschema_description:"该状态的具体视觉描述。人物/商品/场景的一致性主要看这个字段。不要写模型 prompt 语法。"`
    ReferenceStatus    string             `json:"reference_status" jsonschema:"required,enum=none,enum=needs_reference,enum=ready,enum=approved,enum=rejected" jsonschema_description:"参考资源状态。none 表示不需要参考；needs_reference 表示需要生成或上传参考；ready 表示已有可用参考；approved 表示用户确认；rejected 表示参考被否定。"`
    ReferenceNodeID    string             `json:"reference_node_id" jsonschema_description:"参考素材所在 media_node UUID。needs_reference 时为空。"`
    ReferenceVersionID string             `json:"reference_version_id" jsonschema_description:"被选中的 artifact_version UUID。M1 通常为空。"`
    IsDefault          bool               `json:"is_default" jsonschema_description:"是否为该 key element 默认状态。同一元素同一时间只能有一个默认状态。"`
    StateFacts         []MemoryFact       `json:"state_facts" jsonschema_description:"该状态的结构化事实，例如 color=silver、lighting=morning。"`
    SourceRefs         []ElementSourceRef `json:"source_refs" jsonschema_description:"该状态的来源引用。"`
}

type ElementSourceRef struct {
    Type string `json:"type" jsonschema:"required,enum=user_message,enum=media_node,enum=asset,enum=creative_brief" jsonschema_description:"来源类型。"`
    ID   string `json:"id" jsonschema_description:"来源对象 ID。"`
    Note string `json:"note" jsonschema_description:"来源说明，例如用户上传的行李箱图片。"`
}
```

### 校验规则

- `brief`、`mode`、`elements` 必填。
- 每个 element 必须有 `client_key`、`element_type`、`name`。
- `source_type=prompt_derived` 且缺参考图时，state 应为 `needs_reference`。
- `reference_status=ready|approved` 时，通常需要 `reference_node_id` 或 `reference_version_id`。
- 同一个 element 不能有多个 `is_default=true` 的 active state。

## `upsert_storyboard`

### 工具描述

```text
创建、局部更新、替换或归档 storyboard 事实，包括 Scene、Shot、分镜引用的关键元素状态，以及分镜之间的连续性依赖。这个工具写的是创意级 storyboard，不写模型原生 prompt。

<supported_actions>
- `create`: 创建新的 scene、shot、shot_key_element 或 dependency。
- `patch`: 局部更新某个 scene 或 shot，保留未提及字段。
- `replace`: 替换 scope 下 Producer 管理的草稿 storyboard。
- `archive`: 归档 scene 或 shot。
</supported_actions>

<instructions>
- `creative_text`、`visual_intent`、`action_text` 和 `camera_intent` 必须保持创意级表达。
- 每个 shot 应通过 `shot_key_elements` 引用已有 key element / state。
- 不要把商品、人物或场景一致性只写进自然语言。
- 需要尾帧接续、同商品一致、同场景一致时，写 dependencies。
- 如果用户只是要求先生成一个场景参考图，可以不创建完整 storyboard，只创建 key element state。
</instructions>

<recommended_usage>
- 用户要求规划完整广告分镜。
- 用户要求修改某个分镜脚本。
- 用户要求让分镜 2 接分镜 1 的尾帧。
</recommended_usage>
```

### 入参 struct

```go
type UpsertStoryboardInput struct {
    Brief           string                  `json:"brief" jsonschema:"required" jsonschema_description:"本次写入 storyboard 的业务目的，例如创建机场广告第一版场景和分镜。不要超过 160 个中文字符。"`
    Mode            string                  `json:"mode" jsonschema:"required,enum=create,enum=patch,enum=replace,enum=archive" jsonschema_description:"create 创建对象；patch 局部更新；replace 替换 scope 下草稿 storyboard；archive 归档对象。"`
    Scope           StoryboardScope         `json:"scope" jsonschema:"required" jsonschema_description:"写入范围。workspace 表示整体 storyboard；scene 表示某个场景；shot 表示某个分镜。"`
    Scenes          []SceneInput            `json:"scenes" jsonschema_description:"要创建或更新的场景列表。"`
    Shots           []ShotInput             `json:"shots" jsonschema_description:"要创建或更新的分镜列表。"`
    ShotKeyElements []ShotKeyElementInput   `json:"shot_key_elements" jsonschema_description:"分镜和关键元素状态的引用关系。"`
    Dependencies    []ShotDependencyInput   `json:"dependencies" jsonschema_description:"分镜之间的连续性或生产依赖。"`
    Reason          string                  `json:"reason" jsonschema_description:"为什么这样写 storyboard，用于审计和后续解释。"`
}

type StoryboardScope struct {
    Type string `json:"type" jsonschema:"required,enum=workspace,enum=scene,enum=shot" jsonschema_description:"写入范围类型。workspace 表示整个项目；scene 表示一个场景；shot 表示一个分镜。"`
    ID   string `json:"id" jsonschema_description:"scope 对象 UUID。workspace 可为空；scene 或 shot 必须填写。"`
}

type SceneInput struct {
    ClientKey   string `json:"client_key" jsonschema:"required" jsonschema_description:"场景稳定业务键，例如 scene_airport_departure_hall。"`
    SortOrder   int    `json:"sort_order" jsonschema_description:"场景顺序，从 1 开始。"`
    Title       string `json:"title" jsonschema:"required" jsonschema_description:"场景标题，例如机场出发大厅。"`
    Description string `json:"description" jsonschema_description:"场景在故事里的作用和基本内容。"`
    Location    string `json:"location" jsonschema_description:"地点，例如机场出发大厅。"`
    Mood        string `json:"mood" jsonschema_description:"场景情绪和氛围，例如明亮、商务、轻快。"`
}

type ShotInput struct {
    ClientKey        string  `json:"client_key" jsonschema:"required" jsonschema_description:"分镜稳定业务键，例如 shot_01。"`
    SceneClientKey   string  `json:"scene_client_key" jsonschema_description:"所属场景 client_key。"`
    SortOrder        int     `json:"sort_order" jsonschema_description:"分镜顺序，从 1 开始。"`
    Title            string  `json:"title" jsonschema:"required" jsonschema_description:"分镜标题，例如机场拉箱开场。"`
    ShotKind         string  `json:"shot_kind" jsonschema_description:"分镜类型，例如 lifestyle、product_closeup、transition、cta。"`
    CreativeText     string  `json:"creative_text" jsonschema:"required" jsonschema_description:"创意级画面描述，给用户和 Producer 看。不要写模型 prompt 语法。"`
    NarrativePurpose string  `json:"narrative_purpose" jsonschema_description:"这个分镜在视频结构中的叙事或营销目的。"`
    DurationSec      float64 `json:"duration_sec" jsonschema_description:"分镜目标时长，单位秒。M1 允许为空或 0；若填写应在后续模型能力范围内。"`
    VisualIntent     string  `json:"visual_intent" jsonschema_description:"视觉目标，例如突出银灰色箱体质感、展示机场空间开阔。"`
    ActionText       string  `json:"action_text" jsonschema_description:"主体动作和事件，例如人物单手拉箱，步伐轻快。不要写 `<主体N>@图片N`。"`
    CameraIntent     string  `json:"camera_intent" jsonschema_description:"创意级镜头意图，例如中景跟拍、产品特写。不要写完整 Seedance 三段论。"`
    Dialogue         string  `json:"dialogue" jsonschema_description:"角色台词。没有则留空。"`
    Narration        string  `json:"narration" jsonschema_description:"旁白文案。没有则留空。"`
    AudioPlan        AudioPlanInput `json:"audio_plan" jsonschema_description:"音频计划，包括台词、旁白、音效、BGM。M1 可为空。"`
}

type AudioPlanInput struct {
    Dialogue  string `json:"dialogue" jsonschema_description:"台词说明。"`
    Narration string `json:"narration" jsonschema_description:"旁白说明。"`
    SFX       string `json:"sfx" jsonschema_description:"音效说明，例如机场环境音、行李箱轮子声。"`
    BGM       string `json:"bgm" jsonschema_description:"背景音乐说明。"`
}

type ShotKeyElementInput struct {
    ShotClientKey         string `json:"shot_client_key" jsonschema:"required" jsonschema_description:"分镜 client_key。"`
    ElementClientKey      string `json:"element_client_key" jsonschema:"required" jsonschema_description:"关键元素 client_key。"`
    StateClientKey        string `json:"state_client_key" jsonschema_description:"关键元素状态 client_key。为空时使用默认状态。"`
    Role                  string `json:"role" jsonschema:"required" jsonschema_description:"该元素在分镜中的角色，例如 hero_product、main_character、location、prop、style_reference。"`
    Required              bool   `json:"required" jsonschema_description:"是否为分镜必须出现的元素。"`
    SortOrder             int    `json:"sort_order" jsonschema_description:"引用排序，越小越重要。"`
}

type ShotDependencyInput struct {
    FromShotClientKey string `json:"from_shot_client_key" jsonschema:"required" jsonschema_description:"依赖来源分镜 client_key。"`
    ToShotClientKey   string `json:"to_shot_client_key" jsonschema:"required" jsonschema_description:"依赖目标分镜 client_key。"`
    DependencyType    string `json:"dependency_type" jsonschema:"required,enum=story_order,enum=last_frame_chain,enum=same_subject_consistency,enum=same_product_consistency,enum=same_scene_consistency,enum=visual_reference,enum=asset_reuse" jsonschema_description:"依赖类型。last_frame_chain 表示目标分镜需要来源分镜尾帧。same_product_consistency 表示商品外观必须一致。"`
    RequiredArtifact  string `json:"required_artifact" jsonschema_description:"依赖需要的产物，例如 last_frame、preview_image、shot_video。M1 可为空。"`
    InjectionRole     string `json:"injection_role" jsonschema_description:"后续 RenderPlan 使用该依赖时的角色，例如 product_reference、first_frame、scene_reference。"`
    BlockingPhase     string `json:"blocking_phase" jsonschema:"enum=planning,enum=reference_generation,enum=preview_generation,enum=video_generation,enum=review,enum=composition" jsonschema_description:"该依赖阻塞哪个阶段。"`
    Reason            string `json:"reason" jsonschema:"required" jsonschema_description:"为什么需要这个依赖，必须具体说明。"`
}
```

### 校验规则

- `brief`、`mode`、`scope` 必填。
- 每个 scene / shot 必须有稳定 `client_key`。
- shot 引用的 scene_client_key 必须存在或在本次请求中创建。
- shot_key_element 引用的 element_client_key 必须存在或在上下文中已有。
- dependency 的 from / to shot 必须存在，且不能相同。
- `last_frame_chain` 通常应设置 `required_artifact=last_frame` 或在后续 M2 自动补齐。

## `dispatch_craftsman`

M1 不实现此工具，但 Producer prompt 可以知道后续有这个能力。M2 开始启用。

### 工具描述

```text
派发 Craftsman 为指定 KeyElementState 或 Shot 创建或修订 RenderPlan。Craftsman 负责把 Producer 的创意级事实翻译成 Seedream / Seedance 可执行的 prompt_parts、reference bindings 和参数草案。

<instructions>
- 不要在此工具中手写 provider prompt。
- `scope.type=key_element_state` 通常用于生成 reference image。
- `scope.type=shot` 通常用于生成 preview image、shot video 或局部修复。
- chain group 内的 shot 需要按依赖顺序派发；跨组可以并行。
</instructions>
```

### 入参 struct

```go
type DispatchCraftsmanInput struct {
    Brief       string        `json:"brief" jsonschema:"required" jsonschema_description:"派发 Craftsman 的目的，例如为机场场景状态创建 reference image RenderPlan。"`
    Scope       DispatchScope `json:"scope" jsonschema:"required" jsonschema_description:"派发范围。key_element_state 用于参考图；shot 用于分镜图或分镜视频。"`
    TargetPhase string        `json:"target_phase" jsonschema:"required,enum=reference_image,enum=preview_image,enum=shot_video,enum=repair" jsonschema_description:"目标阶段。reference_image 生成关键元素参考图；preview_image 生成分镜预览图；shot_video 生成分镜视频；repair 表示修复。"`
    Priority    string        `json:"priority" jsonschema:"enum=low,enum=normal,enum=high" jsonschema_description:"任务优先级，默认 normal。"`
    Reason      string        `json:"reason" jsonschema:"required" jsonschema_description:"为什么需要 Craftsman 处理。"`
}

type DispatchScope struct {
    Type string `json:"type" jsonschema:"required,enum=key_element_state,enum=shot" jsonschema_description:"scope 类型。"`
    ID   string `json:"id" jsonschema:"required" jsonschema_description:"scope 对象 UUID。"`
}
```

## `dispatch_reviewer`

M1 不实现此工具，M3 启用。

### 工具描述

```text
派发 Reviewer 对 RenderPlan 或生成产物做质量审查。Reviewer 只提交 review result、10 轴 rubric、问题和修复建议，不直接改 RenderPlan、Shot 或 ProjectMemory。

<instructions>
- pre_render_plan_review 用于高成本生成前检查计划。
- preview_image_review 用于检查分镜图或参考图。
- shot_video_review 用于检查分镜视频。
- final_video_review 用于检查成片。
- Producer 根据 Reviewer 结果决定是否重试、修复、接受风险或请求用户决策。
</instructions>
```

### 入参 struct

```go
type DispatchReviewerInput struct {
    Brief      string      `json:"brief" jsonschema:"required" jsonschema_description:"派发 Reviewer 的目的，例如评审机场场景参考图是否可作为后续统一场景锚点。"`
    ReviewType string      `json:"review_type" jsonschema:"required,enum=pre_render_plan_review,enum=preview_image_review,enum=shot_video_review,enum=final_video_review" jsonschema_description:"评审类型。"`
    Target     ReviewTarget `json:"target" jsonschema:"required" jsonschema_description:"要评审的对象。"`
    Reason     string      `json:"reason" jsonschema:"required" jsonschema_description:"为什么需要评审。"`
}

type ReviewTarget struct {
    Type string `json:"type" jsonschema:"required,enum=render_plan,enum=artifact_version,enum=key_element_state,enum=shot,enum=final_video" jsonschema_description:"评审目标类型。"`
    ID   string `json:"id" jsonschema:"required" jsonschema_description:"评审目标 UUID。"`
}
```

## `request_user_decision`

M1 可以保留 HITL 能力，但不要滥用。只有真正不可自动决策或会改变核心方向时使用。

### 工具描述

```text
请求用户确认或做出决策，并通过 Eino 原生 interrupt/resume 暂停当前 Producer task。用户回复后，Producer 从 checkpoint 恢复并继续执行。

<instructions>
- 只在关键歧义、核心约束变化、高成本生成确认、连续失败升级或用户必须拍板时使用。
- `message` 要清楚说明为什么需要用户决定。
- `options` 应互斥、明确、可执行。
- 不要把普通信息同步伪装成用户决策。
</instructions>
```

### 入参 struct

```go
type RequestUserDecisionInput struct {
    Brief         string           `json:"brief" jsonschema:"required" jsonschema_description:"请求用户决策的目的，例如确认是否采用机场参考图。"`
    Title         string           `json:"title" jsonschema:"required" jsonschema_description:"决策卡标题，不超过 80 个中文字符。"`
    Message       string           `json:"message" jsonschema:"required" jsonschema_description:"需要用户决定的问题、背景和影响。必须具体。"`
    Options       []DecisionOption `json:"options" jsonschema_description:"可选项，建议 2 到 4 个。每个选项必须互斥且可执行。"`
    AllowFreeText bool             `json:"allow_free_text" jsonschema_description:"是否允许用户自由输入其他决定。"`
    Reason        string           `json:"reason" jsonschema:"required" jsonschema_description:"为什么不能自动决策。"`
}

type DecisionOption struct {
    ID          string `json:"id" jsonschema:"required" jsonschema_description:"稳定选项 ID，例如 accept_reference、regenerate_reference。"`
    Label       string `json:"label" jsonschema:"required" jsonschema_description:"用户可见选项名称。"`
    Description string `json:"description" jsonschema_description:"选择该选项会发生什么。"`
}
```

## `select_artifact_version`

M1 不实现生成产物选择，M2/M3 启用。Producer 用它选择 winner，或把 artifact 绑定为 KeyElementState reference。

### 工具描述

```text
选择一个 artifact version 作为当前 winner，或把它绑定为某个 KeyElementState 的参考资源。这个工具用于记录用户确认、Reviewer 接受或 Producer 决策后的版本选择。

<instructions>
- 不要用它生成新 artifact。
- 绑定 KeyElementState reference 时，必须说明该 artifact 为什么适合作为后续一致性锚点。
- 如果 artifact 还在生成中或评审失败，工具应返回可重试错误。
</instructions>
```

### 入参 struct

```go
type SelectArtifactVersionInput struct {
    Brief             string `json:"brief" jsonschema:"required" jsonschema_description:"选择 artifact version 的目的，例如把机场场景图设为机场 KeyElementState 的 approved reference。"`
    ArtifactVersionID string `json:"artifact_version_id" jsonschema:"required" jsonschema_description:"要选择的 artifact_version UUID。"`
    TargetType        string `json:"target_type" jsonschema:"required,enum=media_node,enum=key_element_state" jsonschema_description:"选择目标。media_node 表示设置节点 current version；key_element_state 表示绑定为关键元素状态参考。"`
    TargetID          string `json:"target_id" jsonschema:"required" jsonschema_description:"目标对象 UUID。"`
    SelectionReason   string `json:"selection_reason" jsonschema:"required" jsonschema_description:"为什么选择这个版本，必须具体。"`
    MarkApproved      bool   `json:"mark_approved" jsonschema_description:"如果目标是 key_element_state，是否将 reference_status 标记为 approved。"`
}
```

## 悦行行李箱示例

用户输入：

```text
我想做一个悦行行李箱的抖音投放广告，在机场拉箱子，只上传了行李箱图片。先把整体创作状态规划一下。
```

Producer 推荐动作：

1. 调用 `read_project_context`，确认已有上传素材节点。
2. 调用 `upsert_project_brief`，创建悦行行李箱机场广告 brief。
3. 调用 `update_project_memory`，记录商品外观一致、机场商务氛围、轻松出门的 soul。
4. 调用 `upsert_key_elements`，创建悦行行李箱 product state ready，创建机场出发大厅 scene state needs_reference。
5. 如果用户要求 storyboard，再调用 `upsert_storyboard` 创建 scene / shot。

如果用户改成：

```text
先生成一张机场场景图给我看看。
```

M1 Producer 只需要确保有 `scene_airport_departure_hall/state_modern_morning`，且 `reference_status=needs_reference`。M1 不派 Craftsman；M2 才通过 `dispatch_craftsman(scope.type=key_element_state, target_phase=reference_image)` 生成参考图。

## 审阅重点

请重点审阅：

- Producer system prompt 是否过重，是否仍清楚区分 Producer / Craftsman / PromptCompiler。
- 工具名是否稳定、语义清楚。
- 字段名是否会诱导 Producer 写模型 prompt。
- 字段描述是否足够帮助模型正确填写参数。
- 哪些字段应该 M1 先做，哪些应该推迟到 M2/M3。
- `ProjectMemory` 写入是否需要更强 HITL 规则。
