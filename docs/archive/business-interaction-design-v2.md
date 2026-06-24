# ClipAnvil 业务交互设计 v2

## 0. 与既有设计的关键修正

本文档基于 `business-interaction-design.md`（v1）和 `React Flow 多媒体画布与 Agent 视频生成业务交互设计.md`（Manus 调研）重新设计。以下是核心修正点：

| # | v1 设计 | v2 修正 | 理由 |
|---|---|---|---|
| 1 | Studio 与 Agent 融合，不存在模式切换 | **明确分为两个模式，Agent 模式画布只读** | Agent 模式下用户不直接操作画布，通过对话驱动修改；消除并发编辑的状态冲突复杂度 |
| 2 | Agent 通过画布原子命令操作（create_node、create_edge…） | **Agent 改为调用生产级工具**（create_storyboard、generate_shot），系统自动翻译为画布状态 | Agent 不应意识到画布的存在；连线、分组、布局全部由系统从生产关系自动派生 |
| 3 | 单 Agent + Skill 角色列表 | **MultiAgent 架构：Producer 调度 Sub-Agent** | 单 Agent 提示词膨胀导致能力退化；Sub-Agent 并行提升效率；Skill 作为领域知识注入 |
| 4 | UserEditing 状态 + 节点锁 | **Agent 模式下取消 UserEditing**（不需要，因为用户不直接编辑） | 画布只读，用户修改全部走对话 → Agent → 命令 API 通路 |
| 5 | 乐观更新（Studio 模式） | **Studio 模式改为先请求后渲染**（非乐观更新） | 200ms 延迟在业务操作中用户几乎无感；省去回滚逻辑的复杂度（已在 database-design.md 中确认） |
| 6 | 手动布局为主 | 新增 **自动布局机制**（Agent 模式必选，Studio 模式可选） | Agent 批量创建节点时必须自动排列，否则画面混乱 |
| 7 | 交互强度自适应（三档隐式） | **取消三档概念，Agent 模式固定行为：Gate 点暂停 + 对话式修改** | 简化设计；用户通过 Studio/Agent 模式切换表达意图，而非隐式推断 |
| 8 | Brief/Plan 等系统节点不显示在画布 | 保留此原则，**但增加对话面板中的阶段指示器** | 用户需要知道 Agent 执行到了哪个阶段，通过对话面板而非画布呈现 |

| 9 | Agent 感知 Workspace State Summary（画布级描述） | **改为 Production State Summary**（生产级描述） | Agent 眼中只有分镜/素材/进度，没有节点/连线/分组 |

保留不变的核心原则：
- 业务 DB 为唯一事实源，React Flow 只做投影
- 不弹框，不打断画布流
- 节点卡片内联编辑 + 右侧属性面板深度编辑（Studio 模式）

## 1. 四层架构

v1 是"业务层 + 画布层"双层。v2 初版在中间插入 Agent 感知层形成三层，但仍然让 Agent 操作画布原子工具（create_node、create_edge），本质上还是让 Agent 充当"画布操作员"。

修正后的架构在 Agent 和业务层之间插入 **生产翻译层**，形成四层：

```
┌──────────────────────────────────────────────────────┐
│                    用户界面层                          │
│   React Flow 画布（可视化投影）+ 对话面板（Agent 交互）      │
└───────────────────┬──────────────────────────────────┘
                    │ WebSocket 事件流 / REST API
┌───────────────────┴──────────────────────────────────┐
│                    业务层                              │
│   命令 API → PostgreSQL（事实源）→ 事件广播             │
│   生成任务队列 → 模型供应商 → 结果写回                  │
└───────────────────┬──────────────────────────────────┘
                    ▲ 业务命令（自动）
                    │ 一个生产工具调用 → 多个业务命令
┌───────────────────┴──────────────────────────────────┐
│                 生产翻译层（系统自动）                   │
│                                                      │
│  Agent 调用: create_storyboard(shots)                 │
│  系统翻译: batch_create_nodes + batch_create_edges     │
│           + create_group + auto_layout                │
│                                                      │
│  Agent 调用: generate_shot(prompt, reference_inputs)   │
│  系统翻译: update_node(prompt) + create_edges(refs)    │
│           + submit_generation                         │
│                                                      │
│  生成完成（回调）:                                     │
│  系统自动: create_asset + create_version               │
│           + update_node(status, thumbnail)             │
└───────────────────┬──────────────────────────────────┘
                    ▲ 生产级工具（Agent 调用）
                    │ get_production_state（Agent 读取）
┌───────────────────┴──────────────────────────────────┐
│              Agent 层（MultiAgent）                    │
│   Producer → Sub-Agents（Screenwriter/Director/...）  │
│   读取 Skill + Production State                       │
│   调用生产级工具                                       │
│   不知道画布、节点、连线、布局的存在                      │
└──────────────────────────────────────────────────────┘
```

**各层职责边界**：

| 层 | 知道什么 | 不知道什么 |
|---|---|---|
| 用户界面层 | React Flow node、画布坐标、视觉样式、事件渲染 | Agent 内部编排逻辑、生产翻译规则 |
| 业务层 | 完整数据模型、DAG、版本、任务状态 | React Flow、Agent 提示词 |
| 生产翻译层 | 生产操作 ↔ 业务命令的映射规则 | React Flow、Agent 提示词 |
| Agent 层 | 分镜、素材、生成、评审、拼接 | 节点、连线、分组、画布坐标、布局、React Flow |

