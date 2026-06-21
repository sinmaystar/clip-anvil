# M5 Studio Manual Production 设计与工作项

**状态**：待评审
**日期**：2026-06-18
**阶段目标**：把 Studio 从“手动画布编辑器”升级为“专业手动生产工具”，让用户可以在画布上创建生产节点、组织 Reference Pack、配置 Prompt 和模型、手动运行节点、查看版本、处理失败和 stale。

## 1. 背景

M3 已完成 Workspace 模式入口，M4 已完成 Studio / Agent 共用的生产底座。当前 M5 的任务不是重新建设生成链路，而是把 M4 已有的生产事实和接口接入 Studio 体验。

M5 完成后，Studio Workspace 应能独立完成：

```text
素材上传或创建
  -> 建立 dependency 输入候选
  -> 创建 Reference Pack
  -> 在 Prompt 中 @ 引用输入
  -> 选择 operation / model / params
  -> 手动运行节点
  -> 查看 current winner / version / job
  -> 根据 stale 提示逐个重跑
```

## 2. 当前实现校准

### 2.1 已具备

后端已经提供 M5 可以复用的生产底座：

- `media_node` 已有 `operation_type`、`prompt_template`、`prompt_refs`、`model_provider`、`model_id`、`model_params`、`current_version_id` 和 `metadata`。
- 已有 `generation_job`、`artifact_version`、`model_provider`、`model_capability`、`reference_pack_item`、`node_stale_reason`、`sandbox_job`。
- 已有节点运行、重试、版本、生产状态、stale reason、模型能力、Reference Pack membership API。
- `reference_pack` 已经是后端允许的 `node_type`。
- Agent Workspace 的普通用户画布写入已经被后端拒绝，M5 只需要面向 Studio Workspace。

### 2.2 主要缺口

前端 Studio 仍停留在 M1/M3 的画布编辑器形态：

- 前端 `MediaType` 仍只有 `text/image/video/audio`，缺少 `reference_pack`。
- 前端 `MediaNode` 类型尚未显式建模 production fields。
- `canvas-schema` 和 `MediaShapeUtil` 还不能渲染 `reference_pack`。
- 属性面板主要展示分组、依赖和 prompt，不是生产控制台。
- 前端还没有 run node、retry job、production state、model capability、reference pack item 的 API wrapper 和 UI。
- Prompt 仍是普通文本编辑，没有 `@` 引用、`prompt_refs`、隐式输入提示和删除依赖后的失效提示。
- 后端节点更新接口当前主要覆盖 prompt 文本和生产配置；M5.5 需要补齐 `prompt_refs` / `prompt_rich` 的稳定写入方式。
- M5 实现前需要确认 Provider Bridge 是否已经用 `supported_input_node_types` 校验输入节点类型；如果缺失，应在 M5.2 或 M5.5 补齐后端校验。

## 3. 产品原则

### 3.1 节点是生产单元

M5 中的节点不只是卡片。节点表示：

```text
输入配置 + operation + model + params + 输出 current winner + version history
```

画布卡片只展示最关键状态和预览，复杂配置放入属性面板。

### 3.2 连线是输入候选，Prompt @ 是显式引用

用户通过 dependency 连线建立输入候选。Prompt 中的 `@` 表示用户明确在语义上引用某个输入。

规则：

- 已连入节点优先出现在 `@` 菜单中。
- `@` 未连线节点时，系统自动创建 dependency。
- 删除 Prompt 中的 `@` 只删除显式引用，不自动删除 dependency。
- 删除 dependency 时，对应 `@` 应被移除或标记失效，节点不能带着无效引用成功运行。
- 连线但未 `@` 的输入是隐式参考，运行前必须清楚提示。

### 3.3 Reference Pack 是 node，不是 group

Group 是布局组织，Reference Pack 是模型可引用的语义资产包。

规则：

- Reference Pack 自身是 `node_type = reference_pack` 的 `media_node`。
- Pack 成员只能是当前 workspace 的已有非 pack node。
- Pack membership 不等于 dependency。
- Pack 不自动递归收纳成员的上游依赖。
- 其他节点可以 dependency 到整个 Pack。

### 3.4 M5 不引入 Agent 生产语义

M5 不实现 Producer、Craftsman、Worker、shot、PSS、HITL、Eino runtime 或 Composer。M5 产生的节点、Reference Pack、version 和 stale 体验应能被 M6 复用，但不把 Agent 概念提前混入 Studio。

