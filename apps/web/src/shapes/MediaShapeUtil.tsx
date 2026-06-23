import {
  HTMLContainer,
  Rectangle2d,
  ShapeUtil,
  T,
  type Geometry2d,
  type RecordProps,
} from "tldraw";
import {
  useEffect,
  useState,
  type PointerEvent,
  type SyntheticEvent,
} from "react";
import {
  MEDIA_SHAPE_TYPE,
  type MediaShape,
} from "@clip-anvil/canvas-schema";
import { MarkdownPreview } from "../components/MarkdownPreview";
import { materialKindLabel, materialStatusLabel } from "../lib/sourceMaterial";

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

const nodeTypeMeta: Record<
  MediaShape["props"]["nodeType"],
  { icon: string; label: string; emptyTitle: string }
> = {
  text: { icon: "文案", label: "文本", emptyTitle: "未命名文本" },
  image: { icon: "参考", label: "图片", emptyTitle: "未命名图片" },
  video: { icon: "视频", label: "视频", emptyTitle: "未命名视频" },
  audio: { icon: "音频", label: "音频", emptyTitle: "未命名音频" },
  reference_pack: {
    icon: "参考包",
    label: "参考包",
    emptyTitle: "未命名参考包",
  },
};

let activeMediaNodeId: string | null = null;

export class MediaShapeUtil extends ShapeUtil<MediaShape> {
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
    return <MediaNodeShape shape={shape} />;
  }

  override getIndicatorPath(shape: MediaShape) {
    const path = new Path2D();
    path.rect(0, 0, shape.props.w, shape.props.h);
    return path;
  }

  override onClick(shape: MediaShape) {
    window.dispatchEvent(
      new CustomEvent("clip-anvil:select-node", {
        detail: {
          nodeId: shape.props.nodeId,
        },
      }),
    );
  }
}

