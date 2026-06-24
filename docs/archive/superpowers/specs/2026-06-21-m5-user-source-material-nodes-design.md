# M5 User Source Material Nodes 设计

**状态**：待评审  
**日期**：2026-06-21  
**推荐方案**：复用现有 `text/image/video/audio` node，新增明确的“素材模式”交互

## 1. 背景

M5 Studio 已经支持节点运行、版本、参考包、Prompt 引用和真实 provider 调用。但用户生产广告、商品图、视频脚本时，不只是从模型生成素材，也会带入自己的输入：

- 商品图片、品牌图片、包装图。
- 用户已有的视频素材或参考视频。
- 用户自己写好的视频脚本、口播文案、卖点列表。
- 后续可能还有音频参考、旁白参考等。

这些素材在画布里的角色不是“等待模型生成的节点”，而是“可被其他节点依赖的输入资源”。它们需要能进入参考包、能被 Prompt `@` 引用、能作为图像/视频生成 API 的输入，但不应该展示 Prompt、Model、Params，也不应该允许用户点击运行模型。

当前代码已经具备部分基础：

- `FileDropZone` 会上传图片/视频/音频文件，创建同类型 `MediaNode`，并绑定 `asset_id`。
- `UploadHandler` 会把上传文件存成 `media_asset`。
- `runDisabledReason` 已经对 `node.asset_id` 或 `operation_type === "upload"` 禁止运行。

问题在于：Studio UX 还没有把它表达成清晰的一等概念。用户拖入桌面图片时，如果看到的只是“一张图片”，会误以为它不是生产图里的图片节点；用户手写视频脚本时，也缺少一个“文本素材节点”的明确入口。

## 2. 产品原则

### 2.1 Node Type 表达内容类型，Node Mode 表达生产语义

不要新增 `source_image`、`source_video`、`source_text` 这类节点类型。节点类型仍然只表达内容：

- `text`
- `image`
- `video`
- `audio`
- `reference_pack`

同一个内容类型下，节点再分两种生产语义：

- **生成节点**：由模型生成，有 Prompt、Model、Params、Run、Versions。
- **素材节点**：由用户上传或手动输入，只作为输入资源，不运行模型。

这样可以复用现有依赖边、参考包、画布 node、Prompt 引用和 provider input resolver。

### 2.2 素材节点不是草稿

素材节点创建后应立即是可用资源。用户上传商品图或写入脚本后，不应该看到“草稿、等待运行”这类暗示。

素材节点的核心状态是：

- 内容是否存在。
- 是否可作为输入被引用。
- 文件或文本是否可预览。

不是：

- 是否有 Prompt。
- 是否选了模型。
- 是否运行过。

### 2.3 上传素材和生成素材都能成为上游输入

下游节点不应关心上游素材来自用户上传还是模型生成。它只需要拿到当前可用内容：

- 图片素材节点提供 image asset URL。
- 视频素材节点提供 video asset URL。
- 文本素材节点提供 text content。
- 生成节点提供 current version 的产物。

Prompt renderer 和 provider input resolver 应统一处理这两类上游。

### 2.4 参考包是素材集合，不是生成入口

参考包可以包含用户上传素材、用户手写文本素材、模型生成素材。参考包本身不运行模型。它只是把一组资源作为更高层的输入上下文传给下游。

## 3. 术语

### 3.1 生成节点

满足以下特征之一：

- `operation_type` 是模型生成类操作，例如 `text_generation`、`text_to_image`、`text_to_video`。
- 没有直接绑定用户上传的 `asset_id`。
- Inspector 中展示 Prompt、Model、Params、Run。

### 3.2 素材节点

满足以下特征之一：

- `operation_type = "upload"`。
- 节点绑定了 `asset_id`。
- `operation_type = "manual"` 且节点被标记为用户手写内容。

素材节点可以是：

- 图片素材节点。
- 视频素材节点。
- 音频素材节点。
- 文本素材节点。

### 3.3 用户素材

用户直接提供的内容，包括上传文件和手动输入文本。用户素材可以被复制、引用、放入参考包，但不能通过 Run 按钮重新生成。

## 4. 范围

### 4.1 包含

