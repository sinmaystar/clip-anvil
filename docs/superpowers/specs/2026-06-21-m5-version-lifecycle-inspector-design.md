# M5 Version Lifecycle and Inspector Preview 设计

**状态**：待评审
**日期**：2026-06-21
**阶段目标**：把 Studio 的 `artifact_version` 从“成功后才出现的产物记录”升级为“每次运行立即创建、可跟踪状态、可预览历史素材的版本槽位”，并重构 Inspector 的版本交互，让用户能在运行中、成功、失败和历史版本之间清晰决策。

## 1. 背景

M5 已经支持 Studio 手动运行节点、查看 `latest_job`、查看 `versions`，也支持把某个 version 设为 current。当前实现中：

- 点击运行会立即创建 `generation_job`。
- `artifact_version` 只在 provider 成功、asset 持久化完成后创建。
- 运行中的尝试只出现在 Latest Job，不出现在 Versions。
- 失败运行只留下 failed job，不会留下 failed version。
- Versions 列表只显示 version metadata，用户无法在 Inspector 内直接预览历史素材后再决定是否设为 current。

这导致用户心智割裂：

- 用户点击运行后，看不到“正在生成的新版本”。
- Job 和 Version 不是一一绑定，运行历史与素材历史需要跨两个区域理解。
- 用户只能看到历史 version 行，不能直接回看历史图片/视频/文本内容，难以判断是否切换 current。

本 spec 聚焦 Studio 手动生产体验，不引入 Agent 自动重跑策略，不改变底层 provider 能力。

## 2. 当前实现校准

### 2.1 数据模型现状

当前核心表来自 `007_m4_1_production_foundation.sql`：

- `generation_job`
  - 有 `status`、`progress`、`intent`、`rendered_prompt`、`provider_request`、`provider_response`。
  - 表示一次模型运行尝试。
- `artifact_version`
  - 有 `job_id`、`asset_id`、`version_no`、`winner`、`output`、`input_hash`。
  - 没有 `status`、`error_code`、`error_message`、`started_at`、`completed_at`。
  - 当前只在成功持久化 asset 后创建。
- `media_node.current_version_id`
  - 指向当前被选中的 version。
  - 下游节点运行时会读取上游节点的 `current_version_id`，再取对应 `artifact_version.asset_id` 和 `media_asset`。

### 2.2 后端生命周期现状

- `SubmitNodeRun` 创建 queued `generation_job`，不创建 `artifact_version`。
- `ProductionRunner` 执行 provider 并更新 job progress。
- provider 成功后，`persistQueuedJobSuccess` 创建 `media_asset` 和 `artifact_version`，再更新 `media_node.current_version_id`。
- provider 失败后，只标记 job failed，不创建 version。
- `SelectArtifactVersion` 只允许已有 version 被设为 current，并传播下游 stale。

### 2.3 前端交互现状

- Inspector 显示 `latest_job` 和 `versions`，但两者分区独立。
- `versions` 是平铺列表，缺少素材预览。
- 单击历史 version 的主要动作是“设为当前”，用户没有足够上下文判断。
- 运行中的 job 不会占据一个 version 位置，用户感知不到“新版本正在生成”。

## 3. 产品原则

### 3.1 Version 是用户理解生产历史的主线

用户不应该同时理解“job 历史”和“version 历史”两套平行概念。Inspector 中的主线应是 Versions：

- 每次运行产生一个 version slot。
- 这个 slot 从 queued/running 到 succeeded/failed。
- 成功 slot 绑定最终素材。
- 失败 slot 绑定错误和 provider response。
- current 是某个 succeeded version 上的标记。

Job 仍是后端执行和审计实体，但在 UI 中降级为 version 的技术详情。

### 3.2 Current 只指向可用产物

`media_node.current_version_id` 只允许指向 `status=succeeded` 且有可用 asset/text 的 version。

运行中或失败 version 可以被预览、查看详情、重试，但不能设为 current，也不能作为下游模型输入。

### 3.3 一次运行对应一个用户可见版本

