import type { Node, NodeProps } from "@xyflow/react";
import type { AgentWorkbenchSceneNodeData } from "../../lib/agentWorkbenchViewModel";

type SceneNode = Node<AgentWorkbenchSceneNodeData, "agentScene">;

export function AgentSceneGroupNode({
  data,
  selected,
}: NodeProps<SceneNode>) {
  const scene = data.scene;
  const sceneText = [scene.location, scene.summary]
    .map((text) => text?.trim())
    .filter(Boolean)
    .join(" · ");
  return (
    <section className="agent-workbench-scene-node" data-selected={selected}>
      <header className="agent-workbench-scene-header">
        <div>
          <span>Scene</span>
          <strong>{scene.title || "未命名场景"}</strong>
        </div>
        <em>{scene.status || "planned"}</em>
      </header>
      {sceneText ? <p>{sceneText}</p> : null}
    </section>
  );
}
