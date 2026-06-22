export interface AgentModelRefLike {
  provider_id: string;
  model_id: string;
  reasoning_effort?: string;
}

export interface AgentModelOptionLike extends AgentModelRefLike {
  display_name?: string;
}

export function agentModelSelectionValue(model: AgentModelRefLike | undefined) {
  if (!model) {
    return "";
  }
  return `${model.provider_id}:${model.model_id}`;
}

export function agentModelSelectionPayload(value: string, reasoningEffort?: string) {
  const separator = value.indexOf(":");
  if (separator < 1 || separator === value.length - 1) {
    return null;
  }
  const producer: AgentModelRefLike = {
    provider_id: value.slice(0, separator),
    model_id: value.slice(separator + 1),
  };
  if (reasoningEffort) {
    producer.reasoning_effort = reasoningEffort;
  }
  return {
    producer: {
      provider_id: producer.provider_id,
      model_id: producer.model_id,
      ...(producer.reasoning_effort
        ? { reasoning_effort: producer.reasoning_effort }
        : {}),
    },
  };
}

export function formatAgentModelOption(option: AgentModelOptionLike) {
  return option.display_name?.trim() || option.model_id;
}