核心设计决策：**Agent 完全不知道画布的存在。Agent 眼中只有"项目里有哪些分镜、什么进度、用了哪些参考素材"。节点、连线、分组、布局——全部由生产翻译层自动派生。**

**为什么需要生产翻译层而不是让 Agent 直接调业务命令**：

一个生产操作 = 多个业务命令。例如 `create_storyboard(5个分镜)` 在业务层需要：创建5个 media_node + 4条 sequence edge + 1个 group + 计算布局坐标 + 写入 canvas_x/y。如果让 Agent 逐一调用这些命令，Agent 就变成了画布操作员，认知负荷高且容易出错。生产翻译层把这些打包成一步，Agent 只需要表达"我要创建一个5镜头的分镜表"。

## 2. 双模式设计

### 2.1 模式定义

| | Studio 模式 | Agent 模式 |
|---|---|---|
| **定位** | 用户主导的自由创作空间 | Agent 主导的自动化生产，用户观察与对话式修改 |
| **画布交互** | 完全可编辑（创建、拖拽、连线、删除） | **只读**（平移、缩放、点击查看详情，不可编辑） |
| **右侧面板** | 属性面板（选中节点时显示） | **对话面板**（始终显示，Agent 工作状态 + 对话输入） |
| **左侧资源树** | 完全可操作（新建、拖拽、删除） | 只读浏览，点击定位到画布节点 |
| **工具栏** | 显示创建工具（文本/图片/视频/音频） | **不显示创建工具**，仅显示视图控制（缩放、自动整理） |
| **节点编辑** | 内联编辑 + 属性面板 | 点击节点查看详情（只读），修改通过对话 |
| **谁创建内容** | 用户 | Agent |
| **数据通路** | 用户操作 → REST API → DB → 画布 | 用户对话 → Agent → 命令工具 → DB → 事件流 → 画布 |

### 2.2 模式切换

Workspace 级别设置，保存在 `workspace.settings.mode` 中。

**切换规则**：
- Studio → Agent：保留画布上已有的所有节点和连线，Agent 可以识别并复用它们
- Agent → Studio：保留 Agent 创建的所有内容，用户获得完全编辑权限
- Agent 运行中不可切换到 Studio（需先暂停或完成 Agent 任务）

**为什么不融合**：v1 设计中 Studio/Agent 融合的核心问题是 UserEditing 状态管理。当用户和 Agent 可能同时操作同一个节点时，需要锁机制、冲突解决和状态同步。分离后：
- Studio 模式没有 Agent 运行，不存在并发冲突
- Agent 模式画布只读，用户只能通过对话让 Agent 修改，天然串行化

### 2.3 Agent 模式下的用户交互

画布只读不意味着用户无法与画布交互。用户在 Agent 模式下可以：

| 交互 | 行为 | 效果 |
|---|---|---|
| 平移/缩放画布 | 正常操作 | 浏览全局布局 |
| 单击节点 | 节点高亮，右侧对话面板下方浮现节点摘要卡 | 用户可以在对话中引用该节点（"修改这个镜头"）|
| 双击节点 | 弹出节点详情浮层（只读）：完整 Prompt、版本列表、评审记录 | 深度查看 |
| 单击连线 | 连线高亮，显示连线类型和转场信息 | 查看依赖/顺序关系 |
| 右键 | **无菜单**（Agent 模式下禁止右键菜单） | — |
| 悬停节点 | 显示状态 tooltip（生成进度、评分等） | 快速状态确认 |

**节点摘要卡**（单击节点后出现在对话面板底部输入框上方）：

```
┌──────────────────────────────────────────┐
│ 🎬 03-卖点展示 · 6s · 草稿               │
│ Prompt: 燕麦拿铁从上方缓缓倒入杯中...     │
│ 模型: qwen-vl-max · 依赖: 产品主图, Logo  │
│                              [在对话中引用] │
└──────────────────────────────────────────┘
```

点击"在对话中引用"会在对话输入框插入 `@03-卖点展示`，用户可以直接说"把 @03-卖点展示 改成俯拍视角"。

## 3. Agent 感知层：Production State Summary

### 3.1 设计动机

Agent 需要知道当前项目的状态（有哪些分镜、生成进度如何、用了哪些素材），但它不需要——也不应该——知道这些东西在画布上怎么排列。

解决方案：**Production State Summary（PSS）**——一段用生产概念描述的结构化文本。Agent 读它来感知项目状态，完全不涉及画布概念。

这类似 spark-video 的 `shots_state.json`——一个 single source of truth 的结构化摘要，所有角色都从它读取当前进度。

### 3.2 PSS 格式

PSS 分为 **全量快照** 和 **增量更新** 两种形式。注意：没有"节点"、"连线"、"分组"等画布概念，只有"分镜"、"素材"、"进度"等生产概念。

**全量快照**（Agent 启动时、上下文刷新时使用）：

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

**增量更新**（每次生产工具执行后附加到 Agent 上下文）：

```
[完成] shot-01 生成完成, v1★ 评分8.2, 耗时32s, 费用¥0.15
[开始] shot-02 开始生成
```

### 3.3 PSS 读取工具

| 工具 | 用途 | 返回 |
|---|---|---|
| `get_production_state` | 获取全量 PSS 快照 | 上述格式的完整文本 |
| `get_shot_detail(shotId)` | 获取单个分镜的完整信息 | Prompt 全文、模型参数、版本列表（含评审记录）、参考素材 |
| `get_asset_detail(assetId)` | 获取素材的详细信息和内容 URL | 文本内容、图片 URL、视频 URL 等 |

