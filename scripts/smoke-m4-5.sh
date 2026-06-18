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

async function reqAllowError(path, init = {}) {
  const res = await fetch(base + path, init);
  const text = await res.text();
  return {status: res.status, body: text ? JSON.parse(text) : null};
}

const email = `m4-5-${Date.now()}@clip.test`;
const auth = await req("/auth/register", {
  method: "POST",
  headers: {"Content-Type": "application/json"},
  body: JSON.stringify({email, password: "password123", name: "M4.5 Smoke"}),
});
const headers = {Authorization: `Bearer ${auth.token}`};
const jsonHeaders = {...headers, "Content-Type": "application/json"};
const workspace = await req("/workspaces", {
  method: "POST",
  headers: jsonHeaders,
  body: JSON.stringify({name: "M4.5 Smoke", mode: "studio"}),
});

async function createNode(body) {
  return req("/nodes", {method: "POST", headers: jsonHeaders, body: JSON.stringify({workspace_id: workspace.id, ...body})});
}

const imageA = await createNode({
  node_type: "image",
  title: "Image A",
  prompt: "image a",
  operation_type: "text_to_image",
  model_provider: "mock",
  model_id: "mock-image-only",
  model_params: {},
  canvas_x: 20,
  canvas_y: 40,
});
const imageB = await createNode({
  node_type: "image",
  title: "Image B",
  prompt: "image b",
  operation_type: "text_to_image",
  model_provider: "mock",
  model_id: "mock-image-only",
  model_params: {},
  canvas_x: 220,
  canvas_y: 40,
});
const runA1 = await req(`/nodes/${imageA.id}/run`, {method: "POST", headers});
const runB1 = await req(`/nodes/${imageB.id}/run`, {method: "POST", headers});
if (!runA1.version?.input_hash || !runB1.version?.input_hash) throw new Error("image winners missing hashes");

const pack = await createNode({
  node_type: "reference_pack",
  title: "Pack P",
  prompt: "",
  canvas_x: 420,
  canvas_y: 40,
});
const items = await req(`/reference-packs/${pack.id}/items`, {
  method: "PUT",
  headers: jsonHeaders,
  body: JSON.stringify({member_node_ids: [imageA.id, imageB.id]}),
});
if (items.length !== 2) throw new Error(`pack item count = ${items.length}`);
const canvas = await req(`/workspaces/${workspace.id}/canvas`, {headers});
if (canvas.edges.length !== 0) throw new Error(`membership created dependency edges: ${canvas.edges.length}`);

const consumer = await createNode({
  node_type: "text",
  title: "Consumer C",
  prompt: "describe direct pack members",
  operation_type: "text_generation",
  model_provider: "mock",
  model_id: "mock-text",
  model_params: {},
  canvas_x: 620,
  canvas_y: 40,
});
await req("/edges", {
  method: "POST",
  headers: jsonHeaders,
  body: JSON.stringify({workspace_id: workspace.id, from_node_id: pack.id, to_node_id: consumer.id}),
});
const runC1 = await req(`/nodes/${consumer.id}/run`, {method: "POST", headers});
const refs = runC1.job.intent.input_refs || [];
const memberRefs = refs.filter((ref) => ref.kind === "reference_pack_member");
if (memberRefs.length !== 2) throw new Error(`expanded member refs = ${JSON.stringify(refs)}`);

await req(`/reference-packs/${pack.id}/items`, {
  method: "PUT",
  headers: jsonHeaders,
  body: JSON.stringify({member_node_ids: [imageA.id]}),
});
const staleConsumer = await req(`/nodes/${consumer.id}`, {headers});
if (staleConsumer.status !== "stale") throw new Error(`consumer status after membership change = ${staleConsumer.status}`);

await req(`/nodes/${consumer.id}/run`, {method: "POST", headers});
await req(`/nodes/${imageA.id}`, {
  method: "PATCH",
  headers: jsonHeaders,
  body: JSON.stringify({prompt: "image a changed"}),
});
await req(`/nodes/${imageA.id}/run`, {method: "POST", headers});
const staleAfterMember = await req(`/nodes/${consumer.id}`, {headers});
if (staleAfterMember.status !== "stale") throw new Error(`consumer status after member winner change = ${staleAfterMember.status}`);

const video = await createNode({
  node_type: "video",
  title: "Video V",
  prompt: "video v",
  operation_type: "text_to_video",
  model_provider: "mock",
  model_id: "mock-video",
  model_params: {},
  canvas_x: 20,
  canvas_y: 280,
});
await req(`/nodes/${video.id}/run`, {method: "POST", headers});

async function createExtractNode(title, operation, x, params = {}) {
  const node = await createNode({
    node_type: "image",
    title,
    prompt: title,
    operation_type: operation,
    model_provider: "internal_ffmpeg",
    model_id: "ffmpeg",
    model_params: params,
    canvas_x: x,
    canvas_y: 280,
  });
  await req("/edges", {
    method: "POST",
    headers: jsonHeaders,
    body: JSON.stringify({workspace_id: workspace.id, from_node_id: video.id, to_node_id: node.id}),
  });
  return node;
}

const firstFrame = await createExtractNode("First frame", "extract_first_frame", 260);
const lastFrame = await createExtractNode("Last frame", "extract_last_frame", 500);
const firstRun = await req(`/nodes/${firstFrame.id}/run`, {method: "POST", headers});
const lastRun = await req(`/nodes/${lastFrame.id}/run`, {method: "POST", headers});
if (firstRun.job.provider !== "internal_ffmpeg" || lastRun.job.provider !== "internal_ffmpeg") {
  throw new Error("internal ffmpeg jobs were not persisted");
}
if (!firstRun.job.provider_response?.sandbox_job_id || !lastRun.job.provider_response?.sandbox_job_id) {
  throw new Error(`internal ffmpeg did not persist sandbox job ids: ${JSON.stringify({
    first: firstRun.job.provider_response,
    last: lastRun.job.provider_response,
  })}`);
}
if (firstRun.version.asset_id === lastRun.version.asset_id) throw new Error("frame operations reused one asset");

const failingExtract = await createExtractNode("Fail frame", "extract_first_frame", 740, {mock_extract_fail: true});
const failed = await reqAllowError(`/nodes/${failingExtract.id}/run`, {method: "POST", headers});
if (failed.status !== 500 || failed.body.job?.status !== "failed" || failed.body.job.error_code !== "provider_error") {
  throw new Error(`failed extraction was not persisted: ${JSON.stringify(failed)}`);
}

console.log(JSON.stringify({
  workspaceId: workspace.id,
  packId: pack.id,
  consumerId: consumer.id,
  expandedMemberRefs: memberRefs.length,
  firstFrameVersion: firstRun.version.id,
  lastFrameVersion: lastRun.version.id,
  firstFrameSandboxJobId: firstRun.job.provider_response.sandbox_job_id,
  lastFrameSandboxJobId: lastRun.job.provider_response.sandbox_job_id,
  failedJobId: failed.body.job.id,
}, null, 2));
NODE
