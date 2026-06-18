# M2 OpenSandbox 工作区沙箱 — 里程碑

**状态**：基础链路已部分落地（2026-06-17）
**目标**：为 Agent 模式和共享生产底座提供 workspace 级长生命周期沙箱，支持执行命令、预置素材、提交产物，并以数据库作为 sandbox 绑定事实源。

参考文档：

- [设计规格](../superpowers/specs/2026-06-17-opensandbox-workspace-sandbox-design.md)
- [实施计划](../superpowers/plans/2026-06-17-opensandbox-workspace-sandbox.md)

## Codex Goal 建议

按阶段完成 M2 OpenSandbox 工作区沙箱，每完成一个阶段都运行对应验收命令并汇报结果；未通过验收不得进入下一阶段。当前代码已经合入 1-5 阶段的主要实现，仍需以端到端 smoke 和 Agent 集成继续收口。

## 阶段里程碑

| 阶段 | 里程碑 | 可验收标准 |
|---|---|---|
| 1. 基础设施与配置 | OpenSandbox Server 能随本地开发栈启动，后端能读取 sandbox 配置，sandbox 镜像可构建。 | ✅ 已实现；仍需在新环境 smoke 时确认本机 Docker socket 路径。 |
| 2. DB-first Sandbox Manager | 新增 `workspace_sandbox`，后端能以数据库为事实源创建、复用、替换 workspace sandbox。 | ✅ 已实现；`005_add_workspace_sandbox.sql`、sqlc 查询和 manager 单测已在代码中。 |
| 3. Workspace 文件预置 | sandbox 内稳定拥有 `/workspace` 目录结构，后端能把素材预置到 `/workspace/assets` 并生成 manifest。 | ✅ 已实现基础 helper；后续需要接入真实 Agent 素材选择流程。 |
| 4. `sandbox_exec` 工具 | Agent 可在 `/workspace` 内同步执行受限 shell 命令，并获得结构化结果。 | ✅ 已暴露 `POST /api/workspaces/:id/sandbox/exec`；后续 Agent 工具层必须通过 Sandbox Job Service 记录执行事实。 |
| 5. `submit_artifact` 工具 | sandbox output 能经 Go 后端发布为 MinIO asset 和 Studio canvas node。 | ✅ 已暴露 `POST /api/workspaces/:id/sandbox/artifacts`，并通过 MinIO 预签名 PUT 避免后端转发大文件。 |
| 6. 端到端 Smoke | 完成从 workspace 创建、素材上传、sandbox 执行、产物提交、canvas 可见、后端重启恢复的完整闭环。 | ⏳ 待补齐验收记录；需要真实本地环境验证 canvas 可见、WebSocket 更新和后端重启恢复。 |
| 7. Sandbox Job Service | 应用和 Agent 使用高层 sandbox job，不直接管理 sandbox id、volume、输入下载、输出上传或生命周期。 | ✅ M4.S 已加入生产底座；FFmpeg、Agent shell、Composer 等不可预测资源消耗都必须走该服务。 |

## 完成定义

- 六个阶段全部通过各自验收标准，尤其是端到端 smoke 和重启恢复。
- `workspace_sandbox` 是 sandbox 绑定关系的唯一事实源，内存不作为恢复或路由依据。
- Sandbox 服务自行处理 workspace sandbox 会话、持久化 volume、生命周期复用、输入输出传输和执行记录；应用和 Agent 只提交高层执行请求。
- 沙箱内没有 MinIO、数据库、JWT 或模型供应商凭证。
- Agent 第一版可以保留 `sandbox_exec` 和 `submit_artifact` 两个用户/工具入口，但内部执行必须落 `sandbox_job`。
- 输入素材和输出产物都必须经 Go 后端跨越沙箱边界。
