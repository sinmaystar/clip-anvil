# ClipAnvil Agent 模式设计方案

## 1. 定位

Agent 模式是"默认由 Agent 主导生产，用户在关键节点确认、随时可通过对话干预"的自动化生产模式。

画布只读——用户看到 Agent 的工作成果在画布上实时呈现，所有修改通过对话驱动。

当前三 Agent 主链路已落地：Agent Workspace 已有 `/ws/agent` 对话通道、消息/事件/任务持久化、Eino checkpoint/resume、Producer/Craftsman/Reviewer 原生 Eino tool loop、HITL 决策卡、创作事实源、RenderPlan、Worker 执行、Reviewer gate、Producer pending signal、语义键和 Agent Workbench 场景/分镜画布投影。本文仍保留部分目标态交互说明；判断当前事实时以代码、迁移和 `docs/engineering/` 为准。

## 2. 界面布局

```
┌──────────────────────────────────────────────────────────────┐
│  影砧  ·  ☕ 咖啡广告项目    [Agent 模式 ▾]     ⌘K  导出  设置 │
├──────────┬───────────────────────────────────┬───────────────┤
│          │                                   │ 💬 对话       │
│ 🔍搜索    │         画布区域                   │              │
│          │    （React Flow 无限画布，只读）         │ [Agent 状态栏] │
│[类型筛选] │                                   │ 🔵 正在生成... │
│          │    [媒体节点卡片]                   │              │
│ 📁 素材   │    [连线 / 箭头]                   │ [对话消息]    │
│  🖼 ...   │    [分组容器]                      │              │
│ 📁 分镜   │                                   │ [Gate 确认卡] │
│  🎬 ...   │    ┌──────────────────────┐       │              │
│ 📁 音频   │    │ 节点摘要卡（单击时）   │       │ ┌──────────┐ │
│  🔊 ...   │    │ 🎬 03-卖点展示 · 6s   │       │ │ 对话输入   │ │
│          │    │ [在对话中引用]          │       │ │ @引用 上传  │ │
│          │    └──────────────────────┘       │ └──────────┘ │
│          │  [缩放] [自动整理]                  │              │
└──────────┴───────────────────────────────────┴───────────────┘
```

**与 Studio 模式的关键差异**：

| 差异点 | Studio | Agent |
|---|---|---|
| 浮动工具栏 | 显示创建工具 | 不显示（无创建工具） |
| 右侧面板 | 属性面板（选中时） | 对话面板（始终显示） |
| 节点单击 | 选中 + 属性面板 | 画布上浮现摘要卡 |
| 右键菜单 | 创建/编辑菜单 | 无菜单 |

## 3. 画布交互

画布只读不意味着无法交互：

| 交互 | 行为 | 效果 |
|---|---|---|
| 平移/缩放 | 正常操作 | 浏览全局布局 |
| 单击节点 | 节点高亮，画布上浮现摘要卡 | 可在对话中引用 |
| 双击节点 | 弹出节点详情浮层（只读） | 查看完整 Prompt、版本列表、评审记录 |
| 单击连线 | 连线高亮，显示类型和转场信息 | 查看依赖/顺序关系 |
| 右键 | 无菜单 | — |
| 悬停节点 | 显示状态 tooltip | 快速状态确认 |

### 3.1 节点摘要卡

单击节点后出现在画布节点下方（或对话面板底部输入框上方）：

```
┌──────────────────────────────────────────┐
│ 🎬 03-卖点展示 · 6s · 草稿               │
│ Prompt: 燕麦拿铁从上方缓缓倒入杯中...     │
│ 模型: qwen-vl-max · 依赖: 产品主图, Logo  │
│                              [在对话中引用] │
└──────────────────────────────────────────┘
```

点击"在对话中引用" → 对话输入框插入 `@03-卖点展示`，用户可直接说"把 @03-卖点展示 改成俯拍视角"。

## 4. 对话面板

### 4.1 Agent 状态指示器

对话面板顶部始终显示 Agent 当前状态：

```
🔵 正在生成 · 3/5 镜头完成 · 预计剩余 2分钟        [⏸ 暂停]
```

```
🟡 等待确认 · 分镜已规划完成                        [查看分镜]
```

```
⚪ 空闲 · 上次操作: 成片导出完成 (10分钟前)
```

```
🔴 需要帮助 · 第4个镜头生成失败                     [查看详情]
```

### 4.2 对话消息类型

