import type { AgentMediaBlock as AgentMediaBlockData } from "../../lib/agentMessageBlocks";

export function AgentMediaBlock({ block }: { block: AgentMediaBlockData }) {
  const title = block.title || block.kind;
  if (block.kind === "image" && block.url) {
    return (
      <figure className="agent-media-block">
        <img alt={title} src={block.url} />
        <figcaption>{title}</figcaption>
      </figure>
    );
  }
  if ((block.kind === "video" || block.kind === "final_video") && block.url) {
    return (
      <figure className="agent-media-block">
        <video controls poster={block.thumbnail_url} src={block.url} />
        <figcaption>{title}</figcaption>
      </figure>
    );
  }
  return (
    <div className="agent-media-placeholder">
      <strong>{title}</strong>
      <span>{block.asset_id}</span>
    </div>
  );
}
