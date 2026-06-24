import type { MediaEdge, MediaGroup, MediaNode } from "../../lib/api";
import { materialKindLabel, materialStatusLabel } from "../../lib/sourceMaterial";
import type { CanvasFlowMode } from "./flowTypes";
import type { CanvasFlowPolicy } from "./flowModePolicy";

export interface NodeInspectorPopoverProps {
  mode: CanvasFlowMode;
  policy: CanvasFlowPolicy;
  node: MediaNode;
  edges: MediaEdge[];
  groups: MediaGroup[];
  onRunNode: (node: MediaNode) => void;
  onUpdateNode: (nodeId: string, patch: unknown) => void;
}

export function NodeInspectorPopover({
  mode,
  policy,
  node,
  edges,
  groups,
  onRunNode,
  onUpdateNode,
}: NodeInspectorPopoverProps) {
  const upstreamCount = edges.filter((edge) => edge.to_node_id === node.id).length;
  const downstreamCount = edges.filter(
    (edge) => edge.from_node_id === node.id,
  ).length;
  const group = groups.find((item) => item.id === node.group_id);
  const statusLabel = materialStatusLabel(node) || node.status;

  return (
    <aside
      className="node-inspector-popover canvas-flow-inspector nodrag"
      data-mode={mode}
      data-node-id={node.id}
    >
      <header className="canvas-flow-inspector-header">
        <span>{materialKindLabel(node)}</span>
        <strong>{node.title || "未命名节点"}</strong>
      </header>
      <dl className="canvas-flow-inspector-facts">
        <div>
          <dt>状态</dt>
          <dd>{statusLabel}</dd>
        </div>
        <div>
          <dt>分组</dt>
          <dd>{group?.name ?? "未分组"}</dd>
        </div>
        <div>
          <dt>依赖</dt>
          <dd>
            {upstreamCount} 入 / {downstreamCount} 出
          </dd>
        </div>
        <div>
          <dt>版本</dt>
          <dd>{node.production_preview?.version_no ?? "暂无"}</dd>
        </div>
        <div>
          <dt>Reference Pack</dt>
          <dd>{node.reference_pack_preview?.member_count ?? 0} 个成员</dd>
        </div>
      </dl>
      <div className="canvas-flow-inspector-actions">
        <button
          disabled={!policy.canRunNodes}
          onClick={() => onRunNode(node)}
          type="button"
        >
          运行
        </button>
        <button
          disabled={!policy.canEditNodeContent}
          onClick={() => onUpdateNode(node.id, { title: node.title })}
          type="button"
        >
          编辑
        </button>
      </div>
    </aside>
  );
}
