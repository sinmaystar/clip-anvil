# M2 Studio 画布完善 — 里程碑规格

**状态**：📋 待实施
**前置**：M1 Studio 画布基础（auth + workspace + text 节点）
**目标**：补齐 Studio 画布的交互体验——多类型节点、连线、分组、资源树、属性面板、WebSocket、自动布局、文件上传，使其成为完整可用的多媒体画布编辑器

## 1. 里程碑范围

M2 在 M1 基础上交付 8 个可独立验收的阶段：

| 阶段 | 交付 | 端到端验证 |
|---|---|---|
| 1. 多类型节点 | image/video/audio 节点 + 右键菜单扩展 | 右键菜单创建 4 种类型节点，各自渲染正确 |
| 2. 连线 + DAG | dependency 连线 + 环检测 + 端口拖拽 | 拖拽连线、环检测拒绝、刷新后连线保持 |
| 3. 分组 | 自定义 group container node + 折叠/展开 + 拖入拖出 | 选中多节点分组、折叠展开、跨组连线 |
| 4. 左侧资源树 | 树结构 + 搜索/筛选 + 双向同步 | 资源树点击定位到画布节点、分组同步 |
| 5. 右侧属性面板 | 节点详细编辑 + 连线详情 + @引用 | 选中节点编辑 Prompt、@引用上游资源 |
| 6. WebSocket 基础 | `/ws/canvas` 事件推送 | 第二个浏览器标签页实时看到另一个标签页的操作 |
| 7. 自动布局 | @dagrejs/dagre 前端计算 DAG 层级排列 + 批量更新后端坐标 | 点击自动布局按钮后节点按 DAG 层级排列，坐标持久化 |
| 8. 拖拽上传 | 拖拽文件到画布 + MinIO 上传 + 自动创建对应类型节点 | 拖拽图片到画布 → 上传 MinIO → 创建 image 节点并显示缩略图 |

**不在范围内**（留后续迭代）：
- AI 生成能力（DashScope 集成、GenerationJob、生成进度）
- 版本管理（ArtifactVersion、winner 切换、评审记录）
- Stale 传播与增量重算
- Agent 模式（对话面板、生产级工具、PSS）
- 浮动工具栏（节点类型快捷创建入口）

## 2. 阶段 1：多类型节点

### 2.1 后端变更

**前提**：M1 的 `001_init_schema.sql` 按 database-design.md 的完整 schema 建表（包含 `asset_id`、`group_id`、`prompt`、`model_provider` 等所有字段），M2 不需要对 `media_node` 表做 ALTER TABLE。

**node_type 限制放开**：M1 中 POST `/api/nodes` 只允许 `text`，M2 放开为 `text | image | video | audio`。

**默认尺寸按类型设定**：

| node_type | canvas_w | canvas_h |
|---|---|---|
| `text` | 200 | 120 |
| `image` | 200 | 160 |
| `video` | 240 | 180 |
| `audio` | 200 | 80 |

后端在 POST `/api/nodes` 时根据 `node_type` 设置默认 `canvas_w/h`（如果请求体未指定）。

无新增 API 端点，只修改创建节点的校验和默认值逻辑。

### 2.2 前端 MediaFlowNode 扩展

M1 的 MediaFlowNode 只渲染 text 类型。M2 扩展为根据 `nodeType` 渲染不同卡片：

**四种卡片渲染**：

```
text (200×120):                    image (200×160):
┌─────────────────────┐            ┌─────────────────────┐
│ 📝 标题       [状态] │            │ 🖼 标题       [状态] │
├─────────────────────┤            ├─────────────────────┤
│                     │            │                     │
│ 文本预览（前3行）    │            │  [占位图/缩略图]     │
│                     │            │                     │
│                     │            │                     │
└─────────────────────┘            └─────────────────────┘

video (240×180):                   audio (200×80):
┌─────────────────────┐            ┌─────────────────────┐
│ 🎬 标题       [状态] │            │ 🔊 标题       [状态] │
├─────────────────────┤            ├─────────────────────┤
│                     │            │ [波形占位] 0:00      │
│  [占位图/缩略图]     │            └─────────────────────┘
│       ▶ 0:00        │
│                     │
└─────────────────────┘
```

M2 阶段没有生成能力，所以图片/视频/音频节点只有占位图（灰色区域 + 类型图标）。缩略图能力为后续迭代预留字段（`thumbnailUrl` 在 node data 中已有，M2 时为空）。

**状态边框样式**（与 M1 一致，所有节点类型共用）：

| 状态 | 边框 |
|---|---|
| draft | 1px 灰色虚线 |
| ready | 1px 灰色实线 |

M2 只会用到 draft 和 ready 两种状态（无生成能力，不会进入 queued/running/succeeded/failed）。

### 2.3 右键菜单扩展

M1 右键菜单只有"创建文本节点"，M2 扩展为 4 种类型：

```
┌──────────────┐
│ 创建节点 ▶    │
│ ├── 📝 文本   │
│ ├── 🖼 图片   │
│ ├── 🎬 视频   │
│ └── 🔊 音频   │
└──────────────┘
```

节点创建通过右键菜单和拖拽上传完成。浮动工具栏延后到后续迭代。

### 2.4 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 1.1 | 创建 image 节点 | `curl -sf -X POST localhost:8888/api/nodes -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"workspace_id":"'$WID'","node_type":"image","title":"产品图","canvas_x":0,"canvas_y":0}' \| jq '.canvas_w'` | 输出 `200` |
| 1.2 | 创建 video 节点 | 同上，node_type=video | `canvas_w` = `240`，`canvas_h` = `180` |
| 1.3 | 创建 audio 节点 | 同上，node_type=audio | `canvas_w` = `200`，`canvas_h` = `80` |
| 1.4 | 画布包含多类型节点 | `curl -sf localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq '[.nodes[].node_type] \| unique \| sort'` | 输出 `["audio","image","text","video"]` |
| 1.5 | 前端编译通过 | `pnpm --filter @clip-anvil/web build` | 退出码 0 |

## 3. 阶段 2：连线（Edge）+ DAG

### 3.1 数据库迁移

创建 `apps/server/migrations/002_add_edges.sql`：

**media_edge 表**：

与 database-design.md 的完整 schema 保持一致，建表时包含 `edge_type`、`transition_type`、`transition_duration` 字段。M2 的 API 层只允许创建 `dependency` 类型，但 schema 已为后续 reference/sequence 类型就绪，避免未来破坏性迁移。

```sql
CREATE TABLE media_edge (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    from_node_id        UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    to_node_id          UUID NOT NULL REFERENCES media_node(id) ON DELETE CASCADE,
    edge_type           TEXT NOT NULL DEFAULT 'dependency',
    transition_type     TEXT,
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
```

### 3.2 sqlc 新增 queries

`sqlc/queries/edge.sql`：
- `CreateMediaEdge` — INSERT（含 edge_type 字段）返回完整行
- `ListMediaEdgesByWorkspace` — SELECT WHERE workspace_id
- `GetMediaEdgeByID` — SELECT WHERE id
- `DeleteMediaEdge` — DELETE WHERE id
- `ListOutgoingEdges` — SELECT WHERE from_node_id AND edge_type = 'dependency'（用于 DAG 环检测 BFS，只沿 dependency 边走）
- `GetEdgeByEndpoints` — SELECT WHERE from_node_id AND to_node_id AND edge_type（唯一性检查，按类型区分）

