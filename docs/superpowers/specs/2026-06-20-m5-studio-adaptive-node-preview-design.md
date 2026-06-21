# M5 Studio Adaptive Node Preview 设计

**状态**：待评审
**日期**：2026-06-20
**阶段目标**：重构 Studio 画布节点和节点编辑弹窗，让文本、图片、视频等生产结果在画布上按内容类型自适应展示，同时把完整编辑、完整阅读和调试信息放到点击节点后的单一弹窗中。

## 1. 背景

M5 已经把 Studio 手动生产链路接入画布：节点可以配置 Prompt、选择模型、手动运行、产生 `generation_job` 和 `artifact_version`。但当前节点展示仍偏向早期固定卡片：

- 文本输出在画布节点中只露出很少内容，长 Markdown 结果不可读。
- 图片输出被固定节点尺寸压缩，无法按图片比例展示。
- 节点底部和弹窗信息密度不合理，关键的 Prompt / Model / Params 编辑空间不够。
- 用户运行节点后，需要直接在画布上看到“结果已经变化”，而不是仍像草稿占位。

本 spec 聚焦 Studio 手动可用性，不引入 Agent 概念，不改变 Eino / Volcengine 运行链路。

## 2. 当前实现校准

当前相关实现位于：

- `apps/web/src/lib/canvas.ts`
  - `nodeToShapeProps`
  - `mediaNodeDisplaySize`
- `apps/web/src/shapes/MediaShapeUtil.tsx`
  - 自定义 tldraw media shape 渲染。
  - `canResize()` 当前为 `false`。
- `apps/web/src/components/PropertyPanel.tsx`
  - 节点点击后的生产编辑弹窗主体。
- `apps/web/src/main.css`
  - `.media-node-*` 与 `.property-*` 样式。
- `packages/canvas-schema/src/index.ts`
  - `MediaShapeProps` 的 shape props 定义。

当前问题的核心不是后端默认尺寸，而是前端缺少内容感知的展示策略。`mediaNodeDisplaySize` 现在只取 `canvas_w/canvas_h` 和最小尺寸的最大值，不会根据文本长度、Markdown 结构、图片比例或视频比例调整节点尺寸。

## 3. 产品原则

### 3.1 画布节点是结果预览，不是完整表单

画布节点需要快速表达：

- 节点类型。
- 标题和运行状态。
- 当前 winner 的最有用预览。
- 是否失败、运行中、stale。

复杂编辑、完整输出、版本、调用记录、错误详情放入点击节点后的弹窗。

### 3.2 内容类型决定展示方式

不同节点类型的最佳预览不同：

- Text：Markdown preview，优先展示生成结果；没有结果时展示 Prompt 摘要。
- Image：按图片比例完整 contain 展示；没有结果时展示图片占位。
- Video：按视频比例展示 poster 或播放器占位；没有结果时展示视频占位。
- Audio：展示简洁 waveform / duration 占位；完整播放控制后续再扩展。
- Reference Pack：展示成员摘要和缩略项，不参与本轮自动尺寸复杂化。

### 3.3 自动尺寸必须有边界

节点不能无限变大。自动尺寸应遵守：

- 按内容估算一个推荐尺寸。
- 设置不同 node type 的最小值和最大值。
- 超出最大值后节点内部滚动或裁切，但保留进入完整弹窗的入口。
- 不因 hover、状态文案、按钮出现而导致布局抖动。

### 3.4 用户手动 resize 优先于自动尺寸

本轮可以先实现“自动推荐尺寸”。如果开启 tldraw resize，应区分：

- auto size：系统根据内容推导。
- manual size：用户手动调整后持久化，后续不再被自动尺寸覆盖。

如果无法在本轮稳定地区分 manual size，则本轮不开放用户 resize，只完成更好的自动尺寸和弹窗完整查看。

## 4. 范围

### 4.1 包含

- 新增前端内容感知的节点尺寸策略。
- Text 节点在画布上以 Markdown preview 渲染生成结果。
- Image 节点按图片比例和最大边界自适应。
- Video 节点按合理比例展示预览区域。
- 节点状态视觉优化：running / failed / succeeded / stale 更容易识别。
- 点击节点后的弹窗重排：
  - Prompt 编辑作为主区域。
  - Model / Params 放在 Prompt 附近。
  - Output preview 提供完整阅读或完整查看区域。
  - Versions / Latest Job / Stale Reasons / Provider Request 作为二级调试信息。
- 补充前端纯函数测试和浏览器 E2E 验证。

### 4.2 不包含

- 不改变后端 production 数据模型。
- 不新增 Agent / storyboard / shot 概念。
- 不实现多版本 winner 手动切换。
- 不实现完整媒体播放器和时间轴剪辑。
- 不实现富文本 Prompt 编辑器。Prompt 仍以 textarea 编辑，Markdown 仅用于结果预览。
- 不实现复杂图片编辑、裁剪或缩放工具。

