#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

COMMAND="${1:-}"
STATE_FILE="${CLIPANVIL_E2E_STATE_FILE:-/tmp/clipanvil-m3-reviewer-gate-e2e.json}"
BASE="${CLIPANVIL_API_BASE:-http://127.0.0.1:${CLIPANVIL_SERVER_PORT:-8888}/api}"
WEB_BASE="${CLIPANVIL_WEB_BASE:-${CLIPANVIL_PUBLIC_BASE_URL:-http://127.0.0.1:${CLIPANVIL_WEB_PORT:-5173}}}"
DB_DSN="${CLIPANVIL_POSTGRES_DSN:-postgres://clipanvil:clipanvil_dev@localhost:5432/clipanvil?sslmode=disable}"

usage() {
  cat <<'USAGE'
Usage:
  scripts/smoke-m3-reviewer-gate-e2e.sh setup
  scripts/smoke-m3-reviewer-gate-e2e.sh seed-artifact
  scripts/smoke-m3-reviewer-gate-e2e.sh verify

Environment before starting the server:
  CLIPANVIL_E2E_PRODUCER_FIXTURE=m3_reviewer_gate
  CLIPANVIL_E2E_CRAFTSMAN_FIXTURE=m3_reviewer_gate
  CLIPANVIL_E2E_REVIEWER_FIXTURE=m3_reviewer_gate

Browser flow:
  1. Run setup and open the printed agent_url.
  2. Send message_text_m2 in the Agent UI.
  3. Run seed-artifact after M2 finishes.
  4. Send message_text_m3 in the Agent UI.
  5. Run verify.
USAGE
}

if [[ "$COMMAND" != "setup" && "$COMMAND" != "seed-artifact" && "$COMMAND" != "verify" ]]; then
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
const email = `m3-agent-e2e-${suffix}@clipanvil.local`;
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
  body: JSON.stringify({ email, password, name: "M3 Agent E2E" }),
});
const workspace = await req("/workspaces", {
  method: "POST",
  headers: { Authorization: `Bearer ${auth.token}` },
  body: JSON.stringify({ name: "M3 Reviewer Gate E2E", mode: "agent" }),
});
const state = {
  email,
  password,
  token: auth.token,
  account: auth.account,
  workspace,
  agent_url: `${webBase}/workspaces/${workspace.id}/agent`,
  message_text_m2: "E2E_M2_RENDER_PLAN：请为悦行行李箱做一条机场抖音广告，建立 brief、memory、关键元素、两镜头 storyboard，并派 Craftsman 创建预览图 RenderPlan。",
  message_text_m3: "E2E_M3_REVIEWER_GATE：请对第一个已生成的预览图执行 Reviewer Gate 评审，记录 review 和开放问题。",
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

WORKSPACE_ID="$(jq -r '.workspace.id' "$STATE_FILE")"
if [[ -z "$WORKSPACE_ID" || "$WORKSPACE_ID" == "null" ]]; then
  echo "workspace id missing from state file" >&2
  exit 1
fi

if [[ "$COMMAND" == "seed-artifact" ]]; then
  deadline=$((SECONDS + 60))
  while true; do
    ready_rows="$(psql "$DB_DSN" -AtX -F '|' -v workspace_id="$WORKSPACE_ID" <<'SQL'
SELECT
  (SELECT count(*) FROM shot WHERE workspace_id = :'workspace_id'::uuid AND archived_at IS NULL),
  (SELECT count(*) FROM render_plan WHERE workspace_id = :'workspace_id'::uuid AND status = 'compiled' AND target_phase = 'preview_image');
SQL
)"
    IFS='|' read -r shot_count render_plan_count <<< "$ready_rows"
    if (( shot_count >= 1 && render_plan_count >= 1 )); then
      break
    fi
    if (( SECONDS >= deadline )); then
      echo "M2 data not ready: shots=$shot_count render_plans=$render_plan_count" >&2
      exit 1
    fi
    sleep 1
  done

  psql "$DB_DSN" -v workspace_id="$WORKSPACE_ID" <<'SQL'
