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
  const audioSummary = finalOutput.audio_summary;
  const finalReview = finalOutput.final_review;

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
      {audioSummary ? (
        <div className="agent-workbench-final-output-audio">
          <span>
            {audioSummary.track_count} tracks
            {audioSummary.audio_codec ? ` · ${audioSummary.audio_codec}` : ""}
          </span>
          <span>
            {[
              audioSummary.has_voiceover ? "VO" : "",
              audioSummary.has_bgm ? "BGM" : "",
              audioSummary.ducking ? "ducking" : "",
            ]
              .filter(Boolean)
              .join(" · ") || "no audio"}
          </span>
        </div>
      ) : null}
      <dl>
        <div>
          <dt>Plan</dt>
          <dd>{finalOutput.timeline_plan_id}</dd>
        </div>
        <div>
          <dt>Artifact</dt>
          <dd>{finalOutput.artifact_version_id || "none"}</dd>
        </div>
        <div>
          <dt>Review</dt>
          <dd>{finalReview?.verdict || finalReview?.status || "none"}</dd>
        </div>
      </dl>
    </article>
  );
}
