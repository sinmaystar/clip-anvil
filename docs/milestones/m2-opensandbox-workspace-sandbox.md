# M2 OpenSandbox 工作区沙箱 — 里程碑

**状态**：待实施
**目标**：为 Agent 模式提供 workspace 级长生命周期沙箱，支持执行命令、预置素材、提交产物，并以数据库作为 sandbox 绑定事实源。

参考文档：

- [设计规格](../superpowers/specs/2026-06-17-opensandbox-workspace-sandbox-design.md)
- [实施计划](../superpowers/plans/2026-06-17-opensandbox-workspace-sandbox.md)

## Codex Goal 建议

按阶段完成 M2 OpenSandbox 工作区沙箱，每完成一个阶段都运行对应验收命令并汇报结果；未通过验收不得进入下一阶段。

## 阶段里程碑

| 阶段 | 里程碑 | 可验收标准 |
|---|---|---|
| 1. 基础设施与配置 | OpenSandbox Server 能随本地开发栈启动，后端能读取 sandbox 配置，sandbox 镜像可构建。 | `docker compose -f deploy/docker-compose.yml up -d` 成功；`curl http://127.0.0.1:8080/health` 健康；`docker build -t clipanvil-sandbox:dev sandbox-image` 成功；`make server-build` 成功。 |
| 2. DB-first Sandbox Manager | 新增 `workspace_sandbox`，后端能以数据库为事实源创建、复用、替换 workspace sandbox。 | `make migrate-up` 成功；`make sqlc-generate` 成功；manager 单测覆盖创建、复用、替换、失败状态、并发保护；`make server-test` 成功。 |
| 3. Workspace 文件预置 | sandbox 内稳定拥有 `/workspace` 目录结构，后端能把 MinIO 素材预置到 `/workspace/assets` 并生成 manifest。 | 重复执行预置保持幂等；`manifest.json` 不含凭证；sandbox 内能看到预置素材；相关单测和 `make server-test` 成功。 |
| 4. `sandbox_exec` 工具 | Agent 可在 `/workspace` 内同步执行受限 shell 命令，并获得结构化结果。 | 默认 cwd 为 `/workspace`；支持超时、非零退出、输出截断；拒绝越界 cwd；相关单测和 `make server-test` 成功。 |
| 5. `submit_artifact` 工具 | sandbox output 能经 Go 后端发布为 MinIO asset 和 Studio canvas node。 | 只允许提交 `/workspace/output` 下文件；拒绝目录、缺失文件、超大文件、不支持 MIME；创建/更新 `media_asset` 与 `media_node`；广播 WebSocket；`make server-test` 成功。 |
| 6. 端到端 Smoke | 完成从 workspace 创建、素材上传、sandbox 执行、产物提交、canvas 可见、后端重启恢复的完整闭环。 | smoke 生成的 asset 可通过 canvas API 看到；已打开 Studio canvas 收到 WebSocket 更新；`workspace_sandbox` 保存稳定 volume；后端重启后不依赖内存恢复；现有 upload/canvas 测试仍通过。 |

## 完成定义

- 六个阶段全部通过各自验收标准。
- `workspace_sandbox` 是 sandbox 绑定关系的唯一事实源，内存不作为恢复或路由依据。
- 沙箱内没有 MinIO、数据库、JWT 或模型供应商凭证。
- Agent 第一版只需要 `sandbox_exec` 和 `submit_artifact` 两个 sandbox 工具。
- 输入素材和输出产物都必须经 Go 后端跨越沙箱边界。
