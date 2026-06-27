import type { AgentMessage, AgentObservedThread } from "../../lib/agentApi";
import type { AgentStreamState } from "../../lib/agentStreaming";
import {
  formatAgentMessageTime,
  messageDisplayClass,
  visibleAgentMessages,
} from "../../lib/agentMessages";
import { AgentMessageRenderer } from "./AgentMessageRenderer";

export function AgentThreadDrawer({
  thread,
  messages,
  streams = [],
  isLoading,
  onClose,
}: {
  thread?: AgentObservedThread;
  messages: AgentMessage[];
  streams?: AgentStreamState[];
  isLoading: boolean;
  onClose: () => void;
}) {
  if (!thread) {
    return null;
  }
  const visibleMessages = visibleAgentMessages(messages);
  return (
    <aside
      className="agent-thread-drawer"
      aria-label={`${thread.display_name} 只读线程`}
    >
      <header className="agent-thread-drawer-header">
        <div>
          <span className="agent-thread-drawer-kicker">只读 Agent 线程</span>
          <h3>{thread.display_name}</h3>
          <small>{thread.id}</small>
        </div>
        <button aria-label="关闭子 Agent 线程" onClick={onClose} type="button">
          ×
        </button>
      </header>
      <div className="agent-thread-drawer-body">
        {isLoading ? (
          <p className="agent-empty-text">正在加载子 Agent 对话</p>
        ) : visibleMessages.length === 0 ? (
          <p className="agent-empty-text">这个 Agent 还没有消息。</p>
        ) : (
          visibleMessages.map((message) => (
            <article
              className={`agent-thread-message agent-message-${messageDisplayClass(message)}`}
              key={message.id}
            >
              <AgentMessageRenderer message={message} />
              {message.created_at ? (
                <time dateTime={message.created_at}>
                  {formatAgentMessageTime(message.created_at)}
                </time>
              ) : null}
            </article>
          ))
        )}
        {streams.map((stream) => (
          <article
            className="agent-thread-message agent-message-assistant agent-message-streaming"
            key={stream.task_id}
          >
            <AgentMessageRenderer message={streamToThreadMessage(stream)} />
          </article>
        ))}
      </div>
    </aside>
  );
}

function streamToThreadMessage(
  stream: AgentStreamState,
): Pick<AgentMessage, "id" | "message_type" | "content"> {
  return {
    id: `thread-stream-${stream.task_id}`,
    message_type: "text",
    content: {
      schema: "clipanvil.agent.message.v1",
      blocks: [...stream.blocks]
        .sort((left, right) => left.sequence - right.sequence)
        .map((block) =>
          block.type === "thinking"
            ? {
                id: block.id,
                type: "thinking",
                text: block.text,
                status: "streaming",
                default_collapsed: false,
              }
            : {
                id: block.id,
                type: "markdown",
                text: block.text,
              },
        ),
    },
  };
}
