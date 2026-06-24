import {
  type PointerEvent as ReactPointerEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import type {
  ArtifactVersion,
  MediaEdge,
  MediaGroup,
  MediaNode,
  ModelCapability,
  NodeProductionState,
  ReferencePackItem,
  updateMediaNode,
} from "../lib/api";
import {
  getEdgeDetail,
  getGroupMembers,
} from "../lib/canvasSelectors";
import {
  capabilitiesForNode,
  capabilityKey,
  defaultOperationForNode,
  modelParamsForCapability,
  productionConfigPatchForOperation,
  productionConfigPatchForRun,
  productionConfigPatchForSelectedModel,
  retryDisabledReason,
  runDisabledReason,
  selectedCapabilityKey,
  simplifiedOperationOptionsForNode,
} from "../lib/productionPanel";
import {
  staleReasonText,
  versionCallRecordBlocks,
  versionDetailRows,
  versionHasCallRecord,
  versionRows,
} from "../lib/productionPreview";
import { openArtifactVersionInNewTab } from "../lib/artifactViewer";
import {
  isManualTextMaterialNode,
  isSourceMaterialNode,
  materialKindLabel,
} from "../lib/sourceMaterial";
import {
  candidateReferencePackMembers,
  memberIdsAfterToggle,
  memberNodesForPack,
} from "../lib/referencePack";
import {
  buildPromptRefDocument,
  candidatePromptRefNodes,
  collectPromptRefMentions,
  inputReferenceState,
} from "../lib/promptRefs";
import { MarkdownPreview } from "./MarkdownPreview";

type NodePatch = Parameters<typeof updateMediaNode>[1];

interface PropertyPanelProps {
  edges: MediaEdge[];
  groups: MediaGroup[];
  nodes: MediaNode[];
  selectedEdgeId: string | null;
  selectedGroupId: string | null;
  selectedNodeId: string | null;
  isModelCapabilitiesLoading: boolean;
  isProductionStateLoading: boolean;
  isReferencePackItemsLoading: boolean;
  isRetryingJob: boolean;
  isRunningNode: boolean;
  isSelectingVersion: boolean;
  isUpdatingReferencePackItems: boolean;
  isUpdatingGroupMembers: boolean;
  isUpdatingNode: boolean;
  modelCapabilities: ModelCapability[];
  nodeProductionState: NodeProductionState | null;
  referencePackItems: ReferencePackItem[];
  readOnly?: boolean;
  onAddGroupMember?: (groupId: string, nodeId: string) => void;
  onDeleteEdge?: (edgeId: string) => void;
  onDeleteGroup?: (groupId: string) => void;
  onDeleteInputEdge?: (edgeId: string) => void;
  onRemoveGroupMember?: (groupId: string, nodeId: string) => void;
  onRenameGroup?: (groupId: string, name: string) => void;
  onReplaceReferencePackItems: (
    packNodeId: string,
    memberNodeIds: string[],
  ) => void;
  onPromptRefSelect: (
    targetNode: MediaNode,
    refNode: MediaNode,
    prompt: string,
  ) => void;
  onRetryJob: (jobId: string) => void;
  onRunNode: (nodeId: string, patch?: NodePatch) => void;
  onSelectVersion: (nodeId: string, versionId: string) => void;
  onUpdateNode: (nodeId: string, patch: NodePatch) => void;
}

const nodeTypeLabel: Record<MediaNode["node_type"], string> = {
  text: "文本",
  image: "图片",
  video: "视频",
  audio: "音频",
  reference_pack: "参考包",
};

const statusLabel: Record<MediaNode["status"], string> = {
  draft: "草稿",
  ready: "就绪",
  queued: "排队",
  running: "运行中",
  succeeded: "完成",
  failed: "失败",
  stale: "需更新",
  user_editing: "编辑中",
};

export function PropertyPanel({
  edges,
  groups,
  nodes,
  selectedEdgeId,
  selectedGroupId,
  selectedNodeId,
  isModelCapabilitiesLoading,
  isProductionStateLoading,
  isReferencePackItemsLoading,
  isRetryingJob,
  isRunningNode,
  isSelectingVersion,
  isUpdatingReferencePackItems,
  isUpdatingGroupMembers,
  isUpdatingNode,
  modelCapabilities,
  nodeProductionState,
  referencePackItems,
  readOnly = false,
  onAddGroupMember,
  onDeleteEdge,
  onDeleteGroup,
  onDeleteInputEdge,
  onRemoveGroupMember,
  onRenameGroup,
  onReplaceReferencePackItems,
  onPromptRefSelect,
  onRetryJob,
  onRunNode,
  onSelectVersion,
  onUpdateNode,
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
    if (isSourceMaterialNode(selectedNode)) {
      return (
        <SourceMaterialPanel
          isUpdatingNode={isUpdatingNode}
          node={selectedNode}
          readOnly={readOnly}
          onUpdateNode={onUpdateNode}
        />
      );
    }
    return (
      <NodePropertyPanel
        edges={edges}
        isModelCapabilitiesLoading={isModelCapabilitiesLoading}
        isProductionStateLoading={isProductionStateLoading}
        isReferencePackItemsLoading={isReferencePackItemsLoading}
        isRetryingJob={isRetryingJob}
        isRunningNode={isRunningNode}
        isSelectingVersion={isSelectingVersion}
        isUpdatingReferencePackItems={isUpdatingReferencePackItems}
        isUpdatingNode={isUpdatingNode}
        modelCapabilities={modelCapabilities}
        node={selectedNode}
        nodeProductionState={nodeProductionState}
        nodes={nodes}
        referencePackItems={referencePackItems}
        readOnly={readOnly}
        onReplaceReferencePackItems={onReplaceReferencePackItems}
        onPromptRefSelect={onPromptRefSelect}
        onDeleteInputEdge={onDeleteInputEdge ?? onDeleteEdge}
        onRetryJob={onRetryJob}
        onRunNode={onRunNode}
        onSelectVersion={onSelectVersion}
        onUpdateNode={onUpdateNode}
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
  onAddGroupMember?: (groupId: string, nodeId: string) => void;
  onDeleteGroup?: (groupId: string) => void;
  onRemoveGroupMember?: (groupId: string, nodeId: string) => void;
  onRenameGroup?: (groupId: string, name: string) => void;
}) {
  const commitGroupRename = useCallback(
    (input: HTMLInputElement) => {
      const nextName = input.value.trim();
      if (nextName && nextName !== group.name) {
        onRenameGroup?.(group.id, nextName);
      }
    },
    [group.id, group.name, onRenameGroup],
  );

  return (
    <aside className="property-panel">
      <PanelHeader eyebrow="Group" title={group.name} />
      {onRenameGroup ? (
        <label className="property-field">
          <span>名称</span>
          <input
            defaultValue={group.name}
            key={group.id}
            onBlur={(event) => commitGroupRename(event.currentTarget)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                event.currentTarget.blur();
              }
              if (event.key === "Escape") {
                event.preventDefault();
                event.currentTarget.value = group.name;
                event.currentTarget.blur();
              }
            }}
          />
        </label>
      ) : null}
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
      {onAddGroupMember ? (
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
      ) : null}
      <div className="property-section">
        <p className="studio-section-label">Members</p>
        {memberNodes.length > 0 ? (
          memberNodes.map((node) => (
            <div className="property-row property-row-action" key={node.id}>
              <span>{node.title}</span>
              {onRemoveGroupMember ? (
                <button
                  onClick={() => onRemoveGroupMember(group.id, node.id)}
                  type="button"
                >
                  移出
                </button>
              ) : null}
            </div>
          ))
        ) : (
          <p className="property-empty">这个分组暂时没有成员。</p>
        )}
      </div>
      {onDeleteGroup ? (
        <button
          className="studio-secondary-button property-danger"
          onClick={() => onDeleteGroup(group.id)}
          type="button"
        >
          删除分组
        </button>
      ) : null}
    </aside>
  );
}