同一次 `generation_job` 和同一个 `artifact_version` 应一一绑定。用户点击运行后立即看到新 version，是这次运行的可见容器。

推荐约束：

- 对 production-generated versions，`artifact_version.job_id` 必须非空。
- `artifact_version.job_id` 应唯一。
- 若未来支持手动上传或人工编辑 version，可以显式使用不同 source/type，而不是复用 generation job 语义。

### 3.4 历史版本必须可预览再决策

用户切换 current 前，需要能回看历史素材：

- Text：Markdown / 文本预览。
- Image：图片预览。
- Video：可播放视频 + poster。
- Audio：后续音频播放器。
- Failed：错误摘要 + provider request/response。
- Running：进度、已知 provider task id、排队/运行状态。

## 4. 范围

### 4.1 包含

- 扩展 `artifact_version` 生命周期，支持 queued / running / succeeded / failed / cancelled。
- 点击运行后立即创建 pending version，并与 generation job 绑定。
- 异步 job progress 同步到对应 version。
- 成功后补齐 version 的 `asset_id`、`output`、`input_hash`，并自动成为 current。
- 失败后保留 failed version，用于审计、错误查看和重试入口。
- Inspector 版本交互重构：
  - 当前版本预览区。
  - 版本时间线/列表。
  - 历史版本预览模式。
  - “设为当前”只对 succeeded 非 current version 可用。
- 后端 API 返回 version status、job summary、asset preview。
- E2E 覆盖运行中版本、成功版本、失败版本、历史预览和 current 切换。

### 4.2 不包含

- 不实现自动重跑下游节点。
- 不实现版本 diff。
- 不实现多版本 A/B 评分自动选择。
- 不实现分支版本树；本阶段仍是按 `version_no` 排序的线性版本历史。
- 不把 failed/running version 作为下游输入。
- 不改变 provider 调用参数语义。
- 不把 `generation_job` 从数据库中移除；job 仍保留用于运行、审计、sandbox 关联和重试。

## 5. 推荐方案

推荐采用“Version-first lifecycle”方案：

```text
Run clicked
  -> create generation_job(status=queued)
  -> create artifact_version(status=queued, job_id=job.id, version_no=N, winner=false)
  -> return job + version
  -> Inspector immediately shows vN queued
  -> Runner marks job/version running/progress
  -> Success:
       persist media_asset
       update artifact_version(status=succeeded, asset_id, output, input_hash)
       clear old winner
       mark vN winner=true
       media_node.current_version_id = vN
       propagate stale
  -> Failure:
       artifact_version.status=failed
       artifact_version.error_code/error_message/provider_response
       media_node.current_version_id unchanged
```

### 5.1 备选方案

**方案 A：保持现有数据模型，只在 UI 中把 latest job 虚拟显示成 version**

优点：改动小，无 migration。

缺点：job 和 version 仍不是同一个实体；失败运行没有 version id；刷新、重试、历史审计仍要拼接两套数据。用户看到的“运行中版本”不是真实可引用对象。

**方案 B：新建 `version_run` 表**

优点：可以严格区分产物版本和运行尝试。

缺点：概念更多，UI 仍需要解释 version/run 两层关系；当前已有 `artifact_version.job_id`，再加表会过度复杂。

**推荐：方案 C：扩展 `artifact_version`**

优点：最符合用户心智；复用现有 `job_id`、`version_no`、`winner`、`current_version_id`；不新增多余概念。

代价：需要 migration、sqlc、service lifecycle 和 Inspector 同步改造。

## 6. 数据模型设计

### 6.1 artifact_version 字段扩展

新增字段：

```sql
ALTER TABLE artifact_version
  ADD COLUMN status job_status NOT NULL DEFAULT 'succeeded',
  ADD COLUMN progress INT NOT NULL DEFAULT 100,
  ADD COLUMN error_code TEXT,
  ADD COLUMN error_message TEXT,
  ADD COLUMN provider_request JSONB NOT NULL DEFAULT '{}',
  ADD COLUMN provider_response JSONB NOT NULL DEFAULT '{}',
  ADD COLUMN started_at TIMESTAMPTZ,
  ADD COLUMN completed_at TIMESTAMPTZ;
```