- 明确上传文件创建的是同类型“素材节点”，不是 React Flow 原生图片/视频 node。
- 图片/视频/音频上传节点不展示 Prompt、Model、Params、Run。
- 新增文本素材创建路径，用于用户手写或粘贴已有脚本。
- 文本素材节点的编辑区叫“内容”，不是“Prompt”。
- 素材节点可作为依赖上游。
- 素材节点可加入参考包。
- Prompt `@` 引用素材节点时，渲染为可读文本占位或实际文本内容。
- Provider 调用时，把素材节点的 asset URL 或 text content 作为输入传给模型。
- 为素材节点补充前端 helper 测试、API/生产输入解析测试、浏览器 E2E。

### 4.2 不包含

- 不新增新的 DB node type。
- 不实现素材库/资产管理中心。
- 不实现图片裁剪、视频剪辑、转码、缩略图生成后台任务。
- 不实现素材权限共享。
- 不实现上传文件的多版本管理。
- 不改变 Agent 模式的资源抽象。

## 5. 推荐数据设计

首期不新增表，不新增 node type。优先使用现有字段：

### 5.1 上传图片/视频/音频素材

创建 node 时：

```json
{
  "node_type": "image | video | audio",
  "operation_type": "upload",
  "asset_id": "<media_asset.id>",
  "title": "<filename without extension>",
  "status": "succeeded"
}
```

行为：

- `asset_id` 指向用户上传的 `media_asset`。
- `operation_type = "upload"` 明确它不是模型生成节点。
- `status = "succeeded"` 表示资源可用。
- 不创建 `generation_job`。
- 首期可以不创建 `artifact_version`，但 API 需要能把 `asset_id` 对应素材暴露给画布和 provider input resolver。

### 5.2 手动文本素材

创建 node 时：

```json
{
  "node_type": "text",
  "operation_type": "manual",
  "prompt": "<user provided text content>",
  "title": "视频脚本",
  "status": "succeeded"
}
```

首期复用 `prompt` 存正文内容，但 UI 上不称为 Prompt，而称为“内容”。这是为了少改表结构，同时复用现有 `MediaNode.prompt` 的同步、保存和 Prompt 引用链路。

后续如果需要更干净的数据模型，可以新增 `content` 或 `node_role` 字段迁移，但本轮不做。

### 5.3 生成节点

生成节点维持现有模型：

```json
{
  "operation_type": "text_generation | text_to_image | text_to_video | ...",
  "prompt": "<prompt template>",
  "model_provider": "...",
  "model_id": "...",
  "status": "draft | queued | running | succeeded | failed | stale"
}
```

## 6. 素材模式判定

前后端应沉淀统一 helper，避免不同组件各写一套判断。

建议规则：

```ts
isSourceMaterialNode(node) =
  node.operation_type === "upload" ||
  Boolean(node.asset_id) ||
  node.operation_type === "manual"
```

其中 `manual` 需要谨慎：如果未来有“手动编辑生成节点 Prompt”的语义，不能误判。因此首期建议只在“文本素材创建入口”创建 `operation_type = "manual"`，普通文本生成节点默认仍是 `text_generation` 或空操作等待选择。

衍生 helper：

- `isUploadMaterialNode(node)`
- `isManualTextMaterialNode(node)`
- `canRunProductionNode(node)`
- `materialKindLabel(node)`：`图片素材`、`视频素材`、`文本素材`。

## 7. UX 设计

### 7.1 创建入口

#### 拖拽上传

用户从桌面拖入图片/视频/音频文件时：

1. 文件上传到 workspace storage。
2. 创建对应类型素材节点。
3. 节点出现在释放位置。
4. 节点样式和普通 ClipAnvil node 保持一致，不创建 React Flow 原生 image node。

节点文案：

- 图片：`图片素材`
- 视频：`视频素材`
- 音频：`音频素材`

#### 手动文本素材

Studio 顶部创建工具需要提供清晰入口。推荐两种可选实现：

方案 A：保留现有“文本”按钮，点击后出现小菜单：

- `生成文本`
- `文本素材`

方案 B：工具栏直接增加 `文本素材` 按钮。

推荐方案 A。因为它能避免工具栏继续膨胀，也让用户理解“文本”下面有两种意图。

### 7.2 画布节点展示

素材节点 header 应明确显示素材身份：

