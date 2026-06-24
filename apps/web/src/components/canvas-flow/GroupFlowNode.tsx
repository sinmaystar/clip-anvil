import type { Node, NodeProps } from "@xyflow/react";
import type { CanvasFlowGroupData } from "./flowTypes";

type GroupFlowNodeModel = Node<CanvasFlowGroupData, "group">;

export function GroupFlowNode({
  data,
  selected,
}: NodeProps<GroupFlowNodeModel>) {
  const nodeCount = data.nodeIds.length;

  return (
    <section
      className="group-container-shape group-flow-node"
      data-selected={selected}
    >
      <div className="group-container-title group-flow-drag-handle">
        <span>{data.group.name}</span>
        <span>{nodeCount}</span>
      </div>
    </section>
  );
}
