import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  formatInputHash,
  jobDetailRows,
  staleReasonText,
  versionCallRecordBlocks,
  versionDetailRows,
  versionHasCallRecord,
  versionRows,
  winnerPreviewText,
} from "../../dist-test/lib/productionPreview.js";

describe("production preview helpers", () => {
  it("prefers current winner text over prompt text", () => {
    assert.equal(
      winnerPreviewText({
        node_type: "text",
        prompt: "old prompt",
        production_preview: {
          version_id: "version-1",
          version_no: 1,
          text: "new generated output",
          asset_type: "text",
        },
      }),
      "new generated output",
    );
  });

  it("shortens input hashes for version rows", () => {
    assert.equal(formatInputHash("sha256:abcdef1234567890"), "abcdef12");
  });

  it("builds version rows with winner and asset labels", () => {
    const rows = versionRows([
      {
        id: "v1",
        workspace_id: "workspace",
        node_id: "node",
        version_no: 1,
        winner: false,
        status: "succeeded",
        progress: 100,
        input_hash: "sha256:111111111111",
        created_at: "2026-06-19T00:00:00Z",
        output: {},
        provider_request: {},
        provider_response: {},
        asset: {
          id: "asset-1",
          type: "text",
          mime: "text/plain",
          metadata: {},
        },
      },
      {
        id: "v2",
        workspace_id: "workspace",
        node_id: "node",
        version_no: 2,
        winner: true,
        status: "succeeded",
        progress: 100,
        input_hash: "sha256:222222222222",
        created_at: "2026-06-19T00:01:00Z",
        output: {},
        provider_request: {},
        provider_response: {},
        asset: {
          id: "asset-2",
          type: "image",
          mime: "image/png",
          size_bytes: 2048,
          metadata: { width: 1600, height: 900 },
        },
      },
    ]);

    assert.deepEqual(
      rows.map((row) => [row.versionLabel, row.assetLabel, row.isWinner]),
      [
        ["v2", "image", true],
        ["v1", "text", false],
      ],
    );
    assert.equal(rows[0].assetDetail, "image/png · 1600x900 · 2 KB");
    assert.equal(rows[0].canSelect, false);
    assert.equal(rows[1].canSelect, true);
  });

  it("keeps running and failed versions visible but not selectable", () => {
    const rows = versionRows([
      {
        id: "v3",
        workspace_id: "workspace",
        node_id: "node",
        version_no: 3,
        winner: false,
        status: "running",
        progress: 42,
        input_hash: "sha256:333333333333",
        created_at: "2026-06-19T00:02:00Z",
        output: {},
        provider_request: {},
        provider_response: { task_id: "task-1" },
      },
      {
        id: "v4",
        workspace_id: "workspace",
        node_id: "node",
        version_no: 4,
        winner: false,
        status: "failed",
        progress: 0,
        input_hash: "",
        error_message: "provider timeout",
        created_at: "2026-06-19T00:03:00Z",
        output: {},
        provider_request: {},
        provider_response: {},
      },
    ]);

    assert.deepEqual(
      rows.map((row) => [
        row.versionLabel,
        row.statusLabel,
        row.assetDetail,
        row.canSelect,
      ]),
      [
        ["v4", "failed", "provider timeout", false],
        ["v3", "running", "42%", false],
      ],
    );
  });

  it("formats version detail rows from the bound run record", () => {
    const rows = versionDetailRows({
      id: "v5",
      workspace_id: "workspace",
      node_id: "node",
      job_id: "job-5",
      version_no: 5,
      winner: true,
      status: "succeeded",
      progress: 100,
      input_hash: "sha256:abcdef1234567890",
      created_at: "2026-06-19T00:00:00Z",
      started_at: "2026-06-19T00:00:02Z",
      completed_at: "2026-06-19T00:00:12Z",
      output: {},
      provider_request: { prompt: "rendered prompt" },
      provider_response: { task_id: "task-5" },
      asset: {
        id: "asset-5",
        type: "image",
        mime: "image/png",
        size_bytes: 2048,
        metadata: { width: 1600, height: 900 },
      },
    });

    assert.deepEqual(
      rows.map((row) => [row.label, row.value]),
      [
        ["Status", "succeeded"],
        ["Version", "v5 · current"],
        ["Asset", "image/png · 1600x900 · 2 KB"],
        ["Input Hash", "abcdef12"],
        ["Job", "job-5"],
        ["Started", "2026-06-19T00:00:02Z"],
        ["Completed", "2026-06-19T00:00:12Z"],
      ],
    );
  });

  it("exposes version-bound call record blocks only when data exists", () => {
    const version = {
      id: "v6",
      workspace_id: "workspace",
      node_id: "node",
      job_id: "job-6",
      version_no: 6,
      winner: false,
      status: "failed",
      progress: 73,
      input_hash: "",
      error_code: "provider_error",
      error_message: "provider timeout",
      created_at: "2026-06-19T00:00:00Z",
      output: {},
      provider_request: { prompt: "make image" },
      provider_response: { status: "failed", task_id: "task-6" },
    };

    assert.equal(versionHasCallRecord(version), true);
    assert.deepEqual(
      versionCallRecordBlocks(version).map((block) => block.label),
      ["Error", "Provider Request", "Provider Response"],
    );
  });

  it("exposes latest job details including rendered prompt and errors", () => {
    const rows = jobDetailRows({
      status: "failed",
      operation_type: "text_generation",
      provider: "mock",
      model_id: "mock-text",
      rendered_prompt: "hello",
      attempt: 1,
      max_attempts: 1,
      error_code: "provider_failed",
      error_message: "mock provider failure",
    });

    assert.equal(rows.find((row) => row.label === "Prompt")?.value, "hello");
    assert.equal(
      rows.find((row) => row.label === "Error")?.value,
      "provider_failed: mock provider failure",
    );
  });

  it("renders stale reasons with upstream context", () => {
    assert.equal(
      staleReasonText({
        reason_code: "upstream_current_version_changed",
        reason_message: "Upstream dependency current version changed.",
        upstream_node_id: "node-a",
        upstream_version_id: "version-a",
        details: {
          old_input_hash: "sha256:old",
          new_input_hash: "sha256:new",
        },
      }),
      "Upstream dependency current version changed. (node-a -> version-a)",
    );
  });
});
