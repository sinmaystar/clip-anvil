# OpenSandbox 工作区沙箱设计

**状态**：待评审
**日期**：2026-06-17
**范围**：为 Agent 驱动的媒体生产增加 workspace 级执行沙箱。
**核心决策**：基于 OpenSandbox 实现沙箱基础设施；ClipAnvil 后端负责业务状态、权限、存储和产物发布。

## 1. 背景

ClipAnvil 需要一个沙箱，让 Agent 可以执行 shell 命令、生成脚本、调用媒体工具，并把最终文件发布回平台。第一版必须尽量简单：不自建容器管理，不在沙箱容器内运行 ClipAnvil API Server，也不向 Agent 暴露不必要的低层文件工具。

OpenSandbox 提供了合适的基础设施边界：

- Lifecycle Server：创建、暂停、恢复、续期、删除 sandbox。
- Docker 和 Kubernetes runtime backend。
- 自动注入 `execd`，用于命令执行和文件操作。
- Go SDK 覆盖 lifecycle、execd、egress 等接口。
- `pvc` volume 模型：本地 Docker runtime 下映射为 Docker named volume，Kubernetes runtime 下映射为 PVC。

ClipAnvil 只使用 OpenSandbox 做生命周期管理和沙箱内执行。Go 后端仍然是 workspace 权限、素材元数据、MinIO 操作、media node 和 WebSocket 事件的业务边界。

参考资料：

- OpenSandbox 仓库：https://github.com/opensandbox-group/OpenSandbox
- OpenSandbox 架构文档：https://raw.githubusercontent.com/opensandbox-group/OpenSandbox/main/docs/architecture.md
- OpenSandbox Server 文档：https://raw.githubusercontent.com/opensandbox-group/OpenSandbox/main/server/README.md
- OpenSandbox Go SDK README：https://raw.githubusercontent.com/opensandbox-group/OpenSandbox/main/sdks/sandbox/go/README.md

## 2. 目标

- 为每个 workspace 提供一台类似 VM 的长生命周期执行环境。
- 第一版只向 Agent 暴露两个工具：`sandbox_exec` 和 `submit_artifact`。
- 通过稳定的 workspace volume 保留沙箱文件，使 sandbox container 可替换。
- 沙箱内不放任何平台凭证。
- 数据库是 workspace 与 sandbox 绑定关系的唯一事实源。
- 产物必须通过 Go 后端进入 MinIO、`media_asset`、`media_node` 和 WebSocket 事件。
- 本地开发保持当前 Docker Compose 中间件 + 宿主机后端 + Vite 的拓扑。

## 3. 非目标

- 不在沙箱内实现 ClipAnvil API Server。
- 不把 MinIO、PostgreSQL、Redis、JWT、模型供应商等凭证放入沙箱。
- 第一版不做前端终端 UI。
- 第一版不做文件浏览器。
- 第一版不接入 OpenSandbox Credential Vault。
- 第一版不承诺严格网络 egress 隔离。
- 第一版不实现 generation job、artifact version、review record，除非后续里程碑单独加入。

## 4. 总体架构

```text
Go Backend (Hertz)
  |
  | OpenSandbox Go SDK / REST
  v
OpenSandbox Server
  |
  | Docker API，本地开发
  v
Sandbox Container
  |-- execd，由 OpenSandbox 注入
  |-- clipanvil-sandbox image
  `-- /workspace，挂载 workspace volume
```

三层职责：

- ClipAnvil Go 后端：Agent 工具、workspace 权限、数据库状态、MinIO 上传下载、asset/node 创建、WebSocket 广播。
- OpenSandbox Server：sandbox 生命周期、Docker/Kubernetes runtime、资源限制、endpoint 解析、`execd` 注入。
- Sandbox Container：命令执行、脚本、中间文件和 `/workspace/output` 下的最终文件。

Agent 感知上像是在操作一台项目 VM；平台边界仍然收口在 Go 后端。

## 5. Workspace 生命周期模型

第一版采用方案 A：一个 workspace 对应一个逻辑沙箱环境。

```text
workspace.id
  |-- database binding: workspace_sandbox
  |-- workspace volume: sandbox-ws-{workspaceID}
  `-- current OpenSandbox sandbox_id
```

workspace volume 是稳定资源；sandbox container 是可替换资源。

销毁 sandbox container 不删除 workspace volume。后续用同名 volume 重新创建 sandbox 时，`/workspace` 下的文件可以恢复。

## 6. 数据库作为事实源

