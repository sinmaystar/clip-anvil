import {
  HTMLContainer,
  Rectangle2d,
  ShapeUtil,
  T,
  type Geometry2d,
  type RecordProps,
} from "tldraw";
import { useEffect, useState } from "react";
import {
  GROUP_CONTAINER_SHAPE_TYPE,
  type GroupContainerShape,
  type GroupContainerShapeProps,
} from "@clip-anvil/canvas-schema";

export class GroupContainerShapeUtil extends ShapeUtil<GroupContainerShape> {
  static override type = GROUP_CONTAINER_SHAPE_TYPE;

  static override props: RecordProps<GroupContainerShape> = {
    groupId: T.string,
    name: T.string,
    nodeCount: T.number,
    w: T.number,
    h: T.number,
  };

  override getDefaultProps(): GroupContainerShapeProps {
    return {
      groupId: "",
      name: "未命名分组",
      nodeCount: 0,
      w: 320,
      h: 220,
    };
  }

  override canResize() {
    return false;
  }

  override getGeometry(shape: GroupContainerShape): Geometry2d {
    return new Rectangle2d({
      width: shape.props.w,
      height: shape.props.h,
      isFilled: false,
    });
  }

  override component(shape: GroupContainerShape) {
    return <GroupContainer shape={shape} />;
  }

  override getIndicatorPath(shape: GroupContainerShape) {
    const path = new Path2D();
    path.rect(0, 0, shape.props.w, shape.props.h);
    return path;
  }

  override onClick(shape: GroupContainerShape) {
    window.dispatchEvent(
      new CustomEvent("clip-anvil:select-group", {
        detail: { groupId: shape.props.groupId },
      }),
    );
  }
}

function GroupContainer({ shape }: { shape: GroupContainerShape }) {
  const [isFlashing, setIsFlashing] = useState(false);

  useEffect(() => {
    const onFlash = (event: Event) => {
      const detail = (event as CustomEvent<{ groupId?: string }>).detail;
      if (detail?.groupId !== shape.props.groupId) {
        return;
      }
      setIsFlashing(true);
      window.setTimeout(() => setIsFlashing(false), 700);
    };

    window.addEventListener("clip-anvil:group-flash", onFlash);
    return () => {
      window.removeEventListener("clip-anvil:group-flash", onFlash);
    };
  }, [shape.props.groupId]);

  return (
    <HTMLContainer>
      <div
        className="group-container-shape"
        data-flash={isFlashing}
        style={{ width: shape.props.w, height: shape.props.h }}
      >
        <div className="group-container-title">
          <span>{shape.props.name}</span>
          <span>{shape.props.nodeCount}</span>
        </div>
      </div>
    </HTMLContainer>
  );
}