| 类型 | 说明 |
|---|---|
| 用户文本 | 用户输入的需求或修改指令 |
| Agent 回复 | Agent 的文本回复 |
| 阶段分隔线 | "━━━ 阶段2: 分镜规划 ━━━"，标记工作流阶段 |
| Gate 确认卡 | 带按钮的确认卡片（确认 / 修改 / 取消） |
| 进度摘要 | "[完成] shot-01 生成完成, 评分 8.2" |

### 4.3 对话输入

- 文本输入框
- `@` 引用：输入 @ 弹出节点/素材选择器，插入引用
- 上传按钮：附带图片/视频/音频文件
- 暂停按钮：暂停 Agent 执行

## 5. Production State Summary（PSS）

### 5.1 设计动机

Agent 需要知道项目状态（哪些分镜、生成进度、用了哪些素材），但不应知道这些东西在画布上怎么排列。

**PSS** 是一段用生产概念描述的结构化文本。Agent 读它来感知项目状态，完全不涉及画布概念。类似 spark-video 的 `shots_state.json`。

### 5.2 全量快照

Agent 启动时、上下文刷新时使用。注意：没有"节点"、"连线"、"分组"等画布概念。

```markdown
# 项目: 咖啡广告

## 分镜表 (5个镜头, 总时长 30s)
- [shot-01] 产品特写 3s | ✅ 完成 v1★ 评分8.2 | 参考: 产品主图
- [shot-02] 使用场景 7s | 🔵 生成中 45% | 参考: 产品主图, Logo
- [shot-03] 卖点展示 6s | ⏳ 排队 | 参考: Logo
- [shot-04] 情感共鸣 8s | ○ 待生成
- [shot-05] 品牌收尾 5s | ○ 待生成
顺序: shot-01 →crossfade→ shot-02 →cut→ shot-03 →dissolve→ shot-04 →crossfade→ shot-05

## 素材
- [asset-01] 🖼 产品主图 | 已就绪 | 被 shot-01, shot-02 使用
- [asset-02] 🖼 品牌Logo | 已就绪 | 被 shot-02, shot-05 使用
- [asset-03] 🔊 BGM | 已就绪

## 进度
完成: 1/5 | 生成中: 1 | 累计费用: ¥0.8 / 预估总费用: ¥4.2
当前阶段: 视频生成
```

### 5.3 增量更新

每次生产工具执行后自动附加到 Agent 上下文：

```
[完成] shot-01 生成完成, v1★ 评分8.2, 耗时32s, 费用¥0.15
[开始] shot-02 开始生成
```

Agent 无需主动刷新，除非上下文窗口被压缩。

### 5.4 读取工具

| 工具 | 用途 | 返回 |
|---|---|---|
| `read_project_context` | 读取创作事实源和按需生产状态 | Brief、ProjectMemory、KeyElement、Scene、Shot、RenderPlan、review、issue、ObjectIndex、production_state |
| `read_project_memory` | 读取当前项目创作宪法 | 核心意图、soul、品牌事实、不可破坏约束、视觉锚点、允许/禁止项 |

### 5.5 Sub-Agent 的状态可见性

- Producer 可以通过 `read_project_context(include=["production_state"])` 读取完整状态；普通规划优先读取 summary，关键决策再读 full。
- Craftsman / Reviewer 有自己的局部 context loader，并通过 `read_project_memory` 读取全局约束；它们不依赖 Producer 手写摘要作为事实源。
- Sub-Agent 完成后通过 `producer_pending_signal` 唤醒 Producer；Producer 再读取 DB 事实源做下一步决策。

## 6. 生产级工具

### 6.1 设计原则

Agent 的工具不是画布操作（create_node、create_edge），而是**语义创作和生产操作**（upsert brief/memory/key elements/storyboard、dispatch Craftsman、decide RenderPlan、dispatch Reviewer、request HITL）。每个工具在内部自动翻译为多个业务命令，Agent 不感知前端画布快照。

典型调用：

```
dispatch_craftsman({
  shot_id: "shot-01",
  mode: "preview_image",
  prompt: "...",
  references: [{id: "产品主图ID", role: "product"}, {id: "LogoID", role: "brand"}]
})
→ 系统自动: 更新节点 prompt + 建依赖连线 + 提交生成 + 完成后更新状态/缩略图
```

### 6.2 工具列表

**Producer 规划与调度类**：

