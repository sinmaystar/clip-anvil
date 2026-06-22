import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentMessageAttachments,
  agentMessageBlocks,
  agentMessageMarkdownText,
  isDecisionCardBlock,
  isUnsupportedAgentMessage,
} from "../../dist-test/lib/agentMessageBlocks.js";

describe("agent message blocks", () => {
  it("returns v1 blocks", () => {
    const message = {
      content: {
        schema: "clipanvil.agent.message.v1",
        blocks: [{ id: "blk_answer", type: "markdown", text: "hello" }],
      },
    };

    assert.deepEqual(agentMessageBlocks(message), [
      { id: "blk_answer", type: "markdown", text: "hello" },
    ]);
  });

  it("joins markdown blocks only", () => {
    const message = {
      content: {
        schema: "clipanvil.agent.message.v1",
        blocks: [
          { id: "blk_thinking", type: "thinking", text: "hidden" },
          { id: "blk_answer", type: "markdown", text: "visible" },
        ],
      },
    };

    assert.equal(agentMessageMarkdownText(message), "visible");
  });

  it("reads attachment blocks", () => {
    const message = {
      content: {
        schema: "clipanvil.agent.message.v1",
        blocks: [
          {
            id: "blk_attachment",
            type: "attachment",
            attachments: [
              {
                asset_id: "asset-1",
                node_id: "node-1",
                kind: "image",
                name: "hero.png",
                mime: "image/png",
                size_bytes: 123,
              },
            ],
          },
        ],
      },
    };

    assert.equal(agentMessageAttachments(message).length, 1);
    assert.equal(agentMessageAttachments(message)[0].name, "hero.png");
  });

  it("preserves hydrated attachment preview urls", () => {
    const message = {
      content: {
        schema: "clipanvil.agent.message.v1",
        blocks: [
          {
            id: "blk_attachment",
            type: "attachment",
            attachments: [
              {
                asset_id: "asset-1",
                node_id: "node-1",
                kind: "image",
                name: "hero.png",
                mime: "image/png",
                size_bytes: 123,
                url: "http://localhost/hero.png",
                thumbnail_url: "http://localhost/hero-thumb.png",
              },
            ],
          },
        ],
      },
    };

    const [attachment] = agentMessageAttachments(message);

    assert.equal(attachment.url, "http://localhost/hero.png");
    assert.equal(attachment.thumbnail_url, "http://localhost/hero-thumb.png");
  });

  it("guards decision card blocks", () => {
    assert.equal(
      isDecisionCardBlock({
        id: "blk_decision",
        type: "decision_card",
        decision_id: "decision-1",
        title: "确认",
        message: "请选择",
        options: [{ id: "a", label: "方案 A" }],
        allow_free_text: true,
        status: "pending",
      }),
      true,
    );
  });

  it("marks missing schema as unsupported", () => {
    assert.equal(isUnsupportedAgentMessage({ content: { text: "old" } }), true);
  });
});
