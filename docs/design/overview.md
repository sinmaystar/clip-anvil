# ClipAnvil 整体业务交互设计

## 1. 产品定位

一张以 tldraw 为交互底座的多媒体生产画布，一套以业务 DAG 为事实源的生成系统，以及一个以 Skill 为角色分工的可视化 Agent。

用户看到分镜板，Agent 看到工作流。复杂度藏在后端，简洁感留给用户。

## 2. 核心原则

1. **业务 DB 为唯一事实源，tldraw 只做投影** — Workspace、媒体资源、依赖关系、生成任务、版本、评审结果存储在业务数据库。tldraw store 只保存渲染所需的业务字段映射（nodeId、缩略图、状态），画布坐标直接存在业务表的 `canvas_x/y/w/h` 字段中。Agent 后台执行时画布可以不打开；画布损坏不丢失业务数据。

2. **Agent 不知道画布的存在** — Agent 调用生产级工具（create_storyboard、generate_shot），生产翻译层自动将其翻译为画布状态（创建节点、建连线、计算布局）。Agent 眼中只有分镜、素材、进度，没有节点、连线、分组。

3. **不弹框，不打断画布流** — 节点详情和编辑在画布附近的节点浮层完成，不使用常驻右侧 Inspector，也不把用户带回传统管理系统布局。

4. **Studio 与 Agent 明确分离** — 两个模式分别对应不同的画布权限和交互入口，消除并发编辑的状态冲突。切换时数据完全保留。

## 3. 四层架构

```
┌──────────────────────────────────────────────────────┐
│                    用户界面层                          │
│   tldraw 画布（可视化投影）+ 对话面板（Agent 交互）      │
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
| 用户界面层 | tldraw shape、画布坐标、视觉样式、事件渲染 | Agent 内部编排逻辑、生产翻译规则 |
| 业务层 | 完整数据模型、DAG、版本、任务状态 | tldraw、Agent 提示词 |
| 生产翻译层 | 生产操作 ↔ 业务命令的映射规则 | tldraw、Agent 提示词 |
| Agent 层 | 分镜、素材、生成、评审、拼接 | 节点、连线、分组、画布坐标、布局 |

**为什么需要生产翻译层**：一个生产操作 = 多个业务命令。例如 `create_storyboard(5个分镜)` 在业务层需要：创建 5 个 media_node + 4 条 sequence edge + 1 个 group + 计算布局坐标 + 写入 canvas_x/y。如果让 Agent 逐一调用这些命令，Agent 就变成了画布操作员，认知负荷高且容易出错。

## 4. 双模式概要

| | Studio 模式 | Agent 模式 |
|---|---|---|
| **定位** | 用户主导的自由创作空间 | Agent 主导的自动化生产，用户观察与对话式修改 |
| **画布** | 完全可编辑（创建、拖拽、连线、删除） | **只读**（平移、缩放、点击查看详情） |
| **右侧面板** | 无常驻右侧 Inspector，选中节点时使用画布浮层 | 对话面板（始终显示） |
| **左侧资源树** | 完全可操作 | 只读浏览 |
| **工具栏** | 显示创建工具 | 仅视图控制（缩放、自动整理） |
| **谁创建内容** | 用户 | Agent |
| **数据通路** | 用户操作 → REST API → DB → 画布 | 用户对话 → Agent → 生产工具 → DB → 事件流 → 画布 |

**模式切换（目标态）**：Workspace 级别设置，建议保存在 `workspace.settings.mode` 或后续显式 `workspace.mode` 字段。当前代码尚未提供 Studio/Agent 模式切换 UI，所有已落地画布能力默认属于 Studio 模式。

- Studio → Agent：保留画布上已有内容，Agent 可识别并复用
- Agent → Studio：保留 Agent 创建的所有内容，用户获得完全编辑权限
- Agent 运行中不可切换到 Studio（需先暂停或完成）

详细设计：[Studio 模式](studio-mode.md) · [Agent 模式](agent-mode.md)

## 5. 核心实体

完整数据库 schema 见 [../engineering/database.md](../engineering/database.md)。

| 实体 | 关键字段 | 说明 |
|---|---|---|
| Workspace | id, name, owner, mode, settings | 顶层项目容器 |
| CanvasDocument | camera_x/y/zoom, layout_version | 画布视口状态 |
| MediaAsset | type, mime, storage_url | 文件级资产（图片/视频/音频/文本） |
| MediaNode | node_type, status, prompt, canvas_x/y/w/h | 画布上的业务节点 |
| MediaGroup | name, sort_order | 扁平分组 |
| MediaEdge | from/to_node_id, edge_type | 节点间依赖关系 |
| GenerationJob | provider, model, prompt, status | 一次生成任务 |
| ArtifactVersion | version_no, winner, review_score, input_hash | 产物版本 |
| ReviewRecord | axes, score, verdict | 评审记录 |
| AgentStep | step_type, input, output | Agent 操作审计 |
| AgentSession | skill_name, status, current_phase | Agent 工作会话 |
| Skill | name, config, is_builtin | 领域知识模块 |

### 5.1 Studio 当前连线语义

| edgeType | 语义 | 示例 | 画布视觉 |
|---|---|---|---|
| `dependency` | A 的输出作为 B 的输入，B 依赖 A | 产品图 → 视频节点 | SVG 曲线 + 流动动效 + 箭头 |

Studio 模式当前只向用户暴露 dependency。数据库中的 `edge_type` 仍保留 `reference` / `sequence` 兼容和未来 Agent/分镜扩展口径，但它们不是 M2a Studio 用户交互范围。

### 5.2 节点状态机

**Agent 模式**（无 UserEditing，画布只读）：

```
Draft → Ready → Queued → Running → Succeeded
  ↑                                    │
  └──────────── Stale ◄──── 上游变更传播
                                 Failed / Rejected
