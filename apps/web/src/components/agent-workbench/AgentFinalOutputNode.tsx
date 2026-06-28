import type { Node, NodeProps } from "@xyflow/react";
import type { AgentWorkbenchFinalOutputNodeData } from "../../lib/agentWorkbenchViewModel";

type FinalOutputNode = Node<
  AgentWorkbenchFinalOutputNodeData,
  "agentFinalOutput"
>;

export function AgentFinalOutputNode({
  data,
  selected,
}: NodeProps<FinalOutputNode>) {
  const finalOutput = data.finalOutput;
  const mediaURL = finalOutput.asset_url || finalOutput.thumbnail_url;

  return (
    <article
      className="agent-workbench-final-output-node"
      data-selected={selected}
      data-status={finalOutput.status}
    >
      <header>
        <div>
          <span>Final</span>
          <strong>{finalOutput.template_key || "timeline"}</strong>
        </div>
        <em>{finalOutput.status}</em>
      </header>
      <div className="agent-workbench-final-output-preview">
        {mediaURL ? (
          finalOutput.mime?.startsWith("video/") || finalOutput.asset_url ? (
            <video controls preload="metadata" src={mediaURL} />
          ) : (
            <img alt="" src={mediaURL} />
          )
        ) : (
          <span>{finalOutput.status}</span>
        )}
      </div>
      <p>{finalOutput.summary || finalOutput.output_node_id || "pending"}</p>
      <dl>
        <div>
          <dt>Plan</dt>
          <dd>{finalOutput.timeline_plan_id}</dd>
        </div>
        <div>
          <dt>Artifact</dt>
          <dd>{finalOutput.artifact_version_id || "none"}</dd>
        </div>
      </dl>
    </article>
  );
}
