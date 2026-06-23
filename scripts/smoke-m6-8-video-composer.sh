#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${CLIPANVIL_PUBLIC_BASE_URL:-http://localhost:${CLIPANVIL_WEB_PORT:-5173}}"
API_URL="${CLIPANVIL_API_BASE_URL:-http://localhost:${CLIPANVIL_SERVER_PORT:-8888}}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this smoke script." >&2
  exit 1
fi

email="m68-composer-$(date +%s)@example.test"
password="Password123!"

register_payload=$(jq -n --arg email "$email" --arg password "$password" '{email:$email,password:$password,name:"M68 Composer"}')
curl -fsS -X POST "$API_URL/api/auth/register" -H 'content-type: application/json' -d "$register_payload" >/tmp/m68-register.json
token=$(jq -r '.token' /tmp/m68-register.json)
if [[ -z "$token" || "$token" == "null" ]]; then
  echo "failed to read auth token from register response" >&2
  cat /tmp/m68-register.json >&2
  exit 1
fi

workspace_payload='{"name":"m6-8-video-composer","mode":"agent"}'
curl -fsS -X POST "$API_URL/api/workspaces" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$workspace_payload" >/tmp/m68-workspace.json
workspace_id=$(jq -r '.workspace.id // .id' /tmp/m68-workspace.json)
if [[ -z "$workspace_id" || "$workspace_id" == "null" ]]; then
  echo "failed to read workspace id from workspace response" >&2
  cat /tmp/m68-workspace.json >&2
  exit 1
fi

message_payload=$(jq -n \
  --arg text "创建一个 2 个分镜的 10 秒口播种草短视频 storyboard；为每个分镜生成预览图，评审通过后生成分镜视频，最后把所有分镜视频合成为成片并请求我确认。" \
  '{text:$text,client_message_id:"m68-smoke-1"}')
curl -fsS -X POST "$API_URL/api/agent/workspaces/$workspace_id/messages" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$message_payload" >/tmp/m68-message.json

echo "workspace_id=$workspace_id"
echo "agent_url=$BASE_URL/workspaces/$workspace_id/agent"
echo "message_response=/tmp/m68-message.json"
echo "Expected durable facts after background work:"
echo "- agent_task rows include craftsman_turn for preview_image and shot_video, plus composer_turn for final_output"
echo "- media_node rows include agent preview_image nodes, shot_video nodes, and one final_video node"
echo "- generation_job rows include image_to_video and compose_final_video operations"
echo "- sandbox_job rows include compose_final_video for the final video node"
echo "- agent messages can include final_video_card once composition succeeds"