```text
图片素材  商品主图  可用
视频素材  参考视频  可用
文本素材  视频脚本  可用
```

不要展示：

- `草稿`
- `等待输入 prompt`
- `Prompt`
- 模型名称

主体区域：

- 图片素材：完整 contain 图片预览。
- 视频素材：poster 或 video controls。
- 音频素材：audio placeholder 或 waveform。
- 文本素材：Markdown/text preview。

节点仍然有连接点和全屏查看入口。

### 7.3 Inspector

素材节点 Inspector 不展示生成表单。

#### 上传图片/视频/音频素材 Inspector

展示：

- 顶部标题，可双击编辑。
- 类型和可用状态。
- 素材预览。
- 文件信息：mime、尺寸、时长、大小。
- 全屏查看。
- 可加入参考包的入口或提示。
- 被哪些节点依赖的摘要可以后续做，本轮不强制。

不展示：

- Prompt textarea。
- Operation select。
- Model select。
- Params。
- Run button。
- Versions 主流程。
- Provider Request/Response。

#### 文本素材 Inspector

展示：

- 顶部标题，可双击编辑。
- 类型和可用状态。
- `内容` textarea。
- Markdown preview，可折叠或分栏后续再做。
- 保存状态。
- 全屏查看。

不展示：

- Prompt label。
- Model/Params。
- Run button。

### 7.4 参考包

参考包成员候选应包含素材节点和生成节点，但仍遵守：

- 参考包不能包含另一个参考包。
- 参考包不能把自己的成员作为自己的依赖上游形成语义循环。
- 上传素材节点和文本素材节点可以加入参考包。

参考包预览中应区分来源：

```text
商品主图 · 图片素材
视频脚本 · 文本素材
AI 主视觉 · 图片生成
```

## 8. Prompt 引用和 Provider Input

### 8.1 文本素材引用

如果下游 Prompt 写：

```text
根据 @视频脚本 生成一张九宫格 storyboard。
```

渲染规则：

- 如果 `@视频脚本` 指向文本素材节点，`rendered_prompt` 应包含该文本素材的正文内容。
- 可以保留标题边界，例如：

```text
根据如下文本素材生成一张九宫格 storyboard：

[视频脚本]
<用户粘贴的脚本正文>
```

### 8.2 图片/视频素材引用

如果下游 Prompt 写：

```text
参考 @商品主图 的行李箱外观，生成广告主视觉。
```

渲染规则：

- `rendered_prompt` 中不直接插入 URL。
- 文本中替换为稳定可读占位，例如 `图1（商品主图）`、`视频1（参考视频）`。
- Provider request 中必须把对应 image/video URL 加入多模态 input。

这和模型生成图片/视频节点的当前产物应使用同一套 input resolver。

### 8.3 参考包引用

如果下游依赖参考包，provider input resolver 应展开参考包成员：

- 文本素材成员进入 textual context。
- 图片/视频素材成员进入 media inputs。
- 生成节点成员使用 current version。

展开顺序使用参考包成员顺序，生成占位：

```text
图1（商品主图）
图2（AI 主视觉）
文本1（视频脚本）
```

## 9. Version 和 Asset 关系

首期素材节点不需要 versions，因为用户上传素材本身不是一次 provider 运行。

规则：

- 上传素材节点通过 `asset_id` 表达当前资源。
- 文本素材节点通过 `prompt` 表达当前正文。
- 如果用户替换上传素材，本轮可以先创建新节点，不做同节点替换版本。
- 如果后续要支持“替换素材并保留历史”，再引入素材版本或复用 artifact version。

下游依赖解析优先级：

1. 素材节点：
   - `asset_id` 或 manual text content。
2. 生成节点：
   - `current_version_id` 对应 artifact。
3. 兼容旧数据：
   - 如果节点有 `asset_id` 但 `operation_type` 为空，仍按上传素材处理。

## 10. API 和后端行为

### 10.1 创建上传素材节点

`FileDropZone` 创建 node 时应传：

- `operation_type = "upload"`
- `status = "succeeded"`
- `asset_id`

后端 `NodeHandler.Create` 需要允许该组合，并返回可预览 asset 信息。

### 10.2 创建文本素材节点

前端创建 node 时传：

- `node_type = "text"`
- `operation_type = "manual"`
- `status = "succeeded"`
- `prompt = initial content 或空字符串`

