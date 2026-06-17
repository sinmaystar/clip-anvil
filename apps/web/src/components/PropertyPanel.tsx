import type { MediaEdge, MediaGroup, MediaNode } from "../lib/api";
import {
  getEdgeDetail,
  getGroupMembers,
  getNodeDependencies,
} from "../lib/canvasSelectors";

interface PropertyPanelProps {
  edges: MediaEdge[];
  groups: MediaGroup[];
  nodes: MediaNode[];
  selectedEdgeId: string | null;
  selectedGroupId: string | null;
  selectedNodeId: string | null;
  isUpdatingGroupMembers: boolean;
  onAddGroupMember: (groupId: string, nodeId: string) => void;
  onChangeNodeGroup: (nodeId: string, groupId: string | null) => void;
  onDeleteEdge: (edgeId: string) => void;
  onDeleteGroup: (groupId: string) => void;
  onRemoveGroupMember: (groupId: string, nodeId: string) => void;
  onRenameGroup: (groupId: string, name: string) => void;
}

export function PropertyPanel({
  edges,
  groups,
  nodes,
  selectedEdgeId,
  selectedGroupId,
  selectedNodeId,
  isUpdatingGroupMembers,
  onAddGroupMember,
  onChangeNodeGroup,
  onDeleteEdge,
  onDeleteGroup,
  onRemoveGroupMember,
  onRenameGroup,
}: PropertyPanelProps) {
  const selectedEdge = getEdgeDetail(nodes, edges, selectedEdgeId);
  const selectedGroup =
    groups.find((group) => group.id === selectedGroupId) ?? null;
  const selectedNode = nodes.find((node) => node.id === selectedNodeId) ?? null;

  if (selectedEdge) {
    return (
      <EdgePropertyPanel detail={selectedEdge} onDeleteEdge={onDeleteEdge} />
    );
  }

  if (selectedGroup) {
    return (
      <GroupPropertyPanel
        candidateNodes={getGroupCandidateNodes(nodes, selectedGroup)}
        group={selectedGroup}
        isUpdatingMembers={isUpdatingGroupMembers}
        memberNodes={getGroupMembers(nodes, selectedGroup)}
        onAddGroupMember={onAddGroupMember}
        onDeleteGroup={onDeleteGroup}
        onRemoveGroupMember={onRemoveGroupMember}
        onRenameGroup={onRenameGroup}
      />
    );
  }

  if (selectedNode) {
    return (
      <NodePropertyPanel
        edges={edges}
        groups={groups}
        node={selectedNode}
        nodes={nodes}
        onChangeNodeGroup={onChangeNodeGroup}
      />
    );
  }

  return (
    <aside className="property-panel">
      <PanelHeader eyebrow="Inspector" title="未选择" />
      <p className="property-empty">选择节点或分组查看属性。</p>
    </aside>
  );
}

function GroupPropertyPanel({
  candidateNodes,
  group,
  isUpdatingMembers,
  memberNodes,
  onAddGroupMember,
  onDeleteGroup,
  onRemoveGroupMember,
  onRenameGroup,
}: {
  candidateNodes: MediaNode[];
  group: MediaGroup;
  isUpdatingMembers: boolean;
  memberNodes: MediaNode[];
  onAddGroupMember: (groupId: string, nodeId: string) => void;
  onDeleteGroup: (groupId: string) => void;
  onRemoveGroupMember: (groupId: string, nodeId: string) => void;
  onRenameGroup: (groupId: string, name: string) => void;
}) {
  return (
    <aside className="property-panel">
      <PanelHeader eyebrow="Group" title={group.name} />
      <label className="property-field">
        <span>名称</span>
        <input
          defaultValue={group.name}
          key={group.id}
          onBlur={(event) => {
            const nextName = event.currentTarget.value.trim();
            if (nextName && nextName !== group.name) {
              onRenameGroup(group.id, nextName);
            }
          }}
        />
      </label>
      <dl className="property-list">
        <div>
          <dt>成员数量</dt>
          <dd>{memberNodes.length}</dd>
        </div>
        <div>
          <dt>排序</dt>
          <dd>{group.sort_order}</dd>
        </div>
      </dl>
      <label className="property-field">
        <span>添加成员</span>
        <select
          disabled={isUpdatingMembers || candidateNodes.length === 0}
          onChange={(event) => {
            const nodeId = event.currentTarget.value;
            if (nodeId) {
              onAddGroupMember(group.id, nodeId);
            }
          }}
          value=""
        >
          <option value="">
            {isUpdatingMembers
              ? "更新中"
              : candidateNodes.length > 0
                ? "选择节点"
                : "没有可添加节点"}
          </option>
          {candidateNodes.map((node) => (
            <option key={node.id} value={node.id}>
              {node.title}
            </option>
          ))}
        </select>
      </label>
      <div className="property-section">
        <p className="studio-section-label">Members</p>
        {memberNodes.length > 0 ? (
          memberNodes.map((node) => (
            <div className="property-row property-row-action" key={node.id}>
              <span>{node.title}</span>
              <button
                onClick={() => onRemoveGroupMember(group.id, node.id)}
                type="button"
              >
                移出
              </button>
            </div>
          ))
        ) : (
          <p className="property-empty">这个分组暂时没有成员。</p>
        )}
      </div>
      <button
        className="studio-secondary-button property-danger"
        onClick={() => onDeleteGroup(group.id)}
        type="button"
      >
        删除分组
      </button>
    </aside>
  );
}

