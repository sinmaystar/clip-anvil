import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { describe, it } from "node:test";
import { URL } from "node:url";
import {
  agentAttachmentKindForFile,
  attachmentAccept,
  formatAgentAttachmentLabel,
  validAgentAttachmentFiles,
} from "../../dist-test/lib/agentAttachments.js";

describe("agent attachments", () => {
  it("classifies image video and text files", () => {
    assert.equal(
      agentAttachmentKindForFile({ type: "image/png", name: "hero.png" }),
      "image",
    );
    assert.equal(
      agentAttachmentKindForFile({ type: "video/mp4", name: "clip.mp4" }),
      "video",
    );
    assert.equal(
      agentAttachmentKindForFile({ type: "text/plain", name: "brief.txt" }),
      "text",
    );
  });

  it("uses file extension fallback for txt files", () => {
    assert.equal(
      agentAttachmentKindForFile({ type: "", name: "brief.txt" }),
      "text",
    );
  });

  it("rejects unsupported files", () => {
    assert.equal(
      agentAttachmentKindForFile({ type: "application/pdf", name: "deck.pdf" }),
      null,
    );
  });

  it("formats compact attachment labels", () => {
    assert.equal(
      formatAgentAttachmentLabel({ kind: "text", name: "creative-brief.txt" }),
      "TXT creative-brief.txt",
    );
  });

  it("keeps all supported files from a multi-file attachment selection", () => {
    const files = [
      { type: "image/png", name: "front.png" },
      { type: "image/jpeg", name: "side.jpg" },
      { type: "application/pdf", name: "manual.pdf" },
      { type: "text/plain", name: "brief.txt" },
    ];

    assert.deepEqual(
      validAgentAttachmentFiles(files).map((file) => file.name),
      ["front.png", "side.jpg", "brief.txt"],
    );
  });

  it("declares accepted input types", () => {
    assert.equal(attachmentAccept.includes("image/*"), true);
    assert.equal(attachmentAccept.includes(".txt"), true);
  });

  it("renders image attachment thumbnails in message bubbles", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentAttachmentBlock.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /<img/);
    assert.match(source, /agent-attachment-thumbnail/);
  });
});
