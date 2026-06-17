import type { TLBaseShape } from "@tldraw/tlschema";

export const MEDIA_SHAPE_TYPE = "media" as const;
export const GROUP_CONTAINER_SHAPE_TYPE = "group-container" as const;

export type MediaType = "text" | "image" | "video" | "audio";

export type NodeStatus =
  | "draft"
  | "ready"
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "stale"
  | "user_editing";

export interface MediaShapeProps {
  nodeId: string;
  nodeType: MediaType;
  title: string;
  prompt: string;
  status: NodeStatus;
  thumbnailUrl?: string;
  w: number;
  h: number;
}

export type MediaShape = TLBaseShape<
  typeof MEDIA_SHAPE_TYPE,
  MediaShapeProps
>;

export interface GroupContainerShapeProps {
  groupId: string;
  name: string;
  nodeCount: number;
  w: number;
  h: number;
}

export type GroupContainerShape = TLBaseShape<
  typeof GROUP_CONTAINER_SHAPE_TYPE,
  GroupContainerShapeProps
>;

declare module "@tldraw/tlschema" {
  interface TLGlobalShapePropsMap {
    [MEDIA_SHAPE_TYPE]: MediaShapeProps;
    [GROUP_CONTAINER_SHAPE_TYPE]: GroupContainerShapeProps;
  }
}
