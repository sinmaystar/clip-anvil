# Studio / Agent 共享生产画布设计方案

**状态**：待评审
**日期**：2026-06-18
**阶段目标**：定义 Studio 模式和 Agent 模式如何复用同一套底层画布、资源、生成、版本、评审和 Stale 能力，同时保持两种模式的控制权、上下文和用户心智隔离。

## 1. 背景

ClipAnvil 需要同时支持两种创作方式：

- **Studio 模式**：专业用户手动搭建媒体 DAG，创建节点、输入 Prompt、引用上游产物、选择模型、运行节点、手动处理 Stale。
- **Agent 模式**：Producer / Craftsman 自动规划分镜、创建引用、选择模型、调度 Worker、评审、重试、处理 Stale。

这两种方式不应拆成两套底层系统。它们应复用同一套 production core：

- Workspace
- MediaNode
- MediaEdge
- MediaAsset
- GenerationJob
- ArtifactVersion
- ReviewRecord
- Storage
- SandboxJob
- WebSocket 事件
- Stale 传播
- Provider Bridge

但两种模式的控制权必须分离：

- Studio Workspace 只允许用户手动编辑。
- Agent Workspace 只允许 Agent 编辑，画布对用户只读。
- 不做原地无缝切换。两种模式通过复制/导入 Workspace 互通。
- Studio 和 Agent 共享同一个 sandbox execution 边界：内部媒体处理、Agent shell、Composer 命令都走 Sandbox Job Service，不在应用容器执行。

## 2. 核心原则

### 2.1 底层能力共享，模式控制器分离

共享层：

```text
Production Core
  - 节点、边、资源、任务、版本、评审、Stale、存储、事件
```

模式控制器：

```text
StudioController
  - 用户手动创建节点
  - 用户手动写 prompt
  - 用户通过 @ 引用上游节点输出
  - 用户选择模型和参数
  - 用户点击运行、重跑、选择版本

AgentController
  - Producer 规划 shot 和 reference pack
  - Craftsman 自动写 prompt 和引用
  - Worker 执行生成
  - Agent 处理评审、重试和 Stale
```

### 2.2 节点是可运行的产物单元

Studio 和 Agent 共享同一个节点语义：

```text
Node = 输入配置 + 运行操作 + 输出产物 + 版本历史
```

节点不是单纯画布卡片。节点代表一个可以被运行、被引用、被版本化、被评审的生产单元。

### 2.3 连线是输入候选集，@ 是显式 Prompt 引用

Studio 模式保留手动连线。连线不是多种语义边，只表示：

```text
from_node 的 selected output 是 to_node 的输入候选资源。
```

Prompt 里的 `@` 引用表示：

```text
用户在 Prompt 中显式提到了某个已连接输入，并希望模型按该语义位置理解它。
```

因此：

- 用户可以先手动连线，再在 Prompt 的 `@` 菜单里从已连接输入中选择。
- 用户也可以直接在 Prompt 中 `@` 一个未连线节点，系统自动创建连线。
- 有连线但没有 `@` 的输入仍然可以作为隐式参考输入传给模型。
- UI 需要提示“有未在 Prompt 中引用的输入资源”，避免用户误以为 Prompt 已说明这些资源用途。

### 2.4 Asset 是底层存储，不是主要交互对象

用户和 Agent 在画布上主要操作 Node。Asset 是文件级资源存储，属于节点输出或上传结果。

规则：

- 用户上传图片：系统创建 image node，并把文件存为 media_asset。
- 用户上传视频：系统创建 video node，并把文件存为 media_asset。
- 生成任务完成：写 media_asset，再创建 artifact_version，并更新 node winner。
- Reference Pack 只收纳 node，不直接收纳裸 asset。

### 2.5 Studio 不强制分镜

Studio 模式中，分镜可以存在于用户脑子里，也可以由用户用 text node 表达。系统不强制引入 `shot`。

Agent 模式中的 `shot` 是 Producer 的生产管理实体，用于规划、引用、调度、评审和恢复上下文。它不是 Studio 模式的必备画布节点。

### 2.6 Reference Pack 是语义资产包，不是布局分组

Group 和 Reference Pack 都可以“包含节点”，但含义不同：

```text
Group
  = 布局组织。表示这些节点放在一起看。
  = 不产生可被模型整体引用的输出。

Reference Pack
  = 语义参考包。表示这些节点的 selected outputs 一起作为模型参考。
  = 可以被其他节点 @ 引用或 dependency 依赖。
```

