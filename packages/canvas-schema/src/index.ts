import type { TLBaseShape } from "@tldraw/tlschema";

export const MEDIA_SHAPE_TYPE = "media" as const;

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
  w: number;
  h: number;
}

export type MediaShape = TLBaseShape<
  typeof MEDIA_SHAPE_TYPE,
  MediaShapeProps
>;

declare module "@tldraw/tlschema" {
  interface TLGlobalShapePropsMap {
    [MEDIA_SHAPE_TYPE]: MediaShapeProps;
  }
}