`workspace_sandbox` 是 workspace 与 sandbox 绑定关系的唯一事实源。内存只能用于短生命周期 mutex 或可丢弃的 SDK client 对象，不能用于恢复、路由、权限判断，也不能决定某个 workspace 当前属于哪个 sandbox。

新增迁移：

```sql
CREATE TABLE workspace_sandbox (
    workspace_id UUID PRIMARY KEY REFERENCES workspace(id) ON DELETE CASCADE,
    sandbox_id TEXT,
    volume_name TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('creating', 'running', 'unhealthy', 'terminated')),
    last_health_check_at TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_workspace_sandbox_status ON workspace_sandbox(status);
```

第一版使用 `TEXT + CHECK`，不使用 PostgreSQL enum。原因是沙箱生命周期还会演进，`CHECK` 约束调整成本更低。

### 6.1 EnsureSandbox 流程

`EnsureSandbox(ctx, workspaceID)` 以 DB 行为控制点。

```text
EnsureSandbox(workspaceID)
  1. 校验 workspace 属于当前账号，或来自可信 Agent context。
  2. 如果 workspace_sandbox 记录不存在，先创建记录。
  3. 使用 SELECT ... FOR UPDATE 锁定该 workspace_sandbox 行。
  4. 如果 status=running 且 sandbox_id 存在，调用 OpenSandbox get/ping 验证是否健康。
  5. 健康则更新 last_seen_at，返回 sandbox_id。
  6. 不存在、过期、失败或不健康，则更新 status=creating。
  7. 使用同一个 volume_name 创建新的 OpenSandbox sandbox，并挂载到 /workspace。
  8. 创建成功后更新 sandbox_id、status=running、last_seen_at，并清空 error_message。
```

DB 行锁是主要并发控制，能覆盖多后端实例。进程内 mutex 可以减少重复工作，但正确性不能依赖内存。

### 6.2 生命周期状态

ClipAnvil 只映射业务需要的四个状态：

| 状态 | 含义 |
|---|---|
| `creating` | 后端正在创建或替换 sandbox container。 |
| `running` | 当前 `sandbox_id` 存在，并通过最近一次健康检查。 |
| `unhealthy` | 绑定关系存在，但当前 sandbox 健康检查或初始化失败。 |
| `terminated` | 当前 sandbox 被主动删除或已过期；volume 可以仍然存在。 |

OpenSandbox 更细的 `Pending`、`Running`、`Paused`、`Failed`、`Terminated` 等状态不泄露到 Agent 工具层。

## 7. Agent 工具

第一版只暴露两个 sandbox 相关工具：

```text
sandbox_exec(command, cwd?, timeout_seconds?)
submit_artifact(path, title?, node_id?, media_type?)
```

`workspace_id` 从当前 Agent session 或可信 request context 推导，Agent 不能手动传入。

### 7.1 sandbox_exec

用途：在 workspace sandbox 内执行受限 shell 命令。

流程：

```text
Agent 调用 sandbox_exec
  -> Go 后端校验 workspace context
  -> EnsureSandbox(workspaceID)
  -> EnsureWorkspaceLayout(workspaceID, sandboxID)
  -> 通过 OpenSandbox execd 执行命令
  -> 返回结构化结果
```

默认值和限制：

- 默认 `cwd`：`/workspace`。
- 默认超时：120 秒。
- 最大超时：1800 秒。
- shell 形式：`bash -lc`。
- stdout/stderr 独立限制大小，例如各 64 KiB。
- 输出被截断时返回 `truncated=true`。
- 第一版不允许 Agent 传自定义环境变量。
- 后端可以注入非敏感变量，例如 `CLIPANVIL_WORKSPACE_ID`。

响应示例：

```json
{
  "exit_code": 0,
  "stdout": "ok",
  "stderr": "",
  "duration_ms": 1234,
  "truncated": false
}
```

MVP 使用同步命令执行。后台命令、流式终端、PTY WebSocket 后续再扩展。

### 7.2 submit_artifact

用途：把沙箱内生成的文件发布为 ClipAnvil 资产和画布节点。

流程：

```text
Agent 调用 submit_artifact("/workspace/output/final.mp4")
  -> Go 后端校验 workspace context
  -> 校验 path 必须在 /workspace/output 下
  -> 通过 OpenSandbox 从 sandbox 下载文件
  -> 后端 sniff MIME，并推导 media_type
  -> 上传 MinIO
  -> 创建 media_asset
  -> 更新已有 media_node 或创建新的 agent media_node
  -> 广播 WebSocket 事件
  -> 返回 asset_id、node_id、access_url
```

