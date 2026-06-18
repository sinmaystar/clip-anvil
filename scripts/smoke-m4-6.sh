#!/usr/bin/env bash
set -euo pipefail

node <<'NODE'
const base = process.env.CLIPANVIL_API_BASE || `http://127.0.0.1:${process.env.CLIPANVIL_SERVER_PORT || "8888"}/api`;

async function req(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  if (!res.ok) throw new Error(`${init.method || "GET"} ${path} -> ${res.status}: ${text}`);
  return text ? JSON.parse(text) : null;
}

const email = `m4-6-${Date.now()}@clip.test`;
const auth = await req("/auth/register", {
  method: "POST",
  headers: {"Content-Type": "application/json"},
  body: JSON.stringify({email, password: "password123", name: "M4.6 Smoke"}),
});
const headers = {Authorization: `Bearer ${auth.token}`};
const jsonHeaders = {...headers, "Content-Type": "application/json"};

const capabilities = await req("/model-capabilities", {headers});
if (!Array.isArray(capabilities) || !capabilities.some((cap) => cap.provider_id === "mock")) {
  throw new Error("model capabilities did not include mock provider");
}
if (!capabilities.some((cap) => cap.provider_id === "internal_ffmpeg" && cap.supported_operations.includes("extract_first_frame"))) {
  throw new Error("model capabilities did not include internal ffmpeg extract_first_frame");
}

const workspace = await req("/workspaces", {
  method: "POST",
  headers: jsonHeaders,
  body: JSON.stringify({name: "M4.6 Smoke", mode: "studio"}),
});

async function createNode(body) {
  return req("/nodes", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({workspace_id: workspace.id, ...body}),
  });
}

const textNode = await createNode({
  node_type: "text",
  title: "Summary",
  prompt: "hello m4.6",
  operation_type: "text_generation",
  model_provider: "mock",
  model_id: "mock-text",
  model_params: {},
  canvas_x: 10,
  canvas_y: 10,
});
const textRun = await req(`/nodes/${textNode.id}/run`, {method: "POST", headers});
const versions = await req(`/nodes/${textNode.id}/versions`, {headers});
if (versions.length !== 1 || !versions[0].winner || !versions[0].asset?.text_content) {
  throw new Error(`bad versions response: ${JSON.stringify(versions)}`);
}
const state = await req(`/nodes/${textNode.id}/production-state`, {headers});
if (!state.current_version || state.latest_job.id !== textRun.job.id || !state.capability) {
  throw new Error(`bad production state: ${JSON.stringify(state)}`);
}
const job = await req(`/jobs/${textRun.job.id}`, {headers});
if (job.id !== textRun.job.id || job.status !== "succeeded") {
  throw new Error(`bad job detail: ${JSON.stringify(job)}`);
}
const textSandboxJobs = await req(`/jobs/${textRun.job.id}/sandbox-jobs`, {headers});
if (!Array.isArray(textSandboxJobs) || textSandboxJobs.length !== 0) {
  throw new Error(`mock text job should not have sandbox jobs: ${JSON.stringify(textSandboxJobs)}`);
}

const video = await createNode({
  node_type: "video",
  title: "Video V",
  prompt: "video v",
  operation_type: "text_to_video",
  model_provider: "mock",
  model_id: "mock-video",
  model_params: {},
  canvas_x: 10,
  canvas_y: 260,
});
await req(`/nodes/${video.id}/run`, {method: "POST", headers});
const firstFrame = await createNode({
  node_type: "image",
  title: "First frame",
  prompt: "first frame",
  operation_type: "extract_first_frame",
  model_provider: "internal_ffmpeg",
  model_id: "ffmpeg",
  model_params: {},
  canvas_x: 260,
  canvas_y: 260,
});
await req("/edges", {
  method: "POST",
  headers: jsonHeaders,
  body: JSON.stringify({workspace_id: workspace.id, from_node_id: video.id, to_node_id: firstFrame.id}),
});
const frameRun = await req(`/nodes/${firstFrame.id}/run`, {method: "POST", headers});
const frameSandboxJobs = await req(`/jobs/${frameRun.job.id}/sandbox-jobs`, {headers});
if (frameSandboxJobs.length !== 1 || frameSandboxJobs[0].status !== "succeeded") {
  throw new Error(`bad frame sandbox jobs: ${JSON.stringify(frameSandboxJobs)}`);
}
const sandboxJob = await req(`/sandbox-jobs/${frameSandboxJobs[0].id}`, {headers});
if (sandboxJob.id !== frameSandboxJobs[0].id || sandboxJob.generation_job_id !== frameRun.job.id || !sandboxJob.command) {
  throw new Error(`bad sandbox job detail: ${JSON.stringify(sandboxJob)}`);
}
const frameState = await req(`/nodes/${firstFrame.id}/production-state`, {headers});
if (!frameState.current_version?.asset?.access_url || frameState.sandbox_jobs.length !== 1) {
  throw new Error(`bad frame production state: ${JSON.stringify(frameState)}`);
}

console.log(JSON.stringify({
  workspaceId: workspace.id,
  textNodeId: textNode.id,
  capabilityCount: capabilities.length,
  versionId: versions[0].id,
  textJobId: job.id,
  textSandboxJobs: textSandboxJobs.length,
  frameNodeId: firstFrame.id,
  frameJobId: frameRun.job.id,
  frameSandboxJobId: sandboxJob.id,
}, null, 2));
NODE