| 工具 | 参数 | 说明 |
|---|---|---|
| `read_project_context` | brief, include[], scope_ref?, detail_level | 读取创作事实源、ObjectIndex 和按需 production_state。工具返回语义键，避免模型编造 UUID |
| `upsert_project_brief` | brief, fields | 创建或修改 `CreativeBrief` |
| `update_project_memory` | brief, fields | 创建或更新 `ProjectMemory`，由 Producer 维护全局约束和 soul |
| `upsert_key_elements` | brief, elements[] | 创建或修改 `KeyElement` / `KeyElementState` |
| `upsert_storyboard` | brief, scenes[], shots[], dependencies[] | 创建或修改 `Scene`、`Shot`、`shot_key_element`、`shot_dependency` |
| `dispatch_craftsman` | brief, scope_refs[], target_phase, mode | 派发 Craftsman 为 key element state 或 shot 创建/修订 RenderPlan |
| `decide_render_plan` | brief, render_plan_ref 或 decisions[] | Producer 对 waiting_for_approval RenderPlan 做 accept/reject；accept 后入队 Worker |
| `dispatch_reviewer` | brief, target, review_task | 派发 Reviewer 做 pre-render、preview image、shot video 或 final video review |
| `request_user_decision` | title, message, options[] | 请求用户确认并通过 Eino checkpoint/resume 挂起恢复 |

**Craftsman / Reviewer 写入类**：

| 工具 | 参数 | 说明 |
|---|---|---|
| `read_project_memory` | brief | Craftsman / Reviewer 读取全局创作宪法 |
| `upsert_render_plan` | brief, mode, scope, target_phase, operation, prompt_parts, params, bindings | Craftsman 唯一写工具；创建/更新/fork RenderPlan，工具内部完成编译、能力校验和必要的提交准备 |
| `submit_review_result` | brief, verdict, rubric, critique, issues, retry_recommendation | Reviewer 唯一写工具；写 `review_record` 和 `artifact_issue` |

旧 `read_workspace_context`、`get_production_state`、`update_storyboard`、`generate_shot_video`、`review_shot`、`select_version`、`retry_generation`、`compose_final` 仍可在历史文档或留存代码中看到。当前三 Agent 主链路优先使用上表的 native typed tools。

### 6.3 工具执行流程

```
Agent 调用生产工具（如 dispatch_craftsman）
  │
  ▼
生产翻译层
  ├── 解析 reference_inputs → 确定依赖关系
  ├── 生成初始画布坐标
  │
  ├── 调用业务命令（自动，Agent 不感知）:
  │     ├── 创建或更新 shot 相关节点
  │     ├── 提交 generation_job / artifact_version
  │     ├── 更新 agent_task / agent_event
  │     └── 广播 canvas 与 agent 事件
  │
  ├── 广播 WebSocket 事件
  │     └── 前端收到 → 刷新/合并 React Flow nodes 和 edges → 画布刷新
  │
  └── 返回结果给 Agent（生产语言）:
        ├── {shot_id, status: "generating", job_id}
        └── 自然语言工具结果 + DB 事实更新 + producer_pending_signal
```

### 6.4 连线的自动派生

Agent 不创建连线。所有连线从两个来源自动派生：

| 连线类型 | 派生来源 | 时机 |
|---|---|---|
| **dependency** | Worker 解析 RenderPlan 输入引用后的生产输入 | 提交生成时自动创建或复用 |
| **storyboard dependency** | `upsert_storyboard` 的 shot dependencies | 写入 `shot_dependency`，不污染 Studio dependency |
| **reference** | 用户在 Studio 模式手动拖拽 | 仅 Studio 模式 |

好处：
- Agent 永远不需要思考"建一条从 A 到 B 的连线"
- 连线和实际生成输入**始终一致**
- Agent 修改 reference_inputs → 系统自动更新对应连线

## 7. MultiAgent 架构

### 7.1 架构

```
用户对话
  │
  ▼
Producer Agent（顶层编排）
  │
  ├── 读取 ProjectContext / ObjectIndex
  ├── 维护 ProjectMemory（全局约束）
  ├── 管理 Gate（用户确认）
  ├── 处理用户对话修改
  │
  ├── 写创作事实源
  │           └── 调用：upsert_project_brief / update_project_memory / upsert_key_elements / upsert_storyboard
  │
  ├── 调度 → Craftsman（按 key_element_state / shot scope，可并行）
  │           └── 调用：dispatch_craftsman
  │
  ├── 决策 → Worker
  │           └── 调用：decide_render_plan accept/reject
  │
  └── 调度 → Reviewer
              └── 调用：dispatch_reviewer
```

