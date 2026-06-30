# ClipAnvil Docs

本目录按“当前事实入口、完成状态摘要、历史追溯材料”分层组织。

## 当前入口

| 目录 | 内容 | 何时阅读 |
|---|---|---|
| [`engineering/`](engineering/) | 架构、数据库、开发、部署、PR 流程、Agent MultiAgent 现状 | 对照代码实现、启动环境、修改后端或基础设施、发布 PR、规划 Agent 模式 |
| [`design/`](design/) | 产品交互、Studio/Agent、画布、视觉系统 | 修改前端体验、画布交互、产品流程 |
| [`milestones/`](milestones/) | 阶段路线图与已交付总结 | 查某个里程碑已经完成什么、还缺什么、验收依据是什么 |

## 验收记录与归档入口

| 目录 | 内容 | 说明 |
|---|---|---|
| [`archive/`](archive/) | 历史方案、研究稿、旧 spec/plan | 只用于追溯决策背景，不作为当前实现口径 |
| [`superpowers/`](superpowers/) | 最近的验收报告和临时执行记录入口 | 当前没有活跃 spec/plan；已完成的 spec/plan 已归档 |
| [`archive/superpowers/`](archive/superpowers/) | 已完成或过期的 spec / plan / graph 导出 | 只用于追溯历史执行过程，不作为当前任务入口 |

## 当前实现口径

- Studio M1.x-M5 已落地：认证、Workspace、Studio/Agent mode 分流、React Flow 画布、文本/图片/视频/音频/参考包节点、依赖连线、分组、资源树、上传资产、Dagre 自动布局、`/ws/canvas` 事件通道。
- M4/M5 生产链路已落地：`generation_job`、`artifact_version`、current winner、stale reason、Reference Pack、Prompt `@` 引用、模型能力、手动运行、版本查看/选择、调用记录和运行状态同步。
- 真实 provider 已接入到 Studio 手动运行：mock provider 保留本地测试；Volcengine/Doubao 文本、图片、视频模型可通过后端异步执行；TOS 用于供应商输入暂存；生成图片/视频会通过 sandbox 下载并存入 MinIO。
- 用户源素材节点已落地：手动文本素材和上传图片/视频/音频可作为普通依赖或参考包成员使用，但不展示模型运行入口。
- OpenSandbox 工作区沙箱基础已落地：本地 compose 启动 OpenSandbox Server，后端有 DB-first `workspace_sandbox` 绑定、sandbox exec、MinIO 预签名传输、artifact submit 和 sandbox-backed media ingest/FFmpeg 任务记录。
- Agent 三角色主链路已落地：`/ws/agent`、Agent 对话 API、消息/事件/任务持久化、Eino checkpoint/resume、Producer/Craftsman/Reviewer 原生 Eino tool loop、HITL 决策卡、创作事实源、Reference Video Analysis、RenderPlan、Worker 执行、Reviewer gate、Producer pending signal、语义键和 Agent Workbench 画布投影；Doubao 视频理解接入已具备服务接口和 mock/单测覆盖，真实环境需配置后验证；MultiAgent 当前架构详见 [`engineering/agent-multiagent-architecture.md`](engineering/agent-multiagent-architecture.md)。
- 仍需后续完善：Studio/Agent 复制导入、Agent 长期 Skill 配置化、生产级并发/成本控制、外部任务队列、Campaign/Strategist、商业级 Composer 深化、Seedance 首尾帧/编辑/延长/桥接深度支持、工具 schema 继续收紧和更多真实端到端回归。
- 数据库当前迁移到 `001_init_schema.sql` 到 `038_reference_video_analysis.sql`，真实 schema 以 `apps/server/migrations/` 和 sqlc 生成代码为准。

## 维护规则

- 当前行为先查代码、迁移、sqlc 生成代码和 `docs/engineering/`。
- 已完成的阶段 spec / plan 放入 `docs/archive/superpowers/`，不要留在 `docs/superpowers/` 作为活跃任务入口。
- `docs/milestones/` 保留完成状态、验收命令和重要遗留项；不要塞入长篇执行步骤。
- 新的设计或实施计划可以先放入 `docs/superpowers/specs/` 或 `docs/superpowers/plans/`；阶段完成后再归档，并更新本 README。