**增量更新是自动的**：每次 Agent 调用生产工具，工具返回结果中附带 PSS 增量更新摘要。Agent 无需主动刷新，除非上下文窗口被压缩。

### 3.4 Sub-Agent 的状态可见性

- Producer 将当前 PSS 的相关子集传给 Sub-Agent（如 Director 只需要当前分镜 + 可用素材列表）
- Sub-Agent 执行完毕后，Producer 通过工具返回值的增量更新感知变化
- 需要全局视图时 Producer 调用 `get_production_state` 刷新

## 4. 生产级工具（Agent 的唯一操作界面）

### 4.1 设计原则

Agent 的工具不是画布操作（create_node、create_edge），而是**生产操作**（create_storyboard、generate_shot）。每个生产工具在内部自动翻译为多个业务命令（创建节点、建连线、提交任务等），Agent 完全不感知这些底层操作。

典型调用：

```
generate_shot("镜头01", {
  prompt: "...",
  model: "qwen-vl-max",
  reference_inputs: [{id: "产品主图ID", role: "product"}, {id: "LogoID", role: "brand"}]
})
```
  → 系统自动: 更新节点prompt + 从reference_inputs建依赖连线 + 提交生成 + 完成后更新状态/缩略图
```

### 4.2 工具列表

**规划类**：

| 工具 | 参数 | 说明 |
|---|---|---|
| `create_storyboard` | shots[]{title, duration, description, narrative_purpose}, transitions[]{from_index, to_index, type, duration} | 一次性创建完整分镜表。系统自动：创建所有分镜节点 + 顺序连线 + 分组 + 自动布局。返回 shotIds |
| `modify_storyboard` | changes{add_shots?[], remove_shot_ids?[], update_shots?[{id, patch}], update_transitions?[]} | 修改已有分镜。系统自动：增删节点/连线 + 标记下游 Stale + 重新布局 |
| `import_asset` | type(image\|video\|audio\|text), title, source(url\|upload_id\|content) | 导入用户提供的素材。系统自动：创建资产节点 + 上传到存储 + 画布上显示 |

**生成类**：

| 工具 | 参数 | 说明 |
|---|---|---|
| `generate_shot` | shot_id, prompt, model, reference_inputs[]{asset_id, role}?, params? | 为一个分镜生成视频。系统自动：写 prompt 到节点 + 从 reference_inputs 派生依赖连线 + 提交生成任务 + 完成后更新状态/缩略图/版本 |
| `generate_asset` | type, title, prompt, model, reference_inputs?[], params? | 生成辅助素材（参考图、风格图等）。系统自动：创建素材节点 + 提交生成 + 完成后画布显示 |
| `stitch_final` | shot_ids_in_order, bgm_asset_id?, subtitle_config? | 拼接成片。系统自动：创建成片节点 + 连接到所有源分镜 + 提交拼接任务 |

**评审与版本**：

| 工具 | 参数 | 说明 |
|---|---|---|
| `review_shot` | shot_id, rubric? | 对已生成的分镜执行质量评审。返回评分和批评 |
| `select_version` | shot_id, version_id | 将某版本设为 winner。系统自动：更新节点缩略图 + 标记下游成片 Stale |
| `retry_generation` | shot_id, revised_prompt, reason | 改写 Prompt 后重新生成。系统自动：创建新版本 + 提交生成 |

**状态查询**：

| 工具 | 参数 | 说明 |
|---|---|---|
| `get_production_state` | — | 获取全量 PSS 快照 |
| `get_shot_detail` | shot_id | 获取分镜的完整信息（Prompt、模型参数、所有版本及评审） |
| `get_asset_detail` | asset_id | 获取素材内容或 URL |

**流程控制**：

| 工具 | 参数 | 说明 |
|---|---|---|
| `request_gate` | gate_type, summary | 请求用户确认。Agent 暂停直到用户响应 |

### 4.3 工具执行流程

```
Agent 调用生产工具（如 generate_shot）
  │
  ▼
生产翻译层
  ├── 解析 reference_inputs → 确定依赖关系
  ├── 生成画布坐标（自动布局）
  │
  ├── 调用业务命令（自动，Agent 不感知）:
  │     ├── update_media_node(nodeId, {prompt, model})
  │     ├── create_media_edge(ref1 → node, "dependency")  × N
  │     ├── submit_generation_job(nodeId, ...)
  │     └── 审计日志（agent_step 表）
  │
  ├── 广播 WebSocket 事件
  │     └── 前端收到 → 创建/更新 React Flow node + custom edge → 画布刷新
  │
  └── 返回结果给 Agent（生产语言，非画布语言）:
        ├── {shot_id, status: "generating", job_id}
        └── PSS 增量: "[开始] shot-01 开始生成, 模型 qwen-vl-max"