WITH first_shot AS (
  SELECT id, client_key
  FROM shot
  WHERE workspace_id = :'workspace_id'::uuid
    AND archived_at IS NULL
  ORDER BY sort_order, created_at
  LIMIT 1
),
created_node AS (
  INSERT INTO media_node (
    workspace_id,
    node_type,
    title,
    prompt,
    prompt_template,
    operation_type,
    status,
    source,
    canvas_x,
    canvas_y,
    canvas_w,
    canvas_h,
    shot_id,
    model_provider,
    model_id,
    model_params,
    metadata
  )
  SELECT
    :'workspace_id'::uuid,
    'image',
    'E2E M3 预览图',
    '悦行行李箱在现代机场出发大厅的 Seedream 预览图。',
    '悦行行李箱在现代机场出发大厅的 Seedream 预览图。',
    'text_to_image',
    'succeeded',
    'agent',
    760,
    120,
    260,
    360,
    first_shot.id,
    'volcengine',
    'seedream-5',
    '{"ratio":"9:16"}'::jsonb,
    '{"agent_artifact_kind":"preview_image","e2e_fixture":"m3_reviewer_gate"}'::jsonb
  FROM first_shot
  RETURNING id, workspace_id
),
created_job AS (
  INSERT INTO generation_job (
    workspace_id,
    target_node_id,
    operation_type,
    provider,
    model_id,
    intent,
    rendered_prompt,
    provider_request,
    provider_response,
    status,
    progress,
    attempt,
    max_attempts,
    retry_policy,
    requested_by_type,
    requested_by_id,
    started_at,
    completed_at
  )
  SELECT
    created_node.workspace_id,
    created_node.id,
    'text_to_image',
    'volcengine',
    'seedream-5',
    '{"target_phase":"preview_image","e2e_fixture":"m3_reviewer_gate"}'::jsonb,
    '悦行行李箱在现代机场出发大厅，清晨自然光，商业广告质感，9:16。',
    '{"model":"seedream-5","ratio":"9:16"}'::jsonb,
    '{"fixture":"m3_reviewer_gate"}'::jsonb,
    'succeeded',
    100,
    1,
    1,
    '{}'::jsonb,
    'agent',
    'm3-reviewer-gate-e2e',
    now(),
    now()
  FROM created_node
  RETURNING id, workspace_id, target_node_id
),
created_version AS (
  INSERT INTO artifact_version (
    workspace_id,
    node_id,
    job_id,
    version_no,
    winner,
    output,
    review_score,
    input_hash,
    status,
    progress,
    provider_request,
    provider_response,
    started_at,
    completed_at
  )
  SELECT
    created_job.workspace_id,
    created_job.target_node_id,
    created_job.id,
    1,
    true,
    '{"text":"E2E M3 preview image artifact","mime":"image/png"}'::jsonb,
    0.82,
    'm3-reviewer-gate-e2e',
    'succeeded',
    100,
    '{"model":"seedream-5","ratio":"9:16"}'::jsonb,
    '{"fixture":"m3_reviewer_gate"}'::jsonb,
    now(),
    now()
  FROM created_job
  RETURNING id, node_id
)
SELECT node_id, id AS version_id
FROM created_version;
SQL
  updated_rows="$(psql "$DB_DSN" -AtX -v workspace_id="$WORKSPACE_ID" <<'SQL'
WITH updated AS (
  UPDATE media_node mn
  SET current_version_id = av.id,
      status = 'succeeded',
      updated_at = now()
  FROM artifact_version av
  WHERE mn.workspace_id = :'workspace_id'::uuid
    AND av.workspace_id = mn.workspace_id
    AND av.node_id = mn.id
    AND mn.title = 'E2E M3 预览图'
    AND av.status = 'succeeded'
    AND mn.current_version_id IS NULL
  RETURNING mn.id
)
SELECT count(*) FROM updated;
SQL
)"
  if (( updated_rows < 1 )); then
    echo "failed to update media_node.current_version_id for seeded artifact" >&2
    exit 1
  fi
  echo "M3 seed artifact inserted for workspace $WORKSPACE_ID"
  exit 0
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

const messages = await waitFor("M3 reviewer gate final message", async () => {
  const data = await req(`/agent/workspaces/${state.workspace.id}/messages?limit=120`);
  const final = data.messages.find((message) => {
    const raw = JSON.stringify(message.content ?? {});
    return message.role === "assistant" && raw.includes("M3 Reviewer Gate");
  });
  return final ? data.messages : null;
});
const canvas = await waitFor("domain canvas projection with review and issue nodes", async () => {
  const data = await req(`/workspaces/${state.workspace.id}/canvas`);
  const nodes = data.domain_projection?.nodes ?? [];
  const kinds = new Set(nodes.map((node) => node.kind));
  return kinds.has("review_record") && kinds.has("artifact_issue") ? data : null;
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

db_rows="$(psql "$DB_DSN" -AtX -F '|' -v workspace_id="$WORKSPACE_ID" <<'SQL'
SELECT 'render_plan_compiled', count(*), 1 FROM render_plan WHERE workspace_id = :'workspace_id'::uuid AND status = 'compiled' AND target_phase = 'preview_image'
UNION ALL
SELECT 'preview_artifact_succeeded', count(*), 1 FROM artifact_version av JOIN media_node mn ON mn.id = av.node_id WHERE av.workspace_id = :'workspace_id'::uuid AND av.status = 'succeeded' AND mn.node_type = 'image' AND mn.source = 'agent'
UNION ALL
SELECT 'review_record_warning', count(*), 1 FROM review_record WHERE workspace_id = :'workspace_id'::uuid AND review_task = 'preview_image_review' AND status = 'accepted_with_warnings' AND target_object_type = 'artifact_version'
UNION ALL
SELECT 'artifact_issue_open', count(*), 1 FROM artifact_issue WHERE workspace_id = :'workspace_id'::uuid AND status = 'open' AND severity = 'warning' AND suggested_fix = 'revise_render_plan'
UNION ALL
SELECT 'reviewer_task_succeeded', count(*), 1 FROM agent_task WHERE workspace_id = :'workspace_id'::uuid AND task_type = 'reviewer_turn' AND role = 'reviewer' AND status = 'succeeded'
UNION ALL
SELECT 'review_queued_event', count(*), 1 FROM agent_event WHERE workspace_id = :'workspace_id'::uuid AND event_type = 'review_queued' AND source_role = 'producer' AND target_role = 'reviewer'
UNION ALL
SELECT 'reviewer_thread', count(*), 1 FROM agent_thread WHERE workspace_id = :'workspace_id'::uuid AND role = 'reviewer' AND runtime_agent_name = 'ReviewerGate';
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

echo "M3 Reviewer Gate browser+DB E2E checks passed for workspace $WORKSPACE_ID"
