#!/usr/bin/env bash
set -euo pipefail

API_URL="${CLIPANVIL_API_BASE_URL:-http://localhost:${CLIPANVIL_SERVER_PORT:-8888}}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this smoke script." >&2
  exit 1
fi
if ! command -v ffprobe >/dev/null 2>&1; then
  echo "ffprobe is required for this smoke script." >&2
  exit 1
fi

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/clipanvil-m11-2.XXXXXX")"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

TOKEN=""

api() {
  local method="$1"
  local path="$2"
  local payload="${3:-}"
  if [[ -n "$payload" ]]; then
    curl -fsS -X "$method" "$API_URL/api$path" \
      -H 'content-type: application/json' \
      ${TOKEN:+-H "authorization: Bearer $TOKEN"} \
      -d "$payload"
  else
    curl -fsS -X "$method" "$API_URL/api$path" \
      ${TOKEN:+-H "authorization: Bearer $TOKEN"}
  fi
}

email="m11-motion-provider-$(date +%s)@example.test"
password="Password123!"
register_payload="$(jq -n --arg email "$email" --arg password "$password" '{email:$email,password:$password,name:"M11 Motion Provider"}')"
register_response="$(api POST /auth/register "$register_payload")"
TOKEN="$(jq -r '.token' <<<"$register_response")"
if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
  echo "failed to read auth token" >&2
  echo "$register_response" >&2
  exit 1
fi

workspace_payload='{"name":"m11-remotion-motion-shot-provider","mode":"studio"}'
workspace_response="$(api POST /workspaces "$workspace_payload")"
workspace_id="$(jq -r '.workspace.id // .id' <<<"$workspace_response")"
if [[ -z "$workspace_id" || "$workspace_id" == "null" ]]; then
  echo "failed to read workspace id" >&2
  echo "$workspace_response" >&2
  exit 1
fi

cat >"$tmpdir/product.png.b64" <<'B64'
iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=
B64
base64 -d <"$tmpdir/product.png.b64" >"$tmpdir/product.png"

upload_response="$(curl -fsS -X POST "$API_URL/api/upload" \
  -H "authorization: Bearer $TOKEN" \
  -F "workspace_id=$workspace_id" \
  -F "file=@$tmpdir/product.png;type=image/png")"
asset_id="$(jq -r '.id' <<<"$upload_response")"
if [[ -z "$asset_id" || "$asset_id" == "null" ]]; then
  echo "failed to read uploaded asset id" >&2
  echo "$upload_response" >&2
  exit 1
fi

source_node_payload="$(jq -n \
  --arg workspace_id "$workspace_id" \
  --arg asset_id "$asset_id" \
  '{
    workspace_id:$workspace_id,
    node_type:"image",
    title:"M11.2 product source",
    prompt:"source product image",
    asset_id:$asset_id,
    canvas_x:20,
    canvas_y:40
  }')"
source_node_response="$(api POST /nodes "$source_node_payload")"
source_node_id="$(jq -r '.id' <<<"$source_node_response")"

motion_node_payload="$(jq -n \
  --arg workspace_id "$workspace_id" \
  '{
    workspace_id:$workspace_id,
    node_type:"video",
    title:"M11.2 Remotion motion shot",
    prompt:"Render a low-cost Remotion motion shot with product image, headline and CTA.",
    operation_type:"image_to_motion_video",
    model_provider:"internal_motion_video",
    model_id:"remotion-motion-shot-v1",
    model_params:{
      duration_sec:3,
      ratio:"9:16",
      resolution:"1080p",
      fps:30,
      motion_style:"premium_product_ad",
      headline:"轻装出发",
      text_layers:[
        {role:"hook",text:"轻装出发",start_sec:0.2,end_sec:1.8,animation:"pop_slide_up",position:"upper_third"},
        {role:"cta",text:"现在了解",start_sec:1.9,end_sec:2.8,animation:"fade_rise",position:"bottom_safe"}
      ],
      transitions:{in:"soft_zoom",out:"swipe_up"},
      brand_colors:["#111827","#F5C542"]
    },
    canvas_x:420,
    canvas_y:40
  }')"