安全规则：

- `path` 必须解析为 `/workspace/output` 下的普通文件。
- 拒绝 `..`、空路径、逃逸 output 的相对路径和目录提交。
- 如果 OpenSandbox 文件元数据能识别软链逃逸，则直接拒绝；如果不能，下载前用受控命令在 sandbox 内解析真实路径。
- 后端重新 sniff MIME，不信任 Agent 传入的 `media_type`。
- 强制文件大小上限。MVP 建议产物上限 500 MiB，与现有用户上传的 100 MiB 分开。
- MinIO 凭证只存在 Go 后端。

节点行为：

- 如果传入 `node_id`，该 node 必须属于同一 workspace，且 node type 必须与提交产物类型匹配。
- 如果不传 `node_id`，创建一个 `source='agent'`、`status='succeeded'`、绑定 `asset_id` 的 `media_node`。
- 如果当前 insert query 还不能设置 `source`，实现时用最小 schema/query 扩展支持 agent-owned node。

广播事件：

- `AssetCreated`：新 asset payload。
- `NodeUpdated`：提交到已有 node。
- `NodeCreated`：新建 agent node。

## 8. 工作目录约定

sandbox workspace 目录固定为：

```text
/workspace/
  assets/
  manifest.json
  scripts/
  tmp/
  output/
```

目录初始化和输入素材预置由 Go 后端负责。

### 8.1 EnsureWorkspaceLayout

每次 `EnsureSandbox` 后，或第一次 `sandbox_exec` 前，应确保目录存在：

```text
mkdir -p /workspace/assets /workspace/scripts /workspace/tmp /workspace/output
```

该操作必须幂等。

### 8.2 PrepareWorkspaceFiles

输入素材流转：

```text
media_asset rows
  -> Go 后端从 MinIO 下载对象
  -> 通过 OpenSandbox file upload 写入 /workspace/assets
  -> Go 后端写入 /workspace/manifest.json
  -> Go 后端写入 /workspace/assets/index.json
```

manifest 只包含元数据，不包含凭证。

示例：

```json
{
  "workspace_id": "uuid",
  "assets_dir": "/workspace/assets",
  "output_dir": "/workspace/output",
  "assets": [
    {
      "id": "uuid",
      "type": "image",
      "mime": "image/png",
      "path": "/workspace/assets/uuid-product.png",
      "title": "产品主图"
    }
  ]
}
```

MVP 预置策略：

- Agent 开始一次生产任务前调用 `PrepareWorkspaceFiles`。
- 如有必要，直接重新上传当前 workspace 的所有资产。
- 后续如性能不足，再加 asset hash 或 mtime 去重。

## 9. OpenSandbox 部署

本地开发保持现有形态：

```text
docker compose:
  postgres
  redis
  minio
  nginx
  opensandbox-server

host:
  make server-dev
  pnpm --filter @clip-anvil/web dev
```

OpenSandbox Server 是内部基础设施服务，不通过 ClipAnvil NGINX 对外暴露。Go 后端直接访问它。

### 9.1 后端配置

扩展 `apps/server/config.yaml`：

```yaml
sandbox:
  endpoint: "http://localhost:8080/v1"
  api_key: "clipanvil-dev-sandbox-key"
  image: "clipanvil-sandbox:dev"
  timeout_seconds: 1800
  workdir: "/workspace"
  use_server_proxy: true
  resource_limits:
    cpu: "2"
    memory: "4Gi"
```

如果未来 Go 后端也进入 Docker Compose，`endpoint` 改为 `http://opensandbox-server:8080/v1`。

### 9.2 OpenSandbox Server 配置

新增：

```text
deploy/config/sandbox.toml
```

要求：

- 启用 API key 认证。
- 本地使用 Docker runtime。
- 默认使用 bridge 或 compose network，不使用 host network。
- 支持资源限制。
- 使用 OpenSandbox 默认或推荐的 `execd` image。
- OpenSandbox 可以维护自己的 server metadata store；ClipAnvil 的业务事实源仍是 `workspace_sandbox`。

### 9.3 Sandbox 镜像

新增：

```text
sandbox-image/Dockerfile
```

MVP 包含：

- `bash`
- `coreutils`
- `curl`
- `ca-certificates`
- `ffmpeg`
- `python3`
- `python3-pip`
- `imagemagick`
- `jq`
- `file`
- `fonts-noto-cjk`
- `fonts-noto-color-emoji`