## 3. Workspace 模式关系

### 3.1 不原地无缝切换

不建议让同一个 Workspace 在 Studio 和 Agent 之间原地切换。原因：

- Agent Workspace 有 Memory、PSS、shot、Craftsman conversation、decision history。
- Studio Workspace 是用户自由编辑的 DAG，不一定有清晰的 shot、叙事意图或 Agent 上下文。
- 原地切换会让“谁拥有上下文”和“谁可以编辑”变模糊。
- 复制/导入可以让用户明确知道当前正在进入另一种工作方式。

### 3.2 Agent -> Studio

Agent Workspace 可以复制为 Studio Workspace。

复制内容：

- media_node
- media_edge
- media_asset
- artifact_version
- 当前 winner
- reference_pack node
- media_group
- canvas 坐标和布局

不复制为活跃上下文：

- Producer 当前对话状态
- Craftsman 活跃任务
- pending decision
- Agent execution queue

复制后的 Studio Workspace 由用户手动编辑，Agent 不再跟随它。

### 3.3 Studio -> Agent

Studio Workspace 导入为 Agent Workspace 需要一次理解步骤。

流程：

```text
读取 Studio DAG、text nodes、prompt、reference packs、winner outputs、依赖关系
  -> 调用大模型理解用户已经做了什么
  -> 生成 Workspace Memory 草稿
  -> 生成 shots 草稿
  -> 建立 shot 与已有 media_node 的映射建议
  -> 让用户确认
  -> 创建 Agent Workspace
```

这个过程不能假设 Studio DAG 天然就是 Agent PSS。Studio 是自由创作图，Agent 需要结构化生产语义。

## 4. 节点类型

### 4.1 基础节点

首版建议保留少而稳定的节点类型：

| node_type | 输出 | 典型用途 |
|---|---|---|
| `text` | text asset | 想法、脚本、分镜文本、Prompt、字幕、旁白稿 |
| `image` | image asset | 上传图、生图、首帧/尾帧提取、封面图 |
| `video` | video asset | 上传视频、生视频、续写视频、成片 |
| `audio` | audio asset | BGM、旁白、音效 |
| `reference_pack` | reference pack output | 商品身份包、角色定妆包、品牌视觉包、风格参考包 |

不要把模型能力做成节点类型，例如不要新增 `image_to_video_node` 或 `text_to_image_node`。节点类型表示输出是什么，生成方式由 `operation_type` 表示。

### 4.2 Text Node

Text node 是可运行的文本产物节点。

它可以有 subtype：

| subtype | 说明 |
|---|---|
| `idea` | 创意想法 |
| `brief` | 项目/商品简报 |
| `script` | 视频脚本 |
| `storyboard_text` | 文本化分镜 |
| `prompt` | 给图片/视频模型使用的 Prompt |
| `caption` | 字幕/标题 |

Studio 示例：

```text
Text Node A
输入：帮我想一个燕麦拿铁广告创意
输出：创意方向文本

Text Node B
依赖：Text Node A
输入：基于 @A，把它写成 30 秒视频脚本
输出：视频脚本文本

Text Node C
依赖：Text Node B
输入：基于 @B，把它拆成 5 个分镜，每个分镜包含时长、画面和旁白
输出：分镜文本
```

这些 text nodes 不自动变成 Agent `shot`。只有当用户导入到 Agent 模式时，系统才用大模型把它们理解成 shots。

### 4.3 Image Node

Image node 输出图片。

常见 operation_type：

| operation_type | 说明 |
|---|---|
| `upload` | 用户上传图片 |
| `text_to_image` | 文生图 |
| `image_to_image` | 图生图或图像编辑 |
| `multi_image_to_image` | 多图参考生图或组合 |
| `extract_first_frame` | 从视频提取首帧，不调用模型 |
| `extract_last_frame` | 从视频提取尾帧，不调用模型 |

首帧/尾帧提取是 sandbox-backed internal operation。Studio 用户点击运行时仍然得到普通 `generation_job` 和 `artifact_version`，但实际 FFmpeg 命令必须记录为 `sandbox_job`。

### 4.4 Video Node

Video node 输出视频。

常见 operation_type：