motion_node_response="$(api POST /nodes "$motion_node_payload")"
motion_node_id="$(jq -r '.id' <<<"$motion_node_response")"

edge_payload="$(jq -n \
  --arg workspace_id "$workspace_id" \
  --arg from_node_id "$source_node_id" \
  --arg to_node_id "$motion_node_id" \
  '{workspace_id:$workspace_id,from_node_id:$from_node_id,to_node_id:$to_node_id,metadata:{reason:"m11_motion_shot_input"}}')"
api POST /edges "$edge_payload" >/dev/null

run_response="$(api POST "/nodes/$motion_node_id/run" '{"max_attempts":1}')"
generation_job_id="$(jq -r '.job.id' <<<"$run_response")"
provider="$(jq -r '.job.provider' <<<"$run_response")"
if [[ "$provider" != "internal_motion_video" ]]; then
  echo "expected generation_job.provider=internal_motion_video, got $provider" >&2
  echo "$run_response" >&2
  exit 1
fi

state=""
for _ in $(seq 1 90); do
  state="$(api GET "/nodes/$motion_node_id/production-state")"
  status="$(jq -r '.latest_job.status // empty' <<<"$state")"
  if [[ "$status" == "succeeded" ]]; then
    break
  fi
  if [[ "$status" == "failed" ]]; then
    echo "motion shot job failed" >&2
    echo "$state" >&2
    exit 1
  fi
  sleep 2
done

status="$(jq -r '.latest_job.status // empty' <<<"$state")"
if [[ "$status" != "succeeded" ]]; then
  echo "motion shot job did not succeed before timeout; latest status=$status" >&2
  echo "$state" >&2
  exit 1
fi

winner="$(jq -r '.current_version.winner' <<<"$state")"
artifact_version_id="$(jq -r '.current_version.id' <<<"$state")"
asset_url="$(jq -r '.current_version.asset.access_url' <<<"$state")"
sandbox_job_id="$(jq -r '.sandbox_jobs[0].id // empty' <<<"$state")"
renderer_engine="$(jq -r '.latest_job.provider_response.renderer_engine // empty' <<<"$state")"
if [[ "$winner" != "true" || -z "$artifact_version_id" || "$artifact_version_id" == "null" ]]; then
  echo "expected artifact_version winner=true" >&2
  echo "$state" >&2
  exit 1
fi
if [[ "$renderer_engine" != "remotion" ]]; then
  echo "provider_response missing renderer_engine=remotion" >&2
  echo "$state" >&2
  exit 1
fi
if [[ -z "$asset_url" || "$asset_url" == "null" ]]; then
  echo "missing artifact access URL" >&2
  echo "$state" >&2
  exit 1
fi

curl -fsS "$asset_url" -o "$tmpdir/motion.mp4"
ffprobe_json="$(ffprobe -v error -select_streams v:0 -show_entries stream=codec_type,codec_name,width,height,r_frame_rate -of json "$tmpdir/motion.mp4")"
codec_type="$(jq -r '.streams[0].codec_type // empty' <<<"$ffprobe_json")"
if [[ "$codec_type" != "video" ]]; then
  echo "ffprobe did not find a video stream" >&2
  echo "$ffprobe_json" >&2
  exit 1
fi

echo "m11.2 remotion motion shot provider smoke passed"
echo "workspace_id=$workspace_id"
echo "source_node_id=$source_node_id"
echo "motion_node_id=$motion_node_id"
echo "generation_job_id=$generation_job_id"
echo "artifact_version_id=$artifact_version_id"
echo "sandbox_job_id=$sandbox_job_id"
echo "provider=internal_motion_video"
echo "renderer_engine=$renderer_engine"
echo "winner=$winner"
echo "ffprobe=$(jq -c '.streams[0]' <<<"$ffprobe_json")"