说明：

- 复用 `job_status` enum，避免新增相似 enum。
- 旧数据 migration 后默认 `succeeded/progress=100`。
- `input_hash` 对 queued/running/failed 可以暂为空字符串。
- `asset_id` 对 queued/running/failed 可以为空。
- `winner=true` 只允许出现在 `status='succeeded'` 的 version 上。

### 6.2 约束与索引

建议新增：

```sql
CREATE UNIQUE INDEX idx_artifact_version_job_unique
  ON artifact_version(job_id)
  WHERE job_id IS NOT NULL;

CREATE INDEX idx_artifact_version_node_status
  ON artifact_version(node_id, status, created_at DESC);
```

推荐保留现有：

```sql
CREATE UNIQUE INDEX idx_artifact_version_one_winner
  ON artifact_version(node_id)
  WHERE winner = true;
```

应用层必须保证：

- 只有 succeeded version 可以 winner=true。
- `media_node.current_version_id` 只写 succeeded version。

如果 PostgreSQL 层要更严格，可以增加 check/trigger；首期建议先放在 service 层，降低 migration 风险。

## 7. 后端生命周期设计

### 7.1 SubmitNodeRun

当前：

```text
create generation_job queued
return job
```

目标：

```text
create generation_job queued
version_no = NextArtifactVersionNo(node)
create artifact_version:
  job_id = job.id
  status = queued
  progress = 0
  winner = false
  output = {}
  input_hash = planned input hash or ""
return job + version
```

API response `RunNodeResponse` 增加 `version`，即使还没有 asset。

### 7.2 ProductionRunner progress

Runner 每次更新 job 状态时，同步更新 version：

- `job.started` -> version `running`, `started_at`, progress。
- `provider.progress` -> version progress/provider_response。
- `job.succeeded` -> version succeeded。
- `job.failed` -> version failed/error。
- `job.cancelled` -> version cancelled/error。

同步方式可以由 production service 提供方法，例如：

```go
MarkVersionRunningByJob(ctx, jobID, progress, response)
MarkVersionProgressByJob(ctx, jobID, progress, response)
MarkVersionSucceededByJob(ctx, jobID, assetID, output, inputHash, request, response)
MarkVersionFailedByJob(ctx, jobID, err, response)
```

### 7.3 Success persistence

成功后不再新建 version，而是更新 Submit 时创建的 version：

```text
persist media_asset
load existing artifact_version by job_id
update artifact_version:
  status=succeeded
  progress=100
  asset_id=asset.id
  output={asset_id, mime, ...}
  input_hash=current input hash
  provider_request/response
  completed_at=now
clear old winners
mark this version winner=true
update media_node.current_version_id=this version
resolve stale and propagate downstream stale
```

### 7.4 Failure persistence

失败后：

```text
mark generation_job failed
update artifact_version by job_id:
  status=failed
  progress=last progress or 0
  error_code
  error_message
  provider_response
  completed_at=now
do not update media_node.current_version_id
do not clear old winner
do not propagate downstream current-version stale
```

如果失败发生在创建 version 前，需要 fallback 创建 failed version，避免孤儿 job。正常路径不应发生。

取消视为终态，但不是失败重试的同义词：

```text
mark generation_job cancelled
update artifact_version by job_id:
  status=cancelled
  progress=last progress
  error_code='cancelled'
  error_message=user/system cancellation reason
  completed_at=now
do not update media_node.current_version_id
do not clear old winner
```

### 7.5 Retry

Retry 不复用原 version。Retry 创建新的 job 和新的 version：

```text
failed v7/job7 retry
  -> create job8
  -> create v8 queued
  -> parent_job_id=job7
```

这样每次模型尝试都有独立可见记录。

## 8. API 设计

### 8.1 ArtifactVersion response

扩展：

