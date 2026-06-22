import type { AgentMessage } from "./agentApi";
import { agentMessageBlocks, isDecisionCardBlock } from "./agentMessageBlocks.js";

export interface AgentDecisionOption {
  id: string;
  label: string;
}

export interface AgentDecisionCard {
  decision_id: string;
  title: string;
  message: string;
  status: "pending" | "handled";
  options: AgentDecisionOption[];
  allow_free_text: boolean;
}

export function decisionCardFromMessage(
  message: Pick<AgentMessage, "message_type" | "content">,
): AgentDecisionCard | null {
  const block = agentMessageBlocks(message).find(isDecisionCardBlock);
  if (!block) {
    return null;
  }
  return {
    decision_id: block.decision_id,
    title: block.title,
    message: block.message,
    status: block.status === "handled" ? "handled" : "pending",
    options: block.options.filter(isDecisionOption),
    allow_free_text: block.allow_free_text,
  };
}

export function decisionResolvedFromEventPayload(payload: Record<string, unknown>) {
  return typeof payload.decision_id === "string" ? payload.decision_id : "";
}

function isDecisionOption(value: unknown): value is AgentDecisionOption {
  if (!value || typeof value !== "object") {
    return false;
  }
  const option = value as Partial<AgentDecisionOption>;
  return typeof option.id === "string" && typeof option.label === "string";
}
