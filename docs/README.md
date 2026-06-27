# ClipAnvil Docs

本目录按“当前开发入口”和“历史追溯材料”分层组织。

## 当前入口

| 目录 | 内容 | 何时阅读 |
|---|---|---|
| [`engineering/`](engineering/) | 架构、数据库、开发、部署、PR 流程、Agent MultiAgent 现状 | 对照代码实现、启动环境、修改后端或基础设施、发布 PR、规划 Agent 模式 |
| [`design/`](design/) | 产品交互、Studio/Agent、画布、视觉系统 | 修改前端体验、画布交互、产品流程 |
| [`milestones/`](milestones/) | 阶段路线图与已交付总结 | 查某个里程碑计划做什么、已经完成什么、还缺什么 |

## 执行记录与归档入口

| 目录 | 内容 | 说明 |
|---|---|---|
| [`archive/`](archive/) | 历史方案、研究稿、旧 spec/plan | 只用于追溯决策背景，不作为当前实现口径 |
| [`superpowers/`](superpowers/) | 当前仍可审阅的 spec / plan 执行记录 | 记录最近 Agent、画布和语义身份层设计；不替代代码事实 |
| [`archive/superpowers/`](archive/superpowers/) | 已归档的旧 spec / plan / graph 导出 | 只用于追溯历史执行过程 |

## 当前实现口径

- Studio M1.x-M5 已落地：认证、Workspace、Studio/Agent mode 分流、React Flow 画布、文本/图片/视频/音频/参考包节点、依赖连线、分组、资源树、上传资产、Dagre 自动布局、`/ws/canvas` 事件通道。
- M4/M5 生产链路已落地：`generation_job`、`artifact_version`、current winner、stale reason、Reference Pack、Prompt `@` 引用、模型能力、手动运行、版本查看/选择、调用记录和运行状态同步。
- 真实 provider 已接入到 Studio 手动运行：mock provider 保留本地测试；Volcengine/Doubao 文本、图片、视频模型可通过后端异步执行；TOS 用于供应商输入暂存；生成图片/视频会通过 sandbox 下载并存入 MinIO。
- 用户源素材节点已落地：手动文本素材和上传图片/视频/音频可作为普通依赖或参考包成员使用，但不展示模型运行入口。
- OpenSandbox 工作区沙箱基础已落地：本地 compose 启动 OpenSandbox Server，后端有 DB-first `workspace_sandbox` 绑定、sandbox exec、MinIO 预签名传输、artifact submit 和 sandbox-backed media ingest/FFmpeg 任务记录。
- Agent 三角色主链路已落地：`/ws/agent`、Agent 对话 API、消息/事件/任务持久化、Eino checkpoint/resume、Producer/Craftsman/Reviewer 原生 Eino tool loop、HITL 决策卡、创作事实源、RenderPlan、Worker 执行、Reviewer gate、Producer pending signal、语义键和 Agent Workbench 画布投影；MultiAgent 当前架构详见 [`engineering/agent-multiagent-architecture.md`](engineering/agent-multiagent-architecture.md)。
- 仍需后续完善：Studio/Agent 复制导入、Agent 长期 Skill 配置化、生产级并发/成本控制、外部任务队列、Campaign/Strategist、完整音频/TimelinePlan/商业级 Composer、Seedance 首尾帧/视频/音频参考深度支持、工具 schema 继续收紧和更多真实端到端回归。
- 数据库当前迁移到 `001_init_schema.sql` 到 `030_agent_semantic_identity.sql`，真实 schema 以 `apps/server/migrations/` 和 sqlc 生成代码为准。