function MediaNodeShape({ shape }: { shape: MediaShape }) {
  const {
    title,
    prompt,
    status,
    nodeType,
    operationType,
    assetId,
    nodeTypeLabel,
    sourceMaterialStatusLabel,
    thumbnailUrl,
    previewText,
    previewAssetType,
    previewAssetUrl,
    previewThumbnailUrl,
    previewVersionNo,
    activeStaleReasonCount,
    w,
    h,
  } = shape.props;
  const typeMeta = nodeTypeMeta[nodeType];
  const typeLabel =
    nodeTypeLabel ??
    materialKindLabel({
      asset_id: assetId ?? null,
      node_type: nodeType,
      operation_type: operationType ?? "",
      status,
    });
  const sourceStatusLabel =
    sourceMaterialStatusLabel ||
    materialStatusLabel({
      asset_id: assetId ?? null,
      node_type: nodeType,
      operation_type: operationType ?? "",
      status,
    });
  const [activeNodeId, setActiveNodeId] = useState(activeMediaNodeId);
  const [titleValue, setTitleValue] = useState(title);
  const [promptValue, setPromptValue] = useState(prompt);
  const [isFlashing, setIsFlashing] = useState(false);
  const isActive = activeNodeId === shape.props.nodeId;

  useEffect(() => {
    setTitleValue(title);
  }, [title, shape.props.nodeId]);

  useEffect(() => {
    setPromptValue(prompt);
  }, [prompt, shape.props.nodeId]);

  useEffect(() => {
    const onActiveNodeChange = (event: Event) => {
      const detail = (
        event as CustomEvent<{
          nodeId: string | null;
        }>
      ).detail;
      activeMediaNodeId = detail?.nodeId ?? null;
      setActiveNodeId(activeMediaNodeId);
    };

    window.addEventListener(
      "clip-anvil:active-node-changed",
      onActiveNodeChange,
    );
    return () => {
      window.removeEventListener(
        "clip-anvil:active-node-changed",
        onActiveNodeChange,
      );
    };
  }, []);

  useEffect(() => {
    const onFlash = (event: Event) => {
      const detail = (event as CustomEvent<{ nodeIds?: string[] }>).detail;
      if (!detail?.nodeIds?.includes(shape.props.nodeId)) {
        return;
      }
      setIsFlashing(true);
      window.setTimeout(() => setIsFlashing(false), 700);
    };

    window.addEventListener("clip-anvil:node-flash", onFlash);
    return () => {
      window.removeEventListener("clip-anvil:node-flash", onFlash);
    };
  }, [shape.props.nodeId]);

  const dispatchConnectionStart = (
    pointerId: number | null,
    point?: { clientX: number; clientY: number },
  ) => {
    window.dispatchEvent(
      new CustomEvent("clip-anvil:connection-start", {
        detail: {
          clientX: point?.clientX,
          clientY: point?.clientY,
          fromNodeId: shape.props.nodeId,
          pointerId,
        },
      }),
    );
  };

  const startConnectionDrag = (event: PointerEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    event.currentTarget.setPointerCapture(event.pointerId);
    dispatchConnectionStart(event.pointerId, {
      clientX: event.clientX,
      clientY: event.clientY,
    });
  };

  const startConnectionClick = (event: SyntheticEvent) => {
    event.preventDefault();
    event.stopPropagation();
    dispatchConnectionStart(null);
  };

  const requestAssetReview = (event: SyntheticEvent) => {
    event.preventDefault();
    event.stopPropagation();
    window.dispatchEvent(
      new CustomEvent("clip-anvil:node-review-request", {
        detail: {
          accessUrl: previewAssetUrl || thumbnailUrl,
          nodeId: shape.props.nodeId,
          text: nodeType === "text" ? previewText || promptValue : undefined,
          type: previewAssetType || nodeType,
        },
      }),
    );
  };

  const preventNativeMediaDrag = (event: SyntheticEvent) => {
    event.preventDefault();
    event.stopPropagation();
  };

  const hasPreviewContent =
    Boolean(previewText) ||
    Boolean(previewAssetUrl) ||
    Boolean(previewThumbnailUrl) ||
    Boolean(thumbnailUrl);

  return (
    <HTMLContainer>
      <div
        className="media-node-shell"
        data-active={isActive}
        data-flash={isFlashing}
        data-node-id={shape.props.nodeId}
        style={{ width: w }}
      >
        <div
          className="media-node"
          data-status={status}
          data-type={nodeType}
          style={{ width: w, height: h }}
        >
          <button
            aria-label={`从 ${titleValue || typeMeta.emptyTitle} 创建依赖连线`}
            className="media-node-connect-button"
            onClick={startConnectionClick}
            onPointerDown={startConnectionDrag}
            type="button"
          >
            +
          </button>
          {hasPreviewContent ? (
            <button
              aria-label={`全屏查看 ${titleValue || typeMeta.emptyTitle}`}
              className="media-node-expand-button"
              onClick={requestAssetReview}
              type="button"
            >
              ↗
            </button>
          ) : null}
          <div className="media-node-header">
            <span className="media-node-icon">{typeLabel}</span>
            <p className="media-node-title">
              {titleValue || typeMeta.emptyTitle}
            </p>
            <span className="media-node-status">
              {sourceStatusLabel || statusText[status]}
            </span>
            {status === "stale" || Number(activeStaleReasonCount) > 0 ? (
              <span className="media-node-stale-badge">
                stale
                {activeStaleReasonCount
                  ? ` · ${activeStaleReasonCount}`
                  : ""}
              </span>
            ) : null}
          </div>
          <div className="media-node-content" data-type={nodeType}>
            {nodeType === "text" ? (
              <MarkdownPreview
                value={previewText || promptValue || "等待输入 prompt"}
                variant="canvas"
              />
            ) : nodeType === "image" ? (
              <div className="media-node-media-frame" data-kind="image">
                {previewAssetUrl || thumbnailUrl ? (
                  <img
                    alt={titleValue || typeMeta.emptyTitle}
                    draggable={false}
                    onDragStart={preventNativeMediaDrag}
                    src={previewAssetUrl || thumbnailUrl}
                  />
                ) : (
                  <div className="media-node-placeholder">
                    {previewVersionNo
                      ? `image v${previewVersionNo}`
                      : "图片占位"}
                  </div>
                )}
              </div>
            ) : nodeType === "video" ? (
              <div className="media-node-media-frame" data-kind="video">
                {previewAssetUrl ? (
                  <video
                    controls
                    draggable={false}
                    onDragStart={preventNativeMediaDrag}
                    poster={previewThumbnailUrl || thumbnailUrl}
                    preload="metadata"
                    src={previewAssetUrl}
                  />
                ) : (
                  <div className="media-node-placeholder">
                    <span>播放预览</span>
                    <span>
                      {previewAssetType
                        ? `${previewAssetType} v${previewVersionNo ?? "-"}`
                        : "0:00"}
                    </span>
                  </div>
                )}
              </div>
            ) : nodeType === "audio" ? (
              <div className="media-node-placeholder">
                <span className="media-node-waveform" />
                <span>
                  {previewAssetType
                    ? `${previewAssetType} v${previewVersionNo ?? "-"}`
                    : "0:00"}
                </span>
              </div>
            ) : (
              <div className="media-node-placeholder">
                <span>Reference Pack</span>
                <span>{previewText || "等待成员"}</span>
              </div>
            )}
          </div>
        </div>
      </div>
    </HTMLContainer>
  );
}