| operation_type | 说明 |
|---|---|
| `upload` | 用户上传视频 |
| `text_to_video` | 文生视频 |
| `image_to_video` | 图生视频 |
| `video_to_video` | 视频编辑或续写 |
| `multi_reference_to_video` | 多模态参考生视频 |
| `compose` | 多段视频、音频和字幕合成 |

`compose` 是 sandbox-backed internal operation。Composer 不在应用进程中执行 FFmpeg，而是提交 sandbox job，由 sandbox 服务处理输入落位、命令执行、输出上传和失败记录。

### 4.5 Audio Node

Audio node 输出音频。

常见 operation_type：

| operation_type | 说明 |
|---|---|
| `upload` | 用户上传音频 |
| `text_to_speech` | 文本转语音 |
| `music_generation` | BGM 生成 |
| `sound_effect_generation` | 音效生成 |
| `audio_extract` | 从视频中提取音频 |

### 4.6 Reference Pack Node

Reference Pack 是特殊节点。它输出一个可整体引用的参考包。

典型用途：

| role | 说明 |
|---|---|
| `product_identity` | 商品身份包，如主图、侧面图、包装细节、Logo |
| `character_identity` | 人物/角色定妆包，如正脸、侧脸、服装、表情 |
| `brand_visual` | 品牌视觉包，如 Logo、色彩、字体、禁用规则 |
| `style_reference` | 风格参考包，如光影、构图、色调 |
| `scene_reference` | 场景参考包，如室内、户外、货架、办公室 |

## 5. Reference Pack 设计

### 5.1 Reference Pack 是 node

Reference Pack 本身是一个 media_node：

```text
node_type = reference_pack
operation_type = collect_references
```

它可以在画布上渲染、拖动、重命名、折叠、展开、编辑说明，并作为整体被 @ 引用。

画布表现示例：

```text
┌────────────────────────────┐
│ 商品身份包                 │
│ product_identity           │
├────────────────────────────┤
│ [主图] [侧面] [包装]        │
│ [Logo] [九宫格视角 1]       │
├────────────────────────────┤
│ 约束：保持瓶身比例和白绿包装 │
│ 输出：Reference Pack        │
└────────────────────────────┘
```

### 5.2 Reference Pack 只收纳节点

为了概念统一，Reference Pack 不直接收纳裸 asset。它只收纳 node。

规则：

- 用户上传图片到 pack：系统先创建 image node，再把该 image node 加入 pack。
- 用户上传视频到 pack：系统先创建 video node，再把该 video node 加入 pack。
- 用户拖已有节点进 pack：创建 pack membership。
- Agent 生成参考图后加入 pack：创建 image node，再加入 pack。

统一关系：

```text
media_asset = 文件/资源本体，后台存储
media_node = 画布上的可运行/可引用单元
reference_pack = 一组 media_node 的语义集合
```

### 5.3 Pack membership 与 dependency edge 分离

Reference Pack 的成员关系不是 generation dependency。

两类关系：

```text
media_edge
  = 这个节点运行时依赖哪个上游节点输出

reference_pack_membership
  = 哪些节点的 selected outputs 被选入参考包
```

建议表：

```sql
CREATE TABLE reference_pack_item (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    pack_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    member_node_id UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT '',
    label TEXT NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT unique_reference_pack_member UNIQUE (pack_node_id, member_node_id)
);
```

### 5.4 Pack 不自动递归收纳上游依赖

如果存在：

```text
TextNodeA -> ImageNodeB -> ImageNodeC
ImageNodeC 被拖入 ReferencePack
```

结果是：

```text
ReferencePack 只包含 ImageNodeC
不自动包含 TextNodeA / ImageNodeB
```

原因：

- Reference Pack 的语义是“这些输出作为参考”，不是“这些输出的完整生成血缘”。
- 上游文本 prompt 可能只是生成过程，不适合作为模型参考输入。
- 上游中间图可能是失败或过渡版本，不一定应该进入参考。
- 自动递归会让 pack 输入不可控。
- 用户很难理解为什么引用 pack 时带入了很多没有显式加入的内容。

成员节点的上游依赖仍可用于 provenance 展示：

```text
查看来源链路：
TextNodeA -> ImageNodeB -> ImageNodeC
```

但 provenance 不等于 pack 输出内容。

### 5.5 Pack 整体引用

Reference Pack 被引用时，默认展开的是它的直接成员节点 selected outputs 加 pack notes/constraints。

