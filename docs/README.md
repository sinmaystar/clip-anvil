# ClipAnvil Docs

本目录按“当前开发入口”和“历史追溯材料”分层组织。

## 当前入口

| 目录 | 内容 | 何时阅读 |
|---|---|---|
| [`engineering/`](engineering/) | 架构、数据库、开发、部署、PR 流程 | 对照代码实现、启动环境、修改后端或基础设施、发布 PR |
| [`design/`](design/) | 产品交互、Studio/Agent、画布、视觉系统 | 修改前端体验、画布交互、产品流程 |
| [`milestones/`](milestones/) | 阶段路线图与已交付总结 | 查某个里程碑计划做什么、已经完成什么、还缺什么 |

## 执行记录与归档入口

| 目录 | 内容 | 说明 |
|---|---|---|
| [`archive/`](archive/) | 历史方案、研究稿、旧 spec/plan | 只用于追溯决策背景，不作为当前实现口径 |
| [`archive/superpowers/specs/`](archive/superpowers/specs/) | M3-M6 执行过的阶段设计稿 | 作为阶段执行记录阅读，当前实现口径仍以代码、迁移和 `engineering/` 为准 |
| [`archive/superpowers/plans/`](archive/superpowers/plans/) | M3-M6 执行过的阶段实施计划 | 作为任务拆分和验收记录阅读，不替代当前事实文档 |

## 当前实现口径

- Studio M1.x-M5 已落地：认证、Workspace、Studio/Agent mode 分流、React Flow 画布、文本/图片/视频/音频/参考包节点、依赖连线、分组、资源树、上传资产、Dagre 自动布局、`/ws/canvas` 事件通道。
- M4/M5 生产链路已落地：`generation_job`、`artifact_version`、current winner、stale reason、Reference Pack、Prompt `@` 引用、模型能力、手动运行、版本查看/选择、调用记录和运行状态同步。
- 真实 provider 已接入到 Studio 手动运行：mock provider 保留本地测试；Volcengine/Doubao 文本、图片、视频模型可通过后端异步执行；TOS 用于供应商输入暂存；生成图片/视频会通过 sandbox 下载并存入 MinIO。
- 用户源素材节点已落地：手动文本素材和上传图片/视频/音频可作为普通依赖或参考包成员使用，但不展示模型运行入口。
- OpenSandbox 工作区沙箱基础已落地：本地 compose 启动 OpenSandbox Server，后端有 DB-first `workspace_sandbox` 绑定、sandbox exec、MinIO 预签名传输、artifact submit 和 sandbox-backed media ingest/FFmpeg 任务记录。
- M6 Agent 阶段性闭环已落地：`/ws/agent`、Agent 对话 API、消息/事件/任务持久化、Eino checkpoint/resume、Producer 工具调用、HITL 决策卡、Storyboard/PSS、Craftsman/Worker 预览生成、Reviewer 重试调度、Composer 成片合成和 Agent 只读画布。
- 仍需后续完善：Studio/Agent 复制导入、Agent 长期记忆/Skill 配置化、生产级并发/成本控制、完整音频生成链路和更多真实端到端回归。
- 数据库当前迁移到 `001_init_schema.sql` 到 `022_add_doubao_seed_thinking_text_models.sql`，真实 schema 以 `apps/server/migrations/` 和 sqlc 生成代码为准。
