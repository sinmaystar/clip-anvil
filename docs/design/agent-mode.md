# ClipAnvil Agent 模式设计方案

## 1. 定位

Agent 模式是"默认由 Agent 主导生产，用户在关键节点确认、随时可通过对话干预"的自动化生产模式。

画布只读——用户看到 Agent 的工作成果在画布上实时呈现，所有修改通过对话驱动。

## 2. 界面布局

```
┌──────────────────────────────────────────────────────────────┐
│  影砧  ·  ☕ 咖啡广告项目    [Agent 模式 ▾]     ⌘K  导出  设置 │
├──────────┬───────────────────────────────────┬───────────────┤
│          │                                   │ 💬 对话       │
│ 🔍搜索    │         画布区域                   │              │
│          │    （tldraw 无限画布，只读）         │ [Agent 状态栏] │
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
| `get_production_state` | 获取全量 PSS 快照 | 完整文本 |
| `get_shot_detail(shotId)` | 单个分镜完整信息 | Prompt 全文、模型参数、所有版本及评审 |
| `get_asset_detail(assetId)` | 素材详细信息 | 文本内容、图片 URL、视频 URL |

### 5.5 Sub-Agent 的状态可见性

- Producer 将 PSS 的相关子集传给 Sub-Agent（如 Director 只需当前分镜 + 可用素材列表）
- Sub-Agent 执行完毕后，Producer 通过工具返回值的增量更新感知变化
- 需要全局视图时 Producer 调用 `get_production_state` 刷新

## 6. 生产级工具

### 6.1 设计原则

Agent 的工具不是画布操作（create_node、create_edge），而是**生产操作**（create_storyboard、generate_shot）。每个生产工具在内部自动翻译为多个业务命令，Agent 完全不感知底层操作。

**对比**：

```
旧设计（Agent 调 14 个画布原子工具，7 步生成一个镜头）:
  create_media_node("video", "镜头01")
  update_media_node(nodeId, {prompt: "..."})
  create_media_edge(productImage, nodeId, "dependency")
  create_media_edge(logo, nodeId, "dependency")
  submit_generation(nodeId, provider, model, ...)
  [等待完成]
  auto_layout()

新设计（Agent 调 1 个生产工具）:
  generate_shot("镜头01", {
    prompt: "...",
    model: "qwen-vl-max",
    reference_inputs: [{id: "产品主图ID", role: "product"}, {id: "LogoID", role: "brand"}]
  })
  → 系统自动: 更新节点prompt + 建依赖连线 + 提交生成 + 完成后更新状态/缩略图
```

### 6.2 工具列表

**规划类**：

| 工具 | 参数 | 说明 |
|---|---|---|
| `create_storyboard` | shots[]{title, duration, description, narrative_purpose}, transitions[]{from_index, to_index, type, duration} | 一次性创建完整分镜表。系统自动：创建节点 + 顺序连线 + 分组 + 布局。返回 shotIds |
| `modify_storyboard` | changes{add_shots?[], remove_shot_ids?[], update_shots?[], update_transitions?[]} | 修改分镜。系统自动：增删节点/连线 + 标记下游 Stale + 重新布局 |
| `import_asset` | type, title, source(url\|upload_id\|content) | 导入用户素材。系统自动：创建资产节点 + 上传存储 + 画布显示 |

**生成类**：

| 工具 | 参数 | 说明 |
|---|---|---|
| `generate_shot` | shot_id, prompt, model, reference_inputs[]{asset_id, role}?, params? | 为分镜生成视频。系统自动：写 prompt + 建依赖连线 + 提交生成 |
| `generate_asset` | type, title, prompt, model, reference_inputs?[], params? | 生成辅助素材。系统自动：创建素材节点 + 提交生成 |
| `stitch_final` | shot_ids_in_order, bgm_asset_id?, subtitle_config? | 拼接成片。系统自动：创建成片节点 + 连接源分镜 + 提交拼接 |

**评审与版本**：

| 工具 | 参数 | 说明 |
|---|---|---|
| `review_shot` | shot_id, rubric? | 质量评审。返回评分和批评 |
| `select_version` | shot_id, version_id | 设为 winner。系统自动：更新缩略图 + 标记下游 Stale |
| `retry_generation` | shot_id, revised_prompt, reason | 改写 Prompt 重新生成。系统自动：创建新版本 |

**状态查询**：`get_production_state`、`get_shot_detail`、`get_asset_detail`（见 §5.4）

**流程控制**：`request_gate(gate_type, summary)` — 请求用户确认，Agent 暂停直到用户响应。

### 6.3 工具执行流程

```
Agent 调用生产工具（如 generate_shot）
  │
  ▼
