import { formatAgentAttachmentLabel } from "../../lib/agentAttachments";
import type { AgentAttachment } from "../../lib/agentApi";
import type { AgentAttachmentBlock as AgentAttachmentBlockData } from "../../lib/agentMessageBlocks";

export function AgentAttachmentBlock({
  block,
}: {
  block: AgentAttachmentBlockData;
}) {
  if (block.attachments.length === 0) {
    return null;
  }
  return (
    <div className="agent-attachment-row">
      {block.attachments.map((attachment) => (
        <AgentAttachmentItem
          attachment={attachment}
          key={`${block.id}-${attachment.node_id}`}
        />
      ))}
    </div>
  );
}

function AgentAttachmentItem({
  attachment,
}: {
  attachment: AgentAttachment;
}) {
  const previewUrl = attachment.thumbnail_url ?? attachment.url;
  if (attachment.kind === "image" && previewUrl) {
    return (
      <figure className="agent-attachment-preview-card">
        <img
          alt={attachment.name}
          className="agent-attachment-thumbnail"
          loading="lazy"
          src={previewUrl}
        />
        <figcaption className="agent-attachment-preview-meta">
          <span>{attachment.name}</span>
          <small>{attachment.mime}</small>
        </figcaption>
      </figure>
    );
  }

  return (
    <span className="agent-attachment-chip">
      {formatAgentAttachmentLabel(attachment)}
    </span>
  );
}