### 3.3 后端 API

| 端点 | 方法 | 鉴权 | 请求体 | 成功响应 | 错误响应 |
|---|---|---|---|---|---|
| `/api/edges` | POST | JWT | `{workspace_id, from_node_id, to_node_id}` | `200 {完整 edge}` | `400` 参数无效 / `409` 重复 / `422` 成环 |
| `/api/edges/:id` | DELETE | JWT | — | `204` | `403` / `404` |

M2 的 POST `/api/edges` 不接受 `edge_type` 参数，固定创建 `dependency` 类型。后续里程碑开放 reference/sequence 时再扩展请求体。

**业务规则**：
- `from_node_id` 和 `to_node_id` 必须属于同一个 workspace
- 两个节点必须属于当前用户的 workspace
- **DAG 环检测**：创建前从 `to_node_id` 沿出边做 BFS，如果能到达 `from_node_id` 则拒绝（返回 422）
- 自连接由数据库 CHECK 约束拦截

**事务隔离**：环检测 BFS + INSERT 必须在同一个 `SERIALIZABLE` 事务中执行。多标签页场景下（M2 引入了 WebSocket），两个并发请求可能同时创建 A→B 和 B→A，若不加事务保护会绕过环检测。替代方案：在 BFS 遍历的起始节点上使用 `SELECT ... FOR UPDATE` 加行锁。

**Handler 实现**（`internal/api/edge_handler.go`）：
- Create：开启 SERIALIZABLE 事务 → 参数校验 → 归属校验 → 环检测（BFS）→ INSERT → 提交 → 返回
- Delete：归属校验 → DELETE → 返回

**canvas GET 扩展**：`GET /api/workspaces/:id/canvas` 返回体新增 `edges` 数组：

```json
{
  "camera": { "x": 0, "y": 0, "zoom": 1 },
  "nodes": [...],
  "edges": [
    {
      "id": "uuid",
      "from_node_id": "uuid",
      "to_node_id": "uuid",
      "edge_type": "dependency",
      "source": "user"
    }
  ]
}
```

### 3.4 前端 custom dependency edge

**连线视觉规格**：

只有一种连线类型（dependency），含义统一为"A 的输出作为 B 的输入"：

| 线型 | 颜色 | 箭头 |
|---|---|---|
| 2px 实线 | `#3b82f6` 蓝 | 实心三角 |

**实现方式**：使用 React Flow 内置的 custom dependency edge + React Flow edge model。edge 的后端 ID 存在 custom dependency edge 的 meta 字段中。

**edgeToFlowEdge 映射函数**（`apps/web/src/lib/canvas.ts`）：

```typescript
function edgeToFlowEdge(edge: MediaEdgeDTO): {
  edge: Node
  edges: Edge[]
}
```

从 edge 数据构建 custom dependency edge + 两端 edge。

### 3.5 前端端口交互

**端口显示**：
- 每个 MediaFlowNode 左侧中点为输入端口（圆点，半径 4px）
- 右侧中点为输出端口
- 默认隐藏，鼠标悬停节点时显示为灰色实心圆点
- 拖拽端口时，所有合法目标端口高亮为蓝色

**拖拽连线交互**：
1. 鼠标 hover 节点 → 端口显示
2. 从输出端口开始拖拽 → 出现临时连线跟随鼠标
3. 拖拽到目标节点的输入端口 → 释放
4. `POST /api/edges { workspace_id, from_node_id, to_node_id }`
5. 成功 → 创建 custom dependency edge + Binding
6. 失败（成环）→ toast "不能形成循环依赖"
7. 失败（重复）→ toast "连线已存在"

**删除连线**：
- 选中连线 → 按 Delete/Backspace → `DELETE /api/edges/:id` → 删除 custom dependency edge

**端口实现方式**：使用 React Flow 内置的 drag-to-connect interaction + edge 机制。

- MediaFlowNode 通过 `getHandles()` 返回左中（输入）和右中（输出）两个 handle
- 用户使用 React Flow 的 drag-to-connect interaction 从输出 handle 拖拽到目标节点的输入 handle，React Flow 处理吸附和碰撞检测
- custom edge 创建完成后（`onBindingChange`），前端发送 `POST /api/edges` 创建后端记录
- 后端返回失败（成环/重复）时，前端删除刚创建的 custom dependency edge 并 toast 提示
- 端口圆点在 MediaFlowNode 的 `component()` 中渲染为绝对定位元素，仅做视觉提示

### 3.6 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 2.1 | 迁移成功 | `make migrate-up` | 退出码 0，media_edge 表创建 |
| 2.2 | 创建 edge | `curl -sf -X POST localhost:8888/api/edges -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"workspace_id":"'$WID'","from_node_id":"'$NID_A'","to_node_id":"'$NID_B'"}' \| jq -e '.id'` | 输出 UUID |
| 2.3 | 画布包含 edges | `curl -sf localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq '.edges \| length'` | 输出 >= 1 |
| 2.4 | 环检测拒绝 | 已有 A→B，创建 B→A | HTTP 422 |
| 2.5 | 自连接拒绝 | from_node_id = to_node_id | HTTP 400 |
| 2.6 | 重复连线拒绝 | 再次创建 A→B | HTTP 409 |
| 2.7 | 删除 edge | `curl -sf -o /dev/null -w '%{http_code}' -X DELETE localhost:8888/api/edges/$EID -H "Authorization: Bearer $TOKEN"` | 输出 `204` |
| 2.8 | 级联删除：删节点时关联 edge 自动删除 | 删除 node A → 查询 canvas edges | 与 A 相关的 edge 已消失 |
| 2.9 | 后端单测通过 | `make server-test` | 退出码 0 |
| 2.10 | 前端编译通过 | `pnpm --filter @clip-anvil/web build` | 退出码 0 |

## 4. 阶段 3：分组（Group）— 纯逻辑组织

分组是纯粹的视觉组织工具，不对节点施加任何约束或行为限制。用于将相关节点归类（如"分镜组"、"素材组"），便于浏览和管理。

### 4.1 数据库迁移

创建 `apps/server/migrations/003_add_groups.sql`：

```sql
CREATE TABLE media_group (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_media_group_workspace ON media_group(workspace_id);

ALTER TABLE media_node ADD COLUMN group_id UUID REFERENCES media_group(id) ON DELETE SET NULL;
CREATE INDEX idx_media_node_group ON media_node(group_id);
```

### 4.2 sqlc 新增 queries

`sqlc/queries/group.sql`：
- `CreateMediaGroup` — INSERT 返回完整行
- `ListMediaGroupsByWorkspace` — SELECT WHERE workspace_id ORDER BY sort_order
- `GetMediaGroupByID` — SELECT WHERE id
- `UpdateMediaGroup` — UPDATE name/sort_order WHERE id
- `DeleteMediaGroup` — DELETE WHERE id

`sqlc/queries/node.sql` 新增：
- `UpdateMediaNodeGroup` — UPDATE group_id WHERE id
- `BatchUpdateMediaNodeGroup` — UPDATE group_id WHERE id IN (...)

### 4.3 后端 API