生产翻译层
  ├── 解析 reference_inputs → 确定依赖关系
  ├── 生成初始画布坐标
  │
  ├── 调用业务命令（自动，Agent 不感知）:
  │     ├── update_media_node(nodeId, {prompt, model})
  │     ├── create_media_edge(ref → node, "dependency")  × N
  │     ├── submit_generation_job(nodeId, ...)
  │     └── 审计日志（agent_step 表）
  │
  ├── 广播 WebSocket 事件
  │     └── 前端收到 → 创建/更新 tldraw shape → 画布刷新
  │
  └── 返回结果给 Agent（生产语言）:
        ├── {shot_id, status: "generating", job_id}
        └── PSS 增量: "[开始] shot-01 开始生成, 模型 qwen-vl-max"
```

### 6.4 连线的自动派生

Agent 不创建连线。所有连线从两个来源自动派生：

| 连线类型 | 派生来源 | 时机 |
|---|---|---|
| **dependency** | `generate_shot`/`generate_asset` 的 `reference_inputs` | 提交生成时自动创建 |
| **sequence** | `create_storyboard` 的 `transitions` | 创建分镜表时自动创建 |
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
  ├── 读取 Skill（领域知识）
  ├── 读取 PSS（当前进度）
  ├── 管理 Gate（用户确认）
  ├── 处理用户对话修改
  │
  ├── 调度 → Screenwriter Sub-Agent
  │           └── 调用：create_storyboard
  │
  ├── 调度 → Director Sub-Agent（每个镜头一个，可并行）
  │           └── 调用：generate_shot
  │
  ├── 调度 → Art Asset Sub-Agent
  │           └── 调用：generate_asset, import_asset
  │
  ├── 调度 → Review Sub-Agent
  │           └── 调用：review_shot, select_version, retry_generation
  │
  └── 调度 → Stitch Sub-Agent
              └── 调用：stitch_final
```

### 7.2 角色分工

| 角色 | 职责 | 工具权限 |
|---|---|---|
| **Producer** | 解析需求、加载 Skill、编排流程、管理 Gate、处理用户修改 | 全部生产工具 + 调度 Sub-Agent |
| **Screenwriter** | 生成剧本和分镜表 | `create_storyboard`, `modify_storyboard` |
| **Director** | 为单个镜头生成 Prompt 并提交生成 | `generate_shot` |
| **Art Asset** | 生成参考素材 | `generate_asset`, `import_asset` |
| **Prompt Rewrite** | 评审不通过时改写 Prompt | `retry_generation` |
| **Review** | 评审生成结果 | `review_shot`, `select_version` |
| **Stitch** | 拼接成片 | `stitch_final` |

### 7.3 模型选择策略

| 角色 | 推荐模型能力 | 理由 |
|---|---|---|
| Producer | 强推理 | 理解复杂需求、全局规划 |
| Screenwriter | 强创意 + 结构化输出 | 有创意且格式规范的分镜 |
| Director | 领域知识 | 各生成模型的 Prompt 最佳实践 |
| Prompt Rewrite | 领域知识 + 推理 | 分析失败原因并改写 |
| Review | 多模态理解 | 需要"看"生成的图片/视频 |
| Stitch | 工具调用 | 主要编排 FFmpeg，逻辑简单 |

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
  5. style_constraints → 传递给 Director Sub-Agent
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
- VFX Review 独立 Gate → Preflight 检查集成在 Director 提交前校验中

## 10. 完整工作流示例

