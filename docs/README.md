# ClipAnvil Docs

本目录按“当前开发入口”和“历史追溯材料”分层组织。

## 当前入口

| 目录 | 内容 | 何时阅读 |
|---|---|---|
| [`engineering/`](engineering/) | 架构、数据库、开发、部署 | 对照代码实现、启动环境、修改后端或基础设施 |
| [`design/`](design/) | 产品交互、Studio/Agent、画布、视觉系统 | 修改前端体验、画布交互、产品流程 |
| [`milestones/`](milestones/) | 已交付阶段总结 | 查某个里程碑已经完成什么、还缺什么 |

## 归档入口

| 目录 | 内容 | 说明 |
|---|---|---|
| [`archive/`](archive/) | 历史方案、研究稿、旧 spec/plan | 只用于追溯决策背景，不作为当前实现口径 |
| [`archive/superpowers/specs/`](archive/superpowers/specs/) | 已执行过的阶段设计稿 | 从根目录移入归档，避免和当前事实文档混在一起 |
| [`archive/superpowers/plans/`](archive/superpowers/plans/) | 已执行过的阶段实施计划 | 从根目录移入归档，后续新计划完成后也归档到这里 |

## 当前实现口径

- Studio M1.x 已落地：认证、Workspace、tldraw 画布、文本/图片/视频/音频节点、依赖连线、分组、资源树、属性面板、上传资产、Dagre 自动布局、`/ws/canvas` 事件通道。
- OpenSandbox 工作区沙箱基础已部分落地：本地 compose 启动 OpenSandbox Server，后端有 DB-first `workspace_sandbox` 绑定、sandbox exec、MinIO 预签名传输和 artifact 提交流程。
- 尚未落地：Agent 对话模式、生成任务/版本/评审、`/ws/chat`、reference/sequence 的完整交互配置、模型供应商调用。
- 数据库当前迁移到 `001_init_schema.sql` 到 `005_add_workspace_sandbox.sql`，真实 schema 以迁移和 sqlc 生成代码为准。
