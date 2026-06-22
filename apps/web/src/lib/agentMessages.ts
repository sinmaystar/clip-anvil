export interface SequencedAgentMessage {
  id: string;
  seq: number;
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
  return Array.from(byID.values()).sort((a, b) => a.seq - b.seq);
}
