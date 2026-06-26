import type { Edge, Node } from "@xyflow/react";
import type {
  AgentWorkbenchProjection,
  AgentWorkbenchScene,
  AgentWorkbenchShot,
} from "./agentWorkbench";

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
const SHOT_MEDIA_TILE_HEIGHT = 270;
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

export function agentWorkbenchToFlow(workbench: AgentWorkbenchProjection): {
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
    const shotLayouts = layoutShots(shots);
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

function layoutShots(shots: AgentWorkbenchShot[]) {
  const layouts: Array<{ x: number; y: number; height: number }> = [];
  const rowHeights: number[] = [];

  shots.forEach((shot, index) => {
    const row = Math.floor(index / SHOTS_PER_ROW);
    const height = shotNodeHeight(shot);
    rowHeights[row] = Math.max(rowHeights[row] ?? 0, height);
  });

  const rowY = rowHeights.reduce<number[]>((offsets, height, index) => {
    const previous =
      index === 0
        ? 0
        : offsets[index - 1] + rowHeights[index - 1] + SHOT_ROW_GAP;
    offsets.push(previous);
    return offsets;
  }, []);

  shots.forEach((shot, index) => {
    const column = index % SHOTS_PER_ROW;
    const row = Math.floor(index / SHOTS_PER_ROW);
    layouts.push({
      x: column * (SHOT_WIDTH + SHOT_GAP),
      y: rowY[row],
      height: rowHeights[row] || shotNodeHeight(shot),
    });
  });

  return layouts;
}

function shotNodeHeight(shot: AgentWorkbenchShot) {
  const artifactCount = shotArtifactSlotCount(shot);
  if (!shot.creative_text && artifactCount === 0) {
    return SHOT_COMPACT_HEIGHT;
  }
  const mediaRows =
    artifactCount > 2
      ? Math.ceil(artifactCount / 2)
      : Math.max(1, artifactCount);
  const mediaHeight =
    mediaRows * SHOT_MEDIA_TILE_HEIGHT +
    Math.max(0, mediaRows - 1) * SHOT_MEDIA_GAP;
  return Math.max(SHOT_HEIGHT, SHOT_CONTENT_CHROME_HEIGHT + mediaHeight);
}

function shotArtifactSlotCount(shot: AgentWorkbenchShot) {
  if (shot.artifacts && shot.artifacts.length > 0) {
    return shot.artifacts.length;
  }
  return [shot.preview, shot.video].filter((slot) => slot.status !== "missing")
    .length;
}
