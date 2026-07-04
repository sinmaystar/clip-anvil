# M10.2 Template Video Provider Vertical Slice Report

**日期**：2026-07-01
**状态**：通过

## 运行范围

M10.2 交付第一条 `internal_template_video/hyperframes-html` 生产竖切：受控模板库生成 HyperFrames HTML，`TemplateVideoProvider` 调用 sandbox `RenderTemplateVideo`，OpenSandbox 渲染 MP4，MinIO 入库，并由现有 production service 生成 `artifact_version.winner=true`。

本阶段完成：

- 新增 `apps/server/internal/templatevideo` 受控模板库。
- 新增 `static_fallback_ken_burns_v1` 最小模板，支持 `9:16`、`16:9`、`1:1`，`720p` / `1080p`，3/4/5/6/8/10 秒，24/30 fps。
- 新增 sandbox `RenderTemplateVideo`，负责写入 `index.html` / `meta.json` / `variables.json`、下载输入素材、执行 HyperFrames、校验 MP4 并上传 MinIO。
- 新增 production `TemplateVideoProvider`，provider ID 为 `internal_template_video`，model ID 为 `hyperframes-html`。
- `render_plan_submitter` 将 `template_video` profile 映射到 `internal_template_video/hyperframes-html`。
- server 启动时用真实 `sandbox.JobService` 注册 `internal_template_video` provider。
- 新增 `scripts/smoke-m10-2-template-video-provider.sh`，覆盖 API 创建 workspace、上传图片、创建节点和 edge、运行 template video 节点、轮询 production state、下载 artifact 并用 `ffprobe` 校验 video stream。

## 关键修复

第一次 smoke 已打通 provider 和 sandbox job，但 HyperFrames render 超时：

```text
provider execution error: opensandbox: sse read: context deadline exceeded (Client.Timeout or context cancellation while reading body)
```

定位后修复两点：

- HyperFrames root `data-duration` 使用秒，不使用帧数。否则 3 秒 24fps 会被模板写成 `72`，导致渲染时长被放大。
- `window.__timelines[composition_id]` 使用带 `pause`、`seek`、`duration`、`totalDuration`、`getChildren` 的对象，不再只写 `true`。

修复后同一 smoke 路径稳定通过。

## Smoke 结果

运行命令：

```bash
CLIPANVIL_SERVER_PORT=8894 ./scripts/smoke-m10-2-template-video-provider.sh
```

结果：

```text
m10.2 template video provider smoke passed
workspace_id=d89470a2-9d3a-400c-abfc-f44feb67ccb6
source_node_id=a5fe1be9-b5b5-4a19-9e0b-170564bb444e
template_node_id=f8f6abd3-5af8-4c60-801f-c4bac2cb60e6
generation_job_id=a6f31f55-1bde-4add-a4ba-8cff35f2f5d1
artifact_version_id=b2159c43-38e0-4835-bb84-f67c94e3a505
sandbox_job_id=4dade7fa-1df3-4f8b-b589-82ed39a7272b
provider=internal_template_video
winner=true
ffprobe={"codec_name":"h264","codec_type":"video","width":1080,"height":1920,"r_frame_rate":"24/1"}
```

## 验证命令

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/templatevideo ./internal/sandbox ./internal/production ./internal/agent/tools ./cmd/server
bash -n scripts/smoke-m10-2-template-video-provider.sh
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-build
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

## 验证结果

```text
ok  	github.com/sinmaystar/clip-anvil/internal/templatevideo	0.832s
ok  	github.com/sinmaystar/clip-anvil/internal/sandbox
ok  	github.com/sinmaystar/clip-anvil/internal/production	1.448s
ok  	github.com/sinmaystar/clip-anvil/internal/agent/tools	0.735s
ok  	github.com/sinmaystar/clip-anvil/cmd/server	2.111s
```

```text
make sqlc-generate
cd apps/server && /Users/wanwan/go/bin/sqlc generate
```

```text
GOCACHE=/private/tmp/clipanvil-go-build make server-build
cd apps/server && go build -o ../../bin/server ./cmd/server
```

```text
GOCACHE=/private/tmp/clipanvil-go-build make server-test
cd apps/server && go test ./...
...
ok  	github.com/sinmaystar/clip-anvil/internal/production
ok  	github.com/sinmaystar/clip-anvil/internal/sandbox
ok  	github.com/sinmaystar/clip-anvil/internal/templatevideo
```

`bash -n scripts/smoke-m10-2-template-video-provider.sh` 和 `git diff --check` 均通过。

## 结论

M10.2 gate 通过。`internal_template_video/hyperframes-html` 已经可以通过真实 production API 生成 shot video winner，并留下 `generation_job`、`provider_request`、`provider_response`、`sandbox_job` 和 `artifact_version` trace。下一阶段 M10.3 应把 Producer / Craftsman / Worker 主链路接入成本路由，让 Agent 在营销视频中默认生成 Seedance hero shot + template benefit / CTA shot 的混合路线。
