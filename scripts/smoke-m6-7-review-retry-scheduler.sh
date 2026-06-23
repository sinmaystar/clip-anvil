#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${CLIPANVIL_PUBLIC_BASE_URL:-http://localhost:${CLIPANVIL_WEB_PORT:-5173}}"
API_URL="${CLIPANVIL_API_BASE_URL:-http://localhost:${CLIPANVIL_SERVER_PORT:-8888}}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this smoke script." >&2
  exit 1
fi

email="m67-review-$(date +%s)@example.test"
password="Password123!"

register_payload=$(jq -n --arg email "$email" --arg password "$password" '{email:$email,password:$password,name:"M67 Review"}')
curl -fsS -X POST "$API_URL/api/auth/register" -H 'content-type: application/json' -d "$register_payload" >/tmp/m67-register.json
token=$(jq -r '.token' /tmp/m67-register.json)
if [[ -z "$token" || "$token" == "null" ]]; then
  echo "failed to read auth token from register response" >&2
  cat /tmp/m67-register.json >&2
  exit 1
fi

workspace_payload='{"name":"m6-7-review-retry-scheduler","mode":"agent"}'
curl -fsS -X POST "$API_URL/api/workspaces" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$workspace_payload" >/tmp/m67-workspace.json
workspace_id=$(jq -r '.workspace.id // .id' /tmp/m67-workspace.json)
if [[ -z "$workspace_id" || "$workspace_id" == "null" ]]; then
  echo "failed to read workspace id from workspace response" >&2
  cat /tmp/m67-workspace.json >&2
  exit 1
fi

message_payload=$(jq -n \
  --arg text "创建一个 2 个分镜的 10 秒口播种草短视频 storyboard，为每个分镜生成预览图；预览完成后调用 review_shot 评审预览图，若不通过请自动重试，最后查询当前 production state。" \
  '{text:$text,client_message_id:"m67-smoke-1"}')
curl -fsS -X POST "$API_URL/api/agent/workspaces/$workspace_id/messages" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$message_payload" >/tmp/m67-message.json

echo "workspace_id=$workspace_id"
echo "agent_url=$BASE_URL/workspaces/$workspace_id/agent"
echo "message_response=/tmp/m67-message.json"
echo "Expected durable facts after background work:"
echo "- review_record rows for preview_image reviews"
echo "- agent_event rows: review_started, review_accepted/review_rejected, optional retry_requested, shot_blocked/shot_unblocked/dependency_ready"
echo "- production state API includes review_records for reviewed preview nodes"
