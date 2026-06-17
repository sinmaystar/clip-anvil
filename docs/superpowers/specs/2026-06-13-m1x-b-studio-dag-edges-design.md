# M1.x-B Studio 连线与 DAG 规格

**状态**：待实施
**前置**：M1.x-A 多类型节点完成
**目标**：在现有业务 DB 事实源模式下新增 MediaEdge，支持 dependency 连线、后端 DAG 环检测和前端 ArrowShape 投影。

## 1. 当前事实基准

M1.x-A 完成后应具备：

- `media_node` 支持 text/image/video/audio 四种节点。
- `GET /api/workspaces/:id/canvas` 仍只返回 `camera` 和 `nodes`。
- 前端只注册 `MediaShapeUtil`，尚未注册或持久化 ArrowShape。
- 当前没有 `media_edge` 表，没有 edge API，也没有 WebSocket。

本阶段只做 dependency 连线。`reference` 和 `sequence` 的视觉与 API 开放留到后续生成和编排阶段。

## 2. 范围

### 2.1 包含

- 新增 `media_edge` 表和 sqlc 查询。
- 新增 `POST /api/edges` 和 `DELETE /api/edges/:id`。
- `GET /api/workspaces/:id/canvas` 返回 `edges`。
- 后端创建 dependency edge 前做归属校验、重复校验、自连接校验和 DAG 环检测。
- 前端把 edge 投影为 tldraw ArrowShape + binding。
- 前端支持从节点输出端口到输入端口建立 dependency 连线。
- 前端支持选中连线后删除并同步后端。

### 2.2 不包含

- reference/sequence 连线类型的用户入口。
- 转场配置。
- Stale 传播。
- 右侧属性面板里的连线详情，留给 M1.x-C。
- WebSocket 事件推送，留给 M1.x-D。

## 3. 数据库设计

新增 `apps/server/migrations/002_add_edges.sql`。

建议使用数据库 enum，与 `database-design.md` 目标态保持一致：

```sql
-- +goose Up
CREATE TYPE edge_type AS ENUM ('dependency', 'reference', 'sequence');
CREATE TYPE transition_type AS ENUM ('cut', 'crossfade', 'dissolve', 'wipe');

CREATE TABLE media_edge (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    from_node_id        UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    to_node_id          UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    edge_type           edge_type NOT NULL DEFAULT 'dependency',
    transition_type     transition_type,
    transition_duration REAL,
    source              TEXT NOT NULL DEFAULT 'user',
    metadata            JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT no_self_loop CHECK (from_node_id != to_node_id),
    CONSTRAINT unique_edge UNIQUE (from_node_id, to_node_id, edge_type)
);

CREATE INDEX idx_media_edge_workspace ON media_edge(workspace_id);
CREATE INDEX idx_media_edge_from ON media_edge(from_node_id);
CREATE INDEX idx_media_edge_to ON media_edge(to_node_id);

-- +goose Down
DROP INDEX IF EXISTS idx_media_edge_to;
DROP INDEX IF EXISTS idx_media_edge_from;
DROP INDEX IF EXISTS idx_media_edge_workspace;
DROP TABLE IF EXISTS media_edge;
DROP TYPE IF EXISTS transition_type;
DROP TYPE IF EXISTS edge_type;
```

虽然 schema 预留三种 edge type，本阶段 API 固定创建 `dependency`。

## 4. sqlc 查询

新增 `apps/server/sqlc/queries/edge.sql`：

- `CreateMediaEdge`：插入 dependency edge，返回完整行。
- `ListMediaEdgesByWorkspace`：按 workspace 查询。
- `GetMediaEdgeByID`：按 id 查询。
- `DeleteMediaEdge`：按 id 删除。
- `ListOutgoingDependencyEdges`：按 `from_node_id` 查询 dependency 出边，用于环检测。
- `GetDependencyEdgeByEndpoints`：按 from/to 查询重复 edge。

修改 `canvas.sql` 或新增 canvas 聚合逻辑，使 canvas handler 能拿到 workspace 下所有 edges。

## 5. 后端 API

### 5.1 `POST /api/edges`

请求：

```json
{
  "workspace_id": "uuid",
  "from_node_id": "uuid",
  "to_node_id": "uuid"
}
```

响应：

- `200`：完整 edge。
- `400`：参数无效或自连接。
- `403`：workspace 不属于当前用户。
- `404`：workspace 或节点不存在。
- `409`：重复 dependency edge。
- `422`：创建后会形成 dependency 环。

业务规则：

- 两个节点必须存在。
- 两个节点必须属于请求中的同一个 workspace。
- workspace 必须属于当前用户。
- 本阶段忽略请求体中的 `edge_type`，固定创建 `dependency`。

### 5.2 环检测与并发

