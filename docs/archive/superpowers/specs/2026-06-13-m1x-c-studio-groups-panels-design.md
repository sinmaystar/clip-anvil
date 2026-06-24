# M1.x-C Studio 分组、资源树与属性面板规格

**状态**：待实施
**前置**：M1.x-B 连线与 DAG 完成
**目标**：补齐 Studio 的组织和编辑体验：MediaGroup、完整左侧资源树、右侧属性面板、节点上游引用展示和 Prompt 引用辅助。

## 1. 当前事实基准

M1.x-B 完成后应具备：

- canvas 响应包含 `camera`、`nodes`、`edges`。
- 前端可创建四种节点和 dependency 连线。
- 右侧属性面板仍不存在。
- 左侧仍是 M1 轻量节点列表，不是完整资源树。
- `media_node` 仍没有 `group_id`。
- 没有 `media_group` 表和 group API。

本阶段不引入文件上传、WebSocket、自动布局、生成任务或版本系统。

## 2. 范围

### 2.1 包含

- 新增 `media_group` 表。
- 给 `media_node` 增加 `group_id`。
- 新增 group CRUD API 和成员管理 API。
- `GET /api/workspaces/:id/canvas` 返回 `groups`。
- 前端新增自定义 `GroupFlowNode`。
- 左侧升级为完整资源树：搜索、类型筛选、分组折叠、未分组节点。
- 右侧属性面板：节点基础属性、Prompt 编辑、上游输入引用、连线详情。
- 新增 `GET /api/nodes/:id/inputs`。

### 2.2 不包含

- 分组嵌套。
- 资源树拖拽排序。
- 版本列表、模型参数持久化、生成按钮。
- WebSocket 同步，仍以 REST + TanStack Query 缓存更新为主。
- 自动布局，留给 M1.x-D。

## 3. 数据库设计

新增 `apps/server/migrations/003_add_groups.sql`：

```sql
-- +goose Up
CREATE TABLE media_group (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_media_group_workspace ON media_group(workspace_id);

ALTER TABLE media_node
    ADD COLUMN group_id UUID REFERENCES media_group(id) ON DELETE SET NULL;

CREATE INDEX idx_media_node_group ON media_node(group_id);

-- +goose Down
DROP INDEX IF EXISTS idx_media_node_group;
ALTER TABLE media_node DROP COLUMN IF EXISTS group_id;
DROP INDEX IF EXISTS idx_media_group_workspace;
DROP TABLE IF EXISTS media_group;
```

规则：

- 一个节点最多属于一个分组。
- 删除分组不删除节点，节点 `group_id` 自动置空。
- 分组是纯组织工具，不影响 dependency DAG。

## 4. sqlc 查询

新增 `apps/server/sqlc/queries/group.sql`：

- `CreateMediaGroup`。
- `ListMediaGroupsByWorkspace`。
- `GetMediaGroupByID`。
- `UpdateMediaGroup`。
- `DeleteMediaGroup`。

扩展 `apps/server/sqlc/queries/node.sql`：

- `UpdateMediaNodeGroup`。
- `ClearMediaNodeGroup`。
- `ListMediaNodesByGroup`。

如 sqlc 对 `IN (...)` 参数生成不自然，成员批量替换可以在 Go 事务中逐个执行更新，优先保证清晰和正确。

## 5. 后端 API

### 5.1 Group API

| 端点 | 方法 | 请求体 | 成功 |
|---|---|---|---|
| `/api/groups` | POST | `{workspace_id, name, node_ids?}` | `200 {group, node_ids}` |
| `/api/groups/:id` | PATCH | `{name?, sort_order?}` | `200 {group}` |
| `/api/groups/:id` | DELETE | 空 | `204` |
| `/api/groups/:id/nodes` | PUT | `{node_ids}` | `204` |

校验：

- workspace 必须属于当前用户。
- `node_ids` 中的所有节点必须属于同一个 workspace。
- `name` trim 后不能为空。
- `PUT /groups/:id/nodes` 是全量替换：先清空当前组成员，再设置新成员。

### 5.2 Node API 扩展

`PATCH /api/nodes/:id` 请求体新增：

```json
{
  "group_id": "uuid or null"
}
```

规则：

- `group_id` 为 UUID 时，目标 group 必须与 node 同 workspace。
- `group_id: null` 表示移出分组。
- 未传 `group_id` 时保持当前组不变。

### 5.3 节点上游输入 API

新增：

```http
GET /api/nodes/:id/inputs
```

返回通过 dependency edge 连向该节点的上游节点列表：

```json
[
  {
    "id": "uuid",
    "node_type": "image",
    "title": "产品主图",
    "status": "draft",
    "thumbnail_url": null
  }
]
```

查询仅使用 `edge_type = 'dependency'`。

### 5.4 canvas 响应扩展

`GET /api/workspaces/:id/canvas` 增加：

```json
{
  "groups": [
    {
      "id": "uuid",
      "workspace_id": "uuid",
      "name": "分镜组",
      "sort_order": 0,
      "node_ids": ["uuid-a", "uuid-b"]
    }
  ]
}
```

