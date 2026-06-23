export type AgentReasoningEffort = "minimal" | "low" | "medium" | "high";

export interface AgentThinkingOptionLike {
  supports_thinking?: boolean;
  reasoning_efforts?: string[];
  default_reasoning_effort?: string;
}

const effortLabels: Record<AgentReasoningEffort, string> = {
  minimal: "关闭",
  low: "低",
  medium: "中",
  high: "高",
};

const validEfforts: AgentReasoningEffort[] = [
  "minimal",
  "low",
  "medium",
  "high",
];

export function agentThinkingEffortLabel(effort: string) {
  if (isAgentReasoningEffort(effort)) {
    return effortLabels[effort];
  }
  return effort;
}

export function agentModelSupportsThinking(option: AgentThinkingOptionLike | null | undefined) {
  return Boolean(
    option?.supports_thinking &&
      option.reasoning_efforts?.some((effort) => isAgentReasoningEffort(effort)),
  );
}

export function agentThinkingEffortOptions(
  option: AgentThinkingOptionLike | null | undefined,
): AgentReasoningEffort[] {
  if (!agentModelSupportsThinking(option)) {
    return [];
  }
  return (option?.reasoning_efforts ?? []).filter(isAgentReasoningEffort);
}

export function isAgentReasoningEffort(
  effort: string,
): effort is AgentReasoningEffort {
  return validEfforts.includes(effort as AgentReasoningEffort);
}