创建 edge 前，从 `to_node_id` 沿 dependency 出边 BFS。如果能到达 `from_node_id`，创建会形成环，返回 `422`。

BFS 和 INSERT 必须在同一个事务中执行。建议使用 `pgx.BeginTx` 设置 `SERIALIZABLE` 隔离级别。若事务提交遇到 serialization failure，返回 `409` 或重试一次后再返回错误。

原因：两个并发请求可能分别创建 A -> B 和 B -> A，普通读已提交事务会让两个请求都通过环检测。

### 5.3 `DELETE /api/edges/:id`

删除前校验 edge 所属 workspace 归属当前用户。成功返回 `204`。

### 5.4 canvas 响应

`GET /api/workspaces/:id/canvas` 扩展为：

```json
{
  "camera": { "x": 0, "y": 0, "zoom": 1 },
  "nodes": [],
  "edges": [
    {
      "id": "uuid",
      "workspace_id": "uuid",
      "from_node_id": "uuid",
      "to_node_id": "uuid",
      "edge_type": "dependency",
      "source": "user",
      "created_at": "timestamp"
    }
  ]
}
```

前端 DTO 可以继续使用 snake_case API 字段，必要时在 `lib/canvas.ts` 中转换为 shape props。

## 6. 前端设计

### 6.1 技术验证门槛

正式实现前先在当前 `WorkspaceDetailPage` 中验证 tldraw v5 的 ArrowShape 和 binding API：

- 能创建一条从 media shape A 到 media shape B 的 ArrowShape。
- 能把后端 edge id 存在 arrow shape `meta.edgeId` 或等价字段。
- 删除 ArrowShape 时能识别对应 edge id。

如果 tldraw v5 内置 arrow tool 的 handle/binding API 与预期不符，则允许退化为自定义端口拖拽层：拖拽完成后创建后端 edge，再用 tldraw ArrowShape 展示结果。

### 6.2 shape 映射

在 `apps/web/src/lib/canvas.ts` 新增：

- `edgeShapeIdForEdge(edgeId)`。
- `edgeToArrow(edge, nodes)`。
- `isEdgeArrowShape(shape)`。

ArrowShape 需要保存：

- `meta.edgeId`：后端 edge id。
- `meta.edgeType`：本阶段为 `dependency`。
- `from` binding：from node shape。
- `to` binding：to node shape。

### 6.3 端口交互

节点左右两侧显示视觉端口：

- 左侧中点：输入端口。
- 右侧中点：输出端口。
- 默认隐藏，hover 节点时显示。
- 拖拽输出到另一个节点输入时尝试创建 edge。

创建流程：

1. 用户从 A 输出端口拖到 B 输入端口。
2. 前端调用 `POST /api/edges`。
3. 成功后创建 ArrowShape。
4. `409` 显示“连线已存在”。
5. `422` 显示“不能形成循环依赖”。
6. 失败时不保留本地临时线。

删除流程：

1. 用户选中 ArrowShape。
2. 按 Delete/Backspace。
3. 前端调用 `DELETE /api/edges/:id`。
4. 成功后删除 ArrowShape。
5. 失败时 refetch canvas 并恢复事实源状态。

### 6.4 初始加载

画布加载顺序：

1. 创建所有 MediaShape。
2. 创建所有 ArrowShape 和 bindings。
3. 设置 camera。

若 edge 引用的 node 不在响应 nodes 中，前端跳过该 edge 并记录控制台 warning。正常情况下数据库 FK 和 workspace 查询不会产生这种数据。

## 7. 测试与验收

### 7.1 后端验收

```bash
make migrate-up
make sqlc-generate
make server-test
```

测试覆盖：

- 创建 dependency edge 成功。
- 重复 edge 返回 `409`。
- 自连接返回 `400`。
- A -> B 已存在时创建 B -> A 返回 `422`。
- 不同 workspace 的节点不能连线。
- 删除节点后关联 edge 被级联删除。
- canvas 响应包含 edges。
- 并发创建 A -> B 与 B -> A 不会同时成功。

### 7.2 前端验收

```bash
pnpm --filter @clip-anvil/web... build
```

浏览器验收：

- 两个节点之间可建立 dependency 连线。
- 刷新后连线仍存在。
- 删除连线后刷新不再出现。
- 成环和重复连线有明确错误提示。
- 删除节点后关联连线从画布消失。

## 8. 完成条件

- `media_edge` 成为 dependency 事实源。
- canvas 初始加载能从业务 DB 投影 nodes + edges。
- 后端能防止 dependency DAG 成环。
- M1.x-A 的多类型节点能力不回退。

## 9. 关联文档

- [M1.x-A Studio 节点基础完善](./2026-06-13-m1x-a-studio-node-foundation-design.md)
- [画布设计](../../design-canvas.md)
- [Studio 模式设计](../../design-studio-mode.md)
- [数据库设计](../../database-design.md)
