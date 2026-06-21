# Eino Volcengine Production Runtime 设计方案

**状态**：待评审
**日期**：2026-06-20
**阶段目标**：把当前 M4/M5 的 mock 生产链路升级为真实可调用的 Eino + 火山引擎生产运行层，让 Studio 手动运行在本阶段就能真实生成文本、图像、视频，并让 M6 Agent Worker 复用同一套基础能力。音频能力暂时 hold，保留 schema/配置扩展点但不进入本阶段真实验收。

## 1. 背景

M3-M6 roadmap 已经明确：Studio 和 Agent 不应拆成两套底层生产系统。Studio 是用户手动表达，Agent 是 Producer/Craftsman/Worker 自动表达，但两者都要复用同一套 Production Core：

- `media_node`
- `media_edge`
- `GenerationIntent`
- `ProviderBridge`，当前历史 provider 接口
- `generation_job`
- `artifact_version`
- `model_provider`
- `model_capability`
- `media_asset`
- `sandbox_job`
- stale propagation

当前实现已经具备这些数据和 UI 基础，但真实 provider 仍停在边界层：

- `mock` provider 可以生成 mock text/image/video。
- `internal_ffmpeg` provider 可以通过 Sandbox Job Service 做首帧/尾帧提取。
- `volcengine` provider 已注册，但 `Run` 仍返回 adapter not implemented。
- PostgreSQL 中只有 `volcengine/doubao-seed-1-6-lite` 的文本 capability seed。

下一步不能只是“多插几条 model_capability 数据”。要把 Eino runtime 和火山引擎官方 API 真实接起来，并确保 Studio 和 Agent 共同调用同一套生产运行层。

## 2. 设计目标

1. **真实调用火山引擎**
   - 文本生成走火山 Ark 文本模型。
   - 图像生成走火山 Ark 图像生成模型。
   - 视频生成走火山视频生成/Ark 视频能力。
   - 音频生成暂时 hold；后续拿到可用模型后再启用 `text_to_audio`。

2. **使用 Eino 作为 AI 应用运行框架**
   - 文本使用 EinoExt Ark `ChatModel`。
   - 图像使用 EinoExt Ark `ImageGenerationModel`。
   - 视频使用火山引擎官方 API，并封装成 ClipAnvil 自有 Eino component / tool / graph node wrapper。
   - 音频后续使用火山引擎音频/语音合成官方 API，以同样方式封装。
   - 如果 EinoExt 后续提供官方 video/audio model，再替换内部组件实现，不改变 Studio/Agent 调用方式；音频仍需等可用模型后启用。

3. **保留现有 Production Core**
   - `GenerationIntent` 仍是 Studio/Agent 的统一提交格式。
   - `ProviderBridge` 不作为新架构核心，只作为 M4 历史兼容接口逐步迁移。
   - 新核心是 `ProductionRunner` + `EinoProductionRuntime`。
   - 成功后仍写 `media_asset`、`artifact_version`、current winner。
   - 失败后仍写 failed `generation_job`，并可重试。

4. **所有真实模型调用异步化**
   - 真实文本、图像、视频生成都必须走 job queue / runner。
   - 后续启用真实音频时也必须走同一异步 runner，不允许新增同步捷径。
   - HTTP `RunNode` 只创建 job 并返回 `job_id`，不等待 provider 完成。
   - 异步化不仅用于用户体验，也用于容器资源利用率、并发控制、重试、取消和成本治理。
   - Agent 模式必须把 Eino stream、工具调用、job 状态和最终 artifact 统一输出为用户可感知事件。

5. **可配置、可审计、可测试**
   - 模型展示来自 `model_capability`。
   - provider secret 来自 `.env` / environment，不写入数据库。
   - provider request / response summary 要落库，但必须脱敏。
   - 自动测试默认不依赖真实火山 key；真实 E2E 通过显式环境变量开启。

## 3. 非目标