镜像内不写入任何 secret。

### 9.4 Volume 模型

创建 sandbox 时挂载：

```text
type: pvc
name: sandbox-ws-{workspaceID}
mountPath: /workspace
```

Docker runtime 下对应 Docker named volume；Kubernetes runtime 下对应 PVC。ClipAnvil 产品和文档中统一称为 workspace volume，避免绑定某一种 runtime。

## 10. 安全模型

第一版安全边界：

- OpenSandbox API key 必须启用。
- 只有 Go 后端知道 OpenSandbox API key。
- 只有 Go 后端知道 MinIO 凭证。
- sandbox container 内没有平台凭证。
- `sandbox_exec` 有超时和输出大小限制。
- 资源限制目标：2 CPU、4 GiB memory、PID limit。
- 在 OpenSandbox Docker runtime 支持的前提下裁剪危险 capability。
- 在 runtime 支持的前提下启用 `no_new_privileges`。
- sandbox image 内禁用 ImageMagick URL/HTTP/HTTPS coder。
- `submit_artifact` 校验路径、文件类型、MIME、大小和 workspace 所属关系。

网络 egress 分阶段处理：

- MVP 不承诺严格 outbound network isolation。
- 后续版本可以接 OpenSandbox network policy 和 egress sidecar。
- 未来 strict mode 可以 default deny，仅允许任务需要的域名。

这个取舍让第一版能实际跑媒体工具和必要依赖安装，同时通过“无凭证 + 后端代理 + 产物校验”守住平台边界。

## 11. 后端模块结构

新增：

```text
apps/server/internal/sandbox/
  config.go
  store.go
  manager.go
  client.go
  workspace.go
  exec.go
  artifact.go
  paths.go
```

职责：

| 文件 | 职责 |
|---|---|
| `config.go` | sandbox 配置结构和校验。 |
| `store.go` | `workspace_sandbox` query 封装和行锁 helper。 |
| `manager.go` | `EnsureSandbox`、健康检查、reset flow、生命周期编排。 |
| `client.go` | OpenSandbox SDK 适配层。 |
| `workspace.go` | 目录初始化、manifest 生成、素材预置。 |
| `exec.go` | `sandbox_exec` 命令执行和结果结构。 |
| `artifact.go` | `submit_artifact`、MinIO 上传、DB 写入、WebSocket 事件。 |
| `paths.go` | 路径归一化、output path 校验、safe filename。 |

OpenSandbox SDK 细节必须隔离在 `client.go`。业务代码依赖 ClipAnvil 自己的小接口：

```go
type Client interface {
    Create(ctx context.Context, req CreateRequest) (SandboxInfo, error)
    Get(ctx context.Context, sandboxID string) (SandboxInfo, error)
    Ping(ctx context.Context, sandboxID string) error
    Exec(ctx context.Context, sandboxID string, req ExecRequest) (ExecResult, error)
    Upload(ctx context.Context, sandboxID string, path string, r io.Reader) error
    Download(ctx context.Context, sandboxID string, path string) (io.ReadCloser, FileInfo, error)
    Delete(ctx context.Context, sandboxID string) error
}
```

## 12. 可选人类调试 API

以下 API 不作为 Agent 核心生产工具，只用于开发和运维：

```http
GET  /api/workspaces/:id/sandbox/status
POST /api/workspaces/:id/sandbox/reset
```

规则：

- workspace 必须属于当前账号。
- `reset` 删除或终止当前 sandbox container，但保留 workspace volume。
- 未来可以增加清空 workspace volume 的危险操作，但必须要求用户明确确认，且不放入 MVP。

## 13. 分阶段实施与验收标准

### 阶段 1：基础设施与配置

目标：让 OpenSandbox 随现有本地开发栈一起启动。

交付物：

- `sandbox-image/Dockerfile`。
- `deploy/config/sandbox.toml`。
- `deploy/docker-compose.yml` 增加 `opensandbox-server`。
- 后端 config 增加 `sandbox` 段。
- 本地开发文档补充 OpenSandbox health check 和端口。

验收方式：

- 执行 `docker compose -f deploy/docker-compose.yml up -d` 后，PostgreSQL、Redis、MinIO、NGINX、OpenSandbox Server 都能启动。
- `curl http://127.0.0.1:8080/health` 返回 OpenSandbox Server 健康响应。
- 本地能构建 `clipanvil-sandbox:dev` 镜像。
- `make server-build` 通过，说明新 config struct 未破坏后端构建。
- 通过 `http://localhost` 访问应用时，OpenSandbox 不被 NGINX 对外暴露。

