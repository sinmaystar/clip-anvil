# M1.x-D Studio 实时同步、自动布局与上传规格

**状态**：待实施
**前置**：M1.x-C 分组、资源树与属性面板完成
**目标**：补齐 Studio 编辑器的运行时能力：`/ws/canvas` 同步、前端 DAG 自动布局、拖拽上传到 MinIO 并自动创建媒体节点。

## 1. 当前事实基准

M1.x-C 完成后应具备：

- canvas 响应包含 `camera`、`nodes`、`edges`、`groups`。
- 前端可创建四种节点、dependency 连线、分组，并通过资源树和属性面板编辑。
- 后端已连接 PostgreSQL、Redis 和 MinIO；当前服务启动时要求这些中间件可用。
- 还没有 `/ws/canvas`。
- 还没有 `media_asset` 表和 `media_node.asset_id`。
- 前端 `apiFetch` 默认给有 body 的请求设置 `Content-Type: application/json`，上传 multipart 时必须绕开或调整这段逻辑。

本阶段完成后，Studio 作为手动 DAG 编辑器可用；仍不包含 AI 生成、GenerationJob、ArtifactVersion、Stale 传播或 Agent 对话。

## 2. 范围

### 2.1 包含

- 新增 `media_asset` 表和 `media_node.asset_id`。
- 新增上传 API：`POST /api/upload`。
- 前端拖拽文件到画布上传，成功后创建 image/video/audio 节点。
- 新增 `/ws/canvas`，广播节点、连线、分组变化。
- 前端 WebSocket 连接管理、断线重连、重连后 refetch canvas。
- 新增前端自动布局，使用 `@dagrejs/dagre` 计算 dependency DAG 坐标并持久化。
- 自动布局按钮和方向切换。

### 2.2 不包含

- `/ws/chat`。
- 多服务实例的可靠事件总线。
- 生成进度事件和 Job 事件。
- 视频/音频服务端缩略图生成。
- 资产版本管理。

## 3. 中间件前置

本阶段验收前必须先启动中间件：

```bash
docker compose -f deploy/docker-compose.yml up -d
```

后端启动依赖 PostgreSQL、Redis 和 MinIO 均可连接。若后续希望在无 MinIO 时也能跑纯后端测试，应另开技术债处理启动依赖解耦，不放入本阶段。

## 4. 上传与资产设计

### 4.1 数据库迁移

新增 `apps/server/migrations/004_add_assets.sql`：

```sql
-- +goose Up
CREATE TABLE media_asset (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    type          media_type NOT NULL,
    mime          TEXT NOT NULL,
    storage_url   TEXT NOT NULL,
    thumbnail_url TEXT,
    duration_ms   INT,
    size_bytes    BIGINT,
    metadata      JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_media_asset_workspace ON media_asset(workspace_id);

ALTER TABLE media_node
    ADD COLUMN asset_id UUID REFERENCES media_asset(id) ON DELETE SET NULL;

CREATE INDEX idx_media_node_asset ON media_node(asset_id);

-- +goose Down
DROP INDEX IF EXISTS idx_media_node_asset;
ALTER TABLE media_node DROP COLUMN IF EXISTS asset_id;
DROP INDEX IF EXISTS idx_media_asset_workspace;
DROP TABLE IF EXISTS media_asset;
```

`storage_url` 存储 MinIO 对象定位符，例如 `minio://workspace-{id}/assets/{asset_id}/{filename}`，不直接假设 bucket 是公网可访问。

### 4.2 sqlc 查询

新增 `apps/server/sqlc/queries/asset.sql`：

- `CreateMediaAsset`。
- `GetMediaAssetByID`。
- `ListMediaAssetsByWorkspace`。

扩展 node 查询：

- `UpdateMediaNodeAsset`。
- `CreateMediaNode` 和 `CreateMediaNodeWithID` 支持可选 `asset_id`。

canvas 查询返回 node 时需要带上可选 `asset_id` 和用于前端显示的 `thumbnail_url`。可以在 Go handler 层按 asset 组装 DTO，不要求 sqlc 生成复杂 join。

