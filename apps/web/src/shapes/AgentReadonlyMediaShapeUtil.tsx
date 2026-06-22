import {
  HTMLContainer,
  Rectangle2d,
  ShapeUtil,
  T,
  type Geometry2d,
  type RecordProps,
} from "tldraw";
import type { DragEvent, KeyboardEvent, SyntheticEvent } from "react";
import {
  MEDIA_SHAPE_TYPE,
  type MediaShape,
} from "@clip-anvil/canvas-schema";

const statusText: Record<MediaShape["props"]["status"], string> = {
  draft: "草稿",
  ready: "就绪",
  queued: "排队",
  running: "运行中",
  succeeded: "完成",
  failed: "失败",
  stale: "需更新",
  user_editing: "编辑中",
};

const emptyTitleByType: Record<MediaShape["props"]["nodeType"], string> = {
  text: "未命名文本",
  image: "未命名图片",
  video: "未命名视频",
  audio: "未命名音频",
  reference_pack: "未命名参考包",
};

function preventNativeMediaDrag(event: DragEvent<HTMLImageElement>) {
  event.preventDefault();
}

function dispatchReadonlyNodeSelect(nodeId: string) {
  window.dispatchEvent(
    new CustomEvent("clip-anvil:select-node", {
      detail: {
        nodeId,
      },
    }),
  );
}

function stopReadonlyNodeGesture(event: SyntheticEvent<HTMLElement>) {
  event.stopPropagation();
  event.nativeEvent.stopImmediatePropagation();
}

function selectReadonlyNode(
  event: SyntheticEvent<HTMLElement>,
  nodeId: string,
) {
  stopReadonlyNodeGesture(event);
  dispatchReadonlyNodeSelect(nodeId);
}

export class AgentReadonlyMediaShapeUtil extends ShapeUtil<MediaShape> {
  static override type = MEDIA_SHAPE_TYPE;

  static override props: RecordProps<MediaShape> = {
    nodeId: T.string,
    nodeType: T.literalEnum("text", "image", "video", "audio", "reference_pack"),
    operationType: T.optional(T.string),
    assetId: T.optional(T.string),
    nodeTypeLabel: T.optional(T.string),
    sourceMaterialStatusLabel: T.optional(T.string),
    title: T.string,
    prompt: T.string,
    status: T.literalEnum(
      "draft",
      "ready",
      "queued",
      "running",
      "succeeded",
      "failed",
      "stale",
      "user_editing",
    ),
    thumbnailUrl: T.optional(T.string),
    previewText: T.optional(T.string),
    previewAssetType: T.optional(T.string),
    previewAssetUrl: T.optional(T.string),
    previewThumbnailUrl: T.optional(T.string),
    previewVersionNo: T.optional(T.number),
    previewWidth: T.optional(T.number),
    previewHeight: T.optional(T.number),
    previewDurationMs: T.optional(T.number),
    activeStaleReasonCount: T.optional(T.number),
    w: T.number,
    h: T.number,
  };

  override getDefaultProps(): MediaShape["props"] {
    return {
      nodeId: "",
      nodeType: "text",
      title: "未命名文本",
      prompt: "",
      status: "draft",
      w: 200,
      h: 120,
    };
  }

  override canResize() {
    return false;
  }

  override canEdit() {
    return false;
  }

  override getGeometry(shape: MediaShape): Geometry2d {
    return new Rectangle2d({
      width: shape.props.w,
      height: shape.props.h,
      isFilled: true,
    });
  }

  override component(shape: MediaShape) {
    return <AgentReadonlyMediaNodeShape shape={shape} />;
  }

  override getIndicatorPath(shape: MediaShape) {
    const path = new Path2D();
    path.rect(0, 0, shape.props.w, shape.props.h);
    return path;
  }

  override onClick(shape: MediaShape) {
    dispatchReadonlyNodeSelect(shape.props.nodeId);
  }
}

function AgentReadonlyMediaNodeShape({ shape }: { shape: MediaShape }) {
  const {
    nodeType,
    nodeTypeLabel,
    sourceMaterialStatusLabel,
    title,
    prompt,
    status,
    thumbnailUrl,
    previewText,
    previewAssetUrl,
    previewThumbnailUrl,
    activeStaleReasonCount,
    w,
    h,
  } = shape.props;
  const displayTitle = title || emptyTitleByType[nodeType];
  const previewUrl = previewThumbnailUrl || thumbnailUrl || previewAssetUrl;
  const statusLabel = sourceMaterialStatusLabel || statusText[status];
  const selectNodeFromKeyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "Enter" && event.key !== " ") {
      return;
    }
    event.preventDefault();
    selectReadonlyNode(event, shape.props.nodeId);
  };

  return (
    <HTMLContainer>
      <div
        aria-label={`查看 ${displayTitle}`}
        className="agent-readonly-media-node-shell"
        data-node-id={shape.props.nodeId}
        onClick={(event) => selectReadonlyNode(event, shape.props.nodeId)}
        onKeyDown={selectNodeFromKeyboard}
        onMouseDownCapture={stopReadonlyNodeGesture}
        onPointerDownCapture={stopReadonlyNodeGesture}
        role="button"
        style={{ width: w }}
        tabIndex={0}
      >
        <div
          className="agent-readonly-media-node"
          data-status={status}
          data-type={nodeType}
          style={{ width: w, height: h }}
        >
          <div className="agent-readonly-media-node-header">
            <span className="agent-readonly-media-node-icon">
              {nodeTypeLabel || nodeType}
            </span>
            <p className="agent-readonly-media-node-title">{displayTitle}</p>
            <span className="agent-readonly-media-node-status">
              {statusLabel}
            </span>
            {status === "stale" || Number(activeStaleReasonCount) > 0 ? (
              <span className="agent-readonly-media-node-stale">
                {activeStaleReasonCount
                  ? `stale · ${activeStaleReasonCount}`
                  : "stale"}
              </span>
            ) : null}
          </div>
          <div className="agent-readonly-media-node-content" data-type={nodeType}>
            {nodeType === "image" && previewUrl ? (
              <img
                alt={displayTitle}
                decoding="async"
                draggable={false}
                loading="lazy"
                onDragStart={preventNativeMediaDrag}
                src={previewUrl}
              />
            ) : nodeType === "text" ? (
              <p>{previewText || prompt || "等待输入 prompt"}</p>
            ) : (
              <p>{previewText || displayTitle}</p>
            )}
          </div>
        </div>
      </div>
    </HTMLContainer>
  );
}