```

### 4.4 连线的自动派生

Agent 不创建连线。所有连线由系统从两个来源自动派生：

| 连线类型 | 派生来源 | 时机 |
|---|---|---|
| **dependency** | `generate_shot` 或 `generate_asset` 的 `reference_inputs` 参数 | 提交生成时自动创建 |
| **sequence** | `create_storyboard` 的 `transitions` 参数 | 创建分镜表时自动创建 |
| **reference** | 用户在 Studio 模式手动拖拽 | 仅 Studio 模式 |

好处：
- Agent 永远不需要思考"我要建一条从 A 到 B 的连线"
- 连线和实际的生成输入**始终一致**（因为它们从同一数据源派生）
- 如果 Agent 修改了 reference_inputs，系统自动更新对应的连线

## 5. MultiAgent + Skill 体系

### 5.1 MultiAgent 架构

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
  │           ├── 接收：Brief + Skill 中的分镜规则
  │           ├── 产出：分镜列表
  │           └── 调用：create_storyboard（一步搞定）
  │
  ├── 调度 → Director Sub-Agent（每个镜头一个，可并行）
  │           ├── 接收：分镜描述 + 可用素材列表
  │           ├── 产出：生成的视频
  │           └── 调用：generate_shot（prompt + reference_inputs → 系统自动建连线和生成）
  │
  ├── 调度 → Art Asset Sub-Agent
  │           ├── 接收：素材需求
  │           ├── 产出：参考图/风格图
  │           └── 调用：generate_asset（系统自动创建素材节点和生成）
  │
  ├── 调度 → Review Sub-Agent
  │           ├── 接收：生成结果 + 评审标准
  │           ├── 产出：评分 + 判定
  │           └── 调用：review_shot + select_version 或 retry_generation
  │
  └── 调度 → Stitch Sub-Agent
              ├── 接收：所有 winner shot_ids + 转场 + BGM
              ├── 产出：成片视频
              └── 调用：stitch_final（系统自动创建成片节点和拼接）
```

### 5.2 Agent 角色分工

| 角色 | 职责 | 输入 | 输出 | 工具权限 |
|---|---|---|---|---|
| **Producer** | 解析需求、加载 Skill、编排流程、管理 Gate、处理用户修改 | 用户对话 + PSS | 编排指令 | 全部生产工具 + 调度 Sub-Agent |
| **Screenwriter** | 生成剧本和分镜表 | Brief + Skill 规则 | 分镜列表 | `create_storyboard`, `modify_storyboard` |
| **Director** | 为单个镜头生成 Prompt 并提交生成 | 分镜描述 + 可用素材 | 生成的视频 | `generate_shot` |
| **Art Asset** | 生成参考素材（角色参考图、场景图等） | 素材需求 | 参考素材 | `generate_asset`, `import_asset` |
| **Prompt Rewrite** | 评审不通过时分析原因并改写 Prompt | 失败原因 + 原 Prompt | 改写后重新生成 | `retry_generation` |
| **Review** | 评审生成结果 | 生成产物 + 评审标准 | 评分 + 判定 | `review_shot`, `select_version` |
| **Stitch** | 拼接成片 | winner 视频列表 + 转场 + BGM | 成片 | `stitch_final` |

### 5.3 Skill 体系

#### 5.3.1 Skill 定位

借鉴 spark-video 的核心理念：**Skill 描述判断标准和契约，不是刚性脚本。Agent 在 Skill 的框架内拥有自主判断权。**

Skill 在 ClipAnvil 中的定位是 **领域知识模块**：不同类型的营销视频（产品广告、品牌故事、教程视频、口播视频）有不同的制作方法论，Skill 把这些方法论结构化地传递给 Agent。

#### 5.3.2 Skill 结构

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
    - type: image
      role: reference_style
      description: 风格参考图

# 制作阶段
phases:
  - name: brief_analysis
    role: producer
    description: 解析用户需求，提取产品信息、目标平台、卖点、风格
    output: 结构化 Brief

  - name: storyboard
    role: screenwriter
    description: >
      根据 Brief 生成分镜表。营销广告的分镜遵循 AIDA 模型：
      Attention（前3秒抓注意力）→ Interest（展示产品）→
      Desire（传递卖点）→ Action（行动号召）。
      每个镜头必须有明确的营销目的。
    output: 分镜节点列表
    gate: storyboard_review

  - name: asset_preparation
    role: art_asset
    description: 根据分镜需求生成辅助素材（场景图、风格参考等）
    output: 素材节点
    parallel: true

  - name: shot_generation
    role: director
    description: >
      为每个镜头生成 Prompt 并提交视频生成。
      营销视频的 Prompt 要点：
      - 产品必须在画面中突出展示
      - 色调和光线保持品牌一致性
      - 前3秒必须有视觉冲击力
      - 避免文字水印和非品牌元素
    output: 生成的视频片段
    parallel: true
    retry: 3

  - name: review
    role: review
    description: 评审生成结果
    output: 评审记录

  - name: stitch
    role: stitch
    description: 拼接成片，添加转场和 BGM
    output: 成片视频
    gate: final_review

# 质量评审标准（Review Sub-Agent 使用）
review_rubric:
  axes:
    - name: product_visibility
      weight: 0.25
      description: 产品是否清晰可见、是否占据画面主体
    - name: brand_consistency
      weight: 0.20
      description: 色调、风格是否与品牌调性一致
    - name: visual_quality
      weight: 0.20
      description: 画面质量、分辨率、无明显瑕疵
    - name: narrative_flow
      weight: 0.20
      description: 镜头之间的逻辑连贯性和节奏感
    - name: platform_fit
      weight: 0.15
      description: 是否符合目标平台的内容调性和格式要求
  pass_threshold: 7.0

