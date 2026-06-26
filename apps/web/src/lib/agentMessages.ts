export interface SequencedAgentMessage {
  id: string;
  seq: number;
  created_at?: string;
  message_type?: string;
  raw_message?: {
    parent_tool_call_id?: unknown;
    tool_call_id?: unknown;
  };
}

export function mergeAgentMessages<T extends SequencedAgentMessage>(
  current: T[],
  incoming: T[],
) {
  const byID = new Map<string, T>();
  for (const message of current) {
    byID.set(message.id, message);
  }
  for (const message of incoming) {
    byID.set(message.id, message);
  }
  return orderNestedAgentMessages(Array.from(byID.values()).sort(compareAgentMessages));
}

export function visibleAgentMessages<T extends SequencedAgentMessage>(
  messages: T[],
) {
  return messages.filter((message) => message.message_type !== "tool_result");
}

function compareAgentMessages<T extends SequencedAgentMessage>(a: T, b: T) {
  const aCreatedAt = Date.parse(a.created_at ?? "");
  const bCreatedAt = Date.parse(b.created_at ?? "");
  if (!Number.isNaN(aCreatedAt) && !Number.isNaN(bCreatedAt)) {
    if (aCreatedAt !== bCreatedAt) {
      return aCreatedAt - bCreatedAt;
    }
  }
  if (a.seq !== b.seq) {
    return a.seq - b.seq;
  }
  return a.id.localeCompare(b.id);
}

function orderNestedAgentMessages<T extends SequencedAgentMessage>(messages: T[]) {
  const childrenByParent = new Map<string, T[]>();
  const childIDs = new Set<string>();
  for (const message of messages) {
    const parentToolCallID = stringValue(message.raw_message?.parent_tool_call_id);
    if (!parentToolCallID) {
      continue;
    }
    childIDs.add(message.id);
    const children = childrenByParent.get(parentToolCallID) ?? [];
    children.push(message);
    childrenByParent.set(parentToolCallID, children);
  }
  if (childIDs.size === 0) {
    return messages;
  }

  const ordered: T[] = [];
  const emittedChildren = new Set<string>();
  for (const message of messages) {
    if (childIDs.has(message.id)) {
      continue;
    }
    ordered.push(message);
    const toolCallID = stringValue(message.raw_message?.tool_call_id);
    if (!toolCallID) {
      continue;
    }
    for (const child of childrenByParent.get(toolCallID) ?? []) {
      if (emittedChildren.has(child.id)) {
        continue;
      }
      ordered.push(child);
      emittedChildren.add(child.id);
    }
  }
  for (const message of messages) {
    if (childIDs.has(message.id) && !emittedChildren.has(message.id)) {
      ordered.push(message);
    }
  }
  return ordered;
}

function stringValue(value: unknown) {
  return typeof value === "string" && value.trim() ? value.trim() : "";
}