```text
ReferencePack contains: ImageNodeC, ImageNodeD
VideoNodeE @引用 ReferencePack
=> Provider Bridge 读取 ImageNodeC winner、ImageNodeD winner、pack constraints
=> 不自动读取 ImageNodeC / ImageNodeD 的祖先节点输出
```

如果成员节点 stale，pack 可标记为 `degraded` 或 `stale`，并提示用户更新、移除或继续使用当前 winner。

## 6. Prompt @ 引用

### 6.1 @ 引用是 Studio Prompt 的核心输入交互

用户在节点 Prompt 富文本中通过 `@` 引用其他节点输出。`@` 菜单优先展示当前节点的已连接输入资源。

显示层：

```text
基于 [商品主图缩略图] 和 [品牌视觉包]，生成一张适合小红书封面的清爽产品图。
```

底层保存为 prompt template：

```text
基于 {{node:product_image.output}} 和 {{node:brand_pack.output}}，生成一张适合小红书封面的清爽产品图。
```

如果用户 `@` 引用了尚未连线的节点，系统自动创建 dependency edge：

```text
product_image -> current_node
brand_pack -> current_node
```

如果用户删除 Prompt 里的 `@` chip，只删除该显式引用，不自动删除画布连线。用户需要在画布上删除连线，或在属性面板的输入资源列表里移除该输入候选。

### 6.2 @ 引用不是让用户手写模型输入格式

用户不需要手写“图1”“图2”。Provider Bridge 在提交时根据模型能力生成具体模型需要的格式。

示例：

```text
用户 Prompt：
基于 @商品图 和 @品牌视觉包，生成广告主视觉。

模型 A 支持多图参考：
  prompt: "基于图1和图2，生成广告主视觉。"
  images: [asset1, asset2]

模型 B 只支持文生图：
  拒绝运行，提示该模型不支持图片或 Reference Pack 引用。
```

### 6.3 连线但未 @ 的输入

用户可以手动把节点 A 连到节点 B，但不在 B 的 Prompt 中 `@A`。这表示 A 是 B 的输入候选和隐式参考。

UI 需要在 B 的属性面板或 Prompt 编辑器附近提示：

```text
输入资源：
- 商品主图：已连接，未在 Prompt 中引用
```

Provider Bridge 组装请求时：

```text
connected_inputs = media_edge 上游节点
explicit_refs = prompt_refs 中显式 @ 的节点
implicit_refs = connected_inputs - explicit_refs
```

处理规则：

- `explicit_refs` 按 Prompt 中 placeholder 位置渲染。
- `implicit_refs` 作为普通参考输入传给支持参考输入的模型。
- 如果模型不支持隐式参考输入，则运行前提示用户移除连线、换模型，或在 Prompt 中改成纯文本描述。
- 如果 provider 要求文本中出现图序，Bridge 可以自动追加中性说明，或要求用户显式 `@` 后再运行；具体策略由 model capability 配置决定。

### 6.4 首帧/尾帧作为节点操作处理

Studio 不需要先提供复杂的“首帧/尾帧输入槽”。用户可以显式创建中间节点：

```text
Video Node A 输出视频
Image Node B operation_type = extract_last_frame
Image Node B 依赖 Video Node A
Image Node B 输出尾帧图片

Video Node C 在 Prompt 中 @引用 Image Node B
Video Node C 生成续接视频
```

`extract_first_frame` 和 `extract_last_frame` 由 sandbox-backed internal operation 调用 ffmpeg 完成，不调用模型，不在应用容器执行。

Agent 模式可以自动创建这些中间节点。Studio 模式由用户手动创建或通过快捷操作创建。

## 7. Model Capabilities

### 7.1 UI 由模型能力驱动

不同模型支持的 operation 和输入不同。不能把表单写死。

规则：

```text
先确定 output_type 和 operation_type，再筛选可用模型。
```

例如：

```text
output_type = image
operation_type = image_to_image
inputs 包含 reference image
```

模型下拉应优先显示支持 `image_to_image` 的模型。只支持 `text_to_image` 的模型可以置灰，并提示原因。

如果用户先选择模型，则 UI 反过来约束 operation 和输入：

```text
选择只支持 text_to_image 的模型
-> 禁用图片引用
-> 已 @ 引用的图片显示“不被当前模型支持”
-> 运行前要求用户换模型或移除不支持输入
```

### 7.2 后端必须二次校验

前端提示不够，后端提交时必须校验：

```text
GenerationIntent + selected_model
  -> capability validate
  -> 不兼容则拒绝提交并返回可解释错误
```

