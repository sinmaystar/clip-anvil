# M1.x-A Studio 节点基础完善规格

**状态**：待实施
**前置**：M1 Studio 画布基础已落地
**目标**：以当前 M1 代码为事实基准，先把 Studio 从 text-only 画布扩展为支持 text/image/video/audio 四种节点的稳定投影层，为后续连线、分组、上传和实时同步打基础。

## 1. 当前事实基准

当前代码状态：

- 数据库 `001_init_schema.sql` 已创建 `media_type AS ENUM ('text', 'image', 'video', 'audio')`，但 `media_node` 仍是 M1 精简版，不包含 `group_id`、`asset_id`、`model_provider`、`model_name`、`model_params`、`current_version_id`、`sort_order`。
- 后端 `POST /api/nodes` 只允许 `text`，默认尺寸统一为 `200 x 120`。
- `GET /api/workspaces/:id/canvas` 只返回 `camera` 和 `nodes`。
- 前端 `WorkspaceDetailPage` 只有右键创建文本节点、轻量左侧节点列表、节点下方内联编辑面板。
- `MediaShapeProps` 当前包含 `nodeId`、`nodeType`、`title`、`prompt`、`status`、`w`、`h`。`prompt` 暂时保留在 shape props 中，用于节点预览、内联编辑和撤销恢复。

本规格不假设 M1 已经实现完整目标态 schema。后续 spec 会分别补齐 edge、group、asset。

## 2. 范围

### 2.1 包含

- 后端放开节点类型：`text | image | video | audio`。
- 按节点类型设置默认尺寸。
- 前端右键菜单支持创建四种节点。
- `MediaShapeUtil` 按节点类型渲染不同卡片占位。
- 左侧 M1 轻量节点列表显示节点类型和状态。
- `packages/canvas-schema` 明确当前 M1.x shape 契约，新增可选缩略图字段但不要求本阶段产出真实缩略图。
- 后端和前端测试覆盖多类型节点的创建、展示和刷新持久化。

### 2.2 不包含

- 文件上传和 MinIO 资产绑定。
- `media_asset` 表和 `media_node.asset_id`。
- 连线、分组、资源树、右侧属性面板。
- 生成任务、版本管理、模型参数持久化。
- WebSocket 和跨标签页同步。

## 3. 后端设计

### 3.1 数据库

本阶段不新增迁移。现有 `media_type` enum 已包含四种节点类型。

需要避免把目标态字段提前写入查询或 DTO。如果后端代码需要返回 `thumbnail_url`，本阶段应返回空字段或不返回，前端按可选字段处理。

### 3.2 节点创建

修改 `apps/server/internal/api/node_handler.go`：

- 将 `isAllowedM1NodeType` 改为 M1.x 允许四种类型的校验函数。
- `defaultNodeSize` 改为按类型返回：

| node_type | canvas_w | canvas_h |
|---|---:|---:|
| `text` | 200 | 120 |
| `image` | 200 | 160 |
| `video` | 240 | 180 |
| `audio` | 200 | 80 |

请求体保持当前字段集合，不新增 `asset_id`。用户创建 image/video/audio 节点时得到的是 Draft 占位节点。

### 3.3 节点状态

本阶段主要使用 `draft` 和 `ready`。后端仍保留现有 `NodeStatus` 枚举校验，因为后续生成任务会使用 `queued/running/succeeded/failed/stale/user_editing`。

## 4. 前端设计

### 4.1 类型契约

修改 `packages/canvas-schema/src/index.ts`：

- 保留当前 `prompt`、`w`、`h` 字段。
- 新增可选 `thumbnailUrl?: string`，本阶段通常为空。

修改 `apps/web/src/lib/api.ts`：

- `MediaNode` 可新增可选 `thumbnail_url?: string`，本阶段后端不一定返回。
- `nodeToShapeProps` 将 `thumbnail_url` 映射为 `thumbnailUrl`。

命名规则：API DTO 使用 snake_case，shape props 使用 camelCase。

### 4.2 多类型卡片

`MediaShapeUtil` 根据 `nodeType` 渲染：

| nodeType | 内容 |
|---|---|
| `text` | Prompt 摘要，保留当前编辑体验 |
| `image` | 灰色图片占位区域；若未来有 `thumbnailUrl` 则显示图片 |
| `video` | 灰色视频占位区域 + 播放标识 + `0:00` |
| `audio` | 紧凑音频占位区域 + 波形占位 + `0:00` |

状态边框：

- `draft`：灰色虚线。
- `ready`：灰色实线。
- 其他状态保留现有色彩映射，但本阶段不主动产生。

### 4.3 创建入口

右键菜单从单一“文本节点”改为四种节点：

- 文本节点：`node_type: "text"`，默认标题“未命名文本”。
- 图片节点：`node_type: "image"`，默认标题“未命名图片”。
- 视频节点：`node_type: "video"`，默认标题“未命名视频”。
- 音频节点：`node_type: "audio"`，默认标题“未命名音频”。

创建流程仍以后端成功为准：

1. 用户在画布空白处右键选择类型。
2. 前端调用 `POST /api/nodes`。
3. 后端返回节点。
4. 前端 `editor.createShapes([nodeToShape(node)])`。
5. 失败时只显示 toast，不创建本地 shape。

### 4.4 左侧轻量列表

保留 M1 左侧栏，不在本阶段升级为完整资源树。列表行需要：

- 使用类型标识显示 text/image/video/audio。
- 点击列表行仍定位并选中对应 shape。
- 空状态文案改为提示可右键创建多类型节点。

## 5. 测试与验收

### 5.1 后端验收

```bash
make server-test
```

新增或更新测试：

- `POST /api/nodes` 创建四种类型均成功。
- `video` 默认尺寸为 `240 x 180`。
- `audio` 默认尺寸为 `200 x 80`。
- 非法 `node_type` 仍返回 `400`。
- `GET /api/workspaces/:id/canvas` 返回四种节点并保留尺寸。

### 5.2 前端验收

```bash
pnpm --filter @clip-anvil/web... build
```

浏览器验收：

- 右键菜单可创建四种节点。
- 刷新后四种节点仍显示在画布上。
- 不同节点使用不同尺寸和内容占位。
- 节点标题和 Prompt 自动保存仍可用。
- 删除和撤销恢复不破坏节点类型。

## 6. 完成条件

- 四种节点可通过 API 创建并持久化。
- 前端能正确渲染四种占位卡片。
- M1 原有 text 节点编辑、拖拽、删除、撤销恢复、camera 持久化不回退。
- 本阶段不引入 asset/group/edge 字段依赖。

## 7. 关联文档

- [M1 Studio 画布基础](./2026-06-12-m1-studio-canvas-design.md)
- [画布设计](../../design-canvas.md)
- [Studio 模式设计](../../design-studio-mode.md)
- [数据库设计](../../database-design.md)