function getGroupCandidateNodes(nodes: MediaNode[], group: MediaGroup) {
  const memberIds = new Set(group.node_ids ?? []);
  return nodes.filter((node) => !memberIds.has(node.id));
}

function SourceMaterialPanel({
  isUpdatingNode,
  node,
  readOnly,
  onUpdateNode,
}: {
  isUpdatingNode: boolean;
  node: MediaNode;
  readOnly: boolean;
  onUpdateNode: (nodeId: string, patch: NodePatch) => void;
}) {
  const [titleValue, setTitleValue] = useState(node.title);
  const [isTitleEditing, setIsTitleEditing] = useState(false);
  const [contentValue, setContentValue] = useState(node.prompt);
  const isManualTextMaterial = isManualTextMaterialNode(node);
  const titleValueRef = useRef(titleValue);
  const nodeTitleRef = useRef(node.title);
  const contentValueRef = useRef(contentValue);
  const nodePromptRef = useRef(node.prompt);
  const onUpdateNodeRef = useRef(onUpdateNode);

  useEffect(() => {
    onUpdateNodeRef.current = onUpdateNode;
  }, [onUpdateNode]);

  useEffect(() => {
    setTitleValue(node.title);
    titleValueRef.current = node.title;
    nodeTitleRef.current = node.title;
    setContentValue(node.prompt);
    contentValueRef.current = node.prompt;
    nodePromptRef.current = node.prompt;
    setIsTitleEditing(false);
  }, [node.id, node.title, node.prompt]);

  const commitTitle = useCallback(() => {
    if (readOnly) {
      setIsTitleEditing(false);
      return;
    }
    const title = titleValueRef.current.trim();
    if (title && title !== nodeTitleRef.current) {
      onUpdateNodeRef.current(node.id, { title });
      nodeTitleRef.current = title;
    }
    setTitleValue(title || nodeTitleRef.current);
    setIsTitleEditing(false);
  }, [node.id, readOnly]);

  const cancelTitleEdit = useCallback(() => {
    titleValueRef.current = nodeTitleRef.current;
    setTitleValue(nodeTitleRef.current);
    setIsTitleEditing(false);
  }, []);

  const commitContent = useCallback(() => {
    if (readOnly) {
      return;
    }
    const content = contentValueRef.current;
    if (content === nodePromptRef.current) {
      return;
    }
    onUpdateNodeRef.current(node.id, {
      prompt: content,
      prompt_rich: {
        version: 1,
        source: "textarea-at",
        text: content,
      },
    });
    nodePromptRef.current = content;
  }, [node.id, readOnly]);

  useEffect(() => {
    return () => {
      commitTitle();
      if (isManualTextMaterial) {
        commitContent();
      }
    };
  }, [commitContent, commitTitle, isManualTextMaterial]);

  const previewVersion = buildSourceMaterialPreviewVersion(node);

  return (
    <aside className="property-panel node-production-panel node-composer-panel node-composer-source-panel">
      <div className="property-panel-title-row">
        {readOnly ? (
          <PanelHeader eyebrow={materialKindLabel(node)} title={titleValue} />
        ) : (
          <EditablePanelHeader
            eyebrow={materialKindLabel(node)}
            isEditing={isTitleEditing}
            isUpdating={isUpdatingNode}
            onCancel={cancelTitleEdit}
            onCommit={commitTitle}
            onEdit={() => setIsTitleEditing(true)}
            onTitleChange={(next) => {
              titleValueRef.current = next;
              setTitleValue(next);
            }}
            title={titleValue}
          />
        )}
        <span className="property-status-pill" data-status={node.status}>
          可用
        </span>
      </div>
      {isManualTextMaterial ? (
        <label className="property-field property-content-field node-composer-prompt source-material-content-field">
          <span>素材内容</span>
          <textarea
            disabled={isUpdatingNode || readOnly}
            onBlur={commitContent}
            onChange={(event) => {
              const next = event.currentTarget.value;
              contentValueRef.current = next;
              setContentValue(next);
            }}
            placeholder="粘贴视频脚本、商品卖点或参考文案"
            rows={12}
            value={contentValue}
          />
        </label>
      ) : (
        <div className="property-section property-source-preview source-material-preview-card">
          <VersionPreviewBody version={previewVersion} />
        </div>
      )}
      <p className="source-material-readonly-note">
        这是用户素材节点，可作为依赖输入或加入参考包，不需要运行模型。
      </p>
    </aside>
  );
}