## 4. 范围

### 4.1 包含

- Studio production 前端类型和 API 接入。
- `reference_pack` 节点创建、渲染和成员管理。
- 按 node type / operation type 分化的属性面板。
- Prompt `@` 引用和隐式输入提示。
- 模型能力驱动的 operation/model 选择。
- 单节点手动运行、失败展示、手动重试。
- current winner 预览和版本列表。
- stale 标记、stale reason 展示和逐个重跑。
- 首帧/尾帧提取作为 Studio 手动 operation。

### 4.2 不包含

- Agent 对话和自动生产。
- shot / storyboard 业务实体。
- Studio 到 Agent 导入或 Agent 到 Studio 复制。
- 自动批量级联重跑。
- 多版本 winner 手动切换。
- 自动评审 UI。
- 真实供应商参数的完整高保真配置。
- 成片 Composer 和导出流程。

## 5. 分阶段工作项

### M5.1 Studio Production Types And API

目标：让前端能安全理解和调用 M4 生产接口，为后续 UI 做基础。

可交付标准：

- 扩展前端 `MediaType` / `MediaNode` / `canvas-schema`，支持 `reference_pack` 和 production fields。
- 新增或扩展 API helpers：
  - `fetchModelCapabilities`
  - `fetchNodeProductionState`
  - `runNode`
  - `retryJob`
  - `fetchReferencePackItems`
  - `replaceReferencePackItems`
- 规范前端 production state 类型：
  - current version
  - versions
  - latest job
  - active stale reasons
  - capability
  - sandbox jobs
- 让 canvas payload 中现有后端字段进入前端，不要求第一阶段全部展示。

可验收标准：

- TypeScript 可以编译通过，`reference_pack` 不再导致类型断裂。
- 创建 Reference Pack node 的 API 调用可用。
- 前端能读取单个节点的 production state。
- 前端能读取模型能力列表。
- 不改变现有 text/image/video/audio 节点的创建和拖拽行为。

E2E 测试用例：

1. 注册或登录测试用户。
2. 创建 Studio Workspace 并进入 Studio 路由。
3. 通过 UI 创建 Text、Image、Video、Audio 和 Reference Pack 五类节点。
4. 刷新页面，确认五类节点仍存在，画布不报错。
5. 选中任一普通节点，确认前端能拉取 production state。
6. 打开模型选择数据源，确认可读取 mock/internal model capabilities。

通过标准：

- 五类节点都能创建、渲染和刷新恢复。
- Reference Pack 不触发前端类型错误或空白画布。
- production state 和 model capabilities 请求成功。

建议验证：

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

### M5.2 Production Property Panel And Manual Run

目标：把右侧属性面板升级为单节点生产控制入口。

可交付标准：

- 属性面板按 node type 展示不同区域：
  - 基础信息：title、status、node type、group。
  - Operation：operation type。
  - Prompt：普通文本编辑，后续 M5.5 升级为 `@` 编辑。
  - Model：provider/model 下拉。
  - Params：基于 capability defaults 和 limits 的首版表单。
  - Inputs：上游 dependency 列表。
  - Run：运行、重试、运行中状态、错误信息。
- 支持单节点运行：
  - `POST /api/nodes/:id/run`
  - 成功后刷新 canvas 和 production state。
  - 失败后展示 latest job error。
- 支持失败 job 重试：
  - `POST /api/jobs/:id/retry`
  - 展示 parent/attempt 信息的最小摘要。

行为规则：

- `reference_pack` 节点不能运行模型生成；它的核心操作是成员管理。
- `upload` 类型节点可以展示 current asset，但不需要重新运行。
- operation/model 不兼容时，前端要提示；后端返回 capability mismatch 时，UI 展示可读错误。
- 如果后端尚未校验 input node type，M5.2 需要补齐 `GenerationIntent.InputRefs` 与 `model_capability.supported_input_node_types` 的校验，避免只校验 output type 和 operation type。

可验收标准：

- Text Node 可以选择 mock text model 并运行，成功后出现版本和 current winner。
- Image Node 可以选择 mock image model 并运行，成功后画布显示图片预览。
- Video Node 可以选择 mock video model 并运行，成功后属性面板显示 video asset/version 信息。
- 设置 `mock_fail` 后运行失败，属性面板展示失败原因，并可手动重试。