| 端点 | 方法 | 鉴权 | 请求体 | 成功响应 | 错误响应 |
|---|---|---|---|---|---|
| `/api/groups` | POST | JWT | `{workspace_id, name, node_ids?[]}` | `200 {id, name, sort_order, node_ids}` | `400` |
| `/api/groups/:id` | PATCH | JWT | `{name?, sort_order?}` | `200 {更新后 group}` | `403` / `404` |
| `/api/groups/:id` | DELETE | JWT | — | `204` | `403` / `404` |
| `/api/groups/:id/nodes` | PUT | JWT | `{node_ids: [...]}` | `204` | `403` |

**业务规则**：
- 创建分组时可选传入 `node_ids`，批量将这些节点的 `group_id` 设为新分组
- 删除分组不删除节点（节点的 `group_id` 通过 `ON DELETE SET NULL` 自动置空）
- PUT `/api/groups/:id/nodes` 为全量替换该分组的节点成员：
  - 先将所有 `group_id = 本分组` 的节点置空
  - 再将 `node_ids` 列表中的节点 `group_id` 设为本分组
  - 这保证一个节点最多属于一个分组
- 节点的 `group_id` 也可通过 PATCH `/api/nodes/:id` 更新（body 加 `group_id` 字段）

**canvas GET 扩展**：返回体新增 `groups` 数组：

```json
{
  "camera": {...},
  "nodes": [...],
  "edges": [...],
  "groups": [
    {
      "id": "uuid",
      "name": "广告分镜",
      "sort_order": 0,
      "node_ids": ["uuid1", "uuid2", "uuid3"]
    }
  ]
}
```

`groups[].node_ids` 在 Go 层组装：canvas GET 已经查出所有 nodes，遍历 nodes 按 `group_id` 分桶（O(n)），无需额外 SQL 查询。

### 4.4 前端分组交互

**为什么不用 React Flow 内置 group node**：React Flow 的 group node 在删除 group 时会级联删除子 node，且不支持折叠/展开。M2 需要"删除分组保留节点"的语义，因此使用自定义的 `GroupFlowNode`。

**GroupFlowNode 实现**：
- 注册为 `group-container` 类型的自定义 node
- props 包含：`groupId`（后端 group ID）、`name`（分组名）、`collapsed`（折叠状态）、`nodeCount`（成员数）
- 渲染为虚线边框矩形 + 标题栏（名称 + 成员计数 + 折叠按钮）
- 不使用 React Flow 的父子层级关系，分组与节点的关系完全由后端 `group_id` 驱动

**创建分组**：
- 选中多个节点（框选或 Shift+点击）→ 右键菜单 → "创建分组" / 快捷键 ⌘G
- 弹出 input 输入分组名称 → `POST /api/groups { workspace_id, name, node_ids }` → 成功 → 创建 group container node，位置和尺寸根据选中节点的包围盒自动计算（留 padding 20px）

**渲染**：

展开态：
```
┌ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┐
  📁 广告分镜 (4)    [▼折叠]
├ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┤
│                              │
│  [节点A]  [节点B]  [节点C]    │
│           [节点D]             │
└ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘
```

折叠态：
```
┌ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┐
  📁 广告分镜 (4)   [▶展开]
└ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┘
```

折叠时，group container node 缩小为标题栏高度，内部节点通过 `editor.updateNode()` 设置 `opacity: 0` + 禁用交互。展开时恢复。

**拖入/拖出**：
- 拖拽节点进入 group container node 边界 → `PATCH /api/nodes/:id { group_id }` → 节点加入分组
- 拖拽节点移出 group container node 边界 → `PATCH /api/nodes/:id { group_id: null }` → 节点脱离分组
- 判断进入/移出通过 `onTranslateEnd` 事件检测节点坐标是否在 group container node 的边界框内

**移动跟随**：
- 监听 group container node 的 `onTranslate` 事件，计算位移差值
- 遍历 `group_id = 本分组` 的所有 MediaFlowNode，批量 `editor.updateNode()` 跟随偏移
- 移动结束后通过 `PATCH /api/nodes/batch-position` 持久化新坐标

**其他行为**：
- 删除分组 → 只删除 group container node + `DELETE /api/groups/:id`，内部 MediaFlowNode 保留（后端 `ON DELETE SET NULL` 自动清空 `group_id`）
- 分组名称双击可编辑 → `PATCH /api/groups/:id { name }`
- 连线可跨越分组边界（分组不影响依赖关系）

### 4.5 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 3.1 | 迁移成功 | `make migrate-up` | media_group 表创建，media_node 表有 group_id 列 |
| 3.2 | 创建分组 | `curl -sf -X POST localhost:8888/api/groups -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"workspace_id":"'$WID'","name":"分镜","node_ids":["'$NID_A'","'$NID_B'"]}' \| jq -e '.id'` | 输出 UUID |
| 3.3 | 画布包含分组 | `curl -sf localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq '.groups \| length'` | 输出 >= 1 |
| 3.4 | 分组包含节点 | `curl -sf localhost:8888/api/workspaces/$WID/canvas -H "Authorization: Bearer $TOKEN" \| jq '.groups[0].node_ids \| length'` | 输出 `2` |
| 3.5 | 更新分组成员 | `curl -sf -o /dev/null -w '%{http_code}' -X PUT localhost:8888/api/groups/$GID/nodes -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"node_ids":["'$NID_A'"]}'` | 输出 `204` |
| 3.6 | 更新后成员数 | 查询 canvas → groups[0].node_ids length | 输出 `1` |
| 3.7 | 删除分组不删节点 | `curl -sf -o /dev/null -w '%{http_code}' -X DELETE localhost:8888/api/groups/$GID -H "Authorization: Bearer $TOKEN"` → 查询节点仍在 | 节点 id 仍在 nodes 数组中 |
| 3.8 | 节点 group_id 已清空 | 查询 canvas → nodes 中原分组节点无 group 关联 | groups 数组为空 |
| 3.9 | 后端单测通过 | `make server-test` | 退出码 0 |
| 3.10 | 前端编译通过 | `pnpm --filter @clip-anvil/web build` | 退出码 0 |

## 5. 阶段 4：左侧资源树

### 5.1 后端变更

无新增 API。资源树数据从 canvas GET 返回的 nodes + groups 构建。

### 5.2 前端资源树组件

**布局**：画布页面左侧 ~200px 可折叠面板。

```
┌──────────┐
│ 🔍搜索    │  ← 搜索框：按名称模糊过滤
├──────────┤
│ [全部] [📝] [🖼] [🎬] [🔊] │  ← 类型筛选 chips，单选切换
├──────────┤
│ 📁 分镜 (3)        ▼ │  ← 分组折叠/展开
│   🎬 01-产品特写  ✓  │  ← 节点行：图标 + 名称 + 状态
│   🎬 02-使用场景  ○  │
│   🎬 03-卖点展示  ○  │
│ 📁 素材 (2)        ▼ │
│   🖼 产品主图     ○  │
│   🖼 品牌Logo     ○  │
│ ── 未分组 ──        │  ← 不属于任何分组的节点
│   📝 脚本草稿     ○  │
├──────────┤
│ [◁ 收起]           │  ← 折叠侧栏
└──────────┘
```

**交互**：