以"30秒咖啡广告"为例。Agent 调用的全部是生产级工具，没有任何画布操作：

### 阶段 1: 需求解析

```
用户: "帮我做一个30秒的咖啡新品广告，目标抖音，风格时尚简约，
       产品是燕麦拿铁，卖点是低糖健康"

Producer:
  1. 匹配 Skill → marketing-ad-short
  2. get_production_state() → 检查已有素材
  3. 解析需求 → Brief
  4. 回复: "收到！先规划分镜，请稍等..."
```

### 阶段 2: 分镜规划

```
Producer → Screenwriter Sub-Agent:

  create_storyboard({
    shots: [
      {title: "01-产品特写", duration: 3, narrative_purpose: "抓注意力",
       description: "微距拍摄燕麦拿铁表面拉花"},
      {title: "02-生活场景", duration: 7, narrative_purpose: "建立兴趣",
       description: "年轻女性在咖啡馆端起拿铁"},
      ... 共5个镜头
    ],
    transitions: [
      {from_index: 0, to_index: 1, type: "crossfade", duration: 0.5},
      {from_index: 1, to_index: 2, type: "cut"},
      ...
    ]
  })

  → 系统自动（Agent 不感知）:
    创建 5 个 media_node + 4 条 sequence edge
    + 创建分组 + 计算布局 + 广播 WebSocket → 画布出现 5 个分镜卡片
```

### Gate 1: 分镜确认

```
request_gate("storyboard_review", "共5个镜头，总时长30s，预估¥3-5")

  对话面板显示确认卡片，画布上可见分镜排列。

  ├── 用户"开始生成" → 进入阶段3
  ├── 用户"第三个改成倒咖啡" → modify_storyboard → 重新展示 Gate
  └── 用户点击画布卡片 → 查看详情 → 在对话中修改
```

### 阶段 3: 素材准备 + 视频生成

```
Producer 并行调度:

  Art Asset Sub-Agent:
    generate_asset("image", "产品参考图", {prompt: "...", model: "flux-schnell"})

  Director Sub-Agent × 5（无帧依赖的可并行）:
    generate_shot(shot_id, {
      prompt: "微距拍摄燕麦拿铁拉花...",
      model: "qwen-vl-max",
      reference_inputs: [{asset_id: "产品主图ID", role: "product"}]
    })

  画布实时更新:
    生成中 → 蓝色边框 + 进度条
    完成 → 绿色边框 + 缩略图
    失败 → 红色边框
```

### 阶段 4: 评审与重试

```
Review Sub-Agent:
  review_shot(shot_id, rubric)
  → 通过: select_version(shot_id, version_id)
  → 不通过: retry_generation(shot_id, revised_prompt, reason)
  → 3次不通过: 对话中请求用户帮助
```

### 阶段 5: 拼接成片

```
Stitch Sub-Agent:
  stitch_final({shot_ids_in_order: [s1,s2,s3,s4,s5], bgm_asset_id: "BGM_ID"})
```

### Gate 2: 成片预览

对话面板显示视频播放器 + 时间线。

**整个流程工具调用量**：~20 次（vs 旧设计 ~60+ 次画布原子操作）。

## 11. 用户中途干预

Agent 运行过程中，用户随时可通过对话干预：

| 用户说 | Producer 的响应 |
|---|---|
| "停一下" / "暂停" | 暂停所有 Sub-Agent，不取消已提交的生成任务 |
| "继续" | 恢复暂停的流程 |
| "第二个镜头改成户外场景" | 修改分镜描述 → mark_stale 下游 → 重新生成 |
| "换个模型试试" | 修改生成参数 → 重新提交 |
| "这个产品图不好，我上传一张新的" | 等待上传 → 更新 asset → mark_stale 下游 |
| "第3和第4个镜头合并成一个" | 删除一个节点 + 更新另一个 + 调整连线 |
| "整体风格偏暖色调" | 更新 mood_anchor → 所有镜头 mark_stale → 提示确认重新生成范围 |

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
Agent: [select_version] "已切换到 v2。成片需要重新拼接，要现在拼接吗？"
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
