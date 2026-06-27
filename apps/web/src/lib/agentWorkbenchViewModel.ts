import type { Edge, Node } from "@xyflow/react";
import type {
  AgentWorkbenchArtifactSlot,
  AgentWorkbenchProjection,
  AgentWorkbenchScene,
  AgentWorkbenchShot,
} from "./agentWorkbench";
import {
  agentWorkbenchMediaKey,
  agentWorkbenchMediaSize,
  type AgentWorkbenchMediaDimensionsByKey,
} from "./agentWorkbenchMediaLayout.js";

export type AgentWorkbenchNode =
  | Node<AgentWorkbenchOverviewNodeData, "agentOverview">
  | Node<AgentWorkbenchSceneNodeData, "agentScene">
  | Node<AgentWorkbenchShotNodeData, "agentShot">;

export type AgentWorkbenchEdge = Edge<AgentWorkbenchEdgeData, "agentWorkbench">;

export interface AgentWorkbenchOverviewNodeData extends Record<
  string,
  unknown
> {
  kind: "overview";
  workbench: AgentWorkbenchProjection;
}

export interface AgentWorkbenchSceneNodeData extends Record<string, unknown> {
  kind: "scene";
  scene: AgentWorkbenchScene;
}

export interface AgentWorkbenchShotNodeData extends Record<string, unknown> {
  kind: "shot";
  shot: AgentWorkbenchShot;
  mediaDimensions?: AgentWorkbenchMediaDimensionsByKey;
  onMediaDimensionsChange?: (
    key: string,
    dimensions: { width: number; height: number },
  ) => void;
}

export interface AgentWorkbenchEdgeData extends Record<string, unknown> {
  label?: string;
}

const OVERVIEW_WIDTH = 360;
const OVERVIEW_HEIGHT = 248;
const SCENE_PADDING = 28;
const SCENE_HEADER = 116;
const SHOT_WIDTH = 520;
const SHOT_HEIGHT = 560;
const SHOT_COMPACT_HEIGHT = 420;
const SHOT_CONTENT_CHROME_HEIGHT = 252;
const SHOT_MEDIA_GAP = 12;
const SHOT_GAP = 32;
const SHOT_ROW_GAP = 32;
const SHOTS_PER_ROW = 2;
const SCENE_GAP = 72;
const ORIGIN_X = 40;
const ORIGIN_Y = 40;
const SCENE_X = ORIGIN_X + OVERVIEW_WIDTH + 80;

export function overviewNodeId() {
  return "agent-workbench-overview";
}

export function sceneNodeId(sceneId: string) {
  return `agent-scene-${sceneId}`;
}

export function shotNodeId(shotId: string) {
  return `agent-shot-${shotId}`;
}

