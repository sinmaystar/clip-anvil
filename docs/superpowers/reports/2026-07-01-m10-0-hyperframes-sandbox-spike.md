# M10.0 HyperFrames Sandbox Spike Report

**日期**：2026-07-01
**状态**：通过

## 运行范围

- API base: `http://localhost:8894`
- Workspace: `1929333a-08ff-41f1-83a9-a053ff4445c9`
- Sandbox: `0039752d-e781-4dc1-81f5-e2f8f2af1efc`
- Output: `/workspace/output/m10-hyperframes-spike.mp4`
- Sandbox image: `clipanvil-sandbox:dev`

## Runtime Probe

```text
node=/usr/local/bin/node
v22.23.1
npm=/usr/local/bin/npm
10.9.8
hyperframes=/usr/local/bin/hyperframes
0.7.22
chrome=/usr/bin/chromium-headless-shell
ffmpeg=/usr/bin/ffmpeg
ffmpeg version 5.1.9-0+deb12u1
ffprobe=/usr/bin/ffprobe
ffprobe version 5.1.9-0+deb12u1
```

## Render Probe

```text
Command:
hyperframes render . --output /workspace/output/m10-hyperframes-spike.mp4 --fps 24 --quality draft

HyperFrames:
- platform=linux
- arch=arm64
- browser=HeadlessChrome/149.0.7827.196
- totalMemMb=4096
- low-memory profile active
- workerCount=1
- output=/workspace/output/m10-hyperframes-spike.mp4
- render time around 7 seconds for a 3 second 1080x1920 draft probe

ffprobe:
codec_name=h264
width=1080
height=1920
r_frame_rate=24/1
```

## 过程发现

- 原始 sandbox image 基于 `ubuntu:24.04`，已有 FFmpeg / FFprobe / CJK fonts，但没有 Node.js、npm、npx 或 HyperFrames。
- 在 Docker Desktop ARM64 环境中，HyperFrames 自动安装 Chrome Headless Shell 不可用；Ubuntu 24.04 ARM64 的 `chromium-browser` 是 snap transitional package，不适合容器。
- Debian bookworm ARM64 提供可用的 `chromium-headless-shell` apt 包；因此 sandbox image 切到 `node:22-bookworm-slim`，并设置 `HYPERFRAMES_BROWSER_PATH=/usr/bin/chromium-headless-shell`。
- HyperFrames 0.7.22 的 render CLI 是 `hyperframes render [DIR] --output <file>`，不是 `--input index.html`。
- 最小 composition 需要 root `data-composition-id`、`data-start="0"`、`data-duration`、`data-width`、`data-height`，并需要初始化 `window.__timelines`。没有 timeline registry 时会等待到 player ready timeout 后才继续。
- OpenSandbox 容器 cgroup memory limit 为 4096 MiB，HyperFrames 会自动启用 low-memory screenshot render，并把 worker 固定为 1。

## 验证命令

```bash
docker build -t clipanvil-sandbox:dev sandbox-image
docker run --rm clipanvil-sandbox:dev bash -lc '<local HyperFrames render probe>'
curl -fsS http://localhost:8894/api/health
bash -n scripts/smoke-m10-0-hyperframes-sandbox.sh
CLIPANVIL_SERVER_PORT=8894 ./scripts/smoke-m10-0-hyperframes-sandbox.sh
```

## 结论

当前 OpenSandbox runtime 已能执行 HyperFrames 最小 HTML -> MP4 渲染，并能用 `ffprobe` 验证 video stream。M10.0 gate 通过，可以进入 M10.1 Capability 与 RenderPlan 路由基础。