E2E 测试用例：

1. 创建 Studio Workspace。
2. 创建 Text Node，输入 prompt，选择 `text_generation` / `mock-text`，点击运行。
3. 确认节点状态变为 succeeded，属性面板显示 current version 和 latest job。
4. 创建 Image Node，选择 `text_to_image` / `mock-image-only`，点击运行。
5. 确认 Image Node 有图片预览或 current image version。
6. 创建 Video Node，选择 `text_to_video` / `mock-video`，点击运行。
7. 确认 Video Node 的 production state 有 video asset/version。
8. 给任一节点设置 `mock_fail=true` 参数并运行。
9. 确认失败原因可见，再点击重试，确认产生新的 attempt。

通过标准：

- Text/Image/Video 三类节点都能通过 UI 完成手动运行。
- 成功运行会生成 version/current winner。
- 失败运行能展示错误，并能从 failed job 重试。
- operation/model 不兼容时不会静默提交成功。

建议验证：

```bash
make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

### M5.3 Node Preview, Versions And Stale Display

目标：让运行结果和 stale 状态在画布和属性面板中可见。

可交付标准：

- 画布卡片使用 current winner 作为预览来源：
  - text 显示 text output preview。
  - image 显示 version asset image。
  - video 显示视频占位或可播放预览入口。
  - audio 显示音频占位和 metadata。
  - reference_pack 显示成员摘要。
- 属性面板展示版本列表：
  - version number
  - winner 状态
  - asset type
  - created time
  - input hash 简略信息
- 属性面板展示 latest job：
  - status
  - operation
  - provider/model
  - rendered prompt
  - error code/message
- stale 展示：
  - 画布节点显示 stale badge。
  - 属性面板列出 active stale reasons。
  - stale 节点可以手动运行来清除自身 stale。

行为规则：

- 历史产物不因 stale 删除。
- 下游 stale 不自动重跑。
- 手动重跑一个 stale 节点只更新该节点，下游是否 stale 由 M4 stale engine 决定。

可验收标准：

- A -> B -> C 全部运行成功后，重跑 A 会让 B 和 C 显示 stale。
- 手动重跑 B 后，B stale 清除，C 仍 stale。
- 手动重跑 C 后，C stale 清除。
- 属性面板能解释 stale 来源，而不是只显示状态文字。

E2E 测试用例：

1. 创建 Studio Workspace。
2. 创建 Text Node A、Text Node B、Text Node C。
3. 建立 A -> B、B -> C dependency。
4. 依次运行 A、B、C，确认三者都有 current winner。
5. 修改 A prompt 并重跑 A。
6. 确认 B 和 C 在画布卡片上显示 stale。
7. 选中 B，确认属性面板显示 stale reason，并手动重跑 B。
8. 确认 B stale 清除，C 仍显示 stale。
9. 选中 C 并手动重跑 C，确认 C stale 清除。

通过标准：

- current winner 会驱动画布预览和属性面板版本列表。
- stale 状态在画布和属性面板同时可见。
- stale reason 能指向上游变化。
- 逐个重跑行为符合 A -> B -> C 依赖链预期。

建议验证：

```bash
make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

### M5.4 Reference Pack Experience

目标：让 Reference Pack 成为 Studio 中可创建、可显示、可管理、可被引用的语义资产包。

可交付标准：

- 创建 `reference_pack` 节点：
  - 工具栏和右键菜单都能创建。
  - 默认 title 为“参考包”。
  - 默认 operation type 为 `collect_references` 或后端当前默认值，UI 语义显示为 Reference Pack。
- Reference Pack 画布卡片：
  - 显示 pack 名称、成员数量、前几个成员类型或缩略摘要。
  - 显示 stale/degraded 状态。
- Reference Pack 属性面板：
  - 显示候选成员列表。
  - 支持添加已有非 pack node。
  - 支持移除成员。
  - 支持成员排序的首版展示，排序编辑可后续增强。
- Dependency 到 Pack：
  - 其他节点可以连到 Reference Pack。
  - Reference Pack 可以作为 `@` 候选输入。
  - Pack membership 变化后，下游依赖 Pack 的节点显示 stale。

行为规则：