### 7.2 角色分工

| 角色 | 职责 | 工具权限 |
|---|---|---|
| **Producer** | 解析需求、维护全局创作事实源、编排任务、管理决策卡、处理用户修改、读取 signal 决策下一步 | `read_project_context`、`upsert_*`、`dispatch_craftsman`、`decide_render_plan`、`dispatch_reviewer`、`request_user_decision` |
| **Craftsman** | 为 key element state 或 shot 生成 Seedream/Seedance 结构化 RenderPlan | `read_project_memory`、`upsert_render_plan` |
| **Worker** | 确定性执行 RenderPlan 对应的 production intent | 无模型工具；由 `decide_render_plan` accept 后入队 |
| **Reviewer** | 按 10 轴 rubric 评审 RenderPlan 或产物，提交问题和修复建议 | `read_project_context`、`read_project_memory`、`submit_review_result` |

### 7.3 模型选择策略

| 角色 | 推荐模型能力 | 理由 |
|---|---|---|
| Producer | 强推理 | 理解复杂需求、全局规划 |
| Craftsman | 领域知识 + 创意 | 为单个分镜制定 Prompt 和生成策略 |
| Reviewer | 多模态理解 | 需要"看"生成的图片/视频并给出 critique |
| Worker / Scheduler | 不使用 LLM | 执行确定性生产、依赖等待和 signal 派发 |

## 8. Skill 体系

### 8.1 定位

借鉴 spark-video：**Skill 描述判断标准和契约，不是刚性脚本。Agent 在 Skill 框架内拥有自主判断权。**

Skill 是**领域知识模块**——不同类型的营销视频（产品广告、品牌故事、教程视频、口播视频）有不同的制作方法论，Skill 把这些结构化地传递给 Agent。

### 8.2 Skill 结构

```yaml
name: marketing-ad-short
display_name: 短视频营销广告
description: 15-60秒的产品营销短视频，适用于抖音/快手/视频号投放
version: 1

# 适用条件——Producer 根据用户需求匹配 Skill
triggers:
  keywords: [广告, 营销, 推广, 投放, 带货]
  duration_range: [15, 60]
  platforms: [抖音, 快手, 视频号, 小红书]

# 资产需求
assets:
  required:
    - type: image
      role: product_image
      description: 产品主图，清晰展示产品外观
  optional:
    - type: image
      role: brand_logo
      description: 品牌 Logo
    - type: audio
      role: bgm
      description: 背景音乐

# 制作阶段
phases:
  - name: brief_analysis
    role: producer
    description: 解析用户需求，提取产品信息、目标平台、卖点、风格
    output: 结构化 Brief

  - name: storyboard
    role: screenwriter
    description: >
      根据 Brief 生成分镜表。营销广告分镜遵循 AIDA 模型：
      Attention（前3秒抓注意力）→ Interest（展示产品）→
      Desire（传递卖点）→ Action（行动号召）。
    output: 分镜节点列表
    gate: storyboard_review

  - name: asset_preparation
    role: art_asset
    description: 根据分镜需求生成辅助素材
    parallel: true

  - name: shot_generation
    role: director
    description: >
      为每个镜头生成 Prompt 并提交生成。要点：
      - 产品必须在画面中突出展示
      - 色调和光线保持品牌一致性
      - 前3秒必须有视觉冲击力
      - 避免文字水印和非品牌元素
    parallel: true
    retry: 3

  - name: review
    role: review
    description: 评审生成结果

  - name: stitch
    role: stitch
    description: 拼接成片，添加转场和 BGM
    gate: final_review

# 质量评审标准
review_rubric:
  axes:
    - name: product_visibility
      weight: 0.25
      description: 产品是否清晰可见、占据画面主体
    - name: brand_consistency
      weight: 0.20
      description: 色调、风格是否与品牌调性一致
    - name: visual_quality
      weight: 0.20
      description: 画面质量、分辨率、无明显瑕疵
    - name: narrative_flow
      weight: 0.20
      description: 镜头间逻辑连贯性和节奏感
    - name: platform_fit
      weight: 0.15
      description: 是否符合目标平台的调性和格式
  pass_threshold: 7.0

# Gate 策略
gates:
  storyboard_review:
    description: 分镜完成后，消耗生成资源前确认
    show_to_user: 分镜卡片预览 + 总时长 + 镜头数 + 预估费用
  final_review:
    description: 成片完成后确认
    show_to_user: 视频播放器 + 时间线 + 分镜对照

# 风格约束
style_constraints:
  mood_anchor_template: "{brand_tone}, {visual_style}, 商业级画质"
  forbidden: [竞品 Logo, 虚假宣传, 未授权肖像]
  prompt_suffix: "commercial quality, professional lighting"
```

