import type { MediaNode } from "./api";
import { adaptiveMediaNodeSize } from "./nodePreviewLayout";

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
) {
  const size = adaptiveMediaNodeSize(node);
  return { w: size.w, h: size.h };
}