- 不允许 Reference Pack 包含自己。
- 不允许 Pack 嵌套 Pack。
- 不允许直接把裸 asset 放入 Pack；上传文件仍先形成 image/video/audio node。
- Pack membership 不改变普通 group membership。

可验收标准：

- 用户能创建 Pack P，把 Image A 和 Image B 加入 P。
- 用户能创建 Image C，依赖 P 并运行。
- C 的 run intent 包含 P 展开的成员 winner。
- 修改 P 成员后，C 显示 stale reason `reference_pack_membership_changed`。

E2E 测试用例：

1. 创建 Studio Workspace。
2. 上传或创建 Image Node A，并运行得到 current winner。
3. 创建 Image Node B，并运行得到 current winner。
4. 创建 Reference Pack P。
5. 在 P 的属性面板中添加 A 和 B。
6. 刷新页面，确认 P 仍显示两个成员。
7. 创建 Image Node C，建立 P -> C dependency。
8. 配置 C 为 `text_to_image` / `mock-image-only` 并运行。
9. 确认 C 的 latest job intent 包含 P 及其直接成员输入。
10. 从 P 移除 B，确认 C 显示 stale reason。

通过标准：

- Pack 节点可创建、渲染、刷新恢复。
- Pack 成员可添加和移除。
- Pack 不能包含自己或其他 Pack。
- Pack 可以作为 dependency 输入候选。
- Pack membership 变化会标记下游 stale。

建议验证：

```bash
make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

### M5.5 Prompt @ References And Input Validation

目标：让用户能用 Prompt `@` 精确控制输入语义，并避免无效引用或隐式输入悄悄进入运行。

可交付标准：

- Prompt 编辑器首版：
  - 输入 `@` 打开候选菜单。
  - 已连入节点优先展示。
  - 可选择未连线节点，选择后自动创建 dependency。
  - chip 或文本 token 与 `prompt_refs` 同步。
- 后端补齐：
  - 支持更新 `prompt_refs` 和轻量 `prompt_rich`。
  - 运行前拒绝不存在、跨 workspace 或未连接且未成功自动建边的 prompt ref。
  - 在 `GenerationIntent.InputRefs` 中区分 explicit、implicit 和 reference_pack_member，方便 UI、审计和后续 Agent 复用。
- 输入状态展示：
  - explicit refs：Prompt 中已 `@` 的输入。
  - implicit refs：已连线但未 `@` 的输入。
  - invalid refs：Prompt 中还存在但 dependency 已被删除的输入。
- 删除行为：
  - 删除 `@` chip 只更新 `prompt_refs`，保留 dependency。
  - 删除 dependency 后，对应 `@` 被移除或标记 invalid。
- 运行前校验：
  - invalid refs 阻止运行。
  - implicit refs 明确提示。
  - 当前模型不支持某种 input node type 时，前端提示并由后端阻断。

行为规则：

- `prompt_template` 是运行用文本事实源。
- `prompt_refs` 是显式引用事实源。
- `prompt_rich` 可以先保留轻量结构，不要求第一版做完整富文本编辑器。
- 如果实现成本过高，M5.5 首版可用 tokenized textarea + 下方引用列表，不必一次接入复杂富文本库。

可验收标准：

- A -> B dependency 存在，B prompt 中 `@A` 后运行，intent 中 A 是 explicit input。
- A -> B dependency 存在，B prompt 未 `@A`，运行前 UI 提示 A 是 implicit input。
- B prompt 中 `@A` 后删除 A -> B dependency，B 显示 invalid ref，不能成功运行。
- `@` 一个未连线节点 C 后，系统自动创建 C -> B dependency。

E2E 测试用例：

1. 创建 Studio Workspace。
2. 创建 Image Node A 并运行得到 current winner。
3. 创建 Image Node B，并建立 A -> B dependency。
4. 在 B prompt 中输入 `@`，从候选菜单选择 A。
5. 运行 B，确认 latest job intent 中 A 的 input ref 标记为 explicit。
6. 从 B prompt 删除 `@A`，保留 A -> B dependency。
7. 再次准备运行 B，确认 UI 显示 A 是 implicit input。
8. 再次添加 `@A` 后删除 A -> B dependency。
9. 确认 B 显示 invalid ref，运行按钮禁用或运行前被阻断。
10. 创建 Image Node C，不建立连线，在 B prompt 中 `@C`。
11. 确认系统自动创建 C -> B dependency，并写入 `prompt_refs`。

通过标准：

- `prompt_template` 和 `prompt_refs` 能稳定保存并刷新恢复。
- explicit、implicit、invalid 三种输入状态可区分。
- 无效引用不能成功运行。
- `@` 未连线节点能自动创建 dependency。

建议验证：

```bash
make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