- 不在本阶段实现完整 M6 Producer/Craftsman 对话系统。
- 不把 Eino Agent runtime、checkpoint、HITL 一次性纳入本阶段；但生产运行层必须按 Eino-first 设计，避免 M6 重写。
- 不让浏览器直接携带 API key 调火山引擎；前端直连 ClipAnvil 后端，后端用 Eino + 火山 SDK/API 调火山。
- 不把 API key 存入 `model_provider.config` 或 `workspace.settings`。
- 不用 mock 假装真实调用成功；mock 只保留为本地测试 provider。

## 4. 核心架构

```text
Studio UI / Agent Worker
        |
        v
Production Service
  - load media_node
  - build GenerationIntent
  - load input refs
  - validate model_capability
  - create generation_job
  - enqueue production run
        |
        v
ProductionRunner
  - concurrency control
  - retry / cancel / timeout
  - progress event emit
        |
        v
EinoProductionRuntime
  - operation router
  - Eino graph/component runtime
  - Ark text/image adapters
  - Volcengine official video API wrappers
  - deferred audio API wrapper boundary
        |
        v
ProductionOutput
        |
        v
media_asset + artifact_version + current winner + stale propagation
```

### 4.1 Production Service 仍是唯一生产入口

Studio 点击“运行节点”和未来 Agent Worker 都只调用 production service，不直接构造火山请求。

Studio 路径：

```text
PropertyPanel Run
  -> POST /api/nodes/:id/run
  -> production.Service.SubmitNodeRun
  -> GenerationIntent
  -> generation_job(status=queued)
  -> return job_id
  -> ProductionRunner
  -> EinoProductionRuntime
  -> Volcengine official API
  -> job events + artifact_version
```

Agent 路径：

```text
Craftsman
  -> dispatch_worker / generate_image tool
  -> production.Service.SubmitGenerationIntent
  -> generation_job(status=queued)
  -> Worker observes job stream
  -> ProductionRunner
  -> EinoProductionRuntime
  -> job events + artifact_version
```

这保证 Studio 和 Agent 上层表达不同，但底层模型、版本、失败、重试、stale 逻辑一致。

### 4.2 Eino 是生产运行层核心

Eino 不应被隐藏成一个 provider 内部实现细节。未来 Agent 模式也基于 Eino，因此本阶段要把 Eino 作为生产运行层核心，只把火山 SDK/API 的供应商细节隔离在具体 component 内。

建议新增核心边界：

```go
type EinoProductionRuntime interface {
    Start(ctx context.Context, job ProductionJob, intent GenerationIntent) (<-chan ProductionEvent, error)
}
```

`ProductionEvent` 至少覆盖：

- `job.started`
- `model.stream_delta`
- `provider.task_created`
- `provider.progress`
- `asset.downloading`
- `asset.uploading`
- `job.succeeded`
- `job.failed`
- `job.cancelled`

内部按 operation 分发：

| operation | Eino runtime |
|---|---|
| `text_generation` | EinoExt Ark `ChatModel` |
| `text_to_image` | EinoExt Ark `ImageGenerationModel` |
| `image_to_image` | Eino graph: input image normalization + Ark image model |
| `multi_image_to_image` | Eino graph: reference pack/input refs normalization + Ark image model |
| `text_to_video` | ClipAnvil Eino component wrapping Volcengine video official API |
| `image_to_video` | ClipAnvil Eino component wrapping Volcengine video official API |
| `multi_reference_to_video` | ClipAnvil Eino component wrapping Volcengine video official API |
| `text_to_audio` | deferred; capability disabled until a usable Volcengine audio/TTS model is available |

如果 EinoExt 后续提供官方 video/audio component，替换 ClipAnvil 自有 component，不改变 `GenerationIntent`、job API 或 Studio/Agent 调用方式。音频 capability 在拿到可用模型前保持 disabled。

### 4.3 ProviderBridge 的去留

`ProviderBridge` 当前代码只有一个同步方法：

```go
type ProviderBridge interface {
    Run(ctx context.Context, intent GenerationIntent) (ProviderResult, error)
}
```

它在 M4 的价值是快速建立 mock provider、Volcengine adapter boundary 和内部 FFmpeg provider。但第一期真实 provider 只接火山引擎，并且真实模型调用全部异步化后，这个同步 provider 接口不应继续扩展为新核心。

处理策略：

