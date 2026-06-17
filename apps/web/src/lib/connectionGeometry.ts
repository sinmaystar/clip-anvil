export interface ConnectionNodeBounds {
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface CanvasNodeLike {
  id: string;
  canvas_x: number;
  canvas_y: number;
  canvas_w: number;
  canvas_h: number;
}

export interface Point {
  x: number;
  y: number;
}

export function mediaNodeBounds(node: CanvasNodeLike): ConnectionNodeBounds {
  return {
    id: node.id,
    x: node.canvas_x,
    y: node.canvas_y,
    w: node.canvas_w,
    h: node.canvas_h,
  };
}

export function outputAnchor(bounds: ConnectionNodeBounds): Point {
  return {
    x: bounds.x + bounds.w,
    y: bounds.y + bounds.h / 2,
  };
}

export function inputAnchor(bounds: ConnectionNodeBounds): Point {
  return {
    x: bounds.x,
    y: bounds.y + bounds.h / 2,
  };
}

export function connectionPath(start: Point, end: Point): string {
  const distance = Math.abs(end.x - start.x);
  const pull = Math.max(60, Math.min(180, distance * 0.6));
  const c1 = { x: start.x + pull, y: start.y };
  const c2 = { x: end.x - pull, y: end.y };
  return `M ${round(start.x)} ${round(start.y)} C ${round(c1.x)} ${round(
    c1.y,
  )}, ${round(c2.x)} ${round(c2.y)}, ${round(end.x)} ${round(end.y)}`;
}

function round(value: number) {
  return Math.round(value * 100) / 100;
}