function buildSourceMaterialPreviewVersion(node: MediaNode): ArtifactVersion {
  const accessUrl = node.asset_url ?? node.production_preview?.access_url;
  const assetType = node.node_type === "reference_pack" ? "json" : node.node_type;
  return {
    id: node.asset_id ?? node.id,
    workspace_id: node.workspace_id,
    node_id: node.id,
    asset_id: node.asset_id ?? undefined,
    version_no: 1,
    winner: true,
    output: {},
    input_hash: "",
    status: node.status === "failed" ? "failed" : "succeeded",
    progress: 100,
    provider_request: {},
    provider_response: {},
    asset: {
      id: node.asset_id ?? node.id,
      type: assetType,
      mime: node.production_preview?.mime ?? "",
      access_url: accessUrl,
      text_content: node.node_type === "text" ? node.prompt : undefined,
      metadata: {},
    },
    created_at: node.created_at,
  };
}

function NodePropertyPanel({
  edges,
  isModelCapabilitiesLoading,
  isProductionStateLoading,
  isReferencePackItemsLoading,
  isRetryingJob,
  isRunningNode,
  isSelectingVersion,
  isUpdatingReferencePackItems,
  isUpdatingNode,
  modelCapabilities,
  node,
  nodeProductionState,
  nodes,
  referencePackItems,
  readOnly,
  onReplaceReferencePackItems,
  onPromptRefSelect,
  onDeleteInputEdge,
  onRetryJob,
  onRunNode,
  onSelectVersion,
  onUpdateNode,
}: {
  edges: MediaEdge[];
  isModelCapabilitiesLoading: boolean;
  isProductionStateLoading: boolean;
  isReferencePackItemsLoading: boolean;
  isRetryingJob: boolean;
  isRunningNode: boolean;
  isSelectingVersion: boolean;
  isUpdatingReferencePackItems: boolean;
  isUpdatingNode: boolean;
  modelCapabilities: ModelCapability[];
  node: MediaNode;
  nodeProductionState: NodeProductionState | null;
  nodes: MediaNode[];
  referencePackItems: ReferencePackItem[];
  readOnly: boolean;
  onReplaceReferencePackItems: (
    packNodeId: string,
    memberNodeIds: string[],
  ) => void;
  onPromptRefSelect: (
    targetNode: MediaNode,
    refNode: MediaNode,
    prompt: string,
  ) => void;
  onDeleteInputEdge?: (edgeId: string) => void;
  onRetryJob: (jobId: string) => void;
  onRunNode: (nodeId: string, patch?: NodePatch) => void;
  onSelectVersion: (nodeId: string, versionId: string) => void;
  onUpdateNode: (nodeId: string, patch: NodePatch) => void;
}) {
  const refState = inputReferenceState(node, nodes, edges);
  const isReferencePack = node.node_type === "reference_pack";
  const operation = defaultOperationForNode(node);
  const operationOptions = simplifiedOperationOptionsForNode(node);
  const compatibleCapabilities = capabilitiesForNode(
    node,
    modelCapabilities,
    operation,
  );
  const selectedModelKey = selectedCapabilityKey(node, modelCapabilities);
  const selectedCapability = compatibleCapabilities.find(
    (capability) => capabilityKey(capability) === selectedModelKey,
  );
  const modelParams = modelParamsForCapability(node, selectedCapability);
  const disabledReason = runDisabledReason(
    node,
    nodeProductionState,
    modelCapabilities,
    refState,
  );
  const latestJob = nodeProductionState?.latest_job ?? null;
  const versions = nodeProductionState?.versions ?? [];
  const currentVersion =
    nodeProductionState?.current_version ??
    versions.find((version) => version.winner) ??
    null;
  const retryReason = retryDisabledReason(latestJob);
  const durations = durationOptions(selectedCapability);
  const hasDurationParam = "duration_sec" in modelParams;
  const hasTemperatureParam = "temperature" in modelParams;
  const [previewVersionId, setPreviewVersionId] = useState<string | null>(null);
  const [detailVersionId, setDetailVersionId] = useState<string | null>(null);
  const [isMoreOpen, setIsMoreOpen] = useState(false);
  const [detailsPopoverPosition, setDetailsPopoverPosition] = useState<{
    left: number;
    top: number;
  } | null>(null);
  const [openToolbarDropdown, setOpenToolbarDropdown] = useState<
    "operation" | "model" | "duration" | null
  >(null);
  const moreButtonRef = useRef<HTMLButtonElement | null>(null);
  const detailsDragRef = useRef<{
    pointerId: number;
    startClientX: number;
    startClientY: number;
    startLeft: number;
    startTop: number;
  } | null>(null);
  const previewVersion =
    (previewVersionId
      ? versions.find((version) => version.id === previewVersionId)
      : null) ??
    currentVersion ??
    versions[0] ??
    null;

  useEffect(() => {
    setPreviewVersionId(null);
    setDetailVersionId(null);
    setIsMoreOpen(false);
    setDetailsPopoverPosition(null);
    setOpenToolbarDropdown(null);
  }, [node.id]);

  useEffect(() => {
    if (
      previewVersionId &&
      !versions.some((version) => version.id === previewVersionId)
    ) {
      setPreviewVersionId(null);
    }
  }, [previewVersionId, versions]);

  const detailVersion =
    (detailVersionId
      ? versions.find((version) => version.id === detailVersionId)
      : null) ?? null;

  const openDetailsPopover = useCallback(() => {
    const triggerRect = moreButtonRef.current?.getBoundingClientRect();
    const width = Math.min(520, window.innerWidth - 32);
    const height = Math.min(520, window.innerHeight - 32);
    const fallbackLeft = window.innerWidth - width - 16;
    const fallbackTop = window.innerHeight - height - 16;
    const left = triggerRect
      ? triggerRect.right - width
      : fallbackLeft;
    const top = triggerRect
      ? triggerRect.top - height - 8
      : fallbackTop;
    setDetailsPopoverPosition({
      left: Math.round(clamp(left, 16, Math.max(16, window.innerWidth - width - 16))),
      top: Math.round(clamp(top, 16, Math.max(16, window.innerHeight - height - 16))),
    });
    setIsMoreOpen(true);
    setOpenToolbarDropdown(null);
  }, []);

  const beginDetailsDrag = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      if (event.button !== 0 || !detailsPopoverPosition) {
        return;
      }
      event.currentTarget.setPointerCapture(event.pointerId);
      detailsDragRef.current = {
        pointerId: event.pointerId,
        startClientX: event.clientX,
        startClientY: event.clientY,
        startLeft: detailsPopoverPosition.left,
        startTop: detailsPopoverPosition.top,
      };
    },
    [detailsPopoverPosition],
  );

  const moveDetailsDrag = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      const drag = detailsDragRef.current;
      if (!drag || drag.pointerId !== event.pointerId) {
        return;
      }
      const width = Math.min(520, window.innerWidth - 32);
      const height = Math.min(520, window.innerHeight - 32);
      setDetailsPopoverPosition({
        left: Math.round(
          clamp(
            drag.startLeft + event.clientX - drag.startClientX,
            16,
            Math.max(16, window.innerWidth - width - 16),
          ),
        ),
        top: Math.round(
          clamp(
            drag.startTop + event.clientY - drag.startClientY,
            16,
            Math.max(16, window.innerHeight - height - 16),
          ),
        ),
      });
    },
    [],
  );

  const endDetailsDrag = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      const drag = detailsDragRef.current;
      if (!drag || drag.pointerId !== event.pointerId) {
        return;
      }
      detailsDragRef.current = null;
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId);
      }
    },
    [],
  );

  if (isReferencePack) {
    return (
      <aside className="property-panel node-production-panel">
        <div className="property-panel-title-row">
          <PanelHeader
            eyebrow={nodeTypeLabel[node.node_type]}
            title={node.title || "未命名参考包"}
          />
          <span className="property-status-pill" data-status={node.status}>
            {statusLabel[node.status]}
          </span>
        </div>
        <div className="property-section">
          <p className="studio-section-label">Reference Pack</p>
          <p className="property-empty">
            Reference Pack 管理已有节点的直接成员；成员关系不会改变分组，也不会自动包含成员的上游依赖。
          </p>
        </div>
        <ReferencePackMembersSection
          candidates={candidateReferencePackMembers(
            node,
            nodes,
            referencePackItems,
          )}
          isLoading={isReferencePackItemsLoading}
          isUpdating={isUpdatingReferencePackItems || readOnly}
          members={memberNodesForPack(referencePackItems, nodes)}
          onToggleMember={(memberNodeId, checked) => {
            if (readOnly) {
              return;
            }
            onReplaceReferencePackItems(
              node.id,
              memberIdsAfterToggle(referencePackItems, memberNodeId, checked),
            );
          }}
        />
      </aside>
    );
  }

  return (
    <aside className="property-panel node-production-panel node-composer-panel">
      <div className="node-composer-inputs">
          <NodeInputStrip
            edges={edges}
            node={node}
            nodes={nodes}
            onDeleteInputEdge={readOnly ? undefined : onDeleteInputEdge}
          />
      </div>
      <PropertyPromptEditor
        className="node-composer-prompt"
        edges={edges}
        isUpdatingNode={isUpdatingNode || readOnly}
        node={node}
        nodes={nodes}
        onPromptRefSelect={onPromptRefSelect}
        onUpdateNode={onUpdateNode}
      />
      <div className="node-composer-toolbar">
        <div className="node-composer-toolbar-controls">
          <NodeComposerDropdown
            ariaLabel="选择生成任务"
            className="node-composer-operation-select"
            disabled={isUpdatingNode || readOnly}
            isOpen={openToolbarDropdown === "operation"}
            onSelect={(value) => {
              setOpenToolbarDropdown(null);
              if (readOnly) {
                return;
              }
              onUpdateNode(
                node.id,
                productionConfigPatchForOperation(
                  node,
                  modelCapabilities,
                  value,
                ),
              );
            }}
            onToggle={() => {
              setOpenToolbarDropdown((current) =>
                current === "operation" ? null : "operation",
              );
              setIsMoreOpen(false);
            }}
            onDismiss={() => setOpenToolbarDropdown(null)}
            options={operationOptions}
            value={operation}
          />
          <NodeComposerDropdown
            ariaLabel="选择模型"
            className="node-composer-model-select"
            disabled={
              isModelCapabilitiesLoading ||
              isUpdatingNode ||
              readOnly ||
              compatibleCapabilities.length === 0
            }
            isOpen={openToolbarDropdown === "model"}
            onSelect={(value) => {
              setOpenToolbarDropdown(null);
              if (readOnly) {
                return;
              }
              onUpdateNode(
                node.id,
                productionConfigPatchForSelectedModel(
                  node,
                  modelCapabilities,
                  value,
                ),
              );
            }}
            onToggle={() => {
              setOpenToolbarDropdown((current) =>
                current === "model" ? null : "model",
              );
              setIsMoreOpen(false);
            }}
            onDismiss={() => setOpenToolbarDropdown(null)}
            options={
              compatibleCapabilities.length > 0
                ? compatibleCapabilities.map((capability) => ({
                    value: capabilityKey(capability),
                    label: capability.display_name,
                  }))
                : [{ value: "", label: "没有兼容模型" }]
            }
            value={selectedModelKey}
          />
          {hasDurationParam ? (
            <NodeComposerDropdown
              ariaLabel="选择时长"
              className="node-composer-duration-select"
              disabled={isUpdatingNode || readOnly}
              isOpen={openToolbarDropdown === "duration"}
              labelPrefix="时长"
              onSelect={(value) => {
                setOpenToolbarDropdown(null);
                if (readOnly) {
                  return;
                }
                onUpdateNode(node.id, {
                  model_params: {
                    ...modelParams,
                    duration_sec: Number(value),
                  },
                });
              }}
              onToggle={() => {
                setOpenToolbarDropdown((current) =>
                  current === "duration" ? null : "duration",
                );
                setIsMoreOpen(false);
              }}
              onDismiss={() => setOpenToolbarDropdown(null)}
              options={durationControlOptions(
                durations,
                Number(modelParams.duration_sec),
              )}
              value={String(modelParams.duration_sec)}
            />
          ) : null}
          {hasTemperatureParam ? (
            <label className="node-composer-param-control node-composer-temperature-control">
              <span>温度</span>
              <input
                aria-label="设置温度"
                defaultValue={String(modelParams.temperature)}
                disabled={isUpdatingNode || readOnly}
                key={`${node.id}-${modelParams.temperature}`}
                max="1"
                min="0"
                onBlur={(event) => {
                  if (readOnly) {
                    return;
                  }
                  onUpdateNode(node.id, {
                    model_params: {
                      ...modelParams,
                      temperature: Number(event.currentTarget.value),
                    },
                  })
                }}
                step="0.1"
                type="number"
              />
            </label>
          ) : null}
        </div>
        <div className="node-composer-actions">
          <button
            className="node-composer-more-button"
            ref={moreButtonRef}
            onClick={() => {
              if (isMoreOpen) {
                setIsMoreOpen(false);
                return;
              }
              openDetailsPopover();
            }}
            type="button"
          >
            更多
          </button>
          {latestJob?.status === "failed" ? (
            <button
              className="node-composer-secondary-button"
              disabled={isRetryingJob || Boolean(retryReason) || readOnly}
              onClick={() => {
                if (!readOnly) {
                  onRetryJob(latestJob.id);
                }
              }}
              type="button"
            >
              {isRetryingJob ? "重试中" : "重试"}
            </button>
          ) : null}
          <button
            className="node-composer-run-button"
            disabled={
              Boolean(disabledReason) || isRunningNode || isUpdatingNode || readOnly
            }
            onClick={() => {
              if (readOnly) {
                return;
              }
              onRunNode(
                node.id,
                productionConfigPatchForRun(node, modelCapabilities) ??
                  undefined,
              );
            }}
            type="button"
          >
            {isRunningNode ? "…" : "↑"}
          </button>
        </div>
      </div>
      {disabledReason || (latestJob?.status === "failed" && retryReason) ? (
        <p className="node-composer-run-note">
          {disabledReason || retryReason}
        </p>
      ) : null}
      {isMoreOpen ? (
        <div
          aria-label="Versions 与诊断"
          className="node-composer-details-popover"
          role="dialog"
          style={
            detailsPopoverPosition
              ? {
                  left: detailsPopoverPosition.left,
                  top: detailsPopoverPosition.top,
                }
              : undefined
          }
        >
          <div
            className="node-composer-details-header"
            onPointerCancel={endDetailsDrag}
            onPointerDown={beginDetailsDrag}
            onPointerMove={moveDetailsDrag}
            onPointerUp={endDetailsDrag}
          >
            <strong>Versions 与诊断</strong>
            <button
              aria-label="关闭 Versions 与诊断"
              onClick={() => setIsMoreOpen(false)}
              onPointerDown={(event) => event.stopPropagation()}
              type="button"
            >
              ×
            </button>
          </div>
          <div className="property-section property-version-section">
            <div className="property-section-heading">
              <span>Versions</span>
              {previewVersion &&
              currentVersion &&
              previewVersion.id !== currentVersion.id ? (
                <button
                  onClick={() => setPreviewVersionId(null)}
                  type="button"
                >
                  回到当前
                </button>
              ) : null}
            </div>
            {isProductionStateLoading ? (
              <p className="property-empty">正在读取 production state。</p>
            ) : versions.length ? (
              <>
                <VersionPreviewPanel
                  currentVersionId={currentVersion?.id ?? null}
                  detailVersionId={detailVersionId}
                  isSelectingVersion={isSelectingVersion || readOnly}
                  nodeId={node.id}
                  onOpenDetails={(versionId) =>
                    setDetailVersionId((current) =>
                      current === versionId ? null : versionId,
                    )
                  }
                  onSelectVersion={(nodeId, versionId) => {
                    if (!readOnly) {
                      onSelectVersion(nodeId, versionId);
                    }
                  }}
                  version={previewVersion}
                />
                {detailVersion ? (
                  <VersionDetailPanel version={detailVersion} />
                ) : null}
                <div className="property-version-list">
                  {versionRows(versions).map((version) => (
                    <button
                      className="property-version-row"
                      data-active={previewVersion?.id === version.id}
                      data-status={version.status}
                      key={version.id}
                      onClick={() => setPreviewVersionId(version.id)}
                      type="button"
                    >
                      <div>
                        <strong>{version.versionLabel}</strong>
                        <span>{version.statusLabel}</span>
                      </div>
                      <div>
                        <span>{version.assetLabel}</span>
                        <small>{version.assetDetail}</small>
                      </div>
                      <span>{version.inputHash}</span>
                      {version.progressLabel ? (
                        <span>{version.progressLabel}</span>
                      ) : null}
                    </button>
                  ))}
                </div>
              </>
            ) : (
              <p className="property-empty">尚无 artifact version。</p>
            )}
          </div>
          {node.production_preview?.text ? (
            <div className="property-section property-output-section">
              <p className="studio-section-label">Output</p>
              <MarkdownPreview
                value={node.production_preview.text}
                variant="panel"
              />
            </div>
          ) : null}
          <div className="property-section property-details">
            <p className="studio-section-label">Stale Reasons</p>
            {nodeProductionState?.active_stale_reasons.length ? (
              <ul className="property-stale-list">
                {nodeProductionState.active_stale_reasons.map((reason) => (
                  <li key={reason.id}>{staleReasonText(reason)}</li>
                ))}
              </ul>
            ) : (
              <p className="property-empty">当前节点没有 active stale reason。</p>
            )}
          </div>
        </div>
      ) : null}
    </aside>
  );
}

