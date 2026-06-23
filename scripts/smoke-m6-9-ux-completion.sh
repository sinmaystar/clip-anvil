#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${CLIPANVIL_PUBLIC_BASE_URL:-http://localhost:${CLIPANVIL_WEB_PORT:-5173}}"
API_URL="${CLIPANVIL_API_BASE_URL:-http://localhost:${CLIPANVIL_SERVER_PORT:-8888}}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this smoke script." >&2
  exit 1
fi

email="m69-ux-$(date +%s)@example.test"
password="Password123!"

register_payload=$(jq -n --arg email "$email" --arg password "$password" '{email:$email,password:$password,name:"M69 UX"}')
curl -fsS -X POST "$API_URL/api/auth/register" -H 'content-type: application/json' -d "$register_payload" >/tmp/m69-register.json
token=$(jq -r '.token' /tmp/m69-register.json)
if [[ -z "$token" || "$token" == "null" ]]; then
  echo "failed to read auth token from register response" >&2
  cat /tmp/m69-register.json >&2
  exit 1
fi

workspace_payload='{"name":"m6-9-ux-completion","mode":"agent"}'
curl -fsS -X POST "$API_URL/api/workspaces" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$workspace_payload" >/tmp/m69-workspace.json
workspace_id=$(jq -r '.workspace.id // .id' /tmp/m69-workspace.json)
if [[ -z "$workspace_id" || "$workspace_id" == "null" ]]; then
  echo "failed to read workspace id from workspace response" >&2
  cat /tmp/m69-workspace.json >&2
  exit 1
fi

overview_status=$(curl -sS -o /tmp/m69-overview.json -w "%{http_code}" "$API_URL/api/agent/workspaces/$workspace_id/production-overview" -H "authorization: Bearer $token")
if [[ "$overview_status" != "200" ]]; then
  echo "overview endpoint returned $overview_status" >&2
  cat /tmp/m69-overview.json >&2
  exit 1
fi

jq -e '.workspace_id and .phase and .counts and (.shots | type == "array") and (.timeline | type == "array") and (.final_outputs | type == "array")' /tmp/m69-overview.json >/dev/null

message_payload=$(jq -n --arg text "请只回复：M6.9 UX smoke ok。不要调用工具。" '{text:$text,client_message_id:"m69-smoke-1"}')
curl -fsS -X POST "$API_URL/api/agent/workspaces/$workspace_id/messages" -H "authorization: Bearer $token" -H 'content-type: application/json' -d "$message_payload" >/tmp/m69-message.json

echo "email=$email"
echo "password=$password"
echo "workspace_id=$workspace_id"
echo "agent_url=$BASE_URL/workspaces/$workspace_id/agent"
echo "overview_response=/tmp/m69-overview.json"
echo "message_response=/tmp/m69-message.json"
