import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import type { AgentWorkbenchOverviewNodeData } from "../../lib/agentWorkbenchViewModel";
import { useAgentWorkbenchSelection } from "./AgentWorkbenchSelectionContext";

type OverviewNode = Node<AgentWorkbenchOverviewNodeData, "agentOverview">;

export function AgentProjectOverviewNode({
  data,
  selected,
}: NodeProps<OverviewNode>) {
  const workbench = data.workbench;
  const brief = workbench.overview.brief;
  const memory = workbench.overview.memory;
  const audioPlan = workbench.overview.audio_plan;
  const selection = useAgentWorkbenchSelection();

  return (
    <section className="agent-workbench-overview-node" data-selected={selected}>
      <Handle
        className="agent-workbench-handle"
        isConnectable={false}
        position={Position.Right}
        type="source"
      />
      <header>
        <span>Project</span>
        <strong>{brief?.title || "未命名项目"}</strong>
      </header>
      <p>{brief?.concept || memory?.soul || "等待 Producer 创建项目约束。"}</p>
      <dl>
        <div>
          <dt>Scenes</dt>
          <dd>{workbench.counts.scenes}</dd>
        </div>
        <div>
          <dt>Shots</dt>
          <dd>{workbench.counts.shots}</dd>
        </div>
        <div>
          <dt>Issues</dt>
          <dd>{workbench.counts.open_issues}</dd>
        </div>
        <div>
          <dt>Audio</dt>
          <dd>
            {workbench.counts.audio_ready}/{workbench.counts.audio_ready + workbench.counts.audio_missing}
          </dd>
        </div>
      </dl>
      {audioPlan ? (
        <div className="agent-workbench-overview-audio">
          <div>
            <strong>{audioPlan.title || "AudioPlan"}</strong>
            <span>{audioPlan.status}</span>
          </div>
          <div>
            <span>VO {audioPlan.voiceover_status || "missing"}</span>
            <span>BGM {audioPlan.bgm_status || "missing"}</span>
          </div>
          {audioPlan.voiceover_script ? (
            <p>{audioPlan.voiceover_script}</p>
          ) : null}
        </div>
      ) : null}
      {workbench.overview.source_materials.length > 0 ? (
        <div className="agent-workbench-overview-materials">
          {workbench.overview.source_materials.slice(0, 6).map((material) => {
            const previewURL = material.thumbnail_url || material.access_url;
            return (
              <button
                data-selected={selection.isSelected("artifact", material.id)}
                key={material.id}
                onClick={(event) => {
                  event.stopPropagation();
                  selection.select({
                    objectType: "artifact",
                    objectId: material.id,
                    label: material.title,
                  });
                }}
                type="button"
              >
                {previewURL && material.node_type === "image" ? (
                  <img alt="" src={previewURL} />
                ) : (
                  <span>{material.node_type}</span>
                )}
              </button>
            );
          })}
        </div>
      ) : null}
      <div className="agent-workbench-overview-elements">
        {workbench.overview.key_elements.slice(0, 5).map((element) => (
          <button
            data-selected={selection.isSelected("key_element", element.id)}
            key={element.id}
            onClick={(event) => {
              event.stopPropagation();
              selection.select({
                objectType: "key_element",
                objectId: element.id,
                label: element.name,
              });
            }}
            type="button"
          >
            {element.name}
          </button>
        ))}
      </div>
      <div className="agent-workbench-overview-states">
        {workbench.overview.key_element_states.slice(0, 6).map((state) => (
          <button
            data-selected={selection.isSelected("key_element_state", state.id)}
            data-tone={
              state.reference_status === "needs_reference"
                ? "warning"
                : undefined
            }
            key={state.id}
            onClick={(event) => {
              event.stopPropagation();
              selection.select({
                objectType: "key_element_state",
                objectId: state.id,
                label: state.label,
              });
            }}
            type="button"
          >
            {state.label || state.client_key}
          </button>
        ))}
      </div>
    </section>
  );
}