- 不新增新的 `ProviderBridge` 实现作为主路径。
- 保留 `ProviderBridge` 给 mock、历史测试和内部 FFmpeg 兼容。
- 新真实火山路径走 `ProductionRunner -> EinoProductionRuntime -> Volcengine components`。
- 后续如果确实引入第二个外部供应商，也优先在 Eino component 层扩展，而不是恢复 provider-centric 架构。

## 5. Model Capability 与配置

### 5.1 数据库决定“可选项”

`model_provider` 和 `model_capability` 仍是前端可选模型的事实源。

`model_provider.config` 只允许放非敏感配置：

```json
{
  "region": "cn-beijing",
  "base_url": "https://ark.cn-beijing.volces.com/api/v3",
  "docs": "volcengine ark"
}
```

`model_capability.defaults` 用于 UI 默认参数和 provider 默认参数：

```json
{
  "temperature": 0.2,
  "max_tokens": 2048,
  "size": "2048x2048",
  "response_format": "url",
  "duration_sec": 5,
  "aspect_ratio": "16:9"
}
```

### 5.2 环境变量决定“真实凭证”

`.env` / environment 中保留 secret：

```bash
CLIPANVIL_PRODUCTION_PROVIDER_MODE=real
CLIPANVIL_PRODUCTION_DEFAULT_PROVIDER=volcengine
CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY=volcengine-api-key-from-local-env
CLIPANVIL_PRODUCTION_VOLCENGINE_BASE_URL=https://ark.cn-beijing.volces.com/api/v3
CLIPANVIL_PRODUCTION_VOLCENGINE_TEXT_MODEL=doubao-seed-2-0-mini-260428
CLIPANVIL_PRODUCTION_VOLCENGINE_IMAGE_MODEL=doubao-seedream-5-0-260128
CLIPANVIL_PRODUCTION_VOLCENGINE_VIDEO_MODEL=doubao-seedance-1-0-pro-fast-251015
CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_MODEL=
```

需要扩展 config：

- `production.volcengine.region`
- `production.volcengine.audio_model`
- `production.volcengine.request_timeout_seconds`
- `production.volcengine.poll_interval_seconds`
- `production.volcengine.max_poll_seconds`

### 5.3 不允许只靠手改数据库完成上线

开发时可以用 SQL 临时插入 capability，但项目默认配置必须通过 migration seed 固化。

建议新增 migration：

```text
013_real_volcengine_capabilities.sql
```

它负责：

- upsert `volcengine` provider。
- upsert text/image/video capability as enabled when adapters are implemented。
- upsert audio capability as disabled until a usable model is available。
- 默认只启用已有 adapter 覆盖的 capability。
- 对尚未实现的 operation 保持 `enabled=false`，避免 UI 展示不能运行的模型。

## 6. 输出类型设计

### 6.1 Text

文本生成使用 EinoExt Ark `ChatModel`。

输入：

- system prompt：ClipAnvil production system instruction。
- user message：rendered prompt。
- optional multimodal references：只有模型 capability 声明支持时才传。

输出：

- `ProductionOutput.TextContent`
- `ProductionOutput.RenderedPrompt`
- request summary 包含 model、operation、参数摘要、input refs 摘要。
- response summary 包含 Ark request id、token usage、finish reason、原始响应摘要。

文本结果继续写 `media_asset(type=text)` 和 `artifact_version`。

### 6.2 Image

图像生成使用 EinoExt Ark `ImageGenerationModel`。

输入来源：

- prompt text
- explicit `@` image refs
- implicit dependency image refs
- reference pack member winners

首版要求：

- 支持 `text_to_image`。
- 支持单图参考和多图参考时，先统一归一化成 provider input refs。
- 如果底层 Ark image component 不支持某种输入形态，对应 `model_capability.supported_operations` 不启用。

输出处理：

- provider 返回 URL 时，由后端下载并上传到 MinIO。
- provider 返回 base64 时，由后端解码并上传到 MinIO。
- 不把火山临时 URL 作为长期 `media_asset.storage_url`。
- 保存 MIME、size、provider metadata。

### 6.3 Video