export function agentWorkbenchToFlow(
  workbench: AgentWorkbenchProjection,
  mediaDimensions: AgentWorkbenchMediaDimensionsByKey = {},
): {
  nodes: AgentWorkbenchNode[];
  edges: AgentWorkbenchEdge[];
} {
  const nodes: AgentWorkbenchNode[] = [
    {
      id: overviewNodeId(),
      type: "agentOverview",
      position: { x: ORIGIN_X, y: ORIGIN_Y },
      data: { kind: "overview", workbench },
      width: OVERVIEW_WIDTH,
      height: OVERVIEW_HEIGHT,
      measured: { width: OVERVIEW_WIDTH, height: OVERVIEW_HEIGHT },
      style: { width: OVERVIEW_WIDTH, height: OVERVIEW_HEIGHT },
      draggable: false,
      selectable: true,
    },
  ];
  const edges: AgentWorkbenchEdge[] = [];

  let sceneY = ORIGIN_Y;
  for (const scene of workbench.scenes) {
    const shots = sortedShots(scene.shots);
    const shotLayouts = layoutShots(shots, mediaDimensions);
    const sceneWidth =
      SCENE_PADDING * 2 +
      Math.max(1, Math.min(SHOTS_PER_ROW, shots.length)) * SHOT_WIDTH +
      Math.max(0, Math.min(SHOTS_PER_ROW, shots.length) - 1) * SHOT_GAP;
    const sceneHeight =
      SCENE_HEADER +
      shotLayouts.reduce(
        (height, layout) => Math.max(height, layout.y + layout.height),
        0,
      ) +
      SCENE_PADDING;
    const currentSceneNodeId = sceneNodeId(scene.id);

    nodes.push({
      id: currentSceneNodeId,
      type: "agentScene",
      position: { x: SCENE_X, y: sceneY },
      data: { kind: "scene", scene },
      width: sceneWidth,
      height: sceneHeight,
      measured: { width: sceneWidth, height: sceneHeight },
      style: { width: sceneWidth, height: sceneHeight },
      draggable: false,
      selectable: true,
    });
    edges.push({
      id: `agent-edge-overview-${scene.id}`,
      type: "agentWorkbench",
      source: overviewNodeId(),
      target: currentSceneNodeId,
      data: { label: "scene" },
    });

    shots.forEach((shot, index) => {
      const layout = shotLayouts[index];
      const currentShotNodeId = shotNodeId(shot.id);
      nodes.push({
        id: currentShotNodeId,
        type: "agentShot",
        parentId: currentSceneNodeId,
        extent: "parent",
        position: {
          x: SCENE_PADDING + layout.x,
          y: SCENE_HEADER + layout.y,
        },
        data: { kind: "shot", shot },
        width: SHOT_WIDTH,
        height: layout.height,
        measured: { width: SHOT_WIDTH, height: layout.height },
        style: { width: SHOT_WIDTH, height: layout.height },
        draggable: false,
        selectable: true,
      });
      if (index > 0) {
        edges.push({
          id: `agent-edge-shot-${shots[index - 1].id}-${shot.id}`,
          type: "agentWorkbench",
          source: shotNodeId(shots[index - 1].id),
          target: currentShotNodeId,
          data: { label: "next" },
        });
      }
    });

    sceneY += sceneHeight + SCENE_GAP;
  }

  return { nodes, edges };
}

function sortedShots(shots: AgentWorkbenchShot[]) {
  return [...shots].sort((left, right) => {
    if (left.sequence_index !== right.sequence_index) {
      return left.sequence_index - right.sequence_index;
    }
    return left.client_key.localeCompare(right.client_key);
  });
}

function layoutShots(
  shots: AgentWorkbenchShot[],
  mediaDimensions: AgentWorkbenchMediaDimensionsByKey,
) {
  const layouts: Array<{ x: number; y: number; height: number }> = [];
  const columnCount = Math.max(1, Math.min(SHOTS_PER_ROW, shots.length));
  const columnHeights = Array.from({ length: columnCount }, () => 0);

  shots.forEach((shot, index) => {
    const column = index < columnCount ? index : shortestColumn(columnHeights);
    const height = shotNodeHeight(shot, mediaDimensions);
    layouts.push({
      x: column * (SHOT_WIDTH + SHOT_GAP),
      y: columnHeights[column],
      height,
    });
    columnHeights[column] += height + SHOT_ROW_GAP;
  });

  return layouts;
}

function shortestColumn(columnHeights: number[]) {
  return columnHeights.reduce(
    (shortest, height, index) =>
      height < columnHeights[shortest] ? index : shortest,
    0,
  );
}

function shotNodeHeight(
  shot: AgentWorkbenchShot,
  mediaDimensions: AgentWorkbenchMediaDimensionsByKey,
) {
  const mediaSlots = shotArtifactSlots(shot);
  if (!shot.creative_text && mediaSlots.length === 0) {
    return SHOT_COMPACT_HEIGHT;
  }
  const mediaHeight =
    mediaSlots.length > 0
      ? mediaSlots.reduce(
          (height, slot) =>
            height +
            agentWorkbenchMediaSize(
              slot,
              mediaDimensions[agentWorkbenchMediaKey(slot)],
            ).height,
          0,
        ) +
        Math.max(0, mediaSlots.length - 1) * SHOT_MEDIA_GAP
      : agentWorkbenchMediaSize(undefined).height;
  return Math.max(SHOT_HEIGHT, SHOT_CONTENT_CHROME_HEIGHT + mediaHeight);
}

function shotArtifactSlots(shot: AgentWorkbenchShot): AgentWorkbenchArtifactSlot[] {
  if (shot.artifacts && shot.artifacts.length > 0) {
    return shot.artifacts;
  }
  return [shot.preview, shot.video].filter((slot) => slot.status !== "missing")
    .filter((slot): slot is AgentWorkbenchArtifactSlot => Boolean(slot));
}