如果初始内容为空，UI 可以显示“等待输入内容”，但状态仍不应叫“草稿运行”。

### 10.3 Run API

如果调用 `/api/nodes/:id/run` 且节点是素材节点，后端应返回 400 或 409，错误信息明确：

```text
素材节点不需要运行模型。
```

不能只依赖前端隐藏按钮。

### 10.4 Production State API

素材节点的 production state 不应报错。

推荐返回：

- `latest_job = null`
- `versions = []`
- `current_version = null`
- `active_stale_reasons = []`

前端据此展示素材 Inspector，而不是显示“尚无 artifact version”的生成节点空态。

## 11. E2E 测试用例

### 11.1 拖拽图片创建图片素材节点

步骤：

1. 打开 Studio workspace。
2. 从测试 fixture 拖拽一张 PNG 到画布。
3. 等待上传完成。
4. 画布出现 `图片素材` 节点。
5. 节点展示图片预览。
6. 点击节点打开 Inspector。

验收：

- Inspector 不显示 Prompt / Model / Run。
- Inspector 显示素材预览和文件信息。
- 节点可作为依赖上游连接到图片/视频生成节点。

### 11.2 拖拽视频创建视频素材节点

步骤：

1. 拖拽 MP4 到画布。
2. 点击视频素材节点。

验收：

- 节点是 ClipAnvil video node，不是浏览器原生文件对象或 React Flow image node。
- Inspector 不显示 Run。
- 视频可预览或至少展示 poster/视频占位。
- 下游视频生成节点可引用它作为 media input。

### 11.3 创建文本素材节点并进入参考包

步骤：

1. 创建 `文本素材` 节点。
2. 粘贴一段视频脚本。
3. 创建参考包。
4. 把文本素材加入参考包。

验收：

- 文本素材 Inspector 的编辑区叫 `内容`。
- 不显示模型和运行按钮。
- 参考包成员列表包含该文本素材。
- 参考包预览显示文本素材摘要。

### 11.4 下游 Prompt 引用文本素材

步骤：

1. 创建文本素材 `视频脚本`。
2. 创建图片生成节点。
3. Prompt 写 `根据 @视频脚本 生成九宫格 storyboard`。
4. 运行图片生成节点。

验收：

- `generation_job.prompt_template` 保留 `@视频脚本`。
- `generation_job.rendered_prompt` 包含文本素材正文。
- provider request 使用渲染后的文本。

### 11.5 下游 Prompt 引用图片素材

步骤：

1. 上传图片素材 `商品主图`。
2. 创建图片生成节点。
3. Prompt 写 `参考 @商品主图 的外观生成广告图`。
4. 运行。

验收：

- `rendered_prompt` 中出现 `图1（商品主图）` 之类的可读占位。
- provider request 中包含该图片素材的 URL。
- 不把 URL 直接塞进 prompt 文本。

## 12. 验收标准

### 12.1 产品验收

- 用户拖入图片/视频后，看到的是 ClipAnvil 素材节点，而不是孤立媒体 node。
- 素材节点的视觉身份清楚：图片素材、视频素材、文本素材。
- 素材节点没有 Run 按钮，不会诱导用户运行模型。
- 文本素材使用“内容”而不是“Prompt”。
- 素材节点可以连接给下游生成节点。
- 素材节点可以加入参考包。
- 下游真实 provider 调用能拿到素材输入。

### 12.2 技术验收

- 前端存在统一 `isSourceMaterialNode` helper。
- 后端 Run API 对素材节点有明确保护。
- `production-state` 对素材节点返回稳定空状态，不报错。
- Prompt renderer 能处理文本素材、图片素材、视频素材。
- Provider input resolver 对上传素材和生成产物使用统一输入结构。
- E2E 覆盖上传图片、创建文本素材、参考包、Prompt 引用和 provider request。

## 13. 待确认问题

1. 文本工具栏采用“小菜单：生成文本 / 文本素材”，还是直接新增 `文本素材` 按钮？推荐小菜单。
2. 首期是否允许用户替换一个素材节点绑定的上传文件？推荐先不做，替换时新建节点。
3. 上传视频是否必须首期生成封面图？推荐不强制，但 video node 至少能播放或显示文件占位。