视频生成必须按异步任务设计。

原因：

- 视频生成耗时长。
- HTTP 请求不应阻塞到视频完成。
- Agent Producer 不应同步等待。
- 未来需要进度、取消、重试和事件唤醒。

建议语义：

```text
POST /api/nodes/:id/run
  -> create generation_job(status=queued/running)
  -> enqueue production_run
  -> return job immediately

production runner
  -> build GenerationIntent
  -> call Volcengine video task create
  -> poll task status
  -> download result
  -> upload to MinIO
  -> create artifact_version
  -> mark job succeeded/failed
  -> broadcast canvas/job event
```

ProviderResponse 必须记录：

- provider task id
- request id
- poll attempts
- final status
- result asset summary
- redacted raw error

实现方式：

- 使用火山引擎视频生成官方 API 作为底层 provider 能力。
- 在 ClipAnvil 内部封装为 Eino component / graph node。
- component 负责创建 provider task、规范化入参、解析状态和输出。
- runner 负责队列、polling、取消、超时、失败落库、下载上传和事件广播。
- 不把火山任务 API 调用散落在 API handler 或前端代码中。

### 6.4 Audio Hold

音频首版原计划定义为 `text_to_audio`，用于旁白/语音合成。但当前没有可用火山音频模型，因此本阶段 hold。

本阶段要求：

- 保留 `audio_model` 配置字段，默认空字符串。
- 保留 `text_to_audio` capability seed，但 `enabled=false`。
- Studio 不展示可运行的 Volcengine audio generation 模型。
- 用户手工提交 Volcengine audio run 时，后端必须持久化 failed job，不调用外部 provider。
- 既有上传音频资产的播放/预览能力不受影响。

后续启用音频时的设计边界：

- 使用火山引擎音频/语音合成官方 API 作为底层 provider 能力。
- 在 ClipAnvil 内部封装为 Eino component / graph node。
- 如果音频 API 本身同步返回二进制结果，也仍由 runner 异步执行，API 请求只返回 job。
- 如果音频 API 是任务式接口，runner 采用与视频一致的 create/poll/download/upload 流程。

## 7. 异步运行模型

### 7.1 当前同步模型的问题

当前 `production.Service.RunNode` 在一个请求中完成：

```text
build intent -> provider.Run -> persist success/failure
```

这对 mock 和早期验证可用，但不适合真实模型调用。

原因不只是用户等待时间：

- HTTP worker 会被长时间占用，容器并发能力下降。
- provider 超时、客户端断开、服务重启都会让结果处理变复杂。
- 视频任务天然需要 polling 或 callback；后续音频如果是任务式 API，也走同一机制。
- 文本 stream 如果只在请求内消费，Agent 和 Studio 很难复用同一条事件流。
- 统一异步后才能做队列限流、成本控制、取消、重试和优先级调度。

### 7.2 目标模型

新增生产运行状态机：

| 状态 | 含义 |
|---|---|
| `pending` | job 已创建，尚未入队 |
| `queued` | 等待 runner |
| `running` | 正在调用 provider 或 polling |
| `succeeded` | 已产生 artifact version |
| `failed` | 已失败且错误落库 |
| `cancelled` | 用户或系统取消 |

当前表已有 job status，可复用；需要补足 runner 行为和 API 语义。

### 7.3 API 语义

真实运行 API 返回 job，不返回最终 artifact：

```text
POST /api/nodes/:id/run
  -> 202 Accepted
  -> { "generation_job_id": "job-id-example", "status": "queued" }
```

前端订阅：

```text
GET /api/jobs/:id
GET /api/nodes/:id/production-state
WS /api/workspaces/:id/ws
```

事件至少包含：

- job 状态变化
- 文本 stream delta
- provider task id / request id 摘要
- 进度百分比或阶段名
- 最终 artifact version id
- 失败 error code/message

### 7.4 兼容策略

保留当前同步 `RunNode` 仅用于：

- mock provider 单元测试
- 旧测试迁移期
- 内部 helper，不作为真实 Studio/Agent 调用路径

第一期真实能力的验收标准必须基于异步路径。

## 8. Agent 复用方式

