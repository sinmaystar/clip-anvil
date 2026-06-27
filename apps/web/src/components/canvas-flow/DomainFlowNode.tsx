import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import type { CanvasFlowDomainData } from "./flowTypes";

type DomainFlowNodeModel = Node<CanvasFlowDomainData, "domain">;

const domainKindLabel: Record<string, string> = {
  creative_brief: "CreativeBrief",
  project_memory: "ProjectMemory",
  key_element: "KeyElement",
  key_element_state: "ElementState",
  scene: "Scene",
  shot: "Shot",
  render_plan: "RenderPlan",
  review_record: "Review",
  artifact_issue: "Issue",
};

export function DomainFlowNode({ data }: NodeProps<DomainFlowNodeModel>) {
  const node = data.node;
  return (
    <div className="domain-node-shell" data-kind={node.kind}>
      <Handle
        className="domain-node-handle"
        isConnectable={false}
        position={Position.Left}
        type="target"
      />
      <div className="domain-node-header">
        <span className="domain-node-kind">
          {domainKindLabel[node.kind] ?? node.kind}
        </span>
        {node.status ? <span className="domain-node-status">{node.status}</span> : null}
      </div>
      <div className="domain-node-title">{node.title || "未命名对象"}</div>
      {node.subtitle ? (
        <div className="domain-node-subtitle">{node.subtitle}</div>
      ) : null}
      {node.meta ? (
        <div className="domain-node-meta">
          {Object.entries(node.meta).map(([key, value]) => (
            <span key={key}>
              {key}: {value}
            </span>
          ))}
        </div>
      ) : null}
      <Handle
        className="domain-node-handle"
        isConnectable={false}
        position={Position.Right}
        type="source"
      />
    </div>
  );
}
