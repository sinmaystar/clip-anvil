import type { Node, NodeProps } from "@xyflow/react";
import type { AgentWorkbenchAudioNodeData } from "../../lib/agentWorkbenchViewModel";

type AudioNode = Node<AgentWorkbenchAudioNodeData, "agentAudio">;

export function AgentAudioNode({ data, selected }: NodeProps<AudioNode>) {
  const artifact = data.artifact;
  const audioTitle = artifact.title || data.planTitle || data.label;
  return (
    <article
      className="agent-workbench-audio-node"
      data-selected={selected}
      data-status={artifact.status}
      title={audioTitle}
    >
      <header>
        <div>
          <span>Audio</span>
          <strong>{data.label}</strong>
        </div>
        <em>{artifact.status}</em>
      </header>
      {artifact.access_url ? (
        <audio
          aria-label={`播放 ${data.label}: ${audioTitle}`}
          controls
          onClick={(event) => event.stopPropagation()}
          preload="metadata"
          src={artifact.access_url}
          title={audioTitle}
        />
      ) : (
        <div className="agent-workbench-audio-placeholder">
          {artifact.error_message || artifact.status || "暂无音频"}
        </div>
      )}
    </article>
  );
}