M6 Agent 不直接调用火山引擎。

Agent 工具应是 production-level tool：

```text
generate_text(node_id or intent)
generate_image(node_id or intent)
generate_video(node_id or intent)
generate_audio(node_id or intent) // deferred until audio model is available
```

这些工具内部调用 production service，拿到 `generation_job_id` 后订阅 job stream，并写入相同的 `generation_job` / `artifact_version`。

Producer / Craftsman 关心：

- 哪个 shot / node 正在生产。
- 当前 job 状态。
- 成功后 winner 是什么。
- 失败后 error_code 和 provider_response 是什么。
- stream delta 中已经产生了哪些用户可见内容。

它们不关心：

- Ark API key。
- HTTP endpoint。
- provider-specific polling。
- 文件下载上传细节。

## 9. 错误处理与审计

### 9.1 错误分类

沿用当前错误分类：

| error_code | 来源 |
|---|---|
| `capability_mismatch` | 模型不支持 node type / operation / input type |
| `provider_config_error` | 缺少 API key、模型 ID、base URL 等配置 |
| `provider_unavailable` | provider SDK/API 不可用、未实现、服务不可达 |
| `provider_error` | provider 返回失败、内容审核失败、下载失败 |
| `provider_timeout` | polling 或请求超时 |

`provider_timeout` 是新增建议；如果不新增枚举，可先映射到 `provider_error`，但 `ProviderResponse.code` 应保留 timeout 细节。

### 9.2 ProviderRequest 脱敏

禁止落库：

- API key
- Authorization header
- signed request headers
- provider 原始临时下载 URL 的完整 query secret

允许落库：

- provider id
- model id
- operation
- sanitized params
- input node ids
- input asset ids
- prompt hash 或 prompt 摘要
- provider task id
- Ark request id

### 9.3 失败必须可重试

失败 job 必须保留：

- `attempt`
- `max_attempts`
- `parent_job_id`
- `provider_request`
- `provider_response`
- `error_code`
- `error_message`

重试必须重新构造当前 GenerationIntent，而不是盲目复用旧 provider body。这样上游 winner 或 prompt 变更能被纳入新运行。

## 10. 实施分期

### R1: Eino Ark Text

目标：

- 引入 Eino/EinoExt Ark 依赖。
- 用真实 `ark.ChatModel` 实现 `text_generation`。
- 文本也必须通过 `ProductionRunner` 异步执行。
- 把 Eino `Stream` chunk 转成 job/workspace 事件。
- 保留 mock provider。
- 无 API key 时继续失败落库。

验收：

- 配置 `CLIPANVIL_PRODUCTION_PROVIDER_MODE=real` 和 API key。
- Text node 选择 `volcengine` 文本模型。
- 点击运行后立即返回 queued/running job。
- 前端能看到文本增量输出或阶段性状态。
- job/version/asset 全部落库。

### R2: Eino Ark Image

目标：

- 用真实 `ark.ImageGenerationModel` 实现 `text_to_image`。
- 图像也必须通过 `ProductionRunner` 异步执行。
- 输出 URL/base64 都统一上传 MinIO。
- 支持 size、watermark、response_format 等参数。

验收：

- Image node 选择火山图片模型。
- 点击运行后立即返回 queued/running job。
- 文生图最终成功。
- 画布预览显示真实图片。
- `media_asset.storage_url` 指向 MinIO，不指向火山临时 URL。

### R3: Production Async Runner

目标：

- 新增 production runner。
- `RunNode` / `SubmitNodeRun` 支持返回 queued/running job。
- WebSocket/API 可看到 job 状态变化。
- 为视频长任务和未来音频任务铺路。

验收：

- Async fake provider 可从 queued -> running -> succeeded。
- 文本/图片真实 provider 已经通过 runner 执行，不存在真实同步路径。
- 前端 Latest Job 能刷新到最终状态。
- 失败和重试链路仍可追溯。

### R4: Real Video

目标：

- 通过 Eino custom component / graph node 封装火山视频任务 API。
- 支持 `text_to_video` 和 `image_to_video`。
- 支持 polling、超时、下载、MinIO 上传。