### 4.3 上传 API

新增：

```http
POST /api/upload
Content-Type: multipart/form-data
```

表单字段：

- `workspace_id`：必填。
- `file`：必填。

响应：

```json
{
  "id": "uuid",
  "workspace_id": "uuid",
  "type": "image",
  "mime": "image/png",
  "storage_url": "minio://workspace-id/assets/asset-id/file.png",
  "access_url": "presigned-url",
  "thumbnail_url": "presigned-url",
  "size_bytes": 12345
}
```

规则：

- workspace 必须属于当前用户。
- 文件大小上限 100MB。
- MIME 白名单：

| 类型 | MIME |
|---|---|
| image | `image/jpeg`, `image/png`, `image/webp`, `image/gif` |
| video | `video/mp4`, `video/quicktime`, `video/webm` |
| audio | `audio/mpeg`, `audio/wav`, `audio/aac`, `audio/ogg` |

- MinIO bucket 命名：`workspace-{workspace_id}`。
- 上传时若 bucket 不存在则创建。
- 图片类型 `thumbnail_url` 使用短期 presigned URL 指向原图。
- 视频/音频本阶段不生成缩略图，前端显示占位。

### 4.4 Node API 扩展

`POST /api/nodes` 新增可选字段：

```json
{
  "asset_id": "uuid"
}
```

规则：

- asset 必须属于同一 workspace。
- `node_type` 必须与 asset `type` 一致。
- Draft 手动节点可以没有 asset。

### 4.5 前端拖拽上传

新增 `apps/web/src/components/FileDropZone.tsx` 或在画布 host 层处理 drag/drop。

流程：

1. 用户拖拽文件到画布区域。
2. 画布出现半透明上传提示。
3. drop 后前端校验 MIME。
4. 使用 `FormData` 调用 `POST /api/upload`。
5. 上传成功后调用 `POST /api/nodes`，传入 `asset_id`、匹配的 `node_type`、鼠标释放位置。
6. image 节点显示 `thumbnail_url`。
7. video/audio 节点显示占位和文件名。

前端 `apiFetch` 需要支持 multipart：

- 如果 body 是 `FormData`，不要自动设置 `Content-Type`。
- 允许浏览器自动带 boundary。

多文件上传：

- 支持一次拖入多个文件。
- 每个文件独立上传和创建节点。
- 节点从 drop 坐标开始横向排列，间距 20px。
- 某个文件失败不影响其他文件继续处理。

## 5. WebSocket 设计

### 5.1 可靠性边界

本阶段的 `/ws/canvas` 是单服务实例内的实时同步能力，不承诺跨多副本、断线期间不丢事件或 exactly-once。

一致性策略：

- REST 写入成功后，后端广播事件。
- 前端收到事件后做幂等应用。
- WebSocket 断线重连成功后，前端 invalidate/refetch canvas 全量数据，以 DB 事实源修正本地状态。

这个边界与当前单机 Docker Compose 开发形态匹配，也为后续引入 Redis pub/sub 或持久事件日志留下空间。

### 5.2 后端实现

新增依赖：

```text
github.com/hertz-contrib/websocket
```

新增 `internal/api/ws_hub.go`：

- workspace 级连接池。
- `Register(workspaceID, conn)`。
- `Unregister(workspaceID, conn)`。
- `Broadcast(workspaceID, event)`。
- 心跳：服务端定期 ping，连接失效后移除。

连接端点：

```http
GET /ws/canvas?workspaceId=uuid&token=jwt
```

认证：

- 从 query param 读取 token。
- 使用现有 JWT secret 校验。
- 校验 workspace 归属。

事件：

| type | payload |
|---|---|
| `NodeCreated` | `{node}` |
| `NodeUpdated` | `{node}` 或 `{node_id, changes}` |
| `NodeDeleted` | `{node_id}` |
| `EdgeCreated` | `{edge}` |
| `EdgeDeleted` | `{edge_id}` |
| `GroupCreated` | `{group}` |
| `GroupUpdated` | `{group}` |
| `GroupDeleted` | `{group_id}` |

