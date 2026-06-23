import { useState } from "react";
import type { AgentThinkingBlock as AgentThinkingBlockData } from "../../lib/agentMessageBlocks";

export function AgentThinkingBlock({
  block,
}: {
  block: AgentThinkingBlockData;
}) {
  const [expanded, setExpanded] = useState(!block.default_collapsed);
  const streaming = block.status === "streaming";
  const label = streaming ? "ClipAnvil 正在思考" : "ClipAnvil 的思考";

  if (!block.text.trim() && !streaming) {
    return null;
  }

  return (
    <section
      className={`agent-thinking-block${streaming ? " agent-thinking-block-streaming" : ""}`}
    >
      <button
        className="agent-thinking-toggle"
        onClick={() => setExpanded((current) => !current)}
        type="button"
      >
        <span className="agent-thinking-shimmer">{label}</span>
        <span aria-hidden="true">{expanded ? "⌃" : "⌄"}</span>
      </button>
      {expanded && block.text.trim() ? (
        <p className="agent-thinking-content">{block.text}</p>
      ) : null}
    </section>
  );
}