```json
{
  "id": "...",
  "version_no": 7,
  "winner": false,
  "status": "running",
  "progress": 35,
  "job_id": "...",
  "asset": null,
  "error_code": null,
  "error_message": null,
  "provider_request": {},
  "provider_response": {},
  "created_at": "...",
  "started_at": "...",
  "completed_at": null
}
```

成功版本包含 asset 和 access URL。失败版本包含错误。运行中版本包含 progress 和 provider task id。

### 8.2 NodeProductionState

保持结构：

- `current_version`
- `versions`
- `latest_job`
- `active_stale_reasons`
- `sandbox_jobs`

但前端主线使用 `versions`。`latest_job` 仍保留作为调试信息和兼容。

### 8.3 RunNodeResponse

目标：

```json
{
  "job": { "id": "...", "status": "queued" },
  "version": { "id": "...", "version_no": 8, "status": "queued" }
}
```

如果同步 mock provider 仍直接返回成功，也应先创建 version，再更新为 succeeded，保持一致。

## 9. Inspector 交互设计

### 9.1 信息架构

Inspector 主体分为四块：

1. **Header**
   - 节点类型、标题、状态。
   - Run / Retry / Cancel 后续扩展。
2. **Current Preview**
   - 默认显示 current version。
   - 如果用户选中历史 version，则显示历史预览，并出现“正在预览 vN”提示。
3. **Prompt and Model**
   - Prompt 编辑、模型选择、参数。
   - 这是主要编辑区域。
4. **Version Timeline**
   - 紧凑显示版本列表。
   - 支持状态筛选或折叠。
   - 每个 version 行点击后切换 preview target，不直接设为 current。

调试信息放入 version detail 抽屉：

- Job summary。
- Rendered prompt。
- Provider request/response。
- Sandbox jobs。
- Stale reasons。

### 9.2 Version row 状态

每行展示：

```text
v8  Running  35%     created 10:31
v7  Current  image   2048x2048
v6  Failed   provider timeout
v5           video   5s 720p
```

视觉规则：

- Current：高亮 badge。
- Running/Queued：进度条或 spinner。
- Failed：红色错误摘要。
- Succeeded 非 current：显示 asset 类型、尺寸/时长。

### 9.3 历史预览模式

点击 version row：

- Inspector 顶部 Preview 切换到该 version。
- 不改变 `current_version_id`。
- 如果 version 是 succeeded 且不是 current，显示“设为当前”按钮。
- 如果 version 是 failed，显示“重试此版本”按钮。
- 如果 version 是 running/queued，显示进度和 provider task id。

退出历史预览：

- 用户点击“回到当前版本”。
- 或选择当前 version row。

### 9.4 设为当前

“设为当前”只对 succeeded 非 current version 可用。

点击后：

- 调 `POST /nodes/:id/versions/:version_id/select`。
- 后端更新 winner/current。
- 下游节点 stale 更新。
- Inspector 回到 current preview。

### 9.5 运行中版本

点击 Run 后：

- 版本列表立即出现新行 `vN queued`。
- Current preview 可以保持旧 current，但顶部显示“vN 正在生成”提示；或自动切到 running version preview。
- 推荐自动切到 running version preview，因为用户刚刚触发运行，最关注新尝试。
- 旧 current 不丢失，仍标记 Current。

成功后：

- vN 变 succeeded/current。
- Preview 显示新产物。
- 旧 current 变普通 succeeded version。

失败后：

- vN 变 failed。
- Preview 显示错误。
- current 仍是旧版本。

## 10. 下游输入语义

下游运行时只读取上游节点的 `media_node.current_version_id`。

因此：

- 运行中的 version 不会成为下游输入。
- failed version 不会成为下游输入。
- 用户预览历史 version 不会影响下游输入。
- 只有用户点击“设为当前”或新运行成功自动成为 current 后，下游重新运行才会使用新素材。

Prompt refs 渲染保持现有规则：

- 文本 current version 的 `text_content` 替换 `@引用`。
- 图片 current version 在 prompt 中替换为 `图1`、`图2`，并通过 `InputRefs` 把 `storage_url/mime/asset_id` 传给 provider。
- 视频/音频作为输入时通过 `InputRefs` 表达，不把二进制内容展开进 prompt。