function getGroupCandidateNodes(nodes: MediaNode[], group: MediaGroup) {
  const memberIds = new Set(group.node_ids ?? []);
  return nodes.filter((node) => !memberIds.has(node.id));
}

function NodePropertyPanel({
  edges,
  groups,
  node,
  nodes,
  onChangeNodeGroup,
}: {
  edges: MediaEdge[];
  groups: MediaGroup[];
  node: MediaNode;
  nodes: MediaNode[];
  onChangeNodeGroup: (nodeId: string, groupId: string | null) => void;
}) {
  const group = groups.find((item) => item.id === node.group_id);
  const dependencies = getNodeDependencies(nodes, edges, node.id);

  return (
    <aside className="property-panel">
      <PanelHeader eyebrow={node.node_type} title={node.title} />
      <dl className="property-list">
        <div>
          <dt>状态</dt>
          <dd>{node.status}</dd>
        </div>
      </dl>
      <label className="property-field">
        <span>分组</span>
        <select
          onChange={(event) =>
            onChangeNodeGroup(node.id, event.currentTarget.value || null)
          }
          value={group?.id ?? ""}
        >
          <option value="">未分组</option>
          {groups.map((item) => (
            <option key={item.id} value={item.id}>
              {item.name}
            </option>
          ))}
        </select>
      </label>
      <div className="property-section">
        <p className="studio-section-label">依赖于</p>
        <DependencyRows emptyLabel="暂无上游依赖。" nodes={dependencies.upstream} />
      </div>
      <div className="property-section">
        <p className="studio-section-label">被依赖于</p>
        <DependencyRows
          emptyLabel="暂无下游依赖。"
          nodes={dependencies.downstream}
        />
      </div>
      <div className="property-section">
        <p className="studio-section-label">Prompt</p>
        <p className="property-prompt">{node.prompt || "暂无 prompt"}</p>
      </div>
    </aside>
  );
}

function EdgePropertyPanel({
  detail,
  onDeleteEdge,
}: {
  detail: NonNullable<ReturnType<typeof getEdgeDetail>>;
  onDeleteEdge: (edgeId: string) => void;
}) {
  return (
    <aside className="property-panel">
      <PanelHeader eyebrow="Dependency" title="依赖连线" />
      <div className="property-section">
        <p className="studio-section-label">方向</p>
        <div className="property-flow">
          <NodeChip node={detail.fromNode} />
          <span aria-hidden="true">→</span>
          <NodeChip node={detail.toNode} />
        </div>
        <p className="property-empty">
          {detail.toNode.title} 依赖 {detail.fromNode.title}。
        </p>
      </div>
      <dl className="property-list">
        <div>
          <dt>类型</dt>
          <dd>dependency</dd>
        </div>
        <div>
          <dt>来源</dt>
          <dd>{detail.edge.source}</dd>
        </div>
      </dl>
      <button
        className="studio-secondary-button property-danger"
        onClick={() => onDeleteEdge(detail.edge.id)}
        type="button"
      >
        删除依赖
      </button>
    </aside>
  );
}

function DependencyRows({
  emptyLabel,
  nodes,
}: {
  emptyLabel: string;
  nodes: MediaNode[];
}) {
  if (nodes.length === 0) {
    return <p className="property-empty">{emptyLabel}</p>;
  }
  return nodes.map((node) => (
    <div className="property-row" key={node.id}>
      <span>{node.title}</span>
      <span>{node.node_type}</span>
    </div>
  ));
}

function NodeChip({ node }: { node: MediaNode }) {
  return (
    <span className="property-node-chip">
      <span>{node.title}</span>
      <small>{node.node_type}</small>
    </span>
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