## 5. 推荐方案

采用“前端内容感知自动尺寸 + 弹窗完整查看”的方案。

备选方案 A 是只放大所有节点默认尺寸。实现最快，但无法解决长文本、竖图、宽图、视频比例差异，后续还会反复打补丁。

备选方案 B 是立即开放 tldraw 手动 resize。用户自由度高，但需要同时处理持久化、自动尺寸冲突、箭头端点刷新和多工作区一致性，风险更高。

推荐方案先把系统默认展示做专业：基于内容计算推荐尺寸，配合清晰最大值和完整弹窗。手动 resize 可以作为后续增量。

## 6. 详细设计

### 6.1 节点尺寸策略

新增前端纯函数，例如 `adaptiveMediaNodeSize(node)`，作为 `mediaNodeDisplaySize` 的升级版本。

输入：

- `node_type`
- `canvas_w`
- `canvas_h`
- `prompt`
- `production_preview`
- `reference_pack_preview`

输出：

- `w`
- `h`
- `size_mode: "auto" | "persisted"`

首版规则：

- Text：
  - 最小 `360 x 220`。
  - 默认宽度 `460`。
  - 根据 preview 文本长度估算高度。
  - 最大 `620 x 520`。
  - 超出后内容区滚动。
- Image：
  - 最小 `320 x 240`。
  - 默认 `480 x 360`。
  - 有图片比例时按比例 contain 到最大 `680 x 520`。
  - 没有图片比例时使用默认图片比例。
- Video：
  - 最小 `420 x 260`。
  - 默认按 `16:9`，最大 `720 x 460`。
- Audio：
  - 保持紧凑，默认 `360 x 140`。
- Reference Pack：
  - 默认 `360 x 220`，展示成员摘要。

如果当前数据库里已有明显大于默认的 `canvas_w/canvas_h`，首版把它视为 persisted size，避免覆盖用户或历史数据的布局。

### 6.2 图片比例来源

首选从后端 asset metadata 读取宽高。如果当前 API 尚未稳定提供图片宽高，本轮前端使用两层策略：

1. 如果 `production_preview` 或 asset metadata 有 `width/height`，直接使用。
2. 否则先用默认比例渲染，图片加载后通过 `onLoad` 得到 naturalWidth/naturalHeight，并只调整当前 shape 的前端展示尺寸。

首版不要求把 `naturalWidth/naturalHeight` 回写数据库，避免图片加载导致频繁写画布位置。

### 6.3 Markdown 渲染

Text 节点生成结果按 Markdown preview 展示。

依赖建议：

- `react-markdown`
- `remark-gfm`

画布节点中的 Markdown 需要做限制：

- 禁止 HTML。
- 限制表格、代码块、列表的样式，避免撑破节点。
- 代码块使用横向滚动。
- 标题字号受节点内部范围约束，不使用页面 hero 级字号。

弹窗中的 Output preview 可以使用同一套 Markdown 组件，但允许更高的最大高度和更舒适的行距。

### 6.4 节点视觉结构

节点结构调整为：

```text
Header: type icon / title / status
Body: content preview
Footer: optional compact meta only when useful
```

Footer 默认不展示“文本 PROMPT / 图片 PROMPT”这类重复信息。只有在确实有用户需要行动的信息时展示，例如：

- failed error 简短摘要。
- stale count。
- running progress。

### 6.5 单一节点弹窗

点击节点后只展示一个编辑弹窗，不恢复右侧常驻 inspector。

弹窗信息优先级：

1. Title / status / run action。
2. Prompt 编辑。
3. Operation / Model / Params。
4. Output preview。
5. Secondary details：
   - Versions。
   - Latest Job。
   - Stale Reasons。
   - Rendered Prompt。
   - Provider Request / Response。

Prompt textarea 应获得足够空间。调试信息默认折叠，不占主视觉面积。

### 6.6 运行状态刷新

运行节点后，画布节点必须及时从 draft / succeeded 切到 queued / running，并在完成后展示新的 winner preview。

如果 WebSocket 推送延迟，前端需要保留乐观运行状态或短轮询刷新 production state，避免用户点击运行后画布仍像没有反应。

## 7. 数据与接口影响

首版尽量不改后端数据库。

可能需要的前端类型扩展：

- `ProductionPreview` 增加可选 `width` / `height` / `duration_ms`，如果后端已经可从 asset metadata 透出。
- `MediaShapeProps` 可增加 `previewWidth` / `previewHeight`，用于 shape 渲染。

如果后端当前没有透出 asset metadata，首版可以先用前端 `img.onload` 兜底，不阻塞整体体验。

