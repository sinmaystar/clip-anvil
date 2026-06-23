# ClipAnvil Studio 模式设计方案

## 1. 定位

Studio 是用户主导的 AI 视频生成画布。用户可以像使用 Figma 白板一样组织素材，也可以像使用节点编辑器一样把媒体资源连成生成依赖。

当前 Studio 目标是先成为一个手动可用、专业、低干扰的编辑器。Agent/storyboard 自动化是后续阶段，不应抢占当前画布体验。

## 1.1 当前实现快照

当前代码已交付 Studio M1.x-M5 的核心 DAG 编辑和手动生产能力：

- 全屏 React Flow 无限画布。
- 左侧资源导航以浮层形式覆盖在画布上，展开态展示 workspace、连接状态、项目列表、主题切换、资源树、用户信息。
- 左侧导航收起后只保留左上角 project peek，展示“影 + 项目名 + 展开箭头”。
- 顶部居中浮动工具栏可创建视频、文本、图片、音频、参考包和连接，也支持创建手动文本源素材。
- 画布右键菜单可在指定位置创建文本、图片、视频、音频节点；点击其他画布区域会关闭菜单。
- 支持拖拽图片/视频/音频文件上传后创建用户源素材节点。
- 单击节点后在节点附近展示浮层 Inspector，聚焦标题、Prompt、模型、参数、运行、版本和素材预览。
- Inspector 不是常驻右侧面板；只有选中节点时出现，点击画布空白关闭。
- 用户源素材节点不显示模型运行入口，可作为下游依赖或 Reference Pack 成员。
- 支持 dependency 连线、删除连线、节点输入查询、分组创建、Dagre 自动整理。
- 自动整理会根据左侧导航展开/收起状态避开浮层安全区。
- 删除节点、撤销恢复、拖拽位置、Camera 持久化已和后端同步。
- `/ws/canvas` 用于节点、连线、分组事件广播；前端断线重连后重新拉取画布。
- 生产链路已落地：Prompt `@` 引用、Reference Pack、手动运行、异步状态、版本列表、current 选择、调用记录详情、stale reason、真实 Volcengine 文本/图片/视频 provider、sandbox-backed 生成素材入库。
- 尚未落地：完整 Agent 对话生产模式、自动评审、成片 Composer、`/ws/chat`；音频生成模型暂时 hold。

## 2. 界面布局

Studio 不是传统三栏管理界面，而是画布为底、工具浮在画布上：

```text
┌──────────────────────────────────────────────────────┐
│  [影 Project Name ›] or [floating resource sidebar]  │
│                                                      │
│                  [视频 文本 图片 音频 连接]           │
│                                                      │
│                React Flow infinite canvas                │
│             [media nodes]──dependency──>[node]       │
│                                                      │
│                [方向选择] [自动整理]                  │
└──────────────────────────────────────────────────────┘
```

主要区域：

| 区域 | 职责 |
|---|---|
| 画布 | 节点、连线、分组、右键创建、拖拽上传 |
| 左侧浮层导航 | workspace 上下文、资源索引、分组、搜索、主题切换 |
| 顶部浮动工具栏 | 快速创建节点和进入连接模式 |
| 浮层 Inspector | 单个节点的标题、Prompt、模型、参数、运行、版本和素材详情 |
| 底部自动整理控件 | Dagre 自动布局方向和执行入口 |

## 3. 左侧浮层导航

展开态：

- Studio 标识和 workspace 名。
- WebSocket 连接状态。
- 收起按钮。
- 返回项目列表。
- appearance toggle。
- ResourceTree：新建分组、搜索、类型筛选、分组和未分组节点。
- 当前账号和登出。

收起态：

- 只显示左上角 project peek。
- project peek 必须展示项目名。
- 不保留被压缩的资源树内容。
- 点击 project peek 展开导航。

设计目的：

- 展开时提供资源组织能力。
- 收起时把主空间还给画布。
- 用户始终能知道自己在哪个项目里。

## 4. 创建资源

当前四种创建入口：

| 入口 | 操作 | 适用场景 |
|---|---|---|
| 右键菜单 | 画布空白处右键，选择类型 | 精确定位创建 |
| 浮动工具栏 | 点击文本/图片/视频/音频 | 从视口中心创建 |
| 拖拽上传 | 从系统文件管理器拖文件到画布 | 快速导入本地素材 |
| 资源树 | 点击新建分组 | 组织资源和分组 |

节点创建数据流：

```text
用户创建节点
  -> POST /api/nodes
  -> 后端返回 MediaNode
  -> canvas payload 更新为 React Flow node
  -> 选中节点并打开节点编辑浮层
```

## 5. 浮层 Inspector

当前主编辑入口是节点附近的浮层 Inspector。

