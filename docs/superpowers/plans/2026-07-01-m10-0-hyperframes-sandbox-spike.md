# M10.0 HyperFrames Sandbox Spike Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Verify that HyperFrames can render a minimal MP4 inside ClipAnvil's current OpenSandbox / MinIO / FFmpeg runtime boundary before any production provider work starts.

**Architecture:** This spike uses the existing authenticated sandbox API rather than direct local commands. The smoke script creates a temporary Agent workspace, ensures a workspace sandbox through `/api/workspaces/:id/sandbox`, runs runtime probes through `/api/workspaces/:id/sandbox/exec`, renders a tiny HyperFrames composition inside `/workspace`, and verifies the resulting MP4 with `ffprobe` inside the same sandbox.

**Tech Stack:** Bash, curl, jq, ClipAnvil dev server, OpenSandbox, `clipanvil-sandbox:dev`, Node.js, HyperFrames CLI, FFmpeg, FFprobe.

**Execution status:** Completed on 2026-07-01. The initial smoke exposed that the previous Ubuntu sandbox image lacked Node.js and usable ARM64 Chrome. The final implementation updates `sandbox-image/Dockerfile` to `node:22-bookworm-slim`, installs `chromium-headless-shell`, sets `HYPERFRAMES_BROWSER_PATH`, installs HyperFrames 0.7.22, and verifies the real OpenSandbox API path. Result details are recorded in `docs/superpowers/reports/2026-07-01-m10-0-hyperframes-sandbox-spike.md`.

---

## File Map

- Create `scripts/smoke-m10-0-hyperframes-sandbox.sh`: API-level smoke that proves the sandbox can run HyperFrames and FFprobe the output.
- Create `docs/superpowers/reports/2026-07-01-m10-0-hyperframes-sandbox-spike.md`: record the exact runtime result, command output summary, and follow-up if the spike exposes missing runtime dependencies.
- Modify `docs/milestones/m10-hyperframes-template-video-provider.md`: update M10.0 status after the smoke has fresh evidence.

## Task 1: Add The M10.0 Sandbox Smoke Script

**Files:**
- Create: `scripts/smoke-m10-0-hyperframes-sandbox.sh`

- [ ] **Step 1: Create the smoke script**

Use `apply_patch` to create `scripts/smoke-m10-0-hyperframes-sandbox.sh` with this content:

```bash
#!/usr/bin/env bash
set -euo pipefail

API_URL="${CLIPANVIL_API_BASE_URL:-http://localhost:${CLIPANVIL_SERVER_PORT:-8888}}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this smoke script." >&2
  exit 1
fi

email="m10-hyperframes-$(date +%s)@example.test"
password="Password123!"

api() {
  local method="$1"
  local path="$2"
  local payload="${3:-}"
  local output
  output="$(mktemp "${TMPDIR:-/tmp}/clipanvil-m10-api.XXXXXX")"
  if [[ -n "$payload" ]]; then
    curl -fsS -X "$method" "$API_URL/api$path" \
      -H 'content-type: application/json' \
      ${TOKEN:+-H "authorization: Bearer $TOKEN"} \
      -d "$payload" >"$output"
  else
    curl -fsS -X "$method" "$API_URL/api$path" \
      ${TOKEN:+-H "authorization: Bearer $TOKEN"} >"$output"
  fi
  cat "$output"
  rm -f "$output"
}

TOKEN=""
register_payload="$(jq -n --arg email "$email" --arg password "$password" '{email:$email,password:$password,name:"M10 HyperFrames"}')"
register_response="$(api POST /auth/register "$register_payload")"
TOKEN="$(jq -r '.token' <<<"$register_response")"
if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
  echo "failed to read auth token from register response" >&2
  echo "$register_response" >&2
  exit 1
fi

workspace_payload='{"name":"m10-hyperframes-sandbox-spike","mode":"agent"}'
workspace_response="$(api POST /workspaces "$workspace_payload")"
workspace_id="$(jq -r '.workspace.id // .id' <<<"$workspace_response")"
if [[ -z "$workspace_id" || "$workspace_id" == "null" ]]; then
  echo "failed to read workspace id from workspace response" >&2
  echo "$workspace_response" >&2
  exit 1
fi

status_response="$(api GET "/workspaces/$workspace_id/sandbox")"
sandbox_id="$(jq -r '.sandbox_id' <<<"$status_response")"
if [[ -z "$sandbox_id" || "$sandbox_id" == "null" ]]; then
  echo "failed to ensure sandbox" >&2
  echo "$status_response" >&2
  exit 1
fi

exec_sandbox() {
  local command="$1"
  local timeout="${2:-300}"
  local payload
  payload="$(jq -n --arg command "$command" --argjson timeout "$timeout" '{command:$command,cwd:"/workspace",timeout_seconds:$timeout}')"
  api POST "/workspaces/$workspace_id/sandbox/exec" "$payload"
}

runtime_probe='
set -euo pipefail
echo "node=$(command -v node || true)"
if command -v node >/dev/null 2>&1; then node --version; fi
echo "npm=$(command -v npm || true)"
if command -v npm >/dev/null 2>&1; then npm --version; fi
echo "ffmpeg=$(command -v ffmpeg || true)"
if command -v ffmpeg >/dev/null 2>&1; then ffmpeg -version | head -1; fi
echo "ffprobe=$(command -v ffprobe || true)"
if command -v ffprobe >/dev/null 2>&1; then ffprobe -version | head -1; fi
'
probe_response="$(exec_sandbox "$runtime_probe" 120)"
probe_exit="$(jq -r '.exit_code' <<<"$probe_response")"
if [[ "$probe_exit" != "0" ]]; then
  echo "runtime probe failed" >&2
  echo "$probe_response" >&2
  exit 1
fi

render_command='
set -euo pipefail
workdir="/workspace/m10-hyperframes-spike"
rm -rf "$workdir"
mkdir -p "$workdir"
cat > "$workdir/index.html" <<'"'"'HTML'"'"'
<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <style>
      body { margin: 0; width: 100vw; height: 100vh; background: #111827; color: white; font-family: Arial, sans-serif; overflow: hidden; }
      .frame { width: 100vw; height: 100vh; display: grid; place-items: center; }
      .card { width: 720px; padding: 64px; border-radius: 24px; background: #f5c542; color: #111827; text-align: center; }
      h1 { font-size: 72px; margin: 0 0 24px; }
      p { font-size: 36px; margin: 0; }
    </style>
  </head>
  <body data-duration="3" data-width="720" data-height="1280" data-fps="24">
    <main class="frame" data-start="0" data-duration="3">
      <section class="card" data-start="0.2" data-duration="2.6">
        <h1>ClipAnvil</h1>
        <p>HyperFrames sandbox spike</p>
      </section>
    </main>
  </body>
</html>
HTML
cd "$workdir"
npx --yes hyperframes render --input index.html --output /workspace/output/m10-hyperframes-spike.mp4
ffprobe -v error -select_streams v:0 -show_entries stream=codec_name,width,height,r_frame_rate -of json /workspace/output/m10-hyperframes-spike.mp4
'
render_response="$(exec_sandbox "$render_command" 900)"
render_exit="$(jq -r '.exit_code' <<<"$render_response")"
if [[ "$render_exit" != "0" ]]; then
  echo "hyperframes render failed" >&2
  echo "$render_response" >&2
  exit 1
fi

echo "m10.0 hyperframes sandbox smoke passed"
echo "workspace_id=$workspace_id"
echo "sandbox_id=$sandbox_id"
echo "runtime_probe=$(jq -c '{exit_code,stdout,stderr,truncated}' <<<"$probe_response")"
echo "render_probe=$(jq -c '{exit_code,stdout,stderr,truncated}' <<<"$render_response")"
```

- [ ] **Step 2: Make the smoke script executable**

Run:

```bash
chmod +x scripts/smoke-m10-0-hyperframes-sandbox.sh
```

- [ ] **Step 3: Verify script syntax**

Run:

```bash
bash -n scripts/smoke-m10-0-hyperframes-sandbox.sh
```

Expected: exit code 0.

## Task 2: Run The Runtime Spike

**Files:**
- Read: `scripts/dev-start.sh`
- Read: `deploy/docker-compose.yml`
- Execute: `scripts/smoke-m10-0-hyperframes-sandbox.sh`