## 6. 端到端验收场景

### 6.1 商品参考包到广告主视觉

1. 上传商品图，得到 Image Node A。
2. 创建 Image Node B，dependency 到 A，prompt 中 `@A`，选择 mock image model 运行。
3. 创建 Reference Pack P，把 A 和 B 加入 P。
4. 创建 Image Node C，dependency 到 P，prompt 中 `@P`，选择 mock image model 运行。
5. C 成功产生 version/current winner。
6. 修改 P 成员后，C 显示 stale。

通过标准：

- Pack 可创建和管理成员。
- C 的输入包含 P 的直接成员 winner。
- Pack 变化会让 C stale。

### 6.2 隐式输入提示和能力阻断

1. 创建 A -> B dependency。
2. B prompt 不写 `@A`。
3. 选择只支持 text 的模型运行 B。
4. UI 提示 A 是 implicit input。
5. 后端能力校验阻断不兼容输入。

通过标准：

- 隐式输入不静默消失。
- 用户能理解是模型能力不支持，而不是普通运行失败。

### 6.3 Stale 逐个重跑

1. 创建 A -> B -> C。
2. A、B、C 都运行成功。
3. 重跑 A。
4. B、C 显示 stale。
5. 手动重跑 B，B stale 清除，C 仍 stale。
6. 手动重跑 C，C stale 清除。

通过标准：

- stale 传播可见。
- 用户可以按节点逐个处理。
- 不要求批量自动级联重跑。

### 6.4 视频首帧提取

1. 上传 video，得到 Video Node A。
2. 创建 Image Node B，operation 为 `extract_first_frame`。
3. B dependency 到 A，选择 internal ffmpeg model。
4. 运行 B。
5. B 产生 image version，production state 可看到 sandbox job。

通过标准：

- 首帧提取走 generation job / artifact version / sandbox job 链路。
- UI 不暗示应用进程本地执行 FFmpeg。

## 7. 实施顺序建议

建议按 M5.1 -> M5.2 -> M5.3 -> M5.4 -> M5.5 顺序实现。

原因：

- M5.1 是类型和 API 基础，先做可减少后续 UI 的类型债。
- M5.2 先完成最小手动运行闭环，尽快验证 M4 底座可被 Studio 使用。
- M5.3 把运行结果、版本和 stale 可视化，形成生产工具心智。
- M5.4 在稳定预览和状态展示后加入 Reference Pack，避免 Pack UI 先行但产物不可解释。
- M5.5 最复杂，涉及 prompt_refs、自动建边、失效引用和模型能力提示，适合在基础运行体验稳定后实现。

## 8. 风险与取舍

### 8.1 后端 DTO 稳定性

当前部分接口直接返回 sqlc 结构。M5 前端开始依赖 production fields 后，如果 pgtype 或 JSONB 形态让前端类型变复杂，应增加稳定 response DTO，而不是让 UI 到处处理数据库细节。

### 8.2 Prompt 编辑器复杂度

完整富文本编辑器不是 M5 的核心风险。M5.5 首版可以采用 textarea + token/chip 列表，只要 `prompt_template` 和 `prompt_refs` 能稳定表达显式引用。

### 8.3 Capability UI 不要一次做重

模型参数表单先覆盖 mock/internal capabilities 和常见参数即可。真实供应商的完整参数适配可以随 provider 接入逐步增强。

### 8.4 Reference Pack 不做历史快照

M5 中 Pack 被运行时使用当前直接成员 winner。Pack 历史快照、Pack version 和成员锁定留作后续能力。

## 9. M5 完成定义

M5 完成时，Studio 用户应能在不使用 Agent 的情况下完成一条手动生产链路：

- 创建和上传素材节点。
- 用 dependency 和 `@` 控制输入。
- 创建并引用 Reference Pack。
- 选择模型并运行节点。
- 查看版本、失败和 sandbox 追踪。
- 理解并逐个处理 stale。

M5 完成不要求自动生成完整视频，也不要求对话式 Agent 介入。