建议 `NodeUpdated` 直接发送完整 node，减少前端局部字段映射错误。

### 5.3 前端实现

新增 `apps/web/src/lib/ws.ts`：

- `connectCanvasSocket({ workspaceId, token, onEvent, onStatusChange })`。
- 指数退避重连：1s、2s、4s、8s、最大 30s。
- 页面卸载时关闭连接。
- 重连成功后触发 canvas query invalidate。

事件处理：

- `NodeCreated`：node 已存在则 update，不存在则 create。
- `NodeUpdated`：更新 query cache 和 node data。
- `NodeDeleted`：node 不存在则跳过。
- `EdgeCreated`：edge custom edge 已存在则 update，不存在则 create。
- `GroupCreated`：group container 已存在则 update，不存在则 create。
- `GroupDeleted`：只删除 group container，不删除 member MediaFlowNode。

连接状态展示在 Studio 顶部或侧栏底部，文案保持短：已连接、重连中、离线。

## 6. 自动布局设计

### 6.1 依赖

新增前端依赖：

```bash
pnpm --filter @clip-anvil/web add @dagrejs/dagre
```

### 6.2 算法范围

自动布局只使用 dependency edges。

流程：

1. 从 canvas payload 读取 nodes 和 dependency edges。
2. 构造 dagre graph。
3. 每个节点使用实际 `canvas_w/canvas_h`。
4. 计算新坐标。
5. `editor.updateNodes()` 更新 MediaFlowNode。
6. 根据组内节点包围盒更新 group container node。
7. `PATCH /api/nodes/batch-position` 持久化节点坐标。

本阶段不依赖 dagre compound graph。分组聚集通过后处理实现：同组节点布局后，group container 根据成员包围盒重算位置和尺寸。

### 6.3 交互

画布底部或顶部增加紧凑工具：

- 自动整理按钮。
- 方向选择：`LR` 和 `TB`。

参数：

| 参数 | 值 |
|---|---:|
| ranksep | 80 |
| nodesep | 40 |
| marginx | 20 |
| marginy | 20 |

若当前图存在环，说明后端约束失效或历史数据异常。前端显示错误并不执行布局。

## 7. 测试与验收

### 7.1 后端验收

```bash
docker compose -f deploy/docker-compose.yml up -d
make migrate-up
make sqlc-generate
make server-test
```

测试覆盖：

- 上传图片成功创建 asset。
- 非媒体 MIME 返回 `400`。
- 超过 100MB 返回 `413`。
- 不同 workspace 的 asset 不能绑定到 node。
- 创建带 `asset_id` 的 image node 成功。
- node_type 与 asset type 不一致返回 `400`。
- WebSocket 无 token 连接被拒。
- WebSocket 带合法 token 连接成功。
- 创建节点后连接能收到 `NodeCreated`。

### 7.2 前端验收

```bash
pnpm --filter @clip-anvil/web... build
```

浏览器验收：

- 打开两个标签页，同一 workspace 中创建节点，另一标签页能看到变化。
- 断网或手动关闭 WS 后重连，前端 refetch canvas 并恢复一致状态。
- 拖拽 JPG 到画布，上传成功后创建 image node 并显示图片预览。
- 拖拽多个文件，成功项都创建节点，失败项显示错误。
- 点击自动整理后节点按 dependency 层级排列。
- 刷新后自动布局坐标保持。
- 分组容器在自动布局后重新包裹成员节点。

## 8. 完成条件

- Studio 支持本地媒体素材导入。
- Studio 支持单服务实例内多标签页同步。
- Studio 支持用户主动自动整理 dependency DAG。
- 断线或 WS 事件丢失后，刷新或重连 refetch 能恢复 DB 事实源状态。
- 不引入 AI 生成、Agent 对话或版本管理。

## 9. 关联文档

- [M1.x-C Studio 分组、资源树与属性面板](./2026-06-13-m1x-c-studio-groups-panels-design.md)
- [画布设计](../../../design/canvas.md)
- [Studio 模式设计](../../../design/studio-mode.md)
- [数据库设计](../../../engineering/database.md)
- [架构文档](../../../engineering/architecture.md)
