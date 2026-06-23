import { Handle, Position, type NodeProps, type Node } from "@xyflow/react";
import { MarkdownPreview } from "../MarkdownPreview";
import { mediaNodeDisplaySize } from "../../lib/canvas";
import { winnerPreviewText } from "../../lib/productionPreview";
import { materialKindLabel, materialStatusLabel } from "../../lib/sourceMaterial";
import type { CanvasFlowNodeData } from "./flowTypes";
import {
  useCanvasFlowPolicy,
  type CanvasFlowPolicy,
} from "./flowModePolicy";

type MediaFlowNodeModel = Node<CanvasFlowNodeData, "media">;

const statusText: Record<CanvasFlowNodeData["node"]["status"], string> = {
  draft: "草稿",
  ready: "就绪",
  queued: "排队",
  running: "运行中",
  succeeded: "完成",
  failed: "失败",
  stale: "需更新",
  user_editing: "编辑中",
};

export function MediaFlowNode({
  data,
  selected,
}: NodeProps<MediaFlowNodeModel>) {
  const policy: CanvasFlowPolicy = useCanvasFlowPolicy();
  const node = data.node;
  const size = mediaNodeDisplaySize(node);
  const previewText = winnerPreviewText(node);
  const previewAssetUrl = node.production_preview?.access_url ?? node.asset_url;
  const previewThumbnailUrl =
    node.production_preview?.thumbnail_url ?? node.thumbnail_url;
  const statusLabel = materialStatusLabel(node) || statusText[node.status];
  const hasPreviewContent =
    Boolean(previewText) || Boolean(previewAssetUrl) || Boolean(previewThumbnailUrl);

  return (
    <div
      className="media-node-shell"
      data-active={selected}
      data-node-id={node.id}
      style={{ width: size.w }}
    >
      {policy.canCreateEdges ? (
        <>
          <Handle
            className="media-node-target-handle"
            isConnectableStart={false}
            position={Position.Left}
            type="target"
          />
          <Handle
            aria-label={`从 ${node.title || materialKindLabel(node)} 创建依赖连线`}
            className="media-node-connect-handle"
            isConnectableEnd={false}
            position={Position.Right}
            type="source"
          />
        </>
      ) : null}
      {hasPreviewContent ? (
        <button
          aria-label={`查看 ${node.title || materialKindLabel(node)}`}
          className="media-node-expand-button nodrag"
          disabled={!policy.canSelect}
          type="button"
        >
          ↗
        </button>
      ) : null}
      <article
        className="media-node"
        data-status={node.status}
        data-type={node.node_type}
        style={{ width: size.w, height: size.h }}
      >
        <div className="media-node-header">
          <span className="media-node-icon">{materialKindLabel(node)}</span>
          <p className="media-node-title">
            {node.title || `未命名${materialKindLabel(node)}`}
          </p>
          <span className="media-node-status">{statusLabel}</span>
          {node.status === "stale" || Number(node.active_stale_reason_count) > 0 ? (
            <span className="media-node-stale-badge">
              stale
              {node.active_stale_reason_count
                ? ` · ${node.active_stale_reason_count}`
                : ""}
            </span>
          ) : null}
        </div>
        <div className="media-node-content" data-type={node.node_type}>
          {node.node_type === "text" ? (
            <MarkdownPreview
              value={previewText || node.prompt || "等待输入 prompt"}
              variant="canvas"
            />
          ) : node.node_type === "image" ? (
            <div className="media-node-media-frame" data-kind="image">
              {previewAssetUrl || previewThumbnailUrl ? (
                <img
                  alt={node.title || materialKindLabel(node)}
                  draggable={false}
                  src={previewAssetUrl || previewThumbnailUrl}
                />
              ) : (
                <div className="media-node-placeholder">图片占位</div>
              )}
            </div>
          ) : node.node_type === "video" ? (
            <div className="media-node-media-frame" data-kind="video">
              {previewAssetUrl ? (
                <video
                  controls
                  draggable={false}
                  poster={previewThumbnailUrl}
                  preload="metadata"
                  src={previewAssetUrl}
                />
              ) : (
                <div className="media-node-placeholder">播放预览</div>
              )}
            </div>
          ) : node.node_type === "audio" ? (
            <div className="media-node-placeholder">
              <span className="media-node-waveform" />
              <span>音频预览</span>
            </div>
          ) : (
            <div className="media-node-placeholder">
              <span>Reference Pack</span>
              <span>{node.reference_pack_preview?.member_count ?? 0} 个成员</span>
            </div>
          )}
        </div>
      </article>
    </div>
  );
}
