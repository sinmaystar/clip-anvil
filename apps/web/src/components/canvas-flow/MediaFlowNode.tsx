import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
} from "react";
import {
  Handle,
  Position,
  useUpdateNodeInternals,
  type NodeProps,
  type Node,
} from "@xyflow/react";
import { MarkdownPreview } from "../MarkdownPreview";
import { mediaNodeDisplaySize } from "../../lib/canvas";
import { winnerPreviewText } from "../../lib/productionPreview";
import { materialKindLabel } from "../../lib/sourceMaterial";
import type { MediaDimensions } from "../../lib/nodePreviewLayout";
import type { CanvasFlowNodeData } from "./flowTypes";
import {
  useCanvasFlowPolicy,
  type CanvasFlowPolicy,
} from "./flowModePolicy";

type MediaFlowNodeModel = Node<CanvasFlowNodeData, "media">;

const nodeTypeLabel: Record<CanvasFlowNodeData["node"]["node_type"], string> = {
  text: "文本",
  image: "图片",
  video: "视频",
  audio: "音频",
  reference_pack: "参考素材",
};

const nodeTypeIcon: Record<CanvasFlowNodeData["node"]["node_type"], string> = {
  text: "T",
  image: "▧",
  video: "▶",
  audio: "≋",
  reference_pack: "⌘",
};