单击节点后：

- 画布选中该节点。
- 节点下方或附近出现 Inspector。
- 顶部展示节点标题，标题可就地编辑，避免重复的标题表单行。
- 主区域优先展示 Prompt、模型选择、参数和运行按钮。
- 运行后立即创建新版本槽位，版本和 job 一一绑定，版本详情里可查看 provider request / response。
- 文本、图片、视频等大内容可通过预览入口全屏查看。
- 用户源素材节点只展示素材内容编辑或上传资产信息，不提供运行按钮。
- 点击画布空白、按 Escape 或切换选择对象会关闭 Inspector。

右侧常驻 Inspector 已移除。复杂信息通过折叠区、版本详情或全屏预览承载，不让 Prompt 和模型配置被次要信息挤占。

## 6. 节点视觉

节点是画布对象，不是 dashboard card：

- 节点本体使用小圆角，接近方形。
- 文本节点展示 Markdown 摘要，可根据内容自适应到最大尺寸。
- 图片节点按图片原始比例自适应尺寸，避免固定卡片造成留白或裁剪。
- 视频节点显示可播放预览或封面，占位状态必须清晰。
- 音频节点展示音频资产信息和运行状态。
- Reference Pack 节点展示成员摘要。
- 用户源素材节点使用“素材”身份标签，减少和可运行生成节点混淆。
- 选中态使用细蓝边和轻量 halo。
- 状态通过边线、badge、局部色块表达，不使用大面积发光。

## 7. 建立连线

当前 Studio 只暴露 dependency 连线：

| 方式 | 操作 | 结果 |
|---|---|---|
| 节点右侧 `+` | hover 或选中节点后，从右侧外浮 `+` 拖出 | 创建 dependency edge |
| 资源树起点 | 在资源树中点击连接按钮，再点击目标节点 | 创建 dependency edge |
| 顶部连接按钮 | 选中起点节点后点击连接，再点击目标节点 | 创建 dependency edge |

交互要求：

- 只有右侧外浮 `+` 是节点上的连接起点。
- 不显示左侧接收端口。
- 目标节点任意区域都可以接收释放。
- 失败时用连接反馈 toast 说明原因，例如成环。

数据流：

```text
用户拖拽或点击创建连线
  -> POST /api/edges { from_node_id, to_node_id }
  -> 后端做 DAG 环检测
  -> 成功后 canvas payload 增加 edge
  -> SVG overlay 渲染 dependency path
```

## 8. 分组操作

分组是组织工具，不改变 dependency 语义。

当前支持：

- 通过左侧导航新建空分组。
- 画布中拖动节点进入/离开分组容器时同步节点 `group_id`。
- 分组和资源树保持同步。
- 自动整理把分组作为容器处理，避免未分组节点落进分组内部。

目标补充：

- 分组标题内联重命名。
- 直接拖拽资源树节点调整分组。
- 分组折叠，只显示名称和成员计数。

## 9. 自动整理

自动整理使用 `computeDagreLayout`，并带有可选 `origin`。

当前安全区规则：

- 左侧导航展开：整理结果从浮层右侧开始，屏幕安全起点约 `x=360, y=112`。
- 左侧导航收起：整理结果从 project peek 右侧开始，屏幕安全起点约 `x=120, y=112`。
- 安全起点通过 React Flow `screenToFlowPosition()` 转换为画布坐标。

这意味着自动整理不是简单重排 DAG，还必须尊重当前 UI 浮层占位。

## 10. 生成流程

1. 用户在节点编辑浮层中编写 Prompt、选择模型。
2. 点击生成。
3. 后端创建 `generation_job(status=queued)` 和 `artifact_version(status=queued)`。
4. 异步 runner 执行 mock / Volcengine / internal provider。
5. 图片/视频远程结果通过 sandbox 下载并存入 MinIO。
6. 完成后更新 job、version、asset、node current 和画布预览；失败后保留 failed version 和错误信息。
7. 上游 current 变化后，下游节点进入 Stale，由用户决定是否重新运行。

## 11. 节点状态机

```text
Draft -> Ready -> Queued -> Running -> Succeeded
  ^        ^                            |
  |        |                            v
  +------ UserEditing              Stale
                                   |
                                Failed
```

状态显示由 `data-status` 驱动。不要为每个状态硬写独立 className。

## 12. 相关文档

- [前端设计系统](frontend.md) — 当前视觉系统、布局、组件和验收要求
- [整体设计](overview.md) — 架构、原则、路线图
- [画布设计](canvas.md) — 节点/连线/分组视觉规格、数据通路
- [Agent 模式](agent-mode.md) — Agent 驱动的生产交互
- [数据库设计](../engineering/database.md) — schema 和迁移