# Gate 策略
gates:
  storyboard_review:
    description: 分镜完成后，开始消耗生成资源前确认
    show_to_user: 分镜卡片预览 + 总时长 + 镜头数 + 预估费用
  final_review:
    description: 成片完成后确认
    show_to_user: 视频播放器 + 时间线 + 分镜对照

# 风格约束（类似 spark-video 的 lore.md）
style_constraints:
  mood_anchor_template: "{brand_tone}, {visual_style}, 商业级画质"
  forbidden: [竞品 Logo, 虚假宣传, 未授权肖像]
  prompt_suffix: "commercial quality, professional lighting"
```

#### 5.3.3 Skill 如何被 Agent 使用

```
用户: "帮我做一个30秒的咖啡广告，投放抖音"
  │
  ▼
Producer Agent:
  1. 匹配 Skill: 关键词"广告" + 时长30s + 平台"抖音" → 加载 marketing-ad-short
  2. 读取 Skill 的 assets.required → 检查用户是否提供了产品主图
  3. 读取 phases → 构建执行计划
  4. 读取 review_rubric → 传递给 Review Sub-Agent
  5. 读取 style_constraints → 传递给 Director Sub-Agent
  6. 读取 gates → 在对应阶段设置 Gate
  7. 按 phases 顺序调度 Sub-Agent
```

#### 5.3.4 Skill 分层

| 层级 | 说明 | 示例 |
|---|---|---|
| **系统内置 Skill** | 平台预置，覆盖常见视频类型 | marketing-ad-short, product-demo, brand-story, tutorial, talking-head |
| **用户自定义 Skill**（MVP 后期） | 用户基于模板创建，定义自己的制作流程 | my-company-ad-style |

内置 Skill 存储在后端代码或配置中。用户自定义 Skill 存储在 DB 中，关联 workspace。

### 5.4 spark-video 关键借鉴

| spark-video 机制 | ClipAnvil 适配方式 | 价值 |
|---|---|---|
| **SKILL.md 作为领域知识** | Skill YAML 文档，Agent 读取后获得制作方法论 | 不同视频类型有不同的制作流程和质量标准 |
| **Producer 编排 + 子角色分工** | MultiAgent：Producer 调度 Screenwriter/Director/Review 等 | 长 prompt 拆分为角色，避免单 Agent 能力退化 |
| **Cast/Set/Prop 三支柱** | MediaNode 的 reference 连线 + Asset 复用 | 跨镜头角色/场景一致性 |
| **mood_anchor** | Skill 的 style_constraints.mood_anchor_template | 全局风格锚定 |
| **narrative_purpose** | 分镜节点的必填字段，Screenwriter 生成时强制要求 | 每个镜头有明确的叙事目的，避免无意义填充 |
| **shots_state.json** | Workspace State Summary | Agent 的全局状态感知 |
| **6 轴评审** | Skill 的 review_rubric | 结构化多维度质量评审 |
| **chain-DAG 并行渲染** | sequence 连线标记帧连续性 + 并行生成无依赖的镜头 | 渲染效率 |
| **4+2 Gate** | 简化为 2 Gate（分镜确认 + 成片预览） | 在成本不可逆节点确认 |
| **Last-frame continuation** | sequence edge + `use_prev_last_frame` 参数 | 镜头间的物理连续性 |
| **viewer.html 全流程溯源** | 画布就是溯源界面，每个节点可追溯到 Prompt/版本/评审 | 用户知道视频每一步如何生成 |

**不借鉴的部分**：
- spark-video 的文件系统存储模式（cast/folder = one state）→ ClipAnvil 用 DB 建模
- spark-video 的 CLI 交互 → ClipAnvil 是 Web 画布交互
- spark-video 的 VFX Review 独立 Gate → ClipAnvil 将 Preflight 检查集成在 Director 的提交前校验中，不单独设 Gate

## 6. Agent 完整工作流

### 6.1 端到端流程

以"30秒咖啡广告"为例，注意 Agent 调用的全部是生产级工具，没有任何画布操作：

```
用户: "帮我做一个30秒的咖啡新品广告，目标抖音，风格时尚简约，
       产品是燕麦拿铁，卖点是低糖健康"
  │
  ▼
━━━ 阶段1: 需求解析 ━━━━━━━━━━━━━━━━━━━━━━━━━
Producer:
  1. 匹配 Skill → marketing-ad-short
  2. get_production_state() → 检查是否有用户已上传的素材
  3. 解析需求 → Brief
  4. 对话回复: "收到！先规划分镜，请稍等..."
  │
  ▼
━━━ 阶段2: 分镜规划 ━━━━━━━━━━━━━━━━━━━━━━━━━
Producer → 调度 Screenwriter Sub-Agent:

  Screenwriter 调用一个工具完成全部分镜创建:

    create_storyboard({
      shots: [
        {title: "01-产品特写", duration: 3, narrative_purpose: "抓注意力",
         description: "微距拍摄燕麦拿铁表面拉花"},
        {title: "02-生活场景", duration: 7, narrative_purpose: "建立兴趣",
         description: "年轻女性在咖啡馆端起拿铁"},
        ...5个镜头
      ],
      transitions: [
        {from_index: 0, to_index: 1, type: "crossfade", duration: 0.5},
        {from_index: 1, to_index: 2, type: "cut"},
        ...
      ]
    })

  → 系统自动完成（Agent 不感知）:
    - 创建 5 个 media_node
    - 创建 4 条 sequence edge + 转场配置
    - 创建分组 "广告分镜"
    - 计算布局坐标，水平时间线排列
    - 广播 WebSocket 事件 → 画布出现 5 个分镜卡片

  → 返回给 Agent: {shot_ids: [s1,s2,s3,s4,s5]}
  │
  ▼