通过标准：

- 开发者可以用一条 compose 命令启动现有中间件和 OpenSandbox。
- Go 后端能读取 sandbox 配置。
- 现有上传、画布、WebSocket 行为不受影响。

### 阶段 2：DB-first Sandbox Manager

目标：基于数据库创建、恢复和替换 workspace sandbox。

交付物：

- `005_add_workspace_sandbox.sql`。
- `apps/server/sqlc/queries/sandbox.sql`。
- `internal/sandbox/store.go`。
- `internal/sandbox/manager.go`。
- `internal/sandbox/client.go`，并提供 fake client 用于测试。

验收方式：

- `make migrate-up` 创建 `workspace_sandbox`。
- `make sqlc-generate` 更新生成代码。
- 单测覆盖：
  - 缺失记录时创建 `volume_name=sandbox-ws-{workspaceID}`。
  - 健康的 running sandbox 会被复用。
  - OpenSandbox 中 sandbox 不存在或不健康时，使用同名 volume 创建替换 sandbox。
  - 创建成功后 DB 行更新为 `running`。
  - 创建失败后 DB 行更新为 `unhealthy` 并写入 `error_message`。
  - 并发调用不会为同一 workspace 创建多个有效 sandbox 绑定。
- `make server-test` 通过。

通过标准：

- 重启 Go 后端后，不依赖内存即可从 PostgreSQL 找回 workspace 与 sandbox 的绑定关系。
- 内存 map 不是恢复、路由或权限判断所必需。
- 同一个 workspace 不会被设计性地分配多个 workspace volume。

### 阶段 3：Workspace 文件预置

目标：让 `/workspace` 结构稳定，并通过 Go 后端预置 ClipAnvil 素材。

交付物：

- `internal/sandbox/workspace.go`。
- 幂等初始化 `/workspace/assets`、`/workspace/scripts`、`/workspace/tmp`、`/workspace/output`。
- 生成 `manifest.json` 和 `assets/index.json`。
- 从 MinIO 下载 `media_asset` 并通过 OpenSandbox 上传到 `/workspace/assets`。

验收方式：

- 连续调用两次 `PrepareWorkspaceFiles`，目录结构和 manifest shape 保持稳定。
- 给定一个已上传的 `media_asset`，sandbox 中能看到 `/workspace/assets` 下的对应文件。
- `manifest.json` 包含 workspace ID、asset metadata 和 sandbox-local path。
- `manifest.json` 不包含 MinIO 凭证、presigned URL 或其他平台 secret。
- 单测覆盖 safe asset filename 和 manifest 生成。
- 集成 smoke 可以通过 OpenSandbox 执行 `ls /workspace/assets` 并看到预置素材。

通过标准：

- Agent 可以通过 `/workspace/manifest.json` 发现项目输入素材。
- 所有输入素材传输都由 Go 后端代理。
- 重复预置不会破坏已有 `/workspace/output` 文件。

### 阶段 4：sandbox_exec 工具

目标：让 Agent 可以在 workspace sandbox 内执行受限命令。

交付物：

- `internal/sandbox/exec.go`。
- `sandbox_exec` Agent tool definition。
- timeout、cwd、shell、stdout/stderr cap 和结构化结果。
- 基于 fake sandbox client 的单测。

验收方式：

- `sandbox_exec("pwd")` 默认返回 `/workspace`。
- `sandbox_exec("echo ok")` 返回 exit code `0`，stdout 包含 `ok`。
- 非零退出命令返回 exit code 和 stderr，但不导致后端崩溃。
- 超时命令返回结构化 timeout 错误。
- 大 stdout/stderr 被截断，并返回 `truncated=true`。
- 除非后端设计明确允许，否则 `cwd` 超出 `/workspace` 会被拒绝。
- `make server-test` 通过。

通过标准：

- Agent 可以在 `/workspace/scripts` 创建脚本，在 `/workspace/tmp` 写中间文件，在 `/workspace/output` 写最终文件。
- 命令失败能以结构化结果返回给 Agent。
- 后端日志包含 workspace ID 和 sandbox ID，便于调试。

### 阶段 5：submit_artifact 工具

目标：把 sandbox output 发布为 ClipAnvil asset 和 canvas node。

交付物：

