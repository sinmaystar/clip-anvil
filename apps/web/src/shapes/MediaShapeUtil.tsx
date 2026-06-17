import {
  HTMLContainer,
  Rectangle2d,
  ShapeUtil,
  T,
  type Geometry2d,
  type RecordProps,
} from "tldraw";
import { useEffect, useState, type PointerEvent, type SyntheticEvent } from "react";
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

const nodeTypeMeta: Record<
  MediaShape["props"]["nodeType"],
  { icon: string; label: string; emptyTitle: string }
> = {
  text: { icon: "T", label: "Text", emptyTitle: "未命名文本" },
  image: { icon: "I", label: "Image", emptyTitle: "未命名图片" },
  video: { icon: "V", label: "Video", emptyTitle: "未命名视频" },
  audio: { icon: "A", label: "Audio", emptyTitle: "未命名音频" },
};

let activeMediaNodeId: string | null = null;

export class MediaShapeUtil extends ShapeUtil<MediaShape> {
  static override type = MEDIA_SHAPE_TYPE;

  static override props: RecordProps<MediaShape> = {
    nodeId: T.string,
    nodeType: T.literalEnum("text", "image", "video", "audio"),
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
  const { title, prompt, status, nodeType, thumbnailUrl, w, h } = shape.props;
  const typeMeta = nodeTypeMeta[nodeType];
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
          <span
            aria-label="依赖输入端口"
            className="media-node-port media-node-port-input"
            role="img"
          />
          <button
            aria-label={`从 ${titleValue || typeMeta.emptyTitle} 创建依赖连线`}
            className="media-node-port media-node-port-output"
            onClick={startConnectionClick}
            onPointerDown={startConnectionDrag}
            type="button"
          />
          <div className="media-node-header">
            <span className="media-node-icon">{typeMeta.icon}</span>
            <p className="media-node-title">
              {titleValue || typeMeta.emptyTitle}
            </p>
            <span className="media-node-status">{statusText[status]}</span>
          </div>
          <div className="media-node-content" data-type={nodeType}>
            {nodeType === "text" ? (
              <p>{promptValue || "等待输入 prompt"}</p>
            ) : nodeType === "image" ? (
              thumbnailUrl ? (
                <img
                  alt={titleValue || typeMeta.emptyTitle}
                  src={thumbnailUrl}
                />
              ) : (
                <div className="media-node-placeholder">图片占位</div>
              )
            ) : nodeType === "video" ? (
              <div className="media-node-placeholder">
                <span>播放预览</span>
                <span>0:00</span>
              </div>
            ) : (
              <div className="media-node-placeholder">
                <span className="media-node-waveform" />
                <span>0:00</span>
              </div>
            )}
          </div>
          <div className="media-node-footer">
            <span>{typeMeta.label}</span>
            <span>Prompt</span>
          </div>
        </div>
      </div>
    </HTMLContainer>
  );
}