### 8.3 Skill 使用流程

```
用户: "帮我做一个30秒的咖啡广告，投放抖音"
  │
  ▼
Producer Agent:
  1. 匹配 Skill: 关键词"广告" + 时长30s + 平台"抖音" → marketing-ad-short
  2. assets.required → 检查用户是否提供了产品主图
  3. phases → 构建执行计划
  4. review_rubric → 传递给 Review Sub-Agent
  5. style_constraints → 传递给 Craftsman
  6. gates → 在对应阶段设置 Gate
  7. 按 phases 顺序调度 Sub-Agent
```

### 8.4 Skill 分层

| 层级 | 说明 | 存储 |
|---|---|---|
| 系统内置 Skill | 覆盖常见视频类型 | 后端代码或配置 |
| 用户自定义 Skill（MVP 后期） | 用户创建，定义自己的制作流程 | DB skill 表 |

内置 Skill 包括：marketing-ad-short、product-demo、brand-story、tutorial、talking-head。

## 9. spark-video 关键借鉴

| spark-video 机制 | ClipAnvil 适配方式 | 价值 |
|---|---|---|
| SKILL.md 领域知识 | Skill YAML 文档 | 不同视频类型有不同流程和标准 |
| Producer 编排 + 子角色 | MultiAgent 调度 | 避免单 Agent 提示词膨胀 |
| Cast/Set/Prop 三支柱 | reference edge + Asset 复用 | 跨镜头角色/场景一致性 |
| mood_anchor | style_constraints.mood_anchor_template | 全局风格锚定 |
| narrative_purpose | 分镜节点必填字段 | 每个镜头有明确叙事目的 |
| shots_state.json | Production State Summary | Agent 全局状态感知 |
| 6 轴评审 | review_rubric | 结构化多维度评审 |
| chain-DAG 并行渲染 | 并行生成无依赖镜头 | 渲染效率 |
| 4+2 Gate | 简化为 2 Gate | 在成本不可逆节点确认 |
| Last-frame continuation | sequence edge + `use_prev_last_frame` | 镜头间物理连续性 |
| viewer.html 溯源 | 画布即溯源界面 | 每个节点可追溯 |

**不借鉴的部分**：
- 文件系统存储模式（cast/folder）→ 用 DB 建模
- CLI 交互 → Web 画布交互
- VFX Review 独立 Gate → Preflight 检查集成在 Craftsman 提交前校验中

## 10. 完整工作流示例

以"30秒咖啡广告"为例。Agent 调用的全部是生产级工具，没有任何画布操作：

### 阶段 1: 需求解析

```
用户: "帮我做一个30秒的咖啡新品广告，目标抖音，风格时尚简约，
       产品是燕麦拿铁，卖点是低糖健康"

Producer:
  1. 匹配 Skill → marketing-ad-short
  2. read_project_context(include=["object_index","production_state"]) → 检查已有素材和状态
  3. upsert_project_brief + update_project_memory → 固化目标和约束
  4. 回复: "收到！先规划分镜，请稍等..."
```

### 阶段 2: 分镜规划

```
Producer:

  upsert_key_elements({
    elements: [
      {semantic_key: "product_oat_latte", type: "product", name: "低糖燕麦拿铁"}
    ]
  })

  upsert_storyboard({
    scenes: [{client_key: "scene_main", title: "时尚咖啡馆主场景"}],
    shots: [
      {title: "01-产品特写", duration: 3, narrative_purpose: "抓注意力",
       description: "微距拍摄燕麦拿铁表面拉花"},
      {title: "02-生活场景", duration: 7, narrative_purpose: "建立兴趣",
       description: "年轻女性在咖啡馆端起拿铁"},
      ... 共5个镜头
    ],
    dependencies: []
  })

  → 系统自动（Agent 不感知）:
    写入 5 个 shot + 必要的 shot_dependency
    + 更新 Agent Workbench 投影 + 广播 WebSocket → 画布出现场景和分镜
```

### Gate 1: 分镜确认

