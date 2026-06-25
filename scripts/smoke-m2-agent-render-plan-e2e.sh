#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

COMMAND="${1:-}"
STATE_FILE="${CLIPANVIL_E2E_STATE_FILE:-/tmp/clipanvil-m2-agent-render-plan-e2e.json}"
BASE="${CLIPANVIL_API_BASE:-http://127.0.0.1:${CLIPANVIL_SERVER_PORT:-8888}/api}"
WEB_BASE="${CLIPANVIL_WEB_BASE:-${CLIPANVIL_PUBLIC_BASE_URL:-http://127.0.0.1:${CLIPANVIL_WEB_PORT:-5173}}}"
DB_DSN="${CLIPANVIL_POSTGRES_DSN:-postgres://clipanvil:clipanvil_dev@localhost:5432/clipanvil?sslmode=disable}"

usage() {
  cat <<'USAGE'
Usage:
  scripts/smoke-m2-agent-render-plan-e2e.sh setup
  scripts/smoke-m2-agent-render-plan-e2e.sh verify

Environment:
  CLIPANVIL_E2E_PRODUCER_FIXTURE=m2_render_plan and CLIPANVIL_E2E_CRAFTSMAN_FIXTURE=m2_render_plan must be set before starting the server.
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
const email = `m2-agent-e2e-${suffix}@clipanvil.local`;
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
  body: JSON.stringify({ email, password, name: "M2 Agent E2E" }),
});
const workspace = await req("/workspaces", {
  method: "POST",
  headers: { Authorization: `Bearer ${auth.token}` },
  body: JSON.stringify({ name: "M2 Agent RenderPlan E2E", mode: "agent" }),
});
const state = {
  email,
  password,
  token: auth.token,
  account: auth.account,
  workspace,
  agent_url: `${webBase}/workspaces/${workspace.id}/agent`,
  message_text: "E2E_M2_RENDER_PLAN：请为悦行行李箱做一条机场抖音广告，建立 brief、memory、关键元素、两镜头 storyboard，并派 Craftsman 创建预览图 RenderPlan。",
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

async function waitFor(label, fn, timeoutMs = 45000) {
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
  const data = await req(`/agent/workspaces/${state.workspace.id}/messages?limit=80`);
  const final = data.messages.find((message) => {
    const raw = JSON.stringify(message.content ?? {});
    return message.role === "assistant" && raw.includes("M2 生成计划");
  });
  return final ? data.messages : null;
});
const canvas = await waitFor("domain canvas projection with render plans", async () => {
  const data = await req(`/workspaces/${state.workspace.id}/canvas`);
  const nodes = data.domain_projection?.nodes ?? [];
  const renderPlans = nodes.filter((node) => node.kind === "render_plan");
  const shots = nodes.filter((node) => node.kind === "shot");
  return renderPlans.length >= 2 && shots.length >= 2 ? data : null;
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
SELECT 'shot_active', count(*), 2 FROM shot WHERE workspace_id = :'workspace_id'::uuid AND archived_at IS NULL
UNION ALL
SELECT 'render_plan_compiled', count(*), 2 FROM render_plan WHERE workspace_id = :'workspace_id'::uuid AND status = 'compiled' AND target_phase = 'preview_image' AND compiled_prompt <> ''
UNION ALL
SELECT 'craftsman_task_succeeded', count(*), 2 FROM agent_task WHERE workspace_id = :'workspace_id'::uuid AND task_type = 'craftsman_turn' AND status = 'succeeded'
UNION ALL
SELECT 'producer_task_succeeded', count(*), 1 FROM agent_task WHERE workspace_id = :'workspace_id'::uuid AND task_type = 'producer_turn' AND status = 'succeeded'
UNION ALL
SELECT 'craftsman_events', count(*), 2 FROM agent_event WHERE workspace_id = :'workspace_id'::uuid AND source_role = 'craftsman'
UNION ALL
SELECT 'render_plan_edges', count(*), 2 FROM render_plan WHERE workspace_id = :'workspace_id'::uuid AND scope_type = 'shot';
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

echo "M2 Agent RenderPlan browser+DB E2E checks passed for workspace $WORKSPACE_ID"
