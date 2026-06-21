import type {
  MediaNode,
  ReferencePackItem,
  ReferencePackPreview,
} from "./api";
import { materialKindLabel } from "./sourceMaterial.js";

export function orderedReferencePackItems(items: ReferencePackItem[]) {
  return [...items].sort((a, b) => a.position - b.position);
}

export function memberNodesForPack(
  items: ReferencePackItem[],
  nodes: MediaNode[],
) {
  const nodesById = new Map(nodes.map((node) => [node.id, node]));
  return orderedReferencePackItems(items)
    .map((item) => nodesById.get(item.member_node_id))
    .filter((node): node is MediaNode => Boolean(node));
}

export function candidateReferencePackMembers(
  pack: Pick<MediaNode, "id">,
  nodes: MediaNode[],
  items: ReferencePackItem[],
) {
  const memberIds = new Set(items.map((item) => item.member_node_id));
  return nodes.filter(
    (node) =>
      node.id !== pack.id &&
      node.node_type !== "reference_pack" &&
      !memberIds.has(node.id),
  );
}

export function isReferencePackMemberDependency(
  packNodeId: string,
  toNodeId: string,
  items: ReferencePackItem[],
) {
  return items.some(
    (item) =>
      item.pack_node_id === packNodeId && item.member_node_id === toNodeId,
  );
}

export function memberIdsAfterToggle(
  items: ReferencePackItem[],
  memberNodeId: string,
  checked: boolean,
) {
  const orderedIds = orderedReferencePackItems(items).map(
    (item) => item.member_node_id,
  );
  if (checked) {
    return orderedIds.includes(memberNodeId)
      ? orderedIds
      : [...orderedIds, memberNodeId];
  }
  return orderedIds.filter((id) => id !== memberNodeId);
}

export function referencePackSummaryText(preview?: ReferencePackPreview) {
  if (!preview || preview.member_count === 0) {
    return "等待成员";
  }
  const unit = preview.member_count === 1 ? "member" : "members";
  const memberText = preview.members
    .map(
      (member) =>
        `${materialKindLabel({
          asset_id: member.asset_id ?? null,
          node_type: member.node_type,
          operation_type: member.operation_type ?? "",
          status: member.status,
        })} ${member.title || "未命名"}`,
    )
    .join(", ");
  return memberText
    ? `${preview.member_count} ${unit} · ${memberText}`
    : `${preview.member_count} ${unit}`;
}
