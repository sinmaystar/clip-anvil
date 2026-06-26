import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentMessageAttachments,
  agentMessageBlocks,
  agentMessageMarkdownText,
  isDecisionCardBlock,
  isFinalVideoCardBlock,
  isReviewCardBlock,
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

  it("normalizes system-reminder markdown into a visible system reminder block", () => {
    const message = {
      content: {
        schema: "clipanvil.agent.message.v1",
        blocks: [
          {
            id: "blk_text",
            type: "markdown",
            text: "<system-reminder>\n系统事件：Craftsman 已完成 RenderPlan 编译。\n</system-reminder>",
          },
        ],
      },
    };

    assert.deepEqual(agentMessageBlocks(message), [
      {
        id: "blk_text",
        type: "system_reminder",
        text: "系统事件：Craftsman 已完成 RenderPlan 编译。",
        visibility: undefined,
      },
    ]);
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

  it("guards review card blocks", () => {
    assert.equal(
      isReviewCardBlock({
        id: "blk_review",
        type: "review_card",
        review_id: "review-1",
        status: "rejected",
        target_phase: "preview_image",
        shot_ref: "shot-01",
        node_id: "node-1",
        version_id: "version-1",
        overall_score: 0.52,
        rubric: { visual_quality: { score: 0.4, pass: false } },
        critique: "商品不够清晰",
        retry_count: 1,
        max_attempts: 3,
      }),
      true,
    );
  });

  it("guards final video card blocks", () => {
    assert.equal(
      isFinalVideoCardBlock({
        id: "blk_final",
        type: "final_video_card",
        status: "ready",
        node_id: "node-1",
        version_id: "version-1",
        asset_id: "asset-1",
        title: "成片",
        url: "http://localhost/final.mp4",
        source_shots: ["shot-01", "shot-02"],
      }),
      true,
    );
  });

  it("marks missing schema as unsupported", () => {
    assert.equal(isUnsupportedAgentMessage({ content: { text: "old" } }), true);
  });
});