### 7.3 能力配置

建议用配置驱动模型能力：

```json
{
  "provider": "volcengine",
  "model_id": "seedream-image-standard",
  "display_name": "Seedream Image Standard",
  "output_types": ["image"],
  "supported_operations": ["text_to_image", "image_to_image", "multi_image_to_image"],
  "supported_input_node_types": ["text", "image", "reference_pack"],
  "max_reference_images": 9,
  "durations_sec": [],
  "aspect_ratios": ["1:1", "3:4", "4:3", "9:16", "16:9"],
  "price_tier": "balanced",
  "async": true
}
```

视频模型示例：

```json
{
  "provider": "volcengine",
  "model_id": "seedance-video-standard",
  "display_name": "Seedance Video Standard",
  "output_types": ["video"],
  "supported_operations": ["text_to_video", "image_to_video", "multi_reference_to_video"],
  "supported_input_node_types": ["text", "image", "video", "audio", "reference_pack"],
  "max_reference_images": 9,
  "max_reference_videos": 3,
  "max_reference_audios": 3,
  "durations_sec": [4, 5, 8, 10, 15],
  "resolutions": ["720p", "1080p"],
  "aspect_ratios": ["9:16", "16:9", "1:1"],
  "price_tier": "premium",
  "async": true
}
```

具体参数以 provider adapter 的官方文档和控制台接入点配置为准。ClipAnvil 内部只依赖稳定的能力描述，不把某个模型的参数散落在 UI 和业务代码里。

## 8. GenerationIntent

Studio 和 Agent 都应提交同一种生成意图。

```json
{
  "workspace_id": "workspace-123",
  "target_node_id": "node-video-01",
  "output_type": "video",
  "operation_type": "image_to_video",
  "prompt_template": "基于 {{node:product_pack.output}}，生成 5 秒产品广告镜头。",
  "input_refs": [
    {
      "node_id": "node-product-pack",
      "kind": "reference_pack"
    }
  ],
  "model": {
    "provider": "volcengine",
    "model_id": "seedance-video-standard"
  },
  "params": {
    "duration_sec": 5,
    "aspect_ratio": "9:16",
    "resolution": "720p"
  },
  "requested_by": {
    "type": "user",
    "id": "account-123"
  }
}
```

Studio 用户手动填写这些内容。Agent Craftsman 自动生成这些内容。二者共用：

- capability validate
- provider request mapping
- generation_job
- artifact_version
- review_record
- WebSocket events
- Stale 传播

## 9. Provider Bridge

### 9.1 Bridge 的职责

Provider Bridge 负责把 ClipAnvil 的稳定 GenerationIntent 翻译为具体供应商请求。

职责：

- 校验模型能力。
- 展开 @ 引用节点输出。
- 展开 Reference Pack 成直接成员节点 selected outputs。
- 按供应商格式拼接 prompt/messages。
- 上传或转换输入文件。
- 提交同步或异步任务。
- 轮询或接收任务结果。
- 标准化错误、费用、耗时、输出资产。

### 9.2 Eino 与 Provider Bridge 的边界

不要重复造 Eino 已提供的 Agent LLM provider。

两类 provider 要分开：

```text
LLM Agent provider
  - Producer / Craftsman 使用
  - 使用 Eino + Ark AgenticModel
  - 负责对话、工具调用、流式输出、Agent 上下文

Media Generation provider
  - Studio 和 Agent 都使用
  - 使用 ClipAnvil Provider Bridge
  - 负责 image/video/audio generation API 的稳定抽象
```

Eino 适合 Agent 层。Studio 手动运行节点不经过 Eino Agent，但仍然需要调用图片/视频生成能力，所以 Media Generation Provider Bridge 必须由 ClipAnvil 自己维护。

### 9.3 火山引擎接入

本方案默认优先接入火山引擎模型：

- Agent LLM：Eino + Ark AgenticModel。
- 图片生成：火山方舟图片生成 API。
- 视频生成：火山方舟视频生成 API。

火山方舟当前文档导航已包含图片生成 API 和视频生成 API；Eino 官方也提供 Ark AgenticModel 集成。后续实现时需要再按具体接入点文档校准参数、鉴权、任务查询和错误码。

## 10. Stale 传播

Stale engine 共享，处理方式分模式。

触发：