| 操作 | 行为 |
|---|---|
| 搜索框输入 | 按 title 模糊过滤节点和分组 |
| 类型筛选 chips | 单选切换：全部 / text / image / video / audio |
| 点击节点行 | 画布 `editor.zoomToNode(shapeId)` 定位并选中该节点 |
| 点击分组名 | 画布定位到分组区域（展示所有分组内节点） |
| 分组 ▼/▶ | 折叠/展开分组内的节点列表 |
| 底部"收起" | 隐藏资源树面板，画布全屏 |

**状态徽标**：

| 状态 | 徽标 |
|---|---|
| draft | ○ 空心圆 |
| ready | ● 实心圆 |

**数据源**：从 TanStack Query 的 canvas 数据缓存中构建树结构，无需额外 API。当画布数据更新（create/delete/group 变更）时，资源树自动刷新。

**双向同步**：
- 画布上创建分组 → 资源树自动出现新文件夹（数据驱动，无需额外逻辑）
- 画布上删除分组 → 资源树文件夹消失
- 画布上拖入/拖出分组 → 资源树节点位置变化
- 资源树点击节点 → 画布定位（主动交互）

### 5.3 页面布局调整

画布页面从 M1 的全屏画布改为三栏（左侧可折叠）：

```
┌─────────────────────────────────────────────────┐
│  影砧  ·  咖啡广告项目          [← 返回项目列表]  │
├──────────┬──────────────────────────────────────┤
│          │                                      │
│ 资源树    │          React Flow 画布                  │
│ ~200px   │          (flex: 1)                   │
│          │                                      │
│ [收起]   │                                      │
└──────────┴──────────────────────────────────────┘
```

### 5.4 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 4.1 | 前端编译通过 | `pnpm --filter @clip-anvil/web build` | 退出码 0 |
| 4.2 | 资源树渲染节点 | 浏览器：创建节点后资源树中出现对应行 | 资源树实时更新 |
| 4.3 | 搜索过滤 | 浏览器：搜索框输入关键词 → 只显示匹配节点 | 过滤生效 |
| 4.4 | 类型筛选 | 浏览器：选择"🖼"→ 只显示 image 类型节点 | 筛选生效 |
| 4.5 | 点击定位 | 浏览器：点击资源树节点行 → 画布定位到对应节点 | 画布视口移动并选中 |
| 4.6 | 分组同步 | 浏览器：画布上创建分组 → 资源树出现文件夹 | 双向同步 |
| 4.7 | 侧栏折叠 | 浏览器：点击收起 → 侧栏隐藏，画布全屏 | 布局正确 |

## 6. 阶段 5：右侧属性面板

### 6.1 后端变更

**node PATCH 扩展**：`PATCH /api/nodes/:id` 的 body 新增可选字段 `group_id`（阶段 3 已加）。

**新增 API — 获取节点上游依赖列表**：

| 端点 | 方法 | 鉴权 | 说明 | 响应 |
|---|---|---|---|---|
| `/api/nodes/:id/inputs` | GET | JWT | 返回通过 dependency edge 连向本节点的所有上游节点 | `200 [{id, node_type, title, status, thumbnailUrl}]` |

这个接口为属性面板的"输入引用"区域和 Prompt @引用提供数据。查询逻辑：`SELECT media_node.* FROM media_node JOIN media_edge ON media_edge.from_node_id = media_node.id WHERE media_edge.to_node_id = :id`。

### 6.2 前端属性面板组件

**布局**：画布页面右侧 ~280px 可折叠面板，选中节点时展开。

```
┌───────────────┐
│ ⚙ 节点属性     │  ← 面板标题
├───────────────┤
│ 标题           │
│ [可编辑输入框]  │
├───────────────┤
│ 类型: 🎬 视频  │  ← 只读
│ 状态: ○ 草稿   │  ← 只读
│ 创建: 6/13     │  ← 只读
├───────────────┤
│ 输入引用        │
│ ┌────┐ ┌────┐ │  ← 上游依赖节点的缩略图 chips
│ │🖼 A │ │🖼 B │ │     点击 chip → 画布定位到对应节点
│ └────┘ └────┘ │     无上游依赖时显示"暂无输入"
├───────────────┤
│ Prompt         │
│ ┌─────────────┐│
│ │ 文本输入区域  ││  ← textarea，支持 @ 引用
│ │              ││     输入 @ 弹出上游节点选择器
│ │              ││     只能引用已通过 edge 连接的上游节点
│ └─────────────┘│
│ 字数: 120      │
├───────────────┤
│ [保存]         │  ← 保存 Prompt 修改
└───────────────┘
```

**@引用交互**：
1. 在 Prompt textarea 中输入 `@` 字符
2. 弹出浮动选择器，列出通过 dependency edge 连接的所有上游节点（从 `GET /api/nodes/:id/inputs` 获取）
3. 选择某节点 → textarea 中插入 `@节点标题`（富文本标记或纯文本占位符）
4. 如果没有上游依赖（无连线），弹出提示"先建立连线再引用"
5. @引用只做 Prompt 文本插入，不创建 edge（edge 必须先手动建立）

**连线详情**：

选中 custom dependency edge 时，右侧面板切换为连线详情视图：

```
┌───────────────┐
│ ⚙ 连线属性     │
├───────────────┤
│ 起点: 产品主图  │  ← from_node 标题（点击可定位）
│ 终点: 镜头01   │  ← to_node 标题（点击可定位）
│ 创建: 6/13     │  ← 只读
├───────────────┤
│ [删除连线]      │
└───────────────┘
```

### 6.3 页面布局调整

三栏完整布局（左右均可折叠）：

```
┌──────────────────────────────────────────────────────────────┐
│  影砧  ·  咖啡广告项目                    [← 返回项目列表]     │
├──────────┬───────────────────────────────────┬───────────────┤
│          │                                   │              │
│ 资源树    │          React Flow 画布               │  属性面板     │
│ ~200px   │          (flex: 1)                │  ~280px      │
│          │                                   │  (选中时显示) │
│          │                                   │              │
│ [收起]   │                                   │              │
└──────────┴───────────────────────────────────┴───────────────┘
```

**面板联动规则**：

| 场景 | 行为 |
|---|---|
| 未选中任何元素 | 右侧属性面板隐藏 |
| 单击选中一个节点 | 右侧展示节点属性面板 |
| 单击选中一条连线 | 右侧展示连线详情面板 |
| 点击画布空白区域 | 取消选择，属性面板隐藏 |
| 左侧资源树点击节点 | 画布定位 + 选中 + 右侧展示属性 |

### 6.4 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 5.1 | 获取节点上游输入 | `curl -sf localhost:8888/api/nodes/$NID_B/inputs -H "Authorization: Bearer $TOKEN" \| jq 'length'` (已有 A→B dependency) | 输出 >= 1 |
| 5.2 | 无上游输入返回空数组 | `curl -sf localhost:8888/api/nodes/$NID_A/inputs -H "Authorization: Bearer $TOKEN" \| jq 'length'` (A 无上游) | 输出 `0` |
| 5.3 | Prompt 保存 | `curl -sf -X PATCH localhost:8888/api/nodes/$NID -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"prompt":"镜头描述 @产品主图"}' \| jq '.prompt'` | 输出含 `@产品主图` 的字符串 |
| 5.5 | 后端单测通过 | `make server-test` | 退出码 0 |
| 5.6 | 前端编译通过 | `pnpm --filter @clip-anvil/web build` | 退出码 0 |
| 5.7 | 属性面板显示/隐藏 | 浏览器：选中节点 → 面板展开；点击空白 → 面板隐藏 | 联动正确 |
| 5.8 | @引用弹出选择器 | 浏览器：选中有上游依赖的节点 → Prompt 中输入 @ → 弹出上游节点列表 | 选择器只显示已连线的上游节点 |

