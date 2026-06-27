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
  onShotHeightChange?: (shotId: string, height: number) => void;
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
const SHOT_MEDIA_ROW_WIDTH = 480;
const SHOT_GAP = 32;
const SHOT_ROW_GAP = 32;
const SHOTS_PER_ROW = 2;
const SCENE_GAP = 72;
const SCENE_COLUMNS = 2;
const ORIGIN_X = 40;
const ORIGIN_Y = 40;
const SCENE_X = ORIGIN_X + OVERVIEW_WIDTH + 80;
const COMPACT_SCENE_MAX_WIDTH = SCENE_PADDING * 2 + SHOT_WIDTH + 24;

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
  measuredShotHeights: Record<string, number | undefined> = {},
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

  const sceneLayouts = workbench.scenes.map((scene) =>
    buildSceneLayout(scene, mediaDimensions, measuredShotHeights),
  );
  const sceneColumns = sceneColumnCount(sceneLayouts);
  const sceneColumnWidth = sceneLayouts.reduce(
    (width, layout) => Math.max(width, layout.width),
    0,
  );
  const sceneColumnHeights = Array.from({ length: sceneColumns }, () => 0);

  for (const sceneLayout of sceneLayouts) {
    const { scene, shotLayouts, shots } = sceneLayout;
    const sceneColumn =
      sceneColumnHeights.length === 1
        ? 0
        : shortestColumn(sceneColumnHeights);
    const scenePosition = {
      x: SCENE_X + sceneColumn * (sceneColumnWidth + SCENE_GAP),
      y: ORIGIN_Y + sceneColumnHeights[sceneColumn],
    };
    const currentSceneNodeId = sceneNodeId(scene.id);

    nodes.push({
      id: currentSceneNodeId,
      type: "agentScene",
      position: scenePosition,
      data: { kind: "scene", scene },
      width: sceneLayout.width,
      height: sceneLayout.height,
      measured: { width: sceneLayout.width, height: sceneLayout.height },
      style: { width: sceneLayout.width, height: sceneLayout.height },
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

    sceneColumnHeights[sceneColumn] += sceneLayout.height + SCENE_GAP;
  }

  return { nodes, edges };
}

function buildSceneLayout(
  scene: AgentWorkbenchScene,
  mediaDimensions: AgentWorkbenchMediaDimensionsByKey,
  measuredShotHeights: Record<string, number | undefined>,
) {
  const shots = sortedShots(scene.shots);
  const shotLayouts = layoutShots(shots, mediaDimensions, measuredShotHeights);
  const width =
    SCENE_PADDING * 2 +
    Math.max(1, Math.min(SHOTS_PER_ROW, shots.length)) * SHOT_WIDTH +
    Math.max(0, Math.min(SHOTS_PER_ROW, shots.length) - 1) * SHOT_GAP;
  const height =
    SCENE_HEADER +
    shotLayouts.reduce(
      (currentHeight, layout) => Math.max(currentHeight, layout.y + layout.height),
      0,
    ) +
    SCENE_PADDING;
  return { height, scene, shotLayouts, shots, width };
}

function sceneColumnCount(
  sceneLayouts: Array<ReturnType<typeof buildSceneLayout>>,
) {
  if (sceneLayouts.length <= 1) {
    return 1;
  }
  const widestScene = sceneLayouts.reduce(
    (width, layout) => Math.max(width, layout.width),
    0,
  );
  if (widestScene > COMPACT_SCENE_MAX_WIDTH) {
    return 1;
  }
  return Math.min(SCENE_COLUMNS, sceneLayouts.length);
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
  measuredShotHeights: Record<string, number | undefined>,
) {
  const layouts: Array<{ x: number; y: number; height: number }> = [];
  const columnCount = Math.max(1, Math.min(SHOTS_PER_ROW, shots.length));
  const columnHeights = Array.from({ length: columnCount }, () => 0);

  shots.forEach((shot, index) => {
    const column = index < columnCount ? index : shortestColumn(columnHeights);
    const height = Math.max(
      shotNodeHeight(shot, mediaDimensions),
      measuredShotHeights[shot.id] ?? 0,
    );
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
      ? mediaStackHeight(mediaSlots, mediaDimensions)
      : agentWorkbenchMediaSize(undefined).height;
  return Math.max(SHOT_HEIGHT, SHOT_CONTENT_CHROME_HEIGHT + mediaHeight);
}

function mediaStackHeight(
  slots: AgentWorkbenchArtifactSlot[],
  mediaDimensions: AgentWorkbenchMediaDimensionsByKey,
) {
  const rows = slots.reduce<Array<{ height: number; width: number }>>(
    (currentRows, slot) => {
      const size = agentWorkbenchMediaSize(
        slot,
        mediaDimensions[agentWorkbenchMediaKey(slot)],
      );
      const activeRow = currentRows[currentRows.length - 1];
      if (
        activeRow &&
        activeRow.width + SHOT_MEDIA_GAP + size.width <= SHOT_MEDIA_ROW_WIDTH
      ) {
        activeRow.width += SHOT_MEDIA_GAP + size.width;
        activeRow.height = Math.max(activeRow.height, size.height);
        return currentRows;
      }
      currentRows.push({ height: size.height, width: size.width });
      return currentRows;
    },
    [],
  );
  return (
    rows.reduce((height, row) => height + row.height, 0) +
    Math.max(0, rows.length - 1) * SHOT_MEDIA_GAP
  );
}

function shotArtifactSlots(shot: AgentWorkbenchShot): AgentWorkbenchArtifactSlot[] {
  if (shot.artifacts && shot.artifacts.length > 0) {
    return shot.artifacts;
  }
  return [shot.preview, shot.video].filter((slot) => slot.status !== "missing")
    .filter((slot): slot is AgentWorkbenchArtifactSlot => Boolean(slot));
}