━━━ Gate 1: 分镜确认 ━━━━━━━━━━━━━━━━━━━━━━━━━
Producer 调用 request_gate("storyboard_review", "共5个镜头，总时长30s，预估¥3-5")

  对话面板显示确认卡片，画布上可见分镜排列。
  │
  ├── 用户点击"开始生成" → 进入阶段3
  ├── 用户说"第三个改成倒咖啡" → Agent 调用 modify_storyboard → 重新展示 Gate
  └── 用户点击画布上的卡片 → 查看详情 → 在对话中修改
  │
  ▼
━━━ 阶段3: 素材准备 + 视频生成 ━━━━━━━━━━━━━━━
Producer 并行调度:

  Art Asset Sub-Agent:
    generate_asset("image", "产品参考图", {
      prompt: "燕麦拿铁产品图，白色背景...",
      model: "flux-schnell"
    })
    → 系统自动: 创建素材节点 + 生成 + 画布显示

  Director Sub-Agent × 5（无帧依赖的可并行）:
    generate_shot(shot_id, {
      prompt: "微距拍摄燕麦拿铁拉花，咖啡液缓缓流动...",
      model: "qwen-vl-max",
      reference_inputs: [{asset_id: "产品主图ID", role: "product"}]
    })
    → 系统自动: 写prompt + 建依赖连线 + 提交生成 + 画布实时更新进度

  画布实时更新（系统自动，Agent 不操作）:
    - 生成中: 蓝色边框 + 进度条
    - 完成: 绿色边框 + 缩略图
    - 失败: 红色边框
  │
  ▼
━━━ 阶段4: 评审与重试 ━━━━━━━━━━━━━━━━━━━━━━━
  每个镜头完成后 Review Sub-Agent:
    review_shot(shot_id, rubric)
    → 通过: select_version(shot_id, version_id)
    → 不通过: retry_generation(shot_id, revised_prompt, reason)
    → 3次不通过: 对话中请求用户帮助
  │
  ▼
━━━ 阶段5: 拼接成片 ━━━━━━━━━━━━━━━━━━━━━━━━━
  Stitch Sub-Agent:
    stitch_final({
      shot_ids_in_order: [s1,s2,s3,s4,s5],
      bgm_asset_id: "BGM_ID"
    })
    → 系统自动: 创建成片节点 + 连接源分镜 + 执行拼接 + 画布显示
  │
  ▼
━━━ Gate 2: 成片预览 ━━━━━━━━━━━━━━━━━━━━━━━━━
  对话面板显示视频播放器 + 时间线
```

**整个流程中 Agent 调用的工具**：
- `get_production_state` × 1
- `create_storyboard` × 1
- `request_gate` × 2
- `generate_asset` × 若干
- `generate_shot` × 5
- `review_shot` × 5
- `select_version` × 5
- `stitch_final` × 1
- 总计 ~20 次工具调用

整条链路保持生产工具粒度，画布节点、依赖连线、状态更新和版本选择由系统内部业务命令完成。

### 6.2 用户中途干预

Agent 运行过程中，用户随时可以通过对话干预：

| 用户说 | Producer 的响应 |
|---|---|
| "停一下" / "暂停" | 暂停所有进行中的 Sub-Agent，不取消已提交的生成任务 |
| "继续" | 恢复暂停的流程 |
| "第二个镜头改成户外场景" | 调用 update_media_node 修改分镜描述 → mark_stale 下游 → 如果该镜头已生成则重新生成 |
| "换个模型试试" | 修改生成参数 → 重新 submit_generation |
| "这个产品图不好，我上传一张新的" | 等待用户上传 → 更新 asset → mark_stale 下游 |
| "第3和第4个镜头合并成一个" | 删除一个节点 + 更新另一个节点 + 调整连线 |
| "整体风格偏暖色调" | 更新 mood_anchor → 所有镜头 mark_stale → 提示用户确认重新生成范围 |

### 6.3 Agent 状态指示器

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

## 7. 画布自动布局

### 7.1 布局策略

Agent 模式下节点创建后必须自动布局。Studio 模式下用户可手动调整后调用自动整理。

**布局规则**：

```
┌─────────────────────────────────────────────────────┐
│ 📁 参考素材                                         │
│ [产品主图] [品牌Logo] [BGM] [参考图]                 │
├─────────────────────────────────────────────────────┤
│                                                     │
│ 📁 广告分镜                                         │
│ [01 特写] →→ [02 场景] →→ [03 卖点] →→ [04 情感] →→ [05 品牌] │
│   3s          7s           6s          9s          5s │
│                                                     │
│                    ↓ 依赖连线                         │
│              [产品主图] → [01 特写]                    │
│              [产品主图] → [02 场景]                    │
│                                                     │
├─────────────────────────────────────────────────────┤
│ [📼 成片 30s]                                       │
└─────────────────────────────────────────────────────┘
```

**布局算法**：
1. **分组整理**：同一 group 的节点排列在一起
2. **时间线排列**：有 sequence 连线的节点按顺序从左到右排列
3. **依赖分层**：被依赖的节点（素材）在上方，依赖者（分镜）在下方
4. **成片在底部**：最终产物节点在最下方
5. **节点间距**：水平间距 40px，垂直间距 60px（分组间 80px）

### 7.2 增量布局

当 Agent 添加新节点时，不要全部重排（会打乱用户已调整过的位置），而是只布局新增节点：
- 新节点插入到合理位置（时间线末尾、分组内部）
- 已有节点位置不变
- 全局自动整理只在用户主动调用 `auto_layout` 时执行

## 8. 主界面布局

### 8.1 Studio 模式布局

```
┌──────────────────────────────────────────────────────────────┐
│  影砧  ·  ☕ 咖啡广告项目    [Studio 模式 ▾]     ⌘K  导出  设置 │
├──────────┬───────────────────────────────────┬───────────────┤
│          │    浮动工具栏                       │              │
│ 🔍搜索    │  [↖选择|📝文本|🖼图片|🎬视频|🔊音频]  │  ⚙ 节点属性  │
│          │                                   │              │
│[类型筛选] │                                   │ [选中节点时   │
│          │         画布区域                    │  展示属性面板] │
│ 📁 素材   │      （React Flow 无限画布）            │              │
│  🖼 ...   │                                   │ [未选中时     │
│ 📁 分镜   │    [媒体节点卡片]                   │  面板隐藏]    │
│  🎬 ...   │    [连线 / 箭头]                   │              │
│ 📁 音频   │    [分组容器]                      │              │
│  🔊 ...   │                                   │              │
│          │  [缩放] [自动整理]                   │              │
└──────────┴───────────────────────────────────┴───────────────┘
```

### 8.2 Agent 模式布局

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
│  🖼 ...   │    [分组容器]                      │ Agent: 分镜   │
│ 📁 分镜   │                                   │ 规划好了...   │
│  🎬 ...   │    ┌──────────────────────┐       │              │
│ 📁 音频   │    │ 节点摘要卡（单击时）   │       │ [Gate 确认卡] │
│  🔊 ...   │    │ 🎬 03-卖点展示 · 6s   │       │              │
│          │    │ [在对话中引用]          │       │ ┌──────────┐ │
│          │    └──────────────────────┘       │ │ 对话输入   │ │
│          │                                   │ │ @引用 上传  │ │
│          │  [缩放] [自动整理]                  │ └──────────┘ │
└──────────┴───────────────────────────────────┴───────────────┘
```

