import type { Viewport } from "@xyflow/react";
import type { CanvasCamera } from "../../lib/api";

export function cameraToViewport(camera: CanvasCamera): Viewport {
  return {
    x: camera.x,
    y: camera.y,
    zoom: camera.zoom,
  };
}

export function viewportToCamera(viewport: Viewport): CanvasCamera {
  return {
    x: viewport.x,
    y: viewport.y,
    zoom: viewport.zoom,
  };
}