验收：

- Video node 真实生成视频。
- job 先返回 running，完成后出现 artifact version。
- Agent/Studio 不关心 provider polling 细节。

### R5: Audio Hold Guard

目标：

- 不实现真实音频生成。
- 保持 `text_to_audio` capability disabled。
- 验证 Studio 不展示 Volcengine audio generation。
- 验证手工 API 请求不会调用 provider，会持久化 failed job。

验收：

- 无需配置音频模型或 API 能力即可完成最终验证。
- Model API 不返回 enabled Volcengine audio generation。
- 手工提交 audio run 得到 failed `generation_job`，错误为 `capability_mismatch` 或 `provider_config_error`。

### R6: Agent Worker Tool Reuse

目标：

- Agent Worker 工具调用 production service。
- Producer/Craftsman 只提交 node/intent，不直接接触 provider。
- 工程层事件唤醒 Producer。

验收：

- Agent Workspace 中 Worker 生成的图片/视频与 Studio 手动生成落在同一套 `generation_job` / `artifact_version`。

## 11. 数据库与配置变更

### 11.1 Migration

建议新增：

```text
013_real_volcengine_capabilities.sql
014_production_async_runner.sql
```

`013` 内容：

- upsert real Volcengine text/image/video capabilities。
- upsert Volcengine audio capability as disabled。
- 对 adapter 未实现的 capability 设置 `enabled=false`。
- 给 image/video defaults 写入合理默认值；audio 写入 disabled reason metadata。

`014` 内容：

- 如果现有 `generation_job` 字段不足，增加 provider task id、cancel reason 或 progress details。
- 如果不需要新字段，新增 `production_run_queue` 表。

### 11.2 Config

扩展：

```yaml
production:
  provider_mode: "mock"
  default_provider: "mock"
  default_text_model: "mock-text"
  volcengine:
    api_key: ""
    base_url: "https://ark.cn-beijing.volces.com/api/v3"
    region: "cn-beijing"
    text_model: ""
    image_model: ""
    video_model: ""
    audio_model: ""
    request_timeout_seconds: 600
    poll_interval_seconds: 5
    max_poll_seconds: 1800
```

## 12. 测试策略

### 12.1 Unit Tests

- 使用 fake Eino ChatModel 验证 text runtime mapping。
- 使用 fake image model 验证 URL/base64 输出归一化。
- 使用 fake async video client 验证 polling 成功、失败、超时。
- 验证 provider request/response 不包含 API key。
- 验证 capability disabled 时 UI/API 不暴露模型。

### 12.2 Integration Tests

- 用 `httptest.Server` 模拟 Ark API。
- 验证 request body 包含 model、prompt、params。
- 验证 Ark request id 被写入 provider response。
- 验证 provider 错误转成 failed generation job。

### 12.3 Real Provider Smoke

真实 provider smoke 不默认跑，需要显式开启：

```bash
CLIPANVIL_REAL_PROVIDER_SMOKE=1 \
CLIPANVIL_PRODUCTION_PROVIDER_MODE=real \
CLIPANVIL_PRODUCTION_DEFAULT_PROVIDER=volcengine \
CLIPANVIL_PRODUCTION_VOLCENGINE_API_KEY=volcengine-api-key-from-local-env \
scripts/smoke-real-volcengine-text.sh
```

建议 smoke 分文件：

- `scripts/smoke-real-volcengine-text.sh`
- `scripts/smoke-real-volcengine-image.sh`
- `scripts/smoke-real-volcengine-video.sh`

### 12.4 Browser E2E

文本和图片真实 smoke 至少要覆盖：

1. 登录。
2. 创建 Studio workspace。
3. 创建 Text/Image node。
4. 选择火山模型。
5. 点击运行。
6. 立即看到 queued/running job。
7. 文本节点能看到 stream delta 或阶段性输出。
8. 等待 job succeeded。
9. 验证画布 preview 和 Versions 更新。

视频真实 smoke 至少要覆盖：

1. 点击运行后立即出现 running/queued job。
2. 页面不阻塞。
3. 完成后 preview 更新。
4. Latest Job 展示 provider task id / request id 摘要。

