#!/usr/bin/env bash
set -euo pipefail

BASE="${CLIPANVIL_API_BASE:-http://127.0.0.1:${CLIPANVIL_SERVER_PORT:-8888}/api}"
WEB_BASE="${CLIPANVIL_WEB_BASE:-http://127.0.0.1:${CLIPANVIL_WEB_PORT:-5173}}"
EMAIL="react-flow-smoke-$(date +%s)@clipanvil.local"
PASSWORD="clipanvil-smoke-pass"
export BASE WEB_BASE EMAIL PASSWORD

node <<'NODE'
const base = process.env.BASE;
const webBase = process.env.WEB_BASE;
const email = process.env.EMAIL;
const password = process.env.PASSWORD;

async function req(path, init = {}) {
  const res = await fetch(base + path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init.headers ?? {}),
    },
  });
  const text = await res.text();
  if (!res.ok) {
    throw new Error(`${init.method || "GET"} ${path} -> ${res.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
}

const auth = await req("/auth/register", {
  method: "POST",
  body: JSON.stringify({ email, password, name: "React Flow Smoke" }),
});
const headers = { Authorization: `Bearer ${auth.token}` };

async function createWorkspace(name, mode) {
  return req("/workspaces", {
    method: "POST",
    headers,
    body: JSON.stringify({ name, mode }),
  });
}

async function createNode(workspace, title, node_type, x, y) {
  return req("/nodes", {
    method: "POST",
    headers,
    body: JSON.stringify({
      workspace_id: workspace.id,
      node_type,
      title,
      prompt: `${title} prompt`,
      canvas_x: x,
      canvas_y: y,
    }),
  });
}

const studio = await createWorkspace("React Flow Studio Smoke", "studio");
const agent = await createWorkspace("React Flow Agent Smoke", "agent");
const scriptNode = await createNode(studio, "Smoke Script", "text", 40, 80);
const imageNode = await createNode(studio, "Smoke Image", "image", 360, 80);

const edge = await req("/edges", {
  method: "POST",
  headers,
  body: JSON.stringify({
    workspace_id: studio.id,
    from_node_id: scriptNode.id,
    to_node_id: imageNode.id,
    edge_type: "dependency",
  }),
});

const group = await req("/groups", {
  method: "POST",
  headers,
  body: JSON.stringify({
    workspace_id: studio.id,
    name: "Smoke Group",
    node_ids: [scriptNode.id, imageNode.id],
  }),
});

const studioCanvas = await req(`/workspaces/${studio.id}/canvas`, { headers });
const agentCanvas = await req(`/workspaces/${agent.id}/canvas`, { headers });

if (studioCanvas.nodes.length !== 2) {
  throw new Error(`studio canvas node count ${studioCanvas.nodes.length}`);
}
if (studioCanvas.edges.length !== 1) {
  throw new Error(`studio canvas edge count ${studioCanvas.edges.length}`);
}
if (studioCanvas.groups.length !== 1) {
  throw new Error(`studio canvas group count ${studioCanvas.groups.length}`);
}
if (agentCanvas.nodes.length !== 0) {
  throw new Error(`agent canvas node count ${agentCanvas.nodes.length}`);
}

console.log(JSON.stringify({
  email,
  password,
  studio_id: studio.id,
  agent_id: agent.id,
  studio_url: `${webBase}/workspaces/${studio.id}/studio`,
  agent_url: `${webBase}/workspaces/${agent.id}/agent`,
  studio_node_ids: [scriptNode.id, imageNode.id],
  agent_canvas_nodes: agentCanvas.nodes.length,
  edge_id: edge.id,
  group_id: group.group?.id ?? group.id,
}, null, 2));
NODE
