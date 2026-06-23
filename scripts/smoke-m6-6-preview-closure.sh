#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${CLIPANVIL_PUBLIC_BASE_URL:-http://localhost:${CLIPANVIL_WEB_PORT:-5173}}"
API_URL="${CLIPANVIL_API_BASE_URL:-http://localhost:${CLIPANVIL_SERVER_PORT:-8888}}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this smoke script." >&2
  exit 1
fi

email="m66-preview-$(date +%s)@example.test"
password="Password123!"

register_payload=$(jq -n --arg email "$email" --arg password "$password" '{email:$email,password:$password,name:"M66 Preview"}')
curl -fsS -X POST "$API_URL/api/auth/register" -H 'content-type: application/json' -d "$register_payload" >/tmp/m66-register.json
token=$(jq -r '.token' /tmp/m66-register.json)
if [[ -z "$token" || "$token" == "null" ]]; then
  echo "failed to read auth token from register response" >&2
  cat /tmp/m66-register.json >&2
  exit 1
fi

workspace_payload='{"name":"m6-6-preview-closure","mode":"agent"}'
curl -fsS -X POST "$API_URL/api/workspaces" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$workspace_payload" >/tmp/m66-workspace.json
workspace_id=$(jq -r '.workspace.id // .id' /tmp/m66-workspace.json)
if [[ -z "$workspace_id" || "$workspace_id" == "null" ]]; then
  echo "failed to read workspace id from workspace response" >&2
  cat /tmp/m66-workspace.json >&2
  exit 1
fi

message_payload=$(jq -n \
  --arg text "创建一个 3 个分镜的 15 秒口播种草短视频 storyboard，然后为所有分镜生成预览图。" \
  '{text:$text,client_message_id:"m66-smoke-1"}')
curl -fsS -X POST "$API_URL/api/agent/workspaces/$workspace_id/messages" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$message_payload" >/tmp/m66-message.json

echo "workspace_id=$workspace_id"
echo "agent_url=$BASE_URL/workspaces/$workspace_id/agent"
echo "message_response=/tmp/m66-message.json"
echo "Wait for Agent tasks to finish, then run the M6.6 DB spot checks from docs/superpowers/plans/2026-06-23-m6-6-closure-preview-generation.md."
