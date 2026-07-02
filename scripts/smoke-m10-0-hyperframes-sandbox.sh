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
echo "hyperframes=$(command -v hyperframes || true)"
if command -v hyperframes >/dev/null 2>&1; then hyperframes --version; fi
echo "chrome=$(hyperframes browser path 2>/dev/null || true)"
echo "ffmpeg=$(command -v ffmpeg || true)"
if command -v ffmpeg >/dev/null 2>&1; then ffmpeg -version | head -1; fi
echo "ffprobe=$(command -v ffprobe || true)"
if command -v ffprobe >/dev/null 2>&1; then ffprobe -version | head -1; fi
'
probe_response="$(exec_sandbox "$runtime_probe" 120)"
probe_exit="$(jq -r '.ExitCode // .exit_code' <<<"$probe_response")"
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
      body { margin: 0; background: #111827; color: white; font-family: Arial, sans-serif; overflow: hidden; }
      .frame { width: 1080px; height: 1920px; display: grid; place-items: center; }
      .card { width: 720px; padding: 64px; border-radius: 24px; background: #f5c542; color: #111827; text-align: center; }
      h1 { font-size: 72px; margin: 0 0 24px; }
      p { font-size: 36px; margin: 0; }
    </style>
  </head>
  <body>
    <main id="m10-root" class="clip frame" data-composition-id="m10-hyperframes-spike" data-start="0" data-duration="3" data-width="1080" data-height="1920" data-fps="24">
      <section id="m10-card" class="clip card" data-start="0.2" data-duration="2.6">
        <h1>ClipAnvil</h1>
        <p>HyperFrames sandbox spike</p>
      </section>
    </main>
    <script>
      window.__timelines = window.__timelines || {};
      window.__timelines["m10-hyperframes-spike"] = {
        pause() { return this; },
        seek() { return this; },
        time() { return 0; },
        duration() { return 3; },
        totalDuration() { return 3; },
        getChildren() { return []; }
      };
    </script>
  </body>
</html>
HTML
cd "$workdir"
hyperframes render . --output /workspace/output/m10-hyperframes-spike.mp4 --fps 24 --quality draft
ffprobe -v error -select_streams v:0 -show_entries stream=codec_name,width,height,r_frame_rate -of json /workspace/output/m10-hyperframes-spike.mp4
'
render_response="$(exec_sandbox "$render_command" 900)"
render_exit="$(jq -r '.ExitCode // .exit_code' <<<"$render_response")"
if [[ "$render_exit" != "0" ]]; then
  echo "hyperframes render failed" >&2
  echo "$render_response" >&2
  exit 1
fi

echo "m10.0 hyperframes sandbox smoke passed"
echo "workspace_id=$workspace_id"
echo "sandbox_id=$sandbox_id"
echo "runtime_probe=$(jq -c '{exit_code:(.ExitCode // .exit_code),stdout:(.Stdout // .stdout),stderr:(.Stderr // .stderr),truncated:(.Truncated // .truncated)}' <<<"$probe_response")"
echo "render_probe=$(jq -c '{exit_code:(.ExitCode // .exit_code),stdout:(.Stdout // .stdout),stderr:(.Stderr // .stderr),truncated:(.Truncated // .truncated)}' <<<"$render_response")"