**关键差异**：
- Agent 模式没有浮动工具栏（无创建工具）
- 右侧始终显示对话面板（不切换为属性面板）
- 单击节点在画布上显示摘要卡，不打开属性面板
- 对话输入框支持 @引用节点和上传文件

## 9. 关键机制修订

### 9.1 节点状态机（v2）

Agent 模式取消 `user_editing` 状态（用户不直接编辑）。Studio 模式保留但简化。

```
                Agent 模式状态机
Draft → Ready → Queued → Running → Succeeded
  ↑                                    │
  │                                    ▼
  └──────────── Stale ◄──── 上游变更传播
                                       │
                                 Failed / Rejected
```

```
                Studio 模式状态机
Draft → Ready → Queued → Running → Succeeded
  ↑       ↑                            │
  │       │                            ▼
  │   UserEditing                    Stale ◄── 上游变更传播
  │       │                            │
  │       ▼                            │
  └───── Ready                   Failed / Rejected
```

### 9.2 Gate 系统

保持 v1 的 2 Gate 设计，不增加。理由：
- 分镜确认前：Agent 只做了规划（创建节点和连线），零成本可撤回
- 分镜确认 → 成片之间：生成和评审过程，用户通过对话随时可干预
- 成片后：最终确认

### 9.3 Stale 传播

与 v1 一致，但在 Agent 模式下 Stale 的处理方式不同：

**v1（用户驱动）**：画布底部弹出影响分析条，用户决定是否重新生成
**v2（Agent 模式）**：Agent 在对话中报告影响范围，并给出建议

```
Agent: "您修改了产品主图，这会影响 3 个分镜节点（01 特写、02 场景、03 卖点）。
       需要重新生成这些镜头，预估费用 ¥2.1，耗时约 3 分钟。
       要重新生成吗？"
```

### 9.4 版本管理

与 v1 一致。Agent 模式下用户通过对话选择版本：

```
用户: "第二个镜头有没有其他版本？"
Agent: "第2个镜头有 3 个版本：
       - v1: 评分 6.8（面部表情不自然）
       - v2: 评分 7.5（构图偏左）
       - v3: 评分 8.2 ★当前选中
       要切换到其他版本吗？"
用户: "切到 v2 看看"
Agent: [调用 promote_version] "已切换到 v2。
       注意：成片需要重新拼接。要现在重新拼接吗？"
```

### 9.5 跨镜头一致性

借鉴 spark-video 的三支柱机制，在 ClipAnvil 中用 MediaNode + reference edge 实现：

**一致性资产**：角色参考图、场景参考图、风格参考图作为独立的 MediaNode 存在，通过 `reference` 类型的 edge 连接到使用它们的分镜节点。

**Prompt 构成规则**（Director Sub-Agent 遵循）：
```
最终 Prompt = 场景描述 + 动作/表情 + mood_anchor
参考图 = 角色参考图 + 场景参考图 + 道具参考图（按固定顺序）
```

**不在 Prompt 中描述外观**：角色的服装、发型等由参考图锁定，Prompt 只描述动作和表情。这是 spark-video 的铁律。

### 9.6 转场配置

