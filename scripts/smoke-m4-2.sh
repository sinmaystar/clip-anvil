#!/usr/bin/env bash
set -euo pipefail

node <<'NODE'
const base = process.env.CLIPANVIL_API_BASE || `http://127.0.0.1:${process.env.CLIPANVIL_SERVER_PORT || "8888"}/api`;

async function req(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  if (!res.ok) {
    const error = new Error(`${init.method || "GET"} ${path} -> ${res.status}: ${text}`);
    error.status = res.status;
    error.body = text;
    throw error;
  }
  return text ? JSON.parse(text) : null;
}

async function reqAllowError(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  return {status: res.status, body: text ? JSON.parse(text) : null};
}

const email = `m4-2-${Date.now()}@clip.test`;
const auth = await req("/auth/register", {
  method: "POST",
  headers: {"Content-Type": "application/json"},
  body: JSON.stringify({email, password: "password123", name: "M4.2 Smoke"}),
});
const headers = {Authorization: `Bearer ${auth.token}`};
const workspace = await req("/workspaces", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({name: "M4.2 Smoke", mode: "studio"}),
});

const mockNode = await req("/nodes", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({
    workspace_id: workspace.id,
    node_type: "text",
    title: "Mock intent",
    prompt: "Write a crisp product line",
    operation_type: "text_generation",
    model_provider: "mock",
    model_id: "mock-text",
    model_params: {temperature: 0.2},
    canvas_x: 20,
    canvas_y: 40,
  }),
});

const run = await req(`/nodes/${mockNode.id}/run`, {method: "POST", headers});
if (!run.node?.current_version_id || run.job?.status !== "succeeded") {
  throw new Error(`mock run did not succeed: ${JSON.stringify(run)}`);
}
if (run.job.intent.model.provider !== "mock" || run.job.intent.model.model_id !== "mock-text") {
  throw new Error(`unexpected intent model: ${JSON.stringify(run.job.intent.model)}`);
}
if (run.job.intent.params.temperature !== 0.2) {
  throw new Error(`unexpected intent params: ${JSON.stringify(run.job.intent.params)}`);
}
if (run.job.rendered_prompt !== "Write a crisp product line") {
  throw new Error(`unexpected rendered prompt: ${run.job.rendered_prompt}`);
}
if (run.job.provider_request.provider !== "mock" || run.job.provider_response.provider !== "mock") {
  throw new Error(`mock provider summaries missing: ${JSON.stringify(run.job)}`);
}

const jobs = await req(`/nodes/${mockNode.id}/jobs`, {headers});
if (jobs.length !== 1 || jobs[0].id !== run.job.id) {
  throw new Error(`job listing mismatch: ${JSON.stringify(jobs)}`);
}

const realNode = await req("/nodes", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({
    workspace_id: workspace.id,
    node_type: "text",
    title: "Missing key",
    prompt: "This should not call a real provider",
    operation_type: "text_generation",
    model_provider: "volcengine",
    model_id: "doubao-seed-1-6-lite",
    model_params: {temperature: 0.1},
    canvas_x: 40,
    canvas_y: 60,
  }),
});

const failed = await reqAllowError(`/nodes/${realNode.id}/run`, {method: "POST", headers});
if (failed.status !== 400) {
  throw new Error(`expected missing key status 400, got ${failed.status}: ${JSON.stringify(failed.body)}`);
}
if (failed.body.job?.status !== "failed" || failed.body.job?.error_code !== "provider_config_error") {
  throw new Error(`missing key failure was not persisted: ${JSON.stringify(failed.body)}`);
}

console.log(JSON.stringify({
  workspaceId: workspace.id,
  mockNodeId: mockNode.id,
  mockJobId: run.job.id,
  failedNodeId: realNode.id,
  failedJobId: failed.body.job.id,
}, null, 2));
NODE
