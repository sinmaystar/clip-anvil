#!/usr/bin/env bash
set -euo pipefail

node <<'NODE'
const base = process.env.CLIPANVIL_API_BASE || `http://127.0.0.1:${process.env.CLIPANVIL_SERVER_PORT || "8888"}/api`;

async function req(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  if (!res.ok) {
    throw new Error(`${init.method || "GET"} ${path} -> ${res.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
}

const email = `m4-1-${Date.now()}@clip.test`;
const auth = await req("/auth/register", {
  method: "POST",
  headers: {"Content-Type": "application/json"},
  body: JSON.stringify({email, password: "password123", name: "M4.1 Smoke"}),
});
const headers = {Authorization: `Bearer ${auth.token}`};
const workspace = await req("/workspaces", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({name: "M4.1 Smoke", mode: "studio"}),
});
const node = await req("/nodes", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({
    workspace_id: workspace.id,
    node_type: "text",
    title: "Mock copy",
    prompt: "Write a crisp product line",
    canvas_x: 20,
    canvas_y: 40,
  }),
});
const first = await req(`/nodes/${node.id}/run`, {method: "POST", headers});
const second = await req(`/nodes/${node.id}/run`, {method: "POST", headers});
const canvas = await req(`/workspaces/${workspace.id}/canvas`, {headers});

const firstVersion = first.node?.current_version_id || first.current_version_id;
const secondVersion = second.node?.current_version_id || second.current_version_id;

if (!firstVersion || !secondVersion) {
  throw new Error("missing current version");
}
if (firstVersion === secondVersion) {
  throw new Error("re-run did not create a new current version");
}
if (canvas.nodes.length !== 1) {
  throw new Error(`canvas node count ${canvas.nodes.length}`);
}
if (canvas.nodes[0].prompt !== "Write a crisp product line") {
  throw new Error(`canvas prompt ${canvas.nodes[0].prompt}`);
}

console.log(JSON.stringify({
  workspaceId: workspace.id,
  nodeId: node.id,
  firstVersion,
  secondVersion,
  canvasNodes: canvas.nodes.length,
}, null, 2));
NODE
