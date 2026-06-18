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

const email = `m4-4-${Date.now()}@clip.test`;
const auth = await req("/auth/register", {
  method: "POST",
  headers: {"Content-Type": "application/json"},
  body: JSON.stringify({email, password: "password123", name: "M4.4 Smoke"}),
});
const headers = {Authorization: `Bearer ${auth.token}`};
const jsonHeaders = {...headers, "Content-Type": "application/json"};
const workspace = await req("/workspaces", {
  method: "POST",
  headers: jsonHeaders,
  body: JSON.stringify({name: "M4.4 Smoke", mode: "studio"}),
});

async function createTextNode(title, prompt, x) {
  return req("/nodes", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({
      workspace_id: workspace.id,
      node_type: "text",
      title,
      prompt,
      operation_type: "text_generation",
      model_provider: "mock",
      model_id: "mock-text",
      model_params: {},
      canvas_x: x,
      canvas_y: 40,
    }),
  });
}

const nodeA = await createTextNode("A", "write source copy v1", 20);
const nodeB = await createTextNode("B", "summarize A", 260);
await req("/edges", {
  method: "POST",
  headers: jsonHeaders,
  body: JSON.stringify({workspace_id: workspace.id, from_node_id: nodeA.id, to_node_id: nodeB.id}),
});

const runA1 = await req(`/nodes/${nodeA.id}/run`, {method: "POST", headers});
const runB1 = await req(`/nodes/${nodeB.id}/run`, {method: "POST", headers});
if (!runA1.version?.input_hash || !runB1.version?.input_hash) {
  throw new Error(`input hashes must be present: ${JSON.stringify({runA1, runB1})}`);
}
const bVersionBefore = runB1.version.id;
const bHashBefore = runB1.version.input_hash;

await req(`/nodes/${nodeA.id}`, {
  method: "PATCH",
  headers: jsonHeaders,
  body: JSON.stringify({prompt: "write source copy v2"}),
});
const runA2 = await req(`/nodes/${nodeA.id}/run`, {method: "POST", headers});
if (runA2.version.id === runA1.version.id) {
  throw new Error("A did not receive a new winner");
}

const staleB = await req(`/nodes/${nodeB.id}`, {headers});
if (staleB.status !== "stale") {
  throw new Error(`B status = ${staleB.status}, want stale`);
}
if (staleB.current_version_id !== bVersionBefore) {
  throw new Error(`B current version changed during stale propagation: ${staleB.current_version_id}`);
}
const reasons = await req(`/nodes/${nodeB.id}/stale-reasons`, {headers});
if (reasons.length !== 1 || reasons[0].upstream_node_id !== nodeA.id) {
  throw new Error(`unexpected stale reasons: ${JSON.stringify(reasons)}`);
}
if (reasons[0].details.previous_input_hash !== bHashBefore) {
  throw new Error(`stale reason does not preserve previous hash: ${JSON.stringify(reasons[0])}`);
}

const runB2 = await req(`/nodes/${nodeB.id}/run`, {method: "POST", headers});
if (runB2.node.status !== "succeeded") {
  throw new Error(`B did not clear stale after rerun: ${JSON.stringify(runB2.node)}`);
}
if (runB2.version.id === bVersionBefore || runB2.version.input_hash === bHashBefore) {
  throw new Error(`B did not create a refreshed version/hash: ${JSON.stringify(runB2.version)}`);
}
const clearedReasons = await req(`/nodes/${nodeB.id}/stale-reasons`, {headers});
if (clearedReasons.length !== 0) {
  throw new Error(`stale reasons were not cleared: ${JSON.stringify(clearedReasons)}`);
}

console.log(JSON.stringify({
  workspaceId: workspace.id,
  nodeAId: nodeA.id,
  nodeBId: nodeB.id,
  aFirstVersion: runA1.version.id,
  aSecondVersion: runA2.version.id,
  bFirstVersion: bVersionBefore,
  bSecondVersion: runB2.version.id,
  bFirstHash: bHashBefore,
  bSecondHash: runB2.version.input_hash,
}, null, 2));
NODE