## 7. 阶段 6：WebSocket 基础

### 7.1 后端 WebSocket 实现

**连接端点**：`GET /ws/canvas?workspaceId=xxx&token=xxx`

- Hertz WebSocket 升级（使用 `hertz-contrib/websocket`）
- 从 query param 提取 `token`，JWT 校验身份
- 校验 workspace 归属当前用户
- 连接成功后注册到 workspace 级别的连接池（内存 map）

**连接池管理**（`internal/api/ws_hub.go`）：

```go
type Hub struct {
    mu    sync.RWMutex
    conns map[uuid.UUID]map[*websocket.Conn]struct{} // workspaceID → connections
}

func (h *Hub) Register(workspaceID uuid.UUID, conn *websocket.Conn)
func (h *Hub) Unregister(workspaceID uuid.UUID, conn *websocket.Conn)
func (h *Hub) Broadcast(workspaceID uuid.UUID, event Event)
```

**事件类型**：

```go
type Event struct {
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload"`
}
```

| 事件类型 | 触发时机 | Payload |
|---|---|---|
| `NodeCreated` | POST `/api/nodes` 成功 | 完整节点数据 |
| `NodeUpdated` | PATCH `/api/nodes/:id` 成功 | `{nodeId, changes}` |
| `NodeDeleted` | DELETE `/api/nodes/:id` 成功 | `{nodeId}` |
| `EdgeCreated` | POST `/api/edges` 成功 | 完整 edge 数据 |
| `EdgeDeleted` | DELETE `/api/edges/:id` 成功 | `{edgeId}` |
| `GroupCreated` | POST `/api/groups` 成功 | 完整 group 数据 |
| `GroupUpdated` | PATCH `/api/groups/:id` 成功 | `{groupId, changes}` |
| `GroupDeleted` | DELETE `/api/groups/:id` 成功 | `{groupId}` |

**广播逻辑**：在各 handler 成功写 DB 后调用 `hub.Broadcast(workspaceID, event)`。广播给 workspace 下所有连接，**包括操作发起者自己**（前端负责去重——如果收到的事件对应自己刚执行的操作，忽略）。

**心跳**：服务端每 30s 发 ping，客户端回 pong。60s 无 pong 断开。

### 7.2 前端 WebSocket 连接

**连接管理**（`apps/web/src/lib/ws.ts`）：

- 进入画布页面时建立 WebSocket 连接
- 离开页面时关闭连接
- 断线自动重连（指数退避：1s → 2s → 4s → 8s → 最大 30s）
- 连接状态反映在 UI（顶部导航条显示连接状态指示器：🟢 已连接 / 🔴 断开）

**事件处理**：

收到事件后，根据 type 调用对应的 React Flow editor API：

| 事件 | 前端响应 |
|---|---|
| `NodeCreated` | `editor.createNode(nodeToFlowNode(payload.node))` |
| `NodeUpdated` | `editor.updateNode({ id: shapeId, props: payload.changes })` |
| `NodeDeleted` | `editor.deleteNode(shapeId)` |
| `EdgeCreated` | 创建 custom dependency edge + Binding |
| `EdgeDeleted` | 删除 custom dependency edge |
| `GroupCreated` | 创建 group container node |
| `GroupUpdated` | 更新 group container node |
| `GroupDeleted` | 删除 group container node（保留内部 MediaFlowNode） |

**乐观 UI + WS 覆写（无需去重）**：

前端采用"REST 即时反馈 + WS 权威覆写"模式，不维护操作 ID 或去重集合：

- REST 调用成功后立即更新本地 React Flow（乐观 UI，用户零延迟感知）
- WS event 到达后始终用服务端数据覆写本地状态（幂等操作）
- 自己的操作：覆写相同数据 = React Flow 不重渲染，用户无感知
- 他人的操作：正常应用变更

WS handler 必须对每种事件做幂等处理：

```typescript
// NodeCreated — 先查再决定 create 还是 update
const shapeId = createNodeId(event.nodeId)
if (editor.getNode(shapeId)) {
  editor.updateNode({ id: shapeId, ...nodeToFlowNodePatch(event) })
} else {
  editor.createNode(nodeToFlowNode(event))
}

// NodeUpdated — 直接覆写
editor.updateNode({ id: createNodeId(event.nodeId), ...event.changes })

// NodeDeleted — node 不存在则跳过
const shapeId = createNodeId(event.nodeId)
if (editor.getNode(shapeId)) {
  editor.deleteNode(shapeId)
}
```

Edge / GroupContainer 事件同理，均做存在性检查后幂等操作。

**为什么不用 event_id 去重**：乐观 UI + 覆写模式更简单（无需后端传递 event_id、前端维护 TTL Set），且更健壮（不存在 TTL 过期、ID 丢失等边界问题）。两种方案的用户体验完全一致——用户感知不到"跳过"和"覆写相同数据"之间的区别。

### 7.3 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 6.1 | WebSocket 连接成功 | `wscat -c "ws://localhost:8888/ws/canvas?workspaceId=$WID&token=$TOKEN"` (保持连接) | 连接不断开 |
| 6.2 | 无 token 连接被拒 | `wscat -c "ws://localhost:8888/ws/canvas?workspaceId=$WID"` | 连接被关闭 |
| 6.3 | 创建节点后收到事件 | 标签页 1 wscat 监听；标签页 2 POST 创建节点 | wscat 收到 `{"type":"NodeCreated",...}` |
| 6.4 | 更新节点后收到事件 | 标签页 2 PATCH 更新节点 | wscat 收到 `{"type":"NodeUpdated",...}` |
| 6.5 | 删除节点后收到事件 | 标签页 2 DELETE 删除节点 | wscat 收到 `{"type":"NodeDeleted",...}` |
| 6.6 | 创建 edge 后收到事件 | 标签页 2 POST 创建 edge | wscat 收到 `{"type":"EdgeCreated",...}` |
| 6.7 | 心跳保活 | wscat 连接持续 90s | 连接不断开（服务端发 ping） |
| 6.8 | 后端单测通过 | `make server-test` | 退出码 0 |
| 6.9 | 前端编译通过 | `pnpm --filter @clip-anvil/web build` | 退出码 0 |
| 6.10 | 多标签页同步 | 浏览器：打开两个标签页 → 标签页 1 创建节点 → 标签页 2 实时出现 | 实时同步 |

## 8. 阶段 7：自动布局

### 8.1 技术选型

使用 **@dagrejs/dagre v2.0+**（~13.6 KB gzip）。理由：
- TypeScript 原生支持（v2.0 从 TS 重写）
- 专为 DAG 层级布局设计，算法成熟
- 轻量，不引入不必要的布局模式（对比 elkjs 400-600 KB）
- API 简单，适合一次性计算坐标后批量写入

不选 elkjs（过重）、d3-dag（接口风格差异大）、graphology-layout（无 DAG 算法）。

### 8.2 布局计算——纯前端

自动布局在前端完成，后端不参与计算。流程：

1. 前端从 canvas 数据中提取所有 nodes 和 dependency edges
2. 构建 dagre Graph，设置每个节点的实际尺寸（canvas_w × canvas_h）
3. dagre 计算布局，输出每个节点的新 (x, y) 坐标
4. 前端通过 `editor.updateNodes()` 批量更新所有 MediaFlowNode 的位置
5. 通过 `PATCH /api/nodes/batch-position` 批量持久化新坐标到后端

后端只负责存坐标，不关心坐标如何计算。这与 M1 的拖拽 → debounce persist 模式一致。

### 8.3 布局参数

```typescript
const g = new dagre.graphlib.Graph();
g.setGraph({
  rankdir: 'LR',       // 从左到右（上游素材在左，下游生成在右）
  nodesep: 40,         // 同层节点垂直间距
  ranksep: 80,         // 层间水平间距
  marginx: 20,
  marginy: 20,
});