- 上游节点 winner 变更。
- 上游节点 prompt、params 或 model 变更。
- Reference Pack membership 变更。
- Reference Pack 成员节点 winner 变更或 stale。
- 文本节点输出变更。

底层行为：

```text
计算下游 input_hash
  -> input_hash 失效
  -> 下游节点标记 stale
  -> 广播 WebSocket
```

Studio 行为：

- 用户看到 stale 标记。
- 用户决定单个重跑、批量重跑、忽略或手动调整。
- 可以提供“级联重跑”按钮。

Agent 行为：

- Producer 在 PSS 中看到 stale。
- Craftsman 判断是否需要改写 prompt。
- Producer 根据成本、用户偏好和 decision policy 自动重跑或请求确认。

## 11. 运行示例

### 11.1 Studio 从想法到分镜文本

```text
1. 用户创建 Text Node A，输入“帮我想一个燕麦拿铁广告创意”，运行。
2. A 输出创意方向。
3. 用户创建 Text Node B，在 Prompt 中 @A，要求写成 30 秒脚本，运行。
4. B 输出视频脚本。
5. 用户创建 Text Node C，在 Prompt 中 @B，要求拆成 5 个分镜，运行。
6. C 输出分镜文本。
```

### 11.2 Studio 商品参考包

```text
1. 用户上传商品主图，系统创建 Image Node A。
2. 用户基于 A 生成九宫格角度图，系统创建 Image Node B。
3. 用户创建 Reference Pack Node P，role=product_identity。
4. 用户把 A 和 B 拖入 P。
5. 用户创建 Image Node C，在 Prompt 中 @P，生成广告主视觉。
```

如果 A 的上游有 Text Node，不自动进入 P。P 只包含用户显式加入的 A 和 B。

### 11.3 Studio 续接视频

```text
1. Video Node A 生成或上传一段视频。
2. 用户点击“提取尾帧”，系统创建 Image Node B，operation_type=extract_last_frame。
3. B 依赖 A，运行后输出尾帧图片。
4. 用户创建 Video Node C，在 Prompt 中 @B，要求续接动作生成下一段视频。
```

### 11.4 Agent 自动搭建相同 DAG

```text
1. Producer 创建 shots 和 Reference Pack。
2. Craftsman 为某个 shot 自动创建 Prompt 和 @引用。
3. Worker 提交 GenerationIntent。
4. Provider Bridge 调用火山 API。
5. 输出 asset/version，更新画布。
```

Agent 与 Studio 生成链路相同，只是 DAG 由 Agent 自动构建。

## 12. 当前文档需要同步的点

如果本方案通过评审，需要后续更新：

- `docs/design/studio-mode.md`
- `docs/design/agent-mode.md`
- `docs/design/canvas.md`
- `docs/design/overview.md`
- `docs/engineering/database.md`
- `docs/engineering/architecture.md`

重点同步：

- Studio 不强制 shot。
- Agent shot 是生产语义，不是 Studio 必备节点。
- Reference Pack 是可渲染、可拖动、可整体引用的特殊 node。
- Pack membership 只显式收纳 node，不自动递归上游依赖。
- 手动连线表示输入候选集，@ 引用表示 Prompt 中的显式引用。
- 连线但未 @ 的输入可作为隐式参考输入，并需要 UI 提示。
- 首帧/尾帧用 extract frame 节点操作表达。
- Studio 和 Agent 共用 GenerationIntent、Provider Bridge、Job、Version、Review、Stale。
- Eino 用于 Agent LLM provider，Media Generation Provider Bridge 由 ClipAnvil 维护。

## 13. 开放问题

1. `reference_pack` 是否纳入现有 `media_type` enum，还是引入新的 `node_kind` 与 `output_type` 分离？
2. `reference_pack_item` 是否允许成员节点来自其他 workspace 的复制版本，还是必须先复制到当前 workspace？
3. 隐式输入在不同 provider 中的默认行为是自动传入，还是要求用户显式 @ 后才能运行？
4. Text node 的输出是否存为 text 类型 media_asset，还是直接存在 artifact_version output JSON 中？
5. Provider capabilities 是写配置文件、数据库表，还是从 provider API 定期同步？
6. Studio 中 Reference Pack 的成员排序是否影响 Provider Bridge 展开的图序？
7. 是否需要为 Reference Pack 增加 `locked` 状态，避免误改商品身份包影响大量下游节点？
8. 是否需要支持 Pack 版本，即某次生成使用的是 pack membership 的历史快照？
