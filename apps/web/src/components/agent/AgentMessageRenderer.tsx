import type { AgentMessage } from "../../lib/agentApi";
import {
  agentMessageBlocks,
  agentMessageMarkdownText,
  isDecisionCardBlock,
  isFinalVideoCardBlock,
  isReviewCardBlock,
  type AgentAttachmentBlock as AgentAttachmentBlockData,
  type AgentErrorBlock as AgentErrorBlockData,
  type AgentMarkdownBlock as AgentMarkdownBlockData,
  type AgentMediaBlock as AgentMediaBlockData,
  type AgentMessageBlock,
  type AgentSystemReminderBlock as AgentSystemReminderBlockData,
  type AgentThinkingBlock as AgentThinkingBlockData,
  type AgentToolStatusBlock as AgentToolStatusBlockData,
} from "../../lib/agentMessageBlocks";
import {
  AgentDecisionCardBlock,
  type AgentDecisionActions,
} from "./AgentDecisionCardBlock";
import { AgentAttachmentBlock } from "./AgentAttachmentBlock";
import { AgentErrorBlock } from "./AgentErrorBlock";
import { AgentFinalVideoCardBlock } from "./AgentFinalVideoCardBlock";
import { AgentMarkdownBlock } from "./AgentMarkdownBlock";
import { AgentMediaBlock } from "./AgentMediaBlock";
import { AgentThinkingBlock } from "./AgentThinkingBlock";
import { AgentToolStatusBlock } from "./AgentToolStatusBlock";
import { AgentReviewCardBlock } from "./AgentReviewCardBlock";

export type AgentMessageActions = AgentDecisionActions;

export function AgentMessageRenderer({
  message,
  actions,
}: {
  message: Pick<AgentMessage, "id" | "content" | "message_type">;
  actions?: AgentMessageActions;
}) {
  const blocks = agentMessageBlocks(message);
  if (blocks.length === 0) {
    return <p>{fallbackMessageText(message)}</p>;
  }
  return (
    <>
      {blocks.map((block) => (
        <AgentMessageBlockRenderer
          actions={actions}
          block={block}
          key={`${message.id}-${block.id}`}
        />
      ))}
    </>
  );
}

function AgentMessageBlockRenderer({
  block,
  actions,
}: {
  block: AgentMessageBlock;
  actions?: AgentMessageActions;
}) {
  if (block.visibility === "hidden") {
    return null;
  }
  if (isMarkdownBlock(block)) {
    return <AgentMarkdownBlock block={block} />;
  }
  if (isSystemReminderBlock(block)) {
    return <AgentSystemReminderBlock block={block} />;
  }
  if (isThinkingBlock(block)) {
    return <AgentThinkingBlock block={block} />;
  }
  if (isDecisionCardBlock(block)) {
    return <AgentDecisionCardBlock actions={actions} block={block} />;
  }
  if (isReviewCardBlock(block)) {
    return <AgentReviewCardBlock block={block} />;
  }
  if (isFinalVideoCardBlock(block)) {
    return <AgentFinalVideoCardBlock block={block} />;
  }
  if (isToolStatusBlock(block)) {
    return <AgentToolStatusBlock block={block} />;
  }
  if (isAttachmentBlock(block)) {
    return <AgentAttachmentBlock block={block} />;
  }
  if (isMediaBlock(block)) {
    return <AgentMediaBlock block={block} />;
  }
  if (isErrorBlock(block)) {
    return <AgentErrorBlock block={block} />;
  }
  return (
    <div className="agent-unknown-block">
      <strong>暂不支持的消息块</strong>
      <span>{block.type}</span>
    </div>
  );
}

function isMarkdownBlock(
  block: AgentMessageBlock,
): block is AgentMarkdownBlockData {
  return block.type === "markdown" && typeof block.text === "string";
}

function isSystemReminderBlock(
  block: AgentMessageBlock,
): block is AgentSystemReminderBlockData {
  return block.type === "system_reminder" && typeof block.text === "string";
}

function AgentSystemReminderBlock({
  block,
}: {
  block: AgentSystemReminderBlockData;
}) {
  if (!block.text.trim()) {
    return null;
  }
  return (
    <div className="agent-system-reminder-block">
      <span>system-reminder</span>
      <pre>{block.text}</pre>
    </div>
  );
}

function isThinkingBlock(
  block: AgentMessageBlock,
): block is AgentThinkingBlockData {
  return (
    block.type === "thinking" &&
    typeof block.text === "string" &&
    (block.status === "streaming" || block.status === "done") &&
    typeof block.default_collapsed === "boolean"
  );
}

function isToolStatusBlock(
  block: AgentMessageBlock,
): block is AgentToolStatusBlockData {
  return (
    block.type === "tool_status" &&
    typeof block.tool_call_id === "string" &&
    typeof block.tool_name === "string" &&
    typeof block.label === "string" &&
    (block.status === "running" ||
      block.status === "succeeded" ||
      block.status === "failed")
  );
}

function isAttachmentBlock(
  block: AgentMessageBlock,
): block is AgentAttachmentBlockData {
  return block.type === "attachment" && Array.isArray(block.attachments);
}

function isMediaBlock(block: AgentMessageBlock): block is AgentMediaBlockData {
  return (
    block.type === "media" &&
    typeof block.asset_id === "string" &&
    (block.kind === "image" ||
      block.kind === "video" ||
      block.kind === "text" ||
      block.kind === "final_video")
  );
}

function isErrorBlock(block: AgentMessageBlock): block is AgentErrorBlockData {
  return (
    block.type === "error" &&
    typeof block.title === "string" &&
    typeof block.message === "string"
  );
}

function fallbackMessageText(
  message: Pick<AgentMessage, "content" | "message_type">,
) {
  const markdown = agentMessageMarkdownText(message);
  if (markdown) {
    return markdown;
  }
  if (message.message_type === "tool_call") {
    return "正在调用工具";
  }
  if (message.message_type === "tool_result") {
    return "工具执行完成";
  }
  if (message.message_type === "error") {
    return "发生错误";
  }
  return "暂不支持的消息";
}