```

**Studio 模式**（含 UserEditing）：

```
Draft → Ready → Queued → Running → Succeeded
  ↑       ↑                            │
  │   UserEditing                    Stale ◄── 上游变更传播
  │       │                            │
  └───── Ready                   Failed / Rejected
```

| 状态 | 含义 | 画布视觉 |
|---|---|---|
| Draft | 空白节点，未填入内容 | 虚线边框 |
| Ready | 内容就绪，可以提交生成 | 灰色实线 |
| Queued | 已排队等待 | 灰色实线 + ⏳ |
| Running | 正在生成 | 蓝色边框 + 光晕 + 进度条 |
| Succeeded | 生成完成且通过评审 | 绿色边框 + 评分 |
| Failed | 生成失败或评审不通过 | 红色边框 |
| Stale | 上游依赖已变更，当前产物过期 | 黄色虚线边框 |
| UserEditing | 用户正在手动编辑（仅 Studio） | 橙色边框 |

## 6. 关键机制

### 6.1 版本管理与 Winner

每次生成产生一个 ArtifactVersion，一个节点可有多个版本。`winner` 标记当前选中版本。

1. 提交生成 → 创建 GenerationJob → 完成后创建 ArtifactVersion
2. 自动评审（ReviewRecord）给出分数和维度评价
3. 分数达标 → 自动设为 winner
4. 分数不达标且 Agent 模式 → Agent 改写 Prompt 重试（最多 3 次）
5. 3 次都不通过 → 对话中请求用户帮助

**inputHash 机制**：ArtifactVersion 记录 `input_hash`（上游依赖 winner + Prompt + 模型参数的哈希）。上游变更时，通过 inputHash 判断当前版本是否仍有效——如果上游实际输出未变，跳过不必要的重新生成。

### 6.2 Stale 传播与增量重算

上游节点发生实质变更时，下游节点按 DAG 依赖传播标记为 Stale。

**传播规则**：
1. 节点 A 变更 → 遍历 `dependency` 类型的出边（不含 reference 和 sequence）
2. 对下游节点 B，比较 inputHash
3. inputHash 失效 → B 标记 Stale → 继续向 B 的下游传播
4. inputHash 有效（上游实际输出未变）→ 停止传播

**处理方式因模式而异**：
- **Studio**：画布底部弹出影响分析条，展示影响范围和预估费用，用户决定是否重新生成。详见 [Studio 模式 §Stale 处理](studio-mode.md)
- **Agent**：Agent 在对话中报告影响范围并给出建议。详见 [Agent 模式 §Stale 处理](agent-mode.md)

### 6.3 Gate 系统

Agent 在**成本不可逆**的节点暂停等待用户确认。只设 2 个 Gate：

| Gate | 触发时机 | 确认内容 |
|---|---|---|
| 分镜确认 | 分镜规划完成后，开始消耗生成资源前 | 分镜数量、内容、时长分配、预估费用 |
| 成片预览 | 所有镜头生成完毕、拼接完成后 | 整体效果、节奏、转场 |

**为什么只有 2 个 Gate**：分镜确认前 Agent 只做了规划（零成本可撤回）；确认到成片之间用户可通过对话随时干预；过多 Gate 会让自动化退化成表单流程。

### 6.4 跨镜头一致性

借鉴 spark-video 三支柱机制：

- **一致性资产**：角色参考图、场景参考图、风格参考图作为独立 MediaNode，通过 reference edge 连接到使用它们的分镜
- **Prompt 构成规则**：最终 Prompt = 场景描述 + 动作/表情 + mood_anchor；角色外观由参考图锁定，Prompt 不描述外观
- **帧连续性**：sequence edge + `use_prev_last_frame` 参数，前一镜头的最后一帧作为下一镜头的首帧

### 6.5 转场配置

转场是 sequence 类型 MediaEdge 的属性，不是独立节点。

| type | 说明 | 默认时长 |
|---|---|---|
| `cut` | 硬切 | 0s |
| `crossfade` | 交叉淡入淡出 | 0.5s |
| `dissolve` | 溶解过渡 | 1.0s |
| `wipe` | 擦除过渡 | 0.5s |

修改转场不会让镜头节点 Stale（不影响单镜头内容），但会让成片节点 Stale（需重新拼接）。

## 7. 实施路线

### M0: 工程基建（已完成）

目标：建立 monorepo、前后端骨架、Docker Compose 中间件和基础开发命令。

交付：
- `apps/web` Vite + React + TypeScript 基础应用
- `apps/server` Go + Hertz 基础服务
- PostgreSQL / Redis / MinIO / Nginx compose 配置
- 根工作区、lint、hooks、基础文档

### M1: Studio 画布基础（当前已落地）

目标：打通注册登录 → Workspace → 文本节点画布 → 坐标/Camera 持久化的前后端链路。

交付：
- JWT 注册登录和路由守卫
- Workspace 列表、创建、详情入口
- tldraw 自定义 MediaShape（M1 仅 text 节点）
- 画布右键菜单创建文本节点
- 节点拖拽位置持久化
- 节点标题/Prompt 单击后在节点下方编辑并自动保存
- 可折叠左侧栏、明亮/暗夜外观切换

### M1.x: Studio 增量（核心已落地）

目标：把 M1 的文本节点画布扩展为完整 Studio DAG 编辑器。

交付：
- image / video / audio 节点类型（已落地）
- dependency 连线、SVG overlay 投影和 DAG 环检测（已落地）
- 分组（MediaGroup）和左侧资源树（已落地）
- 节点编辑浮层基础信息和 Prompt 自动保存（已落地）；高级模型参数待生成系统落地
- `/ws/canvas` 事件流（已落地）
- 拖拽上传资产和 MinIO 存储（已落地）
- Dagre 自动布局，并根据左侧浮层展开/收起状态避让安全区（已落地）
- 生成任务、版本列表与 winner 切换（未落地，后续阶段）

### M2: OpenSandbox 工作区沙箱（基础已部分落地）

目标：为 Agent 模式提供 workspace 级长生命周期沙箱，支持命令执行、素材预置、MinIO 传输和产物提交。

交付：
- OpenSandbox Server 本地 compose 服务（已落地）
- `workspace_sandbox` 数据库绑定（已落地）
- sandbox exec、workspace 文件布局和素材 manifest（已落地）
- MinIO 预签名下载/上传和 artifact submit（已落地）
- 与 Agent 生产工具的端到端编排（未落地）

### M3: Workspace 模式入口（已完成）

目标：从入口、路由和权限上区分 Studio Workspace 与 Agent Workspace。

交付：
- `workspace.mode`
- Studio / Agent 创建入口
- `/studio` 与 `/agent` 路由分流
- Agent Workspace 只读画布
- Agent Workspace 普通画布写接口后端拒绝

### M4: 共享生产底座（已完成）

目标：建立 Studio 和 Agent 共用的生产数据和执行链路。

交付：
- GenerationIntent
- Provider Bridge
- Sandbox Job Service
- `generation_job` / `artifact_version` / current winner
- capability validation、failure records、retry chain
- input hash、stale propagation、stale reasons
- Reference Pack data foundation
- sandbox-backed internal FFmpeg operations
- Production Read API for M5

### M5: Studio 专业手动模式（待实施）

目标：让专业用户可手动创建、引用、运行、查看版本和处理 stale。

交付：
- 富属性面板
- Prompt `@`
- Reference Pack UI
- 手动运行和版本管理
- stale 可视化和手动重跑
- 模型能力驱动的模型选择

### M6: Agent 自动生产模式（待实施）

目标：Producer / Craftsman / Worker / Composer 复用 M4 生产底座完成分镜到成片。

交付：
- Agent runtime 存储、Eino checkpoint、PSS 和 Memory
- Producer 对话面板与 HITL 决策卡片
- Storyboard / shot / shot dependency
- Craftsman + Worker 生成与评审重试
- Composer sandbox-backed 成片合成
- Studio / Agent 复制导入，而不是原地无缝切换

## 8. 开放问题

1. **生产翻译层的实现位置**：生产级工具是在 Go 后端实现（Agent 通过 HTTP 调用），还是在 Agent 运行时中作为工具包装层？建议后端实现——Agent 可以是任何 LLM，工具就是 REST API。

2. **Agent 框架选型**：MultiAgent 的 Producer ↔ Sub-Agent 通信协议。如果要支持不同 Sub-Agent 用不同模型（如 Review 用多模态模型），需要灵活的调度机制。

3. **长对话上下文管理**：Agent 运行过程中的 PSS 增量更新和对话消息。需要设计上下文窗口策略——哪些保留、哪些压缩、何时刷新全量 PSS。

4. **Studio 模式的 AI 辅助**：Studio 模式是否也有轻量 AI 能力（如 Prompt 建议、自动参考图搜索），而不需要启动完整 Agent？

5. **并发渲染限制**：多个 Director Sub-Agent 并行提交生成时，如何控制并发量？是否需要 workspace 级别的并发配额？

## 相关文档

- [画布设计和交互方案](canvas.md) — tldraw 投影、视觉规格、数据通路
- [Studio 模式设计方案](studio-mode.md) — 用户主导的创作交互
- [Agent 模式设计方案](agent-mode.md) — Agent 驱动的生产交互
- [前端视觉和系统设计方案](frontend.md) - 前端视觉和系统设计方案
- [数据库设计](../engineering/database.md) — 完整 schema、迁移、查询
- [技术架构](../engineering/architecture.md) — 技术选型、项目结构、部署
