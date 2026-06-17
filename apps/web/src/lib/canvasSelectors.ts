import type { MediaEdge, MediaGroup, MediaNode, MediaType } from "./api";

export interface NodeDependencies {
  downstream: MediaNode[];
  upstream: MediaNode[];
}

export interface EdgeDetail {
  edge: MediaEdge;
  fromNode: MediaNode;
  toNode: MediaNode;
}

export interface ResourceTreeGroupSection {
  group: MediaGroup;
  memberCount: number;
  nodes: MediaNode[];
}

export interface ResourceTreeSections {
  groups: ResourceTreeGroupSection[];
  ungroupedNodes: MediaNode[];
}

export interface ResourceTreeFilter {
  query: string;
  type: "all" | MediaType;
}

export function getNodeDependencies(
  nodes: MediaNode[],
  edges: MediaEdge[],
  nodeId: string,
): NodeDependencies {
  const nodeById = mapNodesById(nodes);
  return {
    upstream: edges
      .filter((edge) => edge.to_node_id === nodeId)
      .map((edge) => nodeById.get(edge.from_node_id))
      .filter(isMediaNode),
    downstream: edges
      .filter((edge) => edge.from_node_id === nodeId)
      .map((edge) => nodeById.get(edge.to_node_id))
      .filter(isMediaNode),
  };
}

export function getEdgeDetail(
  nodes: MediaNode[],
  edges: MediaEdge[],
  edgeId: string | null,
): EdgeDetail | null {
  if (!edgeId) {
    return null;
  }
  const edge = edges.find((item) => item.id === edgeId);
  if (!edge) {
    return null;
  }
  const nodeById = mapNodesById(nodes);
  const fromNode = nodeById.get(edge.from_node_id);
  const toNode = nodeById.get(edge.to_node_id);
  if (!fromNode || !toNode) {
    return null;
  }
  return { edge, fromNode, toNode };
}

export function getGroupMembers(
  nodes: MediaNode[],
  group: MediaGroup,
): MediaNode[] {
  const memberIds = new Set(nodeIdsForGroup(group));
  return nodes.filter((node) => memberIds.has(node.id));
}

export function getUngroupedNodes(
  nodes: MediaNode[],
  groups: MediaGroup[],
): MediaNode[] {
  const groupedIds = new Set(groups.flatMap(nodeIdsForGroup));
  return nodes.filter((node) => !groupedIds.has(node.id));
}

export function nodeIdsWithout(
  nodeIds: string[] | null | undefined,
  nodeId: string,
): string[] {
  return (nodeIds ?? []).filter((item) => item !== nodeId);
}

export function nodeIdsWith(
  nodeIds: string[] | null | undefined,
  nodeId: string,
): string[] {
  const ids = nodeIds ?? [];
  return ids.includes(nodeId) ? ids : [...ids, nodeId];
}

export function getResourceTreeSections(
  nodes: MediaNode[],
  groups: MediaGroup[],
  filter: ResourceTreeFilter,
): ResourceTreeSections {
  const query = filter.query.trim().toLowerCase();
  const matchesType = (node: MediaNode) =>
    filter.type === "all" || node.node_type === filter.type;
  const matchesNodeQuery = (node: MediaNode) =>
    query === "" || node.title.toLowerCase().includes(query);

  const groupSections = groups
    .map((group) => {
      const groupMatchesQuery =
        query !== "" && group.name.toLowerCase().includes(query);
      const memberIds = nodeIdsForGroup(group);
      const groupNodeIds = new Set(memberIds);
      const groupNodes = nodes.filter((node) => {
        if (!groupNodeIds.has(node.id) || !matchesType(node)) {
          return false;
        }
        return groupMatchesQuery || matchesNodeQuery(node);
      });
      return {
        group,
        matchesQuery: groupMatchesQuery,
        memberCount: memberIds.length,
        nodes: groupNodes,
      };
    })
    .filter(
      (section) =>
        query === "" || section.matchesQuery || section.nodes.length > 0,
    )
    .map(({ group, memberCount, nodes }) => ({ group, memberCount, nodes }));

  const groupedIds = new Set(groups.flatMap(nodeIdsForGroup));
  const ungroupedNodes = nodes.filter(
    (node) =>
      !groupedIds.has(node.id) && matchesType(node) && matchesNodeQuery(node),
  );

  return {
    groups: groupSections,
    ungroupedNodes,
  };
}

function nodeIdsForGroup(group: MediaGroup): string[] {
  return Array.isArray(group.node_ids) ? group.node_ids : [];
}

function mapNodesById(nodes: MediaNode[]) {
  return new Map(nodes.map((node) => [node.id, node]));
}

function isMediaNode(node: MediaNode | undefined): node is MediaNode {
  return Boolean(node);
}