nodes.forEach(node => {
  g.setNode(node.id, { width: node.canvas_w, height: node.canvas_h });
});

edges.forEach(edge => {
  g.setEdge(edge.from_node_id, edge.to_node_id);
});

dagre.layout(g);
```

**分组处理**：dagre 支持 compound graph（父子节点），可以将 group 设为父节点，组内节点设为子节点。dagre 会自动将同组节点聚集排列。无分组的节点作为顶层节点。

### 8.4 前端交互

**触发方式**：画布底部工具栏增加"自动整理"按钮（图标：网格排列）。

**交互流程**：
1. 用户点击"自动整理"按钮
2. 计算新布局坐标
3. 使用 `editor.animateNodes()` 将节点从当前位置平滑移动到新位置（300ms ease-out 动画）
4. 动画完成后 `PATCH /api/nodes/batch-position` 批量持久化
5. group container node 根据组内节点的新包围盒自动调整位置和尺寸

**布局方向切换**：提供两个方向选项（下拉菜单）：
- `LR`（从左到右）— 默认，适合流程图思维
- `TB`（从上到下）— 适合层级关系展示

### 8.5 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 7.1 | dagre 依赖安装 | `pnpm --filter @clip-anvil/web list @dagrejs/dagre` | 已安装 |
| 7.2 | 自动布局后坐标更新 | 浏览器：创建 4 个节点 + 3 条 dependency → 点击"自动整理" | 节点按 DAG 层级排列，无重叠 |
| 7.3 | 布局持久化 | 自动布局后刷新页面 | 节点位置保持布局后的状态 |
| 7.4 | 分组聚集 | 分组内节点布局后仍聚集在一起 | group container node 包裹组内节点 |
| 7.5 | 方向切换 | 切换 LR → TB → 点击自动整理 | 节点从上到下排列 |
| 7.6 | 前端编译通过 | `pnpm --filter @clip-anvil/web build` | 退出码 0 |

## 9. 阶段 8：拖拽上传

### 9.1 后端变更

**新增迁移** `apps/server/migrations/004_add_assets.sql`：

```sql
CREATE TABLE media_asset (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    type          TEXT NOT NULL,
    mime          TEXT NOT NULL,
    storage_url   TEXT NOT NULL,
    thumbnail_url TEXT,
    duration_ms   INT,
    size_bytes    BIGINT,
    metadata      JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_media_asset_workspace ON media_asset(workspace_id);
```

**新增 API**：

| 端点 | 方法 | 鉴权 | 请求体 | 成功响应 | 错误响应 |
|---|---|---|---|---|---|
| `/api/upload` | POST | JWT | `multipart/form-data`：`file` + `workspace_id` | `200 {asset}` | `400` 文件类型不支持 / `413` 文件过大 |

**Handler 实现**（`internal/api/upload_handler.go`）：

1. 解析 multipart 文件
2. 校验文件类型（MIME 白名单）和大小（上限 100MB）
3. 根据 MIME 判断 media_type：
   - `image/*` → `image`
   - `video/*` → `video`
   - `audio/*` → `audio`
   - 其他 → 拒绝（400）
4. 上传文件到 MinIO（bucket: `workspace-{id}`，key: `assets/{uuid}/{filename}`）
5. 图片类型：使用 MinIO 返回的 URL 作为 `storage_url`
6. 写入 `media_asset` 表
7. 返回完整 asset 数据（含 `storage_url`）

**MIME 白名单**：

| 类别 | 允许的 MIME |
|---|---|
| image | `image/jpeg`, `image/png`, `image/webp`, `image/gif` |
| video | `video/mp4`, `video/quicktime`, `video/webm` |
| audio | `audio/mpeg`, `audio/wav`, `audio/aac`, `audio/ogg` |

**缩略图**：M2 阶段不做服务端缩略图生成。图片类型直接使用原图 URL 作为缩略图（前端 CSS 裁切），视频/音频显示占位图。

### 9.2 前端拖拽上传

**交互流程**：

1. 用户从系统文件管理器拖拽文件到画布区域
2. 画布出现蓝色半透明遮罩 + "释放以上传" 提示
3. 释放文件 → 检查文件类型
4. 上传到 `POST /api/upload`（显示 toast 进度 "上传中..."）
5. 上传成功后：
   - `POST /api/nodes` 创建对应类型的节点（`asset_id` 关联刚上传的 asset）
   - 节点位置为鼠标释放时的画布坐标
   - image 类型：MediaFlowNode 显示上传图片的缩略图
   - video/audio 类型：MediaFlowNode 显示占位图 + 文件名

**多文件上传**：支持一次拖入多个文件，每个文件独立上传 + 创建节点，节点水平排列（每个偏移 canvas_w + 20px）。

**实现方式**：
- 监听画布容器的 `dragover` / `dragleave` / `drop` 事件
- `drop` 事件中通过 `DataTransfer.files` 获取文件列表
- 使用 `editor.screenToPage(point)` 将鼠标坐标转为画布坐标

### 9.3 media_node 关联 asset

M1 的 `media_node` 表需要有 `asset_id` 字段（在 M1 迁移中按 database-design.md 已包含）。创建节点时如果关联了 asset：

- `POST /api/nodes` 请求体新增可选字段 `asset_id`
- 节点创建后前端从 asset 的 `storage_url` 渲染缩略图
- MediaFlowNode 的 props 已有 `thumbnailUrl` 字段，从 node 的关联 asset 获取

### 9.4 验收标准

| # | 验收项 | 自动化命令 | 期望结果 |
|---|---|---|---|
| 8.1 | 迁移成功 | `make migrate-up` | media_asset 表创建 |
| 8.2 | 上传图片 | `curl -sf -X POST localhost:8888/api/upload -H "Authorization: Bearer $TOKEN" -F "file=@test.jpg" -F "workspace_id=$WID" \| jq -e '.id'` | 输出 UUID |
| 8.3 | MIME 校验 | 上传 `.txt` 文件 | HTTP 400 |
| 8.4 | 文件大小校验 | 上传 >100MB 文件 | HTTP 413 |
| 8.5 | MinIO 存储 | 上传后通过 `storage_url` 可访问文件 | HTTP 200 |
| 8.6 | 拖拽创建节点 | 浏览器：拖拽 JPG 到画布 → 创建 image 节点 | 节点显示上传图片缩略图 |
| 8.7 | 多文件上传 | 浏览器：拖拽 3 个文件 → 创建 3 个节点 | 3 个节点水平排列 |
| 8.8 | 后端单测通过 | `make server-test` | 退出码 0 |
| 8.9 | 前端编译通过 | `pnpm --filter @clip-anvil/web build` | 退出码 0 |

## 10. 端到端集成验收脚本

```bash
#!/bin/bash
set -e

BASE="http://localhost:8888"

echo "=== 前置：使用 M1 的 auth 获取 token ==="
TOKEN=$(curl -sf -X POST $BASE/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"e2e@test.com","password":"123456"}' | jq -r '.token')
[ -n "$TOKEN" ] && [ "$TOKEN" != "null" ] || {
  echo "登录失败，先注册"
  TOKEN=$(curl -sf -X POST $BASE/api/auth/register \
    -H 'Content-Type: application/json' \
    -d '{"email":"e2e-m2@test.com","password":"123456","name":"E2E-M2"}' | jq -r '.token')
}
echo "TOKEN=$TOKEN"

WID=$(curl -sf -X POST $BASE/api/workspaces \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"M2测试项目"}' | jq -r '.id')
echo "WID=$WID"

echo ""
echo "=== 阶段 1: 多类型节点 ==="
NID_TEXT=$(curl -sf -X POST $BASE/api/nodes \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","node_type":"text","title":"脚本","canvas_x":0,"canvas_y":0}' | jq -r '.id')
NID_IMG=$(curl -sf -X POST $BASE/api/nodes \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","node_type":"image","title":"产品图","canvas_x":250,"canvas_y":0}' | jq -r '.id')
NID_VID=$(curl -sf -X POST $BASE/api/nodes \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","node_type":"video","title":"镜头01","canvas_x":500,"canvas_y":0}' | jq -r '.id')
NID_AUD=$(curl -sf -X POST $BASE/api/nodes \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","node_type":"audio","title":"BGM","canvas_x":0,"canvas_y":250}' | jq -r '.id')

TYPES=$(curl -sf $BASE/api/workspaces/$WID/canvas \
  -H "Authorization: Bearer $TOKEN" | jq -c '[.nodes[].node_type] | sort | unique')
[ "$TYPES" = '["audio","image","text","video"]' ] && echo "✅ 1.1 四种类型节点创建成功"

VID_W=$(curl -sf $BASE/api/nodes/$NID_VID \
  -H "Authorization: Bearer $TOKEN" | jq '.canvas_w')
[ "$VID_W" = "240" ] && echo "✅ 1.2 video 默认宽度 240"

AUD_H=$(curl -sf $BASE/api/nodes/$NID_AUD \
  -H "Authorization: Bearer $TOKEN" | jq '.canvas_h')
[ "$AUD_H" = "80" ] && echo "✅ 1.3 audio 默认高度 80"

echo ""
echo "=== 阶段 2: 连线 + DAG ==="
make migrate-up 2>/dev/null

# dependency: 产品图 → 镜头01
EID_1=$(curl -sf -X POST $BASE/api/edges \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","from_node_id":"'$NID_IMG'","to_node_id":"'$NID_VID'"}' | jq -r '.id')
[ -n "$EID_1" ] && [ "$EID_1" != "null" ] && echo "✅ 2.1 创建 dependency edge"

# 环检测：镜头01 → 产品图 应被拒绝（DAG 不允许环）
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X POST $BASE/api/edges \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","from_node_id":"'$NID_VID'","to_node_id":"'$NID_IMG'"}')
[ "$HTTP" = "422" ] && echo "✅ 2.2 环检测拒绝"

# 重复连线拒绝
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X POST $BASE/api/edges \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","from_node_id":"'$NID_IMG'","to_node_id":"'$NID_VID'"}')
[ "$HTTP" = "409" ] && echo "✅ 2.3 重复连线拒绝"

# 自连接拒绝
HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X POST $BASE/api/edges \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","from_node_id":"'$NID_IMG'","to_node_id":"'$NID_IMG'"}')
[ "$HTTP" = "400" ] && echo "✅ 2.4 自连接拒绝"

# canvas 包含 edges
EDGE_COUNT=$(curl -sf $BASE/api/workspaces/$WID/canvas \
  -H "Authorization: Bearer $TOKEN" | jq '.edges | length')
[ "$EDGE_COUNT" -ge 1 ] && echo "✅ 2.5 canvas 包含 edges"

# 新增第二条 edge 再删除
EID_2=$(curl -sf -X POST $BASE/api/edges \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","from_node_id":"'$NID_TEXT'","to_node_id":"'$NID_VID'"}' | jq -r '.id')
curl -sf -o /dev/null -w '' -X DELETE $BASE/api/edges/$EID_2 \
  -H "Authorization: Bearer $TOKEN"
EDGE_COUNT=$(curl -sf $BASE/api/workspaces/$WID/canvas \
  -H "Authorization: Bearer $TOKEN" | jq '.edges | length')
[ "$EDGE_COUNT" = "1" ] && echo "✅ 2.6 删除 edge"

echo ""
echo "=== 阶段 3: 分组 ==="

# 创建分组
GID=$(curl -sf -X POST $BASE/api/groups \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","name":"分镜组","node_ids":["'$NID_VID'","'$NID_AUD'"]}' | jq -r '.id')
[ -n "$GID" ] && [ "$GID" != "null" ] && echo "✅ 3.1 创建分组"

# canvas 包含 groups
GROUP_NODES=$(curl -sf $BASE/api/workspaces/$WID/canvas \
  -H "Authorization: Bearer $TOKEN" | jq '.groups[0].node_ids | length')
[ "$GROUP_NODES" = "2" ] && echo "✅ 3.2 分组包含 2 个节点"

# 更新分组成员
curl -sf -o /dev/null -X PUT $BASE/api/groups/$GID/nodes \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"node_ids":["'$NID_VID'"]}'
GROUP_NODES=$(curl -sf $BASE/api/workspaces/$WID/canvas \
  -H "Authorization: Bearer $TOKEN" | jq '.groups[0].node_ids | length')
[ "$GROUP_NODES" = "1" ] && echo "✅ 3.3 更新分组成员"

# 删除分组（不删节点）
curl -sf -o /dev/null -X DELETE $BASE/api/groups/$GID \
  -H "Authorization: Bearer $TOKEN"
NODE_COUNT=$(curl -sf $BASE/api/workspaces/$WID/canvas \
  -H "Authorization: Bearer $TOKEN" | jq '.nodes | length')
[ "$NODE_COUNT" = "4" ] && echo "✅ 3.4 删除分组不删节点"

echo ""
echo "=== 阶段 5: 属性面板相关 API ==="

# 获取节点上游输入（产品图 → 镜头01，所以镜头01 的 inputs 包含产品图）
INPUT_COUNT=$(curl -sf $BASE/api/nodes/$NID_VID/inputs \
  -H "Authorization: Bearer $TOKEN" | jq 'length')
[ "$INPUT_COUNT" = "1" ] && echo "✅ 5.1 获取节点上游输入"

# 无上游输入返回空数组
INPUT_COUNT=$(curl -sf $BASE/api/nodes/$NID_IMG/inputs \
  -H "Authorization: Bearer $TOKEN" | jq 'length')
[ "$INPUT_COUNT" = "0" ] && echo "✅ 5.2 无上游输入返回空数组"

# 更新 Prompt 带 @引用
PROMPT=$(curl -sf -X PATCH $BASE/api/nodes/$NID_VID \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"prompt":"微距拍摄 @产品图 的拉花"}' | jq -r '.prompt')
echo "$PROMPT" | grep -q "@产品图" && echo "✅ 5.3 Prompt 带 @引用保存成功"

echo ""
echo "=== 阶段 6: WebSocket ==="

# 在后台启动 wscat 监听
WS_LOG=$(mktemp)
wscat -c "ws://localhost:8888/ws/canvas?workspaceId=$WID&token=$TOKEN" > "$WS_LOG" 2>/dev/null &
WS_PID=$!
sleep 1

# 创建节点触发事件
curl -sf -X POST $BASE/api/nodes \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"workspace_id":"'$WID'","node_type":"text","title":"WS测试","canvas_x":0,"canvas_y":500}' > /dev/null
sleep 1

# 检查 wscat 是否收到事件
kill $WS_PID 2>/dev/null || true
grep -q "NodeCreated" "$WS_LOG" && echo "✅ 6.1 WebSocket 收到 NodeCreated 事件"
rm -f "$WS_LOG"

echo ""
echo "=== 阶段 8: 拖拽上传 ==="
make migrate-up 2>/dev/null

# 上传图片（需要准备测试图片 test.jpg）
if [ -f "test.jpg" ]; then
  ASSET_ID=$(curl -sf -X POST $BASE/api/upload \
    -H "Authorization: Bearer $TOKEN" \
    -F "file=@test.jpg" -F "workspace_id=$WID" | jq -r '.id')
  [ -n "$ASSET_ID" ] && [ "$ASSET_ID" != "null" ] && echo "✅ 8.1 上传图片成功"

  # MIME 校验
  HTTP=$(curl -s -o /dev/null -w '%{http_code}' -X POST $BASE/api/upload \
    -H "Authorization: Bearer $TOKEN" \
    -F "file=@README.md" -F "workspace_id=$WID")
  [ "$HTTP" = "400" ] && echo "✅ 8.2 MIME 校验拒绝非媒体文件"
else
  echo "⏭ 跳过上传测试（无 test.jpg 文件）"
fi

echo ""
echo "=== 编译与测试 ==="
make server-test && echo "✅ 后端单测通过"
pnpm --filter @clip-anvil/web build && echo "✅ 前端编译通过"

echo ""
echo "=== M2 全部通过 ==="
```

## 11. 技术备注

### 11.1 新增后端文件

```
apps/server/
├── migrations/
│   ├── 001_init_schema.sql        ← M1 已有
│   ├── 002_add_edges.sql          ← 新增：edge 表（含 edge_type）
│   ├── 003_add_groups.sql         ← 新增：group 表 + media_node.group_id
│   └── 004_add_assets.sql         ← 新增：media_asset 表
├── sqlc/queries/
│   ├── account.sql                ← M1 已有
│   ├── workspace.sql              ← M1 已有
│   ├── canvas.sql                 ← M1 已有
│   ├── node.sql                   ← 扩展：group_id / asset_id 更新
│   ├── edge.sql                   ← 新增
│   ├── group.sql                  ← 新增
│   └── asset.sql                  ← 新增
└── internal/
    ├── api/
    │   ├── auth_handler.go        ← M1 已有
    │   ├── workspace_handler.go   ← M1 已有
    │   ├── canvas_handler.go      ← 扩展：返回 edges + groups
    │   ├── node_handler.go        ← 扩展：多类型默认值 + group_id + asset_id + /inputs
    │   ├── edge_handler.go        ← 新增：CRUD + 环检测（SERIALIZABLE 事务）
    │   ├── group_handler.go       ← 新增：CRUD + 成员管理
    │   ├── upload_handler.go      ← 新增：文件上传 + MinIO 存储
    │   └── ws_hub.go              ← 新增：WebSocket 连接池 + 广播
    └── ...
```

### 11.2 新增前端文件

```
apps/web/src/
├── components/
│   ├── ...                        ← M1 已有
│   ├── ResourceTree.tsx           ← 新增：左侧资源树
│   ├── PropertyPanel.tsx          ← 新增：右侧属性面板
│   ├── NodePropertyPanel.tsx      ← 新增：节点属性子面板
│   ├── EdgePropertyPanel.tsx      ← 新增：连线属性子面板
│   ├── PromptReact Flow.tsx           ← 新增：带 @引用的 Prompt 编辑器
│   ├── InputReferences.tsx        ← 新增：上游依赖缩略图 chips
│   ├── ConnectionStatus.tsx       ← 新增：WebSocket 连接状态指示器
│   └── FileDropZone.tsx           ← 新增：拖拽上传遮罩层
├── lib/
│   ├── api.ts                     ← 扩展：edge/group/upload API 函数
│   ├── canvas.ts                  ← 扩展：edgeToFlowEdge + groupContainerToFlowNode
│   ├── layout.ts                  ← 新增：dagre 自动布局计算
│   └── ws.ts                      ← 新增：WebSocket 连接管理 + 事件处理
├── nodes/
│   ├── MediaFlowNode.tsx         ← 扩展：四种类型渲染 + 端口 handle + 缩略图
│   └── GroupFlowNode.tsx ← 新增：自定义分组容器 node
└── pages/
    └── CanvasPage.tsx             ← 扩展：三栏布局 + WebSocket + 拖拽上传
```

### 11.3 Go 依赖新增

```
github.com/hertz-contrib/websocket  — Hertz WebSocket 支持
```

### 11.4 前端依赖新增

```
@dagrejs/dagre  — DAG 自动布局计算
```

## 12. 与设计文档的关系

M2 实现覆盖以下设计文档的核心内容：

| 设计文档 | M2 覆盖的章节 |
|---|---|
| [design-canvas.md](../../../design/canvas.md) | §2 投影层类型、§3 节点卡片视觉规格、§4 连线视觉规格、§5 分组视觉规格、§6 自动布局、§7 数据通路 |
| [design-studio-mode.md](../../../design/studio-mode.md) | §3 资源树、§5 创建资源（右键菜单 + 拖拽上传）、§6 编辑资源、§7 连线（单一 dependency）、§8 分组、§12 面板联动 |
| [database-design.md](../../../engineering/database.md) | §2.6 media_asset、§2.7 media_group、§2.9 media_edge |
| [architecture.md](../../../engineering/architecture.md) | WebSocket 双通道（M2 只做 /ws/canvas）、MinIO 文件存储 |

**M2 未覆盖的设计文档内容**（留后续）：
- design-studio-mode.md §4 浮动工具栏、§9 生成流程、§10 节点状态机（running/succeeded/failed）、§11 Stale 处理
- design-canvas.md §4.1 reference/sequence 连线类型（schema 已就绪，API 未开放）
- design-agent-mode.md 全部
- database-design.md §2.10-§2.13（生成任务、版本、评审、Agent 审计）

M2 完成后，Studio 画布作为编辑器已经完整可用——支持多类型节点、依赖连线、分组管理、资源树导航、属性编辑、实时同步、自动布局和文件上传。下一个里程碑应接入 AI 生成能力（DashScope 集成 + GenerationJob + 版本管理），让画布从"编辑器"变成"生成平台"。
