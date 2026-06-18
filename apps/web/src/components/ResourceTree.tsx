import { useMemo, useState } from "react";
import type { MediaGroup, MediaNode, MediaType } from "../lib/api";
import { getResourceTreeSections } from "../lib/canvasSelectors";

interface ResourceTreeProps {
  nodes: MediaNode[];
  groups: MediaGroup[];
  selectedGroupId: string | null;
  selectedNodeId: string | null;
  onCreateGroup: () => void;
  onSelectGroup: (groupId: string) => void;
  onSelectNode: (nodeId: string) => void;
  onStartConnection: (nodeId: string) => void;
}

const filters: Array<{ value: "all" | MediaType; label: string }> = [
  { value: "all", label: "全部" },
  { value: "text", label: "文本" },
  { value: "image", label: "图片" },
  { value: "video", label: "视频" },
  { value: "audio", label: "音频" },
];

const nodeTypeLabel: Record<MediaType, string> = {
  text: "文案",
  image: "参考",
  video: "视频",
  audio: "音频",
};

export function ResourceTree({
  nodes,
  groups,
  selectedGroupId,
  selectedNodeId,
  onCreateGroup,
  onSelectGroup,
  onSelectNode,
  onStartConnection,
}: ResourceTreeProps) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<"all" | MediaType>("all");
  const [collapsedGroups, setCollapsedGroups] = useState(new Set<string>());

  const sections = useMemo(
    () => getResourceTreeSections(nodes, groups, { query, type: filter }),
    [filter, groups, nodes, query],
  );

  return (
    <div className="resource-tree">
      <div className="studio-action-row">
        <p className="studio-section-label">Resources</p>
        <button
          className="studio-secondary-button resource-tree-create"
          onClick={onCreateGroup}
          type="button"
        >
          新建分组
        </button>
      </div>
      <input
        aria-label="搜索资源"
        className="resource-tree-search"
        onChange={(event) => setQuery(event.target.value)}
        placeholder="搜索资源"
        value={query}
      />
      <div className="resource-tree-filters">
        {filters.map((item) => (
          <button
            data-active={filter === item.value}
            key={item.value}
            onClick={() => setFilter(item.value)}
            type="button"
          >
            {item.label}
          </button>
        ))}
      </div>
      <div className="resource-tree-list">
        {sections.groups.map(({ group, memberCount, nodes: groupNodes }) => {
          const collapsed = collapsedGroups.has(group.id);
          return (
            <section className="resource-tree-group" key={group.id}>
              <div className="resource-tree-group-header">
                <button
                  data-selected={group.id === selectedGroupId}
                  onClick={() => onSelectGroup(group.id)}
                  type="button"
                >
                  {group.name}
                  <span>{memberCount}</span>
                </button>
                <button
                  aria-label={collapsed ? "展开分组" : "收起分组"}
                  onClick={() =>
                    setCollapsedGroups((current) => toggleSet(current, group.id))
                  }
                  type="button"
                >
                  {collapsed ? "+" : "-"}
                </button>
              </div>
              {!collapsed
                ? groupNodes.map((node) => (
                    <ResourceNodeRow
                      key={node.id}
                      node={node}
                      selected={node.id === selectedNodeId}
                      onSelectNode={onSelectNode}
                      onStartConnection={onStartConnection}
                    />
                  ))
                : null}
            </section>
          );
        })}
        <section className="resource-tree-group">
          <p className="resource-tree-ungrouped-label">
            未分组
            <span>{sections.ungroupedNodes.length}</span>
          </p>
          {sections.ungroupedNodes.map((node) => (
            <ResourceNodeRow
              key={node.id}
              node={node}
              selected={node.id === selectedNodeId}
              onSelectNode={onSelectNode}
              onStartConnection={onStartConnection}
            />
          ))}
        </section>
      </div>
    </div>
  );
}

function ResourceNodeRow({
  node,
  selected,
  onSelectNode,
  onStartConnection,
}: {
  node: MediaNode;
  selected: boolean;
  onSelectNode: (nodeId: string) => void;
  onStartConnection: (nodeId: string) => void;
}) {
  return (
    <div className="studio-resource-item" data-selected={selected}>
      <button
        className="studio-resource-select"
        onClick={() => onSelectNode(node.id)}
        type="button"
      >
        <span className="studio-resource-thumb">{nodeTypeLabel[node.node_type]}</span>
        <span className="studio-resource-name">{node.title}</span>
      </button>
      <span className="studio-resource-status" data-status={node.status} />
      <button
        aria-label={`从 ${node.title} 设置连线起点`}
        className="studio-resource-connect"
        onClick={() => onStartConnection(node.id)}
        type="button"
      >
        →
      </button>
    </div>
  );
}

function toggleSet(current: Set<string>, value: string) {
  const next = new Set(current);
  if (next.has(value)) {
    next.delete(value);
  } else {
    next.add(value);
  }
  return next;
}