function NodeComposerDropdown({
  ariaLabel,
  className,
  disabled,
  isOpen,
  labelPrefix,
  options,
  value,
  onDismiss,
  onSelect,
  onToggle,
}: {
  ariaLabel: string;
  className: string;
  disabled: boolean;
  isOpen: boolean;
  labelPrefix?: string;
  options: Array<{ value: string; label: string }>;
  value: string;
  onDismiss: () => void;
  onSelect: (value: string) => void;
  onToggle: () => void;
}) {
  const selectedOption =
    options.find((option) => option.value === value) ?? options[0] ?? null;
  const dropdownRootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!isOpen) {
      return;
    }
    const handleDocumentPointerDown = (event: globalThis.PointerEvent) => {
      if (
        event.target instanceof Node &&
        dropdownRootRef.current &&
        dropdownRootRef.current.contains(event.target)
      ) {
        return;
      }
      onDismiss();
    };
    document.addEventListener("pointerdown", handleDocumentPointerDown, true);
    return () => {
      document.removeEventListener(
        "pointerdown",
        handleDocumentPointerDown,
        true,
      );
    };
  }, [isOpen, onDismiss]);

  return (
    <div
      className={`node-composer-dropdown node-composer-dropdown-root ${className}`}
      ref={dropdownRootRef}
    >
      <button
        aria-expanded={isOpen}
        aria-haspopup="listbox"
        aria-label={ariaLabel}
        className="node-composer-dropdown-button"
        disabled={disabled}
        onClick={onToggle}
        type="button"
      >
        <span className="node-composer-dropdown-button-icon" />
        {labelPrefix ? (
          <span className="node-composer-dropdown-prefix">{labelPrefix}</span>
        ) : null}
        {labelPrefix ? " " : null}
        <span>{selectedOption?.label ?? "选择"}</span>
        <span className="node-composer-dropdown-chevron">⌄</span>
      </button>
      {isOpen ? (
        <div
          aria-label={ariaLabel}
          className="node-composer-dropdown-menu"
          role="listbox"
        >
          {options.map((option) => (
            <button
              aria-selected={option.value === value}
              className="node-composer-dropdown-option"
              data-active={option.value === value}
              key={option.value}
              onClick={() => onSelect(option.value)}
              role="option"
              type="button"
            >
              <span className="node-composer-dropdown-option-icon" />
              <span>{option.label}</span>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function VersionPreviewPanel({
  currentVersionId,
  detailVersionId,
  isSelectingVersion,
  nodeId,
  version,
  onOpenDetails,
  onSelectVersion,
}: {
  currentVersionId: string | null;
  detailVersionId: string | null;
  isSelectingVersion: boolean;
  nodeId: string;
  version: ArtifactVersion | null;
  onOpenDetails: (versionId: string) => void;
  onSelectVersion: (nodeId: string, versionId: string) => void;
}) {
  if (!version) {
    return <p className="property-empty">尚无可预览版本。</p>;
  }
  const isCurrent = version.id === currentVersionId;
  const canSelect =
    version.status === "succeeded" && Boolean(version.asset) && !isCurrent;

  return (
    <div className="property-version-preview" data-status={version.status}>
      <div className="property-version-preview-header">
        <div>
          <strong>v{version.version_no}</strong>
          <span>{isCurrent ? "current" : version.status}</span>
        </div>
        <div className="property-version-actions">
          <button
            onClick={() => openArtifactVersionInNewTab(version)}
            type="button"
          >
            全屏查看
          </button>
          {versionHasCallRecord(version) ? (
            <button onClick={() => onOpenDetails(version.id)} type="button">
              {detailVersionId === version.id ? "收起详情" : "详情"}
            </button>
          ) : null}
          {canSelect ? (
            <button
              disabled={isSelectingVersion}
              onClick={() => onSelectVersion(nodeId, version.id)}
              type="button"
            >
              设为当前
            </button>
          ) : null}
        </div>
      </div>
      <VersionPreviewBody version={version} />
    </div>
  );
}

function VersionDetailPanel({ version }: { version: ArtifactVersion }) {
  const blocks = versionCallRecordBlocks(version);
  return (
    <div className="property-version-detail">
      <dl className="property-list">
        {versionDetailRows(version).map((row) => (
          <div key={row.label}>
            <dt>{row.label}</dt>
            <dd>{row.value}</dd>
          </div>
        ))}
      </dl>
      {blocks.length ? (
        blocks.map((block) => (
          <JsonBlock
            key={block.label}
            label={block.label}
            value={block.value}
          />
        ))
      ) : (
        <p className="property-empty">该版本没有调用记录详情。</p>
      )}
    </div>
  );
}

function VersionPreviewBody({ version }: { version: ArtifactVersion }) {
  if (version.status === "queued" || version.status === "running") {
    return (
      <div className="property-version-progress">
        <span>{Math.max(0, Math.min(100, Math.round(version.progress)))}%</span>
        <div>
          <i style={{ width: `${Math.max(2, Math.min(100, version.progress))}%` }} />
        </div>
      </div>
    );
  }
  if (version.status === "failed" || version.status === "cancelled") {
    return (
      <div className="property-version-error">
        {version.error_message || version.error_code || "运行失败"}
      </div>
    );
  }
  if (!version.asset) {
    return <p className="property-empty">该版本没有素材。</p>;
  }
  if (version.asset.type === "text") {
    return (
      <MarkdownPreview
        value={version.asset.text_content || ""}
        variant="panel"
      />
    );
  }
  if (version.asset.type === "image" && version.asset.access_url) {
    return (
      <img
        alt={`v${version.version_no}`}
        className="property-version-media"
        src={version.asset.access_url}
      />
    );
  }
  if (version.asset.type === "video" && version.asset.access_url) {
    return (
      <video
        className="property-version-media"
        controls
        src={version.asset.access_url}
      />
    );
  }
  return <p className="property-empty">{version.asset.mime || "素材已生成"}</p>;
}

function NodeInputStrip({
  edges,
  node,
  nodes,
  onDeleteInputEdge,
}: {
  edges: MediaEdge[];
  node: MediaNode;
  nodes: MediaNode[];
  onDeleteInputEdge?: (edgeId: string) => void;
}) {
  const upstreamInputs = edges
    .filter((edge) => edge.edge_type === "dependency" && edge.to_node_id === node.id)
    .flatMap((edge) => {
      const inputNode = nodes.find((item) => item.id === edge.from_node_id);
      return inputNode ? [{ edge, node: inputNode }] : [];
    });

  return (
    <div className="property-section property-input-strip">
      <div className="property-section-heading">
        <span>输入</span>
        <small>{upstreamInputs.length} 个依赖</small>
      </div>
      {upstreamInputs.length ? (
        <div className="property-input-chip-list">
          {upstreamInputs.map((input) => (
            <div
              className="property-input-chip"
              data-has-preview={hasInputVisualPreview(input.node)}
              data-type={input.node.node_type}
              key={input.edge.id}
            >
              <InputNodePreview node={input.node} />
              <div>
                <strong>{input.node.title || nodeTypeLabel[input.node.node_type]}</strong>
                <span>{inputSummary(input.node)}</span>
              </div>
              {onDeleteInputEdge ? (
                <button
                  aria-label={`删除 ${input.node.title || "输入"} 依赖`}
                  onClick={() => onDeleteInputEdge(input.edge.id)}
                  type="button"
                >
                  ×
                </button>
              ) : null}
            </div>
          ))}
        </div>
      ) : (
        <p className="property-input-empty">
          拖入依赖连线，或在 Prompt 中输入 @ 引用节点。
        </p>
      )}
    </div>
  );
}

function hasInputVisualPreview(node: MediaNode) {
  return Boolean(
    (node.node_type === "image" || node.node_type === "video") &&
      (node.production_preview?.thumbnail_url ??
        node.thumbnail_url ??
        node.production_preview?.access_url ??
        node.asset_url),
  );
}

function InputNodePreview({ node }: { node: MediaNode }) {
  const previewUrl =
    node.production_preview?.thumbnail_url ??
    node.thumbnail_url ??
    node.production_preview?.access_url ??
    node.asset_url;
  if (
    (node.node_type === "image" || node.node_type === "video") &&
    previewUrl
  ) {
    return (
      <img
        alt={node.title || nodeTypeLabel[node.node_type]}
        className="property-input-thumb"
        src={previewUrl}
      />
    );
  }
  return (
    <span className="property-input-icon" data-type={node.node_type}>
      {nodeTypeLabel[node.node_type].slice(0, 1)}
    </span>
  );
}

function inputSummary(node: MediaNode) {
  if (node.production_preview?.text) {
    return truncateInline(node.production_preview.text, 34);
  }
  if (node.prompt) {
    return truncateInline(node.prompt, 34);
  }
  if (node.reference_pack_preview) {
    return `${node.reference_pack_preview.member_count} 个成员`;
  }
  return nodeTypeLabel[node.node_type];
}

function truncateInline(value: string, maxLength: number) {
  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized.length > maxLength
    ? `${normalized.slice(0, maxLength - 1)}…`
    : normalized;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function PropertyPromptEditor({
  className,
  edges,
  isUpdatingNode,
  node,
  nodes,
  onPromptRefSelect,
  onUpdateNode,
}: {
  className?: string;
  edges: MediaEdge[];
  isUpdatingNode: boolean;
  node: MediaNode;
  nodes: MediaNode[];
  onPromptRefSelect: (
    targetNode: MediaNode,
    refNode: MediaNode,
    prompt: string,
  ) => void;
  onUpdateNode: (nodeId: string, patch: NodePatch) => void;
}) {
  const [promptValue, setPromptValue] = useState(node.prompt);
  const [isPromptFocused, setIsPromptFocused] = useState(false);
  const isRefMenuOpen = isPromptFocused && promptValue.endsWith("@");
  const candidates = candidatePromptRefNodes(node, nodes, edges);
  const candidatesRef = useRef(candidates);
  const promptValueRef = useRef(promptValue);
  const nodePromptRef = useRef(node.prompt);
  const onUpdateNodeRef = useRef(onUpdateNode);

  useEffect(() => {
    candidatesRef.current = candidates;
  }, [candidates]);

  useEffect(() => {
    onUpdateNodeRef.current = onUpdateNode;
  }, [onUpdateNode]);

  useEffect(() => {
    setPromptValue(node.prompt);
    promptValueRef.current = node.prompt;
    nodePromptRef.current = node.prompt;
  }, [node.id, node.prompt]);

  useEffect(() => {
    setIsPromptFocused(false);
  }, [node.id]);

  const commitPrompt = useCallback(
    (prompt: string) => {
      if (prompt === nodePromptRef.current) {
        return;
      }
      onUpdateNodeRef.current(node.id, {
        prompt,
        prompt_refs: buildPromptRefDocument(
          collectPromptRefMentions(prompt, candidatesRef.current),
          candidatesRef.current,
        ),
        prompt_rich: {
          version: 1,
          source: "textarea-at",
          text: prompt,
        },
      });
      nodePromptRef.current = prompt;
    },
    [node.id],
  );

  useEffect(() => {
    return () => {
      commitPrompt(promptValueRef.current);
    };
  }, [commitPrompt]);

  return (
    <label
      className={`property-field property-prompt-editor${
        className ? ` ${className}` : ""
      }`}
    >
      <span>Prompt</span>
      <textarea
        disabled={isUpdatingNode}
        onBlur={(event) => {
          commitPrompt(event.currentTarget.value);
          setIsPromptFocused(false);
        }}
        onChange={(event) => {
          const next = event.currentTarget.value;
          promptValueRef.current = next;
          setPromptValue(next);
        }}
        onFocus={() => setIsPromptFocused(true)}
        placeholder="输入生成文本、画面描述或旁白方向"
        value={promptValue}
      />
      {isRefMenuOpen ? (
        <div className="prompt-ref-menu">
          {candidates.length > 0 ? (
            candidates.map((candidate) => (
              <button
                key={candidate.id}
                onClick={() => {
                  const token = `@${candidate.title || "未命名"}`;
                  const next = promptValue.replace(/@$/, token);
                  promptValueRef.current = next;
                  setPromptValue(next);
                  onPromptRefSelect(node, candidate, next);
                }}
                onMouseDown={(event) => event.preventDefault()}
                type="button"
              >
                <span>{candidate.title}</span>
                <small>{candidate.node_type}</small>
              </button>
            ))
          ) : (
            <span className="prompt-ref-menu-empty">没有可引用节点</span>
          )}
        </div>
      ) : null}
    </label>
  );
}

function EdgePropertyPanel({
  detail,
  onDeleteEdge,
}: {
  detail: NonNullable<ReturnType<typeof getEdgeDetail>>;
  onDeleteEdge?: (edgeId: string) => void;
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
      {onDeleteEdge ? (
        <button
          className="studio-secondary-button property-danger"
          onClick={() => onDeleteEdge(detail.edge.id)}
          type="button"
        >
          删除依赖
        </button>
      ) : null}
    </aside>
  );
}

function ReferencePackMembersSection({
  candidates,
  isLoading,
  isUpdating,
  members,
  onToggleMember,
}: {
  candidates: MediaNode[];
  isLoading: boolean;
  isUpdating: boolean;
  members: MediaNode[];
  onToggleMember: (memberNodeId: string, checked: boolean) => void;
}) {
  return (
    <div className="property-section">
      <p className="studio-section-label">Members</p>
      {isLoading ? (
        <p className="property-empty">正在读取成员。</p>
      ) : members.length ? (
        <div className="reference-pack-member-list">
          {members.map((member, index) => (
            <div className="reference-pack-member-row" key={member.id}>
              <span>{index + 1}</span>
              <NodeChip node={member} />
              <button
                className="studio-secondary-button"
                disabled={isUpdating}
                onClick={() => onToggleMember(member.id, false)}
                type="button"
              >
                移除
              </button>
            </div>
          ))}
        </div>
      ) : (
        <p className="property-empty">暂无成员。</p>
      )}
      <p className="studio-section-label">Candidates</p>
      {candidates.length ? (
        <div className="reference-pack-candidate-list">
          {candidates.map((candidate) => (
            <button
              className="reference-pack-candidate-row"
              disabled={isUpdating}
              key={candidate.id}
              onClick={() => onToggleMember(candidate.id, true)}
              type="button"
            >
              <NodeChip node={candidate} />
              <span>添加</span>
            </button>
          ))}
        </div>
      ) : (
        <p className="property-empty">没有可添加的非 Pack 节点。</p>
      )}
    </div>
  );
}

function NodeChip({ node }: { node: MediaNode }) {
  return (
    <span className="property-node-chip">
      <span>{node.title}</span>
      <small>{nodeTypeLabel[node.node_type]}</small>
    </span>
  );
}

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  return (
    <div className="property-json-block">
      <span>{label}</span>
      <pre>{formatInspectableValue(value)}</pre>
    </div>
  );
}

function formatInspectableValue(value: unknown) {
  if (typeof value === "string") {
    return value;
  }
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return String(value);
  }
}

function durationOptions(capability: ModelCapability | undefined) {
  const durations = capability?.limits.durations_sec;
  if (
    Array.isArray(durations) &&
    durations.every((duration) => typeof duration === "number")
  ) {
    return durations;
  }
  return [4, 5, 8];
}

function durationControlOptions(durations: number[], currentDuration: number) {
  const values = Number.isFinite(currentDuration)
    ? Array.from(new Set([...durations, currentDuration]))
    : durations;
  return values.map((duration) => ({
    value: String(duration),
    label: `${duration}s`,
  }));
}

function PanelHeader({ eyebrow, title }: { eyebrow: string; title: string }) {
  return (
    <div className="property-panel-header">
      <p>{eyebrow}</p>
      <h2>{title}</h2>
    </div>
  );
}

function EditablePanelHeader({
  eyebrow,
  isEditing,
  isUpdating,
  title,
  onCancel,
  onCommit,
  onEdit,
  onTitleChange,
}: {
  eyebrow: string;
  isEditing: boolean;
  isUpdating: boolean;
  title: string;
  onCancel: () => void;
  onCommit: () => void;
  onEdit: () => void;
  onTitleChange: (title: string) => void;
}) {
  return (
    <div className="property-panel-header property-panel-header-editable">
      <p>{eyebrow}</p>
      {isEditing ? (
        <input
          autoFocus
          disabled={isUpdating}
          onBlur={onCommit}
          onChange={(event) => onTitleChange(event.currentTarget.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              onCommit();
            }
            if (event.key === "Escape") {
              event.preventDefault();
              onCancel();
            }
          }}
          value={title}
        />
      ) : (
        <button
          className="property-title-edit-button"
          onDoubleClick={onEdit}
          title="双击编辑标题"
          type="button"
        >
          {title}
        </button>
      )}
    </div>
  );
}
