# M5 Inspector Version Asset Review 设计

**状态**：待评审  
**日期**：2026-06-21  
**推荐方案**：方案 B，编辑优先 + 版本时间线 + 全屏资产审阅

## 1. 背景

当前 Studio 已经进入真实生产链路：节点可以运行、生成 `generation_job`、创建 `artifact_version`，并在 Inspector 中展示版本和调用记录。但现有交互仍有几类问题：

- Inspector 顶部已经有节点标题，下面又有单独“标题”输入栏，信息重复。
- `调用记录`作为独立区域常驻 Inspector，占用空间，但它不是用户的高频关注点。
- `artifact_version.job_id` 已经建立了版本和运行记录的一一绑定关系，但 UI 还没有把调用记录表达成“某个版本的详情”。
- 文本、图片、视频内容都可能很大，Node 和 Inspector 内的小预览不能承担完整审阅。
- 画布 Node 现在为了展示完整内容变得偏大，影响画布密度和连线阅读。

本 spec 聚焦 Studio 手动模式的 Inspector、Node 和版本审阅体验，不改变 provider 调用、不改变版本生命周期、不引入 Agent 自动化。

## 2. 产品原则

### 2.1 Inspector 是编辑工作区，不是审计报表

用户单击 Node 打开 Inspector，主要任务是：

1. 识别当前节点。
2. 编辑 Prompt。
3. 选择模型和参数。
4. 运行节点。
5. 查看和选择版本结果。

调用记录、provider request/response、sandbox jobs 属于排障和审计信息，应从主流程中降级，只在用户主动查看某个版本详情时展示。

### 2.2 Version 是素材历史主线

用户不应该在 `Latest Job` 和 `Versions` 之间来回理解。每个 version 对应一次运行尝试：

- 成功 version 绑定素材。
- 失败 version 绑定错误。
- running version 绑定进度。
- 调用记录通过 `version.job_id` 查询或展示。

因此 UI 中不再有全局 `调用记录`区，只有版本详情里的调用记录。

### 2.3 Node 是画布识别卡，不是完整播放器

画布需要承载多个节点、依赖关系和整体结构。Node 应该只做轻量预览：

- 让用户快速识别节点类型、标题、状态和当前产物摘要。
- 不追求完整阅读长文本。
- 不追求完整显示大图。
- 不追求在画布内完成视频审阅。

完整审阅交给全屏资产审阅层。

### 2.4 大内容必须有统一审阅入口

文本、图片、视频、音频都应该有一致的全屏查看入口：

- 从 Node 当前产物打开。
- 从 Inspector 当前 version 打开。
- 从 Inspector 历史 version 打开。

用户在全屏里做“看清楚内容”的动作，在 Inspector 里做“编辑和决策”的动作。

## 3. 目标用户路径

推荐路径：

```text
单击 Node
  -> 打开 Inspector
  -> 顶部识别节点标题/类型/状态
  -> 必要时双击标题 inline edit
  -> 编辑 Prompt / Model / Params
  -> 点击 Run
  -> Version timeline 立即出现新版本
  -> 默认预览当前或刚运行版本
  -> 需要细看内容时点击 Fullscreen
  -> 需要排障时点击 version detail
  -> 历史 succeeded version 可设为 current
```

## 4. Inspector 信息架构

采用“编辑优先 + 版本时间线”的结构。

### 4.1 Header

Header 展示：

- 节点类型，例如 `文本`、`图片`、`视频`。
- 节点标题。
- 节点状态。
- Run 按钮。

标题编辑规则：

- 默认以文本形式展示标题。
- 双击标题进入 inline edit。
- `Enter` 保存。
- `Esc` 取消。
- 失焦保存。
- 空标题不提交，保留原标题。
- 可展示轻量 edit icon 或 hover affordance，但不新增单独表单行。

移除现有单独“标题”输入栏。

### 4.2 Prompt / Model / Params

这是 Inspector 的主区域。

推荐顺序：

