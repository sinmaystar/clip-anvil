import type { AgentAttachment, AgentMessage } from "./agentApi";

export const agentMessageSchemaV1 = "clipanvil.agent.message.v1";

export type AgentMessageBlock =
  | AgentMarkdownBlock
  | AgentSystemReminderBlock
  | AgentThinkingBlock
  | AgentDecisionCardBlock
  | AgentReviewCardBlock
  | AgentFinalVideoCardBlock
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

export interface AgentSystemReminderBlock extends AgentBaseBlock {
  type: "system_reminder";
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

export interface AgentReviewCardBlock extends AgentBaseBlock {
  type: "review_card";
  review_id: string;
  status: "accepted" | "rejected" | "failed" | "running";
  target_phase: "preview_image" | "shot_video" | "final_video";
  shot_ref: string;
  node_id: string;
  version_id: string;
  overall_score?: number;
  rubric: Record<string, unknown>;
  critique: string;
  retry_count: number;
  max_attempts: number;
  fix_hints?: string[];
}

export interface AgentFinalVideoCardBlock extends AgentBaseBlock {
  type: "final_video_card";
  status: "queued" | "running" | "ready" | "failed" | "waiting_for_confirmation";
  node_id: string;
  version_id: string;
  asset_id: string;
  title: string;
  url?: string;
  thumbnail_url?: string;
  source_shots: string[];
  decision_id?: string;
}

export interface AgentToolStatusBlock extends AgentBaseBlock {
  type: "tool_status";
  tool_call_id: string;
  tool_name: string;
  label: string;
  status: "running" | "succeeded" | "failed";
  summary?: string;
  error_message?: string;
  arguments?: Record<string, unknown>;
  result?: Record<string, unknown>;
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
  return envelope.blocks.filter(isAgentBlock).map(normalizeAgentBlock);
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

export function isReviewCardBlock(
  block: unknown,
): block is AgentReviewCardBlock {
  if (!block || typeof block !== "object") {
    return false;
  }
  const value = block as Partial<AgentReviewCardBlock>;
  return (
    value.type === "review_card" &&
    typeof value.id === "string" &&
    typeof value.review_id === "string" &&
    typeof value.status === "string" &&
    typeof value.target_phase === "string" &&
    typeof value.shot_ref === "string" &&
    typeof value.node_id === "string" &&
    typeof value.version_id === "string" &&
    typeof value.rubric === "object" &&
    value.rubric !== null &&
    typeof value.critique === "string" &&
    typeof value.retry_count === "number" &&
    typeof value.max_attempts === "number"
  );
}

export function isFinalVideoCardBlock(
  block: unknown,
): block is AgentFinalVideoCardBlock {
  if (!block || typeof block !== "object") {
    return false;
  }
  const value = block as Partial<AgentFinalVideoCardBlock>;
  return (
    value.type === "final_video_card" &&
    typeof value.id === "string" &&
    typeof value.status === "string" &&
    typeof value.node_id === "string" &&
    typeof value.version_id === "string" &&
    typeof value.asset_id === "string" &&
    typeof value.title === "string" &&
    Array.isArray(value.source_shots) &&
    value.source_shots.every((shot) => typeof shot === "string")
  );
}

function isAgentBlock(value: unknown): value is AgentMessageBlock {
  if (!value || typeof value !== "object") {
    return false;
  }
  const block = value as Partial<AgentBaseBlock>;
  return typeof block.id === "string" && typeof block.type === "string";
}

function normalizeAgentBlock(block: AgentMessageBlock): AgentMessageBlock {
  if (block.type !== "markdown" || typeof block.text !== "string") {
    return block;
  }
  const systemReminder = extractSystemReminderText(block.text);
  if (!systemReminder) {
    return block;
  }
  return {
    id: block.id,
    type: "system_reminder",
    text: systemReminder,
    visibility: block.visibility,
  };
}

export function extractSystemReminderText(text: string) {
  const match = text.match(/<system-reminder>([\s\S]*?)<\/system-reminder>/i);
  return (match?.[1] ?? "").trim();
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
