import { MarkdownPreview } from "../MarkdownPreview";
import type { AgentMarkdownBlock as AgentMarkdownBlockData } from "../../lib/agentMessageBlocks";

export function AgentMarkdownBlock({
  block,
}: {
  block: AgentMarkdownBlockData;
}) {
  if (!block.text.trim()) {
    return null;
  }
  return (
    <div className="agent-markdown-block">
      <MarkdownPreview value={block.text} variant="panel" />
    </div>
  );
}
