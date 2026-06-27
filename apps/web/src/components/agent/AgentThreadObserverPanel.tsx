import type { AgentObservedThread } from "../../lib/agentApi";
import { formatAgentMessageTime } from "../../lib/agentMessages";

export function AgentThreadObserverPanel({
  threads,
  selectedThreadId,
  onSelectThread,
}: {
  threads: AgentObservedThread[];
  selectedThreadId?: string;
  onSelectThread: (threadId: string) => void;
}) {
  if (threads.length === 0) {
    return null;
  }
  return (
    <label className="agent-thread-selector">
      <span>Agents</span>
      <select
        aria-label="选择子 Agent"
        onChange={(event) => {
          if (event.target.value) {
            onSelectThread(event.target.value);
          }
        }}
        value={selectedThreadId ?? ""}
      >
        <option value="">{threads.length} 个子 Agent</option>
        {threads.map((thread) => (
          <option key={thread.id} title={threadTitle(thread)} value={thread.id}>
            {threadOptionLabel(thread)}
          </option>
        ))}
      </select>
    </label>
  );
}

function threadTitle(thread: AgentObservedThread) {
  return [
    `${thread.role} · ${thread.scope_label || thread.display_name}`,
    threadStatusLabel(thread),
    thread.latest_message_preview,
    thread.latest_message_at ? formatAgentMessageTime(thread.latest_message_at) : "",
  ]
    .filter(Boolean)
    .join("\n");
}

function threadOptionLabel(thread: AgentObservedThread) {
  return `${roleLabel(thread.role)} · ${
    thread.scope_label || thread.display_name
  } · ${threadStatusLabel(thread)}`;
}

function roleLabel(role: AgentObservedThread["role"]) {
  switch (role) {
    case "craftsman":
      return "Craftsman";
    case "reviewer":
      return "Reviewer";
    case "composer":
      return "Composer";
    case "producer":
      return "Producer";
    default:
      return role;
  }
}

function threadStatusLabel(thread: AgentObservedThread) {
  switch (thread.latest_task?.status) {
    case "running":
      return "执行中";
    case "queued":
      return "排队中";
    case "failed":
      return "失败";
    case "succeeded":
      return "完成";
    case "cancelled":
      return "已取消";
    case "waiting_for_user":
      return "等待用户";
    default:
      return thread.status;
  }
}