- `internal/sandbox/artifact.go`。
- `internal/sandbox/paths.go`。
- `submit_artifact` Agent tool definition。
- MinIO 对象路径：`workspace-{id}/artifacts/{timestamp}/{filename}`。
- 写入 `media_asset`，并更新或创建 `media_node`。
- 广播 canvas WebSocket 事件。

验收方式：

- 提交 `/workspace/output/result.png` 创建 type=`image` 的 `media_asset`。
- 提交 `/workspace/output/result.mp4` 创建 type=`video` 的 `media_asset`。
- 提交 `/workspace/output` 之外的路径会被拒绝。
- 提交目录、不存在文件、超大文件、不支持 MIME 会被拒绝。
- 传入 `node_id` 时，只能更新同 workspace 的 node。
- 不传 `node_id` 时，创建 agent-owned media node 并绑定 asset。
- 响应包含 `asset_id`、`node_id` 和短期 access URL。
- WebSocket 客户端收到 `AssetCreated` 和 `NodeCreated` 或 `NodeUpdated`。
- `make server-test` 通过。

通过标准：

- sandbox 内生成的文件能进入 MinIO、`media_asset` 和 Studio canvas。
- sandbox 全程拿不到 MinIO 凭证。
- 从 sandbox output 写入 ClipAnvil 状态的唯一通道是 Go 后端。

### 阶段 6：端到端 Smoke

目标：证明 MVP 链路能从 workspace 走到 sandbox，再回到 canvas。

场景：

1. 启动本地中间件和 OpenSandbox。
2. 执行迁移。
3. 启动 Go 后端。
4. 创建 workspace。
5. 通过现有上传 API 上传一张图片。
6. Ensure workspace sandbox。
7. Prepare workspace files。
8. 用 `sandbox_exec` 生成 `/workspace/output/result.png` 或 `/workspace/output/result.mp4`。
9. 调用 `submit_artifact`。
10. 拉取 canvas，确认生成 node 存在。
11. 重启 Go 后端。
12. 再次调用 `EnsureSandbox`，确认从 `workspace_sandbox` 恢复；如原 sandbox 不可用，则用同名 volume 创建替换 sandbox。

验收方式：

- smoke 生成的 asset 可以通过 canvas API 看到。
- 已打开的 Studio canvas 能通过 WebSocket 看到节点更新。
- `workspace_sandbox` 保存当前 sandbox ID 和稳定 volume name。
- Go 后端重启后，不依赖内存也能继续操作该 workspace sandbox。
- 现有 upload 和 canvas 测试仍然通过。

通过标准：

- sandbox MVP 能完成一个真实的简单 Agent 媒体任务。
- 恢复行为符合 DB-first 设计。
- 实现可以进入 Agent Mode 集成阶段。

## 14. 测试策略

分三层测试：

- 单测：使用 fake OpenSandbox client 覆盖 manager、路径校验、manifest 生成、exec 结果结构、artifact 发布决策。
- 后端集成测试：覆盖 DB query、MinIO 上传、node 更新、WebSocket 事件调用，尽量复用现有测试模式。
- 真实 OpenSandbox smoke：覆盖容器生命周期、volume 挂载和 execd 文件/命令能力。

实施过程中预期使用的命令：

```bash
make migrate-up
make sqlc-generate
make server-build
make server-test
pnpm --filter @clip-anvil/web... build
```

只有修改 shared API types 或前端行为的阶段才要求前端 build。

## 15. 待定问题

- `media_node.source='agent'` 是通过新增 query 支持，还是扩展现有 create query。
- `submit_artifact` 第一版是否为视频生成 `thumbnail_url`，或延后到视频缩略图里程碑。
- sandbox status/reset 调试 API 是否随第一版一起交付，还是等 Agent 集成时再开放。
- 严格 OpenSandbox egress policy 是否应作为生产部署前的单独安全里程碑。

## 16. 设计结论

ClipAnvil 使用 OpenSandbox 作为沙箱基础设施层，业务状态仍由 Go 后端和 PostgreSQL 管理。每个 workspace 拥有一个逻辑沙箱环境，由稳定 workspace volume 承载文件状态；当前 OpenSandbox sandbox container 可以替换。`workspace_sandbox` 表是当前 sandbox ID 和 volume name 的事实源。Agent 只能通过 `sandbox_exec` 执行命令，通过 `submit_artifact` 发布产物。输入素材和输出产物都必须经 Go 后端跨越沙箱边界，由后端执行权限校验、路径校验、MIME/大小检查、MinIO 写入、数据库更新和 WebSocket 广播。
