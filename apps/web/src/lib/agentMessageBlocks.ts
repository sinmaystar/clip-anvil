import type { AgentAttachment, AgentMessage } from "./agentApi";

export const agentMessageSchemaV1 = "clipanvil.agent.message.v1";

export type AgentMessageBlock =
  | AgentMarkdownBlock
  | AgentThinkingBlock
  | AgentDecisionCardBlock
  | AgentToolStatusBlock
  | AgentAttachmentBlock
  | AgentMediaBlock
  | AgentErrorBlock
  | AgentUnknownBlock;

export interface AgentBaseBlock {
  id: string;
  type: string;
  visibility?: "user" | "debug" | "hidden";
}

export interface AgentMarkdownBlock extends AgentBaseBlock {
  type: "markdown";
  text: string;
}

export interface AgentThinkingBlock extends AgentBaseBlock {
  type: "thinking";
  text: string;
  status: "streaming" | "done";
  default_collapsed: boolean;
}

export interface AgentDecisionCardBlock extends AgentBaseBlock {
  type: "decision_card";
  decision_id: string;
  title: string;
  message: string;
  options: Array<{ id: string; label: string; description?: string }>;
  allow_free_text: boolean;
  status: "pending" | "handled" | "failed" | "cancelled";
  selected_option_id?: string;
  free_text?: string;
}

export interface AgentToolStatusBlock extends AgentBaseBlock {
  type: "tool_status";
  tool_call_id: string;
  tool_name: string;
  label: string;
  status: "running" | "succeeded" | "failed";
  summary?: string;
  error_message?: string;
}

export interface AgentAttachmentBlock extends AgentBaseBlock {
  type: "attachment";
  attachments: AgentAttachment[];
}

export interface AgentMediaBlock extends AgentBaseBlock {
  type: "media";
  asset_id: string;
  node_id?: string;
  kind: "image" | "video" | "text" | "final_video";
  title?: string;
  url?: string;
  thumbnail_url?: string;
  mime?: string;
}

export interface AgentErrorBlock extends AgentBaseBlock {
  type: "error";
  title: string;
  message: string;
  code?: string;
  retryable?: boolean;
}

export interface AgentUnknownBlock extends AgentBaseBlock {
  type: string;
  [key: string]: unknown;
}

export function agentMessageBlocks(
  message: Pick<AgentMessage, "content"> | { content: unknown },
): AgentMessageBlock[] {
  const content = message.content;
  if (!content || typeof content !== "object") {
    return [];
  }
  const envelope = content as { schema?: unknown; blocks?: unknown };
  if (
    envelope.schema !== agentMessageSchemaV1 ||
    !Array.isArray(envelope.blocks)
  ) {
    return [];
  }
  return envelope.blocks.filter(isAgentBlock);
}

export function isUnsupportedAgentMessage(message: { content: unknown }) {
  const content = message.content;
  return (
    !content ||
    typeof content !== "object" ||
    (content as { schema?: unknown }).schema !== agentMessageSchemaV1
  );
}

export function agentMessageMarkdownText(message: { content: unknown }) {
  return agentMessageBlocks(message)
    .filter(
      (block): block is AgentMarkdownBlock =>
        block.type === "markdown" && typeof block.text === "string",
    )
    .map((block) => block.text.trim())
    .filter(Boolean)
    .join("\n\n");
}

export function agentMessageAttachments(
  message: { content: unknown },
): AgentAttachment[] {
  return agentMessageBlocks(message)
    .filter(
      (block): block is AgentAttachmentBlock =>
        block.type === "attachment" &&
        Array.isArray((block as AgentAttachmentBlock).attachments),
    )
    .flatMap((block) => block.attachments.filter(isAgentAttachment));
}

export function isDecisionCardBlock(
  block: unknown,
): block is AgentDecisionCardBlock {
  if (!block || typeof block !== "object") {
    return false;
  }
  const value = block as Partial<AgentDecisionCardBlock>;
  return (
    value.type === "decision_card" &&
    typeof value.id === "string" &&
    typeof value.decision_id === "string" &&
    typeof value.title === "string" &&
    typeof value.message === "string" &&
    Array.isArray(value.options) &&
    typeof value.allow_free_text === "boolean" &&
    typeof value.status === "string"
  );
}

function isAgentBlock(value: unknown): value is AgentMessageBlock {
  if (!value || typeof value !== "object") {
    return false;
  }
  const block = value as Partial<AgentBaseBlock>;
  return typeof block.id === "string" && typeof block.type === "string";
}

function isAgentAttachment(value: unknown): value is AgentAttachment {
  if (!value || typeof value !== "object") {
    return false;
  }
  const attachment = value as Partial<AgentAttachment>;
  return (
    typeof attachment.asset_id === "string" &&
    typeof attachment.node_id === "string" &&
    (attachment.kind === "image" ||
      attachment.kind === "video" ||
      attachment.kind === "text") &&
    typeof attachment.name === "string" &&
    typeof attachment.mime === "string" &&
    typeof attachment.size_bytes === "number"
  );
}