与 v1 一致。转场是 sequence 类型 MediaEdge 的属性。

Agent 模式下 Director Sub-Agent 根据镜头语义自动选择转场类型。用户可通过对话修改：
```
用户: "第1和第2个镜头之间改成硬切"
Agent: [修改 edge transition] "已修改。成片需要重新拼接。"
```

## 10. 数据模型增补

### 10.1 workspace 表增加 mode 字段

```sql
ALTER TABLE workspace ADD COLUMN mode TEXT NOT NULL DEFAULT 'studio';
-- 值: 'studio' | 'agent'
```

### 10.2 media_node 表增加分镜相关字段

```sql
ALTER TABLE media_node ADD COLUMN duration_sec REAL;
-- 视频节点的目标时长（秒），分镜规划时设置

ALTER TABLE media_node ADD COLUMN narrative_purpose TEXT NOT NULL DEFAULT '';
-- 叙事目的（借鉴 spark-video 的 narrative_purpose），分镜节点必填

ALTER TABLE media_node ADD COLUMN use_prev_last_frame BOOLEAN NOT NULL DEFAULT false;
-- 是否使用前一个镜头的最后一帧作为本镜头的首帧（帧连续性）
```

### 10.3 新增 skill 表（MVP 后期）

```sql
CREATE TABLE skill (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL,
    config      JSONB NOT NULL,  -- Skill YAML 的 JSON 表示
    is_builtin  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

内置 Skill 在系统初始化时写入。`config` 存储 Skill 的完整定义（phases、review_rubric、gates、style_constraints 等）。

### 10.4 新增 agent_session 表

```sql
CREATE TABLE agent_session (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    skill_name    TEXT,
    brief         JSONB NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL DEFAULT 'running',
    -- 值: 'running' | 'paused' | 'waiting_gate' | 'completed' | 'failed'
    current_phase TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

记录 Agent 模式下的一次完整工作会话。一个 workspace 同时只能有一个 running 的 agent_session。

## 11. 实施路线

### M0: Studio 画布基础（当前阶段）

已有基础：React Flow 空画布、Go 后端骨架、PostgreSQL schema。

目标：用户可手动创建媒体节点、连线、分组，并提交生成。

交付：
- 自定义 MediaFlowNode（四种类型的节点卡片）
- 自定义 custom dependency edge（三种连线类型）
- 右键菜单创建节点
- 拖拽连线
- 左侧资源树
- 节点内联编辑 + 右侧属性面板
- 模型供应商集成（至少一个图片/视频生成 API）
- 版本列表与 winner 切换
- WebSocket 事件流
- 自动布局（基础版）

### M1: Agent 对话基础

目标：用户可通过对话让单 Agent 创建节点和生成内容。

交付：
- 对话面板 UI
- 单 Agent（未拆分 Sub-Agent）+ 画布原子工具
- Workspace State Summary 生成
- Agent 模式画布只读
- 基础 Gate（分镜确认 + 成片预览）
- Agent 状态指示器

### M2: MultiAgent + Skill

目标：Agent 可自动完成从需求到成片的全流程。

交付：
- Producer + Screenwriter + Director + Review + Stitch 五角色拆分
- 内置 Skill（marketing-ad-short）
- 评审与重试循环
- Stale 传播与增量重算
- Sub-Agent 并行生成
- 对话式修改（用户中途干预）

### M3: 一致性与质量

目标：提升生成视频的可用性和可控性。

交付：
- 跨镜头一致性机制（参考图 + mood_anchor + 帧连续）
- 多 Skill（product-demo、brand-story 等）
- 成本预估与预算管理
- 详细审计日志与操作回溯
- Studio ↔ Agent 模式切换

## 12. 开放问题

以下设计点在实施过程中可能需要进一步细化：

1. **生产翻译层的实现位置**：生产级工具是在 Go 后端实现（Agent 通过 HTTP 调用），还是在 Agent 运行时中作为工具包装层实现？前者 Agent 完全解耦，后者延迟更低。建议后端实现——Agent 可以是任何 LLM，工具就是 REST API。

2. **Agent 框架选型**：MultiAgent 的 Producer ↔ Sub-Agent 通信用什么协议？当前假设是后端进程内调度，但如果要支持不同 Sub-Agent 用不同模型（如 Review 用多模态模型），可能需要更灵活的调度机制。

3. **长对话上下文管理**：Agent 运行过程中会产生大量 PSS 增量更新和对话消息。需要设计上下文窗口策略——哪些信息保留、哪些可压缩、何时刷新 PSS 全量快照。

4. **Studio 模式的 AI 辅助**：Studio 模式是否也可以有轻量 AI 能力（如单节点的 Prompt 建议、自动参考图搜索），而不需要启动完整的 Agent 模式？如果需要，Studio 下的 AI 辅助也应该调用生产级工具，而非画布原子操作。

5. **Studio 模式的连线操作**：Studio 模式下用户手动创建连线走的是业务命令 API（create_media_edge），这和 Agent 的生产级工具是两套入口。是否需要统一？建议不统一——Studio 面向用户的是画布操作粒度，Agent 面向的是生产操作粒度，两者通过同一个业务 DB 保持一致。

6. **并发渲染限制**：多个 Director Sub-Agent 并行提交生成任务时，如何控制并发量？是否需要 workspace 级别的并发配额？

7. **Skill 编辑器**（MVP 后期）：用户自定义 Skill 需要什么样的编辑体验？