```
request_user_decision("共5个镜头，总时长30s，预估¥3-5")

  对话面板显示确认卡片，画布上可见分镜排列。

  ├── 用户"开始生成" → 进入阶段3
  ├── 用户"第三个改成倒咖啡" → upsert_storyboard patch → 重新展示 Gate
  └── 用户点击画布卡片 → 查看详情 → 在对话中修改
```

### 阶段 3: 素材准备 + 视频生成

```
Producer 并行调度:

  Craftsman × 5（无阻塞依赖的可并行）:
    dispatch_craftsman({
      scope_refs: [{type: "shot", key: "shot_01"}],
      target_phase: "preview_image",
      instructions: "生成符合 ProjectMemory 和 shot 描述的 Seedream 预览图计划"
    })

  Craftsman:
    upsert_render_plan(...)

  Producer:
    read_project_context(include=["render_plans"])
    decide_render_plan({decisions: [{render_plan_ref: ..., action: "accept"}]})

  Worker:
    提交 generation_job / artifact_version

  画布实时更新:
    生成中 → 蓝色边框 + 进度条
    完成 → 绿色边框 + 缩略图
    失败 → 红色边框
```

### 阶段 4: 评审与重试

```
Producer:
  dispatch_reviewer({review_task: "preview_image_review", target: ...})

Reviewer:
  submit_review_result(...)

Producer:
  → 通过: 继续派 shot_video RenderPlan
  → 不通过: dispatch_craftsman(mode="fork_from", critique / fix_hints)
  → 多次不通过: request_user_decision 请求用户帮助
```

### 阶段 5: 拼接成片

```
当前商业级 TimelinePlan / Composer 仍是后续方向。已有留存 Composer 能力可通过内部 ffmpeg provider 拼接视频，但三 Agent 主链路当前重点是分镜图、分镜视频、评审和局部修复。
```

### Gate 2: 成片预览

对话面板显示视频播放器 + 时间线。

**整个流程工具调用量**：~20 次生产工具调用。

## 11. 用户中途干预

Agent 运行过程中，用户随时可通过对话干预：

| 用户说 | Producer 的响应 |
|---|---|
| "停一下" / "暂停" | 暂停所有 Sub-Agent，不取消已提交的生成任务 |
| "继续" | 恢复暂停的流程 |
| "第二个镜头改成户外场景" | Producer patch Shot → 下游 RenderPlan/artifact stale → 请求确认重跑范围 |
| "换个模型试试" | Producer 派 Craftsman fork RenderPlan，修改 model profile / params |
| "这个产品图不好，我上传一张新的" | 等待上传 → Producer 更新 KeyElementState reference → 下游依赖 stale |
| "第3和第4个镜头合并成一个" | Producer patch Storyboard / dependency，旧分镜归档或标记不再使用 |
| "整体风格偏暖色调" | Producer 更新 ProjectMemory visual anchors → 提示确认影响范围 |

## 12. Stale 处理

Agent 模式下 Stale 由 Agent 在对话中报告（而非 Studio 的画布影响分析条）：

```
Agent: "您修改了产品主图，这会影响 3 个分镜节点（01 特写、02 场景、03 卖点）。
       需要重新生成这些镜头，预估费用 ¥2.1，耗时约 3 分钟。
       要重新生成吗？"
```

## 13. 版本管理

Agent 模式下用户通过对话管理版本：

```
用户: "第二个镜头有没有其他版本？"
Agent: "第2个镜头有 3 个版本：
       - v1: 评分 6.8（面部表情不自然）
       - v2: 评分 7.5（构图偏左）
       - v3: 评分 8.2 ★当前选中
       要切换到其他版本吗？"
用户: "切到 v2 看看"
Agent: "我会把第2个镜头切到 v2，并标记依赖它的视频/成片需要重新生成。要现在继续吗？"
```

## 14. 跨镜头一致性

借鉴 spark-video 三支柱：

- **一致性资产**：角色/场景/风格参考图作为独立 MediaNode，通过 reference edge 连到分镜
- **Prompt 构成规则**：场景描述 + 动作/表情 + mood_anchor。角色外观由参考图锁定，Prompt 不描述外观
- **帧连续性**：sequence edge + `use_prev_last_frame`，前镜头最后一帧 → 下镜头首帧

## 相关文档

- [整体设计](overview.md) — 架构、原则、路线图
- [画布设计](canvas.md) — 节点/连线/分组视觉规格、数据通路
- [Studio 模式](studio-mode.md) — 用户主导的创作交互
- [数据库设计](../engineering/database.md) — schema 和迁移
