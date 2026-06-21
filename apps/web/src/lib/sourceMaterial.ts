import type { MediaNode } from "./api";

type SourceMaterialNode = Pick<
  MediaNode,
  "asset_id" | "node_type" | "operation_type" | "status"
>;

const generatedLabels: Record<MediaNode["node_type"], string> = {
  text: "文本",
  image: "图片",
  video: "视频",
  audio: "音频",
  reference_pack: "参考包",
};

const sourceLabels: Record<MediaNode["node_type"], string> = {
  text: "文本素材",
  image: "图片素材",
  video: "视频素材",
  audio: "音频素材",
  reference_pack: "参考包",
};

export function isUploadMaterialNode(node: SourceMaterialNode) {
  return node.operation_type === "upload" || Boolean(node.asset_id);
}

export function isManualTextMaterialNode(node: SourceMaterialNode) {
  return node.node_type === "text" && node.operation_type === "manual";
}

export function isSourceMaterialNode(node: SourceMaterialNode) {
  return isUploadMaterialNode(node) || isManualTextMaterialNode(node);
}

export function canRunProductionNode(node: SourceMaterialNode) {
  return !isSourceMaterialNode(node) && node.node_type !== "reference_pack";
}

export function materialKindLabel(
  node: Pick<MediaNode, "node_type"> & Partial<SourceMaterialNode>,
) {
  const normalized: SourceMaterialNode = {
    asset_id: node.asset_id ?? null,
    node_type: node.node_type,
    operation_type: node.operation_type ?? "",
    status: node.status ?? "draft",
  };
  return isSourceMaterialNode(normalized)
    ? sourceLabels[node.node_type]
    : generatedLabels[node.node_type];
}

export function materialStatusLabel(node: SourceMaterialNode) {
  if (!isSourceMaterialNode(node)) {
    return "";
  }
  return node.status === "failed" ? "不可用" : "可用";
}