`node_ids` 在 Go 层根据 canvas nodes 的 `group_id` 分桶组装，避免额外复杂查询。

## 6. 前端设计

### 6.1 group container node

新增 `apps/web/src/components/canvas-flow/GroupFlowNode.tsx`。

不使用 React Flow 内置 group node 作为事实源，因为本产品需要“删除分组保留节点”和“折叠分组”的业务语义。自定义 node 只负责视觉容器，成员关系由 `media_node.group_id` 决定。

props：

- `groupId`。
- `name`。
- `nodeCount`。
- `collapsed`。
- `w`、`h`。

行为：

- 创建分组时，根据选中节点包围盒生成容器，padding 20px。
- 删除分组只删除 group container node，并调用 `DELETE /api/groups/:id`，不删除成员节点。
- 拖拽节点进入容器边界，调用 `PATCH /api/nodes/:id { group_id }`。
- 拖拽节点移出容器边界，调用 `PATCH /api/nodes/:id { group_id: null }`。
- 移动分组时，组内节点跟随移动；结束后批量持久化节点坐标。

折叠行为：

- 折叠 group container 后只显示标题栏。
- 成员 MediaFlowNode 在前端隐藏或禁用交互。
- 折叠状态属于前端 UI 状态，本阶段不要求持久化到 DB。

### 6.2 左侧资源树

新增 `apps/web/src/components/ResourceTree.tsx`，替换 M1 轻量节点列表。

数据源：TanStack Query 的 canvas payload，不新增 API。

功能：

- 搜索框：按节点标题和分组名称模糊过滤。
- 类型筛选：全部/text/image/video/audio。
- 分组折叠：仅影响左侧树，不影响画布 group 折叠。
- 未分组节点区域。
- 点击节点：`editor.zoomToNode(shapeId)` 并选中节点。
- 点击分组：定位到 group container；若 group node 不存在，则定位到成员节点包围盒。

资源树结构：

```text
搜索
类型筛选
分镜组
  视频节点 A
  音频节点 B
素材组
  图片节点 C
未分组
  文本节点 D
```

### 6.3 右侧属性面板

新增 `apps/web/src/components/PropertyPanel.tsx`。

面板显示规则：

- 未选中任何元素：隐藏。
- 选中 MediaFlowNode：显示节点属性。
- 选中 custom dependency edge：显示连线属性。
- 选中 group container node：显示分组名称和成员数。

节点属性：

- 标题输入框。
- 类型和状态只读。
- 上游输入引用 chips，来自 `GET /api/nodes/:id/inputs`。
- Prompt textarea，保存使用现有 `PATCH /api/nodes/:id`。
- 输入 `@` 时弹出上游节点选择器，只列出已通过 dependency edge 连接的上游节点。
- 选择上游节点只插入 `@节点标题` 文本，不创建 edge。

连线属性：

- 起点节点标题。
- 终点节点标题。
- edge 类型：dependency，只读。
- 删除连线按钮。

分组属性：

- 分组名称输入框。
- 成员数。
- 删除分组按钮。

### 6.4 M1 内联编辑的处理

M1 的节点下方内联编辑面板保留为轻量编辑入口。右侧属性面板提供同一份 title/prompt 的深度编辑。

同步规则：

- 两处编辑都通过 `updateMediaNode` 写后端。
- 成功后更新 TanStack Query 缓存和 node data。
- 如果两处同时编辑，最后一次成功响应为准。

## 7. 测试与验收

### 7.1 后端验收

```bash
make migrate-up
make sqlc-generate
make server-test
```

测试覆盖：

- 创建分组并关联节点。
- 更新分组名称。
- 全量替换分组成员。
- 删除分组不删除节点。
- 删除分组后节点 `group_id` 为空。
- 不同 workspace 的节点不能加入分组。
- canvas 响应包含 groups 和 node_ids。
- `GET /api/nodes/:id/inputs` 对有上游和无上游节点分别返回正确结果。

### 7.2 前端验收

```bash
pnpm --filter @clip-anvil/web... build
```

浏览器验收：

- 选中多个节点可创建分组。
- 分组显示为容器，删除分组后节点保留。
- 拖入/拖出分组后左侧资源树同步。
- 资源树搜索和类型筛选生效。
- 点击资源树节点能定位画布并打开属性面板。
- 选中节点可在右侧编辑标题和 Prompt。
- 有上游 dependency 的节点输入 `@` 时只显示上游节点。
- 选中连线时右侧显示起点、终点和删除按钮。

## 8. 完成条件

- Studio 具备节点组织能力：groups + resource tree。
- Studio 具备深度编辑入口：property panel。
- 数据事实源仍是业务 DB，React Flow 只做投影。
- 不引入生成任务、asset 或 WebSocket 依赖。

## 9. 关联文档

- [M1.x-B Studio 连线与 DAG](./2026-06-13-m1x-b-studio-dag-edges-design.md)
- [画布设计](../../../design/canvas.md)
- [Studio 模式设计](../../../design/studio-mode.md)
- [数据库设计](../../../engineering/database.md)