## 11. 迁移策略

1. 添加 nullable/default 字段，旧 version 自动 `status=succeeded progress=100`。
2. 为旧 version 回填 `provider_request/provider_response={}`。
3. 保留现有 version_no。
4. 不为历史 job 反向创建 version，避免伪造历史。
5. API 兼容：旧前端字段仍存在，新前端逐步使用 status/progress。
6. 完成后可增加 `job_id` unique partial index。

## 12. 测试计划

### 12.1 后端单元测试

- RunNode 创建 job 时同时创建 queued version。
- Runner started/progress 同步 version status/progress。
- 成功后更新已有 version，不创建第二条 version。
- 成功 version 自动 winner/current。
- 失败后 version failed，current 不变。
- Retry 创建新 version，不复用 failed version。
- Select current 只允许 succeeded version。
- 下游 input hash 使用上游 current version。

### 12.2 前端单元测试

- version row 按 status 显示 queued/running/succeeded/failed/current。
- 点击 version row 进入 preview mode，不触发 select current。
- failed version 展示 error。
- running version 展示 progress。
- succeeded image/video/text version 使用 asset preview。
- current badge 和 preview target 状态正确。

### 12.3 E2E 测试

用 mock provider：

1. 创建文本节点，运行。
2. 立即看到 `v1 queued/running`。
3. 成功后看到 `v1 current` 和文本预览。
4. 修改 prompt 再运行。
5. 立即看到 `v2 queued/running`，旧 `v1 current` 仍可预览。
6. 成功后 `v2 current`，`v1` 可历史预览。
7. 选择 `v1` 预览，不改变 current。
8. 点击“设为当前”，current 切回 `v1`，下游 stale。

失败场景：

1. 使用 mock fail 参数运行。
2. 立即创建 `vN running`。
3. 失败后 `vN failed` 留在版本列表。
4. current 仍为旧 succeeded version。
5. failed version 可展开错误和 provider response。

真实 provider smoke：

- 图像/视频长任务点击运行后 version 立即出现。
- 运行中刷新页面后 version 仍显示 running。
- 成功后同一 version 变 current，可预览素材。
- MinIO asset 和 thumbnail URL 可访问。

## 13. 风险与约束

### 13.1 Version 状态和 Job 状态可能不一致

需要把 job 和 version 更新封装在 production service 方法中，避免调用方只更新 job。

### 13.2 失败版本没有 asset

前端必须支持 `asset=null` 的 version，不要假设每个 version 都有素材。

### 13.3 current/winner 约束

必须保证 failed/running version 不能 winner。否则下游输入会读到不可用版本。

这一约束同时适用于 cancelled version。`winner=true` 和 `media_node.current_version_id` 都只允许指向 succeeded version。

### 13.4 同步 mock provider 路径

即使 provider 很快完成，也要保持“先 create queued version，再更新 succeeded”的语义，保证代码路径一致。

### 13.5 并发运行

同一节点如果允许并发运行，会出现多个 running versions。首期建议禁止同一节点并发运行：

- 如果存在 queued/running job/version，则 Run 按钮 disabled。
- 后续需要并发时再设计 winner 决策。

## 14. 验收标准

- 点击 Run 后，不等 provider 完成，Inspector versions 立刻出现新 version。
- `generation_job.id` 与 `artifact_version.job_id` 一一对应。
- Running/queued/failed versions 在刷新页面后仍可见。
- 成功后同一 version 从 running 变 succeeded/current，不新增第二个成功 version。
- 失败后 failed version 留在历史中，旧 current 不变。
- cancelled version 留在历史中，旧 current 不变，不能设为 current。
- 用户可以预览历史文本/图片/视频版本，再决定是否设为 current。
- 设为 current 只对 succeeded version 开放。
- 下游重新运行只使用上游 current version，不使用正在预览的历史 version。
- 测试覆盖 queued、running、succeeded、failed、select current、retry 和下游 input refs。
