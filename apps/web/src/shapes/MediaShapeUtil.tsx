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
  const [selectedModel, setSelectedModel] = useState("copywriter");
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

  const stopCanvasEvent = (event: SyntheticEvent) => {
    event.stopPropagation();
  };

  const dispatchConnectionStart = (pointerId: number | null) => {
    window.dispatchEvent(
      new CustomEvent("clip-anvil:connection-start", {
        detail: {
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
    dispatchConnectionStart(event.pointerId);
  };

  const startConnectionClick = (event: SyntheticEvent) => {
    event.preventDefault();
    event.stopPropagation();
    dispatchConnectionStart(null);
  };

  const onTitleChange = (value: string) => {
    setTitleValue(value);
    window.dispatchEvent(
      new CustomEvent("clip-anvil:title-change", {
        detail: {
          nodeId: shape.props.nodeId,
          title: value,
        },
      }),
    );
  };

  const commitTitle = () => {
    window.dispatchEvent(
      new CustomEvent("clip-anvil:title-commit", {
        detail: {
          nodeId: shape.props.nodeId,
          title: titleValue,
        },
      }),
    );
  };

  const onPromptChange = (value: string) => {
    setPromptValue(value);
    window.dispatchEvent(
      new CustomEvent("clip-anvil:prompt-change", {
        detail: {
          nodeId: shape.props.nodeId,
          prompt: value,
        },
      }),
    );
  };

  const commitPrompt = () => {
    window.dispatchEvent(
      new CustomEvent("clip-anvil:prompt-commit", {
        detail: {
          nodeId: shape.props.nodeId,
          prompt: promptValue,
        },
      }),
    );
  };

  return (
    <HTMLContainer>
      <div
        className="media-node-shell"
        data-active={isActive}
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

        {isActive ? (
          <div
            className="media-node-inline-editor"
            onContextMenu={stopCanvasEvent}
            onKeyDown={stopCanvasEvent}
            onPointerDown={stopCanvasEvent}
            onWheel={stopCanvasEvent}
            style={{ top: h + 12, width: Math.max(340, w) }}
          >
            <div className="media-node-inline-refs">
              <span>引用资源</span>
              <span>暂无引用</span>
            </div>
            <label className="media-node-inline-title">
              <span>标题</span>
              <input
                onBlur={commitTitle}
                onChange={(event) => onTitleChange(event.target.value)}
                placeholder="输入节点标题"
                value={titleValue}
              />
            </label>
            <label className="media-node-inline-prompt">
              <span>Prompt</span>
              <textarea
                onBlur={commitPrompt}
                onChange={(event) => onPromptChange(event.target.value)}
                placeholder="输入生成文本、画面描述或旁白方向"
                rows={5}
                value={promptValue}
              />
            </label>
            <div className="media-node-inline-footer">
              <label>
                <span>模型</span>
                <select
                  onBlur={commitPrompt}
                  onChange={(event) => setSelectedModel(event.target.value)}
                  value={selectedModel}
                >
                  <option value="copywriter">文案生成</option>
                  <option value="storyboard">分镜草稿</option>
                  <option value="general-video">通用视频</option>
                </select>
              </label>
              <span className="media-node-inline-save-state">自动保存</span>
            </div>
          </div>
        ) : null}
      </div>
    </HTMLContainer>
  );
}