- [ ] **Step 1: Inspect the worktree dev ports**

Run:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

Expected: shell exports for `CLIPANVIL_SERVER_PORT` and `CLIPANVIL_WEB_PORT`.

- [ ] **Step 2: Start the local dev stack if no current server is available**

Run:

```bash
./scripts/dev-start.sh
```

Expected: middleware, Go server, and Vite dev server start; health check reports connected services including sandbox.

- [ ] **Step 3: Execute the M10.0 smoke**

Run:

```bash
./scripts/smoke-m10-0-hyperframes-sandbox.sh
```

Expected if the current sandbox runtime is ready: the script prints `m10.0 hyperframes sandbox smoke passed`, a workspace id, a sandbox id, runtime probe JSON, and render probe JSON.

Expected if the current sandbox runtime is not ready: the script fails with the exact missing dependency or HyperFrames CLI error. Record the failure as M10.0 evidence and do not proceed to M10.1.

## Task 3: Record M10.0 Results

**Files:**
- Create: `docs/superpowers/reports/2026-07-01-m10-0-hyperframes-sandbox-spike.md`
- Modify: `docs/milestones/m10-hyperframes-template-video-provider.md`

- [ ] **Step 1: Create the M10.0 report**

If the smoke passes, create:

```markdown
# M10.0 HyperFrames Sandbox Spike Report

**日期**：2026-07-01
**状态**：通过

## 运行范围

- API base: `<actual API URL>`
- Workspace: `<workspace id>`
- Sandbox: `<sandbox id>`
- Output: `/workspace/output/m10-hyperframes-spike.mp4`

## Runtime Probe

```text
<node/npm/ffmpeg/ffprobe stdout summary>
```

## Render Probe

```text
<ffprobe stdout summary>
```

## 结论

当前 OpenSandbox runtime 已能执行 HyperFrames 最小 HTML -> MP4 渲染，并能用 ffprobe 验证 video stream。可以进入 M10.1 Capability 与 RenderPlan 路由基础。
```

If the smoke fails, create:

```markdown
# M10.0 HyperFrames Sandbox Spike Report

**日期**：2026-07-01
**状态**：未通过

## 运行范围

- API base: `<actual API URL>`
- Workspace: `<workspace id if available>`
- Sandbox: `<sandbox id if available>`

## 失败证据

```text
<exact failing command stderr/stdout summary>
```

## 阻塞原因

M10.0 未通过，不能进入 M10.1。下一步需要补齐 sandbox runtime 依赖，例如 Node.js >= 22、FFmpeg、ffprobe、chrome-headless-shell、中文字体或固定 HyperFrames CLI 参数。
```

- [ ] **Step 2: Update the M10 milestone status**

If the smoke passes, update M10.0 row in `docs/milestones/m10-hyperframes-template-video-provider.md` to state it is completed and link the report.

If the smoke fails, keep M10.0 as in progress / not passed and link the report as evidence.

## Task 4: Verify Documentation And Stop At The Phase Gate

**Files:**
- Verify: `docs/superpowers/plans/2026-07-01-m10-0-hyperframes-sandbox-spike.md`
- Verify: `docs/superpowers/reports/2026-07-01-m10-0-hyperframes-sandbox-spike.md`
- Verify: `docs/milestones/m10-hyperframes-template-video-provider.md`

- [ ] **Step 1: Scan for placeholders**

Run:

```bash
rg -n 'TB[D]|TO''DO|FIX''ME|待''定|x''xx' docs/superpowers/plans/2026-07-01-m10-0-hyperframes-sandbox-spike.md docs/superpowers/reports/2026-07-01-m10-0-hyperframes-sandbox-spike.md docs/milestones/m10-hyperframes-template-video-provider.md
```

Expected: no matches.

- [ ] **Step 2: Run diff whitespace validation**

Run:

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 3: Enforce the M10 phase gate**

If M10.0 passed, stop and ask for review before writing the M10.1 implementation plan.

If M10.0 failed, stop at the runtime dependency fix. Do not create migrations, `template_video` profile, or production provider code until the M10.0 runtime evidence is green.