export function MediaFlowNode({
  data,
  selected,
}: NodeProps<MediaFlowNodeModel>) {
  const policy: CanvasFlowPolicy = useCanvasFlowPolicy();
  const updateNodeInternals = useUpdateNodeInternals();
  const node = data.node;
  const previewText = winnerPreviewText(node);
  const previewAssetUrl = node.production_preview?.access_url ?? node.asset_url;
  const previewThumbnailUrl =
    node.production_preview?.thumbnail_url ?? node.thumbnail_url;
  const imagePreviewUrl = previewAssetUrl || previewThumbnailUrl;
  const [imageNaturalSize, setImageNaturalSize] =
    useState<MediaDimensions | null>(null);
  const [videoNaturalSize, setVideoNaturalSize] =
    useState<MediaDimensions | null>(null);
  const size = mediaNodeDisplaySize(
    node,
    node.node_type === "image"
      ? imageNaturalSize
      : node.node_type === "video"
        ? videoNaturalSize
        : null,
  );
  const hasPreviewContent =
    Boolean(previewText) || Boolean(previewAssetUrl) || Boolean(previewThumbnailUrl);
  const title = node.title || `未命名${materialKindLabel(node)}`;
  const [isEditingTitle, setIsEditingTitle] = useState(false);
  const [titleDraft, setTitleDraft] = useState(title);
  const titleInputRef = useRef<HTMLInputElement | null>(null);
  const canRenameTitle = policy.canEditNodeContent && Boolean(data.onRenameNode);

  useEffect(() => {
    setTitleDraft(title);
  }, [node.id, title]);

  useEffect(() => {
    setImageNaturalSize(null);
  }, [node.id, imagePreviewUrl]);

  useEffect(() => {
    setVideoNaturalSize(null);
  }, [node.id, previewAssetUrl]);

  useEffect(() => {
    updateNodeInternals(node.id);
  }, [node.id, size.h, size.w, updateNodeInternals]);

  useEffect(() => {
    if (isEditingTitle) {
      titleInputRef.current?.focus();
      titleInputRef.current?.select();
    }
  }, [isEditingTitle]);

  const stopTitlePointer = (event: PointerEvent<HTMLElement>) => {
    event.stopPropagation();
  };

  const commitTitleEdit = () => {
    const nextTitle = titleDraft.trim();
    setIsEditingTitle(false);
    if (!nextTitle) {
      setTitleDraft(title);
      return;
    }
    if (nextTitle !== node.title) {
      data.onRenameNode?.(node.id, nextTitle);
    }
  };

  const cancelTitleEdit = () => {
    setTitleDraft(title);
    setIsEditingTitle(false);
  };

  const handleTitleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Enter") {
      event.preventDefault();
      commitTitleEdit();
    }
    if (event.key === "Escape") {
      event.preventDefault();
      cancelTitleEdit();
    }
  };

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
          aria-label={`查看 ${title}`}
          className="media-node-expand-button nodrag"
          disabled={!policy.canSelect}
          type="button"
        >
          ↗
        </button>
      ) : null}
      <div className="media-node-floating-title nodrag nopan">
        <span
          aria-label={nodeTypeLabel[node.node_type]}
          className="media-node-kind-icon"
        >
          {nodeTypeIcon[node.node_type]}
        </span>
        {isEditingTitle ? (
          <input
            aria-label="编辑节点名称"
            className="media-node-title media-node-title-input nodrag nopan"
            onBlur={commitTitleEdit}
            onChange={(event) => setTitleDraft(event.currentTarget.value)}
            onKeyDown={handleTitleKeyDown}
            onPointerDown={stopTitlePointer}
            ref={titleInputRef}
            value={titleDraft}
          />
        ) : (
          <button
            aria-label={`重命名 ${title}`}
            className="media-node-title media-node-title-button nodrag nopan"
            disabled={!canRenameTitle}
            onDoubleClick={(event) => {
              event.stopPropagation();
              if (canRenameTitle) {
                setIsEditingTitle(true);
              }
            }}
            onPointerDown={stopTitlePointer}
            title={title}
            type="button"
          >
            {title}
          </button>
        )}
        {node.status === "stale" || Number(node.active_stale_reason_count) > 0 ? (
          <span className="media-node-stale-badge">
            stale
            {node.active_stale_reason_count
              ? ` · ${node.active_stale_reason_count}`
              : ""}
          </span>
        ) : null}
      </div>
      <article
        className="media-node"
        data-status={node.status}
        data-type={node.node_type}
        style={{ width: size.w, height: size.h }}
      >
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
                  onLoad={(event) => {
                    const { naturalHeight, naturalWidth } = event.currentTarget;
                    if (naturalWidth <= 0 || naturalHeight <= 0) {
                      return;
                    }
                    const nextSize = {
                      width: naturalWidth,
                      height: naturalHeight,
                    };
                    setImageNaturalSize((current) =>
                      current?.width === naturalWidth &&
                      current?.height === naturalHeight
                        ? current
                        : nextSize,
                    );
                    data.onMediaDimensionsChange?.(node.id, nextSize);
                  }}
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
                  onLoadedMetadata={(event) => {
                    const { videoHeight, videoWidth } = event.currentTarget;
                    if (videoWidth <= 0 || videoHeight <= 0) {
                      return;
                    }
                    const nextSize = {
                      width: videoWidth,
                      height: videoHeight,
                    };
                    setVideoNaturalSize((current) =>
                      current?.width === videoWidth &&
                      current?.height === videoHeight
                        ? current
                        : nextSize,
                    );
                    data.onMediaDimensionsChange?.(node.id, nextSize);
                  }}
                  poster={previewThumbnailUrl}
                  preload="metadata"
                  src={previewAssetUrl}
                />
              ) : (
                <div className="media-node-placeholder">播放预览</div>
              )}
            </div>
          ) : node.node_type === "audio" ? (
            <div className="media-node-audio-preview">
              <div className="media-node-audio-summary">
                <span className="media-node-waveform" />
                <span>{previewAssetUrl ? previewText : "音频预览"}</span>
              </div>
              {previewAssetUrl ? (
                <audio
                  aria-label={`播放 ${title}`}
                  className="media-node-audio-player nodrag nopan"
                  controls
                  preload="metadata"
                  src={previewAssetUrl}
                />
              ) : (
                <div className="media-node-placeholder">
                  <span>音频预览</span>
                </div>
              )}
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
