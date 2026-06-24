import type { MediaNode } from "./api";
import {
  adaptiveMediaNodeSize,
  type MediaDimensions,
} from "./nodePreviewLayout";

export function mediaNodeDisplaySize(
  node: Pick<
    MediaNode,
    | "node_type"
    | "canvas_w"
    | "canvas_h"
    | "prompt"
    | "production_preview"
    | "reference_pack_preview"
  >,
  measuredMediaDimensions?: MediaDimensions | null,
) {
  const size = adaptiveMediaNodeSize(node, measuredMediaDimensions);
  return { w: size.w, h: size.h };
}