音频本阶段只覆盖 hold 行为：

1. Studio 不展示 enabled Volcengine audio generation。
2. 手工 API run audio 时不调用外部 provider。
3. 后端持久化 failed job 和明确错误。

## 13. 安全与成本控制

- 默认 provider mode 仍为 `mock`，避免本地误调用真实付费 API。
- 真实 smoke 必须显式设置 `CLIPANVIL_REAL_PROVIDER_SMOKE=1`。
- 前端不展示 API key。
- 前端不直连火山引擎，只直连 ClipAnvil 后端。
- 后端日志不打印 API key。
- ProviderRequest 不落完整 prompt 以外的敏感 headers；如后续 prompt 可能含私密素材描述，可增加 prompt redaction 或 hash。
- 每个 capability 的 `limits` 必须包含 max attempts、最大 prompt 长度、支持时长和最大输入数量。
- Agent 模式下 Producer 需要根据成本策略决定是否发起 `request_user_decision`。
- ProductionRunner 必须有全局和 per-workspace 并发限制，避免容器被真实模型任务耗尽。
- 真实 provider worker 数量必须可配置，不能让 HTTP 请求数量直接决定 provider 并发。

## 14. 与现有 M5/M6 的关系

### M5 Studio

M5 不需要改交互模型：

- 用户仍选择节点、prompt、operation、model、params。
- UI 仍从 `model_capability` 加载模型。
- Run 仍调用 `/api/nodes/:id/run`。

变化是：选择火山模型后会真实生成，而不是 mock。

### M6 Agent

M6 Agent 直接复用：

- `GenerationIntent`
- `ProductionRunner`
- `EinoProductionRuntime`
- job/version/stale
- provider failure/retry

Agent 层只新增生产语义和调度：

- Producer 规划。
- Craftsman 改写 prompt。
- Worker 调 production service。
- Composer 调 sandbox/internal provider。

这避免 Studio 和 Agent 各写一套火山调用。

## 15. 参考资料

- Eino GitHub: https://github.com/cloudwego/eino
- EinoExt GitHub: https://github.com/cloudwego/eino-ext
- EinoExt Ark model docs: https://github.com/cloudwego/eino-ext/tree/main/components/model/ark
- 当前共享生产设计: `docs/superpowers/specs/2026-06-18-studio-agent-shared-production-design.md`
- 当前 Agent 模式设计: `docs/superpowers/specs/2026-06-18-multiagent-agent-mode-design.md`
- 当前 roadmap: `docs/milestones/m3-m6-studio-agent-roadmap.md`

## 16. 决策记录

1. **Eino 是 provider runtime 和 Agent runtime 的共同技术基础，但两者分阶段落地。**
   - 本 spec 先实现 provider runtime。
   - Agent runtime 的 checkpoint / interrupt / Producer 工具在 M6 spec 中继续推进。

2. **真实模型调用不走同步 HTTP。**
   - 文本、图像、视频都必须通过 async runner；未来启用音频也必须通过 async runner。

3. **数据库展示能力，环境变量保存凭证。**
   - `model_capability` 控制 UI 可选模型。
   - `.env` / environment 控制真实 provider 凭证。

4. **Mock 不删除。**
   - mock 是测试和本地低成本开发路径。
   - real provider 是显式配置路径。

5. **没有 adapter 的 capability 不启用。**
   - 不允许 UI 展示一个必然失败的真实模型。

6. **ProviderBridge 不作为第一期真实火山能力的核心接口。**
   - 当前 `ProviderBridge` 是 M4 历史同步接口。
   - 第一阶段只有火山引擎真实能力，不需要抽象成多 provider bridge。
   - 如果未来增加第二供应商，也优先通过 Eino component 扩展。

7. **Studio 本阶段必须真实可执行。**
   - 前端直连 ClipAnvil 后端。
   - 后端通过 Eino + 火山 SDK/API 调用火山。
   - mock 只保留为测试 provider。

8. **所有真实模型调用必须异步。**
   - 文本、图片、视频都走 job queue / runner；未来启用音频也走同一路径。
   - Agent 用户可见输出必须 stream。