1. Prompt 编辑器。
2. Model 选择。
3. Params。
4. Run controls。

如果视觉上更适合，也可以把 Run 放在 Header 右侧，但执行语义不变。

Prompt 输入区应比当前更高，尤其是文本/图片/视频生成场景。参数区域保持紧凑，不抢 Prompt 空间。

### 4.3 Version Preview

Version preview 展示当前选中的 version。

默认选中策略：

- 如果刚点击 Run，自动预览新创建的 running version。
- 如果没有主动预览历史版本，默认显示 current version。
- 如果 current 不存在，显示最新 version。
- 如果用户点击历史 version，只切换 preview target，不改变 current。

预览内容：

- Text：Markdown preview，限制高度，可滚动。
- Image：contain 缩略预览。
- Video：poster + controls，限制高度。
- Audio：后续播放器。
- Running：进度、provider task id 摘要。
- Failed：错误摘要。

Preview actions：

- `Fullscreen`：打开全屏资产审阅。
- `Set current`：只对 succeeded 且非 current version 展示。
- `Details`：打开版本详情。
- `Back to current`：用户正在预览历史版本时展示。

### 4.4 Version Timeline

Timeline 是紧凑版本列表，不承担完整信息展示。

每行展示：

```text
v4 current    image   2048x2048
v3 ready      text    1.2 KB
v2 failed     provider timeout
v1 running    42%
```

行点击行为：

- 点击行只切换 preview target。
- 不直接设为 current。
- 不自动打开详情。

行内状态：

- current badge。
- running progress。
- failed error summary。
- asset type / mime / dimensions / duration。

## 5. Version Detail 设计

### 5.1 入口

Version detail 通过当前 preview 或 version row 的 `Details` 进入。

建议形态：

- 首期使用 Inspector 内 drawer/details panel。
- 后续可升级为 modal 或独立 side sheet。

不要把详情默认展开在 Inspector 主流程里。

### 5.2 内容

Version detail 绑定单个 `artifact_version`，通过 `version.job_id` 展示调用记录。

内容分组：

1. **Summary**
   - version no
   - status
   - current/winner
   - created/started/completed time
   - input hash
   - asset type / mime / size

2. **Rendered Prompt**
   - 展示最终传给 provider 的 prompt。
   - 长文本换行，不产生横向长滚动。

3. **Provider Request**
   - JSON block。
   - 默认折叠或局部折叠。
   - 长字符串 wrap。

4. **Provider Response**
   - JSON block。
   - 默认折叠或局部折叠。
   - 显示 task id、status、asset url 等关键字段。

5. **Sandbox Jobs**
   - 如果该 version/job 关联 sandbox jobs，展示 sandbox job 列表。
   - 显示 download/import/persist 结果。

6. **Error**
   - failed version 展示 error code/message。
   - succeeded version 不展示空错误区。

### 5.3 数据来源

首期可以复用现有 `NodeProductionState.versions` 中的字段：

- `provider_request`
- `provider_response`
- `error_code`
- `error_message`
- `started_at`
- `completed_at`
- `job_id`

如果需要更完整的 rendered prompt 或 sandbox jobs：

- 可以通过现有 `latest_job` 临时覆盖 current/latest 场景。
- 更推荐新增或复用版本详情 API：

```text
GET /api/nodes/:node_id/versions/:version_id/detail
```

返回：

```json
{
  "version": {},
  "job": {},
  "asset": {},
  "sandbox_jobs": []
}
```

如果现有 API 已足够，首期不强制新增接口。

## 6. 全屏资产审阅

### 6.1 入口

统一入口：

- Node 上的 expand icon。
- Inspector version preview 的 `Fullscreen`。
- 历史 version preview 的 `Fullscreen`。

### 6.2 全屏层内容

全屏层应包含：

- 顶部栏：
  - 节点标题。
  - version no。
  - current/historical/running/failed 状态。
  - Close。
  - 对 succeeded 非 current version 展示 `Set current`。
  - 对有调用记录的 version 展示 `Details`。

