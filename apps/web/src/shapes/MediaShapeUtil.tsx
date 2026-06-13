import {
  HTMLContainer,
  Rectangle2d,
  ShapeUtil,
  T,
  type Geometry2d,
  type RecordProps,
} from "tldraw";
import { useEffect, useState, type SyntheticEvent } from "react";
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
  const { title, prompt, status, w, h } = shape.props;
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
        style={{ width: w }}
      >
        <div
          className="media-node"
          data-status={status}
          data-type="text"
          style={{ width: w, height: h }}
        >
          <div className="media-node-header">
            <span className="media-node-icon">T</span>
            <p className="media-node-title">{titleValue || "未命名文本"}</p>
            <span className="media-node-status">{statusText[status]}</span>
          </div>
          <div className="media-node-content">
            <p>{promptValue || "等待输入 prompt"}</p>
          </div>
          <div className="media-node-footer">
            <span>Text</span>
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
