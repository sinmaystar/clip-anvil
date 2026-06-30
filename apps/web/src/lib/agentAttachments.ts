export const attachmentAccept = "image/*,video/*,.txt,text/plain";

export type AgentAttachmentKind = "image" | "video" | "text";

export interface AgentAttachmentLike {
  kind: AgentAttachmentKind;
  name: string;
}

export function agentAttachmentKindForFile(
  file: Pick<File, "type" | "name">,
): AgentAttachmentKind | null {
  const type = file.type.toLowerCase();
  const name = file.name.toLowerCase();
  if (type.startsWith("image/")) {
    return "image";
  }
  if (type.startsWith("video/")) {
    return "video";
  }
  if (type.startsWith("text/plain") || name.endsWith(".txt")) {
    return "text";
  }
  return null;
}

export function validAgentAttachmentFiles<T extends Pick<File, "type" | "name">>(
  files: Iterable<T> | ArrayLike<T>,
) {
  return Array.from(files).filter((file) => agentAttachmentKindForFile(file));
}

export function formatAgentAttachmentLabel(attachment: AgentAttachmentLike) {
  const prefix =
    attachment.kind === "image"
      ? "IMG"
      : attachment.kind === "video"
        ? "VID"
        : "TXT";
  return `${prefix} ${attachment.name}`;
}
