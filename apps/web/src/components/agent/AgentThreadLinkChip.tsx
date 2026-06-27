import type { AgentObservedThread } from "../../lib/agentApi";

export function AgentThreadLinkChip({
  thread,
  onSelect,
}: {
  thread: Pick<AgentObservedThread, "id" | "display_name"> & {
    latest_task?: { status?: string };
  };
  onSelect: (threadId: string) => void;
}) {
  const status = thread.latest_task?.status ?? "idle";
  return (
    <button
      className={`agent-thread-link-chip agent-thread-link-chip-${status}`}
      onClick={() => onSelect(thread.id)}
      type="button"
    >
      <span aria-hidden="true" className="agent-thread-status-dot" />
      <span>{thread.display_name}</span>
    </button>
  );
}