## 8. 可交付标准

- Text 节点生成长 Markdown 后，画布节点可读，弹窗可完整阅读。
- Image 节点生成图片后，画布节点按图片比例完整展示，不只露出局部。
- Video 节点展示稳定比例预览区域。
- 节点底部不再展示无意义的 `文本 PROMPT` / `图片 PROMPT`。
- Prompt / Model / Params 在弹窗中成为主交互区域。
- Versions / Latest Job / Stale Reasons / Provider Request / Response 默认作为二级信息。
- 运行节点后，画布状态能及时进入 queued / running，并在成功后展示 winner。
- 前端纯函数有测试覆盖。
- 浏览器 E2E 覆盖文本和图片核心链路。

## 9. 可验收标准

### 9.1 文本节点

给 Text 节点运行一个包含多级标题、列表、代码块和多段文字的 Markdown 输出：

- 画布节点展示 Markdown preview，而不是纯文本挤压。
- 节点高度随内容增加到最大值。
- 超过最大值后内部滚动。
- 点击节点后，弹窗 Output preview 可以完整阅读。

### 9.2 图片节点

给 Image 节点运行或绑定一张横图、一张竖图：

- 横图完整 contain 到节点内。
- 竖图不被裁切成局部。
- 节点尺寸不超过最大边界。
- 图片加载前后不出现明显布局抖动。

### 9.3 运行反馈

点击运行节点后：

- 节点状态立即显示 queued 或 running。
- 运行完成后显示 succeeded 和新预览。
- 失败时节点显示 failed，弹窗可读错误详情。

### 9.4 弹窗主次关系

选中任一普通生产节点：

- Prompt 编辑区足够大。
- Model / Params 可直接操作。
- Versions / Latest Job / Stale Reasons 不挤占主编辑区。
- Provider Request / Response 可展开查看，用于排查真实调用。

## 10. E2E 测试用例

### 用例 1：长 Markdown 文本输出

1. 创建 Studio Workspace。
2. 创建 Text 节点。
3. 使用 mock 或真实文本模型生成包含标题、列表、代码块和多段文字的输出。
4. 等待节点 succeeded。
5. 验证画布节点高度大于最小高度且不超过最大高度。
6. 验证节点内出现 Markdown 样式元素。
7. 点击节点，验证弹窗 Output preview 能看到完整内容。

### 用例 2：图片比例自适应

1. 创建 Image 节点。
2. 使用 mock image 或真实图像模型生成图片。
3. 等待节点 succeeded。
4. 验证图片在节点中完整可见。
5. 验证图片节点宽高在最大边界内。
6. 验证没有出现只露出图片一角的裁切。

### 用例 3：运行状态即时反馈

1. 创建任意可运行节点。
2. 点击运行。
3. 验证节点状态在短时间内从 draft/succeeded 变为 queued/running。
4. 等待完成。
5. 验证节点状态为 succeeded，并展示当前 winner preview。

### 用例 4：调用记录可排查

1. 运行一个依赖上游文本的图片节点。
2. 点击节点打开弹窗。
3. 展开 Latest Job / Provider Request。
4. 验证可以看到 `rendered_prompt` 或 provider request prompt。
5. 验证 Prompt 主编辑区域仍然可见，不被调试信息挤压。

## 11. 建议拆分

### Phase 1：尺寸策略与节点预览

- 新增自适应尺寸纯函数和测试。
- 移除无意义底部文案。
- Text / Image / Video 节点使用新的展示尺寸。

### Phase 2：Markdown preview

- 引入 Markdown 渲染依赖。
- 抽出 `MarkdownPreview` 组件。
- 画布节点和弹窗 Output preview 共用。

### Phase 3：弹窗主次重排

- Prompt / Model / Params 放到主区域。
- Output preview 放到主区域。
- Versions / Latest Job / Stale Reasons / Provider Request / Response 默认折叠。

### Phase 4：浏览器 E2E

- 使用内置浏览器访问本地 Vite 页面。
- 覆盖长 Markdown、图片比例、运行状态、调用记录四条主路径。

## 12. 风险与约束

- tldraw shape 尺寸变化会影响箭头连接点，需要同步验证 edge geometry。
- 图片 natural size 如果只存在前端内存中，刷新页面后会先回到默认比例再调整；可接受，但需要避免明显闪动。
- Markdown 渲染依赖需要通过前端 build 和 lint。
- 真实 Volcengine 调用耗时和费用不可控，E2E 默认用 mock provider；真实 provider 只做人工 smoke。
- 如果后端不透出 asset metadata，本轮不为了图片尺寸引入数据库迁移。

## 13. 验证命令

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

涉及后端 asset metadata 透出时追加：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```
