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

async function reqAllowError(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  return {status: res.status, body: text ? JSON.parse(text) : null};
}

const email = `m4-3-${Date.now()}@clip.test`;
const auth = await req("/auth/register", {
  method: "POST",
  headers: {"Content-Type": "application/json"},
  body: JSON.stringify({email, password: "password123", name: "M4.3 Smoke"}),
});
const headers = {Authorization: `Bearer ${auth.token}`};
const workspace = await req("/workspaces", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({name: "M4.3 Smoke", mode: "studio"}),
});

const mismatchNode = await req("/nodes", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({
    workspace_id: workspace.id,
    node_type: "video",
    title: "Capability mismatch",
    prompt: "make a video",
    operation_type: "text_to_video",
    model_provider: "mock",
    model_id: "mock-image-only",
    model_params: {},
    canvas_x: 20,
    canvas_y: 40,
  }),
});
const mismatch = await reqAllowError(`/nodes/${mismatchNode.id}/run`, {method: "POST", headers});
if (mismatch.status !== 400 || mismatch.body.job?.error_code !== "capability_mismatch") {
  throw new Error(`capability mismatch was not persisted: ${JSON.stringify(mismatch)}`);
}
if (mismatch.body.job.provider_response.code !== "capability_mismatch") {
  throw new Error(`capability response missing code: ${JSON.stringify(mismatch.body.job)}`);
}

const failNode = await req("/nodes", {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({
    workspace_id: workspace.id,
    node_type: "text",
    title: "Mock failure",
    prompt: "force provider failure",
    operation_type: "text_generation",
    model_provider: "mock",
    model_id: "mock-text",
    model_params: {mock_fail: true},
    canvas_x: 40,
    canvas_y: 80,
  }),
});
const failedRun = await reqAllowError(`/nodes/${failNode.id}/run`, {
  method: "POST",
  headers: {...headers, "Content-Type": "application/json"},
  body: JSON.stringify({max_attempts: 2}),
});
if (failedRun.status !== 500 || failedRun.body.job?.status !== "failed") {
  throw new Error(`provider failure did not return failed job: ${JSON.stringify(failedRun)}`);
}
const jobsAfterAuto = await req(`/nodes/${failNode.id}/jobs`, {headers});
if (jobsAfterAuto.length !== 2) {
  throw new Error(`auto retry should create exactly 2 jobs, got ${jobsAfterAuto.length}`);
}
if (jobsAfterAuto[1].attempt !== 2 || jobsAfterAuto[1].parent_job_id !== jobsAfterAuto[0].id) {
  throw new Error(`second job is not linked attempt 2: ${JSON.stringify(jobsAfterAuto[1])}`);
}
if (!jobsAfterAuto[1].id || jobsAfterAuto[1].id === jobsAfterAuto[0].id) {
  throw new Error(`retry did not create a distinct job: ${JSON.stringify(jobsAfterAuto)}`);
}

const retryAfterMax = await reqAllowError(`/jobs/${jobsAfterAuto[1].id}/retry`, {
  method: "POST",
  headers,
});
if (retryAfterMax.status !== 400) {
  throw new Error(`retry after max returned unexpected status ${retryAfterMax.status}`);
}
const jobsAfterExhaust = await req(`/nodes/${failNode.id}/jobs`, {headers});
if (jobsAfterExhaust.length !== 2) {
  throw new Error(`retry beyond max created extra job: ${jobsAfterExhaust.length}`);
}

console.log(JSON.stringify({
  workspaceId: workspace.id,
  mismatchNodeId: mismatchNode.id,
  mismatchJobId: mismatch.body.job.id,
  failedNodeId: failNode.id,
  failedAttempts: jobsAfterExhaust.length,
  latestFailedJobId: jobsAfterExhaust[jobsAfterExhaust.length - 1].id,
}, null, 2));
NODE