- 主内容区：
  - Text：Markdown 阅读器，可滚动。
  - Image：contain 显示，支持 zoom in/out、fit。
  - Video：播放器，使用 poster。
  - Audio：播放器，后续补齐。
  - Failed：错误摘要。
  - Running：进度状态。

### 6.3 行为

- 打开全屏不改变 current version。
- 关闭后回到原 Inspector preview target。
- 在全屏里点击 `Set current` 后，current 切换，Inspector 同步更新。
- 全屏可通过 `Esc` 关闭。

## 7. Node 缩小策略

Node 的目标是画布可读性，而不是完整审阅。

### 7.1 默认尺寸

建议缩小默认最大尺寸：

- Text：宽 280-360，高 160-260。
- Image：宽 240-360，高按比例，但限制最大高度。
- Video：宽 260-380，固定 16:9 或素材比例附近。

具体数值应与当前 auto layout、connection geometry 一起验证。

### 7.2 文本节点

- 展示 Markdown 摘要。
- 最多显示若干行。
- 底部渐隐或截断。
- 展示 expand icon。

不在 Node 内完整滚动长文本，避免画布节点过大。

### 7.3 图片节点

- 使用 `object-fit: contain`。
- 不裁剪核心内容。
- 节点内部允许有少量背景留白，但整体尺寸不要为了图片无限放大。
- 完整查看通过全屏。

### 7.4 视频节点

- 展示 poster/首帧。
- 可展示轻量播放按钮。
- 不强制在画布内完整播放长视频。
- 完整播放通过全屏。

## 8. 数据与状态语义

### 8.1 Version 和 Job

每个 production-generated version 和 job 一一绑定：

```text
artifact_version.job_id -> generation_job.id
```

UI 语义：

- Version 是用户看到的主线。
- Job 是 version detail 的技术记录。
- Inspector 不再展示全局 latest job。

### 8.2 Current

只有 succeeded version 可以 current。

预览历史 version 不改变 current。

全屏预览历史 version 不改变 current。

只有用户点击 `Set current`，或新运行成功自动成为 current，才改变 current。

### 8.3 Running / Failed

Running version 可以预览进度和详情，但不能设为 current。

Failed version 可以查看错误和调用记录，但不能设为 current。

## 9. 交互细节

### 9.1 标题 inline edit

状态：

- `view`：展示标题。
- `editing`：展示 input。
- `saving`：可轻量 disabled 或保持输入。

事件：

- 双击标题进入编辑。
- Enter 保存。
- Escape 取消。
- Blur 保存。

错误：

- 保存失败时恢复旧标题并提示 toast。

### 9.2 Version detail JSON

JSON block 必须：

- `white-space: pre-wrap`
- `overflow-wrap: anywhere`
- `word-break: break-word`
- 不产生巨大横向滚动条。

### 9.3 全屏层

全屏层应高于 tldraw canvas、Inspector popover 和 toolbar。

打开全屏时不关闭 Inspector。

关闭全屏后焦点回到触发按钮或 Inspector。

## 10. 不包含

本阶段不做：

- 多版本自动评分。
- A/B 自动 winner。
- 版本 diff。
- 分支版本树。
- 复杂媒体编辑器。
- 视频剪辑工具。
- Agent 自动重跑下游。

## 11. 可交付标准

- Inspector 不再有单独标题输入行。
- Inspector 顶部标题支持双击 inline edit。
- Inspector 不再有全局 `调用记录`详情区。
- 每个 version 可打开 detail，detail 展示该 version 绑定 job 的调用记录。
- Version preview 支持 `Fullscreen`。
- Node 当前产物也支持 `Fullscreen`。
- Text/Image/Video 全屏审阅可用。
- Node 默认尺寸比当前更克制，不因长文本或大图无限放大。
- 长 provider request/response 不再造成超长横向滚动。

## 12. 可验收标准

### 12.1 Inspector

