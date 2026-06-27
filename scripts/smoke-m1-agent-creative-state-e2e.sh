#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

COMMAND="${1:-}"
STATE_FILE="${CLIPANVIL_E2E_STATE_FILE:-/tmp/clipanvil-m1-agent-creative-state-e2e.json}"
BASE="${CLIPANVIL_API_BASE:-http://127.0.0.1:${CLIPANVIL_SERVER_PORT:-8888}/api}"
WEB_BASE="${CLIPANVIL_WEB_BASE:-${CLIPANVIL_PUBLIC_BASE_URL:-http://127.0.0.1:${CLIPANVIL_WEB_PORT:-5173}}}"
DB_DSN="${CLIPANVIL_POSTGRES_DSN:-postgres://clipanvil:clipanvil_dev@localhost:5432/clipanvil?sslmode=disable}"

usage() {
  cat <<'USAGE'
Usage:
  scripts/smoke-m1-agent-creative-state-e2e.sh setup
  scripts/smoke-m1-agent-creative-state-e2e.sh verify

Environment:
  CLIPANVIL_E2E_PRODUCER_FIXTURE=m1_creative_state must be set before starting the server.
  CLIPANVIL_SERVER_PORT / CLIPANVIL_WEB_PORT are read from the running dev profile.
  CLIPANVIL_E2E_STATE_FILE can override the setup state path.
USAGE
}

if [[ "$COMMAND" != "setup" && "$COMMAND" != "verify" ]]; then
  usage >&2
  exit 1
fi

if [[ "$COMMAND" == "setup" ]]; then
  BASE="$BASE" WEB_BASE="$WEB_BASE" STATE_FILE="$STATE_FILE" node <<'NODE'
import fs from "node:fs";

const base = process.env.BASE;
const webBase = process.env.WEB_BASE;
const stateFile = process.env.STATE_FILE;
const suffix = `${Date.now()}-${Math.floor(Math.random() * 10000)}`;
const email = `m1-agent-e2e-${suffix}@clipanvil.local`;
const password = "clipanvil-e2e-pass";

