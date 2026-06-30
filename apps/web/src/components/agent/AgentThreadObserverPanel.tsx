import { useEffect, useRef, useState } from "react";

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
  const selectorRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const selectedThread = threads.find((thread) => thread.id === selectedThreadId);
  const triggerTitle = selectedThread
    ? threadTitle(selectedThread)
    : `${threads.length} 个子 Agent`;

  useEffect(() => {
    if (!open) {
      return;
    }
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (
        target instanceof Node &&
        selectorRef.current &&
        !selectorRef.current.contains(target)
      ) {
        setOpen(false);
      }
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", handlePointerDown, true);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown, true);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [open]);

  if (threads.length === 0) {
    return null;
  }

  return (
    <div className="agent-thread-selector" ref={selectorRef}>
      <span className="agent-thread-selector-label">Agents</span>
      <button
        aria-label="选择子 Agent"
        aria-expanded={open}
        aria-haspopup="listbox"
        className="agent-thread-selector-trigger"
        onClick={() => setOpen((current) => !current)}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown") {
            event.preventDefault();
            setOpen(true);
          }
        }}
        title={triggerTitle}
        type="button"
      >
        <span className="agent-thread-selector-trigger-main">
          {selectedThread ? roleLabel(selectedThread.role) : `${threads.length} 个`}
        </span>
        <span className="agent-thread-selector-trigger-sub">
          {selectedThread
            ? selectedThread.scope_label || selectedThread.display_name
            : "子 Agent"}
        </span>
        <span aria-hidden="true" className="agent-thread-selector-chevron">
          ⌄
        </span>
      </button>
      {open ? (
        <div
          aria-label="选择子 Agent"
          className="agent-thread-selector-menu"
          role="listbox"
        >
          {threads.map((thread) => (
            <button
              aria-selected={thread.id === selectedThreadId}
              className="agent-thread-selector-option"
              key={thread.id}
              onClick={() => {
                onSelectThread(thread.id);
                setOpen(false);
              }}
              role="option"
              title={threadTitle(thread)}
              type="button"
            >
              <span className="agent-thread-selector-option-main">
                <span className="agent-thread-selector-role">
                  {roleLabel(thread.role)}
                </span>
                <span className="agent-thread-selector-scope">
                  {thread.scope_label || thread.display_name}
                </span>
                <span
                  className={`agent-thread-selector-status agent-thread-selector-status-${threadStatusClass(
                    thread,
                  )}`}
                >
                  {threadStatusLabel(thread)}
                </span>
              </span>
              <span className="agent-thread-selector-option-meta">
                {thread.latest_message_preview ||
                  (thread.latest_message_at
                    ? formatAgentMessageTime(thread.latest_message_at)
                    : thread.id)}
              </span>
            </button>
          ))}
        </div>
      ) : null}
    </div>
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

function threadStatusClass(thread: AgentObservedThread) {
  switch (thread.latest_task?.status) {
    case "running":
    case "queued":
    case "failed":
    case "succeeded":
    case "cancelled":
    case "waiting_for_user":
      return thread.latest_task.status;
    default:
      return thread.status.replace(/[^a-z0-9_-]/gi, "-").toLowerCase();
  }
}
