import { useQuery } from "@tanstack/react-query";
import type { MediaGroup, MediaNode } from "../lib/api";
import { fetchNodeInputs } from "../lib/api";

interface PropertyPanelProps {
  groups: MediaGroup[];
  nodes: MediaNode[];
  selectedGroupId: string | null;
  selectedNodeId: string | null;
  onDeleteGroup: (groupId: string) => void;
}

export function PropertyPanel({
  groups,
  nodes,
  selectedGroupId,
  selectedNodeId,
  onDeleteGroup,
}: PropertyPanelProps) {
  const selectedGroup =
    groups.find((group) => group.id === selectedGroupId) ?? null;
  const selectedNode = nodes.find((node) => node.id === selectedNodeId) ?? null;

  if (selectedGroup) {
    const memberNodes = nodes.filter((node) =>
      selectedGroup.node_ids.includes(node.id),
    );
    return (
      <aside className="property-panel">
        <PanelHeader eyebrow="Group" title={selectedGroup.name} />
        <dl className="property-list">
          <div>
            <dt>成员数量</dt>
            <dd>{memberNodes.length}</dd>
          </div>
          <div>
            <dt>排序</dt>
            <dd>{selectedGroup.sort_order}</dd>
          </div>
        </dl>
        <div className="property-section">
          <p className="studio-section-label">Members</p>
          {memberNodes.map((node) => (
            <div className="property-row" key={node.id}>
              <span>{node.title}</span>
              <span>{node.node_type}</span>
            </div>
          ))}
        </div>
        <button
          className="studio-secondary-button property-danger"
          onClick={() => onDeleteGroup(selectedGroup.id)}
          type="button"
        >
          删除分组
        </button>
      </aside>
    );
  }

  if (selectedNode) {
    return <NodePropertyPanel groups={groups} node={selectedNode} />;
  }

  return (
    <aside className="property-panel">
      <PanelHeader eyebrow="Inspector" title="未选择" />
      <p className="property-empty">选择节点或分组查看属性。</p>
    </aside>
  );
}

function NodePropertyPanel({
  groups,
  node,
}: {
  groups: MediaGroup[];
  node: MediaNode;
}) {
  const inputsQuery = useQuery({
    queryKey: ["node", node.id, "inputs"],
    queryFn: () => fetchNodeInputs(node.id),
  });
  const group = groups.find((item) => item.id === node.group_id);

  return (
    <aside className="property-panel">
      <PanelHeader eyebrow={node.node_type} title={node.title} />
      <dl className="property-list">
        <div>
          <dt>状态</dt>
          <dd>{node.status}</dd>
        </div>
        <div>
          <dt>分组</dt>
          <dd>{group?.name ?? "未分组"}</dd>
        </div>
      </dl>
      <div className="property-section">
        <p className="studio-section-label">Inputs</p>
        {inputsQuery.data && inputsQuery.data.length > 0 ? (
          inputsQuery.data.map((input) => (
            <div className="property-row" key={input.id}>
              <span>{input.title}</span>
              <span>{input.node_type}</span>
            </div>
          ))
        ) : (
          <p className="property-empty">暂无上游输入。</p>
        )}
      </div>
      <div className="property-section">
        <p className="studio-section-label">Prompt</p>
        <p className="property-prompt">{node.prompt || "暂无 prompt"}</p>
      </div>
    </aside>
  );
}

function PanelHeader({ eyebrow, title }: { eyebrow: string; title: string }) {
  return (
    <div className="property-panel-header">
      <p>{eyebrow}</p>
      <h2>{title}</h2>
    </div>
  );
}