async function req(path, init = {}) {
  const headers = {
    ...(init.headers ?? {}),
    "Content-Type": "application/json",
  };
  const response = await fetch(base + path, { ...init, headers });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`${init.method ?? "GET"} ${path} -> ${response.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
}

const auth = await req("/auth/register", {
  method: "POST",
  body: JSON.stringify({ email, password, name: "M1 Agent E2E" }),
});
const workspace = await req("/workspaces", {
  method: "POST",
  headers: { Authorization: `Bearer ${auth.token}` },
  body: JSON.stringify({ name: "M1 Agent Creative State E2E", mode: "agent" }),
});
const state = {
  email,
  password,
  token: auth.token,
  account: auth.account,
  workspace,
  agent_url: `${webBase}/workspaces/${workspace.id}/agent`,
  message_text: "E2E_M1_CREATIVE_STATE：请为悦行行李箱做一条机场抖音广告，先沉淀 brief、memory、关键元素和两镜头 storyboard。",
};
fs.writeFileSync(stateFile, JSON.stringify(state, null, 2));
console.log(JSON.stringify(state, null, 2));
NODE
  echo "state_file=$STATE_FILE"
  exit 0
fi

if [[ ! -f "$STATE_FILE" ]]; then
  echo "state file not found: $STATE_FILE" >&2
  exit 1
fi

BASE="$BASE" STATE_FILE="$STATE_FILE" node <<'NODE'
import fs from "node:fs";

const base = process.env.BASE;
const state = JSON.parse(fs.readFileSync(process.env.STATE_FILE, "utf8"));
const headers = { Authorization: `Bearer ${state.token}` };

async function req(path) {
  const response = await fetch(base + path, { headers });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`GET ${path} -> ${response.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
}

async function waitFor(label, fn, timeoutMs = 30000) {
  const started = Date.now();
  let lastError;
  while (Date.now() - started < timeoutMs) {
    try {
      const value = await fn();
      if (value) return value;
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`${label} timed out${lastError ? `: ${lastError.message}` : ""}`);
}

const messages = await waitFor("producer final message", async () => {
  const data = await req(`/agent/workspaces/${state.workspace.id}/messages?limit=50`);
  const final = data.messages.find((message) => {
    const raw = JSON.stringify(message.content ?? {});
    return message.role === "assistant" && raw.includes("M1 创作状态建模");
  });
  return final ? data.messages : null;
});
const canvas = await waitFor("domain canvas projection", async () => {
  const data = await req(`/workspaces/${state.workspace.id}/canvas`);
  const kinds = new Set((data.domain_projection?.nodes ?? []).map((node) => node.kind));
  const required = ["creative_brief", "project_memory", "key_element", "key_element_state", "scene", "shot"];
  return required.every((kind) => kinds.has(kind)) ? data : null;
});
const projection = canvas.domain_projection;
const kindCounts = projection.nodes.reduce((acc, node) => {
  acc[node.kind] = (acc[node.kind] ?? 0) + 1;
  return acc;
}, {});
console.log(JSON.stringify({
  message_count: messages.length,
  domain_node_counts: kindCounts,
  domain_edge_count: projection.edges.length,
}, null, 2));
NODE

WORKSPACE_ID="$(jq -r '.workspace.id' "$STATE_FILE")"
if [[ -z "$WORKSPACE_ID" || "$WORKSPACE_ID" == "null" ]]; then
  echo "workspace id missing from state file" >&2
  exit 1
fi

db_rows="$(psql "$DB_DSN" -AtX -F '|' -v workspace_id="$WORKSPACE_ID" <<'SQL'
SELECT 'creative_brief_active', count(*), 1 FROM creative_brief WHERE workspace_id = :'workspace_id'::uuid AND status = 'active'
UNION ALL
SELECT 'project_memory_active', count(*), 1 FROM project_memory WHERE workspace_id = :'workspace_id'::uuid AND status = 'active'
UNION ALL
SELECT 'key_element_active', count(*), 2 FROM key_element WHERE workspace_id = :'workspace_id'::uuid AND archived_at IS NULL
UNION ALL
SELECT 'key_element_state_active', count(*), 2 FROM key_element_state WHERE workspace_id = :'workspace_id'::uuid AND archived_at IS NULL
UNION ALL
SELECT 'scene_active', count(*), 1 FROM scene WHERE workspace_id = :'workspace_id'::uuid AND archived_at IS NULL
UNION ALL
SELECT 'shot_active', count(*), 2 FROM shot WHERE workspace_id = :'workspace_id'::uuid AND archived_at IS NULL
UNION ALL
SELECT 'shot_key_element', count(*), 4 FROM shot_key_element WHERE workspace_id = :'workspace_id'::uuid
UNION ALL
SELECT 'shot_dependency', count(*), 1 FROM shot_dependency WHERE workspace_id = :'workspace_id'::uuid
UNION ALL
SELECT 'agent_task_producer_succeeded', count(*), 1 FROM agent_task WHERE workspace_id = :'workspace_id'::uuid AND task_type = 'producer_turn' AND status = 'succeeded'
UNION ALL
SELECT 'agent_message_assistant_text', count(*), 1 FROM agent_message WHERE workspace_id = :'workspace_id'::uuid AND role = 'assistant' AND message_type = 'text';
SQL
)"

echo "$db_rows"
while IFS='|' read -r name count minimum; do
  if [[ -z "$name" ]]; then
    continue
  fi
  if (( count < minimum )); then
    echo "DB check failed: $name count=$count minimum=$minimum" >&2
    exit 1
  fi
done <<< "$db_rows"

echo "M1 Agent creative state browser+DB E2E checks passed for workspace $WORKSPACE_ID"