- 单击 Node 打开 Inspector。
- 顶部显示节点类型、标题、状态。
- 双击标题可编辑；Enter/Blur 保存；Esc 取消。
- Prompt、Model、Params 是主视觉区域。
- 点击 Run 后，新 version 出现在 timeline。
- 点击历史 version 行只切换 preview，不改变 current。
- succeeded 历史 version 可点击 `Set current`。

### 12.2 Version Detail

- 对任意 version 点击 `Details`。
- detail 展示 version summary。
- detail 展示该 version 对应 job 的 rendered prompt/provider request/provider response。
- failed version 展示错误。
- running version 展示进度和 provider response/task id。
- detail 中长 JSON 不出现巨大横向滚动。

### 12.3 Fullscreen

- Text version 全屏展示 Markdown，可滚动。
- Image version 全屏 contain 展示，可看完整图片。
- Video version 全屏可播放，poster 可见。
- 从 Node 打开全屏展示 current version。
- 从历史 version 打开全屏不改变 current。
- Esc 或 Close 可关闭。

### 12.4 Node

- 长文本 Node 不再扩展到过大尺寸。
- 图片 Node 不再为了完整审阅撑大画布。
- 视频 Node 显示 poster/播放入口。
- 依赖连线锚点仍贴合缩小后的 Node 边缘。
- 自动整理后节点不重叠。

## 13. E2E 测试用例

### 13.1 文本版本详情

1. 创建文本节点。
2. 输入较长 Markdown prompt。
3. 运行节点。
4. 验证 Node 只显示摘要。
5. 打开 Inspector。
6. 验证 version timeline 有 v1 current。
7. 点击 `Details`。
8. 验证 rendered prompt/provider request/provider response 可见且换行。
9. 点击 `Fullscreen`。
10. 验证 Markdown 全屏可滚动。

### 13.2 图片全屏审阅

1. 创建图片节点。
2. 运行 mock 或真实 image provider。
3. 验证 Node 是缩略预览。
4. 点击 Node expand。
5. 验证图片全屏 contain 展示。
6. 关闭全屏后 Inspector 状态保持。

### 13.3 多版本历史预览

1. 对同一节点运行两次，得到 v1、v2。
2. v2 为 current。
3. 点击 v1 row。
4. 验证 preview 切到 v1，但 current 仍是 v2。
5. 点击 v1 `Details`，看到 v1 job 调用记录。
6. 点击 v1 `Set current`。
7. 验证 current 切到 v1，下游 stale 正常。

### 13.4 失败版本详情

1. 使用 mock failure 运行。
2. 验证 failed version 留在 timeline。
3. failed version 不能设为 current。
4. 打开 Details。
5. 验证 error code/message/provider response 可见。

### 13.5 视频全屏审阅

1. 创建视频节点。
2. 使用真实或 mock video provider 生成视频。
3. 验证 Node 展示 poster。
4. 打开全屏。
5. 验证视频可播放，poster 可见。
6. Details 中能看到对应 version/job 调用记录。

## 14. 实施拆分建议

建议分三步：

1. **Inspector 信息架构**
   - 删除标题重复输入。
   - Header inline edit。
   - 移除全局调用记录。
   - Version detail 入口。

2. **全屏资产审阅**
   - 实现统一 fullscreen modal。
   - 支持 Text/Image/Video。
   - Node 和 Inspector 都能打开。

3. **Node 缩小与布局回归**
   - 调整 node preview 尺寸策略。
   - 验证 connection geometry。
   - 验证 auto layout。

## 15. 自检

- 本 spec 不改变底层 provider 调用。
- 本 spec 不改变 version lifecycle，只使用已有 `artifact_version.job_id` 关系。
- 调用记录从全局 latest job 迁移到 version detail，符合用户低频排障诉求。
- 大内容审阅从 Node/Inspector 小区域迁移到全屏层，符合画布密度诉求。
- 标题编辑去重后，Inspector 主区域留给 Prompt/Model/Params。
